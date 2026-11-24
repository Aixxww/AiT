import { describe, expect, it } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'
import {
  DISPLAY_MAPS,
  displayName,
  humanizeCode,
  setupLabel,
  tagLabel,
} from './displayNames'

const repo = path.resolve(__dirname, '../../..')
const read = (rel: string) => fs.readFileSync(path.join(repo, rel), 'utf8')
const SNAKE = /[a-z0-9]+_[a-z0-9_]+/

function goSetupTypes(): string[] {
  const src = read('provider/local/hunter_v7_types.go')
  const v7 = [...src.matchAll(/V7SetupType = "([a-z0-9_]+)"/g)].map((m) => m[1])
  const t2src = read('provider/local/hunter_v7_t2_setups.go')
  const block = t2src.slice(t2src.indexOf('var V7T2SetupTypes'))
  const t2 = [
    ...block.slice(0, block.indexOf('}')).matchAll(/"([a-z0-9_]+)"/g),
  ].map((m) => m[1])
  return [...new Set([...v7, ...t2])]
}

function goRegimes(): string[] {
  const src = read('provider/local/hunter_v7_types.go')
  return [...src.matchAll(/V7MarketRegime = "([a-z_]+)"/g)].map((m) => m[1])
}

function goBlockedReasons(): string[] {
  const src = read('kernel/engine_analysis.go')
  const block = src.slice(src.indexOf('var hunterV7BlockedReasonCodes'))
  return [...block.slice(0, block.indexOf('}')).matchAll(/"([a-z_]+)"/g)].map(
    (m) => m[1]
  )
}

function goTags(): string[] {
  const files = [
    'provider/local/hunter_v7_tag_catalog.go',
    'provider/local/hunter_v7_tag_catalog_ext.go',
  ]
  return [
    ...new Set(
      files.flatMap((f) =>
        [...read(f).matchAll(/tagDef\("([^"]+)"/g)].map((m) => m[1])
      )
    ),
  ]
}

describe('displayNames', () => {
  it('names every backend setup type in both languages, without codes', () => {
    const setups = goSetupTypes()
    expect(setups.length).toBeGreaterThan(30)
    for (const s of setups) {
      for (const lang of ['zh', 'en'] as const) {
        const label = setupLabel(s, lang)
        expect(label, `${s} ${lang}`).toBeTruthy()
        expect(label, `${s} ${lang}`).not.toMatch(SNAKE)
      }
      const base = s.replace(/_(long|short)$/, '')
      expect(
        DISPLAY_MAPS.SETUP_BASE[base] ?? DISPLAY_MAPS.SETUP_BASE[s],
        s
      ).toBeDefined()
    }
  })

  it('names every market regime and blocked reason explicitly', () => {
    for (const r of goRegimes()) expect(DISPLAY_MAPS.REGIME[r], r).toBeDefined()
    for (const b of goBlockedReasons())
      expect(DISPLAY_MAPS.BLOCKED_REASON[b], b).toBeDefined()
  })

  it('renders every catalog tag without snake_case, and the zh text without English words', () => {
    const tags = goTags()
    expect(tags.length).toBeGreaterThan(400)
    const untranslated = new Set<string>()
    for (const tag of tags) {
      const en = tagLabel(tag, 'en')
      const zh = tagLabel(tag, 'zh')
      expect(en, tag).not.toMatch(SNAKE)
      expect(zh, tag).not.toMatch(SNAKE)
      // Allowed Latin in zh: acronyms, timeframes, numbers, TP/SL markers.
      const leftovers = zh
        .replace(
          /\b(OI|VWAP|EMA\d*|RSI(\(5m\))?|RR|LSR|BB|ATR|MMS|BOLL|TP\d|T2|RS|MFE|OBV|VPVR)\b/g,
          ''
        )
        .match(/[a-z]{2,}/gi)
      leftovers?.forEach((w) => untranslated.add(w))
    }
    expect([...untranslated]).toEqual([])
  })

  it('formats setups, tiers, actions and directions', () => {
    expect(setupLabel('momentum_continuation_long', 'zh')).toBe(
      '动量延续 · 做多'
    )
    expect(setupLabel('momentum_continuation_long', 'en')).toBe(
      'Momentum Continuation · Long'
    )
    expect(displayName('tier', 'REVIEWABLE', 'zh')).toBe('待复核')
    expect(displayName('action', 'open_short', 'en')).toBe('Open Short')
    expect(displayName('regime', 'trend_continuation', 'zh')).toBe('趋势延续')
    expect(displayName('blockedReason', 'no_reviewable_candidate', 'en')).toBe(
      'No reviewable candidate'
    )
    expect(tagLabel('liquidity_lt_50', 'zh')).toBe('流动性 < 50')
    expect(tagLabel('confirmation_missing_15m_close_below_vwap', 'en')).toBe(
      'Awaiting: 15m close below VWAP'
    )
    expect(tagLabel('late_entry_risk ×3', 'zh')).toBe('入场偏晚风险 ×3')
    expect(humanizeCode('some_new_code', 'en')).toBe('Some new code')
  })
})
