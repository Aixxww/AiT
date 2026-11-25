import { useEffect, useMemo, useState } from 'react'
import { Gauge, Users } from 'lucide-react'
import type { ResolvedValue, StrategyConfig, TraderInfo } from '../../types'
import { riskControl, ts } from '../../i18n/strategy-translations'
import { SectionHeader } from '../ui/SectionHeader'
import { InfoTip } from '../ui/InfoTip'
import { cn } from '../../lib/cn'

// #95 §5.4.5 / §5.4.6: read-only view of what the backend resolves the
// edited (possibly unsaved) config to, value and source, plus the scan
// interval of every trader attached to this strategy.

const API_BASE = import.meta.env.VITE_API_BASE || ''

interface Props {
  config: StrategyConfig
  strategyId?: string
  token?: string | null
  language: string
}

const SOURCE_STYLE: Record<ResolvedValue['source'], string> = {
  config: 'bg-primary-dim text-primary',
  default: 'bg-surface-alt text-muted-foreground',
  safety_limit: 'bg-loss/10 text-loss',
}

function formatValue(v: unknown): string {
  if (v === null || v === undefined || v === '') return '—'
  if (typeof v === 'number')
    return Number.isInteger(v) ? String(v) : String(Number(v.toFixed(4)))
  if (typeof v === 'boolean') return v ? 'true' : 'false'
  return String(v)
}

export function EffectiveConfigPanel({
  config,
  strategyId,
  token,
  language,
}: Props) {
  const [values, setValues] = useState<ResolvedValue[] | null>(null)
  const [validationError, setValidationError] = useState<string>('')
  const [error, setError] = useState(false)
  const [traders, setTraders] = useState<TraderInfo[]>([])
  const configKey = useMemo(() => JSON.stringify(config), [config])
  const tr = (k: keyof typeof riskControl) => ts(riskControl[k], language)

  useEffect(() => {
    const controller = new AbortController()
    const timer = window.setTimeout(async () => {
      try {
        const res = await fetch(`${API_BASE}/api/strategies/effective-config`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
          body: JSON.stringify({ config: JSON.parse(configKey) }),
          signal: controller.signal,
        })
        if (!res.ok) throw new Error(String(res.status))
        const data = await res.json()
        setValues(Array.isArray(data.values) ? data.values : [])
        setValidationError(
          typeof data.validation_error === 'string' ? data.validation_error : ''
        )
        setError(false)
      } catch (e) {
        if ((e as Error).name !== 'AbortError') setError(true)
      }
    }, 400)
    return () => {
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [configKey, token])

  useEffect(() => {
    if (!strategyId) return
    let cancelled = false
    fetch(`${API_BASE}/api/my-traders`, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
      .then((r) => (r.ok ? r.json() : []))
      .then((list: TraderInfo[]) => {
        if (!cancelled && Array.isArray(list)) {
          setTraders(list.filter((t) => t.strategy_id === strategyId))
        }
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [strategyId, token])

  const groups = useMemo(() => {
    const out = new Map<string, ResolvedValue[]>()
    for (const v of values ?? []) {
      const list = out.get(v.group) ?? []
      list.push(v)
      out.set(v.group, list)
    }
    return [...out.entries()]
  }, [values])

  const sourceLabel = (s: ResolvedValue['source']) =>
    s === 'config'
      ? tr('effectiveSourceConfig')
      : s === 'default'
        ? tr('effectiveSourceDefault')
        : tr('effectiveSourceSafety')

  return (
    <section
      className="ait-card p-4 mt-3 space-y-5"
      data-testid="effective-config"
    >
      <div>
        <SectionHeader
          icon={<Users className="w-4 h-4" />}
          title={tr('attachedTraders')}
          info={tr('attachedTradersDesc')}
          className="mb-2"
        />
        {traders.length === 0 ? (
          <p className="t-caption text-muted-foreground">
            {tr('attachedTradersNone')}
          </p>
        ) : (
          <ul className="flex flex-wrap gap-2">
            {traders.map((t) => (
              <li key={t.trader_id} className="ait-chip">
                <span className="text-foreground">{t.trader_name}</span>
                <span className="t-num">
                  {tr('scanEvery').replace(
                    '{n}',
                    String(t.scan_interval_minutes ?? '—')
                  )}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div>
        <SectionHeader
          icon={<Gauge className="w-4 h-4" />}
          title={tr('effectiveConfig')}
          info={tr('effectiveConfigDesc')}
          className="mb-2"
        />
        {validationError && (
          <p className="t-caption text-loss mb-2" role="alert">
            {tr('effectiveValidation')}
            <span data-raw>{validationError}</span>
          </p>
        )}
        {error && <p className="t-caption text-loss">{tr('effectiveError')}</p>}
        {!error && values === null && (
          <p className="t-caption text-muted-foreground">
            {tr('effectiveLoading')}
          </p>
        )}
        <div className="space-y-4">
          {groups.map(([group, list]) => (
            <div key={group}>
              <div className="t-micro text-muted-foreground mb-1" data-raw>
                {group}
              </div>
              <div className="ait-table-scroll">
                <table className="w-full text-sm">
                  <tbody>
                    {list.map((v) => (
                      <tr
                        key={v.key}
                        className="border-t border-border align-top"
                      >
                        <td className="py-1.5 pr-3 whitespace-nowrap">
                          <code data-raw className="text-xs">
                            {v.key}
                          </code>
                        </td>
                        <td className="py-1.5 pr-3 t-num break-all" data-raw>
                          {formatValue(v.value)}
                        </td>
                        <td className="py-1.5 pr-3 whitespace-nowrap">
                          <span
                            className={cn('ait-pill', SOURCE_STYLE[v.source])}
                          >
                            {sourceLabel(v.source)}
                          </span>
                          {v.enforced === false && (
                            <span className="ait-pill bg-loss/10 text-loss ml-1">
                              {tr('effectiveNotEnforced')}
                            </span>
                          )}
                        </td>
                        <td className="py-1.5 w-6">
                          {(v.note || v.limit) && (
                            <InfoTip
                              content={
                                <span data-raw>
                                  {[v.limit, v.note]
                                    .filter(Boolean)
                                    .join(' · ')}
                                </span>
                              }
                            />
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
