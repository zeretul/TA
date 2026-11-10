package interactive

// A newly forked, rewound or version-selected branch may not have the latest
// turn's before-checkpoint. Reconstruct the exact parent from existing journal
// facts and publish the already-supported head-move checkpoint atomically with
// the replacement. A cache-only repair would revive the removed facts on reopen.
func (s *Store) rebuildRegenerationParentLocked(storyID, branchID, previousHead string, parent *storyBranchProjection, path []StoryEventRecord, ancestors map[string]bool) (*storyBranchProjection, *BranchHeadMovedEvent, error) {
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return nil, nil, err
	}
	state, err := s.stateForStoryAncestorsLocked(handle, ancestors)
	if err != nil {
		return nil, nil, err
	}
	characters, err := s.charactersForStoryAncestorsLocked(handle, ancestors, branchID)
	if err != nil {
		return nil, nil, err
	}
	plan, err := s.planForStoryAncestorsLocked(handle, ancestors)
	if err != nil {
		return nil, nil, err
	}
	rebuilt := *parent
	rebuilt.State, rebuilt.Plan = state, plan
	rebuilt.Realms = forkRealmInstances(parent.Realms, branchID)
	rebuilt.Characters = forkCooperationCharacters(characters, rebuilt.Realms, branchID, rebuilt.worldContent())
	rebuilt.Depth = 0
	for _, record := range path {
		if record.Envelope.Type == StoryEventTypeTurn {
			rebuilt.Depth++
			rebuilt.LatestTurnID = record.Envelope.ID
		}
	}
	event := &BranchHeadMovedEvent{
		V: schemaVersion, Type: StoryEventTypeBranchHeadMoved, ID: newID("bhm"), ParentID: parent.Head, BranchID: branchID,
		PreviousHead: previousHead, NextHead: parent.Head, NextLatestTurnID: rebuilt.LatestTurnID, NextDepth: rebuilt.Depth,
		StateCheckpoint: cloneStoryState(rebuilt.State), PlanCheckpoint: cloneBranchPlan(rebuilt.Plan), Reason: "regenerate_parent_checkpoint",
		CharacterCheckpoint: cloneCharacterState(rebuilt.Characters), CombatCheckpoint: cloneCombatSessions(rebuilt.Combats),
		ClockCheckpoint: cloneStoryClock(rebuilt.Clock), RecoveryCheckpoint: cloneRecoveryActivities(rebuilt.Recoveries), CultivationCheckpoint: cloneCultivationActivities(rebuilt.Cultivations),
		JourneyCheckpoint: cloneJourneySnapshots(rebuilt.Journeys), RealmCheckpoint: cloneRealmInstances(rebuilt.Realms), QuestCheckpoint: cloneQuestInstances(rebuilt.Quests), PublicEventCheckpoint: clonePublicEventInstances(rebuilt.PublicEvents),
		NPCGrowthCheckpoint: cloneNPCGrowthSnapshots(rebuilt.NPCGrowth), LocalAffairCheckpoint: cloneLocalAffairs(rebuilt.LocalAffairs),
	}
	return &rebuilt, event, nil
}
