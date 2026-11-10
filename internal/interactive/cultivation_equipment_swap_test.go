package interactive

import (
	"testing"

	"ta/internal/cultivation"
)

// The tests here cover §2 of 《自制-装备栏与法宝御使》修订2: 战斗中换装要花一个主要行动,
// 同一动作里不能再发动换入的那件, 换装结果要真的落盘.
//
// They go through the same readers the live battle uses — combatantFromState and
// xiuxianItemSlotEntry — so the positions and the pack are read and written
// exactly as a real fight reads and writes them, rather than through hand-built
// values that could drift from what play produces.

// swapStage is the stage the swap tests fight at.
//
// Layer 7 has bare 44 attack and 22 defence, which makes the catalogue's
// percentages distinct integers: 5% gives 46, 15% gives 51 and 18% gives 52.
// A copy priced twice would be visibly wrong rather than lost in rounding.
var swapStage = cultivation.Qi(7)

func swapDefinition(t *testing.T, id string) cultivation.ItemDef {
	t.Helper()
	definition, ok := storyItemCatalog(cultivation.ReferenceRules()).Definition(id)
	if !ok {
		t.Fatalf("the catalogue must register %q", id)
	}
	return definition
}

// wornEntry builds the recorded slot value for a copy, the way equipping does.
func wornEntry(t *testing.T, id, name string) map[string]any {
	t.Helper()
	return xiuxianItemSlotEntry(swapDefinition(t, id), name, swapStage, "", "")
}

// packedEntry builds the recorded pack value for a copy, the way acquiring does.
func packedEntry(t *testing.T, id, name string) map[string]any {
	t.Helper()
	return xiuxianItemPackEntry(swapDefinition(t, id), name, 1)
}

// swapBattle builds a battle for a qi-refining layer 7 character with the given
// positions and pack.
func swapBattle(t *testing.T, slots map[string]any, pack map[string]any) cultivation.CombatSession {
	t.Helper()
	state := xiuxianTestState(t, cultivation.StageKey(swapStage), slots)
	if pack != nil {
		state["actors"].(map[string]any)["protagonist"].(map[string]any)["state"].(map[string]any)[xiuxianFieldImportantItems] = pack
	}
	combatant, err := combatantFromState(cultivation.ReferenceRules(),
		state, storySkillRegistry(cultivation.ReferenceRules()), "protagonist", CombatantSpec{
			Side: cultivation.SidePlayer, RouteExists: true,
		})
	if err != nil {
		t.Fatalf("the protagonist must be readable as a combatant: %v", err)
	}
	enemy := cultivation.Combatant{
		ID: "enemy", Side: cultivation.SideEnemy,
		Cultivation: combatant.Cultivation,
		Attributes:  cultivation.Attributes{Health: 400, HealthMax: 400, Mana: 200, ManaMax: 200, Attack: 10, Defense: 10, Agility: 10, Perception: 10},
	}
	return cultivation.CombatSession{
		ID: "s1", Round: 1, Combatants: []cultivation.Combatant{combatant, enemy},
		Preparations: map[string]cultivation.Preparation{},
	}
}

func halfDraw(string, string, string) float64 { return 0.5 }

// §3: an equipped copy contributes once, not twice.
//
// The character's own modifier list already includes the implement, because the
// settlement prices it there. If the battle read that list AND its own position,
// the same 15% sword would pay twice and 44 attack would settle at 58 rather
// than 51.
func TestAnEquippedCopyIsPricedOnceInBattle(t *testing.T) {
	session := swapBattle(t, map[string]any{
		xiuxianFieldImplement: wornEntry(t, "implement.middle.rare", "青锋剑"),
	}, nil)

	if got := session.Combatants[0].Attributes.Attack; got != 51 {
		t.Errorf("attack with a middle-grade rare implement = %d, want 51 (44 × 1.15 once, not twice)", got)
	}
}

// §1: a copy in the pack adds nothing.
func TestASpareInThePackAddsNothing(t *testing.T) {
	session := swapBattle(t, nil, map[string]any{
		"青锋剑": packedEntry(t, "implement.middle.rare", "青锋剑"),
	})

	if got := session.Combatants[0].Attributes.Attack; got != 44 {
		t.Errorf("attack with an implement only in the pack = %d, want 44 (bare stage)", got)
	}
	if len(session.Combatants[0].Spares) != 1 {
		t.Fatalf("the pack copy must be offered as a spare, got %d", len(session.Combatants[0].Spares))
	}
}

// §2: taking a spare up costs the round's primary action.
//
// The swapped-in copy contributes in the same exchange it was paid for, and the
// copy it replaced stops contributing in that same exchange — otherwise the
// change would be a label rather than a number.
func TestASwapChangesTheNumbersInTheExchangeItWasPaidFor(t *testing.T) {
	session := swapBattle(t, map[string]any{
		xiuxianFieldImplement: wornEntry(t, "implement.lower.common", "旧剑"),
	}, map[string]any{
		"新剑": packedEntry(t, "implement.upper.rare.effect", "新剑"),
	})
	before := session.Combatants[0].Attributes.Attack

	next, outcome := cultivation.ResolveExchange(session, map[string]cultivation.CombatAction{
		"protagonist": {Kind: cultivation.ActionSwapEquipment, SwapInstanceID: "新剑"},
	}, halfDraw)

	if outcome.Ended {
		t.Fatal("a swap does not end a fight")
	}
	hero := next.Combatants[0]
	if len(outcome.Steps) == 0 || !outcome.Steps[0].EquipmentSwapped {
		t.Fatalf("the swap must be reported as its own step, got %+v", outcome.Steps)
	}
	if got := hero.Attributes.Attack; got <= before {
		t.Errorf("attack after swapping a 5%% implement for an 18%% one = %d, want more than %d", got, before)
	}
	weapon, ok := hero.EquipmentInSlot(cultivation.EquipmentSlotWeapon)
	if !ok {
		t.Fatal("the weapon position must be occupied after the swap")
	}
	if weapon.Name != "新剑" {
		t.Errorf("the weapon position must hold the copy that was taken up, got %q", weapon.Name)
	}
}

// §2: the round's action is spent by the swap, so nothing else happens.
//
// The engine resolves one action per combatant, so a swap is the round. What the
// test pins down is that the exchange reports the swap as the actor's step rather
// than a technique alongside it.
func TestASwapIsTheWholeRoundForThatCombatant(t *testing.T) {
	session := swapBattle(t, nil, map[string]any{
		"新剑": packedEntry(t, "implement.upper.rare.effect", "新剑"),
	})

	next, outcome := cultivation.ResolveExchange(session, map[string]cultivation.CombatAction{
		"protagonist": {Kind: cultivation.ActionSwapEquipment, SwapInstanceID: "新剑"},
	}, halfDraw)

	steps := 0
	for _, step := range outcome.Steps {
		if step.ActorID != "protagonist" {
			continue
		}
		steps++
		if step.Kind != cultivation.ActionSwapEquipment {
			t.Errorf("the round's only player step must be the swap, got %q", step.Kind)
		}
		if !step.EquipmentSwapped {
			t.Errorf("the swap must have taken effect, got reason %q", step.ReasonKey)
		}
	}
	if steps != 1 {
		t.Errorf("a swap must be exactly one step, got %d", steps)
	}
	// The enemy still acts: the swap spends the player's action, not the enemy's.
	if next.Round != session.Round+1 {
		t.Errorf("round after the exchange = %d, want %d", next.Round, session.Round+1)
	}
}

// §2: a copy that is not in the pack cannot be taken up.
func TestASwapNamingACopyNotInThePackIsRefused(t *testing.T) {
	session := swapBattle(t, nil, map[string]any{
		"新剑": packedEntry(t, "implement.upper.rare.effect", "新剑"),
	})

	_, outcome := cultivation.ResolveExchange(session, map[string]cultivation.CombatAction{
		"protagonist": {Kind: cultivation.ActionSwapEquipment, SwapInstanceID: "不存在的剑"},
	}, halfDraw)

	for _, step := range outcome.Steps {
		if step.ActorID != "protagonist" {
			continue
		}
		if step.EquipmentSwapped {
			t.Fatal("a swap of a copy the character does not carry must be refused")
		}
		if step.ReasonKey != "reason.spare_not_carried" {
			t.Errorf("refusal reason = %q, want reason.spare_not_carried", step.ReasonKey)
		}
		return
	}
	t.Fatal("the player's step was not reported")
}

// §2: the position is read from the copy, never from the request.
//
// A blade can only go to the weapon position, because that is the row its
// registered definition maps to. Asking for a treasure position is not part of
// the request at all — this pins the mapping down so a later change cannot make
// it accept one.
func TestASpareGoesToThePositionItsDefinitionNames(t *testing.T) {
	session := swapBattle(t, nil, map[string]any{
		"新剑": packedEntry(t, "implement.upper.rare.effect", "新剑"),
	})

	spares := session.Combatants[0].Spares
	if len(spares) != 1 {
		t.Fatalf("expected one spare, got %d", len(spares))
	}
	if spares[0].Slot != cultivation.EquipmentSlotWeapon {
		t.Errorf("an implement must be offered for the weapon position, got %q", spares[0].Slot)
	}
	// The spare carries its numbers so the swap can change a figure, but it must
	// not be in the aggregation while it sits in the pack — that is what
	// TestASpareInThePackAddsNothing pins down from the character's side.
	if len(spares[0].Modifiers) == 0 {
		t.Error("a spare must carry the numbers it will contribute once taken up")
	}
}

// §1: a pill is not equipment and is never offered as a spare.
func TestAPillInThePackIsNotOfferedAsASpare(t *testing.T) {
	session := swapBattle(t, nil, map[string]any{
		"回春丹": xiuxianItemPackEntry(swapDefinition(t, "pill.restore.health.lower"), "回春丹", 2),
	})

	if len(session.Combatants[0].Spares) != 0 {
		t.Errorf("a pill must not be offered as equipment, got %d spares", len(session.Combatants[0].Spares))
	}
}

// §2: 关闭窗口、换视图、暂停或重开不能绕过该动作和消耗.
//
// The positions the battle ended with are what the branch keeps, so the
// settlement writes them back. The copy that came out returns to the pack rather
// than vanishing, and the one that went in leaves it.
func TestASwapSettlesIntoTheSavedArrangement(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(swapStage), map[string]any{
		xiuxianFieldImplement: wornEntry(t, "implement.lower.common", "旧剑"),
		xiuxianFieldImportantItems: map[string]any{
			"新剑": packedEntry(t, "implement.upper.rare.effect", "新剑"),
		},
	})
	combatant, err := combatantFromState(cultivation.ReferenceRules(),
		state, storySkillRegistry(cultivation.ReferenceRules()), "protagonist", CombatantSpec{Side: cultivation.SidePlayer})
	if err != nil {
		t.Fatalf("the protagonist must be readable as a combatant: %v", err)
	}
	enemy := cultivation.Combatant{
		ID: "enemy", Side: cultivation.SideEnemy,
		Cultivation: combatant.Cultivation,
		Attributes:  cultivation.Attributes{Health: 400, HealthMax: 400, Mana: 200, ManaMax: 200, Attack: 10, Defense: 10, Agility: 10, Perception: 10},
	}
	session := cultivation.CombatSession{
		ID: "s1", Round: 1, Combatants: []cultivation.Combatant{combatant, enemy},
		Preparations: map[string]cultivation.Preparation{},
	}
	next, _ := cultivation.ResolveExchange(session, map[string]cultivation.CombatAction{
		"protagonist": {Kind: cultivation.ActionSwapEquipment, SwapInstanceID: "新剑"},
	}, halfDraw)

	ops := xiuxianEquipmentSwapSettlementOps(state, system, next, "ev1")
	settled := appliedXiuxianState(t, state, ops)

	held, ok := xiuxianEquipmentEntry(settled, "protagonist", xiuxianFieldImplement)
	if !ok {
		t.Fatal("the weapon position must be written")
	}
	if got := xiuxianItemName(held); got != "新剑" {
		t.Errorf("the weapon position after settling = %q, want 新剑", got)
	}
	pack, _ := xiuxianEquipmentEntry(settled, "protagonist", xiuxianFieldImportantItems)
	if _, still := pack["新剑"]; still {
		t.Error("the copy taken into hand must leave the pack")
	}
	if _, back := pack["旧剑"]; !back {
		t.Error("the copy that was put away must return to the pack")
	}
}

// §2: a round in which nothing was swapped writes nothing.
func TestARoundWithoutASwapWritesNoArrangement(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(swapStage), map[string]any{
		xiuxianFieldImplement: wornEntry(t, "implement.middle.rare", "青锋剑"),
	})
	combatant, err := combatantFromState(cultivation.ReferenceRules(),
		state, storySkillRegistry(cultivation.ReferenceRules()), "protagonist", CombatantSpec{Side: cultivation.SidePlayer})
	if err != nil {
		t.Fatalf("the protagonist must be readable as a combatant: %v", err)
	}
	session := cultivation.CombatSession{
		ID: "s1", Round: 1, Combatants: []cultivation.Combatant{combatant},
		Preparations: map[string]cultivation.Preparation{},
	}
	if ops := xiuxianEquipmentSwapSettlementOps(state, system, session, "ev1"); len(ops) != 0 {
		t.Errorf("a round that changed no position must write nothing, got %d ops", len(ops))
	}
}

// §2: a retried round states the same arrangement rather than moving a second
// copy.
//
// The settlement writes absolute values, so settling the same session twice
// produces the same writes and the second application changes nothing. This is
// the equipment counterpart of the pool writes' idempotence.
func TestASettledArrangementIsTheSameWhenRetried(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(swapStage), map[string]any{
		xiuxianFieldImplement: wornEntry(t, "implement.lower.common", "旧剑"),
		xiuxianFieldImportantItems: map[string]any{
			"新剑": packedEntry(t, "implement.upper.rare.effect", "新剑"),
		},
	})
	combatant, err := combatantFromState(cultivation.ReferenceRules(),
		state, storySkillRegistry(cultivation.ReferenceRules()), "protagonist", CombatantSpec{Side: cultivation.SidePlayer})
	if err != nil {
		t.Fatalf("the protagonist must be readable as a combatant: %v", err)
	}
	session := cultivation.CombatSession{
		ID: "s1", Round: 1, Combatants: []cultivation.Combatant{combatant},
		Preparations: map[string]cultivation.Preparation{},
	}
	next, _ := cultivation.ResolveExchange(session, map[string]cultivation.CombatAction{
		"protagonist": {Kind: cultivation.ActionSwapEquipment, SwapInstanceID: "新剑"},
	}, halfDraw)

	first := appliedXiuxianState(t, state, xiuxianEquipmentSwapSettlementOps(state, system, next, "ev1"))
	second := appliedXiuxianState(t, first, xiuxianEquipmentSwapSettlementOps(first, system, next, "ev1"))

	held, _ := xiuxianEquipmentEntry(second, "protagonist", xiuxianFieldImplement)
	if got := xiuxianItemName(held); got != "新剑" {
		t.Errorf("a retried settlement must leave the same copy in hand, got %q", got)
	}
	pack, _ := xiuxianEquipmentEntry(second, "protagonist", xiuxianFieldImportantItems)
	if _, back := pack["旧剑"]; !back {
		t.Error("a retried settlement must not lose the copy that was put away")
	}
	if _, dup := pack["新剑"]; dup {
		t.Error("a retried settlement must not return the worn copy to the pack")
	}
}
