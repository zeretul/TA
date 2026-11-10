// Why an action produced nothing, or why a technique cannot be performed now.
// The rules report one of these keys and the interface supplies the wording, so a
// refusal is always a specific, locatable reason rather than a silent dead end.
const reason = {
  'reason.mortal_no_techniques': '尚未踏入修行，没有可施展的术法。',
  'reason.skill_stage_too_low': '当前境界尚不足以施展此术。',
  'reason.unknown_skill': '这门功法并未学过。',
  'reason.insufficient_mana': '灵力不足，这一招暂时施展不出。',
  // 施招负担 is what the body is carrying from casting, not damage to the
  // meridians: §T21 keeps the two apart so a heavy round does not read as an
  // injury.
  'reason.burden_capacity': '身体已承受不住更多施招负担，先缓一缓再出重手。',
  'reason.meridian_unrecovered': '伤势未愈，暂时不能施展重招。',
  'reason.implement_required': '需要法器在手才能施展。',
  'reason.unknown_action': '这不是本人能做的行动。',
  'reason.variant_unsupported': '这门功法没有这种变化。',
  'reason.preparing': '本次只是准备，完成在下一轮。',
  'reason.no_target': '没有可达的目标。',
  'reason.too_many_targets': '这一招打不到这么多目标。',
  // Why a dose was refused. §8 requires a refused use to cost nothing, so each
  // of these is the rules' own key rather than a generic failure.
  'reason.item_not_named': '没有指明要用哪一份。',
  'reason.item_not_carried': '身上没有这一份。',
  'reason.item_exhausted': '这一份已经用完了。',
  'reason.item_not_usable': '这一份现在用不上。',
  'reason.pool_already_full': '状态已满，用了也是白费。',
  'reason.talisman_applied': '护身符已化作护盾。',
  'reason.talisman_shield_active': '现有护盾不弱于此符，此时使用不会增强防护。',
  'reason.item_stage_too_low': '当前境界不足以使用此物。',
  'reason.item_boost_applied': '药力已附着，同类只取最强。',
}

export default reason
