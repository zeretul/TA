package interactive

import "ta/internal/cultivation"

// storyCraftRegistry is the only recipe source used by learning, context and
// settlement. Missing snapshots are the released builtin catalog; explicit
// snapshots are authoritative, including versions no longer in a world file.
func storyCraftRegistry(system StoryDirectorActorStateSystem) *cultivation.CraftRegistry {
	if len(system.CraftMethods) == 0 {
		return cultivation.NewCraftRegistryWithBuiltins(system.CultivationRules)
	}
	registry := cultivation.NewCraftRegistry()
	for _, method := range system.CraftMethods {
		if err := registry.Register(method); err != nil {
			return cultivation.NewCraftRegistry()
		}
	}
	return registry
}

func actorStateHasCraft(system StoryDirectorActorStateSystem) bool {
	for _, template := range system.Templates {
		for _, field := range template.Fields {
			if actorStateFieldID(field) == "已学功法" {
				return true
			}
		}
	}
	return false
}
