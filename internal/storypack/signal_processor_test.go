package storypack

import (
	"strings"
	"ta/internal/interactive"
	"testing"
)

func TestSpaceSignalProcessorAuthoredAcquisitionAndIndependentEnglish(t *testing.T) {
	var pack Pack
	for _, candidate := range builtinPacks() {
		if candidate.ID == "xinghai-689" {
			pack = candidate
		}
	}
	if pack.ID == "" {
		t.Fatal("space source missing")
	}
	english := pack.Localize("en-US")
	want := map[string]string{"xinghai-signal-processor": "Carrier-Mounted High-Power Signal Processor", "xinghai-hopping-recording": "Ordinary Hopping-Traffic Recording", "xinghai-signal-fragments": "Recovered Hopping-Traffic Fragments"}
	for id, name := range want {
		if pack.Localizations["en-US"].Adventure[id] != name {
			t.Fatalf("%s missing localized public item name", id)
		}
		found := false
		for _, entry := range english.World {
			if entry.ID == id {
				found = true
				if entry.Name != name || entry.Content == "" || strings.Contains(entry.Content, "四小时") {
					t.Fatalf("%s missing independent English prose", id)
				}
			}
		}
		if !found {
			t.Fatal("English lore missing " + id)
		}
		for _, origin := range pack.ProtagonistTemplates {
			state := origin.State["历练状态"].(map[string]any)
			inventory := state["inventory"].(map[string]any)
			if inventory[id] != nil {
				t.Fatalf("%s origin injected %s", origin.ID, id)
			}
		}
	}
	var recipe interactive.AdventureRecipe
	for _, candidate := range pack.Story.Gameplay.AdventureRules.Recipes {
		if candidate.ID == "xinghai-decode-hopping" {
			recipe = candidate
			break
		}
	}
	if recipe.ID != "xinghai-decode-hopping" || recipe.RequiredAsset != "xinghai-signal-processor" || recipe.FuelCost != 10 || recipe.Hours != 4 || recipe.SkillID != "xinghai-s29" {
		t.Fatalf("source processing requirements drifted: %+v", recipe)
	}
}
