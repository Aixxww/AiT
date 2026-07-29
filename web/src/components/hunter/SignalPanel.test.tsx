import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import {
  SignalPanel,
  groupLatestCycleByTier,
  takerLadderCodes,
  zonePositionPct,
} from './SignalPanel'
import type { V7Signal, V7SignalRow } from '../../lib/api/hunter'

vi.mock('../../lib/api', () => ({
  api: {
    getV7Signals: vi.fn(),
    getV7TagCatalog: vi.fn(),
  },
}))

import { api } from '../../lib/api'
import { resetTagCatalogCache } from '../../lib/tagCatalog'

const mockedGetV7Signals = vi.mocked(api.getV7Signals)
const mockedGetV7TagCatalog = vi.mocked(api.getV7TagCatalog)

function makeSignal(overrides: Partial<V7Signal> = {}): V7Signal {
  return {
    symbol: 'BTCUSDT',
    direction: 'LONG',
    setup_type: 'volatility_squeeze',
    setup_score: 75,
    risk_score: 30,
    liquidity_score: 80,
    timing_score: 70,
    regime_fit_score: 65,
    ai_priority: 88,
    entry_zone: { lower: 100, upper: 110 },
    invalidation: { price: 95 },
    ...overrides,
  }
}

function makeRow(overrides: Partial<V7SignalRow> = {}): V7SignalRow {
  return {
    id: 1,
    cycle_number: 7,
    timestamp: '2026-07-27T00:00:00Z',
    execution_tier: 'EXECUTABLE',
    tier_reason: 'all clear',
    track_pnl_pct: 0,
    signal: makeSignal(),
    ...overrides,
  }
}

describe('groupLatestCycleByTier', () => {
  it('groups the newest timestamp rows by tier and drops older runs', () => {
    const rows: V7SignalRow[] = [
      makeRow({
        id: 10,
        cycle_number: 1,
        timestamp: '2026-07-28T02:20:17Z',
        execution_tier: 'EXECUTABLE',
      }),
      makeRow({
        id: 9,
        cycle_number: 1,
        timestamp: '2026-07-28T02:20:17Z',
        execution_tier: 'WATCH',
      }),
      makeRow({
        id: 8,
        cycle_number: 1,
        timestamp: '2026-07-28T02:20:17Z',
        execution_tier: 'REJECTED',
      }),
      makeRow({
        id: 7,
        cycle_number: 1,
        timestamp: '2026-07-27T23:18:13Z',
        execution_tier: 'EXECUTABLE',
      }),
    ]
    const grouped = groupLatestCycleByTier(rows)
    expect(grouped.EXECUTABLE).toHaveLength(1)
    expect(grouped.EXECUTABLE[0].id).toBe(10)
    expect(grouped.WATCH).toHaveLength(1)
    expect(grouped.REJECTED).toHaveLength(1)
    expect(grouped.REVIEWABLE).toHaveLength(0)
  })

  it('treats rows without a tier as REJECTED', () => {
    const grouped = groupLatestCycleByTier([
      makeRow({ execution_tier: '' as V7SignalRow['execution_tier'] }),
    ])
    expect(grouped.REJECTED).toHaveLength(1)
  })

  it('returns empty groups for no rows', () => {
    const grouped = groupLatestCycleByTier([])
    expect(grouped.EXECUTABLE).toHaveLength(0)
    expect(grouped.REJECTED).toHaveLength(0)
  })
})

describe('takerLadderCodes', () => {
  it('keeps only the unified taker ladder codes', () => {
    const signal = makeSignal({
      reason_codes: [
        'flow_taker_buy_strong',
        'fresh_oi_present',
        'flow_taker_sell_dominant',
      ],
    })
    expect(takerLadderCodes(signal)).toEqual([
      'flow_taker_buy_strong',
      'flow_taker_sell_dominant',
    ])
  })

  it('returns empty for missing reason codes', () => {
    expect(takerLadderCodes(makeSignal())).toEqual([])
  })
})

describe('zonePositionPct', () => {
  it('returns the readiness zone position clamped to 0-100', () => {
    const signal = makeSignal({
      execution_readiness: {
        tier: 'EXECUTABLE',
        entry_zone_position: 140,
      },
    })
    expect(zonePositionPct(signal)).toBe(100)
  })

  it('returns null for the -1 unknown sentinel', () => {
    const signal = makeSignal({
      execution_readiness: {
        tier: 'WATCH',
        entry_zone_position: -1,
      },
    })
    expect(zonePositionPct(signal)).toBeNull()
  })

  it('falls back to the confirmation summary position', () => {
    const signal = makeSignal({
      confirmation_summary: {
        passed_hard: true,
        passed_review: true,
        entry_zone_position: 45,
      },
    })
    expect(zonePositionPct(signal)).toBe(45)
  })

  it('returns null when no position is available', () => {
    expect(zonePositionPct(makeSignal())).toBeNull()
  })
})

describe('SignalPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetTagCatalogCache()
    mockedGetV7TagCatalog.mockResolvedValue({
      count: 1,
      source: 'hunter_v7_tag_catalog',
      tags: [
        {
          tag: 'fresh_oi_absent',
          source: 'risk_tag',
          category: 'oi',
          polarity: 'neutral',
          llm_action: 'wait_only',
          definition: 'No fresh OI inflow supports the move.',
        },
      ],
    })
  })

  it('attaches catalog definitions as veto chip tooltips', async () => {
    mockedGetV7Signals.mockResolvedValue({
      count: 1,
      window_source: 'hunter_v7_signal_records',
      signals: [
        makeRow({
          id: 1,
          execution_tier: 'REJECTED',
          tier_reason: 'fresh_oi_absent',
          signal: makeSignal({ symbol: 'XRPUSDT' }),
        }),
      ],
    })

    render(<SignalPanel language="en" />)

    const chip = await screen.findByText('fresh_oi_absent')
    await waitFor(() => {
      expect(chip.getAttribute('title')).toContain(
        'No fresh OI inflow supports the move.'
      )
    })
    expect(chip.getAttribute('title')).toContain('wait_only')
    // Lazy-loaded exactly once for the whole panel.
    expect(mockedGetV7TagCatalog).toHaveBeenCalledTimes(1)
  })

  it('renders expanded cards, watch rows and a folded rejected section', async () => {
    mockedGetV7Signals.mockResolvedValue({
      count: 5,
      window_source: 'hunter_v7_signal_records',
      signals: [
        makeRow({
          id: 5,
          execution_tier: 'EXECUTABLE',
          signal: makeSignal({
            symbol: 'BTCUSDT',
            reason_codes: ['flow_taker_buy_strong'],
            confirmation_summary: {
              passed_hard: true,
              passed_review: true,
              rr: 2.4,
            },
            execution_readiness: {
              tier: 'EXECUTABLE',
              entry_zone_position: 40,
            },
          }),
        }),
        makeRow({
          id: 4,
          execution_tier: 'REVIEWABLE',
          tier_reason: 'needs review',
          signal: makeSignal({ symbol: 'SOLUSDT', direction: 'SHORT' }),
        }),
        makeRow({
          id: 3,
          execution_tier: 'WATCH',
          tier_reason: 'timing weak',
          signal: makeSignal({ symbol: 'DOGEUSDT' }),
        }),
        makeRow({
          id: 2,
          execution_tier: 'REJECTED',
          tier_reason: 'fresh_oi_absent',
          signal: makeSignal({ symbol: 'XRPUSDT' }),
        }),
        makeRow({
          id: 1,
          execution_tier: 'REJECTED',
          tier_reason: 'fresh_oi_absent',
          signal: makeSignal({ symbol: 'ADAUSDT' }),
        }),
      ],
    })

    render(<SignalPanel language="en" />)

    // Expanded cards for the actionable tiers
    expect(await screen.findByTestId('signal-card-BTCUSDT')).toBeInTheDocument()
    expect(screen.getByTestId('signal-card-SOLUSDT')).toBeInTheDocument()
    // Direction badge is separate from the tier badge
    expect(screen.getAllByText('LONG').length).toBeGreaterThan(0)
    expect(screen.getAllByText('SHORT').length).toBeGreaterThan(0)
    // Zone position marker rendered from execution readiness
    expect(screen.getAllByTestId('zone-marker').length).toBeGreaterThan(0)
    // Taker ladder chip
    expect(screen.getByText('flow_taker_buy_strong')).toBeInTheDocument()
    // Watch row stays a dim one-liner
    expect(screen.getByTestId('watch-row-DOGEUSDT')).toBeInTheDocument()
    // Rejected rows fold into a count + aggregated veto chip
    expect(screen.getByText('2 rejected')).toBeInTheDocument()
    expect(screen.getByText('fresh_oi_absent ×2')).toBeInTheDocument()
    // Rejected symbols are not expanded by default
    expect(screen.queryByText('XRPUSDT')).not.toBeInTheDocument()
  })

  it('shows the empty state when the latest cycle has no signals', async () => {
    mockedGetV7Signals.mockResolvedValue({
      count: 0,
      window_source: 'hunter_v7_signal_records',
      signals: [],
    })

    render(<SignalPanel language="en" />)

    expect(
      await screen.findByText('No signals in the latest cycle')
    ).toBeInTheDocument()
  })

  it('shows the offline state when the backend is unreachable', async () => {
    mockedGetV7Signals.mockRejectedValue(new Error('connect refused'))

    render(<SignalPanel language="en" />)

    await waitFor(() => {
      expect(screen.getByText('Failed to load signals')).toBeInTheDocument()
    })
    expect(screen.getByText('The backend may be offline')).toBeInTheDocument()
  })
})
