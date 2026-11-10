import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { getQuests } from '../../api'
import type { QuestState } from '../../quest-types'

export interface QuestEntryState {
  data: QuestState | null
  loading: boolean
  /** Non-null when the last read failed. The panel shows it and offers a retry. */
  error: string | null
}

export interface UseQuestsOptions {
  storyId: string
  branchId: string
  /** False keeps the panel from reading until it is actually open. */
  active: boolean
}

export interface UseQuestsResult extends QuestEntryState {
  reload: () => Promise<void>
}

/**
 * Reads 见闻与事务.
 *
 * It is a read and nothing else. Accepting, handing in, collecting and leaving a
 * matter are all said in the main story's own input and settled by the formal
 * path, so this hook has no act() to call: a panel that could also accept would
 * be a second path into the same record, and the two would drift.
 *
 * Opening the panel costs no world time and no generation, and it never settles
 * anything — the expiry it reports was already written by an interval commit.
 */
export function useQuests({ storyId, branchId, active }: UseQuestsOptions): UseQuestsResult {
  const [state, setState] = useState<QuestEntryState>({ data: null, loading: false, error: null })
  const requestSeqRef = useRef(0)

  const read = useCallback(async () => {
    if (!active || !storyId) return
    const seq = ++requestSeqRef.current
    setState(current => ({ ...current, loading: true, error: null }))
    try {
      const data = await getQuests(storyId, { branchId: branchId || undefined })
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
  }, [active, branchId, storyId])

  useEffect(() => {
    if (!active) return
    void read()
  }, [active, read])

  return useMemo(() => ({ ...state, reload: () => read() }), [read, state])
}
