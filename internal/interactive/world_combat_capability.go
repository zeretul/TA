package interactive

import "errors"

// ErrAdventureUnavailableForWorld rejects a mismatched frozen combat family.
var ErrAdventureUnavailableForWorld = errors.New("adventure combat is unavailable for this world")

// CombatFamilyForWorld requires the frozen declaration to agree with the actual
// rules. Legacy stories without a declaration derive capability from their
// frozen schema, never from a world name or a display label.
func CombatFamilyForWorld(binding *StoryPackBinding, system StoryDirectorActorStateSystem) string {
	family := "none"
	if system.AdventureRules != nil {
		switch system.AdventureRules.Family {
		case "wuxia", "space", "fantasy":
			family = system.AdventureRules.Family
		}
	} else {
		for _, template := range system.Templates {
			if template.ID != DefaultActorID {
				continue
			}
			for _, field := range template.Fields {
				if field.ValueFormat == ActorStateValueFormatStage {
					family = "cultivation"
					break
				}
			}
		}
	}
	if binding != nil && binding.Gameplay != nil && binding.Gameplay.Combat != family {
		return "none"
	}
	return family
}
