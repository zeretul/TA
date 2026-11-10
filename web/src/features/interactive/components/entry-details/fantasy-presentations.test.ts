import { afterEach, expect, it } from 'vitest'
import i18n from '@/i18n'
import { adventurePresentations } from './presentations'
import type { AdventureDetailsResponse } from './types'
import { adventureLedgerEntries, type AdventureCatalog } from '../story-state/adventure-state'

afterEach(async () => { await i18n.changeLanguage('zh-CN') })
for (const language of ['zh-CN', 'en-US']) {
  it(`shows fantasy effort and reusable crafting tools without fake possessions in ${language}`, async () => {
    await i18n.changeLanguage(language)
    const data: AdventureDetailsResponse = { head: 'saved', family: 'fantasy', actor: { skills: { forge: 1 }, inventory: { iron: 3 } }, skills: [{ id: 'forge', name: 'Coldhammer Smithing', kind: 'engineering', cost: 4 }], items: [{ id: 'iron', name: 'Iron' }], craft_methods: [{ id: 'gen.craft.blade', name: 'Balanced Blade Method', skill_id: 'forge', skill_rank: 1, hours: 3, required_tool: 'tools', materials: { iron: 3 }, output_id: 'sword', output_count: 1, item_names: { tools: 'Travel Forge Tools', sword: 'Travel Blade', iron: 'Iron' } }] }
    const before = JSON.stringify(data)
    const rows = adventurePresentations(data, i18n.t)
    expect(rows[0].requirements).toContainEqual({ label: i18n.t('entryDetails.fact.requiredTool'), value: 'Travel Forge Tools' })
    expect(rows[0].costs.some(fact => fact.value === 'Travel Forge Tools')).toBe(false)
    expect(rows.find(row => row.id === 'forge')?.costs).toContainEqual({ label: i18n.t('entryDetails.fact.effort'), value: '4' })
    expect(rows.some(row => row.id === 'tools' || row.id === 'sword')).toBe(false)
    expect(JSON.stringify(data)).toBe(before)
    const catalog: AdventureCatalog = { family: 'fantasy', skills: [], items: [] }
    const ledger = adventureLedgerEntries([{ id: '历练状态', label: '历练状态', value: { rank: 1, health: 100, max_health: 100, energy: 52, max_energy: 100, currency: 24 } }, { id: '精力', label: '精力', value: 52 }, { id: '精力上限', label: '精力上限', value: 100 }], catalog, i18n.t)
    expect(ledger.find(row => row.id === 'adventure:energy')?.label).toBe(i18n.t('storyStage.adventure.effort'))
    expect(ledger.filter(row => row.id === '精力' || row.id === '精力上限')).toHaveLength(0)
  })
}
