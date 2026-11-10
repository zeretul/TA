import { describe, expect, it } from 'vitest'
import type { CharacterExchange, CharacterMatter } from '../../types'
import { buildNegotiatedAction, composeNegotiatedMessage } from './negotiated-action'

function matter(overrides: Partial<CharacterMatter> = {}): CharacterMatter {
  return {
    id: 'mat_1',
    character_id: 'liuqing',
    kind: 'promise',
    status: 'open',
    statement: '柳青答应同行探路。',
    world_time: '第3日',
    ...overrides,
  }
}

function exchange(overrides: Partial<CharacterExchange> = {}): CharacterExchange {
  return {
    id: 'xch_1',
    story_id: 'story-1',
    branch_id: 'main',
    character_id: 'liuqing',
    speaker: 'player',
    channel: 'private',
    text: '明早一起去洞府。',
    world_time: '第3日',
    ...overrides,
  }
}

describe('negotiated action affordance', () => {
  it('offers an action only when something was actually agreed', () => {
    // A discussion, a misunderstanding and a change of attitude are all part of
    // the situation, but none of them is a commitment the player can act on.
    expect(buildNegotiatedAction([matter({ kind: 'misunderstanding' }), matter({ id: 'mat_2', kind: 'attitude' })], [exchange()])).toBeNull()
    expect(buildNegotiatedAction([], [exchange()])).toBeNull()
    expect(buildNegotiatedAction(undefined, undefined)).toBeNull()
  })

  it('does not offer an action for a matter that is already settled', () => {
    expect(buildNegotiatedAction([matter({ status: 'settled' })], [exchange()])).toBeNull()
    expect(buildNegotiatedAction([matter({ status: 'abandoned' })], [exchange()])).toBeNull()
  })

  // §T24: 询价、报价、考虑是否买、推荐、客套 are ordinary talk. A price quoted
  // is not a purchase, a recommendation is not a request, and neither is a
  // commitment anyone can act on. None of them writes a matter, so none of them
  // may produce the affordance — even when the player's last line is a question
  // about the price.
  it('does not offer an action for a price, a quote, a recommendation or small talk', () => {
    const asking = [exchange({ text: '这柄剑多少钱？' })]
    const quoted = [exchange({ id: 'xch_2', speaker: 'character', text: '三十块灵石，不还价。' }), ...asking]
    const pondering = [exchange({ text: '我再想想，买不买还没定。' })]
    const recommended = [exchange({ text: '你觉得我该先学哪一门？' })]
    const pleasantries = [exchange({ text: '今日叨扰了，改日再叙。' })]

    for (const lines of [asking, quoted, pondering, recommended, pleasantries]) {
      expect(buildNegotiatedAction([], lines)).toBeNull()
      // A discussion leaves no matter behind; a disclosure is not a deal either.
      expect(buildNegotiatedAction([matter({ kind: 'disclosure', statement: '他说铺子里还剩两柄。' })], lines)).toBeNull()
    }
  })

  // §T24: NPC单方面允许、提出一个机会 — the permission is the other side's, not the
  // player's. The player may act on it, but the message that reaches the story
  // must be the player's own words, never the permission restated as their
  // undertaking.
  it('keeps a granted permission on the other side of the agreement', () => {
    const permission = matter({ kind: 'permission', statement: '守卒准你今夜入城。' })
    const action = buildNegotiatedAction([permission], [exchange({ text: '今夜进城要多少钱？' })])
    expect(action?.agreed.map((item) => item.id)).toEqual(['mat_1'])
    expect(composeNegotiatedMessage(action!, '我今夜就进城。')).toBe('我今夜就进城。')
  })

  it('offers an action for a real agreement, and only for that one', () => {
    const promise = matter({ id: 'mat_p', kind: 'promise', statement: '柳青答应明早在渡口等你。' })
    const accepted = matter({ id: 'mat_a', kind: 'action', statement: '你答应替柳青把药送到渡口。' })

    const fromPromise = buildNegotiatedAction([promise, matter({ id: 'mat_d', kind: 'disclosure', statement: '她说船家认得路。' })], [exchange()])
    expect(fromPromise?.agreed.map((item) => item.id)).toEqual(['mat_p'])

    // A task the player accepted is their own commitment, so it counts too.
    const fromAccepted = buildNegotiatedAction([accepted], [exchange({ text: '好，我答应把药送到。' })])
    expect(fromAccepted?.agreed.map((item) => item.id)).toEqual(['mat_a'])
  })

  it('does not turn an in-transit letter into an immediate agreement', () => {
    const letter = matter({
      kind: 'action',
      statement: '一封寄给柳四的信：请替我留一间房。',
      letter: { body: '请替我留一间房。', sent_at_seconds: 10, earliest_delivery_seconds: 100 },
    })
    expect(buildNegotiatedAction([letter], [exchange()])).toBeNull()
  })

  it('keeps the agreed half read-only and prefills only the player plan', () => {
    const action = buildNegotiatedAction(
      [matter(), matter({ id: 'mat_2', kind: 'permission', statement: '守卒准你今夜入城。' })],
      [exchange({ text: '明早一起去洞府。' })],
    )
    expect(action).not.toBeNull()
    expect(action?.agreed.map((item) => item.id)).toEqual(['mat_1', 'mat_2'])
    expect(action?.plan).toBe('明早一起去洞府。')
  })

  it('prefills from the player\'s own last line, not the character\'s', () => {
    const action = buildNegotiatedAction(
      [matter()],
      [
        exchange({ id: 'xch_1', speaker: 'player', text: '你怎么看？' }),
        exchange({ id: 'xch_2', speaker: 'character', text: '我去，你探路。' }),
        exchange({ id: 'xch_3', speaker: 'player', text: '那就明早出发。' }),
      ],
    )
    expect(action?.plan).toBe('那就明早出发。')
  })

  it('sends only the player\'s own plan down the main story chain', () => {
    const action = buildNegotiatedAction([matter()], [exchange()])
    expect(action).not.toBeNull()
    if (!action) return
    const message = composeNegotiatedMessage(action, '我明早带好解毒药，在洞府外等你。')
    expect(message).toBe('我明早带好解毒药，在洞府外等你。')
    // The NPC's own promise is never restated in the player's message: it is
    // already in the narrator's cast block, read from the same journal, so an
    // edited summary cannot hand them a new obligation.
    expect(message).not.toContain('柳青答应同行探路')
  })

  it('falls back to the agreed statements when the player cleared their plan', () => {
    const action = buildNegotiatedAction([matter()], [exchange()])
    if (!action) throw new Error('expected an affordance')
    expect(composeNegotiatedMessage(action, '   ')).toBe('柳青答应同行探路。')
  })
})
