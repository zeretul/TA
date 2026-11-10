package interactive

import (
	"fmt"

	"ta/internal/cultivation"
)

// This file is §四's off-battle meridian rule for the rest loop.
//
// The rules themselves — the 6-points-per-real-minute rate, the residual floor,
// the condition set for clearing a secret technique's injury — already exist in
// internal/cultivation and are not restated here. What was missing was the
// wiring: a meditation settled through the story's own rest entry restored mana
// and left the meridian burden exactly where it was, so a character could
// regulate for an hour and still be unable to cast.
//
// Two properties are deliberate.
//
//   - The relief is the effect of the interval the rest loop already paid for.
//     §四 forbids counting the same minutes twice, so the minutes used here are
//     the ones this settlement advanced the branch clock by, and clearing a
//     residual adds no second 30-minute interval on top of them.
//
//   - The clearing interval is one continuous safe stretch, not an accumulated
//     total. §四 refuses to splice a stretch that was not legally safe onto a
//     later one, so the interval offered to the rules is the one this single
//     settlement covered. A meditation broken by a real node therefore has to
//     earn its continuous half-hour after the break, and one broken by a change
//     of safety cannot continue at all — the activity layer already refuses to
//     extend across that change.

// planMeditationBurden applies the off-battle meridian rules to one settled
// meditation interval.
//
// It returns the burden before and after, and whether this interval was also the
// safe regulation a residual injury required. A health rest is deliberately not
// a meditation: §四 names 安全调息 for the per-minute relief, so a plain rest
// leaves the meridian burden untouched.
//
// The burden is read for every mode even though only a meditation changes it.
// The two returned numbers are reported as the meridian around the interval, so
// answering zero for a character who really carries a residual would be a false
// statement about the world rather than a neutral default.
func planMeditationBurden(
	projection *storyBranchProjection,
	actorID string,
	mode cultivation.RecoveryMode,
	elapsedSeconds int64,
	eligibility cultivation.RecoveryEligibility,
) (cultivation.BurdenState, cultivation.BurdenState, bool, error) {
	character, err := xiuxianCharacterFromState(projection.cultivationRules(),
		projection.State, actorID)
	if err != nil {
		if mode != cultivation.RecoverySafeMeditation {
			// §四's rule does not reach a plain rest, so an unreadable cultivation
			// record must not fail one. The interval reports no meridian rather
			// than refusing a rest the rule never applied to.
			return cultivation.BurdenState{}, cultivation.BurdenState{}, false, nil
		}
		return cultivation.BurdenState{}, cultivation.BurdenState{}, false, err
	}
	before := character.BurdenState()

	if mode != cultivation.RecoverySafeMeditation || elapsedSeconds <= 0 {
		return before, before, false, nil
	}
	if before.Current == 0 && before.Residual == 0 {
		// Nothing to relieve and nothing to clear, so the interval writes no
		// meridian operation at all. This is the ordinary case and it must not
		// put redundant entries in the journal.
		return before, before, false, nil
	}

	minutes := float64(elapsedSeconds) / 60
	after := cultivation.RecoverBurdenOffBattle(before, minutes)

	// Clearing is the effect of the same minutes, so it is decided after the
	// relief rather than instead of it: a residual that is not earned still has
	// the ordinary relief applied down to its floor.
	cleared := false
	if skill, ok := residualSkillForFloor(after.Residual); ok {
		// The eligibility the interval was admitted under IS §四's "no real
		// injury or seal blocking regulation" finding: a blocking one refuses the
		// whole interval earlier, so reaching here means none is recorded.
		blocked := eligibility != cultivation.RecoveryQualified
		if cultivation.CanClearResidual(skill, minutes, character.Attributes(), blocked) {
			after = cultivation.ClearResidual(after)
			cleared = true
		}
	}
	return before, after, cleared, nil
}

// recoveryBurdenOps builds the meridian writes for one settled interval.
//
// It writes absolutes for the same reason the pool write does: the number is
// what the rules say the burden is, and a delta would accumulate the difference
// between two answers and drift. Both fields are settled fields the narrative
// may not author, so this is the only path that moves them outside a battle.
func recoveryBurdenOps(
	state map[string]any,
	system StoryDirectorActorStateSystem,
	actorID string,
	before, after cultivation.BurdenState,
	cleared bool,
	eventID string,
) ([]ActorStateOp, error) {
	actorID = normalizeStatePanelActorID(actorID)
	if actorID == "" || !ActorStateSystemIsXiuxian(system) {
		return nil, fmt.Errorf("%w: %s", ErrRecoveryUnknownActor, actorID)
	}
	templateID, found := actorTemplateIDFromStateOrSystem(state, system, actorID)
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrRecoveryUnknownActor, actorID)
	}
	template := actorStateTemplateByID(system, templateID)
	if !actorStateTemplateIsXiuxian(template, system.CultivationRules) {
		return nil, fmt.Errorf("%w: %s", ErrRecoveryUnknownActor, actorID)
	}

	reason := "由安全调息的实际时长按每实际分钟六点消解普通负担，不由叙述改写。"
	if cleared {
		reason = "由安全调息的连续合格时长解除经脉余伤并消解负担，不由叙述改写。"
	}
	ops := make([]ActorStateOp, 0, 2)
	if actorStateTemplateDeclaresField(template, xiuxianFieldBurden) && after.Current != before.Current {
		ops = append(ops, recoveryBurdenOp(actorID, xiuxianFieldBurden, after.Current, reason, eventID))
	}
	if actorStateTemplateDeclaresField(template, xiuxianFieldBurdenFloor) && after.Residual != before.Residual {
		ops = append(ops, recoveryBurdenOp(actorID, xiuxianFieldBurdenFloor, after.Residual, reason, eventID))
	}
	return ops, nil
}

// recoveryBurdenOp is one meridian write, under the recovery's own source kind
// so a reader of the journal can tell a burden a regulation relieved from one a
// narrative described.
func recoveryBurdenOp(actorID, fieldID string, value int, reason, eventID string) ActorStateOp {
	return ActorStateOp{
		Op:           "set",
		ActorID:      actorID,
		FieldID:      normalizeActorStateFieldName(fieldID),
		Value:        value,
		Reason:       reason,
		SourceTurnID: eventID,
		SourceKind:   StateOpSourceSafeRecovery,
		SourceID:     "safe_recovery",
	}
}
