package app

import (
	"encoding/json"
	"errors"
	"testing"

	"ta/internal/interactive"
)

func TestWorldCombatEndpointsRejectTheWrongFamilyBeforeModelOrMutation(t *testing.T) {
	for _, declared := range []string{"none", "wuxia", "space", "fantasy", "unknown"} {
		t.Run(declared, func(t *testing.T) {
			root := t.TempDir()
			store := interactive.NewStore(root)
			t.Cleanup(func() { _ = store.Close() })
			system := combatTestSystem(t)
			story, err := store.CreateStory(interactive.CreateStoryRequest{
				Title: "Mismatched combat", ActorState: &system,
				PackBinding: &interactive.StoryPackBinding{PackID: "test", Scope: "pack:test", Gameplay: &interactive.StoryGameplayProfile{
					StatusTemplate: "xiuxian-state", Growth: "cultivation", Combat: declared, Opening: "scene",
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			application := &App{workspace: root, interactive: store}
			before, err := store.StoryContext(story.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := application.CombatWindow(CombatWindowRequest{StoryID: story.ID}); !errors.Is(err, ErrCombatUnavailableForWorld) {
				t.Fatalf("wrong family read: %v", err)
			}
			if _, err := application.PlayCombatRound(CombatRoundRequest{StoryID: story.ID}); !errors.Is(err, ErrCombatUnavailableForWorld) {
				t.Fatalf("wrong family write: %v", err)
			}
			after, err := store.StoryContext(story.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			old, _ := json.Marshal(before.Snapshot)
			current, _ := json.Marshal(after.Snapshot)
			if string(old) != string(current) {
				t.Fatal("refused combat changed the story")
			}
		})
	}
}

func TestWorldCombatKeepsLegacyCultivationSchemaAndRejectsUnrelatedSchemas(t *testing.T) {
	app, storyID := combatTestApp(t)
	if _, err := app.CombatWindow(CombatWindowRequest{StoryID: storyID}); err != nil {
		t.Fatalf("legacy cultivation became unavailable: %v", err)
	}
	store := interactive.NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "No combat schema"})
	if err != nil {
		t.Fatal(err)
	}
	app = &App{interactive: store}
	if _, err := app.CombatWindow(CombatWindowRequest{StoryID: story.ID}); !errors.Is(err, ErrCombatUnavailableForWorld) {
		t.Fatalf("unrelated story got cultivation window: %v", err)
	}
}
