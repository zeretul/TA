package interactive

import (
	"strings"
	"testing"

	"ta/internal/cultivation"
)

// The tests here cover §4.1's equipment loop as it happens during play: a copy is
// obtained, taken into hand, priced, put down and returns to the pack.
//
// Before this module existed, 主法器 and 护具 were ordinary narrative fields. A
// story could write {名称: "斩仙剑", 品级: "顶品"} into the slot and the settlement
// would price it, with nothing checking that the character had ever obtained the
// sword, that its definition existed, or that a 炼气 cultivator could operate a
// 顶品 treasure at all. These tests assert the checks that close that hole.

// TestAnItemMustBeAcquiredBeforeItCanBeEquipped is §7's separation of a
// definition from a copy: the catalogue holding an entry means nothing about the
// character owning one.
func TestAnItemMustBeAcquiredBeforeItCanBeEquipped(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)

	ops, result, ok := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "青锋剑"}},
	}, "ev1")
	if !ok {
		t.Fatal("a tracked character with the declared fields must accept the module")
	}
	if len(ops) != 0 {
		t.Fatalf("equipping an unheld copy wrote state: %+v", ops)
	}
	if len(result.Refused) != 1 {
		t.Fatalf("refused = %+v, want exactly one refusal", result.Refused)
	}
	if !strings.Contains(result.Refused[0].Reason, "行囊中没有这件物品") {
		t.Errorf("refusal reason = %q, want it to name the missing copy", result.Refused[0].Reason)
	}
}

// TestAcquiringWritesTheDefinitionIntoThePack is the obtain half of the loop.
func TestAcquiringWritesTheDefinitionIntoThePack(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)

	ops, result, ok := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "implement.lower.common", Name: "青锋剑"}},
	}, "ev1")
	if !ok || len(result.Acquired) != 1 {
		t.Fatalf("acquired = %+v ok=%v", result.Acquired, ok)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, held := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldImportantItems)
	if !held {
		t.Fatal("the pack was not written")
	}
	entry, ok := pack["青锋剑"].(map[string]any)
	if !ok {
		t.Fatalf("pack = %+v, want a 青锋剑 entry", pack)
	}
	if got := xiuxianEntryText(entry, "定义"); got != "implement.lower.common" {
		t.Errorf("recorded definition = %q, want implement.lower.common", got)
	}
}

// TestAnUnknownDefinitionIsRefusedRatherThanRecorded is §8's "a refused
// operation spends nothing" applied to acquisition: an identifier the catalogue
// does not hold has no published numbers to be priced from.
func TestAnUnknownDefinitionIsRefusedRatherThanRecorded(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "implement.invented.treasure", Name: "自造神器"}},
	}, "ev1")
	if len(ops) != 0 {
		t.Fatalf("an unknown definition wrote state: %+v", ops)
	}
	if len(result.Refused) != 1 {
		t.Fatalf("refused = %+v, want one refusal", result.Refused)
	}
}

// TestAStageTheCharacterHasNotReachedRefusesTheEquip is §4.1's core: 适用阶段不是
// 点击颜色就能越过. A 顶品 treasure is not merely a stronger 下品 one; a 炼气
// character may not be able to operate it.
func TestAStageTheCharacterHasNotReachedRefusesTheEquip(t *testing.T) {
	system := xiuxianTestSystem()
	// The character holds the treasure — the story recorded that they own it —
	// and still cannot wield it.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"斩仙剑": map[string]any{"定义": "implement.top.unique", "数量": float64(1)},
		},
	})

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "斩仙剑"}},
	}, "ev1")
	if len(ops) != 0 {
		t.Fatalf("a below-stage equip wrote state: %+v", ops)
	}
	if len(result.Refused) != 1 {
		t.Fatalf("refused = %+v, want one refusal", result.Refused)
	}
	if !strings.Contains(result.Refused[0].Reason, "境界不足") {
		t.Errorf("refusal reason = %q, want it to name the stage", result.Refused[0].Reason)
	}
}

// TestAReachableStageEquipsAndContributes is the positive half of the same rule,
// priced end to end through the settlement.
func TestAReachableStageEquipsAndContributes(t *testing.T) {
	system := xiuxianTestSystem()
	// 中品稀有 is 15%: 40 × 1.15 = 46.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"青锋剑": map[string]any{"定义": "implement.middle.rare", "数量": float64(1)},
		},
	})

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "青锋剑"}},
	}, "ev1")
	if len(result.Equipped) != 1 {
		t.Fatalf("equipped = %+v, want one copy", result.Equipped)
	}
	applied := appliedXiuxianState(t, state, ops)
	// The copy left the pack and entered the slot: one real object, not a stack.
	pack, _ := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldImportantItems)
	if _, stillHeld := pack["青锋剑"]; stillHeld {
		t.Error("the equipped copy stayed in the pack; equipment is one real object")
	}
	settled := xiuxianSettle(t, applied)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 46 {
		t.Errorf("attack with a middle-rare implement = %v, want 46 (40 × 1.15)", got)
	}
}

// TestEquippingReplacesTheSlotAndReturnsTheOldCopy is §4.1's one-at-a-time rule
// read as a property of contribution rather than ownership: the displaced item is
// still the character's.
func TestEquippingReplacesTheSlotAndReturnsTheOldCopy(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImplement: map[string]any{
			"名称": "旧剑", "定义": "implement.lower.common",
			"品级": "下品", "稀有度": "常见", "主要属性预算": float64(0),
		},
		xiuxianFieldImportantItems: map[string]any{
			"青锋剑": map[string]any{"定义": "implement.middle.rare", "数量": float64(1)},
		},
	})

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "青锋剑"}},
	}, "ev1")
	if len(result.Equipped) != 1 {
		t.Fatalf("equipped = %+v", result.Equipped)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldImportantItems)
	if _, returned := pack["旧剑"]; !returned {
		t.Error("the displaced implement was destroyed instead of returning to the pack")
	}
	settled := xiuxianSettle(t, applied)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 46 {
		t.Errorf("attack after the swap = %v, want 46", got)
	}
}

// TestUnequippingStopsTheBonusAndReturnsTheCopy is the other end of the loop.
func TestUnequippingStopsTheBonusAndReturnsTheCopy(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImplement: map[string]any{
			"名称": "青锋剑", "定义": "implement.middle.rare",
			"品级": "中品", "稀有度": "稀有", "主要属性预算": float64(5),
		},
	})

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Unequip: []TurnItemUnequip{{Slot: xiuxianFieldImplement}},
	}, "ev1")
	if len(result.Unequipped) != 1 {
		t.Fatalf("unequipped = %+v, want one slot", result.Unequipped)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldImportantItems)
	entry, returned := pack["青锋剑"].(map[string]any)
	if !returned {
		t.Fatal("the unequipped copy did not return to the pack")
	}
	if got := xiuxianEntryText(entry, "定义"); got != "implement.middle.rare" {
		t.Errorf("returned copy names definition %q, want implement.middle.rare", got)
	}
	settled := xiuxianSettle(t, applied)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 40 {
		t.Errorf("attack after unequipping = %v, want the bare 40", got)
	}
}

// TestAPillCannotBeEquipped keeps §4.2's consumption separate from §4.1's
// equipment: a dose is spent, and wearing one would let it be taken for free.
func TestAPillCannotBeEquipped(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"回灵丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(2)},
		},
	})

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "回灵丹"}},
	}, "ev1")
	if len(ops) != 0 {
		t.Fatalf("equipping a pill wrote state: %+v", ops)
	}
	if len(result.Refused) != 1 {
		t.Fatalf("refused = %+v, want one refusal", result.Refused)
	}
	if !strings.Contains(result.Refused[0].Reason, "丹药") {
		t.Errorf("refusal reason = %q, want it to say a pill is not worn", result.Refused[0].Reason)
	}
}

// TestAnEquippedImplementCostsManaToDrive is 数值规则 line 128: 普通攻击的真实法器
// 操控成本依装备定义，不因按普攻按钮免费施展.
func TestAnEquippedImplementCostsManaToDrive(t *testing.T) {
	// 上品稀有 with a special effect declares a 2% operating fraction. At 炼气六层
	// the bare mana maximum is 160, so one exchange costs ceil(0.02 × 160) = 4.
	state := equipmentState(t, cultivation.StageKey(cultivation.Qi(6)), xiuxianFieldImplement, map[string]any{
		"名称": "玄水幡", "定义": "implement.upper.rare.effect",
		"品级": "上品", "稀有度": "稀有", "主要属性预算": float64(0),
	})
	if got := xiuxianImplementOperatingCost(cultivation.ReferenceRules(),
		state, "protagonist"); got != 4 {
		t.Errorf("operating cost = %d, want 4 (ceil 0.02 × 160)", got)
	}
}

// TestAnImplementWithoutADeclaredCostIsFreeToSwing is the ordinary case, and the
// distinction between "no cost declared" and "a cost of zero".
func TestAnImplementWithoutADeclaredCostIsFreeToSwing(t *testing.T) {
	state := equipmentState(t, cultivation.StageKey(cultivation.Qi(6)), xiuxianFieldImplement, map[string]any{
		"名称": "青锋剑", "定义": "implement.lower.common",
		"品级": "下品", "稀有度": "常见", "主要属性预算": float64(0),
	})
	if got := xiuxianImplementOperatingCost(cultivation.ReferenceRules(),
		state, "protagonist"); got != 0 {
		t.Errorf("operating cost = %d, want 0 for an entry that declares none", got)
	}
}

// TestTheOperatingCostComesFromTheDefinitionNotTheRecord makes the price
// unforgeable: a story that writes a smaller number into the slot cannot make its
// own treasure cheaper.
func TestTheOperatingCostComesFromTheDefinitionNotTheRecord(t *testing.T) {
	state := equipmentState(t, cultivation.StageKey(cultivation.Qi(6)), xiuxianFieldImplement, map[string]any{
		"名称": "玄水幡", "定义": "implement.upper.rare.effect",
		"品级": "上品", "稀有度": "稀有", "主要属性预算": float64(0),
		"操控耗灵": float64(0),
	})
	if got := xiuxianImplementOperatingCost(cultivation.ReferenceRules(),
		state, "protagonist"); got != 4 {
		t.Errorf("operating cost = %d, want the definition's 4 regardless of the recorded figure", got)
	}
}

// TestTheEquipmentSlotsAreRulesOwned closes the hole the module exists for: a
// story can no longer write a 顶品 treasure straight into the slot.
func TestTheEquipmentSlotsAreRulesOwned(t *testing.T) {
	system := xiuxianTestSystem()
	for _, field := range xiuxianSlotOrder {
		if !IsXiuxianSettledField(system, field) {
			t.Errorf("%s is not rules-owned, so the narrative can write it freely", field)
		}
	}
}

// TestReacquiringAnImplementDoesNotStackIt is the count half of "equipment is
// one real object". A live turn acquired a 青锋剑 the story had already recorded
// in the pack, and the pack came out holding two of a sword the character has
// exactly one of — which then read as a stack the slot could draw from.
func TestReacquiringAnImplementDoesNotStackIt(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"青锋剑": map[string]any{"定义": "implement.middle.rare", "数量": float64(1)},
		},
	})

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "implement.middle.rare", Name: "青锋剑", Quantity: 1}},
	}, "ev1")
	if len(result.Acquired) != 1 {
		t.Fatalf("acquired = %+v", result.Acquired)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldImportantItems)
	entry, _ := pack["青锋剑"].(map[string]any)
	if got := xiuxianCarriedQuantity(entry); got != 1 {
		t.Errorf("held copies = %v, want 1: one sword cannot be recorded as a stack", got)
	}
}

// TestAcquiringAPillStillStacks keeps the fix narrow: a consumable really is a
// stack, and taking two more must still add to the count.
func TestAcquiringAPillStillStacks(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"回灵丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(2)},
		},
	})

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "pill.restore.health.lower", Name: "回灵丹", Quantity: 3}},
	}, "ev1")
	if len(result.Acquired) != 1 {
		t.Fatalf("acquired = %+v", result.Acquired)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldImportantItems)
	entry, _ := pack["回灵丹"].(map[string]any)
	if got := xiuxianCarriedQuantity(entry); got != 5 {
		t.Errorf("held pills = %v, want 5", got)
	}
}

// TestTheSlotInstructionsPointAtTheItemModule keeps the handbook honest.
//
// The slot fields are refused at the state-update gate, so an instruction that
// still told the model to write 品级/稀有度 into them by hand would send it into
// a rejection it cannot learn from — and in a live run it did exactly that. The
// instruction the model actually reads must name the module instead.
func TestTheSlotInstructionsPointAtTheItemModule(t *testing.T) {
	system := xiuxianTestSystem()
	template := actorStateTemplateByID(system, "protagonist")
	for _, fieldID := range xiuxianSlotOrder {
		field, ok := actorStateFieldByID(template, fieldID)
		if !ok {
			t.Fatalf("%s is not declared by the protagonist template", fieldID)
		}
		if !strings.Contains(field.UpdateInstruction, "items.") {
			t.Errorf("%s instruction does not name the item module: %q", fieldID, field.UpdateInstruction)
		}
		if strings.Contains(field.UpdateInstruction, "键使用") {
			t.Errorf("%s instruction still tells the model to write the slot by hand: %q", fieldID, field.UpdateInstruction)
		}
	}
}

// TestDeployingAFormationPaysItsActivationCost is §4.3's 启动耗操作者阶段基准灵力10%,
// 向上取整. At 炼气六层 the bare mana maximum is 160, so raising a formation costs
// ceil(0.10 × 160) = 16 and the pool has to show it.
func TestDeployingAFormationPaysItsActivationCost(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"聚灵阵盘": map[string]any{"定义": "formation.attack.middle", "数量": float64(1)},
		},
		xiuxianFieldMana: float64(160),
	})

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "聚灵阵盘"}},
	}, "ev1")
	if len(result.Equipped) != 1 {
		t.Fatalf("equipped = %+v, want the formation deployed", result.Equipped)
	}
	applied := appliedXiuxianState(t, state, ops)
	if got := xiuxianStoredInt(applied, "protagonist", xiuxianFieldMana); got != 144 {
		t.Errorf("mana after deploying = %d, want 144 (160 − 16)", got)
	}
	slot, ok := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldFormation)
	if !ok {
		t.Fatal("the formation slot was not written")
	}
	if got := xiuxianEntryText(slot, "增益"); got != "攻击" {
		t.Errorf("recorded 增益 = %q, want 攻击 from the definition", got)
	}
	if got, _ := actorStateNumber(slot["启动耗灵"]); got != 16 {
		t.Errorf("recorded 启动耗灵 = %v, want 16", got)
	}
	if got, _ := actorStateNumber(slot["每轮维持"]); got != 4 {
		t.Errorf("recorded 每轮维持 = %v, want 4 (ceil 0.02 × 160)", got)
	}
}

// TestAnUnaffordableFormationIsRefusedRatherThanRecorded is §8's "a refused
// operation spends nothing" applied to deployment: writing the slot and then
// letting the cost be nobody's problem would grant the bonus for free.
func TestAnUnaffordableFormationIsRefusedRatherThanRecorded(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"聚灵阵盘": map[string]any{"定义": "formation.attack.middle", "数量": float64(1)},
		},
		xiuxianFieldMana: float64(10),
	})

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "聚灵阵盘"}},
	}, "ev1")
	if len(ops) != 0 {
		t.Fatalf("an unaffordable deployment wrote state: %+v", ops)
	}
	if len(result.Refused) != 1 {
		t.Fatalf("refused = %+v, want one refusal", result.Refused)
	}
	if !strings.Contains(result.Refused[0].Reason, "灵力不足") {
		t.Errorf("refusal reason = %q, want it to name the mana shortfall", result.Refused[0].Reason)
	}
}

// TestADeployedFormationRaisesAttackAndReturnsOnUnequip is the whole loop for a
// 阵盘: the copy is held, deployed, priced, and taken back. 中品增攻阵 is 8%, so
// attack is 40 × 1.08 = 43 while it stands.
func TestADeployedFormationRaisesAttackAndReturnsOnUnequip(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"聚灵阵盘": map[string]any{"定义": "formation.attack.middle", "数量": float64(1)},
		},
		xiuxianFieldMana: float64(160),
	})

	ops, _, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "聚灵阵盘"}},
	}, "ev1")
	deployed := appliedXiuxianState(t, state, ops)
	settled := xiuxianSettle(t, deployed)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 43 {
		t.Errorf("attack inside the deployed formation = %v, want 43 (40 × 1.08)", got)
	}

	ops, result, _ := xiuxianItemOps(deployed, system, "protagonist", TurnItemsInput{
		Unequip: []TurnItemUnequip{{Slot: xiuxianFieldFormation}},
	}, "ev2")
	if len(result.Unequipped) != 1 {
		t.Fatalf("unequipped = %+v, want the formation dismantled", result.Unequipped)
	}
	dismantled := appliedXiuxianState(t, deployed, ops)
	pack, _ := xiuxianEquipmentEntry(dismantled, "protagonist", xiuxianFieldImportantItems)
	if _, returned := pack["聚灵阵盘"].(map[string]any); !returned {
		t.Error("the dismantled 阵盘 did not return to the pack")
	}
	settled = xiuxianSettle(t, dismantled)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 40 {
		t.Errorf("attack after dismantling = %v, want the bare 40", got)
	}
}

// TestTheFormationPriceComesFromTheDefinitionNotTheRecord keeps the bonus
// unforgeable: a slot that names a registered 阵盘 is priced from its definition,
// so a story cannot write a larger 增益 beside it.
func TestTheFormationPriceComesFromTheDefinitionNotTheRecord(t *testing.T) {
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldFormation: map[string]any{
			"名称": "聚灵阵盘", "定义": "formation.attack.middle",
			"品级": "中品", "稀有度": "常见", "增益": "攻击", "维持中": "是",
		},
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 43 {
		t.Errorf("attack = %v, want the definition's 8%% (43), not a recorded figure", got)
	}
}

// TestAcquiringADifferentDefinitionUnderAHeldNameIsRefused is the acquire-side
// counterpart of the trade path's name-collision rule. One pack key describes one
// definition; letting a 下品青锋剑 be recorded over a 中品青锋剑 would make every
// later price, sale and transfer ambiguous about which blade the key means.
func TestAcquiringADifferentDefinitionUnderAHeldNameIsRefused(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"青锋剑": map[string]any{"定义": "implement.lower.common", "数量": float64(1)},
		},
	})

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "implement.middle.rare", Name: "青锋剑"}},
	}, "ev1")
	if len(ops) != 0 {
		t.Fatalf("a name collision wrote state: %+v", ops)
	}
	if len(result.Refused) != 1 || !strings.Contains(result.Refused[0].Reason, "同名物品") {
		t.Fatalf("refused = %+v, want the name-collision reason", result.Refused)
	}
}

// TestAcquiringRecordsTheCopysDescriptionAndSource is §4.1's journal half: a copy
// carries what it looks like and where it came from, written once when it is
// first obtained.
func TestAcquiringRecordsTheCopysDescriptionAndSource(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)

	ops, _, ok := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{
			DefID:       "implement.lower.common",
			Name:        "青锋剑",
			Description: "剑身泛着一层旧银似的光，刃口缺了一角，握着却比看上去轻。",
			Source:      "城西旧货铺，老板说它上一任主人没还上酒钱。",
		}},
	}, "ev1")
	if !ok {
		t.Fatal("a tracked character with the declared fields must accept the module")
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldImportantItems)
	entry, _ := pack["青锋剑"].(map[string]any)
	if got := xiuxianEntryText(entry, "描述"); !strings.Contains(got, "旧银") {
		t.Errorf("recorded description = %q, want the sentence the acquisition supplied", got)
	}
	if got := xiuxianEntryText(entry, "来历"); !strings.Contains(got, "旧货铺") {
		t.Errorf("recorded source = %q, want the sentence the acquisition supplied", got)
	}
}

// TestTheCopysWordingSurvivesEquippingAndUnequipping is "复看稳定": taking a sword
// in hand and putting it down again is a move, not a new acquisition, so the two
// sentences the player already read must come back unchanged.
func TestTheCopysWordingSurvivesEquippingAndUnequipping(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)

	acquired, _, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{
			DefID:       "implement.middle.rare",
			Name:        "青锋剑",
			Description: "剑脊上一道细纹，像有人用指甲划过。",
			Source:      "师父留下的三件遗物之一。",
		}},
	}, "ev1")
	afterAcquire := appliedXiuxianState(t, state, acquired)

	equipped, equipResult, _ := xiuxianItemOps(afterAcquire, system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "青锋剑"}},
	}, "ev2")
	if len(equipResult.Equipped) != 1 {
		t.Fatalf("equipped = %+v, want one copy", equipResult.Equipped)
	}
	afterEquip := appliedXiuxianState(t, afterAcquire, equipped)
	slot, _ := xiuxianEquipmentEntry(afterEquip, "protagonist", xiuxianFieldImplement)
	if got := xiuxianEntryText(slot, "描述"); !strings.Contains(got, "剑脊") {
		t.Errorf("the slot's description = %q, want the pack's wording carried over", got)
	}
	if got := xiuxianEntryText(slot, "来历"); !strings.Contains(got, "师父") {
		t.Errorf("the slot's source = %q, want the pack's wording carried over", got)
	}

	unequipped, _, _ := xiuxianItemOps(afterEquip, system, "protagonist", TurnItemsInput{
		Unequip: []TurnItemUnequip{{Slot: xiuxianFieldImplement}},
	}, "ev3")
	final := appliedXiuxianState(t, afterEquip, unequipped)
	pack, _ := xiuxianEquipmentEntry(final, "protagonist", xiuxianFieldImportantItems)
	entry, _ := pack["青锋剑"].(map[string]any)
	if got := xiuxianEntryText(entry, "描述"); !strings.Contains(got, "剑脊") {
		t.Errorf("description after the round trip = %q, want it unchanged", got)
	}
	if got := xiuxianEntryText(entry, "来历"); !strings.Contains(got, "师父") {
		t.Errorf("source after the round trip = %q, want it unchanged", got)
	}
}

// TestReacquiringKeepsTheWordingItWasFirstGiven is the other half of "written
// once": a later turn that mentions the same sword must not be able to restyle it.
func TestReacquiringKeepsTheWordingItWasFirstGiven(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"回灵丹": map[string]any{
				"定义": "pill.restore.health.lower", "数量": float64(1),
				"描述": "蜡壳上压着半个药铺的印，剥开时有点黏手。",
				"来历": "巷口老郎中塞的。",
			},
		},
	})

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{
			DefID: "pill.restore.health.lower", Name: "回灵丹", Quantity: 1,
			Description: "一颗通体雪白的新丹。", Source: "山门库房。",
		}},
	}, "ev1")
	if len(result.Acquired) != 1 {
		t.Fatalf("acquired = %+v", result.Acquired)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldImportantItems)
	entry, _ := pack["回灵丹"].(map[string]any)
	if got := xiuxianCarriedQuantity(entry); got != 2 {
		t.Errorf("held doses = %v, want 2: a pill is still a stack", got)
	}
	if got := xiuxianEntryText(entry, "描述"); !strings.Contains(got, "蜡壳") {
		t.Errorf("description = %q, want the first acquisition's wording kept", got)
	}
	if got := xiuxianEntryText(entry, "来历"); !strings.Contains(got, "老郎中") {
		t.Errorf("source = %q, want the first acquisition's wording kept", got)
	}
}

// TestTheItemModuleSurvivesAnUnrelatedStatePath keeps the optional-module rule:
// a turn that obtained and equipped nothing is not a turn awaiting repair.
func TestTheItemModuleSurvivesAnUnrelatedStatePath(t *testing.T) {
	if (&TurnItemsInput{}).Empty() != true {
		t.Fatal("an empty item module must report empty")
	}
	if cloneTurnItemsInput(nil) != nil {
		t.Fatal("cloning nil must not produce a value")
	}
	cloned := cloneTurnItemsInput(&TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "implement.lower.common"}},
	})
	cloned.Acquire[0].DefID = "changed"
	if cloned.Acquire[0].DefID == "implement.lower.common" {
		t.Error("the clone aliased the caller's slice")
	}
}
