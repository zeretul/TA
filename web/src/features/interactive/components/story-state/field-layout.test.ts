import { describe, expect, it } from 'vitest'
import type { ActorStateField } from '../../types'
import { playerFacingGroupKey, resolveStateFieldLayout } from './field-layout'

/**
 * These tests hold the display regrouping to T22's grouping table.
 *
 * The pack declares 当前处境 and 出场与位置 under 战斗与状态, and that declaration is
 * what the rules and the saved layout key on. The design files both under 概况
 * instead: what is pressing on the protagonist and where a character last was
 * are the first rows a reader wants, and neither one is a fight. Because the
 * regrouping happens on the way to the page, it also reaches saves frozen with
 * the old declaration — which is the point of doing it here rather than by
 * editing the preset.
 */

const field = (overrides: Partial<ActorStateField>): ActorStateField => ({
  path: '',
  name: '',
  ...overrides,
} as ActorStateField)

describe('playerFacingGroupKey', () => {
  it('files the protagonist\'s present situation under the overview', () => {
    expect(playerFacingGroupKey(field({ path: 'current.situation', name: '当前处境', group: '战斗与状态' }))).toBe('overview')
  })

  it('files where a tracked character last was under the overview', () => {
    expect(playerFacingGroupKey(field({ path: 'current.presence_location', name: '出场与位置', group: '战斗与状态' }))).toBe('overview')
  })

  it('finds both rows in a save that only carries the pack\'s own names', () => {
    // A story frozen before these paths existed carries the name and not the
    // path, and the row still has to land where the design puts it.
    expect(playerFacingGroupKey(field({ name: '当前处境' }))).toBe('overview')
    expect(playerFacingGroupKey(field({ name: '出场与位置' }))).toBe('overview')
  })

  it('leaves an opponent\'s lifecycle row where the pack declared it', () => {
    // 存续与位置 is a different row with a different question — whether the thing
    // is still there at all — and the design does not move it.
    expect(playerFacingGroupKey(field({ path: 'current.lifecycle_location', name: '存续与位置' }))).toBeNull()
  })

  it('keeps the four regroupings the design already had', () => {
    expect(playerFacingGroupKey(field({ path: 'abilities.records' }))).toBe('skills')
    expect(playerFacingGroupKey(field({ path: 'state.cultivation_method' }))).toBe('methods')
    expect(playerFacingGroupKey(field({ path: 'protagonist_relation.favorability' }))).toBe('relations')
    expect(playerFacingGroupKey(field({ path: 'state.currency' }))).toBe('holdings')
  })
})

describe('resolveStateFieldLayout', () => {
  it('puts the moved row in the overview even though the pack said 战斗与状态', () => {
    const layout = resolveStateFieldLayout(field({ path: 'current.situation', name: '当前处境', group: '战斗与状态' }), '被两个散修盯着')
    expect(layout.group).toBe('overview')
    // A builtin group, not the pack's: the label and icon come from the interface.
    expect(layout.customGroup).toBe(false)
  })

  it('still honours a group the design says nothing about', () => {
    const layout = resolveStateFieldLayout(field({ path: 'threat.assessment', name: '威胁态势', group: '威胁' }), '高')
    expect(layout.group).toBe('威胁')
    expect(layout.customGroup).toBe(true)
  })
})
