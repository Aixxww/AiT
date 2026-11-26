// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'
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

// #110 owner decision: selecting the hunter_v7 source writes an unset
// v7_aggressive as OFF (new strategies default OFF); an explicit value is kept.
describe('CoinSourceEditor selecting hunter_v7', () => {
  const select = (config: CoinSourceConfig) => {
    const onChange = vi.fn()
    render(
      <CoinSourceEditor config={config} onChange={onChange} language="en" />
    )
    fireEvent.click(screen.getByRole('button', { name: /^Hunter v7/ }))
    expect(onChange).toHaveBeenCalledTimes(1)
    return onChange.mock.calls[0][0] as CoinSourceConfig
  }
  it('writes v7_aggressive false when unset', () => {
    const next = select({ source_type: 'static' } as CoinSourceConfig)
    expect(next.source_type).toBe('hunter_v7')
    expect(next.hunter_config?.v7_aggressive).toBe(false)
  })
  it('keeps an explicit v7_aggressive', () => {
    const next = select({
      source_type: 'static',
      hunter_config: { v7_aggressive: true },
    } as CoinSourceConfig)
    expect(next.hunter_config?.v7_aggressive).toBe(true)
  })
})
