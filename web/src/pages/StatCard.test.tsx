import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { StatCard } from './TraderDashboardPage'

describe('StatCard stale state', () => {
  it('renders the value normally when fresh', () => {
    render(<StatCard title="Total Equity" value="1234.56" unit="USDT" />)
    expect(screen.getByText('1234.56')).toBeInTheDocument()
    expect(screen.queryByText('STALE')).not.toBeInTheDocument()
    expect(screen.getByText('1234.56')).toHaveClass('text-foreground')
  })

  it('grays the value and shows the stale tag when stale', () => {
    render(
      <StatCard
        title="Total Equity"
        value="1234.56"
        unit="USDT"
        change={1.5}
        positive
        stale
        staleLabel="STALE"
      />
    )
    expect(screen.getByText('STALE')).toBeInTheDocument()
    expect(screen.getByText('1234.56')).toHaveClass('text-muted-foreground')
    // The semantic PnL color is also muted while stale
    expect(screen.getByText('+1.50%').parentElement).toHaveClass(
      'text-muted-foreground'
    )
  })
})
