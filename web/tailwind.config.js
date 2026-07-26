/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: ['selector', '[data-theme="dark"]'],
  theme: {
    extend: {
      colors: {
        // ── 语义化 Token (引用 CSS 变量) ──
        background: 'var(--color-background)',
        foreground: 'var(--color-foreground)',
        surface: 'var(--color-surface)',
        'surface-alt': 'var(--color-surface-alt)',
        panel: 'var(--color-panel)',
        'panel-hover': 'var(--color-panel-hover)',

        primary: {
          DEFAULT: 'var(--color-primary)',
          foreground: 'var(--color-primary-fg)',
          dim: 'var(--color-primary-dim)',
          glow: 'var(--color-primary-glow)',
        },
        accent: { DEFAULT: 'var(--color-accent)' },
        muted: {
          DEFAULT: 'var(--color-muted)',
          foreground: 'var(--color-muted-fg)',
        },
        border: {
          DEFAULT: 'var(--color-border)',
          hover: 'var(--color-border-hover)',
        },
        input: { DEFAULT: 'var(--color-input)', ring: 'var(--color-ring)' },

        // 交易色
        profit: {
          DEFAULT: 'var(--color-profit)',
          bg: 'var(--color-profit-bg)',
          border: 'var(--color-profit-border)',
        },
        loss: {
          DEFAULT: 'var(--color-loss)',
          bg: 'var(--color-loss-bg)',
          border: 'var(--color-loss-border)',
        },

        // 状态色
        warning: {
          DEFAULT: 'var(--color-warning)',
          bg: 'var(--color-warning-bg)',
        },
        info: { DEFAULT: 'var(--color-info)', bg: 'var(--color-info-bg)' },

        // 头部
        header: {
          DEFAULT: 'var(--color-header)',
          border: 'var(--color-header-border)',
        },

        // ── 向后兼容别名 ──
        'ait-gold': {
          DEFAULT: 'var(--color-primary)',
          dim: 'var(--color-primary-dim)',
          glow: 'var(--color-primary-glow)',
          highlight: '#FFD700',
        },
        'ait-bg': {
          DEFAULT: 'var(--color-background)',
          deeper: '#020304',
          lighter: 'var(--color-surface)',
        },
        'ait-accent': 'var(--color-accent)',
        'ait-text': {
          DEFAULT: 'var(--color-foreground)',
          main: 'var(--color-foreground)',
          muted: 'var(--color-muted-fg)',
        },
        'ait-success': 'var(--color-profit)',
        'ait-danger': 'var(--color-loss)',
      },
      fontFamily: {
        sans: [
          'Inter',
          'ui-sans-serif',
          'system-ui',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'Noto Sans SC',
          'sans-serif',
        ],
        mono: [
          'IBM Plex Mono',
          'JetBrains Mono',
          'ui-monospace',
          'Menlo',
          'Monaco',
          'Courier New',
          'monospace',
        ],
      },
      backgroundImage: {
        'gradient-radial':
          'radial-gradient(circle at center, var(--tw-gradient-stops))',
        'gradient-conic':
          'conic-gradient(from 180deg at 50% 50%, var(--tw-gradient-stops))',
        scanlines:
          "url(\"data:image/svg+xml,%3Csvg width='4' height='4' viewBox='0 0 4 4' fill='none' xmlns='http://www.w3.org/2000/svg'%3E%3Cpath d='M0 0H4V2H0V0Z' fill='rgba(0,0,0,0.4)'/%3E%3C/svg%3E\")",
        'grid-pattern':
          'linear-gradient(to right, #1f2937 1px, transparent 1px), linear-gradient(to bottom, #1f2937 1px, transparent 1px)',
      },
      animation: {
        // landing-only decorative utilities; the operations console must not use these
        'pulse-slow': 'pulse 4s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
      },
      keyframes: {
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' },
        },
      },
      boxShadow: {
        glass: 'var(--shadow-glass)',
        'glass-heavy': 'var(--shadow-glass-heavy)',
      },
      borderRadius: {
        'glass-sm': 'var(--radius-sm)',
        'glass-md': 'var(--radius-md)',
        'glass-lg': 'var(--radius-lg)',
        'glass-xl': 'var(--radius-xl)',
      },
    },
  },
  plugins: [],
}
