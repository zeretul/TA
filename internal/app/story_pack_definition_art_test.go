package app

import (
	"context"
	"reflect"
	"ta/config"
	"ta/internal/catalogart"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
)

func TestDefinitionArtActualBuiltinPresetFreezeAndColdOwnedItem(t *testing.T) {
	t.Setenv("TA_DIR", t.TempDir())
	root, data := t.TempDir(), t.TempDir()
	store := interactive.NewStore(root)
	defer store.Close()
	service := &InteractiveAppService{app: &App{workspace: root, cfg: &config.Config{DenovaDir: data}, interactive: store}}
	pack, err := storypack.NewLibrary(data).Get("fanren-jie")
	if err != nil {
		t.Fatal(err)
	}
	req, err := service.withStoryCreationDefaults(pack.StoryRequest())
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct{ id, preset string }{{"fire_bolt", "cultivation-skill"}, {"method.warm_origin", "cultivation-technique"}, {"implement.lower.common", "cultivation-equipment"}} {
		details := req.ActorState.DefinitionDetails[check.id]
		asset := catalogart.Default().AssetID("fanren-jie", "definition", check.id, 1, "illustration")
		if details.Art == nil || (asset != "" && (details.Art.Source != "catalog" || details.Art.CatalogID != asset)) || (asset == "" && details.Art.PresetID != check.preset) {
			t.Fatalf("real pack art missing for %s: %+v", check.id, details)
		}
		original := pack.Story.Gameplay.DefinitionDetails[check.id]
		details.Art = nil
		if details.QualityGrade != original.QualityGrade || details.Description != original.Description || details.Background != original.Background {
			t.Fatal("preset art rewrote facts/prose")
		}
	}
	for _, id := range []string{"material.talisman.paper", "pill.restore.health.lower", "strike"} {
		art := req.ActorState.DefinitionDetails[id].Art
		asset := catalogart.Default().AssetID("fanren-jie", "definition", id, 1, "illustration")
		if (asset == "" && art != nil) || (asset != "" && (art == nil || art.Source != "catalog" || art.CatalogID != asset)) {
			t.Fatalf("non-equipment/ordinary strike did not respect exact art availability: %s: %+v", id, art)
		}
	}
	ownedIDs := []string{"implement.lower.common", "manual.spirit.integration", "material.talisman.paper", "pill.restore.health.lower"}
	// This is an actual reviewed material, not a synthetic catalog fixture.
	materialArt := req.ActorState.DefinitionDetails["manual.spirit.integration"].Art
	materialAsset := catalogart.Default().AssetID("fanren-jie", "definition", "manual.spirit.integration", 1, "illustration")
	if materialAsset == "" || materialArt == nil || materialArt.Source != "catalog" || materialArt.CatalogID != materialAsset {
		t.Fatalf("registered spirit material did not receive its exact art: %+v", materialArt)
	}
	owned := map[string]any{}
	for _, id := range ownedIDs {
		owned[id] = map[string]any{"定义": id, "数量": float64(1)}
	}
	system := *req.ActorState
	system.InitialActors = []interactive.ActorStateInitialActor{{ID: interactive.DefaultActorID, Name: "Player", TemplateID: interactive.DefaultActorID, State: map[string]any{"当前境界": "qi_3", "重要物品": owned}}}
	req.ActorState = &system
	req.FrozenModules.ActorState = system
	story, err := store.CreateStory(req)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{workspace: root, interactive: store}
	view, err := app.CultivationEntryDetails(story.ID, "main", "en-US")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Items) != len(ownedIDs) {
		t.Fatalf("real owned items missing: %+v", view)
	}
	for _, item := range view.Items {
		if !reflect.DeepEqual(item.Art, req.ActorState.DefinitionDetails[item.DefID].Art) {
			t.Fatalf("owned item artwork changed during projection: %s: %+v", item.DefID, item.Art)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = interactive.NewStore(root)
	defer store.Close()
	app.interactive = store
	cold, err := app.CultivationEntryDetails(story.ID, "main", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(cold.Items) != len(view.Items) {
		t.Fatal("owned items lost after cold reopen")
	}
	for i, item := range cold.Items {
		if item.DefID != view.Items[i].DefID || !reflect.DeepEqual(item.Art, view.Items[i].Art) {
			t.Fatalf("item artwork changed/lost after cold reopen: %s", item.DefID)
		}
	}
}

func TestDefinitionArtPresetCoverageAndOverrides(t *testing.T) {
	t.Setenv("TA_DIR", t.TempDir())
	data := t.TempDir()
	library := storypack.NewLibrary(data)
	total, expected := 0, 0
	for _, id := range []string{"fanren-jie", "lanchuan-qunxia", "xinghai-689"} {
		pack, err := library.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		system := interactive.StoryDirectorActorStateSystem{}
		if err := freezePackDefinitionDetails(&system, pack); err != nil {
			t.Fatal(err)
		}
		counts := map[string]int{}
		count := func(art *cultivation.DefinitionArt) {
			if art == nil {
				return
			}
			if err := art.Validate(); err != nil {
				t.Fatal(err)
			}
			key := art.Source
			if art.Source == "preset" {
				key = art.PresetID
			}
			counts[key]++
		}
		if id == "fanren-jie" {
			expected += len(cultivation.TechniqueTable())
			for key := range cultivation.SkillTable() {
				if key != "strike" || catalogart.Default().AssetID(id, "definition", key, 1, "illustration") != "" {
					expected++
				}
			}
			for _, def := range cultivation.NewItemCatalogWithFirstVersion().Definitions() {
				if (def.Kind != cultivation.KindMaterial && def.Kind != cultivation.KindPill) || catalogart.Default().AssetID(id, "definition", def.ID, 1, "illustration") != "" {
					expected++
				}
			}
		}
		for _, d := range system.DefinitionDetails {
			count(d.Art)
		}
		if system.AdventureRules != nil {
			t.Fatal("unbound rules should not be introduced by metadata freeze")
		}
		if pack.Story.Gameplay.AdventureRules != nil {
			rules := *pack.Story.Gameplay.AdventureRules
			system.AdventureRules = &rules
			if err := freezePackDefinitionDetails(&system, pack); err != nil {
				t.Fatal(err)
			}
			for _, def := range system.AdventureRules.Skills {
				if def.ID != "strike" || catalogart.Default().AssetID(id, "definition", def.ID, 1, "illustration") != "" {
					expected++
				}
				count(def.Art)
			}
			for _, def := range system.AdventureRules.Items {
				if def.Kind == "weapon" || def.Kind == "armor" || def.Kind == "tool" || catalogart.Default().AssetID(id, "definition", def.ID, 1, "illustration") != "" {
					expected++
				}
				count(def.Art)
				if (def.Kind == "material" || def.Kind == "consumable" || def.Kind == "ship" || def.Kind == "machine") && def.Art != nil && def.Art.Source == "preset" {
					t.Fatalf("misleading equipment art: %s", def.ID)
				}
			}
		}
		t.Logf("%s shared preset coverage: %+v", id, counts)
		for _, count := range counts {
			total += count
		}
	}
	if total != expected {
		t.Fatalf("current rule catalog artwork coverage: got %d, want %d", total, expected)
	}
	override := &cultivation.DefinitionArt{Source: "generated", ImagePath: "assets/image/generated/custom.png", ProjectID: "project"}
	details := map[string]cultivation.DefinitionDetails{"fire_bolt": {Art: override}, "manual.spirit.integration": {Art: override}}
	attachBuiltinDefinitionArt(&details, &interactive.StoryDirectorActorStateSystem{}, "fanren-jie")
	for id, detail := range details {
		if !reflect.DeepEqual(detail.Art, override) {
			t.Fatalf("definition thumbnail override replaced: %s", id)
		}
	}
}

func TestDefinitionArtMissingAgentDoesNotNeedAWorkspace(t *testing.T) {
	// The real post-commit callback short-circuits without even reading storage.
	(&interactiveAgentCycle{runtimeCfg: config.Config{}}).queueNewDefinitionArt(context.Background())
}

func TestDefinitionArtNewWorldDoesNotBorrowThemeOrOverrideExplicitArt(t *testing.T) {
	for _, world := range []string{"sui-guan-realms", "custom-adventure-world"} {
		t.Run(world, func(t *testing.T) {
			override := &cultivation.DefinitionArt{Source: "generated", ImagePath: "assets/image/generated/custom.png", ProjectID: "project"}
			rules := &interactive.AdventureRules{
				Family: "fantasy",
				Skills: []interactive.AdventureSkill{
					{ID: "xinghai-s01", Name: "Same ID in a different world", Kind: "attack"},
					{ID: "new-world-skill", Kind: "guard", DefinitionDetails: cultivation.DefinitionDetails{Description: "Keep authored facts", Art: override}},
				},
				Items: []interactive.AdventureItem{
					{ID: "lanchuan-i01", Kind: "weapon"},
					{ID: "new-world-material", Kind: "material"},
					{ID: "new-world-armor", Kind: "armor", DefinitionDetails: cultivation.DefinitionDetails{Art: override}},
				},
			}
			want := *rules
			want.Skills = append([]interactive.AdventureSkill(nil), rules.Skills...)
			want.Items = append([]interactive.AdventureItem(nil), rules.Items...)
			details := map[string]cultivation.DefinitionDetails{}
			attachBuiltinDefinitionArt(&details, &interactive.StoryDirectorActorStateSystem{AdventureRules: rules}, world)
			if !reflect.DeepEqual(*rules, want) {
				t.Fatalf("new world received another world's picture/preset or changed explicit art: %+v", rules)
			}
		})
	}
}
