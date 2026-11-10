package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ta/internal/agents/configresource"
	"ta/internal/interactive"
	"ta/internal/presetlayout"
)

// writeScopedSettingsPack installs one world pack under the data root, which is
// where the world page saves its defaults and where the assistant must write.
func writeScopedSettingsPack(t *testing.T, dataRoot, id, name, direction string) {
	t.Helper()
	packPath := filepath.Join(presetlayout.StoryPacks(dataRoot), id+".json")
	if err := os.MkdirAll(filepath.Dir(packPath), 0o755); err != nil {
		t.Fatal(err)
	}
	document := `{
  "version": 1,
  "id": "` + id + `",
  "name": "` + name + `",
  "description": "` + name + `的世界。",
  "story": {"title": "` + name + `", "origin": "开场。"},
  "settings": {
    "planning_mode": "disabled",
    "style_purposes": {"main": "classic"},
    "reply_target_chars": 900,
    "narrative_requirements": "` + direction + `"
  }
}`
	if err := os.WriteFile(packPath, []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
}

// scopedSettingsFixture builds an application with two worlds and one story
// started from each, so a cross-record write would be visible in the other.
func scopedSettingsFixture(t *testing.T) (agentScopedSettings, string, string) {
	t.Helper()
	application := newExecutionProfileTestApp(t)
	dataRoot := application.cfg.DataDir()
	writeScopedSettingsPack(t, dataRoot, "world-a", "World A", "world A direction")
	writeScopedSettingsPack(t, dataRoot, "world-b", "World B", "world B direction")

	create := func(packID string) string {
		pack, err := application.StoryPacks().Get(packID)
		if err != nil {
			t.Fatalf("read pack %q: %v", packID, err)
		}
		request := pack.StoryRequest()
		request.Title = packID + " story"
		story, err := application.CreateInteractiveStory(request)
		if err != nil {
			t.Fatalf("start a story from %q: %v", packID, err)
		}
		return story.ID
	}
	return agentScopedSettings{app: application}, create("world-a"), create("world-b")
}

// TestScopedSettingsWriterEditsTheRealStoryAndWorldRecords is the app-layer
// half of the isolation fix: the writer a scoped assistant receives is not a
// side store, so a change it accepts is visible to the Game settings page and
// the world page, and only on the record it named.
func TestScopedSettingsWriterEditsTheRealStoryAndWorldRecords(t *testing.T) {
	writer, storyA, storyB := scopedSettingsFixture(t)
	ctx := context.Background()

	read, err := writer.ReadStorySettings(ctx, storyA)
	if err != nil {
		t.Fatalf("read story A settings: %v", err)
	}
	replyLength := 640
	updated, err := writer.UpdateStorySettings(ctx, storyA, configresource.StorySettingsValue{
		Revision: read.Revision, ReplyTargetChars: &replyLength,
	})
	if err != nil {
		t.Fatalf("update story A settings: %v", err)
	}
	if updated.ReplyTargetChars == nil || *updated.ReplyTargetChars != 640 {
		t.Fatalf("updated reply length = %v, want 640", updated.ReplyTargetChars)
	}
	if updated.Revision == read.Revision {
		t.Fatal("the revision did not move after an accepted edit")
	}

	// The change is in the story record the Game settings page reads.
	index, err := writer.app.InteractiveStories()
	if err != nil {
		t.Fatal(err)
	}
	for _, story := range index.Stories {
		switch story.ID {
		case storyA:
			if story.ReplyTargetChars != 640 {
				t.Fatalf("story A reply length = %d, want 640", story.ReplyTargetChars)
			}
		case storyB:
			if story.ReplyTargetChars == 640 {
				t.Fatal("editing story A changed story B")
			}
		}
	}

	// The world's new-game defaults are a separate record and stay put.
	defaults, err := writer.ReadWorldDefaults(ctx, "world-a")
	if err != nil {
		t.Fatalf("read world A defaults: %v", err)
	}
	if defaults.ReplyTargetChars == nil || *defaults.ReplyTargetChars != 900 {
		t.Fatalf("world A defaults changed with a story edit: %v", defaults.ReplyTargetChars)
	}
}

// TestScopedSettingsWriterWritesOneWorldsDefaultsOnly is the world half: a
// save names one world, lands in that world's overlay, and leaves the
// neighbouring world alone.
func TestScopedSettingsWriterWritesOneWorldsDefaultsOnly(t *testing.T) {
	writer, _, _ := scopedSettingsFixture(t)
	ctx := context.Background()

	read, err := writer.ReadWorldDefaults(ctx, "world-a")
	if err != nil {
		t.Fatalf("read world A defaults: %v", err)
	}
	replyLength := 1500
	saved, err := writer.UpdateWorldDefaults(ctx, "world-a", configresource.WorldDefaultsValue{
		ReplyTargetChars: &replyLength,
	}, read.Revision)
	if err != nil {
		t.Fatalf("update world A defaults: %v", err)
	}
	if saved.ReplyTargetChars == nil || *saved.ReplyTargetChars != 1500 {
		t.Fatalf("saved reply length = %v, want 1500", saved.ReplyTargetChars)
	}

	other, err := writer.ReadWorldDefaults(ctx, "world-b")
	if err != nil {
		t.Fatal(err)
	}
	if other.ReplyTargetChars == nil || *other.ReplyTargetChars != 900 {
		t.Fatalf("world B defaults changed to %v", other.ReplyTargetChars)
	}
}

// A world that still stores the retired creative-direction text keeps it. The
// surface no longer carries the field, so the only way it could be lost is a
// whole-block write that starts from an empty record instead of the stored one.
func TestScopedSettingsWriterKeepsRetiredTextItNoLongerExposes(t *testing.T) {
	writer, _, _ := scopedSettingsFixture(t)
	ctx := context.Background()

	read, err := writer.ReadWorldDefaults(ctx, "world-a")
	if err != nil {
		t.Fatalf("read world A defaults: %v", err)
	}
	replyLength := 1500
	if _, err := writer.UpdateWorldDefaults(ctx, "world-a", configresource.WorldDefaultsValue{
		ReplyTargetChars: &replyLength,
	}, read.Revision); err != nil {
		t.Fatalf("update world A defaults: %v", err)
	}

	pack, err := writer.app.StoryPacks().Get("world-a")
	if err != nil {
		t.Fatal(err)
	}
	if pack.Settings.NarrativeRequirements != "world A direction" {
		t.Fatalf("stored text was erased by an unrelated save: %q", pack.Settings.NarrativeRequirements)
	}
}

// TestScopedSettingsWriterRefusesAStaleStoryRevision keeps the compare-and-swap
// the page relies on: an edit made between the read and the write is reported,
// not silently overwritten.
func TestScopedSettingsWriterRefusesAStaleStoryRevision(t *testing.T) {
	writer, storyA, _ := scopedSettingsFixture(t)
	ctx := context.Background()

	read, err := writer.ReadStorySettings(ctx, storyA)
	if err != nil {
		t.Fatal(err)
	}
	first := 400
	if _, err := writer.UpdateStorySettings(ctx, storyA, configresource.StorySettingsValue{
		Revision: read.Revision, ReplyTargetChars: &first,
	}); err != nil {
		t.Fatal(err)
	}
	second := 500
	if _, err := writer.UpdateStorySettings(ctx, storyA, configresource.StorySettingsValue{
		Revision: read.Revision, ReplyTargetChars: &second,
	}); err == nil {
		t.Fatal("an update carrying the pre-edit revision was accepted")
	}
	current, err := writer.ReadStorySettings(ctx, storyA)
	if err != nil {
		t.Fatal(err)
	}
	if current.ReplyTargetChars == nil || *current.ReplyTargetChars != 400 {
		t.Fatalf("reply length = %v, want the first edit's 400", current.ReplyTargetChars)
	}
}

// TestScopedSettingsWriterRefusesAnUnknownRecord is the refusal that keeps a
// wrong identifier from becoming a write somewhere else.
func TestScopedSettingsWriterRefusesAnUnknownRecord(t *testing.T) {
	writer, _, _ := scopedSettingsFixture(t)
	ctx := context.Background()

	if _, err := writer.ReadStorySettings(ctx, "story-missing"); err == nil {
		t.Fatal("reading an unknown story succeeded")
	}
	replyLength := 100
	if _, err := writer.UpdateStorySettings(ctx, "story-missing", configresource.StorySettingsValue{ReplyTargetChars: &replyLength}); err == nil {
		t.Fatal("writing an unknown story succeeded")
	}
	if _, err := writer.ReadWorldDefaults(ctx, "world-missing"); err == nil {
		t.Fatal("reading an unknown world succeeded")
	}
	if _, err := writer.UpdateWorldDefaults(ctx, "world-missing", configresource.WorldDefaultsValue{ReplyTargetChars: &replyLength}, ""); err == nil {
		t.Fatal("writing an unknown world succeeded")
	}
}

// TestScopedSettingsStoryUpdateDoesNotTouchAnotherStorysWorld pins the two
// records apart in the direction that matters most: editing the story in play
// must not move the defaults a new game from another world would begin with.
func TestScopedSettingsStoryUpdateDoesNotTouchAnotherStorysWorld(t *testing.T) {
	writer, storyA, _ := scopedSettingsFixture(t)
	ctx := context.Background()

	before, err := writer.ReadWorldDefaults(ctx, "world-b")
	if err != nil {
		t.Fatal(err)
	}
	read, err := writer.ReadStorySettings(ctx, storyA)
	if err != nil {
		t.Fatal(err)
	}
	replyLength := 2000
	if _, err := writer.UpdateStorySettings(ctx, storyA, configresource.StorySettingsValue{
		Revision: read.Revision, ReplyTargetChars: &replyLength,
	}); err != nil {
		t.Fatal(err)
	}
	after, err := writer.ReadWorldDefaults(ctx, "world-b")
	if err != nil {
		t.Fatal(err)
	}
	if before.Revision != after.Revision {
		t.Fatalf("a story edit moved another world's defaults revision from %s to %s", before.Revision, after.Revision)
	}
	if before.ReplyTargetChars == nil || after.ReplyTargetChars == nil || *before.ReplyTargetChars != *after.ReplyTargetChars {
		t.Fatalf("a story edit changed another world's defaults: %v -> %v", before.ReplyTargetChars, after.ReplyTargetChars)
	}
}

// TestScopedSettingsWriterUsesTheStoryPlanningModeField keeps the mapping
// honest: the writer reaches the same fields the Game settings page edits, so
// the assistant cannot claim a change the page would not show.
func TestScopedSettingsWriterUsesTheStoryPlanningModeField(t *testing.T) {
	writer, storyA, _ := scopedSettingsFixture(t)
	ctx := context.Background()

	read, err := writer.ReadStorySettings(ctx, storyA)
	if err != nil {
		t.Fatal(err)
	}
	mode := interactive.StoryPlanningModeEnabled
	if _, err := writer.UpdateStorySettings(ctx, storyA, configresource.StorySettingsValue{
		Revision: read.Revision, PlanningMode: &mode,
	}); err != nil {
		t.Fatal(err)
	}
	index, err := writer.app.InteractiveStories()
	if err != nil {
		t.Fatal(err)
	}
	for _, story := range index.Stories {
		if story.ID != storyA {
			continue
		}
		if story.PlanningMode != interactive.StoryPlanningModeEnabled {
			t.Fatalf("story planning mode = %q, want %q", story.PlanningMode, interactive.StoryPlanningModeEnabled)
		}
	}
}
