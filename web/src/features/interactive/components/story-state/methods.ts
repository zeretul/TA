/**
 * The row the page shows for a character's cultivation methods.
 *
 * A cultivation sheet can describe the same thing twice. 功法 is the rules'
 * own record — one entry per method, with the definition the settlement prices
 * it from, whether it has been mastered, and whether it is circulating. 功法与专精
 * is free text the character was created with, written before that record
 * existed ("燕家家传功法，强攻，良好").
 *
 * The design asks for one of them: the structured record when there is one, the
 * prose only as the compatibility path for a save written before it. Printing
 * both lets the two disagree — the prose says 强攻 while the record circulates a
 * defensive method — and the player has no way to tell which one the rules use.
 * The settlement already resolves it the same way: a record with entries wins,
 * and the free text is priced only when the record is empty.
 *
 * What the player reads out of the record is the part that is actually a
 * decision: which methods are being cultivated right now. 运转 together with
 * 已掌握 is what circulatingTechniques feeds the settlement, so that is 主修;
 * a mastered method left stopped is 兼修; anything still being learned says so
 * rather than being filed under either.
 *
 * Nothing here writes. The record keeps its shape and the settlement keeps
 * reading it; this decides only what the page draws.
 */

/** The rules-owned method record. */
export const METHOD_RECORD_FIELD = '功法'
/** The path the same field is declared under. */
export const METHOD_RECORD_PATH = 'cultivation.techniques'
/** The free-text field the record supersedes. */
export const METHOD_SPECIALISATION_FIELD = '功法与专精'

/** How a character is holding one method, in the words the record already uses. */
export type MethodKind = 'main' | 'secondary' | 'learning'

export interface MethodLine {
  name: string
  kind: MethodKind
  learningMinutes?: number
  /** What the method feels like in use, when the story wrote it. */
  description: string
  /** Where it came from, when the story wrote it. */
  source: string
}

export interface MethodReport {
  /** True when the structured record holds at least one readable method. */
  structured: boolean
  lines: MethodLine[]
  /** True when the record was withheld, so nothing may be claimed about it. */
  withheld: boolean
}

/** Which of the two fields a declaration names, if either. */
export function isMethodRecordField(field: { name?: string; path?: string } | undefined): boolean {
  if (!field) return false
  const name = typeof field.name === 'string' ? field.name.trim() : ''
  const path = typeof field.path === 'string' ? field.path.trim() : ''
  return name === METHOD_RECORD_FIELD || path === METHOD_RECORD_PATH
}

/** Which of the two fields a declaration names as the superseded prose. */
export function isMethodSpecialisationField(field: { name?: string; path?: string } | undefined): boolean {
  if (!field) return false
  const name = typeof field.name === 'string' ? field.name.trim() : ''
  const path = typeof field.path === 'string' ? field.path.trim() : ''
  return name === METHOD_SPECIALISATION_FIELD || path === 'state.cultivation_method'
}

/**
 * readMethods reads the record into the lines a player sees.
 *
 * A record that holds no readable entry is not a character with no methods: it
 * is a character whose methods live in the free text, and the caller keeps
 * showing that instead. Only an entry with a name says anything here.
 */
export function readMethods(value: unknown, withheld = false): MethodReport {
  const lines = methodLines(value)
  if (lines.length > 0) return { structured: true, lines, withheld: false }
  if (withheld) return { structured: false, lines: [], withheld: true }
  return { structured: false, lines: [], withheld: false }
}

function methodLines(value: unknown): MethodLine[] {
  if (!isRecord(value)) return []
  const lines: MethodLine[] = []
  for (const [key, entry] of Object.entries(value)) {
    const name = key.trim()
    if (!name) continue
    if (!isRecord(entry)) {
      // A record the story wrote as bare prose is still a method it named.
      const text = typeof entry === 'string' ? entry.trim() : ''
      lines.push({ name, kind: 'learning', description: text, source: '' })
      continue
    }
    lines.push({
      name: stringValue(entry['名称']) || name,
      kind: methodKind(entry),
      ...(typeof entry['已修习分钟'] === 'number' && Number.isFinite(entry['已修习分钟']) && entry['已修习分钟'] >= 0 ? { learningMinutes: entry['已修习分钟'] } : {}),
      description: stringValue(entry['描述']),
      source: stringValue(entry['来源']) || stringValue(entry['来历']),
    })
  }
  return lines
}

function methodKind(entry: Record<string, unknown>): MethodKind {
  const mastered = stringValue(entry['掌握程度']) === '已掌握'
  if (!mastered) return 'learning'
  return stringValue(entry['运转']) === '运转' ? 'main' : 'secondary'
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}
