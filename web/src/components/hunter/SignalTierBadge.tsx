/**
 * Hunter v7 signal tier badge — the design contract for the signal panel.
 *
 * Tier expresses signal QUALITY (attention priority), never direction or
 * PnL: profit/loss green/red are forbidden here. Direction is rendered as
 * a separate Long/Short badge next to this one.
 *
 * Presentation rules (design proposal §5.3):
 * - EXECUTABLE  gold filled badge — the only "act now" color on screen
 * - REVIEWABLE  cyan outlined badge, no background wash
 * - WATCH       gray badge, row content stays at normal brightness
 * - REJECTED    dimmed outline + strikethrough; veto chips stay readable
 *
 * The label comes from the shared display-name map (issue #81).
 */
import { cn } from '../../lib/cn'
import { displayName } from '../../i18n/displayNames'
import type { Language } from '../../i18n/translations'

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
  language?: Language
}

export function SignalTierBadge({
  tier,
  className = '',
  language = 'en',
}: SignalTierBadgeProps) {
  return (
    <span
      data-tier={tier}
      className={cn(
        'inline-flex items-center rounded-pill border px-2 py-0.5 text-[11px] tracking-wide',
        TIER_STYLES[tier],
        className
      )}
    >
      {displayName('tier', tier, language)}
    </span>
  )
}
