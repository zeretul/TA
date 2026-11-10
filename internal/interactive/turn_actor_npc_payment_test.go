package interactive

import (
	"strings"
	"testing"

	"ta/internal/cultivation"
)

// These tests cover the other direction of a payment: coin an NPC hands to the
// player.
//
// The module used to have no payer at all. `actor_id` named the purse that paid
// and `to` named the purse that received, so a scene in which a character the
// player had just met counted four 灵石 into the player's hand had nowhere to be
// recorded: the balance read was always the acting character's, and the NPC's
// six coin could never be spent. The prose said the money moved and the rules
// moved nothing — the same class of failure as the opening purse, one direction
// over.

// npcPayerTestState puts a tracked character holding `npcPurse` beside a player
// holding `playerPurse`.
func npcPayerTestState(t *testing.T, playerPurse float64, npcID string, npcPurse float64) map[string]any {
	t.Helper()
	state := tradePaymentTestState(t, playerPurse, 0)
	state[actorStateRoot].(map[string]any)[npcID] = map[string]any{
		"id": npcID, "template_id": ActorStateOpponentTemplateID,
		"state": map[string]any{
			xiuxianFieldRealm:    cultivation.StageKey(cultivation.Qi(2)),
			xiuxianFieldCurrency: npcPurse,
		},
	}
	return state
}

// TestAnNPCCanPayThePlayer is the acceptance scene in the task's own numbers: the
// NPC holds 6, pays 4, keeps 2; the player holds 20, receives 4, ends at 24.
func TestAnNPCCanPayThePlayer(t *testing.T) {
	system := xiuxianTestSystem()
	state := npcPayerTestState(t, 20, "赵父", 6)

	ops, result, ok := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{{From: "赵父", To: DefaultActorID, Amount: 4, Reason: "还你垫的丹钱"}},
	}, "ev1")
	if !ok {
		t.Fatal("an NPC's payment must be settled by the module")
	}
	if len(result.Paid) != 1 || len(result.Refused) != 0 {
		t.Fatalf("paid = %+v refused = %+v", result.Paid, result.Refused)
	}
	if got := result.Paid[0]; got.From != "赵父" || got.To != DefaultActorID || got.Amount != 4 {
		t.Errorf("payment line = %+v, want 赵父 -> protagonist 4", got)
	}

	applied := appliedXiuxianState(t, state, ops)
	if got := xiuxianStoredInt(applied, "赵父", xiuxianFieldCurrency); got != 2 {
		t.Errorf("payer purse = %d, want 6 - 4 = 2", got)
	}
	if got := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency); got != 24 {
		t.Errorf("player purse = %d, want 20 + 4 = 24", got)
	}
	// Coin moved; it was not minted.
	if total := xiuxianStoredInt(applied, "赵父", xiuxianFieldCurrency) +
		xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency); total != 26 {
		t.Errorf("total coin = %d, want the opening 26", total)
	}
}

// TestAPayerWithoutTheCoinPaysNothing keeps the refusal honest and names the
// purse that was short.
//
// The generic insufficient-funds sentence names the acting character, which is
// the wrong person here. A live run's failure was that the refusal never reached
// the model at all and the prose kept saying 已付; naming the real payer is what
// makes the rewritten outcome possible.
func TestAPayerWithoutTheCoinPaysNothing(t *testing.T) {
	system := xiuxianTestSystem()
	state := npcPayerTestState(t, 20, "赵父", 3)

	ops, result, ok := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{{From: "赵父", To: DefaultActorID, Amount: 4}},
	}, "ev1")
	if !ok {
		t.Fatal("the module must accept the submission and report the refusal")
	}
	if len(ops) != 0 {
		t.Fatalf("a refused payment wrote %+v", ops)
	}
	if len(result.Refused) != 1 {
		t.Fatalf("refused = %+v, want one line", result.Refused)
	}
	if !strings.Contains(result.Refused[0].Reason, "赵父") {
		t.Errorf("the refusal does not name the payer: %q", result.Refused[0].Reason)
	}
	applied := appliedXiuxianState(t, state, ops)
	if got := xiuxianStoredInt(applied, "赵父", xiuxianFieldCurrency); got != 3 {
		t.Errorf("payer purse = %d, want the untouched 3", got)
	}
	if got := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency); got != 20 {
		t.Errorf("player purse = %d, want the untouched 20", got)
	}
}

// TestAPayerTheStoryDoesNotTrackIsRefused keeps a payment from inventing a purse:
// the payer has to be a character this branch really has.
func TestAPayerTheStoryDoesNotTrackIsRefused(t *testing.T) {
	system := xiuxianTestSystem()
	state := npcPayerTestState(t, 20, "赵父", 6)

	ops, result, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{{From: "路过的货郎", To: DefaultActorID, Amount: 4}},
	}, "ev1")
	if len(ops) != 0 || len(result.Refused) != 1 {
		t.Fatalf("ops = %+v refused = %+v", ops, result.Refused)
	}
	if _, exists := state[actorStateRoot].(map[string]any)["路过的货郎"]; exists {
		t.Fatal("a payment must not create an Actor")
	}
}

// TestPayingYourselfIsRefusedWhetherOrNotTheNamesDiffer covers the from/to pair:
// an NPC cannot pay itself, and the check is on the resolved ids rather than on
// whichever field happened to be filled.
func TestPayingYourselfIsRefused(t *testing.T) {
	system := xiuxianTestSystem()
	state := npcPayerTestState(t, 20, "赵父", 6)

	_, result, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{{From: "赵父", To: "赵父", Amount: 4}},
	}, "ev1")
	if len(result.Refused) != 1 {
		t.Fatalf("refused = %+v, want the self-payment refused", result.Refused)
	}
}

// TestBothDirectionsInOneTurnKeepEveryPurseRight is the mixed case: the player
// pays 3 to one character and another pays 4 back, in the same turn. Each write
// is absolute, so a module that read the opening purse per line would let the
// second write overwrite the first.
func TestBothDirectionsInOneTurnKeepEveryPurseRight(t *testing.T) {
	system := xiuxianTestSystem()
	state := npcPayerTestState(t, 20, "赵父", 6)
	state[actorStateRoot].(map[string]any)["断道客"] = map[string]any{
		"id": "断道客", "template_id": ActorStateOpponentTemplateID,
		"state": map[string]any{
			xiuxianFieldRealm:    cultivation.StageKey(cultivation.Qi(6)),
			xiuxianFieldCurrency: float64(7),
		},
	}

	ops, result, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{
			{To: "断道客", Amount: 3, Reason: "过路钱"},
			{From: "赵父", To: DefaultActorID, Amount: 4, Reason: "还你丹钱"},
		},
	}, "ev1")
	if len(result.Paid) != 2 || len(result.Refused) != 0 {
		t.Fatalf("paid = %+v refused = %+v, want both", result.Paid, result.Refused)
	}
	applied := appliedXiuxianState(t, state, ops)
	if got := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency); got != 21 {
		t.Errorf("player purse = %d, want 20 - 3 + 4 = 21", got)
	}
	if got := xiuxianStoredInt(applied, "赵父", xiuxianFieldCurrency); got != 2 {
		t.Errorf("赵父 purse = %d, want 2", got)
	}
	if got := xiuxianStoredInt(applied, "断道客", xiuxianFieldCurrency); got != 10 {
		t.Errorf("断道客 purse = %d, want 10", got)
	}
}

// TestAnNPCWhoPaysTwiceCannotSpendTheSameCoin carries the running balance across
// two payments from the same NPC purse.
func TestAnNPCWhoPaysTwiceCannotSpendTheSameCoin(t *testing.T) {
	system := xiuxianTestSystem()
	state := npcPayerTestState(t, 20, "赵父", 6)

	_, result, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Payments: []TurnTradePayment{
			{From: "赵父", To: DefaultActorID, Amount: 4},
			{From: "赵父", To: DefaultActorID, Amount: 4},
		},
	}, "ev1")
	if len(result.Paid) != 1 || len(result.Refused) != 1 {
		t.Fatalf("paid = %+v refused = %+v, want the second refused for want of coin", result.Paid, result.Refused)
	}
}

// TestReplayingAnNPCPaymentTurnMovesTheCoinOnce is the retry rule on the new
// direction: applying the same module to the state its own result produced must
// not move the coin a second time.
func TestReplayingAnNPCPaymentTurnMovesTheCoinOnce(t *testing.T) {
	system := xiuxianTestSystem()
	state := npcPayerTestState(t, 20, "赵父", 6)
	input := TurnTradeInput{Payments: []TurnTradePayment{{From: "赵父", To: DefaultActorID, Amount: 4}}}

	ops, _, _ := xiuxianTradeOps(state, system, DefaultActorID, input, "ev1")
	once := appliedXiuxianState(t, state, ops)
	if got := xiuxianStoredInt(once, "赵父", xiuxianFieldCurrency); got != 2 {
		t.Fatalf("after the first settlement payer = %d, want 2", got)
	}

	replayedOps, replayed, _ := xiuxianTradeOps(once, system, DefaultActorID, input, "ev1")
	if len(replayed.Paid) != 0 || len(replayed.Refused) != 1 {
		t.Fatalf("replay paid = %+v refused = %+v, want a refusal", replayed.Paid, replayed.Refused)
	}
	if len(replayedOps) != 0 {
		t.Fatalf("a refused replay wrote %+v", replayedOps)
	}
	twice := appliedXiuxianState(t, once, replayedOps)
	if got := xiuxianStoredInt(twice, DefaultActorID, xiuxianFieldCurrency); got != 24 {
		t.Errorf("player purse after replay = %d, want the untouched 24", got)
	}
}

// TestARefusedPaymentReachesTheModelAsARewrite is the half the module cannot do
// on its own.
//
// The rules refusing is correct. What was wrong is that the refusal was dropped:
// submission accepted the module, the prose was locked, and the story kept saying
// 已付、已结清. The receipt now carries the refusal and asks for the narrative to
// be written again with the real outcome.
func TestARefusedPaymentReachesTheModelAsARewrite(t *testing.T) {
	system := xiuxianTestSystem()
	state := npcPayerTestState(t, 20, "赵父", 3)

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, TurnSubmissionInput{Trade: &TurnTradeInput{
		Payments: []TurnTradePayment{{From: "赵父", To: DefaultActorID, Amount: 4}},
	}})
	if prepared == nil {
		t.Fatal("submission returned no draft")
	}
	if len(receipt.TradeRefusals) != 1 {
		t.Fatalf("trade_refusals = %+v, want the refusal carried on the receipt", receipt.TradeRefusals)
	}
	if !receipt.NarrativeRewriteRequired {
		t.Fatal("a refused payment must require the narrative to be rewritten")
	}
	if !strings.Contains(receipt.NarrativeRewriteReason, "赵父") {
		t.Errorf("the rewrite reason does not name the payer: %q", receipt.NarrativeRewriteReason)
	}
	if !strings.Contains(receipt.NarrativeRewriteReason, "未付") {
		t.Errorf("the rewrite reason does not say what to write instead: %q", receipt.NarrativeRewriteReason)
	}
	// The module itself stays accepted: an unpaid debt is still a real outcome,
	// and refusing the module would make the model drop the fact entirely.
	if receipt.ModuleStatus.Trade != TurnSubmissionModuleAccepted {
		t.Errorf("trade status = %q, want accepted", receipt.ModuleStatus.Trade)
	}
}

// TestASettledPaymentDoesNotAskForARewrite is the negative control: a payment
// that really settled must not trip the rewrite path, or every ordinary payment
// would discard the prose that described it.
func TestASettledPaymentDoesNotAskForARewrite(t *testing.T) {
	system := xiuxianTestSystem()
	state := npcPayerTestState(t, 20, "赵父", 6)

	_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, TurnSubmissionInput{Trade: &TurnTradeInput{
		Payments: []TurnTradePayment{{From: "赵父", To: DefaultActorID, Amount: 4}},
	}})
	if len(receipt.TradeRefusals) != 0 {
		t.Fatalf("trade_refusals = %+v, want none", receipt.TradeRefusals)
	}
	if receipt.NarrativeRewriteRequired {
		t.Fatalf("a settled payment asked for a rewrite: %q", receipt.NarrativeRewriteReason)
	}
}
