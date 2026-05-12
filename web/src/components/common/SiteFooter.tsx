import { t, type Language } from '../../i18n/translations'

interface SiteFooterProps {
  language: Language
}

export function SiteFooter({ language }: SiteFooterProps) {
  return (
    <footer
      className="mt-16"
      style={{ borderTop: '1px solid #2B3139', background: '#181A20' }}
    >
      <div
        className="max-w-[1920px] mx-auto px-6 py-6 text-center text-sm"
        style={{ color: '#5E6673' }}
      >
        <p>{t('footerTitle', language)}</p>
        <p className="mt-1">{t('footerWarning', language)}</p>
      </div>
    </footer>
  )
}
