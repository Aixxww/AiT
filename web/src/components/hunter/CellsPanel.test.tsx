import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import {
  CellsPanel,
  cellLabel,
  formatPerDay,
  formatR,
  formatRate,
} from './CellsPanel'
import type { V7CellRow, V7CellsResponse } from '../../lib/api/hunter'

vi.mock('../../lib/api', () => ({
  api: {
    getV7Cells: vi.fn(),
  },
}))

import { api } from '../../lib/api'

const mockedGetV7Cells = vi.mocked(api.getV7Cells)

function row(overrides: Partial<V7CellRow> = {}): V7CellRow {
  return {
    signal_class: 'TRIGGER',
    n: 114,
    tp1: 62,
    stop: 52,
    none: 0,
    tp1_first_rate: 0.5439,
    mean_fee_net_r: -0.097,
    ci_lo: -0.236,
    ci_hi: 0.043,
    stamped_status: ['probation'],
    evaluated_status: 'probation',
    ...overrides,
  }
}

function response(overrides: Partial<V7CellsResponse> = {}): V7CellsResponse {
  return {
    from: '2026-11-12T00:00:00Z',
    to: '2026-11-19T00:00:00Z',
    days: 7,
    source: 'prompt_open_review',
    rule: {
      min_samples: 30,
      ci_level: 0.9,
      bootstrap_resamples: 5000,
      seed: 20261118,
      quantiles: 5,
    },
    hysteresis: 'none',
    tables: {
      rows: 144,
      classes: [
        row(),
        row({
          signal_class: 'IMPULSE',
          n: 30,
          evaluated_status: 'earned',
          mean_fee_net_r: 0.2,
          ci_lo: 0.05,
        }),
      ],
      cells: [
        row({ rr_band: '1.2_1.5' }),
        row({
          signal_class: 'REVERSAL',
          rr_band: 'lt_1.0',
          n: 31,
          evaluated_status: 'blocked',
          ci_hi: -0.1,
        }),
      ],
      cell_regimes: [row({ rr_band: '1.2_1.5', market_regime: 'unknown' })],
    },
    excluded_no_r: { degenerate_tp1: 38, degenerate_stop: 12 },
    calibration_drift: [],
    ...overrides,
  }
}

describe('format helpers', () => {
  it('formats R and rates', () => {
    expect(formatR(0.125)).toBe('+0.125')
    expect(formatR(-1)).toBe('-1.000')
    expect(formatRate(0.5439)).toBe('54.4%')
    expect(formatR(Number.NaN)).toBe('—')
  })
  it('labels rows by their level', () => {
    expect(cellLabel(row())).toBe('TRIGGER')
    expect(cellLabel(row({ rr_band: '1.2_1.5', market_regime: 'chop' }))).toBe(
      'TRIGGER · 1.2_1.5 · chop'
    )
  })
})

describe('CellsPanel', () => {
  beforeEach(() => {
    mockedGetV7Cells.mockReset()
  })

  it('renders class and cell tables with stamped and evaluated status', async () => {
    mockedGetV7Cells.mockResolvedValue(response())
    render(<CellsPanel language="en" />)
    expect(await screen.findByText('By class')).toBeInTheDocument()
    expect(mockedGetV7Cells).toHaveBeenCalledWith(7, 'prompt_open_review')
    expect(screen.getByText('TRIGGER · 1.2_1.5')).toBeInTheDocument()
    expect(screen.getByTestId('status-earned')).toHaveTextContent('Earned')
    expect(screen.getByTestId('status-blocked')).toHaveTextContent('Blocked')
    expect(screen.getByText(/no hysteresis/)).toBeInTheDocument()
    expect(screen.getByText(/N_min 30, 90%/)).toBeInTheDocument()
    expect(
      screen.getByText(
        'Excluded (no fee-net R): degenerate_stop 12, degenerate_tp1 38'
      )
    ).toBeInTheDocument()
    // Cell × regime table starts collapsed.
    expect(
      screen.queryByText('TRIGGER · 1.2_1.5 · unknown')
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('By cell × regime'))
    expect(screen.getByText('TRIGGER · 1.2_1.5 · unknown')).toBeInTheDocument()
    expect(
      screen.getByText(/shadow E\[R\] estimator is off/)
    ).toBeInTheDocument()
  })

  it('refetches with the selected source and window', async () => {
    mockedGetV7Cells.mockResolvedValue(response())
    render(<CellsPanel language="en" />)
    await screen.findByText('By class')
    fireEvent.change(screen.getByLabelText('Outcomes'), {
      target: { value: 'shadow' },
    })
    await waitFor(() =>
      expect(mockedGetV7Cells).toHaveBeenLastCalledWith(7, 'shadow')
    )
    fireEvent.change(screen.getByLabelText('Window'), {
      target: { value: '30' },
    })
    await waitFor(() =>
      expect(mockedGetV7Cells).toHaveBeenLastCalledWith(30, 'shadow')
    )
  })

  it('shows the drift flag and quantile tables', async () => {
    const window = (outside: number) => ({
      from: '',
      to: '',
      n: 10,
      sufficient: true,
      quantiles_outside: outside,
      quantiles: [
        {
          quantile: 1,
          n: 10,
          pred_mean: 0.4,
          realized_mean: -0.5,
          ci_lo: -0.8,
          ci_hi: -0.2,
          inside: false,
        },
      ],
    })
    mockedGetV7Cells.mockResolvedValue(
      response({
        calibration_drift: [
          {
            calibration_version: 'cal-a',
            current: window(1),
            previous: window(1),
            drift: true,
          },
          {
            calibration_version: 'cal-b',
            current: window(0),
            previous: { ...window(0), n: 2, sufficient: false, quantiles: [] },
            drift: false,
          },
        ],
      })
    )
    render(<CellsPanel language="en" />)
    expect(await screen.findByTestId('drift-flag')).toHaveTextContent('Drift')
    expect(screen.getByTestId('drift-ok')).toHaveTextContent('No drift')
    expect(
      screen.getByText('Too few rows for the quantile table')
    ).toBeInTheDocument()
    expect(screen.getAllByText('outside').length).toBeGreaterThan(0)
  })

  it('renders the empty and failed states', async () => {
    mockedGetV7Cells.mockResolvedValueOnce(
      response({
        tables: { rows: 0, classes: [], cells: [], cell_regimes: [] },
        excluded_no_r: {},
      })
    )
    const { unmount } = render(<CellsPanel language="zh" />)
    expect(
      await screen.findByText('本窗口内没有扣费 R 结果')
    ).toBeInTheDocument()
    unmount()
    mockedGetV7Cells.mockRejectedValueOnce(new Error('down'))
    render(<CellsPanel language="id" />)
    expect(
      await screen.findByText('Gagal memuat tabel sel')
    ).toBeInTheDocument()
  })

  it('shows the per-regime yield table with the below-target flag', async () => {
    mockedGetV7Cells.mockResolvedValue(
      response({
        yield: {
          available: true,
          stage: 't2_triggered',
          forming_bar_tf: '5m',
          round_minutes: 5,
          ci_level: 0.9,
          t2_records: 40,
          events: 6,
          rounds_per_tier: { LOW: 600, MID: 800, HIGH: 616 },
          rows: [
            {
              stage: 't2_triggered',
              rv24_tier: 'LOW',
              rounds: 600,
              events: 30,
              rate_per_round: 0.05,
              rate_ci_lo: 0.036,
              rate_ci_hi: 0.067,
              target_rate: 0.0408,
              expected_events: 24.5,
              below_target: false,
            },
            {
              stage: 't2_triggered',
              rv24_tier: 'HIGH',
              rounds: 616,
              events: 1,
              rate_per_round: 0.0016,
              rate_ci_lo: 0.0001,
              rate_ci_hi: 0.0077,
              target_rate: 0.0464,
              expected_events: 28.6,
              below_target: true,
            },
          ],
        },
      })
    )
    render(<CellsPanel language="en" />)
    expect(
      await screen.findByText('Yield by BTC RV24 tier (T2 triggered)')
    ).toBeInTheDocument()
    expect(screen.getByTestId('yield-below-HIGH')).toHaveTextContent(
      'below target'
    )
    expect(screen.queryByTestId('yield-below-LOW')).not.toBeInTheDocument()
    expect(screen.getByText('14.40')).toBeInTheDocument() // 0.05 per 5-min round = 14.4/day
  })

  it('explains an unavailable yield table', async () => {
    mockedGetV7Cells.mockResolvedValue(
      response({
        yield: {
          available: false,
          reason: 'no T2 records in the window (T2 module off?)',
          stage: 't2_triggered',
          forming_bar_tf: '5m',
          round_minutes: 5,
          ci_level: 0.9,
          t2_records: 0,
          events: 0,
          rounds_per_tier: {},
          rows: [],
        },
      })
    )
    render(<CellsPanel language="en" />)
    expect(await screen.findByTestId('yield-unavailable')).toHaveTextContent(
      'T2 module off'
    )
  })

  it('converts rates per round to per day', () => {
    expect(formatPerDay(0.05, 5)).toBe('14.40')
    expect(formatPerDay(0.05, 0)).toBe('—')
  })
})
