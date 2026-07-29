import { useEffect, useState, useCallback } from 'react'
import { api } from '../../lib/api'
import type { SquareHeatResponse } from '../../lib/api/data'
import { t, type Language } from '../../i18n/translations'
import {
  ArrowDown,
  ArrowRight,
  Flame,
  Power,
  RadioTower,
  RefreshCw,
  Rocket,
  Sparkles,
  TrendingDown,
  TrendingUp,
} from 'lucide-react'
import { Button } from '../ui/Button'

interface SquareHeatPanelProps {
  language: Language
  refreshInterval?: number
}

function TrendIcon({ trend }: { trend: string }) {
  if (trend.includes('↑↑'))
    return <Rocket className="w-3.5 h-3.5 text-profit" />
  if (trend.includes('↑'))
    return <TrendingUp className="w-3.5 h-3.5 text-profit" />
  if (trend.includes('↓↓'))
    return <ArrowDown className="w-3.5 h-3.5 text-loss" />
  if (trend.includes('↓'))
    return <TrendingDown className="w-3.5 h-3.5 text-loss" />
  if (trend.includes('\u{1F195}'))
    return <Sparkles className="w-3.5 h-3.5 text-primary" />
  return <ArrowRight className="w-3.5 h-3.5 text-muted-foreground" />
}

function getChangeColor(val?: number): string {
  if (val == null) return 'text-muted-foreground'
  if (val > 0) return 'text-profit'
  if (val < 0) return 'text-loss'
  return 'text-muted-foreground'
}

function formatPct(val?: number): string {
  if (val == null) return '--'
  const sign = val > 0 ? '+' : ''
  return `${sign}${val.toFixed(2)}%`
}

export function SquareHeatPanel({
  language,
  refreshInterval = 30000,
}: SquareHeatPanelProps) {
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
        <div className="w-20 h-20 rounded-full bg-warning blur-3xl" />
      </div>

      {/* Header */}
      <div className="flex items-center justify-between mb-4 relative z-10">
        <h2 className="text-lg font-bold flex items-center gap-2 text-foreground uppercase tracking-wide">
          <span className="text-warning">
            <Flame size={20} />
          </span>
          Square Heat
        </h2>
        <div className="flex items-center gap-1">
          <Button
            variant="unstyled"
            onClick={toggleWorker}
            disabled={workerToggling}
            className={`p-1.5 rounded-lg transition-all ${
              workerRunning
                ? 'text-profit hover:bg-profit/20'
                : 'text-muted-foreground hover:bg-white/10 hover:text-foreground'
            } ${workerToggling ? 'opacity-50' : ''}`}
            title={workerRunning ? 'Stop Worker' : 'Start Worker'}
          >
            {workerToggling ? (
              <RefreshCw size={14} className="animate-spin" />
            ) : (
              <Power size={14} />
            )}
          </Button>
          <Button
            variant="unstyled"
            onClick={() => {
              setLoading(true)
              fetchData()
            }}
            className="p-1.5 rounded-lg transition-all hover:bg-white/10 text-muted-foreground hover:text-foreground"
            title="Refresh"
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          </Button>
        </div>
      </div>

      {/* Update time */}
      {data?.updated_at && (
        <div className="text-[10px] text-muted-foreground mb-3 relative z-10">
          {t('traderDashboard.lastUpdate', language)}:{' '}
          {new Date(data.updated_at).toLocaleTimeString()}
          {data.count > 0 && (
            <span className="ml-2">
              {data.count} {t('symbols', language)}
            </span>
          )}
        </div>
      )}

      {/* Error state */}
      {error && !loading && (
        <div className="text-center py-8 text-muted-foreground opacity-60 relative z-10">
          <RadioTower className="w-6 h-6 mx-auto mb-2 opacity-60" />
          <div className="text-xs">{error}</div>
        </div>
      )}

      {/* Loading state */}
      {loading && !data && (
        <div className="text-center py-8 text-muted-foreground relative z-10">
          <RefreshCw size={20} className="animate-spin mx-auto mb-2" />
          <div className="text-xs">Loading...</div>
        </div>
      )}

      {/* Empty state */}
      {!loading && !error && items.length === 0 && (
        <div className="text-center py-8 text-muted-foreground opacity-60 relative z-10">
          <Flame className="w-6 h-6 mx-auto mb-2 opacity-60" />
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
                <span className="text-[10px] text-muted-foreground font-mono w-4 text-right">
                  {i + 1}
                </span>
                <TrendIcon trend={item.trend} />
              </div>

              {/* Token info */}
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="font-mono font-bold text-xs text-foreground truncate">
                    {item.symbol}
                  </span>
                  {item.direction && (
                    <span
                      className={`text-[9px] px-1.5 py-0.5 rounded ${
                        item.direction.includes('多') ||
                        item.direction.includes('↑')
                          ? 'bg-profit/10 text-profit'
                          : item.direction.includes('空') ||
                              item.direction.includes('↓')
                            ? 'bg-loss/10 text-loss'
                            : 'bg-white/5 text-muted-foreground'
                      }`}
                    >
                      {item.direction}
                    </span>
                  )}
                </div>
                {item.verdict && (
                  <div className="text-[9px] text-muted-foreground truncate mt-0.5">
                    {item.verdict}
                  </div>
                )}
              </div>

              {/* Score */}
              <div className="text-right min-w-[50px]">
                <div className="font-mono font-bold text-xs text-primary">
                  {item.composite_score.toFixed(0)}
                </div>
                <div className="text-[9px] text-muted-foreground">
                  {item.mentions}M
                </div>
              </div>

              {/* Price + Change */}
              <div className="text-right min-w-[65px]">
                {item.mark_price != null && (
                  <div className="font-mono text-xs text-foreground">
                    $
                    {item.mark_price < 1
                      ? item.mark_price.toPrecision(4)
                      : item.mark_price.toFixed(2)}
                  </div>
                )}
                <div
                  className={`font-mono text-[10px] ${getChangeColor(item.change_24h_pct)}`}
                >
                  {formatPct(item.change_24h_pct)}
                </div>
              </div>

              {/* Tags */}
              {item.tags && item.tags.length > 0 && (
                <div className="hidden xl:flex flex-col gap-0.5 min-w-[60px]">
                  {item.tags.slice(0, 2).map((tag, j) => (
                    <span
                      key={j}
                      className="text-[8px] px-1 py-0.5 rounded bg-white/5 text-muted-foreground truncate"
                    >
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
