import { expect, it } from 'vitest'
import i18n from '@/i18n'
import { adventureLedgerEntries, type AdventureCatalog } from './adventure-state'
import { buildChangeDigest } from './changes-digest'
import type { ActorStateSchemaSnapshot } from '../../types'

const catalog: AdventureCatalog = { family: 'space', skills: [{ id: 'engineering', name: 'Engineering' }], items: [{ id: 'part', name: 'Parts' }, { id: 'ship', name: 'Shuttle' }] }
const before = { family: 'space', rank: 1, experience: 10, health: 80, max_health: 100, energy: 20, max_energy: 50, injury: 0, currency: 100, skills: { engineering: 1 }, training: { engineering: 3 }, inventory: { part: 2 }, assets: {}, acquired: {}, defeated: {} }
const after = { ...before, experience: 16, training: { engineering: 6 }, acquired: { secret: true }, defeated: { boss: true }, battle: { enemy_inventory: { secret: 1 } } }
const schema = { system: { adventure_rules: catalog } } as ActorStateSchemaSnapshot

it('summarizes actual experience and training changes without treating tracking sets as acquisitions', () => {
  const digest = buildChangeDigest([{ id: 'change', actorId: 'protagonist', path: '历练状态', op: 'set', value: after, previousValue: before }], [], schema)
  expect(digest.map(item => [item.labelKey || item.label, item.change.delta])).toEqual([['storyStage.adventure.experience', 6], ['Engineering', 3]])
  expect(digest[1].verb).toBe('storyStage.adventure.trainingDelta')
  expect(JSON.stringify(digest)).not.toMatch(/acquired|defeated|secret|boss|enemy_inventory/)
})

it('renders a whitelist of formal values without duplicate scalar copies or internal objects', () => {
  const rows = adventureLedgerEntries([{ id: '历练状态', label: '历练状态', value: after }, { id: '生命', label: '生命', value: 80 }], catalog, i18n.t.bind(i18n))
  expect(rows.find(row => row.id === 'adventure:health')?.value).toBe('80 / 100')
  expect(rows.find(row => row.id === 'adventure:skill:engineering')?.value).toContain('6')
  expect(rows.find(row => row.id === 'adventure:item:part')?.label).toBe('Parts')
  expect(JSON.stringify(rows)).not.toMatch(/acquired|defeated|enemy_inventory|secret|boss/)
  expect(rows.some(row => row.id === '生命' || row.id === '历练状态')).toBe(false)
})

it('renders only the player’s saved source conditions with independent English and Chinese captions', async () => {
  const saved = { ...after, soft_limbs: true, fog_area_id: 'source-fog', rush_blocked: 1, last_seal_target: 'private-tracking-id' }
  const original = JSON.stringify(saved)
  await i18n.changeLanguage('en-US')
  const english = adventureLedgerEntries([{ id: '历练状态', label: '历练状态', value: saved }], catalog, i18n.t.bind(i18n))
  expect(english.find(row => row.id === 'adventure:soft-limbs')?.label).toBe('Weakened limbs')
  expect(english.find(row => row.id === 'adventure:fog')?.value).toContain('Rushing footwork is blocked')
  expect(JSON.stringify(english)).not.toContain('private-tracking-id')
  await i18n.changeLanguage('zh-CN')
  const chinese = adventureLedgerEntries([{ id: '历练状态', label: '历练状态', value: saved }], catalog, i18n.t.bind(i18n))
  expect(chinese.find(row => row.id === 'adventure:rush-blocked')?.label).toBe('突进受限')
  expect(JSON.stringify(saved)).toBe(original)
})

it('shows a partially used owned stone and mental corruption without displaying its internal provenance ID', async () => {
  const known: AdventureCatalog = { ...catalog, items: [...catalog.items, { id: 'stone', name: 'Wish stone', max_charges: 3 }] }
  const saved = { ...after, mental_corruption: 60, mental_shock: 1, mental_exposure_id: 'internal-source-id', inventory: { stone: 1 }, item_charges: { stone: 2 } }
  const original = JSON.stringify(saved)
  await i18n.changeLanguage('en-US')
  const rows = adventureLedgerEntries([{ id: '历练状态', label: '历练状态', value: saved }], known, i18n.t.bind(i18n))
  expect(rows.find(row => row.id === 'adventure:charges:stone')?.value).toBe(2)
  expect(rows.find(row => row.id === 'adventure:mental-corruption')?.label).toBe('Mental corruption')
  expect(JSON.stringify(rows)).not.toContain('internal-source-id')
  expect(JSON.stringify(saved)).toBe(original)
  await i18n.changeLanguage('zh-CN')
})

it('reports remaining cooldown from formal clock without changing saved deadlines', () => {
  const saved = { ...after, inventory: { part: 1 }, item_charges: { part: 2 }, item_ready_at: { part: 140 } }
  const original = JSON.stringify(saved)
  const rows = adventureLedgerEntries([{ id: '历练状态', label: '历练状态', value: saved }], catalog, i18n.t.bind(i18n), 20)
  expect(rows.find(row => row.id === 'adventure:cooldown:part')?.value).toBe(120)
  expect(JSON.stringify(saved)).toBe(original)
})

it('keeps permanent ship wear distinct from elapsed propulsion cooldown', async () => {
  const saved = { ...after, assets: { part: { health: 60, max_health: 90, shield: 0, fuel: 0, permanent_damage: 10, disabled_until: 10820 } } }
  const original = JSON.stringify(saved)
  await i18n.changeLanguage('en-US')
  const rows = adventureLedgerEntries([{ id: '历练状态', label: '历练状态', value: saved }], catalog, i18n.t.bind(i18n), 20)
  expect(rows.find(row => row.id === 'adventure:wear:part')?.value).toBe(10)
  expect(rows.find(row => row.id === 'adventure:power-lock:part')?.value).toBe(10800)
  const later = adventureLedgerEntries([{ id: '历练状态', label: '历练状态', value: saved }], catalog, i18n.t.bind(i18n), 10820)
  expect(later.some(row => row.id === 'adventure:power-lock:part')).toBe(false)
  expect(later.find(row => row.id === 'adventure:wear:part')?.value).toBe(10)
  expect(JSON.stringify(saved)).toBe(original)
  await i18n.changeLanguage('zh-CN')
})

it('projects the frozen stage beside rank without relabeling legacy stories', () => {
  const extended = { ...catalog, progression: { max_rank: 60, stages: [{ id: 'f', name: 'F级', english_name: 'F', min_rank: 1 }, { id: 'divine', name: '超神级', english_name: 'Transcendent', min_rank: 51 }] } }
  const entry = { id: '历练状态', label: 'State', value: { ...before, rank: 51 } } as Parameters<typeof adventureLedgerEntries>[0][number]
  const rows = adventureLedgerEntries([entry], extended, ((key: string) => key) as never)
  expect(rows.find(row => row.id === 'adventure:rank')?.value).toBe('超神级 · 51 / 60')
  const legacy = adventureLedgerEntries([entry], catalog, ((key: string) => key) as never)
  expect(legacy.find(row => row.id === 'adventure:rank')?.value).toBe(51)
})
