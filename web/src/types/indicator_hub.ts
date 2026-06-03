/**
 * IndicatorHub v3.0 — Unified scoring engine configuration
 * Replaces the legacy CoinSourceConfig (ai500, hunter, oi_top, etc.)
 *
 * All data comes from a unified DataCollector (Binance REST + WebSocket + LunarCrush),
 * and all scoring is done through a single IndicatorHub pipeline.
 */

// Main configuration for the IndicatorHub scoring engine
export interface IndicatorHubConfig {
  // ── Layer Weights ──
  // Technical indicators layer weight (default 40)
  tech_weight: number;
  // Quantitative indicators layer weight (default 40)
  quant_weight: number;
  // Social indicators layer weight (default 20)
  social_weight: number;

  // ── Technical Indicator Toggles ──
  rsi_enabled: boolean;       // default true
  macd_enabled: boolean;      // default true
  bb_enabled: boolean;        // default true
  ema_enabled: boolean;       // default true
  atr_enabled: boolean;       // default true

  // ── Quantitative Indicator Weights (within quant layer) ──
  oi_weight: number;          // default 25 (OI change rate)
  oi_spike_weight: number;    // default 15 (OI spike detection)
  funding_rate_weight: number; // default 15
  lsr_weight: number;         // default 15 (Long/Short Ratio)
  taker_weight: number;       // default 15 (Taker buy/sell ratio)
  volume_weight: number;      // default 15 (Volume anomaly)

  // ── Social Indicator Weights (within social layer) ──
  heat_score_weight: number;  // default 30
  sentiment_weight: number;   // default 25
  social_volume_weight: number; // default 20
  kol_weight: number;         // default 15
  galaxy_score_weight: number; // default 10

  // ── Direction & Grade Thresholds ──
  // Minimum bull-bear difference to determine direction
  direction_margin: number;   // default 15
  // Grade thresholds
  grade_s_threshold: number;  // default 80 (immediate execute)
  grade_a_threshold: number;  // default 65 (execute)
  grade_b_threshold: number;  // default 50 (AI confirmation)

  // ── SL/TP Multipliers (in ATR units) ──
  stop_loss_atr: number;      // default 2.0
  tp1_atr: number;            // default 1.5
  tp2_atr: number;            // default 3.0
  tp3_atr: number;            // default 5.0

  // ── Engine Cycle Settings ──
  max_signals_per_cycle: number;  // default 5
  min_score: number;              // default 50
  cooldown_minutes: number;       // default 60
  top_n_for_scoring: number;      // default 100 (only score top N by volume)
  top_n_for_detail: number;       // default 100 (only top N get OI/LSR/Klines)
  top_n_for_ws: number;           // default 30 (top N symbols get WS kline/aggTrade streams)

  // ── Data Source Settings ──
  rest_interval_secs: number;     // default 30
  social_interval_mins: number;   // default 15
  social_enabled: boolean;        // default true (requires LunarCrush API key)
  lunarcrush_api_key?: string;    // LunarCrush API key

  // ── Excluded Coins ──
  excluded_coins?: string[];      // Coins to always skip
  // ── Static Coins (always included in scoring) ──
  static_coins?: string[];        // Coins to always include
}

// Grade enum matching Go backend
export type SignalGrade = 'S' | 'A' | 'B' | 'C';

// Direction enum
export type SignalDirection = 'LONG' | 'SHORT' | 'NEUTRAL';

// Trade signal output from the scoring engine
export interface TradeSignal {
  symbol: string;
  direction: SignalDirection;
  final_score: number;        // 0-100
  grade: SignalGrade;
  tech_score: number;         // Technical layer score
  quant_score: number;        // Quantitative layer score
  social_score: number;       // Social layer score

  // Entry/Exit parameters
  entry_price: number;
  stop_loss: number;
  tp1: number;
  tp2: number;
  tp3: number;

  // Signal details
  bull_signals: string[];     // Bullish signals that triggered
  bear_signals: string[];     // Bearish signals that triggered
  reasons: string[];          // Combined reasoning

  timestamp: string;          // ISO 8601
}

// Indicator values for a single symbol (for frontend display)
export interface IndicatorValues {
  symbol: string;

  // Technical
  rsi_14: number;
  macd_line: number;
  macd_signal: number;
  macd_histogram: number;
  bb_upper: number;
  bb_middle: number;
  bb_lower: number;
  bb_width: number;
  ema_20: number;
  ema_50: number;
  ema_200: number;
  atr_14: number;

  // Quantitative
  oi_current: number;
  oi_delta_1h: number;
  oi_delta_4h: number;
  funding_rate: number;
  long_short_ratio: number;
  taker_buy_ratio: number;
  volume_24h: number;

  // Social
  heat_score: number;
  sentiment: number;
  social_volume: number;
  kol_count: number;
  galaxy_score: number;

  // Sub-scores
  tech_bull: number;
  tech_bear: number;
  quant_bull: number;
  quant_bear: number;
  social_bull: number;
  social_bear: number;
}

// Snapshot metadata (displayed in dashboard)
export interface SnapshotMeta {
  symbol_count: number;
  fetch_duration_ms: number;
  rest_errors: number;
  ws_connected: boolean;
  social_fresh: boolean;
  last_updated: string;       // ISO 8601
}

// Default configuration values
export const DEFAULT_INDICATOR_HUB_CONFIG: IndicatorHubConfig = {
  tech_weight: 40,
  quant_weight: 40,
  social_weight: 20,

  rsi_enabled: true,
  macd_enabled: true,
  bb_enabled: true,
  ema_enabled: true,
  atr_enabled: true,

  oi_weight: 25,
  oi_spike_weight: 15,
  funding_rate_weight: 15,
  lsr_weight: 15,
  taker_weight: 15,
  volume_weight: 15,

  heat_score_weight: 30,
  sentiment_weight: 25,
  social_volume_weight: 20,
  kol_weight: 15,
  galaxy_score_weight: 10,

  direction_margin: 15,
  grade_s_threshold: 80,
  grade_a_threshold: 65,
  grade_b_threshold: 50,

  stop_loss_atr: 2.0,
  tp1_atr: 1.5,
  tp2_atr: 3.0,
  tp3_atr: 5.0,

  max_signals_per_cycle: 5,
  min_score: 50,
  cooldown_minutes: 60,
  top_n_for_scoring: 100,
  top_n_for_detail: 100,
  top_n_for_ws: 30,

  rest_interval_secs: 30,
  social_interval_mins: 15,
  social_enabled: true,
};
