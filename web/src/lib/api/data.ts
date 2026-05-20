import type {
  SystemStatus,
  AccountInfo,
  Position,
  DecisionRecord,
  Statistics,
  CompetitionData,
  PositionHistoryResponse,
} from '../../types'
import { API_BASE, httpClient } from './helpers'

export interface SquareHeatItem {
  token: string
  symbol: string
  composite_score: number
  score: number
  mentions: number
  trend: string
  mark_price?: number
  change_24h_pct?: number
  change_1h_pct?: number
  funding_rate_pct?: number
  oi_change_1h_pct?: number
  long_short_ratio?: number
  direction?: string
  verdict?: string
  tags?: string[]
}

export interface SquareHeatResponse {
  items: SquareHeatItem[]
  updated_at: string | null
  count: number
  error?: string
}

export const dataApi = {
  async getStatus(traderId?: string, silent?: boolean): Promise<SystemStatus> {
    const url = traderId
      ? `${API_BASE}/status?trader_id=${traderId}`
      : `${API_BASE}/status`
    const result = await httpClient.request<SystemStatus>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch system status')
    return result.data!
  },

  async getAccount(traderId?: string, silent?: boolean): Promise<AccountInfo> {
    const url = traderId
      ? `${API_BASE}/account?trader_id=${traderId}`
      : `${API_BASE}/account`
    const result = await httpClient.request<AccountInfo>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch account info')
    return result.data!
  },

  async getPositions(
    traderId?: string,
    silent?: boolean
  ): Promise<Position[]> {
    const url = traderId
      ? `${API_BASE}/positions?trader_id=${traderId}`
      : `${API_BASE}/positions`
    const result = await httpClient.request<Position[]>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch positions')
    return result.data!
  },

  async getDecisions(traderId?: string): Promise<DecisionRecord[]> {
    const url = traderId
      ? `${API_BASE}/decisions?trader_id=${traderId}`
      : `${API_BASE}/decisions`
    const result = await httpClient.get<DecisionRecord[]>(url)
    if (!result.success) throw new Error('Failed to fetch decision logs')
    return result.data!
  },

  async getLatestDecisions(
    traderId?: string,
    limit: number = 5,
    silent?: boolean
  ): Promise<DecisionRecord[]> {
    const params = new URLSearchParams()
    if (traderId) {
      params.append('trader_id', traderId)
    }
    params.append('limit', limit.toString())

    const result = await httpClient.request<DecisionRecord[]>(
      `${API_BASE}/decisions/latest?${params}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch latest decisions')
    return result.data!
  },

  async getStatistics(
    traderId?: string,
    silent?: boolean
  ): Promise<Statistics> {
    const url = traderId
      ? `${API_BASE}/statistics?trader_id=${traderId}`
      : `${API_BASE}/statistics`
    const result = await httpClient.request<Statistics>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch statistics')
    return result.data!
  },

  async getEquityHistory(
    traderId?: string,
    silent?: boolean
  ): Promise<any[]> {
    const url = traderId
      ? `${API_BASE}/equity-history?trader_id=${traderId}`
      : `${API_BASE}/equity-history`
    const result = await httpClient.request<any[]>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch equity history')
    return result.data!
  },

  async getEquityHistoryBatch(traderIds: string[], hours?: number): Promise<any> {
    const result = await httpClient.post<any>(
      `${API_BASE}/equity-history-batch`,
      { trader_ids: traderIds, hours: hours || 0 }
    )
    if (!result.success) throw new Error('Failed to fetch batch equity history')
    return result.data!
  },

  async getTopTraders(): Promise<any[]> {
    const result = await httpClient.get<any[]>(`${API_BASE}/top-traders`)
    if (!result.success) throw new Error('Failed to fetch top traders')
    return result.data!
  },

  async getPublicTraderConfig(traderId: string): Promise<any> {
    const result = await httpClient.get<any>(
      `${API_BASE}/traders/${traderId}/public-config`
    )
    if (!result.success) throw new Error('Failed to fetch public trader config')
    return result.data!
  },

  async getCompetition(): Promise<CompetitionData> {
    const result = await httpClient.get<CompetitionData>(
      `${API_BASE}/competition`
    )
    if (!result.success) throw new Error('Failed to fetch competition data')
    return result.data!
  },

  async getPositionHistory(
    traderId: string,
    limit: number = 100,
    silent?: boolean
  ): Promise<PositionHistoryResponse> {
    const result = await httpClient.request<PositionHistoryResponse>(
      `${API_BASE}/positions/history?trader_id=${traderId}&limit=${limit}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch position history')
    return result.data!
  },

  async getSquareHeat(): Promise<SquareHeatResponse> {
    try {
      const result = await httpClient.get<any>(
        `${API_BASE}/square-heat`
      )
      if (result && result.data) {
        return result.data as SquareHeatResponse
      }
      return { items: [], updated_at: null, count: 0 }
    } catch {
      return { items: [], updated_at: null, count: 0 }
    }
  },

  // ── Square Monitor Worker Control ──

  async getSquareMonitorStatus(): Promise<{ running: boolean; pid: number }> {
    const result = await httpClient.get<any>(`${API_BASE}/square-monitor/status`)
    return result.data ?? { running: false, pid: 0 }
  },

  async startSquareMonitor(): Promise<{ message: string; pid: number }> {
    const result = await httpClient.post<any>(`${API_BASE}/square-monitor/start`)
    return result.data ?? { message: 'Unknown error', pid: 0 }
  },

  async stopSquareMonitor(): Promise<{ message: string }> {
    const result = await httpClient.post<any>(`${API_BASE}/square-monitor/stop`)
    return result.data ?? { message: 'Unknown error' }
  },
}
