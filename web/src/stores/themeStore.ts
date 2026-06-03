import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type ThemeStyle = 'pro' | 'glass'
export type ColorScheme = 'dark' | 'light'
export type ThemeMode =
  | 'pro-dark'
  | 'pro-light'
  | 'glass-dark'
  | 'glass-light'
  | 'system'

export interface ResolvedTheme {
  scheme: ColorScheme
  style: ThemeStyle
}

interface ThemeState {
  mode: ThemeMode
  resolved: ResolvedTheme
  setMode: (mode: ThemeMode) => void
}

function resolve(mode: ThemeMode): ResolvedTheme {
  if (mode === 'system') {
    const prefersDark = window.matchMedia(
      '(prefers-color-scheme: dark)'
    ).matches
    return { scheme: prefersDark ? 'dark' : 'light', style: 'pro' }
  }
  const [style, scheme] = mode.split('-') as [ThemeStyle, ColorScheme]
  return { style, scheme }
}

function applyToDOM(resolved: ResolvedTheme) {
  const root = document.documentElement
  root.setAttribute('data-theme', resolved.scheme)
  root.setAttribute('data-style', resolved.style)
  root.classList.toggle('dark', resolved.scheme === 'dark')
  root.classList.toggle('light', resolved.scheme === 'light')
  root.classList.toggle('pro', resolved.style === 'pro')
  root.classList.toggle('glass', resolved.style === 'glass')
  root.style.colorScheme = resolved.scheme
}

export const useThemeStore = create<ThemeState>()(
  persist(
    (set) => ({
      mode: 'system',
      resolved: resolve('system'),

      setMode: (mode: ThemeMode) => {
        const resolved = resolve(mode)
        applyToDOM(resolved)
        set({ mode, resolved })
      },
    }),
    {
      name: 'ait-theme-mode',
      partialize: (state) => ({ mode: state.mode }),
    }
  )
)

// Listen for system theme changes
if (typeof window !== 'undefined') {
  window
    .matchMedia('(prefers-color-scheme: dark)')
    .addEventListener('change', () => {
      const store = useThemeStore.getState()
      if (store.mode === 'system') {
        const resolved = resolve('system')
        applyToDOM(resolved)
        useThemeStore.setState({ resolved })
      }
    })
}
