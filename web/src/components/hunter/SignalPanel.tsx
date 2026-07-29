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
import type { V7Signal, V7SignalRow, V7Tier } from '../../lib/api/hunter'
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
    <div className="grid grid-cols-4 gap-2">
      {cells.map(([label, value]) => (
        <div key={label} className="rounded bg-muted/20 px-2 py-1.5">
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
      className="flex items-center gap-2 px-3 py-1.5 rounded bg-tier-watch-bg text-muted-foreground"
    >
      <span className="font-mono font-semibold text-[11px] text-foreground/80">
        {row.signal.symbol}
      </span>
      <DirectionBadge direction={row.signal.direction} />
      <span className="font-mono text-[10px]">{row.signal.setup_type}</span>
      <span className="ml-auto font-mono text-[10px] tabular-nums">
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
          className="font-mono text-[10px] max-w-[220px] truncate hidden md:inline"
          title={row.tier_reason}
        >
          {row.tier_reason}
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
  const [loading, setLoading] = useState(true)
  const [failed, setFailed] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)

  const fetchSignals = useCallback(async () => {
    try {
      const res = await api.getV7Signals(120)
      setRows(res.signals ?? [])
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

      {!collapsed && (
        <div className="mt-4 space-y-3">
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
