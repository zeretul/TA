package interactive

// WorldStateAtTurnParent is the same public/action projection as WorldStateFor,
// but at the target turn's canonical parent. Regeneration must not learn a
// solved cause, a membership, or a task outcome from the turn it is replacing.
func (s *Store) WorldStateAtTurnParent(storyID, branchID, turnID string) (WorldState, error) {
	ctx, err := s.StoryContextAtTurnParent(storyID, branchID, turnID)
	if err != nil {
		return WorldState{}, err
	}
	view, err := s.TurnWorldView(storyID, branchID, turnID)
	if err != nil {
		return WorldState{}, err
	}
	p := &storyBranchProjection{WorldContent: worldContentForMeta(ctx.Meta), CultivationRules: cultivationRulesForMeta(ctx.Meta), State: ctx.Snapshot.State, PackCatalog: ctx.Meta.PackCatalog, LatestTurnID: view.LatestTurnID, Characters: view.Characters, Combats: view.Combats, Recoveries: view.Recoveries, Cultivations: view.Cultivations, Clock: view.Clock, Journeys: view.Journeys, Realms: view.Realms, Quests: view.Quests, PublicEvents: view.PublicEvents, LocalAffairs: view.LocalAffairs}
	system := actorStateSystemFromSnapshot(ctx.Meta.ActorStateSchema, s.storyDirectorForMeta(ctx.Meta).ActorState)
	p.bindWorld(ctx.Meta)
	return worldStateFromProjection(p, ctx.Meta, system)
}
