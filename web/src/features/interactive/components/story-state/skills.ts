/**
 * The one list the page shows for a character's skills.
 *
 * A cultivation sheet describes what a character can do in two places. 已学功法
 * is the rules' record: the techniques the learning settlement has formally
 * granted, each with the definition a cast is priced from. 技能与能力 is the
 * free-form record the pack keeps for everything else a character can do —
 * a trade, a knack, a habit of the body — with no registered definition behind
 * it.
 *
 * §T20 asks for one 技能 list rather than two. They are both answers to "what can
 * this person do", so printing them under separate headings makes the player
 * check both and work out which one a given line belongs to. The design also
 * asks the two kinds to stay distinguishable and asks for one entry per ability,
 * so the merged list carries a kind tag and drops a name that both records hold:
 * the registered entry wins, because it is the one the rules actually price a
 * cast from and it carries the description the settlement wrote.
 *
 * The merge is display only. 已学功法 stays a rules field that the cast path
 * reads; 技能与能力 stays free text that never becomes a SkillDef. Nothing here
 * invents a definition for a written-down knack, and nothing here turns a
 * registered technique into prose.
 */

/** The rules-owned technique record. */
export const SKILL_RECORD_FIELD = '已学功法'
/** The path the same field is declared under. */
export const SKILL_RECORD_PATH = 'abilities.learned'
/** The free-form record of everything else a character can do. */
export const SKILL_ABILITY_FIELD = '技能与能力'
/** The path the free-form record is declared under. */
export const SKILL_ABILITY_PATH = 'abilities.records'

/**
 * The field key the folded row is known by.
 *
 * It is a name, not a caption: the row's id, the change lookup and the summary
 * ranking all key off it. What the player reads is SKILL_LABEL_KEY, because a
 * page that prints 技能 in the middle of an English sheet is only half
 * translated.
 */
export const SKILL_DISPLAY_NAME = '技能'
/** The i18n key for the caption the folded row is drawn under. */
export const SKILL_LABEL_KEY = 'storyStage.state.skill.label'

/**
 * How one line of the list is backed.
 *
 * `technique` is a registered definition — a cast from it is priced, refused or
 * performed by the rules. `ability` is a written-down knack with no definition,
 * so it reads as a capability and never claims a combat effect.
 */
export type SkillKind = 'technique' | 'ability' | 'learning' | 'craft' | 'unverified'

export interface SkillLine {
  name: string
  kind: SkillKind
  learningMinutes?: number
  /** What the technique looks like in use, or what the knack lets them do. */
  description: string
  /** Where it came from, when the story wrote it. */
  source: string
}

export interface SkillReport {
  /** True when at least one line survived the merge. */
  lines: SkillLine[]
  /** True when either source was withheld, so nothing may be claimed about it. */
  withheld: boolean
}

/** Which field a declaration names as the rules-owned technique record. */
export function isSkillRecordField(field: { name?: string; path?: string } | undefined): boolean {
  if (!field) return false
  const name = typeof field.name === 'string' ? field.name.trim() : ''
  const path = typeof field.path === 'string' ? field.path.trim() : ''
  return name === SKILL_RECORD_FIELD || path === SKILL_RECORD_PATH
}

/** Which field a declaration names as the free-form ability record. */
export function isSkillAbilityField(field: { name?: string; path?: string } | undefined): boolean {
  if (!field) return false
  const name = typeof field.name === 'string' ? field.name.trim() : ''
  const path = typeof field.path === 'string' ? field.path.trim() : ''
  return name === SKILL_ABILITY_FIELD || path === SKILL_ABILITY_PATH
}

/**
 * readSkills merges the two records into the one list the player reads.
 *
 * The registered techniques come first because they are what the character can
 * actually perform in a fight, and a reader looking for 火弹术 should not have
 * to scan past a list of trades to find it. An ability whose name the technique
 * record already holds is dropped: the rules' entry is the authoritative one,
 * and two rows for one ability is what §T20 forbids.
 *
 * A withheld record says nothing either way. It is not evidence that the
 * character has no skills, so the list reports the withholding rather than
 * drawing an empty list as a fact.
 */
export function readSkills(techniques: unknown, abilities: unknown, withheld = false): SkillReport {
  const registered = techniqueLines(techniques)
  const lines = [...registered, ...abilityLines(abilities, new Set(registered.map((line) => line.name)))]
  if (lines.length > 0) return { lines, withheld: false }
  return { lines: [], withheld }
}

function techniqueLines(value: unknown): SkillLine[] {
  if (!isRecord(value)) return []
  const lines: SkillLine[] = []
  for (const [key, entry] of Object.entries(value)) {
    const name = key.trim()
    if (!name) continue
    if (!isRecord(entry)) {
      const text = typeof entry === 'string' ? entry.trim() : ''
      lines.push({ name, kind: 'technique', description: text, source: '' })
      continue
    }
    lines.push({
      name: stringValue(entry['名称']) || name,
      kind: stringValue(entry['掌握程度']) === '修习中' ? 'learning' : entry['类别'] === 'craft' ? 'craft' : entry['类别'] === 'unverified' ? 'unverified' : 'technique',
      ...(typeof entry['已修习分钟'] === 'number' && Number.isFinite(entry['已修习分钟']) && entry['已修习分钟'] >= 0 ? { learningMinutes: entry['已修习分钟'] } : {}),
      description: stringValue(entry['描述']),
      source: stringValue(entry['来源']) || stringValue(entry['来历']),
    })
  }
  return lines
}

function abilityLines(value: unknown, registered: Set<string>): SkillLine[] {
  if (!isRecord(value)) return []
  const lines: SkillLine[] = []
  for (const [key, entry] of Object.entries(value)) {
    const name = key.trim()
    if (!name) continue
    // One ability, one row. The registered technique is the authoritative one
    // — it is what a cast is actually priced from — so a written-down entry
    // with the same name adds nothing the player can act on.
    if (registered.has(name)) continue
    if (!isRecord(entry)) {
      const text = typeof entry === 'string' ? entry.trim() : ''
      if (!text) continue
      lines.push({ name, kind: 'ability', description: text, source: '' })
      continue
    }
    // A free-form record spells its own keys: the pack's instruction asks for
    // 类型, 效果, 代价与限制 and 来源. Anything it did not write stays empty rather
    // than being borrowed from a field that happens to be nearby.
    const description = [stringValue(entry['类型']), stringValue(entry['效果'])]
      .filter(Boolean)
      .join('，')
    lines.push({
      name,
      kind: 'ability',
      description,
      source: stringValue(entry['来源']) || stringValue(entry['来历']),
    })
  }
  return lines
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}
