import type { StoryStateChange } from './model'

export type StateChangeTone = 'positive' | 'negative' | 'neutral'

export type StateChangeKind = 'delta' | 'added' | 'removed' | 'updated' | 'cleared' | 'archived' | 'restored'

/**
 * ClassifiedStateChange is the presentation-ready form of one raw state op.
 * Components localize labels from kind; delta carries signed numeric changes.
 */
export interface ClassifiedStateChange {
  kind: StateChangeKind
  /** Signed numeric delta for inc/decrement ops on numeric fields. */
  delta: number | null
  /** Short preview of an added/removed value, when printable. */
  text: string
  reason?: string
  tone: StateChangeTone
}

/** classifyStateChange folds a raw state op into its presentation shape. */
export function classifyStateChange(change: StoryStateChange, numeric: boolean): ClassifiedStateChange {
  const op = change.op.trim().toLowerCase()
  const delta = numericChangeDelta(change, numeric)
  if (delta !== null) {
    return { kind: 'delta', delta, text: '', reason: change.reason, tone: delta > 0 ? 'positive' : delta < 0 ? 'negative' : 'neutral' }
  }
  const text = printableChangeValue(change.value)
  if (op === 'archive') {
    return { kind: 'archived', delta: null, text: '', reason: change.reason, tone: 'negative' }
  }
  if (op === 'restore') {
    return { kind: 'restored', delta: null, text: '', reason: change.reason, tone: 'positive' }
  }
  if (['push', 'append', 'add'].includes(op)) {
    return { kind: 'added', delta: null, text, reason: change.reason, tone: 'positive' }
  }
  if (['pull', 'remove', 'delete'].includes(op)) {
    return { kind: 'removed', delta: null, text, reason: change.reason, tone: 'negative' }
  }
  if (op === 'unset') {
    return { kind: 'cleared', delta: null, text: '', reason: change.reason, tone: 'neutral' }
  }
  // A list emptied by a plain set is the same news as an unset: the wound list
  // went away. Reading that as "已更新" hides a recovery behind a restatement.
  if (op === 'set' && text === '' && !isEmptyStoredValue(change.previousValue) && isEmptyStoredValue(change.value)) {
    return { kind: 'cleared', delta: null, text: '', reason: change.reason, tone: 'neutral' }
  }
  // A plain set of a readable value carries that value. Without it every text
  // field in the turn reads "已更新" and says nothing about what it became.
  return { kind: 'updated', delta: null, text, reason: change.reason, tone: 'neutral' }
}

/** mergeFieldChanges picks the most informative classification for one field. */
export function mergeFieldChanges(changes: StoryStateChange[], numeric: boolean): ClassifiedStateChange | null {
  if (changes.length === 0) return null
  const classified = changes.map((change) => classifyStateChange(change, numeric))
  return classified.find((change) => change.kind === 'delta')
    || classified.find((change) => change.kind === 'added' || change.kind === 'removed')
    || classified[0]
}

function numericChangeDelta(change: StoryStateChange, numeric: boolean) {
  const op = change.op.trim().toLowerCase()
  if (!numeric || typeof change.value !== 'number') return null
  if (op === 'inc' || op === 'increment') return change.value
  if (op === 'decrement') return -Math.abs(change.value)
  // A settled balance is written as an absolute value, so 灵石=148 on its own
  // cannot say whether the turn spent two stones or none. The backend records
  // what the field held before the write; only the two together are a change.
  // A missing previous value means the field was empty before (or the save
  // predates the record), and no difference can be claimed from that.
  if (op === 'set' && typeof change.previousValue === 'number' && change.previousValue !== change.value) {
    return change.value - change.previousValue
  }
  return null
}

function printableChangeValue(value: unknown): string {
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (Array.isArray(value) && value.every((item) => item === null || ['string', 'number', 'boolean'].includes(typeof item))) {
    return value.map((item) => (item === null ? '' : String(item))).filter(Boolean).join('、')
  }
  return ''
}

/**
 * isEmptyStoredValue answers whether a stored value says "nothing here".
 *
 * A character walking on stage has their whole sheet written at once, and most
 * of that sheet is zeroes and empty lists. A zero is a value, but it is not a
 * change: it is the absence of one, and reporting it is what turns a turn into a
 * page of "已更新".
 */
export function isEmptyStoredValue(value: unknown): boolean {
  if (value === null || value === undefined) return true
  if (typeof value === 'string') return value.trim() === ''
  if (typeof value === 'number') return value === 0
  if (typeof value === 'boolean') return false
  if (Array.isArray(value)) return value.length === 0
  if (typeof value === 'object') return Object.keys(value as Record<string, unknown>).length === 0
  return false
}
