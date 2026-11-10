import { describe, expect, it } from 'vitest'
import { isAgeInRange, normalizeAgeRange, rollAge } from './creation-age'

describe('creation age', () => {
  it('keeps a declared range as given and repairs an inverted one', () => {
    expect(normalizeAgeRange([20, 28])).toEqual([20, 28])
    expect(normalizeAgeRange([28, 20])).toEqual([20, 28])
  })

  it('falls back to a usable range when the pack declares none', () => {
    expect(normalizeAgeRange(undefined)).toEqual([20, 28])
    expect(normalizeAgeRange([0, 0])).toEqual([20, 28])
  })

  it('always rolls inside the range', () => {
    for (let index = 0; index < 400; index += 1) {
      const age = rollAge([20, 28])
      expect(age).toBeGreaterThanOrEqual(20)
      expect(age).toBeLessThanOrEqual(28)
    }
  })

  it('never rerolls to the age already showing', () => {
    for (let index = 0; index < 400; index += 1) {
      expect(rollAge([20, 28], 23)).not.toBe(23)
    }
  })

  it('stays on the only value when the range has one', () => {
    expect(rollAge([21, 21], 21)).toBe(21)
  })

  it('accepts only whole numbers inside the range', () => {
    expect(isAgeInRange('20', [20, 28])).toBe(true)
    expect(isAgeInRange('28', [20, 28])).toBe(true)
    expect(isAgeInRange('19', [20, 28])).toBe(false)
    expect(isAgeInRange('29', [20, 28])).toBe(false)
    expect(isAgeInRange('23.5', [20, 28])).toBe(false)
    expect(isAgeInRange('', [20, 28])).toBe(false)
  })
})
