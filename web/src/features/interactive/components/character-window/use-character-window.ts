import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { getCharacterWindow, sendCharacterMessage, setCharacterMessageStatus } from '../../api'
import type { CharacterWindow } from '../../types'

export interface CharacterWindowState {
  data: CharacterWindow | null
  loading: boolean
  /** Non-null when the last read failed. The panel shows it and offers a retry. */
  error: string | null
  /** True while a reply is being generated. */
  sending: boolean
  /** The player stopped waiting; the server may still commit the exchange. */
  cancelled: boolean
  /** Non-null when the last send failed. The message stays in the box. */
  sendError: string | null
  /**
   * The last committed change of feeling, if this exchange moved it. The panel
   * shows one short line for it and then forgets it; an ordinary exchange that
   * changed nothing leaves this null and says nothing.
   */
  affinityChange: { delta: number; value: number; band: string } | null
  hasMore: boolean
  loadingEarlier: boolean
}

export interface UseCharacterWindowOptions {
  storyId: string
  branchId: string
  /** The character the panel is open on. Empty shows the cast and the scene. */
  characterId: string
  /** False keeps the panel from reading until it is actually open. */
  active: boolean
}

export interface UseCharacterWindowResult extends CharacterWindowState {
  reload: () => Promise<void>
  openCharacter: (characterId: string) => void
  loadEarlier: () => Promise<void>
  send: (text: string) => Promise<boolean>
  cancel: () => void
  /**
   * Records what the player did with one incoming message. Marking a message
   * handled or ignored never accepts what it proposed: only a committed reply
   * can do that.
   */
  markMessage: (messageId: string, status: 'read' | 'handled' | 'ignored') => Promise<void>
}

function emptyCharacterWindowState(): CharacterWindowState {
  return { data: null, loading: false, error: null, sending: false,
    cancelled: false, sendError: null, affinityChange: null, hasMore: false, loadingEarlier: false }
}

/**
 * Reads and drives the character window.
 *
 * The read is a plain projection of committed facts, so opening the panel costs
 * no world time and no generation. Sending is the only thing that calls a model,
 * and it commits the whole exchange in one transaction: the panel never shows a
 * line the journal does not have.
 */
export function useCharacterWindow({ storyId, branchId, characterId, active }: UseCharacterWindowOptions): UseCharacterWindowResult {
  const [state, setState] = useState<CharacterWindowState>(() => emptyCharacterWindowState())
  // Stopping the wait is local only. It cannot cancel a server transaction.
  const abortRef = useRef<{ cancelled: boolean } | null>(null)
  const requestSeqRef = useRef(0)
  const scopeRef = useRef(0)

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
      const data = await getCharacterWindow(storyId, {
        branchId: branchId || undefined,
        characterId: characterId || undefined,
        before,
      })
      // A slower earlier read must not overwrite a newer one.
      if (seq !== requestSeqRef.current) return
      setState(current => ({
        ...current,
        data: before && current.data
          ? { ...data, exchanges: [...(data.exchanges || []), ...(current.data.exchanges || [])] }
          : data,
        hasMore: data.has_more,
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
  }, [active, branchId, characterId, storyId])

  useEffect(() => {
    scopeRef.current += 1
    requestSeqRef.current += 1
    abortRef.current = null
    setState(emptyCharacterWindowState())
    if (active) void read()
    return () => {
      scopeRef.current += 1
      requestSeqRef.current += 1
      abortRef.current = null
    }
  }, [active, read])

  const openCharacter = useCallback((nextCharacterId: string) => {
    setState(current => ({ ...current, sendError: null, cancelled: false, data: current.data ? { ...current.data, character_id: nextCharacterId, exchanges: [] } : current.data }))
  }, [])

  const loadEarlier = useCallback(async () => {
    const cursor = state.data?.cursor
    if (!cursor) return
    await read(cursor)
  }, [read, state.data?.cursor])

  const send = useCallback(async (text: string) => {
    const message = text.trim()
    if (!active || !storyId || !message || !characterId) return false
    const scope = scopeRef.current
    const token = { cancelled: false }
    abortRef.current = token
    setState(current => ({ ...current, sending: true, sendError: null, cancelled: false, affinityChange: null }))
    try {
      const result = await sendCharacterMessage(storyId, { characterId, message, branchId })
      if (scope !== scopeRef.current || token.cancelled || abortRef.current !== token) return false
      // The reply and the player's own message are both already committed, so
      // the panel reads them back rather than inventing local rows.
      await read()
      if (scope !== scopeRef.current || token.cancelled || abortRef.current !== token) return false
      setState(current => ({
        ...current, sending: false, sendError: null,
        // The change is read back from the committed record, so a repeated or
        // already-settled fact reports 0 and shows no line at all.
        affinityChange: result.affinity_delta
          ? { delta: result.affinity_delta, value: result.affinity, band: result.affinity_band }
          : null,
      }))
      return Boolean(result.reply)
    } catch (error) {
      if (scope !== scopeRef.current || token.cancelled || abortRef.current !== token) return false
      setState(current => ({
        ...current, sending: false,
        sendError: error instanceof Error ? error.message : String(error),
      }))
      return false
    } finally {
      if (abortRef.current === token) abortRef.current = null
    }
  }, [active, branchId, characterId, read, storyId])

  const cancel = useCallback(() => {
    if (abortRef.current) abortRef.current.cancelled = true
    setState(current => ({ ...current, sending: false, cancelled: true }))
  }, [])

  const markMessage = useCallback(async (messageId: string, status: 'read' | 'handled' | 'ignored') => {
    if (!active || !storyId || !characterId || !messageId) return
    const scope = scopeRef.current
    // The status is the player's own state, so a failed write is not fatal: the
    // message stays as it was and the next read shows the truth from the journal.
    try {
      await setCharacterMessageStatus(storyId, { characterId, messageId, status, branchId })
      if (scope !== scopeRef.current) return
      await read()
    } catch (error) {
      if (scope !== scopeRef.current) return
      setState(current => ({
        ...current,
        sendError: error instanceof Error ? error.message : String(error),
      }))
    }
  }, [active, branchId, characterId, read, storyId])

  return useMemo(() => ({
    ...state, reload: () => read(), openCharacter, loadEarlier, send, cancel, markMessage,
  }), [cancel, loadEarlier, markMessage, openCharacter, read, send, state])
}
