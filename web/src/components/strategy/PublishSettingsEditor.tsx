import { Globe, Lock, Eye, EyeOff } from 'lucide-react'
import { publishSettings, ts } from '../../i18n/strategy-translations'

interface PublishSettingsEditorProps {
  isPublic: boolean
  configVisible: boolean
  onIsPublicChange: (value: boolean) => void
  onConfigVisibleChange: (value: boolean) => void
  disabled?: boolean
  language: string
}

export function PublishSettingsEditor({
  isPublic,
  configVisible,
  onIsPublicChange,
  onConfigVisibleChange,
  disabled = false,
  language,
}: PublishSettingsEditorProps) {
  return (
    <div className="space-y-3">
      {/* Publish toggle */}
      <div
        className={`relative overflow-hidden rounded-lg transition-all duration-300 ${disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
        style={{
          background: isPublic
            ? 'linear-gradient(135deg, rgba(14, 203, 129, 0.15) 0%, rgba(14, 203, 129, 0.05) 100%)'
            : 'linear-gradient(135deg, var(--color-panel) 0%, var(--color-background) 100%)',
          border: isPublic
            ? '1px solid rgba(14, 203, 129, 0.4)'
            : '1px solid var(--color-border)',
          boxShadow: isPublic ? '0 0 20px rgba(14, 203, 129, 0.1)' : 'none',
        }}
        onClick={() => !disabled && onIsPublicChange(!isPublic)}
      >
        {/* Top glow line */}
        <div
          className="absolute top-0 left-0 w-full h-[1px] transition-opacity duration-300"
          style={{
            background: isPublic
              ? 'linear-gradient(90deg, transparent, var(--color-profit), transparent)'
              : 'linear-gradient(90deg, transparent, var(--color-border), transparent)',
            opacity: isPublic ? 1 : 0.5,
          }}
        />

        <div className="p-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div
              className="p-2.5 rounded-lg transition-all duration-300"
              style={{
                background: isPublic
                  ? 'rgba(14, 203, 129, 0.2)'
                  : 'var(--color-background)',
                border: isPublic
                  ? '1px solid rgba(14, 203, 129, 0.3)'
                  : '1px solid var(--color-border)',
              }}
            >
              {isPublic ? (
                <Globe
                  className="w-5 h-5"
                  style={{ color: 'var(--color-profit)' }}
                />
              ) : (
                <Lock className="w-5 h-5 text-muted-foreground" />
              )}
            </div>
            <div>
              <div className="text-sm font-medium text-foreground">
                {ts(publishSettings.publishToMarket, language)}
              </div>
              <div className="text-xs mt-0.5 text-muted-foreground">
                {ts(publishSettings.publishDesc, language)}
              </div>
            </div>
          </div>

          {/* Toggle with status */}
          <div className="flex items-center gap-3">
            <span
              className="text-[10px] font-mono font-bold tracking-wider"
              style={{
                color: isPublic
                  ? 'var(--color-profit)'
                  : 'var(--color-muted-fg)',
              }}
            >
              {isPublic
                ? ts(publishSettings.public, language)
                : ts(publishSettings.private, language)}
            </span>
            <div
              className="relative w-12 h-6 rounded-full transition-all duration-300"
              style={{
                background: isPublic
                  ? 'var(--color-profit)'
                  : 'var(--color-border)',
                boxShadow: isPublic
                  ? '0 0 10px rgba(14, 203, 129, 0.4)'
                  : 'none',
              }}
            >
              <div
                className="absolute top-1 w-4 h-4 rounded-full transition-all duration-300"
                style={{
                  background: 'var(--color-foreground)',
                  left: isPublic ? '28px' : '4px',
                  boxShadow: '0 2px 4px rgba(0,0,0,0.3)',
                }}
              />
            </div>
          </div>
        </div>
      </div>

      {/* Config visibility toggle - only shown when public */}
      {isPublic && (
        <div
          className={`relative overflow-hidden rounded-lg transition-all duration-300 ${disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
          style={{
            background: configVisible
              ? 'linear-gradient(135deg, rgba(168, 85, 247, 0.15) 0%, rgba(168, 85, 247, 0.05) 100%)'
              : 'linear-gradient(135deg, var(--color-panel) 0%, var(--color-background) 100%)',
            border: configVisible
              ? '1px solid rgba(168, 85, 247, 0.4)'
              : '1px solid var(--color-border)',
            boxShadow: configVisible
              ? '0 0 20px rgba(168, 85, 247, 0.1)'
              : 'none',
          }}
          onClick={() => !disabled && onConfigVisibleChange(!configVisible)}
        >
          {/* Top glow line */}
          <div
            className="absolute top-0 left-0 w-full h-[1px] transition-opacity duration-300"
            style={{
              background: configVisible
                ? 'linear-gradient(90deg, transparent, var(--color-purple), transparent)'
                : 'linear-gradient(90deg, transparent, var(--color-border), transparent)',
              opacity: configVisible ? 1 : 0.5,
            }}
          />

          <div className="p-4 flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div
                className="p-2.5 rounded-lg transition-all duration-300"
                style={{
                  background: configVisible
                    ? 'rgba(168, 85, 247, 0.2)'
                    : 'var(--color-background)',
                  border: configVisible
                    ? '1px solid rgba(168, 85, 247, 0.3)'
                    : '1px solid var(--color-border)',
                }}
              >
                {configVisible ? (
                  <Eye
                    className="w-5 h-5"
                    style={{ color: 'var(--color-purple)' }}
                  />
                ) : (
                  <EyeOff className="w-5 h-5 text-muted-foreground" />
                )}
              </div>
              <div>
                <div className="text-sm font-medium text-foreground">
                  {ts(publishSettings.showConfig, language)}
                </div>
                <div className="text-xs mt-0.5 text-muted-foreground">
                  {ts(publishSettings.showConfigDesc, language)}
                </div>
              </div>
            </div>

            {/* Toggle with status */}
            <div className="flex items-center gap-3">
              <span
                className="text-[10px] font-mono font-bold tracking-wider"
                style={{
                  color: configVisible
                    ? 'var(--color-purple)'
                    : 'var(--color-muted-fg)',
                }}
              >
                {configVisible
                  ? ts(publishSettings.visible, language)
                  : ts(publishSettings.hidden, language)}
              </span>
              <div
                className="relative w-12 h-6 rounded-full transition-all duration-300"
                style={{
                  background: configVisible
                    ? 'var(--color-purple)'
                    : 'var(--color-border)',
                  boxShadow: configVisible
                    ? '0 0 10px rgba(168, 85, 247, 0.4)'
                    : 'none',
                }}
              >
                <div
                  className="absolute top-1 w-4 h-4 rounded-full transition-all duration-300"
                  style={{
                    background: 'var(--color-foreground)',
                    left: configVisible ? '28px' : '4px',
                    boxShadow: '0 2px 4px rgba(0,0,0,0.3)',
                  }}
                />
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default PublishSettingsEditor
