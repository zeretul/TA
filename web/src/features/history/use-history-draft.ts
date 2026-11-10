import { useCallback, useState } from 'react'

/** Browser drafts are scoped to one campaign; canonical outcomes stay on the server. */
export function useHistoryDraft(scope: string, kind: string) {
  const key = `ta:history:draft:${scope}:${kind}`
  const [draft, setDraft] = useState(() => {
    try { return sessionStorage.getItem(key) || '' } catch { return '' }
  })
  const save = useCallback((value: string) => {
    setDraft(value)
    try {
      if (value) sessionStorage.setItem(key, value)
      else sessionStorage.removeItem(key)
    } catch { /* The current editor still retains the draft if storage is unavailable. */ }
  }, [key])
  return [draft, save] as const
}
