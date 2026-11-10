package interactive

import (
	"testing"

	"ta/internal/cultivation"
)

// The same sword used to be able to exist twice, and in two places at once.
//
// Two separate holes, both found in live runs:
//
//   - Taking a held copy into hand removes it from 重要物品, so acquiring a copy
//     under the same name afterwards found nothing there and wrote a fresh one.
//     The slot held one sword and the pack held another, and both were priced.
//   - A copy the story had already given to another tracked character could be
//     acquired by the player without anything removing it from the first holder,
//     so one sword hung on two belts.
//
// These tests pin the reproductions before the fix, so the fix cannot be a
// prompt that asks the model to remember to unequip first.

// swordOnBothSidesState puts one 青锋剑 in the slot of a tracked character.
func swordOnBothSidesState(t *testing.T) map[string]any {
	t.Helper()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)
	state[actorStateRoot].(map[string]any)["断道客"] = map[string]any{
		"id": "断道客", "template_id": ActorStateOpponentTemplateID,
		"state": map[string]any{
			xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(6)),
			xiuxianFieldImplement: map[string]any{
				"名称": "青锋剑", "定义": "implement.middle.rare",
				"品级": "中品", "稀有度": "少见", "适用阶段": cultivation.StageKey(cultivation.Qi(6)),
			},
		},
	}
	return state
}

// TestAcquiringTheEquippedSwordAgainDoesNotDuplicateIt is the first hole: after
// taking a sword into hand, asking for the same sword again must not record a
// second copy in the pack.
func TestAcquiringTheEquippedSwordAgainDoesNotDuplicateIt(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)

	acquired, _, ok := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "implement.lower.common", Name: "青锋剑"}},
	}, "ev1")
	if !ok {
		t.Fatal("acquiring must be accepted")
	}
	equipped, _, _ := xiuxianItemOps(appliedXiuxianState(t, state, acquired), system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "青锋剑"}},
	}, "ev2")
	afterEquip := appliedXiuxianState(t, appliedXiuxianState(t, state, acquired), equipped)

	// The sword is in hand; the pack must not still hold one.
	if pack, held := xiuxianEquipmentEntry(afterEquip, "protagonist", xiuxianFieldImportantItems); held {
		if _, still := pack["青锋剑"]; still {
			t.Fatalf("equipping left a copy in the pack: %+v", pack)
		}
	}

	// Now the model asks for the same sword again — the reproduction.
	_, again, _ := xiuxianItemOps(afterEquip, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "implement.lower.common", Name: "青锋剑"}},
	}, "ev3")
	if len(again.Acquired) != 0 {
		t.Fatalf("the equipped sword was acquired a second time: %+v", again.Acquired)
	}
	if len(again.Refused) != 1 {
		t.Fatalf("refused = %+v, want one refusal naming the sword already in hand", again.Refused)
	}
}

// TestAcquiringACopyAnotherCharacterHoldsMovesIt is the second hole: one sword
// cannot hang on two belts. Acquiring a copy a tracked character is holding has
// to be a transfer, or refused — never a second copy.
func TestAcquiringACopyAnotherCharacterHoldsIsNotASecondCopy(t *testing.T) {
	system := xiuxianTestSystem()
	state := swordOnBothSidesState(t)

	ops, result, ok := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{
			DefID: "implement.middle.rare", Name: "青锋剑", Source: "断道客手中夺来",
		}},
	}, "ev1")
	if !ok {
		t.Fatal("the module must accept the request and decide")
	}
	if len(result.Acquired) != 0 {
		t.Fatalf("the same sword was recorded for a second holder: %+v", result.Acquired)
	}
	if len(result.Refused) != 1 {
		t.Fatalf("refused = %+v, want a refusal naming the other holder", result.Refused)
	}
	// And nothing moved: the other character still holds it.
	applied := appliedXiuxianState(t, state, ops)
	if entry, held := xiuxianEquipmentEntry(applied, "断道客", xiuxianFieldImplement); !held {
		t.Fatal("the first holder lost the sword without a transfer")
	} else if xiuxianItemName(entry) != "青锋剑" {
		t.Errorf("first holder's slot = %+v, want the untouched sword", entry)
	}
}

// TestTransferringASwordTakesItOffTheFirstHolder is the other half: naming the
// giver makes the acquisition a transfer, and one submission has to do both
// sides or the sword ends up on two belts.
func TestTransferringASwordTakesItOffTheFirstHolder(t *testing.T) {
	system := xiuxianTestSystem()
	state := swordOnBothSidesState(t)

	ops, result, ok := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{
			DefID: "implement.middle.rare", Name: "青锋剑", From: "断道客", Source: "断道客败后交出",
		}},
	}, "ev1")
	if !ok || len(result.Acquired) != 1 || len(result.Refused) != 0 {
		t.Fatalf("acquired = %+v refused = %+v", result.Acquired, result.Refused)
	}
	applied := appliedXiuxianState(t, state, ops)

	// The new holder has it.
	pack, held := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldImportantItems)
	if !held {
		t.Fatal("the recipient's pack was not written")
	}
	if _, ok := pack["青锋剑"].(map[string]any); !ok {
		t.Fatalf("recipient pack = %+v, want the sword", pack)
	}
	// The old holder does not. This is the whole point: one object, one belt.
	if entry, still := xiuxianEquipmentEntry(applied, "断道客", xiuxianFieldImplement); still {
		t.Fatalf("the first holder still wears the transferred sword: %+v", entry)
	}
	// And it is not secretly in their pack either.
	if otherPack, held := xiuxianEquipmentEntry(applied, "断道客", xiuxianFieldImportantItems); held {
		if _, still := otherPack["青锋剑"]; still {
			t.Fatalf("the transferred sword stayed in the first holder's pack: %+v", otherPack)
		}
	}
}

// TestTransferringFromSomebodyWhoDoesNotHoldItIsRefused keeps a transfer from
// minting the object: the giver has to really have it.
func TestTransferringFromSomebodyWhoDoesNotHoldItIsRefused(t *testing.T) {
	system := xiuxianTestSystem()
	state := swordOnBothSidesState(t)

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{
			DefID: "implement.middle.rare", Name: "断水刀", From: "断道客",
		}},
	}, "ev1")
	if len(ops) != 0 || len(result.Acquired) != 0 || len(result.Refused) != 1 {
		t.Fatalf("ops = %+v acquired = %+v refused = %+v", ops, result.Acquired, result.Refused)
	}
}

// TestASecondSwordOfTheSameKindIsStillAllowed keeps the fix narrow. The rules
// cannot tell a genuinely different sword from a duplicate by name alone, and
// refusing every second implement would break a character who really did find
// another one.
func TestASecondSwordOfTheSameKindIsStillAllowed(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{
			{DefID: "implement.lower.common", Name: "青锋剑"},
			{DefID: "implement.lower.common", Name: "另一柄青锋剑", Source: "另一名散修遗下"},
		},
	}, "ev1")
	if len(result.Acquired) != 2 || len(result.Refused) != 0 {
		t.Fatalf("acquired = %+v refused = %+v", result.Acquired, result.Refused)
	}
	pack, _ := xiuxianEquipmentEntry(appliedXiuxianState(t, state, ops), "protagonist", xiuxianFieldImportantItems)
	if len(pack) != 2 {
		t.Errorf("pack = %+v, want two separate copies", pack)
	}
}

// TestRenamingDoesNotDuplicateTheObject covers the rename route the task names.
//
// The rules cannot tell a renamed copy from a genuinely different sword of the
// same kind, so they do not try to: they look for the object by the name it is
// recorded under. A transfer that names a copy the giver does not have is
// refused, and the giver keeps what they actually hold. That is what makes a
// rename useless as a way to get a second sword — it never produces one, and it
// never takes the first one off the belt it is on.
func TestRenamingDoesNotDuplicateTheObject(t *testing.T) {
	system := xiuxianTestSystem()
	state := swordOnBothSidesState(t)

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{
			DefID: "implement.middle.rare", Name: "断水刀", From: "断道客",
		}},
	}, "ev1")
	if len(ops) != 0 || len(result.Acquired) != 0 || len(result.Refused) != 1 {
		t.Fatalf("ops = %+v acquired = %+v refused = %+v", ops, result.Acquired, result.Refused)
	}
	if entry, still := xiuxianEquipmentEntry(state, "断道客", xiuxianFieldImplement); !still {
		t.Fatalf("the giver's sword was taken for a copy they never had: %+v", entry)
	}
}

// TestATransferredCopyKeepsItsOwnWording keeps the journal honest: the sentence
// the player already read about the sword must survive the change of hands.
func TestATransferredCopyKeepsItsOwnWording(t *testing.T) {
	system := xiuxianTestSystem()
	state := swordOnBothSidesState(t)
	// The first holder's slot already records a description; the slot value is
	// rewritten here so the fixture states it explicitly. The wording avoids
	// full-width punctuation, which the state reader normalizes on the way out
	// and which would make this assertion about punctuation rather than about
	// the transfer.
	state[actorStateRoot].(map[string]any)["断道客"].(map[string]any)["state"].(map[string]any)[xiuxianFieldImplement] = map[string]any{
		"名称": "青锋剑", "定义": "implement.middle.rare", "品级": "中品", "稀有度": "少见",
		"适用阶段": cultivation.StageKey(cultivation.Qi(6)),
		"描述":  "刃口有一道旧豁",
	}

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "implement.middle.rare", Name: "青锋剑", From: "断道客"}},
	}, "ev1")
	if len(result.Acquired) != 1 {
		t.Fatalf("acquired = %+v", result.Acquired)
	}
	pack, _ := xiuxianEquipmentEntry(appliedXiuxianState(t, state, ops), "protagonist", xiuxianFieldImportantItems)
	entry, _ := pack["青锋剑"].(map[string]any)
	if got := xiuxianEntryText(entry, "描述"); got != "刃口有一道旧豁" {
		t.Errorf("transferred copy's description = %q, want the original wording", got)
	}
}
