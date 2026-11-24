import { lazy, Suspense, useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import useSWR from 'swr'
import { api } from '../../lib/api'
import type {
  TraderInfo,
  CreateTraderRequest,
  AIModel,
  Exchange,
} from '../../types'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { useAuth } from '../../contexts/AuthContext'
import { DeepVoidBackground } from '../common/DeepVoidBackground'
import { ConfigStatusGrid } from './ConfigStatusGrid'
import { TradersList } from './TradersList'
import { Bot, Plus, MessageCircle } from 'lucide-react'
import { confirmToast } from '../../lib/notify'
import { toast } from 'sonner'
import { Button } from '../ui/Button'

const TraderConfigModal = lazy(() =>
  import('./TraderConfigModal').then((m) => ({ default: m.TraderConfigModal }))
)
const ExchangeConfigModal = lazy(() =>
  import('./ExchangeConfigModal').then((m) => ({
    default: m.ExchangeConfigModal,
  }))
)
const TelegramConfigModal = lazy(() =>
  import('./TelegramConfigModal').then((m) => ({
    default: m.TelegramConfigModal,
  }))
)
const ModelConfigModal = lazy(() =>
  import('./ModelConfigModal').then((m) => ({ default: m.ModelConfigModal }))
)

interface AITradersPageProps {
  onTraderSelect?: (traderId: string) => void
}

export function AITradersPage({ onTraderSelect }: AITradersPageProps) {
  const { language } = useLanguage()
  const { user, token } = useAuth()
  const navigate = useNavigate()
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [showEditModal, setShowEditModal] = useState(false)
  const [showModelModal, setShowModelModal] = useState(false)
  const [showExchangeModal, setShowExchangeModal] = useState(false)
  const [showTelegramModal, setShowTelegramModal] = useState(false)
  const [editingModel, setEditingModel] = useState<string | null>(null)
  const [editingExchange, setEditingExchange] = useState<string | null>(null)
  const [editingTrader, setEditingTrader] = useState<any>(null)
  const [allModels, setAllModels] = useState<AIModel[]>([])
  const [allExchanges, setAllExchanges] = useState<Exchange[]>([])
  const [supportedModels, setSupportedModels] = useState<AIModel[]>([])
  const [visibleTraderAddresses, setVisibleTraderAddresses] = useState<
    Set<string>
  >(new Set())
  const [visibleExchangeAddresses, setVisibleExchangeAddresses] = useState<
    Set<string>
  >(new Set())
  const [copiedId, setCopiedId] = useState<string | null>(null)

  const loadConfigs = async () => {
    if (!user || !token) {
      const models = await api.getSupportedModels()
      setSupportedModels(models)
      return
    }

    const [modelConfigs, exchangeConfigs, models] = await Promise.all([
      api.getModelConfigs(),
      api.getExchangeConfigs(),
      api.getSupportedModels(),
    ])
    setAllModels(modelConfigs)
    setAllExchanges(exchangeConfigs)
    setSupportedModels(models)
  }

  // Toggle wallet address visibility for a trader
  const toggleTraderAddressVisibility = (traderId: string) => {
    setVisibleTraderAddresses((prev) => {
      const next = new Set(prev)
      if (next.has(traderId)) {
        next.delete(traderId)
      } else {
        next.add(traderId)
      }
      return next
    })
  }

  // Toggle wallet address visibility for an exchange
  const toggleExchangeAddressVisibility = (exchangeId: string) => {
    setVisibleExchangeAddresses((prev) => {
      const next = new Set(prev)
      if (next.has(exchangeId)) {
        next.delete(exchangeId)
      } else {
        next.add(exchangeId)
      }
      return next
    })
  }

  // Copy wallet address to clipboard
  const handleCopyAddress = async (id: string, address: string) => {
    try {
      await navigator.clipboard.writeText(address)
      setCopiedId(id)
      setTimeout(() => setCopiedId(null), 2000)
    } catch (err) {
      console.error('Failed to copy address:', err)
    }
  }

  const {
    data: traders,
    mutate: mutateTraders,
    isLoading: isTradersLoading,
  } = useSWR<TraderInfo[]>(user && token ? 'traders' : null, api.getTraders, {
    refreshInterval: 15000,
    revalidateOnFocus: false,
  })

  useEffect(() => {
    loadConfigs().catch((error) => {
      console.error('Failed to load configs:', error)
    })
  }, [user, token])

  useEffect(() => {
    const handleRefresh = () => {
      loadConfigs().catch((error) => {
        console.error('Failed to refresh configs:', error)
      })
    }
    window.addEventListener('agent-config-refresh', handleRefresh)
    return () =>
      window.removeEventListener('agent-config-refresh', handleRefresh)
  }, [user, token])

  const configuredModels =
    allModels?.filter((m) => {
      return m.enabled || (m.customApiUrl && m.customApiUrl.trim() !== '')
    }) || []

  const configuredExchanges =
    allExchanges?.filter((e) => {
      if (e.id === 'aster') {
        return e.asterUser && e.asterUser.trim() !== ''
      }
      if (e.id === 'hyperliquid') {
        return e.hyperliquidWalletAddr && e.hyperliquidWalletAddr.trim() !== ''
      }
      return e.enabled
    }) || []

  const enabledModels = allModels?.filter((m) => m.enabled) || []
  const enabledExchanges =
    allExchanges?.filter((e) => {
      if (!e.enabled) return false
      if (e.id === 'aster') {
        return (
          e.asterUser &&
          e.asterUser.trim() !== '' &&
          e.asterSigner &&
          e.asterSigner.trim() !== ''
        )
      }
      if (e.id === 'hyperliquid') {
        return e.hyperliquidWalletAddr && e.hyperliquidWalletAddr.trim() !== ''
      }
      return true
    }) || []

  const isModelInUse = (modelId: string) => {
    return traders?.some((tr) => tr.ai_model === modelId && tr.is_running)
  }

  const getModelUsageInfo = (modelId: string) => {
    const usingTraders = traders?.filter((tr) => tr.ai_model === modelId) || []
    const runningCount = usingTraders.filter((tr) => tr.is_running).length
    const totalCount = usingTraders.length
    return { runningCount, totalCount, usingTraders }
  }

  const isExchangeInUse = (exchangeId: string) => {
    return traders?.some((tr) => tr.exchange_id === exchangeId && tr.is_running)
  }

  const getExchangeUsageInfo = (exchangeId: string) => {
    const usingTraders =
      traders?.filter((tr) => tr.exchange_id === exchangeId) || []
    const runningCount = usingTraders.filter((tr) => tr.is_running).length
    const totalCount = usingTraders.length
    return { runningCount, totalCount, usingTraders }
  }

  const isModelUsedByAnyTrader = (modelId: string) => {
    return traders?.some((tr) => tr.ai_model === modelId) || false
  }

  const isExchangeUsedByAnyTrader = (exchangeId: string) => {
    return traders?.some((tr) => tr.exchange_id === exchangeId) || false
  }

  const getTradersUsingModel = (modelId: string) => {
    return traders?.filter((tr) => tr.ai_model === modelId) || []
  }

  const getTradersUsingExchange = (exchangeId: string) => {
    return traders?.filter((tr) => tr.exchange_id === exchangeId) || []
  }

  const handleCreateTrader = async (data: CreateTraderRequest) => {
    try {
      const model = allModels?.find((m) => m.id === data.ai_model_id)
      const exchange = allExchanges?.find((e) => e.id === data.exchange_id)

      if (!model?.enabled) {
        toast.error(t('modelNotConfigured', language))
        return
      }

      if (!exchange?.enabled) {
        toast.error(t('exchangeNotConfigured', language))
        return
      }

      await api.createTrader(data)
      toast.success(t('aiTradersToast.created', language))
      setShowCreateModal(false)
      await mutateTraders()
    } catch (error) {
      console.error('Failed to create trader:', error)
      toast.error(t('createTraderFailed', language))
    }
  }

  const handleEditTrader = async (traderId: string) => {
    try {
      const traderConfig = await api.getTraderConfig(traderId)
      setEditingTrader(traderConfig)
      setShowEditModal(true)
    } catch (error) {
      console.error('Failed to fetch trader config:', error)
      toast.error(t('getTraderConfigFailed', language))
    }
  }

  const handleSaveEditTrader = async (data: CreateTraderRequest) => {
    if (!editingTrader) return

    try {
      const model = enabledModels?.find((m) => m.id === data.ai_model_id)
      const exchange = enabledExchanges?.find((e) => e.id === data.exchange_id)

      if (!model) {
        toast.error(t('modelConfigNotExist', language))
        return
      }

      if (!exchange) {
        toast.error(t('exchangeConfigNotExist', language))
        return
      }

      const request = {
        name: data.name,
        ai_model_id: data.ai_model_id,
        exchange_id: data.exchange_id,
        strategy_id: data.strategy_id,
        initial_balance: data.initial_balance,
        scan_interval_minutes: data.scan_interval_minutes,
        is_cross_margin: data.is_cross_margin,
        show_in_competition: data.show_in_competition,
      }

      await api.updateTrader(editingTrader.trader_id, request)
      toast.success(t('aiTradersToast.saved', language))
      setShowEditModal(false)
      setEditingTrader(null)
      await mutateTraders()
    } catch (error) {
      console.error('Failed to update trader:', error)
      toast.error(t('updateTraderFailed', language))
    }
  }

  const handleDeleteTrader = async (traderId: string) => {
    {
      const ok = await confirmToast(t('confirmDeleteTrader', language))
      if (!ok) return
    }

    try {
      await api.deleteTrader(traderId)
      toast.success(t('aiTradersToast.deleted', language))

      await mutateTraders()
    } catch (error) {
      console.error('Failed to delete trader:', error)
      toast.error(t('deleteTraderFailed', language))
    }
  }

  const handleToggleTrader = async (traderId: string, running: boolean) => {
    try {
      if (running) {
        await api.stopTrader(traderId)
        toast.success(t('aiTradersToast.stopped', language))
      } else {
        await api.startTrader(traderId)
        toast.success(t('aiTradersToast.started', language))
      }

      await mutateTraders()
    } catch (error) {
      console.error('Failed to toggle trader:', error)
      toast.error(t('operationFailed', language))
    }
  }

  const handleToggleCompetition = async (
    traderId: string,
    currentShowInCompetition: boolean
  ) => {
    try {
      const newValue = !currentShowInCompetition
      await api.toggleCompetition(traderId, newValue)
      toast.success(
        newValue
          ? t('aiTradersToast.showInCompetition', language)
          : t('aiTradersToast.hideInCompetition', language)
      )

      await mutateTraders()
    } catch (error) {
      console.error('Failed to toggle competition visibility:', error)
      toast.error(t('operationFailed', language))
    }
  }

  const handleModelClick = (modelId: string) => {
    if (!isModelInUse(modelId)) {
      setEditingModel(modelId)
      setShowModelModal(true)
    }
  }

  const handleExchangeClick = (exchangeId: string) => {
    if (!isExchangeInUse(exchangeId)) {
      setEditingExchange(exchangeId)
      setShowExchangeModal(true)
    }
  }

  const handleDeleteConfig = async <T extends { id: string }>(config: {
    id: string
    type: 'model' | 'exchange'
    checkInUse: (id: string) => boolean
    getUsingTraders: (id: string) => any[]
    cannotDeleteKey: string
    confirmDeleteKey: string
    allItems: T[] | undefined
    clearFields: (item: T) => T
    buildRequest: (items: T[]) => any
    updateApi: (request: any) => Promise<void>
    refreshApi: () => Promise<T[]>
    setItems: (items: T[]) => void
    closeModal: () => void
    errorKey: string
  }) => {
    if (config.checkInUse(config.id)) {
      const usingTraders = config.getUsingTraders(config.id)
      const traderNames = usingTraders.map((tr) => tr.trader_name).join(', ')
      toast.error(
        `${t(config.cannotDeleteKey, language)} · ${t('tradersUsing', language)}: ${traderNames} · ${t('pleaseDeleteTradersFirst', language)}`
      )
      return
    }

    {
      const ok = await confirmToast(t(config.confirmDeleteKey, language))
      if (!ok) return
    }

    try {
      const updatedItems =
        config.allItems?.map((item) =>
          item.id === config.id ? config.clearFields(item) : item
        ) || []

      const request = config.buildRequest(updatedItems)
      await config.updateApi(request)
      toast.success(t('aiTradersToast.configUpdated', language))

      const refreshedItems = await config.refreshApi()
      config.setItems(refreshedItems)

      config.closeModal()
    } catch (error) {
      console.error(`Failed to delete ${config.type} config:`, error)
      toast.error(t(config.errorKey, language))
    }
  }

  const handleDeleteModelConfig = async (modelId: string) => {
    await handleDeleteConfig({
      id: modelId,
      type: 'model',
      checkInUse: isModelUsedByAnyTrader,
      getUsingTraders: getTradersUsingModel,
      cannotDeleteKey: 'cannotDeleteModelInUse',
      confirmDeleteKey: 'confirmDeleteModel',
      allItems: allModels,
      clearFields: (m) => ({
        ...m,
        apiKey: '',
        customApiUrl: '',
        customModelName: '',
        enabled: false,
      }),
      buildRequest: (models) => ({
        models: Object.fromEntries(
          models.map((model) => [
            model.provider,
            {
              enabled: model.enabled,
              api_key: model.apiKey || '',
              custom_api_url: model.customApiUrl || '',
              custom_model_name: model.customModelName || '',
            },
          ])
        ),
      }),
      updateApi: api.updateModelConfigs,
      refreshApi: api.getModelConfigs,
      setItems: (items) => {
        setAllModels([...items])
      },
      closeModal: () => {
        setShowModelModal(false)
        setEditingModel(null)
      },
      errorKey: 'deleteConfigFailed',
    })
  }

  const handleSaveModelConfig = async (
    modelId: string,
    apiKey: string,
    customApiUrl?: string,
    customModelName?: string
  ) => {
    try {
      const existingModel = allModels?.find((m) => m.id === modelId)
      let updatedModels

      const modelToUpdate =
        existingModel || supportedModels?.find((m) => m.id === modelId)
      if (!modelToUpdate) {
        toast.error(t('modelNotExist', language))
        return
      }

      if (existingModel) {
        updatedModels =
          allModels?.map((m) =>
            m.id === modelId
              ? {
                  ...m,
                  apiKey,
                  customApiUrl: customApiUrl || '',
                  customModelName: customModelName || '',
                  enabled: true,
                }
              : m
          ) || []
      } else {
        const newModel = {
          ...modelToUpdate,
          apiKey,
          customApiUrl: customApiUrl || '',
          customModelName: customModelName || '',
          enabled: true,
        }
        updatedModels = [...(allModels || []), newModel]
      }

      const request = {
        models: Object.fromEntries(
          updatedModels.map((model) => [
            model.provider,
            {
              enabled: model.enabled,
              api_key: model.apiKey || '',
              custom_api_url: model.customApiUrl || '',
              custom_model_name: model.customModelName || '',
            },
          ])
        ),
      }

      await api.updateModelConfigs(request)
      toast.success(t('aiTradersToast.modelConfigUpdated', language))

      const refreshedModels = await api.getModelConfigs()
      setAllModels(refreshedModels)

      setShowModelModal(false)
      setEditingModel(null)
    } catch (error) {
      console.error('Failed to save model config:', error)
      toast.error(t('saveConfigFailed', language))
    }
  }

  const handleDeleteExchangeConfig = async (exchangeId: string) => {
    if (isExchangeUsedByAnyTrader(exchangeId)) {
      const tradersUsing = getTradersUsingExchange(exchangeId)
      toast.error(
        `${t('cannotDeleteExchangeInUse', language)}: ${tradersUsing.join(', ')}`
      )
      return
    }

    const ok = await confirmToast(t('confirmDeleteExchange', language))
    if (!ok) return

    try {
      await api.deleteExchange(exchangeId)
      toast.success(t('aiTradersToast.exchangeDeleted', language))

      const refreshedExchanges = await api.getExchangeConfigs()
      setAllExchanges(refreshedExchanges)

      setShowExchangeModal(false)
      setEditingExchange(null)
    } catch (error) {
      console.error('Failed to delete exchange config:', error)
      toast.error(t('deleteExchangeConfigFailed', language))
    }
  }

  const handleSaveExchangeConfig = async (
    exchangeId: string | null,
    exchangeType: string,
    accountName: string,
    apiKey: string,
    secretKey?: string,
    passphrase?: string,
    testnet?: boolean,
    hyperliquidWalletAddr?: string,
    asterUser?: string,
    asterSigner?: string,
    asterPrivateKey?: string,
    lighterWalletAddr?: string,
    lighterPrivateKey?: string,
    lighterApiKeyPrivateKey?: string,
    lighterApiKeyIndex?: number,
    proxyURL?: string
  ) => {
    try {
      if (exchangeId) {
        const existingExchange = allExchanges?.find((e) => e.id === exchangeId)
        if (!existingExchange) {
          toast.error(t('exchangeNotExist', language))
          return
        }

        const request = {
          exchanges: {
            [exchangeId]: {
              enabled: true,
              api_key: apiKey || '',
              secret_key: secretKey || '',
              passphrase: passphrase || '',
              testnet: testnet || false,
              hyperliquid_wallet_addr: hyperliquidWalletAddr || '',
              aster_user: asterUser || '',
              aster_signer: asterSigner || '',
              aster_private_key: asterPrivateKey || '',
              lighter_wallet_addr: lighterWalletAddr || '',
              lighter_private_key: lighterPrivateKey || '',
              lighter_api_key_private_key: lighterApiKeyPrivateKey || '',
              lighter_api_key_index: lighterApiKeyIndex || 0,
              proxy_url: proxyURL || '',
            },
          },
        }

        await api.updateExchangeConfigsEncrypted(request)
        toast.success(t('aiTradersToast.exchangeConfigUpdated', language))
      } else {
        const createRequest = {
          exchange_type: exchangeType,
          account_name: accountName,
          enabled: true,
          api_key: apiKey || '',
          secret_key: secretKey || '',
          passphrase: passphrase || '',
          testnet: testnet || false,
          hyperliquid_wallet_addr: hyperliquidWalletAddr || '',
          aster_user: asterUser || '',
          aster_signer: asterSigner || '',
          aster_private_key: asterPrivateKey || '',
          lighter_wallet_addr: lighterWalletAddr || '',
          lighter_private_key: lighterPrivateKey || '',
          lighter_api_key_private_key: lighterApiKeyPrivateKey || '',
          lighter_api_key_index: lighterApiKeyIndex || 0,
          proxy_url: proxyURL || '',
        }

        await api.createExchangeEncrypted(createRequest)
        toast.success(t('aiTradersToast.exchangeCreated', language))
      }

      const refreshedExchanges = await api.getExchangeConfigs()
      setAllExchanges(refreshedExchanges)

      setShowExchangeModal(false)
      setEditingExchange(null)
    } catch (error) {
      console.error('Failed to save exchange config:', error)
      toast.error(t('saveConfigFailed', language))
    }
  }

  const handleAddModel = () => {
    setEditingModel(null)
    setShowModelModal(true)
  }

  const handleAddExchange = () => {
    setEditingExchange(null)
    setShowExchangeModal(true)
  }

  return (
    <DeepVoidBackground className="py-8">
      <div className="ait-page space-y-8 animate-fade-in">
        {/* Header */}
        <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-4 border-b border-border pb-6">
          <div className="flex items-center gap-4">
            <div className="w-12 h-12 rounded-card flex items-center justify-center bg-surface-alt text-primary">
              <Bot className="w-6 h-6" />
            </div>
            <div>
              <h1 className="t-title flex items-center gap-2.5">
                {t('aiTraders', language)}
                <span className="ait-pill bg-primary-dim text-primary">
                  {t('tradersPage.count', language, {
                    n: traders?.length || 0,
                  })}
                </span>
              </h1>
              <p className="t-caption text-muted-foreground mt-1 flex items-center gap-2">
                <span className="w-1.5 h-1.5 rounded-full bg-profit"></span>
                {t('tradersPage.ready', language)}
              </p>
            </div>
          </div>

          <div className="flex flex-wrap gap-2 w-full md:w-auto">
            <Button
              variant="unstyled"
              onClick={handleAddModel}
              className="h-9 px-3.5 rounded-control t-caption font-semibold border border-border bg-panel text-foreground hover:border-border-hover whitespace-nowrap inline-flex items-center gap-1.5"
            >
              <Plus className="w-3.5 h-3.5" />
              {t('aiModels', language)}
            </Button>
            <Button
              variant="unstyled"
              onClick={handleAddExchange}
              className="h-9 px-3.5 rounded-control t-caption font-semibold border border-border bg-panel text-foreground hover:border-border-hover whitespace-nowrap inline-flex items-center gap-1.5"
            >
              <Plus className="w-3.5 h-3.5" />
              {t('exchanges', language)}
            </Button>
            <Button
              variant="unstyled"
              onClick={() => setShowTelegramModal(true)}
              className="h-9 px-3.5 rounded-control t-caption font-semibold border border-border bg-panel text-foreground hover:border-border-hover whitespace-nowrap inline-flex items-center gap-1.5"
            >
              <MessageCircle className="w-3.5 h-3.5" />
              {t('tradersPage.telegram', language)}
            </Button>
            <Button
              variant="unstyled"
              onClick={() => setShowCreateModal(true)}
              disabled={
                configuredModels.length === 0 ||
                configuredExchanges.length === 0
              }
              className="h-9 px-4 rounded-control t-caption font-semibold bg-primary text-primary-foreground hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap inline-flex items-center gap-1.5"
            >
              <Plus className="w-4 h-4" />
              {t('createTrader', language)}
            </Button>
          </div>
        </div>

        {/* Configuration Status Grid */}
        <ConfigStatusGrid
          configuredModels={configuredModels}
          configuredExchanges={configuredExchanges}
          visibleExchangeAddresses={visibleExchangeAddresses}
          copiedId={copiedId}
          language={language}
          isModelInUse={isModelInUse}
          getModelUsageInfo={getModelUsageInfo}
          isExchangeInUse={isExchangeInUse}
          getExchangeUsageInfo={getExchangeUsageInfo}
          onModelClick={handleModelClick}
          onExchangeClick={handleExchangeClick}
          onToggleExchangeAddress={toggleExchangeAddressVisibility}
          onCopyAddress={handleCopyAddress}
        />

        {/* Traders List */}
        <TradersList
          traders={traders}
          isLoading={isTradersLoading}
          allExchanges={allExchanges}
          configuredModelsCount={configuredModels.length}
          configuredExchangesCount={configuredExchanges.length}
          visibleTraderAddresses={visibleTraderAddresses}
          copiedId={copiedId}
          language={language}
          onTraderSelect={onTraderSelect}
          onNavigate={(path) => navigate(path)}
          onEditTrader={handleEditTrader}
          onToggleTrader={handleToggleTrader}
          onToggleCompetition={handleToggleCompetition}
          onDeleteTrader={handleDeleteTrader}
          onToggleTraderAddress={toggleTraderAddressVisibility}
          onCopyAddress={handleCopyAddress}
        />

        <Suspense fallback={null}>
          {/* Create Trader Modal */}
          {showCreateModal && (
            <TraderConfigModal
              isOpen={showCreateModal}
              isEditMode={false}
              availableModels={enabledModels}
              availableExchanges={enabledExchanges}
              onSave={handleCreateTrader}
              onClose={() => setShowCreateModal(false)}
            />
          )}

          {/* Edit Trader Modal */}
          {showEditModal && editingTrader && (
            <TraderConfigModal
              isOpen={showEditModal}
              isEditMode={true}
              traderData={editingTrader}
              availableModels={enabledModels}
              availableExchanges={enabledExchanges}
              onSave={handleSaveEditTrader}
              onClose={() => {
                setShowEditModal(false)
                setEditingTrader(null)
              }}
            />
          )}

          {/* Model Configuration Modal */}
          {showModelModal && (
            <ModelConfigModal
              allModels={supportedModels}
              configuredModels={allModels}
              editingModelId={editingModel}
              onSave={handleSaveModelConfig}
              onDelete={handleDeleteModelConfig}
              onClose={() => {
                setShowModelModal(false)
                setEditingModel(null)
              }}
              language={language}
            />
          )}

          {/* Exchange Configuration Modal */}
          {showExchangeModal && (
            <ExchangeConfigModal
              allExchanges={allExchanges}
              editingExchangeId={editingExchange}
              onSave={handleSaveExchangeConfig}
              onDelete={handleDeleteExchangeConfig}
              onClose={() => {
                setShowExchangeModal(false)
                setEditingExchange(null)
              }}
              language={language}
            />
          )}

          {/* Telegram Bot Modal */}
          {showTelegramModal && (
            <TelegramConfigModal
              onClose={() => setShowTelegramModal(false)}
              language={language}
            />
          )}
        </Suspense>
      </div>
    </DeepVoidBackground>
  )
}
