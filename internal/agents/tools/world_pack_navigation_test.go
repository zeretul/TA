package tools

import (
	"encoding/json"
	"strings"
	"testing"

	agent "ta/agent"
	"ta/internal/book/lore"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

func navigationCall(t *testing.T, definition agent.ToolDefinition, input any) string {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	result := runTool(t, definition, string(raw))
	if result.Status != agent.ToolResultSuccess {
		t.Fatalf("tool failed: %s", result.ModelContent)
	}
	return result.ModelContent
}

func navigationNameAfter(t *testing.T, text, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		at := strings.Index(line, prefix+"read_lore_items names=")
		if at < 0 {
			continue
		}
		var names []string
		tail := line[at+len(prefix+"read_lore_items names="):]
		if err := json.NewDecoder(strings.NewReader(tail)).Decode(&names); err != nil {
			t.Fatal(err)
		}
		if len(names) != 1 {
			t.Fatalf("navigation must give one exact name: %s", line)
		}
		return names[0]
	}
	t.Fatalf("no executable name navigation after %q", prefix)
	return ""
}

func TestBuiltinWorldNavigationUsesRealScopedLoreTools(t *testing.T) {
	variants := []string{"doupo-continent", "harry-potter-1991", "harry-potter-1991-student", "lord-of-mysteries", "zichuan-xichuan", "sanguo-190"}
	library := storypack.NewLibrary(t.TempDir())
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			pack, err := library.GetContent(variant + "-ta07-full-gm-candidate")
			if err != nil {
				t.Fatal(err)
			}
			workspace := t.TempDir()
			store := lore.NewStore(workspace)
			inputs := pack.WorldInputs()
			ops := make([]lore.Operation, 0, len(inputs))
			for _, in := range inputs {
				ops = append(ops, lore.Operation{Op: "create", Item: in})
			}
			if _, err := store.ApplyOperations("Seed complete navigation fixture", ops); err != nil {
				t.Fatal(err)
			}
			scope := pack.WorldScope()
			definitions, err := newLoreTools(workspace, false, loreToolsOptions{Scope: &scope})
			if err != nil {
				t.Fatal(err)
			}
			read := findTool(t, definitions, "read_lore_items")
			list := findTool(t, definitions, "list_lore_items")
			logical := "author-" + variant + "-config-index"
			runtimeID := interactive.PackEntryIDForRevision(logical, pack.ContentHash())
			indexed := navigationCall(t, list, map[string]any{"keywords": []string{logical}, "detail": "index", "limit": 50})
			if !strings.Contains(indexed, runtimeID) {
				t.Fatalf("logical keyword did not discover scoped runtime ID: %s", logical)
			}
			miss := navigationCall(t, read, map[string]any{"ids": []string{logical}})
			if !strings.Contains(miss, "No lore item matched") {
				t.Fatal("bare logical ID unexpectedly bypassed revision qualification")
			}
			root := navigationCall(t, read, map[string]any{"names": []string{"当前配置与实体关联"}})
			firstName := navigationNameAfter(t, root, "1. ")
			first := navigationCall(t, read, map[string]any{"names": []string{firstName}})
			if !strings.Contains(first, "原文片段开始") {
				t.Fatal("first page body not returned")
			}
			nextName := navigationNameAfter(t, first, "下一页：")
			next := navigationCall(t, read, map[string]any{"names": []string{nextName}})
			if !strings.Contains(next, "原文片段开始") {
				t.Fatal("next page body not returned")
			}
			// Follow the author-file index to a complete MD/JSON document navigation.
			allSources := navigationCall(t, read, map[string]any{"names": []string{"完整作者来源索引"}})
			docName := navigationNameAfter(t, allSources, "→ ")
			doc := navigationCall(t, read, map[string]any{"names": []string{docName}})
			pageName := navigationNameAfter(t, doc, "1. ")
			sourcePage := navigationCall(t, read, map[string]any{"names": []string{pageName}})
			if !strings.Contains(sourcePage, "作者原文片段开始") {
				t.Fatal("author-file page was not reachable")
			}
			// A role without GM access cannot traverse the same names or logical keyword.
			restricted, err := newLoreTools(workspace, false, loreToolsOptions{Scope: &scope, Visibility: func() func(lore.Item) bool { return lore.IsPublic }})
			if err != nil {
				t.Fatal(err)
			}
			hidden := navigationCall(t, findTool(t, restricted, "read_lore_items"), map[string]any{"names": []string{firstName}})
			if !strings.Contains(hidden, "No lore item matched") {
				t.Fatal("GM page leaked through a names lookup")
			}
			// Revisions keep identical display names isolated. Seed only the target of
			// this boundary check; the original complete pack was seeded above.
			var firstInput lore.ItemInput
			for _, in := range inputs {
				if in.Name == firstName {
					firstInput = in
					break
				}
			}
			if firstInput.ID == "" {
				t.Fatal("missing persisted page input")
			}
			changed := pack
			changed.World = append([]storypack.WorldEntry(nil), pack.World...)
			for i := range changed.World {
				if changed.World[i].Name == firstName {
					changed.World[i].Content = "NEW-REVISION-ONLY"
				}
			}
			var newInput lore.ItemInput
			for _, in := range changed.WorldInputs() {
				if in.Name == firstName {
					newInput = in
					break
				}
			}
			if newInput.Scope == firstInput.Scope || newInput.ID == firstInput.ID {
				t.Fatal("revision identity was not frozen")
			}
			if _, err := store.ApplyOperations("Seed alternate revision target", []lore.Operation{{Op: "create", Item: newInput}}); err != nil {
				t.Fatal(err)
			}
			oldRead := navigationCall(t, read, map[string]any{"names": []string{firstName}})
			if strings.Contains(oldRead, "NEW-REVISION-ONLY") || !strings.Contains(oldRead, "原文片段开始") {
				t.Fatal("old revision name read changed")
			}
			newScope := newInput.Scope
			newTools, err := newLoreTools(workspace, false, loreToolsOptions{Scope: &newScope})
			if err != nil {
				t.Fatal(err)
			}
			newRead := navigationCall(t, findTool(t, newTools, "read_lore_items"), map[string]any{"names": []string{firstName}})
			if !strings.Contains(newRead, "NEW-REVISION-ONLY") {
				t.Fatal("new revision did not resolve its own same-name target")
			}
			foreign := navigationCall(t, read, map[string]any{"ids": []string{newInput.ID}})
			if !strings.Contains(foreign, "No lore item matched") {
				t.Fatal("guessing a foreign revision ID crossed scope")
			}
			if _, err := store.Create(lore.ItemInput{ID: "shared-nav-shadow", Name: firstName, Content: "SHARED-NAME-SHADOW"}); err != nil {
				t.Fatal(err)
			}
			own := navigationCall(t, read, map[string]any{"names": []string{firstName}})
			if strings.Contains(own, "SHARED-NAME-SHADOW") {
				t.Fatal("shared duplicate name shadowed the bound world")
			}
		})
	}
}
