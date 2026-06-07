import {
  type ComponentType,
  type LazyExoticComponent,
  type ReactNode,
  lazy,
  Suspense,
  useEffect,
  useState,
} from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import useSWR from 'swr'
import {
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
  useSearchParams,
} from 'react-router-dom'
import HeaderBar from '../components/common/HeaderBar'
import { SiteFooter } from '../components/common/SiteFooter'
import { LoginRequiredOverlay } from '../components/auth/LoginRequiredOverlay'
import { LoginPage } from '../components/auth/LoginPage'
import { RegisterPage } from '../components/auth/RegisterPage'
import { ResetPasswordPage } from '../components/auth/ResetPasswordPage'
import { useAuth } from '../contexts/AuthContext'
import { useLanguage } from '../contexts/LanguageContext'
import { useSystemConfig } from '../hooks/useSystemConfig'
import { t } from '../i18n/translations'
import { api } from '../lib/api'
import { getUserMode } from '../lib/onboarding'
import type {
  AccountInfo,
  DecisionRecord,
  Exchange,
  Position,
  Statistics,
  SystemStatus,
  TraderInfo,
} from '../types'
import {
  buildDashboardPath,
  LEGACY_HASH_ROUTES,
  ROUTES,
  type Page,
} from './paths'

type PreloadableComponent<T extends ComponentType<any>> =
  LazyExoticComponent<T> & {
    preload: () => Promise<{ default: T }>
  }

type IdleWindow = Window & {
  requestIdleCallback?: (
    callback: IdleRequestCallback,
    options?: IdleRequestOptions
  ) => number
}

function lazyWithPreload<T extends ComponentType<any>>(
  loader: () => Promise<{ default: T }>
): PreloadableComponent<T> {
  const Component = lazy(loader) as PreloadableComponent<T>
  Component.preload = loader
  return Component
}

const loadSetupPage = () =>
  import('../components/modals/SetupPage').then((m) => ({ default: m.SetupPage }))
const loadCompetitionPage = () =>
  import('../components/trader/CompetitionPage').then((m) => ({
    default: m.CompetitionPage,
  }))
const loadAITradersPage = () =>
  import('../components/trader/AITradersPage').then((m) => ({
    default: m.AITradersPage,
  }))
const loadFAQPage = () =>
  import('../pages/FAQPage').then((m) => ({ default: m.FAQPage }))
const loadBeginnerOnboardingPage = () =>
  import('../pages/BeginnerOnboardingPage').then((m) => ({
    default: m.BeginnerOnboardingPage,
  }))
const loadDataPage = () =>
  import('../pages/DataPage').then((m) => ({ default: m.DataPage }))
const loadAgentChatPage = () =>
  import('../pages/AgentChatPage').then((m) => ({ default: m.AgentChatPage }))
const loadSettingsPage = () =>
  import('../pages/SettingsPage').then((m) => ({ default: m.SettingsPage }))
const loadStrategyMarketPage = () =>
  import('../pages/StrategyMarketPage').then((m) => ({
    default: m.StrategyMarketPage,
  }))
const loadStrategyStudioPage = () =>
  import('../pages/StrategyStudioPage').then((m) => ({
    default: m.StrategyStudioPage,
  }))
const loadTraderDashboardPage = () =>
  import('../pages/TraderDashboardPage').then((m) => ({
    default: m.TraderDashboardPage,
  }))
const loadBacktestPage = () =>
  import('../components/backtest/BacktestPage').then((m) => ({
    default: m.BacktestPage,
  }))

// Route-level lazy-loaded pages for code splitting. The preload hook below
// warms common chunks after initial auth so navigation does not pay the full
// parse/transform cost.
const SetupPage = lazyWithPreload(loadSetupPage)
const CompetitionPage = lazyWithPreload(loadCompetitionPage)
const AITradersPage = lazyWithPreload(loadAITradersPage)
const FAQPage = lazyWithPreload(loadFAQPage)
const BeginnerOnboardingPage = lazyWithPreload(loadBeginnerOnboardingPage)
const DataPage = lazyWithPreload(loadDataPage)
const AgentChatPage = lazyWithPreload(loadAgentChatPage)
const SettingsPage = lazyWithPreload(loadSettingsPage)
const StrategyMarketPage = lazyWithPreload(loadStrategyMarketPage)
const StrategyStudioPage = lazyWithPreload(loadStrategyStudioPage)
const TraderDashboardPage = lazyWithPreload(loadTraderDashboardPage)
const BacktestPage = lazyWithPreload(loadBacktestPage)

let routePreloadStarted = false

function scheduleRoutePreloads() {
  if (routePreloadStarted) return
  routePreloadStarted = true

  const queue = [
    TraderDashboardPage.preload,
    AITradersPage.preload,
    SettingsPage.preload,
    StrategyStudioPage.preload,
    StrategyMarketPage.preload,
    BacktestPage.preload,
    CompetitionPage.preload,
    DataPage.preload,
    AgentChatPage.preload,
  ]

  const runNext = () => {
    const preload = queue.shift()
    if (!preload) return
    preload()
      .catch(() => {
        // A preload failure should not break navigation; the route load will
        // surface the real error if the user opens it.
      })
      .finally(() => {
        const idleWindow = window as IdleWindow
        if (idleWindow.requestIdleCallback) {
          idleWindow.requestIdleCallback(runNext, { timeout: 2000 })
        } else {
          globalThis.setTimeout(runNext, 250)
        }
      })
  }

  const idleWindow = window as IdleWindow
  if (idleWindow.requestIdleCallback) {
    idleWindow.requestIdleCallback(runNext, { timeout: 1500 })
  } else {
    globalThis.setTimeout(runNext, 500)
  }
}

function getTraderSlug(trader: TraderInfo) {
  const idPrefix = trader.trader_id.slice(0, 4)
  return `${trader.trader_name}-${idPrefix}`
}

function findTraderBySlug(slug: string, traderList: TraderInfo[]) {
  const lastDashIndex = slug.lastIndexOf('-')
  if (lastDashIndex === -1) {
    return traderList.find((trader) => trader.trader_name === slug)
  }

  const name = slug.slice(0, lastDashIndex)
  const idPrefix = slug.slice(lastDashIndex + 1)
  return traderList.find(
    (trader) =>
      trader.trader_name === name && trader.trader_id.startsWith(idPrefix)
  )
}

function LoadingScreen() {
  const { language } = useLanguage()

  return (
    <div className="min-h-screen flex items-center justify-center bg-background">
      <div className="text-center">
        <img
          src="/icons/ait.svg"
          alt="AiT Logo"
          className="w-16 h-16 mx-auto mb-4 animate-pulse"
        />
        <p className="text-foreground">{t('loading', language)}</p>
      </div>
    </div>
  )
}

function RouteLoadingFallback() {
  const { language } = useLanguage()

  return (
    <div className="min-h-[50vh] flex items-center justify-center bg-background">
      <div className="text-sm text-muted-foreground">
        {t('loading', language)}
      </div>
    </div>
  )
}

function LegacyHashRedirect() {
  const location = useLocation()
  const navigate = useNavigate()

  useEffect(() => {
    const hashRoute = LEGACY_HASH_ROUTES[location.hash.slice(1)]
    if (!hashRoute) {
      return
    }

    if (hashRoute === location.pathname && location.hash === '') {
      return
    }

    navigate(
      {
        pathname: hashRoute,
        search: location.search,
      },
      { replace: true }
    )
  }, [location.hash, location.pathname, location.search, navigate])

  return null
}

interface AppChromeProps {
  children: ReactNode
  currentPage?: Page
  showFooter?: boolean
  wrapInMain?: boolean
  animateContent?: boolean
  extraContent?: ReactNode
}

function AppChrome({
  children,
  currentPage,
  showFooter = true,
  wrapInMain = true,
  animateContent = false,
  extraContent,
}: AppChromeProps) {
  const location = useLocation()
  const { language, setLanguage } = useLanguage()
  const { user, logout } = useAuth()
  const [loginOverlayOpen, setLoginOverlayOpen] = useState(false)
  const [loginOverlayFeature, setLoginOverlayFeature] = useState('')

  const handleLoginRequired = (featureName: string) => {
    setLoginOverlayFeature(featureName)
    setLoginOverlayOpen(true)
  }

  const content = animateContent ? (
    <AnimatePresence mode="wait">
      <motion.div
        key={`${location.pathname}${location.search}`}
        initial={{ opacity: 0, y: 8 }}
        animate={{ opacity: 1, y: 0 }}
        exit={{ opacity: 0, y: -8 }}
        transition={{ duration: 0.15, ease: 'easeOut' }}
      >
        {children}
      </motion.div>
    </AnimatePresence>
  ) : (
    children
  )

  return (
    <div className="ait-app-shell min-h-screen text-foreground">
      <HeaderBar
        isLoggedIn={!!user}
        currentPage={currentPage}
        language={language}
        onLanguageChange={setLanguage}
        user={user}
        onLogout={logout}
        onLoginRequired={handleLoginRequired}
      />

      {wrapInMain ? (
        <main className="min-h-screen pt-16">{content}</main>
      ) : (
        content
      )}

      {showFooter ? <SiteFooter language={language} /> : null}

      <LoginRequiredOverlay
        isOpen={loginOverlayOpen}
        onClose={() => setLoginOverlayOpen(false)}
        featureName={loginOverlayFeature}
      />

      {extraContent}
    </div>
  )
}

function TradersRoute({
  showBeginnerOnboarding = false,
}: {
  showBeginnerOnboarding?: boolean
}) {
  const navigate = useNavigate()
  const { user, token } = useAuth()
  const { data: traders } = useSWR<TraderInfo[]>(
    user && token ? 'traders-route' : null,
    api.getTraders,
    {
      refreshInterval: 15000,
      shouldRetryOnError: false,
    }
  )

  return (
    <AppChrome
      currentPage="traders"
      animateContent
      extraContent={showBeginnerOnboarding ? <BeginnerOnboardingPage /> : null}
    >
      <AITradersPage
        onTraderSelect={(traderId) => {
          const trader = traders?.find((item) => item.trader_id === traderId)
          navigate(
            buildDashboardPath(trader ? getTraderSlug(trader) : undefined)
          )
        }}
      />
    </AppChrome>
  )
}

function DashboardRoute() {
  const { language } = useLanguage()
  const { user, token } = useAuth()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const selectedTraderSlug = searchParams.get('trader') || undefined
  const [selectedTraderId, setSelectedTraderId] = useState<string | undefined>()
  const [lastUpdate, setLastUpdate] = useState<string>('--:--:--')
  const [decisionsLimit, setDecisionsLimit] = useState(5)

  const { data: traders, error: tradersError } = useSWR<TraderInfo[]>(
    user && token ? 'traders-dashboard' : null,
    () => api.getTraders(true),
    {
      refreshInterval: 20000,
      shouldRetryOnError: false,
    }
  )

  const { data: exchanges } = useSWR<Exchange[]>(
    user && token ? 'exchanges-dashboard' : null,
    api.getExchangeConfigs,
    {
      refreshInterval: 60000,
      shouldRetryOnError: false,
    }
  )

  useEffect(() => {
    if (!traders || traders.length === 0) {
      return
    }

    if (selectedTraderSlug) {
      const trader = findTraderBySlug(selectedTraderSlug, traders)
      const nextTraderId = trader?.trader_id || traders[0].trader_id
      if (nextTraderId !== selectedTraderId) {
        setSelectedTraderId(nextTraderId)
      }
      return
    }

    if (!selectedTraderId) {
      setSelectedTraderId(traders[0].trader_id)
    }
  }, [selectedTraderId, selectedTraderSlug, traders])

  const { data: status } = useSWR<SystemStatus>(
    selectedTraderId ? `status-${selectedTraderId}` : null,
    () => api.getStatus(selectedTraderId, true),
    {
      refreshInterval: 30000,
      revalidateOnFocus: false,
      dedupingInterval: 10000,
    }
  )

  const { data: account } = useSWR<AccountInfo>(
    selectedTraderId ? `account-${selectedTraderId}` : null,
    () => api.getAccount(selectedTraderId, true),
    {
      refreshInterval: 30000,
      revalidateOnFocus: false,
      dedupingInterval: 25000,
      onErrorRetry: (error, _key, _config, revalidate, { retryCount }) => {
        if (error.status === 404) return
        if (retryCount >= 5) return
        const delay = Math.min(2000 * 2 ** retryCount, 30000)
        setTimeout(() => revalidate({ retryCount }), delay)
      },
    }
  )

  const { data: positions } = useSWR<Position[]>(
    selectedTraderId ? `positions-${selectedTraderId}` : null,
    () => api.getPositions(selectedTraderId, true),
    {
      refreshInterval: 30000,
      revalidateOnFocus: false,
      dedupingInterval: 25000,
      onErrorRetry: (error, _key, _config, revalidate, { retryCount }) => {
        if (error.status === 404) return
        if (retryCount >= 5) return
        const delay = Math.min(2000 * 2 ** retryCount, 30000)
        setTimeout(() => revalidate({ retryCount }), delay)
      },
    }
  )

  const { data: decisions } = useSWR<DecisionRecord[]>(
    selectedTraderId
      ? `decisions/latest-${selectedTraderId}-${decisionsLimit}`
      : null,
    () => api.getLatestDecisions(selectedTraderId, decisionsLimit, true),
    {
      refreshInterval: 60000,
      revalidateOnFocus: false,
      dedupingInterval: 20000,
      onErrorRetry: (error, _key, _config, revalidate, { retryCount }) => {
        if (error.status === 404) return
        if (retryCount >= 5) return
        const delay = Math.min(2000 * 2 ** retryCount, 30000)
        setTimeout(() => revalidate({ retryCount }), delay)
      },
    }
  )

  const { data: stats } = useSWR<Statistics>(
    selectedTraderId ? `statistics-${selectedTraderId}` : null,
    () => api.getStatistics(selectedTraderId, true),
    {
      refreshInterval: 60000,
      revalidateOnFocus: false,
      dedupingInterval: 20000,
    }
  )

  useEffect(() => {
    if (account) {
      setLastUpdate(new Date().toLocaleTimeString())
    }
  }, [account])

  const selectedTrader = traders?.find(
    (trader) => trader.trader_id === selectedTraderId
  )

  return (
    <AppChrome currentPage="trader" animateContent>
      <TraderDashboardPage
        selectedTrader={selectedTrader}
        status={status}
        account={account}
        accountFailed={false}
        positions={positions}
        positionsFailed={false}
        decisions={decisions}
        decisionsFailed={false}
        decisionsLimit={decisionsLimit}
        onDecisionsLimitChange={setDecisionsLimit}
        stats={stats}
        lastUpdate={lastUpdate}
        language={language}
        traders={traders}
        tradersError={tradersError}
        selectedTraderId={selectedTraderId}
        onTraderSelect={(traderId) => {
          setSelectedTraderId(traderId)
          const trader = traders?.find((item) => item.trader_id === traderId)
          navigate(
            buildDashboardPath(trader ? getTraderSlug(trader) : undefined),
            {
              replace: true,
            }
          )
        }}
        onNavigateToTraders={() => navigate(ROUTES.traders)}
        exchanges={exchanges}
      />
    </AppChrome>
  )
}

export function AppRoutes() {
  const { user, token, isLoading } = useAuth()
  const { config: systemConfig, loading: configLoading } = useSystemConfig()
  const isAuthenticated = !!user && !!token

  useEffect(() => {
    if (isAuthenticated) {
      scheduleRoutePreloads()
    }
  }, [isAuthenticated])

  if (isLoading || configLoading) {
    return <LoadingScreen />
  }

  if (systemConfig && !systemConfig.initialized && !user) {
    return <SetupPage />
  }

  return (
    <>
      <LegacyHashRedirect />
      <Suspense fallback={<RouteLoadingFallback />}>
        <Routes>
        <Route
          path={ROUTES.home}
          element={<Navigate to={ROUTES.login} replace />}
        />
        <Route path={ROUTES.login} element={<LoginPage />} />
        <Route path={ROUTES.register} element={<RegisterPage />} />
        <Route path={ROUTES.resetPassword} element={<ResetPasswordPage />} />
        <Route
          path={ROUTES.setup}
          element={
            user ? (
              <Navigate to={ROUTES.welcome} replace />
            ) : systemConfig?.initialized ? (
              <Navigate to={ROUTES.login} replace />
            ) : (
              <SetupPage />
            )
          }
        />
        <Route
          path={ROUTES.faq}
          element={
            <AppChrome currentPage="faq" showFooter={false} wrapInMain={false}>
              <FAQPage />
            </AppChrome>
          }
        />
        <Route
          path={ROUTES.agent}
          element={
            <AppChrome currentPage="agent" showFooter={false}>
              <AgentChatPage />
            </AppChrome>
          }
        />
        <Route
          path={ROUTES.data}
          element={
            <AppChrome currentPage="data" showFooter={false}>
              <DataPage />
            </AppChrome>
          }
        />
        <Route
          path={ROUTES.settings}
          element={
            isAuthenticated ? (
              <AppChrome showFooter={false}>
                <SettingsPage />
              </AppChrome>
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path={ROUTES.welcome}
          element={
            isAuthenticated ? (
              getUserMode() === 'beginner' ? (
                <TradersRoute showBeginnerOnboarding />
              ) : (
                <Navigate to={ROUTES.traders} replace />
              )
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path={ROUTES.competition}
          element={
            isAuthenticated ? (
              <AppChrome currentPage="competition" animateContent>
                <CompetitionPage />
              </AppChrome>
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path={ROUTES.strategyMarket}
          element={
            isAuthenticated ? (
              <AppChrome currentPage="strategy-market" animateContent>
                <StrategyMarketPage />
              </AppChrome>
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path={ROUTES.traders}
          element={
            isAuthenticated ? (
              <TradersRoute />
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path={ROUTES.dashboard}
          element={
            isAuthenticated ? (
              <DashboardRoute />
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path={ROUTES.strategy}
          element={
            isAuthenticated ? (
              <AppChrome currentPage="strategy" animateContent>
                <StrategyStudioPage />
              </AppChrome>
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route
          path={ROUTES.backtest}
          element={
            isAuthenticated ? (
              <AppChrome currentPage="backtest" animateContent>
                <BacktestPage />
              </AppChrome>
            ) : (
              <Navigate to={ROUTES.login} replace />
            )
          }
        />
        <Route path="*" element={<Navigate to={ROUTES.home} replace />} />
        </Routes>
      </Suspense>
    </>
  )
}
