package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"ta/internal/character"
	"ta/internal/interactive"
)

func TestLetterWorldCommittedReplyReplaysAfterRecipientLeaves(t *testing.T) {
	var calls atomic.Int32
	application, _ := characterChatMattersApp(t, func(string) string {
		calls.Add(1)
		return `{"reply":"我们就在这里谈。"}`
	})
	module, err := interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.ActorStateXiuxianID)
	if err != nil {
		t.Fatal(err)
	}
	story, err := application.interactive.CreateStory(interactive.CreateStoryRequest{
		Title: "Letter reply replay", ActorState: &module.ActorState,
		PackBinding: &interactive.StoryPackBinding{
			PackID: "letter-retry", Scope: interactive.PackScopeFromID("letter-retry"),
			Gameplay: &interactive.StoryGameplayProfile{StatusTemplate: "world-actions-v1", Growth: "none", Combat: "none", Communication: "letter", LetterDeliverySeconds: 86400},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	commitPresence := func(present []any) {
		t.Helper()
		_, _, err := application.interactive.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
			BranchID: "main", User: "Observe", Narrative: "The scene changes.",
			ActorOps:   []interactive.ActorStateOp{{Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "在场角色", Value: present, Reason: "Committed physical presence"}},
			TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	commitPresence([]any{interactive.DefaultActorID, "recipient"})
	_, err = application.interactive.AppendCharacterFacts(story.ID, interactive.AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "午后", Profiles: []character.Profile{{ID: "recipient", Name: "Recipient"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := CharacterChatRequest{StoryID: story.ID, BranchID: "main", CharacterID: "recipient", Message: "Can we talk?", CommandID: "local-reply-once"}
	first, err := application.SendCharacterMessage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || first.Reply == "" {
		t.Fatalf("local reply did not complete once: calls=%d result=%+v", calls.Load(), first)
	}
	commitPresence([]any{interactive.DefaultActorID})
	before, err := application.interactive.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	prior, err := application.interactive.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	retried, err := application.SendCharacterMessage(context.Background(), request)
	if err != nil || retried.Reply != first.Reply || retried.CharacterID != first.CharacterID {
		t.Fatalf("committed reply was not replayed after departure: got=%+v err=%v want=%+v", retried, err, first)
	}
	request.CommandID = "new-remote-command"
	if _, err := application.SendCharacterMessage(context.Background(), request); !errors.Is(err, ErrCharacterChatUnavailable) {
		t.Fatalf("new remote contact was not rejected: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("retry or rejected contact called the model: %d calls", calls.Load())
	}
	after, err := application.interactive.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	current, err := application.interactive.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if before.Snapshot.TurnCount != after.Snapshot.TurnCount || interactive.SnapshotClockSeconds(before.Snapshot) != interactive.SnapshotClockSeconds(after.Snapshot) {
		t.Fatal("replay or rejected remote contact advanced the story clock")
	}
	if interactive.ContentHashOf(prior.ExchangesFor("recipient")) != interactive.ContentHashOf(current.ExchangesFor("recipient")) {
		t.Fatal("replay or rejected remote contact added or changed a conversation")
	}
}
