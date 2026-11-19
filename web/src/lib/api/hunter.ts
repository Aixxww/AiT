/**
 * Hunter v7 signal API — read-only feed for the dashboard signal panel.
 *
 * Mirrors the Go contract from `api/handler_hunter.go` (GET
 * /api/hunter/v7/signals): each row carries the persisted tier verdict plus
 * the full V7SignalOutput snapshot re-hydrated from the kernel recorder.
 */
import { API_BASE, httpClient } from './helpers'

export type V7Tier = 'EXECUTABLE' | 'REVIEWABLE' | 'WATCH' | 'REJECTED'

export interface V7PriceZone {
  lower: number
  upper: number
}

export interface V7InvalidationRule {
  price: number
  reason?: string
}

export interface V7Target {
  price: number
  reason?: string
}

export interface V7ConfirmationSummary {
  passed_hard: boolean
  passed_review: boolean
  entry_zone_position?: number
  stop_distance_pct?: number
  reward_pct?: number
  rr?: number
}

export interface V7ExecutionReadiness {
  tier: V7Tier
  reason?: string
  ready_score?: number
  window_health?: number
  entry_zone_position?: number
  price_deviation_pct?: number
  data_quality?: string
  missing_hard?: string[]
  missing_execution?: string[]
  blocked_gate?: string
  next_confirmations?: string[]
}

export interface V7PriceContext {
  last?: number
  change_1h?: number
  change_4h?: number
  change_24h?: number
  atr_1h?: number
  atr_4h?: number
  vwap_15m?: number
}

export interface V7DerivativesContext {
  oi_value?: number
  oi_change_1h?: number
  oi_change_4h?: number
  funding_rate?: number
  lsr_oldest?: number
  lsr_newest?: number
  taker_buy_ratio_15m?: number
}

export interface V7Signal {
  signal_id?: string
  symbol: string
  direction: 'LONG' | 'SHORT'
  setup_type: string
  status?: string
  setup_score: number
  risk_score: number
  liquidity_score: number
  timing_score: number
  regime_fit_score: number
  ai_priority: number
  reason_codes?: string[]
  risk_tags?: string[]
  entry_zone: V7PriceZone
  invalidation: V7InvalidationRule
  targets?: V7Target[]
  required_confirmations?: string[]
  confirmation_summary?: V7ConfirmationSummary
  market_regime?: string
  execution_readiness?: V7ExecutionReadiness
  price_context?: V7PriceContext
  derivatives_context?: V7DerivativesContext
  tp0_price?: number
  tp0_rr?: number
  tp1_price?: number
  tp1_rr?: number
  tp2_price?: number
  tp2_rr?: number
}

export interface V7SignalRow {
  id: number
  cycle_number: number
  timestamp: string
  execution_tier: V7Tier | ''
  tier_reason: string
  blocked_gate?: string
  track_status?: string
  track_pnl_pct: number
  track_tp0_done?: boolean
  track_tp1_done?: boolean
  track_tp2_done?: boolean
  track_remaining_ratio?: number
  track_realized_pnl_pct?: number
  signal: V7Signal
}

export interface V7SignalsResponse {
  count: number
  signals: V7SignalRow[]
  cycle?: V7LatestCycleSummary
  window_source: string
}

export interface V7CountedReason {
  code: string
  count: number
}

export interface V7LatestCycleSummary {
  timestamp?: string
  cycle_number: number
  persisted_records: number
  tier_counts: Partial<Record<V7Tier, number>>
  status_counts: Record<string, number>
  data_quality_counts: Record<string, number>
  top_vetoes: V7CountedReason[]
}

/** One entry of the Hunter v7 tag catalog (GET /api/hunter/v7/tag-catalog). */
export interface V7TagDefinition {
  tag: string
  source: string
  category: string
  polarity: string
  llm_action: string
  definition: string
}

export interface V7TagCatalogResponse {
  count: number
  tags: V7TagDefinition[]
  source: string
}

/**
 * U12 cell dashboard — GET /api/hunter/v7/cells (api/handler_hunter.go,
 * store/hunter_v7_cells.go). Read-only; nothing here gates admission.
 */
export type V7CellSource =
  | 'prompt_open_review'
  | 'all'
  | 'watch_audit'
  | 'shadow'

export type V7CellStatus = 'earned' | 'probation' | 'blocked'

export interface V7CellRow {
  signal_class: string
  rr_band?: string
  market_regime?: string
  n: number
  tp1: number
  stop: number
  none: number
  tp1_first_rate: number
  mean_fee_net_r: number
  ci_lo: number
  ci_hi: number
  stamped_status: string[]
  evaluated_status: V7CellStatus
}

export interface V7CellTables {
  rows: number
  classes: V7CellRow[]
  cells: V7CellRow[]
  cell_regimes: V7CellRow[]
}

export interface V7CellRule {
  min_samples: number
  ci_level: number
  bootstrap_resamples: number
  seed: number
  quantiles: number
}

export interface V7CalibrationQuantile {
  quantile: number
  n: number
  pred_mean: number
  realized_mean: number
  ci_lo: number
  ci_hi: number
  inside: boolean
}

export interface V7CalibrationWindow {
  from: string
  to: string
  n: number
  sufficient: boolean
  quantiles_outside: number
  quantiles: V7CalibrationQuantile[]
}

export interface V7CalibrationDrift {
  calibration_version: string
  current: V7CalibrationWindow
  previous: V7CalibrationWindow
  drift: boolean
}

export interface V7CellsResponse {
  from: string
  to: string
  days: number
  source: V7CellSource
  rule: V7CellRule
  hysteresis: 'none'
  tables: V7CellTables
  excluded_no_r: Record<string, number>
  calibration_drift: V7CalibrationDrift[]
}

export const hunterApi = {
  async getV7Signals(limit: number = 80): Promise<V7SignalsResponse> {
    const result = await httpClient.request<V7SignalsResponse>(
      `${API_BASE}/hunter/v7/signals?limit=${limit}`,
      { silent: true }
    )
    if (!result.success || !result.data) {
      throw new Error('Failed to fetch v7 signals')
    }
    return result.data
  },

  async getV7Cells(
    days = 7,
    source: V7CellSource = 'prompt_open_review'
  ): Promise<V7CellsResponse> {
    const params = new URLSearchParams({ days: String(days), source })
    const result = await httpClient.request<V7CellsResponse>(
      `${API_BASE}/hunter/v7/cells?${params.toString()}`,
      { silent: true }
    )
    if (!result.success || !result.data) {
      throw new Error('Failed to fetch v7 cell dashboard')
    }
    return result.data
  },

  async getV7TagCatalog(): Promise<V7TagCatalogResponse> {
    const result = await httpClient.request<V7TagCatalogResponse>(
      `${API_BASE}/hunter/v7/tag-catalog`,
      { silent: true }
    )
    if (!result.success || !result.data) {
      throw new Error('Failed to fetch v7 tag catalog')
    }
    return result.data
  },
}
