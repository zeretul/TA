package storypack

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 收录到世界包 is the one write the lore library's 本局新建 page performs: a
// story's own entity becomes part of the world it was played in. These tests pin
// the behaviour the assistant depends on — a retried collection does not leave a
// duplicate behind, a same-name stranger is refused rather than merged, an entry
// edited by hand is updated in place, and a world that ships with the build is
// never shadowed by a user file.

func collectTestLibrary(t *testing.T) (*Library, string) {
	t.Helper()
	dataRoot := t.TempDir()
	library := NewLibrary(dataRoot)
	saved, err := library.SaveWorld(context.Background(), draftTestWorld())
	if err != nil {
		t.Fatalf("building the world to collect into must succeed: %v", err)
	}
	return library, saved.PackID
}

func collectDraft() WorldEntryDraft {
	return WorldEntryDraft{
		Name:             "沈舟",
		Type:             "character",
		Importance:       "high",
		Tags:             []string{"渡口", "摆渡"},
		Keywords:         []string{"船", "旧木箱"},
		LoadMode:         "auto",
		BriefDescription: "在渡口撑了三十年船的人。",
		Content:          "沈舟从十四岁起就在这条河上撑船，认得每一个过河的人，也记得谁欠过谁一趟船钱。",
		SourceRef:        "st_abc/沈舟",
	}
}

// A new entry lands in the world book and reads back through the same library
// the world page uses, with every field it carried.
func TestCollectWorldEntryAppendsAndReadsBack(t *testing.T) {
	library, packID := collectTestLibrary(t)
	before, err := library.WorldBook(packID)
	if err != nil {
		t.Fatal(err)
	}
	if !before.Writable {
		t.Fatal("a world saved by the player must be writable")
	}

	versionBefore := packContentVersion(t, library, packID)

	result, err := library.CollectWorldEntry(context.Background(), packID, collectDraft())
	if err != nil {
		t.Fatalf("collecting into a user world must succeed: %v", err)
	}
	if !result.Created {
		t.Fatal("a new entry must be reported as created")
	}
	if result.EntryID == "" {
		t.Fatal("the collection did not report the entry id it wrote")
	}
	if result.Entry.SourceRef != "st_abc/沈舟" {
		t.Fatalf("the entry lost its source reference: %q", result.Entry.SourceRef)
	}
	if result.ContentVersion != versionBefore+1 {
		t.Fatalf("content version = %d, want %d", result.ContentVersion, versionBefore+1)
	}

	after, err := library.WorldBook(packID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Entries) != len(before.Entries)+1 {
		t.Fatalf("world book has %d entries, want %d", len(after.Entries), len(before.Entries)+1)
	}
	var found *WorldEntry
	for index := range after.Entries {
		if after.Entries[index].ID == result.EntryID {
			found = &after.Entries[index]
		}
	}
	if found == nil {
		t.Fatalf("the collected entry %s is not in the world book", result.EntryID)
	}
	if found.Name != "沈舟" || found.Type != "character" || found.LoadMode != "auto" {
		t.Fatalf("the entry lost its shape: %+v", *found)
	}
	if len(found.Tags) != 2 || len(found.Keywords) != 2 {
		t.Fatalf("the entry lost its tags or keywords: %+v", *found)
	}
	if !strings.Contains(found.Content, "撑船") {
		t.Fatalf("the entry lost its body: %q", found.Content)
	}
}

// packContentVersion reads the world's current content revision off its file.
// The book snapshot carries entries only, and the counter is what a new game
// binds its lore scope to, so the tests read it where the page would.
func packContentVersion(t *testing.T, library *Library, packID string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(presetStoryPacksDir(library.dataRoot), packID+fileExtension))
	if err != nil {
		t.Fatalf("reading the world file: %v", err)
	}
	var pack Pack
	if err := json.Unmarshal(raw, &pack); err != nil {
		t.Fatalf("the world file is not valid JSON: %v", err)
	}
	return pack.ContentVersion
}

// A retried collection of the same story entity is the same entry even when the
// model reworded the title. This is what makes a failed collection safe to
// repeat.
func TestCollectWorldEntryIsIdempotentBySourceRef(t *testing.T) {
	library, packID := collectTestLibrary(t)
	ctx := context.Background()

	first, err := library.CollectWorldEntry(ctx, packID, collectDraft())
	if err != nil {
		t.Fatal(err)
	}

	retried := collectDraft()
	retried.Name = "摆渡人沈舟"
	retried.Content = "沈舟在这条河上撑了三十年船。"
	second, err := library.CollectWorldEntry(ctx, packID, retried)
	if err != nil {
		t.Fatalf("a retried collection must not fail: %v", err)
	}
	if second.Created {
		t.Fatal("a retried collection reported a new entry")
	}
	if second.EntryID != first.EntryID {
		t.Fatalf("retry wrote %s, want the existing %s", second.EntryID, first.EntryID)
	}

	book, err := library.WorldBook(packID)
	if err != nil {
		t.Fatal(err)
	}
	sameSource := 0
	for _, entry := range book.Entries {
		if entry.SourceRef == "st_abc/沈舟" {
			sameSource++
			if entry.Name != "摆渡人沈舟" {
				t.Fatalf("the retry did not apply its wording: %q", entry.Name)
			}
			if len(entry.Tags) != 2 {
				t.Fatalf("the retry dropped the entry's tags: %+v", entry.Tags)
			}
		}
	}
	if sameSource != 1 {
		t.Fatalf("the world book holds %d entries from one source, want 1", sameSource)
	}
}

// An entry the author edited by hand is updated in place when a collection names
// its id, so a collection does not fork a second copy beside the edited one.
func TestCollectWorldEntryUpdatesByID(t *testing.T) {
	library, packID := collectTestLibrary(t)
	ctx := context.Background()

	first, err := library.CollectWorldEntry(ctx, packID, collectDraft())
	if err != nil {
		t.Fatal(err)
	}

	draft := collectDraft()
	draft.ID = first.EntryID
	draft.SourceRef = ""
	draft.Content = "沈舟如今不再撑船，改在渡口替人看箱子。"
	second, err := library.CollectWorldEntry(ctx, packID, draft)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created || second.EntryID != first.EntryID {
		t.Fatalf("update by id created %v / %s, want the existing %s", second.Created, second.EntryID, first.EntryID)
	}
	if !strings.Contains(second.Entry.Content, "看箱子") {
		t.Fatalf("the update did not reach the body: %q", second.Entry.Content)
	}
}

// A draft that leaves a field empty keeps what the entry already had. A
// collection that only rewrites the body must not silently clear the tags or the
// load mode the author set.
func TestCollectWorldEntryKeepsFieldsTheDraftLeavesEmpty(t *testing.T) {
	library, packID := collectTestLibrary(t)
	ctx := context.Background()

	first, err := library.CollectWorldEntry(ctx, packID, collectDraft())
	if err != nil {
		t.Fatal(err)
	}

	sparse := WorldEntryDraft{
		SourceRef: "st_abc/沈舟",
		Name:      "沈舟",
		Content:   "沈舟把船拴在石阶上，等着下一班过河的人。",
	}
	second, err := library.CollectWorldEntry(ctx, packID, sparse)
	if err != nil {
		t.Fatal(err)
	}
	if second.EntryID != first.EntryID {
		t.Fatalf("the sparse draft wrote a new entry %s", second.EntryID)
	}
	if second.Entry.Type != "character" || second.Entry.LoadMode != "auto" {
		t.Fatalf("the sparse draft cleared fields it did not name: %+v", second.Entry)
	}
	if len(second.Entry.Tags) != 2 || second.Entry.BriefDescription == "" {
		t.Fatalf("the sparse draft cleared tags or the summary: %+v", second.Entry)
	}
	if !strings.Contains(second.Entry.Content, "石阶") {
		t.Fatalf("the sparse draft did not apply its body: %q", second.Entry.Content)
	}
}

// A second story's person with the same name is a different person. Merging them
// would make every later lookup ambiguous, so the collection is refused and says
// which entry already holds the name.
func TestCollectWorldEntryRefusesSameNameFromAnotherStory(t *testing.T) {
	library, packID := collectTestLibrary(t)
	ctx := context.Background()

	first, err := library.CollectWorldEntry(ctx, packID, collectDraft())
	if err != nil {
		t.Fatal(err)
	}

	other := collectDraft()
	other.SourceRef = "st_other/沈舟"
	other.Content = "另一个世界里也有一个叫沈舟的人，是个货郎。"
	if _, err := library.CollectWorldEntry(ctx, packID, other); err == nil {
		t.Fatal("a same-name entry from another story must be refused")
	} else if !strings.Contains(err.Error(), first.EntryID) {
		t.Fatalf("the refusal does not name the existing entry: %v", err)
	}

	book, err := library.WorldBook(packID)
	if err != nil {
		t.Fatal(err)
	}
	named := 0
	for _, entry := range book.Entries {
		if entry.Name == "沈舟" {
			named++
		}
	}
	if named != 1 {
		t.Fatalf("the world book holds %d entries named 沈舟, want 1", named)
	}
}

// Recollecting an entry exactly as it already stands writes nothing and does not
// move the content revision: a no-op must not send every new game of this world
// to a fresh lore scope.
func TestCollectWorldEntryNoOpDoesNotMoveTheRevision(t *testing.T) {
	library, packID := collectTestLibrary(t)
	ctx := context.Background()

	first, err := library.CollectWorldEntry(ctx, packID, collectDraft())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(presetStoryPacksDir(library.dataRoot), packID+fileExtension)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	second, err := library.CollectWorldEntry(ctx, packID, collectDraft())
	if err != nil {
		t.Fatal(err)
	}
	if second.Created || second.EntryID != first.EntryID {
		t.Fatalf("the repeat created %v / %s", second.Created, second.EntryID)
	}
	if second.ContentVersion != first.ContentVersion {
		t.Fatalf("a no-op moved the content version from %d to %d", first.ContentVersion, second.ContentVersion)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("a no-op collection rewrote the world file")
	}
}

// The pack file stays a valid pack: collecting one entry must not break the
// module bindings or the catalog the world was saved with.
func TestCollectWorldEntryKeepsTheRestOfThePackIntact(t *testing.T) {
	library, packID := collectTestLibrary(t)
	before, err := library.Get(packID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.CollectWorldEntry(context.Background(), packID, collectDraft()); err != nil {
		t.Fatal(err)
	}
	after, err := library.Get(packID)
	if err != nil {
		t.Fatalf("the world no longer loads after a collection: %v", err)
	}
	if after.Name != before.Name || after.Story.Title != before.Story.Title {
		t.Fatalf("the collection changed the world's identity: %q / %q", after.Name, after.Story.Title)
	}
	if len(after.Characters) != len(before.Characters) {
		t.Fatalf("the collection changed the catalog: %d rows, want %d", len(after.Characters), len(before.Characters))
	}
	if after.Story.ModuleRefs.RuleSystemID != before.Story.ModuleRefs.RuleSystemID ||
		after.Story.ModuleRefs.ActorStateID != before.Story.ModuleRefs.ActorStateID ||
		after.Story.ModuleRefs.ImagePresetID != before.Story.ModuleRefs.ImagePresetID ||
		len(after.Story.ModuleRefs.EventPackageIDs) != len(before.Story.ModuleRefs.EventPackageIDs) {
		t.Fatalf("the collection changed the module bindings: %+v", after.Story.ModuleRefs)
	}
	if after.RecommendedPlanningTemplateID != before.RecommendedPlanningTemplateID {
		t.Fatal("the collection changed the world's planning recommendation")
	}
}

// A world that ships with the build has no file under the data root, so a write
// would shadow the delivered asset for every story started from it. That is
// refused by name rather than attempted.
func TestCollectWorldEntryRefusesABuiltinWorld(t *testing.T) {
	library := NewLibrary(t.TempDir())

	book, err := library.WorldBook(builtinPackIDFanren)
	if err != nil {
		t.Fatalf("a builtin world must still be readable: %v", err)
	}
	if book.Writable {
		t.Fatal("a builtin world must not report itself writable")
	}
	if len(book.Entries) == 0 {
		t.Fatal("a builtin world must hand over its entries")
	}

	_, err = library.CollectWorldEntry(context.Background(), builtinPackIDFanren, collectDraft())
	if !IsBuiltinPackReadOnly(err) {
		t.Fatalf("collecting into a builtin world returned %v, want the builtin refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(presetStoryPacksDir(library.dataRoot), builtinPackIDFanren+fileExtension)); statErr == nil {
		t.Fatal("the refused collection left a user file shadowing the builtin world")
	}
}

// A world id that names nothing at all is a different failure from a builtin:
// the assistant has to tell the model to pick a real world rather than to save a
// copy of one.
func TestCollectWorldEntryReportsAnUnknownWorld(t *testing.T) {
	library := NewLibrary(t.TempDir())
	_, err := library.CollectWorldEntry(context.Background(), "no-such-world", collectDraft())
	if err == nil {
		t.Fatal("collecting into an unknown world must fail")
	}
	if IsBuiltinPackReadOnly(err) {
		t.Fatalf("an unknown world was reported as a builtin: %v", err)
	}
	if !strings.Contains(err.Error(), "剧本包不存在") {
		t.Fatalf("the refusal does not say the world is missing: %v", err)
	}
}

// A draft with no name or no body is refused before anything is written, because
// an unnamed or empty entry would be a row the page cannot show.
func TestCollectWorldEntryRequiresNameAndBody(t *testing.T) {
	library, packID := collectTestLibrary(t)
	ctx := context.Background()

	nameless := collectDraft()
	nameless.Name = "   "
	if _, err := library.CollectWorldEntry(ctx, packID, nameless); err == nil {
		t.Fatal("a draft without a name must be refused")
	}

	bodyless := collectDraft()
	bodyless.Content = " "
	if _, err := library.CollectWorldEntry(ctx, packID, bodyless); err == nil {
		t.Fatal("a draft without a body must be refused")
	}

	book, err := library.WorldBook(packID)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range book.Entries {
		if entry.SourceRef == "st_abc/沈舟" {
			t.Fatal("a refused draft still reached the world book")
		}
	}
}

// The derived id keeps the name's own letters so an author reading the file can
// tell the entries apart, and a second entry with the same derived id becomes a
// separate row rather than an overwrite.
func TestUniqueWorldEntryIDKeepsTheNameAndSuffixesClashes(t *testing.T) {
	entries := []WorldEntry{{ID: "沈舟"}, {ID: "沈舟-2"}}
	if got := uniqueWorldEntryID(entries, "沈舟"); got != "沈舟-3" {
		t.Fatalf("derived id = %q, want 沈舟-3", got)
	}
	if got := uniqueWorldEntryID(nil, "青石渡口"); got != "青石渡口" {
		t.Fatalf("derived id = %q, want the name itself", got)
	}
	if got := uniqueWorldEntryID(nil, "  "); got != "entry" {
		t.Fatalf("a nameless entry derived %q, want entry", got)
	}
}

// The collected entry reaches the pack file as the world page reads it, not only
// as the in-memory result. This is the "保存并读回" half of the collection.
func TestCollectedEntryIsInThePackFileItself(t *testing.T) {
	library, packID := collectTestLibrary(t)
	result, err := library.CollectWorldEntry(context.Background(), packID, collectDraft())
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(presetStoryPacksDir(library.dataRoot), packID+fileExtension))
	if err != nil {
		t.Fatal(err)
	}
	var pack Pack
	if err := json.Unmarshal(raw, &pack); err != nil {
		t.Fatalf("the written pack is not valid JSON: %v", err)
	}
	var found *WorldEntry
	for index := range pack.World {
		if pack.World[index].ID == result.EntryID {
			found = &pack.World[index]
		}
	}
	if found == nil {
		t.Fatalf("the entry %s is not in the pack file", result.EntryID)
	}
	if found.SourceRef != "st_abc/沈舟" {
		t.Fatalf("the pack file lost the source reference: %q", found.SourceRef)
	}
	if pack.ContentVersion != result.ContentVersion {
		t.Fatalf("pack content_version = %d, want the reported %d", pack.ContentVersion, result.ContentVersion)
	}
}
