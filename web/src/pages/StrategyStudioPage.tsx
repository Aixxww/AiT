import {
  lazy,
  Suspense,
  useState,
  useEffect,
  useCallback,
  useRef,
  type ReactNode,
} from 'react'
import { useAuth } from '../contexts/AuthContext'
import { useLanguage } from '../contexts/LanguageContext'
import {
  Plus,
  Copy,
  Trash2,
  Check,
  ChevronDown,
  Settings,
  BarChart3,
  Target,
  Shield,
  Zap,
  Activity,
  Save,
  Sparkles,
  Eye,
  Play,
  FileText,
  Loader2,
  RefreshCw,
  Clock,
  Bot,
  Terminal,
  Code,
  Send,
  Download,
  Upload,
  Globe,
  X,
} from 'lucide-react'
import type {
  Strategy,
  StrategyConfig,
  AIModel,
  GridStrategyConfig,
} from '../types'
import { confirmToast, notify } from '../lib/notify'
import { PublishSettingsEditor } from '../components/strategy/PublishSettingsEditor'
import { defaultGridConfig } from '../components/strategy/gridConfigDefaults'
import { TokenEstimateBar } from '../components/strategy/TokenEstimateBar'
import { DeepVoidBackground } from '../components/common/DeepVoidBackground'
import { t } from '../i18n/translations'
import { displayName } from '../i18n/displayNames'
import { InfoTip } from '../components/ui/InfoTip'
import { SectionHeader } from '../components/ui/SectionHeader'
import { Button } from '../components/ui/Button'

const API_BASE = import.meta.env.VITE_API_BASE || ''

const CoinSourceEditor = lazy(() =>
  import('../components/strategy/CoinSourceEditor').then((m) => ({
    default: m.CoinSourceEditor,
  }))
)
const IndicatorEditor = lazy(() =>
  import('../components/strategy/IndicatorEditor').then((m) => ({
    default: m.IndicatorEditor,
  }))
)
const RiskControlEditor = lazy(() =>
  import('../components/strategy/RiskControlEditor').then((m) => ({
    default: m.RiskControlEditor,
  }))
)
const PromptSectionsEditor = lazy(() =>
  import('../components/strategy/PromptSectionsEditor').then((m) => ({
    default: m.PromptSectionsEditor,
  }))
)
const GridConfigEditor = lazy(() =>
  import('../components/strategy/GridConfigEditor').then((m) => ({
    default: m.GridConfigEditor,
  }))
)

export function StrategyStudioPage() {
  const { token } = useAuth()
  const { language } = useLanguage()

  const [strategies, setStrategies] = useState<Strategy[]>([])
  const [selectedStrategy, setSelectedStrategy] = useState<Strategy | null>(
    null
  )
  const [editingConfig, setEditingConfig] = useState<StrategyConfig | null>(
    null
  )
  const editingConfigRef = useRef<StrategyConfig | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [estimatedTokens, setEstimatedTokens] = useState(0)
  const [error, setError] = useState<string | null>(null)
  const [hasChanges, setHasChanges] = useState(false)

  // AI Models for test run
  const [aiModels, setAiModels] = useState<AIModel[]>([])
  const [selectedModelId, setSelectedModelId] = useState<string>('')

  // Accordion states for left panel
  const [expandedSections, setExpandedSections] = useState({
    gridConfig: true,
    coinSource: true,
    indicators: false,
    riskControl: false,
    promptCompression: false,
    promptSections: false,
    customPrompt: false,
    publishSettings: false,
  })

  // Right panel states
  const [activeRightTab, setActiveRightTab] = useState<'prompt' | 'test'>(
    'prompt'
  )
  // Below xl, editor and preview share one column.
  const [mobilePane, setMobilePane] = useState<'edit' | 'preview'>('edit')
  const [promptPreview, setPromptPreview] = useState<{
    system_prompt: string
    user_prompt?: string
    prompt_variant: string
    config_summary: Record<string, unknown>
  } | null>(null)
  const [isLoadingPrompt, setIsLoadingPrompt] = useState(false)
  const [selectedVariant, setSelectedVariant] = useState('balanced')

  // AI Test Run states
  const [aiTestResult, setAiTestResult] = useState<{
    system_prompt?: string
    user_prompt?: string
    ai_response?: string
    reasoning?: string
    decisions?: unknown[]
    error?: string
    duration_ms?: number
  } | null>(null)
  const [isRunningAiTest, setIsRunningAiTest] = useState(false)
  const gridConfigCacheRef = useRef<Record<string, GridStrategyConfig>>({})

  useEffect(() => {
    editingConfigRef.current = editingConfig
  }, [editingConfig])

  const toggleSection = (section: keyof typeof expandedSections) => {
    setExpandedSections((prev) => ({
      ...prev,
      [section]: !prev[section],
    }))
  }

  // Fetch AI Models
  const fetchAiModels = useCallback(async () => {
    if (!token) return
    try {
      const response = await fetch(`${API_BASE}/api/models`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (response.ok) {
        const data = await response.json()
        // Backend returns an array, not { models: [] }
        const allModels = Array.isArray(data) ? data : data.models || []
        const enabledModels = allModels.filter((m: AIModel) => m.enabled)
        setAiModels(enabledModels)
        if (enabledModels.length > 0 && !selectedModelId) {
          setSelectedModelId(enabledModels[0].id)
        }
      }
    } catch (err) {
      console.error('Failed to fetch AI models:', err)
    }
  }, [token, selectedModelId])

  // Fetch strategies
  const fetchStrategies = useCallback(async () => {
    if (!token) return
    try {
      const response = await fetch(`${API_BASE}/api/strategies`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!response.ok) throw new Error('Failed to fetch strategies')
      const data = await response.json()
      setStrategies(data.strategies || [])

      // Select active or first strategy
      const active = data.strategies?.find((s: Strategy) => s.is_active)
      if (active) {
        setSelectedStrategy(active)
        setEditingConfig(active.config)
      } else if (data.strategies?.length > 0) {
        setSelectedStrategy(data.strategies[0])
        setEditingConfig(data.strategies[0].config)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setIsLoading(false)
    }
  }, [token])

  useEffect(() => {
    fetchStrategies()
    fetchAiModels()
  }, [fetchStrategies, fetchAiModels])

  useEffect(() => {
    if (!selectedStrategy?.id || !editingConfig?.grid_config) return

    gridConfigCacheRef.current[selectedStrategy.id] = {
      ...editingConfig.grid_config,
    }
  }, [selectedStrategy?.id, editingConfig?.grid_config])

  // Track previous language to detect actual changes
  const prevLanguageRef = useRef(language)

  // When language changes, update prompt sections to match the new language
  useEffect(() => {
    const updatePromptSectionsForLanguage = async () => {
      // Only update if language actually changed (not on initial mount)
      if (prevLanguageRef.current === language) return
      prevLanguageRef.current = language

      if (!token) return

      try {
        // Fetch default config for the new language
        const response = await fetch(
          `${API_BASE}/api/strategies/default-config?lang=${language}`,
          { headers: { Authorization: `Bearer ${token}` } }
        )
        if (!response.ok) return
        const defaultConfig = await response.json()

        // Update only the prompt sections and language field
        setEditingConfig((prev) => {
          if (!prev) return prev
          return {
            ...prev,
            language: language as 'zh' | 'en',
            prompt_sections: defaultConfig.prompt_sections,
          }
        })
        setHasChanges(true)
      } catch (err) {
        console.error('Failed to update prompt sections for language:', err)
      }
    }

    updatePromptSectionsForLanguage()
  }, [language, token]) // Only trigger when language changes

  // Create new strategy
  const handleCreateStrategy = async () => {
    if (!token) return
    try {
      const configResponse = await fetch(
        `${API_BASE}/api/strategies/default-config?lang=${language}`,
        { headers: { Authorization: `Bearer ${token}` } }
      )
      if (!configResponse.ok) throw new Error('Failed to fetch default config')
      const defaultConfig = await configResponse.json()

      const response = await fetch(`${API_BASE}/api/strategies`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          name: tr('newStrategyName'),
          description: '',
          config: defaultConfig,
        }),
      })
      if (!response.ok) throw new Error('Failed to create strategy')
      const result = await response.json()
      await fetchStrategies()
      // Auto-select the newly created strategy
      if (result.id) {
        const now = new Date().toISOString()
        const newStrategy = {
          id: result.id,
          name: tr('newStrategyName'),
          description: '',
          is_active: false,
          is_default: false,
          is_public: false,
          config_visible: true,
          config: defaultConfig,
          created_at: now,
          updated_at: now,
        }
        setSelectedStrategy(newStrategy)
        setEditingConfig(defaultConfig)
        setHasChanges(false)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    }
  }

  // Delete strategy
  const handleDeleteStrategy = async (id: string) => {
    if (!token) return
    const strategy = strategies.find((item) => item.id === id)

    if (strategy?.is_active) {
      notify.error(tr('cannotDeleteActiveStrategy'))
      return
    }

    // Check if strategy is in use by any trader before showing dialog
    try {
      const tradersResp = await fetch(`${API_BASE}/api/my-traders`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (tradersResp.ok) {
        const traderList = await tradersResp.json()
        const using = traderList.filter((t: any) => t.strategy_id === id)
        if (using.length > 0) {
          const names = using.map((t: any) => t.trader_name).join(', ')
          notify.error(`Strategy is in use by: ${names}`)
          return
        }
      }
    } catch {
      // fetch failed — proceed, backend will guard
    }

    const confirmed = await confirmToast(tr('confirmDeleteStrategy'), {
      title: tr('confirmDelete'),
      okText: tr('delete'),
      cancelText: tr('cancel'),
    })
    if (!confirmed) return

    try {
      const response = await fetch(`${API_BASE}/api/strategies/${id}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!response.ok) {
        const data = await response.json().catch(() => ({}))
        notify.error(data.error || 'Failed to delete strategy')
        return
      }
      notify.success(tr('strategyDeleted'))
      if (selectedStrategy?.id === id) {
        setSelectedStrategy(null)
        setEditingConfig(null)
        setHasChanges(false)
      }
      await fetchStrategies()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : 'Unknown error')
    }
  }

  // Duplicate strategy
  const handleDuplicateStrategy = async (id: string) => {
    if (!token) return
    try {
      const response = await fetch(
        `${API_BASE}/api/strategies/${id}/duplicate`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify({
            name: tr('strategyCopy'),
          }),
        }
      )
      if (!response.ok) throw new Error('Failed to duplicate strategy')
      await fetchStrategies()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    }
  }

  // Activate strategy
  const handleActivateStrategy = async (id: string) => {
    if (!token) return
    try {
      const response = await fetch(
        `${API_BASE}/api/strategies/${id}/activate`,
        {
          method: 'POST',
          headers: { Authorization: `Bearer ${token}` },
        }
      )
      if (!response.ok) throw new Error('Failed to activate strategy')
      await fetchStrategies()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    }
  }

  // Export strategy as JSON file
  const handleExportStrategy = (strategy: Strategy) => {
    const exportData = {
      name: strategy.name,
      description: strategy.description,
      config: strategy.config,
      exported_at: new Date().toISOString(),
      version: '1.0',
    }
    const blob = new Blob([JSON.stringify(exportData, null, 2)], {
      type: 'application/json',
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `strategy_${strategy.name.replace(/\s+/g, '_')}_${new Date().toISOString().split('T')[0]}.json`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
    notify.success(tr('strategyExported'))
  }

  // Import strategy from JSON file
  const handleImportStrategy = async (
    event: React.ChangeEvent<HTMLInputElement>
  ) => {
    const file = event.target.files?.[0]
    if (!file || !token) return

    try {
      const text = await file.text()
      const importData = JSON.parse(text)

      // Validate imported data
      if (!importData.config || !importData.name) {
        throw new Error(tr('invalidStrategyFile'))
      }

      // Create new strategy with imported config
      const response = await fetch(`${API_BASE}/api/strategies`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          name: `${importData.name} (${tr('imported')})`,
          description: importData.description || '',
          config: importData.config,
        }),
      })
      if (!response.ok) throw new Error('Failed to import strategy')

      notify.success(tr('strategyImported'))
      await fetchStrategies()
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : 'Unknown error'
      notify.error(errorMsg)
    } finally {
      // Reset file input
      event.target.value = ''
    }
  }

  // Save strategy
  const handleSaveStrategy = async () => {
    const latestEditingConfig = editingConfigRef.current || editingConfig
    if (!token || !selectedStrategy || !latestEditingConfig) return
    if (estimatedTokens >= 128000 && currentStrategyType === 'ai_trading') {
      notify.warning(tr('tokenExceedWarning'))
      // continue with save
    }
    setIsSaving(true)
    try {
      // Always sync the config language with the current interface language
      const configWithLanguage = {
        ...latestEditingConfig,
        language: language as 'zh' | 'en',
      }
      const response = await fetch(
        `${API_BASE}/api/strategies/${selectedStrategy.id}`,
        {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify({
            name: selectedStrategy.name,
            description: selectedStrategy.description,
            config: configWithLanguage,
            is_public: selectedStrategy.is_public,
            config_visible: selectedStrategy.config_visible,
          }),
        }
      )
      if (!response.ok) throw new Error('Failed to save strategy')
      setHasChanges(false)
      notify.success(tr('strategySaved'))
      await fetchStrategies()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setIsSaving(false)
    }
  }

  // Update config section
  const updateConfig = <K extends keyof StrategyConfig>(
    section: K,
    value: StrategyConfig[K]
  ) => {
    setEditingConfig((prev) => {
      if (!prev) return prev
      const next = {
        ...prev,
        [section]: value,
      }
      editingConfigRef.current = next
      return next
    })
    setHasChanges(true)
  }

  const handleStrategyTypeChange = (
    strategyType: NonNullable<StrategyConfig['strategy_type']>
  ) => {
    if (selectedStrategy?.is_default) return

    const cachedGridConfig = selectedStrategy?.id
      ? gridConfigCacheRef.current[selectedStrategy.id]
      : null

    setEditingConfig((prev) => {
      if (!prev) return prev

      if (strategyType === 'ai_trading') {
        if (selectedStrategy?.id && prev.grid_config) {
          gridConfigCacheRef.current[selectedStrategy.id] = {
            ...prev.grid_config,
          }
        }

        return {
          ...prev,
          strategy_type: 'ai_trading',
          // Use null so the field is preserved in JSON and backend merge can actually clear it.
          grid_config: null,
        }
      }

      return {
        ...prev,
        strategy_type: 'grid_trading',
        grid_config: cachedGridConfig ??
          prev.grid_config ?? { ...defaultGridConfig },
      }
    })

    setPromptPreview(null)
    setAiTestResult(null)
    setHasChanges(true)
  }

  // Fetch prompt preview
  const fetchPromptPreview = async () => {
    if (!token || !editingConfig) return
    setIsLoadingPrompt(true)
    try {
      const response = await fetch(
        `${API_BASE}/api/strategies/preview-prompt`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify({
            config: editingConfig,
            account_equity: 1000,
            prompt_variant: selectedVariant,
          }),
        }
      )
      if (!response.ok) throw new Error('Failed to fetch prompt preview')
      const data = await response.json()
      setPromptPreview(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setIsLoadingPrompt(false)
    }
  }

  // Run AI test with real AI model
  const runAiTest = async () => {
    if (!token || !editingConfig || !selectedModelId) return
    setIsRunningAiTest(true)
    setAiTestResult(null)
    try {
      const response = await fetch(`${API_BASE}/api/strategies/test-run`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          config: editingConfig,
          prompt_variant: selectedVariant,
          ai_model_id: selectedModelId,
          run_real_ai: true,
        }),
      })
      if (!response.ok) throw new Error('Failed to run AI test')
      const data = await response.json()
      setAiTestResult(data)
    } catch (err) {
      setAiTestResult({
        error: err instanceof Error ? err.message : 'Unknown error',
      })
    } finally {
      setIsRunningAiTest(false)
    }
  }

  const tr = (key: string, params?: Record<string, string | number>) =>
    t(`strategyStudio.${key}`, language, params)

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-[70vh]">
        <div className="text-center">
          <div className="relative">
            <div className="w-16 h-16 rounded-full border-4 border-primary/20 border-t-primary animate-spin" />
            <Zap className="w-6 h-6 text-primary absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2" />
          </div>
        </div>
      </div>
    )
  }

  // Get current strategy type (default to ai_trading if not set)
  const currentStrategyType = editingConfig?.strategy_type || 'ai_trading'
  // Prompt compaction is boolean on the backend: "off" disables it, any other
  // stored value (including legacy modes) enables it.
  const promptCompactEnabled =
    (editingConfig?.prompt_compact_mode || 'current_source')
      .trim()
      .toLowerCase() !== 'off'

  const configSections = [
    // Grid Config - only for grid_trading
    {
      key: 'gridConfig' as const,
      icon: Activity,
      color: 'var(--color-profit)',
      title: tr('gridConfig'),
      summary: tr('gridConfigSummary'),
      forStrategyType: 'grid_trading' as const,
      content: editingConfig?.grid_config && (
        <GridConfigEditor
          config={editingConfig.grid_config}
          onChange={(gridConfig) => updateConfig('grid_config', gridConfig)}
          disabled={selectedStrategy?.is_default}
          language={language}
        />
      ),
    },
    // AI Trading sections
    {
      key: 'coinSource' as const,
      icon: Target,
      color: 'var(--color-primary)',
      title: tr('coinSource'),
      summary: tr('coinSourceSummary'),
      forStrategyType: 'ai_trading' as const,
      content: editingConfig && (
        <CoinSourceEditor
          config={editingConfig.coin_source}
          onChange={(coinSource) => updateConfig('coin_source', coinSource)}
          disabled={selectedStrategy?.is_default}
          language={language}
        />
      ),
    },
    {
      key: 'indicators' as const,
      icon: BarChart3,
      color: 'var(--color-profit)',
      title: tr('indicators'),
      summary: tr('indicatorsSummary'),
      forStrategyType: 'ai_trading' as const,
      content: editingConfig && (
        <IndicatorEditor
          config={editingConfig.indicators}
          onChange={(indicators) => updateConfig('indicators', indicators)}
          disabled={selectedStrategy?.is_default}
          language={language}
        />
      ),
    },
    {
      key: 'riskControl' as const,
      icon: Shield,
      color: 'var(--color-loss)',
      title: tr('riskControl'),
      summary: tr('riskControlSummary'),
      forStrategyType: 'ai_trading' as const,
      content: editingConfig && (
        <RiskControlEditor
          config={editingConfig.risk_control}
          onChange={(riskControl) => updateConfig('risk_control', riskControl)}
          disabled={selectedStrategy?.is_default}
          language={language}
        />
      ),
    },
    {
      key: 'promptCompression' as const,
      icon: Zap,
      color: 'var(--color-warning)',
      title: tr('promptCompression'),
      summary: tr('promptCompressionSummary'),
      forStrategyType: 'ai_trading' as const,
      content: editingConfig && (
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <div>
              <div className="text-sm font-medium text-foreground">
                {tr('promptCompactMode')}
              </div>
              <div className="text-xs text-muted-foreground mt-1">
                {tr('promptCompactModeDesc')}
              </div>
            </div>
            <label className="flex items-center gap-2 cursor-pointer shrink-0">
              <input
                type="checkbox"
                checked={promptCompactEnabled}
                onChange={(e) =>
                  updateConfig(
                    'prompt_compact_mode',
                    e.target.checked ? 'current_source' : 'off'
                  )
                }
                disabled={selectedStrategy?.is_default}
                className="w-5 h-5 rounded accent-primary"
              />
              <span className="text-sm text-foreground">
                {promptCompactEnabled
                  ? tr('promptCompactOn')
                  : tr('promptCompactOff')}
              </span>
            </label>
          </div>
          <p className="text-xs text-muted-foreground">
            {tr('promptCompactReloadHint')}
          </p>
        </div>
      ),
    },
    {
      key: 'promptSections' as const,
      icon: FileText,
      color: 'var(--color-accent)',
      title: tr('promptSections'),
      summary: tr('promptSectionsSummary'),
      forStrategyType: 'ai_trading' as const,
      content: editingConfig && (
        <PromptSectionsEditor
          config={editingConfig.prompt_sections}
          onChange={(promptSections) =>
            updateConfig('prompt_sections', promptSections)
          }
          disabled={selectedStrategy?.is_default}
          language={language}
        />
      ),
    },
    {
      key: 'customPrompt' as const,
      icon: Settings,
      color: 'var(--color-accent)',
      title: tr('customPrompt'),
      summary: tr('customPromptSummary'),
      forStrategyType: 'ai_trading' as const,
      content: editingConfig && (
        <div>
          <p className="text-xs mb-2 text-muted-foreground">
            {tr('customPromptDesc')}
          </p>
          <textarea
            value={editingConfig.custom_prompt || ''}
            onChange={(e) => updateConfig('custom_prompt', e.target.value)}
            disabled={selectedStrategy?.is_default}
            placeholder={tr('customPromptPlaceholder')}
            className="premium-input w-full h-32 resize-none text-xs"
          />
        </div>
      ),
    },
    {
      key: 'publishSettings' as const,
      icon: Globe,
      color: 'var(--color-profit)',
      title: tr('publishSettings'),
      summary: tr('publishSettingsSummary'),
      forStrategyType: 'both' as const,
      content: selectedStrategy && (
        <PublishSettingsEditor
          isPublic={selectedStrategy.is_public ?? false}
          configVisible={selectedStrategy.config_visible ?? true}
          onIsPublicChange={(value) => {
            setSelectedStrategy({ ...selectedStrategy, is_public: value })
            setHasChanges(true)
          }}
          onConfigVisibleChange={(value) => {
            setSelectedStrategy({ ...selectedStrategy, config_visible: value })
            setHasChanges(true)
          }}
          disabled={selectedStrategy?.is_default}
          language={language}
        />
      ),
    },
  ].filter(
    (section) =>
      section.forStrategyType === 'both' ||
      section.forStrategyType === currentStrategyType
  )

  const configSummaryLabels: Record<string, string> = {
    coin_source: tr('summaryCoinSource'),
    primary_tf: tr('summaryPrimaryTf'),
    btc_eth_leverage: tr('summaryBtcEthLeverage'),
    altcoin_leverage: tr('summaryAltcoinLeverage'),
    max_positions: tr('summaryMaxPositions'),
  }
  const formatSummaryValue = (key: string, value: unknown) =>
    key === 'coin_source'
      ? displayName('sourceType', String(value ?? ''), language)
      : String(value ?? '')

  const selectStrategy = (strategy: Strategy) => {
    setSelectedStrategy(strategy)
    setEditingConfig(strategy.config)
    setHasChanges(false)
    setPromptPreview(null)
    setAiTestResult(null)
    setMobilePane('edit')
  }

  const strategyList = (
    <div className="flex lg:flex-col gap-2 overflow-x-auto lg:overflow-x-visible pb-1 lg:pb-0 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
      {strategies.map((strategy) => {
        const selected = selectedStrategy?.id === strategy.id
        return (
          <div
            key={strategy.id}
            role="button"
            tabIndex={0}
            aria-pressed={selected}
            onClick={() => selectStrategy(strategy)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                selectStrategy(strategy)
              }
            }}
            className={`group shrink-0 w-[200px] lg:w-auto px-3 py-2.5 rounded-control cursor-pointer border transition-colors focus-visible:outline-2 focus-visible:outline-primary ${
              selected
                ? 'border-primary/50 bg-primary-dim'
                : 'border-border bg-panel hover:border-border-hover'
            }`}
          >
            <div className="flex items-start justify-between gap-2">
              <span className="t-label line-clamp-2">{strategy.name}</span>
              <div className="ait-hover-reveal flex items-center -mr-1 -mt-0.5">
                <IconAction
                  label={tr('export')}
                  onClick={() => handleExportStrategy(strategy)}
                >
                  <Download className="w-3.5 h-3.5" />
                </IconAction>
                {!strategy.is_default && (
                  <>
                    <IconAction
                      label={tr('duplicate')}
                      onClick={() => handleDuplicateStrategy(strategy.id)}
                    >
                      <Copy className="w-3.5 h-3.5" />
                    </IconAction>
                    <IconAction
                      label={
                        strategy.is_active
                          ? tr('cannotDeleteActiveStrategy')
                          : tr('deleteTooltip')
                      }
                      danger
                      disabled={strategy.is_active}
                      onClick={() => handleDeleteStrategy(strategy.id)}
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </IconAction>
                  </>
                )}
              </div>
            </div>
            {(strategy.is_active ||
              strategy.is_default ||
              strategy.is_public) && (
              <div className="flex items-center gap-1 mt-1.5 flex-wrap">
                {strategy.is_active && (
                  <span className="ait-pill bg-profit/15 text-profit">
                    {tr('active')}
                  </span>
                )}
                {strategy.is_default && (
                  <span className="ait-pill bg-primary/15 text-primary">
                    {tr('default')}
                  </span>
                )}
                {strategy.is_public && (
                  <span className="ait-pill bg-accent/15 text-accent">
                    <Globe className="w-2.5 h-2.5" />
                    {tr('public')}
                  </span>
                )}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )

  const previewPane = (
    <div className="flex flex-col min-h-0 h-full">
      <div className="flex-shrink-0 p-3 border-b border-border">
        <div className="ait-segmented w-full" role="tablist">
          <button
            type="button"
            role="tab"
            aria-selected={activeRightTab === 'prompt'}
            onClick={() => setActiveRightTab('prompt')}
            className="flex-1 inline-flex items-center justify-center gap-1.5"
          >
            <Eye className="w-3.5 h-3.5" />
            {tr('promptPreview')}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeRightTab === 'test'}
            onClick={() => setActiveRightTab('test')}
            className="flex-1 inline-flex items-center justify-center gap-1.5"
          >
            <Play className="w-3.5 h-3.5" />
            {tr('aiTestRun')}
          </button>
        </div>
      </div>

      <div className="flex-1 lg:overflow-y-auto">
        {activeRightTab === 'prompt' ? (
          <div className="p-3 space-y-3">
            <div className="flex items-center gap-2 flex-wrap">
              <select
                value={selectedVariant}
                onChange={(e) => setSelectedVariant(e.target.value)}
                className="premium-input h-9 py-0 text-caption"
              >
                <option value="balanced">{tr('balanced')}</option>
                <option value="aggressive">{tr('aggressive')}</option>
                <option value="conservative">{tr('conservative')}</option>
              </select>
              <Button
                variant="unstyled"
                onClick={fetchPromptPreview}
                disabled={isLoadingPrompt || !editingConfig}
                className="inline-flex items-center gap-1.5 h-9 px-3.5 rounded-control text-caption font-semibold bg-foreground text-background hover:opacity-90 disabled:opacity-50"
              >
                {isLoadingPrompt ? (
                  <Loader2 className="w-3.5 h-3.5 animate-spin" />
                ) : (
                  <RefreshCw className="w-3.5 h-3.5" />
                )}
                {promptPreview ? tr('refreshPrompt') : tr('loadPrompt')}
              </Button>
            </div>

            {promptPreview ? (
              <>
                <div className="ait-card-inset p-3">
                  <div className="flex items-center gap-1.5 mb-2">
                    <Code className="w-3.5 h-3.5 text-muted-foreground" />
                    <span className="t-caption font-semibold text-foreground">
                      {tr('configSummary')}
                    </span>
                  </div>
                  <dl className="grid grid-cols-2 sm:grid-cols-3 gap-x-3 gap-y-2">
                    {Object.entries(promptPreview.config_summary || {}).map(
                      ([key, value]) => (
                        <div key={key} className="min-w-0">
                          <dt className="t-micro text-muted-foreground truncate">
                            {configSummaryLabels[key] ?? tr('configSummary')}
                          </dt>
                          <dd className="t-caption text-foreground t-num truncate">
                            {formatSummaryValue(key, value)}
                          </dd>
                        </div>
                      )
                    )}
                  </dl>
                </div>

                <div>
                  <div className="flex items-center justify-between mb-1.5">
                    <div className="flex items-center gap-1.5">
                      <FileText className="w-3.5 h-3.5 text-muted-foreground" />
                      <span className="t-caption font-semibold text-foreground">
                        {tr('systemPrompt')}
                      </span>
                    </div>
                    <span className="ait-pill bg-surface-alt text-muted-foreground t-num">
                      {promptPreview.system_prompt.length.toLocaleString()}{' '}
                      {t('ui.chars', language)}
                    </span>
                  </div>
                  <pre className="ait-card-inset p-3 text-[11px] leading-relaxed font-mono overflow-auto text-foreground max-h-[400px] whitespace-pre-wrap break-words">
                    {promptPreview.system_prompt}
                  </pre>
                </div>
              </>
            ) : (
              <EmptyState
                icon={<Eye className="w-8 h-8" />}
                text={tr('generatePromptPreview')}
              />
            )}
          </div>
        ) : (
          <div className="p-3 space-y-3">
            <div className="space-y-2">
              <div className="flex items-center gap-1.5">
                <Bot className="w-3.5 h-3.5 text-muted-foreground" />
                <span className="t-caption font-semibold text-foreground">
                  {tr('selectModel')}
                </span>
              </div>
              {aiModels.length > 0 ? (
                <select
                  value={selectedModelId}
                  onChange={(e) => setSelectedModelId(e.target.value)}
                  className="premium-input w-full h-10 py-0"
                >
                  {aiModels.map((model) => (
                    <option key={model.id} value={model.id}>
                      {model.name} ({model.provider})
                    </option>
                  ))}
                </select>
              ) : (
                <div className="px-3 py-2 rounded-control t-caption bg-loss/10 text-loss">
                  {tr('noModel')}
                </div>
              )}

              <div className="flex items-center gap-2">
                <select
                  value={selectedVariant}
                  onChange={(e) => setSelectedVariant(e.target.value)}
                  className="premium-input h-10 py-0 text-caption"
                >
                  <option value="balanced">{tr('balanced')}</option>
                  <option value="aggressive">{tr('aggressive')}</option>
                  <option value="conservative">{tr('conservative')}</option>
                </select>
                <Button
                  variant="unstyled"
                  onClick={runAiTest}
                  disabled={
                    isRunningAiTest || !editingConfig || !selectedModelId
                  }
                  className="flex-1 inline-flex items-center justify-center gap-2 h-10 px-4 rounded-control t-label bg-primary text-primary-foreground hover:opacity-90 disabled:opacity-50"
                >
                  {isRunningAiTest ? (
                    <>
                      <Loader2 className="w-4 h-4 animate-spin" />
                      {tr('running')}
                    </>
                  ) : (
                    <>
                      <Send className="w-4 h-4" />
                      {tr('runTest')}
                    </>
                  )}
                </Button>
              </div>
              <p className="t-micro text-muted-foreground">{tr('testNote')}</p>
            </div>

            {aiTestResult ? (
              <div className="space-y-3">
                {aiTestResult.error ? (
                  <div className="p-3 rounded-control bg-loss/10 border border-loss/30">
                    <p className="t-caption text-loss">{aiTestResult.error}</p>
                  </div>
                ) : (
                  <>
                    {aiTestResult.duration_ms && (
                      <div className="flex items-center gap-2">
                        <Clock className="w-3.5 h-3.5 text-muted-foreground" />
                        <span className="t-caption text-muted-foreground t-num">
                          {tr('duration')}:{' '}
                          {(aiTestResult.duration_ms / 1000).toFixed(2)}s
                        </span>
                      </div>
                    )}
                    {aiTestResult.user_prompt && (
                      <ResultBlock
                        icon={<Terminal className="w-3.5 h-3.5" />}
                        title={tr('userPromptInput')}
                        maxHeight={200}
                        text={aiTestResult.user_prompt}
                      />
                    )}
                    {aiTestResult.reasoning && (
                      <ResultBlock
                        icon={<Sparkles className="w-3.5 h-3.5" />}
                        title={tr('reasoning')}
                        maxHeight={200}
                        text={aiTestResult.reasoning}
                      />
                    )}
                    {aiTestResult.decisions &&
                      aiTestResult.decisions.length > 0 && (
                        <ResultBlock
                          icon={<Activity className="w-3.5 h-3.5" />}
                          title={tr('decisions')}
                          maxHeight={200}
                          text={JSON.stringify(aiTestResult.decisions, null, 2)}
                        />
                      )}
                    {aiTestResult.ai_response && (
                      <ResultBlock
                        icon={<FileText className="w-3.5 h-3.5" />}
                        title={tr('aiOutputRaw')}
                        maxHeight={300}
                        text={aiTestResult.ai_response}
                      />
                    )}
                  </>
                )}
              </div>
            ) : (
              <EmptyState
                icon={<Play className="w-8 h-8" />}
                text={tr('runAiTestHint')}
              />
            )}
          </div>
        )}
      </div>
    </div>
  )

  return (
    <DeepVoidBackground className="lg:h-[calc(100vh-64px)] min-h-[calc(100vh-64px)] flex flex-col bg-background relative lg:overflow-hidden">
      {/* Page header */}
      <header className="flex-shrink-0 border-b border-border bg-background/70 backdrop-blur-md z-10">
        <div className="px-[var(--page-gutter)] py-4 flex items-center justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <div className="hidden sm:flex p-2 rounded-control bg-primary/15 text-primary">
              <Sparkles className="w-5 h-5" />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-1.5">
                <h1 className="t-title">{tr('title')}</h1>
                <InfoTip
                  content={tr('subtitleInfo')}
                  title={tr('title')}
                  side="bottom"
                  align="start"
                />
              </div>
              <p className="t-caption text-muted-foreground truncate">
                {tr('subtitle')}
              </p>
            </div>
          </div>
          {error && (
            <div className="flex items-center gap-2 px-3 py-1.5 rounded-control t-caption bg-loss/10 text-loss max-w-[50%]">
              <span className="truncate">{error}</span>
              <button
                type="button"
                onClick={() => setError(null)}
                className="hover:opacity-70"
                aria-label={t('ui.close', language)}
              >
                <X className="w-3.5 h-3.5" />
              </button>
            </div>
          )}
        </div>

        {/* Below xl the editor and the preview share one column: a segmented switch picks the pane. */}
        <div className="xl:hidden px-[var(--page-gutter)] pb-3">
          <div className="ait-segmented w-full sm:w-auto" role="tablist">
            <button
              type="button"
              role="tab"
              aria-selected={mobilePane === 'edit'}
              onClick={() => setMobilePane('edit')}
              className="flex-1 sm:flex-none sm:px-5"
            >
              {tr('paneEditor')}
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={mobilePane === 'preview'}
              onClick={() => setMobilePane('preview')}
              className="flex-1 sm:flex-none sm:px-5"
            >
              {tr('panePreview')}
            </button>
          </div>
        </div>
      </header>

      <div className="flex-1 flex flex-col lg:flex-row lg:overflow-hidden min-h-0">
        {/* Strategy list: horizontal scroller on phones/tablets, column on desktop */}
        <aside className="flex-shrink-0 lg:w-60 border-b lg:border-b-0 lg:border-r border-border lg:overflow-y-auto bg-background/40 z-10">
          <div className="px-[var(--page-gutter)] lg:px-3 py-3">
            <div className="flex items-center justify-between mb-2">
              <span className="t-micro uppercase tracking-wider text-muted-foreground">
                {tr('strategies')}
              </span>
              <div className="flex items-center gap-0.5">
                <label
                  className="inline-flex items-center justify-center w-8 h-8 rounded-control text-muted-foreground hover:bg-surface-alt hover:text-foreground cursor-pointer"
                  title={tr('importStrategy')}
                  aria-label={tr('importStrategy')}
                >
                  <Upload className="w-4 h-4" />
                  <input
                    type="file"
                    accept=".json"
                    onChange={handleImportStrategy}
                    className="hidden"
                  />
                </label>
                <IconAction
                  label={tr('newStrategyTooltip')}
                  onClick={handleCreateStrategy}
                  size="md"
                >
                  <Plus className="w-4 h-4 text-primary" />
                </IconAction>
              </div>
            </div>
            {strategyList}
          </div>
        </aside>

        {/* Editor */}
        <main
          className={`flex-1 min-w-0 lg:overflow-y-auto xl:border-r border-border ${
            mobilePane === 'edit' ? 'block' : 'hidden xl:block'
          }`}
        >
          {selectedStrategy && editingConfig ? (
            <div className="px-[var(--page-gutter)] lg:px-5 py-4 lg:py-5 max-w-[960px] mx-auto">
              {/* Strategy name & actions */}
              <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-3 mb-4">
                <div className="flex-1 min-w-0">
                  <input
                    type="text"
                    value={selectedStrategy.name}
                    onChange={(e) => {
                      setSelectedStrategy({
                        ...selectedStrategy,
                        name: e.target.value,
                      })
                      setHasChanges(true)
                    }}
                    disabled={selectedStrategy.is_default}
                    className="t-title bg-transparent border-none outline-none w-full placeholder-muted-foreground"
                  />
                  <input
                    type="text"
                    value={selectedStrategy.description || ''}
                    onChange={(e) => {
                      setSelectedStrategy({
                        ...selectedStrategy,
                        description: e.target.value,
                      })
                      setHasChanges(true)
                    }}
                    disabled={selectedStrategy.is_default}
                    placeholder={tr('addDescription')}
                    className="t-caption bg-transparent border-none outline-none w-full text-muted-foreground placeholder-muted-foreground/60 mt-1"
                  />
                  {hasChanges && (
                    <span className="inline-flex items-center gap-1.5 t-micro text-primary mt-1">
                      <span className="w-1.5 h-1.5 rounded-full bg-primary" />
                      {tr('unsaved')}
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-2 flex-shrink-0">
                  {!selectedStrategy.is_active && (
                    <Button
                      variant="unstyled"
                      onClick={() =>
                        handleActivateStrategy(selectedStrategy.id)
                      }
                      className="inline-flex items-center gap-1.5 h-9 px-3.5 rounded-control t-caption font-semibold border border-profit/40 text-profit hover:bg-profit/10"
                    >
                      <Check className="w-3.5 h-3.5" />
                      {tr('activate')}
                    </Button>
                  )}
                  {!selectedStrategy.is_default && (
                    <Button
                      variant="unstyled"
                      onClick={handleSaveStrategy}
                      disabled={isSaving || !hasChanges}
                      className={`inline-flex items-center gap-1.5 h-9 px-4 rounded-control t-caption font-semibold disabled:opacity-50 ${
                        hasChanges
                          ? 'bg-primary text-primary-foreground hover:opacity-90'
                          : 'bg-surface-alt text-muted-foreground cursor-not-allowed'
                      }`}
                    >
                      <Save className="w-3.5 h-3.5" />
                      {isSaving ? tr('saving') : tr('save')}
                    </Button>
                  )}
                </div>
              </div>

              {currentStrategyType === 'ai_trading' && (
                <div className="mb-4">
                  <TokenEstimateBar
                    config={editingConfig}
                    language={language}
                    onTokenCountChange={setEstimatedTokens}
                  />
                </div>
              )}

              {/* Strategy type */}
              <section className="ait-card p-4 mb-3">
                <SectionHeader
                  title={tr('strategyType')}
                  icon={<Zap className="w-4 h-4" />}
                  className="mb-3"
                />
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
                  {(
                    [
                      {
                        type: 'ai_trading',
                        icon: Bot,
                        title: tr('aiTrading'),
                        desc: tr('aiTradingDesc'),
                      },
                      {
                        type: 'grid_trading',
                        icon: Activity,
                        title: tr('gridTrading'),
                        desc: tr('gridTradingDesc'),
                      },
                    ] as const
                  ).map(({ type, icon: Icon, title, desc }) => {
                    const selected =
                      type === 'ai_trading'
                        ? !editingConfig.strategy_type ||
                          editingConfig.strategy_type === 'ai_trading'
                        : editingConfig.strategy_type === 'grid_trading'
                    return (
                      <Button
                        key={type}
                        variant="unstyled"
                        onClick={() => handleStrategyTypeChange(type)}
                        disabled={selectedStrategy?.is_default}
                        aria-pressed={selected}
                        className={`flex flex-col items-start p-3 rounded-control border text-left transition-colors ${
                          selected
                            ? 'border-primary/60 bg-primary-dim'
                            : 'border-border hover:border-border-hover'
                        }`}
                      >
                        <div className="flex items-center gap-2 mb-0.5">
                          <Icon className="w-4 h-4 text-muted-foreground" />
                          <span className="t-label">{title}</span>
                        </div>
                        <p className="t-caption font-normal text-muted-foreground">
                          {desc}
                        </p>
                      </Button>
                    )
                  })}
                </div>
              </section>

              {/* Config sections */}
              <div className="space-y-3">
                {configSections.map(
                  ({ key, icon: Icon, title, summary, content }) => {
                    const open = expandedSections[key]
                    return (
                      <section key={key} className="ait-card overflow-hidden">
                        <button
                          type="button"
                          onClick={() => toggleSection(key)}
                          aria-expanded={open}
                          className="w-full flex items-center justify-between gap-3 px-4 py-3.5 text-left hover:bg-panel-hover"
                        >
                          <div className="flex items-start gap-3 min-w-0">
                            <span className="mt-0.5 inline-flex w-7 h-7 shrink-0 items-center justify-center rounded-control bg-surface-alt text-muted-foreground">
                              <Icon className="w-4 h-4" />
                            </span>
                            <div className="min-w-0">
                              <div className="t-headline">{title}</div>
                              <div className="t-caption font-normal text-muted-foreground truncate">
                                {summary}
                              </div>
                            </div>
                          </div>
                          <ChevronDown
                            className={`w-4 h-4 shrink-0 text-muted-foreground transition-transform duration-200 ${open ? 'rotate-180' : ''}`}
                          />
                        </button>
                        {open && (
                          <div className="px-3 sm:px-4 pb-4 pt-1 border-t border-border">
                            <Suspense
                              fallback={
                                <div className="h-24 mt-3 rounded-control skeleton" />
                              }
                            >
                              <div className="pt-3">{content}</div>
                            </Suspense>
                          </div>
                        )}
                      </section>
                    )
                  }
                )}
              </div>
            </div>
          ) : (
            <EmptyState
              icon={<Activity className="w-10 h-10" />}
              text={tr('selectOrCreate')}
            />
          )}
        </main>

        {/* Preview & AI test */}
        <aside
          className={`xl:w-[420px] flex-shrink-0 flex-col lg:overflow-hidden border-t xl:border-t-0 border-border ${
            mobilePane === 'preview'
              ? 'flex lg:flex-1 xl:flex-none'
              : 'hidden xl:flex'
          }`}
        >
          {previewPane}
        </aside>
      </div>
    </DeepVoidBackground>
  )
}

function IconAction({
  label,
  onClick,
  children,
  danger = false,
  disabled = false,
  size = 'sm',
}: {
  label: string
  onClick: () => void
  children: ReactNode
  danger?: boolean
  disabled?: boolean
  size?: 'sm' | 'md'
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      disabled={disabled}
      onClick={(e) => {
        e.stopPropagation()
        onClick()
      }}
      className={`inline-flex items-center justify-center rounded-control ${
        size === 'md' ? 'w-8 h-8' : 'w-7 h-7'
      } ${
        danger
          ? 'text-loss hover:bg-loss/10'
          : 'text-muted-foreground hover:bg-surface-alt hover:text-foreground'
      } disabled:opacity-40 disabled:cursor-not-allowed`}
    >
      {children}
    </button>
  )
}

function EmptyState({ icon, text }: { icon: ReactNode; text: string }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-14 text-muted-foreground">
      <span className="opacity-40">{icon}</span>
      <p className="t-caption text-center max-w-[260px]">{text}</p>
    </div>
  )
}

function ResultBlock({
  icon,
  title,
  text,
  maxHeight,
}: {
  icon: ReactNode
  title: string
  text: string
  maxHeight: number
}) {
  return (
    <div>
      <div className="flex items-center gap-1.5 mb-1.5 text-muted-foreground">
        {icon}
        <span className="t-caption font-semibold text-foreground">{title}</span>
      </div>
      <pre
        className="ait-card-inset p-3 text-[11px] leading-relaxed font-mono overflow-auto whitespace-pre-wrap break-words text-foreground"
        style={{ maxHeight }}
      >
        {text}
      </pre>
    </div>
  )
}

export default StrategyStudioPage
