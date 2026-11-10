package interactive

import (
	"encoding/json"
	"reflect"
	"ta/internal/cultivation"
	"testing"
)

func TestReviewBattleSwapStillChecksStageEligibility(t *testing.T) {
	definition, ok := storyItemCatalog(cultivation.ReferenceRules()).Definition("implement.top.unique")
	if !ok || definition.UsableBy(swapStage) == nil {
		t.Fatal("fixture must use an above-stage weapon")
	}
	session := swapBattle(t, nil, map[string]any{"高阶剑": packedEntry(t, "implement.top.unique", "高阶剑")})
	next, outcome := cultivation.ResolveExchange(session, map[string]cultivation.CombatAction{
		"protagonist": {Kind: cultivation.ActionSwapEquipment, SwapInstanceID: "高阶剑"},
	}, halfDraw)
	if weapon, held := next.Combatants[0].EquipmentInSlot(cultivation.EquipmentSlotWeapon); held && weapon.DefID == definition.ID {
		t.Fatalf("qi-stage character equipped a foundation-stage weapon through battle swap; attack=%d steps=%+v", next.Combatants[0].Attributes.Attack, outcome.Steps)
	}
}

func TestTreasureEffectsPayOnlyWhenUsed(t *testing.T) {
	strike, _ := cultivation.SkillByID("strike")
	for _, tc := range []struct {
		name                     string
		action                   cultivation.CombatAction
		enemy                    cultivation.CombatAction
		mana, spent              int
		attack, defense, support bool
	}{
		{"attack alone", cultivation.CombatAction{Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "enemy"}, cultivation.CombatAction{Kind: cultivation.ActionSurrender}, 100, 4, true, false, false},
		{"rest does not cast treasures", cultivation.CombatAction{Kind: cultivation.ActionSurrender}, cultivation.CombatAction{Kind: cultivation.ActionSurrender}, 100, 0, false, false, false},
		{"attack and defence coexist", cultivation.CombatAction{Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "enemy"}, cultivation.CombatAction{Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "protagonist"}, 100, 8, true, true, false},
		{"empty mana uses bare attack", cultivation.CombatAction{Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "enemy"}, cultivation.CombatAction{Kind: cultivation.ActionSurrender}, 0, 0, false, false, false},
		{"escape uses perception", cultivation.CombatAction{Kind: cultivation.ActionEscape}, cultivation.CombatAction{Kind: cultivation.ActionSurrender}, 100, 4, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := swapBattle(t, map[string]any{
				xiuxianFieldAttackTreasure:  wornEntry(t, "treasure.attack.middle", "火葫芦"),
				xiuxianFieldDefenseTreasure: wornEntry(t, "treasure.defense.middle", "护身镜"),
				xiuxianFieldSupportTreasure: wornEntry(t, "treasure.support.upper.effect", "灵目镜"),
			}, nil)
			session.Combatants[0].Attributes.Mana = tc.mana
			next, out := cultivation.ResolveExchange(session, map[string]cultivation.CombatAction{"protagonist": tc.action, "enemy": tc.enemy}, halfDraw)
			actor := next.Combatants[0]
			if got := tc.mana - actor.Attributes.Mana; got != tc.spent {
				t.Fatalf("spent %d, want %d; %+v", got, tc.spent, out)
			}
			for slot, want := range map[cultivation.EquipmentSlot]bool{cultivation.EquipmentSlotAttackTreasure: tc.attack, cultivation.EquipmentSlotDefenseTreasure: tc.defense, cultivation.EquipmentSlotSupportTreasure: tc.support} {
				copy, ok := actor.EquipmentInSlot(slot)
				if !ok || copy.Driven != want {
					t.Fatalf("%s driven=%v, want %v", slot, copy.Driven, want)
				}
			}
			if !tc.attack && actor.Attributes.Attack != 44 {
				t.Fatalf("unpaid attack bonus survived: %d", actor.Attributes.Attack)
			}
			if session.Combatants[0].Attributes.Mana != tc.mana {
				t.Fatal("exchange mutated original session")
			}
		})
	}
}

func TestTreasureDefencePaysOnceForMultipleAttackers(t *testing.T) {
	session := swapBattle(t, map[string]any{xiuxianFieldDefenseTreasure: wornEntry(t, "treasure.defense.middle", "护身镜")}, nil)
	other := session.Combatants[1]
	other.ID = "enemy2"
	session.Combatants = append(session.Combatants, other)
	strike, _ := cultivation.SkillByID("strike")
	before := session.Combatants[0].Attributes.Mana
	next, _ := cultivation.ResolveExchange(session, map[string]cultivation.CombatAction{
		"protagonist": {Kind: cultivation.ActionSurrender},
		"enemy":       {Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "protagonist"},
		"enemy2":      {Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "protagonist"},
	}, halfDraw)
	if spent := before - next.Combatants[0].Attributes.Mana; spent != 4 {
		t.Fatalf("defence charged %d for one exchange, want 4", spent)
	}
}

func TestSwappedTreasureWaitsUntilNextExchangeAndSurvivesReload(t *testing.T) {
	session := swapBattle(t, nil, map[string]any{"护身镜": packedEntry(t, "treasure.defense.middle", "护身镜")})
	strike, _ := cultivation.SkillByID("strike")
	before := session.Combatants[0].Attributes.Mana
	next, _ := cultivation.ResolveExchange(session, map[string]cultivation.CombatAction{
		"protagonist": {Kind: cultivation.ActionSwapEquipment, SwapInstanceID: "护身镜"},
		"enemy":       {Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "protagonist"},
	}, halfDraw)
	if next.Combatants[0].Attributes.Mana != before || next.Combatants[0].Attributes.Defense != 22 {
		t.Fatal("swapping also drove the new treasure")
	}
	data, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	var restored cultivation.CombatSession
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	again, _ := cultivation.ResolveExchange(restored, map[string]cultivation.CombatAction{
		"protagonist": {Kind: cultivation.ActionSurrender}, "enemy": {Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "protagonist"},
	}, halfDraw)
	if again.Combatants[0].Attributes.Mana != before-4 || again.Combatants[0].Attributes.Defense != 25 {
		t.Fatalf("restored treasure did not pay and defend: %+v", again.Combatants[0])
	}
}

func TestTransferReservationsAndRefusalsPreserveStock(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		requests                            []TurnItemAcquire
		acquired, refused, donor, recipient int
	}{
		{"two partial gifts", []TurnItemAcquire{{DefID: "pill.restore.health.lower", Name: "回春丹", From: "断道客", Quantity: 2}, {DefID: "pill.restore.health.lower", Name: "回春丹", From: "断道客", Quantity: 2}}, 2, 0, 1, 4},
		{"second gift overdraws", []TurnItemAcquire{{DefID: "pill.restore.health.lower", Name: "回春丹", From: "断道客", Quantity: 3}, {DefID: "pill.restore.health.lower", Name: "回春丹", From: "断道客", Quantity: 3}}, 1, 1, 2, 3},
		{"whole stack", []TurnItemAcquire{{DefID: "pill.restore.health.lower", Name: "回春丹", From: "断道客", Quantity: 5}}, 1, 0, 0, 5},
		{"overdraw", []TurnItemAcquire{{DefID: "pill.restore.health.lower", Name: "回春丹", From: "断道客", Quantity: 6}}, 0, 1, 5, 0},
		{"definition mismatch", []TurnItemAcquire{{DefID: "pill.boost.attack.middle", Name: "回春丹", From: "断道客", Quantity: 1}}, 0, 1, 5, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := swordOnBothSidesState(t)
			donor := state[actorStateRoot].(map[string]any)["断道客"].(map[string]any)["state"].(map[string]any)
			donor[xiuxianFieldImportantItems] = map[string]any{"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(5), "描述": "旧瓷瓶里五颗药", "来历": "自家药炉"}}
			ops, result, _ := xiuxianItemOps(state, xiuxianTestSystem(), "protagonist", TurnItemsInput{Acquire: tc.requests}, "transfer-reservations")
			if len(result.Acquired) != tc.acquired || len(result.Refused) != tc.refused {
				t.Fatalf("unexpected outcome: %+v", result)
			}
			after := appliedXiuxianState(t, state, ops)
			for actor, want := range map[string]int{"断道客": tc.donor, "protagonist": tc.recipient} {
				pack, _ := xiuxianEquipmentEntry(after, actor, xiuxianFieldImportantItems)
				entry, present := pack["回春丹"].(map[string]any)
				got := 0
				if present {
					got = xiuxianCarriedQuantity(entry)
				}
				if got != want {
					t.Fatalf("%s stock=%d, want %d", actor, got, want)
				}
				if present && (xiuxianEntryText(entry, "描述") != "旧瓷瓶里五颗药" || xiuxianEntryText(entry, "来历") != "自家药炉") {
					t.Fatalf("lost provenance: %+v", entry)
				}
			}
			pack, _ := xiuxianEquipmentEntry(state, "断道客", xiuxianFieldImportantItems)
			if xiuxianCarriedQuantity(pack["回春丹"].(map[string]any)) != 5 {
				t.Fatal("planning mutated the original state")
			}
		})
	}
}

func TestRejectedTransferDoesNotDebitDonor(t *testing.T) {
	state := swordOnBothSidesState(t)
	player := state[actorStateRoot].(map[string]any)["protagonist"].(map[string]any)["state"].(map[string]any)
	player[xiuxianFieldImportantItems] = map[string]any{"青锋剑": packedEntry(t, "implement.lower.common", "青锋剑")}
	ops, result, _ := xiuxianItemOps(state, xiuxianTestSystem(), "protagonist", TurnItemsInput{Acquire: []TurnItemAcquire{{DefID: "implement.middle.rare", Name: "青锋剑", From: "断道客"}}}, "conflicting-recipient")
	if len(ops) != 0 || len(result.Refused) != 1 {
		t.Fatalf("refused transfer changed ownership: %+v %+v", ops, result)
	}
}

func TestBattleStageRefusalKeepsEquipmentAndResources(t *testing.T) {
	session := swapBattle(t, map[string]any{xiuxianFieldImplement: wornEntry(t, "implement.lower.common", "旧剑")}, map[string]any{"高阶剑": packedEntry(t, "implement.top.unique", "高阶剑")})
	next, out := cultivation.ResolveExchange(session, map[string]cultivation.CombatAction{"protagonist": {Kind: cultivation.ActionSwapEquipment, SwapInstanceID: "高阶剑"}, "enemy": {Kind: cultivation.ActionSurrender}}, halfDraw)
	if !reflect.DeepEqual(session.Combatants[0].Equipment, next.Combatants[0].Equipment) || !reflect.DeepEqual(session.Combatants[0].Spares, next.Combatants[0].Spares) || session.Combatants[0].Attributes.Mana != next.Combatants[0].Attributes.Mana {
		t.Fatal("refused swap changed equipment, pack or mana")
	}
	if out.Steps[0].ReasonKey != "reason.equipment_stage_too_low" || !out.Steps[0].ActionRejected {
		t.Fatalf("missing authoritative refusal: %+v", out)
	}
}

func TestReviewTransferDoesNotReplaceTheDonorsDefinition(t *testing.T) {
	system := xiuxianTestSystem()
	state := swordOnBothSidesState(t)
	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{Acquire: []TurnItemAcquire{{
		DefID: "implement.top.unique", Name: "青锋剑", From: "断道客",
	}}}, "review-definition-change")
	if len(result.Acquired) > 0 {
		after := appliedXiuxianState(t, state, ops)
		pack, _ := xiuxianEquipmentEntry(after, "protagonist", xiuxianFieldImportantItems)
		entry, _ := pack["青锋剑"].(map[string]any)
		t.Fatalf("transferring an existing middle-grade blade replaced its definition with %q", xiuxianEntryText(entry, "定义"))
	}
}

func TestReviewGivingOneDoseKeepsTheDonorsOtherFour(t *testing.T) {
	system := xiuxianTestSystem()
	state := swordOnBothSidesState(t)
	donor := state[actorStateRoot].(map[string]any)["断道客"].(map[string]any)["state"].(map[string]any)
	donor[xiuxianFieldImportantItems] = map[string]any{"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(5)}}
	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{Acquire: []TurnItemAcquire{{
		DefID: "pill.restore.health.lower", Name: "回春丹", From: "断道客", Quantity: 1,
	}}}, "review-one-dose")
	if len(result.Acquired) != 1 {
		t.Fatalf("valid one-dose transfer was refused: %+v", result)
	}
	after := appliedXiuxianState(t, state, ops)
	donorPack, _ := xiuxianEquipmentEntry(after, "断道客", xiuxianFieldImportantItems)
	donorEntry, _ := donorPack["回春丹"].(map[string]any)
	receiverPack, _ := xiuxianEquipmentEntry(after, "protagonist", xiuxianFieldImportantItems)
	receiverEntry, _ := receiverPack["回春丹"].(map[string]any)
	left, received := 0, 0
	if donorEntry != nil {
		left = xiuxianCarriedQuantity(donorEntry)
	}
	if receiverEntry != nil {
		received = xiuxianCarriedQuantity(receiverEntry)
	}
	if left != 4 || received != 1 {
		t.Fatalf("giving 1 out of 5 doses left donor=%d receiver=%d total=%d; want 4,1,5", left, received, left+received)
	}
}

func TestReviewTreasureCostIsPaidWhenItsCombatBonusApplies(t *testing.T) {
	session := swapBattle(t, map[string]any{
		xiuxianFieldAttackTreasure: wornEntry(t, "treasure.attack.middle", "火葫芦"),
	}, nil)
	definition, _ := storyItemCatalog(cultivation.ReferenceRules()).Definition("treasure.attack.middle")
	cost := definition.OperatingManaCost(session.Combatants[0].Cultivation)
	if cost <= 0 || session.Combatants[0].Attributes.Attack <= 44 {
		t.Fatal("fixture must carry an effective treasure with a declared operating cost")
	}
	before := session.Combatants[0].Attributes.Mana
	strike, ok := cultivation.SkillByID("strike")
	if !ok {
		t.Fatal("missing strike")
	}
	next, outcome := cultivation.ResolveExchange(session, map[string]cultivation.CombatAction{
		"protagonist": {Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "enemy"},
		"enemy":       {Kind: cultivation.ActionSurrender, OfferText: "停手"},
	}, halfDraw)
	spent := before - next.Combatants[0].Attributes.Mana
	if spent < cost {
		t.Fatalf("treasure attack bonus applied but spent=%d mana, declared operating cost=%d; steps=%+v", spent, cost, outcome.Steps)
	}
}
