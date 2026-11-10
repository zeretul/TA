package interactive

import (
	"testing"

	"ta/internal/cultivation"
)

// These tests cover a pill swallowed outside a battle.
//
// §4.2 requires the stock, the dose and the effect to be really saved. The fight
// window settles its own dose, but a character who is out of the fight and hurt —
// the common case after one — had no way to spend a pill: 生命 only rises through
// a rest interval, the item module could acquire and equip but never consume, and
// a prose write to 生命 is refused. A turn whose narrative said 吞下回春丹 left
// the wound and the dose exactly where they were.

// itemUseTestState puts a character at a chosen pool with a chosen carried pill.
func itemUseTestState(t *testing.T, health float64, pills float64) map[string]any {
	t.Helper()
	pack := map[string]any{}
	if pills > 0 {
		pack["回春丹"] = map[string]any{
			"定义": "pill.restore.health.lower",
			"数量": pills,
		}
	}
	return xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldHealth:         health,
		xiuxianFieldImportantItems: pack,
	})
}

// TestASwallowedPillRaisesThePoolAndSpendsTheDose is the closed loop: 生命 really
// rises, the dose really leaves the pack, and both writes carry the item source
// kind so a reroll can discard them together.
func TestASwallowedPillRaisesThePoolAndSpendsTheDose(t *testing.T) {
	system := xiuxianTestSystem()
	// A 炼气六层 character has 200 health. Starting at 1 — the state the live run
	// left after fleeing the road-blocker — a 下品 回血 pill restores 30% of the
	// stage's bare maximum.
	state := itemUseTestState(t, 1, 1)

	ops, result, ok := xiuxianItemOps(state, system, DefaultActorID, TurnItemsInput{
		Use: []TurnItemUse{{InstanceID: "回春丹"}},
	}, "ev1")
	if !ok {
		t.Fatal("a tracked character holding a registered pill must accept the module")
	}
	if len(result.Used) != 1 || len(result.Refused) != 0 {
		t.Fatalf("used = %+v refused = %+v, want one dose and no refusal", result.Used, result.Refused)
	}
	if result.Used[0].RestoredField != xiuxianFieldHealth {
		t.Errorf("restored field = %q, want %q", result.Used[0].RestoredField, xiuxianFieldHealth)
	}
	if result.Used[0].RestoredAmount <= 0 {
		t.Errorf("restored = %d, want a real amount", result.Used[0].RestoredAmount)
	}

	applied := appliedXiuxianState(t, state, ops)
	want := 1 + result.Used[0].RestoredAmount
	if got := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldHealth); got != want {
		t.Errorf("health = %d, want %d", got, want)
	}
	pack, _ := xiuxianEquipmentEntry(applied, DefaultActorID, xiuxianFieldImportantItems)
	entry, _ := pack["回春丹"].(map[string]any)
	if got := xiuxianCarriedQuantity(entry); got != 0 {
		t.Errorf("remaining doses = %d, want 0", got)
	}

	// Every write this module made has to be separable from prose, or a reroll
	// would leave the wound healed and the pill gone.
	for _, op := range ops {
		if op.SourceKind != StateOpSourceItemChange {
			t.Errorf("op %s/%s carries source kind %q", op.ActorID, op.FieldID, op.SourceKind)
		}
	}
}

// TestAPillOnAFullPoolIsRefusedAndNotCharged is the honest reading of a real
// action with no effect: the dose stays in the pack. The battle side states the
// same rule as reason.pool_already_full.
func TestAPillOnAFullPoolIsRefusedAndNotCharged(t *testing.T) {
	system := xiuxianTestSystem()
	state := itemUseTestState(t, 200, 2)

	ops, result, ok := xiuxianItemOps(state, system, DefaultActorID, TurnItemsInput{
		Use: []TurnItemUse{{InstanceID: "回春丹"}},
	}, "ev1")
	if !ok {
		t.Fatal("the module must accept the submission and report the refusal")
	}
	if len(ops) != 0 {
		t.Fatalf("a refused dose wrote %+v", ops)
	}
	if len(result.Used) != 0 || len(result.Refused) != 1 {
		t.Fatalf("used = %+v refused = %+v", result.Used, result.Refused)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, DefaultActorID, xiuxianFieldImportantItems)
	entry, _ := pack["回春丹"].(map[string]any)
	if got := xiuxianCarriedQuantity(entry); got != 2 {
		t.Errorf("remaining doses = %d, want the untouched 2", got)
	}
}

// TestAPillTheCharacterDoesNotHoldIsRefused keeps taking a dose from minting one.
func TestAPillTheCharacterDoesNotHoldIsRefused(t *testing.T) {
	system := xiuxianTestSystem()
	state := itemUseTestState(t, 1, 0)

	ops, result, ok := xiuxianItemOps(state, system, DefaultActorID, TurnItemsInput{
		Use: []TurnItemUse{{InstanceID: "回春丹"}},
	}, "ev1")
	if !ok {
		t.Fatal("the module must accept the submission and report the refusal")
	}
	if len(ops) != 0 || len(result.Used) != 0 || len(result.Refused) != 1 {
		t.Fatalf("ops = %+v used = %+v refused = %+v", ops, result.Used, result.Refused)
	}
	if got := xiuxianStoredInt(state, DefaultActorID, xiuxianFieldHealth); got != 1 {
		t.Errorf("health = %d, want the untouched 1", got)
	}
}

// TestAnAttackPillIsNotSettledOutsideABattle keeps the fight window's own dose
// out of this path: an attack pill lasts three exchanges, which only a battle
// round can count. Recording it here would consume stock nothing paid for.
func TestAnAttackPillIsNotSettledOutsideABattle(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldHealth: 1,
		xiuxianFieldImportantItems: map[string]any{
			"增攻丹": map[string]any{"定义": "pill.boost.attack.middle", "数量": float64(1)},
		},
	})

	ops, result, ok := xiuxianItemOps(state, system, DefaultActorID, TurnItemsInput{
		Use: []TurnItemUse{{InstanceID: "增攻丹"}},
	}, "ev1")
	if !ok {
		t.Fatal("the module must accept the submission and report the refusal")
	}
	if len(ops) != 0 || len(result.Used) != 0 || len(result.Refused) != 1 {
		t.Fatalf("ops = %+v used = %+v refused = %+v", ops, result.Used, result.Refused)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, DefaultActorID, xiuxianFieldImportantItems)
	entry, _ := pack["增攻丹"].(map[string]any)
	if got := xiuxianCarriedQuantity(entry); got != 1 {
		t.Errorf("remaining doses = %d, want the untouched 1", got)
	}
}

// TestTwoRequestsForOneDoseSpendItOnce keeps a turn that names the same copy
// twice from spending two doses.
func TestTwoRequestsForOneDoseSpendItOnce(t *testing.T) {
	system := xiuxianTestSystem()
	state := itemUseTestState(t, 1, 3)

	_, result, _ := xiuxianItemOps(state, system, DefaultActorID, TurnItemsInput{
		Use: []TurnItemUse{{InstanceID: "回春丹"}, {InstanceID: "回春丹"}},
	}, "ev1")
	if len(result.Used) != 1 {
		t.Fatalf("used = %+v, want exactly one dose", result.Used)
	}
}

// TestASwallowedDoseIsDroppedOnReroll keeps the pool write separable: the same
// source kind that lets an equip be re-derived lets a dose be discarded, and the
// settlement that follows re-derives the pool.
func TestASwallowedDoseIsDroppedOnReroll(t *testing.T) {
	system := xiuxianTestSystem()
	state := itemUseTestState(t, 1, 1)
	ops, _, _ := xiuxianItemOps(state, system, DefaultActorID, TurnItemsInput{
		Use: []TurnItemUse{{InstanceID: "回春丹"}},
	}, "ev1")
	if kept := removeItemChangeActorOps(ops); len(kept) != 0 {
		t.Fatalf("reroll kept item writes: %+v", kept)
	}
}

// TestAPillTakenWithAFullManaPoolStillFillsHealth is the mixed case: one pool is
// full and the other is not, and only the dose with something to do is spent.
func TestAPillTakenWithAFullManaPoolStillFillsHealth(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldHealth: 1,
		xiuxianFieldMana:   160,
		xiuxianFieldImportantItems: map[string]any{
			"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(1)},
			"回灵丹": map[string]any{"定义": "pill.restore.mana.lower", "数量": float64(1)},
		},
	})

	ops, result, _ := xiuxianItemOps(state, system, DefaultActorID, TurnItemsInput{
		Use: []TurnItemUse{{InstanceID: "回春丹"}, {InstanceID: "回灵丹"}},
	}, "ev1")
	if len(result.Used) != 1 || len(result.Refused) != 1 {
		t.Fatalf("used = %+v refused = %+v, want the health dose only", result.Used, result.Refused)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, DefaultActorID, xiuxianFieldImportantItems)
	if entry, _ := pack["回春丹"].(map[string]any); xiuxianCarriedQuantity(entry) != 0 {
		t.Error("the swallowed health pill is still in the pack")
	}
	if entry, _ := pack["回灵丹"].(map[string]any); xiuxianCarriedQuantity(entry) != 1 {
		t.Error("the refused mana pill was charged")
	}
}
