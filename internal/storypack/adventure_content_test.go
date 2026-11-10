package storypack

import (
	"encoding/json"
	"strings"
	"testing"
)

func adventureContentPacks(t *testing.T) []Pack {
	t.Helper()
	var result []Pack
	for _, p := range builtinPacks() {
		if p.ID == "lanchuan-qunxia" || p.ID == "xinghai-689" {
			result = append(result, p)
		}
	}
	if len(result) != 2 {
		t.Fatalf("adventure pack count = %d", len(result))
	}
	return result
}

func TestAdventureNPCBaselinesAndDistantDiscovery(t *testing.T) {
	for _, p := range adventureContentPacks(t) {
		rules := p.Story.Gameplay.AdventureRules
		if rules == nil || len(rules.NPCs) != len(p.Characters) {
			t.Fatalf("%s missing sourced NPC baseline", p.ID)
		}
		known := map[string]bool{}
		for _, npc := range rules.NPCs {
			known[npc.CharacterSourceKey] = true
			if npc.Plan.Action == "none" && npc.Plan.TrainingPercent != 0 {
				t.Fatalf("%s gives passive study without a plan", npc.CharacterSourceKey)
			}
		}
		for _, character := range p.Characters {
			if !known[character.SourceKey] {
				t.Fatalf("%s has no baseline", character.SourceKey)
			}
			if p.ID == "xinghai-689" {
				if character.SourceKey == "xinghai-p03" && character.PlaceCandidate != "" {
					t.Fatal("distant warlord must not appear on a local smuggling ship")
				}
				if character.SourceKey == "xinghai-p05" && character.PlaceCandidate != "xinghai-l29" {
					t.Fatal("mechanical nation leader has the wrong geographic identity")
				}
			}
		}
	}
}

func TestAdventureContentHasStableCompleteManuscriptCatalog(t *testing.T) {
	for _, p := range adventureContentPacks(t) {
		t.Run(p.ID, func(t *testing.T) {
			if p.Story.Title != p.Name {
				t.Fatalf("story title drifted to a character: %q", p.Story.Title)
			}
			if err := p.Validate(); err != nil {
				t.Fatal(err)
			}
			if len(p.Characters) != 60 || len(p.ProtagonistTemplates) != 5 {
				t.Fatalf("incomplete catalog: people=%d origins=%d", len(p.Characters), len(p.ProtagonistTemplates))
			}
			byID := map[string]WorldEntry{}
			byName := map[string]string{}
			counts := map[string]int{}
			for _, entry := range p.World {
				if previous, exists := byName[entry.Name]; exists {
					t.Fatalf("lore name collision between %s and %s", previous, entry.ID)
				}
				byName[entry.Name] = entry.ID
				if _, exists := byID[entry.ID]; exists {
					t.Fatalf("duplicate stable identity %s", entry.ID)
				}
				byID[entry.ID] = entry
				parts := strings.Split(entry.ID, "-")
				last := parts[len(parts)-1]
				if len(last) == 3 {
					counts[last[:1]]++
				}
			}
			for category, want := range map[string]int{"l": 40, "p": 60, "a": 20, "e": 6, "q": 3, "s": 36, "i": 60, "d": 12, "r": 8} {
				if counts[category] != want {
					t.Fatalf("category %s = %d; want %d", category, counts[category], want)
				}
			}
			for _, character := range p.Characters {
				if character.SourceKey != character.LoreID || byID[character.LoreID].ID == "" {
					t.Fatalf("broken character binding: %s", character.SourceKey)
				}
				if character.Brief == "" || character.ContactGate == "" || len(character.RegionTokens) == 0 {
					t.Fatalf("missing discovery boundary: %s", character.SourceKey)
				}
				if strings.Contains(character.Brief, "作者参考") {
					t.Fatalf("secret section leaked in public brief: %s", character.SourceKey)
				}
			}
		})
	}
}

func TestAdventureOriginsHaveDistinctCanonicalInventoryAndFreeze(t *testing.T) {
	for _, p := range adventureContentPacks(t) {
		inventories := map[string]bool{}
		for _, origin := range p.ProtagonistTemplates {
			state, ok := origin.State["历练状态"].(map[string]any)
			if !ok {
				t.Fatalf("%s missing canonical adventure state", origin.ID)
			}
			inventory, _ := json.Marshal(state["inventory"])
			if inventories[string(inventory)] {
				t.Fatalf("%s repeats another origin's inventory", origin.ID)
			}
			inventories[string(inventory)] = true
			if state["currency"] != origin.State["钱财"] {
				t.Fatalf("%s currency projection drift", origin.ID)
			}
		}
		frozen := p.FreezeCatalog()
		before, _ := json.Marshal(frozen)
		p.Characters[0].Name = "Changed after the story started"
		after, _ := json.Marshal(frozen)
		if string(before) != string(after) {
			t.Fatal("frozen people changed after author edit")
		}
	}
}

func TestAdventureUnknownAndMinorAgesRemainProtected(t *testing.T) {
	for _, p := range adventureContentPacks(t) {
		for _, c := range p.Characters {
			switch c.SourceKey {
			case "lanchuan-p49":
				if c.Age != 17 {
					t.Fatalf("Yilin age = %d", c.Age)
				}
			case "xinghai-p02", "xinghai-p25", "xinghai-p59", "xinghai-p60":
				if c.Age != 0 {
					t.Fatalf("appearance/unknown age inferred for %s: %d", c.SourceKey, c.Age)
				}
			}
		}
	}
}

func TestAdventureEnglishResourcesCoverEveryPublicEntry(t *testing.T) {
	for _, p := range adventureContentPacks(t) {
		localization, ok := p.Localizations["en-US"]
		if !ok || localization.Name == "" || localization.Creation == nil {
			t.Fatalf("%s English header/creation missing", p.ID)
		}
		for _, entry := range p.World {
			translated := localization.World[entry.ID]
			if translated.Name == "" || translated.Content == "" || translated.BriefDescription == "" {
				t.Fatalf("%s English entry incomplete", entry.ID)
			}
		}
		for _, c := range p.Characters {
			translated := localization.Characters[c.SourceKey]
			if translated.Name == "" || translated.Brief == "" || translated.PlaceName == "" {
				t.Fatalf("%s English character incomplete", c.SourceKey)
			}
		}
	}
}
