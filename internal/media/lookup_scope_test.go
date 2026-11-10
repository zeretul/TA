package media

import "testing"

func TestAWorldPictureOutranksTheSharedDefault(t *testing.T) {
	dir := writeManifest(t, `
		{"reviewed":true,"id":"a-global","source_key":"same","purpose":"avatar","file":"aaaaaaaaaaaaaaaa.png"},
		{"reviewed":true,"id":"z-world","world":"one","source_key":"same","purpose":"avatar","file":"bbbbbbbbbbbbbbbb.png"}
	`)
	asset, ok := NewLibrary(dir).Picture(PictureQuery{World: "one", SourceKey: "same", Purpose: PurposeAvatar})
	if !ok || asset.File != "bbbbbbbbbbbbbbbb.png" {
		t.Fatalf("shared fallback masked this world's picture: %+v", asset)
	}
}

func TestAPrivatePictureDoesNotAppearInAnOrdinaryView(t *testing.T) {
	dir := writeManifest(t, `{"source_key":"same","purpose":"portrait","role":"intimate","file":"aaaaaaaaaaaaaaaa.png"}`)
	if asset, ok := NewLibrary(dir).Picture(PictureQuery{SourceKey: "same", Purpose: PurposePortrait}); ok {
		t.Fatalf("private activity picture escaped into an ordinary portrait: %+v", asset)
	}
}

func TestClipsUseWorldSceneAndActivityRole(t *testing.T) {
	dir := writeManifest(t, `
		{"reviewed":true,"world":"one","source_key":"same","scene":"坊市","role":"general","file":"aaaaaaaaaaaaaaaa.mp4"},
		{"reviewed":true,"world":"other","source_key":"same","role":"general","file":"bbbbbbbbbbbbbbbb.mp4"},
		{"reviewed":true,"world":"one","source_key":"same","scene":"山门","role":"general","file":"cccccccccccccccc.mp4"},
		{"reviewed":true,"world":"one","source_key":"same","role":"intimate","file":"dddddddddddddddd.mp4"}
	`)
	library := NewLibrary(dir)
	clips := library.ClipsFor(PictureQuery{World: "one", SourceKey: "same", SceneKey: "坊市"})
	if len(clips) != 1 || clips[0].File != "aaaaaaaaaaaaaaaa.mp4" {
		t.Fatalf("clips ignored world, scene or role: %+v", clips)
	}
	private := library.ClipsFor(PictureQuery{World: "one", SourceKey: "same", Role: RoleIntimate})
	if len(private) != 1 || private[0].File != "dddddddddddddddd.mp4" {
		t.Fatalf("qualified private view did not get its clip: %+v", private)
	}
}

func TestAnInventedCharactersClipStaysInItsOwnStory(t *testing.T) {
	dir := writeManifest(t, `{"reviewed":true,"scope":"story-one","actor":"same","purpose":"clip","file":"aaaaaaaaaaaaaaaa.mp4"}`)
	library := NewLibrary(dir)
	if clips := library.ClipsFor(PictureQuery{Scope: "story-one", Actor: "same"}); len(clips) != 1 {
		t.Fatalf("invented actor lost its own clip: %+v", clips)
	}
	if clips := library.ClipsFor(PictureQuery{Scope: "story-two", Actor: "same"}); len(clips) != 0 {
		t.Fatalf("invented actor borrowed another story's clip: %+v", clips)
	}
}

// TestAnUnreviewedBindingIsNeverOffered is the review gate: a row's tags say
// where a file was meant to go, not what is actually in it, so a binding nobody
// has looked at must not reach a player even when every tag matches perfectly.
// The same row with the flag set is served, which is what makes the flag the
// only difference.
func TestAnUnreviewedBindingIsNeverOffered(t *testing.T) {
	unreviewed := writeManifest(t, `
		{"world":"one","source_key":"same","subject":"character","purpose":"avatar","file":"aaaaaaaaaaaaaaaa.png"},
		{"world":"one","source_key":"same","role":"general","purpose":"clip","file":"bbbbbbbbbbbbbbbb.mp4"}
	`)
	library := NewLibrary(unreviewed)
	if asset, ok := library.Picture(PictureQuery{World: "one", SourceKey: "same", Purpose: PurposeAvatar}); ok {
		t.Fatalf("an unreviewed picture answered: %+v", asset)
	}
	if clips := library.ClipsFor(PictureQuery{World: "one", SourceKey: "same", Role: RoleGeneral}); len(clips) != 0 {
		t.Fatalf("an unreviewed clip answered: %+v", clips)
	}

	reviewed := writeManifest(t, `
		{"reviewed":true,"world":"one","source_key":"same","subject":"character","purpose":"avatar","file":"aaaaaaaaaaaaaaaa.png"},
		{"reviewed":true,"world":"one","source_key":"same","role":"general","purpose":"clip","file":"bbbbbbbbbbbbbbbb.mp4"}
	`)
	served := NewLibrary(reviewed)
	if _, ok := served.Picture(PictureQuery{World: "one", SourceKey: "same", Purpose: PurposeAvatar}); !ok {
		t.Fatal("the same binding was refused after review")
	}
	if clips := served.ClipsFor(PictureQuery{World: "one", SourceKey: "same", Role: RoleGeneral}); len(clips) != 1 {
		t.Fatalf("the reviewed clip did not answer: %+v", clips)
	}
}

// TestAnImportedPictureCountsAsReviewed: the player chose this file for this
// subject themselves, which is the review the flag stands for. Requiring a
// second acknowledgement would leave their own import unusable.
func TestAnImportedPictureCountsAsReviewed(t *testing.T) {
	library := NewLibrary(t.TempDir())
	asset, err := library.Import(ImportRequest{
		World: "one", Subject: SubjectCharacter, SourceKey: "same",
		Purpose: PurposeAvatar, Data: pngBytes(t, 20),
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !asset.Reviewed {
		t.Fatal("an import was recorded as unreviewed")
	}
	if _, ok := NewLibrary(library.Dir()).Picture(PictureQuery{World: "one", SourceKey: "same", Purpose: PurposeAvatar}); !ok {
		t.Fatal("the imported picture did not answer after a reload")
	}
}
