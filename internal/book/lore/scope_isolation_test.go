package lore

import (
	"strings"
	"testing"
)

// seedItem writes one entry through an unbound store, which is what the pack
// seeder and the lore editor use: the caller's scope is honoured.
func seedItem(t *testing.T, store *Store, scope, id, name, content, loadMode string) Item {
	t.Helper()
	item, err := store.Create(ItemInput{
		ID:       id,
		Scope:    scope,
		Type:     "setting",
		Name:     name,
		Content:  content,
		LoadMode: loadMode,
	})
	if err != nil {
		t.Fatalf("seed %s/%s: %v", scope, id, err)
	}
	return item
}

// TestGetHonoursScope covers the read path that resolves by identity rather than
// through the list. A bound store that could still fetch a sibling world's entry
// by ID would hand the model exactly the lore scoping exists to withhold, and
// the caller would never notice because the list looks correct.
func TestGetHonoursScope(t *testing.T) {
	workspace := t.TempDir()
	seeder := NewStore(workspace)

	seedItem(t, seeder, ScopeShared, "shared-1", "通用规则", "通用设定", LoadModeAuto)
	seedItem(t, seeder, "pack:alpha", "alpha-town", "青石镇", "阿尔法世界的青石镇", LoadModeAuto)
	seedItem(t, seeder, "pack:beta", "beta-town", "青石镇", "贝塔世界的青石镇", LoadModeAuto)

	alpha := NewScopedStore(workspace, "pack:alpha")

	if _, err := alpha.Get("alpha-town"); err != nil {
		t.Fatalf("a bound store could not read its own entry by id: %v", err)
	}
	if _, err := alpha.Get("shared-1"); err != nil {
		t.Fatalf("a bound store could not read the shared entry by id: %v", err)
	}
	if item, err := alpha.Get("beta-town"); err == nil {
		t.Fatalf("a bound store read another world's entry by id: %+v", item)
	}

	// The editor resolves identity across every scope, which is what its review
	// links and tabs depend on.
	if _, err := seeder.Get("beta-town"); err != nil {
		t.Fatalf("the unbound editor lost identity lookup: %v", err)
	}
}

// TestGetHonoursShadowing keeps the identity path consistent with the list. When
// a world's own entry claims a name a shared entry also has, the shared entry is
// hidden from that world's view; fetching it by ID must not route around that
// and answer with the fact the world replaced.
func TestGetHonoursShadowing(t *testing.T) {
	workspace := t.TempDir()
	seeder := NewStore(workspace)

	seedItem(t, seeder, ScopeShared, "shared-town", "青石镇", "用户自己的青石镇", LoadModeAuto)
	seedItem(t, seeder, "pack:alpha", "alpha-town", "青石镇", "阿尔法世界的青石镇", LoadModeAuto)

	alpha := NewScopedStore(workspace, "pack:alpha")
	if _, err := alpha.Get("shared-town"); err == nil {
		t.Fatal("a bound store fetched a shared entry its own world shadows")
	}
	// The world's own entry stays readable; only the shadowed one is hidden.
	own, err := alpha.Get("alpha-town")
	if err != nil {
		t.Fatalf("the world's own entry became unreadable: %v", err)
	}
	if !strings.Contains(own.Content, "阿尔法世界的青石镇") {
		t.Fatalf("the world resolved its own entry to the wrong body: %q", own.Content)
	}

	// A world that does not shadow it still resolves it by ID.
	beta := NewScopedStore(workspace, "pack:beta")
	if _, err := beta.Get("shared-town"); err != nil {
		t.Fatalf("an unrelated world lost the shared entry: %v", err)
	}
	// And the editor always can, because the entry really exists.
	if _, err := seeder.Get("shared-town"); err != nil {
		t.Fatalf("the editor lost the shadowed entry: %v", err)
	}
}

func itemIDs(items []Item) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

// TestScopedStoresDoNotSeeEachOthersWorldBook is the isolation guarantee the
// whole world-binding rests on: two packs sharing one workspace must not leak
// entries, resident context or name lookups into each other, and neither may
// reach the user's own general book.
func TestScopedStoresDoNotSeeEachOthersWorldBook(t *testing.T) {
	workspace := t.TempDir()
	seeder := NewStore(workspace)

	seedItem(t, seeder, ScopeShared, "shared-1", "通用规则", "任何世界都适用的通用设定", LoadModeResident)
	seedItem(t, seeder, "pack:alpha", "alpha-town", "青石镇", "阿尔法世界的青石镇，靠海", LoadModeResident)
	seedItem(t, seeder, "pack:beta", "beta-town", "青石镇", "贝塔世界的青石镇，在内陆", LoadModeResident)
	seedItem(t, seeder, "pack:beta", "beta-secret", "贝塔密辛", "只有贝塔世界知道的事", LoadModeAuto)

	alpha := NewScopedStore(workspace, "pack:alpha")
	if !alpha.Bound() {
		t.Fatal("NewScopedStore must return a bound store")
	}

	items, err := alpha.List()
	if err != nil {
		t.Fatal(err)
	}
	got := itemIDs(items)
	if len(got) != 2 {
		t.Fatalf("alpha saw %v, want its own entry plus the shared one", got)
	}
	for _, id := range got {
		if id == "beta-town" || id == "beta-secret" {
			t.Fatalf("alpha saw another world's entry: %s", id)
		}
	}

	// The same display name exists in both worlds. Each store must answer with
	// its own, never with the sibling's and never with an arbitrary winner.
	resolved, err := alpha.ReadManyNames([]string{"青石镇"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Items) != 1 || resolved.Items[0].ID != "alpha-town" {
		t.Fatalf("alpha resolved 青石镇 to %v, want alpha-town", itemIDs(resolved.Items))
	}
	if !strings.Contains(resolved.Items[0].Content, "靠海") {
		t.Fatalf("alpha resolved the wrong world's body: %q", resolved.Items[0].Content)
	}

	beta := NewScopedStore(workspace, "pack:beta")
	resolvedBeta, err := beta.ReadManyNames([]string{"青石镇"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolvedBeta.Items) != 1 || resolvedBeta.Items[0].ID != "beta-town" {
		t.Fatalf("beta resolved 青石镇 to %v, want beta-town", itemIDs(resolvedBeta.Items))
	}

	// A shared entry stays reachable from every world: it is the user's own
	// general book, not any one pack's.
	shared, err := alpha.ReadManyNames([]string{"通用规则"})
	if err != nil {
		t.Fatal(err)
	}
	if len(shared.Items) != 1 || shared.Items[0].ID != "shared-1" {
		t.Fatalf("a world could not read the user's shared entry: %v", itemIDs(shared.Items))
	}
}

// TestResidentContextIsScoped proves the context actually injected into the
// model is filtered, not merely the browsing list. A leak here is the one that
// would silently put another world's resident lore in front of the narrator.
func TestResidentContextIsScoped(t *testing.T) {
	workspace := t.TempDir()
	seeder := NewStore(workspace)

	seedItem(t, seeder, "pack:alpha", "alpha-town", "青石镇", "阿尔法世界的青石镇，靠海", LoadModeResident)
	seedItem(t, seeder, "pack:beta", "beta-town", "青石镇", "贝塔世界的青石镇，在内陆", LoadModeResident)
	seedItem(t, seeder, "pack:beta", "beta-secret", "贝塔密辛", "只有贝塔世界知道的事", LoadModeResident)

	alpha := NewScopedStore(workspace, "pack:alpha")
	markdown, err := alpha.ResidentContextMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(markdown, "靠海") {
		t.Fatalf("alpha resident context dropped its own entry: %q", markdown)
	}
	if strings.Contains(markdown, "在内陆") || strings.Contains(markdown, "只有贝塔世界知道的事") {
		t.Fatalf("alpha resident context leaked another world's lore: %q", markdown)
	}

	bytes, err := alpha.ResidentContentBytes()
	if err != nil {
		t.Fatal(err)
	}
	if bytes != len([]byte("阿尔法世界的青石镇，靠海")) {
		t.Fatalf("alpha resident byte budget counted %d, want only its own entry", bytes)
	}

	// The editor is unbound and must still see everything, otherwise scoping
	// would have hidden the user's own library from them.
	editorMarkdown, err := seeder.ResidentContextMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"靠海", "在内陆", "只有贝塔世界知道的事"} {
		if !strings.Contains(editorMarkdown, want) {
			t.Fatalf("the unbound editor lost %q from its resident context", want)
		}
	}
}

// TestScopedStoreCannotWriteOutsideItsWorld covers the write side. Reading is
// filtered, but a bound writer that could still edit a sibling world's entry or
// the user's shared book would corrupt state the user never handed it.
func TestScopedStoreCannotWriteOutsideItsWorld(t *testing.T) {
	workspace := t.TempDir()
	seeder := NewStore(workspace)

	seedItem(t, seeder, ScopeShared, "shared-1", "通用规则", "通用设定", LoadModeResident)
	seedItem(t, seeder, "pack:alpha", "alpha-town", "青石镇", "阿尔法世界的青石镇", LoadModeResident)
	seedItem(t, seeder, "pack:beta", "beta-town", "贝塔镇", "贝塔世界的贝塔镇", LoadModeResident)

	alpha := NewScopedStore(workspace, "pack:alpha")

	if _, err := alpha.Update("beta-town", ItemInput{Type: "setting", Name: "贝塔镇", Content: "被篡改"}); err == nil {
		t.Fatal("a bound store rewrote another world's entry")
	}
	if _, err := alpha.Update("shared-1", ItemInput{Type: "setting", Name: "通用规则", Content: "被篡改"}); err == nil {
		t.Fatal("a bound store rewrote the user's shared entry")
	}
	if err := alpha.Delete("beta-town"); err == nil {
		t.Fatal("a bound store deleted another world's entry")
	}
	if err := alpha.Delete("shared-1"); err == nil {
		t.Fatal("a bound store deleted the user's shared entry")
	}

	// Its own entry stays writable, otherwise the story's own book could never
	// be maintained.
	if _, err := alpha.Update("alpha-town", ItemInput{Type: "setting", Name: "青石镇", Content: "阿尔法世界的青石镇，靠海", LoadMode: LoadModeResident}); err != nil {
		t.Fatalf("a bound store could not write its own entry: %v", err)
	}

	// A write carrying a foreign scope is re-homed into the store's own scope
	// rather than landing in the world the caller named.
	created, err := alpha.Create(ItemInput{Scope: "pack:beta", Type: "setting", Name: "走私条目", Content: "试图写进别的世界"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Scope != "pack:alpha" {
		t.Fatalf("a bound store wrote into %q, want its own scope", created.Scope)
	}
	beta := NewScopedStore(workspace, "pack:beta")
	betaItems, err := beta.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range itemIDs(betaItems) {
		if id == created.ID {
			t.Fatal("a bound store leaked an entry into another world")
		}
	}
}

// TestWorldEntryShadowsASharedEntryOfTheSameName covers the one case where a
// name legitimately exists twice in a bound store's view. A name resolves to one
// fact, so the world's version must win regardless of which was written first,
// and the shared entry must still be intact for every other reader.
func TestWorldEntryShadowsASharedEntryOfTheSameName(t *testing.T) {
	workspace := t.TempDir()
	seeder := NewStore(workspace)

	// Written first, so a storage-order tiebreak would pick the shared entry.
	seedItem(t, seeder, ScopeShared, "shared-town", "青石镇", "用户自己的青石镇", LoadModeResident)
	seedItem(t, seeder, "pack:alpha", "alpha-town", "青石镇", "阿尔法世界的青石镇", LoadModeResident)

	alpha := NewScopedStore(workspace, "pack:alpha")
	items, err := alpha.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "alpha-town" {
		t.Fatalf("alpha saw %v, want only its own 青石镇", itemIDs(items))
	}

	markdown, err := alpha.ResidentContextMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(markdown, "用户自己的青石镇") {
		t.Fatalf("the shared entry shadowed the world's own: %q", markdown)
	}

	// A different world still sees the shared entry: shadowing is per-world.
	beta := NewScopedStore(workspace, "pack:beta")
	betaItems, err := beta.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(betaItems) != 1 || betaItems[0].ID != "shared-town" {
		t.Fatalf("beta saw %v, want the shared 青石镇", itemIDs(betaItems))
	}

	// The editor sees both, because both really exist.
	editorItems, err := seeder.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(editorItems) != 2 {
		t.Fatalf("the editor saw %v, want both entries", itemIDs(editorItems))
	}

	// Shadowing hides an entry from a world's view; it must never make that
	// entry unwritable or unreadable by identity for the user.
	if _, err := seeder.Update("shared-town", ItemInput{Type: "setting", Name: "青石镇", Content: "用户自己的青石镇（已改）", LoadMode: LoadModeResident}); err != nil {
		t.Fatalf("the shadowed shared entry became unwritable: %v", err)
	}
}

// TestPacklessStorySeesOnlyTheSharedBook pins the boundary between the two
// sentinels: a story with no pack binding is bound to the shared scope and must
// not inherit every pack's world book, while the unbound editor still sees all.
func TestPacklessStorySeesOnlyTheSharedBook(t *testing.T) {
	workspace := t.TempDir()
	seeder := NewStore(workspace)

	seedItem(t, seeder, ScopeShared, "shared-1", "通用规则", "通用设定", LoadModeResident)
	seedItem(t, seeder, "pack:alpha", "alpha-town", "青石镇", "阿尔法世界的青石镇", LoadModeResident)

	manual := NewScopedStore(workspace, "")
	if !manual.Bound() {
		t.Fatal("a store bound to the shared scope must still be bound")
	}
	items, err := manual.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "shared-1" {
		t.Fatalf("a packless story saw %v, want only the shared book", itemIDs(items))
	}

	editor, err := seeder.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(editor) != 2 {
		t.Fatalf("the unbound editor saw %v, want the whole library", itemIDs(editor))
	}
}

// TestScopedNameUniquenessIsPerScope documents the write rule the pack seeder
// relies on: two worlds may both define 青石镇, but one world may not define it
// twice, and a world's entry does not collide with the shared one.
func TestScopedNameUniquenessIsPerScope(t *testing.T) {
	workspace := t.TempDir()
	seeder := NewStore(workspace)

	seedItem(t, seeder, ScopeShared, "shared-1", "青石镇", "用户自己的青石镇", LoadModeAuto)
	seedItem(t, seeder, "pack:alpha", "alpha-town", "青石镇", "阿尔法世界的青石镇", LoadModeAuto)

	if _, err := seeder.Create(ItemInput{Scope: "pack:alpha", Type: "setting", Name: "青石镇", Content: "重复"}); err == nil {
		t.Fatal("a second entry with the same name in one scope was accepted")
	}
	if _, err := seeder.Create(ItemInput{Scope: "pack:beta", Type: "setting", Name: "青石镇", Content: "贝塔世界的青石镇"}); err != nil {
		t.Fatalf("a second world could not define a same-named entry: %v", err)
	}
}
