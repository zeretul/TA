package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ta/config"
	"ta/internal/interactive"
	"ta/internal/presetlayout"
	"ta/internal/storypack"
)

// TestAStartedStoryKeepsTheDefaultsItBeganWith drives the world page's save
// through the real creation path.
//
// The design's rule is that a world's settings are defaults for the next game,
// not a live switch: a story copies them at creation, so saving new defaults
// afterwards must change the next new game and leave an existing story on what
// it started with. It also pins that the page-visible summary and the started
// story agree, which is what "no checkbox without effect" means.
func TestAStartedStoryKeepsTheDefaultsItBeganWith(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	t.Cleanup(func() { _ = store.Close() })
	service := &InteractiveAppService{app: &App{
		workspace:   workspace,
		cfg:         &config.Config{DenovaDir: dataRoot},
		interactive: store,
	}}

	packPath := filepath.Join(presetlayout.StoryPacks(dataRoot), "defaults.json")
	document := `{
  "version": 1,
  "id": "defaults",
  "name": "默认世界",
  "description": "一个用来检查默认设置的世界。",
  "story": {"title": "默认世界", "origin": "开场。"},
  "settings": {
    "planning_mode": "disabled",
    "style_purposes": {"main": "classic"},
    "reply_target_chars": 900,
    "narrative_requirements": "作者的要求。"
  }
}`
	if err := os.MkdirAll(filepath.Dir(packPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packPath, []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}

	library := storypack.NewLibrary(dataRoot)
	firstPack, err := library.Get("defaults")
	if err != nil {
		t.Fatalf("read the pack: %v", err)
	}
	firstSummary, err := library.List()
	if err != nil {
		t.Fatalf("list packs: %v", err)
	}
	var shown storypack.Summary
	for _, summary := range firstSummary {
		if summary.ID == "defaults" {
			shown = summary
		}
	}
	if shown.Settings.PlanningMode != interactive.StoryPlanningModeDisabled ||
		shown.Settings.StylePurposes.Main != "classic" ||
		shown.Settings.NarrativeRequirements != "作者的要求。" {
		t.Fatalf("the catalog did not project the pack's defaults: %+v", shown.Settings)
	}

	first, err := service.CreateInteractiveStoryContext(context.Background(), firstPack.StoryRequest())
	if err != nil {
		t.Fatalf("start the first story: %v", err)
	}
	if first.PlanningMode != interactive.StoryPlanningModeDisabled ||
		first.StoryTellerID != "classic" ||
		first.ReplyTargetChars != 900 ||
		first.NarrativeRequirements != "作者的要求。" {
		t.Fatalf("the first story did not begin on the pack's defaults: %+v", first)
	}

	// The player changes the world's defaults after that story began.
	base, _ := library.SettingsRevision("defaults")
	saved, err := library.SaveSettings(context.Background(), "defaults", storypack.PackSettings{
		PlanningMode:          interactive.StoryPlanningModeEnabled,
		StylePurposes:         storypack.PackStylePurposes{Main: "screenwriter"},
		ReplyTargetChars:      2400,
		NarrativeRequirements: "玩家改过的要求。",
	}, base)
	if err != nil {
		t.Fatalf("save new defaults: %v", err)
	}
	if saved.Settings.StylePurposes.Main != "screenwriter" {
		t.Fatalf("the saved defaults did not take effect: %+v", saved.Settings)
	}

	reopened, err := store.StoryContext(first.ID, "")
	if err != nil {
		t.Fatalf("reopen the first story: %v", err)
	}
	if reopened.Meta.PlanningMode != interactive.StoryPlanningModeDisabled ||
		reopened.Meta.StoryTellerID != "classic" ||
		reopened.Meta.ReplyTargetChars != 900 ||
		reopened.Meta.NarrativeRequirements != "作者的要求。" {
		t.Fatalf("saving new defaults rewrote a story already under way: %+v", reopened.Meta)
	}

	// The next new game uses the new defaults.
	secondPack, err := library.Get("defaults")
	if err != nil {
		t.Fatalf("read the pack after saving defaults: %v", err)
	}
	second, err := service.CreateInteractiveStoryContext(context.Background(), secondPack.StoryRequest())
	if err != nil {
		t.Fatalf("start the second story: %v", err)
	}
	if second.PlanningMode != interactive.StoryPlanningModeEnabled ||
		second.StoryTellerID != "screenwriter" ||
		second.ReplyTargetChars != 2400 ||
		second.NarrativeRequirements != "玩家改过的要求。" {
		t.Fatalf("the next new game did not use the saved defaults: %+v", second)
	}

	// And the author's file is still the author's.
	after, err := os.ReadFile(packPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != document {
		t.Fatal("saving defaults rewrote the author's pack file")
	}
}
