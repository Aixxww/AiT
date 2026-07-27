/**
 * Veto reason chip — renders a Hunter v7 confirmation/veto code (e.g.
 * `fresh_oi_absent`, `mms_weak_continuation_review_only`) as a compact
 * mono chip.
 *
 * Veto reasons are first-class data in the v7 lean kernel: they must stay
 * readable (muted-fg, ≥4.5:1) even inside an otherwise dimmed REJECTED
 * row — never inherit the row's disabled color.
 */

interface VetoChipProps {
  /** Raw confirmation/veto code, displayed verbatim. */
  code: string
  /** Optional catalog explanation shown on hover. */
  title?: string
  className?: string
}

export function VetoChip({ code, title, className = '' }: VetoChipProps) {
  return (
    <span
      title={title}
      className={`inline-flex items-center rounded border border-border bg-muted/30 px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground ${className}`}
    >
      {code}
    </span>
  )
}
