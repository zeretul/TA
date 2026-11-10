package cultivation

import (
	"testing"
)

// 《自制-技能与施展》§9's acceptance names this case explicitly: 一项多目标只打所选目标,
// and 多目标按实际选择和上限结算，不默认打满敌队.
//
// Before this batch Targets was a field nothing read: flame_wave declared
// Targets: 3, a player could learn and cast it, and it struck exactly the one
// combatant named in target_id. The declared reach was never settled, so the
// published numbers and the played ones disagreed.

// threeWayBattle is one player against three opponents, which is the smallest
// battle in which "only the chosen targets" can be told apart from "the whole
// enemy line".
func threeWayBattle(t *testing.T) CombatSession {
	t.Helper()
	return CombatSession{
		ID: "battle3",
		Combatants: []Combatant{
			combatantAt(t, "hero", SidePlayer, 6),
			combatantAt(t, "foe1", SideEnemy, 6),
			combatantAt(t, "foe2", SideEnemy, 6),
			combatantAt(t, "foe3", SideEnemy, 6),
		},
		Round: 1,
	}
}

// allHit is a draw source that lands every blow at the published centre damage,
// so a target that took no damage took none because it was not struck.
func allHit() DrawSource {
	return func(purpose, actorID, targetID string) float64 {
		switch purpose {
		case "hit":
			return 0.0
		case "damage":
			return 0.5
		}
		return 0.5
	}
}

func healthOf(session CombatSession, id string) int {
	return session.CombatantByID(id).Attributes.Health
}

// A technique that reaches three targets still strikes only the one the player
// named when the player names one. Defaulting to the whole enemy line would make
// the choice meaningless and would wound combatants the player never aimed at.
func TestAMultiTargetTechniqueStrikesOnlyTheChosenTarget(t *testing.T) {
	session := threeWayBattle(t)
	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("flame_wave"), TargetID: "foe1"},
		"foe1": {Kind: ActionRest},
		"foe2": {Kind: ActionRest},
		"foe3": {Kind: ActionRest},
	}, allHit())

	step := findStep(t, outcome, "hero")
	if !step.Resolution.Hit {
		t.Fatalf("the blow was forced to land: %+v", step.Resolution)
	}
	// 40 × 0.75 × 40/(40+20) = 20.
	if got := healthOf(next, "foe1"); got != 180 {
		t.Errorf("the chosen target's health = %d, want 180 (200 − 20)", got)
	}
	if got := healthOf(next, "foe2"); got != 200 {
		t.Errorf("an unchosen opponent's health = %d, want the untouched 200", got)
	}
	if got := healthOf(next, "foe3"); got != 200 {
		t.Errorf("an unchosen opponent's health = %d, want the untouched 200", got)
	}
	if len(step.ExtraTargets) != 0 {
		t.Errorf("extra targets = %+v, want none when the player chose one", step.ExtraTargets)
	}
}

// The second chosen target is a real second strike: its own hit draw, its own
// damage, and its own entry on the step so the round says what happened to it.
func TestAMultiTargetTechniqueSettlesEveryChosenTarget(t *testing.T) {
	session := threeWayBattle(t)
	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {
			Kind: ActionTechnique, Skill: mustSkill("flame_wave"),
			TargetID: "foe1", ExtraTargetIDs: []string{"foe2"},
		},
		"foe1": {Kind: ActionRest},
		"foe2": {Kind: ActionRest},
		"foe3": {Kind: ActionRest},
	}, allHit())

	step := findStep(t, outcome, "hero")
	if got := healthOf(next, "foe1"); got != 180 {
		t.Errorf("the primary target's health = %d, want 180", got)
	}
	if got := healthOf(next, "foe2"); got != 180 {
		t.Errorf("the second chosen target's health = %d, want 180", got)
	}
	if got := healthOf(next, "foe3"); got != 200 {
		t.Errorf("the unchosen opponent's health = %d, want the untouched 200", got)
	}
	if len(step.ExtraTargets) != 1 {
		t.Fatalf("extra targets = %+v, want one settled strike", step.ExtraTargets)
	}
	extra := step.ExtraTargets[0]
	if extra.TargetID != "foe2" {
		t.Errorf("extra target = %q, want foe2", extra.TargetID)
	}
	if !extra.Resolution.Hit || extra.Resolution.Damage != 20 {
		t.Errorf("extra resolution = %+v, want a 20-damage hit", extra.Resolution)
	}
	// The cost is the technique's own, paid once: a second target is not a second
	// cast, and §9 requires 同一成本只扣一次.
	if step.Cast.ManaCost != 36 {
		t.Errorf("mana cost = %d, want the published 36 for one cast", step.Cast.ManaCost)
	}
}

// A miss on the first target does not cancel the others: each chosen target has
// its own check, which is what makes the choice worth making.
func TestEachChosenTargetHasItsOwnHitCheck(t *testing.T) {
	session := threeWayBattle(t)
	draws := func(purpose, actorID, targetID string) float64 {
		if purpose != "hit" {
			return 0.5
		}
		// The first target is missed, the second lands.
		if targetID == "foe1" {
			return 0.99
		}
		return 0.0
	}
	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {
			Kind: ActionTechnique, Skill: mustSkill("flame_wave"),
			TargetID: "foe1", ExtraTargetIDs: []string{"foe2"},
		},
		"foe1": {Kind: ActionRest},
		"foe2": {Kind: ActionRest},
		"foe3": {Kind: ActionRest},
	}, draws)

	step := findStep(t, outcome, "hero")
	if step.Resolution.Hit {
		t.Errorf("the primary blow was drawn to miss: %+v", step.Resolution)
	}
	if got := healthOf(next, "foe1"); got != 200 {
		t.Errorf("the missed target's health = %d, want 200", got)
	}
	if len(step.ExtraTargets) != 1 || !step.ExtraTargets[0].Resolution.Hit {
		t.Fatalf("extra targets = %+v, want one landed strike", step.ExtraTargets)
	}
	if got := healthOf(next, "foe2"); got != 180 {
		t.Errorf("the second target's health = %d, want 180: a miss on the first must not cancel it", got)
	}
}

// Naming more targets than the technique reaches is a declaration the rules
// cannot carry out, so it is refused before the exchange and costs nothing.
func TestNamingMoreTargetsThanTheTechniqueReachesIsRefused(t *testing.T) {
	session := threeWayBattle(t)
	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {
			Kind: ActionTechnique, Skill: mustSkill("fire_bolt"),
			TargetID: "foe1", ExtraTargetIDs: []string{"foe2"},
		},
		"foe1": {Kind: ActionRest},
		"foe2": {Kind: ActionRest},
		"foe3": {Kind: ActionRest},
	}, allHit())

	step := findStep(t, outcome, "hero")
	if !step.ActionRejected {
		t.Fatalf("a single-target technique naming two targets must be refused: %+v", step)
	}
	if step.ReasonKey != ReasonTooManyTargets {
		t.Errorf("reason = %q, want %q", step.ReasonKey, ReasonTooManyTargets)
	}
	// A refused declaration costs nothing and wounds no one.
	if hero := next.CombatantByID("hero"); hero.Attributes.Mana != 160 {
		t.Errorf("caster mana = %d, want the untouched 160", hero.Attributes.Mana)
	}
	for _, id := range []string{"foe1", "foe2", "foe3"} {
		if got := healthOf(next, id); got != 200 {
			t.Errorf("%s health = %d, want the untouched 200", id, got)
		}
	}
}

// An ally is not a reachable target however many the technique can reach.
func TestAnAllyCannotBeNamedAsAnExtraTarget(t *testing.T) {
	session := threeWayBattle(t)
	session.Combatants = append(session.Combatants, combatantAt(t, "ally", SidePlayer, 6))
	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {
			Kind: ActionTechnique, Skill: mustSkill("flame_wave"),
			TargetID: "foe1", ExtraTargetIDs: []string{"ally"},
		},
		"ally": {Kind: ActionRest},
		"foe1": {Kind: ActionRest},
		"foe2": {Kind: ActionRest},
		"foe3": {Kind: ActionRest},
	}, allHit())

	step := findStep(t, outcome, "hero")
	if !step.ActionRejected || step.ReasonKey != "reason.no_target" {
		t.Fatalf("an ally named as a target must be refused: %+v", step)
	}
	if got := healthOf(next, "ally"); got != 200 {
		t.Errorf("the ally's health = %d, want the untouched 200", got)
	}
}

// Naming the same opponent twice is not two targets. It is refused rather than
// collapsed, because a client that sent it would otherwise be told it struck
// twice when only one strike was settled.
func TestNamingTheSameOpponentTwiceIsRefused(t *testing.T) {
	session := threeWayBattle(t)
	_, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {
			Kind: ActionTechnique, Skill: mustSkill("flame_wave"),
			TargetID: "foe1", ExtraTargetIDs: []string{"foe1"},
		},
		"foe1": {Kind: ActionRest},
		"foe2": {Kind: ActionRest},
		"foe3": {Kind: ActionRest},
	}, allHit())

	step := findStep(t, outcome, "hero")
	if !step.ActionRejected {
		t.Fatalf("the same opponent named twice must be refused: %+v", step)
	}
}

// The reach is a cap on how many opponents may be named, and a player may name
// fewer. Naming all three of a three-target technique's reach is legal.
func TestTheFullReachMayBeUsed(t *testing.T) {
	session := threeWayBattle(t)
	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {
			Kind: ActionTechnique, Skill: mustSkill("flame_wave"),
			TargetID: "foe1", ExtraTargetIDs: []string{"foe2", "foe3"},
		},
		"foe1": {Kind: ActionRest},
		"foe2": {Kind: ActionRest},
		"foe3": {Kind: ActionRest},
	}, allHit())

	step := findStep(t, outcome, "hero")
	if step.ActionRejected {
		t.Fatalf("three targets of a three-target technique must be legal: %+v", step)
	}
	for _, id := range []string{"foe1", "foe2", "foe3"} {
		if got := healthOf(next, id); got != 180 {
			t.Errorf("%s health = %d, want 180", id, got)
		}
	}
	// The whole cast is paid once, however many opponents it reached.
	if hero := next.CombatantByID("hero"); hero.Attributes.Mana != 124 {
		t.Errorf("caster mana = %d, want 124 (160 − 36 for one cast)", hero.Attributes.Mana)
	}
}
