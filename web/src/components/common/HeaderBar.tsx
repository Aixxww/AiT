import { useState, useEffect, useRef } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { motion, AnimatePresence } from 'framer-motion'
import { Menu, X, ChevronDown, Settings, Sun, Moon } from 'lucide-react'
import { useTheme, type ThemeMode } from '../../contexts/ThemeContext'
import { t, type Language } from '../../i18n/translations'
import {
  getPostAuthPath,
  getUserMode,
  setUserMode,
  type UserMode,
} from '../../lib/onboarding'
import { getCurrentPageForPath, ROUTES, type Page } from '../../router/paths'
import { Button } from '../ui/Button'

interface HeaderBarProps {
  onLoginClick?: () => void
  isLoggedIn?: boolean
  isHomePage?: boolean
  currentPage?: Page
  language?: Language
  onLanguageChange?: (lang: Language) => void
  user?: { email: string } | null
  onLogout?: () => void
  onPageChange?: (page: Page) => void
  onLoginRequired?: (featureName: string) => void
}

export default function HeaderBar({
  isLoggedIn = false,
  isHomePage = false,
  currentPage,
  language = 'zh' as Language,
  onLanguageChange,
  user,
  onLogout,
  onPageChange,
  onLoginRequired,
}: HeaderBarProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const [languageDropdownOpen, setLanguageDropdownOpen] = useState(false)
  const [userDropdownOpen, setUserDropdownOpen] = useState(false)
  const [userMode, setUserModeState] = useState<UserMode>(
    () => getUserMode() ?? 'advanced'
  )
  const { scheme, style, setMode } = useTheme()
  const dropdownRef = useRef<HTMLDivElement>(null)
  const userDropdownRef = useRef<HTMLDivElement>(null)
  const resolvedCurrentPage =
    currentPage ?? getCurrentPageForPath(location.pathname)

  const navigateInApp = (path: string) => {
    navigate(path)
  }

  const handleSwitchMode = (nextMode: UserMode) => {
    setUserMode(nextMode)
    setUserModeState(nextMode)
    setUserDropdownOpen(false)
    navigateInApp(getPostAuthPath(nextMode))
  }

  const toggleColorScheme = () => {
    const nextScheme = scheme === 'dark' ? 'light' : 'dark'
    setMode(`${style}-${nextScheme}` as ThemeMode)
  }
  // Close dropdown when clicking outside
  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(event.target as Node)
      ) {
        setLanguageDropdownOpen(false)
      }
      if (
        userDropdownRef.current &&
        !userDropdownRef.current.contains(event.target as Node)
      ) {
        setUserDropdownOpen(false)
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [])

  return (
    <nav className="fixed left-0 top-0 w-full z-50 header-bar">
      <div className="flex items-center justify-between h-16 px-3 sm:px-5 lg:px-6 max-w-[1920px] mx-auto">
        {/* Logo - Always go to home page */}
        <div
          onClick={() => {
            navigateInApp(ROUTES.home)
          }}
          className="flex shrink-0 items-center gap-2 hover:opacity-80 transition-opacity cursor-pointer"
        >
          <img src="/icons/ait.svg" alt="AiT Logo" className="w-7 h-7" />
          <span className="text-lg font-bold text-primary">AiT</span>
        </div>

        {/* Desktop Menu */}
        <div className="hidden xl:flex items-center justify-between flex-1 min-w-0 ml-6">
          {/* Left Side - Navigation Tabs - Always show all tabs */}
          <div className="flex items-center gap-1 min-w-0 overflow-x-auto pr-3 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
            {/* Navigation tabs configuration */}
            {(() => {
              // Define all navigation tabs
              const navTabs: {
                page: Page
                path: string
                label: string
                requiresAuth: boolean
                badge?: string
                hidden?: boolean
              }[] = [
                {
                  page: 'agent',
                  path: ROUTES.agent,
                  label: 'Agent',
                  requiresAuth: false,
                  badge: 'Beta',
                  hidden: true,
                },
                {
                  page: 'data',
                  path: ROUTES.data,
                  label:
                    language === 'zh'
                      ? '数据'
                      : language === 'id'
                        ? 'Data'
                        : 'Data',
                  requiresAuth: false,
                },
                {
                  page: 'strategy-market',
                  path: ROUTES.strategyMarket,
                  label:
                    language === 'zh'
                      ? '策略市场'
                      : language === 'id'
                        ? 'Pasar'
                        : 'Market',
                  requiresAuth: true,
                },
                {
                  page: 'traders',
                  path: ROUTES.traders,
                  label: t('configNav', language),
                  requiresAuth: true,
                },
                {
                  page: 'trader',
                  path: ROUTES.dashboard,
                  label: t('dashboardNav', language),
                  requiresAuth: true,
                },
                {
                  page: 'strategy',
                  path: ROUTES.strategy,
                  label: t('strategyNav', language),
                  requiresAuth: true,
                },
                {
                  page: 'backtest',
                  path: ROUTES.backtest,
                  label:
                    language === 'zh'
                      ? '回测'
                      : language === 'id'
                        ? 'Backtest'
                        : 'Backtest',
                  requiresAuth: true,
                },
                {
                  page: 'competition',
                  path: ROUTES.competition,
                  label: t('realtimeNav', language),
                  requiresAuth: true,
                },
                {
                  page: 'faq',
                  path: ROUTES.faq,
                  label: t('faqNav', language),
                  requiresAuth: false,
                },
              ]

              const handleNavClick = (tab: (typeof navTabs)[0]) => {
                // If requires auth and not logged in, show login prompt
                if (tab.requiresAuth && !isLoggedIn) {
                  onLoginRequired?.(tab.label)
                  return
                }
                // Navigate normally
                if (onPageChange) {
                  onPageChange(tab.page)
                }
                navigateInApp(tab.path)
              }

              return navTabs
                .filter((tab) => !tab.hidden)
                .map((tab) => (
                  <Button
                    variant="unstyled"
                    key={tab.page}
                    onClick={() => handleNavClick(tab)}
                    className={`whitespace-nowrap text-sm font-bold transition-all duration-300 relative focus:outline-2 focus:outline-primary px-3 py-2 rounded-lg
                    ${resolvedCurrentPage === tab.page ? 'text-primary' : 'text-muted-foreground hover:text-primary'}`}
                  >
                    {resolvedCurrentPage === tab.page && (
                      <span className="absolute inset-0 rounded-lg bg-primary/15 -z-10" />
                    )}
                    {tab.label}
                    {tab.badge && (
                      <span className="ml-1 text-[10px] px-1.5 py-0.5 rounded-full bg-primary/20 text-primary font-semibold uppercase align-top relative -top-1">
                        {tab.badge}
                      </span>
                    )}
                  </Button>
                ))
            })()}
          </div>

          {/* Right Side - User Actions */}
          <div className="flex shrink-0 items-center gap-3">
            {/* User Info and Actions */}
            {isLoggedIn && user ? (
              <div className="flex items-center gap-3">
                {/* User Info with Dropdown */}
                <div className="relative" ref={userDropdownRef}>
                  <Button
                    variant="unstyled"
                    onClick={() => setUserDropdownOpen(!userDropdownOpen)}
                    className="flex max-w-[260px] items-center gap-2 px-3 py-2 rounded-lg transition-colors bg-surface border border-primary/20 hover:bg-primary/5"
                  >
                    <div className="w-6 h-6 rounded-full flex items-center justify-center text-xs font-bold bg-primary text-primary-foreground">
                      {user.email[0].toUpperCase()}
                    </div>
                    <span className="truncate text-sm text-muted-foreground">
                      {user.email}
                    </span>
                    <ChevronDown className="w-4 h-4 text-muted-foreground" />
                  </Button>

                  {userDropdownOpen && (
                    <div className="absolute right-0 top-full mt-2 w-48 rounded-lg shadow-lg overflow-hidden z-50 bg-surface border border-primary/20">
                      <div className="px-3 py-2 border-b border-primary/20">
                        <div className="text-xs text-muted-foreground">
                          {t('loggedInAs', language)}
                        </div>
                        <div className="text-sm font-medium text-muted-foreground">
                          {user.email}
                        </div>
                      </div>
                      <Button
                        variant="unstyled"
                        onClick={() => {
                          navigateInApp(ROUTES.settings)
                          setUserDropdownOpen(false)
                        }}
                        className="w-full flex items-center gap-2 px-3 py-2 text-sm transition-colors hover:bg-primary/5 text-muted-foreground hover:text-foreground"
                      >
                        <Settings className="w-3.5 h-3.5" />
                        Settings
                      </Button>
                      <Button
                        variant="unstyled"
                        onClick={() =>
                          handleSwitchMode(
                            userMode === 'beginner' ? 'advanced' : 'beginner'
                          )
                        }
                        className="w-full flex items-center gap-2 px-3 py-2 text-sm transition-colors hover:bg-primary/5 text-muted-foreground hover:text-foreground"
                      >
                        <Settings className="w-3.5 h-3.5" />
                        {userMode === 'beginner'
                          ? language === 'zh'
                            ? '切到老手模式'
                            : 'Switch to Advanced'
                          : language === 'zh'
                            ? '切到新手模式'
                            : 'Switch to Beginner'}
                      </Button>
                      {onLogout && (
                        <Button
                          variant="unstyled"
                          onClick={() => {
                            onLogout()
                            setUserDropdownOpen(false)
                          }}
                          className="w-full px-3 py-2 text-sm font-semibold transition-colors hover:opacity-80 text-center bg-loss/20 text-loss"
                        >
                          {t('exitLogin', language)}
                        </Button>
                      )}
                    </div>
                  )}
                </div>
              </div>
            ) : (
              /* Show login/register buttons when not logged in and not on login/register pages */
              resolvedCurrentPage !== 'login' &&
              resolvedCurrentPage !== 'register' && (
                <div className="flex items-center gap-3">
                  <Button
                    variant="unstyled"
                    type="button"
                    onClick={() => navigateInApp(ROUTES.login)}
                    className="px-3 py-2 text-sm font-medium transition-colors rounded text-muted-foreground hover:text-foreground"
                  >
                    {t('signIn', language)}
                  </Button>
                </div>
              )
            )}

            {/* Theme Toggle */}
            <Button
              variant="unstyled"
              type="button"
              onClick={toggleColorScheme}
              className="inline-flex h-10 w-10 items-center justify-center rounded-lg border border-border bg-surface text-muted-foreground hover:border-border-hover hover:text-foreground"
              aria-label={
                scheme === 'dark'
                  ? 'Switch to light theme'
                  : 'Switch to dark theme'
              }
              title={scheme === 'dark' ? 'Light theme' : 'Dark theme'}
            >
              {scheme === 'dark' ? (
                <Sun className="h-4 w-4" />
              ) : (
                <Moon className="h-4 w-4" />
              )}
            </Button>

            {/* Language Toggle - Always at the rightmost */}
            <div className="relative" ref={dropdownRef}>
              <Button
                variant="unstyled"
                onClick={() => setLanguageDropdownOpen(!languageDropdownOpen)}
                className="flex items-center gap-2 px-3 py-2 rounded-lg transition-colors text-muted-foreground hover:bg-primary/5"
              >
                <span className="text-lg">
                  {language === 'zh' ? '🇨🇳' : language === 'id' ? '🇮🇩' : '🇺🇸'}
                </span>
                <ChevronDown className="w-4 h-4" />
              </Button>

              {languageDropdownOpen && (
                <div className="absolute right-0 top-full mt-2 w-32 rounded-lg shadow-lg overflow-hidden z-50 bg-surface border border-primary/20">
                  <Button
                    variant="unstyled"
                    onClick={() => {
                      onLanguageChange?.('zh')
                      setLanguageDropdownOpen(false)
                    }}
                    className={`w-full flex items-center gap-2 px-3 py-2 transition-colors text-muted-foreground hover:text-foreground
                      ${language === 'zh' ? 'bg-primary/10' : 'hover:bg-primary/5'}`}
                  >
                    <span className="text-base">🇨🇳</span>
                    <span className="text-sm">中文</span>
                  </Button>
                  <Button
                    variant="unstyled"
                    onClick={() => {
                      onLanguageChange?.('en')
                      setLanguageDropdownOpen(false)
                    }}
                    className={`w-full flex items-center gap-2 px-3 py-2 transition-colors text-muted-foreground hover:text-foreground
                      ${language === 'en' ? 'bg-primary/10' : 'hover:bg-primary/5'}`}
                  >
                    <span className="text-base">🇺🇸</span>
                    <span className="text-sm">English</span>
                  </Button>
                  <Button
                    variant="unstyled"
                    onClick={() => {
                      onLanguageChange?.('id')
                      setLanguageDropdownOpen(false)
                    }}
                    className={`w-full flex items-center gap-2 px-3 py-2 transition-colors text-muted-foreground hover:text-foreground
                      ${language === 'id' ? 'bg-primary/10' : 'hover:bg-primary/5'}`}
                  >
                    <span className="text-base">🇮🇩</span>
                    <span className="text-sm">Bahasa</span>
                  </Button>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Mobile Menu Button */}
        <motion.button
          onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
          className="xl:hidden inline-flex h-10 w-10 items-center justify-center rounded-lg border border-border bg-surface text-muted-foreground hover:text-foreground"
          whileTap={{ scale: 0.9 }}
        >
          {mobileMenuOpen ? (
            <X className="w-6 h-6" />
          ) : (
            <Menu className="w-6 h-6" />
          )}
        </motion.button>
      </div>

      {/* Mobile Menu Overlay */}
      <AnimatePresence>
        {mobileMenuOpen && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.2 }}
            className="ait-mobile-menu-overlay fixed inset-0 z-40 xl:hidden backdrop-blur-xl"
            style={{ top: '64px' }} // Below header
          >
            <motion.div
              initial={{ y: -20, opacity: 0 }}
              animate={{ y: 0, opacity: 1 }}
              transition={{ delay: 0.1, duration: 0.3 }}
              className="flex h-[calc(100vh-64px)] flex-col overflow-y-auto px-4 py-6 sm:px-6"
            >
              {/* Navigation Links */}
              <div className="flex flex-col gap-3 mb-10">
                {(() => {
                  const navTabs: {
                    page: Page
                    path: string
                    label: string
                    requiresAuth: boolean
                    badge?: string
                    hidden?: boolean
                  }[] = [
                    {
                      page: 'agent',
                      path: ROUTES.agent,
                      label: 'Agent',
                      requiresAuth: false,
                      badge: 'Beta',
                      hidden: true,
                    },
                    {
                      page: 'data',
                      path: ROUTES.data,
                      label:
                        language === 'zh'
                          ? '数据'
                          : language === 'id'
                            ? 'Data'
                            : 'Data',
                      requiresAuth: false,
                    },
                    {
                      page: 'strategy-market',
                      path: ROUTES.strategyMarket,
                      label:
                        language === 'zh'
                          ? '策略市场'
                          : language === 'id'
                            ? 'Pasar'
                            : 'Market',
                      requiresAuth: true,
                    },
                    {
                      page: 'traders',
                      path: ROUTES.traders,
                      label: t('configNav', language),
                      requiresAuth: true,
                    },
                    {
                      page: 'trader',
                      path: ROUTES.dashboard,
                      label: t('dashboardNav', language),
                      requiresAuth: true,
                    },
                    {
                      page: 'strategy',
                      path: ROUTES.strategy,
                      label: t('strategyNav', language),
                      requiresAuth: true,
                    },
                    {
                      page: 'backtest',
                      path: ROUTES.backtest,
                      label:
                        language === 'zh'
                          ? '回测'
                          : language === 'id'
                            ? 'Backtest'
                            : 'Backtest',
                      requiresAuth: true,
                    },
                    {
                      page: 'competition',
                      path: ROUTES.competition,
                      label: t('realtimeNav', language),
                      requiresAuth: true,
                    },
                    {
                      page: 'faq',
                      path: ROUTES.faq,
                      label: t('faqNav', language),
                      requiresAuth: false,
                    },
                  ]

                  const handleMobileNavClick = (tab: (typeof navTabs)[0]) => {
                    if (tab.requiresAuth && !isLoggedIn) {
                      onLoginRequired?.(tab.label)
                      setMobileMenuOpen(false)
                      return
                    }
                    if (onPageChange) {
                      onPageChange(tab.page)
                    }
                    navigateInApp(tab.path)
                    setMobileMenuOpen(false)
                  }

                  return navTabs
                    .filter((tab) => !tab.hidden)
                    .map((tab, i) => {
                      const isActive = resolvedCurrentPage === tab.page

                      return (
                        <motion.button
                          key={tab.page}
                          initial={{ x: -20, opacity: 0 }}
                          animate={{ x: 0, opacity: 1 }}
                          transition={{ delay: 0.1 + i * 0.05 }}
                          onClick={() => handleMobileNavClick(tab)}
                          className="min-h-12 rounded-xl border px-4 text-left text-xl font-black tracking-normal flex items-center gap-3"
                          style={{
                            background: isActive
                              ? 'color-mix(in srgb, var(--color-primary) 14%, var(--color-panel))'
                              : 'var(--color-panel)',
                            borderColor: isActive
                              ? 'var(--color-border-hover)'
                              : 'var(--color-border)',
                            color: isActive
                              ? 'var(--color-primary)'
                              : 'var(--color-foreground)',
                          }}
                        >
                          {isActive && (
                            <motion.div
                              layoutId="active-indicator"
                              className="w-1.5 h-1.5 rounded-full bg-primary"
                            />
                          )}
                          {tab.label}
                          {tab.badge && (
                            <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-primary/20 text-primary font-semibold uppercase align-middle relative -top-1">
                              {tab.badge}
                            </span>
                          )}
                          {tab.requiresAuth && !isLoggedIn && (
                            <span className="text-[10px] px-1.5 py-0.5 rounded border border-border text-muted-foreground font-normal tracking-wide uppercase align-middle relative -top-1">
                              LOGIN_REQ
                            </span>
                          )}
                        </motion.button>
                      )
                    })
                })()}

                {/* Original Page Links */}
                {isHomePage && (
                  <div className="pt-6 border-t border-border space-y-4">
                    {[
                      { key: 'features', label: t('features', language) },
                      { key: 'howItWorks', label: t('howItWorks', language) },
                    ].map((item, i) => (
                      <motion.a
                        key={item.key}
                        initial={{ opacity: 0 }}
                        animate={{ opacity: 1 }}
                        transition={{ delay: 0.5 + i * 0.1 }}
                        href={`#${item.key === 'features' ? 'features' : 'how-it-works'}`}
                        className="block text-lg font-mono text-muted-foreground hover:text-foreground"
                        onClick={() => setMobileMenuOpen(false)}
                      >
                        {'>'} {item.label}
                      </motion.a>
                    ))}
                  </div>
                )}
              </div>

              {/* Bottom Actions */}
              <div className="mt-auto space-y-8">
                {/* Account / Lang */}
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  {/* Lang Switcher */}
                  <div className="flex bg-surface rounded-lg p-1 border border-border">
                    {['zh', 'en', 'id'].map((lang) => (
                      <Button
                        variant="unstyled"
                        key={lang}
                        onClick={() => {
                          onLanguageChange?.(lang as Language)
                          setMobileMenuOpen(false)
                        }}
                        className={`flex-1 py-3 text-sm font-bold rounded-md transition-colors ${
                          language === lang
                            ? 'bg-surface-alt text-foreground shadow-sm'
                            : 'text-muted-foreground'
                        }`}
                      >
                        {lang === 'zh' ? 'CN' : lang === 'id' ? 'ID' : 'EN'}
                      </Button>
                    ))}
                  </div>

                  <Button
                    variant="unstyled"
                    type="button"
                    onClick={toggleColorScheme}
                    className="flex min-h-12 items-center justify-center gap-2 rounded-lg border border-border bg-surface text-sm font-bold text-muted-foreground"
                  >
                    {scheme === 'dark' ? (
                      <Sun className="h-4 w-4" />
                    ) : (
                      <Moon className="h-4 w-4" />
                    )}
                    {scheme === 'dark'
                      ? language === 'zh'
                        ? '日间'
                        : 'Light'
                      : language === 'zh'
                        ? '夜间'
                        : 'Dark'}
                  </Button>

                  {/* Auth Actions */}
                  {isLoggedIn && user ? (
                    <Button
                      variant="unstyled"
                      onClick={() => {
                        onLogout?.()
                        setMobileMenuOpen(false)
                      }}
                      className="bg-loss/10 border border-loss/20 text-loss rounded-lg font-bold text-sm hover:bg-loss/20 transition-colors"
                    >
                      {t('exitLogin', language)}
                    </Button>
                  ) : (
                    resolvedCurrentPage !== 'login' &&
                    resolvedCurrentPage !== 'register' && (
                      <Button
                        variant="unstyled"
                        type="button"
                        onClick={() => {
                          navigateInApp(ROUTES.login)
                          setMobileMenuOpen(false)
                        }}
                        className="flex items-center justify-center bg-primary text-primary-foreground rounded-lg font-bold text-sm hover:opacity-90 transition-colors"
                      >
                        {t('signIn', language)}
                      </Button>
                    )
                  )}
                </div>
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </nav>
  )
}
