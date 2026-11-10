package interactive

import (
	"fmt"
	"strings"

	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/exploration"
	"ta/internal/publiquevent"
	"ta/internal/quest"
)

// TurnWorldView is the committed world a turn's world module is judged against:
// the formal clock, the unfinished travels, the realm instances, the task
// instances and the public-event participations of the branch the turn will be
// appended to.
//
// The submission can dry-run the same planner the commit runs, so a world action
// the rules will refuse is refused while the narrator can still rewrite, instead
// of being logged and dropped after the prose has already been accepted. The
// view is deliberately narrow: it carries the records the planners read and
// nothing else, so it cannot be mistaken for a second projection or a place to
// cache state.
type TurnWorldView struct {
	LocalAffairs []LocalAffairState
	// Discovery uses the same committed turn and cast as the final write.
	LatestTurnID string
	Characters   *character.State
	Combats      map[string]cultivation.CombatSession
	Recoveries   map[string]RecoveryActivity
	Cultivations map[string]CultivationActivity
	Clock        *StoryClock
	Journeys     map[string]exploration.JourneySnapshot
	Realms       map[string]RealmInstanceState
	Quests       map[string]quest.Instance
	PublicEvents map[string]publiquevent.Instance
}

// TurnWorldView reads the committed world for the branch a turn is about to be
// appended to.
//
// replaceTurnID is the regenerate target. A regenerated turn is judged against
// the world as it stood at its parent, not against the live branch, because the
// replacement turn has not happened yet; the caller passes the same target it
// passes to StoryContextAtTurnParent, so the dry run and the model projection
// describe one moment.
func (s *Store) TurnWorldView(storyID, branchID, replaceTurnID string) (TurnWorldView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return TurnWorldView{}, err
	}
	defer release()
	meta, lines, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return TurnWorldView{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return TurnWorldView{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	if target := strings.TrimSpace(replaceTurnID); target != "" {
		if err := requireLatestLogicalTurn(meta, lines, branchID, target); err != nil {
			return TurnWorldView{}, err
		}
		parentID, err := regenerationParentOnCurrentPath(lines, branch.Head, target)
		if err != nil {
			return TurnWorldView{}, err
		}
		path, parentAncestors, err := walkEventPath(parentID, eventsByID(lines))
		if err != nil {
			return TurnWorldView{}, err
		}
		time, err := storyTimeFromRecords(path, meta)
		if err != nil {
			return TurnWorldView{}, err
		}
		latestTurnID := ""
		for _, record := range path {
			if record.Envelope.Type == StoryEventTypeTurn {
				latestTurnID = record.Envelope.ID
			}
		}
		var characters *character.State
		// Side-window combat can begin after the previous main turn. Its exact
		// parent path, not that older turn checkpoint, is the qualification.
		combats, err := combatsFromPath(path)
		if err != nil {
			return TurnWorldView{}, err
		}
		if latestTurnID != "" {
			checkpoint, checkpointErr := s.checkpointAtTurnLocked(storyID, latestTurnID)
			if checkpointErr != nil {
				return TurnWorldView{}, checkpointErr
			}
			characters = cloneCharacterState(checkpoint.Characters)
		}
		projection, projectionErr := s.storyBranchProjectionLocked(storyID, branchID)
		if projectionErr != nil {
			return TurnWorldView{}, projectionErr
		}
		if projection.TimeBeforeLatest != nil {
			// The exact parent includes window exchanges between the previous
			// main turn and this one. An earlier turn checkpoint omits them.
			characters = cloneCharacterState(projection.CharactersBeforeLatest)
		} else {
			handle, handleErr := s.refreshStoryJournalLocked(storyID, true)
			if handleErr != nil {
				return TurnWorldView{}, handleErr
			}
			characters, err = s.charactersForStoryAncestorsLocked(handle, parentAncestors, branchID)
			if err != nil {
				return TurnWorldView{}, err
			}
		}
		return TurnWorldView{
			LatestTurnID: latestTurnID, Characters: characters, Combats: combats,
			Clock:        time.Clock,
			Recoveries:   cloneRecoveryActivities(time.Recoveries),
			Cultivations: cloneCultivationActivities(time.Cultivations),
			Journeys:     time.Journeys,
			Realms:       time.Realms,
			Quests:       time.Quests,
			PublicEvents: time.PublicEvents,
			LocalAffairs: cloneLocalAffairs(time.LocalAffairs),
		}, nil
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return TurnWorldView{}, err
	}
	return TurnWorldView{
		LatestTurnID: projection.LatestTurnID,
		Characters:   cloneCharacterState(projection.Characters),
		Combats:      cloneCombatSessions(projection.Combats),
		Clock:        cloneStoryClock(projection.Clock),
		Recoveries:   cloneRecoveryActivities(projection.Recoveries),
		Cultivations: cloneCultivationActivities(projection.Cultivations),
		Journeys:     cloneJourneySnapshots(projection.Journeys),
		Realms:       cloneRealmInstances(projection.Realms),
		Quests:       cloneQuestInstances(projection.Quests),
		PublicEvents: clonePublicEventInstances(projection.PublicEvents),
		LocalAffairs: cloneLocalAffairs(projection.LocalAffairs),
	}, nil
}
