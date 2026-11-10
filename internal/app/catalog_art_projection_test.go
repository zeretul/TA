package app

import (
	"strings"
	"ta/internal/catalogart"
	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"ta/internal/portrait"
	"testing"
)

func TestCatalogPortraitOnlyDecoratesAlreadyDiscoveredPeople(t *testing.T) {
	sources := pictureSources{portraits: portrait.NewLibrary(), world: "lanchuan-qunxia"}
	empty := character.State{Version: character.StateVersion}
	if rows := characterWindowCast(empty, character.Scene{}, sources); len(rows) != 0 {
		t.Fatal("artwork revealed undiscovered people")
	}
	if err := empty.UpsertProfile(character.Profile{ID: "song", StoryID: "s", Name: "宋老橹", SourceKey: "lanchuan-p01", Revision: 1}); err != nil {
		t.Fatal(err)
	}
	rows := characterWindowCast(empty, character.Scene{}, sources)
	if len(rows) != 1 || !strings.HasPrefix(rows[0].Avatar, catalogart.PathPrefix) || !strings.HasPrefix(rows[0].AvatarDisplay, catalogart.PathPrefix) {
		t.Fatalf("pilot projection: %+v", rows)
	}
	sources.world = "xinghai-689"
	if rows := characterWindowCast(empty, character.Scene{}, sources); len(rows) != 1 || rows[0].Avatar != "" {
		t.Fatalf("cross-world face: %+v", rows)
	}
}

func TestCatalogDefinitionFallbackPreservesExplicitGeneratedArt(t *testing.T) {
	explicit := &cultivation.DefinitionArt{Source: "generated", ProjectID: "p", ImagePath: "assets/image/generated/blade.png"}
	details := map[string]cultivation.DefinitionDetails{"strike": {}, "fire_bolt": {Art: explicit}}
	system := interactive.StoryDirectorActorStateSystem{}
	attachBuiltinDefinitionArt(&details, &system, "fanren-jie")
	if details["fire_bolt"].Art != explicit {
		t.Fatal("generated artwork replaced")
	}
	strike := details["strike"].Art
	if asset := catalogart.Default().AssetID("fanren-jie", "definition", "strike", 1, "illustration"); asset != "" {
		if strike == nil || strike.Source != "catalog" || strike.CatalogID != asset {
			t.Fatal("ordinary strike did not receive its own registered illustration")
		}
	} else if strike != nil {
		t.Fatal("ordinary strike must not inherit a misleading shared skill image")
	}
}

func TestCatalogMissingVehicleArtNeverBorrowsAnEquipmentPreset(t *testing.T) {
	details := map[string]cultivation.DefinitionDetails{}
	system := interactive.StoryDirectorActorStateSystem{AdventureRules: &interactive.AdventureRules{
		Skills: []interactive.AdventureSkill{{ID: "strike"}},
		Items:  []interactive.AdventureItem{{ID: "unillustrated-machine", Kind: "machine"}, {ID: "unillustrated-ship", Kind: "ship"}},
	}}
	attachBuiltinDefinitionArt(&details, &system, "xinghai-689")
	for _, item := range system.AdventureRules.Items {
		if item.Art != nil {
			t.Fatalf("vehicle without dedicated art borrowed an unrelated picture: %+v", item)
		}
	}
	strike := system.AdventureRules.Skills[0].Art
	if asset := catalogart.Default().AssetID("xinghai-689", "definition", "strike", 1, "illustration"); asset == "" && strike != nil {
		t.Fatal("ordinary space-world strike borrowed a shared skill image")
	}
}

func TestCatalogDefinitionFreezesDedicatedWorldIdentityAndKeepsFallback(t *testing.T) {
	details := map[string]cultivation.DefinitionDetails{}
	system := interactive.StoryDirectorActorStateSystem{AdventureRules: &interactive.AdventureRules{Items: []interactive.AdventureItem{{ID: "lanchuan-i01", Kind: "weapon"}, {ID: "unknown-item", Kind: "weapon"}}}}
	attachBuiltinDefinitionArt(&details, &system, "lanchuan-qunxia")
	art := system.AdventureRules.Items[0].Art
	if art == nil || art.Source != "catalog" || art.Validate() != nil {
		t.Fatalf("dedicated art not frozen: %+v", art)
	}
	if got := system.AdventureRules.Items[1].Art; got == nil || got.Source != "preset" {
		t.Fatalf("missing image not a shared fallback: %+v", got)
	}
	other := interactive.StoryDirectorActorStateSystem{AdventureRules: &interactive.AdventureRules{Items: []interactive.AdventureItem{{ID: "lanchuan-i01", Kind: "weapon"}}}}
	attachBuiltinDefinitionArt(&details, &other, "xinghai-689")
	if other.AdventureRules.Items[0].Art.Source != "preset" {
		t.Fatal("dedicated image crossed worlds")
	}
}
