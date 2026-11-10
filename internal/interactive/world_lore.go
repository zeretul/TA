package interactive

import (
	"ta/internal/book/lore"
	"ta/internal/cultivation"
)

// WorldLoreGate restricts model knowledge, not the author's library editor.
type WorldLoreGate struct {
	MinimumStage string   `json:"minimum_stage,omitempty"`
	Locations    []string `json:"locations,omitempty"`
}

// StoryLoreVisibility uses a committed snapshot. Search, direct read and resident
// assembly must share it, so guessing an ID cannot bypass a knowledge gate.
func StoryLoreVisibility(meta StoryMeta, state map[string]any) func(lore.Item) bool {
	blocked := map[string]bool{}
	if c := worldContentForMeta(meta); c != nil && meta.PackBinding != nil {
		for id, g := range c.LoreGates {
			visible := true
			if g.MinimumStage != "" {
				stage, ok := cultivation.ParseStageKey(xiuxianStoredStageKey(state, DefaultActorID), cultivationRulesForMeta(meta))
				visible = ok && stageReaches(stage, g.MinimumStage)
			}
			if len(g.Locations) > 0 {
				visible = visible && storyLocationIsAny(state, g.Locations, c)
			}
			if !visible {
				blocked[PackEntryIDForRevision(id, meta.PackBinding.ContentHash)] = true
			}
		}
	}
	return func(item lore.Item) bool { return !blocked[item.ID] }
}
