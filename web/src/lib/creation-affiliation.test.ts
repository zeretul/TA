import { describe, expect, it } from 'vitest'
import type { StoryPackCreationOption, StoryPackCreationOrigin } from './api-client/story-packs'
import {
  affiliationKindForOrigin,
  affiliationOptionsFor,
  defaultAffiliationFor,
  defaultLocationFor,
  deriveFamilyName,
  isLocationAllowed,
  labelAffiliationOptions,
  locationOptionsFor,
} from './creation-affiliation'

const origins: StoryPackCreationOrigin[] = [
  { id: 'loose', name: '散修', affiliation_kind: 'none' },
  { id: 'sect', name: '宗门弟子', affiliation_kind: 'sect' },
  { id: 'family', name: '修仙世家', affiliation_kind: 'family' },
  { id: 'custom', name: '自定义出身' },
]

const affiliations: StoryPackCreationOption[] = [
  { id: 'none', name: '无宗门（散修）', kind: 'none', default: true },
  { id: 'own', name: '自己的家族', kind: 'family' },
  { id: 'yue.huangfeng', name: '黄枫谷', kind: 'sect', country: '越国' },
  { id: 'yue.yan', name: '燕家', kind: 'family', country: '越国' },
  { id: 'zijin.lu', name: '地方陆氏', kind: 'family', country: '紫金国' },
]

const locations = [
  { id: 'yue.market', name: '公共坊市', country: '越国', default: true },
  { id: 'yue.huangfeng.approach', name: '黄枫谷对外入口', country: '越国' },
  { id: 'yue.yan.approach', name: '燕家对外入口', country: '越国' },
  { id: 'zijin.lu.approach', name: '地方陆氏对外入口', country: '紫金国' },
]

describe('creation affiliation', () => {
  it('offers only the affiliations the origin can belong to', () => {
    expect(affiliationOptionsFor(origins, affiliations, 'loose').map((option) => option.id)).toEqual(['none'])
    expect(affiliationOptionsFor(origins, affiliations, 'sect').map((option) => option.id)).toEqual(['yue.huangfeng'])
    expect(affiliationOptionsFor(origins, affiliations, 'family').map((option) => option.id)).toEqual([
      'own',
    ])
  })

  it('leaves a custom origin free to pair with anything', () => {
    expect(affiliationOptionsFor(origins, affiliations, 'custom')).toHaveLength(affiliations.length)
  })

  it("inherits the affiliation constraint of a custom origin's selected state basis", () => {
    const withTemplates = origins.map(origin => ({ ...origin, template_id: `${origin.id}-basis` }))
    expect(affiliationKindForOrigin(withTemplates, 'custom', 'sect-basis')).toBe('sect')
    expect(affiliationOptionsFor(withTemplates, affiliations, 'custom', 'sect-basis').map(option => option.id)).toEqual(['yue.huangfeng'])
    expect(defaultAffiliationFor(withTemplates, affiliations, 'custom', 'family-basis')).toBe('own')
    expect(affiliationOptionsFor(withTemplates, affiliations, 'custom', 'loose-basis').map(option => option.id)).toEqual(['none'])
  })

  it('starts a family heir on their own household, not a family this world has', () => {
    expect(defaultAffiliationFor(origins, affiliations, 'family')).toBe('own')
    expect(defaultAffiliationFor(origins, affiliations, 'sect')).toBe('yue.huangfeng')
    expect(defaultAffiliationFor(origins, affiliations, 'loose')).toBe('none')
  })

  it('gives a loose cultivator every place and everyone else only their own', () => {
    expect(locationOptionsFor(locations, affiliations, 'none').map((place) => place.id)).toEqual([
      'yue.market',
      'yue.huangfeng.approach',
      'yue.yan.approach',
      'zijin.lu.approach',
    ])
    expect(locationOptionsFor(locations, affiliations, 'yue.huangfeng').map((place) => place.id)).toEqual([
      'yue.market',
      'yue.huangfeng.approach',
    ])
  })

  it('keeps a family heir out of another country', () => {
    expect(locationOptionsFor(locations, affiliations, 'zijin.lu').map((place) => place.id)).toEqual([
      'zijin.lu.approach',
    ])
  })

  it('falls back to a place the new affiliation allows', () => {
    expect(defaultLocationFor(locations, affiliations, 'none')).toBe('yue.market')
    expect(defaultLocationFor(locations, affiliations, 'yue.huangfeng')).toBe('yue.market')
    expect(defaultLocationFor(locations, affiliations, 'zijin.lu')).toBe('zijin.lu.approach')
  })

  it('reports whether the player keeps their place after switching affiliation', () => {
    expect(isLocationAllowed(locations, affiliations, 'none', 'zijin.lu.approach')).toBe(true)
    expect(isLocationAllowed(locations, affiliations, 'yue.huangfeng', 'zijin.lu.approach')).toBe(false)
    expect(isLocationAllowed(locations, affiliations, 'yue.huangfeng', 'yue.huangfeng.approach')).toBe(true)
  })

  it('does not constrain anything when the pack states no kind', () => {
    const plain: StoryPackCreationOrigin[] = [{ id: 'only', name: '唯一出身' }]
    expect(affiliationOptionsFor(plain, affiliations, 'only')).toHaveLength(affiliations.length)
    expect(affiliationOptionsFor(undefined, affiliations, 'sect')).toHaveLength(affiliations.length)
  })
})

describe('own household name', () => {
  it('names the household after a one-character surname', () => {
    expect(deriveFamilyName('叶流云')).toBe('叶氏')
  })

  it('keeps a two-character surname whole', () => {
    expect(deriveFamilyName('欧阳明')).toBe('欧阳氏')
    expect(deriveFamilyName('司马长安')).toBe('司马氏')
  })

  it('leaves an unnamed character without a household', () => {
    expect(deriveFamilyName('')).toBe('')
    expect(deriveFamilyName('   ')).toBe('')
  })

  it('relabels only the own-household option', () => {
    const labelled = labelAffiliationOptions(affiliations, '叶流云')
    expect(labelled.find((option) => option.id === 'own')?.name).toBe('叶氏')
    expect(labelled.find((option) => option.id === 'yue.yan')?.name).toBe('燕家')
    expect(labelAffiliationOptions(affiliations, '')[1].name).toBe('自己的家族')
  })
})
