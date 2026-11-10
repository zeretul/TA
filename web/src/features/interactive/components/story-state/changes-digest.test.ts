import { describe, expect, it } from 'vitest'
import { buildChangeDigest, DIGEST_LIMIT, sameValue } from './changes-digest'
import type { ActorStateEntry, StoryStateChange } from './model'

/**
 * These tests hold the change summary to T20's rules.
 *
 * The summary is not a list of the turn's writes. A turn rewrites most of its
 * fields every time, so a list of writes buries the one fact the player needs
 * and reports restatements as news. What the player reads is the difference
 * between the state the turn started from and the state it settled into, which
 * the backend records on each write as previous_value.
 */

function op(field: string, value: unknown, previousValue: unknown, extra: Partial<StoryStateChange> = {}): StoryStateChange {
  return { id: `${field}:${String(value)}`, actorId: 'protagonist', path: field, op: 'set', value, previousValue, ...extra }
}

function labels(changes: StoryStateChange[]) {
  return buildChangeDigest(changes, []).map((entry) => entry.label)
}

describe('turn change digest', () => {
  it('reads a settled balance as the difference it really was', () => {
    const digest = buildChangeDigest([op('灵石', 148, 150)], [])
    expect(digest).toHaveLength(1)
    expect(digest[0].change.kind).toBe('delta')
    expect(digest[0].change.delta).toBe(-2)
  })

  it('reports nothing for a write that landed on the value already held', () => {
    expect(buildChangeDigest([op('灵石', 150, 150)], [])).toEqual([])
  })

  it('reports nothing for a rewrite of the scene prose', () => {
    const changes = [
      op('当前事件', '新的紧张局势', '旧的紧张局势'),
      op('当前处境', '被围住', '安全'),
      op('当前时间', '第三日辰时', '第三日卯时'),
    ]
    expect(buildChangeDigest(changes, [])).toEqual([])
  })

  it('leaves the opening bootstrap out of the summary', () => {
    const changes = [
      op('生命', 304, undefined, { sourceKind: 'state_schema_initialization' }),
      op('灵石', 150, undefined, { sourceKind: 'state_schema_initialization' }),
    ]
    expect(buildChangeDigest(changes, [])).toEqual([])
  })

  it('leaves a person walking on stage out of the summary', () => {
    // Two NPCs entering a scene have their whole sheet written at once, and most
    // of it is zeroes and empty lists. Reporting "经脉负担 0" for each of them
    // filled the row and pushed the turn's real outcome off it.
    const changes = [
      op('经脉负担', 0, undefined, { actorId: '钱守信' }),
      op('余伤负担下限', 0, undefined, { actorId: '钱守信' }),
      op('持续状态', [], undefined, { actorId: '钱守信' }),
      op('主法器', {}, undefined, { actorId: '钱守信' }),
      op('灵石', 0, undefined, { actorId: '王婆子' }),
      op('当前详细地点', '越国坊市', '落霞村'),
    ]
    const digest = buildChangeDigest(changes, [
      ['钱守信', { name: '钱守信' }],
      ['王婆子', { name: '王婆子' }],
    ])
    expect(digest.map((entry) => entry.label)).toEqual(['越国坊市'])
  })

  it('still reports a real loss to zero', () => {
    // Emptying a field is the opposite of restating it: the stones were spent.
    const digest = buildChangeDigest([op('灵石', 0, 12)], [])
    expect(digest).toHaveLength(1)
    expect(digest[0].change.kind).toBe('delta')
    expect(digest[0].change.delta).toBe(-12)
  })

  it('reads a move as arriving at the place, not as a field name', () => {
    const digest = buildChangeDigest([op('当前详细地点', '青石坊市', '落霞村')], [])
    expect(digest[0].label).toBe('青石坊市')
    expect(digest[0].verb).toBe('storyStage.state.digest.arrived')
    expect(digest[0].change.text).toBe('')
  })

  it('does not credit the player with a shop that the world opened', () => {
    // 商贾 is the story's own ledger of who trades where. Diffing it as a record
    // the player gained reads as "获得东街丹铺", which the player did not do —
    // they bought two pills from it.
    const changes = [
      { id: 's', actorId: 'story', path: '商贾', op: 'set', value: { 东街丹铺: { 现银: 606 } }, previousValue: {} },
      op('灵石', 144, 150),
    ]
    const digest = buildChangeDigest(changes, [['protagonist', { name: '叶流云', role: 'protagonist' }]])
    expect(digest.map((entry) => entry.label)).toEqual(['灵石'])
  })

  it('still reports what the player themself now holds', () => {
    // The same shape on the player's own sheet is an acquisition they made.
    const changes = [
      op('重要物品', { 回春丹: { 数量: 4 } }, { 回春丹: { 数量: 2 } }),
    ]
    const digest = buildChangeDigest(changes, [['protagonist', { name: '叶流云', role: 'protagonist' }]])
    expect(digest[0].label).toBe('回春丹')
    expect(digest[0].change.kind).toBe('delta')
    expect(digest[0].change.delta).toBe(2)
  })

  it('reads a whole-record rewrite as the entries that really moved', () => {
    const previous = { 回春丹: { 定义: 'pill.restore.health.lower', 数量: 2 }, 引气丹: { 定义: 'pill.mana', 数量: 1 } }
    const next = { 回春丹: { 定义: 'pill.restore.health.lower', 数量: 3 }, 火弹术: { 定义: 'skill.fireball' } }
    const digest = buildChangeDigest([op('重要物品', next, previous)], [])
    expect(digest.map((entry) => `${entry.label}:${entry.change.kind}`).sort())
      .toEqual(['回春丹:delta', '引气丹:removed', '火弹术:added'].sort())
  })

  it('keeps a wound above a purchase and a purchase above an unrelated field', () => {
    const changes = [
      op('性格与背景', '更沉稳', '沉稳'),
      op('灵石', 148, 150),
      op('生命', 180, 304),
    ]
    expect(labels(changes).slice(0, 2)).toEqual(['生命', '灵石'])
  })

  it('puts a breakthrough above every purchase', () => {
    const changes = [op('灵石', 148, 150), op('当前境界', 'qi_13', 'qi_12')]
    expect(labels(changes)[0]).toBe('当前境界')
  })

  it('never shows more than the summary can read', () => {
    const changes = [
      op('生命', 180, 304),
      op('经脉负担', 40, 10),
      op('当前境界', 'qi_13', 'qi_12'),
      op('灵石', 148, 150),
      op('重要物品', { 回春丹: { 数量: 3 } }, { 回春丹: { 数量: 2 } }),
      op('已学功法', { 火弹术: {} }, {}),
      op('当前详细地点', '青石坊市', '落霞村'),
    ]
    expect(buildChangeDigest(changes, [])).toHaveLength(DIGEST_LIMIT)
  })

  it('names the art in a learned entry so the line reads as a sentence', () => {
    const digest = buildChangeDigest([op('已学功法', { 火弹术: {} }, {})], [])
    expect(digest[0].label).toBe('火弹术')
    expect(digest[0].verb).toBe('storyStage.state.digest.learned')
  })

  it('says whose money moved when a trade touches both sides', () => {
    // A purchase writes the player's 灵石 down and the seller's up. Both are the
    // same field name, so without an owner the row reads "灵石 -4 · 灵石 +4".
    const actors: ActorStateEntry[] = [
      ['protagonist', { name: '叶流云', role: 'protagonist' }],
      ['npc_wangpozi', { name: '王婆子', role: 'npc' }],
    ]
    const changes: StoryStateChange[] = [
      { id: 'a', actorId: 'protagonist', path: '灵石', op: 'set', value: 146, previousValue: 150 },
      { id: 'b', actorId: 'npc_wangpozi', path: '灵石', op: 'set', value: 4, previousValue: 0 },
    ]
    const digest = buildChangeDigest(changes, actors)
    expect(digest.map((entry) => entry.actor)).toEqual([undefined, '王婆子'])
    expect(digest.map((entry) => entry.change.delta)).toEqual([-4, 4])
  })

  it('names the seller even when the pack keys them by their own name', () => {
    // The built-in pack stores people under their Chinese names, so the id and
    // the name are the same string. That is still a name the player recognises,
    // and dropping it would leave the trade reading as one field moving twice.
    const actors: ActorStateEntry[] = [['王婆子', { name: '王婆子', role: '符箓摊主' }]]
    const changes: StoryStateChange[] = [
      { id: 'b', actorId: '王婆子', path: '灵石', op: 'set', value: 4, previousValue: 0 },
    ]
    expect(buildChangeDigest(changes, actors)[0].actor).toBe('王婆子')
  })

  it('leaves the owner off when the actor has no name to print', () => {
    const changes: StoryStateChange[] = [
      { id: 'b', actorId: 'npc_unknown', path: '灵石', op: 'set', value: 4, previousValue: 0 },
    ]
    expect(buildChangeDigest(changes, [])[0].actor).toBeUndefined()
  })

  it('compares stored values structurally', () => {
    expect(sameValue({ a: 1, b: [2, 3] }, { b: [2, 3], a: 1 })).toBe(true)
    expect(sameValue({ a: 1 }, { a: 2 })).toBe(false)
    expect(sameValue(undefined, null)).toBe(false)
  })
})

/**
 * T21: a wound is one fact written by two fields, so the summary says it once.
 *
 * 余伤负担下限 is the number the rules hold the burden above and 持续状态 holds the
 * story's account of the wound. Reporting them as two writes gave the player
 * "余伤负担下限 30 · 持续状态 已更新" and left them to work out that the two
 * lines were one thing.
 */
describe('the wound in the change summary', () => {
  it('reports a new wound by name, not as two field writes', () => {
    const changes = [
      op('余伤负担下限', 30, 0),
      op('持续状态', ['经脉内伤｜不能施展重招｜调息三日'], []),
    ]
    const digest = buildChangeDigest(changes, [])
    expect(digest).toHaveLength(1)
    expect(digest[0].label).toBe('经脉内伤')
    expect(digest[0].verb).toBe('storyStage.state.digest.wounded')
    expect(digest[0].change.tone).toBe('negative')
  })

  it('never puts the burden number on the summary', () => {
    // The floor has no meaning outside the recovery rules, and the player is
    // not the one applying them. A wound the floor records is reported as the
    // wound it is, never as the number.
    const digest = buildChangeDigest([op('余伤负担下限', 30, 0)], [])
    expect(digest).toHaveLength(1)
    expect(digest[0].label).toBe('伤势')
    expect(digest[0].change.kind).not.toBe('delta')
  })

  it('says the character was wounded when the story has not named it yet', () => {
    const digest = buildChangeDigest([op('余伤负担下限', 30, 0), op('持续状态', [], [])], [])
    expect(digest).toHaveLength(1)
    expect(digest[0].verb).toBe('storyStage.state.digest.woundedUnnamed')
  })

  it('says a wound healed only when the floor came back down with it', () => {
    const changes = [
      op('余伤负担下限', 0, 30),
      op('持续状态', [], ['经脉内伤｜不能施展重招｜调息三日']),
    ]
    const digest = buildChangeDigest(changes, [])
    expect(digest).toHaveLength(1)
    expect(digest[0].verb).toBe('storyStage.state.digest.healed')
    expect(digest[0].change.tone).toBe('positive')
  })

  it('does not claim a recovery from an emptied list alone', () => {
    // The story stopped describing the wound; the burden floor may still be
    // holding the character back. The panel shows the wound either way, so a
    // summary claiming recovery would contradict it.
    const digest = buildChangeDigest([op('持续状态', [], ['经脉内伤｜不能施展重招'])], [])
    expect(digest).toEqual([])
  })

  it('still reports a blessing taken in the same turn', () => {
    // 持续状态 carries both, so a list that gained a pill is a fact the player
    // needs even when no wound moved. It reads as the pill's name: 持续状态 is
    // the rules' container, and "持续状态 已更新" says nothing about what changed.
    const digest = buildChangeDigest([
      op('持续状态', [{ 名称: '增攻药', 属性: '攻击', 数值: 12 }], []),
    ], [])
    expect(digest.map((entry) => entry.label)).toEqual(['增攻药'])
    expect(digest[0].verb).toBe('storyStage.state.digest.gained')
  })

  it('reports a blessing that wore off by name, not as an emptied field', () => {
    // The same list losing the pill is a loss the player should see. Without the
    // name it would read as "持续状态 已更新", which is the wording this digest
    // exists to avoid.
    const digest = buildChangeDigest([
      op('持续状态', [], [{ 名称: '增攻药', 属性: '攻击', 数值: 12 }]),
    ], [])
    expect(digest.map((entry) => entry.label)).toEqual(['增攻药'])
    expect(digest[0].change.kind).toBe('removed')
  })

  it('says nothing when the list was rewritten with the same entries', () => {
    // 持续状态 is rewritten whole on every touch, so an unchanged list is not
    // news — reporting it would fill the row with a restatement.
    const digest = buildChangeDigest([
      op('持续状态', ['经脉内伤｜不能施展重招', '护心符｜抵御心魔｜三日'], ['经脉内伤｜不能施展重招', '护心符｜抵御心魔｜三日']),
    ], [])
    expect(digest).toEqual([])
  })

  it('names only the wound the turn added, not the one already carried', () => {
    const changes = [
      op('余伤负担下限', 40, 30),
      op('持续状态', ['经脉内伤｜不能施展重招', '左臂骨裂｜不能使剑'], ['经脉内伤｜不能施展重招']),
    ]
    const digest = buildChangeDigest(changes, [])
    expect(digest).toHaveLength(1)
    expect(digest[0].label).toBe('左臂骨裂')
  })

  it('names whose wound it is when it is not the protagonist', () => {
    const actors: ActorStateEntry[] = [
      ['protagonist', { name: '叶流云', role: 'protagonist' }],
      ['npc_wangpozi', { name: '王婆子', role: 'npc' }],
    ]
    const digest = buildChangeDigest([
      { id: 'a', actorId: 'npc_wangpozi', path: '余伤负担下限', op: 'set', value: 20, previousValue: 0 },
      { id: 'b', actorId: 'npc_wangpozi', path: '持续状态', op: 'set', value: ['掌骨挫伤｜运符不便｜静养'], previousValue: [] },
    ], actors)
    expect(digest).toHaveLength(1)
    expect(digest[0].actor).toBe('王婆子')
    expect(digest[0].label).toBe('掌骨挫伤')
  })
})


describe('study change wording', () => {
  const key = 'gen.technique.method.warm_origin.abc'
  const entry = (minutes: number, mastery = '修习中', circulating = '停止') => ({ 名称: '松风温元诀', 定义: key, 掌握程度: mastery, 已修习分钟: minutes, 运转: circulating })
  it.each([
    [undefined, entry(0), 'registeredStudy'],
    [undefined, entry(360), 'studied'],
    [entry(0), entry(360), 'studied'],
    [entry(360), entry(14400, '已掌握'), 'learned'],
    [entry(14400, '已掌握'), entry(14400, '已掌握', '运转'), 'circulating'],
    [entry(14400, '已掌握', '运转'), entry(14400, '已掌握'), 'stoppedCirculating'],
  ])('uses actual before/after mastery, not record existence', (before, after, verb) => {
    const digest = buildChangeDigest([op('功法', { [key]: after }, before ? { [key]: before } : {})], [])
    expect(digest[0].label).toBe('松风温元诀')
    expect(digest[0].verb).toBe(`storyStage.state.digest.${verb}`)
  })
  it('does not call a partially studied skill learned', () => {
    const digest = buildChangeDigest([op('已学功法', { 'gen.fire_bolt.one': { 名称: '松火诀', 掌握程度: '修习中', 已修习分钟: 360 } }, {})], [])
    expect(digest[0]).toMatchObject({ label: '松火诀', verb: 'storyStage.state.digest.studied' })
  })
})


it('does not invent study time when only a partial record’s name or source is filled', () => {
  const before = { 定义: 'gen.fire_bolt.one', 掌握程度: '修习中', 已修习分钟: 360 }
  const after = { ...before, 名称: '松火诀', 来源: '补全来源记录' }
  const digest = buildChangeDigest([op('已学功法', { key: after }, { key: before })], [])
  expect(digest[0].label).toBe('松火诀')
  expect(digest[0].verb).toBeUndefined()
})
