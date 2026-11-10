package interactive

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"ta/internal/agents/conversationjournal"
	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/exploration"
	"ta/internal/npcgrowth"
	"ta/internal/publiquevent"
	"ta/internal/quest"
)

const storyCheckpointScanTransactions = 1024

type storyEventCheckpoint struct {
	LocalAffairs   []LocalAffairState
	SourceBranchID string
	LatestTurnID   string
	Depth          int
	State          map[string]any
	Plan           *BranchPlan
	Characters     *character.State
	// Combats is the live battles as of this checkpoint, so a fork or a rewind
	// restores the fights along with the wounds they dealt.
	Combats map[string]cultivation.CombatSession
	// Clock is the formal time axis as of this checkpoint, so a fork or a rewind
	// restores how much in-fiction time the branch had spent along with the pools
	// that time produced.
	Clock *StoryClock
	// Recoveries is the safe recovery activities as of this checkpoint. A
	// half-finished rest restored without its activity would let the remainder be
	// earned a second time.
	Recoveries map[string]RecoveryActivity
	// Cultivations is the player-named cultivation intervals as of this
	// checkpoint. A half-finished interval restored without its activity would let
	// the remainder be trained a second time.
	Cultivations map[string]CultivationActivity
	// Journeys is the branch's travel as of this checkpoint. A half-walked road
	// restored without its progress would let the remainder be charged twice.
	Journeys map[string]exploration.JourneySnapshot
	// Realms is the branch's realm instances as of this checkpoint. A realm
	// restored without its rooms and taken rewards would hand the same treasure
	// out again.
	Realms map[string]RealmInstanceState
	// Quests is the branch's task instances as of this checkpoint. A task
	// restored without its objectives and its settled flag would be completed or
	// paid a second time.
	Quests map[string]quest.Instance
	// PublicEvents is the branch's public event participation as of this
	// checkpoint. A participation restored without the stage it was made at would
	// let the character claim to have been present somewhere they were not.
	PublicEvents map[string]publiquevent.Instance
	// NPCGrowth is each non-protagonist actor's growth watermark as of this
	// checkpoint. A fork restored with a stage but not the instant the rules were
	// paid for would either settle the same time twice or refuse the next
	// interval as overlapping.
	NPCGrowth map[string]npcgrowth.Snapshot
}

// checkpointAtTurnLocked resolves one historical turn without materializing
// the story. The backwards pass retains only ancestor IDs; two forward
// streaming passes then account for append-only state revisions and build the
// exact branch checkpoint in chronological order.
func (s *Store) checkpointAtTurnLocked(storyID, turnID string) (storyEventCheckpoint, error) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return storyEventCheckpoint{}, fmt.Errorf("父回合不能为空 / Parent turn is required")
	}
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return storyEventCheckpoint{}, err
	}
	branchIDs := make([]string, 0, len(handle.projection.Meta.Branches))
	for branchID := range handle.projection.Meta.Branches {
		branchIDs = append(branchIDs, branchID)
	}
	sort.Strings(branchIDs)

	for _, pathBranchID := range branchIDs {
		ancestorIDs := make(map[string]bool)
		cursor := ""
		found := false
		sourceBranchID := ""
		depth := 0
		for {
			loaded, readErr := s.readStoryHistoryPageLocked(storyID, pathBranchID, cursor, maxStoryHistoryPageTurns, true)
			if readErr != nil {
				return storyEventCheckpoint{}, readErr
			}
			path, _, pathErr := walkEventPath(loaded.pageHeadID, eventsByID(loaded.records))
			if pathErr != nil {
				return storyEventCheckpoint{}, pathErr
			}
			through := len(path)
			if !found {
				through = 0
				for index, record := range path {
					if record.Envelope.Type != StoryEventTypeTurn || record.Envelope.ID != turnID {
						continue
					}
					found = true
					through = index + 1
					sourceBranchID = record.Envelope.BranchID
					break
				}
			}
			if found {
				for _, record := range path[:through] {
					if ancestorIDs[record.Envelope.ID] {
						continue
					}
					ancestorIDs[record.Envelope.ID] = true
					if record.Envelope.Type == StoryEventTypeTurn {
						depth++
					}
				}
			}
			if !loaded.page.HasMore || strings.TrimSpace(loaded.page.BeforeCursor) == "" {
				break
			}
			cursor = loaded.page.BeforeCursor
		}
		if !found {
			continue
		}
		state, stateErr := s.stateForStoryAncestorsLocked(handle, ancestorIDs)
		if stateErr != nil {
			return storyEventCheckpoint{}, stateErr
		}
		plan, planErr := s.planForStoryAncestorsLocked(handle, ancestorIDs)
		if planErr != nil {
			return storyEventCheckpoint{}, planErr
		}
		characters, charactersErr := s.charactersForStoryAncestorsLocked(handle, ancestorIDs, sourceBranchID)
		if charactersErr != nil {
			return storyEventCheckpoint{}, charactersErr
		}
		combats, combatsErr := s.combatsForStoryAncestorsLocked(handle, ancestorIDs)
		if combatsErr != nil {
			return storyEventCheckpoint{}, combatsErr
		}
		storyTime, storyTimeErr := s.storyTimeForStoryAncestorsLocked(handle, ancestorIDs)
		if storyTimeErr != nil {
			return storyEventCheckpoint{}, storyTimeErr
		}
		// A creator can revise the plan after the latest Turn without adding a
		// new Turn. Forking that current Turn should inherit the live revision;
		// forks from older Turns must keep their historical checkpoint instead.
		if branch := handle.projection.Branches[pathBranchID]; branch != nil && branch.LatestTurnID == turnID {
			plan = cloneBranchPlan(branch.Plan)
		}
		// Character memory forks the same way: a fork of the current turn
		// inherits what the branch knows now, including anything said in the
		// window after the last Turn. A fork of an older turn gets the memory as
		// of that turn.
		if branch := handle.projection.Branches[pathBranchID]; branch != nil && branch.LatestTurnID == turnID {
			characters = cloneCharacterState(branch.Characters)
		}
		// Battles fork the same way. A fork of the current turn inherits the
		// fights that are still live now; a fork of an older turn gets the fights
		// as they stood at that turn, which the ancestor scan above produced.
		if branch := handle.projection.Branches[pathBranchID]; branch != nil && branch.LatestTurnID == turnID {
			combats = cloneCombatSessions(branch.Combats)
		}
		// The formal clock and the rest activities deliberately take no override
		// here. A rest commits as a record parented to the Turn it followed, so a
		// fork cut at that Turn sits before it and the ancestor scan above already
		// produced the instant and the activities that belong to that Turn. Taking
		// the live branch's clock instead would hand the fork a rest's worth of
		// time and a completed activity while the pools it restored stayed behind
		// in main — §三.5's half-applied commit, and the fork could never earn the
		// pool because its own record reads as finished.
		return storyEventCheckpoint{
			SourceBranchID: sourceBranchID,
			LatestTurnID:   turnID,
			Depth:          depth,
			State:          state,
			Plan:           plan,
			Characters:     characters,
			Combats:        combats,
			Clock:          storyTime.Clock,
			Recoveries:     storyTime.Recoveries,
			Cultivations:   storyTime.Cultivations,
			Journeys:       storyTime.Journeys,
			Realms:         storyTime.Realms,
			LocalAffairs:   cloneLocalAffairs(storyTime.LocalAffairs),
			Quests:         storyTime.Quests,
			PublicEvents:   storyTime.PublicEvents,
			NPCGrowth:      storyTime.NPCGrowth,
		}, nil
	}
	return storyEventCheckpoint{}, fmt.Errorf("父回合不存在 / Parent turn not found: %s", turnID)
}

// belongsToStoryAncestorPath is the one attribution rule every side-record
// projection uses to decide whether a record is part of the forked path.
//
// A record is on the path when its own envelope is an ancestor. When it is not,
// a record that names a Turn is still part of what that Turn established: a Turn
// publishes its clock records, its battles and its character facts *after* itself
// rather than to itself, so an ancestor cut taken exactly at the Turn excludes
// them by envelope while the Turn's own state delta — the growth, the wound, the
// scene — stays in. Keeping the record and dropping its consequence is §三.5's
// half-applied commit in the other direction: the fork would hold the restored
// pool without the interval that earned it, or the enemy standing over it without
// the fight that put them there.
//
// A record that names no Turn keeps matching by position alone. That half is what
// keeps a fork honest: a rest taken from the light entry, or a conversation held
// in the window after a Turn, is not part of that Turn, and a fork cut at it must
// not inherit time it never spent.
func belongsToStoryAncestorPath(ancestorIDs map[string]bool, recordID, turnID string) bool {
	if ancestorIDs[recordID] {
		return true
	}
	turnID = strings.TrimSpace(turnID)
	return turnID != "" && ancestorIDs[turnID]
}

// charactersForStoryAncestorsLocked rebuilds the character projection over a
// set of ancestor records. It mirrors stateForStoryAncestorsLocked so a fork
// restores memory by the same rule it restores state.
func (s *Store) charactersForStoryAncestorsLocked(handle *storyJournalHandle, ancestorIDs map[string]bool, targetBranch string) (*character.State, error) {
	state := &character.State{Version: character.StateVersion}
	lineage := map[string]bool{}
	for id := targetBranch; id != "" && !lineage[id]; {
		lineage[id] = true
		id = handle.projection.Meta.Branches[id].From
	}
	any := false
	if err := scanStoryEventsLocked(handle, func(record StoryEventRecord) error {
		if record.Envelope.Type == StoryEventTypeBranch && lineage[record.Envelope.BranchID] {
			var event BranchEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return err
			}
			if ancestorIDs[event.ParentID] {
				state = forkCooperationCharacters(state, event.RealmCheckpoint, event.BranchID, worldContentForMeta(handle.projection.Meta))
			}
			return nil
		}
		if record.Envelope.Type == StoryEventTypeCooperationRecord {
			var event CooperationRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return err
			}
			if !belongsToStoryAncestorPath(ancestorIDs, record.Envelope.ID, event.TurnID) {
				return nil
			}
			updated, err := applyCooperationRecordCharacters(state, event, handle.projection.StoryID)
			if err != nil {
				return err
			}
			state = updated
			any = true
			return nil
		}
		if record.Envelope.Type != StoryEventTypeCharacterRecord {
			return nil
		}
		var event CharacterRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		// A turn's own facts are parented after the turn rather than to it, so the
		// ancestor cut at the turn excludes the record by envelope. It is still
		// part of what that turn established — the obligation it took on, the
		// people it met — so it travels with the turn it names, the way the branch
		// plan does. A window exchange names no turn and stays matched by its own
		// position, which is what keeps a fork from inheriting what was said in
		// the window after the turn it forked from.
		if !belongsToStoryAncestorPath(ancestorIDs, record.Envelope.ID, event.TurnID) {
			return nil
		}
		branch := &storyBranchProjection{Characters: state}
		if err := applyCharacterRecord(branch, event); err != nil {
			return err
		}
		state = branch.Characters
		any = true
		return nil
	}); err != nil {
		return nil, err
	}
	if !any {
		return nil, nil
	}
	normalized := state.Normalize()
	return &normalized, nil
}

// combatsForStoryAncestorsLocked rebuilds the live battles over a set of
// ancestor records. It mirrors charactersForStoryAncestorsLocked, including the
// path rule: a battle a regenerated turn left behind is not part of this path
// and must not be restored.
func (s *Store) combatsForStoryAncestorsLocked(handle *storyJournalHandle, ancestorIDs map[string]bool) (map[string]cultivation.CombatSession, error) {
	branch := &storyBranchProjection{}
	if err := scanStoryEventsLocked(handle, func(record StoryEventRecord) error {
		if record.Envelope.Type != StoryEventTypeCombatRecord {
			return nil
		}
		var event CombatRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		if !belongsToStoryAncestorPath(ancestorIDs, record.Envelope.ID, event.TurnID) {
			return nil
		}
		return applyCombatRecord(branch, event)
	}); err != nil {
		return nil, err
	}
	return cloneCombatSessions(branch.Combats), nil
}

func (s *Store) planForStoryAncestorsLocked(handle *storyJournalHandle, ancestorIDs map[string]bool) (*BranchPlan, error) {
	var plan *BranchPlan
	if err := scanStoryEventsLocked(handle, func(record StoryEventRecord) error {
		if record.Envelope.Type != StoryEventTypeBranchPlanUpdated && record.Envelope.Type != StoryEventTypeBranchPlanRevised {
			return nil
		}
		var event BranchPlanUpdatedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		belongsToPath := ancestorIDs[event.TurnID]
		if record.Envelope.Type == StoryEventTypeBranchPlanRevised {
			belongsToPath = ancestorIDs[event.ID]
		}
		if belongsToPath {
			plan = &BranchPlan{Markdown: event.Markdown, UpdatedTurnID: event.TurnID, UpdatedAt: event.Ts, Revision: event.ID}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return plan, nil
}

func (s *Store) stateForStoryAncestorsLocked(handle *storyJournalHandle, ancestorIDs map[string]bool) (map[string]any, error) {
	revisions := make(map[string]TurnStateRevisedEvent)
	if err := scanStoryEventsLocked(handle, func(record StoryEventRecord) error {
		if record.Envelope.Type != StoryEventTypeTurnStateRevised {
			return nil
		}
		var revision TurnStateRevisedEvent
		if err := mapToStruct(record.Raw, &revision); err != nil {
			return err
		}
		if ancestorIDs[revision.TurnID] {
			revisions[revision.TurnID] = revision
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// A settlement commits as a side record parented after the Turn it names, and
	// writes its actor writes as a delta parented to that record. Both therefore
	// sit past the ancestor cut even though belongsToStoryAncestorPath already
	// counts the record as part of what the Turn established — which is how the
	// growth watermark advances on the fork. The delta carries no Turn of its own,
	// so the envelope test alone dropped it: the fork inherited a watermark that
	// had already been paid for with a progress of zero, §三.5's half-applied
	// commit. Collect the side records on the path so their writes travel too.
	//
	// The rule is deliberately the same one the clock and the activities use. A
	// settlement taken from the light entry names no Turn, so it is off the path
	// for both its watermark and its writes, and a fork keeps neither.
	settlementRecords := make(map[string]bool)
	if err := scanStoryEventsLocked(handle, func(record StoryEventRecord) error {
		if ancestorIDs[record.Envelope.ID] {
			return nil
		}
		var side struct {
			TurnID string `json:"turn_id"`
		}
		if err := mapToStruct(record.Raw, &side); err != nil {
			return nil
		}
		if belongsToStoryAncestorPath(ancestorIDs, record.Envelope.ID, side.TurnID) {
			settlementRecords[record.Envelope.ID] = true
		}
		return nil
	}); err != nil {
		return nil, err
	}

	state := initialStoryState()
	if err := scanStoryEventsLocked(handle, func(record StoryEventRecord) error {
		if !ancestorIDs[record.Envelope.ID] &&
			!(record.Envelope.Type == StoryEventTypeStateDelta && settlementRecords[parentIDString(record.Envelope.ParentID)]) {
			return nil
		}
		switch record.Envelope.Type {
		case StoryEventTypeStateDelta:
			var delta StateDeltaEvent
			if err := mapToStruct(record.Raw, &delta); err != nil {
				return err
			}
			applyStateDeltaToProjection(state, StateDelta{
				SchemaVersion: delta.SchemaVersion, Ops: delta.Ops, ActorOps: delta.ActorOps,
				WrittenAtSeconds: delta.WrittenAtSeconds, WrittenAtEvent: delta.WrittenAtEvent,
			})
		case StoryEventTypeTurn:
			var turn TurnEvent
			if err := mapToStruct(record.Raw, &turn); err != nil {
				return err
			}
			if revision, ok := revisions[turn.ID]; ok {
				if revision.ClearStateDelta {
					turn.StateDelta = nil
				} else if revision.StateDelta != nil {
					value := *revision.StateDelta
					turn.StateDelta = &value
				}
			}
			applyTurnState(state, turn)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return state, nil
}

func scanStoryEventsLocked(handle *storyJournalHandle, visit func(StoryEventRecord) error) error {
	if handle == nil || handle.journal == nil {
		return fmt.Errorf("story journal is unavailable")
	}
	head := handle.journal.Head().Cursor
	for after := conversationjournal.Cursor(0); after < head; {
		records, err := handle.journal.ReadRange(context.Background(), conversationjournal.Range{
			After: after, Through: head, Limit: storyCheckpointScanTransactions,
		})
		if err != nil {
			return err
		}
		next := after
		for _, physical := range records {
			if physical.Location.Cursor > next {
				next = physical.Location.Cursor
			}
			_, events, decodeErr := decodeStoryProjectionPayload(physical.Payload)
			if decodeErr != nil {
				return decodeErr
			}
			for _, record := range events {
				if err := visit(record); err != nil {
					return err
				}
			}
		}
		if next <= after {
			return fmt.Errorf("story journal scan made no progress after cursor %d", after)
		}
		after = next
	}
	return nil
}
