package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ta/config"
	"ta/internal/interactive"
	"ta/internal/presetlayout"
	"ta/internal/storypack"
)

// TestAnOldStoryKeepsItsPeopleAfterThePackIsEdited drives the U7 requirement
// through the product's real creation path: the pack file, the server-side
// catalog freeze, the story journal, and a reopened story.
//
// The failure it pins is a silent one. Before the freeze, a story's catalog was
// only ever the pack file. Edit the pack after the story began — add a person,
// fix a typo, bump the revision — and the story's next turn found a fingerprint
// that no longer matched its binding, withheld the page, and quietly lost every
// preset person it had. Nothing errored; the world just emptied.
//
// The snapshot is written once, into the story's own meta, so the file can move
// on without reaching back into a story already in progress.
func TestAnOldStoryKeepsItsPeopleAfterThePackIsEdited(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	t.Cleanup(func() { _ = store.Close() })
	service := &InteractiveAppService{app: &App{
		workspace:   workspace,
		cfg:         &config.Config{DenovaDir: dataRoot},
		interactive: store,
	}}

	pack, err := storypack.NewLibrary(dataRoot).Get("fanren-jie")
	if err != nil {
		t.Fatalf("read the 凡人 pack: %v", err)
	}
	const localKey = "npc_song_guanlan"
	if _, ok := pack.CatalogBySourceKey()[localKey]; !ok {
		t.Fatalf("the shipped catalog lost %s", localKey)
	}

	story, err := service.CreateInteractiveStoryContext(t.Context(), interactive.CreateStoryRequest{
		Title:       "凡人人界",
		Origin:      "越国公共坊市的外围。",
		PackBinding: pack.Binding(),
	})
	if err != nil {
		t.Fatalf("create story: %v", err)
	}
	created, err := store.StoryContext(story.ID, "")
	if err != nil {
		t.Fatalf("reopen story: %v", err)
	}
	if created.Meta.PackCatalog == nil || len(created.Meta.PackCatalog.Characters) == 0 {
		t.Fatal("the story was created without a catalog snapshot")
	}
	// The snapshot must be the revision the story bound to, not whatever the
	// library happened to hold a moment later.
	if got := created.Meta.PackCatalog.ContentVersion; got != pack.ContentVersion {
		t.Fatalf("snapshot revision = %d, want %d", got, pack.ContentVersion)
	}

	// The world moves on: a user pack of the same ID is written over the builtin,
	// carrying a different catalog. This is what "the pack was edited" looks like
	// on disk, and it is the exact state that used to empty an old story.
	edited := pack
	edited.Name = pack.Name + "（修订）"
	edited.ContentVersion = pack.ContentVersion + 1
	edited.Characters = pack.Characters[:1]
	if err := os.MkdirAll(presetlayout.StoryPacks(dataRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writePackFileForTest(t, filepath.Join(presetlayout.StoryPacks(dataRoot), pack.ID+".json"), edited); err != nil {
		t.Fatal(err)
	}
	live, err := storypack.NewLibrary(dataRoot).Get(pack.ID)
	if err != nil {
		t.Fatalf("read the edited pack: %v", err)
	}
	if live.ContentHash() == pack.ContentHash() {
		t.Fatal("the edited pack has the same fingerprint; the test would prove nothing")
	}
	if _, ok := live.CatalogBySourceKey()[localKey]; ok {
		t.Fatalf("the edited pack still ships %s; the test would prove nothing", localKey)
	}

	// Reopen through the journal, which is the path a later session takes.
	reopened, err := store.StoryContext(story.ID, "")
	if err != nil {
		t.Fatalf("reopen story after the edit: %v", err)
	}
	if reopened.Meta.PackCatalog == nil {
		t.Fatal("the catalog snapshot did not survive the journal round trip")
	}
	if got := reopened.Meta.PackCatalog.ContentVersion; got != pack.ContentVersion {
		t.Fatalf("snapshot revision after reopen = %d, want %d", got, pack.ContentVersion)
	}
	var kept bool
	for _, person := range reopened.Meta.PackCatalog.Characters {
		if person.SourceKey == localKey {
			kept = true
			break
		}
	}
	if !kept {
		t.Fatalf("%s was lost from the story's own catalog when the pack file was edited", localKey)
	}
}

// writePackFileForTest writes a pack the way the library reads it: the same
// document shape, through the same decoder.
func writePackFileForTest(t *testing.T, path string, pack storypack.Pack) error {
	t.Helper()
	document, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, document, 0o644)
}

// TestTheFrozenCatalogKeepsSpeciesQualifications drives story creation through
// the same entry point the server uses and then reads the species facts back out
// of the story's own meta.
//
// It exists because the freeze had a second copy: the query form in
// interactiveapp carried lifeform and the registered method, while the one this
// package calls dropped them. A spirit beast therefore survived the freeze as a
// name and a brief with no classification, and every growth rule that asks "is
// this a registered spirit beast" answered no. The catalog still described 灵兽
// in the world book, so nothing looked broken — the beast simply never grew.
func TestTheFrozenCatalogKeepsSpeciesQualifications(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	t.Cleanup(func() { _ = store.Close() })
	service := &InteractiveAppService{app: &App{
		workspace:   workspace,
		cfg:         &config.Config{DenovaDir: dataRoot},
		interactive: store,
	}}

	pack, err := storypack.NewLibrary(dataRoot).Get("fanren-jie")
	if err != nil {
		t.Fatalf("read the 凡人 pack: %v", err)
	}
	const beastKey = "species_lingshoushan_qingfeng_que"
	authored, ok := pack.CatalogBySourceKey()[beastKey]
	if !ok {
		t.Fatalf("the shipped catalog lost %s", beastKey)
	}
	if authored.Lifeform != storypack.LifeformSpiritBeast || authored.CultivationMethodID == "" {
		t.Fatalf("the shipped catalog no longer qualifies %s: %+v", beastKey, authored)
	}

	story, err := service.CreateInteractiveStoryContext(t.Context(), interactive.CreateStoryRequest{
		Title:       "凡人人界",
		Origin:      "越国公共坊市的外围。",
		PackBinding: pack.Binding(),
	})
	if err != nil {
		t.Fatalf("create story: %v", err)
	}
	created, err := store.StoryContext(story.ID, "")
	if err != nil {
		t.Fatalf("reopen story: %v", err)
	}
	if created.Meta.PackCatalog == nil {
		t.Fatal("the story was created without a catalog snapshot")
	}
	var frozen *interactive.PackCatalogPerson
	for i := range created.Meta.PackCatalog.Characters {
		if created.Meta.PackCatalog.Characters[i].SourceKey == beastKey {
			frozen = &created.Meta.PackCatalog.Characters[i]
			break
		}
	}
	if frozen == nil {
		t.Fatalf("%s is missing from the story's own catalog", beastKey)
	}
	if frozen.Lifeform != authored.Lifeform || frozen.CultivationMethodID != authored.CultivationMethodID {
		t.Fatalf("the frozen row lost its species qualification: lifeform=%q method=%q, want %q / %q",
			frozen.Lifeform, frozen.CultivationMethodID, authored.Lifeform, authored.CultivationMethodID)
	}

	// A story reopened from its journal must read back the same answer, because
	// that is the copy every later turn's growth rules are decided from.
	reopened, err := store.StoryContext(story.ID, "")
	if err != nil {
		t.Fatalf("reopen story from the journal: %v", err)
	}
	for _, person := range reopened.Meta.PackCatalog.Characters {
		if person.SourceKey != beastKey {
			continue
		}
		if person.Lifeform != authored.Lifeform || person.CultivationMethodID != authored.CultivationMethodID {
			t.Fatalf("the journal round trip lost the species qualification: %+v", person)
		}
		return
	}
	t.Fatalf("%s did not survive the journal round trip", beastKey)
}
