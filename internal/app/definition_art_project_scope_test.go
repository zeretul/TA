package app

import (
	"encoding/json"
	"reflect"
	"strings"
	"ta/config"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"testing"
)

func TestDefinitionArtPublicReferencesCannotSelectAnotherProject(t *testing.T) {
	foreign := cultivation.DefinitionDetails{QualityGrade: cultivation.QualityPurple, Description: "Public text", Art: &cultivation.DefinitionArt{Source: "generated", ImagePath: "assets/image/generated/foreign.png", ProjectID: "other-project"}}
	before := foreign
	for _, projectID := range []string{"this-project", ""} {
		if got := projectDefinitionArt(foreign, projectID); got.Art != nil || got.QualityGrade != foreign.QualityGrade || got.Description != foreign.Description {
			t.Fatalf("foreign art not scoped away: %+v", got)
		}
	}
	if !reflect.DeepEqual(foreign, before) {
		t.Fatal("public projection mutated frozen metadata")
	}
	if got := projectDefinitionArt(foreign, "other-project"); got.Art == nil {
		t.Fatal("same-project override removed")
	}
	preset := cultivation.DefinitionDetails{Art: &cultivation.DefinitionArt{Source: "preset", PresetID: "space-skill"}}
	if got := projectDefinitionArt(preset, ""); got.Art == nil {
		t.Fatal("scope-free preset removed")
	}
}

func TestDefinitionArtActualCultivationDTOClearsForeignNestedOverride(t *testing.T) {
	root := t.TempDir()
	store := interactive.NewStore(root)
	defer store.Close()
	module, err := interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.ActorStateXiuxianID)
	if err != nil {
		t.Fatal(err)
	}
	system := module.ActorState
	foreign := cultivation.DefinitionDetails{QualityGrade: cultivation.QualityWhite, Art: &cultivation.DefinitionArt{Source: "generated", ImagePath: "assets/image/generated/foreign.png", ProjectID: "other-project"}}
	system.DefinitionDetails = map[string]cultivation.DefinitionDetails{"fire_bolt": foreign, "implement.lower.common": foreign}
	system.InitialActors = []interactive.ActorStateInitialActor{{ID: interactive.DefaultActorID, Name: "Player", TemplateID: interactive.DefaultActorID, State: map[string]any{"当前境界": "qi_3", "已学功法": map[string]any{"fire_bolt": map[string]any{"定义": "fire_bolt", "版本": float64(1), "掌握程度": "已学会"}}, "重要物品": map[string]any{"Focus": map[string]any{"定义": "implement.lower.common", "数量": float64(1)}}}}}
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Imported art", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{cfg: &config.Config{ProjectID: "this-project"}, workspace: root, interactive: store}
	view, err := app.CultivationEntryDetails(story.ID, "main", "en-US")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), "other-project") || strings.Contains(string(encoded), "foreign.png") {
		t.Fatalf("foreign top/nested reference leaked: %s", encoded)
	}
	raw, err := store.CultivationEntryDetails(story.ID, "main", "en-US")
	if err != nil {
		t.Fatal(err)
	}
	if raw.Skills[0].Art == nil || raw.Items[0].Art == nil {
		t.Fatal("projection changed saved override")
	}
}

func TestDefinitionArtCombatDTOScopesOwnedAndFallbackMetadata(t *testing.T) {
	foreign := cultivation.DefinitionDetails{Art: &cultivation.DefinitionArt{Source: "generated", ImagePath: "assets/image/generated/foreign.png", ProjectID: "other-project"}}
	window := CombatWindow{Sides: []CombatWindowSide{{ActorID: interactive.DefaultActorID, Exact: true, Skills: []CombatWindowSkill{{ID: "owned"}, {ID: "fallback"}, {ID: "unmatched", DefinitionDetails: foreign}}, Items: []CombatWindowItem{{InstanceID: "item"}}, Equipment: []CombatWindowEquipment{{InstanceID: "item"}}, Spares: []CombatWindowSpare{{InstanceID: "item"}}}}}
	entries := interactive.CultivationEntryDetails{Skills: []interactive.CultivationSkillDetail{{ID: "owned", DefinitionDetails: foreign}}, Items: []interactive.CultivationItemDetail{{ID: "item", DefinitionDetails: foreign}}}
	attachCombatEntryDetails(&window, entries, map[string]cultivation.DefinitionDetails{"fallback": foreign}, "en-US", "this-project")
	encoded, _ := json.Marshal(window)
	if strings.Contains(string(encoded), "other-project") {
		t.Fatalf("foreign combat art leaked: %s", encoded)
	}
}

func TestDefinitionArtActualAdventureDTOClearsForeignOverride(t *testing.T) {
	root := t.TempDir()
	store := interactive.NewStore(root)
	defer store.Close()
	foreign := cultivation.DefinitionDetails{Art: &cultivation.DefinitionArt{Source: "generated", ImagePath: "assets/image/generated/foreign.png", ProjectID: "other-project"}}
	rules := interactive.AdventureRules{Family: "wuxia", Currency: "Coins", StartingSkills: []string{"known"}, StartingItems: map[string]int{"blade": 1}, Skills: []interactive.AdventureSkill{{ID: "known", Name: "Known", Kind: "attack", Scale: "person", Power: 10, RequiredRank: 1, TrainingHours: 1, DefinitionDetails: foreign}}, Items: []interactive.AdventureItem{{ID: "blade", Name: "Blade", Kind: "weapon", Scale: "person", Acquisition: "market", Price: 1, Power: 1, DefinitionDetails: foreign}}}
	actor := interactive.NewAdventureActor(rules)
	encoded, _ := json.Marshal(actor)
	var value map[string]any
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	module, err := interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.DefaultActorStateModuleID)
	if err != nil {
		t.Fatal(err)
	}
	system := module.ActorState
	system.AdventureRules = &rules
	for i := range system.Templates {
		if system.Templates[i].ID == interactive.DefaultActorID {
			system.Templates[i].Fields = interactive.AdventureStateFields(rules)
		}
	}
	system.InitialActors = []interactive.ActorStateInitialActor{{ID: interactive.DefaultActorID, Name: "Player", TemplateID: interactive.DefaultActorID, State: map[string]any{interactive.AdventureStateField: value}}}
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Adventure imported art", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{cfg: &config.Config{ProjectID: "this-project"}, workspace: root, interactive: store}
	view, err := app.AdventureWindow(story.ID, "main", "en-US")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(view)
	if len(view.Skills) != 1 || len(view.Items) != 1 || strings.Contains(string(encoded), "other-project") {
		t.Fatalf("foreign adventure reference leaked: %s", encoded)
	}
}
