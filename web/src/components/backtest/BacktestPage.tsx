import {
  useEffect,
  useMemo,
  useState,
  useCallback,
  type FormEvent,
} from 'react'
import useSWR from 'swr'
import { motion, AnimatePresence } from 'framer-motion'
import {
  Play,
  Pause,
  Square,
  Download,
  Trash2,
  TrendingUp,
  BarChart3,
  Brain,
  Target,
  AlertTriangle,
} from 'lucide-react'
import { DeepVoidBackground } from '../common/DeepVoidBackground'
import { api } from '../../lib/api'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { confirmToast } from '../../lib/notify'
import type {
  BacktestStatusPayload,
  BacktestEquityPoint,
  BacktestTradeEvent,
  BacktestMetrics,
  DecisionRecord,
  AIModel,
  Strategy,
} from '../../types'
import {
  BacktestConfigForm,
  type WizardStep,
  type BacktestFormState,
} from './BacktestConfigForm'
import { BacktestRunList, getStateColor, getStateIcon } from './BacktestRunList'
import { StatCard, ProgressRing, PositionsDisplay } from './BacktestOverviewTab'
import { BacktestOverviewTab } from './BacktestOverviewTab'
import { BacktestChartTab } from './BacktestChartTab'
import { BacktestTradesTab } from './BacktestTradesTab'
import { BacktestDecisionsTab } from './BacktestDecisionsTab'

// ============ Types ============
type ViewTab = 'overview' | 'chart' | 'trades' | 'decisions' | 'compare'

const toLocalInput = (date: Date) => {
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000)
  return local.toISOString().slice(0, 16)
}

// ============ Main Component ============
export function BacktestPage() {
  const { language } = useLanguage()
  const tr = useCallback(
    (key: string, params?: Record<string, string | number>) =>
      t(`backtestPage.${key}`, language, params),
    [language]
  )

  // State
  const now = new Date()
  const [wizardStep, setWizardStep] = useState<WizardStep>(1)
  const [viewTab, setViewTab] = useState<ViewTab>('overview')
  const [selectedRunId, setSelectedRunId] = useState<string>()
  const [compareRunIds, setCompareRunIds] = useState<string[]>([])
  const [isStarting, setIsStarting] = useState(false)
  const [toast, setToast] = useState<{
    text: string
    tone: 'info' | 'error' | 'success'
  } | null>(null)

  // Form state
  const [formState, setFormState] = useState<BacktestFormState>({
    runId: '',
    symbols: 'BTCUSDT,ETHUSDT,SOLUSDT',
    timeframes: ['3m', '15m', '4h'],
    decisionTf: '3m',
    cadence: 20,
    start: toLocalInput(new Date(now.getTime() - 3 * 24 * 3600 * 1000)),
    end: toLocalInput(now),
    balance: 1000,
    fee: 5,
    slippage: 2,
    btcEthLeverage: 5,
    altcoinLeverage: 5,
    fill: 'next_open',
    prompt: 'baseline',
    promptTemplate: 'default',
    customPrompt: '',
    overridePrompt: false,
    cacheAI: true,
    replayOnly: false,
    aiModelId: '',
    strategyId: '',
  })

  // Data fetching
  const { data: runsResp, mutate: refreshRuns } = useSWR(
    ['backtest-runs'],
    () => api.getBacktestRuns({ limit: 100, offset: 0 }),
    { refreshInterval: 5000 }
  )
  const runs = runsResp?.items ?? []

  const { data: aiModels } = useSWR<AIModel[]>(
    'ai-models',
    api.getModelConfigs,
    { refreshInterval: 30000 }
  )
  const { data: strategies } = useSWR<Strategy[]>(
    'strategies',
    api.getStrategies,
    { refreshInterval: 30000 }
  )

  const { data: status } = useSWR<BacktestStatusPayload>(
    selectedRunId ? ['bt-status', selectedRunId] : null,
    () => api.getBacktestStatus(selectedRunId!),
    { refreshInterval: 2000 }
  )

  const { data: equity } = useSWR<BacktestEquityPoint[]>(
    selectedRunId ? ['bt-equity', selectedRunId] : null,
    () => api.getBacktestEquity(selectedRunId!, '1m', 2000),
    { refreshInterval: 5000 }
  )

  const { data: trades } = useSWR<BacktestTradeEvent[]>(
    selectedRunId ? ['bt-trades', selectedRunId] : null,
    () => api.getBacktestTrades(selectedRunId!, 500),
    { refreshInterval: 5000 }
  )

  const { data: metrics } = useSWR<BacktestMetrics>(
    selectedRunId ? ['bt-metrics', selectedRunId] : null,
    () => api.getBacktestMetrics(selectedRunId!),
    { refreshInterval: 10000 }
  )

  const { data: decisions } = useSWR<DecisionRecord[]>(
    selectedRunId ? ['bt-decisions', selectedRunId] : null,
    () => api.getBacktestDecisions(selectedRunId!, 30),
    { refreshInterval: 5000 }
  )

  const selectedRun = runs.find((r) => r.run_id === selectedRunId)
  const selectedModel = aiModels?.find((m) => m.id === formState.aiModelId)
  const selectedStrategy = strategies?.find(
    (s) => s.id === formState.strategyId
  )

  // Check if selected strategy has dynamic coin source (needed for handleStart)
  const strategyHasDynamicCoins = useMemo(() => {
    if (!selectedStrategy) return false
    const coinSource = selectedStrategy.config?.coin_source
    if (!coinSource) return false

    if (
      coinSource.source_type === 'ai500' ||
      coinSource.source_type === 'oi_top'
    ) {
      return true
    }
    if (
      coinSource.source_type === 'mixed' &&
      (coinSource.use_ai500 || coinSource.use_oi_top)
    ) {
      return true
    }

    const srcType = coinSource.source_type as string
    if (!srcType && (coinSource.use_ai500 || coinSource.use_oi_top)) {
      return true
    }

    return false
  }, [selectedStrategy])

  // Auto-select first model
  useEffect(() => {
    if (!formState.aiModelId && aiModels?.length) {
      const enabled = aiModels.find((m) => m.enabled)
      if (enabled) setFormState((s) => ({ ...s, aiModelId: enabled.id }))
    }
  }, [aiModels, formState.aiModelId])

  // Auto-select first run
  useEffect(() => {
    if (!selectedRunId && runs.length > 0) {
      setSelectedRunId(runs[0].run_id)
    }
  }, [runs, selectedRunId])

  // Handlers
  const handleFormChange = (
    key: string,
    value: string | number | boolean | string[]
  ) => {
    setFormState((prev) => ({ ...prev, [key]: value }))
  }

  const handleStart = async (event: FormEvent) => {
    event.preventDefault()
    if (!selectedModel?.enabled) {
      setToast({ text: tr('toasts.selectModel'), tone: 'error' })
      return
    }

    try {
      setIsStarting(true)
      const start = new Date(formState.start).getTime()
      const end = new Date(formState.end).getTime()
      if (end <= start) throw new Error(tr('toasts.invalidRange'))

      const userSymbols = formState.symbols
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
      const symbolsToSend =
        userSymbols.length === 0 && strategyHasDynamicCoins ? [] : userSymbols

      const payload = await api.startBacktest({
        run_id: formState.runId.trim() || undefined,
        strategy_id: formState.strategyId || undefined,
        symbols: symbolsToSend,
        timeframes: formState.timeframes,
        decision_timeframe: formState.decisionTf,
        decision_cadence_nbars: formState.cadence,
        start_ts: Math.floor(start / 1000),
        end_ts: Math.floor(end / 1000),
        initial_balance: formState.balance,
        fee_bps: formState.fee,
        slippage_bps: formState.slippage,
        fill_policy: formState.fill,
        prompt_variant: formState.prompt,
        prompt_template: formState.promptTemplate,
        custom_prompt: formState.customPrompt.trim() || undefined,
        override_prompt: formState.overridePrompt,
        cache_ai: formState.cacheAI,
        replay_only: formState.replayOnly,
        ai_model_id: formState.aiModelId,
        leverage: {
          btc_eth_leverage: formState.btcEthLeverage,
          altcoin_leverage: formState.altcoinLeverage,
        },
      })

      setToast({
        text: tr('toasts.startSuccess', { id: payload.run_id }),
        tone: 'success',
      })
      setSelectedRunId(payload.run_id)
      setWizardStep(1)
      await refreshRuns()
    } catch (error: unknown) {
      const errMsg =
        error instanceof Error ? error.message : tr('toasts.startFailed')
      setToast({ text: errMsg, tone: 'error' })
    } finally {
      setIsStarting(false)
    }
  }

  const handleControl = async (action: 'pause' | 'resume' | 'stop') => {
    if (!selectedRunId) return
    try {
      if (action === 'pause') await api.pauseBacktest(selectedRunId)
      if (action === 'resume') await api.resumeBacktest(selectedRunId)
      if (action === 'stop') await api.stopBacktest(selectedRunId)
      setToast({
        text: tr('toasts.actionSuccess', { action, id: selectedRunId }),
        tone: 'success',
      })
      await refreshRuns()
    } catch (error: unknown) {
      const errMsg =
        error instanceof Error ? error.message : tr('toasts.actionFailed')
      setToast({ text: errMsg, tone: 'error' })
    }
  }

  const handleDelete = async () => {
    if (!selectedRunId) return
    const confirmed = await confirmToast(
      tr('toasts.confirmDelete', { id: selectedRunId }),
      {
        title: t('backtestPageExtra.confirmDelete', language),
        okText: t('backtestPageExtra.delete', language),
        cancelText: t('backtestPageExtra.cancel', language),
      }
    )
    if (!confirmed) return
    try {
      await api.deleteBacktestRun(selectedRunId)
      setToast({ text: tr('toasts.deleteSuccess'), tone: 'success' })
      setSelectedRunId(undefined)
      await refreshRuns()
    } catch (error: unknown) {
      const errMsg =
        error instanceof Error ? error.message : tr('toasts.deleteFailed')
      setToast({ text: errMsg, tone: 'error' })
    }
  }

  const handleExport = async () => {
    if (!selectedRunId) return
    try {
      const blob = await api.exportBacktest(selectedRunId)
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `${selectedRunId}_export.zip`
      link.click()
      URL.revokeObjectURL(url)
      setToast({
        text: tr('toasts.exportSuccess', { id: selectedRunId }),
        tone: 'success',
      })
    } catch (error: unknown) {
      const errMsg =
        error instanceof Error ? error.message : tr('toasts.exportFailed')
      setToast({ text: errMsg, tone: 'error' })
    }
  }

  const toggleCompare = (runId: string) => {
    setCompareRunIds((prev) =>
      prev.includes(runId)
        ? prev.filter((id) => id !== runId)
        : [...prev, runId].slice(-3)
    )
  }

  // Render
  return (
    <DeepVoidBackground className="py-8" disableAnimation>
      <div className="w-full px-4 md:px-8 space-y-6">
        {/* Toast */}
        <AnimatePresence>
          {toast && (
            <motion.div
              initial={{ opacity: 0, y: -10 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -10 }}
              className="p-3 rounded-lg text-sm"
              style={{
                background:
                  toast.tone === 'error'
                    ? 'color-mix(in srgb, var(--color-loss) 15%, transparent)'
                    : toast.tone === 'success'
                      ? 'color-mix(in srgb, var(--color-profit) 15%, transparent)'
                      : 'color-mix(in srgb, var(--color-primary) 15%, transparent)',
                color:
                  toast.tone === 'error'
                    ? 'var(--color-loss)'
                    : toast.tone === 'success'
                      ? 'var(--color-profit)'
                      : 'var(--color-primary)',
                border: `1px solid ${toast.tone === 'error' ? 'color-mix(in srgb, var(--color-loss) 30%, transparent)' : toast.tone === 'success' ? 'color-mix(in srgb, var(--color-profit) 30%, transparent)' : 'color-mix(in srgb, var(--color-primary) 30%, transparent)'}`,
              }}
            >
              {toast.text}
            </motion.div>
          )}
        </AnimatePresence>

        {/* Header */}
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <h1 className="text-2xl font-bold flex items-center gap-3 text-foreground">
              <Brain
                className="w-7 h-7"
                style={{ color: 'var(--color-primary)' }}
              />
              {tr('title')}
            </h1>
            <p className="text-sm mt-1 text-muted-foreground">
              {tr('subtitle')}
            </p>
          </div>
          <button
            onClick={() => setWizardStep(1)}
            className="px-4 py-2 rounded-lg font-medium flex items-center gap-2 transition-all hover:opacity-90"
            style={{
              background: 'var(--color-primary)',
              color: 'var(--background)',
            }}
          >
            <Play className="w-4 h-4" />
            {t('backtestPageExtra.newBacktest', language)}
          </button>
        </div>

        <div className="grid grid-cols-1 xl:grid-cols-3 gap-6">
          {/* Left Panel - Config / History */}
          <div className="space-y-4">
            <BacktestConfigForm
              formState={formState}
              wizardStep={wizardStep}
              isStarting={isStarting}
              aiModels={aiModels}
              strategies={strategies}
              language={language}
              tr={tr}
              onFormChange={handleFormChange}
              onWizardStepChange={setWizardStep}
              onStart={handleStart}
            />

            <BacktestRunList
              runs={runs}
              selectedRunId={selectedRunId}
              compareRunIds={compareRunIds}
              language={language}
              tr={tr}
              onSelectRun={setSelectedRunId}
              onToggleCompare={toggleCompare}
            />
          </div>

          {/* Right Panel - Results */}
          <div className="xl:col-span-2 space-y-4">
            {!selectedRunId ? (
              <div
                className="binance-card p-12 text-center"
                style={{ color: 'var(--color-muted-fg)' }}
              >
                <Brain className="w-12 h-12 mx-auto mb-4 opacity-30" />
                <p>{tr('emptyStates.selectRun')}</p>
              </div>
            ) : (
              <>
                {/* Status Bar */}
                <div className="binance-card p-4">
                  <div className="flex flex-wrap items-center justify-between gap-4">
                    <div className="flex items-center gap-4">
                      <ProgressRing
                        progress={
                          status?.progress_pct ??
                          selectedRun?.summary.progress_pct ??
                          0
                        }
                        size={80}
                      />
                      <div>
                        <h2 className="font-mono font-bold text-foreground">
                          {selectedRunId}
                        </h2>
                        <div className="flex items-center gap-2 mt-1">
                          <span
                            className="flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium"
                            style={{
                              background: `color-mix(in srgb, ${getStateColor(status?.state ?? selectedRun?.state ?? '')} 12.5%, transparent)`,
                              color: getStateColor(
                                status?.state ?? selectedRun?.state ?? ''
                              ),
                            }}
                          >
                            {getStateIcon(
                              status?.state ?? selectedRun?.state ?? ''
                            )}
                            {tr(
                              `states.${status?.state ?? selectedRun?.state}`
                            )}
                          </span>
                          {selectedRun?.summary.decision_tf && (
                            <span className="text-xs text-muted-foreground">
                              {selectedRun.summary.decision_tf} ·{' '}
                              {selectedRun.summary.symbol_count} symbols
                            </span>
                          )}
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-2">
                      {(status?.state === 'running' ||
                        selectedRun?.state === 'running') && (
                        <>
                          <button
                            onClick={() => handleControl('pause')}
                            className="p-2 rounded-lg transition-all hover:bg-[var(--color-border)]"
                            style={{ border: '1px solid var(--color-border)' }}
                            title={tr('actions.pause')}
                          >
                            <Pause
                              className="w-4 h-4"
                              style={{ color: 'var(--color-primary)' }}
                            />
                          </button>
                          <button
                            onClick={() => handleControl('stop')}
                            className="p-2 rounded-lg transition-all hover:bg-[var(--color-border)]"
                            style={{ border: '1px solid var(--color-border)' }}
                            title={tr('actions.stop')}
                          >
                            <Square
                              className="w-4 h-4"
                              style={{ color: 'var(--color-loss)' }}
                            />
                          </button>
                        </>
                      )}
                      {status?.state === 'paused' && (
                        <button
                          onClick={() => handleControl('resume')}
                          className="p-2 rounded-lg transition-all hover:bg-[var(--color-border)]"
                          style={{ border: '1px solid var(--color-border)' }}
                          title={tr('actions.resume')}
                        >
                          <Play
                            className="w-4 h-4"
                            style={{ color: 'var(--color-profit)' }}
                          />
                        </button>
                      )}
                      <button
                        onClick={handleExport}
                        className="p-2 rounded-lg transition-all hover:bg-[var(--color-border)]"
                        style={{ border: '1px solid var(--color-border)' }}
                        title={tr('detail.exportLabel')}
                      >
                        <Download className="w-4 h-4 text-foreground" />
                      </button>
                      <button
                        onClick={handleDelete}
                        className="p-2 rounded-lg transition-all hover:bg-[var(--color-border)]"
                        style={{ border: '1px solid var(--color-border)' }}
                        title={tr('detail.deleteLabel')}
                      >
                        <Trash2
                          className="w-4 h-4"
                          style={{ color: 'var(--color-loss)' }}
                        />
                      </button>
                    </div>
                  </div>

                  {(status?.note || status?.last_error) && (
                    <div
                      className="mt-3 p-2 rounded-lg text-xs flex items-center gap-2"
                      style={{
                        background:
                          'color-mix(in srgb, var(--color-loss) 10%, transparent)',
                        border:
                          '1px solid color-mix(in srgb, var(--color-loss) 30%, transparent)',
                        color: 'var(--color-loss)',
                      }}
                    >
                      <AlertTriangle className="w-4 h-4 flex-shrink-0" />
                      {status?.note || status?.last_error}
                    </div>
                  )}

                  {/* Real-time Positions Display */}
                  {status?.positions && status.positions.length > 0 && (
                    <PositionsDisplay
                      positions={status.positions}
                      language={language}
                    />
                  )}
                </div>

                {/* Stats Grid */}
                <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                  <StatCard
                    icon={Target}
                    label={t('backtestPageExtra.equity', language)}
                    value={(status?.equity ?? 0).toFixed(2)}
                    suffix="USDT"
                    language={language}
                  />
                  <StatCard
                    icon={TrendingUp}
                    label={t('backtestPageExtra.totalReturn', language)}
                    value={`${(metrics?.total_return_pct ?? 0).toFixed(2)}%`}
                    trend={
                      (metrics?.total_return_pct ?? 0) >= 0 ? 'up' : 'down'
                    }
                    color={
                      (metrics?.total_return_pct ?? 0) >= 0
                        ? 'var(--color-profit)'
                        : 'var(--color-loss)'
                    }
                    metricKey="total_return"
                    language={language}
                  />
                  <StatCard
                    icon={AlertTriangle}
                    label={t('backtestPageExtra.maxDD', language)}
                    value={`${(metrics?.max_drawdown_pct ?? 0).toFixed(2)}%`}
                    color="var(--color-loss)"
                    metricKey="max_drawdown"
                    language={language}
                  />
                  <StatCard
                    icon={BarChart3}
                    label={t('backtestPageExtra.sharpe', language)}
                    value={(metrics?.sharpe_ratio ?? 0).toFixed(2)}
                    metricKey="sharpe_ratio"
                    language={language}
                  />
                </div>

                {/* Tabs */}
                <div className="binance-card">
                  <div
                    className="flex border-b"
                    style={{ borderColor: 'var(--color-border)' }}
                  >
                    {(
                      ['overview', 'chart', 'trades', 'decisions'] as ViewTab[]
                    ).map((tab) => (
                      <button
                        key={tab}
                        onClick={() => setViewTab(tab)}
                        className="px-4 py-3 text-sm font-medium transition-all relative"
                        style={{
                          color:
                            viewTab === tab
                              ? 'var(--color-primary)'
                              : 'var(--color-muted-fg)',
                        }}
                      >
                        {tab === 'overview'
                          ? t('backtestPageExtra.tabOverview', language)
                          : tab === 'chart'
                            ? t('backtestPageExtra.tabChart', language)
                            : tab === 'trades'
                              ? t('backtestPageExtra.tabTrades', language)
                              : t('backtestPageExtra.tabDecisions', language)}
                        {viewTab === tab && (
                          <motion.div
                            layoutId="tab-indicator"
                            className="absolute bottom-0 left-0 right-0 h-0.5"
                            style={{ background: 'var(--color-primary)' }}
                          />
                        )}
                      </button>
                    ))}
                  </div>

                  <div className="p-4">
                    <AnimatePresence mode="wait">
                      {viewTab === 'overview' && (
                        <BacktestOverviewTab
                          equity={equity}
                          trades={trades}
                          metrics={metrics}
                          language={language}
                          tr={tr}
                        />
                      )}

                      {viewTab === 'chart' && (
                        <BacktestChartTab
                          equity={equity}
                          trades={trades}
                          selectedRunId={selectedRunId}
                          language={language}
                          tr={tr}
                        />
                      )}

                      {viewTab === 'trades' && (
                        <BacktestTradesTab trades={trades} />
                      )}

                      {viewTab === 'decisions' && (
                        <BacktestDecisionsTab
                          decisions={decisions}
                          language={language}
                          tr={tr}
                        />
                      )}
                    </AnimatePresence>
                  </div>
                </div>
              </>
            )}
          </div>
        </div>
      </div>
    </DeepVoidBackground>
  )
}
