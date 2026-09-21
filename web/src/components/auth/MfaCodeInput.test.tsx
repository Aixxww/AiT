// @vitest-environment jsdom
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { useState } from 'react'
import { MfaCodeInput } from './MfaCodeInput'

/** Mirrors how LoginPage / SecuritySettings own the value. */
function Harness({ onComplete }: { onComplete?: (code: string) => void }) {
  const [value, setValue] = useState('')
  return (
    <MfaCodeInput value={value} onChange={setValue} onComplete={onComplete} />
  )
}

const boxes = () =>
  [1, 2, 3, 4, 5, 6].map((n) => screen.getByLabelText(`Digit ${n}`))

const readValue = () =>
  boxes()
    .map((b) => (b as HTMLInputElement).value)
    .join('')

describe('MfaCodeInput', () => {
  it('renders six single-character boxes', () => {
    render(<Harness />)
    expect(boxes()).toHaveLength(6)
    boxes().forEach((b) => expect(b).toHaveAttribute('maxLength', '1'))
  })

  it('advances through the boxes as digits are typed', () => {
    render(<Harness />)
    const b = boxes()
    fireEvent.change(b[0], { target: { value: '1' } })
    expect(readValue()).toBe('1')
    fireEvent.change(b[1], { target: { value: '2' } })
    fireEvent.change(b[2], { target: { value: '3' } })
    expect(readValue()).toBe('123')
  })

  it('ignores non-numeric input', () => {
    render(<Harness />)
    fireEvent.change(boxes()[0], { target: { value: 'a' } })
    expect(readValue()).toBe('')
  })

  it('fires onComplete once all six digits are present', () => {
    const onComplete = vi.fn()
    render(<Harness onComplete={onComplete} />)
    const b = boxes()
    '123456'.split('').forEach((digit, i) => {
      fireEvent.change(b[i], { target: { value: digit } })
    })
    expect(onComplete).toHaveBeenCalledTimes(1)
    expect(onComplete).toHaveBeenCalledWith('123456')
  })

  it('does not fire onComplete on a partial code', () => {
    const onComplete = vi.fn()
    render(<Harness onComplete={onComplete} />)
    const b = boxes()
    '12345'.split('').forEach((digit, i) => {
      fireEvent.change(b[i], { target: { value: digit } })
    })
    expect(onComplete).not.toHaveBeenCalled()
  })

  it('distributes a pasted code across the boxes', () => {
    const onComplete = vi.fn()
    render(<Harness onComplete={onComplete} />)
    fireEvent.paste(boxes()[0], {
      clipboardData: { getData: () => '654321' },
    })
    expect(readValue()).toBe('654321')
    expect(onComplete).toHaveBeenCalledWith('654321')
  })

  it('strips separators out of a pasted code', () => {
    render(<Harness />)
    fireEvent.paste(boxes()[0], {
      clipboardData: { getData: () => '123 456' },
    })
    expect(readValue()).toBe('123456')
  })

  it('clears the current box on backspace, then steps back', () => {
    render(<Harness />)
    const b = boxes()
    '123'.split('').forEach((digit, i) => {
      fireEvent.change(b[i], { target: { value: digit } })
    })
    expect(readValue()).toBe('123')

    // Cursor sits in box 3 (empty) after typing three digits.
    fireEvent.keyDown(b[3], { key: 'Backspace' })
    expect(readValue()).toBe('12')

    fireEvent.keyDown(b[2], { key: 'Backspace' })
    expect(readValue()).toBe('1')
  })
})
