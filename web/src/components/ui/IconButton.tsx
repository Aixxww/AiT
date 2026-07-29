import { forwardRef, type ButtonHTMLAttributes } from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '../../lib/cn'

const iconButtonVariants = cva(
  'inline-flex shrink-0 items-center justify-center rounded-md transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50',
  {
    variants: {
      variant: {
        ghost: 'text-muted-foreground hover:bg-muted/30 hover:text-foreground',
        secondary:
          'border border-border bg-panel text-muted-foreground hover:border-border-hover hover:bg-panel-hover hover:text-foreground',
        danger: 'text-loss hover:bg-loss/10',
      },
      size: {
        sm: 'h-7 w-7',
        md: 'h-8 w-8',
        lg: 'h-9 w-9',
      },
      active: {
        true: '',
        false: '',
      },
    },
    compoundVariants: [
      {
        variant: 'ghost',
        active: true,
        className: 'bg-muted/40 text-foreground',
      },
      {
        variant: 'secondary',
        active: true,
        className: 'border-primary/30 bg-primary-dim text-primary',
      },
    ],
    defaultVariants: {
      variant: 'ghost',
      size: 'md',
      active: false,
    },
  }
)

export interface IconButtonProps
  extends
    ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof iconButtonVariants> {
  active?: boolean
}

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(
  (
    { className, variant, size, active = false, type = 'button', ...props },
    ref
  ) => {
    return (
      <button
        ref={ref}
        type={type}
        className={cn(iconButtonVariants({ variant, size, active }), className)}
        {...props}
      />
    )
  }
)

IconButton.displayName = 'IconButton'

export { iconButtonVariants }
