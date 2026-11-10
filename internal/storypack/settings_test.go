package storypack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ta/internal/interactive"
	"ta/internal/presetlayout"
)

// settingsPackDocument is a pack with an authored settings block, so a test can
// tell the author's default apart from the player's saved one.
func settingsPackDocument(id, name, planningMode, style string) string {
	return `{
  "version": 1,
  "id": "` + id + `",
  "name": "` + name + `",
  "story": {"title": "` + name + `", "origin": "开场。"},
  "settings": {
    "planning_mode": "` + planningMode + `",
    "style_purposes": {"main": "` + style + `"},
    "reply_target_chars": 900
  }
}`
}

// TestSavedSettingsOverrideTheAuthorsDefaultsAndLeaveThePackFileAlone covers the
// write path the world page uses: the player's defaults win, they are stored
// beside the pack rather than in it, and the author's file is byte-identical
// afterwards.
func TestSavedSettingsOverrideTheAuthorsDefaultsAndLeaveThePackFileAlone(t *testing.T) {
	dataRoot := t.TempDir()
	writePackFile(t, dataRoot, "authored.json", settingsPackDocument("authored", "作者的世界", "enabled", "source-drama"))
	packPath := filepath.Join(presetlayout.StoryPacks(dataRoot), "authored.json")
	before, err := os.ReadFile(packPath)
	if err != nil {
		t.Fatal(err)
	}

	library := NewLibrary(dataRoot)
	base, customized := library.SettingsRevision("authored")
	if customized {
		t.Fatal("a pack with no saved defaults reported them as customized")
	}

	saved, err := library.SaveSettings(context.Background(), "authored", PackSettings{
		PlanningMode:     interactive.StoryPlanningModeDisabled,
		StylePurposes:    PackStylePurposes{Main: "straight-comedy", Dialogue: "deadpan-humor"},
		ReplyTargetChars: 1400,
	}, base)
	if err != nil {
		t.Fatalf("saving defaults: %v", err)
	}
	if saved.Revision == "" || saved.Revision == base {
		t.Fatalf("save did not produce a new revision: %q -> %q", base, saved.Revision)
	}
	if saved.Settings.PlanningMode != interactive.StoryPlanningModeDisabled ||
		saved.Settings.StylePurposes.Main != "straight-comedy" ||
		saved.Settings.ReplyTargetChars != 1400 {
		t.Fatalf("saved settings did not take effect: %+v", saved.Settings)
	}

	// The author's pack file is untouched, and the overlay is its own file.
	after, err := os.ReadFile(packPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("saving defaults rewrote the author's pack file")
	}
	if _, err := os.Stat(filepath.Join(presetlayout.StoryPacks(dataRoot), "authored.settings.json")); err != nil {
		t.Fatalf("the saved defaults were not written beside the pack: %v", err)
	}

	// A later read, and a new game, both see the player's values.
	reloaded, err := library.Get("authored")
	if err != nil {
		t.Fatal(err)
	}
	if request := reloaded.StoryRequest(); request.PlanningMode != interactive.StoryPlanningModeDisabled ||
		request.StoryTellerID != "straight-comedy" {
		t.Fatalf("a reloaded pack did not apply the saved defaults: planning=%q style=%q",
			request.PlanningMode, request.StoryTellerID)
	}

	// The overlay is not a pack and must not appear in the catalog.
	summaries, err := library.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range summaries {
		if strings.Contains(summary.ID, "settings") {
			t.Fatalf("the saved-defaults overlay was listed as a world: %s", summary.ID)
		}
		if summary.ID == "authored" {
			if !summary.SettingsCustomized {
				t.Fatal("the catalog did not report the saved defaults as customized")
			}
			if summary.Settings.PlanningMode != interactive.StoryPlanningModeDisabled {
				t.Fatalf("the catalog projected the author's defaults instead of the player's: %+v", summary.Settings)
			}
		}
	}
}

// TestSettingsSaveRejectsAStaleRevision covers the second-window case: a page
// that saved first must not be silently overwritten by one holding older bytes.
func TestSettingsSaveRejectsAStaleRevision(t *testing.T) {
	dataRoot := t.TempDir()
	writePackFile(t, dataRoot, "shared.json", settingsPackDocument("shared", "共享世界", "enabled", "source-drama"))
	library := NewLibrary(dataRoot)
	base, _ := library.SettingsRevision("shared")

	if _, err := library.SaveSettings(context.Background(), "shared", PackSettings{
		StylePurposes: PackStylePurposes{Main: "source-drama"},
	}, base); err != nil {
		t.Fatal(err)
	}

	_, err := library.SaveSettings(context.Background(), "shared", PackSettings{
		StylePurposes: PackStylePurposes{Main: "straight-comedy"},
	}, base)
	if err == nil {
		t.Fatal("a stale base revision overwrote newer defaults")
	}
	if !IsSettingsRevisionConflict(err) {
		t.Fatalf("the conflict was not reported as a revision conflict: %v", err)
	}
	pack, err := library.Get("shared")
	if err != nil {
		t.Fatal(err)
	}
	if pack.Settings.StylePurposes.Main != "source-drama" {
		t.Fatalf("the rejected save still changed the defaults: %q", pack.Settings.StylePurposes.Main)
	}
}

// TestAClearedDirectionStaysCleared covers the page's one destructive-looking
// edit: emptying the world's creative direction and saving. The author's text
// must not come back, because the page would then show an empty box while new
// games silently ran on text the player had removed.
func TestAClearedDirectionStaysCleared(t *testing.T) {
	dataRoot := t.TempDir()
	writePackFile(t, dataRoot, "direction.json", `{
  "version": 1,
  "id": "direction",
  "name": "带要求的世界",
  "story": {"title": "带要求的世界", "origin": "开场。", "narrative_requirements": "作者的要求。"}
}`)
	library := NewLibrary(dataRoot)

	before, err := library.Get("direction")
	if err != nil {
		t.Fatal(err)
	}
	if got := before.EffectiveSettings().NarrativeRequirements; got != "作者的要求。" {
		t.Fatalf("an unsaved pack did not use the author's direction: %q", got)
	}

	base, _ := library.SettingsRevision("direction")
	if _, err := library.SaveSettings(context.Background(), "direction", PackSettings{
		NarrativeRequirements: "",
	}, base); err != nil {
		t.Fatal(err)
	}

	after, err := library.Get("direction")
	if err != nil {
		t.Fatal(err)
	}
	if got := after.EffectiveSettings().NarrativeRequirements; got != "" {
		t.Fatalf("a cleared direction resurrected the author's text: %q", got)
	}
	if got := after.StoryRequest().NarrativeRequirements; got != "" {
		t.Fatalf("a new game would still start on the cleared text: %q", got)
	}
}

// TestSettingsValidationRejectsUnknownReferences keeps the page from saving a
// style or template that a new game could not actually use.
func TestSettingsValidationRejectsUnknownReferences(t *testing.T) {
	dataRoot := t.TempDir()
	writePackFile(t, dataRoot, "strict.json", settingsPackDocument("strict", "严格世界", "enabled", "source-drama"))
	library := NewLibrary(dataRoot)

	if _, err := library.SaveSettings(context.Background(), "strict", PackSettings{
		StylePurposes: PackStylePurposes{Main: "no-such-style"},
	}, ""); err == nil {
		t.Fatal("a style that is not installed was saved")
	}
	if _, err := library.SaveSettings(context.Background(), "strict", PackSettings{
		PlanningTemplateID: "no-such-template",
	}, ""); err == nil {
		t.Fatal("a planning template that is not installed was saved")
	}
	if _, err := library.SaveSettings(context.Background(), "strict", PackSettings{
		PlanningMode: "sometimes",
	}, ""); err == nil {
		t.Fatal("an unknown planning mode was saved")
	}
	if _, err := library.SaveSettings(context.Background(), "strict", PackSettings{
		CheckSettings: PackCheckSettings{DifficultyShift: 9},
	}, ""); err == nil {
		t.Fatal("an out-of-range difficulty shift was saved")
	}
}

// TestSettingsAreNotPartOfTheContentHash is the old-save protection. Settings are
// a preference for the next new game; if they moved the content hash, changing a
// default would derive a new lore scope, duplicate the world book, and make the
// discovery path refuse a catalog whose facts never changed. The story itself
// keeps the values it started with because creation copies them into its meta.
func TestSettingsAreNotPartOfTheContentHash(t *testing.T) {
	base := Pack{
		Version: 1, ID: "hash", Name: "hash",
		Story: PackStory{Title: "T", Origin: "O"},
	}
	changed := base
	changed.Settings = PackSettings{
		PlanningMode:       interactive.StoryPlanningModeDisabled,
		PlanningTemplateID: "directed-longform",
		StylePurposes:      PackStylePurposes{Main: "straight-comedy"},
		ReplyTargetChars:   800,
	}
	if base.ContentHash() != changed.ContentHash() {
		t.Fatal("changing settings moved the content hash and would fork the world book")
	}
	// What does change for the new game is the request itself.
	if base.StoryRequest().PlanningMode == changed.StoryRequest().PlanningMode {
		t.Fatal("settings did not change the story request")
	}
}

func TestRuleChoicePersistsWithoutReplacingWorldState(t *testing.T) {
	root := t.TempDir()
	lib := NewLibrary(root)
	pack, err := lib.Get("fanren-jie")
	if err != nil {
		t.Fatal(err)
	}
	recommended := pack.Story.ModuleRefs.RuleSystemID
	rev, _ := lib.SettingsRevision(pack.ID)
	settings := pack.EffectiveSettings()
	settings.RuleSystemID = "default"
	saved, err := lib.SaveSettings(context.Background(), pack.ID, settings, rev)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := lib.Get(pack.ID)
	if err != nil {
		t.Fatal(err)
	}
	req := reloaded.StoryRequest()
	if req.ModuleRefs.RuleSystemID != "default" || req.ModuleRefs.ActorStateID != pack.Story.ModuleRefs.ActorStateID || reloaded.Story.ModuleRefs.RuleSystemID != recommended {
		t.Fatal("choice replaced the world recommendation or state")
	}
	settings.RuleSystemID = interactive.RuleSystemDetectiveStatsID
	if _, err = lib.SaveSettings(context.Background(), pack.ID, settings, saved.Revision); err == nil {
		t.Fatal("incompatible rule system accepted")
	}
	settings.RuleSystemID = ""
	if _, err = lib.SaveSettings(context.Background(), pack.ID, settings, saved.Revision); err != nil {
		t.Fatal(err)
	}
	reloaded, err = lib.Get(pack.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.StoryRequest().ModuleRefs.RuleSystemID != recommended {
		t.Fatal("could not restore recommendation")
	}
}
