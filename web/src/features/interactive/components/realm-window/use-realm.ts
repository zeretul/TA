import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getRealm } from '../../api'
import type { RealmState } from '../../realm-types'

export interface RealmEntryState {
  data: RealmState | null
  loading: boolean
  /** Non-null when the last read failed. The panel shows it and offers a retry. */
  error: string | null
}

export interface UseRealmOptions {
  storyId: string
  branchId: string
  /** False keeps the panel from reading until it is actually open. */
  active: boolean
}

export interface UseRealmResult extends RealmEntryState {
  reload: () => Promise<void>
}

/**
 * Reads the realm information panel.
 *
 * It is a read and nothing else. Entering, walking, taking and leaving are all
 * written in the main story's own input and settled by the turn's world action,
 * so this hook deliberately has no act() to call: an information panel that
 * could also mutate the realm would be a second path into it, and the two paths
 * would drift.
 *
 * The read is a plain projection of committed facts, so opening the panel costs
 * no world time and no generation.
 */
export function useRealm({ storyId, branchId, active }: UseRealmOptions): UseRealmResult {
  const { i18n } = useTranslation()
  const [state, setState] = useState<RealmEntryState>({ data: null, loading: false, error: null })
  const requestSeqRef = useRef(0)

  const read = useCallback(async () => {
    if (!active || !storyId) return
    const seq = ++requestSeqRef.current
    setState(current => ({ ...current, loading: true, error: null }))
    try {
      const data = await getRealm(storyId, { branchId: branchId || undefined })
      // A slower earlier read must not overwrite a newer one.
      if (seq !== requestSeqRef.current) return
      setState(current => ({ ...current, data, loading: false, error: null }))
    } catch (error) {
      if (seq !== requestSeqRef.current) return
      setState(current => ({
        ...current, loading: false,
        error: error instanceof Error ? error.message : String(error),
      }))
    }
  }, [active, branchId, storyId, i18n.language])

  useEffect(() => {
    requestSeqRef.current += 1
    setState({ data: null, loading: false, error: null })
    if (active) void read()
    return () => { requestSeqRef.current += 1 }
  }, [active, read])

  return useMemo(() => ({ ...state, reload: () => read() }), [read, state])
}
