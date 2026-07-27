import { useEffect, useRef, useState } from 'react'

/**
 * Flash-on-change: returns a short-lived direction marker whenever a
 * numeric value changes, so data cells can pulse their background —
 * motion in the terminal exists to signal data change, nothing else.
 *
 * Pure presentation: no data flow is altered, the timer only clears a
 * CSS class.
 */
export function useFlashOnChange(
  value: number | undefined,
  duration = 400
): 'up' | 'down' | null {
  const prev = useRef<number | undefined>(value)
  const [flash, setFlash] = useState<'up' | 'down' | null>(null)

  useEffect(() => {
    const last = prev.current
    prev.current = value
    if (
      value === undefined ||
      last === undefined ||
      value === last ||
      Number.isNaN(value) ||
      Number.isNaN(last)
    ) {
      return
    }
    setFlash(value > last ? 'up' : 'down')
    const timer = window.setTimeout(() => setFlash(null), duration)
    return () => window.clearTimeout(timer)
  }, [value, duration])

  return flash
}

/** Tailwind-free helper: class names for the flash states. */
export function flashClass(flash: 'up' | 'down' | null): string {
  if (flash === 'up') return 'flash-up'
  if (flash === 'down') return 'flash-down'
  return ''
}
