import { useState, useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import { useLocation, useNavigate } from 'react-router-dom'
import { motion, AnimatePresence } from 'framer-motion'
import {
  Menu,
  X,
  ChevronDown,
  Settings,
  Sun,
  Moon,
  Repeat,
  LogOut,
} from 'lucide-react'
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

interface NavTab {
  page: Page
  path: string
  label: string
  requiresAuth: boolean
}

/** Shipped languages, in switcher order. Add ko/ja here when they ship. */
const LANGUAGES: { code: Language; short: string; labelKey: string }[] = [
  { code: 'zh', short: '中', labelKey: 'nav.languageZh' },
  { code: 'en', short: 'EN', labelKey: 'nav.languageEn' },
]

function useNavTabs(language: Language): NavTab[] {
  // Single nav definition shared by the desktop bar and the mobile sheet.
  return [
    {
      page: 'data',
      path: ROUTES.data,
      label: t('nav.data', language),
      requiresAuth: false,
    },
    {
      page: 'strategy-market',
      path: ROUTES.strategyMarket,
      label: t('nav.market', language),
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
      label: t('nav.backtest', language),
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
}

function LanguageSegmented({
  language,
  onChange,
  full = false,
}: {
  language: Language
  onChange?: (lang: Language) => void
  full?: boolean
}) {
  return (
    <div
      className={`ait-segmented ${full ? 'w-full' : ''}`}
      role="group"
      aria-label={t('language', language)}
    >
      {LANGUAGES.map((lang) => (
        <button
          key={lang.code}
          type="button"
          aria-pressed={language === lang.code}
          title={t(lang.labelKey, language)}
          onClick={() => onChange?.(lang.code)}
          className={full ? 'flex-1' : ''}
        >
          {full ? t(lang.labelKey, language) : lang.short}
        </button>
      ))}
    </div>
  )
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
  const [userDropdownOpen, setUserDropdownOpen] = useState(false)
  const [userMode, setUserModeState] = useState<UserMode>(
    () => getUserMode() ?? 'advanced'
  )
  const { scheme, style, setMode } = useTheme()
  const userDropdownRef = useRef<HTMLDivElement>(null)
  const resolvedCurrentPage =
    currentPage ?? getCurrentPageForPath(location.pathname)
  const navTabs = useNavTabs(language)

  const handleSwitchMode = (nextMode: UserMode) => {
    setUserMode(nextMode)
    setUserModeState(nextMode)
    setUserDropdownOpen(false)
    navigate(getPostAuthPath(nextMode))
  }

  const toggleColorScheme = () => {
    const nextScheme = scheme === 'dark' ? 'light' : 'dark'
    setMode(`${style}-${nextScheme}` as ThemeMode)
  }

  const handleNavClick = (tab: NavTab) => {
    setMobileMenuOpen(false)
    if (tab.requiresAuth && !isLoggedIn) {
      onLoginRequired?.(tab.label)
      return
    }
    onPageChange?.(tab.page)
    navigate(tab.path)
  }

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (
        userDropdownRef.current &&
        !userDropdownRef.current.contains(event.target as Node)
      ) {
        setUserDropdownOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  // Lock page scroll while the mobile sheet is open.
  useEffect(() => {
    if (!mobileMenuOpen) return
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prev
    }
  }, [mobileMenuOpen])

  const themeLabel =
    scheme === 'dark'
      ? t('nav.switchToLightTheme', language)
      : t('nav.switchToDarkTheme', language)
  const modeLabel =
    userMode === 'beginner'
      ? t('nav.switchToAdvanced', language)
      : t('nav.switchToBeginner', language)

  return (
    <>
      <nav className="fixed left-0 top-0 w-full z-50 header-bar">
        <div className="flex items-center justify-between h-16 px-[var(--page-gutter)] max-w-[1920px] mx-auto gap-4">
          {/* Logo */}
          <button
            type="button"
            onClick={() => navigate(ROUTES.home)}
            className="flex shrink-0 items-center gap-2 rounded-control hover:opacity-80"
            aria-label="AiT"
          >
            <img src="/icons/ait.svg" alt="" className="w-7 h-7" />
            <span className="text-[17px] font-bold tracking-tight text-foreground">
              AiT
            </span>
          </button>

          {/* Desktop navigation */}
          <div className="hidden xl:flex items-center justify-between flex-1 min-w-0">
            <div className="flex items-center gap-0.5 min-w-0 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
              {navTabs.map((tab) => {
                const active = resolvedCurrentPage === tab.page
                return (
                  <button
                    key={tab.page}
                    type="button"
                    onClick={() => handleNavClick(tab)}
                    aria-current={active ? 'page' : undefined}
                    className={`relative whitespace-nowrap px-3 h-9 rounded-control text-[13px] font-medium focus-visible:outline-2 focus-visible:outline-primary ${
                      active
                        ? 'text-foreground'
                        : 'text-muted-foreground hover:text-foreground'
                    }`}
                  >
                    {active && (
                      <motion.span
                        layoutId="ait-nav-active"
                        className="absolute inset-0 -z-10 rounded-control bg-surface-alt"
                        transition={{
                          type: 'spring',
                          stiffness: 500,
                          damping: 40,
                        }}
                      />
                    )}
                    {tab.label}
                  </button>
                )
              })}
            </div>

            <div className="flex shrink-0 items-center gap-2">
              <LanguageSegmented
                language={language}
                onChange={onLanguageChange}
              />
              <button
                type="button"
                onClick={toggleColorScheme}
                className="inline-flex h-9 w-9 items-center justify-center rounded-control text-muted-foreground hover:bg-surface-alt hover:text-foreground"
                aria-label={themeLabel}
                title={themeLabel}
              >
                {scheme === 'dark' ? (
                  <Sun className="h-4 w-4" />
                ) : (
                  <Moon className="h-4 w-4" />
                )}
              </button>

              {isLoggedIn && user ? (
                <div className="relative" ref={userDropdownRef}>
                  <button
                    type="button"
                    onClick={() => setUserDropdownOpen(!userDropdownOpen)}
                    aria-expanded={userDropdownOpen}
                    aria-label={t('nav.account', language)}
                    className="flex max-w-[240px] items-center gap-2 h-9 pl-1 pr-2 rounded-pill border border-border hover:bg-surface-alt"
                  >
                    <span className="w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold bg-primary text-primary-foreground">
                      {user.email[0].toUpperCase()}
                    </span>
                    <span className="truncate text-[13px] text-muted-foreground">
                      {user.email}
                    </span>
                    <ChevronDown className="w-3.5 h-3.5 text-muted-foreground" />
                  </button>

                  <AnimatePresence>
                    {userDropdownOpen && (
                      <motion.div
                        initial={{ opacity: 0, y: -4, scale: 0.98 }}
                        animate={{ opacity: 1, y: 0, scale: 1 }}
                        exit={{ opacity: 0, y: -4, scale: 0.98 }}
                        transition={{ duration: 0.16 }}
                        className="absolute right-0 top-full mt-2 w-56 overflow-hidden z-50 ait-card shadow-e2 p-1"
                      >
                        <div className="px-3 py-2">
                          <div className="t-micro text-muted-foreground">
                            {t('loggedInAs', language)}
                          </div>
                          <div className="t-caption truncate text-foreground">
                            {user.email}
                          </div>
                        </div>
                        <div className="h-px bg-border my-1" />
                        <MenuItem
                          icon={<Settings className="w-4 h-4" />}
                          label={t('nav.settings', language)}
                          onClick={() => {
                            navigate(ROUTES.settings)
                            setUserDropdownOpen(false)
                          }}
                        />
                        <MenuItem
                          icon={<Repeat className="w-4 h-4" />}
                          label={modeLabel}
                          onClick={() =>
                            handleSwitchMode(
                              userMode === 'beginner' ? 'advanced' : 'beginner'
                            )
                          }
                        />
                        {onLogout && (
                          <MenuItem
                            danger
                            icon={<LogOut className="w-4 h-4" />}
                            label={t('exitLogin', language)}
                            onClick={() => {
                              onLogout()
                              setUserDropdownOpen(false)
                            }}
                          />
                        )}
                      </motion.div>
                    )}
                  </AnimatePresence>
                </div>
              ) : (
                resolvedCurrentPage !== 'login' &&
                resolvedCurrentPage !== 'register' && (
                  <Button
                    variant="unstyled"
                    type="button"
                    onClick={() => navigate(ROUTES.login)}
                    className="h-9 px-4 rounded-pill text-[13px] font-semibold bg-primary text-primary-foreground hover:opacity-90"
                  >
                    {t('signIn', language)}
                  </Button>
                )
              )}
            </div>
          </div>

          {/* Mobile menu button */}
          <motion.button
            type="button"
            onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            aria-expanded={mobileMenuOpen}
            aria-label={
              mobileMenuOpen
                ? t('nav.closeMenu', language)
                : t('nav.openMenu', language)
            }
            className="xl:hidden inline-flex h-10 w-10 items-center justify-center rounded-control text-foreground hover:bg-surface-alt"
            whileTap={{ scale: 0.92 }}
          >
            {mobileMenuOpen ? (
              <X className="w-5 h-5" />
            ) : (
              <Menu className="w-5 h-5" />
            )}
          </motion.button>
        </div>
      </nav>
      {/* Mobile sheet. Rendered through a portal to <body>: the nav's
          backdrop-filter makes it the containing block for fixed
          descendants, which collapsed this sheet to the nav's 64px
          (top:64px + bottom:0 = 0 height) (#85). */}
      {typeof document !== 'undefined' &&
        createPortal(
          <AnimatePresence>
            {mobileMenuOpen && (
              <motion.div
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
                transition={{ duration: 0.18 }}
                className="ait-mobile-menu-overlay fixed inset-x-0 bottom-0 z-40 xl:hidden"
                style={{ top: '64px' }}
              >
                <motion.div
                  initial={{ y: -12, opacity: 0 }}
                  animate={{ y: 0, opacity: 1 }}
                  transition={{ duration: 0.24, ease: [0.2, 0.8, 0.2, 1] }}
                  className="flex h-full flex-col overflow-y-auto px-[var(--page-gutter)] py-5"
                >
                  <div className="flex flex-col">
                    {navTabs.map((tab) => {
                      const active = resolvedCurrentPage === tab.page
                      return (
                        <button
                          key={tab.page}
                          type="button"
                          onClick={() => handleNavClick(tab)}
                          aria-current={active ? 'page' : undefined}
                          className={`flex min-h-[52px] items-center justify-between border-b border-border text-left text-[22px] font-semibold tracking-tight ${
                            active ? 'text-primary' : 'text-foreground'
                          }`}
                        >
                          {tab.label}
                          {tab.requiresAuth && !isLoggedIn && (
                            <span className="ait-pill border border-border text-muted-foreground">
                              {t('nav.loginRequired', language)}
                            </span>
                          )}
                        </button>
                      )
                    })}

                    {isHomePage && (
                      <div className="pt-4 flex flex-col">
                        {[
                          { href: '#features', label: t('features', language) },
                          {
                            href: '#how-it-works',
                            label: t('howItWorks', language),
                          },
                        ].map((item) => (
                          <a
                            key={item.href}
                            href={item.href}
                            className="min-h-[44px] flex items-center t-body text-muted-foreground hover:text-foreground"
                            onClick={() => setMobileMenuOpen(false)}
                          >
                            {item.label}
                          </a>
                        ))}
                      </div>
                    )}
                  </div>

                  <div className="mt-auto pt-8 space-y-3">
                    <LanguageSegmented
                      full
                      language={language}
                      onChange={(lang) => {
                        onLanguageChange?.(lang)
                        setMobileMenuOpen(false)
                      }}
                    />
                    <div className="grid grid-cols-2 gap-3">
                      <button
                        type="button"
                        onClick={toggleColorScheme}
                        className="flex min-h-[48px] items-center justify-center gap-2 rounded-control border border-border bg-surface t-label"
                      >
                        {scheme === 'dark' ? (
                          <Sun className="h-4 w-4" />
                        ) : (
                          <Moon className="h-4 w-4" />
                        )}
                        {scheme === 'dark'
                          ? t('nav.themeLight', language)
                          : t('nav.themeDark', language)}
                      </button>
                      {isLoggedIn && user ? (
                        <button
                          type="button"
                          onClick={() => {
                            onLogout?.()
                            setMobileMenuOpen(false)
                          }}
                          className="flex min-h-[48px] items-center justify-center gap-2 rounded-control border border-loss/30 bg-loss/10 text-loss t-label"
                        >
                          <LogOut className="h-4 w-4" />
                          {t('exitLogin', language)}
                        </button>
                      ) : (
                        resolvedCurrentPage !== 'login' &&
                        resolvedCurrentPage !== 'register' && (
                          <button
                            type="button"
                            onClick={() => {
                              navigate(ROUTES.login)
                              setMobileMenuOpen(false)
                            }}
                            className="flex min-h-[48px] items-center justify-center rounded-control bg-primary text-primary-foreground t-label"
                          >
                            {t('signIn', language)}
                          </button>
                        )
                      )}
                    </div>
                    {isLoggedIn && user && (
                      <div className="grid grid-cols-2 gap-3">
                        <button
                          type="button"
                          onClick={() => {
                            navigate(ROUTES.settings)
                            setMobileMenuOpen(false)
                          }}
                          className="flex min-h-[48px] items-center justify-center gap-2 rounded-control border border-border bg-surface t-label"
                        >
                          <Settings className="h-4 w-4" />
                          {t('nav.settings', language)}
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            setMobileMenuOpen(false)
                            handleSwitchMode(
                              userMode === 'beginner' ? 'advanced' : 'beginner'
                            )
                          }}
                          className="flex min-h-[48px] items-center justify-center gap-2 rounded-control border border-border bg-surface t-label"
                        >
                          <Repeat className="h-4 w-4" />
                          {modeLabel}
                        </button>
                      </div>
                    )}
                  </div>
                </motion.div>
              </motion.div>
            )}
          </AnimatePresence>,
          document.body
        )}
    </>
  )
}

function MenuItem({
  icon,
  label,
  onClick,
  danger = false,
}: {
  icon: React.ReactNode
  label: string
  onClick: () => void
  danger?: boolean
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`w-full flex items-center gap-2.5 px-3 h-9 rounded-control text-[13px] text-left ${
        danger
          ? 'text-loss hover:bg-loss/10'
          : 'text-muted-foreground hover:bg-surface-alt hover:text-foreground'
      }`}
    >
      {icon}
      {label}
    </button>
  )
}
