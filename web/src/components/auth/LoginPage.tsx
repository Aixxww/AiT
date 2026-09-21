import React, { useState, useEffect, useRef } from 'react'
import { Eye, EyeOff, ShieldCheck, ArrowLeft, KeyRound } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { useAuth } from '../../contexts/AuthContext'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { DeepVoidBackground } from '../common/DeepVoidBackground'
import { LanguageSwitcher } from '../common/LanguageSwitcher'
import { Button } from '../ui/Button'
import { MfaCodeInput } from './MfaCodeInput'

/** The server keeps an MFA challenge for 5 minutes (see createMFAChallenge). */
const MFA_CHALLENGE_MS = 5 * 60 * 1000

function formatCountdown(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000))
  const minutes = Math.floor(total / 60)
  const seconds = total % 60
  return `${minutes}:${String(seconds).padStart(2, '0')}`
}

export function LoginPage() {
  const { language } = useLanguage()
  const { login, verifyMfa } = useAuth()
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [expiredToastId, setExpiredToastId] = useState<string | number | null>(
    null
  )

  // ── Second phase: authenticator / recovery code ────────────────────────────
  const [mfaToken, setMfaToken] = useState<string | null>(null)
  const [mfaCode, setMfaCode] = useState('')
  const [mfaError, setMfaError] = useState('')
  const [verifying, setVerifying] = useState(false)
  const [useRecovery, setUseRecovery] = useState(false)
  const [deadline, setDeadline] = useState<number | null>(null)
  const [now, setNow] = useState(() => Date.now())
  // Remaining guesses on the live challenge, as reported by the server.
  const [attemptsLeft, setAttemptsLeft] = useState<number | null>(null)
  // Bumped to remount the code boxes so focus returns to the first digit after
  // a rejected code (the boxes are derived from `value`, which we clear).
  const [attemptKey, setAttemptKey] = useState(0)
  // Guards against the code input's onComplete firing twice for one code.
  const submittedCode = useRef('')

  // Clean up stale auth state once on mount
  useEffect(() => {
    localStorage.removeItem('auth_token')
    localStorage.removeItem('auth_user')
    localStorage.removeItem('user_id')
  }, [])

  // Show session-expired toast (re-runs on language change to update text)
  useEffect(() => {
    if (sessionStorage.getItem('from401') === 'true') {
      const id = toast.warning(t('sessionExpired', language), {
        duration: Infinity,
      })
      setExpiredToastId(id)
      sessionStorage.removeItem('from401')
    }
  }, [language])

  // Tick the challenge countdown while the code step is on screen.
  useEffect(() => {
    if (!deadline) return
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [deadline])

  const remaining = deadline ? Math.max(0, deadline - now) : 0
  const challengeExpired = Boolean(deadline) && remaining <= 0

  const resetMfaState = () => {
    setMfaToken(null)
    setMfaCode('')
    setMfaError('')
    setUseRecovery(false)
    setDeadline(null)
    setAttemptsLeft(null)
    setAttemptKey(0)
    submittedCode.current = ''
  }

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    const result = await login(email, password)
    setLoading(false)

    // Password accepted, authenticator still pending — swap to the code step.
    if (result.mfaRequired && result.mfaToken) {
      setMfaToken(result.mfaToken)
      setMfaCode('')
      setMfaError('')
      setUseRecovery(false)
      setAttemptsLeft(null)
      setAttemptKey(0)
      submittedCode.current = ''
      setDeadline(Date.now() + MFA_CHALLENGE_MS)
      setNow(Date.now())
      return
    }

    if (result.success) {
      if (expiredToastId) toast.dismiss(expiredToastId)
    } else {
      const msg = result.message || t('loginFailed', language)
      setError(msg)
      toast.error(msg)
    }
  }

  const submitMfa = async (code: string) => {
    if (!mfaToken || verifying) return
    const trimmed = code.trim()
    if (!trimmed) return

    setMfaError('')
    setVerifying(true)
    const result = await verifyMfa(mfaToken, trimmed)
    setVerifying(false)

    if (result.success) {
      if (expiredToastId) toast.dismiss(expiredToastId)
      return
    }

    // The challenge survives a wrong code (the server only drops it once it is
    // redeemed, expires, or runs out of attempts), so a typo is retryable in
    // place. Only the terminal reasons send the user back through step one.
    //
    // A 429 without a reason comes from the shared IP limiter rather than from
    // the MFA handler — every auth endpoint is shut for that IP, so retrying is
    // pointless until the window rolls over. Treat it as terminal too.
    const rateLimited = result.status === 429 && !result.reason
    const terminal =
      rateLimited ||
      result.reason === 'expired' ||
      result.reason === 'unavailable' ||
      result.reason === 'too_many_attempts'

    if (!terminal) {
      setMfaCode('')
      setMfaError(result.message || t('mfa.invalidCodeRetry', language))
      setAttemptsLeft(
        typeof result.attemptsLeft === 'number' ? result.attemptsLeft : null
      )
      // Re-arm the input: remount clears focus state and lets the same code be
      // submitted again if the user simply mistyped one digit.
      submittedCode.current = ''
      setAttemptKey((k) => k + 1)
      return
    }

    const msg = rateLimited
      ? t('mfa.rateLimited', language)
      : result.reason === 'too_many_attempts'
        ? t('mfa.tooManyAttempts', language)
        : `${result.message || t('mfa.invalidCode', language)} ${t(
            'mfa.retryHint',
            language
          )}`
    resetMfaState()
    setError(msg)
    toast.error(msg)
  }

  const handleVerifySubmit = (e: React.FormEvent) => {
    e.preventDefault()
    void submitMfa(mfaCode)
  }

  const handleCodeComplete = (code: string) => {
    if (submittedCode.current === code) return
    submittedCode.current = code
    void submitMfa(code)
  }

  const backToCredentials = () => {
    resetMfaState()
    setError('')
  }

  const onMfaStep = Boolean(mfaToken)

  return (
    <DeepVoidBackground>
      <LanguageSwitcher />

      <div className="flex-1 flex items-center justify-center px-4 py-16">
        <div className="w-full max-w-sm">
          {/* Logo + Title */}
          <div className="text-center mb-10">
            <div className="flex justify-center mb-5">
              <div className="relative">
                <div className="absolute -inset-3 bg-primary/15 rounded-full blur-2xl" />
                <img
                  src="/icons/ait.svg"
                  alt="AiT"
                  className="w-14 h-14 relative z-10"
                />
              </div>
            </div>
            <h1 className="text-2xl font-bold text-foreground mb-1.5">
              {onMfaStep ? t('mfa.loginTitle', language) : 'Welcome back'}
            </h1>
            <p className="text-muted-foreground text-sm">
              {onMfaStep
                ? t('mfa.loginSubtitle', language)
                : 'Sign in to your account'}
            </p>
          </div>

          {/* Card */}
          <div className="bg-surface/60 backdrop-blur-xl border border-border/80 rounded-2xl p-8 shadow-2xl">
            {onMfaStep ? (
              <form onSubmit={handleVerifySubmit} className="space-y-5">
                <div className="flex justify-center">
                  <div className="w-12 h-12 rounded-full bg-primary/10 border border-primary/20 flex items-center justify-center">
                    {useRecovery ? (
                      <KeyRound size={20} className="text-primary" />
                    ) : (
                      <ShieldCheck size={20} className="text-primary" />
                    )}
                  </div>
                </div>

                {useRecovery ? (
                  <div>
                    <label className="block text-xs font-medium text-muted-foreground mb-2">
                      {t('mfa.recoveryLabel', language)}
                    </label>
                    <input
                      type="text"
                      value={mfaCode}
                      onChange={(e) => setMfaCode(e.target.value)}
                      disabled={verifying || challengeExpired}
                      autoFocus
                      autoComplete="off"
                      spellCheck={false}
                      className="w-full bg-background/80 border border-border/80 rounded-xl px-4 py-3 text-sm font-mono tracking-wider text-foreground placeholder-muted-foreground/60 focus:outline-none focus:border-primary/60 focus:ring-1 focus:ring-primary/30 transition-all disabled:opacity-50"
                      placeholder={t('mfa.recoveryPlaceholder', language)}
                    />
                  </div>
                ) : (
                  <MfaCodeInput
                    key={attemptKey}
                    value={mfaCode}
                    onChange={setMfaCode}
                    onComplete={handleCodeComplete}
                    disabled={verifying || challengeExpired}
                    autoFocus
                  />
                )}

                {/* Countdown */}
                <p className="text-center text-xs text-muted-foreground">
                  {challengeExpired ? (
                    <span className="text-loss">{t('mfa.expired', language)}</span>
                  ) : (
                    t('mfa.expiresIn', language, {
                      time: formatCountdown(remaining),
                    })
                  )}
                </p>

                {/* Error */}
                {mfaError && (
                  <p className="text-xs text-loss bg-loss/10 border border-loss/20 rounded-lg px-3 py-2">
                    {mfaError}
                  </p>
                )}

                {/* Guesses left before the challenge is burned */}
                {attemptsLeft !== null && attemptsLeft > 0 && (
                  <p className="text-center text-xs text-muted-foreground">
                    {t('mfa.attemptsLeft', language, { count: attemptsLeft })}
                  </p>
                )}

                {/* Submit */}
                <Button
                  variant="unstyled"
                  type="submit"
                  disabled={
                    verifying ||
                    challengeExpired ||
                    mfaCode.trim().length === 0 ||
                    (!useRecovery && mfaCode.length !== 6)
                  }
                  className="w-full bg-primary hover:bg-primary/90 active:scale-[0.98] text-black font-semibold py-3 rounded-xl text-sm transition-all disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {verifying
                    ? t('mfa.verifying', language)
                    : t('mfa.verify', language)}
                </Button>

                {/* Toggle recovery / authenticator */}
                <div className="flex flex-col items-center gap-2 pt-1">
                  <Button
                    variant="unstyled"
                    type="button"
                    onClick={() => {
                      setUseRecovery(!useRecovery)
                      setMfaCode('')
                      setMfaError('')
                      submittedCode.current = ''
                    }}
                    className="text-xs text-muted-foreground hover:text-primary transition-colors"
                  >
                    {useRecovery
                      ? t('mfa.useTotp', language)
                      : t('mfa.useRecovery', language)}
                  </Button>

                  <Button
                    variant="unstyled"
                    type="button"
                    onClick={backToCredentials}
                    className="inline-flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors"
                  >
                    <ArrowLeft size={12} />
                    {t('mfa.backToLogin', language)}
                  </Button>
                </div>
              </form>
            ) : (
              <form onSubmit={handleLogin} className="space-y-5">
                {/* Email */}
                <div>
                  <label className="block text-xs font-medium text-muted-foreground mb-2">
                    {t('email', language)}
                  </label>
                  <input
                    type="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    className="w-full bg-background/80 border border-border/80 rounded-xl px-4 py-3 text-sm text-foreground placeholder-muted-foreground/60 focus:outline-none focus:border-primary/60 focus:ring-1 focus:ring-primary/30 transition-all"
                    placeholder="you@example.com"
                    required
                    autoFocus
                  />
                </div>

                {/* Password */}
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <label className="text-xs font-medium text-muted-foreground">
                      {t('password', language)}
                    </label>
                    <Button
                      variant="unstyled"
                      type="button"
                      onClick={() => navigate('/reset-password')}
                      className="text-xs text-muted-foreground hover:text-primary transition-colors"
                    >
                      {t('forgotPassword', language)}
                    </Button>
                  </div>
                  <div className="relative">
                    <input
                      type={showPassword ? 'text' : 'password'}
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      className="w-full bg-background/80 border border-border/80 rounded-xl px-4 py-3 pr-11 text-sm text-foreground placeholder-muted-foreground/60 focus:outline-none focus:border-primary/60 focus:ring-1 focus:ring-primary/30 transition-all"
                      placeholder="••••••••"
                      required
                    />
                    <Button
                      variant="unstyled"
                      type="button"
                      onClick={() => setShowPassword(!showPassword)}
                      className="absolute right-3.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                    >
                      {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                    </Button>
                  </div>
                </div>

                {/* Error */}
                {error && (
                  <p className="text-xs text-loss bg-loss/10 border border-loss/20 rounded-lg px-3 py-2">
                    {error}
                  </p>
                )}

                {/* Submit */}
                <Button
                  variant="unstyled"
                  type="submit"
                  disabled={loading}
                  className="w-full bg-primary hover:bg-primary/90 active:scale-[0.98] text-black font-semibold py-3 rounded-xl text-sm transition-all disabled:opacity-50 disabled:cursor-not-allowed mt-2"
                >
                  {loading
                    ? t('loggingIn', language) || 'Signing in...'
                    : t('signIn', language) || 'Sign In'}
                </Button>
              </form>
            )}
          </div>
        </div>
      </div>
    </DeepVoidBackground>
  )
}
