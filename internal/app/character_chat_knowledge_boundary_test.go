package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ta/config"
	"ta/internal/character"
	"ta/internal/interactive"
)

func TestPrivateChatRejectsAbsentLocalCharacter(t *testing.T) {
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Local contact boundary", PackBinding: &interactive.StoryPackBinding{
		PackID: "contact-test", Scope: interactive.PackScopeFromID("contact-test"),
		Gameplay: &interactive.StoryGameplayProfile{StatusTemplate: "world-actions-v1", Growth: "none", Combat: "none", Communication: "local"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AppendCharacterFacts(story.ID, interactive.AppendCharacterFactsRequest{BranchID: "main", WorldTime: "午后", Profiles: []character.Profile{{ID: "remote", Name: "Remote person"}}})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{workspace: root, interactive: store, cfg: &config.Config{}}
	_, err = app.SendCharacterMessage(context.Background(), CharacterChatRequest{StoryID: story.ID, CharacterID: "remote", Message: "Hello"})
	if !errors.Is(err, ErrCharacterNotPresent) {
		t.Fatalf("absent local character must be rejected before model setup, got %v", err)
	}
}

func TestPrivateChatRemoteSceneDoesNotInventPresence(t *testing.T) {
	c := interactive.StoryContext{Snapshot: interactive.Snapshot{BranchID: "main", State: map[string]any{
		"actors": map[string]any{interactive.DefaultStoryContextActorID: map[string]any{"state": map[string]any{
			"当前详细地点": "UNSEEN_LOCATION", "当前事件": "UNSEEN_EVENT", "在场角色": []any{"LOCAL_WITNESS"},
		}}},
	}}}
	scene := characterSceneFor(c, "remote")
	if len(scene.Present) != 0 || scene.Location != "" || scene.Title != "" {
		t.Fatalf("remote recipient received local scene: %+v", scene)
	}
	if scene.NodeID != "main" || scene.WorldTime == "" {
		t.Fatalf("remote projection lost journal identity/time: %+v", scene)
	}
	local := characterSceneFor(c, "LOCAL_WITNESS")
	if local.Location != "UNSEEN_LOCATION" || !character.ContainsID(local.Present, "LOCAL_WITNESS") {
		t.Fatalf("present recipient lost actual scene: %+v", local)
	}
}

func TestPrivateChatContextDoesNotCopyUnwitnessedNarrative(t *testing.T) {
	app, storyID := characterChatMattersApp(t, answerFor(`{"reply":"你好。"}`))
	c, err := app.interactive.StoryContext(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	c.Snapshot.CurrentTurn.Narrative = "SECRET_NOT_WITNESSED"
	scene := characterSceneFor(c, "陈婆婆")
	context := characterStoryContext(c, scene, character.Profile{ID: "陈婆婆", Attitude: "KNOWN_ATTITUDE"})
	if strings.Contains(context, "SECRET_NOT_WITNESSED") {
		t.Fatal("raw main narrative crossed the character knowledge boundary")
	}
	if !strings.Contains(context, "KNOWN_ATTITUDE") {
		t.Fatal("known character attitude was lost")
	}
}
