package app

import (
	"strings"
	"ta/internal/character"
	"ta/internal/interactive"
	"testing"
)

func TestNPCAuthoredFactsStayPrivateAndKnowledgeStaysExplicit(t *testing.T) {
	schema := &interactive.ActorStateSchemaSnapshot{System: interactive.StoryDirectorActorStateSystem{Templates: []interactive.ActorStateTemplate{{ID: interactive.ActorStateImportantCharacterTemplateID, Fields: []interactive.ActorStateField{
		{Name: "成长记录", Type: "object", Visibility: interactive.ActorStateVisibilityPrivate},
		{Name: "钱财", Type: "number", Visibility: interactive.ActorStateVisibilityPrivate},
		{Name: "公开身份", Type: "string"},
	}}}}}
	state := map[string]any{"actors": map[string]any{
		"npc-stable":               map[string]any{"template_id": interactive.ActorStateImportantCharacterTemplateID, "state": map[string]any{"成长记录": map[string]any{"hidden": "UNKNOWN_SELF_SECRET"}, "钱财": 17, "公开身份": "PUBLIC_IDENTITY"}},
		interactive.DefaultActorID: map[string]any{"template_id": interactive.ActorStateImportantCharacterTemplateID, "state": map[string]any{"钱财": 23}},
	}}
	view := playerKnownState(state, schema)
	actors := view["actors"].(map[string]any)
	npc := actors["npc-stable"].(map[string]any)["state"].(map[string]any)
	if _, ok := npc["钱财"]; ok {
		t.Fatal("NPC private wallet reached player")
	}
	if _, ok := npc["成长记录"]; ok {
		t.Fatal("NPC private growth reached player")
	}
	if npc["公开身份"] != "PUBLIC_IDENTITY" {
		t.Fatal("public field was removed")
	}
	if actors[interactive.DefaultActorID].(map[string]any)["state"].(map[string]any)["钱财"] != 23 {
		t.Fatal("player lost own state")
	}
	if state["actors"].(map[string]any)["npc-stable"].(map[string]any)["state"].(map[string]any)["钱财"] != 17 {
		t.Fatal("player projection mutated GM truth")
	}
	memory := character.State{Knowledge: []character.KnowledgeRecord{
		{ID: "old", CharacterID: "npc-stable", Statement: "OLD_WALLET_17", Nature: character.NatureExperienced, Audience: character.AudienceSingle, SupersededBy: "new", SourceTurnID: "turn-1"},
		{ID: "new", CharacterID: "npc-stable", Statement: "KNOWN_WALLET_15", Nature: character.NatureExperienced, Audience: character.AudienceSingle, Supersedes: "old", SourceTurnID: "turn-2"},
	}}
	own := character.Prepare(character.PrepareInput{Profile: character.Profile{ID: "npc-stable", Name: "Adult trader"}, Knowledge: memory.KnowledgeFor("npc-stable")}).Markdown()
	if !strings.Contains(own, "KNOWN_WALLET_15") || strings.Contains(own, "UNKNOWN_SELF_SECRET") {
		t.Fatalf("explicit self-knowledge boundary failed: %s", own)
	}
	other := character.Prepare(character.PrepareInput{Profile: character.Profile{ID: "other", Name: "Other adult"}, Knowledge: memory.KnowledgeFor("other")}).Markdown()
	if strings.Contains(other, "KNOWN_WALLET_15") || strings.Contains(other, "OLD_WALLET_17") {
		t.Fatal("another NPC learned private wallet")
	}
	live := character.BuildKnowledgeView("npc-stable", memory.KnowledgeFor("npc-stable"), 24)
	if len(live.Records) != 1 || live.Records[0].ID != "new" {
		t.Fatalf("superseded self-knowledge still live: %+v", live)
	}
}
