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
  signal: V7Signal
}

export interface V7SignalsResponse {
  count: number
  signals: V7SignalRow[]
  window_source: string
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
}
