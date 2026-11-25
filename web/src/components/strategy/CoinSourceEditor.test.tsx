// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { CoinSourceEditor } from './CoinSourceEditor'
import type { CoinSourceConfig } from '../../types'

afterEach(cleanup)

function aggressiveSwitch(config: CoinSourceConfig) {
  render(<CoinSourceEditor config={config} onChange={vi.fn()} language="en" />)
  return screen.getByRole('switch', { name: 'Aggressive Weighting' })
}

const v7 = (hunter_config?: CoinSourceConfig['hunter_config']) =>
  ({
    source_type: 'hunter_v7',
    hunter_limit: 10,
    hunter_config,
  }) as CoinSourceConfig

// #95 owner decision: an unset v7_aggressive displays OFF, matching
// store.ResolveHunterV7 (unset = false); an explicit value is shown as set.
describe('CoinSourceEditor v7_aggressive switch', () => {
  it('shows OFF when unset', () => {
    expect(aggressiveSwitch(v7({}))).toHaveAttribute('aria-checked', 'false')
    cleanup()
    expect(aggressiveSwitch(v7(undefined))).toHaveAttribute(
      'aria-checked',
      'false'
    )
  })
  it('shows the explicit value when set', () => {
    expect(aggressiveSwitch(v7({ v7_aggressive: true }))).toHaveAttribute(
      'aria-checked',
      'true'
    )
    cleanup()
    expect(aggressiveSwitch(v7({ v7_aggressive: false }))).toHaveAttribute(
      'aria-checked',
      'false'
    )
  })
})
