package app

import (
	"fmt"
	"log/slog"
	"ta/internal/interactive"
)

func (a *App) CultivationEntryDetails(storyID, branchID, locale string) (interactive.CultivationEntryDetails, error) {
	if a == nil {
		return interactive.CultivationEntryDetails{}, ErrNoWorkspace
	}
	a.mu.RLock()
	store := a.interactive
	projectID := ""
	if a.cfg != nil {
		projectID = a.cfg.ProjectID
	}
	a.mu.RUnlock()
	if store == nil {
		return interactive.CultivationEntryDetails{}, ErrNoWorkspace
	}
	details, err := store.CultivationEntryDetails(storyID, branchID, locale)
	if err != nil {
		return details, err
	}
	refs, artErr := store.DefinitionArtReferences(storyID, branchID)
	if artErr != nil {
		slog.Warn("[definition-art] read optional art failed", "story_id", storyID, "error", artErr)
	}
	for i := range details.Skills {
		if art, ok := refs[fmt.Sprintf("%s:%d", details.Skills[i].ID, details.Skills[i].Version)]; ok {
			details.Skills[i].Art = &art
		}
		details.Skills[i].DefinitionDetails = projectDefinitionArt(details.Skills[i].DefinitionDetails, projectID)
		details.Skills[i].Definition.DefinitionDetails = projectDefinitionArt(details.Skills[i].Definition.DefinitionDetails, projectID)
	}
	for i := range details.Techniques {
		if art, ok := refs[fmt.Sprintf("%s:%d", details.Techniques[i].ID, details.Techniques[i].Definition.Version)]; ok {
			details.Techniques[i].Art = &art
		}
		details.Techniques[i].DefinitionDetails = projectDefinitionArt(details.Techniques[i].DefinitionDetails, projectID)
		details.Techniques[i].Definition.DefinitionDetails = projectDefinitionArt(details.Techniques[i].Definition.DefinitionDetails, projectID)
	}
	for i := range details.CraftMethods {
		if art, ok := refs[fmt.Sprintf("%s:%d", details.CraftMethods[i].ID, details.CraftMethods[i].Definition.Version)]; ok {
			details.CraftMethods[i].Art = &art
		}
		details.CraftMethods[i].DefinitionDetails = projectDefinitionArt(details.CraftMethods[i].DefinitionDetails, projectID)
	}
	for i := range details.Items {
		if art, ok := refs[fmt.Sprintf("%s:1", details.Items[i].DefID)]; ok {
			details.Items[i].Art = &art
		}
		details.Items[i].DefinitionDetails = projectDefinitionArt(details.Items[i].DefinitionDetails, projectID)
		details.Items[i].Definition.DefinitionDetails = projectDefinitionArt(details.Items[i].Definition.DefinitionDetails, projectID)
	}
	return details, nil
}
