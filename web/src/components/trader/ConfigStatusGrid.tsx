import { Brain, Landmark, Eye, EyeOff, Copy, Check } from 'lucide-react'
import type { AIModel, Exchange, ExchangeAccountState } from '../../types'
import type { Language } from '../../i18n/translations'
import { t } from '../../i18n/translations'
import { getModelIcon } from '../common/ModelIcons'
import { getExchangeIcon } from '../common/ExchangeIcons'
import {
  getShortName,
  AI_PROVIDER_CONFIG,
  truncateAddress,
} from './model-constants'
import { Button } from '../ui/Button'

interface UsageInfo {
  runningCount: number
  totalCount: number
}

interface ConfigStatusGridProps {
  configuredModels: AIModel[]
  configuredExchanges: Exchange[]
  exchangeAccountStates?: Record<string, ExchangeAccountState>
  isExchangeAccountStatesLoading?: boolean
  visibleExchangeAddresses: Set<string>
  copiedId: string | null
  language: Language
  isModelInUse: (modelId: string) => boolean | undefined
  getModelUsageInfo: (modelId: string) => UsageInfo
  isExchangeInUse: (exchangeId: string) => boolean | undefined
  getExchangeUsageInfo: (exchangeId: string) => UsageInfo
  onModelClick: (modelId: string) => void
  onExchangeClick: (exchangeId: string) => void
  onToggleExchangeAddress: (exchangeId: string) => void
  onCopyAddress: (id: string, address: string) => void
}

export function ConfigStatusGrid({
  configuredModels,
  configuredExchanges,
  exchangeAccountStates,
  isExchangeAccountStatesLoading,
  visibleExchangeAddresses,
  copiedId,
  language,
  isModelInUse,
  getModelUsageInfo,
  isExchangeInUse,
  getExchangeUsageInfo,
  onModelClick,
  onExchangeClick,
  onToggleExchangeAddress,
  onCopyAddress,
}: ConfigStatusGridProps) {
  const getExchangeStateMeta = (state: ExchangeAccountState | undefined) => {
    if (!state) {
      return {
        label: t('configGrid.notChecked', language),
        className: 'text-muted-foreground border-border/80 bg-surface/40',
      }
    }

    switch (state.status) {
      case 'ok':
        return {
          label: state.display_balance || '0',
          className: 'text-profit border-profit/20 bg-profit/10',
        }
      case 'disabled':
        return {
          label: t('configGrid.disabled', language),
          className: 'text-muted-foreground border-border/80 bg-surface/40',
        }
      case 'missing_credentials':
        return {
          label: t('configGrid.incomplete', language),
          className: 'text-warning border-warning/20 bg-warning/10',
        }
      case 'invalid_credentials':
        return {
          label: t('configGrid.invalidKeys', language),
          className: 'text-loss border-loss/20 bg-loss/10',
        }
      case 'permission_denied':
        return {
          label: t('configGrid.noPermission', language),
          className: 'text-warning border-warning/20 bg-warning/10',
        }
      default:
        return {
          label: t('configGrid.unavailable', language),
          className: 'text-foreground border-border bg-surface-alt',
        }
    }
  }

  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
      {/* AI Models Card */}
      <div className="ait-card overflow-hidden">
        <div className="px-4 py-3 border-b border-border flex items-center gap-2">
          <Brain className="w-4 h-4 text-primary" />
          <h3 className="t-headline">{t('aiModels', language)}</h3>
        </div>

        <div className="p-4 space-y-3">
          {configuredModels.map((model) => {
            const inUse = isModelInUse(model.id)
            const usageInfo = getModelUsageInfo(model.id)
            return (
              <div
                key={model.id}
                className={`group relative flex items-center justify-between p-3 rounded-md transition-all border border-transparent ${
                  inUse
                    ? 'opacity-80'
                    : 'hover:bg-panel-hover hover:border-border-hover cursor-pointer'
                } bg-surface-alt`}
                onClick={() => onModelClick(model.id)}
              >
                <div className="flex items-center gap-4">
                  <div className="relative">
                    <div className="absolute inset-0 bg-info/20 rounded-full blur-sm group-hover:bg-info/30 transition-all"></div>
                    <div className="w-10 h-10 rounded-full flex items-center justify-center bg-panel border border-border relative z-10">
                      {getModelIcon(model.provider || model.id, {
                        width: 20,
                        height: 20,
                      }) || (
                        <span className="text-xs font-bold text-info">
                          {getShortName(model.name)[0]}
                        </span>
                      )}
                    </div>
                  </div>

                  <div className="min-w-0">
                    <div className="text-sm font-medium text-foreground group-hover:text-primary transition-colors">
                      {getShortName(model.name)}
                    </div>
                    <div className="t-caption text-muted-foreground flex items-center gap-2">
                      {model.customModelName ||
                        AI_PROVIDER_CONFIG[model.provider]?.defaultModel ||
                        ''}
                    </div>
                    {model.provider === 'claw402' &&
                    (model.balanceUsdc || model.walletAddress) ? (
                      <div className="mt-1.5 flex flex-wrap items-center gap-2 t-micro">
                        {model.balanceUsdc ? (
                          <span className="rounded border border-profit/20 bg-profit/10 px-1.5 py-0.5 text-profit">
                            {model.balanceUsdc} USDC
                          </span>
                        ) : null}
                        {model.walletAddress ? (
                          <span className="rounded border border-info/20 bg-info/10 px-1.5 py-0.5 text-info">
                            {truncateAddress(model.walletAddress)}
                          </span>
                        ) : null}
                      </div>
                    ) : null}
                  </div>
                </div>

                <div className="text-right">
                  {usageInfo.totalCount > 0 ? (
                    <span
                      className={`t-micro px-2 py-1 rounded-control border ${
                        usageInfo.runningCount > 0
                          ? 'bg-profit/10 border-profit/30 text-profit'
                          : 'bg-warning/10 border-warning/30 text-warning'
                      }`}
                    >
                      {t('configGrid.inUse', language, {
                        running: usageInfo.runningCount,
                        total: usageInfo.totalCount,
                      })}
                    </span>
                  ) : (
                    <span className="text-[11px] text-muted-foreground">
                      {t('configGrid.standby', language)}
                    </span>
                  )}
                </div>
              </div>
            )
          })}

          {configuredModels.length === 0 && (
            <div className="text-center py-10 border border-dashed border-border rounded-card bg-surface-alt">
              <Brain className="w-8 h-8 mx-auto mb-3 text-muted-foreground/40" />
              <div className="t-body text-muted-foreground">
                {t('noModelsConfigured', language)}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Exchanges Card */}
      <div className="ait-card overflow-hidden">
        <div className="px-4 py-3 border-b border-border flex items-center gap-2">
          <Landmark className="w-4 h-4 text-primary" />
          <h3 className="t-headline">{t('exchanges', language)}</h3>
        </div>

        <div className="p-4 space-y-3">
          {configuredExchanges.map((exchange) => {
            const inUse = isExchangeInUse(exchange.id)
            const usageInfo = getExchangeUsageInfo(exchange.id)
            const state = exchangeAccountStates?.[exchange.id]
            const stateMeta = getExchangeStateMeta(state)
            return (
              <div
                key={exchange.id}
                className={`group relative flex items-center justify-between p-3 rounded-md transition-all border border-transparent ${
                  inUse
                    ? 'opacity-80'
                    : 'hover:bg-panel-hover hover:border-border-hover cursor-pointer'
                } bg-surface-alt`}
                onClick={() => onExchangeClick(exchange.id)}
              >
                <div className="flex items-center gap-4 min-w-0">
                  <div className="relative">
                    <div className="absolute inset-0 bg-warning/20 rounded-full blur-sm group-hover:bg-warning/30 transition-all"></div>
                    <div className="w-10 h-10 rounded-full flex items-center justify-center bg-panel border border-border relative z-10">
                      {getExchangeIcon(exchange.exchange_type || exchange.id, {
                        width: 20,
                        height: 20,
                      })}
                    </div>
                  </div>

                  <div className="min-w-0">
                    <div className="text-sm font-medium text-foreground group-hover:text-primary transition-colors truncate">
                      {exchange.exchange_type?.toUpperCase() ||
                        getShortName(exchange.name)}
                      <span className="text-[10px] text-muted-foreground ml-2 border border-border px-1 rounded">
                        {exchange.account_name || 'DEFAULT'}
                      </span>
                    </div>
                    <div className="t-caption text-muted-foreground flex items-center gap-2">
                      {exchange.type?.toUpperCase() || 'CEX'}
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-2 t-micro">
                      <span
                        className={`rounded border px-1.5 py-0.5 ${stateMeta.className}`}
                      >
                        {isExchangeAccountStatesLoading && !state
                          ? t('configGrid.checking', language)
                          : stateMeta.label}
                      </span>
                      {state?.status !== 'ok' && state?.error_message ? (
                        <span className="text-muted-foreground truncate max-w-[220px]">
                          {state.error_message}
                        </span>
                      ) : null}
                    </div>
                  </div>
                </div>

                <div className="flex flex-col items-end gap-1">
                  {/* Wallet Address Display Logic */}
                  {(() => {
                    const walletAddr =
                      exchange.hyperliquidWalletAddr ||
                      exchange.asterUser ||
                      exchange.lighterWalletAddr
                    if (exchange.type !== 'dex' || !walletAddr) return null
                    const isVisible = visibleExchangeAddresses.has(exchange.id)
                    const isCopied = copiedId === `exchange-${exchange.id}`

                    return (
                      <div
                        className="flex items-center gap-1"
                        onClick={(e) => e.stopPropagation()}
                      >
                        <span className="t-micro text-muted-foreground bg-surface-alt px-1.5 py-0.5 rounded border border-border">
                          {isVisible ? walletAddr : truncateAddress(walletAddr)}
                        </span>
                        <Button
                          variant="unstyled"
                          onClick={(e) => {
                            e.stopPropagation()
                            onToggleExchangeAddress(exchange.id)
                          }}
                          className="text-muted-foreground hover:text-foreground"
                        >
                          {isVisible ? <EyeOff size={10} /> : <Eye size={10} />}
                        </Button>
                        <Button
                          variant="unstyled"
                          onClick={(e) => {
                            e.stopPropagation()
                            onCopyAddress(`exchange-${exchange.id}`, walletAddr)
                          }}
                          className="text-muted-foreground hover:text-primary"
                        >
                          {isCopied ? (
                            <Check size={10} className="text-profit" />
                          ) : (
                            <Copy size={10} />
                          )}
                        </Button>
                      </div>
                    )
                  })()}

                  {usageInfo.totalCount > 0 ? (
                    <span
                      className={`t-micro px-2 py-1 rounded-control border ${
                        usageInfo.runningCount > 0
                          ? 'bg-profit/10 border-profit/30 text-profit'
                          : 'bg-warning/10 border-warning/30 text-warning'
                      }`}
                    >
                      {t('configGrid.inUse', language, {
                        running: usageInfo.runningCount,
                        total: usageInfo.totalCount,
                      })}
                    </span>
                  ) : (
                    <span className="text-[11px] text-muted-foreground">
                      {t('configGrid.standby', language)}
                    </span>
                  )}
                </div>
              </div>
            )
          })}
          {configuredExchanges.length === 0 && (
            <div className="text-center py-10 border border-dashed border-border rounded-card bg-surface-alt">
              <Landmark className="w-8 h-8 mx-auto mb-3 text-muted-foreground/40" />
              <div className="t-body text-muted-foreground">
                {t('noExchangesConfigured', language)}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
