import { DeepVoidBackground } from '../components/common/DeepVoidBackground'
import { Compass, Home } from 'lucide-react'
import { useOptionalLanguage } from '../contexts/LanguageContext'
import { t } from '../i18n/translations'

export function PageNotFound() {
  const language = useOptionalLanguage()
  return (
    <DeepVoidBackground className="flex items-center justify-center text-center p-4">
      <div className="ait-card p-8 sm:p-10 max-w-md w-full">
        <div className="flex flex-col items-center gap-5">
          <span className="inline-flex w-14 h-14 items-center justify-center rounded-full bg-surface-alt text-muted-foreground">
            <Compass size={28} />
          </span>
          <div className="space-y-1">
            <div className="t-display t-num text-foreground">404</div>
            <h1 className="t-headline">{t('notFound.title', language)}</h1>
          </div>
          <p className="t-body text-muted-foreground">
            {t('notFound.body', language)}
          </p>
          <a
            href="/"
            className="inline-flex items-center gap-2 h-10 px-5 rounded-control bg-primary text-primary-foreground t-label hover:opacity-90"
          >
            <Home size={16} />
            {t('notFound.home', language)}
          </a>
        </div>
      </div>
    </DeepVoidBackground>
  )
}
