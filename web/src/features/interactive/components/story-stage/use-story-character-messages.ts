import { useCallback, useEffect, useRef, useState } from 'react'
import { getCharacterWindow } from '../../api'
import type { CharacterWindowMessage } from '../../types'

export interface StoryCharacterMessagesState {
  /** Messages the player has not dealt with, newest first, across the whole cast. */
  pending: CharacterWindowMessage[]
  /** The unread subset, which is what the quiet dot counts. */
  unread: number
  refresh: () => Promise<void>
}

/**
 * Reads the story's incoming messages for the stage's quiet hint.
 *
 * It is the same read the character window performs, with no character selected,
 * so it is a plain projection of committed facts: no model call, no world time,
 * and nothing about opening or ignoring a message changes the story. It refreshes
 * when the branch advances, which is the only thing that can produce a new
 * message, so the hint cannot go stale while the player is reading.
 */
export function useStoryCharacterMessages(storyId: string, branchId: string, active: boolean, revisionKey: string): StoryCharacterMessagesState {
  const [pending, setPending] = useState<CharacterWindowMessage[]>([])
  const seqRef = useRef(0)

  const read = useCallback(async () => {
    if (!active || !storyId) return
    const seq = ++seqRef.current
    try {
      const data = await getCharacterWindow(storyId, { branchId: branchId || undefined })
      // A slower earlier read must not overwrite a newer one.
      if (seq !== seqRef.current) return
      setPending(data.pending || [])
    } catch {
      // The hint is not worth a failure state: the window itself reports read
      // errors, and a message list that cannot be read simply stays as it was.
    }
  }, [active, branchId, storyId])

  useEffect(() => {
    void read()
  }, [read, revisionKey])

  const unread = pending.filter((message) => message.status === 'unread').length
  return { pending, unread, refresh: read }
}
