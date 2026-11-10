package storypack

import (
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
	"testing"
)

func TestAuthoredNPCFactsCreateColdReadAndBranch(t *testing.T) {
	root := t.TempDir()
	base, err := interactive.NewActorStateLibrary(root).Get(interactive.ActorStateDetectiveID)
	if err != nil {
		t.Fatal(err)
	}
	g := RecommendedWorldGameplay()
	g.AuthoredStateFields = []interactive.ActorStateField{{Name: "成长记录", Type: "object"}, {Name: "钱财", Type: "number"}, {Name: "生命", Type: "number"}}
	module := buildWorldActorStateModule(base, "npc-facts", g)
	for _, id := range []string{interactive.ActorStateImportantCharacterTemplateID, interactive.ActorStateOpponentTemplateID} {
		fields := map[string]interactive.ActorStateField{}
		for _, template := range module.ActorState.Templates {
			if template.ID == id {
				for _, f := range template.Fields {
					fields[f.Name] = f
				}
			}
		}
		for _, name := range []string{"成长记录", "钱财", "生命"} {
			f, ok := fields[name]
			if !ok {
				t.Fatalf("NPC %s missing authored fact %s", id, name)
			}
			if f.Visibility != interactive.ActorStateVisibilityPrivate || f.Default != nil {
				t.Fatalf("NPC fact exposed or granted defaults: %+v", f)
			}
		}
	}
	store := interactive.NewStore(root)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "NPC shared facts", ChoiceCount: 3, ActorState: &module.ActorState, StateSchemaPolicy: &interactive.StoryStateSchemaPolicy{Mode: interactive.StoryStateSchemaModeFixedTemplate}})
	if err != nil {
		t.Fatal(err)
	}
	updates := []interactivestate.Update{{Op: interactivestate.Create, Path: "/行商", Value: map[string]any{"template_id": interactive.ActorStateImportantCharacterTemplateID, "name": "行商", "description": "An adult trader met at the market.", "state": map[string]any{"钱财": float64(17), "生命": float64(80), "成长记录": map[string]any{"stage": "confirmed"}}}}}
	c, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := interactive.CompileTurnStateUpdates(module.ActorState, c.Snapshot.State, updates, interactive.TurnStateUpdateCompileOptions{}); err != nil {
		t.Fatalf("NPC create cannot commit authored facts: %v", err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "Meet trader", Narrative: "An adult trader arrives.", TurnResult: &interactive.TurnResult{Choices: []string{"继续", "观察", "交谈"}, StateUpdates: updates}})
	if err != nil {
		t.Fatal(err)
	}
	c, err = store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: c.Snapshot.CurrentTurn.ID, Title: "Sibling"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "Record expense", Narrative: "The trader pays two coins.", TurnResult: &interactive.TurnResult{Choices: []string{"继续", "观察", "交谈"}, StateUpdates: []interactivestate.Update{{Op: interactivestate.Replace, Path: "/行商/钱财", Value: float64(15)}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = interactive.NewStore(root)
	defer store.Close()
	for branchID, want := range map[string]float64{"main": 15, branch.ID: 17} {
		c, err = store.StoryContext(story.ID, branchID)
		if err != nil {
			t.Fatal(err)
		}
		actors := c.Snapshot.State["actors"].(map[string]any)
		actor := actors["行商"].(map[string]any)
		state := actor["state"].(map[string]any)
		if interactive.ContentHashOf(state["钱财"]) != interactive.ContentHashOf(want) {
			t.Fatalf("branch %s lost wallet: %+v", branchID, state)
		}
		if state["成长记录"] == nil {
			t.Fatal("cold read lost growth facts")
		}
		registry, err := store.CharacterState(story.ID, branchID)
		if err != nil {
			t.Fatal(err)
		}
		profile, ok := registry.Profile("行商")
		if !ok || profile.ID != "行商" {
			t.Fatal("Actor and dialogue identity diverged")
		}
	}
}
