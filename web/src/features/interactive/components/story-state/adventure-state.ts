import type { TFunction } from 'i18next'
import type { StoryDirectorActorStateSystem } from '../../types'
import type { LedgerFieldEntry, StoryStateChange } from './model'
import type { DigestEntry } from './changes-digest'

export type AdventureCatalog = NonNullable<StoryDirectorActorStateSystem['adventure_rules']>
export const ADVENTURE_FIELD = '历练状态'
const scalarCopies = new Set(['历练等级', '生命', '生命上限', '气力', '气力上限', '内力', '内力上限', '精力', '精力上限', '伤势程度', '钱财'])
const record = (value: unknown): Record<string, unknown> => value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
const number = (value: unknown) => typeof value === 'number' && Number.isFinite(value) ? value : 0
const scalarKeys = ['rank', 'experience', 'health', 'energy', 'injury', 'currency', 'poison', 'control'] as const
const labelKey = (key: string, family: string) => `storyStage.adventure.${key === 'energy' && family === 'wuxia' ? 'innerEnergy' : key === 'energy' && family === 'fantasy' ? 'effort' : key}`

/** A whitelist of actual player possessions and values, never arbitrary object
 * keys. Tracking sets, enemy battle internals and proposed rewards stay out. */
export function adventureLedgerEntries(entries: LedgerFieldEntry[], catalog: AdventureCatalog | undefined, t: TFunction, clockSeconds = 0): LedgerFieldEntry[] {
  const canonical = entries.find(entry => entry.id === ADVENTURE_FIELD)
  if (!canonical || !catalog) return entries
  const value = record(canonical.value)
  const rows: LedgerFieldEntry[] = []
  const add = (id: string, label: string, content: unknown, group: string, type: 'number' | 'string' = 'string') => rows.push({ id: `adventure:${id}`, label, value: content, field: { name: `adventure:${id}`, type, group }, renderer: 'inline' })
  for (const key of scalarKeys) {
    if ((key === 'poison' || key === 'control') && !number(value[key])) continue
    if (key === 'rank' && catalog.progression) {
      const rank = number(value.rank)
      const stage = [...catalog.progression.stages].reverse().find(stage => stage.min_rank <= rank)
      add(key, t(labelKey(key, catalog.family)), `${stage?.name || ''} · ${rank} / ${catalog.progression.max_rank}`, t('storyStage.adventure.status'))
      continue
    }
    const maximum = key === 'health' ? value.max_health : key === 'energy' ? value.max_energy : undefined
    add(key, t(labelKey(key, catalog.family)), maximum === undefined ? number(value[key]) : `${number(value[key])} / ${number(maximum)}`, t('storyStage.adventure.status'))
  }
  if (number(value.mental_corruption)) add('mental-corruption', t('storyStage.adventure.mentalCorruption'), number(value.mental_corruption), t('storyStage.adventure.status'), 'number')
  if (number(value.mental_shock)) add('mental-shock', t('storyStage.adventure.mentalShock'), t('combatWindow.adventure.mentalShock'), t('storyStage.adventure.status'))
  if (value.soft_limbs) add('soft-limbs', t('storyStage.adventure.softLimbs'), t('combatWindow.adventure.softLimbs'), t('storyStage.adventure.status'))
  if (value.fog_area_id && !value.fog_treated) add('fog', t('storyStage.adventure.fog'), t('combatWindow.adventure.fogBlocked'), t('storyStage.adventure.status'))
  if (number(value.rush_blocked)) add('rush-blocked', t('storyStage.adventure.rushBlocked'), t('combatWindow.adventure.rushBlocked'), t('storyStage.adventure.status'))
  const name = (id: string, kind: 'skills' | 'items') => catalog[kind]?.find(entry => entry.id === id)?.name || t(`storyStage.adventure.unknown${kind === 'skills' ? 'Skill' : 'Item'}`)
  for (const [id, level] of Object.entries(record(value.skills))) {
    if (number(level) > 0) add(`skill:${id}`, name(id, 'skills'), t('storyStage.adventure.skillProgress', { level, hours: number(record(value.training)[id]) }), t('storyStage.adventure.skills'))
  }
  for (const [id, quantity] of Object.entries(record(value.inventory))) {
    if (number(quantity) > 0) {
      add(`item:${id}`, name(id, 'items'), t('storyStage.adventure.quantity', { count: number(quantity) }), t('storyStage.adventure.inventory'))
      const maximum = catalog.items?.find(item => item.id === id)?.max_charges
      if (maximum) add(`charges:${id}`, `${name(id, 'items')} · ${t('storyStage.adventure.charges')}`, record(value.item_charges)[id] ?? maximum, t('storyStage.adventure.inventory'), 'number')
    }
  }
  for (const [id, ready] of Object.entries(record(value.item_ready_at))) {
    const remaining = Math.max(0, number(ready) - clockSeconds)
    if (remaining > 0) add(`cooldown:${id}`, `${name(id, 'items')} · ${t('storyStage.adventure.cooldown')}`, remaining, t('storyStage.adventure.equipment'), 'number')
  }
  for (const [slot, id] of Object.entries(record(value.equipment))) {
    if (typeof id !== 'string' || !id) continue
    const [scale, kind] = slot.split(':')
    if (!['person', 'machine', 'ship'].includes(scale) || !['weapon', 'armor', 'tool'].includes(kind)) continue
    add(`equipment:${slot}`, `${t(`combatWindow.adventure.scale.${scale}`)} · ${t(`storyStage.adventure.slot.${kind}`)}`, name(id, 'items'), t('storyStage.adventure.equipment'))
  }
  for (const [id, raw] of Object.entries(record(value.assets))) {
    const asset = record(raw)
    if (number(asset.permanent_damage) > 0) add(`wear:${id}`, `${name(id, 'items')} · ${t('storyStage.adventure.permanentWear')}`, number(asset.permanent_damage), t('storyStage.adventure.assets'), 'number')
    if (number(asset.disabled_until) > clockSeconds) add(`power-lock:${id}`, `${name(id, 'items')} · ${t('storyStage.adventure.powerLock')}`, number(asset.disabled_until) - clockSeconds, t('storyStage.adventure.assets'), 'number')
    add(`asset:${id}`,  name(id, 'items'), t('combatWindow.adventure.asset', { health: number(asset.health), max: number(asset.max_health), shield: number(asset.shield), fuel: number(asset.fuel) }), t('storyStage.adventure.assets'))
  }
  return [...entries.filter(entry => entry.id !== ADVENTURE_FIELD && !scalarCopies.has(entry.id)), ...rows]
}

export function adventureChangeDigest(change: StoryStateChange, catalog: AdventureCatalog): DigestEntry[] {
  const before = record(change.previousValue), after = record(change.value)
  const entries: DigestEntry[] = []
  const add = (id: string, label: string, delta: number, key?: string, verb?: string) => {
    if (delta !== 0) entries.push({ id: `${change.actorId}:adventure:${id}`, label, labelKey: key, verb, change: { kind: 'delta', delta, text: '', tone: delta > 0 ? 'positive' : 'negative' } })
  }
  // With no previous snapshot, do not claim a gain from a bootstrap write.
  if (!Object.keys(before).length) return entries
  for (const key of scalarKeys) add(key, key, number(after[key]) - number(before[key]), labelKey(key, catalog.family))
  for (const kind of ['inventory', 'skills', 'training'] as const) {
    const old = record(before[kind]), next = record(after[kind])
    for (const id of new Set([...Object.keys(old), ...Object.keys(next)])) {
      const definitions = kind === 'inventory' ? catalog.items : catalog.skills
      const name = definitions?.find(entry => entry.id === id)?.name
      add(`${kind}:${id}`, name || '', number(next[id]) - number(old[id]), name ? undefined : `storyStage.adventure.unknown${kind === 'inventory' ? 'Item' : 'Skill'}`, kind === 'training' ? 'storyStage.adventure.trainingDelta' : kind === 'skills' ? 'storyStage.adventure.skillDelta' : undefined)
    }
  }
  return entries
}

export function isAdventureScalarCopy(path: string) { return scalarCopies.has(path) }
