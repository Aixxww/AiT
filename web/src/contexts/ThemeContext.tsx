import { useEffect, type ReactNode } from 'react'
import { useThemeStore, type ThemeMode } from '../stores/themeStore'

export function ThemeProvider({ children }: { children: ReactNode }) {
  const { mode, setMode } = useThemeStore()

  // Initialize theme from persisted state on mount
  useEffect(() => {
    setMode(mode)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return <>{children}</>
}

export function useTheme() {
  const { mode, resolved, setMode } = useThemeStore()
  return {
    mode,
    scheme: resolved.scheme,
    style: resolved.style,
    isDark: resolved.scheme === 'dark',
    isGlass: resolved.style === 'glass',
    setMode,
  }
}

// Re-export types for convenience
export type { ThemeMode }
