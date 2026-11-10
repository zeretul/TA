package app

import (
	"encoding/json"
	"strings"
	"testing"

	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
)

// This is a deterministic journal/projection regression, not a model test.
func TestNPCAuthoredCreateRecordDoesNotLeakThroughPlayerTurn(t *testing.T) {
	root := t.TempDir()
	module, err := interactive.NewActorStateLibrary(root).Get(interactive.ActorStateDetectiveID)
	if err != nil {
		t.Fatal(err)
	}
	for index := range module.ActorState.Templates {
		template := &module.ActorState.Templates[index]
		if template.ID != interactive.ActorStateImportantCharacterTemplateID {
			continue
		}
		template.Fields = append(template.Fields,
			interactive.ActorStateField{Name: "私有成长记录", Type: "object", Visibility: interactive.ActorStateVisibilityPrivate},
			interactive.ActorStateField{Name: "公开称谓", Type: "string"},
		)
	}
	store := interactive.NewStore(root)
	defer store.Close()
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "NPC create projection", ChoiceCount: 3, ActorState: &module.ActorState,
		StateSchemaPolicy: &interactive.StoryStateSchemaPolicy{Mode: interactive.StoryStateSchemaModeFixedTemplate},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "Meet a trader", Narrative: "An adult trader arrives.",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"交谈", "观察", "离开"},
			StateUpdates: []interactivestate.Update{{Op: interactivestate.Create, Path: "/行商", Value: map[string]any{
				"template_id": interactive.ActorStateImportantCharacterTemplateID,
				"name":        "行商", "description": "A trader at the market.",
				"state": map[string]any{
					"私有成长记录": map[string]any{"hidden": "NPC_AUTHORED_CREATE_SECRET"},
					"公开称谓":   "PUBLIC_TRADER_TITLE",
				},
			}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	context, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(context.Snapshot)
	if err != nil || !strings.Contains(string(canonical), "NPC_AUTHORED_CREATE_SECRET") {
		t.Fatalf("fixture did not commit private NPC truth: %v", err)
	}
	if context.Snapshot.CurrentTurn == nil {
		t.Fatal("fixture has no committed turn")
	}
	projectedTurn := playerKnownTurn(*context.Snapshot.CurrentTurn, context.Snapshot.ActorStateSchema)
	for name, value := range map[string]any{
		"turn_result": projectedTurn.TurnResult,
		"state_delta": projectedTurn.StateDelta,
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "NPC_AUTHORED_CREATE_SECRET") {
				t.Fatalf("private NPC create value reached player %s: %s", name, raw)
			}
		})
	}
	projected := context.Snapshot
	projectInteractiveSnapshotForPlayer(&projected)
	raw, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "NPC_AUTHORED_CREATE_SECRET") {
		t.Error("private NPC create value reached the player snapshot/history")
	}
	if !strings.Contains(string(raw), "PUBLIC_TRADER_TITLE") {
		t.Error("projection discarded the NPC's public fields")
	}
	after, err := json.Marshal(context.Snapshot)
	if err != nil || string(after) != string(canonical) {
		t.Fatalf("projection mutated canonical GM truth: %v", err)
	}
}
