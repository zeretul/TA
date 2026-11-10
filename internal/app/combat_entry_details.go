package app

import (
	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// Attach only already-owned metadata. Enemy observations never pass this path.
func attachCombatEntryDetails(window *CombatWindow, entries interactive.CultivationEntryDetails, frozen map[string]cultivation.DefinitionDetails, locale, projectID string) {
	skills := map[string]interactive.CultivationSkillDetail{}
	items := map[string]interactive.CultivationItemDetail{}
	for _, s := range entries.Skills {
		skills[s.ID] = s
	}
	for _, i := range entries.Items {
		items[i.ID] = i
	}
	for n := range window.Sides {
		side := &window.Sides[n]
		for i := range side.Skills {
			side.Skills[i].DefinitionDetails = projectDefinitionArt(side.Skills[i].DefinitionDetails, projectID)
		}
		for i := range side.Items {
			side.Items[i].DefinitionDetails = projectDefinitionArt(side.Items[i].DefinitionDetails, projectID)
		}
		for i := range side.Equipment {
			side.Equipment[i].DefinitionDetails = projectDefinitionArt(side.Equipment[i].DefinitionDetails, projectID)
		}
		for i := range side.Spares {
			side.Spares[i].DefinitionDetails = projectDefinitionArt(side.Spares[i].DefinitionDetails, projectID)
		}
		if !side.Exact || side.ActorID != interactive.DefaultActorID {
			continue
		}
		for n := range side.Skills {
			entry := &side.Skills[n]
			if d, ok := skills[entry.ID]; ok {
				entry.DefinitionDetails = projectDefinitionArt(d.DefinitionDetails, projectID)
				entry.InstanceDescription = d.InstanceDescription
				entry.Source = d.Source
			} else if d, ok := frozen[entry.ID]; ok {
				entry.DefinitionDetails = projectDefinitionArt(d.ForLocale(locale), projectID)
			}
		}
		for n := range side.Items {
			entry := &side.Items[n]
			if d, ok := items[entry.InstanceID]; ok {
				entry.DefinitionDetails = projectDefinitionArt(d.DefinitionDetails, projectID)
				entry.InstanceDescription = d.InstanceDescription
				entry.Source = d.Source
			}
		}
		for n := range side.Equipment {
			entry := &side.Equipment[n]
			if d, ok := items[entry.InstanceID]; ok {
				entry.DefinitionDetails = projectDefinitionArt(d.DefinitionDetails, projectID)
				entry.InstanceDescription = d.InstanceDescription
				entry.Source = d.Source
			}
		}
		for n := range side.Spares {
			entry := &side.Spares[n]
			if d, ok := items[entry.InstanceID]; ok {
				entry.DefinitionDetails = projectDefinitionArt(d.DefinitionDetails, projectID)
				entry.InstanceDescription = d.InstanceDescription
				entry.Source = d.Source
			}
		}
	}
}
