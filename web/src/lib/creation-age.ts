/**
 * The creation page shows a real age the moment it opens and lets the player
 * type over it. Both helpers live here rather than in the dialog so the range
 * rules can be checked without mounting it.
 */

/**
 * A pack may declare its range in either order or not at all; the page still
 * needs a usable pair, so a missing range falls back to a plain adult default.
 */
export function normalizeAgeRange(range: [number, number] | undefined): [number, number] {
  const [first, second] = range ?? [20, 28]
  const low = Math.min(first, second)
  const high = Math.max(first, second)
  if (!Number.isFinite(low) || !Number.isFinite(high) || low <= 0) return [20, 28]
  return [low, high]
}

/**
 * A fresh number from the range, never the one already showing: pressing the
 * reroll button has to produce a visibly different age, and with a range as
 * narrow as 20–28 a plain random roll would repeat far too often.
 */
export function rollAge([low, high]: [number, number], current?: number): number {
  const span = high - low + 1
  if (span <= 1) return low
  for (let attempt = 0; attempt < 12; attempt += 1) {
    const next = low + Math.floor(Math.random() * span)
    if (next !== current) return next
  }
  return current === low ? low + 1 : low
}

export function isAgeInRange(value: string, [low, high]: [number, number]): boolean {
  const age = Number.parseInt(value, 10)
  return Number.isInteger(age) && String(age) === value.trim() && age >= low && age <= high
}
