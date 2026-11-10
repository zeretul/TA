import { describe, expect, it } from 'vitest'
import type { ActorStateField } from '../../types'
import { resolveFieldMax, resolveNumberMeter } from './number-meter'
import { buildLedgerGroups } from './model'
import { isBoundedNumberField } from './field-layout'

/**
 * The cultivation pools are the only settled numbers whose maximum is another
 * settled number. These tests cover the sibling bound that makes 生命 200 / 200
 * readable as a proportion without hard-coding a maximum that a late-stage
 * character would overflow.
 */

const poolField = (name: string, maxField: string): ActorStateField => ({
  name,
  type: 'number',
  min: 0,
  max_field: maxField,
})

describe('resolveFieldMax', () => {
  it('prefers a fixed maximum', () => {
    const field: ActorStateField = { name: '怒气', type: 'number', min: 0, max: 100 }
    expect(resolveFieldMax(field, () => 999)).toBe(100)
  })

  it('reads the declared sibling when there is no fixed maximum', () => {
    expect(resolveFieldMax(poolField('生命', '生命上限'), (name) => (name === '生命上限' ? 200 : undefined))).toBe(200)
  })

  it('leaves the field unbounded when the sibling is missing', () => {
    // Showing "40 / 100" for a character whose real maximum is 200 would be a
    // worse lie than showing 40, so an absent sibling resolves to nothing.
    expect(resolveFieldMax(poolField('生命', '生命上限'), () => undefined)).toBeNull()
  })

  it('leaves the field unbounded when the sibling is not a number', () => {
    expect(resolveFieldMax(poolField('生命', '生命上限'), () => '200')).toBeNull()
  })

  it('leaves the field unbounded when no resolver is supplied', () => {
    expect(resolveFieldMax(poolField('生命', '生命上限'))).toBeNull()
  })
})

describe('resolveNumberMeter with a sibling bound', () => {
  it('draws a real proportion from the settled maximum', () => {
    const meter = resolveNumberMeter(poolField('生命', '生命上限'), 50, () => 200)
    expect(meter).not.toBeNull()
    expect(meter?.max).toBe(200)
    expect(meter?.widthPercent).toBe(25)
  })

  it('returns nothing when the sibling cannot be resolved', () => {
    expect(resolveNumberMeter(poolField('生命', '生命上限'), 50, () => undefined)).toBeNull()
  })

  it('still draws a plain bounded number', () => {
    const field: ActorStateField = { name: '怒气', type: 'number', min: 0, max: 100 }
    expect(resolveNumberMeter(field, 40)?.widthPercent).toBe(40)
  })
})

describe('isBoundedNumberField', () => {
  it('accepts a declared sibling bound', () => {
    // Without this the pool would fall to the inline renderer and lose its meter.
    expect(isBoundedNumberField(poolField('生命', '生命上限'), 200)).toBe(true)
  })

  it('rejects an unbounded number', () => {
    expect(isBoundedNumberField({ name: '修为', type: 'number' }, 200)).toBe(false)
  })
})

describe('buildLedgerGroups sibling lookup', () => {
  it('resolves a maximum that lands in a different group than the pool', () => {
    const groups = buildLedgerGroups(
      [
        { id: 'panel.health', label: '生命', field: poolField('生命', '生命上限'), value: 50 },
        { id: 'state.health_max', label: '生命上限', field: { name: '生命上限', type: 'number', group: 'details' }, value: 200 },
      ],
      [],
    )
    const health = groups.flatMap((group) => group.fields).find((item) => item.label === '生命')
    expect(health?.resolveSibling('生命上限')).toBe(200)
    expect(resolveNumberMeter(health!.field!, health!.value, health!.resolveSibling)?.max).toBe(200)
  })
})
