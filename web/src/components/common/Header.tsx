import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { Container } from './Container'

interface HeaderProps {
  simple?: boolean // For login/register pages
}

export function Header({ simple = false }: HeaderProps) {
  const { language, setLanguage } = useLanguage()

  return (
    <header className="glass sticky top-0 z-50 backdrop-blur-xl">
      <Container className="py-4">
        <div className="flex items-center justify-between">
          {/* Left - Logo and Title */}
          <div className="flex items-center gap-3">
            <div className="flex items-center justify-center">
              <img src="/icons/ait.svg" alt="AiT Logo" className="w-8 h-8" />
            </div>
            <div>
              <h1 className="text-xl font-bold text-foreground">
                {t('appTitle', language)}
              </h1>
              {!simple && (
                <p className="text-xs mono text-muted-foreground">
                  {t('subtitle', language)}
                </p>
              )}
            </div>
          </div>

          {/* Right - Language Toggle (always show) */}
          <div
            className="flex gap-1 rounded p-1"
            style={{ background: 'var(--color-panel)' }}
          >
            <button
              onClick={() => setLanguage('zh')}
              className="px-3 py-1.5 rounded text-xs font-semibold transition-all"
              style={
                language === 'zh'
                  ? { background: 'var(--color-primary)', color: 'var(--color-primary-fg)' }
                  : { background: 'transparent', color: 'var(--color-muted-fg)' }
              }
            >
              中文
            </button>
            <button
              onClick={() => setLanguage('en')}
              className="px-3 py-1.5 rounded text-xs font-semibold transition-all"
              style={
                language === 'en'
                  ? { background: 'var(--color-primary)', color: 'var(--color-primary-fg)' }
                  : { background: 'transparent', color: 'var(--color-muted-fg)' }
              }
            >
              EN
            </button>
            <button
              onClick={() => setLanguage('id')}
              className="px-3 py-1.5 rounded text-xs font-semibold transition-all"
              style={
                language === 'id'
                  ? { background: 'var(--color-primary)', color: 'var(--color-primary-fg)' }
                  : { background: 'transparent', color: 'var(--color-muted-fg)' }
              }
            >
              ID
            </button>
          </div>
        </div>
      </Container>
    </header>
  )
}
