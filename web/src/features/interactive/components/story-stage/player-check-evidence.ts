import type { PublicRuleRoll } from '@/lib/api-client/types'
import type { RuleResolution } from '../../types'
import { builtinCheckLabelKey } from '../../builtin-display-labels'

/** A read-only player view of a committed check, never its adjudicator input.
 * Targets, seeds, alternate outcomes, hidden actors and proposed deltas are
 * deliberately absent. Resource differences come only from applied own ops.
 */
export function playerCheckEvidence(resolution?: RuleResolution): PublicRuleRoll | undefined {
  if (!resolution) return undefined
  const result = resolution.result
  const consumption = resolution.state_consumption
  const changes = ['applied', 'partial'].includes(consumption?.status || '')
    ? (consumption?.applied_actor_ops || []).flatMap((op) => {
      if (op.actor_id !== 'protagonist' || typeof op.value !== 'number' || !Number.isFinite(op.value)) return []
      const change = op.op === 'add' ? op.value : op.op === 'set' && typeof op.previous_value === 'number' ? op.value - op.previous_value : undefined
      return change === undefined || !Number.isFinite(change) || change === 0 ? [] : [{ actor_id: 'protagonist', field_id: op.field_id, change }]
    }) : []
  const labelKey = builtinCheckLabelKey(resolution.display_template_id || resolution.request.rule?.template_id, result.label)
  return {
    ...(labelKey ? { label_key: labelKey } : {}),
    resolution_id: resolution.id,
    label: result.label || undefined,
    dice: result.dice,
    rolls: result.rolls ? [...result.rolls] : undefined,
    kept_roll: result.kept_roll,
    // The settled total may contain hidden binding bonuses. Even alongside
    // public dice and the player's modifier it would reveal their sum.
    total: undefined,
    outcome: result.outcome,
    // Only the player's configured modifier is unambiguously public. Binding
    // audit totals may include unseen target data and do not belong here.
    bonus_total: result.story_roll_modifier,
    state_changes: changes,
  }
}
