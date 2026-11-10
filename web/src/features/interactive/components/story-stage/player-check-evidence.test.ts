import { expect, it } from 'vitest'
import type { RuleResolution } from '../../types'
import { playerCheckEvidence } from './player-check-evidence'

it('exposes saved dice and applied own differences while excluding hidden adjudication and proposals', () => {
  const resolution = {
    id: 'saved-check', seed: 987,
    request: { state: 'secret identity', challenge: 'secret trap', rule: { label: 'Athletics' }, outcomes: { failure: { result: 'hidden alternative' } } },
    result: { dice: '1d20', rolls: [14], total: 17, outcome: 'success', target: 12, seed: 987, story_roll_modifier: 2, bonus_total: 30, bonus_details: [{ actor_id: 'enemy', reason: 'secret weakness', value: 30 }], state_changes: [{ actor_id: 'protagonist', field_id: 'Health', change: -999 }] },
    state_consumption: { status: 'partial', applied_actor_ops: [{ op: 'set', actor_id: 'protagonist', field_id: 'Health', value: 8, previous_value: 10 }, { op: 'set', actor_id: 'enemy', field_id: 'Health', value: 1, previous_value: 999 }, { op: 'set', actor_id: 'protagonist', field_id: 'Unknown before', value: 5 }] },
  } as unknown as RuleResolution
  const evidence = playerCheckEvidence(resolution)
  expect(evidence).toEqual({ resolution_id: 'saved-check', label: undefined, dice: '1d20', rolls: [14], kept_roll: undefined, total: undefined, outcome: 'success', bonus_total: 2, state_changes: [{ actor_id: 'protagonist', field_id: 'Health', change: -2 }] })
  expect(playerCheckEvidence(JSON.parse(JSON.stringify(resolution)))).toEqual(evidence)
  expect(JSON.stringify(evidence)).not.toMatch(/secret|enemy|999|987|target/)
})

it('cannot expose a hidden bonus by subtracting public dice and modifier from the total', () => {
  const evidence = playerCheckEvidence({ id: 'hidden-bonus', request: { rule: { label: 'Check' } }, result: { rolls: [10], kept_roll: 10, total: 19, story_roll_modifier: 2, binding_bonus_total: 7, outcome: 'success' } } as unknown as RuleResolution)
  expect(evidence?.total).toBeUndefined()
  expect(evidence?.rolls).toEqual([10])
  expect(evidence?.bonus_total).toBe(2)
  expect(JSON.stringify(evidence)).not.toMatch(/19|binding|7/)
})


it('does not derive a public caption or source identity from a private request when the result caption is blank', () => {
  const resolution = {
    request: { rule: { template_id: 'dm-mystery-clue-forward', label: '悬疑调查型 DM：线索不断线' } },
    result: { label: '', dice: '1d20', rolls: [4], outcome: 'failure' },
  } as unknown as RuleResolution
  const before = JSON.stringify(resolution)
  const evidence = playerCheckEvidence(resolution)
  expect(evidence?.label).toBeUndefined()
  expect(evidence?.label_key).toBeUndefined()
  expect(JSON.stringify(evidence)).not.toMatch(/悬疑|dm-mystery|template/)
  expect(JSON.stringify(resolution)).toBe(before)
})
