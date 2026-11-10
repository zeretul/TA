import { describe, expect, it } from 'vitest'
import { isMethodRecordField, isMethodSpecialisationField, readMethods } from './methods'

/**
 * These tests hold the 功法 row to T20's naming and grouping decision.
 *
 * The design keeps 功法 as its own section and asks it to say, plainly, which
 * method is being cultivated right now. The record already carries that — 运转
 * together with 已掌握 is exactly what the settlement prices a character from —
 * so the row reads the record rather than inventing a second notion of 主修.
 */

const record = (entries: Record<string, unknown>) => entries

describe('readMethods', () => {
  it('reads a circulating, mastered method as 主修', () => {
    const report = readMethods(record({
      烈阳诀: { 定义: 'method.warm_origin', 版本: 1, 掌握程度: '已掌握', 运转: '运转' },
    }))
    expect(report.structured).toBe(true)
    expect(report.lines).toEqual([{ name: '烈阳诀', kind: 'main', description: '', source: '' }])
  })

  it('reads a mastered method left stopped as 兼修', () => {
    // Held but not circulating: it contributes nothing to the settlement, which
    // is the whole difference the design asks the row to show.
    const report = readMethods(record({
      寒泉诀: { 定义: 'method.cold_spring', 版本: 1, 掌握程度: '已掌握', 运转: '停止' },
    }))
    expect(report.lines[0]).toMatchObject({ name: '寒泉诀', kind: 'secondary' })
  })

  it('does not file a method still being learned under either', () => {
    // 修习中 is not mastered, so it is not circulating either. Calling it 兼修
    // would claim a method the character cannot use yet.
    const report = readMethods(record({
      归元诀: { 定义: 'method.origin_guard', 版本: 1, 掌握程度: '修习中', 运转: '运转' },
    }))
    expect(report.lines[0]).toMatchObject({ name: '归元诀', kind: 'learning' })
  })

  it('carries the story’s own description and origin without the definition ID', () => {
    const report = readMethods(record({
      烈阳诀: {
        定义: 'method.warm_origin',
        版本: 2,
        掌握程度: '已掌握',
        运转: '运转',
        描述: '气从丹田一路烧上来，指尖发烫。',
        来历: '师父在火房外传的。',
      },
    }))
    expect(report.lines[0]).toEqual({
      name: '烈阳诀',
      kind: 'main',
      description: '气从丹田一路烧上来，指尖发烫。',
      source: '师父在火房外传的。',
    })
    // The definition ID is what the rules cast from; it is not a word the
    // player reads.
    expect(JSON.stringify(report)).not.toContain('method.warm_origin')
  })

  it('reports an empty record as no record at all, so the prose keeps its place', () => {
    // A fresh character carries the field with its empty default. That is not
    // an authoritative record, and the settlement falls back to 功法与专精 for
    // exactly this case.
    const report = readMethods({})
    expect(report).toEqual({ structured: false, lines: [], withheld: false })
  })

  it('never claims a withheld record is empty', () => {
    // An opponent whose sheet the player has not seen has methods the player
    // does not know about, not no methods.
    const report = readMethods(undefined, true)
    expect(report.withheld).toBe(true)
    expect(report.structured).toBe(false)
  })
})

describe('method field recognition', () => {
  it('recognises the record by name and by path', () => {
    expect(isMethodRecordField({ name: '功法' })).toBe(true)
    expect(isMethodRecordField({ path: 'cultivation.techniques' })).toBe(true)
    expect(isMethodRecordField({ name: '已学功法' })).toBe(false)
  })

  it('recognises the superseded prose by name and by path', () => {
    expect(isMethodSpecialisationField({ name: '功法与专精' })).toBe(true)
    expect(isMethodSpecialisationField({ path: 'state.cultivation_method' })).toBe(true)
    expect(isMethodSpecialisationField({ name: '功法' })).toBe(false)
  })
})


it('shows the persistent generated method name and actual study minutes', () => {
  const report = readMethods({ '松风温元诀 [gen.technique.method.warm_origin.abc]': {
    名称: '松风温元诀', 定义: 'gen.technique.method.warm_origin.abc', 掌握程度: '修习中', 已修习分钟: 360,
  } })
  expect(report.lines[0]).toMatchObject({ name: '松风温元诀', kind: 'learning', learningMinutes: 360 })
  expect(JSON.stringify(report)).not.toContain('gen.technique.')
})
