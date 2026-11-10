package interactiveapp

import (
	"strings"
	"ta/internal/character"
	"ta/internal/interactive"
	"testing"
)

func TestPlayerAttentionCastPrefersKnownPeopleWithoutChangingPresence(t *testing.T) {
	state := character.State{Profiles: []character.Profile{
		{ID: "a", Name: "Ordinary", Revision: 1}, {ID: "z", Name: "Followed", Revision: 1}, {ID: "present", Name: "Present", Revision: 1},
	}}
	ctx := interactive.StoryContext{Snapshot: interactive.Snapshot{State: map[string]any{
		"player_attention": interactive.PlayerAttention{V: 1, Characters: []string{"z"}, Quests: []string{"quest.known"}},
		"actors":           map[string]any{interactive.DefaultStoryContextActorID: map[string]any{"state": map[string]any{"在场角色": []any{"present"}}}},
	}}}
	text := (&Conversation{}).characterCastContext(ctx, state, true, nil)
	present, followed, ordinary := strings.Index(text, "id: present"), strings.Index(text, "id: z"), strings.Index(text, "id: a")
	if present < 0 || followed <= present || ordinary <= followed {
		t.Fatalf("attention displaced presence or did not prioritize known cast:\n%s", text)
	}
	if !strings.Contains(text, "quest.known") || !strings.Contains(text, "不揭露秘密") {
		t.Fatalf("missing bounded preference instructions:\n%s", text)
	}
	if len(state.Profiles) != 3 || state.Profiles[0].ID != "a" {
		t.Fatal("context changed source profiles")
	}
}
