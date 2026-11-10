/**
 * Pack origin copy may refer to the affiliation the player has not chosen yet
 * (`身为{affiliation}的外门弟子`). The creation flow fills that in from the
 * chosen affiliation; browsing a world has no choice to fill in, so the page
 * states the placeholder's own meaning instead of showing the braces.
 */
export const ORIGIN_AFFILIATION_PLACEHOLDER = '{affiliation}'

export function originCopy(text: string, fallback: string): string {
  if (!text.includes(ORIGIN_AFFILIATION_PLACEHOLDER)) return text
  return text.split(ORIGIN_AFFILIATION_PLACEHOLDER).join(fallback)
}

/** Authored alternatives remain in the pack for previously saved starts. */
export function visibleCreationOrigins(creation: import('@/lib/api-client/story-packs').StoryPackCreation | undefined) {
  const origins = creation?.origins ?? []
  const visible = creation?.visible_origin_ids
  return visible?.length ? origins.filter(origin => origin.id === 'custom' || visible.includes(origin.id)) : origins
}
