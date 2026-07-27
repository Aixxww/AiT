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
        label: language === 'zh' ? '未检查' : 'NOT CHECKED',
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
          label: language === 'zh' ? '已禁用' : 'DISABLED',
          className: 'text-muted-foreground border-border/80 bg-surface/40',
        }
      case 'missing_credentials':
        return {
          label: language === 'zh' ? '配置不完整' : 'INCOMPLETE',
          className: 'text-warning border-warning/20 bg-warning/10',
        }
      case 'invalid_credentials':
        return {
          label: language === 'zh' ? '密钥无效' : 'INVALID KEYS',
          className: 'text-loss border-loss/20 bg-loss/10',
        }
      case 'permission_denied':
        return {
          label: language === 'zh' ? '无余额权限' : 'NO PERMISSION',
          className: 'text-warning border-warning/20 bg-warning/10',
        }
      default:
        return {
          label: language === 'zh' ? '暂时无法获取' : 'UNAVAILABLE',
          className: 'text-foreground border-border bg-surface-alt',
        }
    }
  }

  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
      {/* AI Models Card */}
      <div className="ait-glass rounded-lg border border-white/5 overflow-hidden">
        <div className="px-4 py-3 border-b border-white/5 bg-black/20 flex items-center gap-2 backdrop-blur-sm">
          <Brain className="w-4 h-4 text-primary" />
          <h3 className="text-sm font-mono tracking-widest text-foreground uppercase">
            {t('aiModels', language)}
          </h3>
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
                    : 'hover:bg-white/5 hover:border-white/10 cursor-pointer'
                } bg-black/20`}
                onClick={() => onModelClick(model.id)}
              >
                <div className="flex items-center gap-4">
                  <div className="relative">
                    <div className="absolute inset-0 bg-info/20 rounded-full blur-sm group-hover:bg-info/30 transition-all"></div>
                    <div className="w-10 h-10 rounded-full flex items-center justify-center bg-black border border-white/10 relative z-10">
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
                    <div className="font-mono text-sm text-foreground group-hover:text-primary transition-colors">
                      {getShortName(model.name)}
                    </div>
                    <div className="text-[10px] text-muted-foreground font-mono flex items-center gap-2">
                      {model.customModelName ||
                        AI_PROVIDER_CONFIG[model.provider]?.defaultModel ||
                        ''}
                    </div>
                    {model.provider === 'claw402' &&
                    (model.balanceUsdc || model.walletAddress) ? (
                      <div className="mt-1.5 flex flex-wrap items-center gap-2 text-[10px] font-mono">
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
                      className={`text-[10px] font-mono px-2 py-1 rounded border ${
                        usageInfo.runningCount > 0
                          ? 'bg-profit/10 border-profit/30 text-profit'
                          : 'bg-warning/10 border-warning/30 text-warning'
                      }`}
                    >
                      {usageInfo.runningCount}/{usageInfo.totalCount} ACTIVE
                    </span>
                  ) : (
                    <span className="text-[10px] font-mono text-muted-foreground uppercase tracking-wider">
                      {language === 'zh' ? '就绪' : 'STANDBY'}
                    </span>
                  )}
                </div>
              </div>
            )
          })}

          {configuredModels.length === 0 && (
            <div className="text-center py-10 border border-dashed border-border rounded-lg bg-black/20">
              <Brain className="w-8 h-8 mx-auto mb-3 text-muted-foreground/40" />
              <div className="text-xs font-mono text-muted-foreground uppercase tracking-widest">
                {t('noModelsConfigured', language)}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Exchanges Card */}
      <div className="ait-glass rounded-lg border border-white/5 overflow-hidden">
        <div className="px-4 py-3 border-b border-white/5 bg-black/20 flex items-center gap-2 backdrop-blur-sm">
          <Landmark className="w-4 h-4 text-primary" />
          <h3 className="text-sm font-mono tracking-widest text-foreground uppercase">
            {t('exchanges', language)}
          </h3>
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
                    : 'hover:bg-white/5 hover:border-white/10 cursor-pointer'
                } bg-black/20`}
                onClick={() => onExchangeClick(exchange.id)}
              >
                <div className="flex items-center gap-4 min-w-0">
                  <div className="relative">
                    <div className="absolute inset-0 bg-warning/20 rounded-full blur-sm group-hover:bg-warning/30 transition-all"></div>
                    <div className="w-10 h-10 rounded-full flex items-center justify-center bg-black border border-white/10 relative z-10">
                      {getExchangeIcon(exchange.exchange_type || exchange.id, {
                        width: 20,
                        height: 20,
                      })}
                    </div>
                  </div>

                  <div className="min-w-0">
                    <div className="font-mono text-sm text-foreground group-hover:text-primary transition-colors truncate">
                      {exchange.exchange_type?.toUpperCase() ||
                        getShortName(exchange.name)}
                      <span className="text-[10px] text-muted-foreground ml-2 border border-border px-1 rounded">
                        {exchange.account_name || 'DEFAULT'}
                      </span>
                    </div>
                    <div className="text-[10px] text-muted-foreground font-mono flex items-center gap-2">
                      {exchange.type?.toUpperCase() || 'CEX'}
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-2 text-[10px] font-mono">
                      <span
                        className={`rounded border px-1.5 py-0.5 ${stateMeta.className}`}
                      >
                        {isExchangeAccountStatesLoading && !state
                          ? language === 'zh'
                            ? '检查中...'
                            : 'CHECKING...'
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
                        <span className="text-[10px] font-mono text-muted-foreground bg-black/40 px-1.5 py-0.5 rounded border border-border">
                          {isVisible ? walletAddr : truncateAddress(walletAddr)}
                        </span>
                        <button
                          onClick={(e) => {
                            e.stopPropagation()
                            onToggleExchangeAddress(exchange.id)
                          }}
                          className="text-muted-foreground hover:text-foreground"
                        >
                          {isVisible ? <EyeOff size={10} /> : <Eye size={10} />}
                        </button>
                        <button
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
                        </button>
                      </div>
                    )
                  })()}

                  {usageInfo.totalCount > 0 ? (
                    <span
                      className={`text-[10px] font-mono px-2 py-1 rounded border ${
                        usageInfo.runningCount > 0
                          ? 'bg-profit/10 border-profit/30 text-profit'
                          : 'bg-warning/10 border-warning/30 text-warning'
                      }`}
                    >
                      {usageInfo.runningCount}/{usageInfo.totalCount} ACTIVE
                    </span>
                  ) : (
                    <span className="text-[10px] font-mono text-muted-foreground uppercase tracking-wider">
                      {language === 'zh' ? '就绪' : 'STANDBY'}
                    </span>
                  )}
                </div>
              </div>
            )
          })}
          {configuredExchanges.length === 0 && (
            <div className="text-center py-10 border border-dashed border-border rounded-lg bg-black/20">
              <Landmark className="w-8 h-8 mx-auto mb-3 text-muted-foreground/40" />
              <div className="text-xs font-mono text-muted-foreground uppercase tracking-widest">
                {t('noExchangesConfigured', language)}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
