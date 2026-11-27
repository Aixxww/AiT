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

// #158: square_heat_url, the IndicatorHub parameters and the cooldown key
// are editable and display the persisted values (round trip).
describe('CoinSourceEditor #158 controls', () => {
  const renderWith = (config: CoinSourceConfig) => {
    const onChange = vi.fn()
    render(
      <CoinSourceEditor config={config} onChange={onChange} language="en" />
    )
    return onChange
  }

  it('edits square_heat_url in the mixed square-heat card', () => {
    const onChange = renderWith({
      source_type: 'mixed',
      use_square_heat: true,
    } as CoinSourceConfig)
    const input = screen.getByRole('textbox', { name: /Square heat API URL/ })
    fireEvent.change(input, { target: { value: 'https://sq.example.com' } })
    expect(onChange.mock.calls[0][0].square_heat_url).toBe(
      'https://sq.example.com'
    )
    expect(onChange.mock.calls[0][0].use_square_heat).toBe(true)
    cleanup()
    const onClear = renderWith({
      source_type: 'mixed',
      use_square_heat: true,
      square_heat_url: 'https://sq.example.com',
    } as CoinSourceConfig)
    fireEvent.change(
      screen.getByRole('textbox', { name: /Square heat API URL/ }),
      { target: { value: '' } }
    )
    expect(onClear.mock.calls[0][0].square_heat_url).toBeUndefined()
  })

  it('shows a persisted square_heat_url for the square_heat source', () => {
    renderWith({
      source_type: 'square_heat',
      square_heat_url: 'https://saved.example.com',
    } as CoinSourceConfig)
    expect(
      screen.getByRole('textbox', { name: /Square heat API URL/ })
    ).toHaveValue('https://saved.example.com')
  })

  it('edits and reloads the IndicatorHub parameters', () => {
    const onChange = renderWith({
      source_type: 'indicator_hub',
    } as CoinSourceConfig)
    expect(screen.getByTestId('indicator-hub-section')).toBeInTheDocument()
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Tech weight' }), {
      target: { value: '55' },
    })
    expect(onChange.mock.calls[0][0].indicator_hub).toEqual({ tech_weight: 55 })
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Stop-loss ATR' }),
      { target: { value: '2.5' } }
    )
    expect(onChange.mock.calls[1][0].indicator_hub.stop_loss_atr).toBe(2.5)
    cleanup()

    const saved = {
      source_type: 'indicator_hub',
      indicator_hub: {
        tech_weight: 55,
        min_score: 61,
        top_n_for_scoring: 80,
        lunarcrush_api_key: 'secret-key',
      },
    } as CoinSourceConfig
    const onChange2 = renderWith(saved)
    expect(screen.getByRole('spinbutton', { name: 'Tech weight' })).toHaveValue(
      55
    )
    expect(screen.getByRole('spinbutton', { name: 'Min score' })).toHaveValue(
      61
    )
    const key = screen.getByLabelText('LunarCrush API key')
    expect(key).toHaveAttribute('type', 'password')
    expect(key).toHaveValue('secret-key')
    // Clearing a number unsets it (the backend default applies).
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Min score' }), {
      target: { value: '' },
    })
    expect(onChange2.mock.calls[0][0].indicator_hub.min_score).toBeUndefined()
    expect(onChange2.mock.calls[0][0].indicator_hub.tech_weight).toBe(55)
  })

  it('maps the Smart Cooldown toggle to disable_cooldown', () => {
    const onChange = renderWith({
      source_type: 'hunter',
      use_hunter: true,
      hunter_config: { enable_cooldown: false },
    } as CoinSourceConfig)
    const box = screen.getByRole('checkbox', { name: 'Smart Cooldown' })
    // The legacy enable_cooldown=false every config carries means ON.
    expect(box).toBeChecked()
    fireEvent.click(box)
    expect(onChange.mock.calls[0][0].hunter_config.disable_cooldown).toBe(true)
    cleanup()
    const onChange2 = renderWith({
      source_type: 'hunter',
      use_hunter: true,
      hunter_config: { disable_cooldown: true },
    } as CoinSourceConfig)
    const off = screen.getByRole('checkbox', { name: 'Smart Cooldown' })
    expect(off).not.toBeChecked()
    fireEvent.click(off)
    expect(
      onChange2.mock.calls[0][0].hunter_config.disable_cooldown
    ).toBeUndefined()
  })
})
