import type { UserMode } from '../../lib/onboarding'
import { Button } from '../ui/Button'
import { t, type Language } from '../../i18n/translations'

interface OnboardingModeSelectorProps {
  language: string
  mode: UserMode
  onChange: (mode: UserMode) => void
}

export function OnboardingModeSelector({
  language,
  mode,
  onChange,
}: OnboardingModeSelectorProps) {
  const tr = (key: string) => t(`onboardingMode.${key}`, language as Language)

  const options: Array<{
    id: UserMode
    title: string
    badge?: string
    description: string
  }> = [
    {
      id: 'beginner',
      title: tr('beginnerTitle'),
      badge: tr('recommended'),
      description: tr('beginnerDesc'),
    },
    {
      id: 'advanced',
      title: tr('advancedTitle'),
      description: tr('advancedDesc'),
    },
  ]

  return (
    <div className="space-y-2">
      <div className="text-xs font-medium text-muted-foreground">
        {tr('label')}
      </div>
      <div className="grid grid-cols-1 gap-2">
        {options.map((option) => {
          const selected = option.id === mode
          return (
            <Button
              variant="unstyled"
              key={option.id}
              type="button"
              onClick={() => onChange(option.id)}
              className={`w-full flex-col !items-start rounded-control border px-4 py-3 text-left transition-all ${
                selected
                  ? 'border-primary/60 bg-primary-dim ring-1 ring-primary/15'
                  : 'border-border bg-background/60 hover:border-border-hover'
              }`}
            >
              <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
                <span>{option.title}</span>
                {option.badge ? (
                  <span className="ait-pill bg-primary text-primary-foreground">
                    {option.badge}
                  </span>
                ) : null}
              </div>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {option.description}
              </p>
            </Button>
          )
        })}
      </div>
    </div>
  )
}
