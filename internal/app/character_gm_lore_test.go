package app

import (
	"encoding/json"
	"strings"
	"testing"

	"ta/internal/book/lore"
	"ta/internal/character"
	"ta/internal/interactive"
)

func TestCharacterLoreExcludesGMButRefereeCanRead(t *testing.T) {
	root := t.TempDir()
	store := lore.NewScopedStore(root, "test-world")
	for _, payload := range []string{
		`{"id":"public","name":"Market","type":"location","audience":"public","content":"PUBLIC_MARKER","load_mode":"auto"}`,
		`{"id":"gm","name":"Archive","type":"location","audience":"gm","content":"GM_SECRET_MARKER","load_mode":"auto"}`,
		`{"id":"unknown","name":"Unknown","type":"rule","audience":"conditional_unmapped","content":"UNKNOWN_SECRET_MARKER","load_mode":"auto"}`,
	} {
		var input lore.ItemInput
		if err := json.Unmarshal([]byte(payload), &input); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Create(input); err != nil {
			t.Fatal(err)
		}
	}
	referee := lore.NewScopedStore(root, "test-world").WithVisibility(interactive.StoryLoreVisibility(interactive.StoryMeta{}, nil))
	if item, err := referee.Read("gm"); err != nil || !strings.Contains(item.Content, "GM_SECRET_MARKER") {
		t.Fatalf("referee lost GM: %+v %v", item, err)
	}
	projected := strings.Join(characterLoreFor(root, "test-world", character.Profile{Name: "Market Archive Unknown"}, character.Scene{}, 0), "\n")
	if !strings.Contains(projected, "PUBLIC_MARKER") {
		t.Fatal("public grounding was removed")
	}
	for _, marker := range []string{"GM_SECRET_MARKER", "UNKNOWN_SECRET_MARKER"} {
		if strings.Contains(projected, marker) {
			t.Fatalf("character received %s", marker)
		}
	}
}

func TestCharacterLoreImagesExcludeGM(t *testing.T) {
	binding := &interactive.StoryPackBinding{Scope: "test-world", ContentHash: "revision"}
	catalog := &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{SourceKey: "person", LoreID: "person-entry"}}}
	var item lore.Item
	if err := json.Unmarshal([]byte(`{"audience":"gm","image":{"image_path":"assets/image/secret.png"}}`), &item); err != nil {
		t.Fatal(err)
	}
	item.ID = interactive.PackEntryIDForRevision("person-entry", "revision")
	item.Scope = "test-world"
	if images := characterLoreImageSources(binding, catalog, []lore.Item{item}); len(images) != 0 {
		t.Fatalf("GM image exposed: %+v", images)
	}
}
