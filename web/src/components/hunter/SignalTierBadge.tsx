/**
 * Hunter v7 signal tier badge — the design contract for the signal panel.
 *
 * Tier expresses signal QUALITY (attention priority), never direction or
 * PnL: profit/loss green/red are forbidden here. Direction is rendered as
 * a separate LONG/SHORT badge next to this one.
 *
 * Presentation rules (design proposal §5.3):
 * - EXECUTABLE  gold filled badge — the only "act now" color on screen
 * - REVIEWABLE  cyan outlined badge, no background wash
 * - WATCH       gray badge, row content stays at normal brightness
 * - REJECTED    dimmed outline + strikethrough; veto chips stay readable
 */
import { cn } from '../../lib/cn'

export type SignalTier = 'EXECUTABLE' | 'REVIEWABLE' | 'WATCH' | 'REJECTED'

const TIER_STYLES: Record<SignalTier, string> = {
  EXECUTABLE:
    'bg-tier-executable-bg text-tier-executable border-tier-executable/40 font-bold',
  REVIEWABLE:
    'bg-transparent text-tier-reviewable border-tier-reviewable/50 font-semibold',
  WATCH: 'bg-tier-watch-bg text-tier-watch border-transparent font-medium',
  REJECTED:
    'bg-transparent text-tier-rejected border-tier-rejected/40 font-medium line-through',
}

interface SignalTierBadgeProps {
  tier: SignalTier
  className?: string
}

export function SignalTierBadge({
  tier,
  className = '',
}: SignalTierBadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded border px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wider',
        TIER_STYLES[tier],
        className
      )}
    >
      {tier}
    </span>
  )
}
