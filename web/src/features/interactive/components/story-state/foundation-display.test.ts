import { describe, expect, it } from 'vitest'
import { foundationDisplayText } from './foundation-display'

/**
 * The aptitude line is written once at creation and then settled, so the
 * characters already being played carry the old phrasing. It read like a note
 * about what the rules would allow — "修行资质普通。一般悟性一般。兵器方向偏拳脚，
 * 只是经历与机会的倾向，不代表已学会对应高阶技法。" — rather than a sentence about
 * the character.
 *
 * The stored value cannot be rewritten: the cultivation rules read the tier word
 * out of it. So the display rewrites it, and these cover both directions — that
 * the old line becomes the new one, and that a line already in the new wording
 * or one the narrator wrote itself is left alone.
 */

describe('the old aptitude phrasing reads as a sentence about the character', () => {
  it('turns the shipped old line into the current one', () => {
    const old = '三灵根（金、木、土）；修行资质普通。一般悟性一般。兵器方向偏拳脚，只是经历与机会的倾向，不代表已学会对应高阶技法。'
    expect(foundationDisplayText(old)).toBe('三灵根（金、木、土），资质普通，悟性一般，偏好拳脚。')
  })

  it('keeps the tier word the rules read', () => {
    // The cultivation rules parse 极佳/优秀/良好/较差/普通 out of this line. A
    // display that dropped the tier would show a character the rules do not
    // agree with.
    for (const tier of ['较差', '普通', '良好', '优秀', '极佳']) {
      const old = `单灵根（火）；修行资质${tier}。一般悟性一般。兵器方向偏剑，只是经历与机会的倾向，不代表已学会对应高阶技法。`
      expect(foundationDisplayText(old)).toContain(`资质${tier}`)
    }
  })

  it('keeps the insight word as the pack worded it', () => {
    const old = '双灵根（水、木）；修行资质良好。一般悟性敏锐。兵器方向偏符，只是经历与机会的倾向，不代表已学会对应高阶技法。'
    expect(foundationDisplayText(old)).toContain('悟性敏锐')
  })

  it('leaves the current wording untouched', () => {
    const current = '单灵根（火），资质普通，悟性一般，偏好拳脚。'
    expect(foundationDisplayText(current)).toBe(current)
  })

  it('leaves a line the narrator wrote in its own words alone', () => {
    // A character whose foundation the story described rather than templated
    // must not be rewritten into a template.
    const own = '单火灵根，自幼体弱，靠药汤压住寒毒。'
    expect(foundationDisplayText(own)).toBe(own)
  })

  it('handles a line with no weapon preference', () => {
    const old = '三灵根（金、木、土）；修行资质普通。一般悟性一般。'
    expect(foundationDisplayText(old)).toBe('三灵根（金、木、土），资质普通，悟性一般。')
  })

  it('returns an empty value as it came', () => {
    expect(foundationDisplayText('')).toBe('')
    expect(foundationDisplayText('   ')).toBe('')
  })
})
