import { useMemo, type FormEvent } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import {
  ChevronRight,
  ChevronLeft,
  RefreshCw,
  Zap,
} from 'lucide-react'
import type { AIModel, Strategy } from '../../types'
import { t as globalT } from '../../i18n/translations'
import type { Language } from '../../i18n/translations'

// ============ Types ============

type WizardStep = 1 | 2 | 3

export interface BacktestFormState {
  runId: string
  symbols: string
  timeframes: string[]
  decisionTf: string
  cadence: number
  start: string
  end: string
  balance: number
  fee: number
  slippage: number
  btcEthLeverage: number
  altcoinLeverage: number
  fill: string
  prompt: string
  promptTemplate: string
  customPrompt: string
  overridePrompt: boolean
  cacheAI: boolean
  replayOnly: boolean
  aiModelId: string
  strategyId: string
}

const TIMEFRAME_OPTIONS = ['1m', '3m', '5m', '15m', '30m', '1h', '4h', '1d']
const POPULAR_SYMBOLS = ['BTCUSDT', 'ETHUSDT', 'SOLUSDT', 'BNBUSDT', 'XRPUSDT', 'DOGEUSDT']

// ============ Config Form ============

interface BacktestConfigFormProps {
  formState: BacktestFormState
  wizardStep: WizardStep
  isStarting: boolean
  aiModels: AIModel[] | undefined
  strategies: Strategy[] | undefined
  language: string
  tr: (key: string, params?: Record<string, string | number>) => string
  onFormChange: (key: string, value: string | number | boolean | string[]) => void
  onWizardStepChange: (step: WizardStep) => void
  onStart: (event: FormEvent) => void
}

export function BacktestConfigForm({
  formState,
  wizardStep,
  isStarting,
  aiModels,
  strategies,
  language,
  tr,
  onFormChange,
  onWizardStepChange,
  onStart,
}: BacktestConfigFormProps) {
  const selectedModel = aiModels?.find((m) => m.id === formState.aiModelId)
  const selectedStrategy = strategies?.find((s) => s.id === formState.strategyId)

  const strategyHasDynamicCoins = useMemo(() => {
    const cs = selectedStrategy?.config?.coin_source
    if (!cs) return false
    const st = cs.source_type as string
    if (st === 'ai500' || st === 'oi_top') return true
    if (st === 'mixed' && (cs.use_ai500 || cs.use_oi_top)) return true
    if (!st && (cs.use_ai500 || cs.use_oi_top)) return true
    return false
  }, [selectedStrategy])

  const coinSourceDescription = useMemo(() => {
    const cs = selectedStrategy?.config?.coin_source
    if (!cs) return null
    let st = cs.source_type as string
    if (!st) {
      if (cs.use_ai500 && cs.use_oi_top) st = 'mixed'
      else if (cs.use_ai500) st = 'ai500'
      else if (cs.use_oi_top) st = 'oi_top'
      else if (cs.static_coins?.length) st = 'static'
    }
    switch (st) {
      case 'ai500': return { type: 'AI500', limit: cs.ai500_limit || 30 }
      case 'oi_top': return { type: 'OI Top', limit: cs.oi_top_limit || 30 }
      case 'mixed': {
        const parts: string[] = []
        if (cs.use_ai500) parts.push(`AI500(${cs.ai500_limit || 30})`)
        if (cs.use_oi_top) parts.push(`OI Top(${cs.oi_top_limit || 30})`)
        if (cs.static_coins?.length) parts.push(`Static(${cs.static_coins.length})`)
        return { type: 'Mixed', desc: parts.join(' + ') }
      }
      case 'static': return { type: 'Static', coins: cs.static_coins || [] }
      default: return null
    }
  }, [selectedStrategy])

  const lang = language as Language
  const quickRanges = [
    { label: globalT('backtestConfigForm.quickRange24h', lang), hours: 24 },
    { label: globalT('backtestConfigForm.quickRange3d', lang), hours: 72 },
    { label: globalT('backtestConfigForm.quickRange7d', lang), hours: 168 },
    { label: globalT('backtestConfigForm.quickRange30d', lang), hours: 720 },
  ]

  const applyQuickRange = (hours: number) => {
    const end = new Date()
    const start = new Date(end.getTime() - hours * 3600 * 1000)
    const fmt = (d: Date) => new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
    onFormChange('start', fmt(start))
    onFormChange('end', fmt(end))
  }

  return (
    <div className="binance-card p-5">
      <div className="flex items-center gap-2 mb-4">
        {[1, 2, 3].map((step) => (
          <div key={step} className="flex items-center">
            <button
              onClick={() => onWizardStepChange(step as WizardStep)}
              className="w-8 h-8 rounded-full flex items-center justify-center text-sm font-bold transition-all"
              style={{
                background: wizardStep >= step ? 'var(--color-primary)' : 'var(--color-border)',
                color: wizardStep >= step ? 'var(--background)' : 'var(--color-muted-fg)',
              }}
            >
              {step}
            </button>
            {step < 3 && (
              <div
                className="w-8 h-0.5 mx-1"
                style={{ background: wizardStep > step ? 'var(--color-primary)' : 'var(--color-border)' }}
              />
            )}
          </div>
        ))}
        <span className="ml-2 text-xs text-muted-foreground">
          {wizardStep === 1 ? globalT('backtestConfigForm.selectModel', lang)
            : wizardStep === 2 ? globalT('backtestConfigForm.configure', lang)
            : globalT('backtestConfigForm.confirmStart', lang)}
        </span>
      </div>

      <form onSubmit={onStart}>
        <AnimatePresence mode="wait">
          {/* Step 1: Model & Symbols */}
          {wizardStep === 1 && (
            <motion.div
              key="step1"
              initial={{ opacity: 0, x: 20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -20 }}
              className="space-y-4"
            >
              <div>
                <label className="block text-xs mb-2 text-muted-foreground">
                  {tr('form.aiModelLabel')}
                </label>
                <select
                  className="w-full p-3 rounded-lg text-sm"
                  style={{ background: 'var(--background)', border: '1px solid var(--color-border)', color: 'var(--foreground)' }}
                  value={formState.aiModelId}
                  onChange={(e) => onFormChange('aiModelId', e.target.value)}
                >
                  <option value="">{tr('form.selectAiModel')}</option>
                  {aiModels?.map((m) => (
                    <option key={m.id} value={m.id}>
                      {m.name} ({m.provider}) {!m.enabled && '⚠️'}
                    </option>
                  ))}
                </select>
                {selectedModel && (
                  <div className="mt-2 flex items-center gap-2 text-xs">
                    <span
                      className="px-2 py-0.5 rounded"
                      style={{
                        background: selectedModel.enabled ? 'color-mix(in srgb, var(--color-profit) 10%, transparent)' : 'color-mix(in srgb, var(--color-loss) 10%, transparent)',
                        color: selectedModel.enabled ? 'var(--color-profit)' : 'var(--color-loss)',
                      }}
                    >
                      {selectedModel.enabled ? tr('form.enabled') : tr('form.disabled')}
                    </span>
                  </div>
                )}
              </div>

              {/* Strategy Selection (Optional) */}
              <div>
                <label className="block text-xs mb-2 text-muted-foreground">
                  {globalT('backtestConfigForm.strategyOptional', lang)}
                </label>
                <select
                  className="w-full p-3 rounded-lg text-sm"
                  style={{ background: 'var(--background)', border: '1px solid var(--color-border)', color: 'var(--foreground)' }}
                  value={formState.strategyId}
                  onChange={(e) => onFormChange('strategyId', e.target.value)}
                >
                  <option value="">{globalT('backtestConfigForm.noSavedStrategy', lang)}</option>
                  {strategies?.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name} {s.is_active && '✓'} {s.is_default && '⭐'}
                    </option>
                  ))}
                </select>
                {formState.strategyId && coinSourceDescription && (
                  <div className="mt-2 p-2 rounded" style={{ background: 'color-mix(in srgb, var(--color-primary) 10%, transparent)', border: '1px solid color-mix(in srgb, var(--color-primary) 20%, transparent)' }}>
                    <div className="flex items-center gap-2 text-xs">
                      <span style={{ color: 'var(--color-primary)' }}>
                        {globalT('backtestConfigForm.coinSource', lang)}
                      </span>
                      <span className="font-medium text-foreground">
                        {coinSourceDescription.type}
                        {coinSourceDescription.limit && ` (${coinSourceDescription.limit})`}
                        {coinSourceDescription.desc && ` - ${coinSourceDescription.desc}`}
                      </span>
                    </div>
                    {strategyHasDynamicCoins && (
                      <div className="text-xs mt-1" style={{ color: 'var(--color-primary)' }}>
                        {globalT('backtestConfigForm.clearDynamicCoins', lang)}
                      </div>
                    )}
                  </div>
                )}
              </div>

              <div>
                <label className="block text-xs mb-2 text-muted-foreground">
                  {tr('form.symbolsLabel')}
                  {strategyHasDynamicCoins && (
                    <span className="ml-2" style={{ color: 'var(--color-muted-fg)' }}>
                      ({globalT('backtestConfigForm.optionalCoinSource', lang)})
                    </span>
                  )}
                </label>
                {!strategyHasDynamicCoins && (
                  <div className="flex flex-wrap gap-1 mb-2">
                    {POPULAR_SYMBOLS.map((sym) => {
                      const isSelected = formState.symbols.includes(sym)
                      return (
                        <button
                          key={sym}
                          type="button"
                          onClick={() => {
                            const current = formState.symbols.split(',').map((s) => s.trim()).filter(Boolean)
                            const updated = isSelected
                              ? current.filter((s) => s !== sym)
                              : [...current, sym]
                            onFormChange('symbols', updated.join(','))
                          }}
                          className="px-2 py-1 rounded text-xs transition-all"
                          style={{
                            background: isSelected ? 'color-mix(in srgb, var(--color-primary) 15%, transparent)' : 'var(--color-panel)',
                            border: `1px solid ${isSelected ? 'var(--color-primary)' : 'var(--color-border)'}`,
                            color: isSelected ? 'var(--color-primary)' : 'var(--color-muted-fg)',
                          }}
                        >
                          {sym.replace('USDT', '')}
                        </button>
                      )
                    })}
                  </div>
                )}
                <div className="relative">
                  <textarea
                    className="w-full p-2 rounded-lg text-xs font-mono"
                    style={{
                      background: 'var(--background)',
                      border: '1px solid var(--color-border)',
                      color: 'var(--foreground)',
                    }}
                    value={formState.symbols}
                    onChange={(e) => onFormChange('symbols', e.target.value)}
                    rows={2}
                    placeholder={strategyHasDynamicCoins
                      ? globalT('backtestConfigForm.leavEmptyForStrategy', lang)
                      : ''
                    }
                  />
                  {strategyHasDynamicCoins && formState.symbols && (
                    <button
                      type="button"
                      onClick={() => onFormChange('symbols', '')}
                      className="absolute top-2 right-2 px-2 py-1 rounded text-xs"
                      style={{ background: 'var(--color-primary)', color: 'var(--background)' }}
                    >
                      {globalT('backtestConfigForm.clearToUseStrategy', lang)}
                    </button>
                  )}
                </div>
              </div>

              <button
                type="button"
                onClick={() => onWizardStepChange(2)}
                disabled={!selectedModel?.enabled}
                className="w-full py-2.5 rounded-lg font-medium flex items-center justify-center gap-2 transition-all disabled:opacity-50"
                style={{ background: 'var(--color-primary)', color: 'var(--background)' }}
              >
                {globalT('backtestConfigForm.next', lang)}
                <ChevronRight className="w-4 h-4" />
              </button>
            </motion.div>
          )}

          {/* Step 2: Parameters */}
          {wizardStep === 2 && (
            <motion.div
              key="step2"
              initial={{ opacity: 0, x: 20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -20 }}
              className="space-y-4"
            >
              <div>
                <label className="block text-xs mb-2 text-muted-foreground">
                  {tr('form.timeRangeLabel')}
                </label>
                <div className="flex flex-wrap gap-1 mb-2">
                  {quickRanges.map((r) => (
                    <button
                      key={r.hours}
                      type="button"
                      onClick={() => applyQuickRange(r.hours)}
                      className="px-3 py-1 rounded text-xs"
                      style={{ background: 'var(--color-panel)', border: '1px solid var(--color-border)', color: 'var(--foreground)' }}
                    >
                      {r.label}
                    </button>
                  ))}
                </div>
                <div className="grid grid-cols-2 gap-2">
                  <input
                    type="datetime-local"
                    className="p-2 rounded-lg text-xs"
                    style={{ background: 'var(--background)', border: '1px solid var(--color-border)', color: 'var(--foreground)' }}
                    value={formState.start}
                    onChange={(e) => onFormChange('start', e.target.value)}
                  />
                  <input
                    type="datetime-local"
                    className="p-2 rounded-lg text-xs"
                    style={{ background: 'var(--background)', border: '1px solid var(--color-border)', color: 'var(--foreground)' }}
                    value={formState.end}
                    onChange={(e) => onFormChange('end', e.target.value)}
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs mb-2 text-muted-foreground">
                  {globalT('backtestConfigForm.timeframes', lang)}
                </label>
                <div className="flex flex-wrap gap-1">
                  {TIMEFRAME_OPTIONS.map((tf) => {
                    const isSelected = formState.timeframes.includes(tf)
                    return (
                      <button
                        key={tf}
                        type="button"
                        onClick={() => {
                          const updated = isSelected
                            ? formState.timeframes.filter((t) => t !== tf)
                            : [...formState.timeframes, tf]
                          if (updated.length > 0) onFormChange('timeframes', updated)
                        }}
                        className="px-2 py-1 rounded text-xs transition-all"
                        style={{
                          background: isSelected ? 'color-mix(in srgb, var(--color-primary) 15%, transparent)' : 'var(--color-panel)',
                          border: `1px solid ${isSelected ? 'var(--color-primary)' : 'var(--color-border)'}`,
                          color: isSelected ? 'var(--color-primary)' : 'var(--color-muted-fg)',
                        }}
                      >
                        {tf}
                      </button>
                    )
                  })}
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs mb-1 text-muted-foreground">
                    {tr('form.initialBalanceLabel')}
                  </label>
                  <input
                    type="number"
                    name="backtest-initial-balance"
                    autoComplete="off"
                    inputMode="decimal"
                    className="w-full p-2 rounded-lg text-xs"
                    style={{ background: 'var(--color-input)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
                    value={formState.balance}
                    onChange={(e) => onFormChange('balance', Number(e.target.value))}
                  />
                </div>
                <div>
                  <label className="block text-xs mb-1 text-muted-foreground">
                    {tr('form.decisionTfLabel')}
                  </label>
                  <select
                    className="w-full p-2 rounded-lg text-xs"
                    style={{ background: 'var(--background)', border: '1px solid var(--color-border)', color: 'var(--foreground)' }}
                    value={formState.decisionTf}
                    onChange={(e) => onFormChange('decisionTf', e.target.value)}
                  >
                    {formState.timeframes.map((tf) => (
                      <option key={tf} value={tf}>
                        {tf}
                      </option>
                    ))}
                  </select>
                </div>
              </div>

              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => onWizardStepChange(1)}
                  className="flex-1 py-2 rounded-lg font-medium flex items-center justify-center gap-2"
                  style={{ background: 'var(--color-panel)', border: '1px solid var(--color-border)', color: 'var(--foreground)' }}
                >
                  <ChevronLeft className="w-4 h-4" />
                  {globalT('backtestConfigForm.back', lang)}
                </button>
                <button
                  type="button"
                  onClick={() => onWizardStepChange(3)}
                  className="flex-1 py-2 rounded-lg font-medium flex items-center justify-center gap-2"
                  style={{ background: 'var(--color-primary)', color: 'var(--background)' }}
                >
                  {globalT('backtestConfigForm.next', lang)}
                  <ChevronRight className="w-4 h-4" />
                </button>
              </div>
            </motion.div>
          )}

          {/* Step 3: Advanced & Confirm */}
          {wizardStep === 3 && (
            <motion.div
              key="step3"
              initial={{ opacity: 0, x: 20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -20 }}
              className="space-y-4"
            >
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs mb-1 text-muted-foreground">
                    {tr('form.btcEthLeverageLabel')}
                  </label>
                  <input
                    type="number"
                    name="backtest-btc-eth-leverage"
                    autoComplete="off"
                    inputMode="numeric"
                    className="w-full p-2 rounded-lg text-xs"
                    style={{ background: 'var(--color-input)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
                    value={formState.btcEthLeverage}
                    onChange={(e) => onFormChange('btcEthLeverage', Number(e.target.value))}
                  />
                </div>
                <div>
                  <label className="block text-xs mb-1 text-muted-foreground">
                    {tr('form.altcoinLeverageLabel')}
                  </label>
                  <input
                    type="number"
                    name="backtest-altcoin-leverage"
                    autoComplete="off"
                    inputMode="numeric"
                    className="w-full p-2 rounded-lg text-xs"
                    style={{ background: 'var(--color-input)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
                    value={formState.altcoinLeverage}
                    onChange={(e) => onFormChange('altcoinLeverage', Number(e.target.value))}
                  />
                </div>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
                <div>
                  <label className="block text-xs mb-1 text-muted-foreground">
                    {tr('form.feeLabel')}
                  </label>
                  <input
                    type="number"
                    name="backtest-fee"
                    autoComplete="off"
                    inputMode="decimal"
                    className="w-full p-2 rounded-lg text-xs"
                    style={{ background: 'var(--color-input)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
                    value={formState.fee}
                    onChange={(e) => onFormChange('fee', Number(e.target.value))}
                  />
                </div>
                <div>
                  <label className="block text-xs mb-1 text-muted-foreground">
                    {tr('form.slippageLabel')}
                  </label>
                  <input
                    type="number"
                    name="backtest-slippage"
                    autoComplete="off"
                    inputMode="decimal"
                    className="w-full p-2 rounded-lg text-xs"
                    style={{ background: 'var(--color-input)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
                    value={formState.slippage}
                    onChange={(e) => onFormChange('slippage', Number(e.target.value))}
                  />
                </div>
                <div>
                  <label className="block text-xs mb-1 text-muted-foreground">
                    {tr('form.cadenceLabel')}
                  </label>
                  <input
                    type="number"
                    name="backtest-cadence"
                    autoComplete="off"
                    inputMode="numeric"
                    className="w-full p-2 rounded-lg text-xs"
                    style={{ background: 'var(--color-input)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
                    value={formState.cadence}
                    onChange={(e) => onFormChange('cadence', Number(e.target.value))}
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs mb-1 text-muted-foreground">
                  {globalT('backtestConfigForm.strategyStyle', lang)}
                </label>
                <div className="flex flex-wrap gap-1">
                  {['baseline', 'aggressive', 'conservative', 'scalping'].map((p) => (
                    <button
                      key={p}
                      type="button"
                      onClick={() => onFormChange('prompt', p)}
                      className="px-3 py-1.5 rounded text-xs transition-all"
                      style={{
                        background: formState.prompt === p ? 'color-mix(in srgb, var(--color-primary) 15%, transparent)' : 'var(--color-panel)',
                        border: `1px solid ${formState.prompt === p ? 'var(--color-primary)' : 'var(--color-border)'}`,
                        color: formState.prompt === p ? 'var(--color-primary)' : 'var(--color-muted-fg)',
                      }}
                    >
                      {tr(`form.promptPresets.${p}`)}
                    </button>
                  ))}
                </div>
              </div>

              <div className="flex flex-wrap gap-4 text-xs text-muted-foreground">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={formState.cacheAI}
                    onChange={(e) => onFormChange('cacheAI', e.target.checked)}
                    className="accent-[var(--color-primary)]"
                  />
                  {tr('form.cacheAiLabel')}
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={formState.replayOnly}
                    onChange={(e) => onFormChange('replayOnly', e.target.checked)}
                    className="accent-[var(--color-primary)]"
                  />
                  {tr('form.replayOnlyLabel')}
                </label>
              </div>

              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => onWizardStepChange(2)}
                  className="flex-1 py-2 rounded-lg font-medium flex items-center justify-center gap-2"
                  style={{ background: 'var(--color-panel)', border: '1px solid var(--color-border)', color: 'var(--foreground)' }}
                >
                  <ChevronLeft className="w-4 h-4" />
                  {globalT('backtestConfigForm.back', lang)}
                </button>
                <button
                  type="submit"
                  disabled={isStarting}
                  className="flex-1 py-2 rounded-lg font-bold flex items-center justify-center gap-2 disabled:opacity-50"
                  style={{ background: 'var(--color-primary)', color: 'var(--background)' }}
                >
                  {isStarting ? (
                    <RefreshCw className="w-4 h-4 animate-spin" />
                  ) : (
                    <Zap className="w-4 h-4" />
                  )}
                  {isStarting ? tr('starting') : tr('start')}
                </button>
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </form>
    </div>
  )
}

export type { WizardStep }
