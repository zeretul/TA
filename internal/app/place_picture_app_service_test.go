package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"ta/config"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/media"
	"ta/internal/storypack"
)

// testPNG is the smallest file the media library accepts: it decodes, so the
// import reaches the manifest instead of failing on the format check.
func testPNG(t *testing.T) []byte {
	t.Helper()
	source := image.NewRGBA(image.Rect(0, 0, 2, 2))
	source.Set(0, 0, color.RGBA{R: 180, G: 40, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, source); err != nil {
		t.Fatalf("encode a picture: %v", err)
	}
	return buffer.Bytes()
}

// placePictureFixture builds a story on the shipped 凡人 pack with a real media
// directory, which is what a place picture needs: the place has to come from the
// story's frozen catalog, and the file has to land somewhere the game reads.
func placePictureFixture(t *testing.T) (*App, string, string) {
	t.Helper()
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	mediaDir := t.TempDir()
	store := interactive.NewStore(workspace)
	t.Cleanup(func() { _ = store.Close() })
	application := &App{
		workspace:   workspace,
		cfg:         &config.Config{DenovaDir: dataRoot, MediaDir: mediaDir},
		interactive: store,
	}
	service := &InteractiveAppService{app: application}

	pack, err := storypack.NewLibrary(dataRoot).Get("fanren-jie")
	if err != nil {
		t.Fatalf("read the 凡人 pack: %v", err)
	}
	story, err := service.CreateInteractiveStoryContext(t.Context(), interactive.CreateStoryRequest{
		Title:  "凡人人界",
		Origin: "越国公共坊市的外围。",
		// A pack story starts on a fixed template, so the world entities Actor and
		// its 地点记录 field exist from the first turn. A story that waits for an
		// opening schema draft cannot record a place at all, which is a different
		// story than the one the library's 本局新建 page reads.
		StateSchemaPolicy: &interactive.StoryStateSchemaPolicy{Mode: interactive.StoryStateSchemaModeFixedTemplate},
		PackBinding:       pack.Binding(),
	})
	if err != nil {
		t.Fatalf("create story: %v", err)
	}
	storyContext, err := store.StoryContext(story.ID, "")
	if err != nil {
		t.Fatalf("reopen story: %v", err)
	}
	if storyContext.Meta.PackCatalog == nil || len(storyContext.Meta.PackCatalog.Places) == 0 {
		t.Fatal("the story froze no places, so this fixture cannot test a place picture")
	}
	return application, story.ID, storyContext.Meta.PackCatalog.Places[0].Place
}

// TestAPlacePictureIsBoundToTheStorysOwnWorld is the whole place path: import,
// read back, and remove. The binding carries the world the story is bound to and
// the catalog's own spelling of the place, so the picture answers the lookup the
// window actually makes.
func TestAPlacePictureIsBoundToTheStorysOwnWorld(t *testing.T) {
	application, storyID, place := placePictureFixture(t)

	imported, err := application.PlacePictureImport(PlacePictureImport{
		StoryID: storyID, Place: place, Filename: "street.png", Data: testPNG(t),
	})
	if err != nil {
		t.Fatalf("import a place picture: %v", err)
	}
	if imported.Picture.URL == "" || !imported.Picture.Overridden {
		t.Fatalf("the import did not report a picture: %+v", imported.Picture)
	}
	if imported.Asset.Subject != media.SubjectPlace {
		t.Fatalf("the entry is not a place: %q", imported.Asset.Subject)
	}
	if imported.Asset.Purpose != media.PurposeScene {
		t.Fatalf("the entry is not a scene picture: %q", imported.Asset.Purpose)
	}
	if imported.Asset.World == "" {
		t.Fatal("the entry names no world, so it would answer for every world")
	}
	if got := imported.Asset.Place; got != place {
		t.Fatalf("the entry names %q, want %q", got, place)
	}

	// The window's own lookup answers with the same picture.
	read, err := application.PlacePicture(storyID, place)
	if err != nil {
		t.Fatalf("read the place picture: %v", err)
	}
	if read.URL != imported.Picture.URL {
		t.Fatalf("the window would draw %q, the import reported %q", read.URL, imported.Picture.URL)
	}

	// And removing it leaves nothing behind.
	if _, err := application.PlacePictureClear(PlacePictureImport{StoryID: storyID, Place: place}); err != nil {
		t.Fatalf("clear the place picture: %v", err)
	}
	cleared, err := application.PlacePicture(storyID, place)
	if err != nil {
		t.Fatalf("read after clearing: %v", err)
	}
	if cleared.URL != "" || cleared.Overridden {
		t.Fatalf("the picture survived the clear: %+v", cleared)
	}
}

// TestAPlaceTheStoryNeverRegisteredIsRefused is the guard against a manifest that
// claims a picture nothing will ever draw: a name the frozen catalog does not
// hold is not a target, so the import fails rather than binding a file to a place
// that exists nowhere.
func TestAPlaceTheStoryNeverRegisteredIsRefused(t *testing.T) {
	application, storyID, _ := placePictureFixture(t)

	_, err := application.PlacePictureImport(PlacePictureImport{
		StoryID: storyID, Place: "从未登记的地方", Filename: "x.png", Data: testPNG(t),
	})
	if err == nil {
		t.Fatal("a place the story never registered was accepted")
	}
	if !strings.Contains(err.Error(), "从未登记的地方") {
		t.Fatalf("the refusal does not name the place: %v", err)
	}
	if _, err := application.PlacePicture(storyID, "从未登记的地方"); err == nil {
		t.Fatal("reading an unregistered place was accepted")
	}
}

// TestAPlacePictureAnswersTheProseLocation is the resolution rule at the service
// boundary: the page may hand over the scene's own location text rather than the
// catalog's spelling, and the picture still binds to the registered place. It also
// keeps the state's Place field honest, because that is what the page passes on
// the next request.
func TestAPlacePictureAnswersTheProseLocation(t *testing.T) {
	application, storyID, place := placePictureFixture(t)

	imported, err := application.PlacePictureImport(PlacePictureImport{
		StoryID: storyID, Place: place + "东街", Filename: "street.png", Data: testPNG(t),
	})
	if err != nil {
		t.Fatalf("import through the prose location: %v", err)
	}
	if got := imported.Place; got != place {
		t.Fatalf("the import bound %q, want the registered place %q", got, place)
	}
	if got := imported.Picture.Place; got != place {
		t.Fatalf("the reported state names %q, want %q", got, place)
	}
	// The prose location reads back the same picture, so a narrator adding a
	// detail cannot make the picture disappear.
	read, err := application.PlacePicture(storyID, place+"东街")
	if err != nil {
		t.Fatalf("read through the prose location: %v", err)
	}
	if read.URL != imported.Picture.URL {
		t.Fatalf("the prose location drew %q, the import reported %q", read.URL, imported.Picture.URL)
	}
}

// TestClearingAPlaceWithNoPictureIsNotAnError keeps the editor's button honest:
// asking to restore a place that has nothing imported reports the place as it
// already stands instead of failing.
func TestClearingAPlaceWithNoPictureIsNotAnError(t *testing.T) {
	application, storyID, place := placePictureFixture(t)

	result, err := application.PlacePictureClear(PlacePictureImport{StoryID: storyID, Place: place})
	if err != nil {
		t.Fatalf("clearing a place with no picture failed: %v", err)
	}
	if result.Picture.URL != "" || result.Picture.Overridden {
		t.Fatalf("the clear invented a picture: %+v", result.Picture)
	}
}

// recordLocalPlace writes one place record the way the model does: a whole
// object under the field the handbook names, on the world entities Actor.
func recordLocalPlace(t *testing.T, application *App, storyID, place string) {
	t.Helper()
	if _, err := recordPlaceTurn(t, application, storyID, "main", place); err != nil {
		t.Fatalf("record the place %q: %v", place, err)
	}
}

// recordPlaceTurn is recordLocalPlace that says which branch it wrote and hands
// back the turn, so a test can fork at it.
func recordPlaceTurn(t *testing.T, application *App, storyID, branchID, place string) (interactive.TurnEvent, error) {
	t.Helper()
	turn, _, err := application.interactive.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID:  branchID,
		User:      "我沿着河往西走。",
		Narrative: "你走到一处没名字的渡口，船家说这里叫" + place + "。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"上船", "问船家", "沿河继续走", "在渡口歇一晚", "掉头回集市"},
			StateUpdates: []interactivestate.Update{{
				Op: interactivestate.Replace, Path: "/world/地点记录",
				Value: map[string]any{place: map[string]any{
					"类型": "渡口", "概况": "河湾里的一处小渡口。",
				}},
			}},
		},
	})
	return turn, err
}

// TestAPlaceTheStoryRegisteredItselfGetsAPicture is the case the library's 本局
// 新建 page creates: the narrator invents a place, the rules record it, and the
// page offers a picture for it. Refusing the import would make the setting
// visible and always failing, which is why the record counts as a target.
func TestAPlaceTheStoryRegisteredItselfGetsAPicture(t *testing.T) {
	application, storyID, _ := placePictureFixture(t)
	const place = "落霞集"
	recordLocalPlace(t, application, storyID, place)

	imported, err := application.PlacePictureImport(PlacePictureImport{
		StoryID: storyID, Place: place, Filename: "river.png", Data: testPNG(t),
	})
	if err != nil {
		t.Fatalf("import a picture for a place the story registered: %v", err)
	}
	if got := imported.Asset.Place; got != place {
		t.Fatalf("the entry names %q, want %q", got, place)
	}
	// The scope is what keeps two stories that each invented 落霞集 apart.
	if got := imported.Asset.Scope; got != storyID {
		t.Fatalf("the entry is scoped to %q, want the story %q", got, storyID)
	}

	read, err := application.PlacePicture(storyID, place)
	if err != nil {
		t.Fatalf("read the picture back: %v", err)
	}
	if read.URL != imported.Picture.URL {
		t.Fatalf("the window would draw %q, the import reported %q", read.URL, imported.Picture.URL)
	}

	// The prose location resolves to the recorded place, so a narrator adding a
	// detail does not lose the picture.
	read, err = application.PlacePicture(storyID, place+"外头的河滩")
	if err != nil {
		t.Fatalf("read through the prose location: %v", err)
	}
	if read.URL != imported.Picture.URL {
		t.Fatalf("the prose location drew %q, the import reported %q", read.URL, imported.Picture.URL)
	}
}

// TestASiblingBranchDoesNotKnowAPlaceItNeverSaw is the branch half of the same
// rule: a picture binds to a place the branch's own world record holds, and a
// branch forked before that record was written must not resolve it. Otherwise the
// library would offer a place this timeline never reached.
func TestASiblingBranchDoesNotKnowAPlaceItNeverSaw(t *testing.T) {
	application, storyID, _ := placePictureFixture(t)

	first, err := recordPlaceTurn(t, application, storyID, "main", "青石渡口")
	if err != nil {
		t.Fatalf("record the first place: %v", err)
	}
	recordLocalPlace(t, application, storyID, "落霞集")

	branch, err := application.interactive.CreateBranch(storyID, interactive.CreateBranchRequest{
		ParentEventID: first.ID, Title: "只在青石渡口那一支",
	})
	if err != nil {
		t.Fatalf("fork before the second place: %v", err)
	}
	// CreateBranch switches to the new branch, so every lookup below answers for
	// it: this branch's record still says 青石渡口 and never heard of 落霞集.
	if _, err := application.PlacePicture(storyID, "落霞集"); err == nil {
		t.Fatalf("branch %s resolved a place it never registered", branch.ID)
	}
	// The place it did register still resolves, so the refusal above is about the
	// record and not about forks being unable to hold places at all.
	if _, err := application.PlacePicture(storyID, "青石渡口"); err != nil {
		t.Fatalf("the branch cannot resolve the place it did register: %v", err)
	}
}

// the media layer: two stories that each invented 落霞集 made up the same name
// independently, so one story's picture must not answer for the other.
func TestASiblingStorysPlaceOfTheSameNameKeepsItsOwnPicture(t *testing.T) {
	application, storyID, _ := placePictureFixture(t)
	service := &InteractiveAppService{app: application}
	pack, err := storypack.NewLibrary(application.cfg.DenovaDir).Get("fanren-jie")
	if err != nil {
		t.Fatalf("read the 凡人 pack: %v", err)
	}
	sibling, err := service.CreateInteractiveStoryContext(t.Context(), interactive.CreateStoryRequest{
		Title: "凡人另一界", Origin: "越国公共坊市的外围。",
		StateSchemaPolicy: &interactive.StoryStateSchemaPolicy{Mode: interactive.StoryStateSchemaModeFixedTemplate},
		PackBinding:       pack.Binding(),
	})
	if err != nil {
		t.Fatalf("create the sibling story: %v", err)
	}
	const place = "落霞集"
	recordLocalPlace(t, application, storyID, place)
	recordLocalPlace(t, application, sibling.ID, place)

	first, err := application.PlacePictureImport(PlacePictureImport{
		StoryID: storyID, Place: place, Filename: "a.png", Data: testPNG(t),
	})
	if err != nil {
		t.Fatalf("import the first story's picture: %v", err)
	}
	second, err := application.PlacePictureImport(PlacePictureImport{
		StoryID: sibling.ID, Place: place, Filename: "b.png", Data: testPNG(t),
	})
	if err != nil {
		t.Fatalf("import the sibling's picture: %v", err)
	}
	if first.Asset.ID == second.Asset.ID {
		t.Fatal("the two stories' pictures share one manifest entry, so one replaced the other")
	}

	readFirst, err := application.PlacePicture(storyID, place)
	if err != nil {
		t.Fatalf("read the first story's picture: %v", err)
	}
	if readFirst.URL != first.Picture.URL {
		t.Fatalf("the first story draws %q, its own import reported %q", readFirst.URL, first.Picture.URL)
	}
	readSecond, err := application.PlacePicture(sibling.ID, place)
	if err != nil {
		t.Fatalf("read the sibling's picture: %v", err)
	}
	if readSecond.URL != second.Picture.URL {
		t.Fatalf("the sibling draws %q, its own import reported %q", readSecond.URL, second.Picture.URL)
	}

	// Clearing one leaves the other standing.
	if _, err := application.PlacePictureClear(PlacePictureImport{StoryID: storyID, Place: place}); err != nil {
		t.Fatalf("clear the first story's picture: %v", err)
	}
	readSecond, err = application.PlacePicture(sibling.ID, place)
	if err != nil {
		t.Fatalf("read the sibling after the first was cleared: %v", err)
	}
	if readSecond.URL != second.Picture.URL {
		t.Fatalf("clearing one story's place picture removed the sibling's: %q", readSecond.URL)
	}
}
