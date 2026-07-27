/**
 * Hunter v7 tag glossary — lazy, memoized client for
 * GET /api/hunter/v7/tag-catalog.
 *
 * The catalog is static per backend build, so it is fetched at most once per
 * page load (first chip that mounts triggers the fetch; every later consumer
 * shares the same promise). Failures reset the memo so a later mount can
 * retry, and consumers simply render without tooltips in the meantime.
 */
import { useEffect, useState } from 'react'
import { api } from './api'
import type { V7TagDefinition } from './api/hunter'

export type TagCatalog = Map<string, V7TagDefinition>

let catalogPromise: Promise<TagCatalog> | null = null
let cachedCatalog: TagCatalog | null = null

export function loadTagCatalog(): Promise<TagCatalog> {
  if (!catalogPromise) {
    // Promise.resolve() guard: a synchronous throw from the API layer must
    // surface as a rejection, not crash the calling render/effect.
    catalogPromise = Promise.resolve()
      .then(() => api.getV7TagCatalog())
      .then((res) => {
        const map: TagCatalog = new Map(
          (res.tags ?? []).map((def) => [def.tag, def])
        )
        cachedCatalog = map
        return map
      })
      .catch((err) => {
        catalogPromise = null
        throw err
      })
  }
  return catalogPromise
}

/** Test hook: clear the module-level memo. */
export function resetTagCatalogCache() {
  catalogPromise = null
  cachedCatalog = null
}

/**
 * Resolve a tag code against the catalog. Handles the aggregated-chip suffix
 * (`code ×N`) and the runtime prefix families that cannot be enumerated
 * statically (`live_confirmed_*`, `sector_theme_*`), mirroring
 * describeHunterV7PrefixTag on the Go side.
 */
export function describeTag(
  catalog: TagCatalog | null,
  code: string
): V7TagDefinition | undefined {
  if (!catalog) return undefined
  const base = code.split(' ')[0]
  const exact = catalog.get(base)
  if (exact) return exact
  if (base.startsWith('live_confirmed_') && base.length > 15) {
    return {
      tag: base,
      source: 'required_confirmation',
      category: 'confirmation',
      polarity: 'bullish',
      llm_action: 'supports_open_after_core_checks',
      definition: `The required confirmation '${base.slice(15)}' was machine-verified against live market data in this cycle.`,
    }
  }
  if (base.startsWith('sector_theme_') && base.length > 13) {
    return {
      tag: base,
      source: 'reason_code',
      category: 'state',
      polarity: 'bullish',
      llm_action: 'evidence_only',
      definition: `Symbol belongs to the '${base.slice(13)}' sector theme currently showing broad relative strength.`,
    }
  }
  return undefined
}

/** Multi-line native tooltip text for a tag chip, or undefined when unknown. */
export function tagTooltip(
  catalog: TagCatalog | null,
  code: string
): string | undefined {
  const def = describeTag(catalog, code)
  if (!def) return undefined
  return `${def.definition}\n[${def.source} · ${def.category} · ${def.llm_action}]`
}

/**
 * React hook: returns the shared tag catalog once loaded, null before that
 * (and null forever if the backend is unreachable — chips degrade to plain
 * codes without tooltips).
 */
export function useTagCatalog(): TagCatalog | null {
  const [catalog, setCatalog] = useState<TagCatalog | null>(cachedCatalog)
  useEffect(() => {
    if (catalog) return
    let alive = true
    loadTagCatalog()
      .then((map) => {
        if (alive) setCatalog(map)
      })
      .catch(() => {
        /* degrade silently: chips render without tooltips */
      })
    return () => {
      alive = false
    }
  }, [catalog])
  return catalog
}
