import { motion } from 'framer-motion'
import {
  TrendingUp,
  TrendingDown,
  Activity,
  ArrowUpRight,
  ArrowDownRight,
} from 'lucide-react'
import { MetricTooltip } from '../common/MetricTooltip'
import { t, type Language } from '../../i18n/translations'
import { EquityChart } from './BacktestChartTab'
import type {
  BacktestEquityPoint,
  BacktestTradeEvent,
  BacktestMetrics,
  BacktestPositionStatus,
} from '../../types'

// ============ Stat Card ============

interface StatCardProps {
  icon: typeof TrendingUp
  label: string
  value: string | number
  suffix?: string
  trend?: 'up' | 'down' | 'neutral'
  color?: string
  metricKey?: string
  language?: string
}

export function StatCard({
  icon: Icon,
  label,
  value,
  suffix,
  trend,
  color = 'var(--color-foreground)',
  metricKey,
  language = 'en',
}: StatCardProps) {
  const trendColors = {
    up: 'var(--color-profit)',
    down: 'var(--color-loss)',
    neutral: 'var(--color-muted-fg)',
  }

  return (
    <div
      className="p-4 rounded-xl"
      style={{
        background: 'var(--color-panel)',
        border: '1px solid var(--color-border)',
      }}
    >
      <div className="flex items-center gap-2 mb-2">
        <Icon className="w-4 h-4" style={{ color: 'var(--color-primary)' }} />
        <span className="text-xs text-muted-foreground">{label}</span>
        {metricKey && (
          <MetricTooltip metricKey={metricKey} language={language} size={12} />
        )}
      </div>
      <div className="flex items-baseline gap-1">
        <span className="text-xl font-bold" style={{ color }}>
          {value}
        </span>
        {suffix && (
          <span className="text-xs text-muted-foreground">{suffix}</span>
        )}
        {trend && trend !== 'neutral' && (
          <span style={{ color: trendColors[trend] }}>
            {trend === 'up' ? (
              <ArrowUpRight className="w-4 h-4" />
            ) : (
              <ArrowDownRight className="w-4 h-4" />
            )}
          </span>
        )}
      </div>
    </div>
  )
}

// ============ Progress Ring ============

interface ProgressRingProps {
  progress: number
  size?: number
}

export function ProgressRing({ progress, size = 120 }: ProgressRingProps) {
  const strokeWidth = 8
  const radius = (size - strokeWidth) / 2
  const circumference = radius * 2 * Math.PI
  const offset = circumference - (progress / 100) * circumference

  return (
    <div className="relative" style={{ width: size, height: size }}>
      <svg className="transform -rotate-90" width={size} height={size}>
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          stroke="var(--color-border)"
          strokeWidth={strokeWidth}
          fill="none"
        />
        <motion.circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          stroke="var(--color-primary)"
          strokeWidth={strokeWidth}
          fill="none"
          strokeLinecap="round"
          strokeDasharray={circumference}
          initial={{ strokeDashoffset: circumference }}
          animate={{ strokeDashoffset: offset }}
          transition={{ duration: 0.5 }}
        />
      </svg>
      <div className="absolute inset-0 flex items-center justify-center flex-col">
        <span
          className="text-2xl font-bold"
          style={{ color: 'var(--color-primary)' }}
        >
          {progress.toFixed(0)}%
        </span>
        <span className="text-xs text-muted-foreground">Complete</span>
      </div>
    </div>
  )
}

// ============ Positions Display ============

interface PositionsDisplayProps {
  positions: BacktestPositionStatus[]
  language: Language
}

export function PositionsDisplay({
  positions,
  language,
}: PositionsDisplayProps) {
  if (!positions || positions.length === 0) {
    return null
  }

  const totalUnrealizedPnL = positions.reduce(
    (sum, p) => sum + p.unrealized_pnl,
    0
  )
  const totalMargin = positions.reduce((sum, p) => sum + p.margin_used, 0)

  return (
    <div
      className="mt-3 p-3 rounded-lg"
      style={{
        background: 'var(--color-panel)',
        border: '1px solid var(--color-border)',
      }}
    >
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2">
          <Activity
            className="w-4 h-4"
            style={{ color: 'var(--color-primary)' }}
          />
          <span className="text-sm font-medium text-foreground">
            {t('backtestOverview.activePositions', language)}
          </span>
          <span
            className="px-1.5 py-0.5 rounded text-xs"
            style={{
              background:
                'color-mix(in srgb, var(--color-primary) 12.5%, transparent)',
              color: 'var(--color-primary)',
            }}
          >
            {positions.length}
          </span>
        </div>
        <div className="flex items-center gap-3 text-xs">
          <span className="text-muted-foreground">
            {t('backtestOverview.margin', language)}: ${totalMargin.toFixed(2)}
          </span>
          <span
            className="font-medium"
            style={{
              color:
                totalUnrealizedPnL >= 0
                  ? 'var(--color-profit)'
                  : 'var(--color-loss)',
            }}
          >
            {t('backtestOverview.unrealized', language)}:{' '}
            {totalUnrealizedPnL >= 0 ? '+' : ''}${totalUnrealizedPnL.toFixed(2)}
          </span>
        </div>
      </div>

      <div className="space-y-1.5">
        {positions.map((pos) => {
          const isLong = pos.side === 'long'
          const pnlColor =
            pos.unrealized_pnl >= 0
              ? 'var(--color-profit)'
              : 'var(--color-loss)'

          return (
            <motion.div
              key={`${pos.symbol}-${pos.side}`}
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              className="flex items-center justify-between p-2 rounded"
              style={{ background: 'var(--color-panel)' }}
            >
              <div className="flex items-center gap-2">
                <div
                  className="w-6 h-6 rounded flex items-center justify-center"
                  style={{
                    background: isLong
                      ? 'color-mix(in srgb, var(--color-profit) 12.5%, transparent)'
                      : 'color-mix(in srgb, var(--color-loss) 12.5%, transparent)',
                  }}
                >
                  {isLong ? (
                    <TrendingUp
                      className="w-3.5 h-3.5"
                      style={{ color: 'var(--color-profit)' }}
                    />
                  ) : (
                    <TrendingDown
                      className="w-3.5 h-3.5"
                      style={{ color: 'var(--color-loss)' }}
                    />
                  )}
                </div>
                <div>
                  <div className="flex items-center gap-1.5">
                    <span className="font-mono font-bold text-sm text-foreground">
                      {pos.symbol.replace('USDT', '')}
                    </span>
                    <span
                      className="px-1 py-0.5 rounded text-[10px] font-medium"
                      style={{
                        background: isLong
                          ? 'color-mix(in srgb, var(--color-profit) 12.5%, transparent)'
                          : 'color-mix(in srgb, var(--color-loss) 12.5%, transparent)',
                        color: isLong
                          ? 'var(--color-profit)'
                          : 'var(--color-loss)',
                      }}
                    >
                      {isLong ? 'LONG' : 'SHORT'} {pos.leverage}x
                    </span>
                  </div>
                  <div
                    className="text-[10px]"
                    style={{ color: 'var(--color-muted-fg)' }}
                  >
                    {t('backtestOverview.qty', language)}:{' '}
                    {pos.quantity.toFixed(4)} ·{' '}
                    {t('backtestOverview.margin', language)}: $
                    {pos.margin_used.toFixed(2)}
                  </div>
                </div>
              </div>

              <div className="text-right">
                <div className="flex items-center gap-2 text-xs">
                  <span className="text-muted-foreground">
                    {t('backtestOverview.entry', language)}: $
                    {pos.entry_price.toFixed(2)}
                  </span>
                  <span className="text-foreground">
                    {t('backtestOverview.mark', language)}: $
                    {pos.mark_price.toFixed(2)}
                  </span>
                </div>
                <div className="flex items-center justify-end gap-1.5 mt-0.5">
                  <span
                    className="font-mono font-bold"
                    style={{ color: pnlColor }}
                  >
                    {pos.unrealized_pnl >= 0 ? '+' : ''}$
                    {pos.unrealized_pnl.toFixed(2)}
                  </span>
                  <span
                    className="px-1 py-0.5 rounded text-[10px] font-medium"
                    style={{
                      background: `color-mix(in srgb, ${pnlColor} 12.5%, transparent)`,
                      color: pnlColor,
                    }}
                  >
                    {pos.unrealized_pnl_pct >= 0 ? '+' : ''}
                    {pos.unrealized_pnl_pct.toFixed(2)}%
                  </span>
                </div>
              </div>
            </motion.div>
          )
        })}
      </div>
    </div>
  )
}

// ============ Overview Tab Content ============

interface BacktestOverviewTabProps {
  equity: BacktestEquityPoint[] | undefined
  trades: BacktestTradeEvent[] | undefined
  metrics: BacktestMetrics | undefined
  language: Language
  tr: (key: string) => string
}

export function BacktestOverviewTab({
  equity,
  trades,
  metrics,
  language,
  tr,
}: BacktestOverviewTabProps) {
  return (
    <motion.div
      key="overview"
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
    >
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

      {metrics && (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mt-4">
          <div
            className="p-3 rounded-lg"
            style={{ background: 'var(--color-panel)' }}
          >
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              {t('backtestOverview.winRate', language)}
              <MetricTooltip
                metricKey="win_rate"
                language={language}
                size={11}
              />
            </div>
            <div className="text-lg font-bold text-foreground">
              {(metrics.win_rate ?? 0).toFixed(1)}%
            </div>
          </div>
          <div
            className="p-3 rounded-lg"
            style={{ background: 'var(--color-panel)' }}
          >
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              {t('backtestOverview.profitFactor', language)}
              <MetricTooltip
                metricKey="profit_factor"
                language={language}
                size={11}
              />
            </div>
            <div className="text-lg font-bold text-foreground">
              {(metrics.profit_factor ?? 0).toFixed(2)}
            </div>
          </div>
          <div
            className="p-3 rounded-lg"
            style={{ background: 'var(--color-panel)' }}
          >
            <div className="text-xs text-muted-foreground">
              {t('backtestOverview.totalTrades', language)}
            </div>
            <div className="text-lg font-bold text-foreground">
              {metrics.trades ?? 0}
            </div>
          </div>
          <div
            className="p-3 rounded-lg"
            style={{ background: 'var(--color-panel)' }}
          >
            <div className="text-xs text-muted-foreground">
              {t('backtestOverview.bestSymbol', language)}
            </div>
            <div
              className="text-lg font-bold"
              style={{ color: 'var(--color-profit)' }}
            >
              {metrics.best_symbol?.replace('USDT', '') || '-'}
            </div>
          </div>
        </div>
      )}
    </motion.div>
  )
}
