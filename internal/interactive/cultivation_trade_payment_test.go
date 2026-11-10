package interactive

import (
	"testing"

	"ta/internal/cultivation"
)

// These tests cover coin handed to a person rather than spent in a shop.
//
// The live acceptance run found the gap: the prose counted ten 灵石 into the
// road-blocker's palm, the model's state_changes write to 灵石 was refused
// because the purse is rules-owned, and no module could record the payment at
// all — so the panel still said twenty while the story said ten. §5 requires an
// actual payment to be settled once, and §12.1 keeps the two purses apart, so
// both sides move together or neither does.

// tradePaymentTestState puts a second tracked character beside the protagonist so
// a payment has somewhere to land.
func tradePaymentTestState(t *testing.T, purse float64, otherPurse float64) map[string]any {
	t.Helper()
	state := tradeTestState(t, purse, nil, nil)
	state[actorStateRoot].(map[string]any)["断道客"] = map[string]any{
		"id": "断道客", "template_id": ActorStateOpponentTemplateID,
		"state": map[string]any{
			xiuxianFieldRealm:    cultivation.StageKey(cultivation.Qi(6)),
			xiuxianFieldCurrency: otherPurse,
		},
	}
	return state
}

// TestAPaymentMovesBothPursesOnce is the acceptance run's own scene: ten 灵石
// counted into a person's hand, settled by the rules rather than by the prose.
func TestAPaymentMovesBothPursesOnce(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradePaymentTestState(t, 20, 0)

	ops, result, ok := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{{To: "断道客", Amount: 10, Reason: "过路钱"}},
	}, "ev1")
	if !ok {
		t.Fatal("a payment to a tracked character must be accepted by the module")
	}
	if len(result.Paid) != 1 || len(result.Refused) != 0 {
		t.Fatalf("paid = %+v refused = %+v", result.Paid, result.Refused)
	}
	if result.Balance != 10 {
		t.Errorf("balance = %d, want 10 after paying 10 of 20", result.Balance)
	}

	applied := appliedXiuxianState(t, state, ops)
	if got := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency); got != 10 {
		t.Errorf("payer purse = %d, want 10", got)
	}
	if got := xiuxianStoredInt(applied, "断道客", xiuxianFieldCurrency); got != 10 {
		t.Errorf("recipient purse = %d, want 10", got)
	}
}

// TestAPaymentTheCharacterCannotAffordWritesNothing is §10's all-or-nothing
// rule: a purse too small for the coin leaves both sides where they were.
func TestAPaymentTheCharacterCannotAffordWritesNothing(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradePaymentTestState(t, 4, 7)

	ops, result, ok := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{{To: "断道客", Amount: 10}},
	}, "ev1")
	if !ok {
		t.Fatal("the module must still accept the submission and report the refusal")
	}
	if len(ops) != 0 {
		t.Fatalf("a refused payment wrote %+v", ops)
	}
	if len(result.Refused) != 1 {
		t.Fatalf("refused = %+v, want one line", result.Refused)
	}
	if result.Balance != 4 {
		t.Errorf("balance = %d, want the untouched 4", result.Balance)
	}

	applied := appliedXiuxianState(t, state, ops)
	if got := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency); got != 4 {
		t.Errorf("payer purse = %d, want the untouched 4", got)
	}
	if got := xiuxianStoredInt(applied, "断道客", xiuxianFieldCurrency); got != 7 {
		t.Errorf("recipient purse = %d, want the untouched 7", got)
	}
}

// TestTwoPaymentsCannotSpendTheSameCoin checks the running balance: the second
// payment is checked against what the first left, not against the opening purse.
func TestTwoPaymentsCannotSpendTheSameCoin(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradePaymentTestState(t, 10, 0)

	ops, result, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{
			{To: "断道客", Amount: 8},
			{To: "断道客", Amount: 5},
		},
	}, "ev1")
	if len(result.Paid) != 1 || len(result.Refused) != 1 {
		t.Fatalf("paid = %+v refused = %+v, want one of each", result.Paid, result.Refused)
	}
	if result.Balance != 2 {
		t.Errorf("balance = %d, want 2", result.Balance)
	}
	applied := appliedXiuxianState(t, state, ops)
	if got := xiuxianStoredInt(applied, "断道客", xiuxianFieldCurrency); got != 8 {
		t.Errorf("recipient purse = %d, want 8", got)
	}
}

// TestAPaymentToSomebodyTheStoryDoesNotTrackIsRefused keeps the payment from
// inventing an Actor: a name the state does not hold has no purse to write.
func TestAPaymentToSomebodyTheStoryDoesNotTrackIsRefused(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradePaymentTestState(t, 20, 0)

	ops, result, ok := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{{To: "路过的货郎", Amount: 3}},
	}, "ev1")
	if !ok {
		t.Fatal("the module must accept the submission and report the refusal")
	}
	if len(ops) != 0 || len(result.Refused) != 1 {
		t.Fatalf("ops = %+v refused = %+v", ops, result.Refused)
	}
	if _, exists := state[actorStateRoot].(map[string]any)["路过的货郎"]; exists {
		t.Fatal("a payment must not create an Actor")
	}
}

// TestAPaymentDoesNotRewriteThePack keeps a coin-only turn from claiming the
// pack changed.
func TestAPaymentDoesNotRewriteThePack(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradePaymentTestState(t, 20, 0)

	ops, _, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{{To: "断道客", Amount: 5}},
	}, "ev1")
	for _, op := range ops {
		if op.FieldID == normalizeActorStateFieldName(xiuxianFieldImportantItems) {
			t.Fatalf("a coin-only payment rewrote the pack: %+v", op)
		}
	}
}

// TestAShopDealStillNeedsAMerchant keeps the relaxed check honest: a purchase
// with no merchant is still a submission error, because a shop's cash position
// is what prices the sale.
func TestAShopDealStillNeedsAMerchant(t *testing.T) {
	if diagnostic := validateTurnTradeModule(&TurnTradeInput{
		Buys: []TurnTradeBuy{{DefID: "pill.restore.health.lower", Name: "回春丹"}},
	}); diagnostic == nil {
		t.Fatal("a purchase with no merchant was accepted")
	}
	if diagnostic := validateTurnTradeModule(&TurnTradeInput{
		Payments: []TurnTradePayment{{To: "断道客", Amount: 5}},
	}); diagnostic != nil {
		t.Fatalf("a payment with no merchant was rejected: %+v", diagnostic)
	}
}

// TestAPaymentMustNameSomebodyAndSomeCoin rejects a payment the rules could not
// act on before it reaches the settlement.
func TestAPaymentMustNameSomebodyAndSomeCoin(t *testing.T) {
	if diagnostic := validateTurnTradeModule(&TurnTradeInput{
		Payments: []TurnTradePayment{{Amount: 5}},
	}); diagnostic == nil {
		t.Error("a payment with no recipient was accepted")
	}
	if diagnostic := validateTurnTradeModule(&TurnTradeInput{
		Payments: []TurnTradePayment{{To: "断道客"}},
	}); diagnostic == nil {
		t.Error("a payment of nothing was accepted")
	}
}

// TestAPaymentIsDroppedOnReroll keeps a settled payment separable from prose:
// the same source kind that lets a trade be re-derived lets it be discarded.
func TestAPaymentIsDroppedOnReroll(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradePaymentTestState(t, 20, 0)
	ops, _, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{{To: "断道客", Amount: 10}},
	}, "ev1")
	if kept := removeTradeActorOps(ops); len(kept) != 0 {
		t.Fatalf("reroll kept trade writes: %+v", kept)
	}
}

// TestTwoPaymentsToTheSamePersonKeepTheCoin is the audit's own arithmetic case:
// payer 20, recipient 7, pay 3 then 5, both accepted. The recipient must end at
// 15 — the two set operations are applied in order, so a second write that read
// the opening purse would silently overwrite the first and lose three coin.
func TestTwoPaymentsToTheSamePersonKeepTheCoin(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradePaymentTestState(t, 20, 7)

	ops, result, ok := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{
			{To: "断道客", Amount: 3, Reason: "先付三枚"},
			{To: "断道客", Amount: 5, Reason: "再付五枚"},
		},
	}, "ev1")
	if !ok {
		t.Fatal("two affordable payments must be accepted by the module")
	}
	if len(result.Paid) != 2 || len(result.Refused) != 0 {
		t.Fatalf("paid = %+v refused = %+v, want both accepted", result.Paid, result.Refused)
	}
	if result.Balance != 12 {
		t.Errorf("payer balance = %d, want 12 after paying 8 of 20", result.Balance)
	}

	applied := appliedXiuxianState(t, state, ops)
	if got := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency); got != 12 {
		t.Errorf("payer purse = %d, want 12", got)
	}
	if got := xiuxianStoredInt(applied, "断道客", xiuxianFieldCurrency); got != 15 {
		t.Errorf("recipient purse = %d, want 15 (7 opening + 3 + 5)", got)
	}
	// Conservation is the point of the test: coin moves, it is not minted or
	// destroyed. The two purses together must hold what they held before.
	if total := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency) +
		xiuxianStoredInt(applied, "断道客", xiuxianFieldCurrency); total != 27 {
		t.Errorf("total coin = %d, want the opening 27", total)
	}
}

// TestPaymentsToDifferentPeopleEachLand keeps the running balance per person
// rather than one shared total: paying two people must credit each of them.
func TestPaymentsToDifferentPeopleEachLand(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradePaymentTestState(t, 20, 7)
	state[actorStateRoot].(map[string]any)["赵父"] = map[string]any{
		"id": "赵父", "template_id": ActorStateOpponentTemplateID,
		"state": map[string]any{
			xiuxianFieldRealm:    cultivation.StageKey(cultivation.Qi(1)),
			xiuxianFieldCurrency: float64(4),
		},
	}

	ops, result, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{
			{To: "断道客", Amount: 6},
			{To: "赵父", Amount: 2},
		},
	}, "ev1")
	if len(result.Paid) != 2 {
		t.Fatalf("paid = %+v, want both", result.Paid)
	}
	applied := appliedXiuxianState(t, state, ops)
	if got := xiuxianStoredInt(applied, "断道客", xiuxianFieldCurrency); got != 13 {
		t.Errorf("断道客 purse = %d, want 13", got)
	}
	if got := xiuxianStoredInt(applied, "赵父", xiuxianFieldCurrency); got != 6 {
		t.Errorf("赵父 purse = %d, want 6", got)
	}
	if got := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency); got != 12 {
		t.Errorf("payer purse = %d, want 12", got)
	}
}

// TestASecondPaymentThatFailsLeavesTheFirstOneSettled is the partial case: the
// first payment really happened, the second is refused, and the recipient keeps
// exactly the coin that was handed over.
func TestASecondPaymentThatFailsLeavesTheFirstOneSettled(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradePaymentTestState(t, 10, 7)

	ops, result, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{
			{To: "断道客", Amount: 6},
			{To: "断道客", Amount: 9},
		},
	}, "ev1")
	if len(result.Paid) != 1 || len(result.Refused) != 1 {
		t.Fatalf("paid = %+v refused = %+v, want one of each", result.Paid, result.Refused)
	}
	if result.Balance != 4 {
		t.Errorf("payer balance = %d, want 4", result.Balance)
	}
	applied := appliedXiuxianState(t, state, ops)
	if got := xiuxianStoredInt(applied, "断道客", xiuxianFieldCurrency); got != 13 {
		t.Errorf("recipient purse = %d, want 13 (7 + the 6 that really landed)", got)
	}
	if total := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency) +
		xiuxianStoredInt(applied, "断道客", xiuxianFieldCurrency); total != 17 {
		t.Errorf("total coin = %d, want the opening 17", total)
	}
}

// TestReplayingTheSamePaymentTurnSpendsTheCoinOnce is the retry rule at the
// module level: the same payment module applied to the state its own result
// produced must be refused for want of coin, not spend a second time. The turn
// identity guard is what stops a whole retried turn; this checks the settlement
// underneath cannot mint coin even if it were reached twice.
func TestReplayingTheSamePaymentTurnSpendsTheCoinOnce(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradePaymentTestState(t, 10, 7)
	input := TurnTradeInput{Payments: []TurnTradePayment{{To: "断道客", Amount: 10}}}

	ops, _, _ := xiuxianTradeOps(state, system, DefaultActorID, input, "ev1")
	once := appliedXiuxianState(t, state, ops)
	if got := xiuxianStoredInt(once, "断道客", xiuxianFieldCurrency); got != 17 {
		t.Fatalf("after the first settlement recipient = %d, want 17", got)
	}

	replayedOps, replayed, _ := xiuxianTradeOps(once, system, DefaultActorID, input, "ev1")
	if len(replayed.Paid) != 0 || len(replayed.Refused) != 1 {
		t.Fatalf("replay paid = %+v refused = %+v, want a refusal", replayed.Paid, replayed.Refused)
	}
	if len(replayedOps) != 0 {
		t.Fatalf("a refused replay wrote %+v", replayedOps)
	}
	twice := appliedXiuxianState(t, once, replayedOps)
	if got := xiuxianStoredInt(twice, "断道客", xiuxianFieldCurrency); got != 17 {
		t.Errorf("recipient purse after replay = %d, want the untouched 17", got)
	}
}
