package interactive

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestLocalEntitySettingsSurviveColdReopen is the 本局新建 page's write path end
// to end: the player sets tags and a load mode on an entity, the settings land on
// the story, and a store opened fresh over the same directory reads them back by
// the same entity ID. Nothing here is a second record of the entity — the entity
// itself is still whatever the turns established.
func TestLocalEntitySettingsSurviveColdReopen(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	story, err := store.CreateStory(CreateStoryRequest{Title: "本局设置"})
	if err != nil {
		t.Fatal(err)
	}
	settings := map[string]LocalEntitySettings{
		"沈砚":        {Tags: []string{"药铺", "掌柜"}, LoadMode: LocalEntityLoadResident},
		"world:地点记录:落霞集": {LoadMode: LocalEntityLoadManual},
	}
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{LocalEntitySettings: settings}); err != nil {
		t.Fatal(err)
	}

	reopened := NewStore(dir)
	ctx, err := reopened.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	stored := ctx.Meta.LocalEntitySettings
	if len(stored) != 2 {
		t.Fatalf("stored settings = %+v", stored)
	}
	if got := stored["沈砚"]; got.LoadMode != LocalEntityLoadResident || strings.Join(got.Tags, ",") != "药铺,掌柜" {
		t.Fatalf("character settings = %+v", got)
	}
	if got := stored["world:地点记录:落霞集"].LoadMode; got != LocalEntityLoadManual {
		t.Fatalf("place load mode = %q", got)
	}

	// The summary the page reads carries the same settings, so the page does not
	// need a second request to learn what the turn already reads.
	index, err := reopened.Index()
	if err != nil {
		t.Fatal(err)
	}
	var summary StorySummary
	for _, candidate := range index.Stories {
		if candidate.ID == story.ID {
			summary = candidate
		}
	}
	if summary.LocalEntitySettings["沈砚"].LoadMode != LocalEntityLoadResident {
		t.Fatalf("summary settings = %+v", summary.LocalEntitySettings)
	}
}

// TestLocalEntitySettingsOmittedLeavesThemAlone pins the pointer semantics: a
// rename must not silently clear what the player filed.
func TestLocalEntitySettingsOmittedLeavesThemAlone(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "指针"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{LocalEntitySettings: map[string]LocalEntitySettings{
		"沈砚": {Tags: []string{"药铺"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{Title: "改个名"}); err != nil {
		t.Fatal(err)
	}
	ctx, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.Meta.LocalEntitySettings) != 1 {
		t.Fatalf("an omitted field changed the settings: %+v", ctx.Meta.LocalEntitySettings)
	}

	// An empty map is how the page drops every override, and that has to be
	// distinguishable from omitting the field.
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{LocalEntitySettings: map[string]LocalEntitySettings{}}); err != nil {
		t.Fatal(err)
	}
	ctx, err = store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.Meta.LocalEntitySettings) != 0 {
		t.Fatalf("an empty map did not clear the settings: %+v", ctx.Meta.LocalEntitySettings)
	}
}

// TestLocalEntitySettingsRejectUnknownLoadMode keeps an unreadable setting from
// being stored as though it worked.
func TestLocalEntitySettingsRejectUnknownLoadMode(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "非法取用"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdateStory(story.ID, UpdateStoryRequest{LocalEntitySettings: map[string]LocalEntitySettings{
		"沈砚": {LoadMode: "sometimes"},
	}})
	if err == nil {
		t.Fatal("an unknown load mode was accepted")
	}
	ctx, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.Meta.LocalEntitySettings) != 0 {
		t.Fatalf("a refused update still stored settings: %+v", ctx.Meta.LocalEntitySettings)
	}
}

// TestLocalEntitySettingsAreStoryScoped pins that the settings follow the story
// and not the branch: 常驻 is a reading preference for the work, and the same
// person exists on every branch, so a fork keeps what the player already decided.
func TestLocalEntitySettingsAreStoryScoped(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "跨分支"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "看看四周",
		Narrative: "街上人来人往。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{LocalEntitySettings: map[string]LocalEntitySettings{
		"沈砚": {LoadMode: LocalEntityLoadResident},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "另一条路"}); err != nil {
		t.Fatal(err)
	}
	ctx, err := store.StoryContext(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Meta.LocalEntitySettings["沈砚"].LoadMode != LocalEntityLoadResident {
		t.Fatalf("settings did not follow the story across a branch: %+v", ctx.Meta.LocalEntitySettings)
	}
}

// TestLocalEntitySettingsTagsKeepTheirCharacters is the Unicode side of the same
// cap. A Chinese label is 3 bytes per character, so a byte-length test and a rune
// slice disagree for every label between 11 and 32 characters: the rune slice
// reads past the short string into the zeroed tail of Go's conversion buffer and
// the tag comes back padded with NULs. The player sees a label that looks right
// and a file that holds something else.
func TestLocalEntitySettingsTagsKeepTheirCharacters(t *testing.T) {
	for _, c := range []struct {
		name string
		tag  string
	}{
		{"ten_han", strings.Repeat("中", 10)},
		{"eleven_han", strings.Repeat("中", 11)},
		{"thirty_one_han", strings.Repeat("中", 31)},
		{"thirty_two_han", strings.Repeat("中", 32)},
		{"thirty_three_han", strings.Repeat("中", 33)},
		{"thirty_three_ascii", strings.Repeat("a", 33)},
	} {
		t.Run(c.name, func(t *testing.T) {
			cleaned, keep, err := normalizeLocalEntitySettings(LocalEntitySettings{
				Tags:     []string{c.tag},
				LoadMode: LocalEntityLoadAuto,
			})
			if err != nil || !keep || len(cleaned.Tags) != 1 {
				t.Fatalf("label not saved: %+v %v", cleaned, err)
			}
			got := cleaned.Tags[0]
			if n := strings.Count(got, "\x00"); n != 0 {
				t.Errorf("saved label carries %d NUL runes: %q", n, got)
			}
			want := utf8.RuneCountInString(c.tag)
			if want > 32 {
				want = 32
			}
			if n := utf8.RuneCountInString(got); n != want {
				t.Errorf("saved %d characters, want %d", n, want)
			}
			if want < utf8.RuneCountInString(c.tag) && !strings.HasPrefix(c.tag, got) {
				t.Errorf("the cap did not cut a prefix: %q", got)
			}
		})
	}
}

// TestLocalEntitySettingsNormalizeTags covers the bounds a paste can hit.
func TestLocalEntitySettingsNormalizeTags(t *testing.T) {
	cleaned, keep, err := normalizeLocalEntitySettings(LocalEntitySettings{
		Tags: []string{"  药铺  ", "药铺", "", "   ", strings.Repeat("长", 40)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !keep {
		t.Fatal("a record with tags was dropped")
	}
	if len(cleaned.Tags) != 2 {
		t.Fatalf("tags = %+v, want the duplicate and the blanks dropped", cleaned.Tags)
	}
	if len([]rune(cleaned.Tags[1])) != 32 {
		t.Fatalf("a long tag was not capped: %d runes", len([]rune(cleaned.Tags[1])))
	}

	empty, keep, err := normalizeLocalEntitySettings(LocalEntitySettings{})
	if err != nil {
		t.Fatal(err)
	}
	if keep || len(empty.Tags) != 0 || empty.LoadMode != "" {
		t.Fatalf("an empty record was kept: %+v", empty)
	}
}

// TestResidentEntitySurvivesTheCapWithoutJoiningTheScene is the "常驻只影响上下文，
// 不等于在场或强制出场" rule stated as an assertion. A 常驻 entity is read even
// when the cap would otherwise drop it, and the scene's own 在场 list does not
// learn about them.
func TestResidentEntitySurvivesTheCapWithoutJoiningTheScene(t *testing.T) {
	system := defaultActorStateSystem()
	state := map[string]any{}
	actors := map[string]any{}
	for _, actor := range defaultActorStateInitialActors() {
		actors[actor.ID] = map[string]any{"name": actor.Name, "template_id": actor.TemplateID}
	}
	// Filler that sorts before the resident, so ID order alone would drop them.
	for i := 0; i < maxInteractiveListItems+8; i++ {
		id := fmt.Sprintf("AAA%02d", i)
		actors[id] = map[string]any{"name": id, "template_id": ActorStateImportantCharacterTemplateID}
	}
	const resident = "ZZZ沈砚"
	actors[resident] = map[string]any{"name": "沈砚", "template_id": ActorStateImportantCharacterTemplateID}
	state[actorStateRoot] = actors

	projected := ActorStateRuntimeProjectionWithSettings(system, state, map[string]string{
		resident: LocalEntityLoadResident,
	})
	kept, _ := projected[actorStateRoot].(map[string]any)
	if _, ok := kept[resident]; !ok {
		t.Fatalf("the projection dropped the 常驻 entity: kept %d of %d", len(kept), len(actors))
	}
	present := StorySceneFromState(projected, "").Present
	for _, id := range present {
		if id == resident {
			t.Fatalf("the 常驻 entity was staged into the scene: present = %v", present)
		}
	}
}

// TestManualEntityIsWithheldUnlessOnStage is the other half of the same rule:
// 指定取用 withholds the record until the scene asks for the person.
func TestManualEntityIsWithheldUnlessOnStage(t *testing.T) {
	system := defaultActorStateSystem()
	state := map[string]any{}
	actors := map[string]any{}
	for _, actor := range defaultActorStateInitialActors() {
		actors[actor.ID] = map[string]any{"name": actor.Name, "template_id": actor.TemplateID}
	}
	const offStage = "沈砚"
	actors[offStage] = map[string]any{"name": "沈砚", "template_id": ActorStateImportantCharacterTemplateID}
	state[actorStateRoot] = actors

	projected := ActorStateRuntimeProjectionWithSettings(system, state, map[string]string{
		offStage: LocalEntityLoadManual,
	})
	kept, _ := projected[actorStateRoot].(map[string]any)
	if _, ok := kept[offStage]; ok {
		t.Fatal("a 指定取用 entity that is not on stage was still read")
	}
	// The foundational Actors are never withheld, whatever the settings say.
	for _, id := range []string{DefaultActorID, DefaultStoryContextActorID, DefaultWorldEntitiesActorID} {
		if _, ok := kept[id]; !ok {
			t.Fatalf("a 指定取用 override withheld the foundational Actor %q", id)
		}
	}

	// Once the scene places them on stage, they are read again. The scene lives
	// under the story-context Actor's own state, which is where every turn writes
	// 在场角色.
	onStage := cloneActorStateRoot(state)
	storyActor, _ := onStage[actorStateRoot].(map[string]any)[DefaultStoryContextActorID].(map[string]any)
	storyActor["state"] = map[string]any{storyScenePresentField: []any{offStage}}
	projected = ActorStateRuntimeProjectionWithSettings(system, onStage, map[string]string{
		offStage: LocalEntityLoadManual,
	})
	kept, _ = projected[actorStateRoot].(map[string]any)
	if _, ok := kept[offStage]; !ok {
		t.Fatal("a 指定取用 entity on stage was withheld")
	}
}
