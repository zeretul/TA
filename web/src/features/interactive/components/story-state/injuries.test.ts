import { describe, expect, it } from 'vitest'
import { INJURY_DISPLAY_NAME, INJURY_LABEL_KEY, injuryFieldKind, lastingBenefits, stateInjuries } from './injuries'

/**
 * These tests hold the 伤势 row to T21.
 *
 * The player is shown one row for their wounds, not two parameters. What the
 * story wrote decides what the row says; the burden floor only decides whether
 * a wound exists at all. Nothing here may invent a wound, and nothing may read
 * a withheld sheet as an uninjured one.
 */

describe('stateInjuries', () => {
  it('says 无伤 when nothing is recorded on either field', () => {
    const report = stateInjuries(0, [], false)
    expect(report).toEqual({ wounded: false, lines: [], withheld: false })
  })

  it('reads the story line as the wound, name and recovery condition included', () => {
    const report = stateInjuries(30, ['经脉内伤｜不能施展重招｜调息三日'], false)
    expect(report.wounded).toBe(true)
    expect(report.lines).toEqual(['经脉内伤｜不能施展重招｜调息三日'])
  })

  it('keeps a wound whose burden number the backend withheld', () => {
    // The floor is private, but the story already said the character is hurt.
    // Withholding the number must not hide the wound the player was told about.
    const report = stateInjuries(undefined, ['左臂骨裂｜不能使剑｜静养十日'], true)
    expect(report.wounded).toBe(true)
    expect(report.withheld).toBe(false)
    expect(report.lines).toEqual(['左臂骨裂｜不能使剑｜静养十日'])
  })

  it('reports a wound the floor records but the story has not described', () => {
    const report = stateInjuries(40, [], false)
    expect(report).toEqual({ wounded: true, lines: [], withheld: false })
  })

  it('never reads a withheld sheet as 无伤', () => {
    // An opponent whose sheet the player has not seen is not an uninjured
    // opponent. §3.1 keeps "not found out" apart from "nothing there".
    const report = stateInjuries(undefined, undefined, true)
    expect(report.withheld).toBe(true)
    expect(report.wounded).toBe(false)
  })

  it('names the wound from an object entry the story wrote without prose', () => {
    const report = stateInjuries(0, [{ 名称: '经脉内伤', 影响: '不能施展重招' }], false)
    expect(report.lines).toEqual(['经脉内伤｜不能施展重招'])
  })

  it('ignores a blessing written in the same list', () => {
    // 持续状态 carries both. A pill's bonus is not a wound, and printing it
    // under 伤势 would tell the player they were hurt by their own medicine.
    const effects = [
      { 名称: '增攻药', 属性: '攻击', 数值: 12, 同类: '增攻药', 剩余轮次: 3 },
      '经脉内伤｜不能施展重招｜调息三日',
    ]
    expect(stateInjuries(0, effects, false).lines).toEqual(['经脉内伤｜不能施展重招｜调息三日'])
  })

  it('reads a negative attribute entry as a wound', () => {
    const report = stateInjuries(0, [{ 名称: '气血亏虚', 属性: '生命上限', 数值: -15 }], false)
    expect(report.wounded).toBe(true)
    expect(report.lines).toEqual(['气血亏虚｜生命上限 -15%'])
  })
})

describe('lastingBenefits', () => {
  it('leaves the blessings and drops the wounds', () => {
    const effects = [
      { 名称: '增攻药', 属性: '攻击', 数值: 12 },
      '经脉内伤｜不能施展重招｜调息三日',
      { 名称: '封灵符', 属性: '灵力上限', 数值: -10 },
    ]
    expect(lastingBenefits(effects)).toEqual([{ 名称: '增攻药', 属性: '攻击', 数值: 12 }])
  })

  it('passes a value that is not a list through untouched', () => {
    expect(lastingBenefits('—')).toBe('—')
  })
})

describe('injuryFieldKind', () => {
  it('recognises the two wound fields by the pack name and by the settled id', () => {
    expect(injuryFieldKind({ name: '余伤负担下限' })).toBe('floor')
    expect(injuryFieldKind({ id: 'state.burden_floor' })).toBe('floor')
    expect(injuryFieldKind({ name: '持续状态' })).toBe('effects')
    expect(injuryFieldKind({ path: 'state.effects' })).toBe('effects')
  })

  it('does not claim a field that only looks similar', () => {
    expect(injuryFieldKind({ name: '经脉负担' })).toBeNull()
    expect(injuryFieldKind({ name: '冷却状态' })).toBeNull()
    expect(injuryFieldKind(undefined)).toBeNull()
  })
})

describe('the row label', () => {
  it('is 伤势, not the parameter it stands for', () => {
    expect(INJURY_DISPLAY_NAME).toBe('伤势')
  })

  it('is a field key, and the caption is looked up separately', () => {
    // The constant names the row in the layout, the change lookup and the
    // summary ranking. What the player reads comes from INJURY_LABEL_KEY, so an
    // English sheet says Injury instead of one Chinese word.
    expect(INJURY_LABEL_KEY).not.toBe(INJURY_DISPLAY_NAME)
    expect(INJURY_LABEL_KEY).toBe('storyStage.state.injury.label')
  })
})
