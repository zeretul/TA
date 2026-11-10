package app

import (
	"encoding/json"
	"strings"
	"testing"

	"ta/internal/interactive"
)

// Public relationship notes are the protagonist's learned knowledge; they do
// not reopen the NPC's private relationship dossier through the new reader.
func TestLoreSocialProjectionKeepsKnowledgeSeparateFromPrivateRelationships(t *testing.T) {
	schema := &interactive.ActorStateSchemaSnapshot{System: interactive.StoryDirectorActorStateSystem{Templates: []interactive.ActorStateTemplate{{
		ID: interactive.ActorStateImportantCharacterTemplateID,
		Fields: []interactive.ActorStateField{
			{Name: "所属势力", Path: "identity.affiliations", Type: "list"},
			{Name: "已知人物关系", Path: "knowledge.relationships", Type: "object"},
			{Name: "关系", Path: "relations.records", Type: "object", Visibility: interactive.ActorStateVisibilityPrivate},
		},
	}}}}
	raw := map[string]any{"actors": map[string]any{"npc": map[string]any{
		"template_id": interactive.ActorStateImportantCharacterTemplateID,
		"state":       map[string]any{"所属势力": []any{"已知宗门"}, "已知人物关系": map[string]any{"other": map[string]any{"kind": "本人介绍的同门", "source": "conversation"}}, "关系": map[string]any{"hidden": "秘密仇怨"}},
	}}}
	data, err := json.Marshal(playerKnownState(raw, schema, loreKnowledgeAudience{known: map[string]bool{"npc": true, "other": true}}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "秘密仇怨") || strings.Contains(string(data), "hidden") {
		t.Fatalf("private relationships escaped: %s", data)
	}
	if !strings.Contains(string(data), "已知宗门") || !strings.Contains(string(data), "本人介绍的同门") {
		t.Fatalf("learned social facts disappeared: %s", data)
	}
	private := raw["actors"].(map[string]any)["npc"].(map[string]any)["state"].(map[string]any)["关系"]
	if private == nil {
		t.Fatal("read projection mutated canonical state")
	}
}
