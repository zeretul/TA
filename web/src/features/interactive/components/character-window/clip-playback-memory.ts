/**
 * What the player has already been shown once.
 *
 * A clip is meant to play by itself exactly once per person per scene: the
 * moment the player opens that person's portrait in a scene they have not seen
 * them in yet. Everything else — a new line of dialogue, a redraw, scrolling,
 * closing and reopening the window — must stay quiet, and so must a page reload
 * or a cold restart of the app. That last part is why the memory outlives the
 * module: a reload throws away every in-page structure, and without something
 * on disk the clip would greet the player again as if it were the first time.
 *
 * It is a set of signatures and nothing else. No position, no timestamp, no
 * count, no schedule: nothing here can resume a clip, and nothing reads it to
 * decide anything except "has this exact sighting already happened".
 */
const STORAGE_KEY = 'ta.clip.autoplay.v1'

/** Enough for a long story, small enough to stay a few kilobytes. */
const LIMIT = 96

let memory: Set<string> | null = null

function load(): Set<string> {
  if (memory) return memory
  const loaded = new Set<string>()
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    const parsed = raw ? (JSON.parse(raw) as unknown) : null
    if (Array.isArray(parsed)) {
      for (const item of parsed) {
        if (typeof item === 'string' && item) loaded.add(item)
      }
    }
  } catch {
    // Private mode, a disabled store, or a corrupted entry: the memory starts
    // empty for this page. The cost is one extra autoplay, never a broken clip.
  }
  memory = loaded
  return loaded
}

/** Whether this exact person-in-this-scene sighting has already played itself. */
export function clipAutoPlayed(signature: string): boolean {
  return load().has(signature)
}

/**
 * Record a sighting. The oldest signatures fall off, so a story that runs for
 * hundreds of scenes does not leave an unbounded entry behind.
 */
export function rememberClipAutoPlay(signature: string): void {
  const current = load()
  if (current.has(signature)) return
  const next = [...current, signature]
  const kept = next.slice(Math.max(0, next.length - LIMIT))
  memory = new Set(kept)
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(kept))
  } catch {
    // The page keeps its own copy; only the reload protection is lost.
  }
}

/** Test seam: forget everything this page remembers. */
export function forgetClipAutoPlay(): void {
  memory = new Set()
  try {
    window.localStorage.removeItem(STORAGE_KEY)
  } catch {
    // Nothing to clear.
  }
}

/**
 * The clips currently playing, so a second one can silence the first. Only one
 * picture moves at a time: two people in a group, or two windows, would
 * otherwise run side by side and read as a single confused scene.
 */
const playing = new Set<() => void>()

export function claimClipPlayback(pause: () => void): void {
  for (const other of playing) {
    if (other !== pause) other()
  }
  playing.add(pause)
}

export function releaseClipPlayback(pause: () => void): void {
  playing.delete(pause)
}
