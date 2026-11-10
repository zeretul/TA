package interactive

import (
	"testing"

	"ta/internal/cultivation"
)

// These regressions close the defect the 0.5.7 independent review reproduced: a
// 下品 sword was worn, a second sword of the same name but a better grade was
// bought, and putting the first one back turned both into the cheaper one. The
// pack keys a copy by its display name, so a name alone is not identity, and
// every path that adds counts under one name has to check the definition first.
//
// 《自制-装备与物品品级》§4.1 makes 品级 part of what a copy is. Two grades of one
// name are two different objects, so neither may be recorded over the other, and
// a purchase that would force them into one key is refused before the money
// moves rather than charged for a copy the pack cannot hold.

// packEntryInSnapshot reads one recorded pack entry out of a saved branch.
func packEntryInSnapshot(t *testing.T, store *Store, storyID, name string) map[string]any {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	pack, _ := xiuxianEquipmentEntry(snapshot.State, DefaultActorID, xiuxianFieldImportantItems)
	entry, _ := pack[name].(map[string]any)
	return entry
}

// packNamesInSnapshot lists the pack keys, so a test can assert that a second
// grade was kept under a key of its own.
func packNamesInSnapshot(t *testing.T, store *Store, storyID string) []string {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	pack, _ := xiuxianEquipmentEntry(snapshot.State, DefaultActorID, xiuxianFieldImportantItems)
	names := make([]string, 0, len(pack))
	for name := range pack {
		names = append(names, name)
	}
	return names
}

// slotEntryInSnapshot reads one equipment slot out of a saved branch.
func slotEntryInSnapshot(t *testing.T, store *Store, storyID, slot string) map[string]any {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := xiuxianEquipmentEntry(snapshot.State, DefaultActorID, slot)
	return entry
}

// TestBuyingAnotherGradeUnderAWornNameIsRefusedBeforePayment is the exact
// reproduced path: the better sword is refused while the purse is still whole.
func TestBuyingAnotherGradeUnderAWornNameIsRefusedBeforePayment(t *testing.T) {
	store, storyID, _ := paidCopyStory(t, 1000)
	commitPaidTurn(t, store, storyID, "买下品青锋剑", "你数出二十枚灵石，挑了一柄下品青锋剑。", &TurnResult{
		Trade: &TurnTradeInput{
			Merchant: "坊市兵器铺",
			Buys:     []TurnTradeBuy{{DefID: "implement.lower.common", Name: "青锋剑"}},
		},
	})
	slot, err := xiuxianItemSlotForKind(cultivation.KindImplement)
	if err != nil {
		t.Fatal(err)
	}
	commitPaidTurn(t, store, storyID, "佩上青锋剑", "你把青锋剑佩在腰间。", &TurnResult{
		Items: &TurnItemsInput{Equip: []TurnItemEquip{{InstanceID: "青锋剑"}}},
	})
	if got := purseInSnapshot(t, store, storyID); got != 980 {
		t.Fatalf("after the first purchase: purse = %d, want 980", got)
	}
	if worn := slotEntryInSnapshot(t, store, storyID, slot); xiuxianEntryText(worn, "定义") != "implement.lower.common" {
		t.Fatalf("the worn copy is %+v, want the 下品 sword", worn)
	}

	// The same name at another grade. Nothing may be charged, because the pack
	// has no key that can hold both without losing one.
	_, _, _ = store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "再买同名中品剑", Narrative: "你问掌柜有没有品相更好的同名剑。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			Trade: &TurnTradeInput{
				Merchant: "坊市兵器铺",
				Buys:     []TurnTradeBuy{{DefID: "implement.middle.rare", Name: "青锋剑"}},
			},
		},
	})
	if got := purseInSnapshot(t, store, storyID); got != 980 {
		t.Errorf("a refused collision charged the purse: %d, want 980", got)
	}
	if worn := slotEntryInSnapshot(t, store, storyID, slot); xiuxianEntryText(worn, "定义") != "implement.lower.common" {
		t.Errorf("the refused purchase rewrote the worn copy: %+v", worn)
	}

	// Taking the original off must return exactly the one copy that was bought.
	commitPaidTurn(t, store, storyID, "把剑收回行囊", "你把青锋剑收回行囊。", &TurnResult{
		Items: &TurnItemsInput{Unequip: []TurnItemUnequip{{Slot: slot}}},
	})
	entry := packEntryInSnapshot(t, store, storyID, "青锋剑")
	if got := xiuxianCarriedQuantity(entry); got != 1 {
		t.Errorf("after unequipping: pack holds %d copies, want the 1 that was paid for", got)
	}
	if def := xiuxianEntryText(entry, "定义"); def != "implement.lower.common" {
		t.Errorf("after unequipping: the copy records %q, want implement.lower.common", def)
	}
}

// TestTheRefusedCollisionSurvivesAColdReopen is the persistence half: the pack
// that comes back off disk must be the same one the turn settled.
func TestTheRefusedCollisionSurvivesAColdReopen(t *testing.T) {
	store, storyID, root := paidCopyStory(t, 1000)
	commitPaidTurn(t, store, storyID, "买下品青锋剑", "你数出二十枚灵石，挑了一柄下品青锋剑。", &TurnResult{
		Trade: &TurnTradeInput{
			Merchant: "坊市兵器铺",
			Buys:     []TurnTradeBuy{{DefID: "implement.lower.common", Name: "青锋剑"}},
		},
	})
	slot, err := xiuxianItemSlotForKind(cultivation.KindImplement)
	if err != nil {
		t.Fatal(err)
	}
	commitPaidTurn(t, store, storyID, "佩上青锋剑", "你把青锋剑佩在腰间。", &TurnResult{
		Items: &TurnItemsInput{Equip: []TurnItemEquip{{InstanceID: "青锋剑"}}},
	})
	_, _, _ = store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "再买同名中品剑", Narrative: "你问掌柜有没有品相更好的同名剑。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			Trade: &TurnTradeInput{
				Merchant: "坊市兵器铺",
				Buys:     []TurnTradeBuy{{DefID: "implement.middle.rare", Name: "青锋剑"}},
			},
		},
	})
	commitPaidTurn(t, store, storyID, "把剑收回行囊", "你把青锋剑收回行囊。", &TurnResult{
		Items: &TurnItemsInput{Unequip: []TurnItemUnequip{{Slot: slot}}},
	})

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	t.Cleanup(func() { _ = reopened.Close() })

	if got := purseInSnapshot(t, reopened, storyID); got != 980 {
		t.Errorf("after reopening: purse = %d, want 980", got)
	}
	entry := packEntryInSnapshot(t, reopened, storyID, "青锋剑")
	if got := xiuxianCarriedQuantity(entry); got != 1 {
		t.Errorf("after reopening: pack holds %d copies, want 1", got)
	}
	if def := xiuxianEntryText(entry, "定义"); def != "implement.lower.common" {
		t.Errorf("after reopening: the copy records %q, want the 下品 sword that was bought", def)
	}
}

// TestAPurchaseAfterUnequippingStillWorks proves the refusal is about the worn
// collision and not about the name: once the first sword is in the pack, the
// same purchase still goes through and both grades are kept.
func TestAPurchaseAfterUnequippingStillWorks(t *testing.T) {
	store, storyID, root := paidCopyStory(t, 1000)
	commitPaidTurn(t, store, storyID, "买下品青锋剑", "你数出二十枚灵石，挑了一柄下品青锋剑。", &TurnResult{
		Trade: &TurnTradeInput{
			Merchant: "坊市兵器铺",
			Buys:     []TurnTradeBuy{{DefID: "implement.lower.common", Name: "青锋剑"}},
		},
	})
	slot, err := xiuxianItemSlotForKind(cultivation.KindImplement)
	if err != nil {
		t.Fatal(err)
	}
	commitPaidTurn(t, store, storyID, "佩上青锋剑", "你把青锋剑佩在腰间。", &TurnResult{
		Items: &TurnItemsInput{Equip: []TurnItemEquip{{InstanceID: "青锋剑"}}},
	})
	commitPaidTurn(t, store, storyID, "把剑收回行囊", "你先把剑收回行囊，想挑一柄更好的。", &TurnResult{
		Items: &TurnItemsInput{Unequip: []TurnItemUnequip{{Slot: slot}}},
	})

	// The pack now holds one 下品 sword under its name, so a 中品 of that name
	// is a genuine name collision and is refused rather than overwritten. What
	// matters is that no coin moves and the held copy keeps its grade.
	_, _, _ = store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "再买同名中品剑", Narrative: "你又问起同名但品相更好的剑。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			Trade: &TurnTradeInput{
				Merchant: "坊市兵器铺",
				Buys:     []TurnTradeBuy{{DefID: "implement.middle.rare", Name: "青锋剑"}},
			},
		},
	})
	if got := purseInSnapshot(t, store, storyID); got != 980 {
		t.Errorf("a pack-level name collision charged the purse: %d, want 980", got)
	}
	entry := packEntryInSnapshot(t, store, storyID, "青锋剑")
	if def := xiuxianEntryText(entry, "定义"); def != "implement.lower.common" {
		t.Errorf("the held copy records %q, want implement.lower.common", def)
	}

	// The better sword is a different key, so buying it under its own name is
	// the legitimate way to own both.
	commitPaidTurn(t, store, storyID, "买中品剑", "你另外挑了一柄中品剑，数出六十枚灵石。", &TurnResult{
		Trade: &TurnTradeInput{
			Merchant: "坊市兵器铺",
			Buys:     []TurnTradeBuy{{DefID: "implement.middle.rare", Name: "中品青锋剑"}},
		},
	})
	if got := purseInSnapshot(t, store, storyID); got != 920 {
		t.Errorf("after buying the second grade under its own name: purse = %d, want 920", got)
	}
	if got := xiuxianCarriedQuantity(packEntryInSnapshot(t, store, storyID, "青锋剑")); got != 1 {
		t.Errorf("the 下品 stack holds %d copies, want 1", got)
	}
	middle := packEntryInSnapshot(t, store, storyID, "中品青锋剑")
	if got := xiuxianCarriedQuantity(middle); got != 1 {
		t.Errorf("the 中品 stack holds %d copies, want 1", got)
	}
	if def := xiuxianEntryText(middle, "定义"); def != "implement.middle.rare" {
		t.Errorf("the second stack records %q, want implement.middle.rare", def)
	}

	// Equipping the better one puts the 下品 back without touching either grade.
	commitPaidTurn(t, store, storyID, "换上新剑", "你把中品剑换上，原来的下品剑收回行囊。", &TurnResult{
		Items: &TurnItemsInput{Equip: []TurnItemEquip{{InstanceID: "中品青锋剑"}}},
	})
	worn := slotEntryInSnapshot(t, store, storyID, slot)
	if def := xiuxianEntryText(worn, "定义"); def != "implement.middle.rare" {
		t.Errorf("the worn copy records %q, want implement.middle.rare", def)
	}
	if got := xiuxianCarriedQuantity(packEntryInSnapshot(t, store, storyID, "青锋剑")); got != 1 {
		t.Errorf("after the swap the 下品 stack holds %d copies, want 1", got)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	t.Cleanup(func() { _ = reopened.Close() })
	if got := purseInSnapshot(t, reopened, storyID); got != 920 {
		t.Errorf("after reopening: purse = %d, want 920", got)
	}
	if def := xiuxianEntryText(slotEntryInSnapshot(t, reopened, storyID, slot), "定义"); def != "implement.middle.rare" {
		t.Errorf("after reopening the worn copy records %q, want implement.middle.rare", def)
	}
}

// TestUnequippingTwoGradesUnderOneNameKeepsBothIdentities covers the path where
// the two grades really did end up sharing a name: the second copy is given a
// key of its own rather than being folded into the first one's entry.
func TestUnequippingTwoGradesUnderOneNameKeepsBothIdentities(t *testing.T) {
	system := xiuxianTestSystem()
	slot, err := xiuxianItemSlotForKind(cultivation.KindImplement)
	if err != nil {
		t.Fatal(err)
	}
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldCurrency: float64(500),
		xiuxianFieldImportantItems: map[string]any{
			"青锋剑": map[string]any{"定义": "implement.lower.common", "数量": float64(1)},
		},
		// A worn 中品 sword under the same name, which is the state the old code
		// produced and which existing saves may still hold.
		slot: map[string]any{
			"名称": "青锋剑", "定义": "implement.middle.rare", "品级": "中品", "稀有度": "稀有",
			"主要属性预算": float64(5), "适用阶段": "qi_1",
		},
	})

	ops, result, ok := xiuxianItemOps(state, system, DefaultActorID, TurnItemsInput{
		Unequip: []TurnItemUnequip{{Slot: slot}},
	}, "ev1")
	if !ok {
		t.Fatal("the module declined a state whose template declares its fields")
	}
	if len(result.Refused) != 0 {
		t.Fatalf("refused = %+v", result.Refused)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, DefaultActorID, xiuxianFieldImportantItems)
	if len(pack) != 2 {
		t.Fatalf("pack has %d entries, want the two grades kept apart: %+v", len(pack), pack)
	}
	if entry, ok := pack["青锋剑"].(map[string]any); !ok || xiuxianEntryText(entry, "定义") != "implement.lower.common" {
		t.Errorf("the pack copy is %+v, want the 下品 sword unchanged", pack["青锋剑"])
	}
	kept := false
	for name, raw := range pack {
		entry, _ := raw.(map[string]any)
		if name == "青锋剑" {
			continue
		}
		if xiuxianEntryText(entry, "定义") == "implement.middle.rare" && xiuxianCarriedQuantity(entry) == 1 {
			kept = true
		}
	}
	if !kept {
		t.Errorf("the 中品 sword was lost on the way back into the pack: %+v", pack)
	}
}

// TestSwappingIntoAWornSlotKeepsTheOldGradesIdentity is the automatic-swap half.
// The copy leaving the slot must not overwrite a pack entry of another grade.
func TestSwappingIntoAWornSlotKeepsTheOldGradesIdentity(t *testing.T) {
	system := xiuxianTestSystem()
	slot, err := xiuxianItemSlotForKind(cultivation.KindImplement)
	if err != nil {
		t.Fatal(err)
	}
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldCurrency: float64(500),
		xiuxianFieldImportantItems: map[string]any{
			"青锋剑": map[string]any{"定义": "implement.lower.common", "数量": float64(1)},
			"新剑":  map[string]any{"定义": "implement.middle.rare", "数量": float64(1), "适用阶段": "qi_4"},
		},
		slot: map[string]any{
			"名称": "青锋剑", "定义": "implement.middle.rare", "品级": "中品", "稀有度": "稀有",
			"主要属性预算": float64(5), "适用阶段": "qi_1",
		},
	})

	ops, result, ok := xiuxianItemOps(state, system, DefaultActorID, TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "新剑"}},
	}, "ev1")
	if !ok {
		t.Fatal("the module declined a state whose template declares its fields")
	}
	if len(result.Refused) != 0 {
		t.Fatalf("refused = %+v", result.Refused)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, DefaultActorID, xiuxianFieldImportantItems)
	if entry, ok := pack["青锋剑"].(map[string]any); !ok || xiuxianEntryText(entry, "定义") != "implement.lower.common" {
		t.Errorf("the pack's 下品 sword is %+v, want it unchanged", pack["青锋剑"])
	}
	kept := false
	for name, raw := range pack {
		entry, _ := raw.(map[string]any)
		if name == "青锋剑" {
			continue
		}
		if xiuxianEntryText(entry, "定义") == "implement.middle.rare" {
			kept = true
		}
	}
	if !kept {
		t.Errorf("the 中品 sword that came out of the slot was lost: %+v", pack)
	}
	worn, _ := xiuxianEquipmentEntry(applied, DefaultActorID, slot)
	if xiuxianEntryText(worn, "定义") != "implement.middle.rare" {
		t.Errorf("the slot holds %+v, want the new sword", worn)
	}
}

// TestBuyingMoreOfTheSameDefinitionStillStacks keeps the legitimate case green:
// two copies of one definition are one stack, not two keys.
func TestBuyingMoreOfTheSameDefinitionStillStacks(t *testing.T) {
	store, storyID, _ := paidCopyStory(t, 1000)
	for i := 0; i < 2; i++ {
		commitPaidTurn(t, store, storyID, "买一柄青锋剑", "你又挑了一柄一样的青锋剑。", &TurnResult{
			Trade: &TurnTradeInput{
				Merchant: "坊市兵器铺",
				Buys:     []TurnTradeBuy{{DefID: "implement.lower.common", Name: "青锋剑"}},
			},
		})
	}
	if got := purseInSnapshot(t, store, storyID); got != 960 {
		t.Errorf("purse = %d, want 960 (two swords at 20)", got)
	}
	entry := packEntryInSnapshot(t, store, storyID, "青锋剑")
	if got := xiuxianCarriedQuantity(entry); got != 2 {
		t.Errorf("pack holds %d copies, want the 2 that were paid for", got)
	}
	if def := xiuxianEntryText(entry, "定义"); def != "implement.lower.common" {
		t.Errorf("the stack records %q, want implement.lower.common", def)
	}
	if names := packNamesInSnapshot(t, store, storyID); len(names) != 1 {
		t.Errorf("the pack has %d keys (%v), want one stack", len(names), names)
	}
}

// TestRetryingTheRefusedPurchaseChargesNothing is the duplicate-request half:
// the same refused line twice must leave the purse and the pack untouched.
func TestRetryingTheRefusedPurchaseChargesNothing(t *testing.T) {
	store, storyID, _ := paidCopyStory(t, 1000)
	commitPaidTurn(t, store, storyID, "买下品青锋剑", "你数出二十枚灵石，挑了一柄下品青锋剑。", &TurnResult{
		Trade: &TurnTradeInput{
			Merchant: "坊市兵器铺",
			Buys:     []TurnTradeBuy{{DefID: "implement.lower.common", Name: "青锋剑"}},
		},
	})
	slot, err := xiuxianItemSlotForKind(cultivation.KindImplement)
	if err != nil {
		t.Fatal(err)
	}
	commitPaidTurn(t, store, storyID, "佩上青锋剑", "你把青锋剑佩在腰间。", &TurnResult{
		Items: &TurnItemsInput{Equip: []TurnItemEquip{{InstanceID: "青锋剑"}}},
	})
	for attempt := 0; attempt < 2; attempt++ {
		_, _, _ = store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
			BranchID: "main", User: "再买同名中品剑", Narrative: "你又问起同名但品相更好的剑。",
			TurnResult: &TurnResult{
				Choices: testTurnChoices(),
				Trade: &TurnTradeInput{
					Merchant: "坊市兵器铺",
					Buys:     []TurnTradeBuy{{DefID: "implement.middle.rare", Name: "青锋剑"}},
				},
			},
		})
	}
	if got := purseInSnapshot(t, store, storyID); got != 980 {
		t.Errorf("two refused attempts changed the purse: %d, want 980", got)
	}
	if worn := slotEntryInSnapshot(t, store, storyID, slot); xiuxianEntryText(worn, "定义") != "implement.lower.common" {
		t.Errorf("the worn copy is %+v, want the 下品 sword unchanged", worn)
	}
}

// TestAnArmourNameCollisionIsRefusedTheSameWay keeps the guard from being an
// implement-only special case: armour is bought and worn through the same paths.
func TestAnArmourNameCollisionIsRefusedTheSameWay(t *testing.T) {
	store, storyID, _ := paidCopyStory(t, 1000)
	armourSlot, err := xiuxianItemSlotForKind(cultivation.KindArmor)
	if err != nil {
		t.Fatal(err)
	}
	commitPaidTurn(t, store, storyID, "买中品法袍", "你数出灵石，挑了一件中品云纹法袍。", &TurnResult{
		Trade: &TurnTradeInput{
			Merchant: "坊市法器铺",
			Buys:     []TurnTradeBuy{{DefID: "armor.middle.rare", Name: "云纹法袍"}},
		},
	})
	commitPaidTurn(t, store, storyID, "披上法袍", "你把法袍披上。", &TurnResult{
		Items: &TurnItemsInput{Equip: []TurnItemEquip{{InstanceID: "云纹法袍"}}},
	})
	if worn := slotEntryInSnapshot(t, store, storyID, armourSlot); xiuxianEntryText(worn, "定义") != "armor.middle.rare" {
		t.Fatalf("the worn armour is %+v, want armor.middle.rare", worn)
	}
	before := purseInSnapshot(t, store, storyID)
	// A sword of another grade under the same name would take the implement
	// slot, not the armour slot, so the worn collision is exercised with the
	// implement pair above; this one proves the armour path stores and returns
	// exactly one copy with its own definition.
	commitPaidTurn(t, store, storyID, "脱下法袍", "你把法袍脱下收好。", &TurnResult{
		Items: &TurnItemsInput{Unequip: []TurnItemUnequip{{Slot: armourSlot}}},
	})
	if got := purseInSnapshot(t, store, storyID); got != before {
		t.Errorf("unequipping armour changed the purse: %d -> %d", before, got)
	}
	entry := packEntryInSnapshot(t, store, storyID, "云纹法袍")
	if def := xiuxianEntryText(entry, "定义"); def != "armor.middle.rare" {
		t.Errorf("the armour records %q, want armor.middle.rare", def)
	}
	if got := xiuxianCarriedQuantity(entry); got != 1 {
		t.Errorf("the pack holds %d copies, want the 1 that was paid for", got)
	}
}
// The tests below cover the automatic swap into a slot already holding a copy
// under the same name. The 2026-10-26 independent review reproduced it: a 下品
// 青锋剑 in the pack and a 中品 one worn, then equipping the pack copy turned
// both into 下品 and the better blade was gone. The equip loop treated a shared
// name as proof that the copy leaving the slot and the one arriving were the
// same object, which is only true when they share a definition.
//
// The state these start from is a save written before the purchase check
// existed; the fixtures below are the same shape the review used, and none of
// them is produced by the current purchase path.

// countsByDefinition sums the copies of each definition across the pack and the
// worn slot, so a test can assert that neither grade was consumed.
func countsByDefinition(pack map[string]any, worn map[string]any) map[string]int {
	counts := map[string]int{}
	for _, raw := range pack {
		entry, _ := raw.(map[string]any)
		counts[xiuxianEntryText(entry, "定义")] += xiuxianCarriedQuantity(entry)
	}
	if len(worn) > 0 {
		counts[xiuxianEntryText(worn, "定义")]++
	}
	return counts
}

// collidingGradesState is a protagonist holding a 下品 青锋剑 in the pack and a
// 中品 one in the implement slot, both keyed by the same display name.
func collidingGradesState(t *testing.T, purse float64) (map[string]any, string) {
	t.Helper()
	slot, err := xiuxianItemSlotForKind(cultivation.KindImplement)
	if err != nil {
		t.Fatal(err)
	}
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldCurrency: purse,
		xiuxianFieldImportantItems: map[string]any{
			"青锋剑": map[string]any{"定义": "implement.lower.common", "数量": float64(1)},
		},
		slot: map[string]any{
			"名称": "青锋剑", "定义": "implement.middle.rare", "品级": "中品", "稀有度": "稀有",
			"主要属性预算": float64(5), "适用阶段": "qi_4",
		},
	})
	return state, slot
}

// TestEquippingTheCollidingNameDirectlyKeepsBothGrades is the reproduced case at
// the rules layer: the swap may not change the grade of either copy, and it may
// not move money.
func TestEquippingTheCollidingNameDirectlyKeepsBothGrades(t *testing.T) {
	system := xiuxianTestSystem()
	state, slot := collidingGradesState(t, 500)

	ops, result, ok := xiuxianItemOps(state, system, DefaultActorID, TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "青锋剑"}},
	}, "ev1")
	if !ok {
		t.Fatal("the module declined a state whose template declares its fields")
	}
	if len(result.Refused) != 0 {
		t.Fatalf("refused = %+v", result.Refused)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, DefaultActorID, xiuxianFieldImportantItems)
	worn, _ := xiuxianEquipmentEntry(applied, DefaultActorID, slot)
	counts := countsByDefinition(pack, worn)
	if counts["implement.lower.common"] != 1 || counts["implement.middle.rare"] != 1 {
		t.Fatalf("the swap changed item identities: counts=%+v pack=%+v worn=%+v", counts, pack, worn)
	}
	if got := xiuxianEntryText(worn, "定义"); got != "implement.lower.common" {
		t.Errorf("the slot records %q, want the copy that was equipped", got)
	}
	if got := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency); got != 500 {
		t.Errorf("equipping moved money: purse = %d, want 500", got)
	}
}

// TestEquippingTheSameDefinitionThroughThePackKeepsTheStack is the other half:
// when the two copies really are one thing, the pack keeps its count instead of
// growing a second entry for the copy that came out of the slot.
func TestEquippingTheSameDefinitionThroughThePackKeepsTheStack(t *testing.T) {
	system := xiuxianTestSystem()
	slot, err := xiuxianItemSlotForKind(cultivation.KindImplement)
	if err != nil {
		t.Fatal(err)
	}
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldCurrency: float64(500),
		xiuxianFieldImportantItems: map[string]any{
			"青锋剑": map[string]any{"定义": "implement.middle.rare", "数量": float64(1)},
		},
		slot: map[string]any{
			"名称": "青锋剑", "定义": "implement.middle.rare", "品级": "中品", "稀有度": "稀有",
			"主要属性预算": float64(5), "适用阶段": "qi_4",
		},
	})

	ops, result, ok := xiuxianItemOps(state, system, DefaultActorID, TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "青锋剑"}},
	}, "ev1")
	if !ok {
		t.Fatal("the module declined a state whose template declares its fields")
	}
	if len(result.Refused) != 0 {
		t.Fatalf("refused = %+v", result.Refused)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, DefaultActorID, xiuxianFieldImportantItems)
	worn, _ := xiuxianEquipmentEntry(applied, DefaultActorID, slot)
	if len(pack) != 1 {
		t.Fatalf("pack has %d entries, want the one stack: %+v", len(pack), pack)
	}
	if got := countsByDefinition(pack, worn); got["implement.middle.rare"] != 2 {
		t.Errorf("counts = %+v, want both 中品 swords still owned", got)
	}
	if entry, _ := pack["青锋剑"].(map[string]any); xiuxianCarriedQuantity(entry) != 1 {
		t.Errorf("the pack holds %+v, want one copy", pack["青锋剑"])
	}
}

// TestTheSameNameSwapSurvivesASaveAndColdReopen runs the reproduced case through
// the story's own save, so the fix is proven on the path the player actually
// takes and not only in the rules helper.
func TestTheSameNameSwapSurvivesASaveAndColdReopen(t *testing.T) {
	system := xiuxianTestSystem()
	_, slot := collidingGradesState(t, 0)
	for i := range system.InitialActors {
		if system.InitialActors[i].ID == DefaultActorID {
			system.InitialActors[i].State = map[string]any{
				xiuxianFieldRealm:    cultivation.StageKey(cultivation.Qi(6)),
				xiuxianFieldCurrency: float64(1000),
				xiuxianFieldImportantItems: map[string]any{
					"青锋剑": map[string]any{"定义": "implement.lower.common", "数量": float64(1)},
				},
				slot: map[string]any{
					"名称": "青锋剑", "定义": "implement.middle.rare", "品级": "中品", "稀有度": "稀有",
					"主要属性预算": float64(5), "适用阶段": "qi_4",
				},
			}
		}
	}
	root := t.TempDir()
	store := NewStore(root)
	story, err := store.CreateStory(CreateStoryRequest{Title: "同名换装", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "进了坊市", Narrative: "坊市里人声嘈杂，你掂了掂腰间那柄中品青锋剑。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	commitPaidTurn(t, store, story.ID, "换佩行囊里那柄剑", "你解下中品剑，把行囊里那柄同名的换到腰间。", &TurnResult{
		Items: &TurnItemsInput{Equip: []TurnItemEquip{{InstanceID: "青锋剑"}}},
	})
	if got := purseInSnapshot(t, store, story.ID); got != 1000 {
		t.Errorf("the swap charged the purse: %d, want 1000", got)
	}
	before := countsInSnapshot(t, store, story.ID, slot)
	if before["implement.lower.common"] != 1 || before["implement.middle.rare"] != 1 {
		t.Fatalf("before reopening: counts=%+v", before)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	t.Cleanup(func() { _ = reopened.Close() })

	after := countsInSnapshot(t, reopened, story.ID, slot)
	if after["implement.lower.common"] != 1 || after["implement.middle.rare"] != 1 {
		t.Fatalf("after reopening: counts=%+v", after)
	}
	if got := xiuxianEntryText(slotEntryInSnapshot(t, reopened, story.ID, slot), "定义"); got != "implement.lower.common" {
		t.Errorf("after reopening: the slot records %q, want the copy that was equipped", got)
	}
	if got := purseInSnapshot(t, reopened, story.ID); got != 1000 {
		t.Errorf("after reopening: purse = %d, want 1000", got)
	}
}

// countsInSnapshot sums the copies of each definition across the saved pack and
// the given worn slot.
func countsInSnapshot(t *testing.T, store *Store, storyID, slot string) map[string]int {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	pack, _ := xiuxianEquipmentEntry(snapshot.State, DefaultActorID, xiuxianFieldImportantItems)
	worn, _ := xiuxianEquipmentEntry(snapshot.State, DefaultActorID, slot)
	return countsByDefinition(pack, worn)
}
