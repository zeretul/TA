/**
 * Whether a character clip may start on its own.
 *
 * The answer lives on the document element rather than being threaded through
 * every surface that shows a portrait: a clip appears in the character window,
 * in the intimacy surface and in their tests, and only some of those sit under
 * the app's providers. One attribute read from the DOM keeps a single answer for
 * all of them, and it is the same mechanism the motion preference already uses.
 *
 * It governs only the unasked-for start. A clip the player presses play on is
 * their own request and plays whatever this says.
 */
const ATTRIBUTE = 'clipsEnabled'

export function applyClipPlaybackPreference(enabled?: boolean | null): void {
  if (typeof document === 'undefined') return
  document.documentElement.dataset[ATTRIBUTE] = enabled === false ? 'off' : 'on'
}

/** Absent means allowed: a page that has not read settings yet behaves as before. */
export function clipsAllowed(): boolean {
  if (typeof document === 'undefined') return true
  return document.documentElement.dataset[ATTRIBUTE] !== 'off'
}

/** The attribute a reader of this preference has to watch. */
export const CLIP_PLAYBACK_ATTRIBUTE = 'data-clips-enabled'
