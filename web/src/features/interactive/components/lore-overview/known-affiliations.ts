import type { FactionMembership } from '../../types'
import { knownProvenance, type KnownProvenance } from './known-provenance'

export interface KnownAffiliation {
  factionId?: string
  name: string
  status: 'formal' | 'temporary' | 'pending' | 'left' | 'known' | 'unresolved'
  role?: string
  statement?: string
  statementKey?: string
  provenance?: KnownProvenance
}

/** Stable record keys, never equal display titles, establish faction identity. */
export function knownAffiliations(raw: unknown, factions: Record<string, unknown>, formal: FactionMembership[] = []): KnownAffiliation[] {
  const byID = new Map<string, KnownAffiliation>()
  const unresolved = new Map<string, KnownAffiliation>()
  for (const value of Array.isArray(raw) ? raw : typeof raw === 'string' ? [raw] : []) {
    const object = record(value)
    const explicitID = text(object.faction_id)
    const legacy = typeof value === 'string' ? text(value) : ''
    const factionId = explicitID || (Object.hasOwn(factions, legacy) ? legacy : '')
    const faction = record(factions[factionId])
    const name = text(object.name) || text(faction.name) || text(faction['名称']) || legacy || factionId
    if (!name) continue
    const status = typeof value === 'string' ? (factionId ? 'known' : 'unresolved') : validStatus(text(object.status)) || 'unresolved'
    const item: KnownAffiliation = { factionId: factionId || undefined, name, status: factionId ? status : 'unresolved', role: text(object.role) || undefined, provenance: knownProvenance(object) }
    if (factionId) byID.set(factionId, item)
    else unresolved.set(name, item)
  }
  for (const membership of formal) {
    if (!membership.faction_id || !validStatus(membership.status)) continue
    byID.set(membership.faction_id, { factionId: membership.faction_id, name: membership.faction_name || membership.faction_id,
      status: membership.status, role: membership.role,
      statement: membership.source_kind === 'identity' ? undefined : membership.statement,
      statementKey: membership.source_kind === 'identity' ? 'loreOverview.membership.creationConfirmed' : undefined, provenance: {
        learnedAt: membership.source_kind === 'identity' ? undefined : membership.world_time || undefined, sourceKey: 'loreOverview.provenance.formalMembership', nature: 'formal', confidence: 'confirmed',
        references: membership.source_exchange_id ? [{ labelKey: 'loreOverview.provenance.exchange', id: membership.source_exchange_id }] : [],
      } })
  }
  return [...byID.values(), ...unresolved.values()]
}
export function activeAffiliation(item: KnownAffiliation) { return item.status === 'formal' || item.status === 'temporary' || item.status === 'known' }
function validStatus(value: string): FactionMembership['status'] | undefined {
  return value === 'formal' || value === 'temporary' || value === 'pending' || value === 'left' ? value : undefined
}
function record(value: unknown): Record<string, unknown> { return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {} }
function text(value: unknown): string { return typeof value === 'string' ? value.trim() : '' }
