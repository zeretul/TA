import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  confirmIntimacy, endIntimacy, getIntimacyWindow, inviteIntimacy, respondIntimacy, sendIntimacyMessage,
} from '../../api'
import type { IntimacyWindow } from '../../types'

export interface IntimacyWindowState {
  data: IntimacyWindow | null
  loading: boolean
  error: string | null
  /** True while an invitation or an answer is being generated. */
  inviting: boolean
  inviteError: string | null
  /** The person whose invitation is in flight, so only their row shows progress. */
  invitingCharacterID: string
  /** True while the confirmed roster's opening is being generated. */
  starting: boolean
  startError: string | null
  /** True while a line is being generated. */
  sending: boolean
  sendError: string | null
  ending: boolean
  endError: string | null
  hasMore: boolean
  loadingEarlier: boolean
}

export interface UseIntimacyWindowOptions {
  storyId: string
  branchId: string
  /** The activity the panel is open on. Empty shows the activity list. */
  activityId: string
  /** False keeps the panel from reading until it is actually open. */
  active: boolean
}

export interface UseIntimacyWindowResult extends IntimacyWindowState {
  reload: () => Promise<void>
  loadEarlier: () => Promise<void>
  /** Asks one person to join. oneOnOne is the §3.1 direct-entry path. Returns the activity id. */
  invite: (characterId: string, oneOnOne: boolean) => Promise<string | null>
  /** Asks one invited person again, for an invitation still without an answer. */
  askAgain: (characterId: string) => Promise<boolean>
  /** Starts with the roster the player was shown, at the version they were shown. */
  start: (expectedVersion: number) => Promise<boolean>
  send: (text: string) => Promise<boolean>
  end: () => Promise<boolean>
}

/**
 * Reads and drives the intimacy window.
 *
 * Reading is a plain projection of committed facts, so opening an activity,
 * re-reading a saved one and resuming a running one all cost no world time and
 * no generation. Every write goes through the dedicated model and commits in
 * one transaction, so the panel never shows a line the journal does not have.
 *
 * A failure is a failure: it is reported as an error and never written down as
 * the person refusing. Only their own answer changes a status.
 */
export function useIntimacyWindow({ storyId, branchId, activityId, active }: UseIntimacyWindowOptions): UseIntimacyWindowResult {
  const [state, setState] = useState<IntimacyWindowState>({
    data: null, loading: false, error: null,
    inviting: false, inviteError: null, invitingCharacterID: '',
    starting: false, startError: null,
    sending: false, sendError: null,
    ending: false, endError: null,
    hasMore: false, loadingEarlier: false,
  })
  const requestSeqRef = useRef(0)

  const read = useCallback(async (before?: string) => {
    if (!active || !storyId) return
    const seq = ++requestSeqRef.current
    setState(current => ({
      ...current,
      loading: !before,
      loadingEarlier: Boolean(before),
      error: null,
    }))
    try {
      const data = await getIntimacyWindow(storyId, {
        branchId: branchId || undefined,
        activityId: activityId || undefined,
        before,
      })
      // A slower earlier read must not overwrite a newer one.
      if (seq !== requestSeqRef.current) return
      setState(current => ({
        ...current,
        data: before && current.data
          ? { ...data, exchanges: [...(data.exchanges || []), ...(current.data.exchanges || [])] }
          : data,
        hasMore: false,
        loading: false,
        loadingEarlier: false,
        error: null,
      }))
    } catch (error) {
      if (seq !== requestSeqRef.current) return
      setState(current => ({
        ...current, loading: false, loadingEarlier: false,
        error: error instanceof Error ? error.message : String(error),
      }))
    }
  }, [active, activityId, branchId, storyId])

  useEffect(() => {
    if (!active) return
    void read()
  }, [active, read])

  const loadEarlier = useCallback(async () => {
    // The window read has no cursor yet, so there is nothing earlier to ask for.
    await read()
  }, [read])

  const invite = useCallback(async (characterId: string, oneOnOne: boolean) => {
    const id = characterId.trim()
    if (!id) return null
    setState(current => ({
      ...current, inviting: true, inviteError: null, invitingCharacterID: id,
    }))
    try {
      const result = await inviteIntimacy(storyId, {
        characterId: id,
        activityId: activityId || undefined,
        oneOnOne,
        branchId,
      })
      // Their answer, and any opening it produced, are already committed. Read
      // them back instead of inventing rows from what this attempt expected.
      await read()
      setState(current => ({ ...current, inviting: false, inviteError: null, invitingCharacterID: '' }))
      return result.activity_id || null
    } catch (error) {
      setState(current => ({
        ...current, inviting: false, invitingCharacterID: '',
        inviteError: error instanceof Error ? error.message : String(error),
      }))
      return null
    }
  }, [activityId, branchId, read, storyId])

  const askAgain = useCallback(async (characterId: string) => {
    const id = characterId.trim()
    if (!id || !activityId) return false
    setState(current => ({
      ...current, inviting: true, inviteError: null, invitingCharacterID: id,
    }))
    try {
      await respondIntimacy(storyId, activityId, { characterId: id, branchId })
      await read()
      setState(current => ({ ...current, inviting: false, inviteError: null, invitingCharacterID: '' }))
      return true
    } catch (error) {
      setState(current => ({
        ...current, inviting: false, invitingCharacterID: '',
        inviteError: error instanceof Error ? error.message : String(error),
      }))
      return false
    }
  }, [activityId, branchId, read, storyId])

  const start = useCallback(async (expectedVersion: number) => {
    if (!activityId) return false
    setState(current => ({ ...current, starting: true, startError: null }))
    try {
      await confirmIntimacy(storyId, activityId, { expectedVersion, branchId })
      await read()
      setState(current => ({ ...current, starting: false, startError: null }))
      return true
    } catch (error) {
      setState(current => ({
        ...current, starting: false,
        startError: error instanceof Error ? error.message : String(error),
      }))
      return false
    }
  }, [activityId, branchId, read, storyId])

  const send = useCallback(async (text: string) => {
    const message = text.trim()
    if (!message || !activityId) return false
    setState(current => ({ ...current, sending: true, sendError: null }))
    try {
      await sendIntimacyMessage(storyId, activityId, { message, branchId })
      // The player's line and every answer are committed, so the panel reads
      // them back rather than adding local rows.
      await read()
      setState(current => ({ ...current, sending: false, sendError: null }))
      return true
    } catch (error) {
      setState(current => ({
        ...current, sending: false,
        sendError: error instanceof Error ? error.message : String(error),
      }))
      return false
    }
  }, [activityId, branchId, read, storyId])

  const end = useCallback(async () => {
    if (!activityId) return false
    setState(current => ({ ...current, ending: true, endError: null }))
    try {
      await endIntimacy(storyId, activityId, { branchId })
      await read()
      setState(current => ({ ...current, ending: false, endError: null }))
      return true
    } catch (error) {
      setState(current => ({
        ...current, ending: false,
        endError: error instanceof Error ? error.message : String(error),
      }))
      return false
    }
  }, [activityId, branchId, read, storyId])

  return useMemo(() => ({
    ...state, reload: () => read(), loadEarlier, invite, askAgain, start, send, end,
  }), [askAgain, end, invite, loadEarlier, read, send, start, state])
}
