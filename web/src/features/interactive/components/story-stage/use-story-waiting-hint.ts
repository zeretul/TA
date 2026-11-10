import { useEffect, useState } from 'react'
import { useReducedMotionConfig } from 'motion/react'

/** Number of built-in `storyStage.waiting.*` lines, one per rotation step. */
export const STORY_WAITING_HINT_COUNT = 32
const ROTATION_MS = 5_000

/**
 * The waiting hint is light narration for the interface, not story prose. It
 * rotates on one timer per active run so streamed fragments never restart it.
 * Returns -1 when no run is active.
 */
export function useStoryWaitingHintIndex(active: boolean): number {
  const reducedMotion = useReducedMotionConfig()
  const [index, setIndex] = useState(-1)

  useEffect(() => {
    if (!active) {
      setIndex(-1)
      return undefined
    }
    setIndex(randomStoryWaitingHintIndex())
    // Reduced motion keeps one line instead of cycling: swapping text every few
    // seconds is itself movement.
    if (reducedMotion) return undefined
    const timer = window.setInterval(() => {
      setIndex((current) => nextStoryWaitingHintIndex(current))
    }, ROTATION_MS)
    return () => window.clearInterval(timer)
  }, [active, reducedMotion])

  return index
}

export function randomStoryWaitingHintIndex(random: () => number = Math.random) {
  return Math.floor(random() * STORY_WAITING_HINT_COUNT) % STORY_WAITING_HINT_COUNT
}

/** Never repeats the line already on screen. */
export function nextStoryWaitingHintIndex(current: number, random: () => number = Math.random) {
  const offset = 1 + Math.floor(random() * (STORY_WAITING_HINT_COUNT - 1))
  return ((current < 0 ? 0 : current) + offset) % STORY_WAITING_HINT_COUNT
}

export function storyWaitingHintKey(index: number) {
  return `storyStage.waiting.${index + 1}`
}
