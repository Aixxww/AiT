import { ConfirmDialogProvider } from './components/common/ConfirmDialog'
import { AuthProvider } from './contexts/AuthContext'
import { LanguageProvider } from './contexts/LanguageContext'
import { ThemeProvider, useTheme } from './contexts/ThemeContext'
import { AppRoutes } from './router/AppRoutes'
import { Toaster } from 'sonner'

function ThemedToaster() {
  const { scheme } = useTheme()

  return (
    <Toaster
      theme={scheme}
      richColors
      closeButton
      position="top-center"
      duration={2200}
      toastOptions={{
        className: 'ait-toast',
        style: {
          background: 'var(--color-panel)',
          border: '1px solid var(--color-border)',
          color: 'var(--color-foreground)',
        },
      }}
    />
  )
}

export default function App() {
  return (
    <LanguageProvider>
      <ThemeProvider>
        <ThemedToaster />
        <AuthProvider>
          <ConfirmDialogProvider>
            <AppRoutes />
          </ConfirmDialogProvider>
        </AuthProvider>
      </ThemeProvider>
    </LanguageProvider>
  )
}
