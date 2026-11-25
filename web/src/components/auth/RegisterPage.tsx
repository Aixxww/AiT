import React, { useEffect, useState } from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import PasswordChecklist from 'react-password-checklist'
import { toast } from 'sonner'
import { useAuth } from '../../contexts/AuthContext'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { getSystemConfig } from '../../lib/config'
import { DeepVoidBackground } from '../common/DeepVoidBackground'
import { RegistrationDisabled } from './RegistrationDisabled'
import { WhitelistFullPage } from '../common/WhitelistFullPage'
import { Button } from '../ui/Button'
import { InfoTip } from '../ui/InfoTip'
import { LanguageSwitcher } from '../common/LanguageSwitcher'

export function RegisterPage() {
  const { language } = useLanguage()
  const { register } = useAuth()
  const navigate = useNavigate()
  const [view, setView] = useState<'register' | 'whitelist-full'>('register')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [betaCode, setBetaCode] = useState('')
  const [betaMode, setBetaMode] = useState(false)
  const [registrationEnabled, setRegistrationEnabled] = useState(true)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [passwordValid, setPasswordValid] = useState(false)
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirmPassword, setShowConfirmPassword] = useState(false)

  useEffect(() => {
    getSystemConfig()
      .then((config) => {
        setBetaMode(config.beta_mode || false)
        setRegistrationEnabled(config.initialized === false)
      })
      .catch((err) => {
        console.error('Failed to fetch system config:', err)
      })
  }, [])

  if (!registrationEnabled) {
    return <RegistrationDisabled />
  }

  if (view === 'whitelist-full') {
    return <WhitelistFullPage onBack={() => setView('register')} />
  }

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')

    if (!passwordValid) {
      setError(t('passwordNotMeetRequirements', language))
      return
    }

    if (betaMode && !betaCode.trim()) {
      setError('内测期间，注册需要提供内测码')
      return
    }

    setLoading(true)
    try {
      const result = await register(
        email,
        password,
        betaCode.trim() || undefined
      )

      const isWhitelistError = (msg: string) => {
        const lowerMsg = msg.toLowerCase()
        return (
          lowerMsg.includes('whitelist') ||
          lowerMsg.includes('capacity') ||
          lowerMsg.includes('limit') ||
          lowerMsg.includes('permission denied') ||
          lowerMsg.includes('not on whitelist')
        )
      }

      if (!result.success) {
        const msg = result.message || t('registrationFailed', language)
        if (isWhitelistError(msg)) {
          setView('whitelist-full')
          return
        }
        setError(msg)
        toast.error(msg)
      }
      // success path is handled in AuthContext (auto login + navigation)
    } catch (e) {
      console.error('Registration error:', e)
      const errorMsg =
        e instanceof Error
          ? e.message
          : 'Registration failed due to server error'
      const lowerMsg = errorMsg.toLowerCase()
      if (
        lowerMsg.includes('whitelist') ||
        lowerMsg.includes('capacity') ||
        lowerMsg.includes('limit') ||
        lowerMsg.includes('permission denied') ||
        lowerMsg.includes('not on whitelist')
      ) {
        setView('whitelist-full')
        return
      }
      setError(errorMsg)
      toast.error(errorMsg)
    } finally {
      setLoading(false)
    }
  }

  const inputClass =
    'w-full bg-background/80 border border-border/80 rounded-xl px-4 py-3 text-sm text-foreground placeholder-muted-foreground/60 focus:outline-none focus:border-primary/60 focus:ring-1 focus:ring-primary/30 transition-all'
  const pwToggle = (shown: boolean, toggle: () => void) => (
    <Button
      variant="unstyled"
      type="button"
      onClick={toggle}
      aria-label={
        shown
          ? t('auth.hidePassword', language)
          : t('auth.showPassword', language)
      }
      className="absolute right-3.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
    >
      {shown ? <EyeOff size={16} /> : <Eye size={16} />}
    </Button>
  )

  return (
    <DeepVoidBackground>
      <LanguageSwitcher />
      <div className="flex-1 flex items-center justify-center px-4 py-16">
        <div className="w-full max-w-md">
          <div className="text-center mb-10">
            <div className="flex justify-center mb-5">
              <img src="/icons/ait.svg" alt="" className="w-14 h-14" />
            </div>
            <h1 className="t-title mb-1.5">
              {t('auth.registerTitle', language)}
            </h1>
            <p className="t-caption text-muted-foreground">
              {t('auth.registerSubtitle', language)}
            </p>
          </div>

          <div className="ait-card p-6 sm:p-8 shadow-e2">
            <form onSubmit={handleRegister} className="space-y-5">
              <div>
                <label
                  htmlFor="register-email"
                  className="block t-caption text-muted-foreground mb-2"
                >
                  {t('email', language)}
                </label>
                <input
                  id="register-email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className={inputClass}
                  placeholder="you@example.com"
                  required
                />
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <label
                    htmlFor="register-password"
                    className="block t-caption text-muted-foreground mb-2"
                  >
                    {t('password', language)}
                  </label>
                  <div className="relative">
                    <input
                      id="register-password"
                      type={showPassword ? 'text' : 'password'}
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      className={inputClass + ' pr-11'}
                      placeholder="••••••••"
                      required
                    />
                    {pwToggle(showPassword, () =>
                      setShowPassword(!showPassword)
                    )}
                  </div>
                </div>
                <div>
                  <label
                    htmlFor="register-confirm"
                    className="block t-caption text-muted-foreground mb-2"
                  >
                    {t('confirmPassword', language)}
                  </label>
                  <div className="relative">
                    <input
                      id="register-confirm"
                      type={showConfirmPassword ? 'text' : 'password'}
                      value={confirmPassword}
                      onChange={(e) => setConfirmPassword(e.target.value)}
                      className={inputClass + ' pr-11'}
                      placeholder="••••••••"
                      required
                    />
                    {pwToggle(showConfirmPassword, () =>
                      setShowConfirmPassword(!showConfirmPassword)
                    )}
                  </div>
                </div>
              </div>

              <div className="ait-card-inset p-3">
                <div className="t-micro text-muted-foreground mb-2">
                  {t('auth.passwordRules', language)}
                </div>
                <div className="t-caption text-muted-foreground">
                  <PasswordChecklist
                    rules={[
                      'minLength',
                      'capital',
                      'lowercase',
                      'number',
                      'specialChar',
                      'match',
                    ]}
                    minLength={8}
                    value={password}
                    valueAgain={confirmPassword}
                    messages={{
                      minLength: t('passwordRuleMinLength', language),
                      capital: t('passwordRuleUppercase', language),
                      lowercase: t('passwordRuleLowercase', language),
                      number: t('passwordRuleNumber', language),
                      specialChar: t('passwordRuleSpecial', language),
                      match: t('passwordRuleMatch', language),
                    }}
                    className="grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-1"
                    onChange={(isValid) => setPasswordValid(isValid)}
                    iconSize={10}
                  />
                </div>
              </div>

              {betaMode && (
                <div>
                  <div className="flex items-center gap-1.5 mb-2">
                    <label
                      htmlFor="register-beta"
                      className="t-caption text-muted-foreground"
                    >
                      {t('auth.betaCode', language)}
                    </label>
                    <InfoTip content={t('auth.betaCodeInfo', language)} />
                  </div>
                  <input
                    id="register-beta"
                    type="text"
                    value={betaCode}
                    onChange={(e) =>
                      setBetaCode(
                        e.target.value.replace(/[^a-z0-9]/gi, '').toLowerCase()
                      )
                    }
                    className={inputClass + ' t-num tracking-widest'}
                    placeholder="••••••"
                    maxLength={6}
                    required={betaMode}
                  />
                  <p className="t-micro text-muted-foreground mt-1">
                    {t('auth.betaCodeHint', language)}
                  </p>
                </div>
              )}

              {error && (
                <p className="text-xs text-loss bg-loss/10 border border-loss/20 rounded-lg px-3 py-2">
                  {error}
                </p>
              )}

              <Button
                variant="unstyled"
                type="submit"
                disabled={
                  loading || (betaMode && !betaCode.trim()) || !passwordValid
                }
                className="w-full bg-primary hover:opacity-90 active:scale-[0.98] text-primary-foreground font-semibold py-3 rounded-xl text-sm transition-all disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {loading
                  ? t('auth.creating', language)
                  : t('auth.createAccount', language)}
              </Button>
            </form>
          </div>

          <div className="text-center mt-6 space-y-2 t-caption text-muted-foreground">
            <p>
              {t('auth.haveAccount', language)}{' '}
              <Button
                variant="unstyled"
                onClick={() => navigate('/login')}
                className="text-primary hover:underline"
              >
                {t('auth.goSignIn', language)}
              </Button>
            </p>
            <Button
              variant="unstyled"
              onClick={() => navigate('/')}
              className="hover:text-foreground"
            >
              {t('auth.backHome', language)}
            </Button>
          </div>
        </div>
      </div>
    </DeepVoidBackground>
  )
}
