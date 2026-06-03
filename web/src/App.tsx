import { ConfirmDialogProvider } from './components/common/ConfirmDialog'
import { AuthProvider } from './contexts/AuthContext'
import { LanguageProvider } from './contexts/LanguageContext'
import { ThemeProvider } from './contexts/ThemeContext'
import { AppRoutes } from './router/AppRoutes'

export default function App() {
  return (
    <LanguageProvider>
      <ThemeProvider>
        <AuthProvider>
          <ConfirmDialogProvider>
            <AppRoutes />
          </ConfirmDialogProvider>
        </AuthProvider>
      </ThemeProvider>
    </LanguageProvider>
  )
}
