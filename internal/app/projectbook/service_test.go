package projectbook

import (
	"errors"
	"testing"

	booklore "ta/internal/book/lore"
)

const (
	worldA = "pack:world-a@hash-a"
	worldB = "pack:world-b@hash-b"
)

func TestResolveNewLoreScopeDefaultsToOpenWorld(t *testing.T) {
	scope, err := resolveNewLoreScope([]string{worldA, worldB}, worldA, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope != worldA {
		t.Fatalf("scope = %q, want %q", scope, worldA)
	}
}

func TestResolveNewLoreScopeWithoutOpenStoryStaysShared(t *testing.T) {
	scope, err := resolveNewLoreScope(nil, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope != booklore.ScopeShared {
		t.Fatalf("scope = %q, want shared", scope)
	}
}

func TestResolveNewLoreScopeSharedMustBeNamed(t *testing.T) {
	scope, err := resolveNewLoreScope([]string{worldA}, worldA, "shared")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope != booklore.ScopeShared {
		t.Fatalf("scope = %q, want shared", scope)
	}
}

func TestResolveNewLoreScopeKeepsKnownWorld(t *testing.T) {
	scope, err := resolveNewLoreScope([]string{worldA, worldB}, worldA, worldB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope != worldB {
		t.Fatalf("scope = %q, want %q", scope, worldB)
	}
}

func TestResolveNewLoreScopeRefusesForeignWorld(t *testing.T) {
	_, err := resolveNewLoreScope([]string{worldA}, worldA, worldB)
	if !errors.Is(err, ErrUnknownLoreScope) {
		t.Fatalf("err = %v, want ErrUnknownLoreScope", err)
	}
}

func TestResolveNewLoreScopeRefusesUnknownWhenNoStories(t *testing.T) {
	_, err := resolveNewLoreScope(nil, "", worldA)
	if !errors.Is(err, ErrUnknownLoreScope) {
		t.Fatalf("err = %v, want ErrUnknownLoreScope", err)
	}
}

func TestItemsInWorldScopesDropsSupersededRevisions(t *testing.T) {
	items := []booklore.Item{
		{ID: "npc-old", Name: "护堡执事", Scope: "pack:world-a@hash-old"},
		{ID: "npc-current", Name: "护堡执事", Scope: worldA},
		{ID: "npc-other", Name: "别人", Scope: worldB},
		{ID: "mine", Name: "自建", Scope: booklore.ScopeShared},
	}
	kept := itemsInWorldScopes(items, []string{worldA})
	if len(kept) != 2 {
		t.Fatalf("kept %d items, want 2: %+v", len(kept), kept)
	}
	if kept[0].ID != "npc-current" || kept[1].ID != "mine" {
		t.Fatalf("kept %s and %s, want npc-current and mine", kept[0].ID, kept[1].ID)
	}
}

// With no story open the library has no world to show, so only the shared book
// survives. Anything filed under a world must not appear as if it were the
// author's current setting.
func TestItemsInWorldScopesWithoutOpenStoryKeepsOnlyShared(t *testing.T) {
	items := []booklore.Item{
		{ID: "a", Scope: worldA},
		{ID: "shared", Scope: booklore.ScopeShared},
		{ID: "unscoped"},
	}
	kept := itemsInWorldScopes(items, nil)
	if len(kept) != 2 {
		t.Fatalf("kept %d items, want 2: %+v", len(kept), kept)
	}
	if kept[0].ID != "shared" || kept[1].ID != "unscoped" {
		t.Fatalf("kept %s and %s, want shared and unscoped", kept[0].ID, kept[1].ID)
	}
}

func TestItemsInWorldScopesKeepsEveryWorldOfThisProject(t *testing.T) {
	items := []booklore.Item{
		{ID: "a", Scope: worldA},
		{ID: "b", Scope: worldB},
		{ID: "c", Scope: "pack:world-c@hash-c"},
	}
	kept := itemsInWorldScopes(items, []string{worldA, worldB})
	if len(kept) != 2 {
		t.Fatalf("kept %d items, want 2: %+v", len(kept), kept)
	}
}
