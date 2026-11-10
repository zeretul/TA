import { activeAffiliation, type KnownAffiliation } from './known-affiliations'
import { buildKnownPeople, type KnownPeople, type KnownPerson, type KnownRelation } from './known-people'
import type { KnownProvenance } from './known-provenance'
import type { CharacterWindow, Snapshot } from '../../types'
import type { JourneyState } from '../../journey-types'
import type { PublicEventState } from '../../public-event-types'
import type { QuestState } from '../../quest-types'
import {
  actorFieldEntries,
  actorName,
  actorTemplate,
  buildLedgerGroups,
  type LedgerFieldEntry,
  type LedgerFieldGroup,
} from '../story-state/model'

/**
 * The six tabs. They are reading categories, not six services: every one of
 * them is assembled from a read the game already performs.
 */
export type LoreTabKey = 'player' | 'people' | 'factions' | 'places' | 'world' | 'matters'

export const LORE_TABS: LoreTabKey[] = ['player', 'people', 'factions', 'places', 'world', 'matters']

/**
 * One thing the player can open in the overview.
 *
 * `groups` is the same field model the state ledger draws, so a number shown
 * here is the number the main story shows and there is no second rendering of
 * the same fact. `rows` carries the reads that are already shaped for reading
 * (a person, a matter, an event) and need no field resolution.
 *
 * A title or label that comes from the story's own state is literal and is
 * never translated. A label this panel contributes carries a `Key` instead, so
 * the two languages stay separate without either inventing wording.
 */
export interface LoreEntry {
  /** Stable identity. Never a display name, so two people called the same stay apart. */
  id: string
  tab: LoreTabKey
  title: string
  /** Set for a heading this panel contributes; wins over `title`. */
  titleKey?: string
  /** Identity or status line, one short phrase from the story. */
  subtitle?: string
  /** Set when the status is one of the game's own enumerated states. */
  subtitleKey?: string
  /** A standing fact about the whole entry (e.g. it is over), shown quietly. */
  statusKey?: string
  /** One-line description used in the list. */
  summary?: string
  groups?: LedgerFieldGroup[]
  rows?: LoreRow[]
  /** Other known objects in this same panel. Empty when there is no real link. */
  links?: LoreLink[]
  /** Search-only text. Never rendered. */
  keywords?: string[]
  /**
   * A person's picture, already a URL the read returned. Only people carry this:
   * the other tabs name things, not faces.
   */
  avatar?: string
  /** Larger server-selected picture. Never used by a directory row. */
  avatarDisplay?: string
  /** The server's neutral caption for the selected portrait. */
  imageCaption?: string
  /**
   * The world book's own picture for this person, relative to the project. It is
   * the picture the world gave them when no avatar was chosen, and the panel
   * addresses it through the project it is already reading.
   */
  avatarLorePath?: string
  /**
   * Where this entity came from. The overview does not care; the lore library's
   * 本局新建 page does, because only people the story itself registered belong
   * there. A person the author's world already had keeps its state changes in
   * the story without becoming a second entry, so it is left off that page.
   */
  identitySource?: 'author_catalog' | 'story'
  /**
   * The library category this entity files under. It is set for the entries a
   * library page can list — people, places, factions, holdings — and left unset
   * for the synthetic headings the overview draws for itself.
   */
  section?: LoreSectionKey
  affiliations?: string[]
  memberships?: KnownAffiliation[]
  factionId?: string
  pastMembers?: KnownPerson[]
  pendingMembers?: KnownPerson[]
  unresolvedMembers?: KnownPerson[]
  lastKnownLocation?: string
  locationProvenance?: KnownProvenance
  members?: KnownPerson[]
  relations?: KnownRelation[]
  relationPeople?: KnownPerson[]
}

/**
 * The categories the lore library's 本局新建 page sorts its entities into. They
 * are the same four the world's own entries use, so a person is a person in
 * both pages.
 */
export type LoreSectionKey = 'characters' | 'locations' | 'factions' | 'items'

export interface LoreRow {
  id: string
  label: string
  /** Set for a label this panel contributes; wins over `label`. */
  labelKey?: string
  value: unknown
  /** Set when the value is the game's own catalogue wording; wins over `value`. */
  valueKey?: string
  /** Committed count drawn beside a `valueKey`, as `name × count`. */
  valueCount?: number
  valueSuffix?: string
  /** True when the player has not found this out; renders as 未知, not as empty. */
  withheld?: boolean
  /** Long prose gets its own paragraph instead of an inline value. */
  block?: boolean
}

export interface LoreLink {
  tab: LoreTabKey
  entryId: string
  label: string
}

export interface LoreOverviewSources {
  snapshot: Snapshot | null
  characters: CharacterWindow | null
  journey: JourneyState | null
  quests: QuestState | null
  events: PublicEventState | null
  /** The story's own opening situation, from the story summary. */
  origin?: string
  /** The pack this story bound to, named for a reader. */
  worldName?: string
}

export interface LoreOverviewModel {
  entries: LoreEntry[]
  /** Every entry the player knows, by tab, in reading order. */
  byTab: Record<LoreTabKey, LoreEntry[]>
  knownPeople: KnownPeople
}

const ACTOR_ROOT = 'actors'
const PROTAGONIST_ID = 'protagonist'
const WORLD_ID = 'world'
const STORY_ID = 'story'
const LOCATION_RECORDS = '地点记录'
const FACTION_RECORDS = '势力记录'

/** Synthetic entries: they exist only as a heading in the panel. */
const WORLD_STANDING_ID = 'world:standing'
const NEARBY_ROUTES_ID = 'journey:routes'

/**
 * Assembles the six tabs from what the game has already committed and already
 * read. Nothing here calls a model, advances the clock or writes anything.
 *
 * The snapshot is the player-known projection the rest of the stage reads, so
 * searching and counting happen after the backend has already withheld what the
 * character does not know. That ordering is the whole point: a secret that was
 * never sent cannot be found here either.
 */
export function buildLoreOverview(sources: LoreOverviewSources): LoreOverviewModel {
  const knownPeople = buildKnownPeople(sources.snapshot, sources.characters)
  const byTab: Record<LoreTabKey, LoreEntry[]> = {
    player: playerEntries(sources.snapshot),
    people: peopleEntries(sources.characters),
    factions: factionEntries(sources.snapshot),
    places: placeEntries(sources.snapshot, sources.journey),
    world: worldEntries(sources),
    matters: matterEntries(sources.quests),
  }
  // Links are added first, then the flat list is built from the linked entries,
  // so opening an entry shows the same object the list holds.
  enrichKnownPeople(byTab, knownPeople)
  const linked = linkRecords(byTab)
  return {
    entries: LORE_TABS.flatMap((tab) => linked[tab]),
    byTab: linked,
    knownPeople,
  }
}


/** Attach known social facts without another store or inference from prose. */
function enrichKnownPeople(byTab: Record<LoreTabKey, LoreEntry[]>, known: KnownPeople) {
  for (const person of known.people) {
    const entry = person.id === PROTAGONIST_ID ? byTab.player[0] : byTab.people.find(item => item.id === person.id)
    if (!entry) continue
    entry.affiliations = person.affiliations
    entry.memberships = person.memberships
    // The dedicated row also handles unknown membership, so do not repeat the
    // same field in the generic player ledger above it.
    entry.groups = entry.groups?.map(group => ({ ...group, fields: group.fields.filter(field => field.id !== 'identity.affiliations' && field.label !== '所属势力') })).filter(group => group.fields.length > 0)
    entry.locationProvenance = person.locationProvenance
    entry.lastKnownLocation = person.lastKnownLocation
    entry.rows = [...(entry.rows || []), {
      id: 'affiliations', label: 'affiliations', labelKey: 'loreOverview.row.affiliations',
      value: person.affiliations.join(' · ') || null,
    }]
    if (person.id !== PROTAGONIST_ID) {
      entry.rows.push({ id: 'lastKnownLocation', label: 'lastKnownLocation',
        labelKey: person.seenNow ? 'loreOverview.row.observedLocation' : 'loreOverview.row.lastKnownLocation',
        value: person.lastKnownLocation || null })
      if (person.observedAt) entry.rows.push({ id: 'observedAt', label: 'observedAt', labelKey: 'loreOverview.row.observedAt', value: person.observedAt })
    }
    entry.keywords = [...(entry.keywords || []), ...(person.memberships || []).flatMap(item => [item.name, item.factionId || '']), person.lastKnownLocation || '']
    entry.relations = known.relations.filter(relation => relation.sourceId === person.id || relation.targetId === person.id)
    entry.relationPeople = known.people
    for (const membership of person.memberships || []) {
      if (!membership.factionId) continue
      let faction = byTab.factions.find(item => item.factionId === membership.factionId)
      if (!faction) {
        faction = { id: `${WORLD_ID}:${FACTION_RECORDS}:${membership.factionId}`, factionId: membership.factionId, tab: 'factions', title: membership.name, section: 'factions' }
        byTab.factions.push(faction)
      }
      if (person.id !== PROTAGONIST_ID) {
        const group = activeAffiliation(membership) ? 'members' : membership.status === 'left' ? 'pastMembers' : membership.status === 'pending' ? 'pendingMembers' : 'unresolvedMembers'
        faction[group] = [...(faction[group] || []), person]
      }
      faction.keywords = [...(faction.keywords || []), person.name, person.lastKnownLocation || '']
      entry.links = [...(entry.links || []), { tab: 'factions', entryId: faction.id, label: membership.name }]
    }
  }
}

export { WORLD_STANDING_ID, NEARBY_ROUTES_ID }

const IMPORTANT_ITEMS = '重要物品'

/**
 * The entities this story itself registered, by library category.
 *
 * 本局新建 is not a second store. Everything here already exists on the formal
 * save path: a person the story invented is a profile whose `source_key` is
 * empty, a place or faction is a record under the world's own state, and a
 * holding is the protagonist's item record. This function only decides which of
 * them the page lists and how each one files, so the page cannot drift from
 * what the game actually committed.
 *
 * A person the author's world already had is deliberately absent: the story
 * keeps their changed state without making a second entry of them, and copying
 * them here would be exactly that second entry.
 *
 * The snapshot is the player-known projection, so anything the character never
 * found out is already gone before this runs. That is why the library page and
 * the overview agree: neither can show a fact the other was denied.
 */
export function buildLocalEntities(sources: LoreOverviewSources): Record<LoreSectionKey, LoreEntry[]> {
  const people = peopleEntries(sources.characters).filter((entry) => entry.identitySource === 'story')
  return {
    characters: people,
    locations: recordEntries(sources.snapshot, WORLD_ID, LOCATION_RECORDS, 'places'),
    factions: recordEntries(sources.snapshot, WORLD_ID, FACTION_RECORDS, 'factions'),
    items: itemEntries(sources.snapshot),
  }
}

/**
 * The protagonist's own important items. Another character's holdings are a
 * private field the backend withholds, so only the player's own are ever here.
 */
function itemEntries(snapshot: Snapshot | null): LoreEntry[] {
  const actor = actorState(snapshot, PROTAGONIST_ID)
  const state = actor && isRecord(actor.state) ? actor.state : null
  const assets = state && isRecord(state.assets) ? state.assets : null
  const records = assets ? assets[IMPORTANT_ITEMS] : undefined
  if (!isRecord(records)) return []
  return Object.entries(records)
    .filter((entry): entry is [string, Record<string, unknown>] => isRecord(entry[1]))
    .sort(([left], [right]) => left.localeCompare(right, 'zh-Hans-CN'))
    .map(([name, record]) => {
      const fields: LedgerFieldEntry[] = Object.entries(record).map(([key, value]) => ({
        id: `${name}.${key}`,
        label: key,
        value,
      }))
      const quantity = textValue(record['数量'])
      return {
        id: `assets:${name}`,
        tab: 'world' as const,
        title: name,
        subtitle: quantity || undefined,
        summary: textValue(record['作用']) || textValue(record['描述']) || undefined,
        groups: buildLedgerGroups(fields, []),
        keywords: [name, ...Object.values(record).filter((value): value is string => typeof value === 'string')],
        section: 'items' as const,
      }
    })
}

// --- 玩家 ------------------------------------------------------------------

function playerEntries(snapshot: Snapshot | null): LoreEntry[] {
  const actor = actorState(snapshot, PROTAGONIST_ID)
  if (!actor) return []
  const template = actorTemplate(actor, snapshot?.actor_state_schema)
  const groups = buildLedgerGroups(ledgerEntries(actor, template?.fields), [])
  return [{
    id: `actor:${PROTAGONIST_ID}`,
    tab: 'player',
    title: actorName(PROTAGONIST_ID, actor),
    groups,
  }]
}

// --- 人物 ------------------------------------------------------------------

function peopleEntries(window: CharacterWindow | null): LoreEntry[] {
  if (!window) return []
  return (window.characters || []).map((character) => {
    const rows: LoreRow[] = []
    if (character.role) rows.push({ id: 'role', label: 'role', labelKey: 'loreOverview.row.role', value: character.role })
    // Only a settled gender is drawn. An empty one is not a gap the player can
    // close by looking harder — nobody decided — so printing "unknown" would
    // claim a fact the story does not hold.
    if (character.gender) {
      rows.push({ id: 'gender', label: 'gender', labelKey: 'loreOverview.row.gender', value: null, valueKey: `characterWindow.gender.${character.gender}` })
    }
    if (character.attitude) rows.push({ id: 'attitude', label: 'attitude', labelKey: 'loreOverview.row.attitude', value: character.attitude })
    if (character.tags && character.tags.length > 0) {
      rows.push({ id: 'tags', label: 'tags', labelKey: 'loreOverview.row.tags', value: character.tags.join(' · ') })
    }
    if (character.relationship) {
      rows.push({ id: 'bond', label: 'bond', labelKey: 'loreOverview.row.bond', value: character.relationship.statement })
    }
    if (character.first_seen_world_time) {
      rows.push({ id: 'firstSeen', label: 'firstSeen', labelKey: 'loreOverview.row.firstSeen', value: character.first_seen_world_time })
    }
    if (character.description) {
      rows.push({ id: 'description', label: 'description', labelKey: 'loreOverview.row.description', value: character.description, block: true })
    }
    return {
      id: character.id,
      tab: 'people' as const,
      title: character.known_as || character.name,
      subtitle: character.role || undefined,
      summary: character.attitude || character.description || undefined,
      rows,
      keywords: [character.name, character.known_as || '', character.role || '', ...(character.aliases || []), ...(character.tags || [])],
      // The picture the character window draws, from the same read. A person the
      // story has not put on stage was never sent, so no face can leak here.
      avatar: character.avatar || undefined,
      avatarDisplay: character.avatar_display || undefined,
      imageCaption: character.image_caption || undefined,
      avatarLorePath: character.avatar_lore_path || undefined,
      // Who registered them. A person the world already had is not a new entry
      // the story made, so the library's 本局新建 page leaves them out while the
      // overview keeps showing what the story settled about them.
      identitySource: character.identity_source,
      section: 'characters' as const,
    }
  })
}

// --- 势力 ------------------------------------------------------------------

function factionEntries(snapshot: Snapshot | null): LoreEntry[] {
  return recordEntries(snapshot, WORLD_ID, FACTION_RECORDS, 'factions')
}

// --- 地点 ------------------------------------------------------------------

function placeEntries(snapshot: Snapshot | null, journey: JourneyState | null): LoreEntry[] {
  const entries = recordEntries(snapshot, WORLD_ID, LOCATION_RECORDS, 'places')
  if (!journey) return entries
  // The roads the committed journey record actually offers from where the
  // character stands. A place with no road keeps no row: an unlisted road is not
  // a road, and inventing one would promise travel the rules would refuse.
  const here = journey.location_id || journey.location || ''
  const routes = (journey.routes || []).filter((route) => here !== '' && (
    route.origin_location_id === here || route.origin_label === here
  ))
  if (routes.length === 0) return entries
  entries.unshift({
    id: NEARBY_ROUTES_ID,
    tab: 'places',
    title: 'routes',
    titleKey: 'loreOverview.nearbyRoutes',
    rows: routes.map((route) => ({
      id: route.route_id,
      label: route.name || route.origin_label,
      labelKey: route.name_key || route.origin_name_key || undefined,
      valueKey: route.destination_name_key,
      valueSuffix: route.destination_name_key ? ` · ${route.total_minutes}` : undefined,
      value: `${route.destination_label} · ${route.total_minutes}`,
    })),
    keywords: routes.flatMap((route) => [route.origin_label, route.destination_label, route.name || '']),
  })
  return entries
}

// --- 世界 ------------------------------------------------------------------

function worldEntries(sources: LoreOverviewSources): LoreEntry[] {
  const entries: LoreEntry[] = []
  const story = actorState(sources.snapshot, STORY_ID)
  const storyState = story && isRecord(story.state) ? story.state : null
  const rows: LoreRow[] = []
  if (sources.worldName) rows.push({ id: 'worldName', label: 'worldName', labelKey: 'loreOverview.row.worldName', value: sources.worldName })
  if (sources.origin) rows.push({ id: 'origin', label: 'origin', labelKey: 'loreOverview.row.origin', value: sources.origin, block: true })
  if (storyState) {
    const time = textValue(storyState['当前时间'])
    if (time) rows.push({ id: 'time', label: 'time', labelKey: 'loreOverview.row.time', value: time })
    const location = textValue(storyState['当前详细地点'])
    if (location) rows.push({ id: 'location', label: 'location', labelKey: 'loreOverview.row.location', value: location })
    const situation = textValue(storyState['世界局势'])
    if (situation) rows.push({ id: 'situation', label: 'situation', labelKey: 'loreOverview.row.situation', value: situation, block: true })
  }
  if (rows.length > 0) {
    entries.push({ id: WORLD_STANDING_ID, tab: 'world', title: 'standing', titleKey: 'loreOverview.worldStanding', rows })
  }
  for (const event of sources.events?.events || []) {
    const eventRows: LoreRow[] = [{ id: 'brief', label: 'brief', labelKey: 'loreOverview.row.brief', value: event.brief, block: true }]
    if (event.region) eventRows.push({ id: 'region', label: 'region', labelKey: 'loreOverview.row.region', value: event.region })
    if (event.current_stage_title) eventRows.push({ id: 'stage', label: 'stage', labelKey: 'loreOverview.row.stage', value: event.current_stage_title })
    // Only the stage the character was actually present at carries its lines;
    // the read has already decided that, so this renders what it sent.
    const stage = (event.stages || []).find((item) => item.id === event.current_stage_id)
    for (const line of stage?.lines || []) {
      eventRows.push({ id: `line:${line}`, label: 'line', labelKey: 'loreOverview.row.line', value: line, block: true })
    }
    // The parties the region already knows about, and whether this character
    // took part. Both are fields of the read, not guesses.
    const known = (event.participants || []).filter((person) => person.name)
    if (known.length > 0) {
      eventRows.push({
        id: 'people',
        label: 'people',
        labelKey: 'loreOverview.row.people',
        value: known.map((person) => person.role ? `${person.name}（${person.role}）` : person.name).join('、'),
        block: true,
      })
    }
    if (event.joined && event.participation) {
      eventRows.push({ id: 'tookPart', label: 'tookPart', labelKey: 'loreOverview.row.tookPart', value: event.participation, block: true })
    } else if (event.withdrawn) {
      eventRows.push({ id: 'tookPart', label: 'tookPart', labelKey: 'loreOverview.row.tookPart', value: null, valueKey: 'loreOverview.withdrawn' })
    }
    entries.push({
      id: event.event_id,
      tab: 'world',
      title: event.title,
      subtitle: event.region || undefined,
      // "Over" is a standing fact about the event, so it rides on the entry
      // rather than becoming a field row whose value would read as blank.
      statusKey: event.closed ? 'loreOverview.closed' : undefined,
      summary: event.brief || undefined,
      rows: eventRows,
      keywords: [event.title, event.brief, event.region || '', event.current_stage_title || '', ...known.map((person) => person.name)],
    })
  }
  return entries
}

// --- 见闻与事务 -------------------------------------------------------------

function matterEntries(quests: QuestState | null): LoreEntry[] {
  if (!quests) return []
  return (quests.tasks || []).map((task) => {
    const rows: LoreRow[] = []
    if (task.brief) rows.push({ id: 'brief', label: 'brief', labelKey: 'loreOverview.row.brief', value: task.brief, block: true })
    if (task.giver) rows.push({ id: 'giver', label: 'giver', labelKey: 'loreOverview.row.giver', value: task.giver })
    for (const objective of task.objectives || []) {
      rows.push({
        id: `objective:${objective.id}`,
        label: 'objective',
        labelKey: 'loreOverview.row.objective',
        value: objective.description,
      })
    }
    // The agreed instant in the story's own wording. An agreement with no
    // instant says so; it is not an unknown and it is not a countdown.
    // `remaining_seconds` is deliberately not read.
    rows.push({
      id: 'deadline',
      label: 'deadline',
      labelKey: 'loreOverview.row.deadline',
      value: task.deadline_text || null,
      valueKey: task.deadline_text ? undefined : 'questWindow.noDeadline',
    })
    for (const reward of task.rewards || []) {
      // The catalogue's own name when it has one, so a payment is never shown as
      // an internal reference id. Quantity stays the committed number.
      const named = reward.kind === 'item' && reward.name_key ? reward.name_key : ''
      rows.push({
        id: `reward:${reward.kind}:${reward.ref_id}`,
        label: 'reward',
        labelKey: 'loreOverview.row.reward',
        value: `${reward.ref_id} × ${reward.quantity}`,
        valueKey: named || undefined,
        valueCount: named ? reward.quantity : undefined,
      })
    }
    return {
      id: task.task_id,
      tab: 'matters' as const,
      title: task.title,
      subtitle: task.status,
      subtitleKey: `questWindow.status.${task.status}`,
      summary: task.brief || undefined,
      rows,
      keywords: [task.title, task.brief, task.giver || ''],
    }
  })
}

// --- records ---------------------------------------------------------------

/**
 * Turns a record-valued field (地点记录 / 势力记录) into one entry per record.
 *
 * The record's key is the identity and the readable name at once: the state
 * schema writes these map keys in story language and forbids slugs, so the key
 * is what a reader should see. Each sub-value becomes its own field row through
 * the same group builder the ledger uses.
 */
function recordEntries(snapshot: Snapshot | null, actorId: string, fieldName: string, tab: LoreTabKey): LoreEntry[] {
  const actor = actorState(snapshot, actorId)
  const state = actor && isRecord(actor.state) ? actor.state : null
  const records = state ? state[fieldName] : undefined
  if (!isRecord(records)) return []
  const section: LoreSectionKey = tab === 'factions' ? 'factions' : 'locations'
  return Object.entries(records)
    .filter((entry): entry is [string, Record<string, unknown>] => isRecord(entry[1]))
    .sort(([left], [right]) => left.localeCompare(right, 'zh-Hans-CN'))
    .map(([name, record]) => {
      const entries: LedgerFieldEntry[] = Object.entries(record).map(([key, value]) => ({
        id: `${name}.${key}`,
        label: key,
        value,
      }))
      const status = textValue(record['存续状态']) || textValue(record['当前状态'])
      const summary = textValue(record['核心特征']) || textValue(record['关键特征']) || textValue(record['类型'])
      return {
        id: `${actorId}:${fieldName}:${name}`,
        tab,
        title: tab === 'factions' ? textValue(record.name) || textValue(record['名称']) || name : name,
        factionId: tab === 'factions' ? name : undefined,
        subtitle: status || undefined,
        summary: summary || undefined,
        groups: buildLedgerGroups(entries, []),
        keywords: [name, ...Object.values(record).filter((value): value is string => typeof value === 'string')],
        section,
      }
    })
}

/**
 * Adds the links that rest on a stable identity the record itself already
 * carries: a faction whose field names a place record by its exact key, a place
 * whose field names a faction by its exact key, and a matter whose giver is one
 * of the known cast by a recorded name or alias.
 *
 * Nothing is guessed from substrings. §5 forbids the panel from deciding that
 * two strings are one person, so a near miss stays unlinked and reads as text.
 */
function linkRecords(byTab: Record<LoreTabKey, LoreEntry[]>): Record<LoreTabKey, LoreEntry[]> {
  const factions = byTab.factions
  const places = byTab.places
  const factionByName = new Map(factions.map((entry) => [entry.title, entry]))
  const placeByName = new Map(places.map((entry) => [entry.title, entry]))
  const personByKnownName = new Map<string, LoreEntry>()
  for (const person of byTab.people) {
    for (const name of [person.title, ...(person.keywords || [])]) {
      if (name && !personByKnownName.has(name)) personByKnownName.set(name, person)
    }
  }
  const link = (entry: LoreEntry, target: LoreEntry | undefined): LoreEntry => {
    if (!target || target.id === entry.id) return entry
    const links = entry.links || []
    if (links.some((item) => item.entryId === target.id)) return entry
    return { ...entry, links: [...links, { tab: target.tab, entryId: target.id, label: target.title }] }
  }
  return {
    ...byTab,
    factions: factions.map((entry) => {
      let next = entry
      for (const field of entry.groups?.flatMap((group) => group.fields) || []) {
        const place = placeByName.get(textValue(field.value))
        if (place) next = link(next, place)
      }
      return next
    }),
    places: places.map((entry) => {
      let next = entry
      for (const field of entry.groups?.flatMap((group) => group.fields) || []) {
        const faction = factionByName.get(textValue(field.value))
        if (faction) next = link(next, faction)
      }
      return next
    }),
    matters: byTab.matters.map((entry) => {
      const name = textValue(entry.rows?.find((row) => row.id === 'giver')?.value)
      return name ? link(entry, personByKnownName.get(name)) : entry
    }),
  }
}

// --- helpers ---------------------------------------------------------------

function actorState(snapshot: Snapshot | null, actorId: string): Record<string, unknown> | null {
  const actors = snapshot?.state[ACTOR_ROOT]
  if (!isRecord(actors)) return null
  const actor = actors[actorId]
  return isRecord(actor) ? actor : null
}

/**
 * The same field mapping the state ledger performs, so a value shown here is the
 * value shown there. A private field the pack declares and the actor does not
 * carry is withheld, not empty.
 */
function ledgerEntries(actor: Record<string, unknown>, fields: Parameters<typeof actorFieldEntries>[1]): LedgerFieldEntry[] {
  return actorFieldEntries(actor, fields).map(({ field, value, withheld }) => ({
    id: field.id || field.path || field.name,
    label: field.name,
    field,
    value: withheld ? null : value ?? field.default ?? null,
    withheld,
  }))
}

/**
 * Filters entries by what the player typed.
 *
 * The filtering happens here, over the already-projected reads: an object the
 * character has not found out about never reached this list, so no search term
 * can make it appear or change a count.
 */
export function filterLoreEntries(entries: LoreEntry[], query: string): LoreEntry[] {
  const needle = query.trim().toLocaleLowerCase()
  if (!needle) return entries
  return entries.filter((entry) => {
    const haystack = [
      entry.title,
      entry.subtitle || '',
      entry.summary || '',
      ...(entry.keywords || []),
      ...(entry.rows || []).flatMap((row) => [row.label, row.valueKey || '', textValue(row.value)]),
      ...(entry.groups || []).flatMap((group) => group.fields.flatMap((field) => [field.label, textValue(field.value)])),
    ]
    return haystack.some((value) => value.toLocaleLowerCase().includes(needle))
  })
}

function textValue(value: unknown): string {
  if (typeof value === 'string') return value.trim()
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  return ''
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}
