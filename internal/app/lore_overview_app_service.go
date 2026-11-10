package app

import "ta/internal/interactive"

// LoreOverview is a player-facing atomic reading, never a turn or model call.
type LoreOverview struct {
	Snapshot   interactive.Snapshot `json:"snapshot"`
	Characters CharacterWindow      `json:"characters"`
	Journey    JourneyState         `json:"journey"`
	Quests     QuestAppState        `json:"quests"`
	Events     PublicEventAppState  `json:"events"`
}

func (a *App) ReadLoreOverview(storyID, branchID, locale string) (LoreOverview, error) {
	if a == nil {
		return LoreOverview{}, ErrNoWorkspace
	}
	a.mu.RLock()
	store := a.interactive
	a.mu.RUnlock()
	if store == nil {
		return LoreOverview{}, ErrNoWorkspace
	}
	facts, err := store.ReadLoreOverview(storyID, branchID)
	if err != nil {
		return LoreOverview{}, err
	}
	storyID, branchID = facts.Context.Snapshot.StoryID, facts.Context.Snapshot.BranchID
	a.mu.RLock()
	projectID := ""
	if a.cfg != nil {
		projectID = a.cfg.ProjectID
	}
	a.mu.RUnlock()
	characters, err := a.characterWindowFrom(CharacterWindowRequest{StoryID: storyID, BranchID: branchID, Locale: locale}, projectID, facts.Context, facts.Characters)
	if err != nil {
		return LoreOverview{}, err
	}
	snapshot := facts.Context.Snapshot
	redactInteractiveSnapshotAttachmentPaths(&snapshot)
	projectInteractiveSnapshotForPlayer(&snapshot, loreAudience(facts.Characters))
	return LoreOverview{
		Snapshot: snapshot, Characters: characters,
		Journey: a.localizeJourneyState(journeyStateFrom(storyID, branchID, facts.Clock, facts.Journeys, facts.JourneyOptions), facts.Context, locale),
		Quests:  a.questStateFrom(storyID, branchID, locale, facts.Context, facts.Quests),
		Events:  localizePublicEventState(publicEventAppStateFrom(storyID, branchID, facts.Events), locale),
	}, nil
}
