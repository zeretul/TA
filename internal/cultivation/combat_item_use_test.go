package cultivation

import "testing"

// The tests here cover spending a primary action on a carried item, which is
// §5.2 of the battle design's 物品 and §4.2 of the item design's dose rules.
//
// Before the action existed, a pill could be registered, priced and held and
// still never be taken during a fight: the catalogue was reachable from the
// settlement but not from the exchange loop.

// carrying returns a combatant holding a real dose, priced from the catalogue
// rather than from a number a test wrote.
func carrying(t *testing.T, combatant Combatant, defID, instanceID string, quantity int) Combatant {
	t.Helper()
	def, ok := NewItemCatalogWithFirstVersion().Definition(defID)
	if !ok || def.Pill == nil {
		t.Fatalf("the first-version catalogue has no pill %q", defID)
	}
	combatant.Carried = append(combatant.Carried, CarriedDose{
		InstanceID: instanceID, DefID: def.ID, NameKey: def.NameKey,
		Quantity: quantity, Pill: *def.Pill,
	})
	return combatant
}

func useItem(instanceID string) CombatAction {
	return CombatAction{Kind: ActionUseItem, ItemInstanceID: instanceID}
}

// hitDraws pins the exchange so every attack lands, which keeps these tests about
// the dose rather than about the dice.
func hitDraws() DrawSource {
	return drawByPurpose(map[string]float64{"hit": 0.99})
}

// §4.2: a restoring dose gives back the registered stage's fraction, and the dose
// is spent exactly once.
func TestARestoringDoseHealsAndIsSpentOnce(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	*hero = carrying(t, *hero, "pill.restore.health.lower", "dose1", 2)
	hero.Attributes.Health = 100

	next, outcome := ResolveExchange(session, map[string]CombatAction{"hero": useItem("dose1")}, hitDraws())
	step := findStep(t, outcome, "hero")
	if !step.ItemApplied || step.HealthRecovered <= 0 {
		t.Fatalf("the dose did not take effect: %+v", step)
	}
	after := next.CombatantByID("hero")
	if after.Attributes.Health != 100+step.HealthRecovered {
		t.Fatalf("health = %d, want %d", after.Attributes.Health, 100+step.HealthRecovered)
	}
	if len(after.Carried) != 1 || after.Carried[0].Quantity != 1 {
		t.Fatalf("stock = %+v, want one dose left", after.Carried)
	}
}

// §8: an ineffective use must not deduct the resource, so a full pool refuses the
// dose rather than silently eating it.
func TestADoseIsNotSpentOnAFullPool(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	*hero = carrying(t, *hero, "pill.restore.health.lower", "dose1", 1)
	hero.Attributes.Health = hero.Attributes.HealthMax

	next, outcome := ResolveExchange(session, map[string]CombatAction{"hero": useItem("dose1")}, hitDraws())
	step := findStep(t, outcome, "hero")
	if step.ItemApplied || step.ActionRejected || step.ReasonKey != "reason.pool_already_full" || next.Round != session.Round+1 {
		t.Fatalf("an unnecessary dose must keep stock but spend its declared turn: %+v", step)
	}
	if next.CombatantByID("hero").Carried[0].Quantity != 1 {
		t.Fatalf("a refused dose was still spent")
	}
}

// §7: a definition's existence is not stock. An action naming a copy the
// combatant does not carry is refused rather than served.
func TestADoseTheCombatantDoesNotCarryIsRefused(t *testing.T) {
	session := twoWayBattle(t)
	next, outcome := ResolveExchange(session, map[string]CombatAction{"hero": useItem("not-carried")}, hitDraws())
	step := findStep(t, outcome, "hero")
	if step.ItemApplied || step.ReasonKey != "reason.item_not_carried" {
		t.Fatalf("step = %+v, want a refusal naming the missing copy", step)
	}
	if len(next.CombatantByID("hero").Carried) != 0 {
		t.Fatalf("a refused use invented stock")
	}
}

// An exhausted copy is refused rather than serving a dose that is not there.
func TestAnExhaustedDoseIsRefused(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	*hero = carrying(t, *hero, "pill.restore.health.lower", "dose1", 0)
	hero.Attributes.Health = 100

	next, outcome := ResolveExchange(session, map[string]CombatAction{"hero": useItem("dose1")}, hitDraws())
	step := findStep(t, outcome, "hero")
	if step.ItemApplied || step.ReasonKey != "reason.item_exhausted" {
		t.Fatalf("step = %+v, want an exhausted refusal", step)
	}
	if next.CombatantByID("hero").Attributes.Health != 100 {
		t.Fatalf("an exhausted dose still healed")
	}
}

// §4.2: 增攻持续3轮，同类取最强，重服可刷新相同期限但不叠强度. A second dose of the
// same pill refreshes the term and never adds a second bonus.
func TestASecondBoostDoseRefreshesWithoutStacking(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	*hero = carrying(t, *hero, "pill.boost.attack.middle", "dose1", 3)
	base := hero.Attributes.Attack

	once, _ := ResolveExchange(session, map[string]CombatAction{"hero": useItem("dose1")}, hitDraws())
	boosted := once.CombatantByID("hero")
	if boosted.Attributes.Attack <= base {
		t.Fatalf("attack = %d, want more than the base %d", boosted.Attributes.Attack, base)
	}
	if len(boosted.ActiveDoses) != 1 {
		t.Fatalf("active doses = %+v, want exactly one", boosted.ActiveDoses)
	}

	twice, _ := ResolveExchange(once, map[string]CombatAction{"hero": useItem("dose1")}, hitDraws())
	restacked := twice.CombatantByID("hero")
	if len(restacked.ActiveDoses) != 1 {
		t.Fatalf("a second dose stacked a second effect: %+v", restacked.ActiveDoses)
	}
	if restacked.Attributes.Attack != boosted.Attributes.Attack {
		t.Fatalf("attack = %d after a second dose, want the same %d", restacked.Attributes.Attack, boosted.Attributes.Attack)
	}
}

// §4.2's term is a real countdown: the boost stops paying when it runs out, and
// losing it re-derives the numbers rather than subtracting a percentage back out.
func TestABoostExpiresOnItsOwnTerm(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	*hero = carrying(t, *hero, "pill.boost.attack.middle", "dose1", 1)
	base := hero.Attributes.Attack

	current, _ := ResolveExchange(session, map[string]CombatAction{"hero": useItem("dose1")}, hitDraws())
	if current.CombatantByID("hero").Attributes.Attack <= base {
		t.Fatalf("the boost did not apply")
	}
	for round := 0; round < 4; round++ {
		current, _ = ResolveExchange(current, map[string]CombatAction{"hero": CombatAction{Kind: ActionRest}}, hitDraws())
	}
	after := current.CombatantByID("hero")
	if len(after.ActiveDoses) != 0 {
		t.Fatalf("the boost outlived its term: %+v", after.ActiveDoses)
	}
	if after.Attributes.Attack != base {
		t.Fatalf("attack = %d after expiry, want the base %d", after.Attributes.Attack, base)
	}
}

// §4.2's 同类 is the kind of effect, not one catalogue entry. Two attack pills of
// different grades are the same kind, so the stronger one wins and the weaker
// re-dose only refreshes the term rather than adding a second bonus.
func TestAttackPillsOfDifferentGradesDoNotStack(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	*hero = carrying(t, *hero, "pill.boost.attack.middle", "middle", 1)
	// A lower-grade attack pill, written onto the same combatant as a real second
	// copy: the catalogue ships only the middle grade, and the rule is about the
	// purpose, so the test supplies the second definition the way a later
	// catalogue revision would.
	lower := CarriedDose{
		InstanceID: "lower", DefID: "pill.boost.attack.lower", NameKey: "item.pill.boost_attack_lower.name",
		Quantity: 1,
		Pill: PillDef{
			ID: "pill.boost.attack.lower", Purpose: PillBoostAttack,
			Grade: GradeLower, DurationExchanges: 3,
		},
	}
	hero.Carried = append(hero.Carried, lower)
	base := hero.Attributes.Attack

	once, _ := ResolveExchange(session, map[string]CombatAction{"hero": useItem("middle")}, hitDraws())
	middleBoosted := once.CombatantByID("hero")
	if middleBoosted.Attributes.Attack <= base {
		t.Fatalf("attack = %d, want more than the base %d", middleBoosted.Attributes.Attack, base)
	}
	middleAttack := middleBoosted.Attributes.Attack

	// The weaker pill is taken second. It must not add anything, and it must not
	// replace the stronger effect in force.
	twice, _ := ResolveExchange(once, map[string]CombatAction{"hero": useItem("lower")}, hitDraws())
	mixed := twice.CombatantByID("hero")
	if len(mixed.ActiveDoses) != 1 {
		t.Fatalf("a weaker attack pill stacked a second effect: %+v", mixed.ActiveDoses)
	}
	if mixed.Attributes.Attack != middleAttack {
		t.Fatalf("attack = %d after a weaker pill, want the stronger %d kept", mixed.Attributes.Attack, middleAttack)
	}
	if mixed.ActiveDoses[0].Value != pillAttackBonus[GradeMiddle] {
		t.Fatalf("the surviving dose must be the stronger one: %+v", mixed.ActiveDoses[0])
	}
}

// §5: 所有实际属性从阶段基础值与当前有效来源重新汇总. A dose taken while a main
// implement is equipped must aggregate with it in one pass, not compound onto an
// already-boosted attack.
func TestABoostAggregatesWithEquipmentInOnePass(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	// The 上品 implement at 18% against a 40-attack base is the smallest
	// published pair that still leaves rounding room to tell the two answers
	// apart; at 5% they happen to coincide, which would make the assertion
	// vacuous.
	implement, ok := MainImplementBonus(GradeUpper, RarityCommon, 0)
	if !ok {
		t.Fatal("the published upper/common implement bonus is missing")
	}
	hero.Modifiers = []AttributeModifier{EquipmentModifier("implement:sword", AttrAttack, implement)}
	hero.recomputeAttributes()
	*hero = carrying(t, *hero, "pill.boost.attack.middle", "dose1", 1)

	doseBonus, ok := hero.Carried[0].Pill.AttackBonus()
	if !ok {
		t.Fatal("the boosting pill has no published attack bonus")
	}
	equipmentOnly := AggregateAttributes(BaseAttributes(hero.Cultivation), []AttributeModifier{
		EquipmentModifier("implement:sword", AttrAttack, implement),
	}).Attack

	next, _ := ResolveExchange(session, map[string]CombatAction{"hero": useItem("dose1")}, hitDraws())
	got := next.CombatantByID("hero").Attributes.Attack

	// The right answer is the single-pass aggregation of base + implement + dose,
	// capped once by the published ceiling.
	expected := AggregateAttributes(BaseAttributes(hero.Cultivation), []AttributeModifier{
		EquipmentModifier("implement:sword", AttrAttack, implement),
		{ID: "dose:dose1", Kind: SourceMedicine, Attribute: AttrAttack, Value: doseBonus, Group: "pill:pill.boost.attack.middle", Temporary: true},
	}).Attack
	if got != expected {
		t.Fatalf("attack = %d, want the single-pass aggregate %d", got, expected)
	}

	// Compounding would apply the percentage to the already-boosted attack. Assert
	// the two answers actually differ, so this test cannot pass by coincidence.
	compounded := equipmentOnly + int(roundHalfUp(doseBonus*float64(equipmentOnly)))
	if compounded == expected {
		t.Fatalf("the published numbers make compounding indistinguishable here (%d); pick another pair", compounded)
	}
	if got == compounded {
		t.Fatalf("the dose compounded onto the already-aggregated attack: %d", got)
	}
}

// The engine's contract is that ResolveExchange does not modify the session it is
// given. Spending a dose and expiring one both touch per-combatant slices, so a
// shallow copy would break it.
func TestUsingADoseLeavesTheGivenSessionUntouched(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	*hero = carrying(t, *hero, "pill.restore.health.lower", "dose1", 1)
	hero.Attributes.Health = 100

	_, _ = ResolveExchange(session, map[string]CombatAction{"hero": useItem("dose1")}, hitDraws())

	original := session.CombatantByID("hero")
	if original.Carried[0].Quantity != 1 {
		t.Fatalf("the given session's stock moved: %+v", original.Carried)
	}
	if original.Attributes.Health != 100 {
		t.Fatalf("the given session's health moved: %d", original.Attributes.Health)
	}
}
