/**
 * Chart theme adapter — reads CSS variables and returns config objects
 * for lightweight-charts and recharts.
 */

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
