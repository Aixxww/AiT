/**
 * Hunter v7 signal panel (P1.2) — read-only view over
 * GET /api/hunter/v7/signals.
 *
 * Information architecture (design proposal §5.3):
 * - Tier expresses signal QUALITY via the gold/cyan/gray/dim ladder and is
 *   orthogonal to direction: profit/loss green/red are forbidden on tier
 *   surfaces. Direction is its own LONG/SHORT badge (the only place the
 *   semantic green/red is allowed here).
 * - EXECUTABLE / REVIEWABLE render as expanded cards: score quad, entry
 *   zone position bar, confirmation state, taker-ladder chips.
 * - WATCH renders as dim one-line summary rows.
 * - REJECTED collapses to a count plus aggregated veto-reason chips.
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from '../../lib/api'
import type {
  V7LatestCycleSummary,
  V7Signal,
  V7SignalRow,
  V7Tier,
} from '../../lib/api/hunter'
import { SignalTierBadge, type SignalTier } from './SignalTierBadge'
import { VetoChip } from './VetoChip'
import { tagTooltip, useTagCatalog } from '../../lib/tagCatalog'
import { t, type Language } from '../../i18n/translations'
import { formatPrice } from '../../utils/format'
import { IconButton } from '../ui/IconButton'
import {
  Check,
  ChevronDown,
  ChevronRight,
  Radar,
  RefreshCw,
  ShieldCheck,
  WifiOff,
  X,
} from 'lucide-react'

export const TIER_ORDER: V7Tier[] = [
  'EXECUTABLE',
  'REVIEWABLE',
  'WATCH',
  'REJECTED',
]

/** Rows from the newest cycle only, grouped by tier in display order. */
export function groupLatestCycleByTier(
  rows: V7SignalRow[]
): Record<V7Tier, V7SignalRow[]> {
  const grouped: Record<V7Tier, V7SignalRow[]> = {
    EXECUTABLE: [],
    REVIEWABLE: [],
    WATCH: [],
    REJECTED: [],
  }
  if (rows.length === 0) return grouped
  // Rows arrive newest-first. Validator smoke runs may reuse cycle_number=1,
  // so timestamp is the stable latest-cycle boundary for dashboard grouping.
  const latestTimestamp = rows[0].timestamp
  for (const row of rows) {
    if (row.timestamp !== latestTimestamp) continue
    const tier = (row.execution_tier || 'REJECTED') as V7Tier
    if (grouped[tier]) grouped[tier].push(row)
  }
  return grouped
}

/** Unified flow_taker_buy_* / flow_taker_sell_* ladder codes. */
export function takerLadderCodes(signal: V7Signal): string[] {
  return (signal.reason_codes ?? []).filter(
    (code) =>
      code.startsWith('flow_taker_buy_') || code.startsWith('flow_taker_sell_')
  )
}

/** Zone position in percent (0-100), or null when unknown (-1 sentinel). */
export function zonePositionPct(signal: V7Signal): number | null {
  const pos =
    signal.execution_readiness?.entry_zone_position ??
    signal.confirmation_summary?.entry_zone_position
  if (pos === undefined || pos === null || pos < 0) return null
  return Math.min(100, Math.max(0, pos))
}

export interface SignalSummary {
  total: number
  actionable: number
  openRate: number
  executable: number
  reviewable: number
  watch: number
  rejected: number
  tracked: number
  active: number
  protected: number
  wins: number
  stops: number
  ambiguous: number
  avgPnl: number
}

export function buildSignalSummary(
  grouped: Record<V7Tier, V7SignalRow[]>
): SignalSummary {
  const rows = TIER_ORDER.flatMap((tier) => grouped[tier] ?? [])
  const actionable =
    (grouped.EXECUTABLE?.length ?? 0) + (grouped.REVIEWABLE?.length ?? 0)
  const tracked = rows.filter((row) => row.track_status)
  // ACTIVE and same-candle TP/SL paths are not completed trade evidence. Keep
  // them visible, but do not let them bias the panel's outcome average.
  const pnlRows = tracked.filter(
    (row) =>
      Number.isFinite(row.track_pnl_pct) &&
      row.track_status !== 'ACTIVE' &&
      row.track_status !== 'BOTH_SAME_1M'
  )
  const avgPnl =
    pnlRows.length > 0
      ? pnlRows.reduce((sum, row) => sum + row.track_pnl_pct, 0) /
        pnlRows.length
      : 0
  return {
    total: rows.length,
    actionable,
    openRate: rows.length > 0 ? (actionable / rows.length) * 100 : 0,
    executable: grouped.EXECUTABLE?.length ?? 0,
    reviewable: grouped.REVIEWABLE?.length ?? 0,
    watch: grouped.WATCH?.length ?? 0,
    rejected: grouped.REJECTED?.length ?? 0,
    tracked: tracked.length,
    active: tracked.filter((row) => row.track_status === 'ACTIVE').length,
    protected: tracked.filter((row) => row.track_status === 'PROTECTED_STOP')
      .length,
    wins: tracked.filter((row) => row.track_status?.startsWith('WIN_')).length,
    stops: tracked.filter((row) => row.track_status === 'STOP').length,
    ambiguous: tracked.filter((row) => row.track_status === 'BOTH_SAME_1M')
      .length,
    avgPnl,
  }
}

function copy(language: Language, en: string, zh: string) {
  return language === 'zh' ? zh : en
}

function DirectionBadge({ direction }: { direction: 'LONG' | 'SHORT' }) {
  // Direction is the one place semantic green/red is allowed in this panel.
  return (
    <span
      className={`inline-flex items-center rounded px-1.5 py-0.5 font-mono text-[10px] font-bold uppercase tracking-wider ${
        direction === 'LONG'
          ? 'bg-profit/10 text-profit'
          : 'bg-loss/10 text-loss'
      }`}
    >
      {direction}
    </span>
  )
}

function SummaryCell({
  label,
  value,
  tone = 'default',
}: {
  label: string
  value: string
  tone?: 'default' | 'action' | 'profit' | 'loss'
}) {
  const toneClass =
    tone === 'action'
      ? 'text-tier-executable'
      : tone === 'profit'
        ? 'text-profit'
        : tone === 'loss'
          ? 'text-loss'
          : 'text-foreground'
  return (
    <div className="min-w-0 px-3 py-2">
      <div className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
        {label}
      </div>
      <div
        className={`mt-0.5 font-mono text-sm font-semibold tabular-nums ${toneClass}`}
      >
        {value}
      </div>
    </div>
  )
}

function SignalSummaryStrip({
  summary,
  language,
}: {
  summary: SignalSummary
  language: Language
}) {
  if (summary.total === 0) return null
  const pnlTone =
    summary.avgPnl > 0 ? 'profit' : summary.avgPnl < 0 ? 'loss' : 'default'
  return (
    <div className="grid grid-cols-2 overflow-hidden rounded border border-border/60 bg-surface/30 md:grid-cols-4 xl:grid-cols-6">
      <SummaryCell
        label={t('v7Signals.summaryOpenRate', language)}
        value={`${summary.openRate.toFixed(1)}%`}
        tone={summary.actionable > 0 ? 'action' : 'default'}
      />
      <SummaryCell
        label={t('v7Signals.summaryActionable', language)}
        value={`${summary.actionable}/${summary.total}`}
        tone={summary.actionable > 0 ? 'action' : 'default'}
      />
      <SummaryCell
        label={t('v7Signals.summaryWatch', language)}
        value={String(summary.watch)}
      />
      <SummaryCell
        label={t('v7Signals.summaryRejected', language)}
        value={String(summary.rejected)}
      />
      <SummaryCell
        label={t('v7Signals.summaryOutcomes', language)}
        value={`${summary.protected + summary.wins}/${summary.stops}`}
        tone={
          summary.stops > summary.protected + summary.wins ? 'loss' : 'profit'
        }
      />
      <SummaryCell
        label={t('v7Signals.summaryAvgPnl', language)}
        value={`${summary.avgPnl >= 0 ? '+' : ''}${summary.avgPnl.toFixed(2)}%`}
        tone={pnlTone}
      />
      {summary.ambiguous > 0 && (
        <SummaryCell
          label={copy(language, 'AMBIGUOUS', '同K线歧义')}
          value={String(summary.ambiguous)}
        />
      )}
    </div>
  )
}

/**
 * A compact, deliberately bounded funnel: it explains the persisted latest
 * cycle rather than pretending to be the full exchange universe. Full-universe
 * and REST-quality metrics require the forthcoming cycle-metadata endpoint.
 */
function SignalFunnel({
  summary,
  cycle,
  language,
}: {
  summary: SignalSummary
  cycle?: V7LatestCycleSummary
  language: Language
}) {
  const tierCount = (tier: V7Tier, fallback: number) =>
    cycle?.tier_counts?.[tier] ?? fallback
  const steps = [
    [
      copy(language, 'PERSISTED', '已持久化'),
      cycle?.persisted_records ?? summary.total,
      'text-foreground',
    ],
    [
      copy(language, 'OPEN REVIEW', '开仓复核'),
      summary.actionable,
      'text-tier-reviewable',
    ],
    [
      copy(language, 'WATCH', '等待确认'),
      tierCount('WATCH', summary.watch),
      'text-tier-watch',
    ],
    [
      copy(language, 'VETOED', '已否决'),
      tierCount('REJECTED', summary.rejected),
      'text-tier-rejected',
    ],
  ] as const
  return (
    <div className="rounded-lg border border-border/60 bg-surface/35 px-3 py-2.5">
      <div className="mb-2 flex items-center justify-between gap-3">
        <span className="font-mono text-[10px] font-semibold tracking-[0.14em] text-muted-foreground">
          {copy(language, 'LATEST-CYCLE SIGNAL FUNNEL', '最新轮次信号漏斗')}
        </span>
        <span className="text-[10px] text-muted-foreground">
          {copy(language, 'persisted cycle only', '仅已持久化轮次')}
        </span>
      </div>
      <div className="grid grid-cols-2 gap-px overflow-hidden rounded border border-border/40 bg-border/40 sm:grid-cols-4">
        {steps.map(([label, value, tone]) => (
          <div key={label} className="bg-panel/75 px-2.5 py-2">
            <div className="font-mono text-[9px] tracking-wider text-muted-foreground">
              {label}
            </div>
            <div
              className={`mt-0.5 font-mono text-base font-semibold tabular-nums ${tone}`}
            >
              {value}
            </div>
          </div>
        ))}
      </div>
      {(cycle?.top_vetoes?.length ?? 0) > 0 && (
        <div className="mt-2 flex flex-wrap items-center gap-1 text-[10px] font-mono text-muted-foreground">
          <span>{copy(language, 'TOP VETOES', '主要否决')}:</span>
          {cycle!.top_vetoes.slice(0, 3).map((veto) => (
            <VetoChip key={veto.code} code={`${veto.code} ×${veto.count}`} />
          ))}
        </div>
      )}
    </div>
  )
}

function CycleQuality({
  rows,
  language,
}: {
  rows: V7SignalRow[]
  language: Language
}) {
  const timestamp = rows[0]?.timestamp ? new Date(rows[0].timestamp) : null
  const partial = rows.filter(
    (row) => row.signal.execution_readiness?.data_quality === 'PARTIAL'
  ).length
  const stale = rows.filter(
    (row) => row.signal.execution_readiness?.data_quality === 'STALE'
  ).length
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 border-l border-border/70 pl-3 font-mono text-[10px] text-muted-foreground">
      {timestamp && (
        <span>
          {copy(language, 'signal time', '信号时间')}{' '}
          {timestamp.toLocaleTimeString()}
        </span>
      )}
      <span>
        {copy(language, 'quality', '数据质量')}{' '}
        {stale > 0 ? 'STALE' : partial > 0 ? 'PARTIAL' : 'REPORTED'}
      </span>
      {(partial > 0 || stale > 0) && (
        <span>
          {copy(language, 'affected', '受影响')} {partial + stale}
        </span>
      )}
    </div>
  )
}

function OutcomeBadge({
  row,
  language,
}: {
  row: V7SignalRow
  language: Language
}) {
  if (!row.track_status) return null
  const pnl = Number.isFinite(row.track_pnl_pct) ? row.track_pnl_pct : 0
  const status = row.track_status
  const isProtected = status === 'PROTECTED_STOP'
  const isWin = status.startsWith('WIN_')
  const isStop = status === 'STOP'
  const isAmbiguous = status === 'BOTH_SAME_1M'
  const classes = isProtected
    ? 'border-tier-reviewable/40 bg-tier-reviewable-bg text-tier-reviewable'
    : isWin
      ? 'border-profit/30 bg-profit/10 text-profit'
      : isStop
        ? 'border-loss/30 bg-loss/10 text-loss'
        : 'border-border/60 bg-muted/20 text-muted-foreground'
  return (
    <span
      className={`inline-flex min-w-0 items-center gap-1 rounded border px-1.5 py-0.5 font-mono text-[10px] font-semibold uppercase tracking-wider ${classes}`}
      title={`${status} ${pnl >= 0 ? '+' : ''}${pnl.toFixed(3)}%`}
    >
      {isProtected && <ShieldCheck className="h-3 w-3 shrink-0" />}
      <span className="truncate">
        {isAmbiguous
          ? copy(language, 'AMBIGUOUS 1M', '同K线歧义')
          : isProtected
            ? t('v7Signals.protectedStop', language)
            : status.replace('_', ' ')}
      </span>
      <span className="tabular-nums">
        {pnl >= 0 ? '+' : ''}
        {pnl.toFixed(2)}%
      </span>
    </span>
  )
}

function EvidenceStrip({
  signal,
  language,
}: {
  signal: V7Signal
  language: Language
}) {
  const derivative = signal.derivatives_context
  const price = signal.price_context
  const reasons = (signal.reason_codes ?? []).filter(
    (code) =>
      !code.startsWith('flow_taker_buy_') &&
      !code.startsWith('flow_taker_sell_')
  )
  const metric = (label: string, value: string | null) =>
    value === null ? null : (
      <span key={label}>
        <span className="text-muted-foreground">{label} </span>
        {value}
      </span>
    )
  const percent = (value?: number) =>
    value === undefined || !Number.isFinite(value)
      ? null
      : `${value >= 0 ? '+' : ''}${value.toFixed(2)}%`
  if (!derivative && !price && reasons.length === 0) return null
  return (
    <div className="space-y-1.5 rounded border border-border/50 bg-surface/20 px-2.5 py-2">
      <div className="font-mono text-[9px] font-semibold tracking-[0.14em] text-muted-foreground">
        {copy(language, 'ROUTING EVIDENCE', '路由证据')}
      </div>
      <div className="flex flex-wrap gap-x-3 gap-y-1 font-mono text-[10px] text-foreground/85">
        {metric('ΔP 1h', percent(price?.change_1h))}
        {metric('ΔP 4h', percent(price?.change_4h))}
        {metric('ΔOI 1h', percent(derivative?.oi_change_1h))}
        {metric('ΔOI 4h', percent(derivative?.oi_change_4h))}
        {metric('Funding', percent(derivative?.funding_rate))}
        {metric(
          'LSR',
          derivative?.lsr_newest !== undefined
            ? derivative.lsr_newest.toFixed(2)
            : null
        )}
        {metric(
          'Taker',
          derivative?.taker_buy_ratio_15m !== undefined
            ? derivative.taker_buy_ratio_15m.toFixed(2)
            : null
        )}
      </div>
      {reasons.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {reasons.slice(0, 7).map((code) => (
            <VetoChip key={code} code={code} />
          ))}
        </div>
      )}
    </div>
  )
}

function TrackProgress({
  row,
  language,
}: {
  row: V7SignalRow
  language: Language
}) {
  if (!row.track_status) return null
  const steps = [
    ['TP0', row.track_tp0_done],
    ['TP1', row.track_tp1_done],
    ['TP2', row.track_tp2_done],
  ] as const
  return (
    <div className="flex flex-wrap items-center gap-2 font-mono text-[10px] text-muted-foreground">
      <span>{copy(language, 'exit progress', '退出进度')}</span>
      {steps.map(([label, done]) => (
        <span key={label} className={done ? 'text-tier-executable' : ''}>
          {done ? '●' : '○'} {label}
        </span>
      ))}
      {row.track_remaining_ratio !== undefined && (
        <span>
          {copy(language, 'runner', '剩余仓位')}{' '}
          {(row.track_remaining_ratio * 100).toFixed(0)}%
        </span>
      )}
      {row.track_realized_pnl_pct !== undefined && (
        <span>
          {copy(language, 'realized', '已实现')}{' '}
          {row.track_realized_pnl_pct >= 0 ? '+' : ''}
          {row.track_realized_pnl_pct.toFixed(2)}%
        </span>
      )}
    </div>
  )
}

// Shows captured derivatives context as an analytical aid, never as a
// standalone trade trigger. Market-cap/OI-MC and 3d fields remain absent until
// a source with freshness guarantees is added to the backend contract.
function StructureMonitor({
  rows,
  language,
}: {
  rows: V7SignalRow[]
  language: Language
}) {
  const structuralRows = rows
    .filter((row) => row.signal.derivatives_context || row.signal.price_context)
    .sort((a, b) => b.signal.ai_priority - a.signal.ai_priority)
    .slice(0, 12)
  if (structuralRows.length === 0) return null
  const pct = (value?: number) =>
    value === undefined || !Number.isFinite(value)
      ? '--'
      : `${value >= 0 ? '+' : ''}${value.toFixed(2)}%`
  const oi = (value?: number) =>
    value === undefined || value <= 0
      ? '--'
      : value >= 1_000_000
        ? `$${(value / 1_000_000).toFixed(1)}M`
        : `$${(value / 1000).toFixed(0)}K`
  return (
    <div className="overflow-hidden rounded-lg border border-border/60 bg-surface/25">
      <div className="flex items-center justify-between border-b border-border/50 px-3 py-2.5">
        <div>
          <div className="font-mono text-[10px] font-semibold tracking-[0.14em] text-muted-foreground">
            {copy(language, 'DERIVATIVES STRUCTURE', '合约结构监控')}
          </div>
          <div className="mt-0.5 text-[10px] text-muted-foreground">
            {copy(
              language,
              'Snapshot evidence — not a standalone entry signal',
              '快照证据，不构成独立开仓信号'
            )}
          </div>
        </div>
        <span className="font-mono text-[10px] text-muted-foreground">
          TOP {structuralRows.length}
        </span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[720px] font-mono text-[10px] tabular-nums">
          <thead className="bg-panel/40 text-muted-foreground">
            <tr className="border-b border-border/40">
              {[
                'SYMBOL',
                'SETUP',
                'ΔP 1H',
                'OI',
                'ΔOI 1H',
                'FUNDING',
                'LSR',
                'TAKER',
              ].map((label) => (
                <th
                  key={label}
                  className="px-3 py-2 text-right font-medium tracking-wider first:text-left"
                >
                  {label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {structuralRows.map((row) => {
              const derivative = row.signal.derivatives_context
              const price = row.signal.price_context
              return (
                <tr
                  key={row.id}
                  className="border-b border-border/30 last:border-0 hover:bg-panel/35"
                >
                  <td className="px-3 py-2 text-left font-semibold text-foreground">
                    {row.signal.symbol}
                  </td>
                  <td className="px-3 py-2 text-right text-muted-foreground">
                    {row.signal.setup_type}
                  </td>
                  <td className="px-3 py-2 text-right">
                    {pct(price?.change_1h)}
                  </td>
                  <td className="px-3 py-2 text-right">
                    {oi(derivative?.oi_value)}
                  </td>
                  <td className="px-3 py-2 text-right">
                    {pct(derivative?.oi_change_1h)}
                  </td>
                  <td className="px-3 py-2 text-right">
                    {pct(derivative?.funding_rate)}
                  </td>
                  <td className="px-3 py-2 text-right">
                    {derivative?.lsr_newest?.toFixed(2) ?? '--'}
                  </td>
                  <td className="px-3 py-2 text-right">
                    {derivative?.taker_buy_ratio_15m?.toFixed(2) ?? '--'}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function ScoreQuad({
  signal,
  language,
}: {
  signal: V7Signal
  language: Language
}) {
  const cells: Array<[string, number]> = [
    [t('v7Signals.scoreAiPriority', language), signal.ai_priority],
    [t('v7Signals.scoreSetup', language), signal.setup_score],
    [t('v7Signals.scoreTiming', language), signal.timing_score],
    [t('v7Signals.scoreRisk', language), signal.risk_score],
  ]
  return (
    <div className="grid grid-cols-4 divide-x divide-border/50 overflow-hidden rounded border border-border/50 bg-surface/20">
      {cells.map(([label, value]) => (
        <div key={label} className="min-w-0 px-2 py-1.5">
          <div className="text-[10px] font-mono uppercase tracking-wider text-muted-foreground">
            {label}
          </div>
          <div className="text-sm font-mono font-bold tabular-nums text-foreground text-right">
            {Number.isFinite(value) ? value.toFixed(0) : '--'}
          </div>
        </div>
      ))}
    </div>
  )
}

function ZoneBar({
  signal,
  language,
}: {
  signal: V7Signal
  language: Language
}) {
  const pos = zonePositionPct(signal)
  const { lower, upper } = signal.entry_zone
  if (!(lower > 0) || !(upper > 0)) return null
  return (
    <div>
      <div className="flex items-center justify-between text-[10px] font-mono text-muted-foreground mb-1">
        <span className="uppercase tracking-wider">
          {t('v7Signals.entryZone', language)}
        </span>
        {pos !== null && (
          <span className="tabular-nums">
            {t('v7Signals.zonePosition', language)} {pos.toFixed(0)}%
          </span>
        )}
      </div>
      <div className="relative h-1.5 rounded-full bg-muted/30 overflow-hidden">
        {pos !== null && (
          <div
            data-testid="zone-marker"
            className="absolute top-0 h-full w-1 rounded-full bg-foreground"
            style={{ left: `calc(${pos}% - 2px)` }}
          />
        )}
      </div>
      <div className="flex items-center justify-between text-[11px] font-mono tabular-nums text-foreground mt-1">
        <span>{formatPrice(lower)}</span>
        <span>{formatPrice(upper)}</span>
      </div>
    </div>
  )
}

function ConfirmationState({
  row,
  language,
}: {
  row: V7SignalRow
  language: Language
}) {
  const summary = row.signal.confirmation_summary
  const required = row.signal.required_confirmations ?? []
  const ladder = takerLadderCodes(row.signal)
  if (!summary && required.length === 0 && ladder.length === 0) return null
  return (
    <div className="space-y-1.5">
      {summary && (
        <div className="flex items-center gap-3 text-[11px] font-mono text-muted-foreground">
          <span className="inline-flex items-center gap-1">
            {summary.passed_hard ? (
              <Check className="w-3 h-3 text-foreground" />
            ) : (
              <X className="w-3 h-3" />
            )}
            {t('v7Signals.hardConfirms', language)}
          </span>
          <span className="inline-flex items-center gap-1">
            {summary.passed_review ? (
              <Check className="w-3 h-3 text-foreground" />
            ) : (
              <X className="w-3 h-3" />
            )}
            {t('v7Signals.reviewConfirms', language)}
          </span>
          {summary.rr !== undefined && summary.rr > 0 && (
            <span className="tabular-nums">RR {summary.rr.toFixed(2)}</span>
          )}
        </div>
      )}
      {(required.length > 0 || ladder.length > 0) && (
        <div className="flex flex-wrap gap-1">
          {required.map((code) => (
            <VetoChip key={`req-${code}`} code={code} />
          ))}
          {ladder.map((code) => (
            <VetoChip key={`ladder-${code}`} code={code} />
          ))}
        </div>
      )}
    </div>
  )
}

/** Expanded card for EXECUTABLE / REVIEWABLE rows. */
function SignalCard({
  row,
  language,
}: {
  row: V7SignalRow
  language: Language
}) {
  const { signal } = row
  const isExecutable = row.execution_tier === 'EXECUTABLE'
  const catalog = useTagCatalog()
  return (
    <div
      data-testid={`signal-card-${signal.symbol}`}
      className={`rounded-lg border bg-panel/60 p-4 space-y-3 ${
        isExecutable
          ? 'border-l-2 border-l-tier-executable border-tier-executable/30'
          : 'border-tier-reviewable/30'
      }`}
    >
      <div className="flex items-center gap-2 flex-wrap">
        <span className="font-mono font-bold text-sm text-foreground">
          {signal.symbol}
        </span>
        <DirectionBadge direction={signal.direction} />
        <SignalTierBadge tier={row.execution_tier as SignalTier} />
        <OutcomeBadge row={row} language={language} />
        <span className="font-mono text-[11px] text-muted-foreground">
          {signal.setup_type}
        </span>
        {signal.market_regime && (
          <span className="ml-auto font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
            {signal.market_regime}
          </span>
        )}
      </div>
      {row.tier_reason && (
        <div className="text-[11px] text-muted-foreground">
          {row.tier_reason}
        </div>
      )}
      <ScoreQuad signal={signal} language={language} />
      <ZoneBar signal={signal} language={language} />
      <div className="flex items-center gap-4 text-[11px] font-mono tabular-nums">
        <span className="text-muted-foreground">
          {t('v7Signals.invalidation', language)}{' '}
          <span className="text-foreground">
            {signal.invalidation.price > 0
              ? formatPrice(signal.invalidation.price)
              : '--'}
          </span>
        </span>
        {signal.tp0_price !== undefined && signal.tp0_price > 0 && (
          <span className="text-muted-foreground">
            TP0{' '}
            <span className="text-foreground">
              {formatPrice(signal.tp0_price)}
            </span>
            {signal.tp0_rr !== undefined && signal.tp0_rr > 0 && (
              <span> ({signal.tp0_rr.toFixed(2)}R)</span>
            )}
          </span>
        )}
        {(signal.targets?.length ?? 0) > 0 && (
          <span className="text-muted-foreground">
            TP1{' '}
            <span className="text-foreground">
              {formatPrice(signal.targets![0].price)}
            </span>
          </span>
        )}
      </div>
      <ConfirmationState row={row} language={language} />
      <EvidenceStrip signal={signal} language={language} />
      <TrackProgress row={row} language={language} />
      {(signal.risk_tags?.length ?? 0) > 0 && (
        <div className="flex flex-wrap gap-1">
          {signal.risk_tags!.map((tag) => (
            <span
              key={tag}
              title={tagTooltip(catalog, tag)}
              className="inline-flex items-center rounded border border-warning/30 bg-warning/10 px-1.5 py-0.5 font-mono text-[10px] text-warning"
            >
              {tag}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}

/** Dim one-line summary for WATCH rows. */
function WatchRow({ row, language }: { row: V7SignalRow; language: Language }) {
  const pos = zonePositionPct(row.signal)
  return (
    <div
      data-testid={`watch-row-${row.signal.symbol}`}
      className="grid min-h-9 grid-cols-[minmax(84px,1fr)_auto_minmax(120px,1.4fr)_minmax(92px,auto)] items-center gap-2 rounded border border-transparent bg-tier-watch-bg px-3 py-1.5 text-muted-foreground md:grid-cols-[minmax(96px,1fr)_auto_minmax(180px,1.8fr)_minmax(150px,auto)_minmax(120px,1.2fr)]"
    >
      <div className="flex min-w-0 items-center gap-2">
        <span className="truncate font-mono text-[11px] font-semibold text-foreground/80">
          {row.signal.symbol}
        </span>
        <DirectionBadge direction={row.signal.direction} />
      </div>
      <OutcomeBadge row={row} language={language} />
      <span className="truncate font-mono text-[10px]">
        {row.signal.setup_type}
      </span>
      <span className="text-right font-mono text-[10px] tabular-nums">
        {t('v7Signals.scoreAiPriority', language)}{' '}
        {row.signal.ai_priority.toFixed(0)}
        {pos !== null && (
          <span className="ml-2">
            {t('v7Signals.zonePosition', language)} {pos.toFixed(0)}%
          </span>
        )}
      </span>
      {row.tier_reason && (
        <span
          className="hidden truncate font-mono text-[10px] md:inline"
          title={row.tier_reason}
        >
          {row.tier_reason}
        </span>
      )}
      {(row.signal.required_confirmations?.length ?? 0) > 0 && (
        <span
          className="hidden truncate font-mono text-[10px] text-tier-reviewable xl:inline"
          title={row.signal.required_confirmations!.join(', ')}
        >
          {copy(language, 'next:', '下一步：')}{' '}
          {row.signal.required_confirmations![0]}
        </span>
      )}
    </div>
  )
}

/** REJECTED rows fold into a count plus aggregated veto-reason chips. */
function RejectedSection({
  rows,
  language,
}: {
  rows: V7SignalRow[]
  language: Language
}) {
  const [expanded, setExpanded] = useState(false)
  const reasonCounts = useMemo(() => {
    const counts = new Map<string, number>()
    for (const row of rows) {
      const reason = row.blocked_gate || row.tier_reason || 'unknown'
      counts.set(reason, (counts.get(reason) ?? 0) + 1)
    }
    return [...counts.entries()].sort((a, b) => b[1] - a[1])
  }, [rows])

  if (rows.length === 0) return null
  return (
    <div className="rounded border border-border/50 px-3 py-2">
      <IconButton
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="h-auto w-full justify-start gap-2 rounded-none p-0 text-left hover:bg-transparent"
        aria-expanded={expanded}
      >
        {expanded ? (
          <ChevronDown className="w-3.5 h-3.5 text-muted-foreground" />
        ) : (
          <ChevronRight className="w-3.5 h-3.5 text-muted-foreground" />
        )}
        <SignalTierBadge tier="REJECTED" />
        <span className="text-[11px] font-mono text-muted-foreground">
          {t('v7Signals.rejectedCount', language, { count: rows.length })}
        </span>
        <span className="flex flex-wrap gap-1 ml-2">
          {reasonCounts
            .slice(0, expanded ? undefined : 4)
            .map(([reason, n]) => (
              <VetoChip
                key={reason}
                code={n > 1 ? `${reason} ×${n}` : reason}
              />
            ))}
        </span>
      </IconButton>
      {expanded && (
        <div className="mt-2 space-y-1 pl-6">
          {rows.map((row) => (
            <div
              key={row.id}
              className="flex items-center gap-2 text-[11px] font-mono text-tier-rejected"
            >
              <span className="text-muted-foreground">{row.signal.symbol}</span>
              <span>{row.signal.setup_type}</span>
              <VetoChip
                code={row.blocked_gate || row.tier_reason || 'unknown'}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

interface SignalPanelProps {
  language: Language
  refreshInterval?: number
}

export function SignalPanel({
  language,
  refreshInterval = 60000,
}: SignalPanelProps) {
  const [rows, setRows] = useState<V7SignalRow[] | null>(null)
  const [cycle, setCycle] = useState<V7LatestCycleSummary | undefined>()
  const [loading, setLoading] = useState(true)
  const [failed, setFailed] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)

  const fetchSignals = useCallback(async () => {
    try {
      const res = await api.getV7Signals(120)
      setRows(res.signals ?? [])
      setCycle(res.cycle)
      setUpdatedAt(new Date())
      setFailed(false)
    } catch {
      setFailed(true)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchSignals()
    const timer = setInterval(fetchSignals, refreshInterval)
    return () => clearInterval(timer)
  }, [fetchSignals, refreshInterval])

  const grouped = useMemo(() => groupLatestCycleByTier(rows ?? []), [rows])
  const summary = useMemo(() => buildSignalSummary(grouped), [grouped])
  const actionable = [...grouped.EXECUTABLE, ...grouped.REVIEWABLE]
  const total =
    actionable.length + grouped.WATCH.length + grouped.REJECTED.length
  const latestCycle = rows && rows.length > 0 ? rows[0].cycle_number : null

  return (
    <div className="ait-glass p-6">
      {/* Header */}
      <div className="flex items-center gap-3 flex-wrap">
        <IconButton
          type="button"
          onClick={() => setCollapsed((v) => !v)}
          className="h-auto w-auto justify-start gap-2 rounded-none p-0 text-left hover:bg-transparent"
          aria-expanded={!collapsed}
        >
          {collapsed ? (
            <ChevronRight className="w-4 h-4 text-muted-foreground" />
          ) : (
            <ChevronDown className="w-4 h-4 text-muted-foreground" />
          )}
          <span className="text-tier-executable">
            <Radar size={18} />
          </span>
          <h2 className="text-lg font-bold text-foreground uppercase tracking-wide">
            {t('v7Signals.title', language)}
          </h2>
        </IconButton>
        {total > 0 && (
          <div className="flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
            {TIER_ORDER.map((tier) =>
              grouped[tier].length > 0 ? (
                <span key={tier} className="inline-flex items-center gap-1">
                  <SignalTierBadge tier={tier} />
                  <span className="tabular-nums">{grouped[tier].length}</span>
                </span>
              ) : null
            )}
          </div>
        )}
        <div className="ml-auto flex items-center gap-3">
          {latestCycle !== null && (
            <span className="font-mono text-[10px] text-muted-foreground tabular-nums">
              {t('v7Signals.cycle', language)} #{latestCycle}
            </span>
          )}
          {updatedAt && (
            <span className="font-mono text-[10px] text-muted-foreground">
              {updatedAt.toLocaleTimeString()}
            </span>
          )}
          <IconButton
            type="button"
            onClick={() => {
              setLoading(true)
              fetchSignals()
            }}
            size="sm"
            title={t('v7Signals.refresh', language)}
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          </IconButton>
        </div>
      </div>

      {rows !== null && rows.length > 0 && (
        <div className="mt-2 flex justify-end">
          <CycleQuality
            rows={TIER_ORDER.flatMap((tier) => grouped[tier])}
            language={language}
          />
        </div>
      )}

      {!collapsed && (
        <div className="mt-4 space-y-3">
          <SignalSummaryStrip summary={summary} language={language} />
          <SignalFunnel summary={summary} cycle={cycle} language={language} />

          {/* Loading state */}
          {loading && rows === null && !failed && (
            <div className="text-center py-10 text-muted-foreground">
              <RefreshCw size={18} className="animate-spin mx-auto mb-2" />
              <div className="text-xs">{t('v7Signals.loading', language)}</div>
            </div>
          )}

          {/* Backend down / error state */}
          {failed && rows === null && (
            <div className="text-center py-10 text-muted-foreground">
              <WifiOff size={18} className="mx-auto mb-2 opacity-60" />
              <div className="text-xs font-semibold">
                {t('v7Signals.loadFailed', language)}
              </div>
              <div className="text-[10px] mt-1 opacity-70">
                {t('v7Signals.loadFailedHint', language)}
              </div>
            </div>
          )}

          {/* Empty state */}
          {rows !== null && total === 0 && (
            <div className="text-center py-10 text-muted-foreground">
              <Radar size={18} className="mx-auto mb-2 opacity-50" />
              <div className="text-xs font-semibold">
                {t('v7Signals.empty', language)}
              </div>
              <div className="text-[10px] mt-1 opacity-70">
                {t('v7Signals.emptyHint', language)}
              </div>
            </div>
          )}

          {/* Executable / Reviewable cards */}
          {actionable.length > 0 && (
            <div className="grid grid-cols-1 xl:grid-cols-2 gap-3">
              {actionable.map((row) => (
                <SignalCard key={row.id} row={row} language={language} />
              ))}
            </div>
          )}

          <StructureMonitor
            rows={TIER_ORDER.flatMap((tier) => grouped[tier])}
            language={language}
          />

          {/* Watch summary rows */}
          {grouped.WATCH.length > 0 && (
            <div className="space-y-1">
              {grouped.WATCH.map((row) => (
                <WatchRow key={row.id} row={row} language={language} />
              ))}
            </div>
          )}

          {/* Rejected fold */}
          <RejectedSection rows={grouped.REJECTED} language={language} />
        </div>
      )}
    </div>
  )
}
