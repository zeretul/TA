import { afterEach, expect, it } from 'vitest'
import i18n from '@/i18n'
import { adventurePresentations, cultivationPresentation } from './presentations'
import type { AdventureDetailsResponse, CultivationEntry } from './types'

afterEach(async () => { await i18n.changeLanguage('zh-CN') })

for (const language of ['zh-CN', 'en-US']) {
  it(`preserves generated material names and reports non-consumed tools in ${language}`, async () => {
    await i18n.changeLanguage(language)
    const entry: CultivationEntry = {
      id: 'gen.craft.test', name: '归潮护符法', source: '亲手改良的绘符笔记', mastered: true,
      material_names: { paper: '潮纹纸', ward: '归潮护符', brush: '青玉符笔' },
      definition: {
        minimum_stage: 'qi_3', minutes_per_batch: 30, loss_chance: 0.1,
        inputs: [{ def_id: 'paper', quantity: 2 }], outputs: [{ def_id: 'ward', quantity: 1 }],
        required_tools: ['brush'], failure_cost: 'consume_inputs', required_location: '静室',
      },
    }
    const before = JSON.stringify(entry)
    const result = cultivationPresentation(entry, 'craft', i18n.t)
    expect(result.costs).toContainEqual({ label: i18n.t('entryDetails.fact.materials'), value: '潮纹纸 × 2' })
    expect(result.effects).toContainEqual({ label: i18n.t('entryDetails.fact.products'), value: '归潮护符 × 1' })
    expect(result.requirements).toContainEqual({ label: i18n.t('entryDetails.fact.requiredTool'), value: '青玉符笔' })
    expect(result.costs).toContainEqual({ label: i18n.t('entryDetails.fact.failureCost'), value: i18n.t('entryDetails.failureConsumesInputs') })
    expect(result.costs.some(fact => fact.value.includes('青玉符笔'))).toBe(false)
    expect(JSON.stringify(entry)).toBe(before)
  })

  it(`shows actual recipe time, skill mastery, required asset and fuel in ${language}`, async () => {
    await i18n.changeLanguage(language)
    const data: AdventureDetailsResponse = {
      head: 'committed', family: 'space', actor: { skills: { engineering: 3 }, inventory: { wire: 1 } },
      skills: [{ id: 'engineering', name: '舰船工程' }],
      items: [{ id: 'wire', name: '导线' }],
      craft_methods: [{ id: 'gen.craft.navigation', name: '导航器改制', skill_id: 'engineering', skill_rank: 3,
        hours: 6, materials: { wire: 2 }, output_id: 'device', output_count: 1,
        required_asset: 'ship', fuel_cost: 9,
        item_names: { wire: '导线', ship: '维修艇', device: '导航器' }, skill_name: '舰船工程',
      }],
    }
    const before = JSON.stringify(data)
    const result = adventurePresentations(data, i18n.t)[0]
    expect(result.requirements).toContainEqual({ label: i18n.t('entryDetails.fact.requiredAsset'), value: '维修艇' })
    expect(result.requirements).toContainEqual({ label: i18n.t('entryDetails.fact.requiredSkillRank'), value: '3' })
    expect(result.costs).toContainEqual({ label: i18n.t('entryDetails.fact.craftingHours'), value: '6' })
    expect(result.costs).toContainEqual({ label: i18n.t('entryDetails.fact.fuelCost'), value: '9' })
    expect(result.costs.some(fact => fact.label === i18n.t('entryDetails.fact.trainingHours'))).toBe(false)
    expect(adventurePresentations(data, i18n.t).some(row => row.id === 'ship' || row.id === 'device')).toBe(false)
    expect(JSON.stringify(data)).toBe(before)
  })
}

it('shows incomplete study without advertising zero-cost casting or mastery', () => {
  const entry: CultivationEntry = { id: 'studying', name: 'Still learning', mastered: false, learning_minutes: 15,
    required_learning_minutes: 1440, castable: false, reason_key: 'entryDetails.learningIncomplete', mana_cost: 0, burden_cost: 0 }
  const result = cultivationPresentation(entry, 'skills', i18n.t)
  expect(result.status).toContainEqual({ label: i18n.t('entryDetails.fact.mastered'), value: i18n.t('entryDetails.no') })
  expect(result.status).toContainEqual({ label: i18n.t('entryDetails.fact.learning_minutes'), value: '15' })
  expect(result.requirements).toContainEqual({ label: i18n.t('entryDetails.fact.requiredLearningMinutes'), value: '1440' })
  expect(result.status).toContainEqual({ label: i18n.t('entryDetails.fact.reason'), value: i18n.t('entryDetails.learningIncomplete') })
  expect(result.costs).toEqual([])
})


it('reports the default material loss for a recipe with failed batches', () => {
  const result = cultivationPresentation({ id: 'ward', name: '护符', definition: { loss_chance: 0.1 } }, 'craft', i18n.t)
  expect(result.costs).toContainEqual({ label: i18n.t('entryDetails.fact.failureCost'), value: i18n.t('entryDetails.failureConsumesInputs') })
})
