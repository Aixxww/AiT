import { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { useNavigate } from 'react-router-dom'
import useSWR from 'swr'
import {
  TrendingUp,
  Shield,
  Zap,
  Eye,
  EyeOff,
  Copy,
  Check,
  Hexagon,
  Layers,
  Target,
  Activity,
  Database,
  Search,
} from 'lucide-react'
import { useLanguage } from '../contexts/LanguageContext'
import { useAuth } from '../contexts/AuthContext'
import { toast } from 'sonner'
import { t } from '../i18n/translations'
import { DeepVoidBackground } from '../components/common/DeepVoidBackground'
import { Button } from '../components/ui/Button'
import { InfoTip } from '../components/ui/InfoTip'

interface PublicStrategy {
  id: string
  name: string
  description: string
  author_email?: string
  is_public: boolean
  config_visible: boolean
  config?: any
  stats?: {
    used_by: number
    rating: number
  }
  created_at: string
  updated_at: string
}

const strategyStyles: Record<
  string,
  {
    color: string
    border: string
    icon: any
    bg: string
  }
> = {
  scalper: {
    color: 'text-primary',
    border: 'border-primary/30',
    bg: 'bg-primary-dim',
    icon: Zap,
  },
  swing: {
    color: 'text-accent',
    border: 'border-accent/30',
    bg: 'bg-accent/5',
    icon: TrendingUp,
  },
  arbitrage: {
    color: 'text-accent-purple',
    border: 'border-accent-purple/30',
    bg: 'bg-accent-purple/5',
    icon: Layers,
  },
  conservative: {
    color: 'text-profit',
    border: 'border-profit/30',
    bg: 'bg-profit/5',
    icon: Shield,
  },
  aggressive: {
    color: 'text-loss',
    border: 'border-loss/30',
    bg: 'bg-loss/5',
    icon: Target,
  },
  default: {
    color: 'text-muted-foreground',
    border: 'border-border',
    bg: 'bg-surface-alt/20',
    icon: Activity,
  },
}

function getStrategyStyle(name: string) {
  const lowerName = name.toLowerCase()
  if (lowerName.includes('scalp')) return strategyStyles.scalper
  if (lowerName.includes('swing')) return strategyStyles.swing
  if (lowerName.includes('arb')) return strategyStyles.arbitrage
  if (lowerName.includes('safe') || lowerName.includes('conserv'))
    return strategyStyles.conservative
  if (lowerName.includes('aggress') || lowerName.includes('high'))
    return strategyStyles.aggressive
  return strategyStyles.default
}

export function StrategyMarketPage() {
  const navigate = useNavigate()
  const { language } = useLanguage()
  const { token, user } = useAuth()
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedCategory, setSelectedCategory] = useState<string>('all')
  const [copiedId, setCopiedId] = useState<string | null>(null)

  const tr = (key: string) => t(`strategyMarket.${key}`, language)

  // Fetch public strategies
  const { data: strategies, isLoading } = useSWR<PublicStrategy[]>(
    'public-strategies',
    async () => {
      const response = await fetch('/api/strategies/public')
      if (!response.ok) throw new Error('Failed to fetch strategies')
      const data = await response.json()
      return data.strategies || []
    },
    {
      refreshInterval: 60000,
      revalidateOnFocus: false,
    }
  )

  const filteredStrategies =
    strategies?.filter((s) => {
      if (searchQuery) {
        const query = searchQuery.toLowerCase()
        return (
          s.name.toLowerCase().includes(query) ||
          s.description?.toLowerCase().includes(query)
        )
      }
      return true
    }) || []

  const handleCopyConfig = async (strategy: PublicStrategy) => {
    if (!strategy.config) return
    try {
      await navigator.clipboard.writeText(
        JSON.stringify(strategy.config, null, 2)
      )
      setCopiedId(strategy.id)
      toast.success(tr('copied'))
      setTimeout(() => setCopiedId(null), 2000)
    } catch (err) {
      console.error('Failed to copy:', err)
    }
  }

  const formatDate = (dateStr: string) =>
    new Date(dateStr).toLocaleDateString(
      language === 'zh' ? 'zh-CN' : 'en-US',
      {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
      }
    )

  const getIndicatorList = (config: any) => {
    if (!config?.indicators) return []
    const indicators = []
    if (config.indicators.enable_ema) indicators.push('EMA')
    if (config.indicators.enable_macd) indicators.push('MACD')
    if (config.indicators.enable_rsi) indicators.push('RSI')
    if (config.indicators.enable_atr) indicators.push('ATR')
    if (config.indicators.enable_boll) indicators.push('BOLL')
    if (config.indicators.enable_volume) indicators.push('VOL')
    if (config.indicators.enable_oi) indicators.push('OI')
    if (config.indicators.enable_funding_rate) indicators.push('FR')
    return indicators
  }

  return (
    <DeepVoidBackground className="min-h-screen text-foreground py-6 sm:py-10">
      <div className="ait-page space-y-6">
        {/* Header */}
        <header className="flex items-start gap-3">
          <div className="ait-card-inset p-2.5 shrink-0">
            <Database className="w-6 h-6 text-primary" />
          </div>
          <div className="min-w-0">
            <h1 className="t-title">{tr('title')}</h1>
            <p className="t-body text-muted-foreground mt-1">
              {tr('description')}
            </p>
          </div>
        </header>

        {/* Search and filter */}
        <div className="flex flex-col md:flex-row gap-3">
          <label className="relative flex-1 block">
            <Search
              size={16}
              className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground pointer-events-none"
            />
            <input
              type="text"
              placeholder={tr('search')}
              aria-label={tr('search')}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full rounded-control py-2.5 pl-9 pr-3 text-sm"
            />
          </label>
          <div className="ait-segmented self-start" role="group">
            {['all', 'popular', 'recent'].map((cat) => (
              <button
                type="button"
                key={cat}
                aria-pressed={selectedCategory === cat}
                onClick={() => setSelectedCategory(cat)}
              >
                {tr(cat)}
              </button>
            ))}
          </div>
        </div>

        {/* Loading */}
        {isLoading && (
          <div className="flex flex-col items-center justify-center py-24 gap-3 text-muted-foreground">
            <div className="w-8 h-8 border-2 border-border border-t-primary rounded-full animate-spin" />
            <p className="t-caption">{tr('loading')}</p>
          </div>
        )}

        {/* Empty */}
        {!isLoading && filteredStrategies.length === 0 && (
          <div className="ait-card flex flex-col items-center justify-center py-20 px-6 text-center border-dashed">
            <Activity className="w-10 h-10 text-muted-foreground mb-4" />
            <h3 className="t-headline mb-1">{tr('noStrategies')}</h3>
            <p className="t-body text-muted-foreground">
              {tr('noStrategiesDesc')}
            </p>
          </div>
        )}

        {/* Strategy grid */}
        {!isLoading && filteredStrategies.length > 0 && (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
            <AnimatePresence>
              {filteredStrategies.map((strategy, i) => {
                const style = getStrategyStyle(strategy.name)
                const Icon = style.icon
                const visible = strategy.config_visible && strategy.config
                const indicators = visible
                  ? getIndicatorList(strategy.config)
                  : []

                return (
                  <motion.div
                    key={strategy.id}
                    initial={{ opacity: 0, y: 8 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0 }}
                    transition={{ delay: Math.min(i, 8) * 0.04 }}
                    className="ait-card p-5 flex flex-col gap-4 hover:border-border-hover transition-colors"
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="flex items-center gap-3 min-w-0">
                        <div
                          className={`p-2 rounded-control border shrink-0 ${style.border} ${style.bg}`}
                        >
                          <Icon className={`w-4 h-4 ${style.color}`} />
                        </div>
                        <h3 className="t-headline truncate">{strategy.name}</h3>
                      </div>
                      {visible ? (
                        <span className="ait-pill bg-profit-bg text-profit shrink-0">
                          <Eye size={12} />
                          {tr('configPublic')}
                        </span>
                      ) : (
                        <InfoTip content={tr('configHiddenDesc')}>
                          <button
                            type="button"
                            className="ait-pill bg-surface-alt text-muted-foreground shrink-0 cursor-help"
                          >
                            <EyeOff size={12} />
                            {tr('configPrivate')}
                          </button>
                        </InfoTip>
                      )}
                    </div>

                    <p className="t-body text-muted-foreground line-clamp-2 min-h-[2.5rem]">
                      {strategy.description || tr('noDescription')}
                    </p>

                    <dl className="grid grid-cols-2 gap-2 t-caption">
                      <div>
                        <dt className="text-muted-foreground">
                          {tr('author')}
                        </dt>
                        <dd className="text-foreground truncate">
                          {strategy.author_email
                            ? '@' + strategy.author_email.split('@')[0]
                            : tr('unknownAuthor')}
                        </dd>
                      </div>
                      <div className="text-right">
                        <dt className="text-muted-foreground">
                          {tr('createdAt')}
                        </dt>
                        <dd className="text-foreground t-num">
                          {formatDate(strategy.created_at)}
                        </dd>
                      </div>
                    </dl>

                    {visible && (
                      <div className="ait-card-inset p-3 space-y-3">
                        <div className="flex flex-wrap gap-1.5">
                          {indicators.length > 0 ? (
                            indicators.map((ind) => (
                              <span
                                key={ind}
                                className="ait-pill bg-panel border border-border text-foreground"
                              >
                                {ind}
                              </span>
                            ))
                          ) : (
                            <span className="t-caption text-muted-foreground">
                              {tr('noIndicators')}
                            </span>
                          )}
                        </div>
                        {strategy.config.risk_control && (
                          <div className="flex gap-6 t-caption">
                            <div>
                              <div className="text-muted-foreground">
                                {tr('leverageShort')}
                              </div>
                              <div className="t-num text-foreground font-semibold">
                                {strategy.config.risk_control
                                  .btc_eth_max_leverage
                                  ? `${strategy.config.risk_control.btc_eth_max_leverage}x`
                                  : '-'}
                              </div>
                            </div>
                            <div>
                              <div className="text-muted-foreground">
                                {tr('positionsShort')}
                              </div>
                              <div className="t-num text-foreground font-semibold">
                                {strategy.config.risk_control.max_positions ||
                                  '-'}
                              </div>
                            </div>
                          </div>
                        )}
                      </div>
                    )}

                    <div className="mt-auto">
                      {visible ? (
                        <Button
                          variant="secondary"
                          onClick={() => handleCopyConfig(strategy)}
                          className="w-full"
                        >
                          {copiedId === strategy.id ? (
                            <>
                              <Check className="w-4 h-4 text-profit" />
                              <span className="text-profit">
                                {tr('copied')}
                              </span>
                            </>
                          ) : (
                            <>
                              <Copy className="w-4 h-4" />
                              {tr('copyConfig')}
                            </>
                          )}
                        </Button>
                      ) : (
                        <Button variant="secondary" disabled className="w-full">
                          <Shield className="w-4 h-4" />
                          {tr('configHidden')}
                        </Button>
                      )}
                    </div>
                  </motion.div>
                )
              })}
            </AnimatePresence>
          </div>
        )}

        {/* Share CTA */}
        {user && token && (
          <div className="ait-card p-5 flex flex-col sm:flex-row sm:items-center gap-4 justify-between">
            <div className="flex items-center gap-3">
              <Hexagon className="text-primary shrink-0" size={22} />
              <div>
                <div className="t-label">{tr('shareYours')}</div>
                <div className="t-caption text-muted-foreground">
                  {tr('shareYoursDesc')}
                </div>
              </div>
            </div>
            <Button variant="primary" onClick={() => navigate('/strategy')}>
              {tr('openStudio')}
            </Button>
          </div>
        )}
      </div>
    </DeepVoidBackground>
  )
}
