import { useEffect, useMemo, useState, useRef } from 'react'
import { motion } from 'framer-motion'
import {
  createChart,
  ColorType,
  CrosshairMode,
  CandlestickSeries,
  createSeriesMarkers,
  type IChartApi,
  type ISeriesApi,
  type CandlestickData,
  type UTCTimestamp,
  type SeriesMarker,
} from 'lightweight-charts'
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ReferenceDot,
} from 'recharts'
import {
  Clock,
  AlertTriangle,
  RefreshCw,
  CandlestickChart as CandlestickIcon,
} from 'lucide-react'
import { api } from '../../lib/api'
import { t, type Language } from '../../i18n/translations'
import type {
  BacktestEquityPoint,
  BacktestTradeEvent,
  BacktestKlinesResponse,
} from '../../types'
import { Button } from '../ui/Button'

// ============ Equity Chart (Recharts) ============

interface EquityChartProps {
  equity: BacktestEquityPoint[]
  trades: BacktestTradeEvent[]
}

export function EquityChart({ equity, trades }: EquityChartProps) {
  const chartData = useMemo(() => {
    return equity.map((point) => ({
      time: new Date(point.ts).toLocaleString(),
      ts: point.ts,
      equity: point.equity,
      pnl_pct: point.pnl_pct,
    }))
  }, [equity])

  const tradeMarkers = useMemo(() => {
    if (!trades.length || !equity.length) return []
    return trades
      .filter((t) => t.action.includes('open') || t.action.includes('close'))
      .map((trade) => {
        const closest = equity.reduce((prev, curr) =>
          Math.abs(curr.ts - trade.ts) < Math.abs(prev.ts - trade.ts)
            ? curr
            : prev
        )
        return {
          ts: closest.ts,
          equity: closest.equity,
          action: trade.action,
          symbol: trade.symbol,
          isOpen: trade.action.includes('open'),
        }
      })
      .slice(-30)
  }, [trades, equity])

  return (
    <div className="w-full h-[300px]">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart
          data={chartData}
          margin={{ top: 10, right: 10, left: 0, bottom: 0 }}
        >
          <defs>
            <linearGradient id="equityGradient" x1="0" y1="0" x2="0" y2="1">
              <stop
                offset="5%"
                stopColor="var(--color-primary)"
                stopOpacity={0.4}
              />
              <stop
                offset="95%"
                stopColor="var(--color-primary)"
                stopOpacity={0}
              />
            </linearGradient>
          </defs>
          <CartesianGrid stroke="var(--chart-grid)" strokeDasharray="3 3" />
          <XAxis
            dataKey="time"
            tick={{ fill: 'var(--color-muted-fg)', fontSize: 10 }}
            axisLine={{ stroke: 'var(--color-border)' }}
            tickLine={{ stroke: 'var(--color-border)' }}
            hide
          />
          <YAxis
            tick={{ fill: 'var(--color-muted-fg)', fontSize: 10 }}
            axisLine={{ stroke: 'var(--color-border)' }}
            tickLine={{ stroke: 'var(--color-border)' }}
            width={60}
            domain={['auto', 'auto']}
          />
          <Tooltip
            contentStyle={{
              background: 'var(--color-panel)',
              border: '1px solid var(--color-border)',
              borderRadius: 8,
              color: 'var(--color-foreground)',
            }}
            labelStyle={{ color: 'var(--color-muted-fg)' }}
            formatter={(value: number) => [`$${value.toFixed(2)}`, 'Equity']}
          />
          <Area
            type="monotone"
            dataKey="equity"
            stroke="var(--color-primary)"
            strokeWidth={2}
            fill="url(#equityGradient)"
            dot={false}
            activeDot={{ r: 4, fill: 'var(--color-primary)' }}
          />
          {tradeMarkers.map((marker, idx) => (
            <ReferenceDot
              key={`${marker.ts}-${idx}`}
              x={chartData.findIndex((d) => d.ts === marker.ts)}
              y={marker.equity}
              r={4}
              fill={marker.isOpen ? 'var(--color-profit)' : 'var(--color-loss)'}
              stroke={
                marker.isOpen ? 'var(--color-profit)' : 'var(--color-loss)'
              }
            />
          ))}
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}

// ============ Candlestick Chart with Trade Markers ============

interface CandlestickChartProps {
  runId: string
  trades: BacktestTradeEvent[]
  language: Language
}

export function CandlestickChartComponent({
  runId,
  trades,
  language,
}: CandlestickChartProps) {
  const chartContainerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<IChartApi | null>(null)
  const candleSeriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null)

  const symbols = useMemo(() => {
    const symbolSet = new Set(trades.map((t) => t.symbol))
    return Array.from(symbolSet).sort()
  }, [trades])

  const [selectedSymbol, setSelectedSymbol] = useState<string>(symbols[0] || '')
  const [selectedTimeframe, setSelectedTimeframe] = useState<string>('15m')
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const CHART_TIMEFRAMES = ['1m', '3m', '5m', '15m', '30m', '1h', '4h', '1d']

  useEffect(() => {
    if (symbols.length > 0 && !symbols.includes(selectedSymbol)) {
      setSelectedSymbol(symbols[0])
    }
  }, [symbols, selectedSymbol])

  const symbolTrades = useMemo(() => {
    return trades.filter((t) => t.symbol === selectedSymbol)
  }, [trades, selectedSymbol])

  useEffect(() => {
    if (!chartContainerRef.current || !selectedSymbol || !runId) return

    const container = chartContainerRef.current

    const chart = createChart(container, {
      layout: {
        background: { type: ColorType.Solid, color: 'var(--color-background)' },
        textColor: 'var(--color-muted-fg)',
      },
      grid: {
        vertLines: { color: 'var(--chart-grid)' },
        horzLines: { color: 'var(--chart-grid)' },
      },
      crosshair: {
        mode: CrosshairMode.Normal,
      },
      rightPriceScale: {
        borderColor: 'var(--color-border)',
      },
      timeScale: {
        borderColor: 'var(--color-border)',
        timeVisible: true,
        secondsVisible: false,
      },
      width: container.clientWidth,
      height: 400,
    })

    chartRef.current = chart

    const candleSeries = chart.addSeries(CandlestickSeries, {
      upColor: 'var(--color-profit)',
      downColor: 'var(--color-loss)',
      borderUpColor: 'var(--color-profit)',
      borderDownColor: 'var(--color-loss)',
      wickUpColor: 'var(--color-profit)',
      wickDownColor: 'var(--color-loss)',
    })
    candleSeriesRef.current = candleSeries

    setIsLoading(true)
    setError(null)

    api
      .getBacktestKlines(runId, selectedSymbol, selectedTimeframe)
      .then((data: BacktestKlinesResponse) => {
        const klineData: CandlestickData<UTCTimestamp>[] = data.klines.map(
          (k) => ({
            time: k.time as UTCTimestamp,
            open: k.open,
            high: k.high,
            low: k.low,
            close: k.close,
          })
        )
        candleSeries.setData(klineData)

        const markers: SeriesMarker<UTCTimestamp>[] = symbolTrades
          .map((trade) => {
            const tradeTime = Math.floor(trade.ts / 1000)
            const closestKline = data.klines.reduce((prev, curr) =>
              Math.abs(curr.time - tradeTime) < Math.abs(prev.time - tradeTime)
                ? curr
                : prev
            )
            const isOpen = trade.action.includes('open')
            const isLong =
              trade.side === 'long' || trade.action.includes('long')
            const pnl = trade.realized_pnl

            let text = ''
            let color = 'var(--color-profit)'

            if (isOpen) {
              if (isLong) {
                text = `▲ Long @${trade.price.toFixed(2)}`
                color = 'var(--color-profit)'
              } else {
                text = `▼ Short @${trade.price.toFixed(2)}`
                color = 'var(--color-loss)'
              }
            } else {
              const pnlStr =
                pnl >= 0
                  ? `+$${pnl.toFixed(2)}`
                  : `-$${Math.abs(pnl).toFixed(2)}`
              text = `× ${pnlStr}`
              color = pnl >= 0 ? 'var(--color-profit)' : 'var(--color-loss)'
            }

            return {
              time: closestKline.time as UTCTimestamp,
              position: isOpen
                ? isLong
                  ? ('belowBar' as const)
                  : ('aboveBar' as const)
                : isLong
                  ? ('aboveBar' as const)
                  : ('belowBar' as const),
              color,
              shape: 'circle' as const,
              size: 2,
              text,
            }
          })
          .sort((a, b) => (a.time as number) - (b.time as number))

        createSeriesMarkers(candleSeries, markers)
        chart.timeScale().fitContent()
        setIsLoading(false)
      })
      .catch((err) => {
        setError(err.message || 'Failed to load klines')
        setIsLoading(false)
      })

    const handleResize = () => {
      if (chartContainerRef.current) {
        chart.applyOptions({ width: chartContainerRef.current.clientWidth })
      }
    }
    window.addEventListener('resize', handleResize)

    return () => {
      window.removeEventListener('resize', handleResize)
      chart.remove()
      chartRef.current = null
      candleSeriesRef.current = null
    }
  }, [runId, selectedSymbol, selectedTimeframe, symbolTrades])

  if (symbols.length === 0) {
    return (
      <div
        className="py-12 text-center"
        style={{ color: 'var(--color-muted-fg)' }}
      >
        {t('backtestChart.noTrades', language)}
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-4 flex-wrap">
        <div className="flex items-center gap-2">
          <CandlestickIcon
            size={16}
            style={{ color: 'var(--color-primary)' }}
          />
          <span className="text-sm text-muted-foreground">
            {t('backtestChart.symbol', language)}
          </span>
          <select
            value={selectedSymbol}
            onChange={(e) => setSelectedSymbol(e.target.value)}
            className="px-3 py-1.5 rounded text-sm"
            style={{
              background: 'var(--color-panel)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
            }}
          >
            {symbols.map((sym) => (
              <option key={sym} value={sym}>
                {sym}
              </option>
            ))}
          </select>
        </div>

        <div className="flex items-center gap-2">
          <Clock size={14} className="text-muted-foreground" />
          <span className="text-sm text-muted-foreground">
            {t('backtestChart.interval', language)}
          </span>
          <div
            className="flex rounded overflow-hidden"
            style={{ border: '1px solid var(--color-border)' }}
          >
            {CHART_TIMEFRAMES.map((tf) => (
              <Button
                variant="unstyled"
                key={tf}
                onClick={() => setSelectedTimeframe(tf)}
                className="px-2.5 py-1 text-xs font-medium transition-colors"
                style={{
                  background:
                    selectedTimeframe === tf
                      ? 'var(--color-primary)'
                      : 'var(--color-panel)',
                  color:
                    selectedTimeframe === tf
                      ? 'var(--color-background)'
                      : 'var(--color-muted-fg)',
                }}
              >
                {tf}
              </Button>
            ))}
          </div>
        </div>

        <span className="text-xs" style={{ color: 'var(--color-muted-fg)' }}>
          ({symbolTrades.length} {t('backtestChart.trades', language)})
        </span>
      </div>

      <div
        ref={chartContainerRef}
        className="w-full rounded-lg overflow-hidden"
        style={{ background: 'var(--color-background)', minHeight: 400 }}
      >
        {isLoading && (
          <div className="flex items-center justify-center h-[400px] text-muted-foreground">
            <RefreshCw className="animate-spin mr-2" size={16} />
            {t('backtestChart.loadingKline', language)}
          </div>
        )}
        {error && (
          <div
            className="flex items-center justify-center h-[400px]"
            style={{ color: 'var(--color-loss)' }}
          >
            <AlertTriangle className="mr-2" size={16} />
            {error}
          </div>
        )}
      </div>

      <div className="flex items-center gap-4 text-xs text-muted-foreground">
        <div className="flex items-center gap-1.5">
          <div
            className="w-2.5 h-2.5 rounded-full"
            style={{ background: 'var(--color-profit)' }}
          />
          <span>{t('backtestChart.openProfit', language)}</span>
        </div>
        <div className="flex items-center gap-1.5">
          <div
            className="w-2.5 h-2.5 rounded-full"
            style={{ background: 'var(--color-loss)' }}
          />
          <span>{t('backtestChart.lossClose', language)}</span>
        </div>
        <span style={{ color: 'var(--color-muted-fg)' }}>|</span>
        <span>▲ Long · ▼ Short · × {t('backtestChart.close', language)}</span>
      </div>
    </div>
  )
}

// ============ Chart Tab Content ============

interface BacktestChartTabProps {
  equity: BacktestEquityPoint[] | undefined
  trades: BacktestTradeEvent[] | undefined
  selectedRunId: string
  language: Language
  tr: (key: string) => string
}

export function BacktestChartTab({
  equity,
  trades,
  selectedRunId,
  language,
  tr,
}: BacktestChartTabProps) {
  return (
    <motion.div
      key="chart"
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      className="space-y-6"
    >
      <div>
        <h4 className="text-sm font-medium mb-3 text-foreground">
          {t('backtestChart.equityCurve', language)}
        </h4>
        {equity && equity.length > 0 ? (
          <EquityChart equity={equity} trades={trades ?? []} />
        ) : (
          <div
            className="py-12 text-center"
            style={{ color: 'var(--color-muted-fg)' }}
          >
            {tr('charts.equityEmpty')}
          </div>
        )}
      </div>

      {selectedRunId && trades && trades.length > 0 && (
        <div>
          <h4 className="text-sm font-medium mb-3 text-foreground">
            {t('backtestChart.candlestickTradeMarkers', language)}
          </h4>
          <CandlestickChartComponent
            runId={selectedRunId}
            trades={trades}
            language={language}
          />
        </div>
      )}
    </motion.div>
  )
}
