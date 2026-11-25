import type { ReactNode } from 'react'
import { SlidersHorizontal } from 'lucide-react'
import type {
  CooldownConfig,
  DailyLossBreakerConfig,
  ProtectorCoreConfig,
  ProtectorLevel,
  RiskControlConfig,
  SetupGuardConfig,
} from '../../types'
import { riskControl, ts } from '../../i18n/strategy-translations'
import { SectionHeader } from '../ui/SectionHeader'
import { AiTSelect } from '../ui/select'
import { setupLabel } from '../../i18n/displayNames'

// #95 §5.4.4: inputs for the backend-only knobs. Every field is optional:
// an empty input stores "unset" and the backend resolver applies the
// documented default (shown as the placeholder). These defaults mirror
// store/resolve_hunter_v7.go and store/protector.go; the read-only
// "effective config" panel shows what the backend actually resolved.
export const DEFAULT_MIN_RR = 1.2
export const MAX_POSITIONS_SAFETY_CEILING = 3

const DEFAULTS = {
  maxSingleTradeLossPct: 8,
  maxTakeProfitMovePct: 3,
  minStopLossMovePct: 2,
  breakerMaxLossPct: 15,
  breakerNearFraction: 0.8,
  failedOpenCycles: 3,
  recentLossWindow: 60,
  recentLossMinLosses: 2,
  recentLossPct: 5,
  tp0: 8,
  tp1: 6,
  tp0CloseRatio: 0.35,
  tp1CloseRatio: 0.4,
  givebackCloseRatio: 0.5,
  breakevenBufferFrac: 0.001,
}

/** Built-in setup guard defaults (store.BuiltinSetupGuards). */
const BUILTIN_SETUP_GUARDS: Record<
  string,
  Required<Omit<SetupGuardConfig, 'min_ai_priority'>>
> = {
  funding_reversal: {
    min_zone_pos_short: 0,
    max_zone_pos_long: 35,
    require_oi_flush: true,
  },
  distribution_short: {
    min_zone_pos_short: 60,
    max_zone_pos_long: 100,
    require_oi_flush: false,
  },
  long_squeeze_short: {
    min_zone_pos_short: 60,
    max_zone_pos_long: 100,
    require_oi_flush: false,
  },
  breakdown_momentum_short: {
    min_zone_pos_short: 45,
    max_zone_pos_long: 100,
    require_oi_flush: false,
  },
  alt_ladder_breakdown_short: {
    min_zone_pos_short: 40,
    max_zone_pos_long: 100,
    require_oi_flush: false,
  },
  range_reversion: {
    min_zone_pos_short: 55,
    max_zone_pos_long: 45,
    require_oi_flush: false,
  },
  pullback_reversal_long: {
    min_zone_pos_short: 0,
    max_zone_pos_long: 50,
    require_oi_flush: false,
  },
  alt_ladder_momentum_long: {
    min_zone_pos_short: 0,
    max_zone_pos_long: 72,
    require_oi_flush: false,
  },
  mms_bottom_wake_long: {
    min_zone_pos_short: 0,
    max_zone_pos_long: 65,
    require_oi_flush: false,
  },
  mms_trend_ride_long: {
    min_zone_pos_short: 0,
    max_zone_pos_long: 70,
    require_oi_flush: false,
  },
  mms_squeeze_engine_long: {
    min_zone_pos_short: 0,
    max_zone_pos_long: 75,
    require_oi_flush: false,
  },
}

const INPUT_STYLE = {
  background: 'var(--color-input)',
  border: '1px solid var(--color-border)',
  color: 'var(--color-foreground)',
}

function parseOptional(raw: string): number | undefined {
  if (raw.trim() === '') return undefined
  const n = Number(raw)
  return Number.isFinite(n) ? n : undefined
}

interface OptNumberProps {
  name: string
  label: ReactNode
  value: number | undefined
  defaultValue: number
  onChange: (v: number | undefined) => void
  disabled?: boolean
  min?: number
  max?: number
  step?: number
  language: string
}

/** Optional number: empty = unset (the default is the placeholder). */
function OptNumber({
  name,
  label,
  value,
  defaultValue,
  onChange,
  disabled,
  min,
  max,
  step,
  language,
}: OptNumberProps) {
  return (
    <label className="flex items-center justify-between gap-3 min-h-[40px]">
      <span className="t-caption text-muted-foreground">{label}</span>
      <input
        type="number"
        name={name}
        autoComplete="off"
        inputMode="decimal"
        value={value ?? ''}
        placeholder={String(defaultValue)}
        title={ts(riskControl.unsetUsesDefault, language).replace(
          '{v}',
          String(defaultValue)
        )}
        onChange={(e) => onChange(parseOptional(e.target.value))}
        disabled={disabled}
        min={min}
        max={max}
        step={step}
        className="w-28 px-3 py-1.5 rounded-control text-sm"
        style={INPUT_STYLE}
      />
    </label>
  )
}

function Group({
  title,
  summary,
  children,
}: {
  title: string
  summary?: string
  children: ReactNode
}) {
  return (
    <div
      className="p-4 rounded-lg space-y-1"
      style={{
        background: 'var(--color-background)',
        border: '1px solid var(--color-border)',
      }}
    >
      <SectionHeader
        level="group"
        title={title}
        summary={summary}
        className="mb-2"
      />
      {children}
    </div>
  )
}

interface Props {
  config: RiskControlConfig
  onChange: (config: RiskControlConfig) => void
  disabled?: boolean
  language: string
}

export function RiskControlAdvanced({
  config,
  onChange,
  disabled,
  language,
}: Props) {
  const set = (patch: Partial<RiskControlConfig>) => {
    if (!disabled) onChange({ ...config, ...patch })
  }
  const breaker: DailyLossBreakerConfig = config.daily_loss_breaker ?? {}
  const setBreaker = (patch: Partial<DailyLossBreakerConfig>) =>
    set({ daily_loss_breaker: { ...breaker, ...patch } })
  const cooldowns: CooldownConfig = config.cooldowns ?? {}
  const recentLoss = cooldowns.recent_loss ?? {}
  const setCooldowns = (patch: Partial<CooldownConfig>) =>
    set({ cooldowns: { ...cooldowns, ...patch } })
  const protector: ProtectorCoreConfig = config.protector ?? {}
  const setProtector = (patch: Partial<ProtectorCoreConfig>) =>
    set({ protector: { ...protector, ...patch } })
  const roeLevel = (v: number | undefined): ProtectorLevel | undefined =>
    v === undefined ? undefined : { value: v, unit: 'roe_pct' }
  const guards = config.setup_guard ?? {}
  const setGuard = (setup: string, patch: Partial<SetupGuardConfig>) => {
    const base = guards[setup] ?? {}
    const next: SetupGuardConfig = {
      ...BUILTIN_SETUP_GUARDS[setup],
      ...base,
      ...patch,
    }
    set({ setup_guard: { ...guards, [setup]: next } })
  }
  const tr = (k: keyof typeof riskControl) => ts(riskControl[k], language)
  const setupName = (setup: string) =>
    setupLabel(setup, language === 'zh' ? 'zh' : 'en')

  return (
    <div data-testid="risk-advanced">
      <SectionHeader
        icon={<SlidersHorizontal className="w-5 h-5" />}
        title={tr('advancedRisk')}
        summary={tr('advancedRiskDesc')}
        className="mb-4"
      />
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Group title={tr('geometryRepair')} summary={tr('geometryRepairDesc')}>
          <AiTSelect
            value={config.geometry_repair === 'reject' ? 'reject' : 'repair'}
            onChange={(v) =>
              set({ geometry_repair: v === 'reject' ? 'reject' : 'repair' })
            }
            disabled={disabled}
            className="w-full px-3 py-2 rounded-control text-sm"
            options={[
              { value: 'repair', label: tr('geometryRepairRepair') },
              { value: 'reject', label: tr('geometryRepairReject') },
            ]}
          />
          <OptNumber
            name="risk-max-single-trade-loss-pct"
            label={tr('maxSingleTradeLoss')}
            value={config.max_single_trade_loss_pct || undefined}
            defaultValue={DEFAULTS.maxSingleTradeLossPct}
            onChange={(v) => set({ max_single_trade_loss_pct: v })}
            disabled={disabled}
            min={0}
            max={100}
            step={0.5}
            language={language}
          />
          <OptNumber
            name="risk-max-take-profit-move-pct"
            label={tr('maxTakeProfitMove')}
            value={config.max_take_profit_price_move_pct || undefined}
            defaultValue={DEFAULTS.maxTakeProfitMovePct}
            onChange={(v) => set({ max_take_profit_price_move_pct: v })}
            disabled={disabled}
            min={0}
            max={100}
            step={0.1}
            language={language}
          />
          <OptNumber
            name="risk-min-stop-loss-move-pct"
            label={tr('minStopLossMove')}
            value={config.min_stop_loss_price_move_pct || undefined}
            defaultValue={DEFAULTS.minStopLossMovePct}
            onChange={(v) => set({ min_stop_loss_price_move_pct: v })}
            disabled={disabled}
            min={0}
            max={100}
            step={0.1}
            language={language}
          />
        </Group>

        <Group
          title={tr('dailyLossBreaker')}
          summary={tr('dailyLossBreakerDesc')}
        >
          <AiTSelect
            value={breaker.mode ?? 'shadow'}
            onChange={(v) =>
              setBreaker({ mode: v as DailyLossBreakerConfig['mode'] })
            }
            disabled={disabled}
            className="w-full px-3 py-2 rounded-control text-sm"
            options={[
              { value: 'off', label: tr('breakerOff') },
              { value: 'shadow', label: tr('breakerShadow') },
              { value: 'armed', label: tr('breakerArmed') },
            ]}
          />
          <OptNumber
            name="risk-breaker-max-loss-pct"
            label={tr('breakerMaxLoss')}
            value={breaker.max_daily_loss_pct || undefined}
            defaultValue={DEFAULTS.breakerMaxLossPct}
            onChange={(v) => setBreaker({ max_daily_loss_pct: v })}
            disabled={disabled}
            min={0}
            max={100}
            step={0.5}
            language={language}
          />
          <OptNumber
            name="risk-breaker-near-fraction"
            label={tr('breakerNearFraction')}
            value={breaker.near_trip_fraction || undefined}
            defaultValue={DEFAULTS.breakerNearFraction}
            onChange={(v) => setBreaker({ near_trip_fraction: v })}
            disabled={disabled}
            min={0}
            max={0.99}
            step={0.05}
            language={language}
          />
        </Group>

        <Group title={tr('cooldowns')} summary={tr('cooldownsDesc')}>
          <OptNumber
            name="risk-failed-open-cycles"
            label={tr('failedOpenCycles')}
            value={cooldowns.failed_open_cycles}
            defaultValue={DEFAULTS.failedOpenCycles}
            onChange={(v) => setCooldowns({ failed_open_cycles: v })}
            disabled={disabled}
            min={0}
            step={1}
            language={language}
          />
          <OptNumber
            name="risk-recent-loss-window"
            label={tr('recentLossWindow')}
            value={recentLoss.window_minutes}
            defaultValue={DEFAULTS.recentLossWindow}
            onChange={(v) =>
              setCooldowns({
                recent_loss: { ...recentLoss, window_minutes: v },
              })
            }
            disabled={disabled}
            min={0}
            step={5}
            language={language}
          />
          <OptNumber
            name="risk-recent-loss-min-losses"
            label={tr('recentLossMinLosses')}
            value={recentLoss.min_losses}
            defaultValue={DEFAULTS.recentLossMinLosses}
            onChange={(v) =>
              setCooldowns({ recent_loss: { ...recentLoss, min_losses: v } })
            }
            disabled={disabled}
            min={1}
            step={1}
            language={language}
          />
          <OptNumber
            name="risk-recent-loss-pct"
            label={tr('recentLossPct')}
            value={recentLoss.loss_pct}
            defaultValue={DEFAULTS.recentLossPct}
            onChange={(v) =>
              setCooldowns({ recent_loss: { ...recentLoss, loss_pct: v } })
            }
            disabled={disabled}
            min={0.1}
            step={0.5}
            language={language}
          />
        </Group>

        <Group title={tr('protectorCore')} summary={tr('protectorCoreDesc')}>
          <OptNumber
            name="risk-protector-tp0"
            label={tr('tp0Level')}
            value={protector.tp0?.unit ? protector.tp0.value : undefined}
            defaultValue={DEFAULTS.tp0}
            onChange={(v) => setProtector({ tp0: roeLevel(v) })}
            disabled={disabled}
            step={0.5}
            language={language}
          />
          <OptNumber
            name="risk-protector-tp1"
            label={tr('tp1Level')}
            value={protector.tp1?.unit ? protector.tp1.value : undefined}
            defaultValue={DEFAULTS.tp1}
            onChange={(v) => setProtector({ tp1: roeLevel(v) })}
            disabled={disabled}
            step={0.5}
            language={language}
          />
          <OptNumber
            name="risk-protector-tp0-close"
            label={tr('tp0CloseRatio')}
            value={protector.tp0_close_ratio || undefined}
            defaultValue={DEFAULTS.tp0CloseRatio}
            onChange={(v) => setProtector({ tp0_close_ratio: v })}
            disabled={disabled}
            min={0}
            max={1}
            step={0.05}
            language={language}
          />
          <OptNumber
            name="risk-protector-tp1-close"
            label={tr('tp1CloseRatio')}
            value={protector.tp1_close_ratio || undefined}
            defaultValue={DEFAULTS.tp1CloseRatio}
            onChange={(v) => setProtector({ tp1_close_ratio: v })}
            disabled={disabled}
            min={0}
            max={1}
            step={0.05}
            language={language}
          />
          <OptNumber
            name="risk-protector-giveback-close"
            label={tr('givebackCloseRatio')}
            value={protector.giveback_close_ratio || undefined}
            defaultValue={DEFAULTS.givebackCloseRatio}
            onChange={(v) => setProtector({ giveback_close_ratio: v })}
            disabled={disabled}
            min={0}
            max={1}
            step={0.05}
            language={language}
          />
          <OptNumber
            name="risk-protector-breakeven-buffer"
            label={tr('breakevenBuffer')}
            value={protector.breakeven_buffer_frac || undefined}
            defaultValue={DEFAULTS.breakevenBufferFrac}
            onChange={(v) => setProtector({ breakeven_buffer_frac: v })}
            disabled={disabled}
            min={0}
            step={0.0005}
            language={language}
          />
        </Group>
      </div>

      <div className="mt-4">
        <Group title={tr('setupGuard')} summary={tr('setupGuardDesc')}>
          <div className="ait-table-scroll">
            <table className="w-full text-sm">
              <thead>
                <tr className="t-caption text-muted-foreground text-left">
                  <th className="py-1.5 pr-3 font-medium">
                    {tr('setupGuardSetup')}
                  </th>
                  <th className="py-1.5 pr-3 font-medium">
                    {tr('setupGuardMinZoneShort')}
                  </th>
                  <th className="py-1.5 pr-3 font-medium">
                    {tr('setupGuardMaxZoneLong')}
                  </th>
                  <th className="py-1.5 font-medium">
                    {tr('setupGuardOIFlush')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {Object.entries(BUILTIN_SETUP_GUARDS).map(([setup, def]) => {
                  const g = guards[setup]
                  return (
                    <tr key={setup} className="border-t border-border">
                      <td className="py-1.5 pr-3 whitespace-nowrap">
                        {setupName(setup)}
                      </td>
                      <td className="py-1.5 pr-3">
                        <input
                          type="number"
                          name={`guard-${setup}-min-zone-short`}
                          aria-label={`${setupName(setup)} ${tr('setupGuardMinZoneShort')}`}
                          value={g?.min_zone_pos_short ?? ''}
                          placeholder={String(def.min_zone_pos_short)}
                          min={0}
                          max={100}
                          step={1}
                          disabled={disabled}
                          onChange={(e) =>
                            setGuard(setup, {
                              min_zone_pos_short:
                                parseOptional(e.target.value) ??
                                def.min_zone_pos_short,
                            })
                          }
                          className="w-20 px-2 py-1 rounded-control"
                          style={INPUT_STYLE}
                        />
                      </td>
                      <td className="py-1.5 pr-3">
                        <input
                          type="number"
                          name={`guard-${setup}-max-zone-long`}
                          aria-label={`${setupName(setup)} ${tr('setupGuardMaxZoneLong')}`}
                          value={g?.max_zone_pos_long ?? ''}
                          placeholder={String(def.max_zone_pos_long)}
                          min={0}
                          max={100}
                          step={1}
                          disabled={disabled}
                          onChange={(e) =>
                            setGuard(setup, {
                              max_zone_pos_long:
                                parseOptional(e.target.value) ??
                                def.max_zone_pos_long,
                            })
                          }
                          className="w-20 px-2 py-1 rounded-control"
                          style={INPUT_STYLE}
                        />
                      </td>
                      <td className="py-1.5">
                        <input
                          type="checkbox"
                          name={`guard-${setup}-oi-flush`}
                          aria-label={`${setupName(setup)} ${tr('setupGuardOIFlush')}`}
                          checked={g?.require_oi_flush ?? def.require_oi_flush}
                          disabled={disabled}
                          onChange={(e) =>
                            setGuard(setup, {
                              require_oi_flush: e.target.checked,
                            })
                          }
                        />
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </Group>
      </div>
    </div>
  )
}
