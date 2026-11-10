package media

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// pngBytes makes a real PNG so the importer's own decode is exercised rather
// than a hand-written header it might accept by accident.
func pngBytes(t *testing.T, shade uint8) []byte {
	t.Helper()
	source := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			source.Set(x, y, color.RGBA{R: shade, G: 0, B: 0, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, source); err != nil {
		t.Fatalf("encode a picture: %v", err)
	}
	return buffer.Bytes()
}

// TestAPictureIsFoundByWorldAndAuthorKey is the binding rule: a face belongs to
// one world's author key, and the same key in another world is another person.
func TestAPictureIsFoundByWorldAndAuthorKey(t *testing.T) {
	dir := writeManifest(t, `
		{"reviewed":true,"world":"fanren-jie","subject":"character","source_key":"npc_ning_fuling","purpose":"avatar","file":"aaaaaaaaaaaaaaaa.png"},
		{"reviewed":true,"world":"other-world","subject":"character","source_key":"npc_ning_fuling","purpose":"avatar","file":"bbbbbbbbbbbbbbbb.png"}
	`)
	library := NewLibrary(dir)

	asset, ok := library.Picture(PictureQuery{
		World: "fanren-jie", SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar,
	})
	if !ok || asset.File != "aaaaaaaaaaaaaaaa.png" {
		t.Fatalf("the world's own picture did not answer: %+v ok=%v", asset, ok)
	}

	// The other world's picture for the same key must not answer this world, and
	// the same key in a world with no picture gets nothing rather than a loan.
	if asset, ok := library.Picture(PictureQuery{
		World: "third-world", SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar,
	}); ok {
		t.Fatalf("another world's face was lent: %+v", asset)
	}
}

// TestAPictureWithNoWorldAnswersEveryWorld is what makes one delivery usable by
// several world packs: an author may ship a default face without binding it to
// one world.
func TestAPictureWithNoWorldAnswersEveryWorld(t *testing.T) {
	dir := writeManifest(t, `
		{"reviewed":true,"subject":"character","source_key":"npc_ning_fuling","purpose":"avatar","file":"aaaaaaaaaaaaaaaa.png"}
	`)
	library := NewLibrary(dir)
	for _, world := range []string{"fanren-jie", "wuxia", ""} {
		if _, ok := library.Picture(PictureQuery{
			World: world, SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar,
		}); !ok {
			t.Fatalf("a world-agnostic picture did not answer %q", world)
		}
	}
	// The reverse is not true: a picture that names a world answers only that
	// world, so it cannot leak into a story that names none.
	bound := writeManifest(t, `
		{"reviewed":true,"world":"fanren-jie","subject":"character","source_key":"npc_ning_fuling","purpose":"avatar","file":"aaaaaaaaaaaaaaaa.png"}
	`)
	if _, ok := NewLibrary(bound).Picture(PictureQuery{
		SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar,
	}); ok {
		t.Fatal("a world-bound picture answered a request that named no world")
	}
}

// TestAStoryInventedPersonIsBoundByScopeAndActor: a person the story invented
// has no author key, so their picture follows the actor inside one story and
// must not follow the same actor ID into another.
func TestAStoryInventedPersonIsBoundByScopeAndActor(t *testing.T) {
	dir := writeManifest(t, `
		{"reviewed":true,"subject":"character","scope":"story-a","actor":"actor-1","purpose":"avatar","file":"aaaaaaaaaaaaaaaa.png"}
	`)
	library := NewLibrary(dir)
	if _, ok := library.Picture(PictureQuery{
		Scope: "story-a", Actor: "actor-1", Purpose: PurposeAvatar,
	}); !ok {
		t.Fatal("the story's own actor did not answer")
	}
	if _, ok := library.Picture(PictureQuery{
		Scope: "story-b", Actor: "actor-1", Purpose: PurposeAvatar,
	}); ok {
		t.Fatal("a face leaked into another story through a shared actor ID")
	}
}

// TestASceneBoundPictureOutranksTheOrdinaryOneAndAnotherSceneIsNotACandidate
// is the scene rule: a picture authored for this scene is drawn first, and one
// authored for a different scene is not a fallback.
func TestASceneBoundPictureOutranksTheOrdinaryOneAndAnotherSceneIsNotACandidate(t *testing.T) {
	dir := writeManifest(t, `
		{"reviewed":true,"subject":"character","source_key":"npc_ning_fuling","purpose":"avatar","file":"aaaaaaaaaaaaaaaa.png"},
		{"reviewed":true,"subject":"character","source_key":"npc_ning_fuling","purpose":"avatar","scene":"node-1|坊市","file":"bbbbbbbbbbbbbbbb.png"},
		{"reviewed":true,"subject":"character","source_key":"npc_ning_fuling","purpose":"avatar","scene":"node-9|黄枫谷","file":"cccccccccccccccc.png"}
	`)
	library := NewLibrary(dir)

	asset, ok := library.Picture(PictureQuery{
		SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar, SceneKey: "node-1|坊市",
	})
	if !ok || asset.File != "bbbbbbbbbbbbbbbb.png" {
		t.Fatalf("the scene's own picture did not outrank the ordinary one: %+v ok=%v", asset, ok)
	}
	// Another scene's picture is not a candidate; the ordinary picture answers
	// instead, which is what keeps a moment's costume off an unrelated scene.
	asset, ok = library.Picture(PictureQuery{
		SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar, SceneKey: "node-2|别处",
	})
	if !ok || asset.File != "aaaaaaaaaaaaaaaa.png" {
		t.Fatalf("another scene's picture answered: %+v ok=%v", asset, ok)
	}
}

// TestAPlacePictureNeverAnswersAPerson keeps the two subjects apart: a scene's
// own illustration must not become a person's face.
func TestAPlacePictureNeverAnswersAPerson(t *testing.T) {
	dir := writeManifest(t, `
		{"reviewed":true,"subject":"place","place":"越国西境坊市","purpose":"scene","file":"aaaaaaaaaaaaaaaa.png"}
	`)
	library := NewLibrary(dir)
	if _, ok := library.Picture(PictureQuery{
		SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar,
	}); ok {
		t.Fatal("a place picture was offered as a person's face")
	}
	if asset, ok := library.Picture(PictureQuery{
		Place: "越国西境坊市", Purpose: PurposeScene,
	}); !ok || asset.File != "aaaaaaaaaaaaaaaa.png" {
		t.Fatalf("the place's own picture did not answer: %+v ok=%v", asset, ok)
	}
	// Two worlds may each have a 坊市, so a picture that names one world answers
	// only that world; an unbound picture is the place's own default and answers
	// every world, which is what lets one delivery serve several packs.
	bound := writeManifest(t, `
		{"reviewed":true,"world":"fanren-jie","subject":"place","place":"越国西境坊市","purpose":"scene","file":"bbbbbbbbbbbbbbbb.png"}
	`)
	if _, ok := NewLibrary(bound).Picture(PictureQuery{
		World: "other-world", Place: "越国西境坊市", Purpose: PurposeScene,
	}); ok {
		t.Fatal("a world-bound place picture answered another world")
	}
}

// TestAClipIsNeverOfferedAsAPicture states the purpose boundary: a clip is
// played, not drawn into an image slot.
func TestAClipIsNeverOfferedAsAPicture(t *testing.T) {
	dir := writeManifest(t, `
		{"reviewed":true,"subject":"character","source_key":"npc_ning_fuling","purpose":"clip","role":"general","file":"aaaaaaaaaaaaaaaa.mp4"}
	`)
	library := NewLibrary(dir)
	for _, purpose := range []string{PurposeAvatar, PurposePortrait, PurposeScene, PurposeClip, ""} {
		if asset, ok := library.Picture(PictureQuery{
			SourceKey: "npc_ning_fuling", Purpose: purpose,
		}); ok {
			t.Fatalf("purpose %q drew a clip into a picture slot: %+v", purpose, asset)
		}
	}
}

// TestImportCopiesTheFileAndBindsIt is the operation the author performs: one
// uploaded picture, one manifest row, one file named after its content.
func TestImportCopiesTheFileAndBindsIt(t *testing.T) {
	dir := t.TempDir()
	library := NewLibrary(dir)
	data := pngBytes(t, 10)

	asset, err := library.Import(ImportRequest{
		World: "fanren-jie", Subject: SubjectCharacter, SourceKey: "npc_ning_fuling",
		Purpose: PurposeAvatar, Data: data,
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if asset.Width != 4 || asset.Height != 3 {
		t.Fatalf("the picture's own shape was not read: %dx%d", asset.Width, asset.Height)
	}
	if asset.File == "" || asset.File[0] != 'u' {
		t.Fatalf("an import is not named as one: %q", asset.File)
	}
	if _, err := os.Stat(filepath.Join(dir, asset.File)); err != nil {
		t.Fatalf("the file was not written: %v", err)
	}
	// A fresh library reads the same binding, which is what makes the file and
	// the manifest the only state.
	if _, ok := NewLibrary(dir).Picture(PictureQuery{
		World: "fanren-jie", SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar,
	}); !ok {
		t.Fatal("the imported picture did not survive a reload")
	}
}

// TestImportingTheSamePictureTwiceStoresOneFile is the de-duplication rule: the
// file is named after its content, so a repeated import is not a second copy.
func TestImportingTheSamePictureTwiceStoresOneFile(t *testing.T) {
	dir := t.TempDir()
	library := NewLibrary(dir)
	data := pngBytes(t, 20)
	for i := 0; i < 2; i++ {
		if _, err := library.Import(ImportRequest{
			SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar, Data: data,
		}); err != nil {
			t.Fatalf("import %d: %v", i, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, entry := range entries {
		if entry.Name() != ManifestFile {
			files++
		}
	}
	if files != 1 {
		t.Fatalf("the same picture was stored %d times", files)
	}
	if got := len(library.Entries()); got != 1 {
		t.Fatalf("the same binding was listed %d times", got)
	}
}

// TestReplacingAPictureChangesItsNameAndDropsTheOldFile is what stops a replaced
// picture from being served from a cache, and what keeps the directory from
// growing a file per revision.
func TestReplacingAPictureChangesItsNameAndDropsTheOldFile(t *testing.T) {
	dir := t.TempDir()
	library := NewLibrary(dir)
	first, err := library.Import(ImportRequest{
		SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar, Data: pngBytes(t, 30),
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	second, err := library.Import(ImportRequest{
		SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar, Data: pngBytes(t, 90),
	})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if first.File == second.File {
		t.Fatal("a replaced picture kept its name, so a cache would serve the old one")
	}
	if _, err := os.Stat(filepath.Join(dir, first.File)); !os.IsNotExist(err) {
		t.Fatalf("the replaced file was left behind: %v", err)
	}
	if got := len(library.Entries()); got != 1 {
		t.Fatalf("a replacement added a second entry: %d", got)
	}
}

// TestRemovingAnOverrideRestoresTheDefault is the "restore the default" rule: an
// imported picture is one entry, and removing it leaves nothing behind.
func TestRemovingAnOverrideRestoresTheDefault(t *testing.T) {
	dir := t.TempDir()
	library := NewLibrary(dir)
	asset, err := library.Import(ImportRequest{
		SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar, Data: pngBytes(t, 40),
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	removed, err := library.Remove(asset.ID)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed.File != asset.File {
		t.Fatalf("removed %q, imported %q", removed.File, asset.File)
	}
	if _, err := os.Stat(filepath.Join(dir, asset.File)); !os.IsNotExist(err) {
		t.Fatalf("the removed file was left behind: %v", err)
	}
	if _, ok := library.Picture(PictureQuery{
		SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar,
	}); ok {
		t.Fatal("the removed picture still answered")
	}
	if _, err := library.Remove(asset.ID); err == nil {
		t.Fatal("removing an entry twice reported success")
	}
}

// TestRemovingAShippedPictureKeepsItsFile: an author's delivered file is not the
// player's to delete, so dropping the binding must not delete the file.
func TestRemovingAShippedPictureKeepsItsFile(t *testing.T) {
	dir := writeManifest(t, `
		{"reviewed":true,"subject":"character","source_key":"npc_ning_fuling","purpose":"avatar","file":"aaaaaaaaaaaaaaaa.png"}
	`)
	writeClip(t, dir, "aaaaaaaaaaaaaaaa.png", "shipped")
	library := NewLibrary(dir)
	asset, ok := library.Picture(PictureQuery{SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar})
	if !ok {
		t.Fatal("the shipped picture did not load")
	}
	if _, err := library.Remove(asset.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "aaaaaaaaaaaaaaaa.png")); err != nil {
		t.Fatalf("removing a binding deleted the author's file: %v", err)
	}
}

// TestImportRefusesWhatItCannotServe covers the refusals: empty bytes, bytes
// that are neither picture nor clip, and bytes past the size bound.
func TestImportRefusesWhatItCannotServe(t *testing.T) {
	library := NewLibrary(t.TempDir())
	cases := map[string]ImportRequest{
		"empty":    {SourceKey: "npc_ning_fuling", Data: nil},
		"not a picture": {SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar,
			Data: []byte("this is not a picture")},
		"clip bytes as a picture": {SourceKey: "npc_ning_fuling", Purpose: PurposeAvatar,
			Data: []byte("\x00\x00\x00\x18ftypmp42")},
		"too large": {SourceKey: "npc_ning_fuling", Data: make([]byte, MaxAssetBytes+1)},
		"no identity": {Purpose: PurposeAvatar, Data: pngBytes(t, 50)},
	}
	for name, request := range cases {
		if _, err := library.Import(request); err == nil {
			t.Fatalf("%s: the import was accepted", name)
		}
	}
	if _, err := NewLibrary("").Import(ImportRequest{
		SourceKey: "npc_ning_fuling", Data: pngBytes(t, 60),
	}); err != ErrNoMediaDir {
		t.Fatalf("a build with no media directory reported %v", err)
	}
}

// TestAClipIsImportedAsAClip keeps the clip path working through the same call:
// the purpose decides which magic numbers are acceptable.
func TestAClipIsImportedAsAClip(t *testing.T) {
	library := NewLibrary(t.TempDir())
	// An ISO base media header is what the browser decodes; this build does not
	// decode the clip itself, so the header is all it checks.
	clip := append([]byte("\x00\x00\x00\x18ftypmp42"), make([]byte, 64)...)
	asset, err := library.Import(ImportRequest{
		SourceKey: "npc_ning_fuling", Purpose: PurposeClip, Role: RoleGeneral, Data: clip,
	})
	if err != nil {
		t.Fatalf("import a clip: %v", err)
	}
	if asset.File[len(asset.File)-4:] != ".mp4" {
		t.Fatalf("a clip was named %q", asset.File)
	}
	if asset.Width != 0 || asset.Height != 0 {
		t.Fatalf("a clip claimed a picture's shape: %dx%d", asset.Width, asset.Height)
	}
	if _, ok := library.General("npc_ning_fuling"); !ok {
		t.Fatal("the imported clip is not reachable as an ordinary-day clip")
	}
}
