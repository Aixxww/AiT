import { useState } from 'react'
import type { DecisionRecord, DecisionAction } from '../../types'
import { t, type Language } from '../../i18n/translations'

interface DecisionCardProps {
  decision: DecisionRecord
  language: Language
  onSymbolClick?: (symbol: string) => void
}

// Action type configuration
const ACTION_CONFIG: Record<string, { color: string; bg: string; icon: string; label: string }> = {
  open_long: { color: 'var(--color-profit)', bg: 'color-mix(in srgb, var(--color-profit) 15%, transparent)', icon: '📈', label: 'LONG' },
  open_short: { color: 'var(--color-loss)', bg: 'color-mix(in srgb, var(--color-loss) 15%, transparent)', icon: '📉', label: 'SHORT' },
  close_long: { color: 'var(--color-primary)', bg: 'color-mix(in srgb, var(--color-primary) 15%, transparent)', icon: '💰', label: 'CLOSE' },
  close_short: { color: 'var(--color-primary)', bg: 'color-mix(in srgb, var(--color-primary) 15%, transparent)', icon: '💰', label: 'CLOSE' },
  hold: { color: 'var(--color-muted-fg)', bg: 'rgba(132, 142, 156, 0.15)', icon: '⏸️', label: 'HOLD' },
  wait: { color: 'var(--color-muted-fg)', bg: 'rgba(132, 142, 156, 0.15)', icon: '⏳', label: 'WAIT' },
}

// Format price with proper decimals
function formatPrice(price: number | undefined): string {
  if (!price || price === 0) return '-'
  if (price >= 1000) return price.toFixed(2)
  if (price >= 1) return price.toFixed(4)
  return price.toFixed(6)
}

// Calculate percentage change
function calcPctChange(entry: number | undefined, target: number | undefined, isLong: boolean): string {
  if (!entry || !target || entry === 0) return '-'
  const pct = ((target - entry) / entry) * 100
  const adjustedPct = isLong ? pct : -pct
  return `${adjustedPct >= 0 ? '+' : ''}${adjustedPct.toFixed(2)}%`
}

// Get confidence color
function getConfidenceColor(confidence: number | undefined): string {
  if (!confidence) return 'var(--color-muted-fg)'
  if (confidence >= 80) return 'var(--color-profit)'
  if (confidence >= 60) return 'var(--color-primary)'
  return 'var(--color-loss)'
}

// Single Action Card Component
function ActionCard({ action, language, onSymbolClick }: { action: DecisionAction; language: Language; onSymbolClick?: (symbol: string) => void }) {
  const config = ACTION_CONFIG[action.action] || ACTION_CONFIG.wait
  const isLong = action.action.includes('long')
  const isOpen = action.action.includes('open')

  return (
    <div
      className="rounded-lg p-4 transition-all duration-200 hover:scale-[1.01]"
      style={{
        background: 'linear-gradient(135deg, var(--color-panel) 0%, #181C21 100%)',
        border: `1px solid ${config.color}33`,
        boxShadow: `0 4px 12px rgba(0, 0, 0, 0.2), inset 0 1px 0 rgba(255, 255, 255, 0.03)`,
      }}
    >
      {/* Header Row */}
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-3">
          <span className="text-xl">{config.icon}</span>
          <span
            className="font-mono font-bold text-lg cursor-pointer transition-all duration-200 hover:scale-110 text-foreground"
            onClick={() => onSymbolClick?.(action.symbol)}
            title="Click to view chart"
          >
            {action.symbol.replace('USDT', '')}
          </span>
          <span
            className="px-3 py-1 rounded-full text-xs font-bold uppercase tracking-wider"
            style={{ background: config.bg, color: config.color, border: `1px solid ${config.color}55` }}
          >
            {config.label}
          </span>
        </div>

        {/* Status Badge */}
        <div className="flex items-center gap-2">
          {action.confidence !== undefined && action.confidence > 0 && (
            <div
              className="px-2 py-1 rounded text-xs font-semibold"
              style={{
                background: `${getConfidenceColor(action.confidence)}22`,
                color: getConfidenceColor(action.confidence)
              }}
            >
              {action.confidence.toFixed(0)}%
            </div>
          )}
          <div
            className="w-2 h-2 rounded-full"
            style={{ background: action.success ? 'var(--color-profit)' : 'var(--color-loss)' }}
          />
        </div>
      </div>

      {/* Trading Details Grid */}
      {isOpen && (
        <div className="grid grid-cols-4 gap-3 mt-3 pt-3" style={{ borderTop: '1px solid var(--color-border)' }}>
          {/* Entry Price */}
          <div className="text-center">
            <div className="text-xs mb-1 text-muted-foreground">
              {t('entryPrice', language)}
            </div>
            <div className="font-mono font-semibold text-foreground">
              {formatPrice(action.price)}
            </div>
          </div>

          {/* Stop Loss */}
          <div className="text-center">
            <div className="text-xs mb-1" style={{ color: 'var(--color-loss)' }}>
              {t('stopLoss', language)}
            </div>
            <div className="font-mono font-semibold" style={{ color: 'var(--color-loss)' }}>
              {formatPrice(action.stop_loss)}
            </div>
            {action.stop_loss && action.price && (
              <div className="text-xs mt-0.5 text-muted-foreground">
                {calcPctChange(action.price, action.stop_loss, isLong)}
              </div>
            )}
          </div>

          {/* Take Profit */}
          <div className="text-center">
            <div className="text-xs mb-1" style={{ color: 'var(--color-profit)' }}>
              {t('takeProfit', language)}
            </div>
            <div className="font-mono font-semibold" style={{ color: 'var(--color-profit)' }}>
              {formatPrice(action.take_profit)}
            </div>
            {action.take_profit && action.price && (
              <div className="text-xs mt-0.5 text-muted-foreground">
                {calcPctChange(action.price, action.take_profit, isLong)}
              </div>
            )}
          </div>

          {/* Leverage */}
          <div className="text-center">
            <div className="text-xs mb-1 text-muted-foreground">
              {t('leverage', language)}
            </div>
            <div className="font-mono font-semibold" style={{ color: 'var(--color-primary)' }}>
              {action.leverage}x
            </div>
          </div>
        </div>
      )}

      {/* Risk/Reward Ratio for open positions */}
      {isOpen && action.stop_loss && action.take_profit && action.price && (
        <div className="mt-3 pt-3 flex items-center justify-between" style={{ borderTop: '1px solid var(--color-border)' }}>
          <span className="text-xs text-muted-foreground">{t('riskReward', language)}</span>
          <div className="flex items-center gap-2">
            {(() => {
              const slDist = Math.abs(action.price - action.stop_loss)
              const tpDist = Math.abs(action.take_profit - action.price)
              const ratio = slDist > 0 ? (tpDist / slDist) : 0
              const ratioColor = ratio >= 3 ? 'var(--color-profit)' : ratio >= 2 ? 'var(--color-primary)' : 'var(--color-loss)'
              return (
                <>
                  <div className="flex gap-1">
                    <span style={{ color: 'var(--color-loss)' }}>1</span>
                    <span className="text-muted-foreground">:</span>
                    <span style={{ color: 'var(--color-profit)' }}>{ratio.toFixed(1)}</span>
                  </div>
                  <div
                    className="h-1.5 rounded-full"
                    style={{
                      width: '60px',
                      background: 'var(--color-border)',
                    }}
                  >
                    <div
                      className="h-full rounded-full transition-all duration-300"
                      style={{
                        width: `${Math.min(ratio / 5 * 100, 100)}%`,
                        background: ratioColor
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
        <div className="mt-3 pt-3" style={{ borderTop: '1px solid var(--color-border)' }}>
          <div className="text-xs line-clamp-2 text-muted-foreground">
            💡 {action.reasoning}
          </div>
        </div>
      )}

      {/* Error Message */}
      {action.error && (
        <div
          className="mt-3 rounded p-2 text-xs"
          style={{
            background: 'rgba(246, 70, 93, 0.1)',
            border: '1px solid rgba(246, 70, 93, 0.3)',
            color: 'var(--color-loss)',
          }}
        >
          ❌ {action.error}
        </div>
      )}
    </div>
  )
}

export function DecisionCard({ decision, language, onSymbolClick }: DecisionCardProps) {
  const [showSystemPrompt, setShowSystemPrompt] = useState(false)
  const [showInputPrompt, setShowInputPrompt] = useState(false)
  const [showCoT, setShowCoT] = useState(false)

  // Copy text to clipboard
  const copyToClipboard = async (text: string, label: string) => {
    try {
      await navigator.clipboard.writeText(text)
      alert(`${label} copied!`)
    } catch (err) {
      console.error('Failed to copy:', err)
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

  return (
    <div
      className="rounded-xl p-5 transition-all duration-300 hover:translate-y-[-2px]"
      style={{
        border: '1px solid var(--color-border)',
        background: 'linear-gradient(180deg, var(--color-panel) 0%, #181C21 100%)',
        boxShadow: '0 4px 16px rgba(0, 0, 0, 0.3)',
      }}
    >
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <div
            className="w-10 h-10 rounded-lg flex items-center justify-center"
            style={{ background: 'color-mix(in srgb, var(--color-primary) 15%, transparent)' }}
          >
            <span className="text-xl">🤖</span>
          </div>
          <div>
            <div className="font-bold text-foreground">
              {t('cycle', language)} #{decision.cycle_number}
            </div>
            <div className="text-xs text-muted-foreground">
              {new Date(decision.timestamp).toLocaleString()}
            </div>
          </div>
        </div>
        <div
          className="px-4 py-1.5 rounded-full text-xs font-bold tracking-wider"
          style={
            decision.success
              ? { background: 'color-mix(in srgb, var(--color-profit) 15%, transparent)', color: 'var(--color-profit)', border: '1px solid color-mix(in srgb, var(--color-profit) 30%, transparent)' }
              : { background: 'color-mix(in srgb, var(--color-loss) 15%, transparent)', color: 'var(--color-loss)', border: '1px solid color-mix(in srgb, var(--color-loss) 30%, transparent)' }
          }
        >
          {t(decision.success ? 'success' : 'failed', language)}
        </div>
      </div>

      {/* Decision Actions - Beautiful Grid */}
      {decision.decisions && decision.decisions.length > 0 && (
        <div className="space-y-3 mb-4">
          {decision.decisions.map((action, index) => (
            <ActionCard key={`${action.symbol}-${index}`} action={action} language={language} onSymbolClick={onSymbolClick} />
          ))}
        </div>
      )}

      {/* Collapsible Sections */}
      <div className="space-y-2">
        {/* System Prompt */}
        {decision.system_prompt && (
          <div>
            <button
              onClick={() => setShowSystemPrompt(!showSystemPrompt)}
              className="flex items-center gap-2 text-sm transition-colors w-full justify-between p-2 rounded hover:bg-white/5"
            >
              <div className="flex items-center gap-2">
                <span className="text-base">⚙️</span>
                <span className="font-semibold" style={{ color: '#a78bfa' }}>
                  System Prompt
                </span>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    copyToClipboard(decision.system_prompt, 'System Prompt')
                  }}
                  className="text-xs px-2.5 py-1 rounded hover:opacity-80 transition-opacity flex items-center gap-1"
                  style={{ background: 'rgba(167, 139, 250, 0.2)', color: '#a78bfa', border: '1px solid rgba(167, 139, 250, 0.3)' }}
                  title="Copy to clipboard"
                >
                  <span>📋</span>
                </button>
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    downloadAsFile(decision.system_prompt, `system-prompt-cycle-${decision.cycle_number}.txt`)
                  }}
                  className="text-xs px-2.5 py-1 rounded hover:opacity-80 transition-opacity flex items-center gap-1"
                  style={{ background: 'rgba(167, 139, 250, 0.2)', color: '#a78bfa', border: '1px solid rgba(167, 139, 250, 0.3)' }}
                  title="Download as file"
                >
                  <span>💾</span>
                </button>
                <span
                  className="text-xs px-2 py-0.5 rounded"
                  style={{ background: 'rgba(167, 139, 250, 0.15)', color: '#a78bfa' }}
                >
                  {showSystemPrompt ? t('collapse', language) : t('expand', language)}
                </span>
              </div>
            </button>
            {showSystemPrompt && (
              <div
                className="mt-2 rounded-lg p-4 text-sm font-mono whitespace-pre-wrap max-h-96 overflow-y-auto"
                style={{
                  background: 'var(--background)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--foreground)',
                }}
              >
                {decision.system_prompt}
              </div>
            )}
          </div>
        )}

        {/* User/Input Prompt */}
        {decision.input_prompt && (
          <div>
            <button
              onClick={() => setShowInputPrompt(!showInputPrompt)}
              className="flex items-center gap-2 text-sm transition-colors w-full justify-between p-2 rounded hover:bg-white/5"
            >
              <div className="flex items-center gap-2">
                <span className="text-base">📥</span>
                <span className="font-semibold" style={{ color: '#60a5fa' }}>
                  User Prompt
                </span>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    copyToClipboard(decision.input_prompt, 'User Prompt')
                  }}
                  className="text-xs px-2.5 py-1 rounded hover:opacity-80 transition-opacity flex items-center gap-1"
                  style={{ background: 'rgba(96, 165, 250, 0.2)', color: '#60a5fa', border: '1px solid rgba(96, 165, 250, 0.3)' }}
                  title="Copy to clipboard"
                >
                  <span>📋</span>
                </button>
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    downloadAsFile(decision.input_prompt, `user-prompt-cycle-${decision.cycle_number}.txt`)
                  }}
                  className="text-xs px-2.5 py-1 rounded hover:opacity-80 transition-opacity flex items-center gap-1"
                  style={{ background: 'rgba(96, 165, 250, 0.2)', color: '#60a5fa', border: '1px solid rgba(96, 165, 250, 0.3)' }}
                  title="Download as file"
                >
                  <span>💾</span>
                </button>
                <span
                  className="text-xs px-2 py-0.5 rounded"
                  style={{ background: 'rgba(96, 165, 250, 0.15)', color: '#60a5fa' }}
                >
                  {showInputPrompt ? t('collapse', language) : t('expand', language)}
                </span>
              </div>
            </button>
            {showInputPrompt && (
              <div
                className="mt-2 rounded-lg p-4 text-sm font-mono whitespace-pre-wrap max-h-96 overflow-y-auto"
                style={{
                  background: 'var(--background)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--foreground)',
                }}
              >
                {decision.input_prompt}
              </div>
            )}
          </div>
        )}

        {/* AI Thinking */}
        {decision.cot_trace && (
          <div>
            <button
              onClick={() => setShowCoT(!showCoT)}
              className="flex items-center gap-2 text-sm transition-colors w-full justify-between p-2 rounded hover:bg-white/5"
            >
              <div className="flex items-center gap-2">
                <span className="text-base">🧠</span>
                <span className="font-semibold" style={{ color: 'var(--color-primary)' }}>
                  {t('aiThinking', language)}
                </span>
              </div>
              <span
                className="text-xs px-2 py-0.5 rounded"
                style={{ background: 'color-mix(in srgb, var(--color-primary) 15%, transparent)', color: 'var(--color-primary)' }}
              >
                {showCoT ? t('collapse', language) : t('expand', language)}
              </span>
            </button>
            {showCoT && (
              <div
                className="mt-2 rounded-lg p-4 text-sm font-mono whitespace-pre-wrap max-h-96 overflow-y-auto"
                style={{
                  background: 'var(--background)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--foreground)',
                }}
              >
                {decision.cot_trace}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Execution Log */}
      {decision.execution_log && decision.execution_log.length > 0 && (
        <div
          className="rounded-lg p-3 mt-4 text-xs font-mono space-y-1"
          style={{ background: 'var(--background)', border: '1px solid var(--color-border)' }}
        >
          {decision.execution_log.map((log, index) => (
            <div key={`${log}-${index}`} className="text-foreground">
              {log}
            </div>
          ))}
        </div>
      )}

      {/* AI Token Usage Stats */}
      {(decision.ai_request_duration_ms || decision.total_tokens) ? (
        <div
          className="rounded-lg p-2 mt-2 text-xs font-mono"
          style={{ background: 'rgba(30, 154, 247, 0.08)', border: '1px solid rgba(30, 154, 247, 0.2)' }}
        >
          <div className="flex items-center gap-4 flex-wrap">
            {decision.ai_request_duration_ms ? (
              <span style={{ color: '#1e9af7' }}>⏱ {(decision.ai_request_duration_ms / 1000).toFixed(1)}s</span>
            ) : null}
            {decision.prompt_tokens ? (
              <span style={{ color: 'var(--color-primary)' }}>📥 Prompt: {decision.prompt_tokens.toLocaleString()} tokens</span>
            ) : null}
            {decision.completion_tokens ? (
              <span style={{ color: 'var(--color-profit)' }}>📤 Output: {decision.completion_tokens.toLocaleString()} tokens</span>
            ) : null}
            {decision.total_tokens ? (
              <span style={{ color: '#8899aa' }}>Σ Total: {decision.total_tokens.toLocaleString()}</span>
            ) : null}
          </div>
        </div>
      ) : null}

      {/* Error Message */}
      {decision.error_message && (
        <div
          className="rounded-lg p-3 mt-4 text-sm"
          style={{
            background: 'rgba(246, 70, 93, 0.1)',
            border: '1px solid rgba(246, 70, 93, 0.4)',
            color: 'var(--color-loss)',
          }}
        >
          ❌ {decision.error_message}
        </div>
      )}
    </div>
  )
}
