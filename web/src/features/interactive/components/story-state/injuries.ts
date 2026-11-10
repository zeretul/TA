/**
 * The one row the page shows for a character's wounds.
 *
 * A cultivation sheet records a wound twice: 持续状态 carries what the story
 * says about it — the name, what it stops the character doing, how it heals —
 * and 余伤负担下限 carries the number the rules hold the burden above. Showing
 * both leaves the player reading the same wound twice, once as prose and once
 * as a parameter that means nothing outside the recovery rules.
 *
 * So the page folds them into one 伤势 row and leaves 持续状态 holding only what
 * is not a wound. The split is the pack's own: its instruction for 持续状态 says
 * an effect that participates in attribute settlement is written in object form
 * with 属性 and 数值, and everything else is a "名称｜影响｜解除条件" line. An
 * object with a positive 数值 is therefore a pill, a talisman or a blessing; a
 * negative one and every text entry are what the character is suffering from.
 *
 * Nothing here invents a wound. A row with no prose and a zero floor is 无伤,
 * and a row whose fields the backend withheld is 未知 — an opponent whose sheet
 * the player has not seen is not an uninjured opponent.
 */

/** The settled field a wound holds the burden above. */
export const INJURY_FLOOR_FIELD = '余伤负担下限'
/** The field the story writes its lasting effects into. */
export const INJURY_EFFECTS_FIELD = '持续状态'
/**
 * The field key the folded row is known by.
 *
 * It is a name, not a caption: the row's id, the change lookup and the summary
 * ranking all key off it. What the player reads is INJURY_LABEL_KEY, because a
 * page that prints 伤势 in the middle of an English sheet is only half translated.
 */
export const INJURY_DISPLAY_NAME = '伤势'
/** The i18n key for the caption the folded row is drawn under. */
export const INJURY_LABEL_KEY = 'storyStage.state.injury.label'

/**
 * The names and ids the built-in cultivation pack gives the two wound fields.
 *
 * The template lookup is the real answer — a pack may rename either field — but
 * a change carries a field id and not always the template that declares it, so
 * the ids the pack is known to use stand in when there is nothing to look up.
 */
const INJURY_FLOOR_KEYS = new Set([INJURY_FLOOR_FIELD, 'state.burden_floor', 'burden_floor'])
const INJURY_EFFECTS_KEYS = new Set([INJURY_EFFECTS_FIELD, 'state.effects'])

export type InjuryFieldKind = 'floor' | 'effects'

/** Which of the two wound fields a declaration names, if either. */
export function injuryFieldKind(field: { id?: string; path?: string; name?: string } | undefined): InjuryFieldKind | null {
  if (!field) return null
  const keys = [field.name, field.id, field.path]
    .map((value) => (typeof value === 'string' ? value.trim() : ''))
    .filter(Boolean)
  if (keys.some((key) => INJURY_FLOOR_KEYS.has(key))) return 'floor'
  if (keys.some((key) => INJURY_EFFECTS_KEYS.has(key))) return 'effects'
  return null
}

export interface StateInjuryReport {
  /** True when a wound is really recorded, by prose or by the burden floor. */
  wounded: boolean
  /**
   * The wounds in the story's own words: the name, what it affects, and the
   * recovery condition the pack's instruction asks the story to write.
   */
  lines: string[]
  /** True when nothing was readable and a declared field was withheld. */
  withheld: boolean
}

/**
 * stateInjuries reads one character's wounds out of the two settled fields.
 *
 * `fieldsWithheld` is the backend's own answer that this actor's sheet was not
 * sent to the player. It only decides the answer when there is no prose to
 * read: a wound the story has already described is still a wound even if the
 * burden number behind it stayed private.
 */
export function stateInjuries(floorValue: unknown, effectsValue: unknown, fieldsWithheld: boolean): StateInjuryReport {
  const lines = injuryLines(effectsValue)
  if (lines.length > 0) return { wounded: true, lines, withheld: false }
  if (fieldsWithheld) return { wounded: false, lines: [], withheld: true }
  const floor = typeof floorValue === 'number' && Number.isFinite(floorValue) ? floorValue : 0
  return { wounded: floor > 0, lines: [], withheld: false }
}

/**
 * lastingBenefits is what 持续状态 keeps after the wounds move to 伤势.
 *
 * The field is not rewritten — the story and the rules still read the whole
 * list. This only decides which of its entries the lasting-effects row still
 * has to draw, so a blessing stays where the player expects it and a wound is
 * not printed in two places.
 */
export function lastingBenefits(value: unknown): unknown {
  if (!Array.isArray(value)) return value
  return value.filter((item) => !isHarm(item))
}

/**
 * injuryNames is what a change summary can say about a wound list.
 *
 * The summary is one line, so it names the wound rather than quoting the whole
 * "名称｜影响｜恢复条件" line. A text entry the story wrote without the pack's
 * separators is its own name.
 */
export function injuryNames(value: unknown): string[] {
  return harmEntries(value).map((entry) => entry.name)
}

/**
 * effectNames names what a 持续状态 list carries, for a one-line summary.
 *
 * A wound and a blessing are both written here, so the summary has to be able
 * to say that a pill was taken without repeating the wound beside it.
 */
export function effectNames(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  const names: string[] = []
  for (const item of value) {
    if (typeof item === 'string') {
      const text = item.trim()
      const name = text.split(/[｜|]/)[0].trim()
      if (name) names.push(name)
      continue
    }
    if (!isRecord(item)) continue
    const name = stringValue(item['名称'])
    if (name) names.push(name)
  }
  return names
}

/**
 * benefitNames is effectNames with the wounds taken out.
 *
 * The wound half of 持续状态 is reported as 伤势, so a summary that also listed
 * it here would say the same thing twice — and an emptied list would read as
 * "失去经脉内伤", which claims a recovery the burden floor may contradict.
 */
export function benefitNames(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return effectNames(value.filter((item) => !isHarm(item)))
}

/** The lines a player reads, in the order the story wrote them. */
export function injuryLines(value: unknown): string[] {
  return harmEntries(value).map((entry) => entry.line)
}

function harmEntries(value: unknown): Array<{ name: string; line: string }> {
  if (!Array.isArray(value)) return []
  const result: Array<{ name: string; line: string }> = []
  for (const item of value) {
    if (!isHarm(item)) continue
    if (typeof item === 'string') {
      const text = item.trim()
      const name = text.split(/[｜|]/)[0].trim() || text
      result.push({ name, line: text })
      continue
    }
    const record = item as Record<string, unknown>
    const name = stringValue(record['名称'])
    const amount = numericValue(record['数值'])
    if (amount !== null) {
      const attribute = stringValue(record['属性'])
      result.push({
        name: name || attribute,
        line: attribute ? `${name}｜${attribute} ${amount}%` : `${name}｜${amount}%`,
      })
      continue
    }
    const effect = stringValue(record['影响'])
    result.push({
      name: name || effect,
      line: name && effect ? `${name}｜${effect}` : name || effect,
    })
  }
  return result
}

/** A text entry, or an object entry whose 数值 is negative. */
function isHarm(item: unknown): boolean {
  if (typeof item === 'string') return item.trim() !== ''
  if (!isRecord(item)) return false
  const amount = numericValue(item['数值'])
  if (amount !== null) return amount < 0
  return stringValue(item['名称']) !== '' || stringValue(item['影响']) !== ''
}

function numericValue(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}
