import { describe, expect, it } from 'vitest'
import { addStagedAction, composeStagedRequest, type StagedAction } from './staged-actions'

function action(overrides: Partial<StagedAction> = {}): StagedAction {
  return { id: 'act_1', message: '明早一起出发。', ...overrides }
}

describe('staged actions', () => {
  it('keeps each arrangement and ignores an exact repeat', () => {
    const first = addStagedAction([], action())
    expect(first).toHaveLength(1)
    // Pressing the same button twice is one intention, not two.
    expect(addStagedAction(first, action({ id: 'act_2' }))).toHaveLength(1)
  })

  it('treats different words as a different arrangement', () => {
    const staged = addStagedAction(addStagedAction([], action()), action({ id: 'act_2', message: '今夜就走。' }))
    expect(staged.map((item) => item.message)).toEqual(['明早一起出发。', '今夜就走。'])
  })

  it('ignores an empty arrangement', () => {
    expect(addStagedAction([], action({ message: '   ' }))).toHaveLength(0)
  })

  it('joins the staged plans and the typed line into one request', () => {
    const staged = [action(), action({ id: 'act_2', message: '带上解毒药。' })]
    expect(composeStagedRequest(staged, ' 我先行一步。 ').message).toBe('明早一起出发。\n带上解毒药。\n我先行一步。')
  })

  it('sends the typed line alone when nothing is staged', () => {
    expect(composeStagedRequest([], '我先行一步。').message).toBe('我先行一步。')
    expect(composeStagedRequest([], '我先行一步。').playerLetter).toBeUndefined()
  })

  it('carries a letter as its own field rather than inside the words', () => {
    const letter = { character_id: 'liuqing', body: '请替我留一间房。' }
    const staged = addStagedAction([], action({ message: '我写信给柳青。', playerLetter: letter }))
    const request = composeStagedRequest(staged, '')
    expect(request.message).toBe('我写信给柳青。')
    expect(request.playerLetter).toEqual(letter)
    // The body is a formal record, so it must not be duplicated into the text.
    expect(request.message).not.toContain('请替我留一间房')
  })

  it('does not merge two letters to the same person into one entry', () => {
    const first = addStagedAction([], action({ message: '第一封信', playerLetter: { character_id: 'a', body: '甲' } }))
    const second = addStagedAction(first, action({ id: 'act_2', message: '第二封信', playerLetter: { character_id: 'b', body: '乙' } }))
    expect(second).toHaveLength(2)
  })
})
