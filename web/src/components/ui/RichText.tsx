/**
 * Minimal rich text for dictionary strings (#81 PR 2): `code` spans,
 * **bold** and [label](url) links. Keeps long guides in the i18n
 * dictionaries instead of per-language JSX.
 */
export function RichText({ text }: { text: string }) {
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
