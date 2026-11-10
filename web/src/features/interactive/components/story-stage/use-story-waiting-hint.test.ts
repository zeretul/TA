import { describe, expect, it } from 'vitest'
import {
  STORY_WAITING_HINT_COUNT,
  nextStoryWaitingHintIndex,
  randomStoryWaitingHintIndex,
  storyWaitingHintKey,
} from './use-story-waiting-hint'

describe('story waiting hint', () => {
  it('names a key inside the rotation for every index it can return', () => {
    for (let index = 0; index < STORY_WAITING_HINT_COUNT; index++) {
      expect(storyWaitingHintKey(index)).toBe(`storyStage.waiting.${index + 1}`)
    }
  })

  it('never starts past the last line, even when the source returns 1', () => {
    // Math.random is documented as [0, 1), but a source that returns exactly 1
    // would index one past the end and show an empty hint.
    expect(randomStoryWaitingHintIndex(() => 1)).toBe(0)
    expect(randomStoryWaitingHintIndex(() => 0)).toBe(0)
    expect(randomStoryWaitingHintIndex(() => 0.9999999)).toBe(STORY_WAITING_HINT_COUNT - 1)
  })

  it('never repeats the line already on screen', () => {
    for (let current = 0; current < STORY_WAITING_HINT_COUNT; current++) {
      for (const draw of [0, 0.25, 0.5, 0.75, 0.9999999]) {
        expect(nextStoryWaitingHintIndex(current, () => draw)).not.toBe(current)
      }
    }
  })

  it('treats the resting -1 as the first line when it has to advance', () => {
    // The hook holds -1 while no run is active. Advancing from it must land on a
    // real line rather than on -1 or a negative index.
    for (const draw of [0, 0.5, 0.9999999]) {
      const next = nextStoryWaitingHintIndex(-1, () => draw)
      expect(next).toBeGreaterThanOrEqual(0)
      expect(next).toBeLessThan(STORY_WAITING_HINT_COUNT)
    }
  })

  it('stays inside the rotation for every start and draw', () => {
    for (let current = -1; current < STORY_WAITING_HINT_COUNT; current++) {
      for (const draw of [0, 0.1, 0.5, 0.9, 0.9999999]) {
        const next = nextStoryWaitingHintIndex(current, () => draw)
        expect(Number.isInteger(next)).toBe(true)
        expect(next).toBeGreaterThanOrEqual(0)
        expect(next).toBeLessThan(STORY_WAITING_HINT_COUNT)
      }
    }
  })
})
