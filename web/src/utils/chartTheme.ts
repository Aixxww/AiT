/**
 * Chart theme adapter — reads CSS variables and returns config objects
 * for lightweight-charts and recharts.
 */

// lightweight-charts paints on <canvas>, which cannot resolve CSS var()
// references — an unresolved color is silently ignored and the series is
// painted in the default black. Resolve theme variables to concrete
// color values before handing them to the chart.
export function cssVar(name: string, fallback: string): string {
  if (typeof window === 'undefined') return fallback
  const v = window
    .getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim()
  return v || fallback
}

// Dark-theme values from index.css, the cssVar() fallbacks for canvas-bound
// chart colors (#153). A palette data file, whitelisted by the design-token
// lint guard.
export const CHART_COLOR_FALLBACK = {
  chartBg: '#0b0e11',
  chartText: '#8b95a5',
  chartGrid: 'rgba(43, 49, 57, 0.2)',
  primary: '#f0b90b',
  border: 'rgba(255, 255, 255, 0.075)',
  profit: '#0ecb81',
  loss: '#f6465d',
  profitBorder: 'rgba(14, 203, 129, 0.25)',
  lossBorder: 'rgba(246, 70, 93, 0.25)',
  background: '#0b0e11',
  foreground: '#eaecef',
  mutedFg: '#929aa5',
} as const

function getCSSVar(name: string): string {
  return getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim()
}

export function getChartTheme() {
  const scheme = document.documentElement.getAttribute('data-theme') as
    | 'dark'
    | 'light'
  const isDark = scheme === 'dark'

  return {
    layout: {
      background: { color: getCSSVar('--chart-bg') },
      textColor: getCSSVar('--chart-text'),
      fontFamily: "'IBM Plex Mono', monospace",
      fontSize: 11,
    },
    grid: {
      vertLines: { color: getCSSVar('--chart-grid') },
      horzLines: { color: getCSSVar('--chart-grid') },
    },
    crosshair: {
      vertLine: {
        color: getCSSVar('--chart-crosshair'),
        labelBackgroundColor: getCSSVar('--color-primary'),
      },
      horzLine: {
        color: getCSSVar('--chart-crosshair'),
        labelBackgroundColor: getCSSVar('--color-primary'),
      },
    },
    timeScale: {
      borderColor: getCSSVar('--chart-border'),
    },
    rightPriceScale: {
      borderColor: getCSSVar('--chart-border'),
    },
    candlestick: {
      upColor: getCSSVar('--color-profit'),
      downColor: getCSSVar('--color-loss'),
      borderUpColor: getCSSVar('--color-profit'),
      borderDownColor: getCSSVar('--color-loss'),
      wickUpColor: getCSSVar('--color-profit'),
      wickDownColor: getCSSVar('--color-loss'),
    },
    indicators: {
      volume: isDark ? '#3B82F6' : '#2563EB',
      ma5: '#FF6B6B',
      ma10: '#4ECDC4',
      ma20: isDark ? '#FFD93D' : '#D97706',
      ma60: '#95E1D3',
      ema12: '#A8E6CF',
      ema26: '#FFD3B6',
      bb: '#9B59B6',
    },
    markers: {
      longOpen: getCSSVar('--color-profit'),
      longClose: isDark ? '#6EE7B7' : '#34D399',
      shortOpen: getCSSVar('--color-loss'),
      shortClose: isDark ? '#FCA5A5' : '#F87171',
    },
  }
}

export function getRechartTheme() {
  return {
    grid: {
      stroke: getCSSVar('--chart-grid'),
      strokeDasharray: '3 3',
    },
    axis: {
      stroke: getCSSVar('--chart-text'),
      tickFill: getCSSVar('--chart-text'),
      tickFontSize: 11,
      tickLine: getCSSVar('--chart-grid'),
    },
    line: {
      stroke: getCSSVar('--color-primary'),
      strokeWidth: 2,
    },
    gradient: {
      startColor: getCSSVar('--color-primary'),
      startOpacity: 0.8,
      endColor: getCSSVar('--color-primary'),
      endOpacity: 0.1,
    },
    reference: {
      stroke: getCSSVar('--color-muted-fg'),
      strokeDasharray: '3 3',
      labelFill: getCSSVar('--color-muted-fg'),
    },
    tooltip: {
      background: getCSSVar('--color-panel'),
      border: getCSSVar('--color-border'),
      text: getCSSVar('--color-foreground'),
      subtext: getCSSVar('--color-muted-fg'),
    },
  }
}
