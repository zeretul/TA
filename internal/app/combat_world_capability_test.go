package app

import (
	"errors"
	"testing"

	"ta/internal/interactive"
)

func TestWorldWithoutIndependentCombatRejectsWindowAndRoundAPI(t *testing.T) {
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "雾夜",
		PackBinding: &interactive.StoryPackBinding{
			PackID: "no-combat", Scope: "pack:no-combat",
			Gameplay: &interactive.StoryGameplayProfile{
				StatusTemplate: "world-actions-v1", Growth: "none",
				Combat: "none", Communication: "local", Opening: "commission",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	application := &App{workspace: root, interactive: store}
	if _, err := application.CombatWindow(CombatWindowRequest{StoryID: story.ID}); !errors.Is(err, ErrCombatUnavailableForWorld) {
		t.Fatalf("combat window error = %v, want world capability refusal", err)
	}
	if _, err := application.PlayCombatRound(CombatRoundRequest{StoryID: story.ID}); !errors.Is(err, ErrCombatUnavailableForWorld) {
		t.Fatalf("combat round error = %v, want world capability refusal", err)
	}
}
