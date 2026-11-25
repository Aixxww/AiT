import { useState, useEffect } from 'react'
import { toast } from 'sonner'
import {
  User,
  Cpu,
  Building2,
  MessageCircle,
  Eye,
  EyeOff,
  ChevronRight,
  Plus,
  Pencil,
  Palette,
  Monitor,
  ShieldCheck,
} from 'lucide-react'
import { useAuth } from '../contexts/AuthContext'
import { useTheme } from '../contexts/ThemeContext'
import { useLanguage } from '../contexts/LanguageContext'
import { api } from '../lib/api'
import { ExchangeConfigModal } from '../components/trader/ExchangeConfigModal'
import { TelegramConfigModal } from '../components/trader/TelegramConfigModal'
import { ModelConfigModal } from '../components/trader/ModelConfigModal'
import { CUSTOM_PROVIDER } from '../components/trader/model-constants'
import { SecuritySettings } from '../components/settings/SecuritySettings'
import type { Exchange, AIModel } from '../types'
import { Button } from '../components/ui/Button'
import { InfoTip } from '../components/ui/InfoTip'
import { t, type Language } from '../i18n/translations'

type Tab =
  | 'account'
  | 'security'
  | 'models'
  | 'exchanges'
  | 'telegram'
  | 'appearance'

function configBadge(label: string, active: boolean) {
  return (
    <span
      className={`text-[11px] px-2 py-0.5 rounded-full ${
        active
          ? 'bg-profit/10 text-profit'
          : 'bg-surface-alt text-muted-foreground'
      }`}
    >
      {label}
    </span>
  )
}

function AppearanceTab({ language }: { language: Language }) {
  const { mode, setMode, isDark } = useTheme()
  const tr = (k: string, p?: Record<string, string | number>) =>
    t(`settingsPage.${k}`, language, p)

  const themeOptions = [
    // Swatch values are per-theme preview data: they must render each
    // theme's own palette regardless of the active theme, so they stay
    // literal (kept in sync with the token blocks in index.css).
    {
      mode: 'pro-dark' as const,
      label: tr('themeProDark'),
      description: tr('themeProDarkDesc'),
      preview: { bg: '#0A0D10', surface: '#171C24', accent: '#F0B90B' },
    },
    {
      mode: 'pro-light' as const,
      label: tr('themeProLight'),
      description: tr('themeProLightDesc'),
      preview: { bg: '#F8F9FA', surface: '#FFFFFF', accent: '#D4A00A' },
    },
    {
      mode: 'glass-dark' as const,
      label: tr('themeGlassDark'),
      description: tr('themeGlassDarkDesc'),
      preview: {
        bg: '#0A0C12',
        surface: 'rgba(18,22,34,0.72)',
        accent: '#22D3EE',
      },
    },
    {
      mode: 'glass-light' as const,
      label: tr('themeGlassLight'),
      description: tr('themeGlassLightDesc'),
      preview: {
        bg: '#F0F2F5',
        surface: 'rgba(255,255,255,0.72)',
        accent: '#0891B2',
      },
    },
    {
      mode: 'system' as const,
      label: tr('themeSystem'),
      description: tr('themeSystemDesc'),
      preview: null,
    },
  ]

  return (
    <div className="space-y-6">
      <div>
        <h3 className="t-headline mb-4">{tr('themeTitle')}</h3>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
          {themeOptions.map((option) => (
            <Button
              variant="unstyled"
              key={option.mode}
              onClick={() => setMode(option.mode)}
              className={`relative flex flex-col items-center gap-2 p-4 rounded-xl border-2 transition-all
                ${
                  mode === option.mode
                    ? 'border-primary bg-primary-dim'
                    : 'border-border hover:border-border-hover bg-surface'
                }`}
            >
              {option.preview ? (
                <div
                  className="w-full h-12 rounded-lg overflow-hidden relative"
                  style={{ background: option.preview.bg }}
                >
                  <div
                    className="absolute bottom-0 left-0 right-0 h-6 rounded-t"
                    style={{ background: option.preview.surface }}
                  />
                  <div
                    className="absolute top-1 left-1 w-2 h-2 rounded-full"
                    style={{ background: option.preview.accent }}
                  />
                </div>
              ) : (
                <div className="w-full h-12 rounded-lg bg-gradient-to-br from-surface to-panel flex items-center justify-center">
                  <Monitor size={20} className="text-muted-foreground" />
                </div>
              )}
              <span className="text-xs font-medium text-foreground">
                {option.label}
              </span>
              <span className="text-[10px] text-muted-foreground">
                {option.description}
              </span>
              {mode === option.mode && (
                <div className="absolute top-2 right-2 w-2 h-2 rounded-full bg-primary" />
              )}
            </Button>
          ))}
        </div>
      </div>

      <div className="text-xs text-muted-foreground p-3 rounded-lg bg-surface-alt border border-border">
        {mode === 'system' ? (
          tr('themeFollowing', {
            mode: isDark ? tr('themeDark') : tr('themeLight'),
          })
        ) : (
          <>
            {mode.startsWith('glass')
              ? tr('themeGlassNote')
              : tr('themeProNote')}
          </>
        )}
      </div>
    </div>
  )
}

export function SettingsPage() {
  const { user } = useAuth()
  const { language } = useLanguage()
  const [activeTab, setActiveTab] = useState<Tab>('account')
  const tr = (k: string, p?: Record<string, string | number>) =>
    t(`settingsPage.${k}`, language, p)

  // Account state
  const [newPassword, setNewPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [changingPassword, setChangingPassword] = useState(false)

  // AI Models state
  const [configuredModels, setConfiguredModels] = useState<AIModel[]>([])
  const [supportedModels, setSupportedModels] = useState<AIModel[]>([])
  const [showModelModal, setShowModelModal] = useState(false)
  const [editingModel, setEditingModel] = useState<string | null>(null)

  // Exchanges state
  const [exchanges, setExchanges] = useState<Exchange[]>([])
  const [showExchangeModal, setShowExchangeModal] = useState(false)
  const [editingExchange, setEditingExchange] = useState<string | null>(null)

  // Telegram state
  const [showTelegramModal, setShowTelegramModal] = useState(false)

  const refreshModelConfigs = async () => {
    const [configs, supported] = await Promise.all([
      api.getModelConfigs(),
      api.getSupportedModels(),
    ])
    setConfiguredModels(configs)
    setSupportedModels(supported)
  }

  const refreshExchangeConfigs = async () => {
    const refreshed = await api.getExchangeConfigs()
    setExchanges(refreshed)
  }

  // Fetch data when tabs are visited
  useEffect(() => {
    if (activeTab === 'models') {
      refreshModelConfigs().catch(() => toast.error(tr('loadModelsFailed')))
    }
    if (activeTab === 'exchanges') {
      refreshExchangeConfigs().catch(() =>
        toast.error(tr('loadExchangesFailed'))
      )
    }
  }, [activeTab])

  useEffect(() => {
    const handleRefresh = () => {
      refreshModelConfigs().catch(() => {})
      refreshExchangeConfigs().catch(() => {})
    }
    window.addEventListener('agent-config-refresh', handleRefresh)
    return () =>
      window.removeEventListener('agent-config-refresh', handleRefresh)
  }, [])

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault()
    if (newPassword.length < 8) {
      toast.error(tr('passwordTooShort'))
      return
    }
    setChangingPassword(true)
    try {
      const res = await fetch('/api/user/password', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('auth_token') || ''}`,
        },
        body: JSON.stringify({ new_password: newPassword }),
      })
      if (!res.ok) {
        const data = await res.json().catch(() => ({}))
        throw new Error(data.error || tr('passwordUpdateFailed'))
      }
      toast.success(tr('passwordUpdated'))
      setNewPassword('')
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : tr('passwordUpdateFailed')
      )
    } finally {
      setChangingPassword(false)
    }
  }

  const handleSaveModel = async (
    modelId: string,
    apiKey: string,
    customApiUrl?: string,
    customModelName?: string,
    displayName?: string
  ) => {
    try {
      const existingModel = configuredModels.find((m) => m.id === modelId)
      const modelTemplate = supportedModels.find((m) => m.id === modelId)
      const modelToUpdate = existingModel || modelTemplate
      if (!modelToUpdate) {
        toast.error(tr('modelNotFound'))
        return
      }

      // #75: a custom (other) entry is saved on its own, by id ("custom"
      // creates a new entry), with its display name.
      if (modelToUpdate.provider === CUSTOM_PROVIDER) {
        await api.updateModelConfigs({
          models: {
            [existingModel ? existingModel.id : CUSTOM_PROVIDER]: {
              provider: CUSTOM_PROVIDER,
              name: displayName || modelToUpdate.name,
              enabled: true,
              api_key: apiKey,
              custom_api_url: customApiUrl || '',
              custom_model_name: customModelName || '',
            },
          },
        })
        toast.success(tr('modelSaved'))
        await refreshModelConfigs()
        setShowModelModal(false)
        setEditingModel(null)
        return
      }

      let updatedModels: AIModel[]
      if (existingModel) {
        updatedModels = configuredModels.map((m) =>
          m.id === modelId
            ? {
                ...m,
                apiKey,
                customApiUrl: customApiUrl || '',
                customModelName: customModelName || '',
                enabled: true,
              }
            : m
        )
      } else {
        updatedModels = [
          ...configuredModels,
          {
            ...modelToUpdate,
            apiKey,
            customApiUrl: customApiUrl || '',
            customModelName: customModelName || '',
            enabled: true,
          },
        ]
      }

      const request = {
        models: Object.fromEntries(
          updatedModels
            .filter((m) => m.provider !== CUSTOM_PROVIDER)
            .map((m) => [
              m.provider,
              {
                enabled: m.enabled,
                api_key: m.apiKey || '',
                custom_api_url: m.customApiUrl || '',
                custom_model_name: m.customModelName || '',
              },
            ])
        ),
      }
      await api.updateModelConfigs(request)
      toast.success(tr('modelSaved'))
      await refreshModelConfigs()
      setShowModelModal(false)
      setEditingModel(null)
    } catch {
      toast.error(tr('modelSaveFailed'))
    }
  }

  const handleDeleteModel = async (modelId: string) => {
    try {
      const target = configuredModels.find((m) => m.id === modelId)
      if (target?.provider === CUSTOM_PROVIDER) {
        await api.deleteModelConfig(modelId)
        await refreshModelConfigs()
        setShowModelModal(false)
        setEditingModel(null)
        toast.success(tr('modelRemoved'))
        return
      }
      const updatedModels = configuredModels.map((m) =>
        m.id === modelId
          ? {
              ...m,
              apiKey: '',
              customApiUrl: '',
              customModelName: '',
              enabled: false,
            }
          : m
      )
      const request = {
        models: Object.fromEntries(
          updatedModels
            .filter((m) => m.provider !== CUSTOM_PROVIDER)
            .map((m) => [
              m.provider,
              {
                enabled: m.enabled,
                api_key: m.apiKey || '',
                custom_api_url: m.customApiUrl || '',
                custom_model_name: m.customModelName || '',
              },
            ])
        ),
      }
      await api.updateModelConfigs(request)
      await refreshModelConfigs()
      setShowModelModal(false)
      setEditingModel(null)
      toast.success(tr('modelRemoved'))
    } catch {
      toast.error(tr('modelRemoveFailed'))
    }
  }

  const handleSaveExchange = async (
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
        toast.success(tr('exchangeUpdated'))
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
        toast.success(tr('exchangeCreated'))
      }
      await refreshExchangeConfigs()
      setShowExchangeModal(false)
      setEditingExchange(null)
    } catch {
      toast.error(tr('exchangeSaveFailed'))
    }
  }

  const handleDeleteExchange = async (exchangeId: string) => {
    try {
      await api.deleteExchange(exchangeId)
      toast.success(tr('exchangeDeleted'))
      await refreshExchangeConfigs()
      setShowExchangeModal(false)
      setEditingExchange(null)
    } catch {
      toast.error(tr('exchangeDeleteFailed'))
    }
  }

  const tabs: { key: Tab; label: string; icon: React.ReactNode }[] = [
    { key: 'account', label: tr('tabAccount'), icon: <User size={16} /> },
    {
      key: 'security',
      label: tr('tabSecurity'),
      icon: <ShieldCheck size={16} />,
    },
    { key: 'models', label: tr('tabModels'), icon: <Cpu size={16} /> },
    {
      key: 'exchanges',
      label: tr('tabExchanges'),
      icon: <Building2 size={16} />,
    },
    {
      key: 'telegram',
      label: tr('tabTelegram'),
      icon: <MessageCircle size={16} />,
    },
    {
      key: 'appearance',
      label: tr('tabAppearance'),
      icon: <Palette size={16} />,
    },
  ]

  return (
    <div className="min-h-screen pt-20 pb-12 bg-background">
      <div className="max-w-3xl mx-auto px-[var(--page-gutter)]">
        <h1 className="t-title mb-6">{tr('title')}</h1>

        {/* Tabs: one row that scrolls horizontally when it does not fit
            (#81 PR 2: they used to wrap into two lines at 1280px); labels
            stay visible on phones. */}
        <div
          data-shot="settings-tabs"
          role="tablist"
          className="ait-segmented w-full mb-6 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
        >
          {tabs.map((tab) => (
            <button
              type="button"
              role="tab"
              key={tab.key}
              aria-selected={activeTab === tab.key}
              onClick={() => setActiveTab(tab.key)}
              className="flex-1 shrink-0 inline-flex items-center justify-center gap-1.5 whitespace-nowrap px-3"
            >
              {tab.icon}
              {tab.label}
            </button>
          ))}
        </div>

        {/* Tab Content */}
        <div className="ait-card p-4 sm:p-6">
          {/* Account Tab */}
          {activeTab === 'account' && (
            <div className="space-y-6">
              <div>
                <p className="t-micro text-muted-foreground mb-1">
                  {tr('email')}
                </p>
                <p className="text-sm text-foreground font-medium">
                  {user?.email}
                </p>
              </div>

              <div className="border-t border-border pt-6">
                <h3 className="t-headline mb-4">{tr('changePassword')}</h3>
                <form onSubmit={handleChangePassword} className="space-y-4">
                  <div>
                    <label
                      htmlFor="settings-new-password"
                      className="block t-caption text-muted-foreground mb-2"
                    >
                      {tr('newPassword')}
                    </label>
                    <div className="relative">
                      <input
                        id="settings-new-password"
                        type={showPassword ? 'text' : 'password'}
                        value={newPassword}
                        onChange={(e) => setNewPassword(e.target.value)}
                        className="w-full bg-input border border-border rounded-xl px-4 py-3 pr-11 text-sm text-foreground placeholder-muted-foreground/60 focus:outline-none focus:border-primary/60 focus:ring-1 focus:ring-primary/30 transition-all"
                        placeholder={tr('passwordPlaceholder')}
                        required
                      />
                      <Button
                        variant="unstyled"
                        type="button"
                        onClick={() => setShowPassword(!showPassword)}
                        aria-label={
                          showPassword ? tr('hidePassword') : tr('showPassword')
                        }
                        className="absolute right-3.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                      >
                        {showPassword ? (
                          <EyeOff size={16} />
                        ) : (
                          <Eye size={16} />
                        )}
                      </Button>
                    </div>
                  </div>
                  <Button
                    variant="unstyled"
                    type="submit"
                    disabled={changingPassword || newPassword.length < 8}
                    className="w-full bg-primary hover:opacity-90 active:scale-[0.98] text-black font-semibold py-3 rounded-xl text-sm transition-all disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {changingPassword ? tr('updating') : tr('updatePassword')}
                  </Button>
                </form>
              </div>
            </div>
          )}

          {/* Security Tab */}
          {activeTab === 'security' && <SecuritySettings />}

          {/* AI Models Tab */}
          {activeTab === 'models' && (
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <p className="text-sm text-muted-foreground">
                  {tr('modelsCount', { n: configuredModels.length })}
                </p>
                <Button
                  variant="unstyled"
                  onClick={() => {
                    setEditingModel(null)
                    setShowModelModal(true)
                  }}
                  className="flex items-center gap-1.5 text-xs font-medium bg-primary-dim hover:bg-primary/20 text-primary px-3 py-1.5 rounded-lg transition-colors"
                >
                  <Plus size={14} />
                  {tr('addModel')}
                </Button>
              </div>

              {configuredModels.length === 0 ? (
                <div className="text-center py-8 text-muted-foreground text-sm">
                  {tr('noModels')}
                </div>
              ) : (
                <div className="space-y-2">
                  {configuredModels.map((model) => (
                    <Button
                      variant="unstyled"
                      key={model.id}
                      onClick={() => {
                        setEditingModel(model.id)
                        setShowModelModal(true)
                      }}
                      className="w-full flex items-center justify-between px-4 py-3 rounded-xl bg-surface-alt hover:bg-panel-hover border border-border transition-colors group"
                    >
                      <div className="flex items-center gap-3">
                        <div className="w-8 h-8 rounded-lg bg-muted flex items-center justify-center">
                          <Cpu size={14} className="text-foreground" />
                        </div>
                        <div className="text-left">
                          <p className="text-sm font-medium text-foreground">
                            {model.name}
                          </p>
                          <div className="flex flex-wrap items-center gap-1.5 mt-1">
                            <p className="text-xs text-muted-foreground">
                              {model.provider}
                            </p>
                            {configBadge(
                              tr('badgeApiKey'),
                              !!model.has_api_key
                            )}
                            {model.customModelName
                              ? configBadge(tr('badgeCustomModel'), true)
                              : null}
                            {model.customApiUrl
                              ? configBadge(tr('badgeBaseUrl'), true)
                              : null}
                          </div>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <span
                          className={`text-xs px-2 py-0.5 rounded-full ${model.enabled ? 'bg-profit/10 text-profit' : 'bg-muted text-muted-foreground'}`}
                        >
                          {model.enabled ? tr('active') : tr('inactive')}
                        </span>
                        <Pencil
                          size={14}
                          className="text-muted-foreground group-hover:text-muted-foreground transition-colors"
                        />
                      </div>
                    </Button>
                  ))}
                </div>
              )}
            </div>
          )}

          {/* Exchanges Tab */}
          {activeTab === 'exchanges' && (
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <p className="text-sm text-muted-foreground">
                  {tr('exchangesCount', { n: exchanges.length })}
                </p>
                <Button
                  variant="unstyled"
                  onClick={() => {
                    setEditingExchange(null)
                    setShowExchangeModal(true)
                  }}
                  className="flex items-center gap-1.5 text-xs font-medium bg-primary-dim hover:bg-primary/20 text-primary px-3 py-1.5 rounded-lg transition-colors"
                >
                  <Plus size={14} />
                  {tr('addExchange')}
                </Button>
              </div>

              {exchanges.length === 0 ? (
                <div className="text-center py-8 text-muted-foreground text-sm">
                  {tr('noExchanges')}
                </div>
              ) : (
                <div className="space-y-2">
                  {exchanges.map((exchange) => (
                    <Button
                      variant="unstyled"
                      key={exchange.id}
                      onClick={() => {
                        setEditingExchange(exchange.id)
                        setShowExchangeModal(true)
                      }}
                      className="w-full flex items-center justify-between px-4 py-3 rounded-xl bg-surface-alt hover:bg-panel-hover border border-border transition-colors group"
                    >
                      <div className="flex items-center gap-3">
                        <div className="w-8 h-8 rounded-lg bg-muted flex items-center justify-center">
                          <Building2 size={14} className="text-foreground" />
                        </div>
                        <div className="text-left">
                          <p className="text-sm font-medium text-foreground">
                            {exchange.account_name || exchange.name}
                          </p>
                          <div className="flex flex-wrap items-center gap-1.5 mt-1">
                            <p className="text-xs text-muted-foreground capitalize">
                              {exchange.exchange_type || exchange.type}
                            </p>
                            {configBadge(
                              tr('badgeApiKey'),
                              !!exchange.has_api_key
                            )}
                            {configBadge(
                              tr('badgeSecret'),
                              !!exchange.has_secret_key
                            )}
                            {exchange.has_passphrase
                              ? configBadge(tr('badgePassphrase'), true)
                              : null}
                            {exchange.hyperliquidWalletAddr
                              ? configBadge(tr('badgeWallet'), true)
                              : null}
                            {exchange.has_aster_private_key
                              ? configBadge(tr('badgeAsterKey'), true)
                              : null}
                            {exchange.has_lighter_private_key ||
                            exchange.has_lighter_api_key_private_key
                              ? configBadge(tr('badgeLighterKey'), true)
                              : null}
                          </div>
                        </div>
                      </div>
                      <ChevronRight
                        size={14}
                        className="text-muted-foreground group-hover:text-muted-foreground transition-colors"
                      />
                    </Button>
                  ))}
                </div>
              )}
            </div>
          )}

          {/* Telegram Tab */}
          {activeTab === 'telegram' && (
            <div className="space-y-4">
              <p className="t-caption text-muted-foreground inline-flex items-center gap-1.5">
                {tr('telegramSummary')}
                <InfoTip
                  content={tr('telegramInfo')}
                  title={tr('tabTelegram')}
                />
              </p>
              <Button
                variant="unstyled"
                onClick={() => setShowTelegramModal(true)}
                className="w-full flex items-center justify-between px-4 py-3 rounded-xl bg-surface-alt hover:bg-panel-hover border border-border transition-colors group"
              >
                <div className="flex items-center gap-3">
                  <div className="w-8 h-8 rounded-lg bg-[#0088cc]/20 flex items-center justify-center">
                    <MessageCircle size={14} className="text-[#0088cc]" />
                  </div>
                  <span className="text-sm font-medium text-foreground">
                    {tr('configureTelegram')}
                  </span>
                </div>
                <ChevronRight
                  size={14}
                  className="text-muted-foreground group-hover:text-muted-foreground transition-colors"
                />
              </Button>
            </div>
          )}

          {/* Appearance Tab */}
          {activeTab === 'appearance' && <AppearanceTab language={language} />}
        </div>
      </div>

      {/* AI Model Modal */}
      {showModelModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm px-4">
          <ModelConfigModal
            allModels={supportedModels}
            configuredModels={configuredModels}
            editingModelId={editingModel}
            onSave={handleSaveModel}
            onDelete={handleDeleteModel}
            onClose={() => {
              setShowModelModal(false)
              setEditingModel(null)
            }}
            language={language}
          />
        </div>
      )}

      {/* Exchange Modal */}
      {showExchangeModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm px-4">
          <ExchangeConfigModal
            allExchanges={exchanges}
            editingExchangeId={editingExchange}
            onSave={handleSaveExchange}
            onDelete={handleDeleteExchange}
            onClose={() => {
              setShowExchangeModal(false)
              setEditingExchange(null)
            }}
            language={language}
          />
        </div>
      )}

      {/* Telegram Modal */}
      {showTelegramModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm px-4">
          <TelegramConfigModal
            onClose={() => setShowTelegramModal(false)}
            language={language}
          />
        </div>
      )}
    </div>
  )
}
