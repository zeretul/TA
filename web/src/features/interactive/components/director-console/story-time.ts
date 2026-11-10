import type { TurnEvent } from '../../types'

type Translate = (key: string, options?: Record<string, unknown>) => string

const DAY = 24 * 60 * 60
const HOUR = 60 * 60
const QUARTER = 15 * 60

/**
 * Renders an elapsed interval in the units the story's clock counts in.
 *
 * It mirrors the backend's `DescribeElapsed` on purpose: the same interval must
 * not read as 三日 here and 72 时辰 in the turn context. Both are deliberately
 * coarse — the world pack binds no calendar, so a precise date would be invented.
 */
export function formatStoryElapsed(seconds: number, t: Translate): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return ''
  if (seconds % DAY === 0) return t('directorPanel.history.elapsedDays', { count: seconds / DAY })
  if (seconds % HOUR === 0) return t('directorPanel.history.elapsedHours', { count: (seconds / HOUR) * 2 })
  if (seconds % QUARTER === 0) return t('directorPanel.history.elapsedQuarters', { count: seconds / QUARTER })
  return t('directorPanel.history.elapsedBreaths', { count: seconds })
}

/**
 * The formal instant each played turn ended at, keyed by turn id.
 *
 * A turn only moves the clock when an explicit wait was committed, so an
 * untouched turn keeps the instant before it. Only turns that actually moved it
 * get an entry: repeating the same time down the list would look like the story
 * advanced when it did not.
 */
export function storyInstantsByTurn(turns: TurnEvent[]): Map<string, number> {
  const instants = new Map<string, number>()
  let current = 0
  for (const turn of turns) {
    const after = turn.clock_advance?.after?.seconds
    if (typeof after === 'number' && after > current) {
      current = after
      instants.set(turn.id, after)
    }
  }
  return instants
}
