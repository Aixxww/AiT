import { t, type Language } from '../../i18n/translations'

interface SiteFooterProps {
  language: Language
}

export function SiteFooter({ language }: SiteFooterProps) {
  return (
    <footer className="mt-16 border-t border-border bg-surface">
      <div className="max-w-[1920px] mx-auto px-6 py-6 text-center text-sm text-muted-foreground">
        <p>{t('footerTitle', language)}</p>
        <p className="mt-1">{t('footerWarning', language)}</p>
      </div>
    </footer>
  )
}
