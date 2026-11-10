import { activeAffiliation, knownAffiliations, type KnownAffiliation } from './known-affiliations'
import type { CharacterWindow, Snapshot } from '../../types'
import { knownProvenance, provenanceKeys, type KnownProvenance } from './known-provenance'
import { actorFieldEntries, actorTemplate } from '../story-state/model'

export interface KnownPerson {
  id: string
  name: string
  affiliations: string[]
  memberships?: KnownAffiliation[]
  lastKnownLocation?: string
  seenNow: boolean
  observedAt?: string
  locationProvenance?: KnownProvenance
}

export interface KnownRelation {
  id: string
  sourceId: string
  targetId: string
  label: string
  labelKey?: string
  statement?: string
  mutual: boolean
  undirected?: boolean
  provenance?: KnownProvenance
}

export interface KnownPeople {
  people: KnownPerson[]
  relations: KnownRelation[]
}

/** Reads only the player-facing snapshot and known cast. Never consults author
 * metadata, guesses memberships from tags, or resolves people by display name. */
export function buildKnownPeople(snapshot: Snapshot | null, window: CharacterWindow | null): KnownPeople {
  if (snapshot && window && (snapshot.story_id !== window.story_id || snapshot.branch_id !== window.branch_id)) return { people: [], relations: [] }
  const actors = record(snapshot?.state.actors)
  const player = record(actors.protagonist)
  const factions = record(field(snapshot, record(actors.world), 'world.factions', '势力记录'))
  const cast = (window?.characters || []).filter(person => person.id !== 'protagonist')
  const identities = [
    ...(Object.keys(player).length ? [{ id: 'protagonist', name: text(player.name) || 'protagonist', present: false }] : []),
    ...cast.map(person => ({ id: person.id, name: person.known_as || person.name, present: person.present })),
  ]
  const people: KnownPerson[] = identities.map(person => {
    const actor = record(actors[person.id])
    const memberships = knownAffiliations(field(snapshot, actor, 'identity.affiliations', '所属势力'), factions, person.id === 'protagonist' ? window?.faction_memberships : [])
    const locationNote = record(field(snapshot, actor, 'knowledge.location', '已知位置'))
    // Generic lifecycle/presence text can be an off-screen engine update. It
    // is not evidence of what the protagonist learned or when.
    const location = text(locationNote.location) || text(locationNote['位置'])
    const seenNow = person.present && Boolean(window?.scene.location)
    return {
      id: person.id,
      name: person.name,
      affiliations: memberships.filter(activeAffiliation).map(item => item.name),
      memberships,
      lastKnownLocation: seenNow ? window?.scene.location : location || undefined,
      seenNow,
      observedAt: seenNow ? window?.scene.world_time : undefined,
      locationProvenance: seenNow ? { learnedAt: window?.scene.world_time, sourceKey: 'loreOverview.provenance.currentScene', nature: 'experienced', confidence: 'confirmed' } : knownProvenance(locationNote),
    }
  })
  const known = new Set(people.map(person => person.id))
  const relations: KnownRelation[] = []
  const formal = new Set(cast.filter(person => person.relationship).map(person => person.id))
  for (const person of people) {
    const actor = record(actors[person.id])
    // NPC private relations stay private. The separate public field records
    // only relationships the protagonist actually learned about.
    const records = record(field(snapshot, actor,
      person.id === 'protagonist' ? 'relations.records' : 'knowledge.relationships',
      person.id === 'protagonist' ? '关系' : '已知人物关系'))
    for (const [targetId, value] of Object.entries(records)) {
      if (targetId === person.id || !known.has(targetId)) continue
      const details = record(value)
      const label = text(details['类型']) || text(details['关系类型']) || text(details.kind) || text(details.type) || text(value)
      if (!label) continue
      // Formal social bonds replace older labels on the same axis. A distinct
      // relationship, such as kinship or apprenticeship, can still coexist.
      const hasFormalBond = (person.id === 'protagonist' && formal.has(targetId)) || (targetId === 'protagonist' && formal.has(person.id))
      if (hasFormalBond && formalBondLabels.has(label)) continue
      const statement = Object.entries(details).filter(([key]) => !['类型', '关系类型', 'kind', 'type'].includes(key) && !provenanceKeys.has(key))
        .flatMap(([key, item]) => text(item) ? [`${key}：${text(item)}`] : []).join(' · ')
      relations.push({ id: `known:${person.id}:${targetId}`, sourceId: person.id, targetId, label, statement: statement || undefined, mutual: false, provenance: knownProvenance(details) })
    }
  }
  for (const person of cast) {
    if (!person.relationship || !known.has('protagonist')) continue
    const ended = person.relationship.kind === 'estranged' || person.relationship.kind === 'hostile'
    relations.push({ id: `formal:${person.id}`, sourceId: 'protagonist', targetId: person.id,
      label: person.relationship.kind, labelKey: `characterWindow.relationship.${person.relationship.kind}`,
      statement: person.relationship.statement, mutual: !ended, undirected: ended || undefined, provenance: {
        learnedAt: person.relationship.world_time || undefined, sourceKey: 'loreOverview.provenance.formalRecord', nature: 'formal', confidence: 'confirmed',
        references: [
          ...(person.relationship.expressed_by ? [{ labelKey: 'loreOverview.provenance.expression', id: person.relationship.expressed_by }] : []),
          ...(person.relationship.answered_by ? [{ labelKey: 'loreOverview.provenance.answer', id: person.relationship.answered_by }] : []),
        ],
      } })
  }
  return { people, relations }
}

const formalBondLabels = new Set(['friend', 'close_friend', 'sworn', 'romantic', 'partner', 'estranged', 'hostile',
  '朋友', '好友', '挚友', '密友', '结义', '恋爱', '道侣', '伴侣', '疏远', '敌对', '同伴', '伙伴'])

function field(snapshot: Snapshot | null, actor: Record<string, unknown>, path: string, legacyName: string): unknown {
  const template = actorTemplate(actor, snapshot?.actor_state_schema)
  const schemaField = template?.fields?.find(field => field.path === path || field.id === path || field.name === legacyName)
  if (schemaField) {
    const entry = actorFieldEntries(actor, [schemaField])[0]
    return entry?.withheld ? undefined : entry?.value
  }
  const state = record(actor.state)
  // Legacy records still use their established field name. Never use a schema
  // default as evidence that a person belongs somewhere or knows someone.
  return state[legacyName] ?? state[path]
}

function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : typeof value === 'number' ? String(value) : ''
}
