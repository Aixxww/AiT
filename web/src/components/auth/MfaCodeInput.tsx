import { useEffect, useRef, type ClipboardEvent, type KeyboardEvent } from 'react'

const DIGITS_ONLY = /^[0-9]$/
const LENGTH = 6

interface MfaCodeInputProps {
  /** Raw concatenated digits, e.g. "123456". Always length <= 6. */
  value: string
  onChange: (value: string) => void
  /** Fired when all six boxes are filled — by typing or by pasting. */
  onComplete?: (value: string) => void
  disabled?: boolean
  autoFocus?: boolean
}

/**
 * Six single-character boxes for a TOTP code.
 *
 * Deliberately uncontrolled-per-box and derived from `value`, so the parent
 * owns the truth (it needs the code for the submit handler and must be able to
 * clear it after a failure). Supports paste — including pasting the whole code
 * into any box — and backspace-to-previous, which is what users expect from
 * this pattern. `autoComplete="one-time-code"` lets iOS/Android offer the code
 * from an SMS/app autofill without extra work.
 */
export function MfaCodeInput({
  value,
  onChange,
  onComplete,
  disabled = false,
  autoFocus = false,
}: MfaCodeInputProps) {
  const inputs = useRef<Array<HTMLInputElement | null>>([])
  const chars = Array.from({ length: LENGTH }, (_, i) => value[i] ?? '')

  useEffect(() => {
    if (autoFocus) {
      inputs.current[0]?.focus()
    }
  }, [autoFocus])

  const focusAt = (index: number) => {
    const clamped = Math.max(0, Math.min(LENGTH - 1, index))
    const target = inputs.current[clamped]
    target?.focus()
    target?.select()
  }

  const commit = (next: string[]) => {
    const joined = next.join('')
    onChange(joined)
    if (joined.length === LENGTH && next.every((c) => DIGITS_ONLY.test(c))) {
      onComplete?.(joined)
    }
  }

  /** Writes `source` starting at `index`, advancing one box per character. */
  const writeFrom = (index: number, source: string) => {
    const next = [...chars]
    let cursor = index
    for (const char of source) {
      if (cursor >= LENGTH) break
      next[cursor] = char
      cursor += 1
    }
    commit(next)
    focusAt(cursor)
  }

  const handleChange = (index: number, raw: string) => {
    const typed = raw.replace(/\D/g, '')
    if (!typed) {
      return
    }
    // Typing into a filled box overwrites it rather than appending.
    writeFrom(index, typed)
  }

  const handleKeyDown = (
    index: number,
    event: KeyboardEvent<HTMLInputElement>
  ) => {
    if (event.key === 'Backspace') {
      event.preventDefault()
      const next = [...chars]
      if (next[index]) {
        next[index] = ''
        commit(next)
        focusAt(index)
      } else if (index > 0) {
        next[index - 1] = ''
        commit(next)
        focusAt(index - 1)
      }
      return
    }
    if (event.key === 'ArrowLeft') {
      event.preventDefault()
      focusAt(index - 1)
    }
    if (event.key === 'ArrowRight') {
      event.preventDefault()
      focusAt(index + 1)
    }
  }

  const handlePaste = (
    index: number,
    event: ClipboardEvent<HTMLInputElement>
  ) => {
    const pasted = event.clipboardData.getData('text').replace(/\D/g, '')
    if (!pasted) {
      return
    }
    event.preventDefault()
    writeFrom(index, pasted)
  }

  return (
    <div className="flex items-center justify-between gap-2">
      {chars.map((char, index) => (
        <input
          key={index}
          ref={(el) => {
            inputs.current[index] = el
          }}
          type="text"
          inputMode="numeric"
          autoComplete={index === 0 ? 'one-time-code' : 'off'}
          maxLength={1}
          value={char}
          disabled={disabled}
          aria-label={`Digit ${index + 1}`}
          onChange={(e) => handleChange(index, e.target.value)}
          onKeyDown={(e) => handleKeyDown(index, e)}
          onPaste={(e) => handlePaste(index, e)}
          onFocus={(e) => e.target.select()}
          className="w-11 h-14 text-center text-xl font-mono bg-background/80 border border-border/80 rounded-xl text-foreground focus:outline-none focus:border-primary/60 focus:ring-1 focus:ring-primary/30 transition-all disabled:opacity-50"
        />
      ))}
    </div>
  )
}
