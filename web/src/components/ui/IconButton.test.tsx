import { render, screen } from '@testing-library/react'
import { IconButton } from './IconButton'

describe('IconButton', () => {
  it('defaults to type button', () => {
    render(<IconButton aria-label="refresh">R</IconButton>)
    expect(screen.getByLabelText('refresh')).toHaveAttribute('type', 'button')
  })

  it('applies variants, sizes and active state', () => {
    const { rerender } = render(
      <IconButton aria-label="open" variant="secondary" size="sm" active>
        O
      </IconButton>
    )
    expect(screen.getByLabelText('open').className).toContain('border-primary')

    rerender(
      <IconButton aria-label="delete" variant="danger" size="lg">
        D
      </IconButton>
    )
    expect(screen.getByLabelText('delete').className).toContain('text-loss')
    expect(screen.getByLabelText('delete').className).toContain('h-9')
  })

  it('merges caller classes and forwards refs', () => {
    const ref = { current: null as HTMLButtonElement | null }
    render(
      <IconButton ref={ref} aria-label="pin" className="ml-auto">
        P
      </IconButton>
    )
    expect(ref.current).toBe(screen.getByLabelText('pin'))
    expect(screen.getByLabelText('pin').className).toContain('ml-auto')
  })
})
