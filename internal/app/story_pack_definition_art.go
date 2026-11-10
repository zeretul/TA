package app

import (
	"ta/internal/catalogart"
	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// Per-entry art is frozen only at new-story creation. Missing entries keep the
// honest shared-archetype fallback; explicit non-preset artwork always wins.
func attachBuiltinDefinitionArt(details *map[string]cultivation.DefinitionDetails, system *interactive.StoryDirectorActorStateSystem, packID string) {
	set := func(id string, version int, d cultivation.DefinitionDetails, preset string) cultivation.DefinitionDetails {
		if d.Art == nil || d.Art.Source == "preset" {
			if asset := catalogart.Default().AssetID(packID, "definition", id, version, "illustration"); asset != "" {
				d.Art = &cultivation.DefinitionArt{Source: "catalog", CatalogID: asset}
			} else if d.Art == nil && preset != "" {
				d.Art = &cultivation.DefinitionArt{Source: "preset", PresetID: preset}
			}
		}
		return d
	}
	if packID == "fanren-jie" {
		categories := map[string]string{}
		for _, def := range cultivation.TechniqueTable(system.CultivationRules) {
			categories[def.ID] = "technique"
		}
		for _, def := range cultivation.NewSkillRegistryWithBuiltins(system.CultivationRules).Definitions() {
			categories[def.ID] = "skill"
		}
		for _, def := range cultivation.NewItemCatalogWithFirstVersion(system.CultivationRules).Definitions() {
			// Materials and consumables can have their own registered artwork,
			// but must never borrow a shared equipment illustration.
			categories[def.ID] = ""
			if def.Kind != cultivation.KindMaterial && def.Kind != cultivation.KindPill {
				categories[def.ID] = "equipment"
			}
		}
		for id, category := range categories {
			if d, ok := (*details)[id]; ok {
				preset := ""
				if category != "" && id != "strike" {
					preset = "cultivation-" + category
				}
				(*details)[id] = set(id, 1, d, preset)
			}
		}
	}
	// Exact catalog entries work for every world. Shared archetypes belong only
	// to the known themes; an unillustrated new world keeps an honest empty state.
	theme := map[string]string{"lanchuan-qunxia": "wuxia", "xinghai-689": "space"}[packID]
	if system.AdventureRules != nil {
		for i := range system.AdventureRules.Skills {
			def := &system.AdventureRules.Skills[i]
			preset := ""
			if theme != "" && def.ID != "strike" {
				preset = theme + "-skill"
			}
			def.DefinitionDetails = set(def.ID, 1, def.DefinitionDetails, preset)
		}
		for i := range system.AdventureRules.Items {
			def := &system.AdventureRules.Items[i]
			preset := ""
			if theme != "" && (def.Kind == "weapon" || def.Kind == "armor" || def.Kind == "tool") {
				preset = theme + "-equipment"
			}
			def.DefinitionDetails = set(def.ID, 1, def.DefinitionDetails, preset)
		}
		for i := range system.AdventureRules.Recipes {
			def := &system.AdventureRules.Recipes[i]
			def.DefinitionDetails = set(def.ID, 1, def.DefinitionDetails, "")
		}
	}
}
