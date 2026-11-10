import type { ActorStateField, ActorStateSchemaSnapshot, ActorTraitInstance, Snapshot, StateOp, TurnEvent } from '../../types'
import type { ClassifiedStateChange } from './changes'
import { mergeFieldChanges } from './changes'
import { playerFacingFieldLabelKey, playerFacingFieldName, resolveStateFieldLayout, type StateFieldRenderer } from './field-layout'
import {
  INJURY_DISPLAY_NAME,
  INJURY_EFFECTS_FIELD,
  INJURY_FLOOR_FIELD,
  INJURY_LABEL_KEY,
  lastingBenefits,
  stateInjuries,
  type StateInjuryReport,
} from './injuries'
import {
  isMethodRecordField,
  isMethodSpecialisationField,
  readMethods,
  type MethodReport,
} from './methods'
import {
  isSkillAbilityField,
  isSkillRecordField,
  readSkills,
  SKILL_ABILITY_FIELD,
  SKILL_ABILITY_PATH,
  SKILL_DISPLAY_NAME,
  SKILL_LABEL_KEY,
  SKILL_RECORD_FIELD,
  SKILL_RECORD_PATH,
  type SkillReport,
} from './skills'

export type ActorStateEntry = [string, Record<string, unknown>]

export interface ActorArchiveEntry {
  actorId: string
  name: string
  templateId: string
  reason: string
  sourceTurnId: string
}

export interface StoryStateChange {
  id: string
  actorId?: string
  path: string
  op: string
  value?: unknown
  /**
   * What the field held before this write, when the backend recorded it.
   *
   * A write carries an absolute value, so the difference the player is shown
   * cannot be computed from the op alone. Undefined means the field was empty
   * before, or the save predates the field.
   */
  previousValue?: unknown
  reason?: string
  /** The module that produced the write, when the backend named one. */
  sourceKind?: string
}

export interface StoryStateModel {
  actors: ActorStateEntry[]
  archivedActors: ActorArchiveEntry[]
  worldFacts: Array<[string, unknown]>
  changes: StoryStateChange[]
  hasState: boolean
}

export function buildStoryStateModel(snapshot: Snapshot | null): StoryStateModel {
  const stateFacts = snapshot
    ? Object.entries(snapshot.state).filter(([, value]) => value !== undefined && value !== null)
    : []
  const { actors, archivedActors, worldFacts } = splitStoryStateFacts(stateFacts)
  return {
    actors,
    archivedActors,
    worldFacts,
    changes: stateChanges(snapshot?.current_turn?.state_delta),
    hasState: actors.length > 0 || archivedActors.length > 0 || worldFacts.length > 0,
  }
}

export function splitStoryStateFacts(stateFacts: Array<[string, unknown]>) {
  const stateObjects = actorEntries(stateFacts)
  const archivedActors = actorArchiveEntries(stateFacts, stateObjects)
  const archivedIds = new Set(archivedActors.map((entry) => entry.actorId))
  const activeStateObjects = stateObjects.filter(([actorId]) => !archivedIds.has(actorId))
  const actors = activeStateObjects.filter(([actorId, actor]) => isActorLike(actorId, actor))
  const otherFacts = stateFacts.filter(([key]) => key !== 'actors' && key !== 'actor_archives')
  const worldFacts = ([
    ...otherFacts,
    ...activeStateObjects
      .filter(([actorId, actor]) => !isActorLike(actorId, actor))
      .map(([actorId, actor]): [string, unknown] => [actorName(actorId, actor), stateObjectValue(actor)]),
  ] satisfies Array<[string, unknown]>)
    .map(([key, value]): [string, unknown] => [key, compactStateValue(value)])
    .filter(([, value]) => value !== undefined)
  return { actors, archivedActors, worldFacts, stateObjects, otherFacts }
}

export function actorArchiveEntries(stateFacts: Array<[string, unknown]>, actors = actorEntries(stateFacts)): ActorArchiveEntry[] {
  const archives = stateFacts.find(([key]) => key === 'actor_archives')?.[1]
  if (!isRecord(archives)) return []
  const actorsById = new Map(actors)
  return Object.entries(archives)
    .map(([actorId, rawArchive]) => {
      const actor = actorsById.get(actorId) || {}
      const archive = isRecord(rawArchive) ? rawArchive : {}
      return {
        actorId,
        name: actorName(actorId, actor),
        templateId: stringValue(actor.template_id),
        reason: stringValue(archive.reason),
        sourceTurnId: stringValue(archive.source_turn_id),
      }
    })
    .sort((left, right) => left.name.localeCompare(right.name))
}

export function actorEntries(stateFacts: Array<[string, unknown]>): ActorStateEntry[] {
  const actors = stateFacts.find(([key]) => key === 'actors')?.[1]
  if (!isRecord(actors)) return []
  return Object.entries(actors)
    .filter((entry): entry is ActorStateEntry => isRecord(entry[1]))
    .sort(([leftId, left], [rightId, right]) => actorPriority(leftId, left) - actorPriority(rightId, right))
}

export function actorName(actorId: string, actor: Record<string, unknown>) {
  return stringValue(actor.name) || actorId
}

export function actorTemplate(actor: Record<string, unknown>, schema?: ActorStateSchemaSnapshot) {
  const templateId = stringValue(actor.template_id)
  return schema?.system.templates?.find((item) => item.id === templateId)
}

export function actorTraits(actor: Record<string, unknown>) {
  if (!Array.isArray(actor.traits)) return []
  return actor.traits.filter(isActorTrait)
}

export function actorFieldEntries(
  actor: Record<string, unknown>,
  schemaFields: ActorStateField[] | undefined,
): Array<{ field: ActorStateField; value: unknown; withheld: boolean }> {
  const state = isRecord(actor.state) ? actor.state : {}
  if (schemaFields !== undefined) {
    return schemaFields.map((field) => {
      const value = state[field.name] ?? (field.id ? state[field.id] : undefined) ?? (field.path ? state[field.path] : undefined)
      // A private field the pack declares and the actor does not carry is one
      // the player has not found out about, not one that is empty. The backend
      // already withheld it, so the absence is the only evidence left.
      const withheld = value === undefined && field.visibility === 'private'
      return { field, value, withheld }
    })
  }

  const directState = Object.fromEntries(Object.entries(actor).filter(([key]) => !['name', 'role', 'template_id', 'state', 'traits'].includes(key)))
  return Object.entries({ ...directState, ...state }).map(([name, value]) => ({
    field: { name: humanizeStateKey(name), type: inferredFieldType(value) } satisfies ActorStateField,
    value,
    withheld: false,
  }))
}

export function stateChanges(delta: TurnEvent['state_delta']): StoryStateChange[] {
  if (!delta) return []
  const actorChanges: StoryStateChange[] = (delta.actor_ops || []).map((op, index) => ({
    id: `actor:${op.actor_id}:${op.field_id}:${index}`,
    actorId: op.actor_id,
    path: op.field_id,
    op: op.op,
    value: op.value,
    previousValue: op.previous_value,
    reason: op.reason,
    sourceKind: op.source_kind,
  }))
  const sharedChanges: StoryStateChange[] = (delta.ops || []).map((op, index) => {
    const lifecycle = actorLifecycleChange(op)
    if (lifecycle) {
      return {
        id: `actor-lifecycle:${lifecycle.actorId}:${lifecycle.op}:${index}`,
        actorId: lifecycle.actorId,
        path: '',
        op: lifecycle.op,
        reason: lifecycle.reason,
        sourceKind: op.source_kind,
      }
    }
    return {
      id: `state:${op.path}:${index}`,
      path: op.path,
      op: op.op,
      value: op.value,
      reason: op.reason,
      sourceKind: op.source_kind,
    }
  })
  return [...actorChanges, ...sharedChanges]
}

export function statePathLabel(path: string) {
  const parts = path.split('.').filter(Boolean)
  const useful = parts[0] === 'actors' && parts.length > 3 ? parts.slice(3) : parts
  return useful.map(humanizeStateKey).join(' / ')
}

export function humanizeStateKey(value: string) {
  return value
    .replace(/[_-]+/g, ' ')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/^\w/, (letter) => letter.toUpperCase())
}

function actorPriority(actorId: string, actor: Record<string, unknown>) {
  return isLeadActor(actorId, actor) ? 0 : 1
}

export function isLeadActor(actorId: string, actor: Record<string, unknown>) {
  const candidates = [actorId, stringValue(actor.role), stringValue(actor.template_id)].map((value) => value.toLowerCase())
  return candidates.some((value) => value === 'protagonist' || value === 'lead' || value === 'player' || value === '主角')
}

function isActorLike(actorId: string, actor: Record<string, unknown>) {
  const role = stringValue(actor.role).toLowerCase()
  const templateId = stringValue(actor.template_id).toLowerCase()
  if (['story_context', 'world', 'scene', 'global', 'faction', 'base', 'instance', 'location', 'clock', 'quest'].some((marker) => role === marker || templateId === marker)) return false
  if (role) return true
  if (isLeadActor(actorId, actor)) return true
  const identity = `${actorId} ${templateId}`.toLowerCase()
  return ['important_character', 'opponent', 'supporting', 'character', 'npc', 'enemy', 'monster'].some((marker) => identity.includes(marker))
}

function stateObjectValue(actor: Record<string, unknown>) {
  if (isRecord(actor.state)) return actor.state
  return Object.fromEntries(Object.entries(actor).filter(([key]) => !['name', 'role', 'template_id', 'traits'].includes(key)))
}

function compactStateValue(value: unknown): unknown {
  if (value === undefined || value === null) return undefined
  if (typeof value === 'string') return value.trim() ? value : undefined
  if (Array.isArray(value)) {
    const items = value.map(compactStateValue).filter((item) => item !== undefined)
    return items.length > 0 ? items : undefined
  }
  if (isRecord(value)) {
    const entries = Object.entries(value)
      .map(([key, item]): [string, unknown] => [key, compactStateValue(item)])
      .filter(([, item]) => item !== undefined)
    return entries.length > 0 ? Object.fromEntries(entries) : undefined
  }
  return value
}

function inferredFieldType(value: unknown) {
  if (Array.isArray(value)) return 'list'
  if (value === null) return 'object'
  if (typeof value === 'boolean') return 'bool'
  return typeof value
}

function isActorTrait(value: unknown): value is ActorTraitInstance {
  return isRecord(value)
    && typeof value.pool_id === 'string'
    && typeof value.trait_id === 'string'
    && typeof value.name === 'string'
}

// --- Ledger field grouping -------------------------------------------------

export interface LedgerFieldEntry {
  id: string
  /**
   * The field's own name.
   *
   * Most rows carry the pack's field name here and that is what the player
   * reads. A row this interface invents — the folded wound row — carries a key
   * instead and puts its caption in `labelKey`, because the pack has no word
   * for it and a caption is the one part of a row that has to be translated.
   */
  label: string
  /** The i18n key for the row's caption, when the caption is not the field name. */
  labelKey?: string
  field?: ActorStateField
  value: unknown
  /**
   * True when the pack marks the field private and this actor's value was not
   * sent. It is the difference between "not found out yet" and "has no value",
   * so the field reads as 未知 rather than as an empty slot.
   */
  withheld?: boolean
  /**
   * Overrides the renderer the layout would otherwise resolve.
   *
   * The folded 伤势 row carries two settled fields at once, so its shape is not
   * one either field describes. Everything else leaves this unset and lets the
   * layout decide from the field and the value.
   */
  renderer?: StateFieldRenderer
  /**
   * Extra paths whose turn changes belong to this row.
   *
   * The 伤势 row is written by two fields — the prose and the burden floor — and
   * a change to either is a change to the wound. Without this the row would
   * report only the half whose name it kept.
   */
  changePaths?: string[]
  /** The folded wound report, on the 伤势 row only. */
  injuries?: StateInjuryReport
  /** The folded method report, on the 功法 row only. */
  methods?: MethodReport
  /** The folded skill report, on the 技能 row only. */
  skills?: SkillReport
}

export interface LedgerFieldItem extends LedgerFieldEntry {
  renderer: StateFieldRenderer
  change: ClassifiedStateChange | null
  /**
   * True when another field already shows this value as its maximum.
   *
   * The pools carry 生命 and 生命上限 as two settled numbers because the rules
   * need both. The page shows one "current / maximum" row, so the maximum's own
   * row is hidden while the pool resolves. The field stays in the layout and in
   * the journal either way.
   */
  hidden?: boolean
  /**
   * Resolves a sibling field's current value by name.
   *
   * A field may declare `max_field` instead of a fixed `max`, which is how the
   * cultivation pools express "current / maximum" when the maximum is itself a
   * settled number. The ledger supplies this so the meter can read that bound.
   */
  resolveSibling: (fieldName: string) => unknown
}

export interface LedgerFieldGroup {
  /** Builtin group key or the template-declared group name. */
  key: string
  custom: boolean
  labelKey?: string
  fields: LedgerFieldItem[]
}

/**
 * foldInjuryEntries turns the two fields that describe a wound into one row.
 *
 * 余伤负担下限 is the number the rules hold the burden above, and 持续状态 holds
 * the story's own account of the wound beside its blessings. Neither is
 * something to read on its own: the first has no meaning outside the recovery
 * rules, and the second mixes a wound in with a pill's bonus. The page shows
 * one 伤势 row carrying the prose, and leaves 持续状态 holding only what is not
 * a wound.
 *
 * The fields themselves are untouched. The story and the settlement keep
 * reading the whole list; this decides only what the page draws.
 */
export function foldInjuryEntries(entries: LedgerFieldEntry[]): LedgerFieldEntry[] {
  const floorIndex = entries.findIndex((entry) => entry.field?.name === INJURY_FLOOR_FIELD)
  const effectsIndex = entries.findIndex((entry) => entry.field?.name === INJURY_EFFECTS_FIELD)
  if (floorIndex < 0 && effectsIndex < 0) return entries

  const floor = floorIndex >= 0 ? entries[floorIndex] : undefined
  const effects = effectsIndex >= 0 ? entries[effectsIndex] : undefined
  const report = stateInjuries(floor?.value, effects?.value, Boolean(floor?.withheld || effects?.withheld))
  const folded: LedgerFieldEntry = {
    id: 'injuries',
    label: INJURY_DISPLAY_NAME,
    labelKey: INJURY_LABEL_KEY,
    field: floor?.field ?? effects?.field,
    value: report.lines.length > 0 ? report.lines : report.wounded ? true : false,
    withheld: report.withheld,
    renderer: 'injuries',
    injuries: report,
    changePaths: [INJURY_FLOOR_FIELD, INJURY_EFFECTS_FIELD],
  }

  const anchor = floorIndex >= 0 ? floorIndex : effectsIndex
  return entries.flatMap((entry, index) => {
    if (index === anchor) {
      // The wound row takes the floor's place in the sheet's own order, so a
      // pack that files it beside 生命 still shows it there.
      return index === effectsIndex
        ? [folded, { ...entry, value: lastingBenefits(entry.value) }]
        : [folded]
    }
    if (index === floorIndex) return []
    if (index === effectsIndex) return [{ ...entry, value: lastingBenefits(entry.value) }]
    return [entry]
  })
}

/**
 * foldMethodEntries turns the method record and the free-text specialisation
 * into the one 功法 row the design asks for.
 *
 * 功法 is the rules' record: what the character has mastered, which of it is
 * circulating, and the definition the settlement prices each from. 功法与专精 is
 * the prose a character was created with before that record existed. When the
 * record holds anything it is authoritative — the settlement prices from it and
 * ignores the prose — so showing both would let the two disagree with nothing
 * to say which one the rules used.
 *
 * A record with no entries is not an authoritative one. That character's
 * methods really do live in the free text, and the settlement reads them from
 * there, so the prose stays and the empty record simply draws nothing.
 *
 * The fields themselves are untouched; only what the page draws changes.
 */
export function foldMethodEntries(entries: LedgerFieldEntry[]): LedgerFieldEntry[] {
  const recordIndex = entries.findIndex((entry) => isMethodRecordField(entry.field))
  if (recordIndex < 0) return entries
  const record = entries[recordIndex]
  const report = readMethods(record.value, Boolean(record.withheld))
  if (report.structured) {
    const folded: LedgerFieldEntry = { ...record, renderer: 'methods', methods: report }
    return entries.flatMap((entry, index) => {
      if (index === recordIndex) return [folded]
      // The prose is what the record superseded. Keeping it beside the record
      // is the "two accounts of one thing" the design forbids.
      if (isMethodSpecialisationField(entry.field)) return []
      return [entry]
    })
  }
  // No readable record. A record the backend withheld is not evidence of an
  // empty one — what the player knows about this person's methods is whatever
  // the prose says, and a 未知 row beside it would claim a second answer.
  return entries.filter((_, index) => index !== recordIndex)
}

/**
 * foldSkillEntries turns the two records of what a character can do into the
 * one 技能 row §T20 asks for.
 *
 * 已学功法 is the rules' record — the techniques a cast is actually priced from.
 * 技能与能力 is the pack's free-form record for everything else: a trade, a
 * knack, a habit of the body. Both answer "what can this person do", so two
 * headings make the player check both and work out which one a line belongs
 * to, which is exactly what the design forbids.
 *
 * The two stay distinguishable: the folded row tags each line with the kind it
 * came from. A name both records hold appears once, taken from the technique
 * record, because that is the entry the rules price and the one the settlement
 * wrote a description for.
 *
 * Nothing here changes what either field means. 已学功法 keeps driving casts and
 * 技能与能力 never becomes a SkillDef; only the drawing is merged.
 */
export function foldSkillEntries(entries: LedgerFieldEntry[]): LedgerFieldEntry[] {
  const recordIndex = entries.findIndex((entry) => isSkillRecordField(entry.field))
  const abilityIndex = entries.findIndex((entry) => isSkillAbilityField(entry.field))
  const anchor = recordIndex >= 0 ? recordIndex : abilityIndex
  if (anchor < 0) return entries
  const record = recordIndex >= 0 ? entries[recordIndex] : undefined
  const abilities = abilityIndex >= 0 ? entries[abilityIndex] : undefined
  const report = readSkills(record?.value, abilities?.value, Boolean(record?.withheld || abilities?.withheld))
  // An empty row is drawn once too, so the section says "nothing yet" in one
  // place instead of leaving two blank slots under the same heading.
  const folded: LedgerFieldEntry = {
    ...(record ?? abilities)!,
    id: 'skills',
    label: SKILL_DISPLAY_NAME,
    labelKey: SKILL_LABEL_KEY,
    // The value decides whether the row survives hasReadableValue: an empty
    // report is a row that still has something to say, so it carries the
    // report itself rather than an empty object.
    value: report.lines.length > 0 ? report.lines : true,
    renderer: 'skills',
    skills: report,
    changePaths: [SKILL_RECORD_FIELD, SKILL_RECORD_PATH, SKILL_ABILITY_FIELD, SKILL_ABILITY_PATH],
  }
  return entries.flatMap((entry, index) => {
    if (index === anchor) return [folded]
    if (index === recordIndex || index === abilityIndex) return []
    return [entry]
  })
}

/**
 * Fields that are the game's own record, not something the protagonist found out.
 *
 * 可承接钩子 is the seam the next turn continues from — the thing the narrator
 * is about to reach for. 商贾 is the trade settlement's ledger: it holds each
 * shop's prepared cash position and buyback rate, written once by the
 * settlement and never back-filled, and T22 says that knowing an asking price
 * is not knowing the wallet. Both are read by the rules and the director, and
 * neither is an observed fact, so neither is drawn on the player's panel — and
 * the design's seven world groups have no slot for a shop's books anyway. The
 * change digest drops the same fields for the same reason; keeping the list in
 * one place is what stops a save from showing one of them in one surface and
 * hiding it in the other.
 */
const INTERNAL_FIELDS = new Set(['可承接钩子', 'scene.continuation_hook', '商贾', 'economy.merchants'])

function isInternalField(entry: LedgerFieldEntry) {
  const candidates = [
    entry.id,
    entry.label,
    ...(entry.field ? actorFieldPaths(entry.field) : []),
  ]
  return candidates.some((value) => typeof value === 'string' && INTERNAL_FIELDS.has(value.trim()))
}

/**
 * buildLedgerGroups resolves each field's renderer + group (template hints
 * first, shape heuristics as fallback) and attaches the classified turn
 * change. The schema's field array is the fallback order; any user ordering
 * is applied later as a UI-only preference.
 */
export function buildLedgerGroups(entries: LedgerFieldEntry[], changes: StoryStateChange[]): LedgerFieldGroup[] {
  const folded = foldSkillEntries(foldMethodEntries(foldInjuryEntries(entries.filter((entry) => !isInternalField(entry)))))
  const groups: LedgerFieldGroup[] = []
  const byKey = new Map<string, LedgerFieldGroup>()
  // The sibling lookup is built from the whole entry list, so a pool field can
  // read its maximum regardless of which group the maximum lands in.
  const valuesByName = new Map<string, unknown>()
  for (const entry of entries) {
    for (const path of entry.field ? actorFieldPaths(entry.field) : [entry.id, entry.label]) {
      if (!valuesByName.has(path)) valuesByName.set(path, entry.value)
    }
  }
  const resolveSibling = (fieldName: string) => valuesByName.get(fieldName)
  // The maximum of a pool is shown inside the pool's own row, so its separate
  // row would repeat the same number. Which field names a pool's maximum is the
  // pack's declaration (max_field), not a name this interface guesses.
  const shownAsMaximum = new Set(
    entries
      .map((entry) => entry.field?.max_field?.trim())
      .filter((name): name is string => Boolean(name)),
  )
  for (const entry of folded) {
    const layout = resolveStateFieldLayout(entry.field, entry.value)
    let group = byKey.get(layout.group)
    if (!group) {
      group = { key: layout.group, custom: layout.customGroup, ...(layout.groupLabelKey ? { labelKey: layout.groupLabelKey } : {}), fields: [] }
      byKey.set(layout.group, group)
      groups.push(group)
    }
    const fieldChanges = matchFieldChanges(changes, entry)
    group.fields.push({
      ...entry,
      renderer: entry.renderer ?? layout.renderer,
      change: mergeFieldChanges(fieldChanges, typeof entry.value === 'number'),
      resolveSibling,
      hidden: actorFieldPaths(entry.field ?? { name: entry.label, type: 'string' }).some((path) => shownAsMaximum.has(path)),
    })
  }
  return groups
    .map((group) => ({ ...group, fields: group.fields.filter((field) => !field.hidden && hasReadableValue(field)) }))
    .filter((group) => group.fields.length > 0)
    .sort((left, right) => (
      ledgerGroupPriority(left) - ledgerGroupPriority(right)
    ))
}

/**
 * hasReadableValue answers whether a field has anything to say.
 *
 * A fresh character has empty 关系 and 已学功法, and a pack declares plenty of
 * slots a given story never fills. Printing one "暂无内容" per empty slot turns
 * the state area into a page of placeholders, so an empty slot is simply not
 * drawn — and a group that is empty throughout loses its heading too. A
 * withheld field is not empty: 未知 is something the player has yet to find out.
 */
function hasReadableValue(item: LedgerFieldItem) {
  if (item.withheld) return true
  const value = item.value
  if (value === null || value === undefined) return false
  if (typeof value === 'string') return value.trim() !== ''
  if (Array.isArray(value)) return value.length > 0
  if (typeof value === 'object') return Object.keys(value as Record<string, unknown>).length > 0
  return true
}

const BUILTIN_GROUP_PRIORITY: Record<string, number> = {
  // The three world groups lead the world tab, where nothing else competes for
  // the first two slots. They are negative so they also stay ahead of the pack's
  // own groups (经济, 当前任务) on the one tab that carries both; an actor's tab
  // never produces them, so the actor ordering below is unchanged.
  world: -3,
  present: -2,
  situation: -1,
  overview: 0,
  holdings: 1,
  skills: 4,
  methods: 5,
  relations: 6,
  details: 7,
}

/**
 * Custom groups sit between holdings and the four player-facing groups.
 *
 * The four player-facing groups are ranked last because each is a display
 * regroup of fields the pack had filed elsewhere, and a pack that declares its
 * own group has said something about ordering this interface did not. Details
 * is last of all: it holds long prose, and prose after numbers is what the
 * preview is for.
 */
function ledgerGroupPriority(group: LedgerFieldGroup) {
  if (group.custom) return 2
  return BUILTIN_GROUP_PRIORITY[group.key] ?? 3
}

/** Splits the current ordered layout into its first two preview groups and the remainder. */
export function splitLedgerGroupsForPreview(groups: LedgerFieldGroup[]): { preview: LedgerFieldGroup[]; hidden: LedgerFieldGroup[] } {
  return { preview: groups.slice(0, 2), hidden: groups.slice(2) }
}

function matchFieldChanges(changes: StoryStateChange[], entry: LedgerFieldEntry) {
  const candidates = [
    ...(entry.field ? actorFieldPaths(entry.field) : [entry.id]),
    ...(entry.changePaths ?? []),
  ]
  return changes.filter((change) => candidates.some((path) => sameFieldPath(change.path, path) || change.path.startsWith(`${path}.`)))
}

export function actorFieldPaths(field: ActorStateField) {
  return [field.id, field.path, field.name]
    .filter((value): value is string => typeof value === 'string' && value.trim() !== '')
}

export function sameFieldPath(left: string, right: string) {
  const normalizedLeft = humanizeStateKey(left.trim()).toLocaleLowerCase()
  const normalizedRight = humanizeStateKey(right.trim()).toLocaleLowerCase()
  return normalizedLeft === normalizedRight
}

/**
 * changeFieldLabel resolves a change to its template field name so the
 * summary row shows localized names instead of raw field ids.
 */
export function changeFieldLabel(change: StoryStateChange, actors: ActorStateEntry[], schema?: ActorStateSchemaSnapshot): string {
  const field = changeField(change, actors, schema)
  if (change.actorId) {
    const actor = actors.find(([actorId]) => actorId === change.actorId)?.[1]
    if (change.op === 'archive' || change.op === 'restore') return actor ? actorName(change.actorId, actor) : change.actorId
    if (field) return playerFacingFieldName(field, field.name)
    return humanizeStateKey(change.path)
  }
  const segments = change.path.split('.').filter(Boolean)
  return humanizeStateKey(segments[segments.length - 1] || change.path)
}

/**
 * changeFieldLabelKey is the translation key for a change's field label, when
 * the label is one this interface renamed rather than a name the pack declares.
 */
export function changeFieldLabelKey(change: StoryStateChange, actors: ActorStateEntry[], schema?: ActorStateSchemaSnapshot): string | undefined {
  if (!change.actorId || change.op === 'archive' || change.op === 'restore') return undefined
  return playerFacingFieldLabelKey(changeField(change, actors, schema))
}

function changeField(change: StoryStateChange, actors: ActorStateEntry[], schema?: ActorStateSchemaSnapshot) {
  if (!change.actorId) return undefined
  const actor = actors.find(([actorId]) => actorId === change.actorId)?.[1]
  const template = actor ? actorTemplate(actor, schema) : undefined
  return template?.fields?.find((candidate) => actorFieldPaths(candidate).some((path) => sameFieldPath(path, change.path)))
}

function actorLifecycleChange(op: StateOp) {
  const prefix = 'actor_archives.'
  if (!op.path.startsWith(prefix)) return null
  const actorId = op.path.slice(prefix.length).trim()
  if (!actorId) return null
  const normalizedOp = op.op.trim().toLowerCase()
  if (normalizedOp !== 'set' && normalizedOp !== 'unset') return null
  const archive = isRecord(op.value) ? op.value : {}
  return {
    actorId,
    op: normalizedOp === 'set' ? 'archive' : 'restore',
    reason: stringValue(op.reason) || stringValue(archive.reason),
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function stringValue(value: unknown) {
  return typeof value === 'string' ? value : ''
}
