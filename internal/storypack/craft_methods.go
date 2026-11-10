package storypack

import (
	"fmt"
	"reflect"
	"ta/internal/cultivation"
)

// upsertPackCraftMethod shares the world's existing atomic collection write.
// The method is content, never inventory or a grant of character proficiency.
func upsertPackCraftMethod(pack *Pack, method *cultivation.CraftMethodDef) (bool, error) {
	if method == nil {
		return false, nil
	}
	if pack.Story.Gameplay.StatusTemplate != WorldStatusCultivation {
		return false, fmt.Errorf("craft method requires a cultivation world")
	}
	if err := cultivation.ValidateCraftMethods([]cultivation.CraftMethodDef{*method}, cultivation.AuthoredCraftCatalog); err != nil {
		return false, err
	}
	methods := append([]cultivation.CraftMethodDef(nil), pack.Story.Gameplay.CraftMethods...)
	found := false
	for index, current := range methods {
		if current.ID != method.ID {
			continue
		}
		if reflect.DeepEqual(current, *method) {
			return false, nil
		}
		if method.Version <= current.Version {
			return false, fmt.Errorf("craft method %q: changed definition requires a higher version", method.ID)
		}
		methods[index] = *method
		found = true
		break
	}
	if !found {
		methods = append(methods, *method)
	}
	if err := cultivation.ValidateCraftMethods(methods, cultivation.AuthoredCraftCatalog); err != nil {
		return false, err
	}
	pack.Story.Gameplay.CraftMethods = methods
	pack.Story.ModuleRefs.ActorStateID, pack.Story.ModuleRefs.RuleSystemID = gameplayModuleIDs(pack.ID, pack.Story.Gameplay)
	return true, nil
}
