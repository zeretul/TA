package app

import (
	"encoding/json"
	"strings"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
	"testing"
)

// Uses real journal commits, CreateBranch and a fresh Store, not JSON cloning.
func TestNPCPlayerProjectionRealBranchAndColdStore(t *testing.T) {
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
	defer func() { _ = store.Close() }()
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

	checkpoint, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Snapshot.CurrentTurn == nil {
		t.Fatal("missing fork point")
	}
	fork, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: checkpoint.Snapshot.CurrentTurn.ID, Title: "Trader fork"})
	if err != nil {
		t.Fatal(err)
	}
	branches := []struct{ id, public, secret string }{
		{"main", "PUBLIC_MAIN_TRADER", "NPC_AUTHORED_MAIN_SECRET"},
		{fork.ID, "PUBLIC_FORK_TRADER", "NPC_AUTHORED_FORK_SECRET"},
	}
	for _, branch := range branches {
		_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
			BranchID: branch.id, User: "Talk to the trader", Narrative: "The trader changes their public title.",
			TurnResult: &interactive.TurnResult{Choices: []string{"交谈", "观察", "离开"}, StateUpdates: []interactivestate.Update{
				{Op: "replace", Path: "/行商/公开称谓", Value: branch.public},
				{Op: "replace", Path: "/行商/私有成长记录", Value: map[string]any{"hidden": branch.secret}},
			}},
		})
		if err != nil {
			t.Fatalf("append %s: %v", branch.id, err)
		}
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = interactive.NewStore(root)
	for index, branch := range branches {
		t.Run(branch.id, func(t *testing.T) {
			context, err := store.StoryContext(story.ID, branch.id)
			if err != nil {
				t.Fatal(err)
			}
			before, err := json.Marshal(context.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			actor := adventurePlayerMap(adventurePlayerMap(context.Snapshot.State["actors"])["行商"])
			if actor["name"] != "行商" {
				t.Fatalf("shared actor identity changed: %+v", actor)
			}
			state := adventurePlayerMap(actor["state"])
			if state["公开称谓"] != branch.public || adventurePlayerMap(state["私有成长记录"])["hidden"] != branch.secret {
				t.Fatalf("cold branch state mismatch: %+v", state)
			}
			if strings.Contains(string(before), branches[1-index].secret) {
				t.Fatal("canonical private state crossed branch")
			}
			assertPlayer := func(label string, value any) {
				t.Helper()
				raw, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(raw), "NPC_AUTHORED_") {
					t.Errorf("%s leaked private journal value: %s", label, raw)
				}
			}
			if context.Snapshot.CurrentTurn == nil {
				t.Fatal("cold branch lost current turn")
			}
			projectedTurn := playerKnownTurn(*context.Snapshot.CurrentTurn, context.Snapshot.ActorStateSchema)
			assertPlayer("stream turn result", projectedTurn.TurnResult)
			assertPlayer("stream state delta", projectedTurn.StateDelta)
			assertPlayer("stream full turn", projectedTurn)
			for _, turn := range context.Snapshot.Turns {
				assertPlayer("history turn", playerKnownTurn(turn, context.Snapshot.ActorStateSchema))
			}
			projected := context.Snapshot
			projectInteractiveSnapshotForPlayer(&projected)
			assertPlayer("snapshot and history", projected)
			visibleActor := adventurePlayerMap(adventurePlayerMap(projected.State["actors"])["行商"])
			if visibleActor["name"] != "行商" || adventurePlayerMap(visibleActor["state"])["公开称谓"] != branch.public {
				t.Fatalf("public branch card lost or crossed: %+v", visibleActor)
			}
			if projectedTurn.TurnResult == nil {
				t.Fatal("lost public turn result")
			}
			publicUpdate := false
			for _, update := range projectedTurn.TurnResult.StateUpdates {
				if update.Path == "/行商/公开称谓" && update.Value == branch.public {
					publicUpdate = true
				}
			}
			if !publicUpdate {
				t.Fatal("stream lost branch-specific public update")
			}
			after, err := json.Marshal(context.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("projection mutated canonical in-memory branch truth")
			}
			reread, err := store.StoryContext(story.ID, branch.id)
			if err != nil {
				t.Fatal(err)
			}
			persisted, err := json.Marshal(reread.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if string(persisted) != string(before) {
				t.Fatal("projection mutated persisted branch truth")
			}
		})
	}
}
