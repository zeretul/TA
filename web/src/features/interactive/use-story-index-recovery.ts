import { useEffect, useRef } from 'react'
import { getInteractiveStories } from './api'
import { useInteractiveStore } from './stores/interactive-store'

/**
 * Restores the current story for the surfaces that do not mount the game.
 *
 * The game surface owns the story index while it is mounted, and it used to be
 * the only reader of `/api/interactive/stories`. A direct reload on the lore
 * library mounts no game surface, so `currentStoryId` stayed empty and the
 * 本局新建 page stayed disabled until the player detoured through the game.
 *
 * This reads that same index and hands it to the store, which is where the
 * decision lives: the server's `current_story_id` is kept only while it still
 * names a story in the list, and an empty workspace leaves the index empty
 * rather than falling back to whatever a previous workspace left behind.
 */
export function useStoryIndexRecovery(workspace: string | undefined, enabled: boolean) {
  const setStories = useInteractiveStore((state) => state.setStories)
  const resetWorkspaceState = useInteractiveStore((state) => state.resetWorkspaceState)
  const requestSeqRef = useRef(0)
  const workspaceRef = useRef<string | undefined>(undefined)

  useEffect(() => {
    if (!enabled || workspace === undefined) return
    // A switch has to drop the previous workspace's index before the new read
    // lands, or the library would briefly offer another world's stories.
    if (workspaceRef.current !== workspace) {
      workspaceRef.current = workspace
      resetWorkspaceState()
    }
    if (!workspace) return

    const requestSeq = requestSeqRef.current + 1
    requestSeqRef.current = requestSeq
    let cancelled = false
    void getInteractiveStories()
      .then((index) => {
        if (cancelled || requestSeq !== requestSeqRef.current) return
        setStories(index.stories || [], index.current_story_id)
      })
      .catch((error: unknown) => {
        if (cancelled || requestSeq !== requestSeqRef.current) return
        console.error('[story-index-recovery] Failed to restore the current story', { workspace, error })
      })
    return () => {
      cancelled = true
      // The game surface takes over on the next render; a reply that arrives
      // after that must not overwrite the index it is now responsible for.
      requestSeqRef.current += 1
    }
  }, [enabled, resetWorkspaceState, setStories, workspace])
}
