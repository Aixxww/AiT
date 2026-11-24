import {
  useEffect,
  useRef,
  useState,
  type ReactElement,
  type ReactNode,
} from 'react'
import * as Popover from '@radix-ui/react-popover'
import { Info } from 'lucide-react'
import { useOptionalLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { cn } from '../../lib/cn'

interface InfoTipProps {
  /** The explanation. Plain text or rich content. */
  content: ReactNode
  /** Optional heading inside the panel (usually the section title). */
  title?: ReactNode
  /** Accessible name for the trigger; defaults to "More info". */
  label?: string
  side?: 'top' | 'right' | 'bottom' | 'left'
  align?: 'start' | 'center' | 'end'
  className?: string
  /**
   * Custom trigger (e.g. a tag chip). Must be a single element that accepts
   * DOM props; defaults to the circled-info button.
   */
  children?: ReactElement
}

const OPEN_DELAY = 120
const CLOSE_DELAY = 160

/**
 * Circled-info affordance (issue #81). One component for every long
 * explanation in the app.
 *
 * - Desktop (mouse): opens on hover after a short delay and closes when the
 *   pointer leaves both the trigger and the panel.
 * - Touch / pen / keyboard: opens on tap, click, Enter or Space and stays
 *   open until tapped again, Escape, or a tap outside. Nothing is hover-only.
 *
 * Built on Radix Popover so focus, Escape, outside-click and collision
 * handling come from one audited primitive.
 */
export function InfoTip({
  content,
  title,
  label,
  side = 'top',
  align = 'center',
  className,
  children,
}: InfoTipProps) {
  const language = useOptionalLanguage()
  const [open, setOpen] = useState(false)
  // pinned = opened by click/tap/keyboard; hover-leave must not close it.
  const pinned = useRef(false)
  const timer = useRef<number | undefined>(undefined)

  useEffect(() => () => window.clearTimeout(timer.current), [])

  const schedule = (next: boolean, delay: number) => {
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => setOpen(next), delay)
  }
  const onHoverStart = (e: React.PointerEvent) => {
    if (e.pointerType !== 'mouse' || pinned.current) return
    schedule(true, OPEN_DELAY)
  }
  const onHoverEnd = (e: React.PointerEvent) => {
    if (e.pointerType !== 'mouse' || pinned.current) return
    schedule(false, CLOSE_DELAY)
  }

  return (
    <Popover.Root
      open={open}
      onOpenChange={(next) => {
        window.clearTimeout(timer.current)
        if (!next) pinned.current = false
        setOpen(next)
      }}
    >
      <Popover.Trigger
        asChild
        aria-label={
          children ? undefined : (label ?? t('ui.moreInfo', language))
        }
        onPointerEnter={onHoverStart}
        onPointerLeave={onHoverEnd}
        onClick={(e) => {
          // Radix toggles on click; mark the state as pinned so a mouse
          // leaving the trigger does not close a panel the user asked for.
          e.preventDefault()
          e.stopPropagation()
          window.clearTimeout(timer.current)
          const next = !(open && pinned.current)
          pinned.current = next
          setOpen(next)
        }}
      >
        {children ?? (
          <button
            type="button"
            className={cn('ait-infotip-trigger', className)}
            data-state-open={open || undefined}
          >
            <Info aria-hidden className="h-[15px] w-[15px]" strokeWidth={2} />
          </button>
        )}
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          side={side}
          align={align}
          sideOffset={8}
          collisionPadding={12}
          onPointerEnter={(e) => {
            if (e.pointerType === 'mouse') window.clearTimeout(timer.current)
          }}
          onPointerLeave={onHoverEnd}
          onOpenAutoFocus={(e) => e.preventDefault()}
          className="ait-infotip-panel"
        >
          {title && <div className="t-headline mb-1">{title}</div>}
          <div className="t-body text-muted-foreground whitespace-pre-line">
            {content}
          </div>
          <Popover.Arrow className="ait-infotip-arrow" width={12} height={6} />
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}
