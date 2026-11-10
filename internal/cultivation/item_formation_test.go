package cultivation

import "testing"

// The tests here cover §4.3's formation costs inside a battle: a formation is
// raised for 10% of the operator's stage base mana and kept for 2% per exchange,
// and it stops working when the operator can no longer pay.
//
// Before this batch a deployed formation was a field the settlement read and the
// battle never advanced: the bonus was paid, the upkeep was never charged, and
// nothing ever ended it.

// formationBattle is a two-way battle with a middle-grade attack formation
// already standing on the hero's side. 中品增攻阵 is 8%.
func formationBattle(t *testing.T) CombatSession {
	t.Helper()
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	hero.Formation = FormationState{
		Def:    FormationDef{ID: "formation.attack.middle", Grade: GradeMiddle, AttackBonus: 0.08},
		Active: true,
	}
	hero.recomputeAttributes()
	return session
}

// TestADeployedFormationRaisesAttackWhileItStands is the bonus half: 40 × 1.08 = 43.
func TestADeployedFormationRaisesAttackWhileItStands(t *testing.T) {
	session := formationBattle(t)
	if got := session.CombatantByID("hero").Attributes.Attack; got != 43 {
		t.Fatalf("attack inside a middle-grade attack formation = %d, want 43 (40 × 1.08)", got)
	}
}

// TestKeepingAFormationCostsItsPublishedUpkeep is §4.3's 每轮维持2%，向上取整. At
// 炼气六层 the bare mana maximum is 160, so one exchange costs ceil(0.02 × 160) = 4.
//
// The actions are plain strikes rather than rests: a rest restores mana at the end
// of the exchange, which would mask the upkeep in the pool this test reads.
func TestKeepingAFormationCostsItsPublishedUpkeep(t *testing.T) {
	session := formationBattle(t)
	next, _ := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "foe"},
		"foe":  {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "hero"},
	}, fixedDraws(0.5))
	hero := next.CombatantByID("hero")
	if got := hero.Attributes.Mana; got != 156 {
		t.Errorf("mana after one exchange of upkeep = %d, want 156 (160 − 4)", got)
	}
	if !hero.Formation.Active {
		t.Error("a formation whose upkeep was paid must still be standing")
	}
	if got := hero.Attributes.Attack; got != 43 {
		t.Errorf("attack after upkeep = %d, want the still-standing 43", got)
	}
}

// TestAFormationWhoseUpkeepFailsEndsAndStopsPaying is §4.3's 失去维持能力. The
// operator cannot pay, so the formation is over — and the bonus has to leave the
// numbers in that same exchange, not linger on the total it was folded into.
func TestAFormationWhoseUpkeepFailsEndsAndStopsPaying(t *testing.T) {
	session := formationBattle(t)
	hero := session.CombatantByID("hero")
	hero.Attributes.Mana = 3
	hero.recomputeAttributes()

	next, _ := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "foe"},
		"foe":  {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "hero"},
	}, fixedDraws(0.5))
	after := next.CombatantByID("hero")
	if after.Formation.Active {
		t.Error("a formation the operator cannot maintain must stop")
	}
	if got := after.Attributes.Attack; got != 40 {
		t.Errorf("attack after the formation ended = %d, want the bare 40", got)
	}
	if got := after.Attributes.Mana; got != 3 {
		t.Errorf("mana after a failed upkeep = %d, want the untouched 3", got)
	}
}

// TestAFormationWithNoImposedTermDoesNotExpire is the zero-value reading §4.3
// requires: a formation has a cost and an upkeep, not a duration, so a deployed
// 阵盘 is not something that runs out on its own.
func TestAFormationWithNoImposedTermDoesNotExpire(t *testing.T) {
	session := formationBattle(t)
	for round := 0; round < 3; round++ {
		next, _ := ResolveExchange(session, map[string]CombatAction{
			"hero": {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "foe"},
			"foe":  {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "hero"},
		}, fixedDraws(0.5))
		session = next
	}
	hero := session.CombatantByID("hero")
	if !hero.Formation.Active {
		t.Fatal("a formation with no imposed term expired on its own")
	}
	if got := hero.Attributes.Mana; got != 148 {
		t.Errorf("mana after three exchanges = %d, want 148 (160 − 3 × 4)", got)
	}
}

// TestActivatingAFormationChargesTheStartCost is 启动耗操作者阶段基准灵力10%: 16 at
// 炼气六层, refused rather than free when the pool cannot cover it.
func TestActivatingAFormationChargesTheStartCost(t *testing.T) {
	state := FormationState{Def: FormationDef{ID: "formation.attack.middle", Grade: GradeMiddle, AttackBonus: 0.08}}
	operator := BaseAttributes(Qi(6))

	raised, after, ok := state.Activate(operator, Qi(6))
	if !ok {
		t.Fatal("an operator with a full pool must be able to raise the formation")
	}
	if !raised.Active {
		t.Error("a paid activation must leave the formation standing")
	}
	if after.Mana != 144 {
		t.Errorf("mana after raising = %d, want 144 (160 − 16)", after.Mana)
	}

	poor := operator
	poor.Mana = 10
	_, unchanged, ok := state.Activate(poor, Qi(6))
	if ok {
		t.Error("an operator who cannot pay must not raise the formation")
	}
	if unchanged.Mana != 10 {
		t.Errorf("a refused activation spent %d mana, want the untouched 10", 10-unchanged.Mana)
	}
}
