package interactive

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"ta/internal/publiquevent"
	"ta/internal/storytime"
)

// This file makes a public event a real, shared situation on the branch.
//
// 自制-事件与任务 §4.4, §4.5 and §6 ask for a public event that many people can
// care about, whose stage moves with the world, whose participants are told
// different things, and whose player view shows only what they really know. The
// properties that make that honest are all in the commit and the read below.
//
//   - The public situation is not stored. A stage is a function of the branch's
//     committed formal clock, so it is read rather than written. That is why
//     there is no scheduler here and no per-turn roll: the same journal always
//     reports the same stage, a fork that happened earlier sees an earlier
//     stage, and no narration can make the event advance when no time was spent.
//
//   - Participation is the only fact. What the character did, at which stage and
//     at what instant is a side record. It changes nothing about the event, which
//     is what §4.4 means by keeping the event's state apart from a participant's.
//
//   - Knowledge is filtered by the rules package. The secret lines never leave
//     it, and the participant lines are handed out only for the stage the
//     character was actually present at.
//
// It is a side record for the same reason a journey is: taking part in a public
// event is not a story turn, and making it one would let a window open advance
// the world.

// StateOpSourcePublicEvent marks an Actor State operation a committed public
// event participation produced.
const StateOpSourcePublicEvent = "public_event"

// StoryEventTypePublicEventRecord is a side record carrying one character's
// participation in a public event. The type itself is declared in
// story_schema.go beside the other record types.
//
// There is no record for the event's own state. Storing a stage would create a
// second clock that could disagree with the branch's, which is exactly the
// failure the queue's "时间与知情真实" rules out.

// PublicEventRecordEvent is one participation action together with the whole
// instance after it.
type PublicEventRecordEvent struct {
	V        int    `json:"v"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	BranchID string `json:"branch_id"`
	Ts       string `json:"ts"`
	TurnID   string `json:"turn_id,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
	// CommandID and CommandHash identify one attempt, so a retry replays.
	CommandID   string `json:"command_id,omitempty"`
	CommandHash string `json:"command_hash,omitempty"`
	WorldTime   string `json:"world_time,omitempty"`
	// EventID and EventVersion bind the participation to the authored definition
	// it was recorded against, so a definition that changed underneath is
	// detected rather than re-read under new meaning.
	EventID      string `json:"event_id"`
	EventVersion string `json:"event_version"`
	// Action is join or withdraw.
	Action string `json:"action"`
	// Instance is the whole participation after this action.
	Instance publiquevent.Instance `json:"instance"`
	// ClockAfter is the branch's formal instant when this was committed. It is
	// stored so a replay can prove the stage the character joined at was really
	// the stage the clock was in, rather than trusting the stored stage id alone.
	ClockAfter StoryClock `json:"clock_after"`
	// Rewards are what this action really paid, empty for a withdrawal.
	Rewards []publiquevent.Reward `json:"rewards,omitempty"`
}

// The participation actions one record may settle.
const (
	PublicEventActionJoin     = "join"
	PublicEventActionWithdraw = "withdraw"
)

var (
	// ErrPublicEventUnknown means the named event is not authored.
	ErrPublicEventUnknown = errors.New("public event is not registered")
	// ErrPublicEventNotHere means the character has not been told about the
	// event, because they are not in the place it is happening.
	ErrPublicEventNotHere = errors.New("the character is not where this event is known")
	// ErrPublicEventClosed means the event has already settled.
	ErrPublicEventClosed = errors.New("public event has already settled")
	// ErrPublicEventNotJoined means a withdrawal names an event the character
	// never took part in.
	ErrPublicEventNotJoined = errors.New("the character has not taken part in this event")
	// ErrPublicEventAlreadyJoined means a second participation was requested.
	ErrPublicEventAlreadyJoined = errors.New("the character has already taken part in this event")
	// ErrPublicEventUnknownAction means the request named an action this module
	// does not settle.
	ErrPublicEventUnknownAction = errors.New("public event action is not supported")
	// ErrPublicEventCommandConflict means a command ID was reused for different
	// content.
	ErrPublicEventCommandConflict = errors.New("public event command identity conflict")
)

// PublicEventRequest is one player request to take part in, or step back from, a
// public event.
type PublicEventRequest struct {
	BranchID  string
	TurnID    string
	WorldTime string
	// EventID names the authored event.
	EventID string
	// Action is join or withdraw.
	Action string
	// ParticipationAction is how the character takes part when joining:
	// observe, help, investigate or withdraw. It is separate from Action so a
	// request cannot confuse "record my choice to stand aside" with "step back
	// from a participation I already made".
	ParticipationAction string
	CommandID           string
	CommandHash         string
}

// PublicEventResult is what a committed participation action reports back.
type PublicEventResult struct {
	EventID  string
	Action   string
	Instance publiquevent.Instance
	// Rewards is what this action really paid, empty for a withdrawal.
	Rewards []publiquevent.Reward
	// SettlementOps are the Actor State writes this commit produced.
	SettlementOps []ActorStateOp
	Replayed      bool
}

// ApplyPublicEvent settles one participation action and commits it as a side
// record plus its payment in a single journal transaction.
func (s *Store) ApplyPublicEvent(storyID string, req PublicEventRequest) (PublicEventResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return PublicEventResult{}, err
	}
	defer releaseStory()
	meta, records, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return PublicEventResult{}, err
	}
	return s.applyPublicEventLocked(storyID, meta, records, req)
}

// publicEventPlannedAction is one participation decision the rules have already
// accepted, together with the clock it was judged against.
type publicEventPlannedAction struct {
	Definition publiquevent.Definition
	Action     string
	Instance   publiquevent.Instance
	Rewards    []publiquevent.Reward
	Ops        []ActorStateOp
	Clock      StoryClock
}

// planPublicEventAction decides one join or withdraw against a branch projection.
//
// ApplyPublicEvent calls it, and so does a main-story turn: §6 makes taking part
// a fact about where the character stands and what the formal clock says, so a
// scene that says the player joined the spring fair must record the same
// participation, judged at the same minute, that the entry would.
//
// scene is where the character really stands when the decision is made. During a
// turn that is the committed place the turn began on, not the state the turn's
// own prose wrote: a narrator who describes the character at the fair has not
// put them there, and taking part is only theirs to settle if they were already
// in the event's region. Rewards are still computed from projection.State, so a
// participation that pays reads the balance this turn really left.
func planPublicEventAction(projection *storyBranchProjection, scene map[string]any, meta StoryMeta, system StoryDirectorActorStateSystem, req PublicEventRequest) (publicEventPlannedAction, error) {
	eventID := strings.TrimSpace(req.EventID)
	definition, found := worldEventDefinition(projection.worldContent(), eventID)
	if !found {
		return publicEventPlannedAction{}, fmt.Errorf("%w: %s", ErrPublicEventUnknown, eventID)
	}
	// The character can only take part in an event they have been told about, and
	// §6 makes that a fact about where they stand rather than a guess about what
	// a scene said. An event whose region they are not in, or that belongs to
	// another world pack revision, is not theirs to join.
	if !publicEventDefinitionEligible(meta, definition, scene) {
		return publicEventPlannedAction{}, fmt.Errorf("%w: %s happens in %s", ErrPublicEventNotHere, eventID, definition.Region)
	}
	clock, err := branchStoryClock(projection, DefaultStoryContextActorID, system)
	if err != nil {
		return publicEventPlannedAction{}, err
	}

	action := strings.TrimSpace(req.Action)
	prior, exists := publicEventPriorInstance(projection, definition.ID)
	instance := prior
	var rewards []publiquevent.Reward
	switch action {
	case PublicEventActionJoin:
		if definition.Closed(clock.Seconds) {
			return publicEventPlannedAction{}, fmt.Errorf("%w: %s", ErrPublicEventClosed, eventID)
		}
		participation := publiquevent.ActionKind(strings.TrimSpace(req.ParticipationAction))
		if participation == "" {
			participation = publiquevent.ActionObserve
		}
		joined, joinErr := publiquevent.Join(definition, priorPtr(prior, exists), clock.Seconds, participation)
		if joinErr != nil {
			switch {
			case errors.Is(joinErr, publiquevent.ErrAlreadyJoined):
				return publicEventPlannedAction{}, fmt.Errorf("%w: %s", ErrPublicEventAlreadyJoined, eventID)
			case errors.Is(joinErr, publiquevent.ErrEventClosed):
				return publicEventPlannedAction{}, fmt.Errorf("%w: %s", ErrPublicEventClosed, eventID)
			case errors.Is(joinErr, publiquevent.ErrUnknownAction):
				return publicEventPlannedAction{}, fmt.Errorf("%w: %q", ErrPublicEventUnknownAction, participation)
			default:
				return publicEventPlannedAction{}, joinErr
			}
		}
		instance = joined
		if chosen, ok := definition.ActionByKind(participation); ok && chosen.Reward != nil {
			rewards = append(rewards, *chosen.Reward)
		}
	case PublicEventActionWithdraw:
		if !exists {
			return publicEventPlannedAction{}, fmt.Errorf("%w: %s", ErrPublicEventNotJoined, eventID)
		}
		withdrawn, withdrawErr := publiquevent.Withdraw(prior)
		if withdrawErr != nil {
			if errors.Is(withdrawErr, publiquevent.ErrAlreadyJoined) {
				return publicEventPlannedAction{}, fmt.Errorf("%w: %s already withdrew", ErrPublicEventAlreadyJoined, eventID)
			}
			return publicEventPlannedAction{}, withdrawErr
		}
		instance = withdrawn
	default:
		return publicEventPlannedAction{}, fmt.Errorf("%w: %q", ErrPublicEventUnknownAction, action)
	}

	if diagnostics := publiquevent.ValidateInstance(definition, instance); len(diagnostics) > 0 {
		return publicEventPlannedAction{}, fmt.Errorf("%w: %s", publiquevent.ErrInvalidState, diagnostics[0].Message)
	}
	ops, err := publicEventSettlementOps(projection, rewards)
	if err != nil {
		return publicEventPlannedAction{}, err
	}
	for _, op := range ops {
		if err := validateActorStateOp(op); err != nil {
			return publicEventPlannedAction{}, err
		}
	}
	return publicEventPlannedAction{
		Definition: definition, Action: action, Instance: instance,
		Rewards: rewards, Ops: ops, Clock: clock,
	}, nil
}

// publicEventSettlementEvents turns a planned participation into the events a
// commit writes.
func publicEventRecordEvent(branchID, parentID, now string, req PublicEventRequest, plan publicEventPlannedAction, recordID string) PublicEventRecordEvent {
	return PublicEventRecordEvent{
		V: schemaVersion, Type: StoryEventTypePublicEventRecord, ID: recordID,
		ParentID: parentID, BranchID: branchID, Ts: now,
		TurnID: strings.TrimSpace(req.TurnID), Cursor: recordID,
		CommandID: strings.TrimSpace(req.CommandID), CommandHash: strings.TrimSpace(req.CommandHash),
		WorldTime:    strings.TrimSpace(req.WorldTime),
		EventID:      plan.Definition.ID,
		EventVersion: plan.Definition.Version,
		Action:       plan.Action,
		Instance:     plan.Instance,
		ClockAfter:   plan.Clock,
		Rewards:      plan.Rewards,
	}
}

// publicEventSettlementEvents is the record plus its reward write, for callers
// that commit the participation entry on its own.
func publicEventSettlementEvents(branchID, parentID, now string, req PublicEventRequest, plan publicEventPlannedAction, recordID string) ([]any, string) {
	record := publicEventRecordEvent(branchID, parentID, now, req, plan, recordID)
	headID := recordID
	newEvents := []any{record}
	if len(plan.Ops) > 0 {
		deltaID := newID("sd")
		newEvents = append(newEvents, newStateDeltaEventWithActorOps(deltaID, recordID, branchID, now, nil, plan.Ops))
		headID = deltaID
	}
	return newEvents, headID
}

// applyPublicEventLocked is the commit itself.
func (s *Store) applyPublicEventLocked(storyID string, meta StoryMeta, records []StoryEventRecord, req PublicEventRequest) (PublicEventResult, error) {
	commandID := strings.TrimSpace(req.CommandID)
	commandHash := strings.TrimSpace(req.CommandHash)
	if commandID != "" && commandHash == "" {
		return PublicEventResult{}, fmt.Errorf("%w: command_id=%q requires a command hash", ErrPublicEventCommandConflict, commandID)
	}
	branchID := strings.TrimSpace(req.BranchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return PublicEventResult{}, fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}
	if commandID != "" {
		committed, found, err := publicEventRecordForCommand(records, branchID, commandID, commandHash)
		if err != nil {
			return PublicEventResult{}, err
		}
		if found {
			committed.Replayed = true
			return committed, nil
		}
	}
	if branchIsTerminal(records, branch.Head) {
		return PublicEventResult{}, branchTerminalError()
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return PublicEventResult{}, err
	}
	plan, err := planPublicEventAction(projection, projection.State, meta,
		actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState), req)
	if err != nil {
		return PublicEventResult{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	recordID := newID("evtrec")
	newEvents, headID := publicEventSettlementEvents(branchID, branch.Head, now, req, plan, recordID)

	branch.Head = headID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, newEvents...); err != nil {
		return PublicEventResult{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return PublicEventResult{}, err
	}
	return PublicEventResult{
		EventID: plan.Definition.ID, Action: plan.Action, Instance: plan.Instance,
		Rewards: plan.Rewards, SettlementOps: plan.Ops,
	}, nil
}

// priorPtr turns an optional instance into the pointer Join expects.
func priorPtr(instance publiquevent.Instance, exists bool) *publiquevent.Instance {
	if !exists {
		return nil
	}
	return &instance
}

// publicEventPriorInstance finds the participation a request acts on.
func publicEventPriorInstance(projection *storyBranchProjection, eventID string) (publiquevent.Instance, bool) {
	if projection == nil {
		return publiquevent.Instance{}, false
	}
	instance, ok := projection.PublicEvents[eventID]
	return instance, ok
}

// publicEventSettlementOps writes what taking part really paid.
func publicEventSettlementOps(projection *storyBranchProjection, rewards []publiquevent.Reward) ([]ActorStateOp, error) {
	if len(rewards) == 0 {
		return nil, nil
	}
	var state map[string]any
	if projection != nil {
		state = projection.State
	}
	var ops []ActorStateOp
	var currency int64
	changed := false
	for _, reward := range rewards {
		switch reward.Kind {
		case publiquevent.RewardCurrency:
			currency += reward.Quantity
			changed = true
		default:
			return nil, fmt.Errorf("%w: reward kind %q", publiquevent.ErrInvalidState, reward.Kind)
		}
	}
	if changed {
		balance := int64(xiuxianStoredInt(state, DefaultActorID, xiuxianFieldCurrency)) + currency
		ops = append(ops, ActorStateOp{
			Op:         "set",
			ActorID:    DefaultActorID,
			FieldID:    normalizeActorStateFieldName(xiuxianFieldCurrency),
			Value:      float64(balance),
			Reason:     "由已提交的公共事件参与结算写入余额，不由叙述改写。",
			SourceKind: StateOpSourcePublicEvent,
			SourceID:   "public_event.reward",
		})
	}
	return ops, nil
}

// publicEventRecordForCommand finds the action a previous attempt committed.
func publicEventRecordForCommand(records []StoryEventRecord, branchID, commandID, commandHash string) (PublicEventResult, bool, error) {
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypePublicEventRecord {
			continue
		}
		var event PublicEventRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return PublicEventResult{}, false, fmt.Errorf("decode committed public event record: %w", err)
		}
		if event.BranchID != branchID || strings.TrimSpace(event.CommandID) != commandID {
			continue
		}
		if strings.TrimSpace(event.CommandHash) != commandHash {
			return PublicEventResult{}, false, fmt.Errorf("%w: command_id=%q existing_hash=%q requested_hash=%q",
				ErrPublicEventCommandConflict, commandID, event.CommandHash, commandHash)
		}
		return PublicEventResult{
			EventID: event.EventID, Action: event.Action,
			Instance: event.Instance, Rewards: event.Rewards, Replayed: true,
		}, true, nil
	}
	return PublicEventResult{}, false, nil
}

// applyPublicEventRecord merges a side record into the branch projection.
//
// The stored stage is not trusted on its own: it is checked against the clock
// the record committed, so a corrupt row cannot hand the branch a participation
// at a stage that was not current. This is the same guard a journey applies to
// its claimed distance.
func applyPublicEventRecord(branch *storyBranchProjection, event PublicEventRecordEvent) error {
	definition, ok := worldEventDefinition(branch.worldContent(), event.EventID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrPublicEventUnknown, event.EventID)
	}
	if definition.Version != event.EventVersion {
		return fmt.Errorf("%w: %s version %s", ErrPublicEventUnknown, event.EventID, event.EventVersion)
	}
	if err := event.ClockAfter.Validate(); err != nil {
		return err
	}
	if event.ClockAfter.Seconds < branch.clockSeconds() {
		return fmt.Errorf("%w: public event record moves the story clock backwards", storytime.ErrTimeReversal)
	}
	if diagnostics := publiquevent.ValidateInstance(definition, event.Instance); len(diagnostics) > 0 {
		return fmt.Errorf("%w: %s", publiquevent.ErrInvalidState, diagnostics[0].Message)
	}
	// The stage recorded must be the stage the committed clock was really in.
	stage, ok := definition.CurrentStage(event.ClockAfter.Seconds)
	if !ok || stage.ID != event.Instance.JoinedStage {
		return fmt.Errorf("公共事件记录与时钟不一致 / public event record disagrees with the clock")
	}
	if branch.PublicEvents == nil {
		branch.PublicEvents = map[string]publiquevent.Instance{}
	}
	branch.PublicEvents[event.EventID] = event.Instance
	clock := event.ClockAfter
	branch.Clock = &clock
	return nil
}

// validatePublicEventRecordEvent rejects a record that could not have been
// honestly attributed.
func validatePublicEventRecordEvent(event PublicEventRecordEvent) error {
	if strings.TrimSpace(event.BranchID) == "" {
		return fmt.Errorf("公共事件记录缺少 branch_id / a public event record requires a branch")
	}
	if strings.TrimSpace(event.EventID) == "" {
		return fmt.Errorf("公共事件记录缺少 event_id / a public event record requires an event")
	}
	switch event.Action {
	case PublicEventActionJoin, PublicEventActionWithdraw:
	default:
		return fmt.Errorf("公共事件记录的动作未知 / unknown public event action %q", event.Action)
	}
	return nil
}

// clonePublicEventInstances copies the participation map so one branch's record
// cannot be mutated through an alias into another.
func clonePublicEventInstances(events map[string]publiquevent.Instance) map[string]publiquevent.Instance {
	if len(events) == 0 {
		return nil
	}
	cloned := make(map[string]publiquevent.Instance, len(events))
	for id, instance := range events {
		cloned[id] = instance
	}
	return cloned
}

// PublicEventActionView is one way to take part as the entry offers it.
type PublicEventActionView struct {
	Kind string `json:"kind"`
	// Label and Brief are the authored wording, so the panel never invents a
	// button caption or a consequence.
	Label string `json:"label"`
	Brief string `json:"brief"`
	// Reward is what this choice really pays, empty when it pays nothing.
	Reward *PublicEventRewardView `json:"reward,omitempty"`
}

// PublicEventRewardView is one payment in the story's own wording.
type PublicEventRewardView struct {
	Kind     string `json:"kind"`
	RefID    string `json:"ref_id"`
	Quantity int64  `json:"quantity"`
}

// PublicEventStageView is one stage and what the character may be told about it.
type PublicEventStageView struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	// Lines are what this character may be shown: the public summary always, and
	// the participant lines only for the stage they were present at. The secret
	// lines are never included because the rules package never hands them out.
	Lines []string `json:"lines"`
	// OpensAtSeconds is the branch clock instant this stage becomes current, so
	// the entry can say when the next thing happens rather than leaving the
	// player to guess.
	OpensAtSeconds int64 `json:"opens_at_seconds"`
}

// PublicEventParticipantView is one party the region knows about.
type PublicEventParticipantView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// PublicEventView is one public event as the entry reads it.
type PublicEventView struct {
	EventID      string `json:"event_id"`
	EventVersion string `json:"event_version"`
	Title        string `json:"title"`
	Brief        string `json:"brief"`
	Region       string `json:"region"`
	// CurrentStageID is empty before the event opens, which is how the entry
	// knows to say "not yet" rather than showing a stage that has not begun.
	CurrentStageID    string                       `json:"current_stage_id,omitempty"`
	CurrentStageTitle string                       `json:"current_stage_title,omitempty"`
	Closed            bool                         `json:"closed"`
	Stages            []PublicEventStageView       `json:"stages"`
	Participants      []PublicEventParticipantView `json:"participants"`
	Actions           []PublicEventActionView      `json:"actions"`
	// Joined reports that this character took part, and Participation is how.
	Joined        bool   `json:"joined"`
	Participation string `json:"participation,omitempty"`
	Withdrawn     bool   `json:"withdrawn,omitempty"`
	// JoinedAtSeconds and JoinedStage are when and where the character took part.
	JoinedAtSeconds int64  `json:"joined_at_seconds,omitempty"`
	JoinedStage     string `json:"joined_stage,omitempty"`
}

// PublicEventState is the entry's read: the public events this character knows
// about, plus the branch's formal instant.
type PublicEventState struct {
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	// Location is the scene's committed location text, which is what decides
	// which events the character has been told about.
	Location string `json:"location,omitempty"`
	// ClockSeconds and ClockText are the branch's formal instant and its display
	// wording, so the entry can show real world time rather than a counter.
	ClockSeconds int64             `json:"clock_seconds"`
	ClockText    string            `json:"clock_text,omitempty"`
	Events       []PublicEventView `json:"events"`
}

// PublicEventStateFor reads the events this branch knows about without writing
// anything.
//
// The stage of each event is derived from the committed clock, so opening the
// entry costs no world time and cannot advance anything.
func (s *Store) PublicEventStateFor(storyID, branchID string) (PublicEventState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return PublicEventState{}, err
	}
	defer release()
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return PublicEventState{}, err
	}
	if strings.TrimSpace(branchID) == "" {
		branchID = handle.projection.Meta.CurrentBranch
	}
	meta := normalizeStoryMeta(handle.projection.Meta)
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return PublicEventState{}, err
	}
	clock, err := branchStoryClock(projection, DefaultStoryContextActorID,
		actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState))
	if err != nil {
		return PublicEventState{}, err
	}
	state := publicEventStateFrom(meta, projection, clock)
	state.StoryID = storyID
	state.BranchID = branchID
	return state, nil
}

// publicEventStateFrom builds the event view from a projection the caller has
// already read.
//
// It is split out so the main story's World Actions page can report the same
// events from one journal read, with the same eligibility answer the entry and
// the commit use. §6 shows only what this character is placed to hear about, and
// a second copy of that filter would be a second answer.
func publicEventStateFrom(meta StoryMeta, projection *storyBranchProjection, clock StoryClock) PublicEventState {
	state := PublicEventState{
		Location:     storySceneText(projection.State, storySceneLocationField),
		ClockSeconds: clock.Seconds,
		ClockText:    storyClockText(clock),
		Events:       []PublicEventView{},
	}
	for _, definition := range worldEventDefinitions(worldContentForMeta(meta)) {
		// §6: an event the character is not placed to hear about is not shown,
		// even though it is really happening. That is the difference between the
		// world and this character's knowledge of it. The commit asks the same
		// question, so nothing shown here is refused when acted on.
		if !publicEventDefinitionEligible(meta, definition, projection.State) {
			continue
		}
		instance, joined := projection.PublicEvents[definition.ID]
		state.Events = append(state.Events, publicEventView(definition, instance, joined, clock.Seconds))
	}
	sort.Slice(state.Events, func(i, j int) bool { return state.Events[i].EventID < state.Events[j].EventID })
	return state
}

// publicEventView renders one event from its authored wording, its derived stage
// and this character's own participation.
func publicEventView(definition publiquevent.Definition, instance publiquevent.Instance, joined bool, seconds int64) PublicEventView {
	current, open := definition.CurrentStage(seconds)
	statuses := definition.StageStatuses(seconds)
	view := PublicEventView{
		EventID: definition.ID, EventVersion: definition.Version,
		Title: definition.Title, Brief: definition.Brief, Region: definition.Region,
		Closed: definition.Closed(seconds),
		Stages: make([]PublicEventStageView, 0, len(definition.Stages)),
		// The participant list is what the region knows, so it is public. The
		// secret motives are not part of this view and the rules package never
		// returns them.
		Participants: make([]PublicEventParticipantView, 0, len(definition.Participants)),
		Actions:      make([]PublicEventActionView, 0, len(definition.Actions)),
	}
	if open {
		view.CurrentStageID = current.ID
		view.CurrentStageTitle = current.Title
	}
	// The lines a character may be shown at a stage: the public summary always,
	// and the participant lines only for the stage they were actually present at.
	// That is what keeps a character who joined during the rush from learning the
	// surge's participant lines they were not there for.
	for index, stage := range definition.Stages {
		lines := []string{stage.Summary}
		if joined && instance.JoinedStage == stage.ID {
			lines = publiquevent.JoinedStageLines(definition, instance)
		} else if definition.ID == publiquevent.PublicEventIDTideGauge && statuses[index] != publiquevent.StageCurrent {
			lines = []string{}
		}
		view.Stages = append(view.Stages, PublicEventStageView{
			ID: stage.ID, Title: stage.Title,
			Status:         string(statuses[index]),
			Lines:          lines,
			OpensAtSeconds: stage.OpenAfterSeconds,
		})
	}
	for _, participant := range definition.Participants {
		view.Participants = append(view.Participants, PublicEventParticipantView{
			ID: participant.ID, Name: participant.Name, Role: participant.PublicRole,
		})
	}
	for _, action := range definition.Actions {
		if !action.AvailableIn(current.ID) {
			continue
		}
		actionView := PublicEventActionView{Kind: string(action.Kind), Label: action.Label, Brief: action.Brief}
		if action.Reward != nil {
			actionView.Reward = &PublicEventRewardView{
				Kind: string(action.Reward.Kind), RefID: action.Reward.RefID, Quantity: action.Reward.Quantity,
			}
		}
		view.Actions = append(view.Actions, actionView)
	}
	if joined {
		view.Joined = true
		view.Participation = string(instance.Action)
		view.Withdrawn = instance.Withdrawn
		view.JoinedAtSeconds = instance.JoinedAtSeconds
		view.JoinedStage = instance.JoinedStage
	}
	return view
}

// storyClockText renders a formal instant in the story's own wording.
func storyClockText(clock StoryClock) string {
	text := strings.TrimSpace(clock.Anchor)
	elapsed := DescribeElapsed(clock.Seconds)
	if elapsed == "" {
		return text
	}
	if text == "" {
		return elapsed
	}
	return text + " 后 " + elapsed
}
