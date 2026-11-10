package app

import (
	"fmt"
	"log/slog"
	"strings"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// Freeze definition translations beside the discoverable catalog at creation.
// Cultivation may use the standard state preset; adventure modules intentionally
// contain canonical mechanics/prose only. Neither is a later UI metadata source.
func (s *InteractiveAppService) attachPackDefinitionDetails(system *interactive.StoryDirectorActorStateSystem, binding *interactive.StoryPackBinding) error {
	if system == nil || binding == nil || strings.TrimSpace(binding.PackID) == "" {
		return nil
	}
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil
	}
	pack, err := storypack.NewLibrary(cfg.DataDir()).Get(binding.PackID)
	if err != nil {
		slog.Warn("[interactive] story started without definition metadata", "pack_id", binding.PackID, "error", err)
		return nil
	}
	if len(pack.Story.Gameplay.DefinitionDetails) == 0 && pack.Story.Gameplay.AdventureRules == nil {
		return nil
	}
	if binding.ContentHash != "" && pack.ContentHash() != binding.ContentHash {
		return fmt.Errorf("pack definition metadata revision differs from story binding")
	}
	return freezePackDefinitionDetails(system, pack)
}

// freezePackDefinitionDetails is the metadata projection at the new-Story
// boundary, separate from resolving and checking the canonical pack revision.
func freezePackDefinitionDetails(system *interactive.StoryDirectorActorStateSystem, pack storypack.Pack) error {
	gameplay, err := pack.FrozenGameplay()
	if err != nil {
		return err
	}
	if len(gameplay.DefinitionDetails) == 0 && gameplay.AdventureRules == nil {
		return nil
	}

	details := map[string]cultivation.DefinitionDetails{}
	for id, d := range system.DefinitionDetails {
		details[id] = d
	}
	for id, d := range gameplay.DefinitionDetails {
		if err := d.Validate(); err != nil {
			return err
		}
		details[id] = d
	}
	// Metadata is joined at this new-story boundary, never looked up during
	// later public reads. Reusable mechanics modules keep their canonical hash.
	if gameplay.AdventureRules != nil && system.AdventureRules != nil {
		definitions := map[string]cultivation.DefinitionDetails{}
		for _, definition := range gameplay.AdventureRules.Skills {
			definitions[definition.ID] = definition.DefinitionDetails
		}
		for _, definition := range gameplay.AdventureRules.Items {
			definitions[definition.ID] = definition.DefinitionDetails
		}
		rules := *system.AdventureRules
		rules.Skills = append([]interactive.AdventureSkill(nil), rules.Skills...)
		rules.Items = append([]interactive.AdventureItem(nil), rules.Items...)
		for i := range rules.Skills {
			if d, ok := definitions[rules.Skills[i].ID]; ok {
				if err := d.Validate(); err != nil {
					return err
				}
				rules.Skills[i].DefinitionDetails = d
			}
		}
		for i := range rules.Items {
			if d, ok := definitions[rules.Items[i].ID]; ok {
				if err := d.Validate(); err != nil {
					return err
				}
				rules.Items[i].DefinitionDetails = d
			}
		}
		system.AdventureRules = &rules
	}
	attachBuiltinDefinitionArt(&details, system, pack.ID)
	system.DefinitionDetails = details
	return nil
}
