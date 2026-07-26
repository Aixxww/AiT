#!/usr/bin/env node

import fs from 'fs';
import path from 'path';
import { execFileSync } from 'child_process';

function usage() {
  console.error('Usage: node scripts/hunter_v7_live_analysis.mjs <report-dir> [interval-label]');
  console.error('  interval-label: round spacing shown in the report, e.g. "10m" (default) or "5m"');
  process.exit(1);
}

// parseGeneratedAt parses the validator's "2006-01-02 15:04:05 MST" stamp as
// machine-local time. Node would misread the bare "CST" abbreviation as US
// Central, so the zone token is dropped: the validator and this script run on
// the same machine.
function parseGeneratedAt(value) {
  const m = String(value || '').match(/^(\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}:\d{2})/);
  if (!m) return 0;
  const ms = new Date(`${m[1]}T${m[2]}`).getTime();
  return Number.isFinite(ms) ? ms : 0;
}

function readJson(file) {
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

function round2(n) {
  return Math.round(n * 100) / 100;
}

function pct(n, digits = 1) {
  return `${n.toFixed(digits)}%`;
}

function fmtSignedPct(n, digits = 2) {
  return `${n >= 0 ? '+' : ''}${n.toFixed(digits)}%`;
}

function parseExpandedSignals(promptText) {
  const out = [];
  for (const line of promptText.split(/\r?\n/)) {
    const idx = line.indexOf('hunter_v7_signal_json:');
    if (idx < 0) continue;
    const raw = line.slice(idx + 'hunter_v7_signal_json:'.length).trim();
    if (!raw.startsWith('{')) continue;
    try {
      out.push(JSON.parse(raw));
    } catch (err) {
      throw new Error(`failed to parse signal json: ${err.message}`);
    }
  }
  return out;
}

function parseOpenReviewCandidates(promptText) {
  const out = [];
  const lines = promptText.split(/\r?\n/);
  let inSection = false;
  for (const line of lines) {
    if (line.startsWith('### Open-review candidates')) {
      inSection = true;
      continue;
    }
    if (line.startsWith('### WATCH candidates')) {
      break;
    }
    if (!inSection) continue;
    if (line.startsWith('execution_tier=')) {
      const tier = line.match(/^execution_tier=([A-Z_]+)/)?.[1] || 'WATCH';
      const reason = line.match(/tier_reason=([^\s]+)/)?.[1] || '';
      continue;
    }
    const jsonMatch = line.match(/hunter_v7_signal_json:\s*(\{.*\})/);
    if (jsonMatch) {
      try {
        const sig = JSON.parse(jsonMatch[1]);
        out.push({ ...sig, prompt_form: 'full' });
      } catch (err) {
        throw new Error(`failed to parse open-review signal json: ${err.message}`);
      }
      continue;
    }
    // Overflow candidates now carry a compact execution JSON instead of a bare
    // summary line. Parse it so PromptOR counts the full open-review surface.
    const compactMatch = line.match(/^-\s+([A-Z0-9]+)\s+(LONG|SHORT)\s+tier=([A-Z_]+)\s+compact_execution_json:?=(\{.*\})\s*$/);
    if (compactMatch) {
      const [, symbol, direction, tier, json] = compactMatch;
      let parsed = {};
      try {
        parsed = JSON.parse(json);
      } catch {
        parsed = {};
      }
      out.push({
        ...parsed,
        symbol,
        direction,
        execution_tier: tier,
        prompt_form: 'compact',
      });
      continue;
    }
    const summaryMatch = line.match(/^-\s+([A-Z0-9]+)\s+(LONG|SHORT)\s+setup=([^\s]+)(?:\s+shape=\S*)?(?:\s+entry_signal=\S*)?\s+ai_priority=([0-9.]+)\s+reason=([^\s]+)\s+\((?:not expanded|compact only); lower priority\)$/);
    if (summaryMatch) {
      const [, symbol, direction, setupType, aiPriority, reason] = summaryMatch;
      out.push({
        symbol,
        direction,
        setup_type: setupType,
        execution_tier: inferTierFromReason(reason),
        tier_reason: reason,
        prompt_form: 'summary',
        ai_priority: Number(aiPriority),
      });
    }
  }
  return out;
}

function inferTierFromReason(reason) {
  const r = String(reason || '').toLowerCase();
  if (r.includes('reviewable')) return 'REVIEWABLE';
  if (r.includes('ready_confirmed') || r.includes('ready')) return 'EXECUTABLE';
  if (r.includes('wait')) return 'WATCH';
  return 'REVIEWABLE';
}

function sumMap(dst, src) {
  for (const [k, v] of Object.entries(src || {})) {
    dst[k] = (dst[k] || 0) + v;
  }
  return dst;
}

function ensureStat(map, key) {
  if (!map[key]) {
    map[key] = {
      rounds: 0,
      universe: 0,
      signals: 0,
      long: 0,
      short: 0,
      exec: 0,
      review: 0,
      watch: 0,
      reject: 0,
      visible: 0,
      tp0: 0,
      tp: 0,
      sl: 0,
      both: 0,
      open_profit: 0,
      open_loss: 0,
      open_flat: 0,
      sum_pnl: 0,
      sum_mfe: 0,
      sum_mae: 0,
    };
  }
  return map[key];
}

function fetchKlines(symbol, startMs, endMs) {
  const url = new URL('https://fapi.binance.com/fapi/v1/klines');
  url.searchParams.set('symbol', symbol);
  url.searchParams.set('interval', '1m');
  url.searchParams.set('startTime', String(startMs));
  url.searchParams.set('endTime', String(endMs));
  url.searchParams.set('limit', '1000');
  const raw = execFileSync('curl', ['-sS', url.toString()], { encoding: 'utf8', maxBuffer: 20 * 1024 * 1024 });
  return JSON.parse(raw);
}

function classifyOutcome(sig, klines) {
  const entry = Number(sig.execution_geometry?.current_price || sig.price_context?.last || 0);
  const tp0 = Number(sig.tp0_price || sig.targets?.[0]?.price || 0);
  const tpTarget = Number((sig.targets && sig.targets.length > 0 ? sig.targets[0].price : 0) || 0);
  const sl = Number(sig.invalidation?.price || 0);
  const dir = String(sig.direction || '').toUpperCase();
  if (!entry || !tp0 || !sl || !dir) {
    return { outcome: 'OPEN_FLAT', pnl: 0, mfe: 0, mae: 0, last: entry };
  }

  let firstHit = null;
  let firstIndex = -1;
  let tp0Hit = false;
  let tpHit = false;
  let slHit = false;
  let mfe = 0;
  let mae = 0;
  let lastClose = entry;
  for (let i = 0; i < klines.length; i++) {
    const k = klines[i];
    const high = Number(k[2]);
    const low = Number(k[3]);
    const close = Number(k[4]);
    lastClose = close;

    const favorable = dir === 'LONG' ? (high / entry - 1) * 100 : (entry / low - 1) * 100;
    const adverse = dir === 'LONG' ? (low / entry - 1) * 100 : (entry / high - 1) * 100;
    mfe = Math.max(mfe, favorable);
    mae = Math.min(mae, adverse);

    const hitTp0 = dir === 'LONG' ? high >= tp0 : low <= tp0;
    const hitTp = tpTarget > 0 && tpTarget !== tp0
      ? (dir === 'LONG' ? high >= tpTarget : low <= tpTarget)
      : false;
    const hitSl = dir === 'LONG' ? low <= sl : high >= sl;

    const hitCount = Number(hitTp0) + Number(hitTp) + Number(hitSl);
    if (hitCount >= 2) {
      return {
        outcome: 'BOTH_SAME_1M',
        pnl: dir === 'LONG' ? (close / entry - 1) * 100 : (entry / close - 1) * 100,
        mfe,
        mae,
        last: close,
      };
    }
    if (!firstHit && hitTp) {
      firstHit = 'TP';
      firstIndex = i;
    }
    if (!firstHit && hitTp0) {
      firstHit = 'TP0';
      firstIndex = i;
    }
    if (!firstHit && hitSl) {
      firstHit = 'SL';
      firstIndex = i;
    }
    if (hitTp) tpHit = true;
    if (hitTp0) tp0Hit = true;
    if (hitSl) slHit = true;

    if (firstHit && firstIndex < i) {
      if (firstHit === 'TP' && slHit) break;
      if (firstHit === 'TP0' && (tpHit || slHit)) break;
      if (firstHit === 'SL' && (tpHit || tp0Hit)) break;
    }
  }

  let pnl = dir === 'LONG' ? (lastClose / entry - 1) * 100 : (entry / lastClose - 1) * 100;
  if (tp0Hit) {
    pnl = Math.max(0, pnl);
  }
  if (tpHit) {
    return { outcome: 'TP', pnl, mfe, mae, last: lastClose };
  }
  if (tp0Hit) {
    return { outcome: 'TP0', pnl, mfe, mae, last: lastClose };
  }
  if (slHit) {
    return { outcome: 'SL', pnl, mfe, mae, last: lastClose };
  }
  if (pnl > 0.05) {
    return { outcome: 'OPEN_PROFIT', pnl, mfe, mae, last: lastClose };
  }
  if (pnl < -0.05) {
    return { outcome: 'OPEN_LOSS', pnl, mfe, mae, last: lastClose };
  }
  return { outcome: 'OPEN_FLAT', pnl, mfe, mae, last: lastClose };
}

function formatTable(rows, headers) {
  const widths = headers.map((h) => h.length);
  for (const row of rows) {
    row.forEach((cell, i) => {
      widths[i] = Math.max(widths[i], String(cell).length);
    });
  }
  const fmtRow = (row) => `| ${row.map((cell, i) => String(cell).padEnd(widths[i])).join(' | ')} |`;
  const sep = `| ${widths.map((w) => '-'.repeat(w)).join(' | ')} |`;
  return [fmtRow(headers), sep, ...rows.map(fmtRow)].join('\n');
}

async function main() {
  const reportDir = process.argv[2];
  if (!reportDir) usage();
  const intervalLabel = process.argv[3] || '10m';
  const intervalText = intervalLabel.endsWith('m') ? `${intervalLabel.slice(0, -1)} 分钟` : intervalLabel;

  const absDir = path.resolve(reportDir);
  const rawFiles = fs.readdirSync(absDir).filter((f) => f.startsWith('hunter-v7-live-validation-raw-') && f.endsWith('.json')).sort();
  if (rawFiles.length < 3) {
    throw new Error(`expected at least 3 raw files in ${absDir}, got ${rawFiles.length}`);
  }

  const rounds = rawFiles.map((file, index) => {
    const rawPath = path.join(absDir, file);
    const raw = readJson(rawPath);
    const promptPath = raw.prompt_preview_path ? path.resolve(path.dirname(path.join(absDir, file)), path.basename(raw.prompt_preview_path)) : null;
    const promptText = fs.readFileSync(promptPath || path.join(absDir, file.replace('hunter-v7-live-validation-raw-', 'hunter-v7-live-prompt-').replace('.json', '.txt')), 'utf8');
    const expanded = parseExpandedSignals(promptText);
    const openReview = parseOpenReviewCandidates(promptText);
    const stat = fs.statSync(rawPath);
    const promptStat = fs.statSync(promptPath);
    // generated_at is authoritative for entry timing: file mtimes are destroyed
    // by any cp/rsync/checkout, which silently zeroes the PnL tracking window.
    const generatedMs = parseGeneratedAt(raw.generated_at);
    return {
      round: index + 1,
      rawPath,
      promptPath,
      raw,
      rawMtimeMs: generatedMs || stat.mtimeMs,
      promptMtimeMs: generatedMs || promptStat.mtimeMs,
      expanded,
      openReview,
    };
  });

  const cutoff = rounds[rounds.length - 1].rawMtimeMs;
  const roundRows = [];
  const setupStats = {};
  const regimeStats = {};
  const outcomeStats = {};
  const tierOutcomeStats = {};
  const tracked = [];

  for (const r of rounds) {
    const signals = r.raw.signals || [];
    const cover = r.raw.opportunity_cover || {};
    const exec = Number(cover.by_execution_tier?.EXECUTABLE || 0);
    const review = Number(cover.by_execution_tier?.REVIEWABLE || 0);
    const watch = Number(cover.by_execution_tier?.WATCH || 0);
    const reject = Number(cover.by_execution_tier?.REJECTED || 0);
    const openReview = exec + review;
    const signalRate = r.raw.snapshot?.universe_count ? (signals.length / r.raw.snapshot.universe_count) * 100 : 0;
    const openRate = signals.length ? (openReview / signals.length) * 100 : 0;
    const fullJson = r.expanded.length;
    const promptOpenReview = r.openReview.length;
    const fullJsonCoverage = openReview ? (fullJson / openReview) * 100 : 0;
    const staleCount = signals.filter((sig) => (sig.risk_tags || []).includes('stale_data_risk')).length;
    const priceAges = signals
      .map((sig) => Number(sig.data_freshness?.price_age_ms || 0))
      .filter((age) => age > 0)
      .sort((a, b) => a - b);
    const priceAgeP50 = priceAges.length ? priceAges[Math.floor(priceAges.length / 2)] / 1000 : 0;

    roundRows.push([
      r.round,
      r.raw.generated_at,
      r.raw.snapshot?.regime || '-',
      r.raw.snapshot?.universe_count || 0,
      signals.length,
      pct(signalRate),
      exec,
      review,
      watch,
      reject,
      openReview,
      pct(openRate),
      fullJson,
      pct(fullJsonCoverage),
      promptOpenReview,
      pct(signals.length ? (staleCount / signals.length) * 100 : 0),
      `${priceAgeP50.toFixed(1)}s`,
      r.raw.snapshot?.rest_errors || 0,
    ]);

    const regime = String(r.raw.snapshot?.regime || 'unknown');
    const reg = ensureStat(regimeStats, regime);
    reg.rounds += 1;
    reg.universe += Number(r.raw.snapshot?.universe_count || 0);
    reg.signals += signals.length;
    reg.long += Number(cover.long_count || 0);
    reg.short += Number(cover.short_count || 0);
    reg.exec += exec;
    reg.review += review;
    reg.watch += watch;
    reg.reject += reject;
    reg.visible += fullJson;

    for (const sig of signals) {
      const s = ensureStat(setupStats, String(sig.setup_type || 'unknown'));
      s.rounds += 1;
      s.universe += Number(r.raw.snapshot?.universe_count || 0);
      s.signals += 1;
      if (sig.direction === 'LONG') s.long += 1;
      if (sig.direction === 'SHORT') s.short += 1;
    }

    for (const sig of r.openReview) {
      const s = ensureStat(setupStats, String(sig.setup_type || 'unknown'));
      const tier = String(sig.execution_tier || 'REVIEWABLE');
      if (tier === 'EXECUTABLE') s.exec += 1;
      else if (tier === 'REVIEWABLE') s.review += 1;
    }

    if (r.round <= 2) {
      for (const sig of r.expanded) {
        const entryMs = r.promptMtimeMs;
        const klines = fetchKlines(sig.symbol, Math.floor(entryMs), Math.floor(cutoff));
        const result = classifyOutcome(sig, klines);
        const entry = Number(sig.execution_geometry?.current_price || sig.price_context?.last || 0);
        const tp0 = Number(sig.tp0_price || sig.targets?.[0]?.price || 0);
        const sl = Number(sig.invalidation?.price || 0);
        const row = {
          round: r.round,
          symbol: sig.symbol,
          direction: sig.direction,
          setup_type: sig.setup_type,
          execution_tier: sig.execution_tier,
          market_regime: sig.market_regime,
          entry,
          tp0,
          invalidation: sl,
          outcome: result.outcome,
          pnl_pct: round2(result.pnl),
          mfe_pct: round2(result.mfe),
          mae_pct: round2(result.mae),
          prompt_path: r.promptPath,
          raw_path: r.rawPath,
        };
        tracked.push(row);
        // Track by setup and, separately, by execution tier. Mixing EXECUTABLE
        // with REVIEWABLE hides that the review pool is an observation set, not
        // a tradable win rate.
        for (const bucket of [
          ensureStat(outcomeStats, sig.setup_type || 'unknown'),
          ensureStat(tierOutcomeStats, String(sig.execution_tier || 'UNKNOWN')),
        ]) {
          bucket.signals += 1;
          bucket.sum_pnl += result.pnl;
          bucket.sum_mfe += result.mfe;
          bucket.sum_mae += result.mae;
          if (result.outcome === 'TP0') bucket.tp0 += 1;
          else if (result.outcome === 'TP') bucket.tp += 1;
          else if (result.outcome === 'SL') bucket.sl += 1;
          else if (result.outcome === 'BOTH_SAME_1M') bucket.both += 1;
          else if (result.outcome === 'OPEN_PROFIT') bucket.open_profit += 1;
          else if (result.outcome === 'OPEN_LOSS') bucket.open_loss += 1;
          else if (result.outcome === 'OPEN_FLAT') bucket.open_flat += 1;
        }
      }
    }
  }

  const totalSignals = rounds.reduce((a, r) => a + (r.raw.signals?.length || 0), 0);
  const totalOpenReview = rounds.reduce((a, r) => {
    const cover = r.raw.opportunity_cover?.by_execution_tier || {};
    return a + Number(cover.EXECUTABLE || 0) + Number(cover.REVIEWABLE || 0);
  }, 0);
  const totalFullJSON = rounds.reduce((a, r) => a + r.expanded.length, 0);
  const trackedFullJSON = rounds.slice(0, 2).reduce((a, r) => a + r.expanded.length, 0);
  const totalPromptOpenReview = rounds.reduce((a, r) => a + r.openReview.length, 0);

  const setupRows = Object.entries(setupStats)
    .sort((a, b) => b[1].signals - a[1].signals)
    .map(([setup, s]) => {
      const openReview = s.exec + s.review;
      return [
        setup,
        s.signals,
        pct(totalSignals ? (s.signals / totalSignals) * 100 : 0),
        s.exec,
        s.review,
        pct(s.signals ? (openReview / s.signals) * 100 : 0),
        pct(s.signals ? (s.exec / s.signals) * 100 : 0),
        pct(s.signals ? (s.review / s.signals) * 100 : 0),
      ];
    });

  const regimeRows = Object.entries(regimeStats)
    .sort((a, b) => b[1].signals - a[1].signals)
    .map(([regime, s]) => {
      const openReview = s.exec + s.review;
      return [
        regime,
        s.rounds,
        s.universe,
        s.signals,
        pct(totalSignals ? (s.signals / totalSignals) * 100 : 0),
        pct(s.universe ? (s.signals / s.universe) * 100 : 0),
        s.exec,
        s.review,
        s.watch,
        s.reject,
        pct(s.signals ? (openReview / s.signals) * 100 : 0),
        pct(s.signals ? (s.exec / s.signals) * 100 : 0),
      ];
    });

  const outcomeRows = Object.entries(outcomeStats)
    .sort((a, b) => b[1].signals - a[1].signals)
    .map(([setup, s]) => {
      const wins = s.tp0 + s.tp + s.open_profit;
      return [
        setup,
        s.signals,
        s.tp0,
        s.tp,
        s.sl,
        s.both,
        s.open_profit,
        s.open_loss,
        s.open_flat,
        pct(s.signals ? (wins / s.signals) * 100 : 0),
        fmtSignedPct(s.signals ? s.sum_pnl / s.signals : 0),
        fmtSignedPct(s.signals ? s.sum_mfe / s.signals : 0),
        fmtSignedPct(s.signals ? s.sum_mae / s.signals : 0),
      ];
    });

  const outcomeRowsFor = (stats) => Object.entries(stats)
    .sort((a, b) => b[1].signals - a[1].signals)
    .map(([key, s]) => {
      const wins = s.tp0 + s.tp + s.open_profit;
      return [
        key, s.signals, s.tp0, s.tp, s.sl, s.both,
        s.open_profit, s.open_loss, s.open_flat,
        pct(s.signals ? (wins / s.signals) * 100 : 0),
        fmtSignedPct(s.signals ? s.sum_pnl / s.signals : 0),
        fmtSignedPct(s.signals ? s.sum_mfe / s.signals : 0),
        fmtSignedPct(s.signals ? s.sum_mae / s.signals : 0),
      ];
    });
  const tierOutcomeRows = outcomeRowsFor(tierOutcomeStats);

  const reportLines = [];
  reportLines.push(`# Hunter v7 三轮 ${intervalText}实时跟踪报告`);
  reportLines.push('');
  reportLines.push(`生成时间：${new Date().toISOString()}`);
  reportLines.push(`数据目录：\`${absDir}\``);
  reportLines.push(`测试口径：每 ${intervalText}调取一轮 Binance USD-M 实时数据，共 3 轮；跟踪前 2 轮 prompt 展开的 EXECUTABLE/REVIEWABLE 候选，以第 3 轮时间为截止，用 Binance 1m K 线判断 TP0/TP/SL/浮盈浮亏。`);
  reportLines.push('');
  reportLines.push('## 1. 轮询概览');
  reportLines.push('');
  reportLines.push(formatTable(roundRows, ['Round', 'Time', 'Regime', 'Universe', 'Signals', 'SignalRate', 'EXEC', 'REVIEW', 'WATCH', 'REJECT', 'OpenReview', 'OpenRate', 'FullJSON', 'FullCover', 'PromptOR', 'StalePct', 'AgeP50', 'REST']));
  reportLines.push('');
  reportLines.push(`- 三轮总信号 ${totalSignals}，Open-review 总计 ${totalOpenReview}。`);
  reportLines.push(`- 三轮 prompt open-review 列表候选 ${totalPromptOpenReview} 个，但完整 JSON 只展开 ${totalFullJSON} 个；前两轮用于盈亏跟踪的完整 JSON 候选为 ${trackedFullJSON} 个。`);
  reportLines.push('');
  reportLines.push('## 2. 行情形态 / 路由统计');
  reportLines.push('');
  reportLines.push(formatTable(regimeRows, ['Regime', 'Rounds', 'Universe', 'Signals', 'SignalShare', 'SignalRate', 'EXEC', 'REVIEW', 'WATCH', 'REJECT', 'OpenRate', 'ExecRate']));
  reportLines.push('');
  reportLines.push(formatTable(setupRows, ['Setup', 'Signals', 'SignalShare', 'EXEC', 'REVIEW', 'OpenRate', 'ExecRate', 'ReviewRate']));
  reportLines.push('');
  reportLines.push('## 3. 盈亏 / TP / SL');
  reportLines.push('');
  if (outcomeRows.length) {
    reportLines.push(formatTable(outcomeRows, ['Setup', 'Samples', 'TP0', 'TP', 'SL', 'Both', 'OpenProfit', 'OpenLoss', 'OpenFlat', 'WinRate', 'AvgPnL', 'AvgMFE', 'AvgMAE']));
    reportLines.push('');
  }
  if (tierOutcomeRows.length) {
    reportLines.push('按执行分层拆分（EXECUTABLE 是可开仓面，REVIEWABLE 是待复核观察池，不能混算胜率）：');
    reportLines.push('');
    reportLines.push(formatTable(tierOutcomeRows, ['Tier', 'Samples', 'TP0', 'TP', 'SL', 'Both', 'OpenProfit', 'OpenLoss', 'OpenFlat', 'WinRate', 'AvgPnL', 'AvgMFE', 'AvgMAE']));
    reportLines.push('');
  }
  if (tracked.length) {
    const trackedRows = tracked
      .sort((a, b) => Math.abs(b.pnl_pct) - Math.abs(a.pnl_pct))
      .map((x) => [
        x.symbol,
        x.round,
        x.direction,
        x.setup_type,
        x.execution_tier,
        x.outcome,
        x.entry.toFixed(8),
        x.tp0.toFixed(8),
        x.invalidation.toFixed(8),
        fmtSignedPct(x.pnl_pct),
        fmtSignedPct(x.mfe_pct),
        fmtSignedPct(x.mae_pct),
      ]);
    reportLines.push(formatTable(trackedRows, ['Symbol', 'Round', 'Dir', 'Setup', 'Tier', 'Outcome', 'Entry', 'TP0', 'SL', 'PnL', 'MFE', 'MAE']));
    reportLines.push('');
  } else {
    reportLines.push('- 无可跟踪样本。');
    reportLines.push('');
  }

  reportLines.push('## 4. 结论');
  reportLines.push('');
  const trend = regimeRows[0] || [];
  if (trend.length) {
    reportLines.push(`- 当前主要 regime 是 ${trend[0]}，路由输出明显集中在这一类行情。`);
  }
  const avgFullCover = totalOpenReview ? (totalFullJSON / totalOpenReview) * 100 : 0;
  const totalExec = rounds.reduce((a, r) => a + Number(r.raw.opportunity_cover?.by_execution_tier?.EXECUTABLE || 0), 0);
  const execRate = totalSignals ? (totalExec / totalSignals) * 100 : 0;
  reportLines.push(`- Prompt 完整 JSON 覆盖率 ${pct(avgFullCover)}（${totalFullJSON}/${totalOpenReview}）。`);
  if (avgFullCover < 60) {
    reportLines.push('- 完整候选覆盖率偏低，高优先级 REVIEWABLE 仍被压缩成摘要，开仓面会被 prompt 截断。');
  } else {
    reportLines.push('- 完整候选覆盖率已经足够，开仓面瓶颈不在 prompt 展开层。');
  }
  reportLines.push(`- EXECUTABLE 合计 ${totalExec}，ExecRate ${pct(execRate)}。`);
  if (totalExec === 0) {
    reportLines.push('- 本轮没有任何 EXECUTABLE：瓶颈在最后一层可执行判定，需要按 tier_reason / risk_tags 逐条复盘是哪一个门禁在拦截。');
  }

  const mdPath = path.join(absDir, `hunter-v7-3round-${intervalLabel}-live-analysis.md`);
  const dataPath = path.join(absDir, `hunter-v7-3round-${intervalLabel}-live-analysis-data.json`);
  fs.writeFileSync(mdPath, reportLines.join('\n'));
  fs.writeFileSync(dataPath, JSON.stringify({
    generated_at: new Date().toISOString(),
    rounds: rounds.map((r) => ({
      round: r.round,
      raw_path: r.rawPath,
      prompt_path: r.promptPath,
      raw: r.raw,
      expanded_count: r.expanded.length,
    })),
    regime_stats: regimeStats,
    setup_stats: setupStats,
    outcome_stats: outcomeStats,
    tracked,
    totals: {
      signals: totalSignals,
      open_review: totalOpenReview,
    full_json: totalFullJSON,
    prompt_open_review: totalPromptOpenReview,
    },
  }, null, 2));

  console.log(`wrote ${mdPath}`);
  console.log(`wrote ${dataPath}`);
}

main().catch((err) => {
  console.error(err.stack || err.message || String(err));
  process.exit(1);
});
