package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	agent "ta/agent"
	"ta/internal/book/lore"
)

// This uses the public tool parser and inferred schema, not a prefilled store.
func TestWriteLoreItemsAudienceToolRoundTrip(t *testing.T) {
	workspace := t.TempDir()
	definitions, err := newLoreTools(workspace, true)
	if err != nil {
		t.Fatal(err)
	}
	var write agent.Tool
	for _, definition := range definitions {
		info, err := definition.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == "write_lore_items" {
			write = definition.Tool
			schema, err := info.ToJSONSchema()
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(schema)
			if err != nil || !strings.Contains(string(raw), `"audience"`) {
				t.Fatalf("tool schema cannot express audience: %s, %v", raw, err)
			}
		}
	}
	if write == nil {
		t.Fatal("write_lore_items was not registered")
	}
	run := func(payload string) {
		t.Helper()
		result, err := write.Run(context.Background(), payload)
		if err != nil || result.Status != agent.ToolResultSuccess {
			t.Fatalf("tool write failed: %v, %s", err, result.ModelContent)
		}
	}
	run(`{"items":[{"id":"secret","name":"Archive","audience":"gm","content":"GM_SECRET_MARKER"},{"id":"legacy","name":"Market","content":"PUBLIC_MARKER"},{"id":"unknown","name":"Unknown","audience":"conditional_unmapped","content":"UNKNOWN_SECRET_MARKER"}]}`)
	hot := lore.NewStore(workspace)
	assertItem := func(reader *lore.Store, id, audience, content string) {
		t.Helper()
		item, err := reader.Read(id)
		if err != nil || item.Audience != audience || item.Content != content {
			t.Fatalf("author read lost %s: %+v, %v", id, item, err)
		}
	}
	assertItem(hot, "secret", "gm", "GM_SECRET_MARKER")
	assertItem(lore.NewStore(workspace), "unknown", "gm", "UNKNOWN_SECRET_MARKER")
	assertItem(lore.NewStore(workspace), "legacy", "", "PUBLIC_MARKER")
	run(`{"items":[{"id":"secret","content":"EDITED_SECRET_MARKER"}]}`)
	assertItem(hot, "secret", "gm", "EDITED_SECRET_MARKER")
	assertItem(lore.NewStore(workspace), "secret", "gm", "EDITED_SECRET_MARKER")
	// A visibility-only update is a real change, and an omitted audience must
	// not reset it on the following ordinary content edit.
	run(`{"items":[{"id":"legacy","audience":"gm"}]}`)
	assertItem(lore.NewStore(workspace), "legacy", "gm", "PUBLIC_MARKER")
	filtered := lore.NewStore(workspace).WithVisibility(lore.IsPublic)
	for _, id := range []string{"secret", "legacy", "unknown"} {
		if _, err := filtered.Read(id); err == nil {
			t.Fatalf("public reader received %s", id)
		}
	}
	run(`{"items":[{"id":"legacy","audience":"public"}]}`)
	assertItem(filtered, "legacy", "public", "PUBLIC_MARKER")
}
