import type { StorySummary } from './types'

/**
 * Puts a just-saved story back into the index before the server's list lands.
 *
 * Every surface that writes a story (the game stage and game settings) has to
 * publish the same index shape, otherwise the surface the player is not looking
 * at would show the previous title or settings until the next reload.
 */
export function mergePreferredStory(stories: StorySummary[], preferredStory?: StorySummary) {
  if (!preferredStory) return stories
  let found = false
  const nextStories = stories.map((story) => {
    if (story.id !== preferredStory.id) return story
    found = true
    return preferredStory
  })
  return found ? nextStories : [preferredStory, ...nextStories]
}
