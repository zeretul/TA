import { useCallback, useLayoutEffect, useRef } from 'react'

/** Prepending history must preserve the line being read, not jump to the tail. */
export function useTranscriptScroll(scope: string, exchanges: { id: string }[] | undefined, loadingEarlier: boolean) {
  const listRef = useRef<HTMLDivElement | null>(null)
  const earlierAnchor = useRef<{ scope: string; top: number; height: number } | null>(null)
  const previous = useRef({ scope, first: '', last: '' })
  const first = exchanges?.[0]?.id || ''
  const last = exchanges?.at(-1)?.id || ''
  useLayoutEffect(() => {
    const node = listRef.current
    const prior = previous.current
    const anchor = earlierAnchor.current
    if (node && first) {
      if (anchor?.scope === scope && prior.first !== first) {
        node.scrollTop = anchor.top + (node.scrollHeight - anchor.height)
        earlierAnchor.current = null
      } else if (prior.scope !== scope || prior.last !== last) {
        node.scrollTop = node.scrollHeight
      }
    }
    if (!loadingEarlier || prior.scope !== scope) earlierAnchor.current = null
    previous.current = { scope, first, last }
  }, [scope, first, last, loadingEarlier])

  const preserveEarlier = useCallback((load: () => Promise<void>) => {
    const node = listRef.current
    if (node) earlierAnchor.current = { scope, top: node.scrollTop, height: node.scrollHeight }
    return load()
  }, [scope])
  return { listRef, preserveEarlier }
}
