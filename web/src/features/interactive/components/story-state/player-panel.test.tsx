import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import type { ActorStateField } from '../../types'
import { playerFacingFieldLabelKey, playerFacingFieldName } from './field-layout'
import { LedgerFieldView } from './ledger-fields'
import { buildLedgerGroups, type LedgerFieldEntry } from './model'

/**
 * These tests hold the character panel to U3's display rules.
 *
 * The backend withholds a private field for an actor the player is not playing,
 * so the panel receives an absence rather than a value. §3.1 keeps that absence
 * apart from "nothing here": a withheld dossier is drawn and reads as 未知,
 * while a field the pack does not declare private that holds nothing has nothing
 * to say and is not drawn at all. Printing one placeholder per unfilled slot is
 * what turns the state area into a page of "暂无内容". The panel also has to
 * show the wording a player reads — a stage key is storage, not prose — and must
 * not print a pool's maximum twice.
 */

// The entry a caller hands the panel. StoryStateLedger is the one that renames
// a field and marks the caption as translatable, so the helper does the same
// rather than inventing a shape the panel never actually receives.
function entry(field: ActorStateField, value: unknown, withheld = false): LedgerFieldEntry {
  return {
    id: field.name,
    label: playerFacingFieldName(field, field.name),
    labelKey: playerFacingFieldLabelKey(field),
    field,
    value,
    withheld,
  }
}

function renderField(item: ReturnType<typeof buildLedgerGroups>[number]['fields'][number]) {
  return render(
    <table>
      <tbody>
        <tr>
          <td>
            <LedgerFieldView item={item} />
          </td>
        </tr>
      </tbody>
    </table>,
  )
}

describe('player character panel', () => {
  it('reads a withheld dossier as unknown rather than as an empty slot', () => {
    const field: ActorStateField = { name: '已学功法', type: 'object', visibility: 'private' }
    const groups = buildLedgerGroups([entry(field, null, true)], [])
    renderField(groups[0].fields[0])
    expect(screen.getByText('未知')).toBeInTheDocument()
    expect(screen.queryByText('—')).toBeNull()
  })

  it('leaves an unfilled open field out instead of printing a placeholder', () => {
    const field: ActorStateField = { name: '当前处境', type: 'string' }
    const groups = buildLedgerGroups([entry(field, null)], [])
    expect(groups).toEqual([])
  })

  it('keeps an open field that holds something, and only that field', () => {
    const empty: ActorStateField = { name: '当前处境', type: 'string' }
    const filled: ActorStateField = { name: '性格与背景', type: 'string' }
    const groups = buildLedgerGroups([entry(empty, null), entry(filled, '沉稳寡言')], [])
    const fields = groups.flatMap((group) => group.fields)
    expect(fields.map((field) => field.label)).toEqual(['性格与背景'])
  })

  it('shows a stored stage key as the wording a player reads', () => {
    const field: ActorStateField = { name: '当前境界', type: 'enum', value_format: 'stage' }
    const groups = buildLedgerGroups([entry(field, 'mortal')], [])
    renderField(groups[0].fields[0])
    expect(screen.getByText('凡人')).toBeInTheDocument()
    expect(screen.queryByText('mortal')).toBeNull()
  })

  it('prints a pool maximum once, inside the pool row', () => {
    const pool: ActorStateField = { name: '生命', type: 'number', display: 'stat', max_field: '生命上限' }
    const maximum: ActorStateField = { name: '生命上限', type: 'number', display: 'stat' }
    const groups = buildLedgerGroups([entry(pool, 12), entry(maximum, 40)], [])
    const fields = groups.flatMap((group) => group.fields)
    expect(fields.map((field) => field.label)).toEqual(['生命'])
    renderField(fields[0])
    expect(screen.getByText('12 / 40')).toBeInTheDocument()
  })
})

/**
 * T21 asks the page to show one 伤势 row instead of two parameters.
 *
 * 余伤负担下限 is the number the rules hold the burden above and 持续状态 mixes a
 * wound in with a pill's bonus. Neither reads on its own, and printing both
 * makes the player compare two things to work out they are one.
 */
describe('the 伤势 row', () => {
  const floorField: ActorStateField = { name: '余伤负担下限', type: 'number', display: 'stat', min: 0, max: 100, default: 0 }
  const effectsField: ActorStateField = { name: '持续状态', type: 'list', group: '战斗与状态' }

  function fields(entries: LedgerFieldEntry[]) {
    return buildLedgerGroups(entries, []).flatMap((group) => group.fields)
  }

  it('replaces the burden floor with a row named 伤势', () => {
    const shown = fields([entry(floorField, 30), entry(effectsField, ['经脉内伤｜不能施展重招｜调息三日'])])
    expect(shown.map((field) => field.label)).toEqual(['伤势'])
    expect(shown[0].injuries?.lines).toEqual(['经脉内伤｜不能施展重招｜调息三日'])
  })

  it('says 无伤 for a character who is not hurt', () => {
    const shown = fields([entry(floorField, 0), entry(effectsField, [])])
    expect(shown.map((field) => field.label)).toEqual(['伤势'])
    renderField(shown[0])
    expect(screen.getByText('无伤')).toBeInTheDocument()
    // The raw parameter is not something the player is asked to read.
    expect(screen.queryByText('0')).toBeNull()
  })

  it('says the wound is not yet described when only the floor records one', () => {
    const shown = fields([entry(floorField, 40), entry(effectsField, [])])
    renderField(shown[0])
    expect(screen.getByText('有伤，尚未说清')).toBeInTheDocument()
  })

  it('leaves a blessing where the player expects it instead of under 伤势', () => {
    const effects = [
      { 名称: '增攻药', 属性: '攻击', 数值: 12, 同类: '增攻药', 剩余轮次: 3 },
      '经脉内伤｜不能施展重招｜调息三日',
    ]
    const shown = fields([entry(floorField, 30), entry(effectsField, effects)])
    const wound = shown.find((field) => field.label === '伤势')
    const lasting = shown.find((field) => field.label === '持续状态')
    expect(wound?.injuries?.lines).toEqual(['经脉内伤｜不能施展重招｜调息三日'])
    expect(lasting?.value).toEqual([{ 名称: '增攻药', 属性: '攻击', 数值: 12, 同类: '增攻药', 剩余轮次: 3 }])
  })

  it('keeps the wound row where the pack filed the floor', () => {
    // A pack that files 余伤负担下限 beside 生命 should still show the wound
    // there, not move it to wherever 持续状态 happened to sit.
    const before: ActorStateField = { name: '灵石', type: 'number' }
    const shown = fields([entry(before, 12), entry(floorField, 30), entry(effectsField, ['经脉内伤｜不能施展重招'])])
    expect(shown.map((field) => field.label)).toEqual(['灵石', '伤势'])
  })

  it('never reads a withheld wound as 无伤', () => {
    const shown = fields([entry(floorField, null, true), entry({ ...effectsField, visibility: 'private' }, null, true)])
    // 持续状态 keeps its own row: a withheld list may hold blessings as well as
    // wounds, and hiding it would hide the blessings too.
    const wound = shown[0]
    expect(wound.label).toBe('伤势')
    renderField(wound)
    expect(screen.getByText('未知')).toBeInTheDocument()
    expect(screen.queryByText('无伤')).toBeNull()
  })
})

/**
 * The wound row is the one caption this interface wrote itself.
 *
 * Every other row is captioned with the pack's own field name, and a pack's
 * names are the language the story is written in. 伤势 is not a field: it is a
 * word the page invented for a row it folded together, so it is the one caption
 * that has to follow the interface language.
 */
describe('the 伤势 row caption in English', () => {
  afterEach(async () => { await i18next.changeLanguage('zh-CN') })

  const floorField: ActorStateField = { name: '余伤负担下限', type: 'number', display: 'stat', min: 0, max: 100, default: 0 }
  const effectsField: ActorStateField = { name: '持续状态', type: 'list', group: '战斗与状态' }

  it('reads as Injury rather than carrying a Chinese caption into the English sheet', async () => {
    await i18next.changeLanguage('en-US')
    const shown = buildLedgerGroups([entry(floorField, 30), entry(effectsField, ['经脉内伤｜不能施展重招'])], []).flatMap((group) => group.fields)
    renderField(shown[0])
    expect(screen.getByText('Injury')).toBeInTheDocument()
    expect(screen.queryByText('伤势')).toBeNull()
    // The field key the row is known by does not change with the language.
    expect(shown[0].label).toBe('伤势')
  })

  it('says Uninjured, and does not fall back to the Chinese 无伤', async () => {
    await i18next.changeLanguage('en-US')
    const shown = buildLedgerGroups([entry(floorField, 0), entry(effectsField, [])], []).flatMap((group) => group.fields)
    renderField(shown[0])
    expect(screen.getByText('Uninjured')).toBeInTheDocument()
    expect(screen.queryByText('无伤')).toBeNull()
  })

  it('translates the stat renderer’s own label, not just the wrapper’s', async () => {
    // The stat body draws its own heading, so a label resolved only in the outer
    // view leaves 施招负担 sitting in the middle of an English sheet.
    await i18next.changeLanguage('en-US')
    const burden: ActorStateField = { name: '经脉负担', type: 'number', display: 'stat', min: 0, max: 100, default: 0 }
    const shown = buildLedgerGroups([entry(burden, 0)], []).flatMap((group) => group.fields)
    renderField(shown[0])
    expect(screen.getByText('Casting strain')).toBeInTheDocument()
    expect(screen.queryByText('施招负担')).toBeNull()
  })
})

/**
 * T20 keeps 功法 as its own section and asks it to say which method is 主修.
 *
 * The record already carries that answer — 运转 together with 已掌握 is what the
 * settlement prices a character from — and 功法与专精 is the prose a character was
 * created with before the record existed. Showing both would let the two
 * disagree, and the player would have no way to tell which one the rules used.
 */
describe('the 功法 row', () => {
  const recordField: ActorStateField = { name: '功法', type: 'object', path: 'cultivation.techniques', visibility: 'private' }
  const proseField: ActorStateField = { name: '功法与专精', type: 'string', path: 'state.cultivation_method' }

  function fields(entries: LedgerFieldEntry[]) {
    return buildLedgerGroups(entries, []).flatMap((group) => group.fields)
  }

  it('shows the record instead of the prose it superseded', () => {
    const shown = fields([
      entry(proseField, '燕家家传功法，强攻，良好。'),
      entry(recordField, { 燕氏心法: { 定义: 'method.warm_origin', 掌握程度: '已掌握', 运转: '运转' } }),
    ])
    expect(shown.map((field) => field.label)).toEqual(['功法'])
    expect(shown[0].methods?.lines.map((line) => line.name)).toEqual(['燕氏心法'])
  })

  it('says 主修 for the circulating method and 兼修 for a stopped one', () => {
    const shown = fields([entry(recordField, {
      燕氏心法: { 定义: 'method.warm_origin', 掌握程度: '已掌握', 运转: '运转' },
      静心诀: { 定义: 'method.clear_mind', 掌握程度: '已掌握', 运转: '停止' },
    })])
    renderField(shown[0])
    expect(screen.getByText('主修')).toBeInTheDocument()
    expect(screen.getByText('兼修')).toBeInTheDocument()
  })

  it('keeps the prose when the record is still empty', () => {
    // Every character created before the record existed carries the field with
    // its empty default, and the settlement prices them from the prose. Dropping
    // it would leave those characters with no methods shown at all.
    const shown = fields([entry(proseField, '无名散修功法，均衡，普通。'), entry(recordField, {})])
    expect(shown.map((field) => field.label)).toEqual(['功法与专精'])
    expect(shown[0].value).toBe('无名散修功法，均衡，普通。')
  })

  it('does not print the definition ID the settlement casts from', () => {
    const shown = fields([entry(recordField, {
      燕氏心法: { 定义: 'method.warm_origin', 版本: 1, 掌握程度: '已掌握', 运转: '运转', 描述: '气从丹田一路烧上来。' },
    })])
    renderField(shown[0])
    expect(screen.getByText('气从丹田一路烧上来。')).toBeInTheDocument()
    expect(screen.queryByText(/method\.warm_origin/)).toBeNull()
    expect(screen.queryByText('1')).toBeNull()
  })

  it('leaves the group named 功法 rather than opening a second 技能 list', () => {
    const shown = buildLedgerGroups([entry(recordField, {
      燕氏心法: { 定义: 'method.warm_origin', 掌握程度: '已掌握', 运转: '运转' },
    })], [])
    expect(shown.map((group) => group.key)).toEqual(['methods'])
  })
})

/**
 * T22 hides the narrator's own plan for the next beat.
 *
 * 可承接钩子 is the seam the next turn continues from, not something the
 * protagonist has found out. The journal keeps it — it is what the narrator
 * reads to continue — and the change digest already drops it. The panel has to
 * drop it too, or the player is handed the next beat before it is played.
 */
describe('the narrator\'s continuation hook', () => {
  it('is not drawn on the world tab even though the journal holds it', () => {
    const hook: ActorStateField = { path: 'scene.continuation_hook', name: '可承接钩子', type: 'string' }
    const event: ActorStateField = { path: 'scene.current_event', name: '当前事件', type: 'string' }
    const groups = buildLedgerGroups([
      entry(event, '钱守信等着叶流云一句准话。'),
      entry(hook, '钱守信站在柜台后等着，要叶流云一句话：接不接这趟押车。'),
    ], [])
    const shown = groups.flatMap((group) => group.fields)
    expect(shown.map((field) => field.label)).toEqual(['当前事件'])
  })

  it('is still recognised by name when a save carries no path', () => {
    const groups = buildLedgerGroups([{ id: '可承接钩子', label: '可承接钩子', value: '柜台后的人还等着' }], [])
    expect(groups).toEqual([])
  })
})

describe('the trade settlement\'s merchant ledger', () => {
  // 商贾 holds each shop's prepared cash position and buyback rate. T22 says a
  // price is not a wallet, and the design's seven world groups have no slot for
  // a shop's books, so the row is read by the rules and never drawn.
  it('is not drawn on the world tab', () => {
    const merchants: ActorStateField = { path: 'economy.merchants', name: '商贾', type: 'object', group: '经济' }
    const time: ActorStateField = { path: 'scene.current_time', name: '当前时间', type: 'string' }
    const groups = buildLedgerGroups([
      entry(time, '入夜，坊市摊子已收。'),
      entry(merchants, { 东街丹铺: { 现银: 606, 回购率: 600 } }),
    ], [])
    const shown = groups.flatMap((group) => group.fields)
    expect(shown.map((field) => field.label)).toEqual(['当前时间'])
  })

  it('is still recognised by name when a save carries no path', () => {
    const groups = buildLedgerGroups([{ id: '商贾', label: '商贾', value: { 东街丹铺: { 现银: 606 } } }], [])
    expect(groups).toEqual([])
  })
})


it('shows partial skills and methods as study, with names rather than registry identities', () => {
  const fields = buildLedgerGroups([
    entry({ name: '已学功法', type: 'object', path: 'abilities.learned' }, {
      'gen.fire_bolt.one': { 名称: '松火诀', 掌握程度: '修习中', 已修习分钟: 360 },
    }),
    entry({ name: '功法', type: 'object', path: 'cultivation.techniques' }, {
      '松风温元诀 [gen.technique.method.warm_origin.one]': { 名称: '松风温元诀', 掌握程度: '修习中', 已修习分钟: 360 },
    }),
  ], []).flatMap(group => group.fields)
  for (const item of fields) renderField(item)
  expect(screen.getByText('松火诀')).toBeInTheDocument()
  expect(screen.getByText('松风温元诀')).toBeInTheDocument()
  expect(screen.getAllByText('修习中')).toHaveLength(2)
  expect(screen.getAllByText('已研习 360 分钟')).toHaveLength(2)
  expect(screen.queryByText(/gen\./)).toBeNull()
  expect(screen.queryByText('招式')).toBeNull()
})
