import useSWR from 'swr'
import { useAuth } from '../../contexts/AuthContext'
import { api } from '../../lib/api'
import { ArrowUpRight, ArrowDownRight, Wallet } from 'lucide-react'
import type { Position, TraderInfo } from '../../types'

export function PositionsPanel() {
  const { user, token } = useAuth()

  const { data: traders } = useSWR<TraderInfo[]>(
    user && token ? 'agent-traders' : null,
    api.getTraders,
    { refreshInterval: 30000, shouldRetryOnError: false }
  )

  // Get first running trader's positions
  const runningTrader = traders?.find((t) => t.is_running)
  const traderId = runningTrader?.trader_id

  const { data: positions } = useSWR<Position[]>(
    traderId ? `agent-positions-${traderId}` : null,
    () => api.getPositions(traderId),
    { refreshInterval: 15000, shouldRetryOnError: false }
  )

  if (!user || !token) {
    return (
      <div
        style={{
          padding: '20px 14px',
          textAlign: 'center',
          color: 'var(--color-muted-fg)',
          fontSize: 12,
        }}
      >
        <Wallet size={20} style={{ margin: '0 auto 8px', opacity: 0.5 }} />
        <div>Login to view positions</div>
      </div>
    )
  }

  const openPositions = positions?.filter((p) => p.quantity !== 0) || []

  if (openPositions.length === 0) {
    return (
      <div
        style={{
          padding: '16px 14px',
          textAlign: 'center',
          color: 'var(--color-muted-fg)',
          fontSize: 12,
        }}
      >
        No open positions
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      {openPositions.map((pos, i) => {
        const pnl = pos.unrealized_pnl
        const isProfit = pnl >= 0
        const color = isProfit ? 'var(--color-profit)' : 'var(--color-loss)'
        const side =
          pos.side?.toUpperCase() || (pos.quantity > 0 ? 'LONG' : 'SHORT')
        const rawSymbol = pos.symbol || ''
        // Stock symbols are pure letters (1-5 chars), crypto has USDT suffix
        const isStock =
          /^[A-Z]{1,5}$/.test(rawSymbol) && !rawSymbol.endsWith('USDT')
        const symbol = isStock ? rawSymbol : rawSymbol.replace('USDT', '')
        const currencyPrefix = isStock ? '$' : ''
        const marginUsed =
          pos.margin_used > 0
            ? pos.margin_used
            : pos.leverage > 0
              ? Math.abs(pos.quantity * pos.mark_price) / pos.leverage
              : 0

        return (
          <div
            key={i}
            style={{
              padding: '10px 12px',
              background: 'var(--color-surface)',
              borderRadius: 10,
              border: '1px solid var(--color-border)',
            }}
          >
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 6,
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <span
                  style={{
                    fontSize: 13,
                    fontWeight: 600,
                    color: 'var(--color-foreground)',
                  }}
                >
                  {symbol}
                </span>
                {isStock && (
                  <span
                    style={{ fontSize: 10, color: 'var(--color-muted-fg)' }}
                  >
                    🇺🇸
                  </span>
                )}
                <span
                  style={{
                    fontSize: 10,
                    fontWeight: 600,
                    padding: '1px 5px',
                    borderRadius: 4,
                    background:
                      side === 'LONG'
                        ? 'color-mix(in srgb, var(--color-profit) 12%, transparent)'
                        : 'color-mix(in srgb, var(--color-loss) 12%, transparent)',
                    color:
                      side === 'LONG'
                        ? 'var(--color-profit)'
                        : 'var(--color-loss)',
                  }}
                >
                  {isStock ? (side === 'LONG' ? 'HOLD' : 'SHORT') : side}
                </span>
              </div>
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 3,
                  color,
                  fontSize: 12,
                  fontWeight: 600,
                }}
              >
                {isProfit ? (
                  <ArrowUpRight size={12} />
                ) : (
                  <ArrowDownRight size={12} />
                )}
                {isProfit ? '+' : ''}
                {currencyPrefix}
                {pnl.toFixed(2)}
              </div>
            </div>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                fontSize: 11,
                color: 'var(--color-muted-fg)',
              }}
            >
              <span>
                {isStock ? 'Shares' : 'Qty'}: {pos.quantity}
              </span>
              <span>
                Entry: {currencyPrefix}
                {pos.entry_price.toFixed(2)}
              </span>
            </div>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                fontSize: 11,
                color: 'var(--color-muted-fg)',
                marginTop: 3,
              }}
            >
              <span>
                Margin: {currencyPrefix}
                {marginUsed > 0 ? marginUsed.toFixed(2) : '--'}
              </span>
              <span>Lev: {pos.leverage}x</span>
            </div>
          </div>
        )
      })}
    </div>
  )
}
