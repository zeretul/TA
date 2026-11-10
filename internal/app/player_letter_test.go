package app

import (
	"context"
	"errors"
	"testing"

	"ta/config"
	"ta/internal/character"
	"ta/internal/interactive"
)

func TestLetterWorldCannotUseImmediateRemoteCharacterChatRoute(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "异地书信",
		PackBinding: &interactive.StoryPackBinding{
			PackID: "letter-world", Scope: interactive.PackScopeFromID("letter-world"),
			Gameplay: &interactive.StoryGameplayProfile{
				StatusTemplate: "world-actions-v1", Growth: "none",
				Combat: "none", Communication: "letter", LetterDeliverySeconds: 24 * 60 * 60,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AppendCharacterFacts(story.ID, interactive.AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "午后", Profiles: []character.Profile{{ID: "liu-si", Name: "刘四"}},
	}); err != nil {
		t.Fatal(err)
	}
	application := &App{workspace: workspace, interactive: store, cfg: &config.Config{}}
	_, err = application.SendCharacterMessage(context.Background(), CharacterChatRequest{
		StoryID: story.ID, CharacterID: "liu-si", Message: "你收到我的信了吗？",
	})
	if !errors.Is(err, ErrCharacterChatUnavailable) {
		t.Fatalf("immediate character-chat error = %v, want remote letter-world refusal", err)
	}
}
