package storypack

import (
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"testing"
)

func TestQualityBuiltinCatalogCoverageAndCoreProse(t *testing.T) {
	grades, core := 0, 0
	check := func(id string, d cultivation.DefinitionDetails, required bool) {
		t.Helper()
		grades++
		if !d.QualityGrade.Valid() {
			t.Errorf("definition %s has no explicit approved grade", id)
		}
		if required {
			core++
			if err := d.ValidateComplete(); err != nil {
				t.Errorf("core prose %s: %v", id, err)
			}
			if d.Localizations == nil || d.Localizations.English.Description == "" || d.Localizations.English.Background == "" {
				t.Errorf("core prose %s missing independent English", id)
			}
		}
	}
	for _, pack := range builtinPacks() {
		gameplay, err := pack.FrozenGameplay()
		if err != nil {
			t.Fatal(err)
		}
		switch pack.ID {
		case "fanren-jie":
			expected := cultivation.BuiltinDefinitionDetails()
			if len(gameplay.DefinitionDetails) != len(expected) {
				t.Errorf("cultivation stable ID coverage %d != %d", len(gameplay.DefinitionDetails), len(expected))
			}
			for id, d := range gameplay.DefinitionDetails {
				if _, ok := expected[id]; !ok {
					t.Errorf("unknown cultivation metadata %s", id)
				}
				check(id, d, true)
			}
		case "lanchuan-qunxia", "xinghai-689":
			rules := gameplay.AdventureRules
			if err := rules.Validate(); err != nil {
				t.Fatal(err)
			}
			for _, d := range rules.Skills {
				check(d.ID, d.DefinitionDetails, true)
			}
			for _, d := range rules.Items {
				check(d.ID, d.DefinitionDetails, d.Kind == "weapon" || d.Kind == "armor" || d.Kind == "machine" || d.Kind == "ship")
			}
			// Full authored bilingual prose cannot enter the authoritative model prefix.
			plain := *rules
			plain.Skills = append([]interactive.AdventureSkill(nil), rules.Skills...)
			plain.Items = append([]interactive.AdventureItem(nil), rules.Items...)
			plain.Recipes = append([]interactive.AdventureRecipe(nil), rules.Recipes...)
			for i := range plain.Skills {
				plain.Skills[i].DefinitionDetails = cultivation.DefinitionDetails{}
			}
			for i := range plain.Items {
				plain.Items[i].DefinitionDetails = cultivation.DefinitionDetails{}
			}
			for i := range plain.Recipes {
				plain.Recipes[i].DefinitionDetails = cultivation.DefinitionDetails{}
			}
			if interactive.AdventureRulesContext(*rules) != interactive.AdventureRulesContext(plain) {
				t.Errorf("author prose changed mechanical context %s", pack.ID)
			}
		}
	}
	// Reviewed growth additions: 60 cultivation definitions, plus 56 skills,
	// 28 equipment items and 14 learning sources across the adventure worlds,
	// then NPC/person-ship content, and 52 creation definitions (17 skills,
	// 18 equipment and 17 manuals). The adventure catalogs have 483 definitions,
	// of which 246 require complete skill/equipment prose.
	expectedCultivation := len(cultivation.BuiltinDefinitionDetails())
	if grades != expectedCultivation+483 || core != expectedCultivation+246 {
		t.Fatalf("reviewed coverage changed: grades=%d core=%d", grades, core)
	}
}
