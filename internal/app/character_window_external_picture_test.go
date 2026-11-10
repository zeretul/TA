package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"ta/internal/character"
	"ta/internal/media"
	"ta/internal/portrait"
)

// importPicture writes one picture into a media directory through the same call
// the author's import uses, so the test exercises the real binding rather than a
// hand-written manifest.
func importPicture(t *testing.T, library *media.Library, request media.ImportRequest) media.Asset {
	t.Helper()
	source := image.NewRGBA(image.Rect(0, 0, 2, 2))
	source.Set(0, 0, color.RGBA{R: 200, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, source); err != nil {
		t.Fatalf("encode a picture: %v", err)
	}
	request.Data = buffer.Bytes()
	asset, err := library.Import(request)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	return asset
}

// TestAnExternalPictureOutranksTheShippedDefault is the design's order stated as
// a failure: an author's own picture for a person is drawn before the default
// that lives inside the binary.
func TestAnExternalPictureOutranksTheShippedDefault(t *testing.T) {
	shipped := portrait.NewLibrary()
	authored := shipped.Keys()[0]

	library := media.NewLibrary(t.TempDir())
	asset := importPicture(t, library, media.ImportRequest{
		World: "fanren-jie", SourceKey: authored, Purpose: media.PurposeAvatar,
	})

	state := character.State{Version: character.StateVersion}
	if err := state.UpsertProfile(character.Profile{
		ID: "p", StoryID: "st", Name: "有来源的人", SourceKey: authored, Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rows := characterWindowCast(state, character.Scene{}, pictureSources{
		portraits: shipped, media: library, world: "fanren-jie",
	})
	if len(rows) != 1 {
		t.Fatalf("the cast lists %d rows", len(rows))
	}
	if got, want := rows[0].Avatar, media.URL(asset); got != want {
		t.Fatalf("avatar = %q, want the external picture %q", got, want)
	}
	// The list row draws the external picture; the open conversation asks for the
	// portrait purpose first and falls back to the same avatar, because this
	// delivery shipped only a face.
	if got, want := rows[0].AvatarDisplay, media.URL(asset); got != want {
		t.Fatalf("display = %q, want %q", got, want)
	}
}

// TestAChosenAvatarStillOutranksAnExternalPicture keeps the player's own choice
// on top: an author's delivery must not replace what the player set.
func TestAChosenAvatarStillOutranksAnExternalPicture(t *testing.T) {
	library := media.NewLibrary(t.TempDir())
	importPicture(t, library, media.ImportRequest{
		SourceKey: "npc_ning_fuling", Purpose: media.PurposeAvatar,
	})
	state := character.State{Version: character.StateVersion}
	if err := state.UpsertProfile(character.Profile{
		ID: "p", StoryID: "st", Name: "宁茯苓", SourceKey: "npc_ning_fuling",
		Avatar: "/api/projects/p/attachments/mine", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rows := characterWindowCast(state, character.Scene{}, pictureSources{media: library})
	if got, want := rows[0].Avatar, "/api/projects/p/attachments/mine"; got != want {
		t.Fatalf("an external picture replaced the player's choice: %q", got)
	}
}

// TestAnExternalPictureOverridesTheWorldBooksPicture keeps the delivery's word
// last: the world book already had a way to give a person a picture, and the
// external file is the author overriding it without editing the pack. A person
// the delivery says nothing about still falls back to the world book, so the
// override adds a layer rather than replacing the chain.
func TestAnExternalPictureOverridesTheWorldBooksPicture(t *testing.T) {
	library := media.NewLibrary(t.TempDir())
	asset := importPicture(t, library, media.ImportRequest{
		SourceKey: "npc_ning_fuling", Purpose: media.PurposeAvatar,
	})
	state := character.State{Version: character.StateVersion}
	if err := state.UpsertProfile(character.Profile{
		ID: "p", StoryID: "st", Name: "宁茯苓", SourceKey: "npc_ning_fuling", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	// The world book's picture is present in both halves; only the delivery
	// differs, which is what isolates the override.
	worldBook := map[string]string{"npc_ning_fuling": "assets/lore/images/entry-1/image.png"}

	overridden := characterWindowCast(state, character.Scene{}, pictureSources{
		media: library, loreImages: worldBook,
	})[0]
	if got, want := overridden.Avatar, media.URL(asset); got != want {
		t.Fatalf("the world book's picture outranks the delivery: %q, want %q", got, want)
	}
	// Only one picture is drawn: the lore path must not also be reported, or the
	// client would have two answers for one face.
	if overridden.AvatarLorePath != "" {
		t.Fatalf("the overridden lore path was reported too: %q", overridden.AvatarLorePath)
	}

	// With the override gone the world book answers again.
	if _, err := library.Remove(asset.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	restored := characterWindowCast(state, character.Scene{}, pictureSources{
		media: library, loreImages: worldBook,
	})[0]
	if got, want := restored.AvatarLorePath, "assets/lore/images/entry-1/image.png"; got != want {
		t.Fatalf("lore path = %q, want %q", got, want)
	}
	if restored.Avatar != "" {
		t.Fatalf("a ready URL was reported beside the lore path: %q", restored.Avatar)
	}
}

// TestAScenePictureFollowsTheSceneOnScreen is the scene rule: a picture authored
// for the scene the player is in is drawn, and one authored for another scene is
// not.
func TestAScenePictureFollowsTheSceneOnScreen(t *testing.T) {
	library := media.NewLibrary(t.TempDir())
	importPicture(t, library, media.ImportRequest{
		SourceKey: "npc_ning_fuling", Purpose: media.PurposeAvatar, Scene: "node-1|坊市",
	})
	state := character.State{Version: character.StateVersion}
	if err := state.UpsertProfile(character.Profile{
		ID: "p", StoryID: "st", Name: "宁茯苓", SourceKey: "npc_ning_fuling", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}

	here := pictureSources{media: library, sceneKey: "node-1|坊市"}
	if got := characterWindowCast(state, character.Scene{}, here)[0].Avatar; got == "" {
		t.Fatal("the scene's own picture did not answer")
	}
	elsewhere := pictureSources{media: library, sceneKey: "node-2|别处"}
	if got := characterWindowCast(state, character.Scene{}, elsewhere)[0].Avatar; got != "" {
		t.Fatalf("another scene's picture answered: %q", got)
	}
}

// TestAStoryInventedPersonKeepsTheirBindingAcrossReopen is the dynamic-person
// rule: a person the story invented has no author key, so their picture follows
// the actor inside one story, survives a reopen, and does not follow the same
// actor ID into another story.
func TestAStoryInventedPersonKeepsTheirBindingAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	library := media.NewLibrary(dir)
	asset := importPicture(t, library, media.ImportRequest{
		Scope: "story-a", Actor: "actor-1", Purpose: media.PurposeAvatar,
	})

	state := character.State{Version: character.StateVersion}
	if err := state.UpsertProfile(character.Profile{
		ID: "actor-1", StoryID: "story-a", Name: "自造的人", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	sources := pictureSources{media: library}
	first := characterWindowCast(state, character.Scene{}, sources)
	if got, want := first[0].Avatar, media.URL(asset); got != want {
		t.Fatalf("the invented person's picture did not answer: %q", got)
	}

	// A cold reopen reads the same manifest from disk.
	reopened := media.NewLibrary(dir)
	second := characterWindowCast(state, character.Scene{}, pictureSources{media: reopened})
	if got, want := second[0].Avatar, media.URL(asset); got != want {
		t.Fatalf("the binding did not survive a reopen: %q, want %q", got, want)
	}

	// The same actor ID in another story is another person.
	other := character.State{Version: character.StateVersion}
	if err := other.UpsertProfile(character.Profile{
		ID: "actor-1", StoryID: "story-b", Name: "另一个自造的人", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if got := characterWindowCast(other, character.Scene{}, sources)[0].Avatar; got != "" {
		t.Fatalf("a face followed an actor ID into another story: %q", got)
	}
}

// TestAnExternalPlacePictureIsDrawnForThePlace is the place half: a place's own
// picture illustrates where the story already put the player, and a place with
// no picture stays text alone rather than a blank frame.
func TestAnExternalPlacePictureIsDrawnForThePlace(t *testing.T) {
	library := media.NewLibrary(t.TempDir())
	asset := importPicture(t, library, media.ImportRequest{
		World: "fanren-jie", Subject: media.SubjectPlace, Place: "越国西境坊市",
		Purpose: media.PurposeScene,
	})
	sources := pictureSources{
		media: library, world: "fanren-jie",
		places: []placeRef{{Name: "越国西境坊市"}},
	}
	if got, want := sources.place("越国西境坊市"), media.URL(asset); got != want {
		t.Fatalf("place picture = %q, want %q", got, want)
	}
	if got := sources.place("黄枫谷"); got != "" {
		t.Fatalf("an unknown place got a picture: %q", got)
	}
	if got := sources.place(""); got != "" {
		t.Fatalf("an empty place got a picture: %q", got)
	}
	// Another world's 坊市 is another place.
	otherWorld := pictureSources{
		media: library, world: "other-world",
		places: []placeRef{{Name: "越国西境坊市"}},
	}
	if got := otherWorld.place("越国西境坊市"); got != "" {
		t.Fatalf("another world's place picture answered: %q", got)
	}
}

// TestAPlacePictureFollowsTheProseLocation is the resolution rule: the scene's
// location is prose the story wrote, while a picture binds to the registered
// place. A narrator adding a detail must not make the picture disappear.
func TestAPlacePictureFollowsTheProseLocation(t *testing.T) {
	library := media.NewLibrary(t.TempDir())
	asset := importPicture(t, library, media.ImportRequest{
		World: "fanren-jie", Subject: media.SubjectPlace, Place: "越国西境坊市",
		Purpose: media.PurposeScene,
	})
	sources := pictureSources{
		media: library, world: "fanren-jie",
		places: []placeRef{{Name: "越国西境坊市"}},
	}
	for _, location := range []string{"越国西境坊市", "越国西境坊市东街", " 越国西境坊市 的丹房"} {
		if got, want := sources.place(location), media.URL(asset); got != want {
			t.Fatalf("location %q drew %q, want %q", location, got, want)
		}
		if got, want := sources.placeName(location), "越国西境坊市"; got != want {
			t.Fatalf("location %q resolved to %q, want %q", location, got, want)
		}
	}
	if got := sources.placeName("黄枫谷"); got != "" {
		t.Fatalf("an unregistered location resolved to %q", got)
	}
}

// TestANestedPlaceNameResolvesToTheMoreSpecificPlace is why the catalog is
// searched longest first: a world registering both 坊市 and 越国西境坊市 means the
// latter when the story says the player is in 越国西境坊市.
func TestANestedPlaceNameResolvesToTheMoreSpecificPlace(t *testing.T) {
	sources := pictureSources{places: []placeRef{{Name: "越国西境坊市"}, {Name: "坊市"}}}
	if got, want := sources.placeName("越国西境坊市东街"), "越国西境坊市"; got != want {
		t.Fatalf("nested place resolved to %q, want %q", got, want)
	}
	if got, want := sources.placeName("黄枫谷坊市"), "坊市"; got != want {
		t.Fatalf("the only matching place resolved to %q, want %q", got, want)
	}
}

// TestARemovedPictureFallsBackToTheShippedDefault is the "restore the default"
// rule as the window sees it: removing the override returns the shipped picture
// with nothing left over.
func TestARemovedPictureFallsBackToTheShippedDefault(t *testing.T) {
	shipped := portrait.NewLibrary()
	authored := shipped.Keys()[0]
	library := media.NewLibrary(t.TempDir())
	asset := importPicture(t, library, media.ImportRequest{
		SourceKey: authored, Purpose: media.PurposeAvatar,
	})
	state := character.State{Version: character.StateVersion}
	if err := state.UpsertProfile(character.Profile{
		ID: "p", StoryID: "st", Name: "有来源的人", SourceKey: authored, Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	sources := pictureSources{portraits: shipped, media: library}
	if got := characterWindowCast(state, character.Scene{}, sources)[0].Avatar; got == shipped.URL(authored) {
		t.Fatal("the external picture did not answer before removal")
	}
	if _, err := library.Remove(asset.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got, want := characterWindowCast(state, character.Scene{}, sources)[0].Avatar, shipped.URL(authored); got != want {
		t.Fatalf("after removal the avatar is %q, want the shipped default %q", got, want)
	}
}

// TestABrokenFileDoesNotStopTheCast is the failure rule: a manifest that names a
// file which is not on disk still lets the window list people, and the picture
// falls back to the shipped default.
func TestABrokenFileDoesNotStopTheCast(t *testing.T) {
	dir := t.TempDir()
	document := `{"schema":"` + media.ManifestSchema + `","entries":[
		{"subject":"character","source_key":"npc_ning_fuling","purpose":"avatar","file":"u-0000000000000000.png"}
	]}`
	if err := os.WriteFile(filepath.Join(dir, media.ManifestFile), []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	shipped := portrait.NewLibrary()
	authored := shipped.Keys()[0]
	state := character.State{Version: character.StateVersion}
	if err := state.UpsertProfile(character.Profile{
		ID: "p", StoryID: "st", Name: "宁茯苓", SourceKey: authored, Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	// The entry names a file that is absent. The route answers 404 for it, and the
	// window still lists the person; what it must not do is fail to build a cast.
	rows := characterWindowCast(state, character.Scene{}, pictureSources{
		portraits: shipped, media: media.NewLibrary(dir),
	})
	if len(rows) != 1 {
		t.Fatalf("a missing file stopped the cast: %d rows", len(rows))
	}
}
