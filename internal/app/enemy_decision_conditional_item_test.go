package app

import (
	"sync/atomic"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"testing"
)

func TestEnemyDecisionConditionalFullPoolUseCannotRerollModel(t *testing.T) {
	application, storyID := combatTestAppCarrying(t, map[string]any{"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(2)}})
	var calls atomic.Int32
	application.cfg = enemyDecisionTestConfig(t, application.workspace, func(request enemyDecisionTestRequest) string {
		if calls.Add(1) > 1 {
			return `{"candidate_id":"technique:strike:protagonist:"}`
		}
		return `{"candidate_id":"rest"}`
	})
	opened, err := application.PlayCombatRound(CombatRoundRequest{StoryID: storyID, CommandID: "full-pool-open", Combatants: combatTestSpecs(), SkipNarration: true})
	if err != nil {
		t.Fatal(err)
	}
	hero := combatWindowSide(t, opened, interactive.DefaultActorID)
	if *hero.Health != *hero.HealthMax {
		t.Fatal("fixture is not full health")
	}
	request := CombatRoundRequest{StoryID: storyID, BranchID: "main", SessionID: opened.SessionID, CommandID: "full-pool-dose", SkipNarration: true, Actions: map[string]cultivation.CombatAction{interactive.DefaultActorID: {Kind: cultivation.ActionUseItem, ItemInstanceID: "回春丹"}}}
	first, err := application.PlayCombatRound(request)
	if err != nil || first.LastRound == nil {
		t.Fatalf("conditional use undid the chosen enemy turn: %v", err)
	}
	step, ok := combatTestStep(first.LastRound.Outcome, interactive.DefaultActorID)
	if !ok || step.ReasonKey != "reason.pool_already_full" || step.ItemApplied || step.ActionRejected {
		t.Fatalf("wrong conditional outcome: %+v", step)
	}
	if calls.Load() != 1 || combatTestWindowItem(t, first, interactive.DefaultActorID, "回春丹").Quantity != 2 {
		t.Fatal("ineffective dose spent stock or asked twice")
	}
	second, err := application.PlayCombatRound(request)
	if err != nil || second.LastRound == nil || second.LastRound.EventID != first.LastRound.EventID || calls.Load() != 1 {
		t.Fatalf("same command rerolled AI: calls=%d err=%v", calls.Load(), err)
	}
	if len(second.History) != 1 {
		t.Fatalf("retry added another turn: %d", len(second.History))
	}
}
