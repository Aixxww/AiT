import { useState, type ReactNode } from 'react'
import {
  Plus,
  X,
  Database,
  TrendingUp,
  TrendingDown,
  List,
  Ban,
  Zap,
  Shuffle,
  Flame,
  Crosshair,
  Radar,
  Activity,
  Check,
} from 'lucide-react'
import type { CoinSourceConfig } from '../../types'
import { coinSource, ts } from '../../i18n/strategy-translations'
import { AiTSelect } from '../ui/select'
import { Button } from '../ui/Button'
import { SectionHeader } from '../ui/SectionHeader'
import { SwitchRow } from '../ui/SwitchRow'
import {
  displayName,
  setupBaseLabel,
  setupLabel,
} from '../../i18n/displayNames'
import type { Language } from '../../i18n/translations'

// Canonical setup_type list for the Task-2 transplant module checklist.
// Source of truth: V7T2SetupTypes in provider/local/hunter_v7_t2_setups.go
// (dispatcher evaluation order) — keep this mirror in sync with the Go registry.
const V7_T2_SETUP_TYPES = [
  'momentum_continuation_long',
  'momentum_continuation_short',
  'momentum_continuation_watch_long',
  'momentum_continuation_watch_short',
  'bb_oi_accumulation_long',
  'bb_oi_accumulation_short',
  'confirmed_breakout_long',
  'confirmed_breakout_short',
  'reclaim_breakout_long',
  'reclaim_breakout_short',
  'trend_pullback_long',
  'trend_pullback_short',
  'ignition_early_long',
  'ignition_early_short',
  'compression_oi_watch_long',
  'compression_oi_watch_short',
]

// Families (Long/Short twins) in dispatcher order, derived from the list above.
const V7_T2_FAMILIES = Array.from(
  new Set(V7_T2_SETUP_TYPES.map((s) => s.replace(/_(long|short)$/, '')))
)

const FIELD_SELECT =
  'h-9 min-w-[96px] px-3 rounded-control bg-background border border-border text-foreground'

function FieldRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3 min-h-[40px]">
      <span className="t-caption text-muted-foreground">{label}</span>
      {children}
    </div>
  )
}

interface CoinSourceEditorProps {
  config: CoinSourceConfig
  onChange: (config: CoinSourceConfig) => void
  disabled?: boolean
  language: string
}

export function CoinSourceEditor({
  config,
  onChange,
  disabled,
  language,
}: CoinSourceEditorProps) {
  const [newCoin, setNewCoin] = useState('')
  const [newExcludedCoin, setNewExcludedCoin] = useState('')
  const selectedT2 = config.hunter_config?.v7_t2_setups || []
  const lang: Language = language === 'zh' ? 'zh' : 'en'

  // Toggle one setup family in the Task-2 module checklist ([]string round-trip;
  // empty list means all families enabled on the backend)
  const toggleT2Setup = (setup: string) => {
    if (disabled) return
    const current = config.hunter_config?.v7_t2_setups || []
    const next = current.includes(setup)
      ? current.filter((s) => s !== setup)
      : [...current, setup]
    onChange({
      ...config,
      hunter_config: {
        ...(config.hunter_config || {}),
        v7_t2_setups: next,
      },
    })
  }

  const sourceTypes = [
    { value: 'static', icon: List, color: 'var(--color-muted-fg)' },
    { value: 'ai500', icon: Database, color: 'var(--color-primary)' },
    { value: 'oi_top', icon: TrendingUp, color: 'var(--color-profit)' },
    { value: 'oi_low', icon: TrendingDown, color: 'var(--color-loss)' },
    { value: 'square_heat', icon: Flame, color: 'var(--color-warning)' },
    { value: 'hunter', icon: Crosshair, color: 'var(--color-purple)' },
    { value: 'hunter_sniff', icon: Radar, color: 'var(--color-purple)' },
    { value: 'hunter_v7', icon: Zap, color: 'var(--color-accent)' },
    { value: 'indicator_hub', icon: Activity, color: 'var(--color-info)' },
  ] as const

  // Calculate mixed mode summary
  const getMixedSummary = () => {
    const sources: string[] = []
    let totalLimit = 0

    if (config.use_ai500) {
      sources.push(`AI500(${config.ai500_limit || 3})`)
      totalLimit += config.ai500_limit || 3
    }
    if (config.use_oi_top) {
      sources.push(
        `${ts(coinSource.oiIncreaseShort, language)}(${config.oi_top_limit || 3})`
      )
      totalLimit += config.oi_top_limit || 3
    }
    if (config.use_oi_low) {
      sources.push(
        `${ts(coinSource.oiDecreaseShort, language)}(${config.oi_low_limit || 3})`
      )
      totalLimit += config.oi_low_limit || 3
    }
    if ((config.static_coins || []).length > 0) {
      sources.push(
        `${ts(coinSource.custom, language)}(${config.static_coins?.length || 0})`
      )
      totalLimit += config.static_coins?.length || 0
    }
    if (config.use_square_heat) {
      sources.push(`SQ(${config.square_heat_limit || 10})`)
      totalLimit += config.square_heat_limit || 10
    }
    if (config.use_hunter) {
      const dirBadge =
        config.hunter_direction === 'LONG'
          ? '↑'
          : config.hunter_direction === 'SHORT'
            ? '↓'
            : ''
      sources.push(`HN${dirBadge}(${config.hunter_limit || 10})`)
      totalLimit += config.hunter_limit || 10
    }

    return { sources, totalLimit }
  }

  // xyz dex assets (stocks, forex, commodities) - should NOT get USDT suffix
  const xyzDexAssets = new Set([
    // Stocks
    'TSLA',
    'NVDA',
    'AAPL',
    'MSFT',
    'META',
    'AMZN',
    'GOOGL',
    'AMD',
    'COIN',
    'NFLX',
    'PLTR',
    'HOOD',
    'INTC',
    'MSTR',
    'TSM',
    'ORCL',
    'MU',
    'RIVN',
    'COST',
    'LLY',
    'CRCL',
    'SKHX',
    'SNDK',
    // Forex
    'EUR',
    'JPY',
    // Commodities
    'GOLD',
    'SILVER',
    // Index
    'XYZ100',
  ])

  const isXyzDexAsset = (symbol: string): boolean => {
    const base = symbol
      .toUpperCase()
      .replace(/^XYZ:/, '')
      .replace(/USDT$|USD$|-USDC$/, '')
    return xyzDexAssets.has(base)
  }

  const MAX_STATIC_COINS = 10

  const showToast = (msg: string) => {
    const toast = document.createElement('div')
    toast.textContent = msg
    toast.className =
      'fixed top-4 left-1/2 -translate-x-1/2 px-4 py-2 rounded-lg text-sm z-50 shadow-lg'
    toast.style.cssText = 'background:var(--color-loss);color:white;'
    document.body.appendChild(toast)
    setTimeout(() => toast.remove(), 2000)
  }

  const handleAddCoin = () => {
    if (!newCoin.trim()) return

    const currentCoins = config.static_coins || []
    if (currentCoins.length >= MAX_STATIC_COINS) {
      showToast(
        ts(coinSource.maxCoinsToast, language).replace(
          '{n}',
          String(MAX_STATIC_COINS)
        )
      )
      return
    }

    const symbol = newCoin.toUpperCase().trim()

    // For xyz dex assets (stocks, forex, commodities), use xyz: prefix without USDT
    let formattedSymbol: string
    if (isXyzDexAsset(symbol)) {
      // Remove xyz: prefix (case-insensitive) and any USD suffixes
      const base = symbol
        .replace(/^xyz:/i, '')
        .replace(/USDT$|USD$|-USDC$/i, '')
      formattedSymbol = `xyz:${base}`
    } else {
      formattedSymbol = symbol.endsWith('USDT') ? symbol : `${symbol}USDT`
    }

    if (!currentCoins.includes(formattedSymbol)) {
      onChange({
        ...config,
        static_coins: [...currentCoins, formattedSymbol],
      })
    }
    setNewCoin('')
  }

  const handleRemoveCoin = (coin: string) => {
    onChange({
      ...config,
      static_coins: (config.static_coins || []).filter((c) => c !== coin),
    })
  }

  const handleAddExcludedCoin = () => {
    if (!newExcludedCoin.trim()) return
    const symbol = newExcludedCoin.toUpperCase().trim()

    // For xyz dex assets, use xyz: prefix without USDT
    let formattedSymbol: string
    if (isXyzDexAsset(symbol)) {
      const base = symbol
        .replace(/^xyz:/i, '')
        .replace(/USDT$|USD$|-USDC$/i, '')
      formattedSymbol = `xyz:${base}`
    } else {
      formattedSymbol = symbol.endsWith('USDT') ? symbol : `${symbol}USDT`
    }

    const currentExcluded = config.excluded_coins || []
    if (!currentExcluded.includes(formattedSymbol)) {
      onChange({
        ...config,
        excluded_coins: [...currentExcluded, formattedSymbol],
      })
    }
    setNewExcludedCoin('')
  }

  const handleRemoveExcludedCoin = (coin: string) => {
    onChange({
      ...config,
      excluded_coins: (config.excluded_coins || []).filter((c) => c !== coin),
    })
  }

  // Binance data source badge component
  const BinanceBadge = () => (
    <span className="text-[9px] px-1.5 py-0.5 rounded font-medium bg-accent-purple/20 text-accent-purple border border-accent-purple/30">
      Binance
    </span>
  )

  return (
    <div className="space-y-6">
      {/* Source Type Selector */}
      <div>
        <label className="block t-label mb-3">
          {ts(coinSource.sourceType, language)}
        </label>
        <div className="grid grid-cols-2 sm:grid-cols-3 2xl:grid-cols-4 gap-2">
          {sourceTypes.map(({ value, icon: Icon, color }) => (
            <Button
              variant="unstyled"
              key={value}
              onClick={() => {
                if (disabled) return
                const sourceType = value as CoinSourceConfig['source_type']
                if (sourceType === 'hunter_v7') {
                  onChange({
                    ...config,
                    source_type: sourceType,
                    use_hunter: true,
                    hunter_limit: config.hunter_limit || 10,
                    hunter_direction: config.hunter_direction || 'BOTH',
                    hunter_config: {
                      ...(config.hunter_config || {}),
                      v7_max_output: config.hunter_config?.v7_max_output ?? 30,
                      v7_watch_output:
                        config.hunter_config?.v7_watch_output ?? 5,
                      v7_min_ai_priority:
                        config.hunter_config?.v7_min_ai_priority ?? 55,
                      v7_aggressive:
                        config.hunter_config?.v7_aggressive ?? true,
                      include_non_crypto_futures:
                        config.hunter_config?.include_non_crypto_futures ??
                        false,
                    },
                  })
                  return
                }
                onChange({ ...config, source_type: sourceType })
              }}
              disabled={disabled}
              aria-pressed={config.source_type === value}
              className={`flex flex-col items-start p-3 rounded-control border text-left transition-colors ${
                config.source_type === value
                  ? 'border-primary/60 bg-primary-dim'
                  : 'border-border bg-panel hover:border-border-hover'
              }`}
            >
              <Icon className="w-5 h-5 mb-2" style={{ color }} />
              <div className="t-label">
                {ts(coinSource[value as keyof typeof coinSource], language)}
              </div>
              <div className="t-micro font-normal mt-0.5 text-muted-foreground line-clamp-2">
                {ts(
                  coinSource[`${value}Desc` as keyof typeof coinSource],
                  language
                )}
              </div>
            </Button>
          ))}
        </div>
      </div>

      {/* Static Coins - only for static mode */}
      {config.source_type === 'static' && (
        <div>
          <label className="block text-sm font-medium mb-3 text-foreground">
            {ts(coinSource.staticCoins, language)}
          </label>
          <div className="flex flex-wrap gap-2 mb-3">
            {(config.static_coins || []).map((coin) => (
              <span
                key={coin}
                className="flex items-center gap-1 px-3 py-1.5 rounded-full text-sm bg-surface text-foreground"
              >
                {coin}
                {!disabled && (
                  <Button
                    variant="unstyled"
                    onClick={() => handleRemoveCoin(coin)}
                    className="ml-1 hover:text-loss transition-colors"
                  >
                    <X className="w-3 h-3" />
                  </Button>
                )}
              </span>
            ))}
          </div>
          {!disabled && (
            <div className="flex gap-2">
              <input
                type="text"
                value={newCoin}
                onChange={(e) => setNewCoin(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleAddCoin()}
                placeholder="BTC, ETH, SOL..."
                className="flex-1 min-w-0 h-10 px-3 rounded-control bg-background border border-border text-foreground"
              />
              <Button
                variant="unstyled"
                onClick={handleAddCoin}
                className="shrink-0 whitespace-nowrap px-4 py-2 rounded-control flex items-center gap-2 transition-colors bg-primary text-primary-foreground hover:opacity-90"
              >
                <Plus className="w-4 h-4" />
                {ts(coinSource.addCoin, language)}
              </Button>
            </div>
          )}
        </div>
      )}

      {/* Excluded Coins */}
      <div>
        <div className="flex items-center gap-2 mb-3">
          <Ban className="w-4 h-4 text-loss" />
          <label className="text-sm font-medium text-foreground">
            {ts(coinSource.excludedCoins, language)}
          </label>
        </div>
        <p className="text-xs mb-3 text-muted-foreground">
          {ts(coinSource.excludedCoinsDesc, language)}
        </p>
        <div className="flex flex-wrap gap-2 mb-3">
          {(config.excluded_coins || []).map((coin) => (
            <span
              key={coin}
              className="flex items-center gap-1 px-3 py-1.5 rounded-full text-sm bg-loss/15 text-loss"
            >
              {coin}
              {!disabled && (
                <Button
                  variant="unstyled"
                  onClick={() => handleRemoveExcludedCoin(coin)}
                  className="ml-1 hover:text-foreground transition-colors"
                >
                  <X className="w-3 h-3" />
                </Button>
              )}
            </span>
          ))}
          {(config.excluded_coins || []).length === 0 && (
            <span className="text-xs italic text-muted-foreground">
              {ts(coinSource.excludedNone, language)}
            </span>
          )}
        </div>
        {!disabled && (
          <div className="flex gap-2">
            <input
              type="text"
              value={newExcludedCoin}
              onChange={(e) => setNewExcludedCoin(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleAddExcludedCoin()}
              placeholder="BTC, ETH, DOGE..."
              className="flex-1 min-w-0 h-10 px-3 rounded-control text-sm bg-background border border-border text-foreground"
            />
            <Button
              variant="unstyled"
              onClick={handleAddExcludedCoin}
              className="shrink-0 whitespace-nowrap px-4 py-2 rounded-control flex items-center gap-2 transition-colors text-sm bg-loss text-white hover:opacity-90"
            >
              <Ban className="w-4 h-4" />
              {ts(coinSource.addExcludedCoin, language)}
            </Button>
          </div>
        )}
      </div>

      {/* AI500 Options - only for ai500 mode */}
      {config.source_type === 'ai500' && (
        <div className="p-4 rounded-lg bg-primary/5 border border-primary/20">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              <Zap className="w-4 h-4 text-primary" />
              <span className="text-sm font-medium text-foreground">
                AI500 {ts(coinSource.dataSourceConfig, language)}
              </span>
              <BinanceBadge />
            </div>
          </div>

          <div className="space-y-3">
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                checked={config.use_ai500}
                onChange={(e) =>
                  !disabled &&
                  onChange({ ...config, use_ai500: e.target.checked })
                }
                disabled={disabled}
                className="w-5 h-5 rounded accent-primary"
              />
              <span className="text-foreground">
                {ts(coinSource.useAI500, language)}
              </span>
            </label>

            {config.use_ai500 && (
              <div className="flex items-center gap-3 pl-8">
                <span className="text-sm text-muted-foreground">
                  {ts(coinSource.ai500Limit, language)}:
                </span>
                <AiTSelect
                  value={config.ai500_limit || 3}
                  onChange={(val) =>
                    !disabled &&
                    onChange({ ...config, ai500_limit: parseInt(val) || 3 })
                  }
                  disabled={disabled}
                  options={[1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((n) => ({
                    value: n,
                    label: String(n),
                  }))}
                  className="px-3 py-1.5 rounded bg-background border border-primary/20 text-foreground"
                />
              </div>
            )}

            <p className="text-xs pl-8 text-muted-foreground">
              {ts(coinSource.binanceNote, language)}
            </p>
          </div>
        </div>
      )}

      {/* OI Top Options - only for oi_top mode */}
      {config.source_type === 'oi_top' && (
        <div className="p-4 rounded-lg bg-profit/5 border border-profit/20">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              <TrendingUp className="w-4 h-4 text-profit" />
              <span className="text-sm font-medium text-foreground">
                {ts(coinSource.oiIncreaseTitle, language)}{' '}
                {ts(coinSource.dataSourceConfig, language)}
              </span>
              <BinanceBadge />
            </div>
          </div>

          <div className="space-y-3">
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                checked={config.use_oi_top}
                onChange={(e) =>
                  !disabled &&
                  onChange({ ...config, use_oi_top: e.target.checked })
                }
                disabled={disabled}
                className="w-5 h-5 rounded accent-profit"
              />
              <span className="text-foreground">
                {ts(coinSource.useOITop, language)}
              </span>
            </label>

            {config.use_oi_top && (
              <div className="flex items-center gap-3 pl-8">
                <span className="text-sm text-muted-foreground">
                  {ts(coinSource.oiTopLimit, language)}:
                </span>
                <AiTSelect
                  value={config.oi_top_limit || 3}
                  onChange={(val) =>
                    !disabled &&
                    onChange({ ...config, oi_top_limit: parseInt(val) || 3 })
                  }
                  disabled={disabled}
                  options={[1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((n) => ({
                    value: n,
                    label: String(n),
                  }))}
                  className="px-3 py-1.5 rounded bg-background border border-primary/20 text-foreground"
                />
              </div>
            )}

            <p className="text-xs pl-8 text-muted-foreground">
              {ts(coinSource.binanceNote, language)}
            </p>
          </div>
        </div>
      )}

      {/* OI Low Options - only for oi_low mode */}
      {config.source_type === 'oi_low' && (
        <div className="p-4 rounded-lg bg-loss/5 border border-loss/20">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              <TrendingDown className="w-4 h-4 text-loss" />
              <span className="text-sm font-medium text-foreground">
                {ts(coinSource.oiDecreaseTitle, language)}{' '}
                {ts(coinSource.dataSourceConfig, language)}
              </span>
              <BinanceBadge />
            </div>
          </div>

          <div className="space-y-3">
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                checked={config.use_oi_low}
                onChange={(e) =>
                  !disabled &&
                  onChange({ ...config, use_oi_low: e.target.checked })
                }
                disabled={disabled}
                className="w-5 h-5 rounded accent-loss"
              />
              <span className="text-foreground">
                {ts(coinSource.useOILow, language)}
              </span>
            </label>

            {config.use_oi_low && (
              <div className="flex items-center gap-3 pl-8">
                <span className="text-sm text-muted-foreground">
                  {ts(coinSource.oiLowLimit, language)}:
                </span>
                <AiTSelect
                  value={config.oi_low_limit || 3}
                  onChange={(val) =>
                    !disabled &&
                    onChange({ ...config, oi_low_limit: parseInt(val) || 3 })
                  }
                  disabled={disabled}
                  options={[1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((n) => ({
                    value: n,
                    label: String(n),
                  }))}
                  className="px-3 py-1.5 rounded bg-background border border-primary/20 text-foreground"
                />
              </div>
            )}

            <p className="text-xs pl-8 text-muted-foreground">
              {ts(coinSource.binanceNote, language)}
            </p>
          </div>
        </div>
      )}

      {/* Hunter Options - only for hunter mode */}
      {config.source_type === 'hunter' && (
        <div className="p-4 rounded-lg bg-accent-purple/5 border border-accent-purple/20">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              <Crosshair className="w-4 h-4 text-accent-purple" />
              <span className="text-sm font-medium text-foreground">
                {ts(coinSource.hunter, language)}{' '}
                {ts(coinSource.dataSourceConfig, language)}
              </span>
              <BinanceBadge />
            </div>
          </div>

          <div className="space-y-3">
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                checked={config.use_hunter || false}
                onChange={(e) =>
                  !disabled &&
                  onChange({ ...config, use_hunter: e.target.checked })
                }
                disabled={disabled}
                className="w-5 h-5 rounded accent-accent-purple"
              />
              <span className="text-foreground">
                {ts(coinSource.useHunter, language)}
              </span>
            </label>

            {config.use_hunter && (
              <div className="flex items-center gap-3 pl-8">
                <span className="text-sm text-muted-foreground">
                  {ts(coinSource.hunterLimit, language)}:
                </span>
                <AiTSelect
                  value={config.hunter_limit || 10}
                  onChange={(val) =>
                    !disabled &&
                    onChange({ ...config, hunter_limit: parseInt(val) || 10 })
                  }
                  disabled={disabled}
                  options={[3, 5, 8, 10, 15, 20].map((n) => ({
                    value: n,
                    label: String(n),
                  }))}
                  className="px-3 py-1.5 rounded bg-background border border-primary/20 text-foreground"
                />
              </div>
            )}

            {/* Hunter Direction Badge — multi-select: LONG / SHORT / BOTH */}
            {config.use_hunter && (
              <div className="flex items-center gap-3 pl-8">
                <span className="text-sm text-muted-foreground">
                  {ts(coinSource.hunterDirection, language)}:
                </span>
                <div className="flex gap-2">
                  <Button
                    variant="unstyled"
                    onClick={() => {
                      if (disabled) return
                      const d = config.hunter_direction
                      const hasLong = d === 'LONG' || d === 'BOTH'
                      const hasShort = d === 'SHORT' || d === 'BOTH'
                      const nextLong = !hasLong
                      if (nextLong && hasShort)
                        onChange({ ...config, hunter_direction: 'BOTH' })
                      else if (nextLong)
                        onChange({ ...config, hunter_direction: 'LONG' })
                      else if (hasShort)
                        onChange({ ...config, hunter_direction: 'SHORT' })
                      else onChange({ ...config, hunter_direction: undefined })
                    }}
                    disabled={disabled}
                    className={`px-3 py-1.5 rounded text-xs font-medium transition-all ${
                      config.hunter_direction === 'LONG' ||
                      config.hunter_direction === 'BOTH'
                        ? 'bg-profit/20 text-profit border border-profit/50'
                        : 'bg-background text-muted-foreground border border-ait-border hover:border-profit/30'
                    }`}
                  >
                    ▲ {ts(coinSource.hunterDirectionLong, language)}
                  </Button>
                  <Button
                    variant="unstyled"
                    onClick={() => {
                      if (disabled) return
                      const d = config.hunter_direction
                      const hasLong = d === 'LONG' || d === 'BOTH'
                      const hasShort = d === 'SHORT' || d === 'BOTH'
                      const nextShort = !hasShort
                      if (nextShort && hasLong)
                        onChange({ ...config, hunter_direction: 'BOTH' })
                      else if (nextShort)
                        onChange({ ...config, hunter_direction: 'SHORT' })
                      else if (hasLong)
                        onChange({ ...config, hunter_direction: 'LONG' })
                      else onChange({ ...config, hunter_direction: undefined })
                    }}
                    disabled={disabled}
                    className={`px-3 py-1.5 rounded text-xs font-medium transition-all ${
                      config.hunter_direction === 'SHORT' ||
                      config.hunter_direction === 'BOTH'
                        ? 'bg-loss/20 text-loss border border-loss/50'
                        : 'bg-background text-muted-foreground border border-ait-border hover:border-loss/30'
                    }`}
                  >
                    ▼ {ts(coinSource.hunterDirectionShort, language)}
                  </Button>
                </div>
              </div>
            )}

            {config.use_hunter && (
              <details className="pl-8 mt-2">
                <summary className="text-xs cursor-pointer text-muted-foreground hover:text-foreground select-none">
                  {ts(coinSource.advAdvancedFilters, language)} ▸
                </summary>
                <div className="mt-3 space-y-2.5 p-3 rounded-lg bg-background border border-ait-border">
                  {/* Strategy Mode Toggle */}
                  <div className="flex items-center gap-2 mb-1">
                    <span className="text-xs text-muted-foreground">
                      {ts(coinSource.advStrategyMode, language)}:
                    </span>
                    <div className="flex gap-1.5">
                      <Button
                        variant="unstyled"
                        onClick={() =>
                          !disabled &&
                          onChange({
                            ...config,
                            hunter_config: {
                              ...(config.hunter_config || {}),
                              strategy_mode: 'default',
                            },
                          })
                        }
                        disabled={disabled}
                        className={`px-2.5 py-1 rounded text-xs font-medium transition-all ${
                          (config.hunter_config?.strategy_mode || 'default') ===
                          'default'
                            ? 'bg-info/20 text-info border border-info/50'
                            : 'bg-background text-muted-foreground border border-ait-border hover:border-info/30'
                        }`}
                      >
                        {ts(coinSource.advDefault, language)}
                      </Button>
                      <Button
                        variant="unstyled"
                        onClick={() =>
                          !disabled &&
                          onChange({
                            ...config,
                            hunter_config: {
                              ...(config.hunter_config || {}),
                              strategy_mode: 'breakout',
                            },
                          })
                        }
                        disabled={disabled}
                        className={`px-2.5 py-1 rounded text-xs font-medium transition-all ${
                          config.hunter_config?.strategy_mode === 'breakout'
                            ? 'bg-warning/20 text-warning border border-warning/50'
                            : 'bg-background text-muted-foreground border border-ait-border hover:border-warning/30'
                        }`}
                      >
                        {ts(coinSource.advBreakout, language)}
                      </Button>
                    </div>
                  </div>

                  {/* Breakout mode info box */}
                  {config.hunter_config?.strategy_mode === 'breakout' && (
                    <div className="p-2 rounded bg-warning/5 border border-warning/20 text-xs text-warning/80 space-y-0.5">
                      {ts(coinSource.breakoutModeInfo, language)
                        .split('\n')
                        .map((line, i) => (
                          <div key={i}>{line}</div>
                        ))}
                    </div>
                  )}

                  {/* Min OI Value */}
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-muted-foreground">
                      {ts(coinSource.advMinOI, language)}
                    </span>
                    <AiTSelect
                      value={String(
                        config.hunter_config?.min_oi_value || 5000000
                      )}
                      onChange={(val) =>
                        !disabled &&
                        onChange({
                          ...config,
                          hunter_config: {
                            ...(config.hunter_config || {}),
                            min_oi_value: Number(val),
                          },
                        })
                      }
                      disabled={disabled}
                      options={[
                        { value: '2000000', label: '$2M' },
                        { value: '5000000', label: '$5M' },
                        { value: '10000000', label: '$10M' },
                        { value: '15000000', label: '$15M' },
                      ]}
                      className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                    />
                  </div>

                  {/* RSI Oversold */}
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-muted-foreground">
                      {ts(coinSource.advRSIOversold, language)}
                    </span>
                    <AiTSelect
                      value={String(
                        config.hunter_config?.rsi_oversold_threshold || 30
                      )}
                      onChange={(val) =>
                        !disabled &&
                        onChange({
                          ...config,
                          hunter_config: {
                            ...(config.hunter_config || {}),
                            rsi_oversold_threshold: Number(val),
                          },
                        })
                      }
                      disabled={disabled}
                      options={[20, 25, 30, 35, 40].map((n) => ({
                        value: String(n),
                        label: String(n),
                      }))}
                      className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                    />
                  </div>

                  {/* RSI Overbought */}
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-muted-foreground">
                      {ts(coinSource.advRSIOverbought, language)}
                    </span>
                    <AiTSelect
                      value={String(
                        config.hunter_config?.rsi_overbought_threshold || 70
                      )}
                      onChange={(val) =>
                        !disabled &&
                        onChange({
                          ...config,
                          hunter_config: {
                            ...(config.hunter_config || {}),
                            rsi_overbought_threshold: Number(val),
                          },
                        })
                      }
                      disabled={disabled}
                      options={[60, 65, 70, 75, 80].map((n) => ({
                        value: String(n),
                        label: String(n),
                      }))}
                      className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                    />
                  </div>

                  {/* Funding Rate Signal toggle */}
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={
                        config.hunter_config?.enable_funding_rate_signal !==
                        false
                      }
                      onChange={(e) =>
                        !disabled &&
                        onChange({
                          ...config,
                          hunter_config: {
                            ...(config.hunter_config || {}),
                            enable_funding_rate_signal: e.target.checked,
                          },
                        })
                      }
                      disabled={disabled}
                      className="w-4 h-4 rounded accent-accent-purple"
                    />
                    <span className="text-xs text-foreground">
                      {ts(coinSource.advFundingRateSignal, language)}
                    </span>
                  </label>

                  {/* Max 24h Change */}
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-muted-foreground">
                      {ts(coinSource.advMax24hChange, language)}
                    </span>
                    <AiTSelect
                      value={String(config.hunter_config?.max_24h_change || 50)}
                      onChange={(val) =>
                        !disabled &&
                        onChange({
                          ...config,
                          hunter_config: {
                            ...(config.hunter_config || {}),
                            max_24h_change: Number(val),
                          },
                        })
                      }
                      disabled={disabled}
                      options={[
                        {
                          value: '0',
                          label: ts(coinSource.advNone, language),
                        },
                        { value: '20', label: '20%' },
                        { value: '30', label: '30%' },
                        { value: '50', label: '50%' },
                        { value: '100', label: '100%' },
                      ]}
                      className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                    />
                  </div>

                  {/* Wash Trade Sensitivity */}
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-muted-foreground">
                      {ts(coinSource.advWashDetection, language)}
                    </span>
                    <AiTSelect
                      value={
                        config.hunter_config?.wash_trade_sensitivity || 'medium'
                      }
                      onChange={(val) =>
                        !disabled &&
                        onChange({
                          ...config,
                          hunter_config: {
                            ...(config.hunter_config || {}),
                            wash_trade_sensitivity: val as
                              | 'low'
                              | 'medium'
                              | 'high',
                          },
                        })
                      }
                      disabled={disabled}
                      options={[
                        {
                          value: 'low',
                          label: ts(coinSource.advLow, language),
                        },
                        {
                          value: 'medium',
                          label: ts(coinSource.advMedium, language),
                        },
                        {
                          value: 'high',
                          label: ts(coinSource.advHigh, language),
                        },
                      ]}
                      className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                    />
                  </div>

                  {/* Cooldown toggle */}
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={config.hunter_config?.enable_cooldown !== false}
                      onChange={(e) =>
                        !disabled &&
                        onChange({
                          ...config,
                          hunter_config: {
                            ...(config.hunter_config || {}),
                            enable_cooldown: e.target.checked,
                          },
                        })
                      }
                      disabled={disabled}
                      className="w-4 h-4 rounded accent-accent-purple"
                    />
                    <span className="text-xs text-foreground">
                      {ts(coinSource.advSmartCooldown, language)}
                    </span>
                  </label>

                  {/* Min Trade Count */}
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-muted-foreground">
                      {ts(coinSource.advMinTrades, language)}
                    </span>
                    <AiTSelect
                      value={String(
                        config.hunter_config?.min_trade_count || 10000
                      )}
                      onChange={(val) =>
                        !disabled &&
                        onChange({
                          ...config,
                          hunter_config: {
                            ...(config.hunter_config || {}),
                            min_trade_count: Number(val),
                          },
                        })
                      }
                      disabled={disabled}
                      options={[
                        { value: '1000', label: '1K' },
                        { value: '5000', label: '5K' },
                        { value: '10000', label: '10K' },
                        { value: '50000', label: '50K' },
                      ]}
                      className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                    />
                  </div>
                </div>
              </details>
            )}

            <p className="text-xs pl-8 text-muted-foreground">
              {ts(coinSource.hunterDesc, language)}
            </p>
          </div>
        </div>
      )}

      {/* Hunter v7 Options - only for hunter_v7 mode */}
      {config.source_type === 'hunter_v7' && (
        <div className="sm:ait-card-inset sm:p-4 space-y-5">
          <SectionHeader
            icon={<Zap className="w-4 h-4" />}
            title={`${ts(coinSource.hunter_v7, language)} ${ts(coinSource.dataSourceConfig, language)}`}
            summary={ts(coinSource.dataFromBinance, language)}
            info={ts(coinSource.hunter_v7Desc, language)}
          />

          {/* Candidate output */}
          <div>
            <SectionHeader
              level="group"
              title={ts(coinSource.v7OutputGroup, language)}
              summary={ts(coinSource.v7OutputGroupDesc, language)}
              className="mb-3"
            />
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-2.5">
              <FieldRow label={ts(coinSource.hunterLimit, language)}>
                <AiTSelect
                  value={config.hunter_limit || 10}
                  onChange={(val) =>
                    !disabled &&
                    onChange({ ...config, hunter_limit: parseInt(val) || 10 })
                  }
                  disabled={disabled}
                  options={[3, 5, 8, 10, 15, 20].map((n) => ({
                    value: n,
                    label: String(n),
                  }))}
                  className={FIELD_SELECT}
                />
              </FieldRow>
              <FieldRow label={ts(coinSource.hunterDirection, language)}>
                <AiTSelect
                  value={config.hunter_direction || 'BOTH'}
                  onChange={(val) =>
                    !disabled &&
                    onChange({
                      ...config,
                      hunter_direction: val as 'LONG' | 'SHORT' | 'BOTH',
                    })
                  }
                  disabled={disabled}
                  options={[
                    {
                      value: 'BOTH',
                      label: ts(coinSource.hunterDirectionBoth, language),
                    },
                    {
                      value: 'LONG',
                      label: ts(coinSource.hunterDirectionLong, language),
                    },
                    {
                      value: 'SHORT',
                      label: ts(coinSource.hunterDirectionShort, language),
                    },
                  ]}
                  className={FIELD_SELECT}
                />
              </FieldRow>
              <FieldRow label={ts(coinSource.v7MaxOutput, language)}>
                <AiTSelect
                  value={config.hunter_config?.v7_max_output ?? 30}
                  onChange={(val) =>
                    !disabled &&
                    onChange({
                      ...config,
                      hunter_config: {
                        ...(config.hunter_config || {}),
                        v7_max_output: parseInt(val) || 30,
                      },
                    })
                  }
                  disabled={disabled}
                  options={[10, 20, 30, 40, 50, 75].map((n) => ({
                    value: n,
                    label: String(n),
                  }))}
                  className={FIELD_SELECT}
                />
              </FieldRow>
              <FieldRow label={ts(coinSource.v7WatchOutput, language)}>
                <AiTSelect
                  value={String(config.hunter_config?.v7_watch_output ?? 5)}
                  onChange={(val) => {
                    if (disabled) return
                    const n = Number(val)
                    onChange({
                      ...config,
                      hunter_config: {
                        ...(config.hunter_config || {}),
                        v7_watch_output: Number.isFinite(n) ? n : 5,
                      },
                    })
                  }}
                  disabled={disabled}
                  options={[3, 5, 8, 10].map((n) => ({
                    value: String(n),
                    label: String(n),
                  }))}
                  className={FIELD_SELECT}
                />
              </FieldRow>
              <FieldRow label={ts(coinSource.v7MinAIPriority, language)}>
                <AiTSelect
                  value={String(config.hunter_config?.v7_min_ai_priority ?? 55)}
                  onChange={(val) =>
                    !disabled &&
                    onChange({
                      ...config,
                      hunter_config: {
                        ...(config.hunter_config || {}),
                        v7_min_ai_priority: Number(val) || 55,
                      },
                    })
                  }
                  disabled={disabled}
                  options={[40, 45, 50, 55, 60, 65, 70].map((n) => ({
                    value: String(n),
                    label: String(n),
                  }))}
                  className={FIELD_SELECT}
                />
              </FieldRow>
            </div>
          </div>

          {/* Screening preferences */}
          <div>
            <SectionHeader
              level="group"
              title={ts(coinSource.v7FiltersGroup, language)}
              className="mb-1"
            />
            <div className="divide-y divide-border">
              <SwitchRow
                title={ts(coinSource.v7Aggressive, language)}
                summary={ts(coinSource.v7AggressiveDesc, language)}
                checked={config.hunter_config?.v7_aggressive ?? true}
                disabled={disabled}
                onChange={(checked) =>
                  onChange({
                    ...config,
                    hunter_config: {
                      ...(config.hunter_config || {}),
                      v7_aggressive: checked,
                    },
                  })
                }
              />
              <SwitchRow
                title={ts(coinSource.v7ExcludeNonCryptoFutures, language)}
                info={ts(coinSource.v7ExcludeNonCryptoFuturesDesc, language)}
                checked={
                  !(config.hunter_config?.include_non_crypto_futures ?? false)
                }
                disabled={disabled}
                onChange={(checked) =>
                  onChange({
                    ...config,
                    hunter_config: {
                      ...(config.hunter_config || {}),
                      include_non_crypto_futures: !checked,
                    },
                  })
                }
              />
            </div>
          </div>

          {/* Task-2 transplant module (hunter_v7_t2_setups) */}
          <div className="ait-card p-3 sm:p-4" data-shot="t2-modules">
            <SwitchRow
              className="py-0"
              title={ts(coinSource.v7T2Enabled, language)}
              summary={ts(coinSource.v7T2EnabledDesc, language)}
              info={ts(coinSource.v7T2Info, language)}
              checked={config.hunter_config?.v7_t2_enabled ?? false}
              disabled={disabled}
              onChange={(checked) =>
                onChange({
                  ...config,
                  hunter_config: {
                    ...(config.hunter_config || {}),
                    v7_t2_enabled: checked,
                  },
                })
              }
            />

            {(config.hunter_config?.v7_t2_enabled ?? false) && (
              <div className="mt-4 pt-4 border-t border-border space-y-4">
                <div className="flex items-center justify-between gap-3">
                  <SectionHeader
                    level="group"
                    title={ts(coinSource.v7T2Setups, language)}
                    summary={ts(coinSource.v7T2SetupsDesc, language)}
                  />
                  <span className="ait-pill bg-surface-alt text-muted-foreground shrink-0">
                    {selectedT2.length === 0
                      ? ts(coinSource.v7T2AllEnabled, language)
                      : ts(coinSource.v7T2Selected, language).replace(
                          '{n}',
                          String(selectedT2.length)
                        )}
                  </span>
                </div>
                <ul className="divide-y divide-border rounded-control border border-border">
                  {V7_T2_FAMILIES.map((family) => (
                    <li
                      key={family}
                      className="flex items-center justify-between gap-2 px-3 py-2"
                    >
                      <span className="t-label">
                        {setupBaseLabel(family, lang)}
                      </span>
                      <div className="flex gap-2">
                        {(['long', 'short'] as const).map((dir) => {
                          const setup = `${family}_${dir}`
                          const isSelected = selectedT2.includes(setup)
                          return (
                            <button
                              key={setup}
                              type="button"
                              onClick={() => toggleT2Setup(setup)}
                              disabled={disabled}
                              aria-pressed={isSelected}
                              aria-label={setupLabel(setup, lang)}
                              title={setupLabel(setup, lang)}
                              className="ait-chip justify-center min-w-[64px]"
                            >
                              {isSelected && (
                                <Check className="w-3.5 h-3.5 text-primary" />
                              )}
                              {displayName('direction', dir, lang)}
                            </button>
                          )
                        })}
                      </div>
                    </li>
                  ))}
                </ul>

                <SwitchRow
                  className="py-0"
                  title={ts(coinSource.v7T2SymbolValidityGate, language)}
                  summary={ts(coinSource.v7T2SymbolValidityGateDesc, language)}
                  checked={
                    config.hunter_config?.v7_t2_symbol_validity_gate ?? false
                  }
                  disabled={disabled}
                  onChange={(checked) =>
                    onChange({
                      ...config,
                      hunter_config: {
                        ...(config.hunter_config || {}),
                        v7_t2_symbol_validity_gate: checked,
                      },
                    })
                  }
                />
              </div>
            )}
          </div>
        </div>
      )}

      {/* Hunter Sniff Options - only for hunter_sniff mode */}
      {config.source_type === 'hunter_sniff' && (
        <div className="p-4 rounded-lg bg-accent-purple/5 border border-accent-purple/20">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              <Radar className="w-4 h-4 text-accent-purple" />
              <span className="text-sm font-medium text-foreground">
                {ts(coinSource.hunter_sniff, language)}{' '}
                {ts(coinSource.dataSourceConfig, language)}
              </span>
              <BinanceBadge />
            </div>
          </div>

          <div className="space-y-3">
            {/* Limit config — same as hunter */}
            <div className="flex items-center gap-3">
              <span className="text-sm text-muted-foreground">
                {ts(coinSource.hunterLimit, language)}:
              </span>
              <AiTSelect
                value={config.hunter_limit || 10}
                onChange={(val) =>
                  !disabled &&
                  onChange({ ...config, hunter_limit: parseInt(val) || 10 })
                }
                disabled={disabled}
                options={[3, 5, 8, 10, 15, 20].map((n) => ({
                  value: n,
                  label: String(n),
                }))}
                className="px-3 py-1.5 rounded bg-background border border-primary/20 text-foreground"
              />
            </div>

            {/* Sniff mode info box */}
            <div className="p-3 rounded bg-accent-purple/5 border border-accent-purple/20 text-xs text-accent-purple/80 space-y-0.5">
              {ts(coinSource.sniffFilterInfo, language)
                .split('\n')
                .map((line, i) => (
                  <div key={i} className={i === 0 ? 'font-medium mb-1' : ''}>
                    {line}
                  </div>
                ))}
            </div>

            <p className="text-xs text-muted-foreground">
              {ts(coinSource.sniffSummary, language)}
            </p>
          </div>
        </div>
      )}

      {/* Mixed Mode - Unified Card Selector */}
      {config.source_type === 'mixed' && (
        <div className="p-4 rounded-lg bg-info/5 border border-info/20">
          <div className="flex items-center gap-2 mb-4">
            <Shuffle className="w-4 h-4 text-info" />
            <span className="text-sm font-medium text-foreground">
              {ts(coinSource.mixedConfig, language)}
            </span>
          </div>

          {/* 4 Source Cards in 2x2 Grid */}
          <div className="grid grid-cols-2 gap-3 mb-4">
            {/* AI500 Card */}
            <div
              className={`p-3 rounded-lg border transition-all cursor-pointer ${
                config.use_ai500
                  ? 'bg-primary-dim border-primary/50'
                  : 'bg-background border-ait-border hover:border-primary/30'
              }`}
              onClick={() =>
                !disabled &&
                onChange({ ...config, use_ai500: !config.use_ai500 })
              }
            >
              <div className="flex items-center gap-2 mb-2">
                <input
                  type="checkbox"
                  checked={config.use_ai500}
                  onChange={(e) =>
                    !disabled &&
                    onChange({ ...config, use_ai500: e.target.checked })
                  }
                  disabled={disabled}
                  className="w-4 h-4 rounded accent-primary"
                  onClick={(e) => e.stopPropagation()}
                />
                <Database className="w-4 h-4 text-primary" />
                <span className="text-sm font-medium text-foreground">
                  AI500
                </span>
                <BinanceBadge />
              </div>
              {config.use_ai500 && (
                <div className="flex items-center gap-2 mt-2 pl-6">
                  <span className="text-xs text-muted-foreground">
                    {ts(coinSource.ai500Limit, language)}:
                  </span>
                  <AiTSelect
                    value={config.ai500_limit || 3}
                    onChange={(val) =>
                      !disabled &&
                      onChange({ ...config, ai500_limit: parseInt(val) || 3 })
                    }
                    disabled={disabled}
                    options={[1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((n) => ({
                      value: n,
                      label: String(n),
                    }))}
                    className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                  />
                </div>
              )}
            </div>

            {/* OI Top Card */}
            <div
              className={`p-3 rounded-lg border transition-all cursor-pointer ${
                config.use_oi_top
                  ? 'bg-profit/10 border-profit/50'
                  : 'bg-background border-ait-border hover:border-profit/30'
              }`}
              onClick={() =>
                !disabled &&
                onChange({ ...config, use_oi_top: !config.use_oi_top })
              }
            >
              <div className="flex items-center gap-2 mb-2">
                <input
                  type="checkbox"
                  checked={config.use_oi_top}
                  onChange={(e) =>
                    !disabled &&
                    onChange({ ...config, use_oi_top: e.target.checked })
                  }
                  disabled={disabled}
                  className="w-4 h-4 rounded accent-profit"
                  onClick={(e) => e.stopPropagation()}
                />
                <TrendingUp className="w-4 h-4 text-profit" />
                <span className="text-sm font-medium text-foreground">
                  {ts(coinSource.oiIncreaseLabel, language)}
                </span>
              </div>
              <p className="text-xs text-muted-foreground pl-6 mb-1">
                {ts(coinSource.forLong, language)}
              </p>
              {config.use_oi_top && (
                <div className="flex items-center gap-2 mt-2 pl-6">
                  <span className="text-xs text-muted-foreground">
                    {ts(coinSource.ai500Limit, language)}:
                  </span>
                  <AiTSelect
                    value={config.oi_top_limit || 3}
                    onChange={(val) =>
                      !disabled &&
                      onChange({ ...config, oi_top_limit: parseInt(val) || 3 })
                    }
                    disabled={disabled}
                    options={[1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((n) => ({
                      value: n,
                      label: String(n),
                    }))}
                    className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                  />
                </div>
              )}
            </div>

            {/* OI Low Card */}
            <div
              className={`p-3 rounded-lg border transition-all cursor-pointer ${
                config.use_oi_low
                  ? 'bg-loss/10 border-loss/50'
                  : 'bg-background border-ait-border hover:border-loss/30'
              }`}
              onClick={() =>
                !disabled &&
                onChange({ ...config, use_oi_low: !config.use_oi_low })
              }
            >
              <div className="flex items-center gap-2 mb-2">
                <input
                  type="checkbox"
                  checked={config.use_oi_low}
                  onChange={(e) =>
                    !disabled &&
                    onChange({ ...config, use_oi_low: e.target.checked })
                  }
                  disabled={disabled}
                  className="w-4 h-4 rounded accent-loss"
                  onClick={(e) => e.stopPropagation()}
                />
                <TrendingDown className="w-4 h-4 text-loss" />
                <span className="text-sm font-medium text-foreground">
                  {ts(coinSource.oiDecreaseLabel, language)}
                </span>
              </div>
              <p className="text-xs text-muted-foreground pl-6 mb-1">
                {ts(coinSource.forShort, language)}
              </p>
              {config.use_oi_low && (
                <div className="flex items-center gap-2 mt-2 pl-6">
                  <span className="text-xs text-muted-foreground">
                    {ts(coinSource.ai500Limit, language)}:
                  </span>
                  <AiTSelect
                    value={config.oi_low_limit || 3}
                    onChange={(val) =>
                      !disabled &&
                      onChange({ ...config, oi_low_limit: parseInt(val) || 3 })
                    }
                    disabled={disabled}
                    options={[1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((n) => ({
                      value: n,
                      label: String(n),
                    }))}
                    className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                  />
                </div>
              )}
            </div>

            {/* Binance Square Heat Card */}
            <div
              className={`p-3 rounded-lg border transition-all cursor-pointer ${
                config.use_square_heat
                  ? 'bg-warning/10 border-warning/50'
                  : 'bg-background border-ait-border hover:border-warning/30'
              }`}
              onClick={() =>
                !disabled &&
                onChange({
                  ...config,
                  use_square_heat: !config.use_square_heat,
                })
              }
            >
              <div className="flex items-center gap-2 mb-2">
                <input
                  type="checkbox"
                  checked={config.use_square_heat || false}
                  onChange={(e) =>
                    !disabled &&
                    onChange({ ...config, use_square_heat: e.target.checked })
                  }
                  disabled={disabled}
                  className="w-4 h-4 rounded accent-warning"
                  onClick={(e) => e.stopPropagation()}
                />
                <Flame className="w-4 h-4 text-warning" />
                <span className="text-sm font-medium text-foreground">
                  {ts(coinSource.square_heat, language)}
                </span>
              </div>
              <p className="text-xs text-muted-foreground pl-6 mb-1">
                {ts(coinSource.square_heatDesc, language)}
              </p>
              {config.use_square_heat && (
                <div className="flex items-center gap-2 mt-2 pl-6">
                  <span className="text-xs text-muted-foreground">
                    {ts(coinSource.ai500Limit, language)}:
                  </span>
                  <AiTSelect
                    value={config.square_heat_limit || 10}
                    onChange={(val) =>
                      !disabled &&
                      onChange({
                        ...config,
                        square_heat_limit: parseInt(val) || 10,
                      })
                    }
                    disabled={disabled}
                    options={[3, 5, 8, 10, 15, 20].map((n) => ({
                      value: n,
                      label: String(n),
                    }))}
                    className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                  />
                  <span className="text-xs text-muted-foreground">
                    {ts(coinSource.advMinScore, language)}:
                  </span>
                  <AiTSelect
                    value={String(config.square_min_score || 25)}
                    onChange={(val) =>
                      !disabled &&
                      onChange({
                        ...config,
                        square_min_score: parseFloat(val) || 25,
                      })
                    }
                    disabled={disabled}
                    options={[10, 15, 20, 25, 30, 40, 50].map((n) => ({
                      value: String(n),
                      label: String(n),
                    }))}
                    className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                  />
                </div>
              )}
            </div>

            {/* Hunter Card */}
            <div
              className={`p-3 rounded-lg border transition-all cursor-pointer ${
                config.use_hunter
                  ? 'bg-accent-purple/10 border-accent-purple/50'
                  : 'bg-background border-ait-border hover:border-accent-purple/30'
              }`}
              onClick={() =>
                !disabled &&
                onChange({ ...config, use_hunter: !config.use_hunter })
              }
            >
              <div className="flex items-center gap-2 mb-2">
                <input
                  type="checkbox"
                  checked={config.use_hunter || false}
                  onChange={(e) =>
                    !disabled &&
                    onChange({ ...config, use_hunter: e.target.checked })
                  }
                  disabled={disabled}
                  className="w-4 h-4 rounded accent-accent-purple"
                  onClick={(e) => e.stopPropagation()}
                />
                <Crosshair className="w-4 h-4 text-accent-purple" />
                <span className="text-sm font-medium text-foreground">
                  {ts(coinSource.hunter, language)}
                </span>
              </div>
              <p className="text-xs text-muted-foreground pl-6 mb-1">
                {ts(coinSource.hunterDesc, language)}
              </p>
              {config.use_hunter && (
                <div className="flex items-center gap-2 mt-2 pl-6">
                  <span className="text-xs text-muted-foreground">
                    {ts(coinSource.ai500Limit, language)}:
                  </span>
                  <AiTSelect
                    value={config.hunter_limit || 10}
                    onChange={(val) =>
                      !disabled &&
                      onChange({ ...config, hunter_limit: parseInt(val) || 10 })
                    }
                    disabled={disabled}
                    options={[3, 5, 8, 10, 15, 20].map((n) => ({
                      value: n,
                      label: String(n),
                    }))}
                    className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                  />
                </div>
              )}
              {config.use_hunter && (
                <div className="flex items-center gap-2 mt-2 pl-6">
                  <span className="text-xs text-muted-foreground">
                    {ts(coinSource.hunterDirection, language)}:
                  </span>
                  <Button
                    variant="unstyled"
                    onClick={(e) => {
                      e.stopPropagation()
                      if (disabled) return
                      const d = config.hunter_direction
                      const hasLong = d === 'LONG' || d === 'BOTH'
                      const hasShort = d === 'SHORT' || d === 'BOTH'
                      const nextLong = !hasLong
                      if (nextLong && hasShort)
                        onChange({ ...config, hunter_direction: 'BOTH' })
                      else if (nextLong)
                        onChange({ ...config, hunter_direction: 'LONG' })
                      else if (hasShort)
                        onChange({ ...config, hunter_direction: 'SHORT' })
                      else onChange({ ...config, hunter_direction: undefined })
                    }}
                    disabled={disabled}
                    className={`px-2 py-0.5 rounded text-[10px] font-medium transition-all ${
                      config.hunter_direction === 'LONG' ||
                      config.hunter_direction === 'BOTH'
                        ? 'bg-profit/20 text-profit border border-profit/50'
                        : 'bg-background text-muted-foreground border border-ait-border hover:border-profit/30'
                    }`}
                  >
                    ▲ {ts(coinSource.hunterDirectionLong, language)}
                  </Button>
                  <Button
                    variant="unstyled"
                    onClick={(e) => {
                      e.stopPropagation()
                      if (disabled) return
                      const d = config.hunter_direction
                      const hasLong = d === 'LONG' || d === 'BOTH'
                      const hasShort = d === 'SHORT' || d === 'BOTH'
                      const nextShort = !hasShort
                      if (nextShort && hasLong)
                        onChange({ ...config, hunter_direction: 'BOTH' })
                      else if (nextShort)
                        onChange({ ...config, hunter_direction: 'SHORT' })
                      else if (hasLong)
                        onChange({ ...config, hunter_direction: 'LONG' })
                      else onChange({ ...config, hunter_direction: undefined })
                    }}
                    disabled={disabled}
                    className={`px-2 py-0.5 rounded text-[10px] font-medium transition-all ${
                      config.hunter_direction === 'SHORT' ||
                      config.hunter_direction === 'BOTH'
                        ? 'bg-loss/20 text-loss border border-loss/50'
                        : 'bg-background text-muted-foreground border border-ait-border hover:border-loss/30'
                    }`}
                  >
                    ▼ {ts(coinSource.hunterDirectionShort, language)}
                  </Button>
                </div>
              )}
              {config.use_hunter && (
                <details className="pl-8 mt-2">
                  <summary className="text-xs cursor-pointer text-muted-foreground hover:text-foreground select-none">
                    {ts(coinSource.advAdvancedFilters, language)} ▸
                  </summary>
                  <div className="mt-3 space-y-2.5 p-3 rounded-lg bg-background border border-ait-border">
                    {/* Min OI Value */}
                    <div className="flex items-center justify-between">
                      <span className="text-xs text-muted-foreground">
                        {ts(coinSource.advMinOI, language)}
                      </span>
                      <AiTSelect
                        value={String(
                          config.hunter_config?.min_oi_value || 5000000
                        )}
                        onChange={(val) =>
                          !disabled &&
                          onChange({
                            ...config,
                            hunter_config: {
                              ...(config.hunter_config || {}),
                              min_oi_value: Number(val),
                            },
                          })
                        }
                        disabled={disabled}
                        options={[
                          { value: '2000000', label: '$2M' },
                          { value: '5000000', label: '$5M' },
                          { value: '10000000', label: '$10M' },
                          { value: '15000000', label: '$15M' },
                        ]}
                        className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                      />
                    </div>

                    {/* RSI Oversold */}
                    <div className="flex items-center justify-between">
                      <span className="text-xs text-muted-foreground">
                        {ts(coinSource.advRSIOversold, language)}
                      </span>
                      <AiTSelect
                        value={String(
                          config.hunter_config?.rsi_oversold_threshold || 30
                        )}
                        onChange={(val) =>
                          !disabled &&
                          onChange({
                            ...config,
                            hunter_config: {
                              ...(config.hunter_config || {}),
                              rsi_oversold_threshold: Number(val),
                            },
                          })
                        }
                        disabled={disabled}
                        options={[20, 25, 30, 35, 40].map((n) => ({
                          value: String(n),
                          label: String(n),
                        }))}
                        className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                      />
                    </div>

                    {/* RSI Overbought */}
                    <div className="flex items-center justify-between">
                      <span className="text-xs text-muted-foreground">
                        {ts(coinSource.advRSIOverbought, language)}
                      </span>
                      <AiTSelect
                        value={String(
                          config.hunter_config?.rsi_overbought_threshold || 70
                        )}
                        onChange={(val) =>
                          !disabled &&
                          onChange({
                            ...config,
                            hunter_config: {
                              ...(config.hunter_config || {}),
                              rsi_overbought_threshold: Number(val),
                            },
                          })
                        }
                        disabled={disabled}
                        options={[60, 65, 70, 75, 80].map((n) => ({
                          value: String(n),
                          label: String(n),
                        }))}
                        className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                      />
                    </div>

                    {/* Funding Rate Signal toggle */}
                    <label className="flex items-center gap-2 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={
                          config.hunter_config?.enable_funding_rate_signal !==
                          false
                        }
                        onChange={(e) =>
                          !disabled &&
                          onChange({
                            ...config,
                            hunter_config: {
                              ...(config.hunter_config || {}),
                              enable_funding_rate_signal: e.target.checked,
                            },
                          })
                        }
                        disabled={disabled}
                        className="w-4 h-4 rounded accent-accent-purple"
                      />
                      <span className="text-xs text-foreground">
                        {ts(coinSource.advFundingRateSignal, language)}
                      </span>
                    </label>

                    {/* Max 24h Change */}
                    <div className="flex items-center justify-between">
                      <span className="text-xs text-muted-foreground">
                        {ts(coinSource.advMax24hChange, language)}
                      </span>
                      <AiTSelect
                        value={String(
                          config.hunter_config?.max_24h_change || 50
                        )}
                        onChange={(val) =>
                          !disabled &&
                          onChange({
                            ...config,
                            hunter_config: {
                              ...(config.hunter_config || {}),
                              max_24h_change: Number(val),
                            },
                          })
                        }
                        disabled={disabled}
                        options={[
                          {
                            value: '0',
                            label: ts(coinSource.advNone, language),
                          },
                          { value: '20', label: '20%' },
                          { value: '30', label: '30%' },
                          { value: '50', label: '50%' },
                          { value: '100', label: '100%' },
                        ]}
                        className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                      />
                    </div>

                    {/* Wash Trade Sensitivity */}
                    <div className="flex items-center justify-between">
                      <span className="text-xs text-muted-foreground">
                        {ts(coinSource.advWashDetection, language)}
                      </span>
                      <AiTSelect
                        value={
                          config.hunter_config?.wash_trade_sensitivity ||
                          'medium'
                        }
                        onChange={(val) =>
                          !disabled &&
                          onChange({
                            ...config,
                            hunter_config: {
                              ...(config.hunter_config || {}),
                              wash_trade_sensitivity: val as
                                | 'low'
                                | 'medium'
                                | 'high',
                            },
                          })
                        }
                        disabled={disabled}
                        options={[
                          {
                            value: 'low',
                            label: ts(coinSource.advLow, language),
                          },
                          {
                            value: 'medium',
                            label: ts(coinSource.advMedium, language),
                          },
                          {
                            value: 'high',
                            label: ts(coinSource.advHigh, language),
                          },
                        ]}
                        className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                      />
                    </div>

                    {/* Cooldown toggle */}
                    <label className="flex items-center gap-2 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={
                          config.hunter_config?.enable_cooldown !== false
                        }
                        onChange={(e) =>
                          !disabled &&
                          onChange({
                            ...config,
                            hunter_config: {
                              ...(config.hunter_config || {}),
                              enable_cooldown: e.target.checked,
                            },
                          })
                        }
                        disabled={disabled}
                        className="w-4 h-4 rounded accent-accent-purple"
                      />
                      <span className="text-xs text-foreground">
                        {ts(coinSource.advSmartCooldown, language)}
                      </span>
                    </label>

                    {/* Min Trade Count */}
                    <div className="flex items-center justify-between">
                      <span className="text-xs text-muted-foreground">
                        {ts(coinSource.advMinTrades, language)}
                      </span>
                      <AiTSelect
                        value={String(
                          config.hunter_config?.min_trade_count || 10000
                        )}
                        onChange={(val) =>
                          !disabled &&
                          onChange({
                            ...config,
                            hunter_config: {
                              ...(config.hunter_config || {}),
                              min_trade_count: Number(val),
                            },
                          })
                        }
                        disabled={disabled}
                        options={[
                          { value: '1000', label: '1K' },
                          { value: '5000', label: '5K' },
                          { value: '10000', label: '10K' },
                          { value: '50000', label: '50K' },
                        ]}
                        className="px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                      />
                    </div>
                  </div>
                </details>
              )}
            </div>

            {/* Static/Custom Card */}
            <div
              className={`p-3 rounded-lg border transition-all cursor-pointer ${
                (config.static_coins || []).length > 0
                  ? 'bg-muted-foreground/10 border-muted-foreground/50'
                  : 'bg-background border-ait-border hover:border-muted-foreground/30'
              }`}
            >
              <div className="flex items-center gap-2 mb-2">
                <List className="w-4 h-4 text-muted-foreground" />
                <span className="text-sm font-medium text-foreground">
                  {ts(coinSource.custom, language)}
                </span>
                {(config.static_coins || []).length > 0 && (
                  <span className="text-xs px-1.5 py-0.5 rounded bg-muted-foreground/20 text-muted-foreground">
                    {config.static_coins?.length}
                  </span>
                )}
              </div>
              <div className="flex flex-wrap gap-1 mt-2">
                {(config.static_coins || []).slice(0, 3).map((coin) => (
                  <span
                    key={coin}
                    className="flex items-center gap-1 px-2 py-0.5 rounded text-xs bg-surface text-foreground"
                  >
                    {coin}
                    {!disabled && (
                      <Button
                        variant="unstyled"
                        onClick={(e) => {
                          e.stopPropagation()
                          handleRemoveCoin(coin)
                        }}
                        className="hover:text-loss transition-colors"
                      >
                        <X className="w-2.5 h-2.5" />
                      </Button>
                    )}
                  </span>
                ))}
                {(config.static_coins || []).length > 3 && (
                  <span className="text-xs text-muted-foreground">
                    +{(config.static_coins?.length || 0) - 3}
                  </span>
                )}
              </div>
              {!disabled && (
                <div className="flex gap-1 mt-2">
                  <input
                    type="text"
                    value={newCoin}
                    onChange={(e) => setNewCoin(e.target.value)}
                    onKeyDown={(e) => {
                      e.stopPropagation()
                      if (e.key === 'Enter') handleAddCoin()
                    }}
                    onClick={(e) => e.stopPropagation()}
                    placeholder="BTC, ETH..."
                    className="flex-1 px-2 py-1 rounded text-xs bg-background border border-primary/20 text-foreground"
                  />
                  <Button
                    variant="unstyled"
                    onClick={(e) => {
                      e.stopPropagation()
                      handleAddCoin()
                    }}
                    className="px-2 py-1 rounded text-xs bg-primary text-primary-foreground hover:bg-primary"
                  >
                    <Plus className="w-3 h-3" />
                  </Button>
                </div>
              )}
            </div>
          </div>

          {/* Summary */}
          {(() => {
            const { sources, totalLimit } = getMixedSummary()
            if (sources.length === 0) return null
            return (
              <div className="p-2 rounded bg-background border border-ait-border">
                <div className="flex items-center justify-between text-xs">
                  <span className="text-muted-foreground">
                    {ts(coinSource.mixedSummary, language)}:
                  </span>
                  <span className="text-foreground font-medium">
                    {sources.join(' + ')}
                  </span>
                </div>
                <div className="text-xs text-muted-foreground mt-1">
                  {ts(coinSource.maxCoins, language)} {totalLimit}{' '}
                  {ts(coinSource.coins, language)}
                </div>
              </div>
            )
          })()}
        </div>
      )}
    </div>
  )
}
