#!/usr/bin/env node

import { execFile, execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { promisify } from 'node:util';

const DEFAULT_SINCE = '2026-08-02 01:50:00';
const DEFAULT_OUT_DIR = 'reports/hunter-v7-latest-binance-replay-20260802';
const DEFAULT_DOC = 'docs/hunter-v7-latest-binance-replay-optimization-20260802.md';
const execFileAsync = promisify(execFile);
const KEY_SETUPS = new Set([
  'alt_ladder_breakdown_short',
  'alt_ladder_momentum_long',
  'displacement_momentum_long',
  'funding_reversal',
  'range_expansion_event',
  'trend_breakout_long',
  'whale_flow_reversal',
]);

const args = parseArgs(process.argv.slice(2));
const since = args.since || DEFAULT_SINCE;
const outDir = args.outDir || DEFAULT_OUT_DIR;
const docPath = args.doc || DEFAULT_DOC;
const proxy = args.proxy || process.env.AIT_BINANCE_PROXY_URL || process.env.HTTPS_PROXY || process.env.HTTP_PROXY || '';

fs.mkdirSync(outDir, { recursive: true });
fs.mkdirSync(path.dirname(docPath), { recursive: true });

function parseArgs(values) {
  const out = {};
  for (let i = 0; i < values.length; i += 1) {
    const v = values[i];
    if (v === '--since') out.since = values[++i];
    else if (v === '--out-dir') out.outDir = values[++i];
    else if (v === '--doc') out.doc = values[++i];
    else if (v === '--proxy') out.proxy = values[++i];
    else if (v === '--help' || v === '-h') {
      console.error('usage: node scripts/hunter_v7_latest_binance_replay.mjs [--since "YYYY-MM-DD HH:mm:ss"] [--out-dir reports/...] [--doc docs/...] [--proxy http://host:port]');
      process.exit(2);
    }
  }
  return out;
}

function shJson(command, argv, options = {}) {
  const raw = execFileSync(command, argv, {
    encoding: 'utf8',
    timeout: options.timeout || 30000,
    maxBuffer: options.maxBuffer || 64 * 1024 * 1024,
  });
  return JSON.parse(raw);
}

function sqliteJson(sql) {
  return shJson('sqlite3', ['-json', 'data/data.db', sql], { maxBuffer: 128 * 1024 * 1024 });
}

function curlJson(url) {
  const argv = ['-fsSL', '--max-time', '25'];
  if (proxy) argv.push('--proxy', proxy);
  argv.push(url);
  return shJson('curl', argv, { timeout: 30000, maxBuffer: 128 * 1024 * 1024 });
}

async function curlJsonAsync(url) {
  const argv = ['-fsSL', '--max-time', '25'];
  if (proxy) argv.push('--proxy', proxy);
  argv.push(url);
  const { stdout } = await execFileAsync('curl', argv, {
    encoding: 'utf8',
    timeout: 30000,
    maxBuffer: 128 * 1024 * 1024,
  });
  return JSON.parse(stdout);
}

function utcMs(value) {
  const m = String(value || '').match(/^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(?:\+00:00|Z)?/);
  if (!m) return 0;
  const [, y, mo, d, h, mi, s, frac = '0'] = m;
  const ms = Number(`${frac}000`.slice(0, 3));
  return Date.UTC(Number(y), Number(mo) - 1, Number(d), Number(h), Number(mi), Number(s), ms);
}

function cst(ms) {
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(new Date(ms)).replace(/\//g, '-');
}

function f(n, digits = 2) {
  if (!Number.isFinite(n)) return 'NA';
  return n.toFixed(digits);
}

function sp(n, digits = 2) {
  if (!Number.isFinite(n)) return 'NA';
  return `${n >= 0 ? '+' : ''}${n.toFixed(digits)}%`;
}

function sidePnl(direction, entry, price) {
  if (!entry || !price) return 0;
  return direction === 'SHORT' ? ((entry - price) / entry) * 100 : ((price - entry) / entry) * 100;
}

function touch(direction, high, low, level, kind) {
  if (!level) return false;
  if (direction === 'SHORT') {
    return kind === 'tp' ? low <= level : high >= level;
  }
  return kind === 'tp' ? high >= level : low <= level;
}

function firstDefined(...values) {
  for (const v of values) {
    const n = Number(v);
    if (Number.isFinite(n) && n > 0) return n;
  }
  return 0;
}

function getReasonList(value) {
  if (!value) return [];
  if (Array.isArray(value)) return value.map(String);
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) ? parsed.map(String) : [];
  } catch {
    return String(value).split(',').map((s) => s.trim()).filter(Boolean);
  }
}

function candidateQuery() {
  const setupList = [...KEY_SETUPS].map((s) => `'${s}'`).join(',');
  return `
select
  id, cycle_number, timestamp, symbol, direction, setup_type, status,
  execution_tier, tier_reason, ai_priority, setup_score, timing_score, risk_score,
  liquidity_score, regime_fit_score, market_regime, reason_codes, risk_tags,
  entry_zone_lower, entry_zone_upper, invalidation_price, target_1,
  tp0_price, tp1_price, tp2_price, oi_delta_1h, oi_delta_4h,
  funding_rate, taker_buy_15m, change_1h, change_4h, change_24h,
  raw_json
from hunter_v7_signal_records
where timestamp >= '${since.replaceAll("'", "''")}'
  and symbol like '%USDT'
  and direction in ('LONG','SHORT')
  and (
    execution_tier in ('EXECUTABLE','REVIEWABLE')
    or setup_type in (${setupList})
  )
order by timestamp, id;
`;
}

function normalize(row) {
  let raw = {};
  try {
    raw = row.raw_json ? JSON.parse(row.raw_json) : {};
  } catch {
    raw = {};
  }
  const entry = firstDefined(raw.price_context?.last, row.entry_zone_lower && row.entry_zone_upper ? (Number(row.entry_zone_lower) + Number(row.entry_zone_upper)) / 2 : 0);
  const tp0 = firstDefined(row.tp0_price, raw.tp0_price, raw.take_profit_plan?.tp0_price, raw.targets?.[0]?.price, row.target_1);
  const tp1 = firstDefined(row.tp1_price, raw.tp1_price, raw.targets?.[1]?.price, raw.targets?.[0]?.price, row.target_1);
  const tp2 = firstDefined(row.tp2_price, raw.tp2_price, raw.targets?.[2]?.price);
  const sl = firstDefined(row.invalidation_price, raw.invalidation?.price);
  return {
    ...row,
    timestamp_ms: utcMs(row.timestamp),
    entry_price: entry,
    tp0_price: tp0,
    tp1_price: tp1,
    tp2_price: tp2,
    stop_price: sl,
    reason_codes_list: getReasonList(row.reason_codes || raw.reason_codes),
    risk_tags_list: getReasonList(row.risk_tags || raw.risk_tags),
    raw: undefined,
  };
}

function dedupe(rows) {
  const byBucket = new Map();
  const all = [];
  for (const row of rows) {
    const bucket = Math.floor(row.timestamp_ms / (30 * 60 * 1000));
    const key = `${bucket}|${row.symbol}|${row.direction}|${row.setup_type}`;
    const existing = byBucket.get(key);
    if (!existing) {
      const copy = { ...row, duplicate_ids: [] };
      byBucket.set(key, copy);
      all.push(copy);
      continue;
    }
    existing.duplicate_ids.push(row.id);
    const tierRank = tierValue(row.execution_tier);
    if (tierRank > tierValue(existing.execution_tier)) {
      Object.assign(existing, row, { duplicate_ids: existing.duplicate_ids });
    }
  }
  return all.sort((a, b) => a.timestamp_ms - b.timestamp_ms || a.id - b.id);
}

function tierValue(tier) {
  if (tier === 'EXECUTABLE') return 3;
  if (tier === 'REVIEWABLE') return 2;
  if (tier === 'WATCH') return 1;
  return 0;
}

async function fetchKlines(symbol, startMs, endMs) {
  const rows = [];
  let cursor = startMs;
  while (cursor < endMs) {
    const url = new URL('https://fapi.binance.com/fapi/v1/klines');
    url.searchParams.set('symbol', symbol);
    url.searchParams.set('interval', '1m');
    url.searchParams.set('startTime', String(cursor));
    url.searchParams.set('endTime', String(endMs));
    url.searchParams.set('limit', '1000');
    const chunk = await curlJsonAsync(url.toString());
    if (!Array.isArray(chunk) || chunk.length === 0) break;
    for (const k of chunk) {
      rows.push({
        open_time: Number(k[0]),
        open: Number(k[1]),
        high: Number(k[2]),
        low: Number(k[3]),
        close: Number(k[4]),
        volume: Number(k[5]),
      });
    }
    const next = Number(chunk[chunk.length - 1][0]) + 60_000;
    if (next <= cursor || chunk.length < 1000) break;
    cursor = next;
  }
  return rows;
}

function fetchCurrentPrices(symbols) {
  const out = new Map();
  const all = curlJson('https://fapi.binance.com/fapi/v1/ticker/price');
  const wanted = new Set(symbols);
  if (Array.isArray(all)) {
    for (const row of all) {
      if (wanted.has(row.symbol)) out.set(row.symbol, Number(row.price));
    }
  }
  return out;
}

async function mapLimit(values, limit, fn) {
  const out = new Array(values.length);
  let next = 0;
  async function worker() {
    for (;;) {
      const i = next;
      next += 1;
      if (i >= values.length) return;
      out[i] = await fn(values[i], i);
    }
  }
  await Promise.all(Array.from({ length: Math.min(limit, values.length) }, worker));
  return out;
}

function analysePath(row, klines, currentPrice, serverTime) {
  const direction = row.direction;
  const entry = row.entry_price;
  const horizons = {};
  for (const minutes of [15, 30, 60]) {
    const until = row.timestamp_ms + minutes * 60_000;
    const slice = klines.filter((k) => k.open_time <= until);
    const k = slice.length ? slice[slice.length - 1] : null;
    horizons[`${minutes}m`] = k ? sidePnl(direction, entry, k.close) : null;
  }
  const currentPnl = sidePnl(direction, entry, currentPrice || (klines.at(-1)?.close || 0));
  const full = klines.filter((k) => k.open_time >= row.timestamp_ms && k.open_time <= serverTime);
  let mfe = 0;
  let mae = 0;
  let firstEvent = 'NONE';
  let firstEventTime = null;
  let anyTp0 = false;
  let anyTp1 = false;
  let anyTp2 = false;
  let anySl = false;
  let tp0BeforeSl = false;
  let slBeforeTp0 = false;
  let maxFavorableTime = null;
  let maxAdverseTime = null;

  for (const k of full) {
    const favorablePrice = direction === 'SHORT' ? k.low : k.high;
    const adversePrice = direction === 'SHORT' ? k.high : k.low;
    const fav = sidePnl(direction, entry, favorablePrice);
    const adv = sidePnl(direction, entry, adversePrice);
    if (fav > mfe) {
      mfe = fav;
      maxFavorableTime = k.open_time;
    }
    if (adv < mae) {
      mae = adv;
      maxAdverseTime = k.open_time;
    }

    const hitTp0 = touch(direction, k.high, k.low, row.tp0_price, 'tp');
    const hitTp1 = touch(direction, k.high, k.low, row.tp1_price, 'tp');
    const hitTp2 = touch(direction, k.high, k.low, row.tp2_price, 'tp');
    const hitSl = touch(direction, k.high, k.low, row.stop_price, 'sl');
    anyTp0 ||= hitTp0;
    anyTp1 ||= hitTp1;
    anyTp2 ||= hitTp2;
    anySl ||= hitSl;

    if (firstEvent === 'NONE' && (hitTp0 || hitTp1 || hitTp2 || hitSl)) {
      const tpHit = hitTp0 || hitTp1 || hitTp2;
      if (tpHit && hitSl) firstEvent = 'TP_SL_SAME_1M';
      else if (hitTp2) firstEvent = 'TP2_FIRST';
      else if (hitTp1) firstEvent = 'TP1_FIRST';
      else if (hitTp0) firstEvent = 'TP0_FIRST';
      else firstEvent = 'SL_FIRST';
      firstEventTime = k.open_time;
    }
    if (!tp0BeforeSl && hitTp0 && !anySl) tp0BeforeSl = true;
    if (!slBeforeTp0 && hitSl && !anyTp0) slBeforeTp0 = true;
  }

  let outcome = 'ACTIVE_FLAT';
  if (firstEvent === 'SL_FIRST') outcome = 'STOP_FIRST';
  else if (firstEvent === 'TP_SL_SAME_1M') outcome = 'AMBIGUOUS_TP_SL';
  else if (firstEvent === 'TP2_FIRST' || anyTp2) outcome = 'WIN_TP2';
  else if (firstEvent === 'TP1_FIRST' || anyTp1) outcome = 'WIN_TP1';
  else if (firstEvent === 'TP0_FIRST' || anyTp0) outcome = anySl ? 'PROTECTED_AFTER_TP0' : 'WIN_TP0_ACTIVE';
  else if (currentPnl > 0.05) outcome = 'ACTIVE_PROFIT';
  else if (currentPnl < -0.05) outcome = 'ACTIVE_LOSS';

  return {
    candle_count: full.length,
    horizon_pnl_pct: horizons,
    current_price: currentPrice,
    current_pnl_pct: currentPnl,
    mfe_pct: mfe,
    mae_pct: mae,
    mfe_time_cst: maxFavorableTime ? cst(maxFavorableTime) : null,
    mae_time_cst: maxAdverseTime ? cst(maxAdverseTime) : null,
    first_event: firstEvent,
    first_event_time_cst: firstEventTime ? cst(firstEventTime) : null,
    any_tp0: anyTp0,
    any_tp1: anyTp1,
    any_tp2: anyTp2,
    any_sl: anySl,
    tp0_before_sl: tp0BeforeSl,
    sl_before_tp0: slBeforeTp0,
    outcome,
  };
}

function initAgg() {
  return {
    n: 0,
    exec: 0,
    review: 0,
    watch_or_blocked: 0,
    positive_15m: 0,
    positive_30m: 0,
    positive_1h: 0,
    positive_current: 0,
    tp0_before_sl: 0,
    any_tp0: 0,
    tp1_or_tp2: 0,
    stop_first: 0,
    protected: 0,
    active_loss: 0,
    ambiguous: 0,
    duplicate_rows: 0,
    sum_15m: 0,
    sum_30m: 0,
    sum_1h: 0,
    sum_current: 0,
    sum_mfe: 0,
    sum_mae: 0,
  };
}

function addAgg(agg, row) {
  const p = row.path;
  agg.n += 1;
  if (row.execution_tier === 'EXECUTABLE') agg.exec += 1;
  else if (row.execution_tier === 'REVIEWABLE') agg.review += 1;
  else agg.watch_or_blocked += 1;
  if ((p.horizon_pnl_pct['15m'] || 0) > 0) agg.positive_15m += 1;
  if ((p.horizon_pnl_pct['30m'] || 0) > 0) agg.positive_30m += 1;
  if ((p.horizon_pnl_pct['60m'] || 0) > 0) agg.positive_1h += 1;
  if (p.current_pnl_pct > 0) agg.positive_current += 1;
  if (p.tp0_before_sl) agg.tp0_before_sl += 1;
  if (p.any_tp0) agg.any_tp0 += 1;
  if (p.any_tp1 || p.any_tp2) agg.tp1_or_tp2 += 1;
  if (p.outcome === 'STOP_FIRST') agg.stop_first += 1;
  if (p.outcome === 'PROTECTED_AFTER_TP0') agg.protected += 1;
  if (p.outcome === 'ACTIVE_LOSS') agg.active_loss += 1;
  if (p.outcome === 'AMBIGUOUS_TP_SL') agg.ambiguous += 1;
  agg.duplicate_rows += row.duplicate_ids?.length || 0;
  agg.sum_15m += p.horizon_pnl_pct['15m'] || 0;
  agg.sum_30m += p.horizon_pnl_pct['30m'] || 0;
  agg.sum_1h += p.horizon_pnl_pct['60m'] || 0;
  agg.sum_current += p.current_pnl_pct || 0;
  agg.sum_mfe += p.mfe_pct || 0;
  agg.sum_mae += p.mae_pct || 0;
}

function aggregate(rows, predicate = () => true) {
  const bySetup = {};
  const byTier = {};
  const total = initAgg();
  for (const row of rows.filter(predicate)) {
    addAgg(total, row);
    bySetup[row.setup_type] ||= initAgg();
    addAgg(bySetup[row.setup_type], row);
    const tier = row.execution_tier || 'BLOCKED';
    byTier[tier] ||= initAgg();
    addAgg(byTier[tier], row);
  }
  return { total, by_setup: bySetup, by_tier: byTier };
}

function aggLine(name, agg) {
  const n = agg.n || 1;
  return `| ${name} | ${agg.n} | ${agg.exec}/${agg.review}/${agg.watch_or_blocked} | ${f((agg.positive_15m / n) * 100, 1)}% | ${f((agg.positive_30m / n) * 100, 1)}% | ${f((agg.positive_1h / n) * 100, 1)}% | ${f((agg.positive_current / n) * 100, 1)}% | ${f((agg.tp0_before_sl / n) * 100, 1)}% | ${f((agg.stop_first / n) * 100, 1)}% | ${sp(agg.sum_current / n)} | ${sp(agg.sum_mfe / n)} | ${sp(agg.sum_mae / n)} |`;
}

function csvValue(value) {
  const s = String(value ?? '');
  return /[",\n]/.test(s) ? `"${s.replaceAll('"', '""')}"` : s;
}

function toCsv(rows) {
  const headers = [
    'id', 'cycle_number', 'timestamp_utc', 'timestamp_cst', 'symbol', 'direction', 'setup_type',
    'execution_tier', 'tier_reason', 'entry_price', 'current_price', 'pnl_15m_pct', 'pnl_30m_pct',
    'pnl_1h_pct', 'current_pnl_pct', 'mfe_pct', 'mae_pct', 'first_event', 'first_event_time_cst',
    'outcome', 'tp0_price', 'tp1_price', 'tp2_price', 'stop_price', 'oi_delta_1h', 'oi_delta_4h',
    'taker_buy_15m', 'funding_rate', 'reason_codes', 'risk_tags', 'duplicate_ids',
  ];
  const lines = [headers.join(',')];
  for (const row of rows) {
    const p = row.path;
    lines.push(headers.map((h) => {
      const value = {
        id: row.id,
        cycle_number: row.cycle_number,
        timestamp_utc: row.timestamp,
        timestamp_cst: cst(row.timestamp_ms),
        symbol: row.symbol,
        direction: row.direction,
        setup_type: row.setup_type,
        execution_tier: row.execution_tier || 'BLOCKED',
        tier_reason: row.tier_reason || '',
        entry_price: row.entry_price,
        current_price: p.current_price,
        pnl_15m_pct: p.horizon_pnl_pct['15m'],
        pnl_30m_pct: p.horizon_pnl_pct['30m'],
        pnl_1h_pct: p.horizon_pnl_pct['60m'],
        current_pnl_pct: p.current_pnl_pct,
        mfe_pct: p.mfe_pct,
        mae_pct: p.mae_pct,
        first_event: p.first_event,
        first_event_time_cst: p.first_event_time_cst || '',
        outcome: p.outcome,
        tp0_price: row.tp0_price,
        tp1_price: row.tp1_price,
        tp2_price: row.tp2_price,
        stop_price: row.stop_price,
        oi_delta_1h: row.oi_delta_1h,
        oi_delta_4h: row.oi_delta_4h,
        taker_buy_15m: row.taker_buy_15m,
        funding_rate: row.funding_rate,
        reason_codes: row.reason_codes_list.join('|'),
        risk_tags: row.risk_tags_list.join('|'),
        duplicate_ids: (row.duplicate_ids || []).join('|'),
      }[h];
      return csvValue(value);
    }).join(','));
  }
  return `${lines.join('\n')}\n`;
}

function topRows(rows, filter, limit = 12) {
  return rows.filter(filter).slice().sort((a, b) => b.path.current_pnl_pct - a.path.current_pnl_pct).slice(0, limit);
}

function rowLine(row) {
  const p = row.path;
  return `| ${row.symbol} | ${row.direction} | ${row.setup_type} | ${row.execution_tier || 'BLOCKED'} | ${sp(p.horizon_pnl_pct['15m'] || 0)} | ${sp(p.horizon_pnl_pct['30m'] || 0)} | ${sp(p.horizon_pnl_pct['60m'] || 0)} | ${sp(p.current_pnl_pct)} | ${sp(p.mfe_pct)} / ${sp(p.mae_pct)} | ${p.outcome} | ${p.first_event_time_cst || '-'} |`;
}

function buildRecommendations(rows, primaryAgg, allAgg) {
  const recs = [];
  const setup = allAgg.by_setup;
  const altLong = setup.alt_ladder_momentum_long;
  if (altLong) {
    recs.push('`alt_ladder_momentum_long`：保持“中段 + OI 正增 + taker > 0.52”作为主通道，但把 `alt_ladder_stage_late + high_volatility/funding_elevated` 组合降为 REVIEW/WATCH，并要求 TP0 触发后立即保本；本次样本里该形态早段有止盈能力，但追高样本 MFE 与 MAE 摆动过大。');
  }
  const range = setup.range_expansion_event;
  if (range) {
    recs.push('`range_expansion_event`：LONG 必须增加“反抽失败未出现”的二次确认：高位 entry_zone_position、1h OI 转负或 reason 含 `short_covering_not_new_long_build` 时，不进入主开仓率；只有 5m 回踩 VWAP/EMA20 后重新放量站回，才升到 REVIEWABLE。');
  }
  const whale = setup.whale_flow_reversal;
  if (whale) {
    recs.push('`whale_flow_reversal`：当前逻辑对 taker 不足的拦截总体合理，但需要把 `OI 快速增加 + 价格未充分标记 + funding_not_crowded` 作为“错失机会复核池”；若 15m/30m 后价格仍在入场带上方且 taker 回到 0.54-0.56，再允许二次确认升档。');
  }
  const short = setup.alt_ladder_breakdown_short;
  if (short) {
    recs.push('`alt_ladder_breakdown_short`：跨轮二次确认应以“下一轮低点下移/5m EMA20 下压/买盘 taker 未反包”为核心；若首轮只是挤压后的短线回撤，不应直接开空。当前复盘应把 BLOCKED/WATCH 的顺向下跌标的纳入 missed-opportunity 队列，而不是放宽主开仓门槛。');
  }
  const funding = setup.funding_reversal;
  if (funding) {
    recs.push('`funding_reversal`：继续从主开仓率拆桶，单独做事件型观察。只有 funding 极值、OI 与价格反向、taker 已转向三项同时满足时才进入 REVIEW；否则仅参与风控解释，不参与执行候选。');
  }
  if (primaryAgg.total.n > 0 && primaryAgg.total.stop_first > 0) {
    recs.push('执行提示词：开仓指令里加入“TP0 距离小于 0.45R 或 stop_distance > TP0_distance * 2 时禁止市价追入”，优先等待回踩入场；这能减少先触发 SL 的单边追高样本。');
  }
  recs.push('统计口径：`PROTECTED_AFTER_TP0` 计为风控有效，不计入亏损止损；但若同形态连续出现 TP0 后快速回撤，策略提示词应从“可追随”改成“只做 TP0 快进快出”。');
  recs.push('数据质量：上一轮 validator 有 universe coverage 低的问题，实盘开仓率评估必须保留 valid_rounds 门槛；低覆盖轮只能用于形态复盘，不能用于放宽执行层阈值。');
  return recs;
}

function renderReport(result) {
  const rows = result.rows;
  const primaryRows = rows.filter((r) => r.execution_tier === 'EXECUTABLE' || r.execution_tier === 'REVIEWABLE');
  const watchRows = rows.filter((r) => r.execution_tier !== 'EXECUTABLE' && r.execution_tier !== 'REVIEWABLE');
  const primaryAgg = result.aggregate.primary;
  const allAgg = result.aggregate.all;
  const pathWinners = rows.filter((r) => r.path.outcome.startsWith('WIN_')).sort((a, b) => b.path.mfe_pct - a.path.mfe_pct).slice(0, 10);
  const losers = rows.filter((r) => r.path.current_pnl_pct < 0).sort((a, b) => a.path.current_pnl_pct - b.path.current_pnl_pct).slice(0, 10);
  const stopRows = rows.filter((r) => r.path.outcome === 'STOP_FIRST' || r.path.outcome === 'AMBIGUOUS_TP_SL');
  const protectedRows = rows.filter((r) => r.path.outcome === 'PROTECTED_AFTER_TP0');
  const repairedAfterStop = stopRows.filter((r) => r.path.current_pnl_pct > 0).sort((a, b) => b.path.current_pnl_pct - a.path.current_pnl_pct).slice(0, 8);
  const missedRows = watchRows.filter((r) => r.path.tp0_before_sl || r.path.current_pnl_pct > 0.6).sort((a, b) => b.path.current_pnl_pct - a.path.current_pnl_pct).slice(0, 12);
  const primaryFullWin = primaryRows.filter((r) => r.path.outcome === 'WIN_TP1' || r.path.outcome === 'WIN_TP2').length;
  const primaryTp0OrProtected = primaryRows.filter((r) => r.path.tp0_before_sl).length;
  const primaryProtected = primaryRows.filter((r) => r.path.outcome === 'PROTECTED_AFTER_TP0').length;
  const primaryStopFirst = primaryRows.filter((r) => r.path.outcome === 'STOP_FIRST').length;

  const lines = [];
  lines.push('# Hunter v7 最新币安合约信号复盘与优化方案');
  lines.push('');
  lines.push(`- 生成时间：${cst(result.generated_at_ms)} CST`);
  lines.push(`- 数据源：Binance USDS-M Futures public REST，1m K 线 + 当前 ticker，未执行任何交易动作。`);
  lines.push(`- 复盘信号：数据库 \`hunter_v7_signal_records\` 自 ${since} UTC 起的执行层信号，以及核心形态 WATCH/BLOCKED 候选。`);
  lines.push(`- 去重口径：同一 30 分钟内相同 \`symbol + direction + setup_type\` 只保留最高执行层记录，重复轮次只计入 duplicate_rows。`);
  lines.push(`- 当前币安服务器时间：${cst(result.binance_server_time_ms)} CST。`);
  lines.push('');
  lines.push('## 结论');
  lines.push('');
  lines.push(`1. 执行层样本 ${primaryAgg.total.n} 个，15M/30M/1H 方向正收益率分别为 ${f((primaryAgg.total.positive_15m / Math.max(primaryAgg.total.n, 1)) * 100, 1)}%、${f((primaryAgg.total.positive_30m / Math.max(primaryAgg.total.n, 1)) * 100, 1)}%、${f((primaryAgg.total.positive_1h / Math.max(primaryAgg.total.n, 1)) * 100, 1)}%；当前正收益率 ${f((primaryAgg.total.positive_current / Math.max(primaryAgg.total.n, 1)) * 100, 1)}%，平均当前 PnL ${sp(primaryAgg.total.sum_current / Math.max(primaryAgg.total.n, 1))}。`);
  lines.push(`2. 按真实交易路径看，执行层 TP0 先于 SL 的有效保护/止盈率为 ${f((primaryTp0OrProtected / Math.max(primaryRows.length, 1)) * 100, 1)}%（${primaryTp0OrProtected}/${primaryRows.length}），其中 TP1/TP2 完整止盈 ${primaryFullWin} 个，TP0 后保护 ${primaryProtected} 个，先打损 ${primaryStopFirst} 个。`);
  lines.push(`3. 全形态观察样本 ${allAgg.total.n} 个，TP0 先于止损触发率 ${f((allAgg.total.tp0_before_sl / Math.max(allAgg.total.n, 1)) * 100, 1)}%，先止损率 ${f((allAgg.total.stop_first / Math.max(allAgg.total.n, 1)) * 100, 1)}%；说明筛选层仍能发现波动机会，但主开仓层必须继续过滤追高、反抽失败和先扫损后修复。`);
  lines.push(`4. 当前价格一致性不能等同于实盘胜率：本次有 ${repairedAfterStop.length} 个样本先触发 SL 后才转为当前顺向，提示词应要求“等待回踩确认/二次确认”而不是扩大市价追入。`);
  lines.push('');
  lines.push('## 按形态统计');
  lines.push('');
  lines.push('| 形态 | 样本 | EXEC/REVIEW/WATCH+ | 15M正向 | 30M正向 | 1H正向 | 当前正向 | TP0先于SL | SL先触发 | 当前均值 | MFE均值 | MAE均值 |');
  lines.push('| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |');
  for (const [name, agg] of Object.entries(allAgg.by_setup).sort((a, b) => b[1].n - a[1].n)) {
    lines.push(aggLine(name, agg));
  }
  lines.push('');
  lines.push('## 执行层统计');
  lines.push('');
  lines.push('| 分组 | 样本 | EXEC/REVIEW/WATCH+ | 15M正向 | 30M正向 | 1H正向 | 当前正向 | TP0先于SL | SL先触发 | 当前均值 | MFE均值 | MAE均值 |');
  lines.push('| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |');
  lines.push(aggLine('EXECUTABLE+REVIEWABLE', primaryAgg.total));
  for (const [name, agg] of Object.entries(primaryAgg.by_setup).sort((a, b) => b[1].n - a[1].n)) {
    lines.push(aggLine(name, agg));
  }
  lines.push('');
  lines.push('## 代表性标的');
  lines.push('');
  lines.push('### 有效止盈样本');
  lines.push('');
  lines.push('| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |');
  lines.push('| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |');
  for (const row of pathWinners) lines.push(rowLine(row));
  lines.push('');
  lines.push('### 逆向/亏损样本');
  lines.push('');
  lines.push('| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |');
  lines.push('| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |');
  for (const row of losers) lines.push(rowLine(row));
  lines.push('');
  if (stopRows.length) {
    lines.push('### 先止损或同 K 歧义样本');
    lines.push('');
    lines.push('| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |');
    lines.push('| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |');
    for (const row of stopRows.slice(0, 12)) lines.push(rowLine(row));
    lines.push('');
  }
  if (repairedAfterStop.length) {
    lines.push('### 先打损后行情修复样本');
    lines.push('');
    lines.push('| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |');
    lines.push('| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |');
    for (const row of repairedAfterStop) lines.push(rowLine(row));
    lines.push('');
  }
  if (protectedRows.length) {
    lines.push('### TP0 后保护样本');
    lines.push('');
    lines.push('| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |');
    lines.push('| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |');
    for (const row of protectedRows.slice(0, 12)) lines.push(rowLine(row));
    lines.push('');
  }
  if (missedRows.length) {
    lines.push('### WATCH/BLOCKED 错失机会复核池');
    lines.push('');
    lines.push('| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |');
    lines.push('| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |');
    for (const row of missedRows) lines.push(rowLine(row));
    lines.push('');
  }
  lines.push('## 可实施优化方案');
  lines.push('');
  buildRecommendations(rows, primaryAgg, allAgg).forEach((rec, i) => lines.push(`${i + 1}. ${rec}`));
  lines.push('');
  lines.push('## 落地优先级');
  lines.push('');
  lines.push('1. P0：在 `provider/local/hunter_v7_mod_range_expansion_event.go` 把 LONG 的高位 + OI 转负/非新多建仓改为硬降档；提示词同步要求“反抽失败未出现 + 回踩后重新放量站回”。');
  lines.push('2. P0：在 `kernel/hunter_v7_prompt_doctrine.go` 或提示词 payload 中，把开仓许可改成双阈值：`entry_zone_position <= 72` 且 `TP0_distance >= 0.45R`；不满足时只能 REVIEW/WATCH。');
  lines.push('3. P1：在 `provider/local/hunter_v7_mod_alt_ladder.go` 和策略提示词中，对 `alt_ladder_stage_late + high_volatility/funding_elevated` 降档，并把 TP0 后保本写成强制动作。');
  lines.push('4. P1：在 `provider/local/hunter_v7_mod_whale_flow.go` 增加二次确认池：首轮 taker 不足但 OI 明显累积的样本，下一轮 taker 回升且价格未跌破入场带再升档。');
  lines.push('5. P2：在 `cmd/hunter_v7_validate` 和长期 outcome 表中增加 `direction_accuracy_15m/30m/1h/current`、`tp0_before_sl_rate`、`protected_after_tp0_rate`，让开仓率优化不和盈利质量混在一个指标里。');
  lines.push('');
  lines.push('## 附件');
  lines.push('');
  lines.push(`- 明细 JSON：${result.output_json}`);
  lines.push(`- 明细 CSV：${result.output_csv}`);
  lines.push('');
  return `${lines.join('\n')}\n`;
}

async function main() {
  const server = curlJson('https://fapi.binance.com/fapi/v1/time');
  const serverTime = Number(server.serverTime);
  const rawRows = sqliteJson(candidateQuery()).map(normalize).filter((r) => r.timestamp_ms > 0 && r.entry_price > 0 && r.stop_price > 0);
  const rows = dedupe(rawRows);
  const currentPrices = fetchCurrentPrices([...new Set(rows.map((r) => r.symbol))]);

  const evaluated = await mapLimit(rows, 6, async (row) => {
    const klines = await fetchKlines(row.symbol, row.timestamp_ms, serverTime);
    return {
      ...row,
      path: analysePath(row, klines, currentPrices.get(row.symbol), serverTime),
    };
  });

  const generatedAt = Date.now();
  const jsonPath = path.join(outDir, `hunter-v7-latest-binance-replay-${new Date(generatedAt).toISOString().replace(/[-:]/g, '').slice(0, 15)}.json`);
  const csvPath = jsonPath.replace(/\.json$/, '.csv');
  const result = {
    generated_at_ms: generatedAt,
    binance_server_time_ms: serverTime,
    since_utc: since,
    source_rows: rawRows.length,
    deduped_rows: evaluated.length,
    proxy_used: Boolean(proxy),
    output_json: jsonPath,
    output_csv: csvPath,
    aggregate: {
      all: aggregate(evaluated),
      primary: aggregate(evaluated, (r) => r.execution_tier === 'EXECUTABLE' || r.execution_tier === 'REVIEWABLE'),
    },
    rows: evaluated,
  };

  fs.writeFileSync(jsonPath, JSON.stringify(result, null, 2));
  fs.writeFileSync(csvPath, toCsv(evaluated));
  fs.writeFileSync(docPath, renderReport(result));

  console.log(JSON.stringify({
    doc: docPath,
    json: jsonPath,
    csv: csvPath,
    source_rows: result.source_rows,
    deduped_rows: result.deduped_rows,
    primary_rows: result.aggregate.primary.total.n,
  }, null, 2));
}

main().catch((err) => {
  console.error(err?.stack || err?.message || String(err));
  process.exit(1);
});
