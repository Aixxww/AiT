import { useState } from 'react'
import { Trophy } from 'lucide-react'
import useSWR from 'swr'
import { api } from '../../lib/api'
import type { CompetitionData } from '../../types'
import { ComparisonChart } from '../charts/ComparisonChart'
import { TraderConfigViewModal } from './TraderConfigViewModal'
import { getTraderColor } from '../../utils/traderColors'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { PunkAvatar, getTraderAvatar } from '../common/PunkAvatar'
import { DeepVoidBackground } from '../common/DeepVoidBackground'

export function CompetitionPage() {
  const { language } = useLanguage()
  const [selectedTrader, setSelectedTrader] = useState<any>(null)
  const [isModalOpen, setIsModalOpen] = useState(false)

  const { data: competition } = useSWR<CompetitionData>(
    'competition',
    api.getCompetition,
    {
      refreshInterval: 15000, // 15秒刷新（竞赛数据不需要太频繁更新）
      revalidateOnFocus: false,
      dedupingInterval: 10000,
    }
  )

  const handleTraderClick = async (traderId: string) => {
    try {
      const traderConfig = await api.getPublicTraderConfig(traderId)
      setSelectedTrader(traderConfig)
      setIsModalOpen(true)
    } catch (error) {
      console.error('Failed to fetch trader config:', error)
      // 对于未登录用户，不显示详细配置，这是正常行为
      // 竞赛页面主要用于查看排行榜和基本信息
    }
  }

  const closeModal = () => {
    setIsModalOpen(false)
    setSelectedTrader(null)
  }

  if (!competition) {
    return (
      <DeepVoidBackground className="py-8" disableAnimation>
        <div className="container mx-auto max-w-7xl px-4 md:px-8">
          <div className="space-y-6">
            <div className="animate-pulse bg-black/40 border border-white/10 rounded-xl p-8 backdrop-blur-md">
              <div className="flex items-center justify-between mb-6">
                <div className="space-y-3 flex-1">
                  <div className="h-8 w-64 bg-white/5 rounded"></div>
                  <div className="h-4 w-48 bg-white/5 rounded"></div>
                </div>
                <div className="h-12 w-32 bg-white/5 rounded"></div>
              </div>
            </div>
            <div className="bg-black/40 border border-white/10 rounded-xl p-6 backdrop-blur-md">
              <div className="h-6 w-40 mb-4 bg-white/5 rounded"></div>
              <div className="space-y-3">
                <div className="h-20 w-full bg-white/5 rounded"></div>
                <div className="h-20 w-full bg-white/5 rounded"></div>
              </div>
            </div>
          </div>
        </div>
      </DeepVoidBackground>
    )
  }

  // 如果有数据返回但没有交易员，显示空状态
  if (!competition.traders || competition.traders.length === 0) {
    return (
      <DeepVoidBackground className="py-8" disableAnimation>
        <div className="container mx-auto max-w-7xl px-4 md:px-8 space-y-8 animate-fade-in">
          {/* Competition Header - 精简版 */}
          <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-3 md:gap-0">
            <div className="flex items-center gap-3 md:gap-4">
              <div className="w-10 h-10 md:w-12 md:h-12 rounded-xl flex items-center justify-center bg-black/60 border border-ait-gold/30 shadow-[0_0_15px_rgba(240,185,11,0.2)]">
                <Trophy className="w-6 h-6 md:w-7 md:h-7 text-primary" />
              </div>
              <div>
                <h1 className="text-xl md:text-2xl font-bold flex items-center gap-2 text-foreground">
                  {t('aiCompetition', language)}
                  <span className="text-xs font-normal px-2 py-1 rounded bg-primary-dim text-primary border border-ait-gold/20">
                    0 {t('traders', language)}
                  </span>
                </h1>
                <p className="text-xs text-muted-foreground">
                  {t('liveBattle', language)}
                </p>
              </div>
            </div>
          </div>

          {/* Empty State */}
          <div className="bg-black/40 border border-white/10 rounded-xl p-16 text-center backdrop-blur-md">
            <Trophy className="w-16 h-16 mx-auto mb-4 text-zinc-700" />
            <h3 className="text-lg font-bold mb-2 text-foreground">
              {t('noTraders', language)}
            </h3>
            <p className="text-sm text-muted-foreground">
              {t('createFirstTrader', language)}
            </p>
          </div>
        </div>
      </DeepVoidBackground>
    )
  }

  // 按收益率排序
  const sortedTraders = [...competition.traders].sort(
    (a, b) => b.total_pnl_pct - a.total_pnl_pct
  )

  // 找出领先者
  const leader = sortedTraders[0]

  return (
    <DeepVoidBackground className="py-8" disableAnimation>
      <div className="w-full px-4 md:px-8 space-y-8 animate-fade-in">
        {/* Competition Header - 精简版 */}
        <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-3 md:gap-0">
          <div className="flex items-center gap-3 md:gap-4">
            <div className="w-10 h-10 md:w-12 md:h-12 rounded-xl flex items-center justify-center bg-black/60 border border-ait-gold/30 shadow-[0_0_15px_rgba(240,185,11,0.2)]">
              <Trophy className="w-6 h-6 md:w-7 md:h-7 text-primary" />
            </div>
            <div>
              <h1 className="text-xl md:text-2xl font-bold flex items-center gap-2 text-foreground">
                {t('aiCompetition', language)}
                <span className="text-xs font-normal px-2 py-1 rounded bg-primary-dim text-primary border border-ait-gold/20">
                  {competition.count} {t('traders', language)}
                </span>
              </h1>
              <p className="text-xs text-muted-foreground">
                {t('liveBattle', language)}
              </p>
            </div>
          </div>
          <div className="text-left md:text-right w-full md:w-auto">
            <div className="text-xs mb-1 text-muted-foreground">
              {t('leader', language)}
            </div>
            <div className="text-base md:text-lg font-bold text-primary">
              {leader?.trader_name}
            </div>
            <div
              className="text-sm font-semibold"
              style={{
                color:
                  (leader?.total_pnl ?? 0) >= 0
                    ? 'var(--color-profit)'
                    : 'var(--color-loss)',
              }}
            >
              {(leader?.total_pnl ?? 0) >= 0 ? '+' : ''}
              {leader?.total_pnl_pct?.toFixed(2) || '0.00'}%
            </div>
          </div>
        </div>

        {/* Left/Right Split: Performance Chart + Leaderboard */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Left: Performance Comparison Chart */}
          <div
            className="bg-black/40 border border-white/10 rounded-xl p-6 backdrop-blur-md animate-slide-in hover:border-white/20 transition-colors"
            style={{ animationDelay: '0.1s' }}
          >
            <div className="flex items-center justify-between mb-6">
              <h2 className="text-lg font-bold flex items-center gap-2 text-foreground">
                {t('performanceComparison', language)}
              </h2>
              <div className="text-xs text-muted-foreground">
                {t('realTimePnL', language)}
              </div>
            </div>
            <ComparisonChart traders={sortedTraders.slice(0, 10)} />
          </div>

          {/* Right: Leaderboard */}
          <div
            className="bg-black/40 border border-white/10 rounded-xl p-6 backdrop-blur-md animate-slide-in hover:border-white/20 transition-colors"
            style={{ animationDelay: '0.1s' }}
          >
            <div className="flex items-center justify-between mb-6">
              <h2 className="text-lg font-bold flex items-center gap-2 text-foreground">
                {t('leaderboard', language)}
              </h2>
              <div className="text-xs px-2 py-1 rounded bg-primary-dim text-primary border border-ait-gold/20 shadow-[0_0_8px_rgba(240,185,11,0.1)]">
                {t('live', language)}
              </div>
            </div>
            <div className="space-y-2">
              {sortedTraders.map((trader, index) => {
                const isLeader = index === 0
                const traderColor = getTraderColor(
                  sortedTraders,
                  trader.trader_id
                )

                return (
                  <div
                    key={trader.trader_id}
                    onClick={() => handleTraderClick(trader.trader_id)}
                    className="rounded p-3 transition-all duration-300 hover:translate-y-[-1px] cursor-pointer hover:shadow-lg"
                    style={{
                      background: isLeader
                        ? 'linear-gradient(135deg, color-mix(in srgb, var(--color-primary) 8%, transparent) 0%, var(--background) 100%)'
                        : 'var(--background)',
                      border: `1px solid ${isLeader ? 'color-mix(in srgb, var(--color-primary) 40%, transparent)' : 'var(--color-border)'}`,
                      boxShadow: isLeader
                        ? '0 3px 15px color-mix(in srgb, var(--color-primary) 12%, transparent), 0 0 0 1px color-mix(in srgb, var(--color-primary) 15%, transparent)'
                        : '0 1px 4px rgba(0, 0, 0, 0.3)',
                    }}
                  >
                    <div className="flex items-center justify-between">
                      {/* Rank & Avatar & Name */}
                      <div className="flex items-center gap-3">
                        {/* Rank Badge */}
                        <div
                          className="w-6 h-6 rounded-full flex items-center justify-center text-xs font-bold"
                          style={{
                            background:
                              index === 0
                                ? 'linear-gradient(135deg, var(--color-primary) 0%, var(--color-primary) 100%)'
                                : index === 1
                                  ? 'linear-gradient(135deg, #C0C0C0 0%, #E8E8E8 100%)'
                                  : index === 2
                                    ? 'linear-gradient(135deg, #CD7F32 0%, #E8A64C 100%)'
                                    : 'var(--color-border)',
                            color:
                              index < 3
                                ? 'var(--color-primary-fg)'
                                : 'var(--color-muted-fg)',
                          }}
                        >
                          {index + 1}
                        </div>
                        {/* Punk Avatar */}
                        <PunkAvatar
                          seed={getTraderAvatar(
                            trader.trader_id,
                            trader.trader_name
                          )}
                          size={36}
                          className="rounded-lg"
                        />
                        <div>
                          <div className="font-bold text-sm text-foreground">
                            {trader.trader_name}
                          </div>
                          <div
                            className="text-xs mono font-semibold"
                            style={{ color: traderColor }}
                          >
                            {trader.ai_model.toUpperCase()} +{' '}
                            {trader.exchange.toUpperCase()}
                          </div>
                        </div>
                      </div>

                      {/* Stats */}
                      <div className="flex items-center gap-2 md:gap-3 flex-wrap md:flex-nowrap">
                        {/* Total Equity */}
                        <div className="text-right">
                          <div className="text-xs text-muted-foreground">
                            {t('equity', language)}
                          </div>
                          <div className="text-xs md:text-sm font-bold mono text-foreground">
                            {trader.total_equity?.toFixed(2) || '0.00'}
                          </div>
                        </div>

                        {/* P&L */}
                        <div className="text-right min-w-[70px] md:min-w-[90px]">
                          <div className="text-xs text-muted-foreground">
                            {t('pnl', language)}
                          </div>
                          <div
                            className="text-base md:text-lg font-bold mono"
                            style={{
                              color:
                                (trader.total_pnl ?? 0) >= 0
                                  ? 'var(--color-profit)'
                                  : 'var(--color-loss)',
                            }}
                          >
                            {(trader.total_pnl ?? 0) >= 0 ? '+' : ''}
                            {trader.total_pnl_pct?.toFixed(2) || '0.00'}%
                          </div>
                          <div className="text-xs mono text-muted-foreground">
                            {(trader.total_pnl ?? 0) >= 0 ? '+' : ''}
                            {trader.total_pnl?.toFixed(2) || '0.00'}
                          </div>
                        </div>

                        {/* Positions */}
                        <div className="text-right">
                          <div className="text-xs text-muted-foreground">
                            {t('pos', language)}
                          </div>
                          <div className="text-xs md:text-sm font-bold mono text-foreground">
                            {trader.position_count}
                          </div>
                          <div className="text-xs text-muted-foreground">
                            {trader.margin_used_pct.toFixed(1)}%
                          </div>
                        </div>

                        {/* Status */}
                        <div>
                          <div
                            className="px-2 py-1 rounded text-xs font-bold"
                            style={
                              trader.is_running
                                ? {
                                    background:
                                      'color-mix(in srgb, var(--color-profit) 10%, transparent)',
                                    color: 'var(--color-profit)',
                                  }
                                : {
                                    background:
                                      'color-mix(in srgb, var(--color-loss) 10%, transparent)',
                                    color: 'var(--color-loss)',
                                  }
                            }
                          >
                            {trader.is_running ? '●' : '○'}
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </div>

        {/* Head-to-Head Stats */}
        {competition.traders.length === 2 && (
          <div
            className="bg-black/40 border border-white/10 rounded-xl p-6 backdrop-blur-md animate-slide-in"
            style={{ animationDelay: '0.3s' }}
          >
            <h2 className="text-lg font-bold mb-6 flex items-center gap-2 text-foreground">
              {t('headToHead', language)}
            </h2>
            <div className="grid grid-cols-2 gap-4">
              {sortedTraders.map((trader, index) => {
                const isWinning = index === 0
                const opponent = sortedTraders[1 - index]

                // Check if both values are valid numbers
                const hasValidData =
                  trader.total_pnl_pct != null &&
                  opponent.total_pnl_pct != null &&
                  !isNaN(trader.total_pnl_pct) &&
                  !isNaN(opponent.total_pnl_pct)

                const gap = hasValidData
                  ? trader.total_pnl_pct - opponent.total_pnl_pct
                  : NaN

                return (
                  <div
                    key={trader.trader_id}
                    className="p-4 rounded transition-all duration-300 hover:scale-[1.02]"
                    style={
                      isWinning
                        ? {
                            background:
                              'linear-gradient(135deg, color-mix(in srgb, var(--color-profit) 8%, transparent) 0%, color-mix(in srgb, var(--color-profit) 2%, transparent) 100%)',
                            border:
                              '2px solid color-mix(in srgb, var(--color-profit) 30%, transparent)',
                            boxShadow:
                              '0 3px 15px color-mix(in srgb, var(--color-profit) 12%, transparent)',
                          }
                        : {
                            background: 'var(--background)',
                            border: '1px solid var(--color-border)',
                            boxShadow: '0 1px 4px rgba(0, 0, 0, 0.3)',
                          }
                    }
                  >
                    <div className="text-center">
                      {/* Avatar */}
                      <div className="flex justify-center mb-3">
                        <PunkAvatar
                          seed={getTraderAvatar(
                            trader.trader_id,
                            trader.trader_name
                          )}
                          size={56}
                          className="rounded-xl"
                        />
                      </div>
                      <div
                        className="text-sm md:text-base font-bold mb-2"
                        style={{
                          color: getTraderColor(
                            sortedTraders,
                            trader.trader_id
                          ),
                        }}
                      >
                        {trader.trader_name}
                      </div>
                      <div
                        className="text-lg md:text-2xl font-bold mono mb-1"
                        style={{
                          color:
                            (trader.total_pnl ?? 0) >= 0
                              ? 'var(--color-profit)'
                              : 'var(--color-loss)',
                        }}
                      >
                        {trader.total_pnl_pct != null &&
                        !isNaN(trader.total_pnl_pct)
                          ? `${trader.total_pnl_pct >= 0 ? '+' : ''}${trader.total_pnl_pct.toFixed(2)}%`
                          : '—'}
                      </div>
                      {hasValidData && isWinning && gap > 0 && (
                        <div
                          className="text-xs font-semibold"
                          style={{ color: 'var(--color-profit)' }}
                        >
                          {t('leadingBy', language, { gap: gap.toFixed(2) })}
                        </div>
                      )}
                      {hasValidData && !isWinning && gap < 0 && (
                        <div
                          className="text-xs font-semibold"
                          style={{ color: 'var(--color-loss)' }}
                        >
                          {t('behindBy', language, {
                            gap: Math.abs(gap).toFixed(2),
                          })}
                        </div>
                      )}
                      {!hasValidData && (
                        <div className="text-xs font-semibold text-muted-foreground">
                          —
                        </div>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        )}

        {/* Trader Config View Modal */}
        <TraderConfigViewModal
          isOpen={isModalOpen}
          onClose={closeModal}
          traderData={selectedTrader}
        />
      </div>
    </DeepVoidBackground>
  )
}
