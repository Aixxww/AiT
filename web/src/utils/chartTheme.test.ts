import { afterEach, describe, expect, it } from 'vitest'
import { cssVar, CHART_COLOR_FALLBACK } from './chartTheme'

// #153: canvas (lightweight-charts) cannot resolve var(); colors must be
// resolved to concrete values before reaching chart options.
describe('cssVar', () => {
  afterEach(() => {
    document.documentElement.style.removeProperty('--chart-test-color')
  })

  it('resolves a defined CSS custom property to its concrete value', () => {
    document.documentElement.style.setProperty(
      '--chart-test-color',
      'rgb(1, 2, 3)'
    )
    expect(cssVar('--chart-test-color', 'rgb(0, 0, 0)')).toBe('rgb(1, 2, 3)')
  })

  it('falls back when the property is undefined', () => {
    expect(cssVar('--chart-undefined-color', CHART_COLOR_FALLBACK.profit)).toBe(
      CHART_COLOR_FALLBACK.profit
    )
  })

  it('has no var() or color-mix() in any fallback', () => {
    for (const v of Object.values(CHART_COLOR_FALLBACK)) {
      expect(v).not.toMatch(/var\(|color-mix\(/)
    }
  })
})
