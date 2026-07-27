import { forwardRef, type ButtonHTMLAttributes } from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '../../lib/cn'

/**
 * Design-token Button — the shared primitive for the P2 component convergence
 * (frontend-design-overhaul-proposal-20260727 §P2).
 *
 * Variants pick semantic tokens only (primary / secondary / ghost / danger,
 * plus the segmented-tab family `tab`/`pill`/`segment` for mode switchers),
 * so the chart-cleanup token guard stays green and the four skins stay in sync.
 * Sizes are sm/md/lg. `active` is a separate boolean so switchers can flip
 * appearance without reusing the directional/disabled semantics of `aria-pressed`
 * — chart tabs need an "active" look that is *not* a pressed toggle state.
 *
 * The cn() merge lets callers append overrides (extra px, layout) on top of the
 * variant base; twMerge ensures conflicting Tailwind utilties resolve predictably.
 */

const buttonVariants = cva(
  // base — shared interaction; subclasses add color, weight and corner radius
  // (segment stays flat by omitting a `rounded` util, matching ingress bar strips).
  'inline-flex items-center justify-center gap-1.5 font-medium transition-all focus:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none',
  {
    variants: {
      variant: {
        primary:
          'bg-primary text-primary-foreground rounded hover:bg-primary/90 active:scale-[0.98]',
        secondary:
          'bg-panel text-foreground border border-border rounded hover:bg-panel-hover hover:border-border-hover',
        ghost: 'text-muted-foreground hover:text-foreground hover:bg-white/5',
        danger:
          'bg-loss text-white rounded hover:bg-loss/90 active:scale-[0.98]',
        // mode switchers (chart tabs etc.)
        tab: 'px-3 py-1.5 rounded-md text-[11px] border',
        pill: 'px-2.5 py-1 text-[10px] rounded border',
        segment: 'px-2 py-1 text-[10px]',
      },
      size: {
        sm: 'text-[11px] px-2 py-1',
        md: 'text-sm px-4 py-2',
        lg: 'text-sm px-4 py-3',
      },
      active: {
        true: '',
        false: '',
      },
    },
    compoundVariants: [
      // tab — primary-dim when active, ghost-like otherwise (chart tabs)
      {
        variant: 'tab',
        active: true,
        className: 'bg-primary-dim text-primary border-primary/20',
      },
      {
        variant: 'tab',
        active: false,
        className:
          'text-muted-foreground hover:text-foreground hover:bg-white/5 border-transparent',
      },
      // pill — elevated neutral when active, transparent otherwise (market pills)
      {
        variant: 'pill',
        active: true,
        className: 'bg-white/10 text-foreground border-white/20',
      },
      {
        variant: 'pill',
        active: false,
        className:
          'text-muted-foreground border-transparent hover:text-foreground hover:bg-white/5',
      },
      // segment — primary wash when active (interval selector)
      {
        variant: 'segment',
        active: true,
        className: 'bg-primary/20 text-primary',
      },
      {
        variant: 'segment',
        active: false,
        className:
          'text-muted-foreground hover:text-foreground hover:bg-white/5',
      },
    ],
    defaultVariants: {
      variant: 'primary',
      size: 'md',
      active: false,
    },
  }
)

export interface ButtonProps
  extends
    ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  /** Presentational active state for switchers; not a toggle/aria-pressed semantic. */
  active?: boolean
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  (
    { className, variant, size, active = false, type = 'button', ...props },
    ref
  ) => {
    return (
      <button
        ref={ref}
        type={type}
        className={cn(buttonVariants({ variant, size, active }), className)}
        {...props}
      />
    )
  }
)
Button.displayName = 'Button'

export { buttonVariants }
