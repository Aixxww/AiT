import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import { QRCodeSVG } from 'qrcode.react'
import {
  ShieldCheck,
  ShieldOff,
  Copy,
  Check,
  Download,
  Loader2,
  AlertTriangle,
  KeyRound,
} from 'lucide-react'
import { useAuth } from '../../contexts/AuthContext'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { Button } from '../ui/Button'
import { MfaCodeInput } from '../auth/MfaCodeInput'
import { mfaApi, normalizeCode, type MfaSetupPayload } from '../../lib/mfa'

type Phase = 'idle' | 'setup' | 'codes' | 'disable'

/** Writes `text` to the clipboard, with a fallback for non-secure contexts. */
async function writeClipboard(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    /* fall through to the legacy path */
  }
  try {
    const area = document.createElement('textarea')
    area.value = text
    area.setAttribute('readonly', '')
    area.style.position = 'fixed'
    area.style.top = '-1000px'
    area.style.opacity = '0'
    document.body.appendChild(area)
    area.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(area)
    return ok
  } catch {
    return false
  }
}

function buildRecoveryFile(email: string | undefined, codes: string[]): string {
  const header = [
    'AiT — two-factor authentication recovery codes',
    email ? `Account: ${email}` : '',
    `Generated: ${new Date().toISOString()}`,
    '',
    'Each code can be used once, in place of an authenticator code.',
    'Keep this file somewhere safe and offline.',
    '',
  ].filter(Boolean)

  const body = codes.map((code, i) => `${String(i + 1).padStart(2, ' ')}. ${code}`)
  return [...header, ...body, ''].join('\n')
}

export function SecuritySettings() {
  const { language } = useLanguage()
  const { user, refreshSessionToken } = useAuth()

  const [status, setStatus] = useState<boolean | null>(null)
  const [loadError, setLoadError] = useState('')
  const [phase, setPhase] = useState<Phase>('idle')
  const [error, setError] = useState('')

  const [setup, setSetup] = useState<MfaSetupPayload | null>(null)
  const [setupCode, setSetupCode] = useState('')
  const [setupBusy, setSetupBusy] = useState(false)

  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null)

  const [disableCode, setDisableCode] = useState('')
  const [disableBusy, setDisableBusy] = useState(false)

  const [copied, setCopied] = useState<string | null>(null)

  const loadStatus = useCallback(async () => {
    setLoadError('')
    const result = await mfaApi.status()
    if (result.ok) {
      setStatus(result.data.enabled)
    } else {
      setStatus(null)
      setLoadError(result.error)
    }
  }, [])

  useEffect(() => {
    void loadStatus()
  }, [loadStatus])

  const flashCopied = (tag: string) => {
    setCopied(tag)
    window.setTimeout(() => {
      setCopied((current) => (current === tag ? null : current))
    }, 2000)
  }

  const copy = async (text: string, tag: string) => {
    const ok = await writeClipboard(text)
    if (ok) {
      flashCopied(tag)
    } else {
      toast.error(t('mfa.copyFailed', language))
    }
  }

  const startSetup = async () => {
    setError('')
    const result = await mfaApi.setup()
    if (!result.ok) {
      setError(result.error || t('mfa.setupFailed', language))
      return
    }
    setSetup(result.data)
    setSetupCode('')
    setPhase('setup')
  }

  const cancelSetup = () => {
    setSetup(null)
    setSetupCode('')
    setError('')
    setPhase('idle')
  }

  const confirmEnable = async (code: string) => {
    if (!setup || setupBusy) return
    const value = normalizeCode(code)
    if (value.length !== 6) return

    setError('')
    setSetupBusy(true)
    const result = await mfaApi.enable(setup.secret, value)
    setSetupBusy(false)

    if (!result.ok) {
      setError(result.error)
      setSetupCode('')
      return
    }

    // The server rotates the JWT on enable — keep the stored one in step.
    if (result.data.token) {
      refreshSessionToken(result.data.token)
    }
    setRecoveryCodes(result.data.recovery_codes ?? [])
    setSetup(null)
    setSetupCode('')
    setStatus(true)
    setPhase('codes')
    toast.success(t('mfa.enabledToast', language))
  }

  const finishRecoveryCodes = () => {
    setRecoveryCodes(null)
    setPhase('idle')
  }

  const confirmDisable = async (e: FormEvent) => {
    e.preventDefault()
    if (disableBusy) return
    const value = normalizeCode(disableCode)
    if (!value) return

    setError('')
    setDisableBusy(true)
    const result = await mfaApi.disable(value)
    setDisableBusy(false)

    if (!result.ok) {
      setError(result.error)
      return
    }

    setDisableCode('')
    setStatus(false)
    setPhase('idle')
    toast.success(t('mfa.disabledToast', language))
  }

  // ── Loading ────────────────────────────────────────────────────────────────
  if (status === null) {
    return (
      <div className="space-y-4">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 size={16} className="animate-spin" />
          {t('mfa.settingsTitle', language)}
        </div>
        {loadError && (
          <div className="flex items-start gap-2 text-xs text-loss bg-loss/10 border border-loss/20 rounded-lg px-3 py-2">
            <AlertTriangle size={14} className="mt-0.5 shrink-0" />
            <span className="flex-1">{loadError}</span>
            <Button
              variant="unstyled"
              type="button"
              onClick={() => void loadStatus()}
              className="underline hover:no-underline"
            >
              {t('mfa.retry', language)}
            </Button>
          </div>
        )}
      </div>
    )
  }

  // ── Recovery codes (one-time reveal) ───────────────────────────────────────
  if (phase === 'codes' && recoveryCodes) {
    return (
      <div className="space-y-5">
        <div className="flex items-start gap-3">
          <div className="w-10 h-10 rounded-xl flex items-center justify-center border bg-warning/10 border-warning/20">
            <KeyRound size={18} className="text-warning" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-foreground">
              {t('mfa.codesTitle', language)}
            </h3>
            <p className="text-xs text-muted-foreground mt-1">
              {t('mfa.codesHint', language)}
            </p>
          </div>
        </div>

        <div className="grid grid-cols-2 gap-2">
          {recoveryCodes.map((code) => (
            <div
              key={code}
              className="font-mono text-xs tracking-wider text-foreground bg-surface-alt border border-border rounded-lg px-3 py-2 text-center select-all"
            >
              {code}
            </div>
          ))}
        </div>

        <p className="text-xs text-warning flex items-center gap-1.5">
          <AlertTriangle size={13} className="shrink-0" />
          {t('mfa.codesWarning', language)}
        </p>

        <div className="flex flex-wrap gap-2">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => void copy(recoveryCodes.join('\n'), 'all')}
          >
            {copied === 'all' ? <Check size={14} /> : <Copy size={14} />}
            {copied === 'all'
              ? t('mfa.copied', language)
              : t('mfa.copyAll', language)}
          </Button>

          <Button
            variant="secondary"
            size="sm"
            onClick={() => {
              const blob = new Blob(
                [buildRecoveryFile(user?.email, recoveryCodes)],
                { type: 'text/plain;charset=utf-8' }
              )
              const url = URL.createObjectURL(blob)
              const link = document.createElement('a')
              link.href = url
              link.download = 'ait-recovery-codes.txt'
              document.body.appendChild(link)
              link.click()
              document.body.removeChild(link)
              URL.revokeObjectURL(url)
            }}
          >
            <Download size={14} />
            {t('mfa.download', language)}
          </Button>

          <Button variant="primary" size="sm" onClick={finishRecoveryCodes}>
            {t('mfa.codesDone', language)}
          </Button>
        </div>
      </div>
    )
  }

  // ── Setup: scan QR, then confirm ───────────────────────────────────────────
  if (phase === 'setup' && setup) {
    return (
      <div className="space-y-5">
        <div>
          <h3 className="text-sm font-semibold text-foreground mb-1">
            {t('mfa.scanTitle', language)}
          </h3>
          <p className="text-xs text-muted-foreground">
            {t('mfa.scanHint', language)}
          </p>
        </div>

        {/* Always rendered dark-on-white: a themed QR would be unscannable. */}
        <div className="flex justify-center">
          <div className="bg-white rounded-xl p-4 border border-border">
            <QRCodeSVG
              value={setup.otpauth_uri}
              size={168}
              level="M"
              marginSize={0}
              bgColor="#ffffff"
              fgColor="#000000"
            />
          </div>
        </div>

        <div>
          <p className="text-xs text-muted-foreground mb-2">
            {t('mfa.manualTitle', language)}
          </p>
          <div className="flex items-center gap-2">
            <code className="flex-1 font-mono text-xs tracking-wider text-foreground bg-surface-alt border border-border rounded-lg px-3 py-2 break-all">
              {setup.secret}
            </code>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => void copy(setup.secret, 'secret')}
            >
              {copied === 'secret' ? <Check size={14} /> : <Copy size={14} />}
              {copied === 'secret'
                ? t('mfa.copied', language)
                : t('mfa.copy', language)}
            </Button>
          </div>
        </div>

        <div className="border-t border-border pt-5">
          <h3 className="text-sm font-semibold text-foreground mb-1">
            {t('mfa.confirmTitle', language)}
          </h3>
          <p className="text-xs text-muted-foreground mb-4">
            {t('mfa.confirmHint', language)}
          </p>

          <div className="flex flex-col items-center gap-4">
            <MfaCodeInput
              value={setupCode}
              onChange={setSetupCode}
              onComplete={(code) => void confirmEnable(code)}
              disabled={setupBusy}
            />

            {error && (
              <p className="w-full text-xs text-loss bg-loss/10 border border-loss/20 rounded-lg px-3 py-2">
                {error}
              </p>
            )}

            <div className="flex gap-2 w-full">
              <Button
                variant="secondary"
                size="md"
                className="flex-1"
                onClick={cancelSetup}
                disabled={setupBusy}
              >
                {t('mfa.cancel', language)}
              </Button>
              <Button
                variant="primary"
                size="md"
                className="flex-1"
                onClick={() => void confirmEnable(setupCode)}
                disabled={setupBusy || setupCode.length !== 6}
              >
                {setupBusy && <Loader2 size={14} className="animate-spin" />}
                {setupBusy
                  ? t('mfa.enabling', language)
                  : t('mfa.confirmEnable', language)}
              </Button>
            </div>
          </div>
        </div>
      </div>
    )
  }

  // ── Disable ────────────────────────────────────────────────────────────────
  if (phase === 'disable') {
    return (
      <form onSubmit={confirmDisable} className="space-y-5">
        <div>
          <h3 className="text-sm font-semibold text-foreground mb-1">
            {t('mfa.disableTitle', language)}
          </h3>
          <p className="text-xs text-muted-foreground">
            {t('mfa.disableHint', language)}
          </p>
        </div>

        <div className="flex justify-center">
          <MfaCodeInput
            value={disableCode}
            onChange={setDisableCode}
            onComplete={(code) => {
              if (disableBusy) return
              setDisableCode(code)
            }}
            disabled={disableBusy}
            autoFocus
          />
        </div>

        {error && (
          <p className="text-xs text-loss bg-loss/10 border border-loss/20 rounded-lg px-3 py-2">
            {error}
          </p>
        )}

        <div className="flex gap-2">
          <Button
            variant="secondary"
            size="md"
            className="flex-1"
            onClick={() => {
              setPhase('idle')
              setDisableCode('')
              setError('')
            }}
            disabled={disableBusy}
          >
            {t('mfa.cancel', language)}
          </Button>
          <Button
            variant="danger"
            size="md"
            className="flex-1"
            type="submit"
            disabled={disableBusy || disableCode.length !== 6}
          >
            {disableBusy && <Loader2 size={14} className="animate-spin" />}
            {disableBusy
              ? t('mfa.disabling', language)
              : t('mfa.confirmDisable', language)}
          </Button>
        </div>
      </form>
    )
  }

  // ── Idle: status summary ───────────────────────────────────────────────────
  const enabled = status === true

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <div
            className={`w-10 h-10 rounded-xl flex items-center justify-center border shrink-0 ${
              enabled
                ? 'bg-profit/10 border-profit/20'
                : 'bg-surface-alt border-border'
            }`}
          >
            {enabled ? (
              <ShieldCheck size={18} className="text-profit" />
            ) : (
              <ShieldOff size={18} className="text-muted-foreground" />
            )}
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-semibold text-foreground">
                {t('mfa.settingsTitle', language)}
              </h3>
              <span
                className={`text-[11px] px-2 py-0.5 rounded-full ${
                  enabled
                    ? 'bg-profit/10 text-profit'
                    : 'bg-surface-alt text-muted-foreground'
                }`}
              >
                {enabled
                  ? t('mfa.statusOn', language)
                  : t('mfa.statusOff', language)}
              </span>
            </div>
            <p className="text-xs text-muted-foreground mt-1.5 max-w-md leading-relaxed">
              {enabled
                ? t('mfa.descOn', language)
                : t('mfa.descOff', language)}
            </p>
          </div>
        </div>

        <Button
          variant={enabled ? 'secondary' : 'primary'}
          size="sm"
          className="shrink-0"
          onClick={() => {
            setError('')
            if (enabled) {
              setDisableCode('')
              setPhase('disable')
            } else {
              void startSetup()
            }
          }}
        >
          {enabled ? t('mfa.turnOff', language) : t('mfa.setup', language)}
        </Button>
      </div>

      {error && (
        <p className="text-xs text-loss bg-loss/10 border border-loss/20 rounded-lg px-3 py-2">
          {error}
        </p>
      )}
    </div>
  )
}
