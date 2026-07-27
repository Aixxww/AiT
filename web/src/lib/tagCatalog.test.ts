import { describe, it, expect, vi } from 'vitest'
import { describeTag, tagTooltip, type TagCatalog } from './tagCatalog'
import type { V7TagDefinition } from './api/hunter'

vi.mock('./api', () => ({
  api: {
    getV7TagCatalog: vi.fn(),
  },
}))

function makeCatalog(): TagCatalog {
  const def: V7TagDefinition = {
    tag: 'fresh_oi_absent',
    source: 'risk_tag',
    category: 'oi',
    polarity: 'neutral',
    llm_action: 'wait_only',
    definition: 'No fresh OI inflow supports the move.',
  }
  return new Map([[def.tag, def]])
}

describe('describeTag', () => {
  it('resolves an exact catalog entry', () => {
    const def = describeTag(makeCatalog(), 'fresh_oi_absent')
    expect(def?.llm_action).toBe('wait_only')
  })

  it('strips the aggregated-chip count suffix before lookup', () => {
    const def = describeTag(makeCatalog(), 'fresh_oi_absent ×3')
    expect(def?.definition).toContain('No fresh OI inflow')
  })

  it('synthesizes live_confirmed_* prefix-family definitions', () => {
    const def = describeTag(makeCatalog(), 'live_confirmed_taker_flow')
    expect(def?.category).toBe('confirmation')
    expect(def?.definition).toContain("'taker_flow'")
    expect(def?.llm_action).toBe('supports_open_after_core_checks')
  })

  it('synthesizes sector_theme_* prefix-family definitions', () => {
    const def = describeTag(makeCatalog(), 'sector_theme_ai')
    expect(def?.definition).toContain("'ai'")
    expect(def?.llm_action).toBe('evidence_only')
  })

  it('returns undefined for unknown codes and a null catalog', () => {
    expect(describeTag(makeCatalog(), 'not_in_catalog')).toBeUndefined()
    expect(describeTag(null, 'fresh_oi_absent')).toBeUndefined()
  })
})

describe('tagTooltip', () => {
  it('formats definition plus source/category/action metadata', () => {
    const tooltip = tagTooltip(makeCatalog(), 'fresh_oi_absent')
    expect(tooltip).toBe(
      'No fresh OI inflow supports the move.\n[risk_tag · oi · wait_only]'
    )
  })

  it('returns undefined when the catalog has no entry', () => {
    expect(tagTooltip(makeCatalog(), 'unknown_tag')).toBeUndefined()
  })
})
