import { describe, expect, it } from 'vitest'
import { createInstance } from 'i18next'
import zhCN from '@/i18n/locales/zh-CN'
import { catalogName, playerVisibleEntries, stateEntryLabel } from './state-entry-display'

/**
 * These tests hold the state ledger to one rule: the player reads the world, not
 * the rule layer's bookkeeping.
 *
 * The stored record keeps its definition ID and revision — the cast path prices
 * an action from them — so the assertion here is about what is drawn, never
 * about what is saved.
 */

function translator() {
  const instance = createInstance()
  instance.init({ lng: 'zh-CN', resources: { 'zh-CN': { translation: zhCN } }, interpolation: { escapeValue: false } })
  return instance.t
}

describe('state entry display', () => {
  it('continues to the item name when the UI returns blank for an absent skill name', () => {
    const instance = createInstance()
    instance.init({ lng: 'zh-CN', resources: { 'zh-CN': { translation: zhCN } }, keySeparator: false, parseMissingKeyHandler: () => '' })
    expect(catalogName('talisman.ward.lower', instance.t)).toBe('护身符')
    expect(catalogName('no.such.definition', instance.t)).toBe('')
  })

  it('drops the definition id, revision and template from a learned technique', () => {
    const entry = { 定义: 'strike', 版本: 1, 掌握程度: '熟练' }
    expect(playerVisibleEntries(entry)).toEqual([['掌握程度', '熟练']])
  })

  it('drops the definition id from a held item but keeps its count and origin', () => {
    const entry = { 定义: 'pill.restore.health.lower', 数量: 1, 作用: '疗伤', 来源: '坊市买入的保命丹' }
    expect(playerVisibleEntries(entry).map(([key]) => key)).toEqual(['数量', '作用', '来源'])
  })

  it('does not expose rule metadata when a record has no readable details', () => {
    const entry = { 定义: 'shield', 版本: 1 }
    expect(playerVisibleEntries(entry)).toEqual([])
  })

  it('reads a story-given name as itself', () => {
    const t = translator()
    expect(stateEntryLabel('凝气拳', { 定义: 'strike', 掌握程度: '熟练' }, t)).toBe('凝气拳')
  })

  it('names a generated technique by the wording the story gave it', () => {
    const t = translator()
    expect(stateEntryLabel('gen.fire_bolt.赤炎诀', { 定义: 'gen.fire_bolt.赤炎诀', 名称: '赤炎诀' }, t)).toBe('赤炎诀')
  })

  it('names a technique keyed by its definition id from the interface catalogue', () => {
    const t = translator()
    expect(stateEntryLabel('fire_bolt', { 掌握程度: '初学' }, t)).toBe('火弹术')
  })

  it('names a held item from the catalogue even though its key spells the id differently', () => {
    const t = translator()
    expect(catalogName('pill.restore.health.lower', t)).toBe('回春丹')
    expect(catalogName('implement.lower.common', t)).toBe('凡铁剑')
  })

  it('reports no catalogue wording for a definition the catalogue does not hold', () => {
    const t = translator()
    expect(catalogName('pill.no_such_pill.upper', t)).toBe('')
  })
})
