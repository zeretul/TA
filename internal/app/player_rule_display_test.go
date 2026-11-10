package app

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"ta/internal/interactive"
)

// The same wire fixture is rendered by the frontend regression. Starting from
// the canonical journal alone misses the player privacy projection entirely.
func TestPlayerRuleDisplaySnapshotWireContract(t *testing.T) {
	turn := interactive.TurnEvent{ID: "saved-turn", BranchID: "main", Narrative: "Saved body", RuleResolution: &interactive.RuleResolution{
		ID: "saved-check", Seed: 987,
		Request:      interactive.TurnCheckRequest{Action: "secret-action", Challenge: "secret-trap", State: "secret-state", Rule: interactive.TurnCheckRule{TemplateID: interactive.RuleSystemMysteryClueForwardID, Label: "悬疑调查型 DM：线索不断线", BindingID: "secret-binding", TargetActorID: "secret-target"}},
		Result:       interactive.RuleResult{ID: "check_1", Label: "悬疑调查型 DM：线索不断线", Kind: "dice_check", Dice: "1d20", Rolls: []int{4}, KeptRoll: 4, Outcome: "failure", Total: 999, Target: 888, BonusTotal: 995},
		StateBinding: &interactive.RuleStateBindingAudit{},
	}}
	before, _ := json.Marshal(turn)
	snapshot := interactive.Snapshot{StoryID: "saved-story", BranchID: "main", Turns: []interactive.TurnEvent{turn}, CurrentTurn: &turn, State: map[string]any{}, TurnCount: 1}
	projectInteractiveSnapshotForPlayer(&snapshot)
	stream := playerKnownTurn(turn, nil)
	if !reflect.DeepEqual(snapshot.CurrentTurn.RuleResolution, stream.RuleResolution) || !reflect.DeepEqual(snapshot.Turns[0].RuleResolution, stream.RuleResolution) {
		t.Fatal("snapshot, history and streaming disagree on public check identity")
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"secret-", "999", "888", "995", "987", "state_binding"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("private adjudication survived: %s", private)
		}
	}
	projectInteractiveSnapshotForPlayer(&snapshot)
	twice, _ := json.Marshal(snapshot)
	if string(encoded) != string(twice) {
		t.Fatal("double projection changed public check metadata")
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	resolution := wire["current_turn"].(map[string]any)["rule_resolution"].(map[string]any)
	if resolution["display_template_id"] != interactive.RuleSystemMysteryClueForwardID {
		t.Fatalf("public display identity was lost: %s", encoded)
	}
	after, _ := json.Marshal(turn)
	if string(before) != string(after) {
		t.Fatal("projection rewrote the journal fact")
	}
	fixture, err := os.ReadFile("../../web/src/features/interactive/components/story-stage/__fixtures__/player-rule-snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]any
	if err := json.Unmarshal(fixture, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wire, expected) {
		t.Fatalf("shared frontend wire fixture differs; actual: %s", encoded)
	}
}

func TestPlayerRuleDisplayWithholdsCustomPrivateIdentity(t *testing.T) {
	for _, test := range []struct{ name, id, label string }{
		{"private custom ID", "secret-template-choice", "悬疑调查型 DM：线索不断线"},
		{"renamed builtin", interactive.RuleSystemMysteryClueForwardID, "Player-authored caption"},
		{"label alone", "", "悬疑调查型 DM：线索不断线"},
		{"no public caption", interactive.RuleSystemMysteryClueForwardID, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := &interactive.RuleResolution{Request: interactive.TurnCheckRequest{Rule: interactive.TurnCheckRule{TemplateID: test.id, Label: "悬疑调查型 DM：线索不断线"}}, Result: interactive.RuleResult{Label: test.label}}
			projected := playerPublicRuleResolution(original)
			encoded, _ := json.Marshal(projected)
			if strings.Contains(string(encoded), "display_template_id") || strings.Contains(string(encoded), "secret-template-choice") {
				t.Fatalf("private identity became display metadata: %s", encoded)
			}
			if projected.Result.Label != test.label {
				t.Fatal("custom caption was rewritten")
			}
			twice := playerPublicRuleResolution(projected)
			if !reflect.DeepEqual(projected, twice) {
				t.Fatal("double projection changed custom/private metadata")
			}
			if projected.Request.Rule.TemplateID != "" {
				t.Fatal("private request was restored")
			}
		})
	}
}
