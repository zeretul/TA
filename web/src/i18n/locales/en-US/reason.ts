// Why an action produced nothing, or why a technique cannot be performed now.
// The rules report one of these keys and the interface supplies the wording, so a
// refusal is always a specific, locatable reason rather than a silent dead end.
const reason = {
  'reason.mortal_no_techniques': 'Not yet on the path; there is no technique to perform.',
  'reason.skill_stage_too_low': 'Your cultivation stage is too low to perform this technique.',
  'reason.unknown_skill': 'That technique was never learned.',
  'reason.insufficient_mana': 'Not enough mana to perform this technique.',
  // Casting strain is what the body is carrying from casting, not damage to the
  // meridians: §T21 keeps the two apart so a heavy round does not read as an
  // injury.
  'reason.burden_capacity': 'The body cannot carry more casting strain; let it settle before another heavy move.',
  'reason.meridian_unrecovered': 'A wound has not healed, so a heavy move is out of reach for now.',
  'reason.implement_required': 'This needs a focus in hand.',
  'reason.unknown_action': 'That is not an action this combatant can take.',
  'reason.variant_unsupported': 'This technique has no such variant.',
  'reason.preparing': 'This round only prepares it; it completes on the next.',
  'reason.no_target': 'There is no reachable target.',
  'reason.too_many_targets': 'This technique cannot reach that many targets.',
  // Why a dose was refused. §8 requires a refused use to cost nothing, so each
  // of these is the rules' own key rather than a generic failure.
  'reason.item_not_named': 'No copy was named.',
  'reason.item_not_carried': 'That copy is not on hand.',
  'reason.item_exhausted': 'That copy is used up.',
  'reason.item_not_usable': 'That copy is of no use right now.',
  'reason.pool_already_full': 'Already at full, so the dose would be wasted.',
  'reason.talisman_applied': 'The ward talisman forms a shield.',
  'reason.talisman_shield_active': 'The current shield is at least as strong; this talisman would add no protection.',
  'reason.item_stage_too_low': 'Your cultivation is too low to use this item.',
  'reason.item_boost_applied': 'The dose is in force; only the strongest of a kind counts.',
}

export default reason
