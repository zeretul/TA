package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	agent "ta/agent"
	"ta/internal/agents/configresource"
	"ta/internal/book/lore"
	"ta/internal/character"
	"ta/internal/storypack"
)

// Exercise config_apply/config_read through the real app writer and pack file.
// The imported lore then passes through the same character projection as chat.
func TestWorldBookAuthorToolAudiencePersistenceAndNPCBoundary(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	root := application.cfg.DataDir()
	writeScopedSettingsPack(t, root, "audience-world", "Audience world", "")
	writer := agentScopedSettings{app: application}
	definitions, err := configresource.NewScopedTools(application.cfg, 0, []string{configresource.WorldBookResource}, &configresource.ScopedTarget{
		WorldID: "audience-world", Writer: writer,
	})
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]agent.Tool{}
	for _, definition := range definitions {
		info, err := definition.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		tools[info.Name] = definition.Tool
	}
	run := func(name, payload string) string {
		t.Helper()
		tool := tools[name]
		if tool == nil {
			t.Fatalf("missing tool %s", name)
		}
		result, err := tool.Run(context.Background(), payload)
		if err != nil || result.Status != agent.ToolResultSuccess {
			t.Fatalf("%s failed: %v, %s", name, err, result.ModelContent)
		}
		return result.ModelContent
	}
	for _, entry := range []struct{ id, name, audience, content string }{
		{"archive", "Archive", "gm", "GM_SECRET_MARKER"},
		{"unknown", "Unknown", "conditional_unmapped", "UNKNOWN_SECRET_MARKER"},
		{"market", "Market", "", "PUBLIC_MARKER"},
	} {
		value := map[string]any{"entry_id": entry.id, "name": entry.name, "type": "rule", "load_mode": "auto", "content": entry.content}
		if entry.audience != "" {
			value["audience"] = entry.audience
		}
		raw, err := json.Marshal(map[string]any{"operation": "create", "resource": "world_book", "value": value})
		if err != nil {
			t.Fatal(err)
		}
		run("config_apply", string(raw))
	}
	// A legacy partial collection keeps the saved audience even though the
	// old caller does not send the newly introduced field.
	run("config_apply", `{"operation":"create","resource":"world_book","value":{"entry_id":"archive","name":"Archive","content":"EDITED_SECRET_MARKER"}}`)
	// config_apply intentionally returns only an operation receipt. Check the
	// writer's richer collected value independently before the canonical read.
	collected, err := writer.CollectWorldEntry(context.Background(), "audience-world", configresource.WorldEntryValue{
		EntryID: "archive", Name: "Archive", Content: "EDITED_SECRET_MARKER",
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := json.Marshal(collected)
	if err != nil || !strings.Contains(string(receipt), `"audience":"gm"`) {
		t.Fatalf("writer receipt lost GM audience: %s, %v", receipt, err)
	}
	for _, request := range []string{
		`{"operation":"list","resource":"world_book"}`,
		`{"operation":"get","resource":"world_book","ids":["audience-world"]}`,
	} {
		output := run("config_read", request)
		if !strings.Contains(output, `"audience":"gm"`) || !strings.Contains(output, "EDITED_SECRET_MARKER") {
			t.Fatalf("author tool read lost audience or content: %s", output)
		}
	}
	pack, err := storypack.NewLibrary(root).GetContent("audience-world")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range pack.World {
		want := "gm"
		if entry.ID == "market" {
			want = ""
		}
		if entry.Audience != want {
			t.Fatalf("cold pack read changed %s audience: got %q, want %q", entry.ID, entry.Audience, want)
		}
	}
	workspace := t.TempDir()
	store := lore.NewStore(workspace)
	var ops []lore.Operation
	for _, input := range pack.WorldInputs() {
		ops = append(ops, lore.Operation{Op: "create", Item: input})
	}
	if _, err := store.ApplyOperations("Import authored lore", ops); err != nil {
		t.Fatal(err)
	}
	for _, reader := range []*lore.Store{store, lore.NewStore(workspace)} {
		items, err := reader.ListAll()
		if err != nil || len(items) != 3 {
			t.Fatalf("author editor lost imported records: %d, %v", len(items), err)
		}
	}
	projected := strings.Join(characterLoreFor(workspace, pack.WorldScope(), character.Profile{Name: "Archive Unknown Market"}, character.Scene{}, 0), "\n")
	if !strings.Contains(projected, "PUBLIC_MARKER") {
		t.Fatalf("public lore disappeared: %s", projected)
	}
	for _, marker := range []string{"EDITED_SECRET_MARKER", "UNKNOWN_SECRET_MARKER"} {
		if strings.Contains(projected, marker) {
			t.Fatalf("NPC projection leaked %s: %s", marker, projected)
		}
	}
}
