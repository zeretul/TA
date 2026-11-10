package interactive

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ta/internal/cultivation"
)

func TestEnemyDecisionReceiptNormalizesActualSource(t *testing.T) {
	for _, tc := range []struct {
		status   string
		accepted map[string]string
		enemies  int
		want     string
	}{
		{"ai", nil, 1, "fallback_invalid"},
		{"ai", map[string]string{"one": "secret-skill"}, 2, "ai_partial_fallback"},
		{"fallback_unavailable", nil, 1, "fallback_unavailable"},
		{"fallback_context_limit", nil, 1, "fallback_context_limit"},
		{"fallback_invalid", nil, 1, "fallback_invalid"},
		{"", nil, 1, "rules"},
		{"provider-secret", nil, 1, "unknown"},
	} {
		got := enemyDecisionReceipt(tc.status, tc.accepted, tc.enemies)
		if got.Status != tc.want {
			t.Fatalf("got %s, want %s", got.Status, tc.want)
		}
		raw, _ := json.Marshal(got)
		if strings.Contains(string(raw), "secret") {
			t.Fatalf("private content persisted: %s", raw)
		}
	}
	if enemyDecisionReceipt("ai", nil, 0) != nil {
		t.Fatal("no decision marked as AI")
	}
	var old CombatRecordEvent
	if err := json.Unmarshal([]byte(`{"id":"old"}`), &old); err != nil || old.EnemyDecision != nil {
		t.Fatal("historical provenance invented", err)
	}
}

func TestEnemyDecisionFallbackReceiptColdReplayAndBranch(t *testing.T) {
	for _, status := range []string{"fallback_unavailable", "fallback_invalid", "fallback_context_limit"} {
		t.Run(status, func(t *testing.T) {
			store, id := combatTestStory(t)
			defer func() { store.Close() }()
			opened, err := store.StartBattle(id, BattleStartRequest{BranchID: "main", Combatants: combatTestSpecs()})
			if err != nil {
				t.Fatal(err)
			}
			if opened.EnemyDecision != nil {
				t.Fatal("opening has decision provenance")
			}
			fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: combatTestLatestTurnID(t, store, id), Title: "Before decision"})
			if err != nil {
				t.Fatal(err)
			}
			req := BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, CommandID: "receipt-round", CommandHash: "round", Actions: map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionRest}}}
			prepared, err := store.PrepareBattleEnemyDecision(id, req)
			if err != nil {
				t.Fatal(err)
			}
			req.ExpectedHead, req.ExpectedRound = prepared.Snapshot.Head, prepared.Snapshot.Round
			req.DecisionStatus = status
			got, err := store.ResolveBattleRound(id, req)
			if err != nil {
				t.Fatal(err)
			}
			if got.EnemyDecision == nil || got.EnemyDecision.Status != status || len(got.EnemyDecision.AIActors) != 0 {
				t.Fatalf("wrong fallback provenance: %+v", got.EnemyDecision)
			}
			root := store.Root()
			store.Close()
			store = NewStore(root)
			req.DecisionStatus = "ai"
			req.EnemyChoices = map[string]string{"rival": "rest"}
			replay, err := store.ResolveBattleRound(id, req)
			if err != nil || !reflect.DeepEqual(replay.EnemyDecision, got.EnemyDecision) {
				t.Fatal("retry rewrote fallback provenance", err)
			}
			history, err := store.CombatHistory(id, "main", opened.Session.ID)
			if err != nil || len(history) != 1 || !reflect.DeepEqual(history[0].EnemyDecision, got.EnemyDecision) {
				t.Fatal("cold history lost provenance", err)
			}
			history, err = store.CombatHistory(id, fork.ID, opened.Session.ID)
			if err != nil || len(history) != 0 {
				t.Fatal("future provenance crossed fork", err)
			}
			req.CommandID = "stale-receipt"
			if _, err := store.ResolveBattleRound(id, req); err == nil {
				t.Fatal("stale decision committed")
			}
			history, err = store.CombatHistory(id, "main", opened.Session.ID)
			if err != nil || len(history) != 1 {
				t.Fatal("failed decision left a receipt", err)
			}
		})
	}
}

func TestEnemyDecisionAdventureFallbackReceiptColdReplay(t *testing.T) {
	for _, family := range []string{"wuxia", "space"} {
		t.Run(family, func(t *testing.T) {
			r, _ := enemyTacticsFixture(t, family)
			r.Opponents[0].Location = "船坞"
			store, id := adventureTestStoryWithRules(t, r)
			defer func() { store.Close() }()
			_, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "迎战", Narrative: "对手上前。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "迎战", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "rival"}}}})
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.AdventureWindow(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			in := AdventureAction{Action: "battle_round", Choice: "attack"}
			got, err := store.CommitAdventureActionWithEnemyDecision(id, "main", "fallback-receipt", before.Head, in, nil, "fallback_unavailable")
			if err != nil {
				t.Fatal(err)
			}
			if got.EnemyDecision == nil || got.EnemyDecision.Status != "fallback_unavailable" {
				t.Fatalf("wrong provenance: %+v", got.EnemyDecision)
			}
			root := store.Root()
			store.Close()
			store = NewStore(root)
			replay, err := store.PrepareAdventureEnemyDecision(id, "main", "fallback-receipt", before.Head, in)
			if err != nil || !replay.Replayed || !reflect.DeepEqual(replay.Adventure.EnemyDecision, got.EnemyDecision) {
				t.Fatal("cold adventure lost provenance", err)
			}
			fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: before.Head, Title: "Before fallback"})
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := store.PrepareAdventureEnemyDecision(id, fork.ID, "fallback-receipt", before.Head, in)
			if err != nil || prepared.Replayed {
				t.Fatal("future receipt crossed adventure fork", err)
			}
		})
	}
}
