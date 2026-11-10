package storypack

import (
	"os"
	"path/filepath"
	"testing"

	"ta/internal/interactive"
	"ta/internal/style"
)

func TestBuiltinPacksAreValidAndDistinct(t *testing.T) {
	packs := builtinPacks()
	if len(packs) < 1 {
		t.Fatalf("expected at least one builtin pack, got %d", len(packs))
	}
	seen := map[string]bool{}
	for _, pack := range packs {
		if err := pack.Validate(); err != nil {
			t.Fatalf("builtin pack %s is invalid: %v", pack.ID, err)
		}
		if !pack.Builtin {
			t.Fatalf("builtin pack %s did not carry the builtin marker", pack.ID)
		}
		if seen[pack.ID] {
			t.Fatalf("duplicate builtin pack id: %s", pack.ID)
		}
		seen[pack.ID] = true
	}
	// 凡人人界 is the delivered world; a build without it has nothing to play.
	if !seen[builtinPackIDFanren] {
		t.Fatalf("the delivered world %s is missing from the builtins", builtinPackIDFanren)
	}
}

func TestBuiltinPacksResolveToDistinctGameplay(t *testing.T) {
	packs := builtinPacks()
	ruleSystems := map[string]bool{}
	for _, pack := range packs {
		request := pack.StoryRequest()
		if request.Title == "" || request.Origin == "" {
			t.Fatalf("pack %s produced an incomplete story request", pack.ID)
		}
		if request.ModuleRefs == nil || request.ModuleRefs.RuleSystemID == "" {
			t.Fatalf("pack %s did not bind a rule system", pack.ID)
		}
		ruleSystems[request.ModuleRefs.RuleSystemID] = true
		if request.Opening.Mode != "ai" || request.Opening.CustomText != "" {
			t.Fatalf("pack %s should open on the model-written path, got %+v", pack.ID, request.Opening)
		}
		if request.Protagonist.Mode != "custom" || request.Protagonist.Name == "" {
			t.Fatalf("pack %s should bind a named protagonist", pack.ID)
		}
		if len(pack.WorldInputs()) != len(pack.World) || len(pack.World) == 0 {
			t.Fatalf("pack %s world book did not convert", pack.ID)
		}
	}
	// Two packs that resolve to the same rule system would not be independently
	// playable worlds.
	if len(ruleSystems) != len(packs) {
		t.Fatalf("packs must bind distinct rule systems, got %v", ruleSystems)
	}
}

// TestFanrenPackBindsTheCultivationStateSystem guards the one binding that makes
// the cultivation numbers reachable from the ordinary pack entry. The pack is a
// cultivation world; while it bound the generic state system, a player who took
// the one-click pack path got no 境界/灵力/攻防 panel and no cultivation skill
// guidance, so the batch's cultivation work was only visible to someone who
// opened 高级设置 and switched 角色状态系统 by hand.
func TestFanrenPackBindsTheCultivationStateSystem(t *testing.T) {
	var fanren *Pack
	for _, pack := range builtinPacks() {
		if pack.ID == "fanren-jie" {
			copied := pack
			fanren = &copied
		}
	}
	if fanren == nil {
		t.Fatal("the fanren-jie builtin pack is missing")
	}
	refs := fanren.StoryRequest().ModuleRefs
	if refs == nil {
		t.Fatal("fanren-jie did not produce module refs")
	}
	root := t.TempDir()
	if err := fanren.ValidateModules(root); err != nil {
		t.Fatal(err)
	}
	module, err := interactive.NewActorStateLibrary(root).Get(refs.ActorStateID)
	if err != nil {
		t.Fatal(err)
	}
	if !interactive.ActorStateSystemIsXiuxian(module.ActorState) || module.ActorState.WorldContent == nil || module.ActorState.WorldContent.PackID != fanren.ID {
		t.Fatal("fanren did not resolve its own cultivation module and world content")
	}

}

func TestStoryRequestAppliesPanelDefaults(t *testing.T) {
	pack := Pack{
		Version: 1,
		ID:      "sparse",
		Name:    "sparse",
		Story: PackStory{
			Title:  "T",
			Origin: "O",
		},
	}
	request := pack.StoryRequest()
	// A pack that states no settings leaves every field at the application's
	// fallback, exactly as a hand-created story would. In particular planning is
	// on and the default style is used, so a sparse pack never silently disables
	// planning or forces a voice of its own.
	if request.StoryTellerID != style.DefaultID {
		t.Fatalf("a settings-less pack should use the default style %q, got %q", style.DefaultID, request.StoryTellerID)
	}
	if request.PlanningMode != interactive.StoryPlanningModeEnabled {
		t.Fatalf("a settings-less pack should leave planning enabled, got %q", request.PlanningMode)
	}
	if request.PlanningTemplateID != interactive.DefaultGamePlanningTemplateID {
		t.Fatalf("planning template default = %q", request.PlanningTemplateID)
	}
	if request.ModuleRefs.NarrativeStyleID != "" {
		t.Fatalf("a pack bound a narrative style through the module graph: %q", request.ModuleRefs.NarrativeStyleID)
	}
	if len(request.ModuleRefs.EventPackageIDs) != 1 || request.ModuleRefs.EventPackageIDs[0] != DefaultEventPackageID {
		t.Fatalf("event package default = %v", request.ModuleRefs.EventPackageIDs)
	}
	if request.ReplyTargetChars != DefaultReplyTargetChars {
		t.Fatalf("reply target default = %d", request.ReplyTargetChars)
	}
	if request.ChoiceCount != DefaultChoiceCount {
		t.Fatalf("choice count default = %d", request.ChoiceCount)
	}
	if request.Opening.Mode != "ai" {
		t.Fatalf("an opening-less pack should let the model write the first scene, got %q", request.Opening.Mode)
	}
}

// TestPackSettingsReachTheStoryRequest covers the settings block the world page
// edits: the values a pack declares are what a new game of that world starts
// with, rather than the old hard-coded planning-off and no-style behaviour.
func TestPackSettingsReachTheStoryRequest(t *testing.T) {
	pack := Pack{
		Version: 1,
		ID:      "configured",
		Name:    "configured",
		Story:   PackStory{Title: "T", Origin: "O"},
		Settings: PackSettings{
			PlanningMode:       interactive.StoryPlanningModeDisabled,
			PlanningTemplateID: "directed-longform",
			StylePurposes: PackStylePurposes{
				Main:     "source-drama",
				Dialogue: "marketplace-light",
			},
			ReplyTargetChars: 1200,
			CheckSettings: PackCheckSettings{
				DifficultyShift:          -1,
				RollModifier:             5,
				RuleStateConsumptionMode: interactive.RuleStateConsumptionModeSuggestionsOnly,
				RuleVisibilityMode:       interactive.RuleVisibilityModePublicRoll,
			},
		},
	}
	request := pack.StoryRequest()
	if request.PlanningMode != interactive.StoryPlanningModeDisabled {
		t.Fatalf("planning mode = %q, want the pack's own disabled", request.PlanningMode)
	}
	if request.PlanningTemplateID != "directed-longform" {
		t.Fatalf("planning template = %q", request.PlanningTemplateID)
	}
	if request.StoryTellerID != "source-drama" {
		t.Fatalf("main style = %q", request.StoryTellerID)
	}
	if request.StylePurposes.Dialogue != "marketplace-light" || request.StylePurposes.Combat != "" {
		t.Fatalf("style purposes = %+v; an unset combat must follow the main narrative", request.StylePurposes)
	}
	if request.ReplyTargetChars != 1200 {
		t.Fatalf("reply target = %d", request.ReplyTargetChars)
	}
	if request.CheckSettings.DifficultyShift != -1 || request.CheckSettings.RollModifier != 5 ||
		request.CheckSettings.RuleStateConsumptionMode != interactive.RuleStateConsumptionModeSuggestionsOnly ||
		request.CheckSettings.RuleVisibilityMode != interactive.RuleVisibilityModePublicRoll {
		t.Fatalf("check settings = %+v", request.CheckSettings)
	}
}

// TestTwoPacksCanDefaultToDifferentSettings is the acceptance check for pack
// defaults: two worlds are allowed to start new games differently, which is what
// makes a default belong to the pack rather than to the application.
func TestTwoPacksCanDefaultToDifferentSettings(t *testing.T) {
	base := Pack{Version: 1, Name: "n", Story: PackStory{Title: "T", Origin: "O"}}
	first := base
	first.ID = "first"
	first.Settings = PackSettings{PlanningMode: interactive.StoryPlanningModeEnabled, StylePurposes: PackStylePurposes{Main: "source-drama"}}
	second := base
	second.ID = "second"
	second.Settings = PackSettings{PlanningMode: interactive.StoryPlanningModeDisabled, StylePurposes: PackStylePurposes{Main: "straight-comedy"}}

	firstRequest := first.StoryRequest()
	secondRequest := second.StoryRequest()
	if firstRequest.PlanningMode == secondRequest.PlanningMode {
		t.Fatalf("both packs resolved the same planning mode: %q", firstRequest.PlanningMode)
	}
	if firstRequest.StoryTellerID == secondRequest.StoryTellerID {
		t.Fatalf("both packs resolved the same style: %q", firstRequest.StoryTellerID)
	}
}

func TestLibraryListPrefersUserPackOverBuiltin(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "presets", "story-packs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	override := `{"version":1,"id":"fanren-jie","name":"自定义凡人人界","story":{"title":"X","origin":"Y"}}`
	if err := os.WriteFile(filepath.Join(dir, "fanren-jie.json"), []byte(override), 0o644); err != nil {
		t.Fatal(err)
	}

	summaries, err := NewLibrary(root).List()
	if err != nil {
		t.Fatal(err)
	}
	var fanren *Summary
	count := 0
	for i := range summaries {
		if summaries[i].ID == "fanren-jie" {
			fanren = &summaries[i]
		}
		count++
	}
	if fanren == nil {
		t.Fatal("overridden pack disappeared from the list")
	}
	if fanren.Name != "自定义凡人人界" || fanren.Builtin {
		t.Fatalf("user pack did not override the builtin: %+v", *fanren)
	}
	if count != len(builtinPacks())-1 {
		t.Fatalf("override should replace rather than duplicate: %d entries", count)
	}

	pack, err := NewLibrary(root).Get("fanren-jie")
	if err != nil {
		t.Fatal(err)
	}
	if pack.Name != "自定义凡人人界" || pack.Builtin {
		t.Fatalf("Get returned the builtin instead of the override: %+v", pack)
	}
}

func TestLibraryKeepsBrokenPackVisible(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "presets", "story-packs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	summaries, err := NewLibrary(root).List()
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range summaries {
		if summary.ID != "broken" {
			continue
		}
		if !summary.Invalid || summary.Error == "" {
			t.Fatalf("broken pack should report why it is unusable: %+v", summary)
		}
		return
	}
	t.Fatal("a malformed pack silently disappeared from the catalog")
}

func TestGetRejectsUnknownAndUnsafeIDs(t *testing.T) {
	root := t.TempDir()
	library := NewLibrary(root)
	if _, err := library.Get("does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown pack")
	}
	if _, err := library.Get("../escape"); err == nil {
		t.Fatal("expected an error for a path-traversing pack id")
	}
	if _, err := library.Get(""); err == nil {
		t.Fatal("expected an error for an empty pack id")
	}
}
