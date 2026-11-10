package interactive

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"ta/internal/character"
)

// ErrBranchTerminal is what every write to a finished branch reports.
//
// 自制-死亡与分支结局 §3 ends the branch, not the story: the history stays
// readable and a fork from an earlier turn is the way on. The check is the same
// one AppendTurn makes, and it has to be made by every command that moves the
// branch head — a quest accepted after the protagonist died would otherwise be
// recorded as something they did. It is a sentinel so a caller can tell this
// apart from a malformed request and offer the fork instead of a retry.
var ErrBranchTerminal = errors.New("story branch has reached its terminal outcome")

func branchTerminalError() error {
	return fmt.Errorf("%w: 当前分支已终局，请从历史回合创建新分支后继续", ErrBranchTerminal)
}

func (s *Store) AppendTurn(storyID string, req AppendTurnRequest) (TurnEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return TurnEvent{}, err
	}
	defer releaseStory()

	meta, lines, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return TurnEvent{}, err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return TurnEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	if branchIsTerminal(lines, branch.Head) {
		return TurnEvent{}, fmt.Errorf("当前分支已终局，请从历史回合创建新分支后继续")
	}
	parentID := any(nil)
	if branch.Head != "" {
		parentID = branch.Head
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := TurnEvent{
		V:                    schemaVersion,
		Type:                 StoryEventTypeTurn,
		ID:                   newID("ev"),
		ParentID:             parentID,
		BranchID:             branchID,
		Ts:                   now,
		User:                 req.User,
		Narrative:            req.Narrative,
		Thinking:             strings.TrimSpace(req.Thinking),
		DisplayEvents:        sanitizeDisplayEvents(req.DisplayEvents),
		ModelContextMessages: sanitizeModelContextMessages(req.ModelContextMessages),
		Flags:                map[string]bool{"pinned": false, "locked": false},
	}
	branch.Head = event.ID
	generatePendingStoryTitle(&meta, event.Narrative)
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	continuationEvents, err := newModelContextProviderContinuationEvents(event.ID, event.BranchID, event.Ts, event.ModelContextMessages)
	if err != nil {
		return TurnEvent{}, err
	}
	newEvents := []any{event}
	newEvents = append(newEvents, continuationEvents...)
	if err := s.appendStoryTransactionLocked(storyID, meta, newEvents...); err != nil {
		return TurnEvent{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return TurnEvent{}, err
	}
	return event, nil
}

// AppendTurnDisplayEvent appends a display-only event to an existing turn.
// The event is kept out of future model context and does not move branch head.
func (s *Store) AppendTurnDisplayEvent(storyID, branchID, turnID string, event DisplayEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return err
	}
	defer releaseStory()

	events := sanitizeDisplayEvents([]DisplayEvent{event})
	if len(events) == 0 {
		return nil
	}
	meta, lines, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	if _, ok := meta.Branches[branchID]; !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return fmt.Errorf("展示事件缺少所属回合")
	}
	if err := requireLatestLogicalTurn(meta, lines, branchID, turnID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	revision := TurnDisplayAppendedEvent{
		V: schemaVersion, Type: StoryEventTypeTurnDisplayAppended, ID: newID("tda"),
		ParentID: turnID, BranchID: branchID, Ts: now, TurnID: turnID, Display: events[0],
	}
	if err := s.appendStoryTransactionLocked(storyID, meta, revision); err != nil {
		return err
	}
	return s.syncStorySummaryLocked(storyID)
}

func (s *Store) RewindToTurnParent(storyID string, req RewindTurnRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return err
	}
	defer releaseStory()

	turnID := strings.TrimSpace(req.TurnID)
	if turnID == "" {
		return fmt.Errorf("回合 ID 不能为空")
	}
	meta, lines, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	events := eventsByID(lines)
	path, pathSet, err := walkEventPath(branch.Head, events)
	if err != nil {
		return err
	}
	if !pathSet[turnID] {
		return fmt.Errorf("只能编辑当前剧情路径上的回合: %s", turnID)
	}
	var target *StoryEventRecord
	for i := range path {
		if path[i].Envelope.ID == turnID && path[i].Envelope.Type == StoryEventTypeTurn {
			target = &path[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("回合不存在: %s", turnID)
	}
	previousHead := branch.Head
	branch.Head = parentIDFromRaw(target.Raw)
	projected, err := projectStoryEventOverlays(lines)
	if err != nil {
		return err
	}
	pathToParent, _, err := walkEventPath(branch.Head, eventsByID(projected))
	if err != nil {
		return err
	}
	nextLatestTurnID := nearestTurnAncestor(branch.Head, eventsByID(projected))
	nextDepth := 0
	for _, record := range pathToParent {
		if record.Envelope.Type == StoryEventTypeTurn {
			nextDepth++
		}
	}
	meta.Branches[branchID] = branch
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	stateCheckpoint := stateFromPath(pathToParent)
	// The fights come from the same path the state does. Deriving them from the
	// nearest Turn ancestor instead would restore a state whose pools carry a
	// battle's wounds while dropping the battle that dealt them, and the window
	// would then show damage with nothing behind it.
	combatCheckpoint, err := combatsFromPath(pathToParent)
	if err != nil {
		return err
	}
	// The clock and the activities that spent it come from the same path for the
	// same reason. Restoring the pools to an earlier instant while keeping the
	// later clock would leave the branch claiming time it rewound past.
	storyTime, err := storyTimeFromRecords(pathToParent, meta)
	if err != nil {
		return err
	}
	clockCheckpoint, recoveryCheckpoint, cultivationCheckpoint := storyTime.Clock, storyTime.Recoveries, storyTime.Cultivations
	var planCheckpoint *BranchPlan
	var characterCheckpoint *character.State
	if turnID == latestLogicalTurnID(meta, lines, branchID) {
		if projection, projectionErr := s.storyBranchProjectionLocked(storyID, branchID); projectionErr == nil {
			stateCheckpoint = cloneStoryState(projection.StateBeforeLatest)
			planCheckpoint = cloneBranchPlan(projection.PlanBeforeLatest)
		}
	} else if nextLatestTurnID != "" {
		if checkpoint, checkpointErr := s.checkpointAtTurnLocked(storyID, nextLatestTurnID); checkpointErr == nil {
			planCheckpoint = cloneBranchPlan(checkpoint.Plan)
			characterCheckpoint = cloneCharacterState(checkpoint.Characters)
		}
	}
	if characterCheckpoint == nil && turnID == latestLogicalTurnID(meta, lines, branchID) {
		// Rewinding to just before the latest Turn restores memory to the same
		// point the state checkpoint restores to.
		if projection, projectionErr := s.storyBranchProjectionLocked(storyID, branchID); projectionErr == nil {
			characterCheckpoint = cloneCharacterState(projection.Characters)
		}
	}
	event := BranchHeadMovedEvent{
		V: schemaVersion, Type: StoryEventTypeBranchHeadMoved, ID: newID("bhm"),
		ParentID: branch.Head, BranchID: branchID, Ts: now,
		PreviousHead: previousHead, NextHead: branch.Head, NextLatestTurnID: nextLatestTurnID,
		NextDepth: nextDepth, StateCheckpoint: stateCheckpoint, PlanCheckpoint: planCheckpoint, Reason: "rewind_to_turn_parent",
		CharacterCheckpoint:   characterCheckpoint,
		CombatCheckpoint:      combatCheckpoint,
		ClockCheckpoint:       clockCheckpoint,
		RecoveryCheckpoint:    recoveryCheckpoint,
		CultivationCheckpoint: cultivationCheckpoint,
		JourneyCheckpoint:     storyTime.Journeys,
		LocalAffairCheckpoint: cloneLocalAffairs(storyTime.LocalAffairs),
		RealmCheckpoint:       storyTime.Realms,
		QuestCheckpoint:       storyTime.Quests,
		PublicEventCheckpoint: storyTime.PublicEvents,
	}
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return err
	}
	return s.syncStorySummaryLocked(storyID)
}
