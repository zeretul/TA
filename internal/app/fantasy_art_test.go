package app

import (
	"bytes"
	"image"
	"ta/internal/catalogart"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"ta/internal/portrait"
	"ta/internal/storypack"
	"testing"
)

// Verifies the shipped runtime lookup, not image-generation or browser behavior.
func TestFantasyPortraitsUseExactWorldAndKeepMissingFacesEmpty(t *testing.T) {
	p, err := storypack.NewLibrary(t.TempDir()).Get("sui-guan-realms")
	if err != nil {
		t.Fatal(err)
	}
	library := portrait.NewLibrary()
	known := map[string]bool{"sg-p01": true, "sg-p02": true, "sg-p03": true, "sg-p04": true, "sg-p05": true, "sg-p06": true, "sg-p07": true, "sg-p08": true, "sg-p09": true, "sg-p10": true, "sg-p11": true, "sg-p12": true, "sg-p13": true, "sg-p14": true, "sg-p15": true, "sg-p16": true, "sg-p17": true, "sg-p18": true, "sg-p19": true, "sg-p20": true, "sg-p21": true, "sg-p22": true, "sg-p23": true, "sg-p24": true, "sg-p25": true, "sg-p26": true, "sg-p27": true, "sg-p28": true, "sg-p29": true, "sg-p30": true, "sg-p31": true, "sg-p32": true, "sg-p33": true, "sg-p34": true, "sg-p35": true, "sg-p36": true, "sg-p37": true, "sg-p38": true, "sg-p39": true, "sg-p40": true, "sg-p41": true, "sg-p42": true, "sg-p43": true, "sg-p44": true, "sg-p45": true, "sg-p46": true, "sg-p47": true, "sg-p48": true, "sg-p49": true, "sg-p50": true, "sg-p51": true, "sg-p52": true, "sg-p53": true, "sg-p54": true, "sg-p55": true, "sg-p56": true, "sg-p57": true, "sg-p58": true, "sg-p59": true, "sg-p60": true}
	got := loreEntryPortraitsByEntryID(loreItemsForPack(p, nil), []storypack.Pack{p}, library)
	for _, person := range p.Characters {
		url := library.WorldURL(p.ID, person.SourceKey, false)
		id := interactive.PackEntryIDForRevision(person.LoreID, p.ContentHash())
		face, found := got[id]
		if known[person.SourceKey] {
			if url == "" || !found || face.ThumbnailURL != url || face.DisplayURL != library.WorldURL(p.ID, person.SourceKey, true) {
				t.Fatalf("reviewed face did not reach library: %s %+v", person.SourceKey, face)
			}
		} else if url != "" || found {
			t.Fatalf("unillustrated person borrowed a face: %s", person.SourceKey)
		}
		if library.WorldURL("xinghai-689", person.SourceKey, false) != "" {
			t.Fatalf("world boundary crossed: %s", person.SourceKey)
		}
	}
}

// Every imported definition is resolved by its exact source identity. A missing
// illustration stays empty and user-generated overrides remain authoritative.
func TestFantasyDefinitionArtUsesOnlyReviewedExactEntries(t *testing.T) {
	p, err := storypack.NewLibrary(t.TempDir()).Get("sui-guan-realms")
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := p.FrozenGameplay()
	if err != nil {
		t.Fatal(err)
	}
	rules := frozen.AdventureRules
	system := interactive.StoryDirectorActorStateSystem{AdventureRules: rules}
	details := map[string]cultivation.DefinitionDetails{}
	attachBuiltinDefinitionArt(&details, &system, p.ID)
	checked, illustrated := 0, 0
	check := func(id string, d cultivation.DefinitionDetails) {
		checked++
		expected := catalogart.Default().AssetID(p.ID, "definition", id, 1, "illustration")
		if expected == "" {
			if d.Art != nil {
				t.Fatal("unillustrated definition borrowed art", id)
			}
			return
		}
		illustrated++
		if d.Art == nil || d.Art.Source != "catalog" || d.Art.CatalogID != expected {
			t.Fatal("exact definition image did not freeze", id, d.Art)
		}
		data, e := catalogart.Default().Read(catalogart.Default().CanonicalFile(expected + ".png"))
		if e != nil {
			t.Fatal(e)
		}
		config, _, e := image.DecodeConfig(bytes.NewReader(data))
		if e != nil || config.Width < 1 || config.Height < 1 {
			t.Fatal("runtime image cannot decode", id, e)
		}
		if catalogart.Default().AssetID("xinghai-689", "definition", id, 1, "illustration") != "" {
			t.Fatal("image crossed world", id)
		}
	}
	for _, d := range rules.Skills {
		check(d.ID, d.DefinitionDetails)
	}
	for _, d := range rules.Items {
		check(d.ID, d.DefinitionDetails)
	}
	for _, d := range rules.Recipes {
		check(d.ID, d.DefinitionDetails)
	}
	if checked != 214 || illustrated < 16 {
		t.Fatal("unexpected examined catalog", checked, illustrated)
	}
	override := &cultivation.DefinitionArt{Source: "generated", ImagePath: "assets/image/generated/player.png", ProjectID: "player-project"}
	rules.Skills[0].Art = override
	attachBuiltinDefinitionArt(&details, &system, p.ID)
	if rules.Skills[0].Art != override {
		t.Fatal("explicit user image was replaced")
	}
}
