import { useEffect, useMemo, useRef, useState } from 'react'
import { getInteractiveTeller } from './api'
import type { StorySummary, Teller } from './types'

/**
 * The style ids one story refers to.
 *
 * A story keeps the id it was started with even after the recommended set
 * absorbs that style, and its dialogue and combat purposes may each name one
 * too. All three have to resolve to a name, or a picker shows a raw id.
 */
function referencedStyleIDs(story: StorySummary | undefined): string[] {
  if (!story) return []
  const ids = [story.story_teller_id, story.style_purposes?.dialogue, story.style_purposes?.combat]
  return ids.filter((id): id is string => Boolean(id))
}

/**
 * The preset list plus any style a story names that the list leaves out.
 *
 * `/api/interactive/tellers` reports the preset files that exist on disk. The
 * absorbed styles are deliberately not among them — offering twelve names for
 * six styles is what the merge removed — but the server still resolves every one
 * of them for a story that already saved it. This fills those in by id so the
 * selection shows its own Chinese name instead of looking unset.
 */
export function useNarrativeStyleCatalog(tellers: Teller[], story: StorySummary | undefined): Teller[] {
  const [resolved, setResolved] = useState<Record<string, Teller>>({})
  const requested = useRef(new Set<string>())

  const available = useMemo(() => new Set(tellers.map((teller) => teller.id)), [tellers])
  const missing = referencedStyleIDs(story).filter((id) => !available.has(id) && !resolved[id])
  // The effect has to depend on a stable value; the list itself is rebuilt on
  // every render.
  const missingKey = missing.join('\u0000')

  useEffect(() => {
    const pending = missingKey ? missingKey.split('\u0000') : []
    const fresh = pending.filter((id) => !requested.current.has(id))
    if (fresh.length === 0) return
    for (const id of fresh) requested.current.add(id)
    let cancelled = false
    void Promise.all(fresh.map((id) => getInteractiveTeller(id).catch(() => null))).then((loaded) => {
      if (cancelled) return
      const next: Record<string, Teller> = {}
      for (const teller of loaded) {
        if (teller?.id) next[teller.id] = teller
      }
      if (Object.keys(next).length > 0) setResolved((current) => ({ ...current, ...next }))
    })
    return () => { cancelled = true }
  }, [missingKey])

  return useMemo(() => {
    const extra = Object.values(resolved).filter((teller) => !available.has(teller.id))
    return extra.length === 0 ? tellers : [...tellers, ...extra]
  }, [available, resolved, tellers])
}
