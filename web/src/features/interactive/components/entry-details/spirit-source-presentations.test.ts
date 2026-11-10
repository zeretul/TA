import { afterEach, expect, it } from 'vitest'
import i18n from '@/i18n'
import { cultivationPresentation } from './presentations'

afterEach(async () => { await i18n.changeLanguage('zh-CN') })
for (const language of ['zh-CN', 'en-US']) {
  it(`shows the physical source for a higher method in ${language}`, async () => {
    await i18n.changeLanguage(language)
    const method = cultivationPresentation({ id: 'method.void_unification', name: 'Five-element Unification',
      definition: { required_manual_id: 'manual.spirit.void' } }, 'techniques', i18n.t)
    expect(method.requirements).toContainEqual({ label: i18n.t('entryDetails.fact.required_manual_id'), value: i18n.t('item.spirit.void_manual.name') })
    expect(JSON.stringify(method)).not.toContain('manual.spirit.')
    expect(JSON.stringify(method)).not.toContain('item.spirit.')
  })
}
