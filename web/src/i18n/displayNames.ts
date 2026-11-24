/**
 * One display-name map for every backend code the UI shows (issue #81).
 *
 * Rule: no raw snake_case code reaches the screen. Components call
 * `displayName(kind, code, lang)` (or the typed helpers below) instead of
 * rendering `setup_type`, `market_regime`, tiers, actions, sides,
 * `blocked_reason_code`, reason / risk / confirmation tags or source types.
 *
 * Sources of truth for the finite sets (kept in sync by
 * displayNames.test.ts, which reads the Go files):
 *   setup types     provider/local/hunter_v7_types.go, hunter_v7_t2_setups.go
 *   regimes         provider/local/hunter_v7_types.go (V7MarketRegimes)
 *   blocked reasons kernel/engine_analysis.go (hunterV7BlockedReasonCodes)
 *   tags            provider/local/hunter_v7_tag_catalog*.go
 *
 * Tags (≈470) are rendered by phrase overrides first, then by a word-level
 * dictionary, so a new backend tag still renders readable text in both
 * languages. Market acronyms (OI, VWAP, EMA20, RSI, RR, LSR, BB, ATR, MMS)
 * stay as acronyms in both languages, as on Binance's zh UI.
 *
 * Adding a language later (ko/ja): add a key to `Pair` and fill each map.
 */
import type { Language } from './translations'

type Pair = Record<Language, string>
const p = (zh: string, en: string): Pair => ({ zh, en })

// ── Setup types: base name (direction suffix stripped) ──────────────────────
const SETUP_BASE: Record<string, Pair> = {
  // Hunter v7 router setups
  accumulation_breakout: p('吸筹突破', 'Accumulation Breakout'),
  accumulation_watch: p('吸筹观察', 'Accumulation Watch'),
  alt_ladder_momentum: p('山寨阶梯动量', 'Alt Ladder Momentum'),
  alt_ladder_breakdown: p('山寨阶梯破位', 'Alt Ladder Breakdown'),
  breakdown_momentum: p('破位动量', 'Breakdown Momentum'),
  displacement_momentum: p('位移动量', 'Displacement Momentum'),
  distribution: p('高位派发', 'Distribution'),
  funding_reversal: p('资金费率反转', 'Funding Reversal'),
  intraday_scalp: p('日内短线', 'Intraday Scalp'),
  leader_momentum: p('龙头动量', 'Leader Momentum'),
  long_squeeze: p('多头挤压', 'Long Squeeze'),
  mms_bottom_wake: p('MMS 底部苏醒', 'MMS Bottom Wake'),
  mms_squeeze_engine: p('MMS 轧空引擎', 'MMS Squeeze Engine'),
  mms_trend_ride: p('MMS 趋势跟随', 'MMS Trend Ride'),
  module_no_match: p('无匹配形态', 'No Matching Setup'),
  panic_reversal: p('恐慌反转', 'Panic Reversal'),
  pre_breakout_watch: p('突破前观察', 'Pre-Breakout Watch'),
  pre_distribution_watch: p('派发前观察', 'Pre-Distribution Watch'),
  pre_squeeze_watch: p('挤压前观察', 'Pre-Squeeze Watch'),
  pullback_reversal: p('回调反转', 'Pullback Reversal'),
  range_expansion_event: p('区间扩张', 'Range Expansion'),
  range_reversion: p('区间回归', 'Range Reversion'),
  relative_weakness: p('相对弱势', 'Relative Weakness'),
  short_squeeze: p('空头回补', 'Short Squeeze'),
  trend_breakout: p('趋势突破', 'Trend Breakout'),
  volatility_squeeze_breakout: p('波动收敛突破', 'Volatility Squeeze Breakout'),
  whale_flow_reversal: p('巨鲸资金反转', 'Whale Flow Reversal'),
  // Task-2 transplanted families
  momentum_continuation: p('动量延续', 'Momentum Continuation'),
  momentum_continuation_watch: p('动量延续观察', 'Momentum Continuation Watch'),
  bb_oi_accumulation: p('布林收口 OI 吸筹', 'BB Squeeze OI Accumulation'),
  confirmed_breakout: p('确认突破', 'Confirmed Breakout'),
  reclaim_breakout: p('收复突破', 'Reclaim Breakout'),
  trend_pullback: p('趋势回踩', 'Trend Pullback'),
  ignition_early: p('早期点火', 'Early Ignition'),
  compression_oi_watch: p('收敛 OI 观察', 'Compression OI Watch'),
}

const DIRECTION: Record<'long' | 'short', Pair> = {
  long: p('做多', 'Long'),
  short: p('做空', 'Short'),
}

// ── Setup families (doctrine §3 / Task-2 dispatcher) ────────────────────────
const FAMILY: Record<string, Pair> = {
  momentum: p('动量延续族', 'Momentum Continuation'),
  breakout: p('突破族', 'Breakout'),
  ladder: p('阶梯族', 'Ladder'),
  reversal: p('反转修复族', 'Reversal Repair'),
  weakness: p('弱势与派发族', 'Weakness & Distribution'),
}

const REGIME: Record<string, Pair> = {
  trend_up: p('上升趋势', 'Uptrend'),
  trend_down: p('下降趋势', 'Downtrend'),
  range: p('区间震荡', 'Range'),
  trend_continuation: p('趋势延续', 'Trend Continuation'),
  panic_dump: p('恐慌下跌', 'Panic Dump'),
  market_pullback: p('市场回调', 'Market Pullback'),
  mania_pump: p('狂热拉升', 'Mania Pump'),
  compression: p('收敛整理', 'Compression'),
  rotation: p('板块轮动', 'Rotation'),
  mixed: p('混合', 'Mixed'),
  unknown: p('未知', 'Unknown'),
  chop: p('震荡', 'Chop'),
}

const TIER: Record<string, Pair> = {
  EXECUTABLE: p('可执行', 'Executable'),
  REVIEWABLE: p('待复核', 'Reviewable'),
  WATCH: p('观察', 'Watch'),
  REJECTED: p('已拒绝', 'Rejected'),
}

const ACTION: Record<string, Pair> = {
  open_long: p('开多', 'Open Long'),
  open_short: p('开空', 'Open Short'),
  close_long: p('平多', 'Close Long'),
  close_short: p('平空', 'Close Short'),
  hold: p('持有', 'Hold'),
  wait: p('观望', 'Wait'),
}

const SIDE: Record<string, Pair> = {
  long: p('多', 'Long'),
  short: p('空', 'Short'),
  both: p('双向', 'Both'),
  buy: p('买入', 'Buy'),
  sell: p('卖出', 'Sell'),
}

const BLOCKED_REASON: Record<string, Pair> = {
  entry_not_in_zone: p('价格不在入场区', 'Price outside the entry zone'),
  rr_insufficient: p('盈亏比不足', 'Risk-reward too low'),
  confirmation_missing: p('确认条件未满足', 'Confirmation missing'),
  oi_too_low: p('持仓量过低', 'Open interest too low'),
  funding_crowded: p('资金费率拥挤', 'Funding crowded'),
  account_risk: p('账户风险', 'Account risk'),
  backend_guard_risk: p('后端风控拦截', 'Blocked by backend guard'),
  no_reviewable_candidate: p('无可复核候选', 'No reviewable candidate'),
}

const SOURCE_TYPE: Record<string, Pair> = {
  static: p('静态列表', 'Static List'),
  ai500: p('AI500 精选', 'AI500'),
  oi_top: p('OI 持仓增加', 'OI Increase'),
  oi_low: p('OI 持仓减少', 'OI Decrease'),
  mixed: p('混合模式', 'Mixed Mode'),
  square_heat: p('广场热度', 'Square Heat'),
  hunter: p('猎手选币', 'Hunter'),
  hunter_sniff: p('庄家嗅探', 'Hunter Sniff'),
  hunter_v7: p('猎手 v7', 'Hunter v7'),
  indicator_hub: p('多因子引擎', 'Indicator Hub'),
}

const DATA_QUALITY: Record<string, Pair> = {
  fresh: p('新鲜', 'Fresh'),
  stale: p('过期', 'Stale'),
  partial: p('部分缺失', 'Partial'),
  missing: p('缺失', 'Missing'),
  unknown: p('未知', 'Unknown'),
}

const TRACK_STATUS: Record<string, Pair> = {
  ACTIVE: p('跟踪中', 'Tracking'),
  TP0: p('已达 TP0', 'TP0 hit'),
  TP1: p('已达 TP1', 'TP1 hit'),
  TP2: p('已达 TP2', 'TP2 hit'),
  STOPPED: p('已止损', 'Stopped'),
  STOP: p('已止损', 'Stopped'),
  PROTECTED_STOP: p('保护止损', 'Protected stop'),
  WIN_TP0: p('达成 TP0', 'Won at TP0'),
  WIN_TP1: p('达成 TP1', 'Won at TP1'),
  WIN_TP2: p('达成 TP2', 'Won at TP2'),
  BOTH_SAME_1M: p('同 K 线歧义', 'Same-candle ambiguity'),
  EXPIRED: p('已过期', 'Expired'),
  RESTART_ORPHANED: p('重启后失联', 'Orphaned on restart'),
}

const SIGNAL_CLASS: Record<string, Pair> = {
  TRIGGER: p('已触发', 'Trigger'),
  TRIGGERED: p('已触发', 'Triggered'),
  ARMED: p('待触发', 'Armed'),
  WATCH: p('观察', 'Watch'),
  OBSERVE: p('仅观察', 'Observe'),
}

const RV24_TIER: Record<string, Pair> = {
  LOW: p('低波动', 'Low'),
  MID: p('中波动', 'Mid'),
  HIGH: p('高波动', 'High'),
}

// ── Reason / risk / confirmation tags ───────────────────────────────────────
// Phrase overrides for the tags users see most often. Everything else goes
// through the word dictionary.
const TAG_PHRASE: Record<string, Pair> = {
  long_setup_ready_confirmed: p('多头形态已确认', 'Long setup confirmed'),
  short_setup_ready_confirmed: p('空头形态已确认', 'Short setup confirmed'),
  watch_only_no_direct_open: p(
    '仅观察，不可直接开仓',
    'Watch only, no direct open'
  ),
  do_not_open_until_confirmed: p('确认前不开仓', 'Do not open until confirmed'),
  late_entry_risk: p('入场偏晚风险', 'Late entry risk'),
  no_pullback_still_running: p(
    '未回踩仍在运行',
    'Still running without a pullback'
  ),
  no_new_high_after_rejection: p('拒绝后无新高', 'No new high after rejection'),
  regime_against_direction: p('方向逆市场环境', 'Against the market regime'),
  oi_expanding: p('OI 扩张', 'OI expanding'),
  taker_buy_strong: p('主动买盘强劲', 'Strong taker buying'),
  taker_sell_strong: p('主动卖盘强劲', 'Strong taker selling'),
  already_pumped_24h: p('24 小时已大涨', 'Already pumped in 24h'),
  squeeze_chase_risk: p('追涨挤压风险', 'Squeeze chase risk'),
  funding_expensive: p('资金费率偏贵', 'Funding expensive'),
  lsr_extreme_long: p('多空比极端偏多', 'Long/short ratio extremely long'),
  module_no_match: p('无匹配形态', 'No matching setup'),
  t2_observe_only: p('任务二形态仅观察', 'Task-2 setup, observe only'),
  t2_triggered: p('任务二形态已触发', 'Task-2 setup triggered'),
  break_1h_low_squeeze_failed: p(
    '跌破 1 小时低点，回补失败',
    'Broke the 1h low, squeeze failed'
  ),
  '5m_price_holds_ema20_or_trailing_support': p(
    '5 分钟价格守住 EMA20 或跟踪支撑',
    '5m price holds EMA20 or trailing support'
  ),
  '5m_or_15m_close_through_breakout_level': p(
    '5/15 分钟收盘穿越突破位',
    '5m or 15m close through the breakout level'
  ),
  '15m_close_below_vwap': p('15 分钟收盘低于 VWAP', '15m close below VWAP'),
  taker_flow_not_flipping_against_direction: p(
    '主动成交流未反向',
    'Taker flow not flipping against direction'
  ),
  live_price_in_entry_zone: p(
    '实时价格位于入场区',
    'Live price inside the entry zone'
  ),
  displacement_rr_insufficient: p('位移盈亏比不足', 'Displacement RR too low'),
  displacement_rr_repaired_needs_review: p(
    '位移盈亏比已修复，需复核',
    'Displacement RR repaired, needs review'
  ),
}

// Word dictionary for tag tokens (split on "_"). Timeframes and numbers are
// handled in code; acronyms are kept.
const ACRONYMS: Record<string, string> = {
  oi: 'OI',
  vwap: 'VWAP',
  ema: 'EMA',
  ema20: 'EMA20',
  ema25: 'EMA25',
  ema60: 'EMA60',
  rsi: 'RSI',
  rsi5m: 'RSI(5m)',
  rr: 'RR',
  lsr: 'LSR',
  bb: 'BB',
  atr: 'ATR',
  mms: 'MMS',
  boll: 'BOLL',
  tp0: 'TP0',
  tp1: 'TP1',
  tp2: 'TP2',
  t2: 'T2',
  rs: 'RS',
  mfe: 'MFE',
}

const WORD_ZH: Record<string, string> = {
  taker: '主动成交',
  or: '或',
  range: '区间',
  short: '空头',
  funding: '资金费率',
  no: '无',
  long: '多头',
  buy: '买',
  entry: '入场',
  extreme: '极端',
  momentum: '动量',
  alt: '山寨',
  ladder: '阶梯',
  volume: '成交量',
  flow: '资金流',
  strong: '强',
  breakout: '突破',
  high: '高点',
  moderate: '温和',
  crowding: '拥挤',
  zone: '区',
  price: '价格',
  chase: '追单',
  flush: '清洗',
  sell: '卖',
  close: '收盘',
  reclaim: '收复',
  expansion: '扩张',
  squeeze: '挤压',
  below: '低于',
  accumulation: '吸筹',
  event: '事件',
  low: '低点',
  near: '接近',
  displacement: '位移',
  above: '高于',
  watch: '观察',
  late: '后期',
  risk: '风险',
  not: '未',
  wait: '等待',
  only: '仅',
  after: '之后',
  new: '新',
  crowded: '拥挤',
  pullback: '回踩',
  match: '匹配',
  shape: '形态',
  at: '于',
  confirmed: '已确认',
  weak: '弱',
  mild: '轻微',
  retest: '回测',
  trigger: '触发位',
  support: '支撑',
  continuation: '延续',
  solid: '稳固',
  mid: '中段',
  approaching: '逼近',
  context: '背景',
  deep: '深度',
  compression: '收敛',
  neutral: '中性',
  heavy: '沉重',
  reversal: '反转',
  downside: '下行',
  rejection: '拒绝',
  resistance: '阻力',
  from: '自',
  bottom: '底部',
  top: '顶部',
  early: '早期',
  pre: '前置',
  directional: '方向性',
  needs: '需要',
  liquidity: '流动性',
  rally: '反弹',
  fast: '快速',
  selling: '抛售',
  without: '无',
  trend: '趋势',
  bleed: '阴跌',
  impulse: '冲击',
  level: '位',
  breakdown: '破位',
  inflow: '流入',
  stage: '阶段',
  amplitude: '振幅',
  elevated: '偏高',
  turning: '转向',
  confirmation: '确认',
  drop: '下跌',
  with: '伴随',
  bullish: '看涨',
  massive: '巨量',
  signal: '信号',
  stalling: '停滞',
  building: '积累',
  confirms: '确认',
  declining: '下降',
  down: '下降',
  regime: '环境',
  overbought: '超买',
  upper: '上沿',
  lower: '下沿',
  downshift: '下移',
  shorts: '空头',
  extension: '延伸',
  distribution: '派发',
  candidate: '候选',
  floor: '底线',
  negative: '为负',
  capitulation: '投降式抛售',
  move: '波动',
  do: '',
  open: '开仓',
  followthrough: '跟进',
  surge: '急涨',
  volatility: '波动率',
  reviewable: '待复核',
  confirm: '确认',
  recovering: '恢复中',
  fresh: '新鲜',
  micro: '微观',
  timing: '时机',
  positive: '为正',
  cascade: '连锁',
  potential: '潜在',
  scalp: '短线',
  leader: '龙头',
  up: '上升',
  vol: '量',
  against: '逆向',
  stealth: '隐蔽',
  still: '仍',
  build: '积累',
  detected: '检测到',
  stable: '稳定',
  oversold: '超卖',
  tight: '紧',
  burst: '爆发',
  through: '穿越',
  holds: '守住',
  trailing: '跟踪',
  multi: '多',
  cycle: '周期',
  compressed: '收敛',
  breaks: '突破',
  conflict: '冲突',
  overextended: '过度延伸',
  insufficient: '不足',
  repaired: '已修复',
  review: '复核',
  execution: '执行',
  trade: '交易',
  invalid: '无效',
  live: '实时',
  aligned: '同向',
  emerging: '浮现',
  remains: '保持',
  healthy: '健康',
  pump: '拉升',
  filtered: '已过滤',
  improving: '改善',
  wake: '苏醒',
  quiet: '安静',
  missing: '缺失',
  overheated: '过热',
  failed: '失败',
  immediate: '即时',
  of: '的',
  noise: '噪音',
  delta: '变化',
  expands: '扩张',
  growth: '增长',
  covering: '回补',
  exhaustion: '衰竭',
  direction: '方向',
  deeply: '深度',
  geometry: '几何',
  decelerating: '减速',
  shallow: '浅',
  velocity: '速度',
  symbol: '标的',
  override: '覆盖',
  gt: '>',
  lt: '<',
  before: '之前',
  required: '必需',
  upgraded: '已升级',
  green: '绿',
  shoot: '芽',
  accelerating: '加速',
  already: '已',
  pumped: '拉升',
  major: '主流币',
  attempt: '尝试',
  protection: '保护',
  clear: '清晰',
  air: '空间',
  priority: '优先级',
  continuous: '持续',
  correlation: '相关性',
  longs: '多头',
  daily: '日线',
  defined: '明确',
  valid: '有效',
  revalidated: '已重新验证',
  repair: '修复',
  market: '市场',
  until: '直到',
  blocked: '已拦截',
  now: '当前',
  repairable: '可修复',
  normalized: '已归一',
  needed: '需要',
  quality: '质量',
  stop: '止损',
  tightened: '已收紧',
  extended: '已延伸',
  gain: '涨幅',
  fallback: '兜底',
  far: '远离',
  tracked: '已跟踪',
  aggressive: '激进',
  sustained: '持续',
  dominant: '主导',
  absent: '缺失',
  rest: '其余',
  expensive: '偏贵',
  track: '跟踪',
  edge: '边缘',
  stronger: '更强',
  holding: '守住',
  ideal: '理想',
  width: '宽度',
  intraday: '日内',
  in: '在',
  break: '突破',
  increase: '增加',
  balanced: '均衡',
  shifting: '转移',
  hold: '守住',
  fan: '扇形',
  fuel: '燃料',
  ban: '禁止',
  active: '生效',
  small: '小',
  cap: '市值',
  proxy: '代理',
  engine: '引擎',
  trader: '交易者',
  lock: '锁定',
  ride: '跟随',
  distance: '距离',
  module: '模块',
  exhausted: '衰竭',
  middle: '中部',
  back: '回到',
  inside: '内部',
  loss: '亏损',
  unknown: '未知',
  running: '运行中',
  setup: '形态',
  matched: '已匹配',
  confirming: '确认中',
  anomaly: '异常',
  markup: '拉升',
  clearing: '出清',
  demand: '需求',
  consistent: '一致',
  quote: '报价',
  expanding: '扩张',
  rebuild: '重建',
  flushing: '清洗中',
  increasing: '增加',
  invisible: '隐形',
  buildup: '积累',
  minimal: '极少',
  rising: '上升',
  stabilize: '企稳',
  stabilizes: '企稳',
  starts: '开始',
  radar: '雷达',
  bouncing: '反弹中',
  flattening: '走平',
  action: '动作',
  grind: '慢磨',
  channel: '通道',
  remote: '远端',
  target: '目标',
  rescue: '补救',
  all: '全部',
  frames: '周期',
  decline: '下降',
  leaking: '流失',
  backend: '后端',
  global: '全局',
  incompatible: '不兼容',
  sector: '板块',
  rotation: '轮动',
  pressure: '压力',
  clean: '干净',
  prebreakout: '突破前',
  panic: '恐慌',
  reversion: '回归',
  sharp: '急剧',
  lose: '失守',
  some: '部分',
  overhead: '上方',
  stale: '过期',
  data: '数据',
  steady: '平稳',
  observe: '观察',
  demoted: '已降级',
  triggered: '已触发',
  stays: '保持',
  bias: '偏向',
  biased: '偏向',
  borderline: '临界',
  ratio: '比率',
  recovery: '恢复',
  weakening: '走弱',
  buying: '买入',
  flipping: '反转',
  during: '期间',
  thin: '稀薄',
  memory: '记忆',
  ultra: '超',
  uptrend: '上升趋势',
  intact: '完好',
  very: '非常',
  adequate: '充足',
  decent: '尚可',
  for: '用于',
  wash: '刷量',
  direct: '直接',
  executable: '可执行',
  bounce: '反弹',
  whale: '巨鲸',
  ignition: '点火',
  liquidation: '清算',
  theme: '主题',
  sniff: '嗅探',
  exit: '离场',
  sweep: '扫单',
  gap: '缺口',
  vacuum: '真空',
  lag: '滞后',
  laggard: '落后者',
  catchup: '补涨',
  spot: '现货',
  basis: '基差',
  premium: '溢价',
  discount: '折价',
  value: '价值',
  heat: '热度',
  rank: '排名',
  score: '评分',
  low_priority: '低优先级',
  confirmation_summary: '确认摘要',
  zone_position: '区位',
  first: '首次',
  second: '二次',
  touch: '触及',
  wick: '影线',
  body: '实体',
  doji: '十字星',
  engulfing: '吞没',
  hammer: '锤子线',
  star: '星线',
  inverse: '反向',
  double: '双',
  triple: '三重',
  head: '头',
  shoulders: '肩',
  flag: '旗形',
  pennant: '三角旗',
  wedge: '楔形',
  cup: '杯',
  handle: '柄',
  base: '底座',
  climax: '高潮',
  spring: '弹簧',
  upthrust: '上冲',
  test: '测试',
  sign: '迹象',
  strength: '强度',
  weakness: '弱势',
  effort: '努力',
  result: '结果',
  relative: '相对',
  short_squeeze: '空头回补',
  long_squeeze: '多头挤压',
  obv: 'OBV',
  vpvr: 'VPVR',
}

const DURATION_ZH: Record<string, string> = { m: '分钟', h: '小时', d: '日' }

function isTimeframe(tok: string): boolean {
  return /^\d+(m|h|d)$/.test(tok)
}

function timeframe(tok: string, lang: Language): string {
  if (lang === 'en') return tok
  const m = tok.match(/^(\d+)(m|h|d)$/)
  return m ? `${m[1]}${DURATION_ZH[m[2]]}` : tok
}

function titleCase(word: string): string {
  return word ? word[0].toUpperCase() + word.slice(1) : word
}

/** Word-level rendering of an unknown code (never returns snake_case). */
export function humanizeCode(code: string, lang: Language): string {
  const clean = code.trim().replace(/[:]+/g, ' ').replace(/\s+/g, '_')
  const tokens = clean.split('_').filter(Boolean)
  if (lang === 'en') {
    const out = tokens.map((tok) => {
      const lower = tok.toLowerCase()
      if (ACRONYMS[lower]) return ACRONYMS[lower]
      if (isTimeframe(lower)) return lower
      if (/^[A-Z0-9]+$/.test(tok) && tok.length > 1) return titleCase(lower)
      return lower
    })
    const s = out.join(' ')
    return titleCase(s)
  }
  const parts: string[] = []
  tokens.forEach((tok, i) => {
    const lower = tok.toLowerCase()
    const prev = (tokens[i - 1] || '').toLowerCase()
    const isLast = i === tokens.length - 1
    // "low"/"high" are price levels at the end or after "new"/a timeframe,
    // adjectives elsewhere (low_volume -> 低成交量, break_1h_low -> 跌破1小时低点).
    if (
      (lower === 'low' || lower === 'high') &&
      !isLast &&
      prev !== 'new' &&
      !isTimeframe(prev)
    ) {
      parts.push(lower === 'low' ? '低' : '高')
      return
    }
    if (ACRONYMS[lower]) parts.push(` ${ACRONYMS[lower]} `)
    else if (isTimeframe(lower)) parts.push(timeframe(lower, lang))
    else if (/^\d+(\.\d+)?$/.test(lower)) parts.push(` ${lower} `)
    else if (WORD_ZH[lower] !== undefined) parts.push(WORD_ZH[lower])
    else parts.push(` ${lower} `)
  })
  return parts.join('').replace(/\s+/g, ' ').trim()
}

// ── Public API ──────────────────────────────────────────────────────────────
export type DisplayKind =
  | 'setup'
  | 'family'
  | 'regime'
  | 'tier'
  | 'action'
  | 'side'
  | 'direction'
  | 'blockedReason'
  | 'sourceType'
  | 'tag'
  | 'dataQuality'
  | 'trackStatus'
  | 'signalClass'
  | 'rv24Tier'
  | 'rrBand'

function lookup(map: Record<string, Pair>, code: string, lang: Language) {
  return map[code]?.[lang]
}

/** "动量延续 · 做多" / "Momentum Continuation · Long" */
export function setupLabel(code: string, lang: Language): string {
  const c = (code || '').trim().toLowerCase()
  if (!c) return ''
  const m = c.match(/^(.*)_(long|short)$/)
  if (m && SETUP_BASE[m[1]]) {
    return `${SETUP_BASE[m[1]][lang]} · ${DIRECTION[m[2] as 'long' | 'short'][lang]}`
  }
  if (SETUP_BASE[c]) return SETUP_BASE[c][lang]
  return humanizeCode(c, lang)
}

/** Base setup name without direction, for grouping Long/Short twins. */
export function setupBaseLabel(base: string, lang: Language): string {
  return SETUP_BASE[base]?.[lang] ?? humanizeCode(base, lang)
}

/** Tier reasons and tags: phrase map, known prefixes, then word rendering. */
export function tagLabel(code: string, lang: Language): string {
  const raw = (code || '').trim()
  if (!raw) return ''
  const mult = raw.match(/^(.*?)\s*×\s*(\d+)$/)
  if (mult) return `${tagLabel(mult[1], lang)} ×${mult[2]}`
  const c = raw.toLowerCase()
  const phrase = TAG_PHRASE[c]?.[lang] ?? BLOCKED_REASON[c]?.[lang]
  if (phrase) return phrase
  const prefixes: [string, Pair][] = [
    ['confirmation_missing_', p('待确认：', 'Awaiting: ')],
    ['live_reviewable_', p('实时复核：', 'Live review: ')],
    ['live_confirmed_', p('实时已确认：', 'Live confirmed: ')],
    ['prompt_readiness_', p('就绪度：', 'Readiness: ')],
    ['sector_theme_', p('板块主题：', 'Sector theme: ')],
  ]
  for (const [prefix, label] of prefixes) {
    if (c.startsWith(prefix)) {
      return label[lang] + tagLabel(c.slice(prefix.length), lang)
    }
  }
  const lt = c.match(/^([a-z_]+)_(lt|gt)_(\d+(?:\.\d+)?)$/)
  if (lt) {
    return `${tagLabel(lt[1], lang)} ${lt[2] === 'lt' ? '<' : '>'} ${lt[3]}`
  }
  if (SETUP_BASE[c] || /_(long|short)$/.test(c)) {
    const s = setupLabel(c, lang)
    if (s) return s
  }
  return humanizeCode(c, lang)
}

export function displayName(
  kind: DisplayKind,
  code: string | undefined | null,
  lang: Language
): string {
  const raw = (code ?? '').toString().trim()
  if (!raw) return ''
  const lower = raw.toLowerCase()
  switch (kind) {
    case 'setup':
      return setupLabel(lower, lang)
    case 'family':
      return lookup(FAMILY, lower, lang) ?? humanizeCode(lower, lang)
    case 'regime':
      return lookup(REGIME, lower, lang) ?? humanizeCode(lower, lang)
    case 'tier':
      return lookup(TIER, raw.toUpperCase(), lang) ?? humanizeCode(lower, lang)
    case 'action':
      return lookup(ACTION, lower, lang) ?? humanizeCode(lower, lang)
    case 'side':
      return lookup(SIDE, lower, lang) ?? humanizeCode(lower, lang)
    case 'direction':
      return (
        DIRECTION[lower as 'long' | 'short']?.[lang] ??
        lookup(SIDE, lower, lang) ??
        humanizeCode(lower, lang)
      )
    case 'blockedReason':
      return lookup(BLOCKED_REASON, lower, lang) ?? tagLabel(lower, lang)
    case 'sourceType':
      return lookup(SOURCE_TYPE, lower, lang) ?? humanizeCode(lower, lang)
    case 'dataQuality':
      return lookup(DATA_QUALITY, lower, lang) ?? humanizeCode(lower, lang)
    case 'trackStatus':
      return (
        lookup(TRACK_STATUS, raw.toUpperCase(), lang) ??
        humanizeCode(lower, lang)
      )
    case 'signalClass':
      return (
        lookup(SIGNAL_CLASS, raw.toUpperCase(), lang) ??
        (SETUP_BASE[lower] || /_(long|short)$/.test(lower)
          ? setupLabel(lower, lang)
          : humanizeCode(lower, lang))
      )
    case 'rv24Tier':
      return (
        lookup(RV24_TIER, raw.toUpperCase(), lang) ?? humanizeCode(lower, lang)
      )
    case 'rrBand':
      return `RR ${raw.replace(/^rr_?/i, '').replace(/_/g, '–')}`
    case 'tag':
    default:
      return tagLabel(raw, lang)
  }
}

// ── Tag catalog metadata (category / polarity / model instruction) ─────────
const TAG_META: Record<
  'category' | 'polarity' | 'action',
  Record<string, Pair>
> = {
  category: {
    price: p('价格', 'Price'),
    oi: p('持仓量', 'Open interest'),
    flow: p('资金流', 'Flow'),
    state: p('状态', 'State'),
    risk: p('风险', 'Risk'),
    funding: p('资金费率', 'Funding'),
    confirmation: p('确认条件', 'Confirmation'),
    tier: p('分层', 'Tier'),
    liquidity: p('流动性', 'Liquidity'),
    positioning: p('持仓结构', 'Positioning'),
  },
  polarity: {
    bullish: p('看涨', 'Bullish'),
    bearish: p('看跌', 'Bearish'),
    neutral: p('中性', 'Neutral'),
    mixed: p('多空混合', 'Mixed'),
  },
  action: {
    evidence_only: p('仅作证据', 'Evidence only'),
    required_confirmation: p('必须确认', 'Must be confirmed'),
    wait_only: p('只能等待', 'Wait only'),
    reject_only: p('直接拒绝', 'Reject'),
    reviewable_only_if_live_confirmed: p(
      '实时确认后可复核',
      'Reviewable once live-confirmed'
    ),
    context_only: p('仅作背景', 'Context only'),
    reduce_size_or_wait: p('减仓或等待', 'Reduce size or wait'),
    supports_open_after_core_checks: p(
      '核心检查通过后支持开仓',
      'Supports an open after core checks'
    ),
    unknown_context_only: p('未知，仅作背景', 'Unknown, context only'),
  },
}

export function tagMetaLabel(
  field: 'category' | 'polarity' | 'action',
  value: string | undefined,
  lang: Language
): string {
  if (!value) return ''
  return TAG_META[field][value]?.[lang] ?? humanizeCode(value, lang)
}

/** Exposed for the consistency test and the design doc. */
export const DISPLAY_MAPS = {
  SETUP_BASE,
  FAMILY,
  REGIME,
  TIER,
  ACTION,
  SIDE,
  BLOCKED_REASON,
  SOURCE_TYPE,
  TAG_PHRASE,
  WORD_ZH,
  ACRONYMS,
}
