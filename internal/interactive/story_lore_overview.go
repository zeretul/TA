package interactive

import (
	"strings"
	"ta/internal/character"
	"ta/internal/exploration"
)

// LoreOverview is a detached set of views of one canonical journal commit.
// Callers must not combine it with later reads when projecting player knowledge.
type LoreOverview struct {
	Context        StoryContext
	Characters     character.State
	Clock          StoryClock
	Journeys       map[string]exploration.JourneySnapshot
	JourneyOptions JourneyOptions
	Quests         QuestState
	Events         PublicEventState
}

// ReadLoreOverview holds both the Store mutex and the canonical story lease for
// the entire read, including snapshot materialization. Other Store instances or
// processes cannot append between its component projections.
func (s *Store) ReadLoreOverview(storyID, branchID string) (LoreOverview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	storyID, branchID = strings.TrimSpace(storyID), strings.TrimSpace(branchID)
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return LoreOverview{}, err
	}
	defer release()
	meta, snapshot, err := s.boundedStorySnapshotLocked(storyID, branchID)
	if err != nil {
		return LoreOverview{}, err
	}
	projection, err := s.storyBranchProjectionLocked(storyID, snapshot.BranchID)
	if err != nil {
		return LoreOverview{}, err
	}
	snapshot.ActorStateSchema = actorStateSchemaWithPresetFloor(snapshot.ActorStateSchema, meta)
	clock, err := branchStoryClock(projection, DefaultStoryContextActorID, actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState))
	if err != nil {
		return LoreOverview{}, err
	}
	characters := character.State{Version: character.StateVersion}
	if projection.Characters != nil {
		characters = cloneCharacterState(projection.Characters).Normalize()
	}
	return LoreOverview{
		Context: StoryContext{Meta: meta, Snapshot: snapshot}, Characters: characters,
		Clock: clock, Journeys: cloneJourneySnapshots(projection.Journeys), JourneyOptions: journeyOptionsFrom(projection),
		Quests: questStateFrom(meta, projection, clock), Events: publicEventStateFrom(meta, projection, clock),
	}, nil
}
