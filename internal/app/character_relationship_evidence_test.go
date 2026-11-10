package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/interactive"
)

// Earn the floor through normal settlement, without injecting stored records.
func earnPrivateRelationshipAffinity(t *testing.T, application *App, storyID string) {
	t.Helper()
	_, err := application.interactive.AppendCharacterFacts(storyID, interactive.AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "午后",
		Facts: character.TurnCharacterFacts{Affinities: []character.AffinityDraft{
			{CharacterID: "陈婆婆", Kind: character.AffinityMajor, Delta: 15, Basis: "主角冒险救出被扣押的阿九"},
			{CharacterID: "陈婆婆", Kind: character.AffinityMeaningful, Delta: 8, Basis: "主角替老妇修好漏雨的屋顶"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := state.AffinityFor("陈婆婆"); value != 23 {
		t.Fatalf("settled affinity = %d, want 23", value)
	}
}

func TestPrivateRelationshipRejectsOneSidedDoubleCitation(t *testing.T) {
	application, storyID := characterChatMattersApp(t, answerFor(
		`{"reply":"我把你当朋友，你慢慢考虑。","relationships":[{"character_id":"陈婆婆","kind":"friend","statement":"两人结为朋友","expressed_by_index":2,"answered_by_index":2}]}`))
	earnPrivateRelationshipAffinity(t, application, storyID)
	before, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	_, err = application.SendCharacterMessage(context.Background(), CharacterChatRequest{
		StoryID: storyID, CharacterID: "陈婆婆", Message: "你如何看待我？", CommandID: "relationship-one-sided",
	})
	if err == nil || !strings.Contains(err.Error(), "relationship") {
		t.Fatalf("one-sided relationship should be rejected, got %v", err)
	}
	after, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Relationships) != 0 || len(after.Exchanges) != len(before.Exchanges) {
		t.Fatalf("rejected relationship partially committed: relationships=%+v, exchanges=%d -> %d", after.Relationships, len(before.Exchanges), len(after.Exchanges))
	}
}

func TestPrivateRelationshipCurrentBondReachesWindowAndNextPrompt(t *testing.T) {
	const friendship = "两人当面结为朋友"
	const parting = "陈婆婆明确结束了这段友情"
	replies := []string{
		`{"reply":"好，往后我们就是朋友。","relationships":[{"character_id":"陈婆婆","kind":"friend","statement":"` + friendship + `","expressed_by_index":1,"answered_by_index":2}]}`,
		`{"reply":"朋友，坐下喝茶吧。"}`,
		`{"reply":"这段友情到此为止。","relationships":[{"character_id":"陈婆婆","kind":"estranged","statement":"` + parting + `","expressed_by_index":2,"answered_by_index":0}]}`,
		`{"reply":"我们已不再是朋友。"}`,
	}
	// Each request consumes one response; channels also synchronize the server
	// goroutine with assertions about the next request's actual model context.
	answers := make(chan string, len(replies))
	prompts := make(chan string, len(replies))
	for _, reply := range replies {
		answers <- reply
	}
	close(answers)
	application, storyID := characterChatMattersApp(t, func(prompt string) string {
		prompts <- prompt
		return <-answers
	})
	earnPrivateRelationshipAffinity(t, application, storyID)
	for index, message := range []string{"愿意和我做朋友吗？", "我来看你了。", "我听你说。", "你现在怎样看待我们？"} {
		result, err := application.SendCharacterMessage(context.Background(), CharacterChatRequest{
			StoryID: storyID, CharacterID: "陈婆婆", Message: message, CommandID: fmt.Sprintf("relationship-step-%d", index),
		})
		if err != nil {
			t.Fatalf("step %d: %v", index, err)
		}
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal([]byte(<-prompts), &request); err != nil {
			t.Fatal(err)
		}
		system := ""
		for _, entry := range request.Messages {
			if entry.Role == "system" {
				system += entry.Content
			}
		}
		if index == 1 && !strings.Contains(system, "What stands between you: friend. "+friendship) {
			t.Fatal("the next model request did not receive the committed friendship")
		}
		if index == 3 && (!strings.Contains(system, "What stands between you: estranged. "+parting) || strings.Contains(system, "What stands between you: friend.")) {
			t.Fatal("the next model request retained the superseded friendship instead of the parting")
		}
		want := character.RelationshipFriend
		if index >= 2 {
			want = character.RelationshipEstranged
		}
		if result.Relationship == nil || result.Relationship.Kind != want {
			t.Fatalf("step %d chat relationship = %+v, want %s", index, result.Relationship, want)
		}
		window, err := application.CharacterWindow(CharacterWindowRequest{StoryID: storyID, CharacterID: "陈婆婆"})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, person := range window.Characters {
			if person.ID == "陈婆婆" {
				found = true
				if person.Relationship == nil || person.Relationship.Kind != want || person.Relationship.ID != result.Relationship.ID {
					t.Fatalf("step %d window relationship = %+v, chat = %+v", index, person.Relationship, result.Relationship)
				}
			}
		}
		if !found {
			t.Fatal("the character disappeared from the window")
		}
	}
	state, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Relationships) != 2 || state.Relationships[0].Kind != character.RelationshipFriend || state.Relationships[1].Kind != character.RelationshipEstranged {
		t.Fatalf("relationship history = %+v, want friendship followed by parting", state.Relationships)
	}
}
