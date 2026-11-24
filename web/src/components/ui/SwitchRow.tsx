import { useId, type ReactNode } from 'react'
import { cn } from '../../lib/cn'
import { InfoTip } from './InfoTip'

interface SwitchRowProps {
  title: ReactNode
  /** One line under the title. */
  summary?: ReactNode
  /** Long explanation behind the info affordance. */
  info?: ReactNode
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
  className?: string
}

/** Setting row: title, one-line summary, optional info, switch on the right. */
export function SwitchRow({
  title,
  summary,
  info,
  checked,
  onChange,
  disabled,
  className,
}: SwitchRowProps) {
  const id = useId()
  return (
    <div
      className={cn('flex items-start justify-between gap-4 py-2.5', className)}
    >
      <div className="min-w-0">
        <div className="flex items-center gap-1.5">
          <label htmlFor={id} className="t-label cursor-pointer">
            {title}
          </label>
          {info && (
            <InfoTip
              content={info}
              title={typeof title === 'string' ? title : undefined}
            />
          )}
        </div>
        {summary && (
          <p className="t-caption text-muted-foreground mt-0.5">{summary}</p>
        )}
      </div>
      <button
        id={id}
        type="button"
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        onClick={() => !disabled && onChange(!checked)}
        className={cn(
          'relative mt-0.5 inline-flex h-[26px] w-[44px] shrink-0 items-center rounded-pill border transition-colors',
          checked
            ? 'bg-primary border-primary'
            : 'bg-surface-alt border-border',
          'disabled:opacity-50 disabled:cursor-not-allowed focus-visible:outline-2 focus-visible:outline-primary'
        )}
      >
        <span
          className={cn(
            'inline-block h-[20px] w-[20px] rounded-full bg-white shadow-e1 transition-transform duration-200',
            checked ? 'translate-x-[20px]' : 'translate-x-[2px]'
          )}
        />
      </button>
    </div>
  )
}
