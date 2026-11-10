import { afterEach, describe, expect, it } from 'vitest'
import i18n from '@/i18n'
import { realmLabel } from './realm-label'
import { realmLabel as combatRealmLabel } from '../combat-window/combat-format'

afterEach(async () => { await i18n.changeLanguage('zh-CN') })

describe('cultivation realm labels across state and combat', () => {
  it.each(['zh-CN', 'en-US'])('words every Spirit Realm stage in %s without exposing rule keys', async language => {
    await i18n.changeLanguage(language)
    const realms = language === 'zh-CN'
      ? { void: '炼虚', integration: '合体', mahayana: '大乘' }
      : { void: 'Void Refinement, ', integration: 'Body Integration, ', mahayana: 'Mahayana, ' }
    const stages = language === 'zh-CN'
      ? { early: '初期', middle: '中期', late: '后期' }
      : { early: 'early', middle: 'middle', late: 'late' }
    for (const [realm, name] of Object.entries(realms)) {
      for (const [stage, suffix] of Object.entries(stages)) {
        const key = `${realm}_${stage}`
        expect(realmLabel(key, i18n.t)).toBe(name + suffix)
        expect(combatRealmLabel(key, i18n.t)).toBe(name + suffix)
      }
    }
  })
  it('preserves custom labels and absent data without showing unknown rule keys', () => {
    expect(realmLabel('自定义境界', i18n.t)).toBe('自定义境界')
    expect(realmLabel('Custom Realm', i18n.t)).toBe('Custom Realm')
    expect(realmLabel('custom_realm', i18n.t)).toBe('custom_realm')
    expect(realmLabel(undefined, i18n.t)).toBe('')
    expect(realmLabel('nascent_peak', i18n.t)).toBe('境界未明')
    expect(realmLabel('spirit_late', i18n.t)).toBe('化神后期')
  })
})
