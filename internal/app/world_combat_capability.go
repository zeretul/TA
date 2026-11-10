package app

import "ta/internal/interactive"

func storyUsesCultivationCombat(ctx interactive.StoryContext) bool {
	schema := ctx.Snapshot.ActorStateSchema
	if schema == nil {
		schema = ctx.Meta.ActorStateSchema
	}
	if schema == nil {
		return false
	}
	return interactive.CombatFamilyForWorld(ctx.Meta.PackBinding, schema.System) == "cultivation"
}
