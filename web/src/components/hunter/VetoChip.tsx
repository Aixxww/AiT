/**
 * Tag chip: renders a Hunter v7 reason / veto / confirmation code (e.g.
 * `fresh_oi_absent`) as its user-facing name from the shared display-name
 * map (issue #81). Raw codes never reach the screen.
 *
 * Veto reasons are first-class data in the v7 lean kernel: they must stay
 * readable (muted-fg, ≥4.5:1) even inside an otherwise dimmed REJECTED
 * row — never inherit the row's disabled color.
 *
 * The chip is also the trigger for its explanation (hover on desktop, tap
 * on touch): category, polarity and the model instruction from the tag
 * catalog, plus the English catalog definition when the UI is in English.
 */
import { describeTag, useTagCatalog } from '../../lib/tagCatalog'
import { cn } from '../../lib/cn'
import { InfoTip } from '../ui/InfoTip'
import { tagLabel, tagMetaLabel } from '../../i18n/displayNames'
import type { Language } from '../../i18n/translations'

interface VetoChipProps {
  /** Raw confirmation/veto code; rendered through the display-name map. */
  code: string
  /** Optional explicit explanation; overrides the catalog definition. */
  title?: string
  className?: string
  language?: Language
}

export function VetoChip({
  code,
  title,
  className = '',
  language = 'en',
}: VetoChipProps) {
  const catalog = useTagCatalog()
  const label = tagLabel(code, language)
  const def = describeTag(catalog, code)
  const chip = (
    <button
      type="button"
      className={cn(
        'inline-flex max-w-full items-center rounded-pill border border-border bg-surface-alt px-2 py-0.5 text-[11px] text-muted-foreground hover:text-foreground',
        className
      )}
    >
      <span className="truncate">{label}</span>
    </button>
  )
  if (!def && !title) return chip
  const meta = def
    ? [
        tagMetaLabel('category', def.category, language),
        tagMetaLabel('polarity', def.polarity, language),
        tagMetaLabel('action', def.llm_action, language),
      ]
        .filter(Boolean)
        .join(' · ')
    : ''
  const content = (
    <>
      {meta && <span className="block t-caption">{meta}</span>}
      {language === 'en' && (title ?? def?.definition) && (
        <span className="block mt-1">{title ?? def?.definition}</span>
      )}
      {language !== 'en' && title && (
        <span className="block mt-1">{title}</span>
      )}
    </>
  )
  return (
    <InfoTip content={content} title={label}>
      {chip}
    </InfoTip>
  )
}
