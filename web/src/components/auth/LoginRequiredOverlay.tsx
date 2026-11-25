import { motion, AnimatePresence } from 'framer-motion'
import { LogIn, UserPlus, X, Lock, Check } from 'lucide-react'
import { Link } from 'react-router-dom'
import { DeepVoidBackground } from '../common/DeepVoidBackground'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { Button } from '../ui/Button'

interface LoginRequiredOverlayProps {
  isOpen: boolean
  onClose: () => void
  featureName?: string
}

export function LoginRequiredOverlay({
  isOpen,
  onClose,
  featureName,
}: LoginRequiredOverlayProps) {
  const { language } = useLanguage()

  const tr = (key: string, params?: Record<string, string | number>) =>
    t(`loginRequired.${key}`, language, params)

  const subtitle = featureName
    ? tr('subtitleWithFeature', { featureName })
    : tr('subtitleDefault')

  const benefits = [tr('benefit1'), tr('benefit2'), tr('benefit4')]

  return (
    <AnimatePresence>
      {isOpen && (
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          className="fixed inset-0 z-50"
        >
          <DeepVoidBackground
            className="w-full h-full bg-background/95 backdrop-blur-md flex items-center justify-center p-4 text-foreground"
            onClick={onClose}
          >
            <motion.div
              initial={{ opacity: 0, scale: 0.97, y: 8 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              exit={{ opacity: 0, scale: 0.97, y: 8 }}
              transition={{ type: 'spring', damping: 24, stiffness: 300 }}
              className="relative max-w-md w-full ait-card shadow-e3 rounded-sheet p-6 sm:p-8"
              role="dialog"
              aria-modal="true"
              aria-labelledby="login-required-title"
              onClick={(e) => e.stopPropagation()}
            >
              <Button
                variant="ghost"
                onClick={onClose}
                aria-label={tr('abort')}
                className="absolute top-3 right-3 rounded-control p-1.5"
              >
                <X size={16} />
              </Button>

              <div className="flex flex-col items-center text-center gap-3 mb-6">
                <div className="w-11 h-11 rounded-pill bg-primary-dim text-primary flex items-center justify-center">
                  <Lock size={20} />
                </div>
                <h2 id="login-required-title" className="t-title">
                  {tr('title')}
                </h2>
                <p className="t-body text-muted-foreground">{subtitle}</p>
              </div>

              <p className="t-body text-muted-foreground mb-4">
                {tr('description')}
              </p>

              <ul className="space-y-2 mb-6">
                {benefits.map((benefit, i) => (
                  <li key={i} className="flex items-center gap-2 t-body">
                    <Check className="w-4 h-4 text-primary shrink-0" />
                    {benefit}
                  </li>
                ))}
              </ul>

              <div className="space-y-3">
                <Link
                  to="/login"
                  className="flex items-center justify-center gap-2 w-full min-h-[44px] rounded-control bg-primary text-primary-foreground font-semibold text-sm hover:bg-primary/90 transition-colors"
                >
                  <LogIn size={16} />
                  <span>{tr('loginButton')}</span>
                </Link>
                <Link
                  to="/register"
                  className="flex items-center justify-center gap-2 w-full min-h-[44px] rounded-control border border-border text-foreground font-medium text-sm hover:bg-panel-hover hover:border-border-hover transition-colors"
                >
                  <UserPlus size={16} />
                  <span>{tr('registerButton')}</span>
                </Link>
              </div>
            </motion.div>
          </DeepVoidBackground>
        </motion.div>
      )}
    </AnimatePresence>
  )
}
