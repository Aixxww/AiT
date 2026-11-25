import { useEffect, useRef } from 'react'
import { t, type Language } from '../../i18n/translations'
import type { FAQCategory } from '../../data/faqData'
// RoadmapWidget 移除动态嵌入，按需仅展示外部链接

interface FAQContentProps {
  categories: FAQCategory[]
  language: Language
  onActiveItemChange: (itemId: string) => void
}

export function FAQContent({
  categories,
  language,
  onActiveItemChange,
}: FAQContentProps) {
  const sectionRefs = useRef<Map<string, HTMLElement>>(new Map())

  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            const itemId = entry.target.getAttribute('data-item-id')
            if (itemId) {
              onActiveItemChange(itemId)
            }
          }
        })
      },
      {
        rootMargin: '-100px 0px -80% 0px',
        threshold: 0,
      }
    )

    sectionRefs.current.forEach((ref) => {
      if (ref) observer.observe(ref)
    })

    return () => {
      sectionRefs.current.forEach((ref) => {
        if (ref) observer.unobserve(ref)
      })
    }
  }, [onActiveItemChange])

  const setRef = (itemId: string, element: HTMLElement | null) => {
    if (element) {
      sectionRefs.current.set(itemId, element)
    } else {
      sectionRefs.current.delete(itemId)
    }
  }

  return (
    <div className="space-y-12">
      {categories.map((category) => (
        <div key={category.id} className="ait-card p-5 sm:p-8">
          {/* Category Header */}
          <div className="flex items-center gap-3 mb-6 pb-3 border-b border-white/10">
            <category.icon className="w-7 h-7 text-primary" />
            <h2 className="text-2xl font-bold text-foreground">
              {t(category.titleKey, language)}
            </h2>
          </div>

          {/* FAQ Items */}
          <div className="space-y-8">
            {category.items.map((item) => (
              <section
                key={item.id}
                id={item.id}
                data-item-id={item.id}
                ref={(el) => setRef(item.id, el)}
                className="scroll-mt-24"
              >
                {/* Question */}
                <h3 className="text-xl font-semibold mb-3 text-foreground">
                  {t(item.questionKey, language)}
                </h3>

                {/* Answer */}
                <div className="prose prose-invert max-w-none text-muted-foreground leading-relaxed">
                  {item.id === 'github-projects-tasks' ? (
                    <ContributeGuide
                      language={language}
                      lead="faqGuide.links"
                      links={[
                        {
                          key: 'faqGuide.roadmap',
                          href: 'https://github.com/orgs/AiTAiOS/projects/3',
                        },
                        {
                          key: 'faqGuide.taskBoard',
                          href: 'https://github.com/orgs/AiTAiOS/projects/5',
                        },
                      ]}
                      steps="faqGuide.tasksSteps"
                      note="faqGuide.tasksNote"
                    />
                  ) : item.id === 'contribute-pr-guidelines' ? (
                    <ContributeGuide
                      language={language}
                      lead="faqGuide.references"
                      links={[
                        {
                          label: 'CONTRIBUTING.md',
                          href: 'https://github.com/Aixxww/AiT/blob/dev/CONTRIBUTING.md',
                        },
                        {
                          label: 'PR_TITLE_GUIDE.md',
                          href: 'https://github.com/Aixxww/AiT/blob/dev/.github/PR_TITLE_GUIDE.md',
                        },
                      ]}
                      steps="faqGuide.prSteps"
                      note="faqGuide.prNote"
                    />
                  ) : (
                    <p className="text-base">
                      <RichText text={t(item.answerKey, language)} />
                    </p>
                  )}
                </div>

                {/* Divider */}
                <div className="mt-6 h-px bg-border" />
              </section>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

/**
 * Minimal rich text for dictionary strings (#81 PR 2): `code` spans,
 * **bold** and [label](url) links. Keeps long guides in the i18n
 * dictionaries instead of per-language JSX.
 */
function RichText({ text }: { text: string }) {
  const parts = text.split(/(`[^`]+`|\*\*[^*]+\*\*|\[[^\]]+\]\([^)]+\))/g)
  return (
    <>
      {parts.map((p, i) => {
        if (p.startsWith('`') && p.endsWith('`')) {
          return (
            <code key={i} className="mx-0.5">
              {p.slice(1, -1)}
            </code>
          )
        }
        if (p.startsWith('**') && p.endsWith('**')) {
          return (
            <strong key={i} className="text-primary">
              {p.slice(2, -2)}
            </strong>
          )
        }
        const link = p.match(/^\[([^\]]+)\]\(([^)]+)\)$/)
        if (link) {
          return (
            <a
              key={i}
              href={link[2]}
              target="_blank"
              rel="noreferrer"
              className="text-primary hover:underline"
            >
              {link[1]}
            </a>
          )
        }
        return <span key={i}>{p}</span>
      })}
    </>
  )
}

function ContributeGuide({
  language,
  lead,
  links,
  steps,
  note,
}: {
  language: Language
  lead: string
  links: { key?: string; label?: string; href: string }[]
  steps: string
  note: string
}) {
  return (
    <div className="space-y-3">
      <div className="text-base">
        {t(lead, language)}{' '}
        {links.map((l, i) => (
          <span key={l.href}>
            {i > 0 && '  |  '}
            <a
              href={l.href}
              target="_blank"
              rel="noreferrer"
              className="text-primary hover:underline"
            >
              {l.key ? t(l.key, language) : <span data-raw>{l.label}</span>}
            </a>
          </span>
        ))}
      </div>
      <ol className="list-decimal pl-5 space-y-1 text-base">
        {t(steps, language)
          .split('\n')
          .map((line, i) => (
            <li key={i}>
              <RichText text={line} />
            </li>
          ))}
      </ol>
      <div className="rounded-control p-3 mt-3 bg-primary-dim border border-primary/25 text-sm">
        <RichText text={t(note, language)} />
      </div>
    </div>
  )
}
