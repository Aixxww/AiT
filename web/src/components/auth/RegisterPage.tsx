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

  return (
    <DeepVoidBackground className="min-h-screen flex items-center justify-center py-12 font-mono">
      <div className="w-full max-w-lg relative z-10 px-6">
        <div className="flex justify-between items-center mb-8">
          <Button
            variant="unstyled"
            onClick={() => navigate('/')}
            className="flex items-center gap-2 text-muted-foreground hover:text-foreground transition-colors group px-3 py-1.5 rounded border border-transparent hover:border-border bg-black/20 backdrop-blur-sm"
          >
            <div className="w-2 h-2 rounded-full bg-loss group-hover:animate-pulse"></div>
            <span className="text-xs font-mono uppercase tracking-widest">
              &lt; ABORT_REGISTRATION
            </span>
          </Button>
        </div>

        <div className="mb-8 text-center">
          <div className="flex justify-center mb-6">
            <div className="relative">
              <div className="absolute -inset-2 bg-primary/20 rounded-full blur-xl animate-pulse"></div>
              <img
                src="/icons/ait.svg"
                alt="AiT Logo"
                className="w-16 h-16 object-contain relative z-10 opacity-90"
              />
            </div>
          </div>
          <h1 className="text-3xl font-bold tracking-tighter text-foreground uppercase mb-2">
            <span className="text-primary">NEW_USER</span> ONBOARDING
          </h1>
          <p className="text-muted-foreground text-xs tracking-[0.2em] uppercase">
            Initializing Registration Sequence...
          </p>
        </div>

        <div className="bg-surface/40 backdrop-blur-md border border-border rounded-lg overflow-hidden shadow-2xl relative group">
          <div className="absolute inset-0 bg-surface/50 opacity-0 group-hover:opacity-100 transition duration-700 pointer-events-none"></div>

          <div className="flex items-center justify-between px-4 py-2 bg-surface/80 border-b border-border">
            <div className="flex gap-1.5">
              <div
                className="w-2.5 h-2.5 rounded-full bg-loss/50 hover:bg-loss cursor-pointer transition-colors"
                onClick={() => navigate('/')}
                title="Close / Return Home"
              ></div>
              <div className="w-2.5 h-2.5 rounded-full bg-warning/50"></div>
              <div className="w-2.5 h-2.5 rounded-full bg-profit/50"></div>
            </div>
            <div className="text-[10px] text-muted-foreground font-mono flex items-center gap-1">
              <span className="text-profit">$</span> setup_account.sh
            </div>
          </div>

          <div className="p-6 md:p-8 relative">
            <div className="mb-6 font-mono text-xs space-y-1 text-muted-foreground border-b border-border/50 pb-4">
              <div className="flex gap-2">
                <span className="text-profit">$</span>
                <span>
                  System Check: <span className="text-profit">READY</span>
                </span>
              </div>
              <div className="flex gap-2">
                <span className="text-profit">$</span>
                <span>Mode: {betaMode ? 'CLOSED_BETA CA1' : 'PUBLIC'}</span>
              </div>
            </div>

            <form onSubmit={handleRegister} className="space-y-5">
              <div>
                <label className="block text-xs uppercase tracking-wider text-muted-foreground mb-1.5 ml-1 font-bold">
                  {t('email', language)}
                </label>
                <input
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="w-full bg-black/50 border border-border rounded px-4 py-3 text-sm focus:border-primary focus:ring-1 focus:ring-primary/50 outline-none transition-all placeholder-muted-foreground/40 text-foreground font-mono"
                  placeholder="user@ait.os"
                  required
                />
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs uppercase tracking-wider text-muted-foreground mb-1.5 ml-1 font-bold">
                    {t('password', language)}
                  </label>
                  <div className="relative">
                    <input
                      type={showPassword ? 'text' : 'password'}
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      className="w-full bg-black/50 border border-border rounded px-4 py-3 text-sm focus:border-primary focus:ring-1 focus:ring-primary/50 outline-none transition-all placeholder-muted-foreground/40 text-foreground font-mono pr-10"
                      placeholder="••••••••"
                      required
                    />
                    <Button
                      variant="unstyled"
                      type="button"
                      onClick={() => setShowPassword(!showPassword)}
                      className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-muted-foreground transition-colors"
                    >
                      {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                    </Button>
                  </div>
                </div>

                <div>
                  <label className="block text-xs uppercase tracking-wider text-muted-foreground mb-1.5 ml-1 font-bold">
                    {t('confirmPassword', language)}
                  </label>
                  <div className="relative">
                    <input
                      type={showConfirmPassword ? 'text' : 'password'}
                      value={confirmPassword}
                      onChange={(e) => setConfirmPassword(e.target.value)}
                      className="w-full bg-black/50 border border-border rounded px-4 py-3 text-sm focus:border-primary focus:ring-1 focus:ring-primary/50 outline-none transition-all placeholder-muted-foreground/40 text-foreground font-mono pr-10"
                      placeholder="••••••••"
                      required
                    />
                    <Button
                      variant="unstyled"
                      type="button"
                      onClick={() =>
                        setShowConfirmPassword(!showConfirmPassword)
                      }
                      className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-muted-foreground transition-colors"
                    >
                      {showConfirmPassword ? (
                        <EyeOff size={16} />
                      ) : (
                        <Eye size={16} />
                      )}
                    </Button>
                  </div>
                </div>
              </div>

              <div className="bg-surface/50 p-3 rounded border border-border/50">
                <div className="text-[10px] uppercase tracking-wider text-muted-foreground mb-2 font-bold flex items-center gap-2">
                  <div className="w-1 h-1 rounded-full bg-muted"></div>
                  Password Strength Protocol
                </div>
                <div className="text-xs font-mono text-muted-foreground">
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
                    className="grid grid-cols-2 gap-x-4 gap-y-1"
                    onChange={(isValid) => setPasswordValid(isValid)}
                    iconSize={10}
                  />
                </div>
              </div>

              {betaMode && (
                <div>
                  <label className="block text-xs uppercase tracking-wider text-primary mb-1.5 ml-1 font-bold">
                    Priority Access Code
                  </label>
                  <input
                    type="text"
                    value={betaCode}
                    onChange={(e) =>
                      setBetaCode(
                        e.target.value.replace(/[^a-z0-9]/gi, '').toLowerCase()
                      )
                    }
                    className="w-full bg-black/50 border border-border rounded px-4 py-3 text-sm focus:border-primary focus:ring-1 focus:ring-primary/50 outline-none transition-all placeholder-muted-foreground/40 text-foreground font-mono tracking-widest"
                    placeholder="XXXXXX"
                    maxLength={6}
                    required={betaMode}
                  />
                  <p className="text-[10px] text-muted-foreground font-mono mt-1 ml-1">
                    * CASE SENSITIVE ALPHANUMERIC
                  </p>
                </div>
              )}

              {error && (
                <div className="text-xs bg-loss/10 border border-loss/30 text-loss px-3 py-2 rounded font-mono">
                  [REGISTRATION_ERROR]: {error}
                </div>
              )}

              <Button
                variant="unstyled"
                type="submit"
                disabled={
                  loading || (betaMode && !betaCode.trim()) || !passwordValid
                }
                className="w-full bg-primary text-primary-foreground font-bold py-3 px-4 rounded text-sm tracking-wide uppercase hover:bg-primary/90 transition-all transform active:scale-[0.98] disabled:opacity-50 disabled:cursor-not-allowed font-mono flex items-center justify-center gap-2 group mt-4"
              >
                {loading ? (
                  <span className="animate-pulse">INITIALIZING...</span>
                ) : (
                  <>
                    <span>CREATE_ACCOUNT</span>
                    <span className="group-hover:translate-x-1 transition-transform">
                      -&gt;
                    </span>
                  </>
                )}
              </Button>
            </form>
          </div>

          <div className="bg-surface/50 p-3 flex justify-between items-center text-[10px] font-mono text-muted-foreground border-t border-border">
            <div>ENCRYPTION: AES-256</div>
            <div>SECURE_REGISTRY</div>
          </div>
        </div>

        <div className="text-center mt-8 space-y-4">
          <p className="text-xs font-mono text-muted-foreground">
            EXISTING_OPERATOR?{' '}
            <Button
              variant="unstyled"
              onClick={() => navigate('/login')}
              className="text-primary hover:underline hover:text-primary/80 transition-colors ml-1 uppercase"
            >
              ACCESS TERMINAL
            </Button>
          </p>
          <Button
            variant="unstyled"
            onClick={() => navigate('/')}
            className="text-[10px] text-muted-foreground hover:text-loss transition-colors uppercase tracking-widest hover:underline decoration-loss/30 font-mono"
          >
            [ ABORT_REGISTRATION_RETURN_HOME ]
          </Button>
        </div>
      </div>
    </DeepVoidBackground>
  )
}
