package app

import (
	"encoding/json"
	"strings"
	"ta/internal/interactive"
	"testing"
)

func TestPlayerProjectionLegacyKnowledgeIsSanitizedNotBroadlyPublic(t *testing.T) {
	raw := map[string]any{"actors": map[string]any{"npc": map[string]any{"template_id": "important_character", "state": map[string]any{
		"已知位置": map[string]any{"location": "PUBLIC_DOCK", "source": "observation", "nature": "experienced", "evidence": "SECRET_EVIDENCE"},
		"未声明":  "SECRET_UNKNOWN",
	}}}}
	audience := loreKnowledgeAudience{known: map[string]bool{"npc": true}}
	out := playerKnownState(raw, nil, audience)
	encoded, _ := json.Marshal(out)
	if !strings.Contains(string(encoded), "PUBLIC_DOCK") || strings.Contains(string(encoded), "SECRET_") {
		t.Fatalf("legacy authorized projection lost its boundary: %s", encoded)
	}
	noAudience, _ := json.Marshal(playerKnownState(raw, nil))
	if strings.Contains(string(noAudience), "PUBLIC_DOCK") {
		t.Fatal("unwitnessed location exposed")
	}
	schema := &interactive.ActorStateSchemaSnapshot{System: interactive.StoryDirectorActorStateSystem{Templates: []interactive.ActorStateTemplate{{ID: "important_character", Fields: []interactive.ActorStateField{{Name: "已知位置", Type: "object", Visibility: interactive.ActorStateVisibilityPrivate}}}}}}
	private, _ := json.Marshal(playerKnownState(raw, schema, audience))
	if strings.Contains(string(private), "PUBLIC_DOCK") {
		t.Fatal("legacy projection overrode explicit private visibility")
	}
}
