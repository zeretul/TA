import { afterEach, expect, it } from 'vitest'
import i18n from '@/i18n'
import { cultivationPresentation } from './presentations'

afterEach(async () => { await i18n.changeLanguage('zh-CN') })
const treasures = [
  ['treasure.attack.middle', 'item.treasure.attack_middle.name', '飞叶梭', 'Flying Leaf Spindle'],
  ['treasure.defense.middle', 'item.treasure.defense_middle.name', '叠纹护盘', 'Layered Ward Disc'],
  ['treasure.support.upper.effect', 'item.treasure.support_upper_effect.name', '寻迹佩', 'Trailfinding Pendant'],
]
for (const language of ['zh-CN', 'en-US']) {
  it(`renders each newly illustrated treasure name in ${language}`, async () => {
    await i18n.changeLanguage(language)
    for (const [id, nameKey, zh, en] of treasures) {
      const view = cultivationPresentation({ id, name: id, name_key: nameKey }, 'equipment', i18n.t)
      expect(view.name).toBe(language === 'zh-CN' ? zh : en)
      expect(view.name).not.toContain('treasure.')
    }
  })
}
