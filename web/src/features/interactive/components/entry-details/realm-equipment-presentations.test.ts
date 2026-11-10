import { afterEach, expect, it } from 'vitest'
import i18n from '@/i18n'
import { cultivationPresentation } from './presentations'

afterEach(async () => { await i18n.changeLanguage('zh-CN') })
for (const language of ['zh-CN', 'en-US']) {
  it(`shows rated capacity, real upkeep and current support in ${language}`, async () => {
    await i18n.changeLanguage(language)
    const blade = cultivationPresentation({ id: 'blade', name: '云纹剑', operating_mana_cost: 300,
      definition: { applicable_stage: 'void_early', effect_stage: 'void_late', implement: {
        attack_bonus: .29, defense_pierce: 59375, operating_mana_fraction: .03,
        special_effect: 'item.effect.implement.rated_piercing',
      } } }, 'equipment', i18n.t)
    expect(blade.effects).toContainEqual({ label: i18n.t('entryDetails.fact.defense_pierce'), value: '59375' })
    expect(blade.requirements).toContainEqual({ label: i18n.t('entryDetails.fact.effect_stage'), value: i18n.t('combatWindow.realm.void', { stage: i18n.t('combatWindow.realmStage.late') }) })
    expect(blade.costs).toContainEqual({ label: i18n.t('entryDetails.fact.operatingMana'), value: '300' })
    const robe = cultivationPresentation({ id: 'robe', name: '云纹袍', effective_strain_reduction: .057,
      definition: { effect_stage: 'void_late', armor: { strain_reduction: .15, special_effect: 'item.effect.armor.rated_stability' } } }, 'equipment', i18n.t)
    expect(robe.effects).toContainEqual({ label: i18n.t('entryDetails.fact.rated_strain_reduction'), value: '15%' })
    expect(robe.effects).toContainEqual({ label: i18n.t('entryDetails.fact.effective_strain_reduction'), value: '5.7%' })
    expect(JSON.stringify([blade, robe])).not.toContain('item.effect.')
  })
}
