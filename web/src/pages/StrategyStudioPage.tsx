import { lazy, Suspense, useState, useEffect, useCallback, useRef } from 'react'
import { useAuth } from '../contexts/AuthContext'
import { useLanguage } from '../contexts/LanguageContext'
import {
  Plus,
  Copy,
  Trash2,
  Check,
  ChevronDown,
  ChevronRight,
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

  return (
    <DeepVoidBackground className="h-[calc(100vh-64px)] flex flex-col bg-background relative overflow-hidden">
      {/* Header */}
      {/* Header */}
      <div className="flex-shrink-0 px-4 py-3 border-b border-primary/20 bg-background/60 backdrop-blur-md z-10">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-gradient-to-br from-primary to-primary/70">
              <Sparkles className="w-5 h-5 text-black" />
            </div>
            <div>
              <h1 className="text-lg font-bold text-foreground">
                {tr('title')}
              </h1>
              <p className="text-xs text-muted-foreground">{tr('subtitle')}</p>
            </div>
          </div>
          {error && (
            <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs bg-loss/10 text-loss">
              {error}
              <button
                onClick={() => setError(null)}
                className="hover:underline"
              >
                ×
              </button>
            </div>
          )}
        </div>
      </div>

      {/* Main Content - Three Columns */}
      <div className="flex-1 flex overflow-hidden">
        {/* Left Column - Strategy List */}
        <div className="w-48 flex-shrink-0 border-r border-primary/20 overflow-y-auto bg-background/30 backdrop-blur-sm z-10">
          <div className="p-2">
            <div className="flex items-center justify-between mb-2 px-2">
              <span className="text-xs font-medium text-muted-foreground">
                {tr('strategies')}
              </span>
              <div className="flex items-center gap-1">
                {/* Import button with hidden file input */}
                <label
                  className="p-1 rounded hover:bg-white/10 transition-colors cursor-pointer text-muted-foreground hover:text-foreground"
                  title={tr('importStrategy')}
                >
                  <Upload className="w-4 h-4" />
                  <input
                    type="file"
                    accept=".json"
                    onChange={handleImportStrategy}
                    className="hidden"
                  />
                </label>
                <button
                  onClick={handleCreateStrategy}
                  className="p-1 rounded hover:bg-white/10 transition-colors text-primary"
                  title={tr('newStrategyTooltip')}
                >
                  <Plus className="w-4 h-4" />
                </button>
              </div>
            </div>
            <div className="space-y-2">
              {strategies.map((strategy) => (
                <div
                  key={strategy.id}
                  onClick={() => {
                    setSelectedStrategy(strategy)
                    setEditingConfig(strategy.config)
                    setHasChanges(false)
                    setPromptPreview(null)
                    setAiTestResult(null)
                  }}
                  className={`group px-2 py-2 rounded-lg cursor-pointer transition-all ${
                    selectedStrategy?.id === strategy.id
                      ? 'ring-1 ring-primary/50 bg-primary-dim'
                      : 'hover:bg-surface/60 ring-1 ring-white/10 hover:ring-primary/20 bg-transparent'
                  }`}
                >
                  <div className="flex items-start justify-between">
                    <span
                      className={`line-clamp-2 text-foreground ${language === 'zh' ? 'text-sm' : 'text-xs'}`}
                    >
                      {strategy.name}
                    </span>
                    <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button
                        onClick={(e) => {
                          e.stopPropagation()
                          handleExportStrategy(strategy)
                        }}
                        className="p-1 rounded hover:bg-white/10 text-muted-foreground hover:text-foreground"
                        title={tr('export')}
                      >
                        <Download className="w-3 h-3" />
                      </button>
                      {!strategy.is_default && (
                        <>
                          <button
                            onClick={(e) => {
                              e.stopPropagation()
                              handleDuplicateStrategy(strategy.id)
                            }}
                            className="p-1 rounded hover:bg-white/10 text-muted-foreground hover:text-foreground"
                            title={tr('duplicate')}
                          >
                            <Copy className="w-3 h-3" />
                          </button>
                          <button
                            onClick={(e) => {
                              e.stopPropagation()
                              handleDeleteStrategy(strategy.id)
                            }}
                            disabled={strategy.is_active}
                            className="p-1 rounded hover:bg-loss/20 text-loss disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:bg-transparent"
                            title={
                              strategy.is_active
                                ? tr('cannotDeleteActiveStrategy')
                                : tr('deleteTooltip')
                            }
                          >
                            <Trash2 className="w-3 h-3" />
                          </button>
                        </>
                      )}
                    </div>
                  </div>
                  <div className="flex items-center gap-1 mt-1 flex-wrap">
                    {strategy.is_active && (
                      <span className="px-1.5 py-0.5 text-[10px] rounded bg-profit/15 text-profit">
                        {tr('active')}
                      </span>
                    )}
                    {strategy.is_default && (
                      <span className="px-1.5 py-0.5 text-[10px] rounded bg-primary/15 text-primary">
                        {tr('default')}
                      </span>
                    )}
                    {strategy.is_public && (
                      <span className="px-1.5 py-0.5 text-[10px] rounded flex items-center gap-0.5 bg-accent/15 text-accent">
                        <Globe className="w-2.5 h-2.5" />
                        {tr('public')}
                      </span>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Middle Column - Config Editor */}
        <div className="flex-1 min-w-0 overflow-y-auto border-r border-primary/20">
          {selectedStrategy && editingConfig ? (
            <div className="p-4">
              {/* Strategy Name & Actions */}
              <div className="flex items-center justify-between mb-4">
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
                    className="text-lg font-bold bg-transparent border-none outline-none w-full text-foreground placeholder-muted-foreground"
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
                    className="text-xs bg-transparent border-none outline-none w-full text-muted-foreground placeholder-muted-foreground/50 mt-1"
                  />
                  {hasChanges && (
                    <span className="text-xs text-primary">
                      ● {tr('unsaved')}
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-2 flex-shrink-0">
                  {!selectedStrategy.is_active && (
                    <button
                      onClick={() =>
                        handleActivateStrategy(selectedStrategy.id)
                      }
                      className="flex items-center gap-1 px-3 py-1.5 rounded-lg text-xs transition-colors bg-profit/10 border border-profit/30 text-profit hover:bg-profit/20"
                    >
                      <Check className="w-3 h-3" />
                      {tr('activate')}
                    </button>
                  )}
                  {!selectedStrategy.is_default && (
                    <button
                      onClick={handleSaveStrategy}
                      disabled={isSaving || !hasChanges}
                      className={`flex items-center gap-1 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors disabled:opacity-50
                        ${hasChanges ? 'bg-primary text-primary-foreground hover:bg-primary/90' : 'bg-surface text-muted-foreground cursor-not-allowed'}`}
                    >
                      <Save className="w-3 h-3" />
                      {isSaving ? tr('saving') : tr('save')}
                    </button>
                  )}
                </div>
              </div>

              {/* Token Estimate Bar */}
              {currentStrategyType === 'ai_trading' && (
                <div className="mb-4">
                  <TokenEstimateBar
                    config={editingConfig}
                    language={language}
                    onTokenCountChange={setEstimatedTokens}
                  />
                </div>
              )}

              {/* Strategy Type Selector */}
              {editingConfig && (
                <div className="mb-4 p-4 rounded-lg bg-surface border border-primary/20">
                  <div className="flex items-center gap-2 mb-3">
                    <Zap
                      className="w-4 h-4"
                      style={{ color: 'var(--color-primary)' }}
                    />
                    <span className="text-sm font-medium text-foreground">
                      {tr('strategyType')}
                    </span>
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <button
                      onClick={() => handleStrategyTypeChange('ai_trading')}
                      disabled={selectedStrategy?.is_default}
                      className={`p-3 rounded-lg border transition-all ${
                        !editingConfig.strategy_type ||
                        editingConfig.strategy_type === 'ai_trading'
                          ? 'border-primary bg-primary-dim'
                          : 'border-ait-border hover:border-primary/50'
                      }`}
                    >
                      <div className="flex items-center gap-2 mb-1">
                        <Bot
                          className="w-4 h-4"
                          style={{ color: 'var(--color-primary)' }}
                        />
                        <span className="text-sm font-medium text-foreground">
                          {tr('aiTrading')}
                        </span>
                      </div>
                      <p className="text-xs text-muted-foreground text-left">
                        {tr('aiTradingDesc')}
                      </p>
                    </button>
                    <button
                      onClick={() => handleStrategyTypeChange('grid_trading')}
                      disabled={selectedStrategy?.is_default}
                      className={`p-3 rounded-lg border transition-all ${
                        editingConfig.strategy_type === 'grid_trading'
                          ? 'border-primary bg-primary-dim'
                          : 'border-ait-border hover:border-primary/50'
                      }`}
                    >
                      <div className="flex items-center gap-2 mb-1">
                        <Activity
                          className="w-4 h-4"
                          style={{ color: 'var(--color-profit)' }}
                        />
                        <span className="text-sm font-medium text-foreground">
                          {tr('gridTrading')}
                        </span>
                      </div>
                      <p className="text-xs text-muted-foreground text-left">
                        {tr('gridTradingDesc')}
                      </p>
                    </button>
                  </div>
                </div>
              )}

              {/* Config Sections */}
              <div className="space-y-2">
                {configSections.map(
                  ({ key, icon: Icon, color, title, content }) => (
                    <div
                      key={key}
                      className="rounded-lg overflow-hidden bg-surface border border-primary/20"
                    >
                      <button
                        onClick={() => toggleSection(key)}
                        className="w-full flex items-center justify-between px-3 py-2.5 hover:bg-white/5 transition-colors"
                      >
                        <div className="flex items-center gap-2">
                          <Icon className="w-4 h-4" style={{ color }} />
                          <span className="text-sm font-medium text-foreground">
                            {title}
                          </span>
                        </div>
                        {expandedSections[key] ? (
                          <ChevronDown className="w-4 h-4 text-muted-foreground" />
                        ) : (
                          <ChevronRight className="w-4 h-4 text-muted-foreground" />
                        )}
                      </button>
                      {expandedSections[key] && (
                        <div className="px-3 pb-3">
                          <Suspense
                            fallback={
                              <div className="h-24 rounded bg-white/5 animate-pulse" />
                            }
                          >
                            {content}
                          </Suspense>
                        </div>
                      )}
                    </div>
                  )
                )}
              </div>
            </div>
          ) : (
            <div className="flex items-center justify-center h-full">
              <div className="text-center">
                <Activity className="w-12 h-12 mx-auto mb-2 opacity-30 text-muted-foreground" />
                <p className="text-sm text-muted-foreground">
                  {tr('selectOrCreate')}
                </p>
              </div>
            </div>
          )}
        </div>

        {/* Right Column - Prompt Preview & AI Test */}
        <div className="w-[420px] flex-shrink-0 flex flex-col overflow-hidden">
          {/* Tabs */}
          <div className="flex-shrink-0 flex border-b border-primary/20">
            <button
              onClick={() => setActiveRightTab('prompt')}
              className={`flex-1 flex items-center justify-center gap-2 px-4 py-2.5 text-sm font-medium transition-colors ${
                activeRightTab === 'prompt'
                  ? 'border-b-2 border-accent text-accent'
                  : 'opacity-60 hover:opacity-100 text-muted-foreground'
              }`}
            >
              <Eye className="w-4 h-4" />
              {tr('promptPreview')}
            </button>
            <button
              onClick={() => setActiveRightTab('test')}
              className={`flex-1 flex items-center justify-center gap-2 px-4 py-2.5 text-sm font-medium transition-colors ${
                activeRightTab === 'test'
                  ? 'border-b-2 border-profit text-profit'
                  : 'opacity-60 hover:opacity-100 text-muted-foreground'
              }`}
            >
              <Play className="w-4 h-4" />
              {tr('aiTestRun')}
            </button>
          </div>

          {/* Tab Content */}
          <div className="flex-1 overflow-y-auto">
            {activeRightTab === 'prompt' ? (
              /* Prompt Preview Tab */
              <div className="p-3 space-y-3">
                {/* Controls */}
                <div className="flex items-center gap-2 flex-wrap">
                  <select
                    value={selectedVariant}
                    onChange={(e) => setSelectedVariant(e.target.value)}
                    className="px-2 py-1.5 rounded text-xs bg-background border border-primary/20 text-foreground outline-none focus:border-primary"
                  >
                    <option value="balanced">{tr('balanced')}</option>
                    <option value="aggressive">{tr('aggressive')}</option>
                    <option value="conservative">{tr('conservative')}</option>
                  </select>
                  <button
                    onClick={fetchPromptPreview}
                    disabled={isLoadingPrompt || !editingConfig}
                    className="flex items-center gap-1.5 px-3 py-1.5 rounded text-xs font-medium transition-colors disabled:opacity-50 bg-accent hover:bg-accent/80 text-foreground"
                  >
                    {isLoadingPrompt ? (
                      <Loader2 className="w-3 h-3 animate-spin" />
                    ) : (
                      <RefreshCw className="w-3 h-3" />
                    )}
                    {promptPreview ? tr('refreshPrompt') : tr('loadPrompt')}
                  </button>
                </div>

                {promptPreview ? (
                  <>
                    {/* Config Summary */}
                    <div className="p-2 rounded-lg bg-background border border-primary/20">
                      <div className="flex items-center gap-1.5 mb-2">
                        <Code className="w-3 h-3 text-accent" />
                        <span className="text-xs font-medium text-accent">
                          Config
                        </span>
                      </div>
                      <div className="grid grid-cols-3 gap-2 text-xs">
                        {Object.entries(promptPreview.config_summary || {}).map(
                          ([key, value]) => (
                            <div key={key}>
                              <div className="text-muted-foreground">
                                {key.replace(/_/g, ' ')}
                              </div>
                              <div className="text-foreground">
                                {String(value)}
                              </div>
                            </div>
                          )
                        )}
                      </div>
                    </div>

                    {/* System Prompt */}
                    <div>
                      <div className="flex items-center justify-between mb-1.5">
                        <div className="flex items-center gap-1.5">
                          <FileText className="w-3 h-3 text-accent" />
                          <span className="text-xs font-medium text-foreground">
                            {tr('systemPrompt')}
                          </span>
                        </div>
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-surface text-muted-foreground">
                          {promptPreview.system_prompt.length.toLocaleString()}{' '}
                          chars
                        </span>
                      </div>
                      <pre
                        className="p-2 rounded-lg text-[11px] font-mono overflow-auto bg-background border border-primary/20 text-foreground"
                        style={{ maxHeight: '400px' }}
                      >
                        {promptPreview.system_prompt}
                      </pre>
                    </div>
                  </>
                ) : (
                  <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                    <Eye className="w-10 h-10 mb-2 opacity-30" />
                    <p className="text-sm">{tr('generatePromptPreview')}</p>
                  </div>
                )}
              </div>
            ) : (
              /* AI Test Tab */
              <div className="p-3 space-y-3">
                {/* Controls */}
                <div className="space-y-2">
                  <div className="flex items-center gap-2">
                    <Bot className="w-4 h-4 text-profit" />
                    <span className="text-xs font-medium text-foreground">
                      {tr('selectModel')}
                    </span>
                  </div>
                  {aiModels.length > 0 ? (
                    <select
                      value={selectedModelId}
                      onChange={(e) => setSelectedModelId(e.target.value)}
                      className="w-full px-3 py-2 rounded-lg text-sm bg-background border border-primary/20 text-foreground"
                    >
                      {aiModels.map((model) => (
                        <option key={model.id} value={model.id}>
                          {model.name} ({model.provider})
                        </option>
                      ))}
                    </select>
                  ) : (
                    <div className="px-3 py-2 rounded-lg text-sm bg-loss/10 text-loss">
                      {tr('noModel')}
                    </div>
                  )}

                  <div className="flex items-center gap-2">
                    <select
                      value={selectedVariant}
                      onChange={(e) => setSelectedVariant(e.target.value)}
                      className="px-2 py-1.5 rounded text-xs bg-background border border-primary/20 text-foreground"
                    >
                      <option value="balanced">{tr('balanced')}</option>
                      <option value="aggressive">{tr('aggressive')}</option>
                      <option value="conservative">{tr('conservative')}</option>
                    </select>
                    <button
                      onClick={runAiTest}
                      disabled={
                        isRunningAiTest || !editingConfig || !selectedModelId
                      }
                      className="flex-1 flex items-center justify-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition-all disabled:opacity-50 text-foreground shadow-lg bg-gradient-to-br from-profit to-profit/80"
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
                    </button>
                  </div>
                  <p className="text-[10px] text-muted-foreground">
                    {tr('testNote')}
                  </p>
                </div>

                {/* Test Results */}
                {aiTestResult ? (
                  <div className="space-y-3">
                    {aiTestResult.error ? (
                      <div className="p-3 rounded-lg bg-loss/10 border border-loss/30">
                        <p className="text-sm text-loss">
                          {aiTestResult.error}
                        </p>
                      </div>
                    ) : (
                      <>
                        {aiTestResult.duration_ms && (
                          <div className="flex items-center gap-2">
                            <Clock className="w-3 h-3 text-muted-foreground" />
                            <span className="text-xs text-muted-foreground">
                              {tr('duration')}:{' '}
                              {(aiTestResult.duration_ms / 1000).toFixed(2)}s
                            </span>
                          </div>
                        )}

                        {/* User Prompt Input */}
                        {aiTestResult.user_prompt && (
                          <div>
                            <div className="flex items-center gap-1.5 mb-1.5">
                              <Terminal className="w-3 h-3 text-accent" />
                              <span className="text-xs font-medium text-foreground">
                                {tr('userPrompt')} (Input)
                              </span>
                            </div>
                            <pre
                              className="p-2 rounded-lg text-[10px] font-mono overflow-auto bg-background border border-primary/20 text-foreground"
                              style={{ maxHeight: '200px' }}
                            >
                              {aiTestResult.user_prompt}
                            </pre>
                          </div>
                        )}

                        {/* AI Reasoning */}
                        {aiTestResult.reasoning && (
                          <div>
                            <div className="flex items-center gap-1.5 mb-1.5">
                              <Sparkles className="w-3 h-3 text-primary" />
                              <span className="text-xs font-medium text-foreground">
                                {tr('reasoning')}
                              </span>
                            </div>
                            <pre
                              className="p-2 rounded-lg text-[10px] font-mono overflow-auto whitespace-pre-wrap bg-background border border-primary/30 text-foreground"
                              style={{ maxHeight: '200px' }}
                            >
                              {aiTestResult.reasoning}
                            </pre>
                          </div>
                        )}

                        {/* AI Decisions */}
                        {aiTestResult.decisions &&
                          aiTestResult.decisions.length > 0 && (
                            <div>
                              <div className="flex items-center gap-1.5 mb-1.5">
                                <Activity className="w-3 h-3 text-profit" />
                                <span className="text-xs font-medium text-foreground">
                                  {tr('decisions')}
                                </span>
                              </div>
                              <pre
                                className="p-2 rounded-lg text-[10px] font-mono overflow-auto bg-background border border-profit/30 text-foreground"
                                style={{ maxHeight: '200px' }}
                              >
                                {JSON.stringify(
                                  aiTestResult.decisions,
                                  null,
                                  2
                                )}
                              </pre>
                            </div>
                          )}

                        {/* Raw AI Response */}
                        {aiTestResult.ai_response && (
                          <div>
                            <div className="flex items-center gap-1.5 mb-1.5">
                              <FileText className="w-3 h-3 text-muted-foreground" />
                              <span className="text-xs font-medium text-foreground">
                                {tr('aiOutput')} (Raw)
                              </span>
                            </div>
                            <pre
                              className="p-2 rounded-lg text-[10px] font-mono overflow-auto whitespace-pre-wrap bg-background border border-primary/20 text-foreground"
                              style={{ maxHeight: '300px' }}
                            >
                              {aiTestResult.ai_response}
                            </pre>
                          </div>
                        )}
                      </>
                    )}
                  </div>
                ) : (
                  <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                    <Play className="w-10 h-10 mb-2 opacity-30" />
                    <p className="text-sm">{tr('runAiTestHint')}</p>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </DeepVoidBackground>
  )
}

export default StrategyStudioPage
