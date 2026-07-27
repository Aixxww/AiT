import { useState } from 'react'
import {
  TrendingUp,
  TrendingDown,
  Wallet,
  Pause,
  Hourglass,
  Bot,
  Settings,
  Inbox,
  Brain,
  Clipboard,
  Download,
  Lightbulb,
  AlertCircle,
  Check,
  X,
  Timer,
  type LucideIcon,
} from 'lucide-react'
import type { DecisionRecord, DecisionAction } from '../../types'
import { t, type Language } from '../../i18n/translations'
import { notify } from '../../lib/notify'
import { formatPrice as formatPriceRaw } from '../../utils/format'

interface DecisionCardProps {
  decision: DecisionRecord
  language: Language
  onSymbolClick?: (symbol: string) => void
}

// Action type configuration (semantic colors: profit/loss = direction only)
const ACTION_CONFIG: Record<
  string,
  { color: string; icon: LucideIcon; label: string }
> = {
  open_long: {
    color: 'var(--color-profit)',
    icon: TrendingUp,
    label: 'LONG',
  },
  open_short: {
    color: 'var(--color-loss)',
    icon: TrendingDown,
    label: 'SHORT',
  },
  close_long: {
    color: 'var(--color-primary)',
    icon: Wallet,
    label: 'CLOSE',
  },
  close_short: {
    color: 'var(--color-primary)',
    icon: Wallet,
    label: 'CLOSE',
  },
  hold: {
    color: 'var(--color-muted-fg)',
    icon: Pause,
    label: 'HOLD',
  },
  wait: {
    color: 'var(--color-muted-fg)',
    icon: Hourglass,
    label: 'WAIT',
  },
}

// Format price for display ('-' for absent values)
function formatPrice(price: number | undefined): string {
  if (!price || price === 0) return '-'
  return formatPriceRaw(price)
}

// Calculate percentage change
function calcPctChange(
  entry: number | undefined,
  target: number | undefined,
  isLong: boolean
): string {
  if (!entry || !target || entry === 0) return '-'
  const pct = ((target - entry) / entry) * 100
  const adjustedPct = isLong ? pct : -pct
  return `${adjustedPct >= 0 ? '+' : ''}${adjustedPct.toFixed(2)}%`
}

// Confidence is a quality grade, not a direction: neutral gray scale only
function getConfidenceColor(confidence: number | undefined): string {
  if (!confidence) return 'var(--color-muted-fg)'
  if (confidence >= 80) return 'var(--color-foreground)'
  if (confidence >= 60) return 'var(--color-muted-fg)'
  return 'var(--color-disabled-fg)'
}

// Single Action Card Component
function ActionCard({
  action,
  language,
  onSymbolClick,
}: {
  action: DecisionAction
  language: Language
  onSymbolClick?: (symbol: string) => void
}) {
  const config = ACTION_CONFIG[action.action] || ACTION_CONFIG.wait
  const ActionIcon = config.icon
  const isLong = action.action.includes('long')
  const isOpen = action.action.includes('open')

  return (
    <div
      className="rounded-lg border border-border bg-panel p-4"
      style={{
        borderLeft: `2px solid color-mix(in srgb, ${config.color} 55%, transparent)`,
      }}
    >
      {/* Header Row */}
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-3">
          <ActionIcon className="w-5 h-5" style={{ color: config.color }} />
          <span
            className="font-mono font-bold text-lg cursor-pointer transition-colors duration-200 text-foreground hover:text-primary"
            onClick={() => onSymbolClick?.(action.symbol)}
            title="Click to view chart"
          >
            {action.symbol.replace('USDT', '')}
          </span>
          <span
            className="px-3 py-1 rounded text-xs font-bold uppercase tracking-wider"
            style={{
              background: `color-mix(in srgb, ${config.color} 12%, transparent)`,
              color: config.color,
              border: `1px solid color-mix(in srgb, ${config.color} 33%, transparent)`,
            }}
          >
            {config.label}
          </span>
        </div>

        {/* Status Badge */}
        <div className="flex items-center gap-2">
          {action.confidence !== undefined && action.confidence > 0 && (
            <div
              className="px-2 py-1 rounded text-xs font-semibold font-mono bg-muted/40"
              style={{ color: getConfidenceColor(action.confidence) }}
            >
              {action.confidence.toFixed(0)}%
            </div>
          )}
          {action.success ? (
            <Check className="w-3.5 h-3.5 text-muted-foreground" />
          ) : (
            <X className="w-3.5 h-3.5 text-warning" />
          )}
        </div>
      </div>

      {/* Trading Details Grid */}
      {isOpen && (
        <div className="grid grid-cols-4 gap-3 mt-3 pt-3 border-t border-border">
          {/* Entry Price */}
          <div className="text-right">
            <div className="text-xs mb-1 text-muted-foreground">
              {t('entryPrice', language)}
            </div>
            <div className="font-mono font-semibold text-foreground">
              {formatPrice(action.price)}
            </div>
          </div>

          {/* Stop Loss */}
          <div className="text-right">
            <div className="text-xs mb-1 text-muted-foreground">
              {t('stopLoss', language)}
            </div>
            <div className="font-mono font-semibold text-loss">
              {formatPrice(action.stop_loss)}
            </div>
            {action.stop_loss && action.price && (
              <div className="text-xs mt-0.5 font-mono text-muted-foreground">
                {calcPctChange(action.price, action.stop_loss, isLong)}
              </div>
            )}
          </div>

          {/* Take Profit */}
          <div className="text-right">
            <div className="text-xs mb-1 text-muted-foreground">
              {t('takeProfit', language)}
            </div>
            <div className="font-mono font-semibold text-profit">
              {formatPrice(action.take_profit)}
            </div>
            {action.take_profit && action.price && (
              <div className="text-xs mt-0.5 font-mono text-muted-foreground">
                {calcPctChange(action.price, action.take_profit, isLong)}
              </div>
            )}
          </div>

          {/* Leverage */}
          <div className="text-right">
            <div className="text-xs mb-1 text-muted-foreground">
              {t('leverage', language)}
            </div>
            <div className="font-mono font-semibold text-foreground">
              {action.leverage}x
            </div>
          </div>
        </div>
      )}

      {/* Risk/Reward Ratio for open positions */}
      {isOpen && action.stop_loss && action.take_profit && action.price && (
        <div className="mt-3 pt-3 flex items-center justify-between border-t border-border">
          <span className="text-xs text-muted-foreground">
            {t('riskReward', language)}
          </span>
          <div className="flex items-center gap-2">
            {(() => {
              const slDist = Math.abs(action.price - action.stop_loss)
              const tpDist = Math.abs(action.take_profit - action.price)
              const ratio = slDist > 0 ? tpDist / slDist : 0
              return (
                <>
                  <div className="flex gap-1 font-mono text-xs">
                    <span className="text-loss">1</span>
                    <span className="text-muted-foreground">:</span>
                    <span className="text-profit">{ratio.toFixed(1)}</span>
                  </div>
                  <div className="h-1.5 w-[60px] rounded-full bg-muted/60">
                    <div
                      className="h-full rounded-full transition-all duration-300"
                      style={{
                        width: `${Math.min((ratio / 5) * 100, 100)}%`,
                        background:
                          ratio >= 2
                            ? 'var(--color-primary)'
                            : 'var(--color-muted-fg)',
                      }}
                    />
                  </div>
                </>
              )
            })()}
          </div>
        </div>
      )}

      {/* Reasoning */}
      {action.reasoning && (
        <div className="mt-3 pt-3 border-t border-border">
          <div className="text-xs line-clamp-2 text-muted-foreground flex items-start gap-1.5">
            <Lightbulb className="w-3.5 h-3.5 shrink-0 mt-px" />
            <span>{action.reasoning}</span>
          </div>
        </div>
      )}

      {/* Error Message */}
      {action.error && (
        <div className="mt-3 rounded p-2 text-xs bg-loss-bg border border-loss-border text-loss flex items-start gap-1.5">
          <AlertCircle className="w-3.5 h-3.5 shrink-0 mt-px" />
          <span>{action.error}</span>
        </div>
      )}
    </div>
  )
}

// Collapsible prompt/trace section without nested buttons (valid HTML)
function CollapsibleSection({
  icon: Icon,
  title,
  color,
  content,
  isOpen,
  onToggle,
  onCopy,
  onDownload,
  toggleLabel,
}: {
  icon: LucideIcon
  title: string
  color: string
  content: string
  isOpen: boolean
  onToggle: () => void
  onCopy: () => void
  onDownload: () => void
  toggleLabel: string
}) {
  return (
    <div>
      <div className="flex items-center gap-2 text-sm w-full justify-between p-2 rounded hover:bg-white/5">
        <button
          onClick={onToggle}
          className="flex flex-1 items-center gap-2 text-left"
        >
          <Icon className="w-4 h-4" style={{ color }} />
          <span className="font-semibold" style={{ color }}>
            {title}
          </span>
        </button>
        <div className="flex items-center gap-2">
          <button
            onClick={onCopy}
            className="text-xs px-2.5 py-1 rounded hover:opacity-80 transition-opacity flex items-center gap-1"
            style={{
              background: `color-mix(in srgb, ${color} 20%, transparent)`,
              color,
              border: `1px solid color-mix(in srgb, ${color} 30%, transparent)`,
            }}
            title="Copy to clipboard"
          >
            <Clipboard className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={onDownload}
            className="text-xs px-2.5 py-1 rounded hover:opacity-80 transition-opacity flex items-center gap-1"
            style={{
              background: `color-mix(in srgb, ${color} 20%, transparent)`,
              color,
              border: `1px solid color-mix(in srgb, ${color} 30%, transparent)`,
            }}
            title="Download as file"
          >
            <Download className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={onToggle}
            className="text-xs px-2 py-0.5 rounded"
            style={{
              background: `color-mix(in srgb, ${color} 15%, transparent)`,
              color,
            }}
          >
            {toggleLabel}
          </button>
        </div>
      </div>
      {isOpen && (
        <div className="mt-2 rounded-lg p-4 text-sm font-mono whitespace-pre-wrap max-h-96 overflow-y-auto bg-background border border-border text-foreground">
          {content}
        </div>
      )}
    </div>
  )
}

export function DecisionCard({
  decision,
  language,
  onSymbolClick,
}: DecisionCardProps) {
  const [showSystemPrompt, setShowSystemPrompt] = useState(false)
  const [showInputPrompt, setShowInputPrompt] = useState(false)
  const [showCoT, setShowCoT] = useState(false)

  // Copy text to clipboard
  const copyToClipboard = async (text: string, label: string) => {
    const localizedLabel =
      language === 'zh'
        ? {
            'System Prompt': '系统提示词',
            'User Prompt': '用户提示词',
            'AI Thinking': 'AI思维链分析',
          }[label] || label
        : label
    try {
      await navigator.clipboard.writeText(text)
      notify.success(
        language === 'zh'
          ? `${localizedLabel}已复制`
          : `${localizedLabel} copied`,
        {
          duration: 500,
        }
      )
    } catch (err) {
      console.error('Failed to copy:', err)
      notify.error(language === 'zh' ? '复制失败' : 'Copy failed', {
        duration: 1200,
      })
    }
  }

  // Download text as file
  const downloadAsFile = (text: string, filename: string) => {
    const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(url)
  }

  const toggleLabel = (open: boolean) =>
    open ? t('collapse', language) : t('expand', language)

  return (
    <div className="rounded-xl p-5 border border-border bg-panel shadow-sm">
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-lg flex items-center justify-center bg-primary-dim">
            <Bot className="w-5 h-5 text-primary" />
          </div>
          <div>
            <div className="font-bold text-foreground">
              {t('cycle', language)} #{decision.cycle_number}
            </div>
            <div className="text-xs text-muted-foreground font-mono">
              {new Date(decision.timestamp).toLocaleString()}
            </div>
          </div>
        </div>
        {/* Operation result: icon + neutral text (green/red stay reserved
            for direction and PnL) */}
        <div
          className={`px-3 py-1.5 rounded-full text-xs font-bold tracking-wider flex items-center gap-1.5 border ${
            decision.success
              ? 'border-border text-muted-foreground'
              : 'border-warning/40 text-warning bg-warning-bg'
          }`}
        >
          {decision.success ? (
            <Check className="w-3.5 h-3.5" />
          ) : (
            <X className="w-3.5 h-3.5" />
          )}
          {t(decision.success ? 'success' : 'failed', language)}
        </div>
      </div>

      {/* Decision Actions */}
      {decision.decisions && decision.decisions.length > 0 && (
        <div className="space-y-3 mb-4">
          {decision.decisions.map((action, index) => (
            <ActionCard
              key={`${action.symbol}-${index}`}
              action={action}
              language={language}
              onSymbolClick={onSymbolClick}
            />
          ))}
        </div>
      )}

      {/* Collapsible Sections */}
      <div className="space-y-2">
        {decision.system_prompt && (
          <CollapsibleSection
            icon={Settings}
            title="System Prompt"
            color="var(--color-purple)"
            content={decision.system_prompt}
            isOpen={showSystemPrompt}
            onToggle={() => setShowSystemPrompt(!showSystemPrompt)}
            onCopy={() =>
              copyToClipboard(decision.system_prompt, 'System Prompt')
            }
            onDownload={() =>
              downloadAsFile(
                decision.system_prompt,
                `system-prompt-cycle-${decision.cycle_number}.txt`
              )
            }
            toggleLabel={toggleLabel(showSystemPrompt)}
          />
        )}

        {decision.input_prompt && (
          <CollapsibleSection
            icon={Inbox}
            title="User Prompt"
            color="var(--color-info)"
            content={decision.input_prompt}
            isOpen={showInputPrompt}
            onToggle={() => setShowInputPrompt(!showInputPrompt)}
            onCopy={() => copyToClipboard(decision.input_prompt, 'User Prompt')}
            onDownload={() =>
              downloadAsFile(
                decision.input_prompt,
                `user-prompt-cycle-${decision.cycle_number}.txt`
              )
            }
            toggleLabel={toggleLabel(showInputPrompt)}
          />
        )}

        {decision.cot_trace && (
          <CollapsibleSection
            icon={Brain}
            title={t('aiThinking', language)}
            color="var(--color-primary)"
            content={decision.cot_trace}
            isOpen={showCoT}
            onToggle={() => setShowCoT(!showCoT)}
            onCopy={() => copyToClipboard(decision.cot_trace, 'AI Thinking')}
            onDownload={() =>
              downloadAsFile(
                decision.cot_trace,
                `ai-thinking-cycle-${decision.cycle_number}.txt`
              )
            }
            toggleLabel={toggleLabel(showCoT)}
          />
        )}
      </div>

      {/* Execution Log */}
      {decision.execution_log && decision.execution_log.length > 0 && (
        <div className="rounded-lg p-3 mt-4 text-xs font-mono space-y-1 bg-background border border-border">
          {decision.execution_log.map((log, index) => (
            <div key={`${log}-${index}`} className="text-foreground">
              {log}
            </div>
          ))}
        </div>
      )}

      {/* AI Token Usage Stats */}
      {decision.ai_request_duration_ms || decision.total_tokens ? (
        <div className="rounded-lg p-2 mt-2 text-xs font-mono bg-info-bg border border-info/20">
          <div className="flex items-center gap-4 flex-wrap text-muted-foreground">
            {decision.ai_request_duration_ms ? (
              <span className="flex items-center gap-1 text-info">
                <Timer className="w-3.5 h-3.5" />
                {(decision.ai_request_duration_ms / 1000).toFixed(1)}s
              </span>
            ) : null}
            {decision.prompt_tokens ? (
              <span>
                Prompt: {decision.prompt_tokens.toLocaleString()} tokens
              </span>
            ) : null}
            {decision.completion_tokens ? (
              <span>
                Output: {decision.completion_tokens.toLocaleString()} tokens
              </span>
            ) : null}
            {decision.total_tokens ? (
              <span className="text-foreground">
                Σ Total: {decision.total_tokens.toLocaleString()}
              </span>
            ) : null}
          </div>
        </div>
      ) : null}

      {/* Error Message */}
      {decision.error_message && (
        <div className="rounded-lg p-3 mt-4 text-sm bg-loss-bg border border-loss-border text-loss flex items-start gap-1.5">
          <AlertCircle className="w-4 h-4 shrink-0 mt-0.5" />
          <span>{decision.error_message}</span>
        </div>
      )}
    </div>
  )
}
