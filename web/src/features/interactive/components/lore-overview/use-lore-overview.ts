import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { getLoreOverview } from '../../api'
import type { CharacterWindow, Snapshot } from '../../types'
import type { JourneyState } from '../../journey-types'
import type { PublicEventState } from '../../public-event-types'
import type { QuestState } from '../../quest-types'

interface UseLoreOverviewOptions {
  storyId: string
  branchId: string
  /** True while the overview is on screen. It is what starts the reads. */
  active: boolean
  /**
   * Changes when the committed turn changes. A story that moves on while the
   * panel is open should show the new turn's people and matters, so the reads
   * follow it. This is still only a read: it never asks for a turn.
   */
  revision?: string
  /** Kept for existing callers; an atomic response always includes its snapshot. */
  withSnapshot?: boolean
}

interface LoreOverviewRead {
  snapshot: Snapshot | null
  characters: CharacterWindow | null
  journey: JourneyState | null
  quests: QuestState | null
  events: PublicEventState | null
}

interface LoreOverviewState {
  data: LoreOverviewRead | null
  loading: boolean
  error: string | null
}

/**
 * Reads all views and the player snapshot under one server-side journal lease.
 * A commit cannot land between their projections. Opening costs no world time
 * and makes no model call.
 *
 * A response is only allowed to land if the panel is still asking the same
 * question. Sequence numbers alone are not enough: a branch switch clears the
 * screen without starting a new read, so an in-flight read from the branch just
 * left would still carry the newest sequence and repaint its people and matters
 * over the branch now on screen. The read therefore carries the identity it was
 * started for — story, branch, and whether the panel is on screen — and both the
 * success and the failure path drop it once that identity is no longer current.
 * Leaving the panel or unmounting retires the identity too, so a late answer
 * cannot write into a screen nobody is looking at.
 */
export function useLoreOverview({ storyId, branchId, active, revision = '', withSnapshot = false }: UseLoreOverviewOptions) {
  const [state, setState] = useState<LoreOverviewState>({ data: null, loading: false, error: null })
  const requestSeqRef = useRef(0)
  // The identity of the question currently worth answering. A read compares
  // against it after its awaits, so it must change whenever the question does.
  const identityRef = useRef('')
  const identity = active ? `${storyId}\u0000${branchId}\u0000${revision}` : ''

  // A branch change invalidates the previous read entirely: keeping it on screen
  // would show one life's people beside another life's matters. Retiring the
  // sequence is what stops the response still in flight from landing, and it
  // runs before the read effect below so that read starts under the new identity.
  // Leaving the panel is the same kind of invalidation.
  useEffect(() => {
    identityRef.current = identity
    requestSeqRef.current++
    setState({ data: null, loading: false, error: null })
  }, [identity])

  useEffect(() => () => {
    requestSeqRef.current++
    identityRef.current = ''
  }, [])

  const read = useCallback(async () => {
    if (!active || !storyId) return
    const seq = ++requestSeqRef.current
    const startedFor = identityRef.current
    setState((current) => ({ ...current, loading: true, error: null }))
    const isCurrent = () => seq === requestSeqRef.current && startedFor === identityRef.current
    try {
      const options = { branchId: branchId || undefined }
      const { characters, journey, quests, events, snapshot } = await getLoreOverview(storyId, options)
      if (!isCurrent()) return
      const views = [characters, journey, quests, events, snapshot]
      if (views.some(view => !view || view.story_id !== storyId || (branchId && view.branch_id !== branchId))
        || views.some(view => view.branch_id !== characters.branch_id)) {
        throw new Error('loreOverview.changedDuringRead')
      }
      setState({ data: { snapshot, characters, journey, quests, events }, loading: false, error: null })
    } catch (error) {
      // A failure from a branch the reader already left must not put an error on
      // the branch now on screen either.
      if (!isCurrent()) return
      setState((current) => ({
        ...current,
        loading: false,
        error: error instanceof Error ? error.message : String(error),
      }))
    }
  }, [active, branchId, storyId, withSnapshot])

  useEffect(() => {
    if (!active) return
    void read()
  }, [active, read, revision])

  return useMemo(() => {
    const matches = identity === identityRef.current
    return { data: matches ? state.data : null, loading: active && (!matches || state.loading), error: matches ? state.error : null, reload: () => read() }
  }, [active, identity, read, state])
}
