package app

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ta/internal/interactive"
)

func TestAdventurePlayerProjectionWithholdsNestedEnemyFacts(t *testing.T) {
	actor := map[string]any{"family": "space", "health": 91, "skills": map[string]any{"known-skill": 1}, "inventory": map[string]any{"known-item": 2}, "battle": map[string]any{
		"id": "fight", "status": "active", "opponent_name": "Visible rival", "round": 2,
		"health": 877, "max_health": 999, "attack": 76, "defense": 43,
		"enemy_inventory": map[string]any{"secret-ammunition": 12}, "enemy_energy": 641,
		"future_secret_field":        "unpublished-target-fact",
		"electronic_distance_meters": 8, "enemy_item_ready_at": map[string]any{"secret-shield": 660},
	}}
	schema := &interactive.ActorStateSchemaSnapshot{System: interactive.StoryDirectorActorStateSystem{
		AdventureRules: &interactive.AdventureRules{Family: "space", Currency: "credits", Opponents: []interactive.AdventureOpponent{{ID: "hidden-opponent", Name: "Unmet boss", Attack: 765}}, Skills: []interactive.AdventureSkill{{ID: "known-skill", Name: "Known art"}, {ID: "secret-skill", Name: "Unlearned secret"}}, Items: []interactive.AdventureItem{{ID: "known-item", Name: "Known tool"}, {ID: "secret-item", Name: "Unseen relic"}}},
		Templates:      []interactive.ActorStateTemplate{{ID: "protagonist", Fields: []interactive.ActorStateField{{Name: interactive.AdventureStateField, Default: actor}}}},
		InitialActors:  []interactive.ActorStateInitialActor{{ID: "protagonist", State: map[string]any{interactive.AdventureStateField: actor}}},
	}}
	turn := interactive.TurnEvent{ID: "turn", StateDelta: &interactive.StateDelta{ActorOps: []interactive.ActorStateOp{{ActorID: "protagonist", FieldID: interactive.AdventureStateField, Value: actor, PreviousValue: actor}}}}
	snapshot := interactive.Snapshot{State: map[string]any{"actors": map[string]any{"protagonist": map[string]any{"template_id": "protagonist", "state": map[string]any{interactive.AdventureStateField: actor}}}}, ActorStateSchema: schema, Turns: []interactive.TurnEvent{turn}, CurrentTurn: &turn}
	snapshot.State["actors"].(map[string]any)["rival"] = map[string]any{"template_id": "older-npc-template", "state": map[string]any{interactive.AdventureStateField: actor, "生命": 777, "钱财": 777}}
	before, _ := json.Marshal(schema)
	actorBefore, _ := json.Marshal(actor)
	projectInteractiveSnapshotForPlayer(&snapshot)
	encoded, _ := json.Marshal(snapshot)
	for _, forbidden := range []string{"enemy_inventory", "enemy_energy", "electronic_distance_meters", "enemy_item_ready_at", "secret-shield", "secret-ammunition", "unpublished-target-fact", "hidden-opponent", "secret-skill", "secret-item"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("player response contains %s", forbidden)
		}
	}
	rival := playerProjectionActorState(t, snapshot, "rival")
	if _, visible := rival["生命"]; visible {
		t.Fatal("a transitional NPC template exposed its private health")
	}
	if _, visible := rival[interactive.AdventureStateField]; visible {
		t.Fatal("a transitional NPC template exposed its private ledger")
	}
	state := playerProjectionActorState(t, snapshot, "protagonist")[interactive.AdventureStateField].(map[string]any)
	if state["health"] != 91 {
		t.Fatalf("own health lost: %#v", state)
	}
	battle := state["battle"].(map[string]any)
	if battle["status"] != "active" || battle["opponent_name"] != "Visible rival" {
		t.Fatalf("known encounter identity lost: %#v", battle)
	}
	after, _ := json.Marshal(schema)
	actorAfter, _ := json.Marshal(actor)
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(actorBefore, actorAfter) {
		t.Fatal("player projection mutated canonical facts")
	}
	// History pagination and streaming call this function without a snapshot.
	stream, _ := json.Marshal(playerKnownTurn(turn, schema))
	if strings.Contains(string(stream), "enemy_inventory") {
		t.Fatal("stream/history carries hidden battle inventory")
	}
}

func TestPlayerRuleProjectionCannotReconstructHiddenBindingBonus(t *testing.T) {
	turn := interactive.TurnEvent{RuleResolution: &interactive.RuleResolution{Seed: 1234, Result: interactive.RuleResult{Rolls: []int{10}, KeptRoll: 10, StoryRollModifier: 2, Total: 19, BonusTotal: 9, Outcome: "success"}, StateBinding: &interactive.RuleStateBindingAudit{}}}
	projected := playerKnownTurn(turn, nil)
	if projected.RuleResolution.Result.Total != 0 || projected.RuleResolution.Result.BonusTotal != 0 || projected.RuleResolution.StateBinding != nil || projected.RuleResolution.Seed != 0 {
		t.Fatal("private adjudication arithmetic survived player projection")
	}
	if projected.RuleResolution.Result.Outcome != "success" || projected.RuleResolution.Result.KeptRoll != 10 {
		t.Fatal("committed public result was lost")
	}
	if turn.RuleResolution.Result.Total != 19 || turn.RuleResolution.StateBinding == nil {
		t.Fatal("canonical resolution was changed")
	}
}
