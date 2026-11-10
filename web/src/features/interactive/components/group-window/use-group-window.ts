import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { changeGroupMembers, createGroup, getGroupWindow, sendGroupMessage } from '../../api'
import type { GroupWindow, GroupWindowGroup } from '../../types'

export interface GroupWindowState {
  data: GroupWindow | null
  loading: boolean
  /** Non-null when the last read failed. The panel shows it and offers a retry. */
  error: string | null
  /** True while a round is being generated. */
  sending: boolean
  /** Non-null when the last round failed. The draft stays in the box. */
  sendError: string | null
  /** True while a group is being created. */
  creating: boolean
  /** Non-null when the last creation failed. */
  createError: string | null
  /** True while a membership change is in flight. */
  changingMembers: boolean
  memberError: string | null
  hasMore: boolean
  loadingEarlier: boolean
  /**
   * Members who could not hear the last round because they were not reachable.
   * The panel names them, so the player is told who did not hear instead of
   * assuming the whole group answered.
   */
  silenced: string[]
}

export interface UseGroupWindowOptions {
  storyId: string
  branchId: string
  /** The group the panel is open on. Empty shows the group list. */
  groupId: string
  /** False keeps the panel from reading until it is actually open. */
  active: boolean
}

export interface UseGroupWindowResult extends GroupWindowState {
  reload: () => Promise<void>
  loadEarlier: () => Promise<void>
  send: (text: string) => Promise<boolean>
  /**
   * Builds a group and returns it. The caller needs the new group's id to open
   * it, and only the server knows what id it settled on.
   */
  create: (name: string, memberIds: string[]) => Promise<GroupWindowGroup | null>
  addMembers: (memberIds: string[]) => Promise<boolean>
  removeMember: (memberId: string) => Promise<boolean>
}

function emptyGroupWindowState(): GroupWindowState {
  return {
    data: null, loading: false, error: null, sending: false, sendError: null,
    creating: false, createError: null, changingMembers: false, memberError: null,
    hasMore: false, loadingEarlier: false, silenced: [],
  }
}

/**
 * Reads and drives the group window.
 *
 * Reading is a plain projection of committed facts, so opening a group costs no
 * world time and no generation. Sending is the only thing that calls a model,
 * and one send is one round: the whole exchange commits in one transaction, so
 * the panel never shows a line the journal does not have.
 */
export function useGroupWindow({ storyId, branchId, groupId, active }: UseGroupWindowOptions): UseGroupWindowResult {
  const [state, setState] = useState(emptyGroupWindowState)
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
      const data = await getGroupWindow(storyId, {
        branchId: branchId || undefined,
        groupId: groupId || undefined,
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
  }, [active, branchId, groupId, storyId])

  useEffect(() => {
    scopeRef.current++
    requestSeqRef.current++
    setState(emptyGroupWindowState())
    if (active) void read()
    return () => {
      scopeRef.current++
      requestSeqRef.current++
    }
  }, [active, read])

  const loadEarlier = useCallback(async () => {
    const cursor = state.data?.cursor
    if (!cursor) return
    await read(cursor)
  }, [read, state.data?.cursor])

  const send = useCallback(async (text: string) => {
    const message = text.trim()
    if (!active || !storyId || !message || !groupId) return false
    const scope = scopeRef.current
    setState(current => ({ ...current, sending: true, sendError: null, silenced: [] }))
    try {
      const result = await sendGroupMessage(storyId, { groupId, message, branchId })
      // The server may have committed to the old group. Do not refresh that
      // scope into the new panel or tell its host to clear the current draft.
      if (scope !== scopeRef.current) return false
      // The player's own line and every answer are already committed, so the
      // panel reads them back rather than inventing local rows.
      await read()
      if (scope !== scopeRef.current) return false
      setState(current => ({
        ...current, sending: false, sendError: null,
        silenced: result.silenced || [],
      }))
      return true
    } catch (error) {
      if (scope !== scopeRef.current) return false
      setState(current => ({
        ...current, sending: false,
        sendError: error instanceof Error ? error.message : String(error),
      }))
      return false
    }
  }, [active, branchId, groupId, read, storyId])

  const create = useCallback(async (name: string, memberIds: string[]) => {
    const trimmed = name.trim()
    if (!active || !storyId || !trimmed || memberIds.length === 0) return null
    const scope = scopeRef.current
    setState(current => ({ ...current, creating: true, createError: null }))
    try {
      const made = await createGroup(storyId, { name: trimmed, members: memberIds, branchId })
      if (scope !== scopeRef.current) return null
      await read()
      if (scope !== scopeRef.current) return null
      setState(current => ({ ...current, creating: false, createError: null }))
      return made
    } catch (error) {
      if (scope !== scopeRef.current) return null
      setState(current => ({
        ...current, creating: false,
        createError: error instanceof Error ? error.message : String(error),
      }))
      return null
    }
  }, [active, branchId, read, storyId])

  const changeMembers = useCallback(async (add: string[], remove: string[]) => {
    if (!active || !storyId || !groupId) return false
    const scope = scopeRef.current
    setState(current => ({ ...current, changingMembers: true, memberError: null }))
    try {
      await changeGroupMembers(storyId, groupId, { add, remove, branchId })
      if (scope !== scopeRef.current) return false
      await read()
      if (scope !== scopeRef.current) return false
      setState(current => ({ ...current, changingMembers: false, memberError: null }))
      return true
    } catch (error) {
      if (scope !== scopeRef.current) return false
      setState(current => ({
        ...current, changingMembers: false,
        memberError: error instanceof Error ? error.message : String(error),
      }))
      return false
    }
  }, [active, branchId, groupId, read, storyId])

  const addMembers = useCallback((memberIds: string[]) => changeMembers(memberIds, []), [changeMembers])
  const removeMember = useCallback((memberId: string) => changeMembers([], [memberId]), [changeMembers])

  return useMemo(() => ({
    ...state, reload: () => read(), loadEarlier, send, create, addMembers, removeMember,
  }), [addMembers, loadEarlier, read, removeMember, send, create, state])
}
