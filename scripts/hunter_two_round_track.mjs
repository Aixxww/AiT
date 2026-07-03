import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';

function usage() {
  console.error('usage: node scripts/hunter_two_round_track.mjs <round1-raw.json> <round2-raw.json> <out.json>');
  process.exit(2);
}

const [r1Path, r2Path, outPath] = process.argv.slice(2);
if (!r1Path || !r2Path || !outPath) usage();

const r1 = JSON.parse(fs.readFileSync(r1Path, 'utf8'));
const r2 = JSON.parse(fs.readFileSync(r2Path, 'utf8'));

function parseCst(value) {
  const m = String(value).match(/(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2}) CST/);
  if (!m) throw new Error(`cannot parse CST timestamp: ${value}`);
  const [, y, mo, d, h, mi, s] = m.map(Number);
  return Date.UTC(y, mo - 1, d, h - 8, mi, s);
}

function parsePromptTiers(promptPath, rawPath) {
  const fullPath = path.resolve(path.dirname(rawPath), '..', path.basename(promptPath));
  const fallback = path.resolve(promptPath);
  const text = fs.readFileSync(fs.existsSync(fullPath) ? fullPath : fallback, 'utf8');
  const tiers = {};
  const lines = text.split(/\r?\n/);
  let current = null;
  for (const line of lines) {
    const header = line.match(/^####\s+\d+\.\s+([A-Z0-9]+USDT)\s+\[(LONG|SHORT)\]/);
    if (header) {
      current = header[1];
      continue;
    }
    const tier = line.match(/^execution_tier=([A-Z_]+)\s+tier_reason=([^\s]+)/);
    if (current && tier) {
      tiers[current] = { tier: tier[1], reason: tier[2] };
      current = null;
      continue;
    }
    const watch = line.match(/^-\s+([A-Z0-9]+USDT)\s+(LONG|SHORT).*reason=([^\s]+)/);
    if (watch) {
      tiers[watch[1]] = { tier: 'WATCH', reason: watch[3] };
    }
  }
  return tiers;
}

function sidePnl(direction, entry, price) {
  if (!entry || !price) return 0;
  return direction === 'SHORT' ? ((entry - price) / entry) * 100 : ((price - entry) / entry) * 100;
}

function firstTarget(signal) {
  if (signal.targets && signal.targets.length) return signal.targets[0].price;
  if (signal.tp1_price) return signal.tp1_price;
  return 0;
}

function fetchKlines(symbol, startMs, endMs) {
  const url = `https://fapi.binance.com/fapi/v1/klines?symbol=${symbol}&interval=1m&startTime=${startMs}&endTime=${endMs}&limit=1000`;
  const body = execFileSync('curl', ['-fsS', url], { encoding: 'utf8', timeout: 20000 });
  return JSON.parse(body).map((k) => ({
    openTime: k[0],
    open: Number(k[1]),
    high: Number(k[2]),
    low: Number(k[3]),
    close: Number(k[4]),
    volume: Number(k[5]),
  }));
}

function evaluatePath(signal, klines) {
  const direction = signal.direction;
  const entry = signal.price_context?.last || 0;
  const tp = firstTarget(signal);
  const sl = signal.invalidation?.price || 0;
  let firstEvent = 'none';
  let firstEventTime = null;
  let maxFavorable = Number.NEGATIVE_INFINITY;
  let maxAdverse = Number.POSITIVE_INFINITY;
  let endPrice = klines.length ? klines[klines.length - 1].close : 0;

  for (const k of klines) {
    const favorablePrice = direction === 'SHORT' ? k.low : k.high;
    const adversePrice = direction === 'SHORT' ? k.high : k.low;
    maxFavorable = Math.max(maxFavorable, sidePnl(direction, entry, favorablePrice));
    maxAdverse = Math.min(maxAdverse, sidePnl(direction, entry, adversePrice));

    const tpHit = direction === 'SHORT' ? k.low <= tp : k.high >= tp;
    const slHit = direction === 'SHORT' ? k.high >= sl : k.low <= sl;
    if (firstEvent === 'none' && (tpHit || slHit)) {
      if (tpHit && slHit) {
        firstEvent = 'both_same_candle';
      } else {
        firstEvent = tpHit ? 'tp_first' : 'sl_first';
      }
      firstEventTime = new Date(k.openTime).toISOString();
    }
  }

  if (!Number.isFinite(maxFavorable)) maxFavorable = 0;
  if (!Number.isFinite(maxAdverse)) maxAdverse = 0;
  return {
    entry,
    tp,
    sl,
    end_price: endPrice,
    end_pnl_pct: sidePnl(direction, entry, endPrice),
    max_favorable_pct: maxFavorable,
    max_adverse_pct: maxAdverse,
    first_event: firstEvent,
    first_event_time: firstEventTime,
    candle_count: klines.length,
  };
}

const r1PromptTiers = parsePromptTiers(r1.prompt_preview_path, r1Path);
const r2PromptTiers = parsePromptTiers(r2.prompt_preview_path, r2Path);
const r2BySymbol = Object.fromEntries(r2.signals.map((s) => [s.symbol, s]));
const startMs = parseCst(r1.generated_at);
const endMs = parseCst(r2.generated_at);

const tracked = [];
for (const signal of r1.signals) {
  const promptTier = r1PromptTiers[signal.symbol]?.tier || signal.execution_readiness?.tier || 'UNKNOWN';
  const promptReason = r1PromptTiers[signal.symbol]?.reason || '';
  const shouldTrack = promptTier === 'EXECUTABLE' || promptTier === 'REVIEWABLE';
  if (!shouldTrack) continue;
  const klines = fetchKlines(signal.symbol, startMs, endMs);
  tracked.push({
    symbol: signal.symbol,
    direction: signal.direction,
    setup_type: signal.setup_type,
    runtime_tier: signal.execution_readiness?.tier || '',
    prompt_tier: promptTier,
    prompt_reason: promptReason,
    ai_priority: signal.ai_priority,
    timing_score: signal.timing_score,
    risk_score: signal.risk_score,
    liquidity_score: signal.liquidity_score,
    r2_present: Boolean(r2BySymbol[signal.symbol]),
    path: evaluatePath(signal, klines),
  });
}

function groupBySetup(signals, promptTiers) {
  const out = {};
  for (const signal of signals) {
    const setup = signal.setup_type;
    if (!out[setup]) out[setup] = { signals: 0, runtime_open: 0, prompt_open_review: 0, prompt_exec: 0 };
    out[setup].signals += 1;
    const runtimeTier = signal.execution_readiness?.tier || '';
    if (runtimeTier === 'EXECUTABLE' || runtimeTier === 'REVIEWABLE') out[setup].runtime_open += 1;
    if (promptTiers) {
      const promptTier = promptTiers[signal.symbol]?.tier || runtimeTier;
      if (promptTier === 'EXECUTABLE') out[setup].prompt_exec += 1;
      if (promptTier === 'EXECUTABLE' || promptTier === 'REVIEWABLE') out[setup].prompt_open_review += 1;
    }
  }
  return out;
}

const result = {
  generated_at: new Date().toISOString(),
  round1_raw: r1Path,
  round2_raw: r2Path,
  window: {
    round1_generated_at: r1.generated_at,
    round2_generated_at: r2.generated_at,
    start_ms: startMs,
    end_ms: endMs,
    minutes: (endMs - startMs) / 60000,
  },
  round1: {
    snapshot: r1.snapshot,
    opportunity_cover: r1.opportunity_cover,
    prompt_tiers: r1.ai_recognition?.prompt_tier_counts || {},
    by_setup: groupBySetup(r1.signals, r1PromptTiers),
  },
  round2: {
    snapshot: r2.snapshot,
    opportunity_cover: r2.opportunity_cover,
    prompt_tiers: r2.ai_recognition?.prompt_tier_counts || {},
    by_setup: groupBySetup(r2.signals, r2PromptTiers),
  },
  combined_by_setup: (() => {
    const out = {};
    for (const row of [groupBySetup(r1.signals, r1PromptTiers), groupBySetup(r2.signals, r2PromptTiers)]) {
      for (const [setup, stats] of Object.entries(row)) {
        if (!out[setup]) out[setup] = { signals: 0, runtime_open: 0, prompt_open_review: 0, prompt_exec: 0 };
        out[setup].signals += stats.signals;
        out[setup].runtime_open += stats.runtime_open;
        out[setup].prompt_open_review += stats.prompt_open_review;
        out[setup].prompt_exec += stats.prompt_exec;
      }
    }
    return out;
  })(),
  tracked,
};

fs.writeFileSync(outPath, JSON.stringify(result, null, 2));
console.log(outPath);
