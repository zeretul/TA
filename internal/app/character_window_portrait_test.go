package app

import (
	"strings"
	"testing"

	booklore "ta/internal/book/lore"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/portrait"
)

// TestTheCharacterWindowGivesAPersonTheirShippedPortrait is F1 stated as the
// player sees it: a person the formal catalog authored shows their own picture
// without anything being generated or stored per story.
//
// The key is the author's source key, not the display name. Two worlds may each
// hold a 陈巧倩, so resolving by name would put one world's face on the other's
// person; a story-invented person has no key at all and keeps the placeholder.
func TestTheCharacterWindowGivesAPersonTheirShippedPortrait(t *testing.T) {
	library := portrait.NewLibrary()
	keys := library.Keys()
	if len(keys) == 0 {
		t.Fatal("the shipped set is empty")
	}
	authored := keys[0]

	state := character.State{Version: character.StateVersion}
	for _, profile := range []character.Profile{
		{ID: "authored", StoryID: "st", Name: "有来源的人", SourceKey: authored, Revision: 1},
		{ID: "invented", StoryID: "st", Name: "自造的人", Revision: 1},
	} {
		if err := state.UpsertProfile(profile); err != nil {
			t.Fatal(err)
		}
	}

	rows := characterWindowCast(state, character.Scene{}, pictureSources{portraits: library})
	byID := map[string]CharacterWindowCharacter{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	if got, want := byID["authored"].Avatar, library.URL(authored); got != want {
		t.Fatalf("the authored person's avatar is %q, want %q", got, want)
	}
	if !strings.HasPrefix(byID["authored"].Avatar, portrait.PathPrefix) {
		t.Fatalf("the avatar is not served by the portrait route: %q", byID["authored"].Avatar)
	}
	if got := byID["invented"].Avatar; got != "" {
		t.Fatalf("a person the story invented got avatar %q", got)
	}
	// The open conversation draws the same person larger, from the same key.
	if got, want := byID["authored"].AvatarDisplay, library.DisplayURL(authored); got != want {
		t.Fatalf("the authored person's display portrait is %q, want %q", got, want)
	}
	if got := byID["invented"].AvatarDisplay; got != "" {
		t.Fatalf("a person the story invented got display portrait %q", got)
	}
}

// TestAPlayerChosenAvatarOutranksTheShippedDefault is the precedence rule. A
// picture the player set (or imported from a character card) is the player's
// own choice; a shipped default must not quietly replace it on the next read.
func TestAPlayerChosenAvatarOutranksTheShippedDefault(t *testing.T) {
	library := portrait.NewLibrary()
	authored := library.Keys()[0]

	state := character.State{Version: character.StateVersion}
	if err := state.UpsertProfile(character.Profile{
		ID: "authored", StoryID: "st", Name: "有来源的人", SourceKey: authored,
		Avatar: "/api/projects/p/attachments/mine", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rows := characterWindowCast(state, character.Scene{}, pictureSources{portraits: library})
	if len(rows) != 1 {
		t.Fatalf("the cast lists %d rows", len(rows))
	}
	if got, want := rows[0].Avatar, "/api/projects/p/attachments/mine"; got != want {
		t.Fatalf("the chosen avatar was replaced: got %q, want %q", got, want)
	}
	// The larger picture is the shipped set's, and the player did not choose it,
	// so there is nothing honest to draw larger: the window keeps the chosen one.
	if got := rows[0].AvatarDisplay; got != "" {
		t.Fatalf("a chosen avatar got a shipped display portrait %q", got)
	}
}

// TestNoLibraryStillListsTheCast guards the degraded case: if the shipped set
// could not be loaded at all, the roster must still be usable rather than
// disappearing. The picture is decoration; the people are the feature.
func TestNoLibraryStillListsTheCast(t *testing.T) {
	state := character.State{Version: character.StateVersion}
	if err := state.UpsertProfile(character.Profile{
		ID: "authored", StoryID: "st", Name: "有来源的人", SourceKey: "canon_fairy_nichang", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rows := characterWindowCast(state, character.Scene{}, pictureSources{})
	if len(rows) != 1 || rows[0].Avatar != "" {
		t.Fatalf("rows = %+v", rows)
	}
}

// TestAWorldBookPictureOutranksTheShippedDefault is the design's取图 order: a
// picture the world book carries for this person's own entry is the world's
// choice, so it is drawn before the shipped default. It is reported as a
// project-relative path rather than a ready URL, because only the client knows
// which project it is reading.
func TestAWorldBookPictureOutranksTheShippedDefault(t *testing.T) {
	library := portrait.NewLibrary()
	authored := library.Keys()[0]

	state := character.State{Version: character.StateVersion}
	if err := state.UpsertProfile(character.Profile{
		ID: "authored", StoryID: "st", Name: "有来源的人", SourceKey: authored, Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rows := characterWindowCast(state, character.Scene{}, pictureSources{portraits: library, loreImages: map[string]string{
		authored: "assets/lore/images/entry-1/20261021-101010-default/image.png",
	}})
	if len(rows) != 1 {
		t.Fatalf("the cast lists %d rows", len(rows))
	}
	if got := rows[0].Avatar; got != "" {
		t.Fatalf("the shipped default was drawn beside a world-book picture: %q", got)
	}
	if got, want := rows[0].AvatarLorePath, "assets/lore/images/entry-1/20261021-101010-default/image.png"; got != want {
		t.Fatalf("world-book picture = %q, want %q", got, want)
	}
	// The lore entry has no 192px derivative, so the open conversation keeps the
	// small picture rather than asking for a size the route cannot serve.
	if got := rows[0].AvatarDisplay; got != "" {
		t.Fatalf("a world-book picture got a display portrait %q", got)
	}
}

// TestAChosenAvatarOutranksTheWorldBookPicture keeps the player's own choice on
// top. Choosing a picture is the one thing the player owns here; neither the
// world book nor the shipped set may replace it.
func TestAChosenAvatarOutranksTheWorldBookPicture(t *testing.T) {
	state := character.State{Version: character.StateVersion}
	if err := state.UpsertProfile(character.Profile{
		ID: "authored", StoryID: "st", Name: "有来源的人", SourceKey: "canon_fairy_nichang",
		Avatar: "/api/projects/p/attachments/mine", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rows := characterWindowCast(state, character.Scene{}, pictureSources{loreImages: map[string]string{
		"canon_fairy_nichang": "assets/lore/images/entry-1/image.png",
	}})
	if len(rows) != 1 {
		t.Fatalf("the cast lists %d rows", len(rows))
	}
	if got, want := rows[0].Avatar, "/api/projects/p/attachments/mine"; got != want {
		t.Fatalf("the chosen avatar was replaced: got %q, want %q", got, want)
	}
	if got := rows[0].AvatarLorePath; got != "" {
		t.Fatalf("a chosen avatar was overridden by the world book: %q", got)
	}
}

// TestAWorldBookPictureIsNeverAnAbsolutePath keeps one machine's layout out of
// the read. A stored path that is absolute, carries a scheme or climbs out of
// the project is not something the window can address, so it is dropped and the
// placeholder stands in.
func TestAWorldBookPictureIsNeverAnAbsolutePath(t *testing.T) {
	cases := map[string]string{
		"absolute":     `C:/Users/someone/pictures/face.png`,
		"posix":        "/home/someone/face.png",
		"scheme":       "file:///C:/face.png",
		"traversal":    "assets/../../secret/face.png",
		"empty":        "",
		"only a slash": "/",
	}
	for name, stored := range cases {
		if got, ok := projectAssetPath(stored); ok {
			t.Fatalf("%s: %q was accepted as %q", name, stored, got)
		}
	}
	if got, ok := projectAssetPath("assets/lore/images/a/image.png"); !ok || got != "assets/lore/images/a/image.png" {
		t.Fatalf("a project-relative asset was refused: %q %v", got, ok)
	}
}

// TestAnotherWorldsSameNamedEntryCannotLendAFace is the isolation rule stated as
// a failure: two worlds may each author a 陈巧倩 whose entry IDs and source keys
// are identical, so only the revision-qualified ID plus the binding's own scope
// keeps one world's picture off the other's person.
func TestAnotherWorldsSameNamedEntryCannotLendAFace(t *testing.T) {
	const hash = "0123456789ab"
	binding := &interactive.StoryPackBinding{
		PackID: "pack-a", ContentHash: hash,
		Scope: interactive.PackScopeForRevision("pack-a", hash),
	}
	catalog := &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{
		{SourceKey: "npc_chen", Name: "陈巧倩", LoreID: "world-chen"},
	}}
	entryID := interactive.PackEntryIDForRevision("world-chen", hash)

	// The same authored entry, written by another world: same ID, other scope.
	other := []booklore.Item{{
		ID: entryID, Scope: interactive.PackScopeForRevision("pack-b", hash), Name: "陈巧倩",
		Image: &booklore.Image{ImagePath: "assets/lore/images/other/image.png"},
	}}
	if got := characterLoreImageSources(binding, catalog, other); got != nil {
		t.Fatalf("another world's entry supplied a face: %v", got)
	}

	// The same pack at a later revision: same scope prefix, other revision, and
	// therefore a different entry ID that this story must not read.
	later := []booklore.Item{{
		ID: interactive.PackEntryIDForRevision("world-chen", "ffffffffffff"),
		Scope: interactive.PackScopeForRevision("pack-a", "ffffffffffff"), Name: "陈巧倩",
		Image: &booklore.Image{ImagePath: "assets/lore/images/later/image.png"},
	}}
	if got := characterLoreImageSources(binding, catalog, later); got != nil {
		t.Fatalf("a later revision supplied a face: %v", got)
	}

	// This story's own entry does answer, which is what proves the two refusals
	// above were the isolation rule and not an empty world book.
	own := []booklore.Item{{
		ID: entryID, Scope: binding.Scope, Name: "陈巧倩",
		Image: &booklore.Image{ImagePath: "assets/lore/images/own/image.png"},
	}}
	got := characterLoreImageSources(binding, catalog, own)
	if got["npc_chen"] != "assets/lore/images/own/image.png" {
		t.Fatalf("the story's own entry did not answer: %v", got)
	}
}

// TestAnEntryWithNoPictureLeavesTheShippedDefaultInPlace: an entry the user never
// gave a picture is an ordinary state, and the shipped default still answers.
func TestAnEntryWithNoPictureLeavesTheShippedDefaultInPlace(t *testing.T) {
	const hash = "0123456789ab"
	binding := &interactive.StoryPackBinding{
		PackID: "pack-a", ContentHash: hash, Scope: interactive.PackScopeForRevision("pack-a", hash),
	}
	catalog := &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{
		{SourceKey: "npc_chen", Name: "陈巧倩", LoreID: "world-chen"},
	}}
	items := []booklore.Item{{
		ID: interactive.PackEntryIDForRevision("world-chen", hash), Scope: binding.Scope, Name: "陈巧倩",
	}}
	if got := characterLoreImageSources(binding, catalog, items); got != nil {
		t.Fatalf("an entry without a picture produced %v", got)
	}

	library := portrait.NewLibrary()
	authored := library.Keys()[0]
	state := character.State{Version: character.StateVersion}
	if err := state.UpsertProfile(character.Profile{
		ID: "chen", StoryID: "st", Name: "陈巧倩", SourceKey: authored, Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rows := characterWindowCast(state, character.Scene{}, pictureSources{portraits: library})
	if len(rows) != 1 || rows[0].Avatar != library.URL(authored) {
		t.Fatalf("the shipped default stopped answering: %+v", rows)
	}
}
