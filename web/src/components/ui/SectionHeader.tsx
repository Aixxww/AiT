import type { ReactNode } from 'react'
import { cn } from '../../lib/cn'
import { InfoTip } from './InfoTip'

interface SectionHeaderProps {
  title: ReactNode
  /** One line, shown under the title. Long text belongs in `info`. */
  summary?: ReactNode
  /** Long explanation, behind the circled-info affordance. */
  info?: ReactNode
  icon?: ReactNode
  /** Right-aligned controls (toggles, buttons). */
  actions?: ReactNode
  /** 'section' for page sections, 'group' for sub-groups inside a card. */
  level?: 'section' | 'group'
  className?: string
}

/**
 * Title + one-line summary + optional info affordance (issue #81).
 * Every module/section header in the app uses this shape.
 */
export function SectionHeader({
  title,
  summary,
  info,
  icon,
  actions,
  level = 'section',
  className,
}: SectionHeaderProps) {
  return (
    <div className={cn('flex items-start justify-between gap-3', className)}>
      <div className="flex min-w-0 items-start gap-2.5">
        {icon && (
          <span className="mt-0.5 shrink-0 text-muted-foreground">{icon}</span>
        )}
        <div className="min-w-0">
          <div className="flex items-center gap-1.5">
            <span className={level === 'section' ? 't-headline' : 't-label'}>
              {title}
            </span>
            {info && (
              <InfoTip
                content={info}
                title={typeof title === 'string' ? title : undefined}
              />
            )}
          </div>
          {summary && (
            <p className="t-caption mt-0.5 text-muted-foreground line-clamp-2">
              {summary}
            </p>
          )}
        </div>
      </div>
      {actions && (
        <div className="flex shrink-0 items-center gap-2">{actions}</div>
      )}
    </div>
  )
}
