package storypack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	booklore "ta/internal/book/lore"
	"ta/internal/interactive"
	"ta/internal/presetlayout"
)

// TestTwoPacksWorldBooksCoexistInOneWorkspace is the end-to-end version of the
// isolation rule: both builtin packs materialize into one workspace, each keeps
// its own world book, and neither story can read the other's.
func TestTwoPacksWorldBooksCoexistInOneWorkspace(t *testing.T) {
	workspace := t.TempDir()
	seeder := booklore.NewStore(workspace)

	packs := builtinPacks()
	for _, pack := range packs {
		seedPack(t, seeder, pack)
	}

	// Every pack's entries carry that pack's scope, so the workspace holds two
	// disjoint world books rather than one merged book.
	all, err := seeder.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, pack := range packs {
		total += len(pack.World)
	}
	if len(all) != total {
		t.Fatalf("workspace holds %d entries, want %d", len(all), total)
	}
	scopes := map[string]bool{}
	for _, item := range all {
		scopes[item.Scope] = true
	}
	if len(scopes) != len(packs) {
		t.Fatalf("entries landed in %d scopes, want one per pack: %v", len(scopes), scopes)
	}

	// Each pack's story context reads only its own world book. The shared book
	// is empty here, so a pack's view is exactly its own entries.
	for _, pack := range packs {
		store := booklore.NewScopedStore(workspace, pack.WorldScope())
		visible, err := store.List()
		if err != nil {
			t.Fatal(err)
		}
		if len(visible) != len(pack.World) {
			t.Fatalf("pack %s sees %d entries, want its own %d", pack.ID, len(visible), len(pack.World))
		}
		own := map[string]bool{}
		for _, input := range pack.WorldInputs() {
			own[input.ID] = true
		}
		for _, item := range visible {
			if !own[item.ID] {
				t.Fatalf("pack %s saw foreign entry %s", pack.ID, item.ID)
			}
		}
	}
}

// TestApplyingTwoPacksStaysIdempotent re-applies both packs. Seeding must not
// duplicate entries, and the second pack must still land even though the first
// already wrote a book into the same workspace.
func TestApplyingTwoPacksStaysIdempotent(t *testing.T) {
	workspace := t.TempDir()
	seeder := booklore.NewStore(workspace)
	packs := builtinPacks()

	seed := func() {
		t.Helper()
		for _, pack := range packs {
			seedPack(t, seeder, pack)
		}
	}
	seed()
	seed()

	all, err := seeder.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, pack := range packs {
		total += len(pack.World)
	}
	if len(all) != total {
		t.Fatalf("re-applying duplicated entries: %d present, want %d", len(all), total)
	}
}

// TestPackBindingIsFrozenAndDerivedFromPackID pins the properties the binding
// exists for: it is keyed by pack identity and content revision rather than by
// story, so two stories of one pack revision share a world book, two packs never
// collide on a scope, and two revisions of one pack do not either.
func TestPackBindingIsFrozenAndDerivedFromPackID(t *testing.T) {
	packs := builtinPacks()
	for _, pack := range packs {
		binding := pack.StoryRequest().PackBinding
		if binding == nil {
			t.Fatalf("pack %s produced no binding", pack.ID)
		}
		if binding.PackID != pack.ID {
			t.Fatalf("binding pack id = %q, want %q", binding.PackID, pack.ID)
		}
		if binding.Scope != pack.WorldScope() {
			t.Fatalf("binding scope = %q, want the pack's own world scope %q", binding.Scope, pack.WorldScope())
		}
		if binding.ContentHash == "" {
			t.Fatalf("pack %s produced no content fingerprint", pack.ID)
		}
	}

	seen := map[string]string{}
	for _, pack := range packs {
		scope := pack.WorldScope()
		if other, ok := seen[scope]; ok {
			t.Fatalf("packs %s and %s share scope %q", other, pack.ID, scope)
		}
		seen[scope] = pack.ID
	}
}

// TestPackContentHashTracksContentNotPresentation separates the two reasons a
// pack can change: rewriting the world must be detectable, renaming the pack
// must not invalidate a running story.
func TestPackContentHashTracksContentNotPresentation(t *testing.T) {
	base := builtinPacks()[0]
	renamed := base
	renamed.Name = base.Name + "（改名）"
	renamed.Description = base.Description + " 说明"
	if base.ContentHash() != renamed.ContentHash() {
		t.Fatal("renaming a pack changed its content fingerprint")
	}

	rewritten := base
	rewritten.World = append([]WorldEntry(nil), base.World...)
	rewritten.World[0].Content = base.World[0].Content + " 新增设定"
	if base.ContentHash() == rewritten.ContentHash() {
		t.Fatal("editing the world book did not change the content fingerprint")
	}
}

// TestStoryCreatedFromPackKeepsItsScope proves the binding survives the real
// creation path and comes back out of the store, which is what makes an
// existing story keep reading its own world after a restart.
func TestStoryCreatedFromPackKeepsItsScope(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	pack := builtinPacks()[0]
	request := pack.StoryRequest()

	created, err := store.CreateStory(request)
	if err != nil {
		t.Fatalf("CreateStory from pack %s: %v", pack.ID, err)
	}
	if created.PackBinding == nil {
		t.Fatal("the created story lost its pack binding")
	}
	if created.PackBinding.Scope != pack.WorldScope() {
		t.Fatalf("stored scope = %q, want %q", created.PackBinding.Scope, pack.WorldScope())
	}

	// Re-reading from disk must reproduce the binding, not merely the in-memory
	// copy the create call returned.
	reopened, err := interactive.NewStore(store.Root()).StoryContext(created.ID, "")
	if err != nil {
		t.Fatalf("reopening the story: %v", err)
	}
	if reopened.Meta.PackBinding == nil {
		t.Fatal("the binding did not survive the journal round trip")
	}
	if got := reopened.Meta.LoreScope(); got != pack.WorldScope() {
		t.Fatalf("reopened lore scope = %q, want the pack world scope", got)
	}
}

// TestEditedPackServesNewContentToNewGamesAndOldContentToOldGames is the
// regression the revision-qualified scope exists for.
//
// A pack is edited in place under the same ID: one world entry is rewritten and
// another is added. Because a pack's world book is written once at story start
// and entries whose names already exist are left untouched, an unqualified scope
// would let the old revision's names shadow the new content, so a brand new game
// would still read the old world. The revision-qualified scope must give the new
// revision its own book.
func TestEditedPackServesNewContentToNewGamesAndOldContentToOldGames(t *testing.T) {
	workspace := t.TempDir()
	store := booklore.NewStore(workspace)

	original := builtinPacks()[0]
	seedPack(t, store, original)

	// A story started before the edit.
	oldStory, err := interactive.NewStore(workspace).CreateStory(original.StoryRequest())
	if err != nil {
		t.Fatalf("creating the pre-edit story: %v", err)
	}

	// The author edits the same pack ID: rewrite one entry, add another.
	edited := original
	edited.World = append([]WorldEntry(nil), original.World...)
	edited.World[0].Content = original.World[0].Content + "（修订后的事实）"
	edited.World = append(edited.World, WorldEntry{
		ID: "fanren-jie-new-entry", Name: "修订新增地点", Type: "location", Content: "只有新版本才有的地点。",
	})
	if edited.ContentHash() == original.ContentHash() {
		t.Fatal("the edit did not change the pack content fingerprint")
	}
	seedPack(t, store, edited)

	newStory, err := interactive.NewStore(workspace).CreateStory(edited.StoryRequest())
	if err != nil {
		t.Fatalf("creating the post-edit story: %v", err)
	}

	if oldStory.PackBinding.Scope == newStory.PackBinding.Scope {
		t.Fatalf("both revisions share scope %q, so the new game cannot read new content", oldStory.PackBinding.Scope)
	}

	// The old game still reads the original text and must not see the entry the
	// edit added.
	oldItems := readScopedItems(t, workspace, oldStory.PackBinding.Scope)
	if got := itemContent(oldItems, original.World[0].Name); !strings.Contains(got, original.World[0].Content) || strings.Contains(got, "修订后的事实") {
		t.Fatalf("old story read content %q, want the pre-edit text", got)
	}
	if itemContent(oldItems, "修订新增地点") != "" {
		t.Fatal("the old story saw an entry that only the edited pack added")
	}

	// The new game reads the revised text and sees the added entry.
	newItems := readScopedItems(t, workspace, newStory.PackBinding.Scope)
	if got := itemContent(newItems, edited.World[0].Name); !strings.Contains(got, "修订后的事实") {
		t.Fatalf("new story read content %q, want the revised text", got)
	}
	if itemContent(newItems, "修订新增地点") == "" {
		t.Fatal("the new story did not see the entry the edit added")
	}
}

// TestReapplyingTheSamePackRevisionReusesItsWorldBook pins the sharing half of
// the contract: two stories of one pack revision read one book, and re-applying
// that revision neither duplicates entries nor overwrites later user edits.
func TestReapplyingTheSamePackRevisionReusesItsWorldBook(t *testing.T) {
	workspace := t.TempDir()
	store := booklore.NewStore(workspace)
	pack := builtinPacks()[0]

	seedPack(t, store, pack)
	first, err := interactive.NewStore(workspace).CreateStory(pack.StoryRequest())
	if err != nil {
		t.Fatal(err)
	}

	// A user corrects an entry the pack wrote. Re-applying the pack must not
	// overwrite that correction.
	items := readScopedItems(t, workspace, first.PackBinding.Scope)
	target := items[0]
	edited := target.Content + "（用户补充）"
	if _, err := store.Update(target.ID, booklore.ItemInput{Name: target.Name, Content: edited}); err != nil {
		t.Fatalf("editing a pack entry: %v", err)
	}
	seedPack(t, store, pack)

	second, err := interactive.NewStore(workspace).CreateStory(pack.StoryRequest())
	if err != nil {
		t.Fatal(err)
	}
	if second.PackBinding.Scope != first.PackBinding.Scope {
		t.Fatalf("same revision produced scopes %q and %q", first.PackBinding.Scope, second.PackBinding.Scope)
	}
	after := readScopedItems(t, workspace, first.PackBinding.Scope)
	if len(after) != len(items) {
		t.Fatalf("re-applying the same revision changed the entry count: %d, want %d", len(after), len(items))
	}
	if got := itemContent(after, target.Name); !strings.Contains(got, "用户补充") {
		t.Fatalf("re-applying the pack overwrote the user's edit: %q", got)
	}
}

// TestStoryKeepsPlayingAfterItsSourcePackIsRemoved is the moved-source-pack
// clause of the version-freeze contract.
//
// A story binds to a pack revision by ID and content fingerprint, and its world
// book is already written into the workspace. The pack file itself is only an
// input to story creation, so moving or deleting it must leave a game in
// progress untouched: the binding is read from the story's own journal, not
// re-derived from a file that may no longer be there. What must change is only
// what happens to a *new* game, which can no longer start from a pack that is
// gone.
func TestStoryKeepsPlayingAfterItsSourcePackIsRemoved(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	writePackFile(t, dataRoot, "portable-world.json", externalPackDocument("portable-world", "可移走的世界"))

	pack, err := NewLibrary(dataRoot).Get("portable-world")
	if err != nil {
		t.Fatalf("reading the pack: %v", err)
	}
	store := booklore.NewStore(workspace)
	seedPack(t, store, pack)

	story, err := interactive.NewStore(workspace).CreateStory(pack.StoryRequest())
	if err != nil {
		t.Fatalf("starting the story: %v", err)
	}
	scope := story.PackBinding.Scope
	before := readScopedItems(t, workspace, scope)
	if len(before) != len(pack.World) {
		t.Fatalf("story reads %d world entries, want %d", len(before), len(pack.World))
	}

	// The author moves the pack out of the data root entirely.
	packPath := filepath.Join(presetlayout.StoryPacks(dataRoot), "portable-world.json")
	if err := os.Remove(packPath); err != nil {
		t.Fatalf("removing the source pack: %v", err)
	}

	// The running story is unaffected: it re-reads the binding from its journal
	// and its world book from the workspace, neither of which lived in the pack.
	reopened, err := interactive.NewStore(workspace).StoryContext(story.ID, "")
	if err != nil {
		t.Fatalf("reopening the story after its pack was removed: %v", err)
	}
	if reopened.Meta.PackBinding == nil {
		t.Fatal("the story lost its binding when the source pack was removed")
	}
	if reopened.Meta.PackBinding.Scope != scope {
		t.Fatalf("binding scope drifted to %q after the pack was removed", reopened.Meta.PackBinding.Scope)
	}
	if reopened.Meta.PackBinding.PackName != pack.Name {
		t.Fatalf("binding name = %q, want the name recorded at binding time %q", reopened.Meta.PackBinding.PackName, pack.Name)
	}
	after := readScopedItems(t, workspace, scope)
	if len(after) != len(before) {
		t.Fatalf("the story's world book changed from %d to %d entries when the pack was removed", len(before), len(after))
	}

	// A new game, by contrast, cannot start: the pack is genuinely gone and must
	// be reported rather than silently starting on defaults.
	if _, err := NewLibrary(dataRoot).Get("portable-world"); err == nil {
		t.Fatal("a removed pack was still readable")
	}
}

// TestIncompatibleRuleSystemIsReportedNotSilentlySubstituted covers the last
// clause: a pack that binds a rule module whose content no longer validates.
//
// The module exists under the expected ID, so a naive "does the ID resolve"
// check passes and the story would start on whatever the reader could salvage.
// A pack must instead be refused, because a world whose mechanics silently
// changed under it is a different game than its author shipped.
func TestIncompatibleRuleSystemIsReportedNotSilentlySubstituted(t *testing.T) {
	dataRoot := t.TempDir()
	// A rule system whose state binding names no actor-state system is rejected
	// by the module validator, which is exactly the "present but incompatible"
	// case: the file is there, the ID resolves, the content does not load.
	dir := presetlayout.RuleSystems(dataRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	document := `{
  "version": 1,
  "id": "broken-rules",
  "name": "不兼容的检定",
  "trpg_system": {
    "rule_templates": [
      {"id": "broken", "label": "坏检定", "dice": "1d20",
       "state_bindings": [{"field": "hp", "delta": -1}]}
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "broken-rules.json"), []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}

	packDoc := strings.Replace(
		externalPackDocument("incompatible-world", "规则不兼容的世界"),
		`"rule_system_id": "default"`,
		`"rule_system_id": "broken-rules"`,
		1,
	)
	writePackFile(t, dataRoot, "incompatible-world.json", packDoc)

	library := NewLibrary(dataRoot)
	if _, err := library.Get("incompatible-world"); err == nil {
		t.Fatal("a pack binding an incompatible rule system loaded successfully")
	} else if !strings.Contains(err.Error(), "broken-rules") {
		t.Fatalf("the error does not name the incompatible module: %v", err)
	}

	summaries, err := library.List()
	if err != nil {
		t.Fatal(err)
	}
	var broken *Summary
	for i := range summaries {
		if summaries[i].ID == "incompatible-world" {
			broken = &summaries[i]
		}
	}
	if broken == nil || !broken.Invalid {
		t.Fatalf("a pack with an incompatible rule system was offered as playable: %+v", broken)
	}
}

// seedPack uses the real store transaction used by the application seed path.
// The storypack package cannot import loreapp because its host dependencies
// already depend on storypack. Service behavior has separate loreapp tests.
func seedPack(t *testing.T, store *booklore.Store, pack Pack) {
	t.Helper()
	existing, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]bool, len(existing))
	for _, item := range existing {
		ids[item.ID] = true
	}
	ops := make([]booklore.Operation, 0, len(pack.World))
	for _, input := range pack.WorldInputs() {
		if !ids[input.ID] {
			ops = append(ops, booklore.Operation{Op: "create", Item: input})
			ids[input.ID] = true
		}
	}
	if len(ops) == 0 {
		return
	}
	if _, err := store.ApplyOperations("Seed complete builtin world", ops); err != nil {
		t.Fatalf("seeding pack %s: %v", pack.ID, err)
	}
}

func readScopedItems(t *testing.T, workspace, scope string) []booklore.Item {
	t.Helper()
	items, err := booklore.NewScopedStore(workspace, scope).List()
	if err != nil {
		t.Fatalf("reading scope %q: %v", scope, err)
	}
	return items
}

func itemContent(items []booklore.Item, name string) string {
	for _, item := range items {
		if item.Name == name {
			return item.Content
		}
	}
	return ""
}

// TestManualStoryWithoutPackStaysOnTheSharedBook pins the packless path: a
// manually created story reads the user's own book and no pack's world book.
func TestManualStoryWithoutPackStaysOnTheSharedBook(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	created, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:         "手动开局",
		Origin:        "手动创建的故事",
		StoryTellerID: "rhythm",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.PackBinding != nil {
		t.Fatalf("a manual story acquired a pack binding: %+v", created.PackBinding)
	}
	reopened, err := interactive.NewStore(store.Root()).StoryContext(created.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if scope := reopened.Meta.LoreScope(); scope != "" {
		t.Fatalf("manual story lore scope = %q, want the shared scope", scope)
	}
}

func isDuplicateLoreError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "已存在")
}
