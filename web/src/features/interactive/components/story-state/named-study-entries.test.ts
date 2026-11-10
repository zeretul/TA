import { afterEach, expect, it } from 'vitest'
import i18n from '@/i18n'
afterEach(async () => { await i18n.changeLanguage('zh-CN') })
import type { LedgerFieldEntry } from './model'
import { namedStudyEntries } from './named-study-entries'
import { readSkills } from './skills'
const id = 'gen.craft.craft.talisman.ward.hash'
const field = { name: '已学功法', type: 'object', path: 'abilities.learned' }
const entries = (value: unknown, withheld = false): LedgerFieldEntry[] => [{ id: 'skills', label: 'Skills', field, value, withheld }]
const state = (name = '松烟护符法') => ({ actors: { story: { state: { 技能定义: {
  [id]: { 定义: id, 版本: 1, 名称: name, 类别: 'craft' },
  unknown: { 定义: 'unknown', 版本: 1, 名称: 'Never acquired' },
} } } } })
it('names only an already held old record from the exact snapshot, and labels craft honestly', () => {
  const input = entries({ [id]: { 定义: id, 版本: 1, 掌握程度: '已学会' } })
  const snapshot = state()
  const before = JSON.stringify({ input, snapshot })
  const output = namedStudyEntries(input, snapshot, 'protagonist', 'Unnamed record')
  expect(readSkills(output[0].value, {}).lines).toEqual([{ name: '松烟护符法', kind: 'craft', description: '', source: '' }])
  expect(JSON.stringify(output)).not.toContain('Never acquired')
  expect(JSON.stringify({ input, snapshot })).toBe(before)
  expect(readSkills(namedStudyEntries(input, state('另一局名称'), 'protagonist', 'Unnamed record')[0].value, {}).lines[0].name).toBe('另一局名称')
})
it('keeps explicit names and incomplete mastery when metadata is available', () => {
  const output = namedStudyEntries(entries({ [id]: { 定义: id, 版本: 1, 名称: '我的称呼', 掌握程度: '修习中', 已修习分钟: 12 } }), state(), 'protagonist', 'Unnamed record')
  expect(readSkills(output[0].value, {}).lines[0]).toMatchObject({ name: '我的称呼', kind: 'learning', learningMinutes: 12 })
})
it('does not guess a hashed name or category from a missing or mismatched definition', () => {
  for (const snapshot of [{}, state()]) {
    const output = namedStudyEntries(entries({ [id]: { 定义: id, 版本: 99 } }), snapshot, 'protagonist', 'Unnamed record')
    expect(readSkills(output[0].value, {}).lines[0]).toMatchObject({ name: 'Unnamed record', kind: 'unverified' })
    expect((output[0].value as Record<string, Record<string, unknown>>)[id]['类别']).toBe('unverified')
  }
})
it('never enriches withheld records or another person’s sheet', () => {
  const input = entries({ [id]: { 定义: id, 版本: 1 } }, true)
  expect(namedStudyEntries(input, state(), 'protagonist', 'Unnamed record')[0]).toBe(input[0])
  expect(namedStudyEntries(input, state(), 'unknown-npc', 'Unnamed record')).toBe(input)
})


it.each(['zh-CN', 'en-US'])('names native craft from the exact frozen version in %s without granting catalog entries', async language => {
  await i18n.changeLanguage(language)
  const nativeID = 'craft.talisman.ward'
  const methods = [{ id: nativeID, version: 1, name_key: 'craft.talisman.ward.name' }, { id: 'unheld', version: 1, name_key: 'Unheld method' }]
  const input = entries({ [nativeID]: { 定义: nativeID, 版本: 1, 掌握程度: '已学会' } })
  const before = JSON.stringify({ input, methods })
  const output = namedStudyEntries(input, {}, 'protagonist', 'Unnamed record', { craftMethods: methods, name: key => i18n.t(key) })
  expect(readSkills(output[0].value, {}).lines).toEqual([{ name: i18n.t('craft.talisman.ward.name'), kind: 'craft', description: '', source: '' }])
  expect(JSON.stringify(output)).not.toContain('Unheld method')
  expect(JSON.stringify({ input, methods })).toBe(before)
  const mismatch = namedStudyEntries(input, {}, 'protagonist', 'Unnamed record', { craftMethods: [{ ...methods[0], version: 2 }], name: key => i18n.t(key) })
  expect(readSkills(mismatch[0].value, {}).lines[0]).toMatchObject({ name: 'Unnamed record', kind: 'unverified' })
})
