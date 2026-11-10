package app

import (
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
)

// These tests cover what the battle prose is told a dose did.
//
// The window and the rules are covered elsewhere; what is asserted here is the
// facts block the narration model receives, because that text is the only thing
// standing between a settled consumption and prose that contradicts it. A dose
// that really took hold must not be described as a refusal, and §4.2's boost is
// the case that makes this easy to get wrong: it changes no pool, so a step read
// only through its pools or its reason key reports a real spend as nothing
// happening — and the story then writes the pill back into the bottle.

// doseNarrationCast names the combatants so the facts block reads as prose
// rather than as actor IDs.
func doseNarrationCast() character.State {
	return character.State{
		Version: character.StateVersion,
		Profiles: []character.Profile{
			{ID: "hero", StoryID: "st", Name: "陆青"},
			{ID: "rival", StoryID: "st", Name: "石头"},
		},
	}
}

// boostSession is a battle where the protagonist has just taken a 中品 增攻药.
func boostSession(remaining int) cultivation.CombatSession {
	return cultivation.CombatSession{
		ID: "cbt_1", Round: 2,
		Combatants: []cultivation.Combatant{
			{
				ID: "hero", Side: cultivation.SidePlayer,
				Attributes: cultivation.Attributes{Health: 64, HealthMax: 80},
				ActiveDoses: []cultivation.ActiveDose{{
					InstanceID: "赤阳丹", DefID: "pill.boost.attack.middle",
					Attribute: cultivation.AttrAttack, Value: 0.08,
					Group: "pill:pill.boost.attack.middle", RemainingExchanges: remaining,
				}},
			},
			{ID: "rival", Side: cultivation.SideEnemy, Attributes: cultivation.Attributes{Health: 40, HealthMax: 100}},
		},
	}
}

// TestCombatNarrationReportsAnAppliedBoostAsTaken is the defect this test exists
// for. A boost that took hold carries reason.item_boost_applied and moves no
// pool, so a step worded from its reason key alone reaches the "refused ...
// spent nothing" branch and tells the model the pill was never consumed.
func TestCombatNarrationReportsAnAppliedBoostAsTaken(t *testing.T) {
	session := boostSession(3)
	step := cultivation.ExchangeStep{
		ActorID: "hero", Kind: cultivation.ActionUseItem,
		SkillID: "pill.boost.attack.middle",
		// Both set, which is exactly what the rules produce for a boost: the
		// effect took hold and the reason key explains what the effect is.
		ItemApplied: true,
		ReasonKey:   "reason.item_boost_applied",
	}

	facts := combatNarrationFacts(
		cultivation.ExchangeOutcome{Round: 2, Order: []string{"hero"}, Steps: []cultivation.ExchangeStep{step}},
		session, doseNarrationCast(),
	)

	if strings.Contains(facts, "refused") {
		t.Fatalf("a dose that took hold was reported as refused:\n%s", facts)
	}
	if !strings.Contains(facts, "陆青 took a dose") {
		t.Fatalf("the dose was not reported as taken:\n%s", facts)
	}
	// The published budget, not a number the narration invented: §4.2 fixes a
	// 中品 增攻药 at 8%.
	if !strings.Contains(facts, "8%") {
		t.Fatalf("the boost's real strength is missing from the facts:\n%s", facts)
	}
	if !strings.Contains(facts, "3 exchanges") {
		t.Fatalf("the boost's real remaining term is missing from the facts:\n%s", facts)
	}
}

// TestCombatNarrationReportsARefusedDoseAsUnspent is the other half of §8. A
// refusal must say so and must say that nothing was consumed, or the story has
// no way to know the difference between a dose that was taken and one that was
// turned down.
func TestCombatNarrationReportsARefusedDoseAsUnspent(t *testing.T) {
	step := cultivation.ExchangeStep{
		ActorID: "hero", Kind: cultivation.ActionUseItem,
		SkillID:   "pill.boost.attack.middle",
		ReasonKey: "reason.item_not_carried",
	}

	facts := combatNarrationFacts(
		cultivation.ExchangeOutcome{Round: 2, Order: []string{"hero"}, Steps: []cultivation.ExchangeStep{step}},
		boostSession(3), doseNarrationCast(),
	)

	if !strings.Contains(facts, "refused") {
		t.Fatalf("a refused dose was not reported as refused:\n%s", facts)
	}
	if !strings.Contains(facts, "Nothing was consumed") {
		t.Fatalf("a refused dose did not say that nothing was spent:\n%s", facts)
	}
	// The refusal names the engine's own reason key, so the model is not handed
	// a fresh vocabulary for a cause the rules already named.
	if !strings.Contains(facts, "reason.item_not_carried") {
		t.Fatalf("the refusal's real cause is missing from the facts:\n%s", facts)
	}
}

// TestCombatNarrationReportsARestoringDoseByThePoolItMoved checks that the three
// pill kinds are not collapsed into one sentence. A dose that restored health
// says which pool moved, because §5 keeps health and spiritual power apart.
func TestCombatNarrationReportsARestoringDoseByThePoolItMoved(t *testing.T) {
	step := cultivation.ExchangeStep{
		ActorID: "hero", Kind: cultivation.ActionUseItem,
		SkillID:         "pill.restore.health.lower",
		ItemApplied:     true,
		HealthRecovered: 20,
	}

	facts := combatNarrationFacts(
		cultivation.ExchangeOutcome{Round: 2, Order: []string{"hero"}, Steps: []cultivation.ExchangeStep{step}},
		boostSession(3), doseNarrationCast(),
	)

	if !strings.Contains(facts, "recovered 20 health") {
		t.Fatalf("a restoring dose was not reported by the pool it moved:\n%s", facts)
	}
	if strings.Contains(facts, "refused") {
		t.Fatalf("a dose that restored health was reported as refused:\n%s", facts)
	}
}

// TestCombatNarrationStillReportsATechniqueAsATechnique guards the branch the
// dose case was inserted ahead of. An ordinary attack must still be described as
// an attack, or the fix would have traded one wrong report for another.
func TestCombatNarrationStillReportsATechniqueAsATechnique(t *testing.T) {
	step := cultivation.ExchangeStep{
		ActorID: "hero", TargetID: "rival", Kind: cultivation.ActionTechnique,
		SkillID:     "strike",
		Resolution:  cultivation.AttackResolution{Hit: true, Damage: 7, HealthLost: 7},
		Cast:        cultivation.CastCheck{Allowed: true},
	}

	facts := combatNarrationFacts(
		cultivation.ExchangeOutcome{Round: 2, Order: []string{"hero"}, Steps: []cultivation.ExchangeStep{step}},
		boostSession(3), doseNarrationCast(),
	)

	if !strings.Contains(facts, "the technique strike") {
		t.Fatalf("a technique was not described as a technique:\n%s", facts)
	}
	if strings.Contains(facts, "took a dose") {
		t.Fatalf("a technique was described as a dose:\n%s", facts)
	}
}

// TestCombatNarrationNamesTheFormTheTechniqueWasCastAs is §5 reaching the prose.
// A variant shares its base technique's identifier and deliberately casts at a
// different power, so a passage written from the identifier alone would describe
// the base technique's blow while the rules settled the form's. The form has to
// be in the facts for the story to be able to say what happened.
func TestCombatNarrationNamesTheFormTheTechniqueWasCastAs(t *testing.T) {
	step := cultivation.ExchangeStep{
		ActorID: "hero", TargetID: "rival", Kind: cultivation.ActionTechnique,
		SkillID:     "fire_bolt",
		Variant:     cultivation.VariantBurn,
		Resolution:  cultivation.AttackResolution{Hit: true, Damage: 16, HealthLost: 16},
		Cast:        cultivation.CastCheck{Allowed: true, ManaCost: 8, BurdenCost: 6},
		BurnApplied: true,
	}

	facts := combatNarrationFacts(
		cultivation.ExchangeOutcome{Round: 2, Order: []string{"hero"}, Steps: []cultivation.ExchangeStep{step}},
		boostSession(3), doseNarrationCast(),
	)

	if !strings.Contains(facts, string(cultivation.VariantBurn)) {
		t.Fatalf("the form the technique was cast as is missing from the facts:\n%s", facts)
	}
	if !strings.Contains(facts, "the technique fire_bolt") {
		t.Fatalf("the base technique must still be named:\n%s", facts)
	}
}
