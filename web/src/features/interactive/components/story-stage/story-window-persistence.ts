/**
 * Where a story window was last put, kept as a local interface preference.
 *
 * A window's size and place are the player's, not the story's: reopening the
 * app must return the conversation where they left it, and none of this belongs
 * in the story record, the journal, or anything the model reads. It is written
 * to local storage beside the other panel widths for the same reason.
 *
 * Storage can be unavailable (a private window, a disabled quota) and can hold
 * something a previous version wrote. Neither is worth failing a window over, so
 * both cases read as "nothing remembered" and the window opens at its default.
 */
import type { StoryWindowGeometry } from './story-window'

/** A remembered geometry, plus whether the window was floating when it was put there. */
export interface RememberedStoryWindowGeometry extends StoryWindowGeometry {
  floating: boolean
}

const STORAGE_PREFIX = 'ta.story-window.geometry.'

function storageKey(windowKey: string): string {
  return `${STORAGE_PREFIX}${windowKey}`
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

/**
 * The geometry last remembered for this kind of window, or null.
 *
 * A value that is not four finite numbers is discarded rather than repaired:
 * half a geometry would put the window somewhere the player never chose, and
 * the default is both predictable and reachable.
 */
export function loadStoryWindowGeometry(windowKey: string): RememberedStoryWindowGeometry | null {
  if (!windowKey) return null
  try {
    const raw = window.localStorage.getItem(storageKey(windowKey))
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return null
    const record = parsed as Record<string, unknown>
    const { x, y, width, height } = record
    if (!isFiniteNumber(x) || !isFiniteNumber(y) || !isFiniteNumber(width) || !isFiniteNumber(height)) return null
    return { x, y, width, height, floating: record.floating === true }
  } catch {
    return null
  }
}

/** Remembers a geometry, or forgets one when the window is docked again. */
export function saveStoryWindowGeometry(
  windowKey: string,
  geometry: StoryWindowGeometry | null,
  floating: boolean,
): void {
  if (!windowKey) return
  try {
    if (!geometry) {
      window.localStorage.removeItem(storageKey(windowKey))
      return
    }
    const remembered: RememberedStoryWindowGeometry = {
      x: Math.round(geometry.x),
      y: Math.round(geometry.y),
      width: Math.round(geometry.width),
      height: Math.round(geometry.height),
      floating,
    }
    window.localStorage.setItem(storageKey(windowKey), JSON.stringify(remembered))
  } catch {
    // A window that cannot remember its place is still a working window.
  }
}
