import { useEffect, useState, useCallback } from 'react'
import { api } from '../../lib/api'
import type { SquareHeatResponse } from '../../lib/api/data'
import { t, type Language } from '../../i18n/translations'
import { Flame, RefreshCw, Power } from 'lucide-react'

interface SquareHeatPanelProps {
  language: Language
  refreshInterval?: number
}

function getTrendIcon(trend: string): string {
  if (trend.includes('↑↑')) return '🚀'
  if (trend.includes('↑')) return '📈'
  if (trend.includes('↓↓')) return '🔻'
  if (trend.includes('↓')) return '📉'
  if (trend.includes('🆕')) return '🌟'
  return '➡️'
}

function getChangeColor(val?: number): string {
  if (val == null) return 'text-ait-text-muted'
  if (val > 0) return 'text-ait-green'
  if (val < 0) return 'text-ait-red'
  return 'text-ait-text-muted'
}

function formatPct(val?: number): string {
  if (val == null) return '--'
  const sign = val > 0 ? '+' : ''
  return `${sign}${val.toFixed(2)}%`
}

export function SquareHeatPanel({ language, refreshInterval = 30000 }: SquareHeatPanelProps) {
  const [data, setData] = useState<SquareHeatResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [workerRunning, setWorkerRunning] = useState(false)
  const [workerToggling, setWorkerToggling] = useState(false)

  const fetchData = useCallback(async () => {
    try {
      const res = await api.getSquareHeat()
      setData(res)
      setError(res.error || null)
    } catch {
      setError('Failed to fetch Square Heat')
    } finally {
      setLoading(false)
    }
  }, [])

  const fetchWorkerStatus = useCallback(async () => {
    try {
      const status = await api.getSquareMonitorStatus()
      setWorkerRunning(status.running)
    } catch {
      setWorkerRunning(false)
    }
  }, [])

  const toggleWorker = useCallback(async () => {
    setWorkerToggling(true)
    try {
      if (workerRunning) {
        await api.stopSquareMonitor()
        setWorkerRunning(false)
      } else {
        await api.startSquareMonitor()
        setWorkerRunning(true)
      }
    } catch {
      // re-fetch actual status on error
      await fetchWorkerStatus()
    } finally {
      setWorkerToggling(false)
    }
  }, [workerRunning, fetchWorkerStatus])

  useEffect(() => {
    fetchData()
    fetchWorkerStatus()
    const timer = setInterval(fetchData, refreshInterval)
    return () => clearInterval(timer)
  }, [fetchData, fetchWorkerStatus, refreshInterval])

  const items = data?.items ?? []

  return (
    <div
      className="ait-glass p-6 animate-slide-in relative overflow-hidden group"
      style={{ animationDelay: '0.18s' }}
    >
      <div className="absolute top-0 right-0 p-3 opacity-10 group-hover:opacity-20 transition-opacity">
        <div className="w-20 h-20 rounded-full bg-orange-500 blur-3xl" />
      </div>

      {/* Header */}
      <div className="flex items-center justify-between mb-4 relative z-10">
        <h2 className="text-lg font-bold flex items-center gap-2 text-ait-text-main uppercase tracking-wide">
          <span className="text-orange-500">
            <Flame size={20} />
          </span>
          Square Heat
        </h2>
        <div className="flex items-center gap-1">
          <button
            onClick={toggleWorker}
            disabled={workerToggling}
            className={`p-1.5 rounded-lg transition-all ${
              workerRunning
                ? 'text-green-400 hover:bg-green-500/20'
                : 'text-ait-text-muted hover:bg-white/10 hover:text-white'
            } ${workerToggling ? 'opacity-50' : ''}`}
            title={workerRunning ? 'Stop Worker' : 'Start Worker'}
          >
            {workerToggling ? (
              <RefreshCw size={14} className="animate-spin" />
            ) : (
              <Power size={14} />
            )}
          </button>
          <button
            onClick={() => { setLoading(true); fetchData() }}
            className="p-1.5 rounded-lg transition-all hover:bg-white/10 text-ait-text-muted hover:text-white"
            title="Refresh"
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          </button>
        </div>
      </div>

      {/* Update time */}
      {data?.updated_at && (
        <div className="text-[10px] text-ait-text-muted mb-3 relative z-10">
          {t('traderDashboard.lastUpdate', language)}: {new Date(data.updated_at).toLocaleTimeString()}
          {data.count > 0 && <span className="ml-2">{data.count} {t('symbols', language)}</span>}
        </div>
      )}

      {/* Error state */}
      {error && !loading && (
        <div className="text-center py-8 text-ait-text-muted opacity-60 relative z-10">
          <div className="text-3xl mb-2">📡</div>
          <div className="text-xs">{error}</div>
        </div>
      )}

      {/* Loading state */}
      {loading && !data && (
        <div className="text-center py-8 text-ait-text-muted relative z-10">
          <RefreshCw size={20} className="animate-spin mx-auto mb-2" />
          <div className="text-xs">Loading...</div>
        </div>
      )}

      {/* Empty state */}
      {!loading && !error && items.length === 0 && (
        <div className="text-center py-8 text-ait-text-muted opacity-60 relative z-10">
          <div className="text-3xl mb-2">🔥</div>
          <div className="text-xs">No heat signals</div>
          <div className="text-[10px] mt-1">Square Monitor may be offline</div>
        </div>
      )}

      {/* Items list */}
      {items.length > 0 && (
        <div className="space-y-2 relative z-10 max-h-[420px] overflow-y-auto scrollbar-thin">
          {items.map((item, i) => (
            <div
              key={item.token + i}
              className="flex items-center gap-3 px-3 py-2.5 rounded-lg bg-white/5 hover:bg-white/10 transition-all group/item cursor-default"
            >
              {/* Rank + Trend */}
              <div className="flex items-center gap-1.5 min-w-[36px]">
                <span className="text-[10px] text-ait-text-muted font-mono w-4 text-right">{i + 1}</span>
                <span className="text-sm">{getTrendIcon(item.trend)}</span>
              </div>

              {/* Token info */}
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="font-mono font-bold text-xs text-ait-text-main truncate">
                    {item.symbol}
                  </span>
                  {item.direction && (
                    <span className={`text-[9px] px-1.5 py-0.5 rounded ${
                      item.direction.includes('多') || item.direction.includes('↑')
                        ? 'bg-ait-green/10 text-ait-green'
                        : item.direction.includes('空') || item.direction.includes('↓')
                          ? 'bg-ait-red/10 text-ait-red'
                          : 'bg-white/5 text-ait-text-muted'
                    }`}>
                      {item.direction}
                    </span>
                  )}
                </div>
                {item.verdict && (
                  <div className="text-[9px] text-ait-text-muted truncate mt-0.5">
                    {item.verdict}
                  </div>
                )}
              </div>

              {/* Score */}
              <div className="text-right min-w-[50px]">
                <div className="font-mono font-bold text-xs text-ait-gold">
                  {item.composite_score.toFixed(0)}
                </div>
                <div className="text-[9px] text-ait-text-muted">
                  {item.mentions}M
                </div>
              </div>

              {/* Price + Change */}
              <div className="text-right min-w-[65px]">
                {item.mark_price != null && (
                  <div className="font-mono text-xs text-ait-text-main">
                    ${item.mark_price < 1 ? item.mark_price.toPrecision(4) : item.mark_price.toFixed(2)}
                  </div>
                )}
                <div className={`font-mono text-[10px] ${getChangeColor(item.change_24h_pct)}`}>
                  {formatPct(item.change_24h_pct)}
                </div>
              </div>

              {/* Tags */}
              {item.tags && item.tags.length > 0 && (
                <div className="hidden xl:flex flex-col gap-0.5 min-w-[60px]">
                  {item.tags.slice(0, 2).map((tag, j) => (
                    <span key={j} className="text-[8px] px-1 py-0.5 rounded bg-white/5 text-ait-text-muted truncate">
                      {tag}
                    </span>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
