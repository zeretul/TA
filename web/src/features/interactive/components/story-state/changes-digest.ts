import type { ActorStateSchemaSnapshot } from '../../types'
import { ADVENTURE_FIELD, adventureChangeDigest, isAdventureScalarCopy } from './adventure-state'
import { classifyStateChange, isEmptyStoredValue, type ClassifiedStateChange } from './changes'
import { INJURY_DISPLAY_NAME, INJURY_LABEL_KEY, benefitNames, injuryFieldKind, injuryLines, injuryNames, type InjuryFieldKind } from './injuries'
import { actorFieldPaths, actorTemplate, changeFieldLabel, changeFieldLabelKey, isLeadActor, sameFieldPath, type ActorStateEntry, type StoryStateChange } from './model'

/**
 * One line of the turn's change summary.
 *
 * The summary is the difference between the state the turn started from and the
 * state it settled into. That difference is already computed by the backend and
 * carried on each write as previous_value; this module decides which of those
 * differences a player would call a change, and in what order they read.
 *
 * `verb` turns a bare "火弹术" into "学会火弹术". It is an i18n key, so the
 * wording stays translatable, and it is only set where the story's own wording
 * for the event is unambiguous: learning an art, arriving at a place.
 */
export interface DigestEntry {
  id: string
  /** The subject: a field name, an item's name, or an art's name. */
  label: string
  /** The i18n key for the subject, when it is a caption rather than a name. */
  labelKey?: string
  /**
   * Whose field moved, when it is not the protagonist's.
   *
   * A trade writes both sides of the same field: the player's 灵石 falls by
   * four and the seller's rises by four. Without the name the row reads
   * "灵石 -4 · 灵石 +4" and contradicts itself, so every entry that is not the
   * protagonist's carries the person it belongs to.
   */
  actor?: string
  /** i18n key for a verb the entry reads better with, when one applies. */
  verb?: string
  change: ClassifiedStateChange
}

/** How many lines the summary row shows before it stops being a summary. */
export const DIGEST_LIMIT = 5

/**
 * The order a player would want these facts in.
 *
 * This is a fixed table, not a score: each entry names one field the story
 * rules actually settle and the rank it reads at. A wound outranks a purchase
 * because a wound changes what the character can do next; a stage breakthrough
 * outranks both because it changes what every number means. Anything the table
 * does not name reads last.
 */
const FIELD_PRIORITY: Record<string, number> = {
  生命: 1,
  生命上限: 1,
  经脉负担: 1,
  伤势: 1,
  余伤负担下限: 1,
  持续状态: 2,
  当前境界: 2,
  境界进度: 2,
  灵石: 3,
  重要物品: 3,
  主法器: 3,
  护具: 3,
  布阵: 3,
  攻击法宝: 3,
  防御法宝: 3,
  辅助法宝: 3,
  已学功法: 4,
  功法: 4,
  技能与能力: 4,
  当前详细地点: 5,
  当前任务: 6,
  关系: 7,
  对主角好感度: 7,
}

const UNRANKED_PRIORITY = 8

/**
 * Fields whose every write is a scene detail rather than a change of standing.
 *
 * The scene's prose fields are rewritten almost every turn — the current event,
 * the hook the next scene continues from, the character's immediate situation.
 * Repeating them as "changes" would fill the row with sentences that already
 * appear in the narration above it. The formal clock is left out for the same
 * reason: it has its own display, and a turn that advanced an hour has not
 * changed anything the player can act on.
 */
const ROUTINE_FIELDS = new Set([
  '当前事件',
  '可承接钩子',
  '当前处境',
  '世界局势',
  '本次交战',
  '在场角色',
  '当前时间',
  'scene.current_event',
  'scene.continuation_hook',
  'scene.present_actors',
  'scene.engagement',
  'scene.current_time',
  'world.situation',
  'current.situation',
])

/** Shared ops address a scene field by path; actor ops address it by name. */
const FIELD_ALIASES: Record<string, string> = {
  'scene.location': '当前详细地点',
  'tasks.current': '当前任务',
  'scene.current_time': '当前时间',
  'scene.current_event': '当前事件',
  'scene.continuation_hook': '可承接钩子',
  'scene.present_actors': '在场角色',
  'scene.engagement': '本次交战',
  'world.situation': '世界局势',
  'current.situation': '当前处境',
  'state.currency': '灵石',
}

/** Fields whose entries read as "learned X". */
const LEARNED_FIELDS = new Set(['已学功法', '功法', '技能与能力'])
/** Fields whose value names a place. */
const PLACE_FIELDS = new Set(['当前详细地点'])
/** The field that carries a blessing or an ailment by name. */
const EFFECTS_FIELDS = new Set(['持续状态', 'state.effects'])

/**
 * buildChangeDigest turns the turn's writes into the few lines worth reading.
 *
 * It drops what is not a change (a write that landed on the value the field
 * already held), what is not a change the player made (the opening schema
 * bootstrap), and what is a scene detail rather than a change of standing.
 * Object-valued fields are diffed key by key, because 重要物品 is rewritten
 * whole and the only readable fact is which entry appeared or went up by one.
 */
export function buildChangeDigest(
  changes: StoryStateChange[],
  actors: ActorStateEntry[],
  schema?: ActorStateSchemaSnapshot,
): DigestEntry[] {
  const ranked: Array<DigestEntry & { priority: number }> = []
  const merged = new Map<string, number>()

  const push = (entry: DigestEntry, priority: number) => {
    const existing = merged.get(entry.id)
    if (existing !== undefined) {
      // One field written twice in a turn is one change. Keep the entry that
      // says more: a number the player can read beats "已更新".
      if (ranked[existing].change.kind === 'updated' && entry.change.kind !== 'updated') {
        ranked[existing] = { ...entry, priority }
      }
      return
    }
    merged.set(entry.id, ranked.length)
    ranked.push({ ...entry, priority })
  }

  // A wound is one fact written by two fields, so it is read once, before the
  // per-field loop, and the two raw writes are kept out of it. Reporting them
  // separately is what put "余伤负担下限 0" and "持续状态 已更新" on the row.
  const wounds = injuryDigest(changes, actors, schema)
  for (const entry of wounds.entries) push(entry, FIELD_PRIORITY[INJURY_DISPLAY_NAME] ?? 1)

  for (const change of changes) {
    if (wounds.consumed.has(change.id)) continue
    if (change.sourceKind === 'state_schema_initialization') continue
    const adventure = schema?.system.adventure_rules
    if (adventure && change.path === ADVENTURE_FIELD) {
      for (const entry of adventureChangeDigest(change, adventure)) push(entry, 2)
      continue
    }
    if (adventure && isAdventureScalarCopy(change.path)) continue
    const op = change.op.trim().toLowerCase()
    if (op === 'set' && sameValue(change.previousValue, change.value)) continue
    // A person walking on stage has their whole sheet written at once, and most
    // of that sheet is zeroes and empty lists. Writing "nothing" over "nothing"
    // is the shape the sheet was already in, so it is not news. Without this,
    // two NPCs entering a scene fill the summary with their own blank fields and
    // push out the one thing the turn actually did.
    if (op === 'set' && isEmptyStoredValue(change.value) && (change.previousValue === undefined || isEmptyStoredValue(change.previousValue))) continue
    const field = FIELD_ALIASES[change.path.trim()] || change.path.trim()
    if (ROUTINE_FIELDS.has(field)) continue
    const priority = FIELD_PRIORITY[field] ?? UNRANKED_PRIORITY
    const label = changeFieldLabel(change, actors, schema)
    const labelKey = changeFieldLabelKey(change, actors, schema)
    const baseId = `${change.actorId || ''}:${change.path}`
    const actor = otherActorName(change.actorId, actors)

    if (isRecord(change.previousValue) && isRecord(change.value)) {
      // A record the table does not rank is bookkeeping the interface does not
      // understand — the shop ledger, a registry of who trades where. Diffing it
      // reads as "获得东街丹铺", which claims the player took the shop. The
      // player's own sheet is different: an unranked record there is still
      // something they now hold.
      if (priority === UNRANKED_PRIORITY && !isLeadChange(change, actors)) continue
      for (const record of recordChanges(change.previousValue, change.value)) {
        push({ ...recordEntry(baseId, record, field), actor }, priority)
      }
      continue
    }

    const classified = classifyStateChange(change, true)
    // 持续状态 carries blessings and ailments by name, and the wound half of it
    // was already read above. What is left is a list of named things, and naming
    // the one that appeared reads as "获得增攻药" where "持续状态 已更新" reads as
    // the interface noticing a field it does not understand.
    if (EFFECTS_FIELDS.has(field)) {
      for (const entry of effectEntries(baseId, change, actor)) push(entry, priority)
      continue
    }
    // A field the table does not rank is a restatement when all that happened
    // is that it was rewritten: a person's dossier, their situation, the scene's
    // prose. The narration above already says what they are and what changed,
    // and a row of dossiers would push the turn's real outcome off the summary.
    // An acquisition or a loss on such a field still counts — it is news.
    if (priority === UNRANKED_PRIORITY && classified.kind === 'updated') continue
    // "抵达越国坊市" reads as what happened; "当前详细地点 越国坊市" reads as a
    // field the interface happened to notice. The place is the news, so it takes
    // the label slot and the field name steps out of the way.
    if (PLACE_FIELDS.has(field) && classified.kind === 'updated' && classified.text) {
      push({ id: baseId, label: classified.text, actor, verb: 'storyStage.state.digest.arrived', change: { ...classified, text: '' } }, priority)
      continue
    }
    push({ id: baseId, label, labelKey, actor, change: classified }, priority)
  }

  return ranked
    .sort((left, right) => left.priority - right.priority)
    .slice(0, DIGEST_LIMIT)
    .map(({ priority: _priority, ...entry }) => entry)
}

interface RecordChange {
  key: string
  kind: 'added' | 'removed' | 'delta' | 'changed'
  delta?: number
  previous?: unknown
  next?: unknown
}

/**
 * injuryDigest reads the two wound fields as the one change they are.
 *
 * 余伤负担下限 and 持续状态 are written by the same event — a character takes a
 * wound, a wound heals, or the story fills in what the wound costs them — so
 * reporting them as two writes put "余伤负担下限 30" and "持续状态 已更新" on the
 * row and left the player to work out that the two lines were one thing. This
 * reads them together and answers with one line.
 *
 * The floor's writes are always consumed: that number never belongs on the
 * summary. A 持续状态 write is consumed only when the wound half of it moved —
 * a list that gained a pill and nothing else is a different fact, and the
 * per-field loop still reports it.
 */
function injuryDigest(
  changes: StoryStateChange[],
  actors: ActorStateEntry[],
  schema?: ActorStateSchemaSnapshot,
): { entries: DigestEntry[]; consumed: Set<string> } {
  const consumed = new Set<string>()
  const entries: DigestEntry[] = []
  const byActor = new Map<string, { floor: StoryStateChange[]; effects: StoryStateChange[] }>()
  for (const change of changes) {
    if (change.sourceKind === 'state_schema_initialization') continue
    const kind = injuryKindOfChange(change, actors, schema)
    if (!kind) continue
    const key = change.actorId || ''
    const bucket = byActor.get(key) ?? { floor: [], effects: [] }
    bucket[kind].push(change)
    byActor.set(key, bucket)
  }

  for (const [actorId, bucket] of byActor) {
    const before = injuryState(bucket.floor[0]?.previousValue, bucket.effects[0]?.previousValue)
    const after = injuryState(lastValue(bucket.floor), lastValue(bucket.effects))
    const linesMoved = !sameLines(before.lines, after.lines)
    for (const change of bucket.floor) consumed.add(change.id)
    if (linesMoved) for (const change of bucket.effects) consumed.add(change.id)
    if (before.wounded === after.wounded && !linesMoved) continue
    // Claiming a recovery needs the floor to say so. An emptied 持续状态 on its
    // own means the story stopped describing the wound, and the burden floor may
    // still be holding the character back — the panel shows the wound either way.
    if (!after.wounded && bucket.floor.length === 0) continue
    entries.push(injuryEntry(actorId, before, after, otherActorName(actorId || undefined, actors)))
  }
  return { entries, consumed }
}

interface InjuryState {
  wounded: boolean
  lines: string[]
  names: string[]
}

/**
 * injuryState reads one side of a wound write.
 *
 * A field the turn did not write reads as absent, not as zero: a turn that only
 * rewrote the prose knows nothing about the burden floor, and reading that
 * silence as "the wound is gone" would report a recovery the sheet contradicts.
 * An absent floor simply does not add a wound of its own.
 */
function injuryState(floorValue: unknown, effectsValue: unknown): InjuryState {
  const lines = injuryLines(effectsValue)
  const floor = typeof floorValue === 'number' && Number.isFinite(floorValue) ? floorValue : 0
  return { wounded: lines.length > 0 || floor > 0, lines, names: injuryNames(effectsValue) }
}

function lastValue(changes: StoryStateChange[]): unknown {
  return changes.length > 0 ? changes[changes.length - 1].value : undefined
}

function sameLines(left: string[], right: string[]): boolean {
  return left.length === right.length && left.every((line, index) => line === right[index])
}

function injuryEntry(actorId: string, before: InjuryState, after: InjuryState, actor: string | undefined): DigestEntry {
  const id = `${actorId}:${INJURY_DISPLAY_NAME}`
  if (!after.wounded) {
    // A wound that is gone is the good news of the turn, and "伤势 已痊愈" says
    // it without making the player compare two numbers to find it.
    return { id, label: INJURY_DISPLAY_NAME, labelKey: INJURY_LABEL_KEY, actor, verb: 'storyStage.state.digest.healed', change: { kind: 'updated', delta: null, text: '', tone: 'positive' } }
  }
  const fresh = after.names.filter((name) => !before.names.includes(name))
  if (fresh.length > 0) {
    // Only what the turn added: a character who already carried one wound and
    // took another should not read the old wound as new.
    return { id, label: fresh.join('、'), actor, verb: 'storyStage.state.digest.wounded', change: { kind: 'added', delta: null, text: '', tone: 'negative' } }
  }
  if (before.names.length === 0) {
    // The burden floor moved but the story has not said what the wound is. The
    // wound is real, and saying so beats inventing a name for it.
    return { id, label: INJURY_DISPLAY_NAME, labelKey: INJURY_LABEL_KEY, actor, verb: 'storyStage.state.digest.woundedUnnamed', change: { kind: 'added', delta: null, text: '', tone: 'negative' } }
  }
  // The same wound, described again — the story filled in what it stops the
  // character doing or how it heals. That is worth a line, and the line is the
  // description itself.
  return { id, label: INJURY_DISPLAY_NAME, labelKey: INJURY_LABEL_KEY, actor, change: { kind: 'updated', delta: null, text: after.lines.join('、'), tone: 'neutral' } }
}

/**
 * injuryKindOfChange resolves a write to one of the two wound fields.
 *
 * A write names its field by the pack's own id or name, and a pack may declare
 * either. The template is asked first because it is the pack's declaration; the
 * built-in pack's two keys stand in when the template is not at hand.
 */
function injuryKindOfChange(
  change: StoryStateChange,
  actors: ActorStateEntry[],
  schema?: ActorStateSchemaSnapshot,
): InjuryFieldKind | null {
  const direct = injuryFieldKind({ name: change.path, id: change.path })
  if (direct) return direct
  if (!change.actorId) return null
  const actor = actors.find(([actorId]) => actorId === change.actorId)?.[1]
  const template = actor ? actorTemplate(actor, schema) : undefined
  const field = template?.fields?.find((candidate) => actorFieldPaths(candidate).some((path) => sameFieldPath(path, change.path)))
  return injuryFieldKind(field)
}

/**
 * effectEntries reads a 持续状态 write as the named things that came or went.
 *
 * The field is a list of names, so the only readable facts are which entry
 * appeared and which is gone. Naming them is what lets the summary say 获得增攻药
 * instead of 持续状态 已更新, and it is why a list rewritten with the same
 * entries produces nothing at all.
 *
 * Wounds are left out: they are reported once, as 伤势, by injuryDigest above.
 */
function effectEntries(baseId: string, change: StoryStateChange, actor: string | undefined): DigestEntry[] {
  const before = benefitNames(change.previousValue)
  const after = benefitNames(change.value)
  const entries: DigestEntry[] = []
  for (const name of after) {
    if (before.includes(name)) continue
    entries.push({ id: `${baseId}:${name}:added`, label: name, actor, verb: 'storyStage.state.digest.gained', change: { kind: 'added', delta: null, text: '', tone: 'positive' } })
  }
  for (const name of before) {
    if (after.includes(name)) continue
    entries.push({ id: `${baseId}:${name}:removed`, label: name, actor, change: { kind: 'removed', delta: null, text: '', tone: 'negative' } })
  }
  return entries
}

function recordEntry(baseId: string, record: RecordChange, field: string): DigestEntry {
  const id = `${baseId}:${record.key}:${record.kind}`
  const value = isRecord(record.next) ? record.next : isRecord(record.previous) ? record.previous : {}
  const label = typeof value['名称'] === 'string' && value['名称'].trim() ? value['名称'].trim() : record.key
  const verb = record.kind === 'removed'
    ? undefined
    : LEARNED_FIELDS.has(field) ? studyChangeVerb(record, field)
      : PLACE_FIELDS.has(field) ? 'storyStage.state.digest.arrived'
        : undefined
  switch (record.kind) {
    case 'delta':
      return { id, label, change: { kind: 'delta', delta: record.delta ?? 0, text: '', tone: (record.delta ?? 0) > 0 ? 'positive' : 'negative' } }
    case 'added':
      return { id, label, verb, change: { kind: 'added', delta: null, text: '', tone: 'positive' } }
    case 'removed':
      return { id, label, change: { kind: 'removed', delta: null, text: '', tone: 'negative' } }
    default:
      return { id, label, verb, change: { kind: 'updated', delta: null, text: '', tone: 'neutral' } }
  }
}

// Structured study is not granted merely because a record appeared. Legacy
// records without a partial flag retain their released learned semantics.
function studyChangeVerb(record: RecordChange, field: string): string | undefined {
  const next = isRecord(record.next) ? record.next : {}
  const previous = isRecord(record.previous) ? record.previous : {}
  if (next['掌握程度'] === '修习中') {
    if (record.kind === 'changed' && previous['掌握程度'] === '修习中' && next['已修习分钟'] === previous['已修习分钟']) return undefined
    return typeof next['已修习分钟'] === 'number' && next['已修习分钟'] > 0
      ? 'storyStage.state.digest.studied' : 'storyStage.state.digest.registeredStudy'
  }
  const mastered = field === '功法' ? next['掌握程度'] === '已掌握' : next['掌握程度'] !== '修习中'
  if (mastered && previous['掌握程度'] === '修习中') return 'storyStage.state.digest.learned'
  if (mastered && field === '功法' && previous['运转'] !== next['运转']) {
    if (next['运转'] === '运转') return 'storyStage.state.digest.circulating'
    if (previous['运转'] === '运转') return 'storyStage.state.digest.stoppedCirculating'
  }
  return record.kind === 'added' ? 'storyStage.state.digest.learned' : undefined
}

/**
 * recordChanges reads a whole-record rewrite as the entries that really moved.
 *
 * A record-valued field is written whole on every touch, so the write alone
 * cannot say whether a potion was bought or the same pack was restated. The
 * difference between the two records can: a key that appeared is an acquisition,
 * a key that vanished is a loss, and an entry whose 数量 changed is that many
 * units either way.
 */
function recordChanges(previous: Record<string, unknown>, next: Record<string, unknown>): RecordChange[] {
  const result: RecordChange[] = []
  for (const [key, nextEntry] of Object.entries(next)) {
    const previousEntry = previous[key]
    if (previousEntry === undefined) {
      result.push({ key, kind: 'added', next: nextEntry })
      continue
    }
    const previousCount = quantityOf(previousEntry)
    const nextCount = quantityOf(nextEntry)
    if (previousCount !== null && nextCount !== null) {
      if (previousCount !== nextCount) result.push({ key, kind: 'delta', delta: nextCount - previousCount })
      continue
    }
    if (!sameValue(previousEntry, nextEntry)) result.push({ key, kind: 'changed', previous: previousEntry, next: nextEntry })
  }
  for (const key of Object.keys(previous)) {
    if (next[key] === undefined) result.push({ key, kind: 'removed', previous: previous[key] })
  }
  return result
}

function quantityOf(value: unknown): number | null {
  if (!isRecord(value)) return null
  const count = value['数量']
  return typeof count === 'number' && Number.isFinite(count) ? count : null
}

/**
 * otherActorName names the person a change belongs to, unless it is the player.
 *
 * The protagonist's own numbers need no owner: "灵石 -4" is what the player did.
 * Any other actor's does, because a trade moves both sides of the same field.
 *
 * The name comes from the actor's own name field, not from its id. This pack
 * keys its people by their names, so an id and a name are often the same string,
 * and refusing a name that matches its id would silently drop the owner from
 * every trade with a named NPC. What must not be printed is a bare id standing
 * in for a name the actor does not have.
 */
function otherActorName(actorId: string | undefined, actors: ActorStateEntry[]): string | undefined {
  if (!actorId) return undefined
  const actor = actors.find(([id]) => id === actorId)?.[1]
  if (isLeadActor(actorId, actor ?? {})) return undefined
  const rawName = actor ? actor.name : undefined
  const name = typeof rawName === 'string' ? rawName.trim() : ''
  return name || undefined
}

/**
 * isLeadChange answers whether a write landed on the player's own sheet.
 *
 * Only the player's fields describe what the player now holds. A story-level
 * record — a shop's till, a registry — says something about the world, and
 * diffing it as an acquisition would credit the player with a shop.
 */
function isLeadChange(change: StoryStateChange, actors: ActorStateEntry[]): boolean {
  if (!change.actorId) return false
  const actor = actors.find(([id]) => id === change.actorId)?.[1]
  return isLeadActor(change.actorId, actor ?? {})
}

/**
 * sameValue answers whether two stored values are the same fact.
 *
 * A write that restates the value a field already held is not a change, and the
 * summary must not report it as one. The comparison is structural because the
 * values are decoded JSON, and a record rewritten with the same keys in a
 * different order is the same record.
 */
export function sameValue(left: unknown, right: unknown): boolean {
  if (left === right) return true
  if (left === null || right === null || left === undefined || right === undefined) return false
  if (typeof left !== typeof right) return false
  if (typeof left !== 'object') return false
  if (Array.isArray(left) !== Array.isArray(right)) return false
  if (Array.isArray(left) && Array.isArray(right)) {
    return left.length === right.length && left.every((item, index) => sameValue(item, right[index]))
  }
  const leftRecord = left as Record<string, unknown>
  const rightRecord = right as Record<string, unknown>
  const leftKeys = Object.keys(leftRecord)
  const rightKeys = Object.keys(rightRecord)
  if (leftKeys.length !== rightKeys.length) return false
  return leftKeys.every((key) => key in rightRecord && sameValue(leftRecord[key], rightRecord[key]))
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}
