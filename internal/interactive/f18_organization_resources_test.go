package interactive

import (
	"testing"

	interactivestate "ta/internal/interactive/state"
)

// These tests cover the F18 finding the stage review left open: 无权限不能调动
// 组织资源. The design (自制-家族与宗门经营 §4) forbids building a permission
// platform for it, so the question this file answers is narrower — whether an
// existing formal submission boundary already lets the narrator write a resource
// that a rule owns.
//
// A faction's own treasury does not exist in this game. §12.1 deliberately keeps
// three purses apart: the character's own 灵石, a shop's prepared 现银, and a
// faction's assets (which are prose, not a number). So the reviewable question is
// whether either of the two real purses can be moved by prose.

// TestTheNarratorCannotMoveAShopsCash is the shop purse's own gate.
//
// 商贾's field instruction already says 由交易结算一次性写入，不由叙述改写, and
// §12.4 makes the prepared cash position a one-time commitment that later turns
// must read rather than choose. If the narrator could write 现银, it could fund
// any purchase from a shop that never had the money — which is the same failure
// the character's own purse is protected against, one level out.
func TestTheNarratorCannotMoveAShopsCash(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivationStageForTest(), map[string]any{
		xiuxianFieldMerchants: map[string]any{
			"周氏药铺": map[string]any{"现银": float64(500), "回购率": float64(600)},
		},
	})
	updates := []interactivestate.Update{{
		Op:    interactivestate.Replace,
		Path:  interactivestate.FormatPath([]string{DefaultStoryContextActorID, xiuxianFieldMerchants}),
		Value: map[string]any{"周氏药铺": map[string]any{"现银": float64(999999), "回购率": float64(600)}},
	}}
	if _, err := CompileTurnStateUpdates(system, state, updates, TurnStateUpdateCompileOptions{}); err == nil {
		t.Fatal("the narrator moved a shop's cash, want a refusal")
	}
}

// TestTheNarratorCannotRaiseAFactionsCash is the same gate reached through the
// nested path, because a whole-object replace is only one of the two ways to
// write it. A gate that only covered the object form would be trivially bypassed.
func TestTheNarratorCannotRaiseAFactionsCash(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivationStageForTest(), map[string]any{
		xiuxianFieldMerchants: map[string]any{
			"周氏药铺": map[string]any{"现银": float64(500), "回购率": float64(600)},
		},
	})
	updates := []interactivestate.Update{{
		Op:    interactivestate.Delta,
		Path:  interactivestate.FormatPath([]string{DefaultStoryContextActorID, xiuxianFieldMerchants, "周氏药铺", "现银"}),
		Value: float64(100000),
	}}
	if _, err := CompileTurnStateUpdates(system, state, updates, TurnStateUpdateCompileOptions{}); err == nil {
		t.Fatal("the narrator raised a shop's cash by delta, want a refusal")
	}
}

// TestTheTradeSettlementStillWritesTheShopRecord is the other half: refusing the
// narrator must not refuse the rule. The trade module's own write carries the
// trade source kind and must still compile.
func TestTheTradeSettlementStillWritesTheShopRecord(t *testing.T) {
	state := xiuxianTestState(t, cultivationStageForTest(), map[string]any{
		xiuxianFieldMerchants: map[string]any{
			"周氏药铺": map[string]any{"现银": float64(500), "回购率": float64(600)},
		},
	})
	next := map[string]any{"周氏药铺": map[string]any{"现银": float64(420), "回购率": float64(600)}}
	ops := []ActorStateOp{{
		Op: "set", ActorID: DefaultStoryContextActorID,
		FieldID: normalizeActorStateFieldName(xiuxianFieldMerchants),
		Value:   next, SourceKind: StateOpSourceTrade, SourceID: "trade",
	}}
	working := cloneActorStateRoot(state)
	for _, op := range ops {
		applyActorStateOp(working, op)
	}
	record, _ := xiuxianMerchantRecord(working, "周氏药铺")
	if record.Cash != 420 {
		t.Fatalf("the settlement's own write did not land: %+v", record)
	}
}

func cultivationStageForTest() string {
	return "炼气一层"
}
