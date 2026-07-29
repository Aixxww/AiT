import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { Button } from './Button'

describe('Button', () => {
  it('renders with default primary variant and md size', () => {
    render(<Button>Confirm</Button>)
    const btn = screen.getByText('Confirm')
    expect(btn.tagName).toBe('BUTTON')
    expect(btn.className).toMatch(/bg-primary text-primary-foreground/)
    expect(btn.className).toMatch(/text-sm px-4 py-2/)
  })

  it('defaults type to "button" to avoid accidental form submits', () => {
    render(<Button>Ok</Button>)
    expect(screen.getByText('Ok').getAttribute('type')).toBe('button')
  })

  it('keeps unstyled buttons native so migrated legacy form buttons still submit', () => {
    render(<Button variant="unstyled">Legacy</Button>)
    expect(screen.getByText('Legacy')).not.toHaveAttribute('type')
  })

  it('forwards an explicit type override', () => {
    render(<Button type="submit">Go</Button>)
    expect(screen.getByText('Go').getAttribute('type')).toBe('submit')
  })

  it.each([
    ['primary', /bg-primary text-primary-foreground/],
    ['unstyled', /inline-flex items-center justify-center/],
    ['secondary', /bg-panel text-foreground/],
    ['ghost', /text-muted-foreground hover:text-foreground/],
    ['danger', /bg-loss text-white/],
  ] as const)('applies the %s variant palette', (variant, pattern) => {
    render(<Button variant={variant}>{variant}</Button>)
    expect(screen.getByText(variant).className).toMatch(pattern)
  })

  it.each([
    ['sm', /text-\[11px\] px-2 py-1/],
    ['md', /text-sm px-4 py-2/],
    ['lg', /text-sm px-4 py-3/],
  ] as const)('applies the %s size tokens', (size, pattern) => {
    render(<Button size={size}>{size}</Button>)
    expect(screen.getByText(size).className).toMatch(pattern)
  })

  it('flips tab appearance on active without relying on aria-pressed', () => {
    const { rerender } = render(<Button variant="tab">Tab</Button>)
    const inactive = screen.getByText('Tab').className
    expect(inactive).toMatch(/text-muted-foreground/)
    expect(inactive).toContain('border-transparent')
    rerender(
      <Button variant="tab" active>
        Tab
      </Button>
    )
    expect(screen.getByText('Tab').className).toMatch(
      /bg-primary-dim text-primary border-primary\/20/
    )
  })

  it('renders the pill active/inactive and segment active/inactive shapes', () => {
    const { rerender } = render(<Button variant="pill">P</Button>)
    const inactive = screen.getByText('P').className
    expect(inactive).toMatch(/text-muted-foreground/)
    expect(inactive).toContain('border-transparent')
    rerender(
      <Button variant="pill" active>
        P
      </Button>
    )
    expect(screen.getByText('P').className).toMatch(
      /bg-white\/10 text-foreground border-white\/20/
    )
    rerender(<Button variant="segment">S</Button>)
    expect(screen.getByText('S').className).toMatch(/text-muted-foreground/)
    rerender(
      <Button variant="segment" active>
        S
      </Button>
    )
    expect(screen.getByText('S').className).toMatch(
      /bg-primary\/20 text-primary/
    )
  })

  it('forwards ref and arbitrary button attributes (aria-label, disabled)', () => {
    const ref = { current: null as HTMLButtonElement | null }
    render(
      <Button ref={ref as any} disabled aria-label="send" data-testid="x">
        Send
      </Button>
    )
    const btn = screen.getByTestId('x')
    expect(btn).toHaveAttribute('aria-label', 'send')
    expect(btn).toBeDisabled()
    expect(ref.current).toBe(btn)
  })

  it('passes className overrides via the cn() tailwind-merge path', () => {
    render(
      <Button className="w-full mt-4 py-3" variant="primary">
        Signup
      </Button>
    )
    const cls = screen.getByText('Signup').className
    expect(cls).toMatch(/w-full mt-4/)
    // py-2 (md default) should be overridden by py-3 via twMerge
    expect(cls).not.toMatch(/py-2(?!.*py-3)/)
    expect(cls).toMatch(/py-3/)
  })

  it('fires the click handler', () => {
    const onClick = vi.fn()
    render(<Button onClick={onClick}>Tap</Button>)
    fireEvent.click(screen.getByText('Tap'))
    expect(onClick).toHaveBeenCalledTimes(1)
  })
})
