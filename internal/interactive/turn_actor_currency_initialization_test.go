package interactive

import (
	"errors"
	"testing"

	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

// These tests cover the opening half of the purse's life.
//
// 灵石 is rules-owned, so every later write to it is refused and the trade
// module is the only thing that may move it. The live run found what that
// rule costs when it is applied to the opening too: the prose counted six 灵石
// into a newly met character's hand, the create left the field out because its
// instruction said the rules own it, the template default of 0 was materialized,
// and from that moment the story and the panel disagreed with no path left to
// repair it — the character could never pay the money the story said they held.
//
// The create branch is deliberately exempt from the settled-field refusal
// (turn_state_updates.go's create branch), so the fix is not a new exemption:
// it is telling the model that the opening amount is a story fact it must state,
// and pinning that behaviour with a test so a later tightening cannot quietly
// take the exemption away.

func xiuxianCreateUpdate(actorID string, state map[string]any) interactivestate.Update {
	return interactivestate.Update{
		Op:   interactivestate.Create,
		Path: "/" + actorID,
		Value: map[string]any{
			"template_id": ActorStateImportantCharacterTemplateID,
			"name":        actorID,
			"state":       state,
		},
	}
}

// TestCreateInitializesThePurseTheProseStated is the acceptance scene: a new
// character who is said to carry six 灵石 is created holding six, not the
// template default of zero.
func TestCreateInitializesThePurseTheProseStated(t *testing.T) {
	system := xiuxianTestSystem()
	compiled, err := CompileTurnStateUpdates(system, map[string]any{}, []interactivestate.Update{
		xiuxianCreateUpdate("赵父", map[string]any{
			xiuxianFieldRealm:    cultivation.StageKey(cultivation.Qi(3)),
			xiuxianFieldCurrency: float64(6),
		}),
	}, TurnStateUpdateCompileOptions{})
	if err != nil {
		t.Fatalf("create stating a purse must compile: %v", err)
	}

	applied := map[string]any{}
	for _, op := range compiled.Ops {
		applyStateOp(applied, op)
	}
	for _, op := range compiled.ActorOps {
		applyActorStateOp(applied, op)
	}
	if got := xiuxianStoredInt(applied, "赵父", xiuxianFieldCurrency); got != 6 {
		t.Fatalf("new character's purse = %d, want the 6 the prose stated", got)
	}
}

// TestCreateLeavesThePurseEmptyWhenNothingWasSaid keeps the default honest: a
// character nobody said anything about really has no money, and the create does
// not invent an amount for them.
func TestCreateLeavesThePurseEmptyWhenNothingWasSaid(t *testing.T) {
	system := xiuxianTestSystem()
	compiled, err := CompileTurnStateUpdates(system, map[string]any{}, []interactivestate.Update{
		xiuxianCreateUpdate("无名路人", map[string]any{
			xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(1)),
		}),
	}, TurnStateUpdateCompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	applied := map[string]any{}
	for _, op := range compiled.Ops {
		applyStateOp(applied, op)
	}
	for _, op := range compiled.ActorOps {
		applyActorStateOp(applied, op)
	}
	if got := xiuxianStoredInt(applied, "无名路人", xiuxianFieldCurrency); got != 0 {
		t.Fatalf("unstated purse = %d, want the template's 0", got)
	}
}

// TestTheOpeningIsTheOnlyTurnThatMayWriteThePurse is the other half of the same
// rule. Allowing the create to state the amount must not open a general
// "align the balance" write: a later turn that finds the panel disagreeing with
// the prose still cannot move the number by describing it.
func TestTheOpeningIsTheOnlyTurnThatMayWriteThePurse(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradePaymentTestState(t, 20, 0)

	_, err := CompileTurnStateUpdates(system, state, []interactivestate.Update{{
		Op:   interactivestate.Replace,
		Path: "/断道客/" + xiuxianFieldCurrency,
		Value: float64(6),
	}}, TurnStateUpdateCompileOptions{})
	var validationError *StateUpdateValidationError
	if !errors.As(err, &validationError) || validationError.Code != "cultivation_owned_state_update" {
		t.Fatalf("a later turn writing the purse should be refused as rules-owned, got %v", err)
	}
}
