import { describe, expect, it } from 'vitest'
import i18n from '@/i18n'
import { resolveWorldPresentation } from './world-presentation'
import type { Snapshot, StorySummary } from './types'

const profile = (family: string) => ({
  status_template: family === 'cultivation' ? 'xiuxian-state' : family + '-state-v1',
  growth: family, combat: family, communication: family === 'space' ? 'radio' : 'letter', opening: 'scene',
})
const native = (family: 'wuxia' | 'space' | 'fantasy'): Pick<Snapshot, 'actor_state_schema'> => ({
  actor_state_schema: { version: 1, revision: 1, system: { adventure_rules: { family, skills: [], items: [] } } },
})
const story = (family: string): Pick<StorySummary, 'pack_binding' | 'module_refs'> => ({
  pack_binding: { pack_id: 'author-world', scope: 'pack:author-world', gameplay: profile(family) },
})
const read = (current: ReturnType<typeof story>, locale = 'zh-CN', snapshot?: Pick<Snapshot, 'actor_state_schema'>) =>
  resolveWorldPresentation({ story: current, snapshot }, locale, i18n.getFixedT(locale))

describe('frozen world presentation and supported UI capabilities', () => {
  it('uses the fantasy encounter surface and refuses mismatched or unknown combat rules', () => {
    expect(read(story('fantasy'), 'zh-CN', native('fantasy')).combatMode).toBe('adventure')
    expect(read(story('fantasy'), 'zh-CN', native('space')).combatMode).toBe('none')
    expect(read(story('unimplemented')).combatMode).toBe('none')
    const noCombat = story('fantasy')
    noCombat.pack_binding!.gameplay!.combat = 'none'
    expect(read(noCombat, 'zh-CN', native('fantasy')).combatMode).toBe('none')
  })
  it('keeps custom cultivation module IDs usable while honoring disabled state and unknown templates', () => {
    const custom = { ...story('cultivation'), module_refs: { actor_state_id: 'world-actor-state-custom' } }
    expect(read(custom).intimacyAvailable).toBe(true)
    expect(read({ ...custom, module_refs: { ...custom.module_refs, actor_state_disabled: true } }).intimacyAvailable).toBe(false)
    expect(read(story('unknown')).intimacyAvailable).toBe(false)
    expect(read({ module_refs: { actor_state_id: 'xiuxian-state' } }).combatMode).toBe('cultivation')
  })
  it('uses the story language without leaking another world or untranslated author labels', () => {
    const school = story('fantasy')
    school.pack_binding!.gameplay!.presentation = {
      'zh-CN': { exploration: '校园探访', energy: '魔力', techniques: '咒语', partner: '伴侣' },
      'en-US': { exploration: 'School exploration', energy: 'Magic', techniques: 'Spells' },
    }
    const snapshot = native('fantasy')
    expect(read(school, 'zh-CN', snapshot).terms.exploration).toBe('校园探访')
    const english = read(school, 'en-US', snapshot)
    expect(english.terms.exploration).toBe('School exploration')
    expect(english.t('storyStage.adventure.effort')).toBe('Magic')
    expect(english.t('entryDetails.fact.effort')).toBe('Magic cost')
    expect(english.t('combatWindow.adventure.cost', { count: 7 })).toBe('Costs 7 Magic')
    expect(read(story('space'), 'zh-CN', native('space')).terms.exploration).toBe('设施探索')
    delete school.pack_binding!.gameplay!.presentation!['en-US']
    expect(read(school, 'en-US', snapshot).terms.exploration).toBe('Ruin exploration')
    expect(school.pack_binding!.gameplay!.presentation!['zh-CN']!.energy).toBe('魔力')
  })
  it('derives legacy cultivation from its real stage schema, not from missing metadata', () => {
    const stage: Pick<Snapshot, 'actor_state_schema'> = { actor_state_schema: { version: 1, revision: 1, system: { templates: [{ id: 'protagonist', name: 'Cultivator', fields: [{ name: '境界', type: 'string', value_format: 'stage' }] }] } } }
    expect(read({}, 'zh-CN', stage).combatMode).toBe('cultivation')
    expect(read({}, 'zh-CN', { actor_state_schema: { version: 1, revision: 1, system: { templates: [] } } }).combatMode).toBe('none')
  })
})

it('does not activate unsupported imported schema families', () => {
  const unknown = native('fantasy')
  unknown.actor_state_schema!.system.adventure_rules!.family = 'unknown-import' as 'fantasy'
  const actual = read({}, 'zh-CN', unknown)
  expect(actual.combatMode).toBe('none')
  expect(actual.family).toBe('general')
  expect(actual.terms.exploration).toBe('探索')
})
