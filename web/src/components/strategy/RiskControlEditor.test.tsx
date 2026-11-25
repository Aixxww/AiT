// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  render,
  screen,
  fireEvent,
  waitFor,
  cleanup,
} from '@testing-library/react'
import { RiskControlEditor } from './RiskControlEditor'
import { EffectiveConfigPanel } from './EffectiveConfigPanel'
import type { RiskControlConfig, StrategyConfig } from '../../types'

const base: RiskControlConfig = {
  max_positions: 1,
  btc_eth_max_leverage: 5,
  altcoin_max_leverage: 5,
  max_margin_usage: 0.9,
  min_position_size: 12,
  min_risk_reward_ratio: 1.2,
  min_confidence: 75,
}

function renderEditor(config: RiskControlConfig = base) {
  const onChange = vi.fn()
  render(
    <RiskControlEditor config={config} onChange={onChange} language="en" />
  )
  return onChange
}

const input = (name: string) =>
  document.querySelector(`input[name="${name}"]`) as HTMLInputElement

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('RiskControlEditor (#95)', () => {
  it('RR floor steps by 0.1 and never falls back to 3', () => {
    const onChange = renderEditor({ ...base, min_risk_reward_ratio: 0 })
    const rr = input('risk-min-risk-reward-ratio')
    expect(rr.step).toBe('0.1')
    expect(rr.value).toBe('')
    expect(rr.placeholder).toBe('1.2')
    fireEvent.change(rr, { target: { value: '1.3' } })
    expect(onChange).toHaveBeenLastCalledWith({
      ...base,
      min_risk_reward_ratio: 1.3,
    })
    cleanup()
    // Clearing a set value stores "unset" (0); the backend resolves it to
    // 1.2, never to 3.
    const onChange2 = renderEditor({ ...base, min_risk_reward_ratio: 1.3 })
    fireEvent.change(input('risk-min-risk-reward-ratio'), {
      target: { value: '' },
    })
    expect(onChange2).toHaveBeenLastCalledWith({
      ...base,
      min_risk_reward_ratio: 0,
    })
  })

  it('shows the max-positions safety ceiling', () => {
    renderEditor()
    expect(input('risk-max-positions').max).toBe('3')
    expect(screen.getByText(/Safety ceiling 3/)).toBeInTheDocument()
  })

  it('edits backend-only knobs; empty means unset', () => {
    const onChange = renderEditor()
    fireEvent.change(input('risk-max-single-trade-loss-pct'), {
      target: { value: '6' },
    })
    expect(onChange).toHaveBeenLastCalledWith({
      ...base,
      max_single_trade_loss_pct: 6,
    })
    fireEvent.change(input('risk-failed-open-cycles'), {
      target: { value: '0' },
    })
    expect(onChange).toHaveBeenLastCalledWith({
      ...base,
      cooldowns: { failed_open_cycles: 0 },
    })
    fireEvent.change(input('risk-protector-tp0'), { target: { value: '10' } })
    expect(onChange).toHaveBeenLastCalledWith({
      ...base,
      protector: { tp0: { value: 10, unit: 'roe_pct' } },
    })
    fireEvent.change(input('risk-breaker-max-loss-pct'), {
      target: { value: '20' },
    })
    expect(onChange).toHaveBeenLastCalledWith({
      ...base,
      daily_loss_breaker: { max_daily_loss_pct: 20 },
    })
    cleanup()
    const onChange3 = renderEditor({
      ...base,
      daily_loss_breaker: { max_daily_loss_pct: 20 },
    })
    fireEvent.change(input('risk-breaker-max-loss-pct'), {
      target: { value: '' },
    })
    expect(onChange3).toHaveBeenLastCalledWith({
      ...base,
      daily_loss_breaker: { max_daily_loss_pct: undefined },
    })
    expect(input('risk-breaker-max-loss-pct').placeholder).toBe('15')
  })

  it('setup guard edits keep the built-in values of the other fields', () => {
    const onChange = renderEditor()
    fireEvent.change(input('guard-range_reversion-max-zone-long'), {
      target: { value: '40' },
    })
    expect(onChange).toHaveBeenLastCalledWith({
      ...base,
      setup_guard: {
        range_reversion: {
          min_zone_pos_short: 55,
          max_zone_pos_long: 40,
          require_oi_flush: false,
        },
      },
    })
  })
})

describe('EffectiveConfigPanel (#95)', () => {
  it('shows each resolved value with its source and attached trader intervals', async () => {
    const fetchMock = vi.fn(async (url: string) => {
      if (url.endsWith('/api/strategies/effective-config')) {
        return new Response(
          JSON.stringify({
            values: [
              {
                key: 'min_risk_reward_ratio',
                group: 'decision',
                value: 1.2,
                source: 'default',
              },
              {
                key: 'hunter_limit',
                group: 'router',
                value: 10,
                source: 'safety_limit',
                note: 'configured 15 clamped to 10',
              },
              {
                key: 'max_margin_usage',
                group: 'sizing',
                value: 0.9,
                source: 'config',
                enforced: false,
              },
            ],
          }),
          { status: 200 }
        )
      }
      return new Response(
        JSON.stringify([
          {
            trader_id: 't1',
            trader_name: 'Alpha',
            ai_model: 'm',
            strategy_id: 's1',
            scan_interval_minutes: 3,
          },
          {
            trader_id: 't2',
            trader_name: 'Beta',
            ai_model: 'm',
            strategy_id: 's1',
            scan_interval_minutes: 15,
          },
          {
            trader_id: 't3',
            trader_name: 'Other',
            ai_model: 'm',
            strategy_id: 's2',
            scan_interval_minutes: 5,
          },
        ]),
        { status: 200 }
      )
    })
    vi.stubGlobal('fetch', fetchMock)
    render(
      <EffectiveConfigPanel
        config={{} as StrategyConfig}
        strategyId="s1"
        token="tok"
        language="en"
      />
    )
    await waitFor(() =>
      expect(screen.getByText('hunter_limit')).toBeInTheDocument()
    )
    expect(screen.getByText('safety limit')).toBeInTheDocument()
    expect(screen.getByText('default')).toBeInTheDocument()
    expect(screen.getByText('not enforced')).toBeInTheDocument()
    expect(screen.getByText('Alpha')).toBeInTheDocument()
    expect(screen.getByText('every 15 min')).toBeInTheDocument()
    expect(screen.queryByText('Other')).not.toBeInTheDocument()
    vi.unstubAllGlobals()
  })
})
