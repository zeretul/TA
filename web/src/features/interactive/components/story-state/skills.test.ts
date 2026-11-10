import { describe, expect, it } from 'vitest'
import { isSkillAbilityField, isSkillRecordField, readSkills } from './skills'

/**
 * These tests hold the 技能 list to T20's merging decision.
 *
 * The design merges the rules' technique record and the pack's free-form record
 * of knacks into one list, keeps them distinguishable, and asks that the same
 * ability appear once. What must not happen is the merge changing what either
 * record means: a knack gains no combat numbers, and a registered technique is
 * never demoted to prose.
 */

const record = (entries: Record<string, unknown>) => entries

describe('readSkills', () => {
  it('reads a registered technique with the description the settlement wrote', () => {
    const report = readSkills(record({
      火弹术: {
        定义: 'fire_bolt',
        版本: 1,
        掌握程度: '熟练',
        描述: '一团火从指间弹出。',
        来历: '师父传的。',
      },
    }), {})
    expect(report.lines).toEqual([
      { name: '火弹术', kind: 'technique', description: '一团火从指间弹出。', source: '师父传的。' },
    ])
    // The definition ID is what the rules cast from; it is not a word the
    // player reads.
    expect(JSON.stringify(report)).not.toContain('fire_bolt')
  })

  it('keeps a written-down knack as a knack rather than a combat technique', () => {
    const report = readSkills({}, record({
      医术: { 类型: '技艺', 效果: '能处理常见的刀伤与风寒', 来源: '跟镇上郎中学的' },
    }))
    expect(report.lines).toEqual([
      { name: '医术', kind: 'ability', description: '技艺，能处理常见的刀伤与风寒', source: '跟镇上郎中学的' },
    ])
  })

  it('lists the registered techniques before the knacks', () => {
    // A reader looking for a technique to cast should not scan past the trades.
    const report = readSkills(record({ 火弹术: { 定义: 'fire_bolt' } }), record({ 医术: {} }))
    expect(report.lines.map((line) => line.kind)).toEqual(['technique', 'ability'])
  })

  it('shows one ability once, taken from the registered record', () => {
    // Both records can name the same thing. The registered entry is the one the
    // rules price a cast from, so it is the one that survives.
    const report = readSkills(
      record({ 火弹术: { 定义: 'fire_bolt', 描述: '一团火从指间弹出。' } }),
      record({ 火弹术: { 类型: '术法', 效果: '掌心冒火' }, 医术: {} }),
    )
    expect(report.lines.map((line) => line.name)).toEqual(['火弹术', '医术'])
    expect(report.lines[0].description).toBe('一团火从指间弹出。')
  })

  it('reports both records empty as an empty list rather than as an unknown one', () => {
    expect(readSkills({}, {})).toEqual({ lines: [], withheld: false })
  })

  it('never claims a withheld record is empty', () => {
    // An opponent whose sheet the player has not seen has skills the player does
    // not know about, not no skills.
    const report = readSkills(undefined, undefined, true)
    expect(report.withheld).toBe(true)
    expect(report.lines).toEqual([])
  })

  it('reads a record written as bare prose as a named entry', () => {
    const report = readSkills({}, record({ 追踪: '看脚印能认出走过几天' }))
    expect(report.lines[0]).toMatchObject({ name: '追踪', kind: 'ability', description: '看脚印能认出走过几天' })
  })
})

describe('skill field recognition', () => {
  it('recognises the technique record by name and by path', () => {
    expect(isSkillRecordField({ name: '已学功法' })).toBe(true)
    expect(isSkillRecordField({ path: 'abilities.learned' })).toBe(true)
    expect(isSkillRecordField({ name: '技能与能力' })).toBe(false)
  })

  it('recognises the free-form record by name and by path', () => {
    expect(isSkillAbilityField({ name: '技能与能力' })).toBe(true)
    expect(isSkillAbilityField({ path: 'abilities.records' })).toBe(true)
    expect(isSkillAbilityField({ name: '已学功法' })).toBe(false)
  })
})


describe('generated study records', () => {
  it('uses the explicit name and distinguishes incomplete study without collapsing same names', () => {
    const report = readSkills({
      'gen.fire_bolt.one': { 名称: '松火诀', 定义: 'gen.fire_bolt.one', 掌握程度: '修习中', 已修习分钟: 360, 来源: '符师指点' },
      'gen.fire_bolt.two': { 名称: '松火诀', 定义: 'gen.fire_bolt.two', 掌握程度: '已学会' },
    }, {})
    expect(report.lines).toHaveLength(2)
    expect(report.lines[0]).toMatchObject({ name: '松火诀', kind: 'learning', learningMinutes: 360, source: '符师指点' })
    expect(report.lines[1]).toMatchObject({ name: '松火诀', kind: 'technique' })
    expect(JSON.stringify(report)).not.toContain('gen.')
  })
})
