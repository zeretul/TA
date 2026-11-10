package cultivation

import "testing"

// A prepared technique is priced when it starts. A declaration that names no
// opponent has no legal target, so it must be refused before the cost is paid
// rather than discovered when the technique completes: paying for a strike at
// nothing is the half-deduction §4.2 forbids.
func TestAPreparedTechniqueWithoutATargetSpendsNothing(t *testing.T) {
	session := withBladeInHand(twoWayBattle(t), "hero")
	secret := mustSkill("meridian_secret_blade")
	before := session.CombatantByID("hero")

	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: secret},
		"foe":  {Kind: ActionRest},
	}, fixedDraws(0))
	step := findStep(t, outcome, "hero")
	if step.ReasonKey != "reason.no_target" {
		t.Fatalf("reason = %q, want the no-target refusal", step.ReasonKey)
	}
	hero := next.CombatantByID("hero")
	if hero.Attributes.Mana != before.Attributes.Mana {
		t.Errorf("mana after a targetless preparation = %d, want the unpaid %d", hero.Attributes.Mana, before.Attributes.Mana)
	}
	if hero.Burden.Current != before.Burden.Current || hero.Burden.Residual != before.Burden.Residual {
		t.Errorf("burden after a targetless preparation = %+v, want the unpaid %+v", hero.Burden, before.Burden)
	}
	if _, preparing := next.Preparations["hero"]; preparing {
		t.Error("a refused preparation must not be recorded as begun")
	}
}

// The same rule for an instant technique: a damaging technique with no live
// opponent spends nothing.
func TestAnInstantTechniqueWithoutATargetSpendsNothing(t *testing.T) {
	session := twoWayBattle(t)
	before := session.CombatantByID("hero")
	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("condensed_blade")},
		"foe":  {Kind: ActionRest},
	}, fixedDraws(0))
	step := findStep(t, outcome, "hero")
	if step.ReasonKey != "reason.no_target" {
		t.Fatalf("reason = %q, want the no-target refusal", step.ReasonKey)
	}
	if got := next.CombatantByID("hero").Attributes.Mana; got != before.Attributes.Mana {
		t.Errorf("mana after a targetless cast = %d, want the unpaid %d", got, before.Attributes.Mana)
	}
}

// A target on the caster's own side is not a legal target for a strike, and
// naming one must not be charged either.
func TestATechniqueCannotStrikeAnAlly(t *testing.T) {
	session := twoWayBattle(t)
	ally := session.CombatantByID("hero")
	session.Combatants = append(session.Combatants, Combatant{
		ID: "ally", Side: SidePlayer, Cultivation: ally.Cultivation,
		Attributes: ally.Attributes, Burden: ally.Burden,
	})
	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("condensed_blade"), TargetID: "ally"},
		"foe":  {Kind: ActionRest},
	}, fixedDraws(0))
	step := findStep(t, outcome, "hero")
	if step.ReasonKey != "reason.no_target" {
		t.Fatalf("reason = %q, want the no-target refusal", step.ReasonKey)
	}
	if got := next.CombatantByID("ally").Attributes.Health; got != ally.Attributes.Health {
		t.Errorf("ally health = %d, want %d", got, ally.Attributes.Health)
	}
}

// A shield technique is the exception: §5 allows it to be laid on the caster,
// so it needs no target and must still be payable without one.
func TestAShieldTechniqueNeedsNoTarget(t *testing.T) {
	session := twoWayBattle(t)
	shield := mustSkill("shield")
	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: shield},
		"foe":  {Kind: ActionRest},
	}, fixedDraws(0))
	step := findStep(t, outcome, "hero")
	if step.ReasonKey == "reason.no_target" {
		t.Fatalf("a shield must not be refused for lacking a target: %+v", step)
	}
	if next.CombatantByID("hero").Shield.Amount <= 0 {
		t.Errorf("shield after casting = %+v, want a real shield on the caster", next.CombatantByID("hero").Shield)
	}
}
