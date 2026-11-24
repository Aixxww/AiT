/**
 * Hunter v7 cell dashboard (U12): fee-net outcomes per class, cell
 * (class × RR band) and cell × regime, with the bootstrap CI, the stamped
 * and the evaluated §5.3 cell status (one evaluation, no hysteresis), and
 * the calibration drift table per calibration_version.
 *
 * Read-only. Mirrors GET /api/hunter/v7/cells.
 */
import { useCallback, useEffect, useState } from 'react'
import {
  ChevronDown,
  ChevronRight,
  RefreshCw,
  Table2,
  WifiOff,
} from 'lucide-react'
import { api } from '../../lib/api'
import type {
  V7CalibrationDrift,
  V7CalibrationWindow,
  V7CellRow,
  V7CellSource,
  V7CellStatus,
  V7CellsResponse,
  V7YieldTable,
} from '../../lib/api/hunter'
import { displayName, tagLabel } from '../../i18n/displayNames'
import { t, type Language } from '../../i18n/translations'
import { cn } from '../../lib/cn'
import { IconButton } from '../ui/IconButton'

export const CELL_SOURCES: V7CellSource[] = [
  'prompt_open_review',
  'all',
  'watch_audit',
  'shadow',
]
export const CELL_WINDOWS = [7, 30, 90]

const SOURCE_KEYS: Record<V7CellSource, string> = {
  prompt_open_review: 'v7Cells.sourceOpenReview',
  all: 'v7Cells.sourceAll',
  watch_audit: 'v7Cells.sourceWatchAudit',
  shadow: 'v7Cells.sourceShadow',
}

const STATUS_STYLES: Record<V7CellStatus, string> = {
  earned: 'text-tier-executable border-tier-executable/40',
  probation: 'text-muted-foreground border-border',
  blocked: 'text-tier-rejected border-tier-rejected/40',
}

/** Signed fixed-point R, e.g. +0.125 / -1.000. */
export function formatR(v: number): string {
  if (!Number.isFinite(v)) return '—'
  return `${v >= 0 ? '+' : ''}${v.toFixed(3)}`
}

/** Percentage with one decimal. */
export function formatRate(v: number): string {
  if (!Number.isFinite(v)) return '—'
  return `${(v * 100).toFixed(1)}%`
}

/** Row label for a table level. */
export function cellLabel(row: V7CellRow, language: Language = 'en'): string {
  return [
    row.signal_class
      ? displayName('signalClass', row.signal_class, language)
      : '',
    row.rr_band ? displayName('rrBand', row.rr_band, language) : '',
    row.market_regime ? displayName('regime', row.market_regime, language) : '',
  ]
    .filter((p) => p !== '')
    .join(' · ')
}

function StatusBadge({
  status,
  language,
}: {
  status: V7CellStatus
  language: Language
}) {
  return (
    <span
      data-testid={`status-${status}`}
      className={cn(
        'inline-block rounded border px-1.5 py-0.5 font-mono text-[10px] uppercase',
        STATUS_STYLES[status] ?? STATUS_STYLES.probation
      )}
    >
      {t(`v7Cells.status_${status}`, language)}
    </span>
  )
}

function CellTable({
  title,
  rows,
  language,
  defaultOpen,
}: {
  title: string
  rows: V7CellRow[]
  language: Language
  defaultOpen: boolean
}) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div>
      <IconButton
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="h-auto w-auto justify-start gap-1 rounded-none p-0 text-left hover:bg-transparent"
        aria-expanded={open}
      >
        {open ? (
          <ChevronDown className="w-3.5 h-3.5 text-muted-foreground" />
        ) : (
          <ChevronRight className="w-3.5 h-3.5 text-muted-foreground" />
        )}
        <span className="text-xs font-semibold uppercase tracking-wide text-foreground">
          {title}
        </span>
        <span className="font-mono text-[10px] text-muted-foreground">
          ({rows.length})
        </span>
      </IconButton>
      {open && (
        <div className="mt-2 overflow-x-auto">
          <table className="w-full text-[11px] font-mono tabular-nums">
            <thead>
              <tr className="text-left text-muted-foreground">
                <th className="py-1 pr-3 font-medium">
                  {t('v7Cells.colGroup', language)}
                </th>
                <th className="py-1 pr-3 font-medium text-right">
                  {t('v7Cells.colN', language)}
                </th>
                <th className="py-1 pr-3 font-medium text-right">
                  {t('v7Cells.colTP1Rate', language)}
                </th>
                <th className="py-1 pr-3 font-medium text-right">
                  {t('v7Cells.colMeanR', language)}
                </th>
                <th className="py-1 pr-3 font-medium text-right">
                  {t('v7Cells.colCI', language)}
                </th>
                <th className="py-1 pr-3 font-medium">
                  {t('v7Cells.colStamped', language)}
                </th>
                <th className="py-1 font-medium">
                  {t('v7Cells.colEvaluated', language)}
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={cellLabel(row)} className="border-t border-border/40">
                  <td className="py-1 pr-3 text-foreground">
                    {cellLabel(row, language)}
                  </td>
                  <td className="py-1 pr-3 text-right">{row.n}</td>
                  <td className="py-1 pr-3 text-right">
                    {formatRate(row.tp1_first_rate)}
                  </td>
                  <td className="py-1 pr-3 text-right">
                    {formatR(row.mean_fee_net_r)}
                  </td>
                  <td className="py-1 pr-3 text-right text-muted-foreground">
                    [{formatR(row.ci_lo)}, {formatR(row.ci_hi)}]
                  </td>
                  <td className="py-1 pr-3 text-muted-foreground">
                    {row.stamped_status
                      .map((st) => {
                        const key = `v7Cells.status_${st}`
                        const label = t(key, language)
                        return label === key ? tagLabel(st, language) : label
                      })
                      .join(', ')}
                  </td>
                  <td className="py-1">
                    <StatusBadge
                      status={row.evaluated_status}
                      language={language}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function WindowTable({
  label,
  window,
  language,
}: {
  label: string
  window: V7CalibrationWindow
  language: Language
}) {
  return (
    <div className="min-w-0">
      <div className="text-[10px] uppercase text-muted-foreground mb-1">
        {label} · n={window.n}
      </div>
      {!window.sufficient ? (
        <div className="text-[11px] text-muted-foreground">
          {t('v7Cells.insufficient', language)}
        </div>
      ) : (
        <table className="w-full text-[11px] font-mono tabular-nums">
          <thead>
            <tr className="text-left text-muted-foreground">
              <th className="pr-2 font-medium">Q</th>
              <th className="pr-2 font-medium text-right">
                {t('v7Cells.colN', language)}
              </th>
              <th className="pr-2 font-medium text-right">
                {t('v7Cells.colPred', language)}
              </th>
              <th className="pr-2 font-medium text-right">
                {t('v7Cells.colRealized', language)}
              </th>
              <th className="pr-2 font-medium text-right">
                {t('v7Cells.colCI', language)}
              </th>
              <th className="font-medium" />
            </tr>
          </thead>
          <tbody>
            {window.quantiles.map((q) => (
              <tr key={q.quantile} className="border-t border-border/40">
                <td className="pr-2">{q.quantile}</td>
                <td className="pr-2 text-right">{q.n}</td>
                <td className="pr-2 text-right">{formatR(q.pred_mean)}</td>
                <td className="pr-2 text-right">{formatR(q.realized_mean)}</td>
                <td className="pr-2 text-right text-muted-foreground">
                  [{formatR(q.ci_lo)}, {formatR(q.ci_hi)}]
                </td>
                <td
                  className={
                    q.inside ? 'text-muted-foreground' : 'text-tier-rejected'
                  }
                >
                  {t(q.inside ? 'v7Cells.inside' : 'v7Cells.outside', language)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

function DriftSection({
  drift,
  language,
}: {
  drift: V7CalibrationDrift[]
  language: Language
}) {
  return (
    <div>
      <div className="text-xs font-semibold uppercase tracking-wide text-foreground mb-2">
        {t('v7Cells.driftTitle', language)}
      </div>
      {drift.length === 0 ? (
        <div className="text-[11px] text-muted-foreground">
          {t('v7Cells.driftEmpty', language)}
        </div>
      ) : (
        <div className="space-y-3">
          {drift.map((d) => (
            <div
              key={d.calibration_version}
              className="rounded border border-border/50 p-2"
            >
              <div className="flex items-center gap-2 mb-2">
                <span className="font-mono text-[11px] text-foreground">
                  {d.calibration_version || '—'}
                </span>
                <span
                  data-testid={`drift-${d.drift ? 'flag' : 'ok'}`}
                  className={cn(
                    'rounded border px-1.5 py-0.5 font-mono text-[10px] uppercase',
                    d.drift
                      ? 'text-tier-rejected border-tier-rejected/40'
                      : 'text-muted-foreground border-border'
                  )}
                >
                  {t(
                    d.drift ? 'v7Cells.driftFlag' : 'v7Cells.noDrift',
                    language
                  )}
                </span>
              </div>
              <div className="grid grid-cols-1 xl:grid-cols-2 gap-3">
                <WindowTable
                  label={t('v7Cells.current', language)}
                  window={d.current}
                  language={language}
                />
                <WindowTable
                  label={t('v7Cells.previous', language)}
                  window={d.previous}
                  language={language}
                />
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

/** Rate per round, as events per day for readability. */
export function formatPerDay(
  ratePerRound: number,
  roundMinutes: number
): string {
  if (!Number.isFinite(ratePerRound) || roundMinutes <= 0) return '—'
  return ((ratePerRound * 24 * 60) / roundMinutes).toFixed(2)
}

function YieldSection({
  table,
  language,
}: {
  table: V7YieldTable
  language: Language
}) {
  return (
    <div>
      <div className="text-xs font-semibold uppercase tracking-wide text-foreground mb-1">
        {t('v7Cells.yieldTitle', language)}
      </div>
      <div className="text-[10px] text-muted-foreground mb-2">
        {t('v7Cells.yieldNote', language, {
          ci: Math.round(table.ci_level * 100),
          tf: table.forming_bar_tf,
        })}
      </div>
      {!table.available ? (
        <div
          data-testid="yield-unavailable"
          className="text-[11px] text-muted-foreground"
        >
          {t('v7Cells.yieldUnavailable', language)}
          {/* The backend reason is English prose; zh shows the generic message only. */}
          {table.reason && language === 'en' ? ` (${table.reason})` : ''}
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-[11px] font-mono tabular-nums">
            <thead>
              <tr className="text-left text-muted-foreground">
                <th className="py-1 pr-3 font-medium">
                  {t('v7Cells.colTier', language)}
                </th>
                <th className="py-1 pr-3 font-medium text-right">
                  {t('v7Cells.colRounds', language)}
                </th>
                <th className="py-1 pr-3 font-medium text-right">
                  {t('v7Cells.colEvents', language)}
                </th>
                <th className="py-1 pr-3 font-medium text-right">
                  {t('v7Cells.colPerDay', language)}
                </th>
                <th className="py-1 pr-3 font-medium text-right">
                  {t('v7Cells.colCI', language)}
                </th>
                <th className="py-1 pr-3 font-medium text-right">
                  {t('v7Cells.colTarget', language)}
                </th>
                <th className="py-1 font-medium" />
              </tr>
            </thead>
            <tbody>
              {table.rows.map((row) => (
                <tr key={row.rv24_tier} className="border-t border-border/40">
                  <td className="py-1 pr-3 text-foreground">
                    {displayName('rv24Tier', row.rv24_tier, language)}
                  </td>
                  <td className="py-1 pr-3 text-right">{row.rounds}</td>
                  <td className="py-1 pr-3 text-right">{row.events}</td>
                  <td className="py-1 pr-3 text-right">
                    {formatPerDay(row.rate_per_round, table.round_minutes)}
                  </td>
                  <td className="py-1 pr-3 text-right text-muted-foreground">
                    [{formatPerDay(row.rate_ci_lo, table.round_minutes)},{' '}
                    {formatPerDay(row.rate_ci_hi, table.round_minutes)}]
                  </td>
                  <td className="py-1 pr-3 text-right">
                    {row.target_rate !== undefined
                      ? formatPerDay(row.target_rate, table.round_minutes)
                      : '—'}
                  </td>
                  <td
                    className={
                      row.below_target
                        ? 'text-tier-rejected'
                        : 'text-muted-foreground'
                    }
                  >
                    {row.below_target ? (
                      <span data-testid={`yield-below-${row.rv24_tier}`}>
                        {t('v7Cells.belowTarget', language)}
                      </span>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

interface CellsPanelProps {
  language: Language
  refreshInterval?: number
}

export function CellsPanel({
  language,
  refreshInterval = 300000,
}: CellsPanelProps) {
  const [days, setDays] = useState(CELL_WINDOWS[0])
  const [source, setSource] = useState<V7CellSource>('prompt_open_review')
  const [data, setData] = useState<V7CellsResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [failed, setFailed] = useState(false)
  const [collapsed, setCollapsed] = useState(false)

  const fetchCells = useCallback(async () => {
    try {
      const res = await api.getV7Cells(days, source)
      setData(res)
      setFailed(false)
    } catch {
      setFailed(true)
    } finally {
      setLoading(false)
    }
  }, [days, source])

  useEffect(() => {
    setLoading(true)
    fetchCells()
    const timer = setInterval(fetchCells, refreshInterval)
    return () => clearInterval(timer)
  }, [fetchCells, refreshInterval])

  const excluded = data
    ? Object.entries(data.excluded_no_r ?? {})
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([k, n]) => `${k} ${n}`)
        .join(', ')
    : ''

  return (
    <div className="ait-glass p-6">
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
          <Table2 size={18} className="text-tier-reviewable" />
          <h2 className="text-lg font-bold text-foreground uppercase tracking-wide">
            {t('v7Cells.title', language)}
          </h2>
        </IconButton>
        <div className="ml-auto flex items-center gap-2">
          <label className="flex items-center gap-1 text-[10px] text-muted-foreground">
            {t('v7Cells.window', language)}
            <select
              aria-label={t('v7Cells.window', language)}
              value={days}
              onChange={(e) => setDays(Number(e.target.value))}
              className="bg-transparent border border-border rounded px-1 py-0.5 font-mono text-[11px] text-foreground"
            >
              {CELL_WINDOWS.map((d) => (
                <option key={d} value={d}>
                  {t('v7Cells.days', language, { n: d })}
                </option>
              ))}
            </select>
          </label>
          <label className="flex items-center gap-1 text-[10px] text-muted-foreground">
            {t('v7Cells.source', language)}
            <select
              aria-label={t('v7Cells.source', language)}
              value={source}
              onChange={(e) => setSource(e.target.value as V7CellSource)}
              className="bg-transparent border border-border rounded px-1 py-0.5 font-mono text-[11px] text-foreground"
            >
              {CELL_SOURCES.map((s) => (
                <option key={s} value={s}>
                  {t(SOURCE_KEYS[s], language)}
                </option>
              ))}
            </select>
          </label>
          <IconButton
            type="button"
            onClick={() => {
              setLoading(true)
              fetchCells()
            }}
            size="sm"
            title={t('v7Cells.refresh', language)}
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          </IconButton>
        </div>
      </div>

      {!collapsed && (
        <div className="mt-4 space-y-4">
          {loading && data === null && !failed && (
            <div className="text-center py-8 text-xs text-muted-foreground">
              {t('v7Cells.loading', language)}
            </div>
          )}
          {failed && data === null && (
            <div className="text-center py-8 text-muted-foreground">
              <WifiOff size={18} className="mx-auto mb-2 opacity-60" />
              <div className="text-xs font-semibold">
                {t('v7Cells.loadFailed', language)}
              </div>
            </div>
          )}
          {data && (
            <>
              <div className="text-[10px] text-muted-foreground">
                {t('v7Cells.ruleNote', language, {
                  n: data.rule.min_samples,
                  ci: Math.round(data.rule.ci_level * 100),
                })}
              </div>
              {data.tables.rows === 0 ? (
                <div className="text-center py-6 text-xs text-muted-foreground">
                  {t('v7Cells.empty', language)}
                </div>
              ) : (
                <div className="space-y-3">
                  <CellTable
                    title={t('v7Cells.classes', language)}
                    rows={data.tables.classes}
                    language={language}
                    defaultOpen
                  />
                  <CellTable
                    title={t('v7Cells.cells', language)}
                    rows={data.tables.cells}
                    language={language}
                    defaultOpen
                  />
                  <CellTable
                    title={t('v7Cells.cellRegimes', language)}
                    rows={data.tables.cell_regimes}
                    language={language}
                    defaultOpen={false}
                  />
                </div>
              )}
              {excluded && (
                <div className="text-[10px] text-muted-foreground">
                  {t('v7Cells.excluded', language, { list: excluded })}
                </div>
              )}
              <DriftSection
                drift={data.calibration_drift ?? []}
                language={language}
              />
              {data.yield && (
                <YieldSection table={data.yield} language={language} />
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}
