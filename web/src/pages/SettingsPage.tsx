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
} from 'lucide-react'
import { useAuth } from '../contexts/AuthContext'
import { useTheme } from '../contexts/ThemeContext'
import { useLanguage } from '../contexts/LanguageContext'
import { api } from '../lib/api'
import { ExchangeConfigModal } from '../components/trader/ExchangeConfigModal'
import { TelegramConfigModal } from '../components/trader/TelegramConfigModal'
import { ModelConfigModal } from '../components/trader/ModelConfigModal'
import type { Exchange, AIModel } from '../types'

type Tab = 'account' | 'models' | 'exchanges' | 'telegram' | 'appearance'

function configBadge(label: string, active: boolean) {
  return (
    <span
      className={`text-[11px] px-2 py-0.5 rounded-full ${
        active
          ? 'bg-emerald-500/10 text-emerald-300'
          : 'bg-surface-alt text-muted-foreground'
      }`}
    >
      {label}
    </span>
  )
}

function AppearanceTab() {
  const { mode, setMode, isDark } = useTheme()

  const themeOptions = [
    {
      mode: 'pro-dark' as const,
      label: 'Cyber Dark',
      description: 'Neon night',
      preview: { bg: '#08111B', surface: '#0E1A28', accent: '#35E6FF' },
    },
    {
      mode: 'pro-light' as const,
      label: 'Cyber Light',
      description: 'Daylight grid',
      preview: { bg: '#EDF7FB', surface: '#FFFFFF', accent: '#007EA7' },
    },
    {
      mode: 'glass-dark' as const,
      label: 'Glass Dark',
      description: 'Neon glass',
      preview: {
        bg: '#08111B',
        surface: 'rgba(17,33,52,0.72)',
        accent: '#FF4FD8',
      },
    },
    {
      mode: 'glass-light' as const,
      label: 'Glass Light',
      description: 'Frosted cyber',
      preview: {
        bg: '#EDF7FB',
        surface: 'rgba(255,255,255,0.72)',
        accent: '#D92FBF',
      },
    },
    {
      mode: 'system' as const,
      label: 'System',
      description: 'Auto',
      preview: null,
    },
  ]

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-semibold text-foreground mb-4">Theme</h3>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
          {themeOptions.map((option) => (
            <button
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
            </button>
          ))}
        </div>
      </div>

      <div className="text-xs text-muted-foreground p-3 rounded-lg bg-surface-alt border border-border">
        {mode === 'system' ? (
          <>
            Following system preference. Currently:{' '}
            <span className="font-medium text-foreground">
              {isDark ? 'Dark' : 'Light'}
            </span>
          </>
        ) : (
          <>
            {mode.startsWith('glass')
              ? 'Cyber glass with translucent panels and neon depth.'
              : 'High-density cyber interface with adaptive day/night contrast.'}
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
      refreshModelConfigs().catch(() => toast.error('Failed to load AI models'))
    }
    if (activeTab === 'exchanges') {
      refreshExchangeConfigs().catch(() =>
        toast.error('Failed to load exchanges')
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
      toast.error('Password must be at least 8 characters')
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
        throw new Error(data.error || 'Failed to update password')
      }
      toast.success('Password updated successfully')
      setNewPassword('')
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to update password'
      )
    } finally {
      setChangingPassword(false)
    }
  }

  const handleSaveModel = async (
    modelId: string,
    apiKey: string,
    customApiUrl?: string,
    customModelName?: string
  ) => {
    try {
      const existingModel = configuredModels.find((m) => m.id === modelId)
      const modelTemplate = supportedModels.find((m) => m.id === modelId)
      const modelToUpdate = existingModel || modelTemplate
      if (!modelToUpdate) {
        toast.error('Model not found')
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
          updatedModels.map((m) => [
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
      toast.success('Model config saved')
      await refreshModelConfigs()
      setShowModelModal(false)
      setEditingModel(null)
    } catch {
      toast.error('Failed to save model config')
    }
  }

  const handleDeleteModel = async (modelId: string) => {
    try {
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
          updatedModels.map((m) => [
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
      toast.success('Model config removed')
    } catch {
      toast.error('Failed to remove model config')
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
        toast.success('Exchange config updated')
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
        toast.success('Exchange account created')
      }
      await refreshExchangeConfigs()
      setShowExchangeModal(false)
      setEditingExchange(null)
    } catch {
      toast.error('Failed to save exchange config')
    }
  }

  const handleDeleteExchange = async (exchangeId: string) => {
    try {
      await api.deleteExchange(exchangeId)
      toast.success('Exchange account deleted')
      await refreshExchangeConfigs()
      setShowExchangeModal(false)
      setEditingExchange(null)
    } catch {
      toast.error('Failed to delete exchange account')
    }
  }

  const tabs: { key: Tab; label: string; icon: React.ReactNode }[] = [
    { key: 'account', label: 'Account', icon: <User size={16} /> },
    { key: 'models', label: 'AI Models', icon: <Cpu size={16} /> },
    { key: 'exchanges', label: 'Exchanges', icon: <Building2 size={16} /> },
    { key: 'telegram', label: 'Telegram', icon: <MessageCircle size={16} /> },
    { key: 'appearance', label: 'Appearance', icon: <Palette size={16} /> },
  ]

  return (
    <div className="min-h-screen pt-20 pb-12 px-4 bg-background">
      <div className="max-w-2xl mx-auto">
        <h1 className="text-xl font-bold text-foreground mb-6">Settings</h1>

        {/* Tabs */}
        <div className="flex gap-1 mb-6 bg-panel border border-border rounded-xl p-1">
          {tabs.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-lg text-sm font-medium transition-all
                ${
                  activeTab === tab.key
                    ? 'bg-primary text-primary-foreground'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
            >
              {tab.icon}
              <span className="hidden sm:inline">{tab.label}</span>
            </button>
          ))}
        </div>

        {/* Tab Content */}
        <div className="bg-panel backdrop-blur-xl border border-border rounded-2xl p-6">
          {/* Account Tab */}
          {activeTab === 'account' && (
            <div className="space-y-6">
              <div>
                <p className="text-xs text-muted-foreground mb-1">Email</p>
                <p className="text-sm text-foreground font-medium">
                  {user?.email}
                </p>
              </div>

              <div className="border-t border-border pt-6">
                <h3 className="text-sm font-semibold text-foreground mb-4">
                  Change Password
                </h3>
                <form onSubmit={handleChangePassword} className="space-y-4">
                  <div>
                    <label className="block text-xs font-medium text-muted-foreground mb-2">
                      New Password
                    </label>
                    <div className="relative">
                      <input
                        type={showPassword ? 'text' : 'password'}
                        value={newPassword}
                        onChange={(e) => setNewPassword(e.target.value)}
                        className="w-full bg-input border border-border rounded-xl px-4 py-3 pr-11 text-sm text-foreground placeholder-zinc-600 focus:outline-none focus:border-primary/60 focus:ring-1 focus:ring-primary/30 transition-all"
                        placeholder="At least 8 characters"
                        required
                      />
                      <button
                        type="button"
                        onClick={() => setShowPassword(!showPassword)}
                        className="absolute right-3.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                      >
                        {showPassword ? (
                          <EyeOff size={16} />
                        ) : (
                          <Eye size={16} />
                        )}
                      </button>
                    </div>
                  </div>
                  <button
                    type="submit"
                    disabled={changingPassword || newPassword.length < 8}
                    className="w-full bg-primary hover:opacity-90 active:scale-[0.98] text-black font-semibold py-3 rounded-xl text-sm transition-all disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {changingPassword ? 'Updating...' : 'Update Password'}
                  </button>
                </form>
              </div>
            </div>
          )}

          {/* AI Models Tab */}
          {activeTab === 'models' && (
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <p className="text-sm text-muted-foreground">
                  {configuredModels.length} model
                  {configuredModels.length !== 1 ? 's' : ''} configured
                </p>
                <button
                  onClick={() => {
                    setEditingModel(null)
                    setShowModelModal(true)
                  }}
                  className="flex items-center gap-1.5 text-xs font-medium bg-primary-dim hover:bg-primary/20 text-primary px-3 py-1.5 rounded-lg transition-colors"
                >
                  <Plus size={14} />
                  Add Model
                </button>
              </div>

              {configuredModels.length === 0 ? (
                <div className="text-center py-8 text-muted-foreground text-sm">
                  No AI models configured yet
                </div>
              ) : (
                <div className="space-y-2">
                  {configuredModels.map((model) => (
                    <button
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
                            {configBadge('API Key', !!model.has_api_key)}
                            {model.customModelName
                              ? configBadge('Custom Model', true)
                              : null}
                            {model.customApiUrl
                              ? configBadge('Base URL', true)
                              : null}
                          </div>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <span
                          className={`text-xs px-2 py-0.5 rounded-full ${model.enabled ? 'bg-emerald-500/10 text-emerald-400' : 'bg-muted text-muted-foreground'}`}
                        >
                          {model.enabled ? 'Active' : 'Inactive'}
                        </span>
                        <Pencil
                          size={14}
                          className="text-muted-foreground group-hover:text-muted-foreground transition-colors"
                        />
                      </div>
                    </button>
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
                  {exchanges.length} account{exchanges.length !== 1 ? 's' : ''}{' '}
                  connected
                </p>
                <button
                  onClick={() => {
                    setEditingExchange(null)
                    setShowExchangeModal(true)
                  }}
                  className="flex items-center gap-1.5 text-xs font-medium bg-primary-dim hover:bg-primary/20 text-primary px-3 py-1.5 rounded-lg transition-colors"
                >
                  <Plus size={14} />
                  Add Exchange
                </button>
              </div>

              {exchanges.length === 0 ? (
                <div className="text-center py-8 text-muted-foreground text-sm">
                  No exchange accounts connected yet
                </div>
              ) : (
                <div className="space-y-2">
                  {exchanges.map((exchange) => (
                    <button
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
                            {configBadge('API Key', !!exchange.has_api_key)}
                            {configBadge('Secret', !!exchange.has_secret_key)}
                            {exchange.has_passphrase
                              ? configBadge('Passphrase', true)
                              : null}
                            {exchange.hyperliquidWalletAddr
                              ? configBadge('Wallet', true)
                              : null}
                            {exchange.has_aster_private_key
                              ? configBadge('Aster Key', true)
                              : null}
                            {exchange.has_lighter_private_key ||
                            exchange.has_lighter_api_key_private_key
                              ? configBadge('Lighter Key', true)
                              : null}
                          </div>
                        </div>
                      </div>
                      <ChevronRight
                        size={14}
                        className="text-muted-foreground group-hover:text-muted-foreground transition-colors"
                      />
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}

          {/* Telegram Tab */}
          {activeTab === 'telegram' && (
            <div className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Connect a Telegram bot to receive trading notifications and
                interact with your traders.
              </p>
              <button
                onClick={() => setShowTelegramModal(true)}
                className="w-full flex items-center justify-between px-4 py-3 rounded-xl bg-surface-alt hover:bg-panel-hover border border-border transition-colors group"
              >
                <div className="flex items-center gap-3">
                  <div className="w-8 h-8 rounded-lg bg-[#0088cc]/20 flex items-center justify-center">
                    <MessageCircle size={14} className="text-[#0088cc]" />
                  </div>
                  <span className="text-sm font-medium text-foreground">
                    Configure Telegram Bot
                  </span>
                </div>
                <ChevronRight
                  size={14}
                  className="text-muted-foreground group-hover:text-muted-foreground transition-colors"
                />
              </button>
            </div>
          )}

          {/* Appearance Tab */}
          {activeTab === 'appearance' && <AppearanceTab />}
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
