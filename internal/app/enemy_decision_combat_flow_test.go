package app

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

func TestCombatRoundUsesModelChoiceAndRetryDoesNotCallAgain(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	var calls atomic.Int32
	application.cfg = enemyDecisionTestConfig(t, application.workspace, func(request enemyDecisionTestRequest) string {
		calls.Add(1)
		if strings.Contains(request.Messages[1].Content, "unperformed-player-choice-canary") {
			t.Error("enemy read the player's current unperformed action")
		}
		return `{"candidate_id":"rest"}`
	})
	opened, err := application.PlayCombatRound(CombatRoundRequest{StoryID: storyID, CommandID: "enemy-ai-open", Combatants: combatTestSpecs(), SkipNarration: true})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("opening an encounter generated an unchosen round")
	}
	request := CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "enemy-ai-round", SkipNarration: true,
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionRest, OfferText: "unperformed-player-choice-canary"},
			// A client cannot bypass enemy AI by declaring its preferred enemy action.
			"rival": {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: interactive.DefaultActorID},
		},
	}
	first, err := application.PlayCombatRound(request)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || first.LastRound == nil {
		t.Fatalf("round did not consult model exactly once: calls = %d, round = %+v", calls.Load(), first.LastRound)
	}
	if first.LastRound.EnemyDecision == nil || first.LastRound.EnemyDecision.Status != "ai" {
		t.Fatalf("API omitted committed provenance: %+v", first.LastRound)
	}
	found := false
	for _, step := range first.LastRound.Outcome.Steps {
		if step.ActorID == "rival" {
			found = true
			if step.Kind != cultivation.ActionRest {
				t.Fatalf("model choice did not reach settlement: %+v", step)
			}
		}
	}
	if !found {
		t.Fatal("settled round omitted the enemy")
	}
	replayed, err := application.PlayCombatRound(request)
	if err != nil || replayed.LastRound == nil || replayed.LastRound.EventID != first.LastRound.EventID || calls.Load() != 1 {
		t.Fatalf("retry bought another decision or changed result: calls = %d, replay = %+v, err = %v", calls.Load(), replayed.LastRound, err)
	}
}

func TestCombatModelDecisionRejectsStateChangedDuringCall(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	opened, err := application.PlayCombatRound(CombatRoundRequest{StoryID: storyID, CommandID: "stale-ai-open", Combatants: combatTestSpecs(), SkipNarration: true})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	application.cfg = enemyDecisionTestConfig(t, application.workspace, func(request enemyDecisionTestRequest) string {
		calls.Add(1)
		// A competing real commit must complete while Generate is waiting for
		// this response. If the App held the Store lock, this would deadlock.
		_, err := application.interactive.ResolveBattleRound(storyID, interactive.BattleRoundRequest{
			BranchID: "main", SessionID: opened.SessionID, CommandID: "competing-round", CommandHash: "competing-round-hash",
			Actions: map[string]cultivation.CombatAction{interactive.DefaultActorID: {Kind: cultivation.ActionRest}, "rival": {Kind: cultivation.ActionRest}},
		})
		if err != nil {
			t.Errorf("competing round failed: %v", err)
		}
		return `{"candidate_id":"rest"}`
	})
	_, err = application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "stale-ai-round", SkipNarration: true,
		Actions: map[string]cultivation.CombatAction{interactive.DefaultActorID: {Kind: cultivation.ActionRest}},
	})
	if err == nil || calls.Load() != 1 {
		t.Fatalf("stale model choice was committed: calls = %d, err = %v", calls.Load(), err)
	}
	after, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID, BranchID: "main", SessionID: opened.SessionID})
	if err != nil || len(after.History) != 1 {
		t.Fatalf("stale decision spent a second round: history = %d, err = %v", len(after.History), err)
	}
}

func TestCombatCanceledDecisionDoesNotSpendRound(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	opened, err := application.PlayCombatRound(CombatRoundRequest{StoryID: storyID, CommandID: "cancel-ai-open", Combatants: combatTestSpecs(), SkipNarration: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "cancel-ai-round", SkipNarration: true, Context: ctx,
		Actions: map[string]cultivation.CombatAction{interactive.DefaultActorID: {Kind: cultivation.ActionRest}},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation settled as fallback: %v", err)
	}
	after, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID, BranchID: "main", SessionID: opened.SessionID})
	if err != nil || len(after.History) != 0 {
		t.Fatalf("canceled decision spent a round: history = %d, err = %v", len(after.History), err)
	}
}
