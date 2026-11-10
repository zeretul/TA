import { describe, expect, it } from 'vitest'
import type { TurnEvent } from '../../types'
import { formatStoryElapsed, storyInstantsByTurn } from './story-time'

const t = (key: string, options?: Record<string, unknown>) => `${key}:${String(options?.count ?? '')}`

function turn(id: string, afterSeconds?: number): TurnEvent {
  return {
    id,
    parent_id: null,
    branch_id: 'main',
    ts: '2026-06-28T00:00:00Z',
    user: '等候',
    narrative: '时间过去了。',
    clock_advance: afterSeconds === undefined ? undefined : {
      action: 'wait',
      source_text: '等候',
      duration_seconds: 3600,
      before: { version: 1, seconds: 0 },
      after: { version: 1, seconds: afterSeconds },
    },
  }
}

describe('formatStoryElapsed', () => {
  it('states an interval in the units the story clock counts in', () => {
    expect(formatStoryElapsed(2 * 24 * 60 * 60, t)).toBe('directorPanel.history.elapsedDays:2')
    expect(formatStoryElapsed(2 * 60 * 60, t)).toBe('directorPanel.history.elapsedHours:4')
    expect(formatStoryElapsed(30 * 60, t)).toBe('directorPanel.history.elapsedQuarters:2')
    expect(formatStoryElapsed(90, t)).toBe('directorPanel.history.elapsedBreaths:90')
  })

  it('says nothing for a turn that spent no time', () => {
    expect(formatStoryElapsed(0, t)).toBe('')
    expect(formatStoryElapsed(-60, t)).toBe('')
    expect(formatStoryElapsed(Number.NaN, t)).toBe('')
  })
})

describe('storyInstantsByTurn', () => {
  it('dates only the turns that actually moved the clock', () => {
    const instants = storyInstantsByTurn([
      turn('t1'),
      turn('t2', 3600),
      turn('t3'),
      turn('t4', 7200),
    ])

    expect(Array.from(instants.entries())).toEqual([['t2', 3600], ['t4', 7200]])
  })

  it('never walks the clock backwards when a turn replays an earlier instant', () => {
    const instants = storyInstantsByTurn([turn('t1', 7200), turn('t2', 3600)])

    expect(Array.from(instants.keys())).toEqual(['t1'])
  })
})
