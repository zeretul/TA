package interactive

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"ta/internal/cultivation"
	"ta/internal/exploration"
)

// StateOpSourceRealm marks an Actor State operation a committed realm action
// produced.
//
// It is its own source kind for the same reason a journey's is: a reader of the
// journal can tell an item the rules handed over from one the prose described.
const StateOpSourceRealm = "realm"

// StoryEventTypeRealmRecord is a side record on the canonical story journal.
//
// It carries one real realm action — entering, moving, taking a reward or
// leaving — together with the whole instance after it. It creates no Turn and
// consumes no turn number, for the same reason a journey record does not.
//
// Three properties are deliberate.
//
//   - The instance snapshot is the whole truth. Every action writes the complete
//     instance after it rather than a patch, so a replay that reaches this record
//     produces exactly the state that was live, and a record that disagrees with
//     the rules is refused rather than trusted.
//
//   - Rewards are part of the snapshot. §11.4 of 自制-探索与行路 keeps a re-entered
//     realm's history rather than resetting its drops, so a reward already taken
//     has to be a fact a later read can see. The snapshot carries the granted
//     list and the item write is committed with it in one transaction.
//
//   - Entering and leaving are ordinary records, not special events. A realm the
//     player walked out of is still the same realm when they walk back in.
type RealmRecordEvent struct {
	V        int    `json:"v"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	BranchID string `json:"branch_id"`
	Ts       string `json:"ts"`
	// TurnID is the main-story turn this action belongs to. It is empty when the
	// action was settled from the realm panel rather than inside a turn.
	TurnID string `json:"turn_id,omitempty"`
	// Cursor is the canonical commit position this record was projected at.
	Cursor string `json:"cursor,omitempty"`
	// CommandID is the caller-owned idempotency key and CommandHash fingerprints
	// the content behind it.
	CommandID   string `json:"command_id,omitempty"`
	CommandHash string `json:"command_hash,omitempty"`
	// WorldTime is the story's own display text for this moment. It is a label;
	// nothing here calculates from it.
	WorldTime string `json:"world_time,omitempty"`
	// RealmID names the authored definition the instance was created from, and
	// RealmVersion the version it was bound to, so a definition that changed
	// underneath an instance is detected rather than silently re-read.
	RealmID      string `json:"realm_id"`
	RealmVersion string `json:"realm_version"`
	// Action is the operation this record settled, one of the realm action
	// constants below.
	Action string `json:"action"`
	// ContentRefID is the declared reward this action took, empty for any other
	// action. It is stored so a replay can report the same reward rather than a
	// grant with no identity.
	ContentRefID string `json:"content_ref_id,omitempty"`
	// Instance is the whole realm instance after this action.
	Instance exploration.DungeonInstanceSnapshot `json:"instance"`
	// OriginLocation is where the character stood when they entered, captured on
	// the entering record. Leaving writes it back, so a realm that has no road
	// label of its own never invents a place to return to.
	OriginLocation string                       `json:"origin_location,omitempty"`
	ApproachID     string                       `json:"approach_id,omitempty"`
	Rewards        []exploration.InstanceReward `json:"rewards,omitempty"`
	ClockBefore    *StoryClock                  `json:"clock_before,omitempty"`
	ClockAfter     *StoryClock                  `json:"clock_after,omitempty"`
}

// The realm actions one record may settle.
const (
	RealmActionEnter    = "enter"
	RealmActionMove     = "move"
	RealmActionReward   = "reward"
	RealmActionExit     = "exit"
	RealmActionReenter  = "reenter"
	RealmActionSurvey   = "survey"
	RealmActionPrepare  = "prepare"
	RealmActionApproach = "approach"
	RealmActionResolve  = "resolve"
)

// ErrRealmUnknownRealm means the named realm is not authored.
var ErrRealmUnknownRealm = errors.New("realm is not registered")

// ErrRealmNotPresent means the story is not standing in the realm the request
// names, so the action would act on a place the character is not in.
var ErrRealmNotPresent = errors.New("character is not in this realm")

// ErrRealmNotAtEntrance refuses an entry the scene does not stand at.
var ErrRealmNotAtEntrance = errors.New("character is not at the realm's entrance")

// ErrRealmCommandConflict means a command ID was reused for different content.
var ErrRealmCommandConflict = errors.New("realm command identity conflict")

// ErrRealmUnknownAction means a request named an action this module does not
// settle. Guessing one would move the character through a place they did not
// choose.
var ErrRealmUnknownAction = errors.New("realm action is not supported")

// RealmInstanceState is one branch's realm instance plus the place the
// character left from.
//
// The origin is kept beside the snapshot rather than inside it because it is not
// a fact about the realm: it is a fact about the story that walked in, and the
// rules layer has no business knowing where the road outside goes.
type RealmInstanceState struct {
	Snapshot       exploration.DungeonInstanceSnapshot `json:"snapshot"`
	OriginLocation string                              `json:"origin_location,omitempty"`
	ExploreSources map[string]string                   `json:"explore_sources,omitempty"`
}

// RealmRequest is one player request to act inside a realm.
type RealmRequest struct {
	BranchID  string
	TurnID    string
	WorldTime string
	// RealmID names the authored realm. Required to enter.
	RealmID string
	// InstanceID names the instance a later action acts on. Empty uses the
	// branch's single instance, which is what "继续探索" means without repeating
	// it.
	InstanceID string
	// Action is the operation to settle.
	Action string
	// ConnectionID and DestinationAreaID move the character through one declared
	// directed connection.
	ConnectionID      string
	DestinationAreaID string
	// ContentRefID names the declared reward to take.
	ApproachID   string
	ContentRefID string
	// CommandID and CommandHash identify one attempt.
	CommandID   string
	CommandHash string
}

// RealmResult is what a committed realm action reports back.
type RealmResult struct {
	EventID    string
	InstanceID string
	RealmID    string
	Action     string
	// Instance is the whole realm after this action.
	Instance exploration.DungeonInstanceSnapshot
	// View is the player-known projection, so the panel never rebuilds it.
	View exploration.DungeonInstanceView
	// Reward is the reward this action granted, if any.
	Reward          *exploration.InstanceReward
	Rewards         []exploration.InstanceReward
	DurationSeconds int64
	// SettlementOps are the Actor State writes this commit produced.
	SettlementOps []ActorStateOp
	// Replayed reports that this answer came from an action already committed
	// under the same command, so nothing was applied twice.
	Replayed bool
}

// ApplyRealm settles one realm action and commits it as a side record plus its
// item write in a single journal transaction.
func (s *Store) ApplyRealm(storyID string, req RealmRequest) (RealmResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return RealmResult{}, err
	}
	defer releaseStory()

	meta, records, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return RealmResult{}, err
	}
	return s.applyRealmLocked(storyID, meta, records, req)
}

// realmPlannedAction is one realm action the rules have already accepted,
// together with the view the player may be shown and any reward really taken.
type realmPlannedAction struct {
	Definition     exploration.DungeonDefinition
	Action         string
	Instance       exploration.DungeonInstanceSnapshot
	View           exploration.DungeonInstanceView
	OriginLocation string
	Reward         *exploration.InstanceReward
	Ops            []ActorStateOp
	Rewards        []exploration.InstanceReward
	ClockBefore    *StoryClock
	ClockAfter     *StoryClock
}

// planRealmAction settles one enter, move, reward or exit against a branch
// projection without writing anything.
//
// ApplyRealm calls it, and so does a main-story turn: §11.2 makes the entrance a
// real condition rather than a scene's assertion, so a player who writes 进入秘境
// must be judged at the same place, against the same eligibility and the same
// instance, that the entry would use.
// planRealmAction decides one realm action.
//
// scene is the state the action is judged against — where the character really
// stands when the decision is made. It is separate from projection.State because
// the two answer different questions and, during a turn, are different moments:
//
//   - Where does this turn find the character? That is a precondition, and it is
//     read from the committed state the turn began on. A narrator that describes
//     the character already inside the cave has not moved them there; the turn's
//     own prose cannot be the evidence for its own permission.
//   - What does the character carry? That is arithmetic, and it is read from
//     projection.State, which during a turn is the state that turn's own writes
//     produced.
//
// Callers that commit the action on its own pass projection.State for both.
func planRealmAction(projection *storyBranchProjection, scene map[string]any, meta StoryMeta, branchID string, req RealmRequest) (realmPlannedAction, error) {
	action := strings.TrimSpace(req.Action)
	if err := realmActionBlocked(projection, scene, meta); err != nil {
		return realmPlannedAction{}, err
	}
	prior, exists := realmPriorInstance(projection, req)
	if strings.TrimSpace(req.InstanceID) != "" && !exists {
		return realmPlannedAction{}, ErrRealmNotPresent
	}

	var definition exploration.DungeonDefinition
	var err error
	switch action {
	case RealmActionEnter, RealmActionReenter:
		definition, err = realmResolveDefinition(req, prior, exists, projection.worldContent())
	case RealmActionMove, RealmActionReward, RealmActionExit, RealmActionSurvey, RealmActionPrepare, RealmActionApproach, RealmActionResolve:
		if !exists {
			return realmPlannedAction{}, fmt.Errorf("%w: no realm instance to act on", ErrRealmNotPresent)
		}
		definition, err = realmDefinitionForInstance(prior.Snapshot, projection.worldContent())
	default:
		return realmPlannedAction{}, fmt.Errorf("%w: %q", ErrRealmUnknownAction, action)
	}
	if err != nil {
		return realmPlannedAction{}, err
	}

	if action == RealmActionEnter || action == RealmActionReenter {
		for _, existing := range projection.Realms {
			if existing.Snapshot.Presence == exploration.InstanceInside && (!exists || existing.Snapshot.Binding.InstanceID != prior.Snapshot.Binding.InstanceID) {
				return realmPlannedAction{}, fmt.Errorf("%w: leave the current place first", ErrRealmNotPresent)
			}
		}
	}
	if action == RealmActionEnter && exists {
		// §11.4 keeps one realm rather than generating a fresh copy each time the
		// player comes back, so a second entry continues the same instance.
		if prior.Snapshot.Presence == exploration.InstanceInside {
			return realmPlannedAction{}, fmt.Errorf("%w: the character is already inside", ErrRealmNotPresent)
		}
		action = RealmActionReenter
	}

	scope := exploration.DungeonScope{WorldID: realmWorldID(meta, branchID), BranchID: branchID}
	origin := prior.OriginLocation
	var transition exploration.InstanceTransition
	var reward *exploration.InstanceReward
	var explorePlan *realmPlannedAction

	switch action {
	case RealmActionEnter:
		origin = storySceneText(scene, storySceneLocationField)
		// §11.2 asks each realm to declare its entry conditions, and the review
		// found the entrance open from anywhere. Both halves of the answer are
		// the same predicate the entry uses, so a realm the panel does not offer
		// cannot be entered by naming it.
		if !realmDefinitionEligible(meta, definition, scene) {
			return realmPlannedAction{}, fmt.Errorf("%w: %s cannot be entered from %q", ErrRealmNotAtEntrance, definition.ID, origin)
		}
		instanceID := newID("rlm")
		permit, err := realmCreatePermit(definition, scope, instanceID, "realm.enter")
		if err != nil {
			return realmPlannedAction{}, err
		}
		transition, err = exploration.CreateDungeonInstance(exploration.CreateDungeonInstanceInput{
			Definition: definition, Scope: scope, InstanceID: instanceID,
			EntryAreaID: definition.EntryAreaIDs[0],
			Permit:      permit, Battle: exploration.BattleNone, Blocking: exploration.BlockingNone,
			InitialDiscoveries: realmOpeningDiscoveries(definition),
		})
		if err != nil {
			return realmPlannedAction{}, err
		}
	case RealmActionReenter:
		if !realmDefinitionEligible(meta, definition, scene) {
			return realmPlannedAction{}, ErrRealmNotAtEntrance
		}
		if prior.Snapshot.Presence != exploration.InstanceOutside {
			return realmPlannedAction{}, fmt.Errorf("%w: re-entry requires being outside", ErrRealmNotPresent)
		}
		entry := definition.EntryAreaIDs[0]
		ctx, err := realmActionContext(prior.Snapshot, scope, exploration.ActionReenter, entry, "", "", "realm.reenter")
		if err != nil {
			return realmPlannedAction{}, err
		}
		transition, err = exploration.ReenterDungeonInstance(definition, scope, prior.Snapshot, ctx, entry)
		if err != nil {
			return realmPlannedAction{}, err
		}
	case RealmActionMove:
		if err := realmRequireInside(prior.Snapshot); err != nil {
			return realmPlannedAction{}, err
		}
		connectionID := strings.TrimSpace(req.ConnectionID)
		destination := strings.TrimSpace(req.DestinationAreaID)
		if connectionID == "" {
			// A player says where they go, not which door id it is. When the
			// destination alone is named, the door is the visible connection from
			// the room they stand in; an ambiguous or absent one is refused rather
			// than guessed.
			connectionID = realmConnectionTo(definition, prior.Snapshot.CurrentAreaID, destination)
			if connectionID == "" {
				return realmPlannedAction{}, fmt.Errorf("%w: no visible connection leads from %q to %q", exploration.ErrInvalidInstanceAction, prior.Snapshot.CurrentAreaID, destination)
			}
		}
		ctx, err := realmActionContext(prior.Snapshot, scope, exploration.ActionMove,
			connectionID, prior.Snapshot.CurrentAreaID, destination, "realm.move")
		if err != nil {
			return realmPlannedAction{}, err
		}
		transition, err = exploration.MoveDungeonInstance(definition, scope, prior.Snapshot, ctx,
			exploration.MoveDungeonInstanceInput{ConnectionID: connectionID, DestinationAreaID: destination})
		if err != nil {
			return realmPlannedAction{}, err
		}
	case RealmActionReward:
		if err := realmRequireInside(prior.Snapshot); err != nil {
			return realmPlannedAction{}, err
		}
		contentRefID := strings.TrimSpace(req.ContentRefID)
		ctx, err := realmActionContext(prior.Snapshot, scope, exploration.ActionDiscover,
			contentRefID, "", "", "realm.reward")
		if err != nil {
			return realmPlannedAction{}, err
		}
		var granted exploration.InstanceReward
		transition, granted, err = exploration.GrantInstanceReward(definition, scope, prior.Snapshot, ctx,
			exploration.RealmRewardRequest{ContentRefID: contentRefID, SourceRef: "realm.reward"})
		if err != nil {
			return realmPlannedAction{}, err
		}
		reward = &granted
	case RealmActionSurvey, RealmActionPrepare, RealmActionApproach, RealmActionResolve:
		planned, err := planRealmExplore(projection, scene, meta, scope, prior, definition, req)
		if err != nil {
			return realmPlannedAction{}, err
		}
		explorePlan = &planned
		transition.Snapshot = planned.Instance
	case RealmActionExit:
		if err := realmRequireInside(prior.Snapshot); err != nil {
			return realmPlannedAction{}, err
		}
		ctx, err := realmActionContext(prior.Snapshot, scope, exploration.ActionExit,
			prior.Snapshot.CurrentAreaID, "", "", "realm.exit")
		if err != nil {
			return realmPlannedAction{}, err
		}
		transition, err = exploration.ExitDungeonInstance(definition, scope, prior.Snapshot, ctx)
		if err != nil {
			return realmPlannedAction{}, err
		}
	}

	view, diagnostics := exploration.ProjectDungeonInstanceView(definition, scope, transition.Snapshot)
	if len(diagnostics) > 0 {
		return realmPlannedAction{}, fmt.Errorf("%w: %s", exploration.ErrInvalidInstance, diagnostics[0].Message)
	}

	ops := realmActorOps(projection, definition, transition.Snapshot, reward, origin)
	if explorePlan != nil {
		ops = explorePlan.Ops
	}
	for _, op := range ops {
		if err := validateActorStateOp(op); err != nil {
			return realmPlannedAction{}, err
		}
	}
	plan := realmPlannedAction{Definition: definition, Action: action, Instance: transition.Snapshot,
		View: view, OriginLocation: origin, Reward: reward, Ops: ops}
	if explorePlan != nil {
		plan.Rewards = explorePlan.Rewards
		plan.ClockBefore = explorePlan.ClockBefore
		plan.ClockAfter = explorePlan.ClockAfter
	}
	return plan, nil
}

// realmSettlementEvents turns a planned realm action into the events a commit
// writes: the record, its item and place writes, and any task objective the
// reward ticks.
func realmRecordEvent(branchID, parentID, now string, req RealmRequest, plan realmPlannedAction, eventID string) RealmRecordEvent {
	return RealmRecordEvent{
		V: schemaVersion, Type: StoryEventTypeRealmRecord, ID: eventID,
		ParentID: parentID, BranchID: branchID, Ts: now,
		TurnID: strings.TrimSpace(req.TurnID), Cursor: eventID,
		CommandID: strings.TrimSpace(req.CommandID), CommandHash: strings.TrimSpace(req.CommandHash),
		WorldTime:      strings.TrimSpace(req.WorldTime),
		RealmID:        plan.Definition.ID,
		RealmVersion:   plan.Definition.Version,
		Action:         plan.Action,
		ContentRefID:   realmRecordedContentRef(plan.Action, req.ContentRefID),
		Instance:       plan.Instance,
		OriginLocation: plan.OriginLocation,
		ApproachID:     strings.TrimSpace(req.ApproachID), Rewards: append([]exploration.InstanceReward(nil), plan.Rewards...),
		ClockBefore: cloneStoryClock(plan.ClockBefore), ClockAfter: cloneStoryClock(plan.ClockAfter),
	}
}

// realmSettlementEvents is the record plus its place and item writes, for
// callers that commit the realm entry on its own.
func realmSettlementEvents(projection *storyBranchProjection, system StoryDirectorActorStateSystem, branchID, parentID, now string, req RealmRequest, plan realmPlannedAction, eventID string) ([]any, string, error) {
	record := realmRecordEvent(branchID, parentID, now, req, plan, eventID)
	headID := eventID
	newEvents := []any{record}
	if used, err := cooperationUseEvent(projection, branchID, headID, now, req.TurnID, record); err != nil {
		return nil, "", err
	} else if used != nil {
		newEvents = append(newEvents, *used)
		headID = used.ID
	}
	if len(plan.Ops) > 0 {
		// The item and place writes are a separate event parented to the record,
		// for the same reason the rest's pool write is: an event whose ID is also
		// its parent closes the branch's parent chain into a cycle.
		deltaID := newID("sd")
		newEvents = append(newEvents, newStateDeltaEventWithActorOps(deltaID, headID, branchID, now, nil, plan.Ops))
		headID = deltaID
	}
	// Taking a declared reward is a fact, and a task objective may be waiting on
	// it. The tick is appended here so the reward and the task land together: a
	// task cannot complete on a reward the branch did not really take.
	if plan.Reward != nil {
		ticks, _, err := questAdvanceEvents(projection, branchID, req.TurnID, questFactFromRealmReward(*plan.Reward), eventID, headID, now, projection.clockSeconds())
		if err != nil {
			return nil, "", err
		}
		for _, tick := range ticks {
			newEvents = append(newEvents, tick)
			if tickRecord, ok := tick.(QuestRecordEvent); ok {
				headID = tickRecord.ID
			}
		}
	}
	for _, reward := range plan.Rewards {
		ticks, _, err := questAdvanceEvents(projection, branchID, req.TurnID, questFactFromRealmReward(reward), eventID, headID, now, plan.ClockAfter.Seconds)
		if err != nil {
			return nil, "", err
		}
		for _, tick := range ticks {
			newEvents = append(newEvents, tick)
			if r, ok := tick.(QuestRecordEvent); ok {
				headID = r.ID
			}
		}
	}
	if plan.ClockBefore != nil && plan.ClockAfter != nil {
		closed, next := storyIntervalEvents(intervalProjectionAfterOps(projection, plan.Ops), system, branchID, headID, now, req.TurnID, "", *plan.ClockBefore, *plan.ClockAfter)
		newEvents = append(newEvents, closed...)
		headID = next
	}
	return newEvents, headID, nil
}

// applyRealmLocked is the commit itself, for callers that already hold the store
// lock and have already read the branch.
func (s *Store) applyRealmLocked(storyID string, meta StoryMeta, records []StoryEventRecord, req RealmRequest) (RealmResult, error) {
	commandID := strings.TrimSpace(req.CommandID)
	commandHash := strings.TrimSpace(req.CommandHash)
	if commandID != "" && commandHash == "" {
		return RealmResult{}, fmt.Errorf("%w: command_id=%q requires a command hash", ErrRealmCommandConflict, commandID)
	}
	branchID := strings.TrimSpace(req.BranchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return RealmResult{}, fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}
	if commandID != "" {
		committed, found, err := realmRecordForCommand(records, branchID, commandID, commandHash, worldContentForMeta(meta))
		if err != nil {
			return RealmResult{}, err
		}
		if found {
			committed.Replayed = true
			return committed, nil
		}
	}
	if branchIsTerminal(records, branch.Head) {
		return RealmResult{}, branchTerminalError()
	}

	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return RealmResult{}, err
	}
	plan, err := planRealmAction(projection, projection.State, meta, branchID, req)
	if err != nil {
		return RealmResult{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	eventID := newID("rlmrec")
	newEvents, headID, err := realmSettlementEvents(projection, actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState), branchID, branch.Head, now, req, plan, eventID)
	if err != nil {
		return RealmResult{}, err
	}
	branch.Head = headID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, newEvents...); err != nil {
		return RealmResult{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return RealmResult{}, err
	}
	return RealmResult{
		EventID: eventID, InstanceID: plan.Instance.Binding.InstanceID,
		RealmID: plan.Definition.ID, Action: plan.Action,
		Instance: plan.Instance, View: plan.View,
		Reward: plan.Reward, Rewards: plan.Rewards, DurationSeconds: realmPlanDuration(plan), SettlementOps: plan.Ops,
	}, nil
}

// realmPriorInstance finds the instance a request acts on.
//
// An explicit instance ID wins. Otherwise the branch's single instance is the
// one meant, because a branch can only be standing in one realm at a time.
func realmPriorInstance(projection *storyBranchProjection, req RealmRequest) (RealmInstanceState, bool) {
	if projection == nil {
		return RealmInstanceState{}, false
	}
	if id := strings.TrimSpace(req.InstanceID); id != "" {
		state, ok := projection.Realms[id]
		return state, ok
	}
	if realmID := strings.TrimSpace(req.RealmID); realmID != "" {
		for _, state := range projection.Realms {
			if state.Snapshot.Binding.DefinitionID == realmID {
				return state, true
			}
		}
		return RealmInstanceState{}, false
	}
	var active RealmInstanceState
	count := 0
	for _, state := range projection.Realms {
		if state.Snapshot.Presence == exploration.InstanceInside {
			active = state
			count++
		}
	}
	if count == 1 {
		return active, true
	}
	if len(projection.Realms) == 1 {
		for _, state := range projection.Realms {
			return state, true
		}
	}
	return RealmInstanceState{}, false
}

// realmResolveDefinition picks the definition an entry request names.
func realmResolveDefinition(req RealmRequest, prior RealmInstanceState, exists bool, content *WorldContent) (exploration.DungeonDefinition, error) {
	realmID := strings.TrimSpace(req.RealmID)
	if realmID == "" && exists {
		realmID = prior.Snapshot.Binding.DefinitionID
	}
	if realmID == "" {
		return exploration.DungeonDefinition{}, fmt.Errorf("%w: no realm named", ErrRealmUnknownRealm)
	}
	definition, ok := worldRealmDefinition(content, realmID)
	if !ok {
		return exploration.DungeonDefinition{}, fmt.Errorf("%w: %s", ErrRealmUnknownRealm, realmID)
	}
	if exists && (definition.ID != prior.Snapshot.Binding.DefinitionID || definition.Version != prior.Snapshot.Binding.DefinitionVersion) {
		return exploration.DungeonDefinition{}, fmt.Errorf("%w: %s changed from %s to %s",
			ErrRealmUnknownRealm, realmID, prior.Snapshot.Binding.DefinitionVersion, definition.Version)
	}
	return definition, nil
}

// realmDefinitionForInstance re-reads the definition an existing instance was
// bound to.
func realmDefinitionForInstance(snapshot exploration.DungeonInstanceSnapshot, content *WorldContent) (exploration.DungeonDefinition, error) {
	definition, ok := worldRealmDefinition(content, snapshot.Binding.DefinitionID)
	if !ok {
		return exploration.DungeonDefinition{}, fmt.Errorf("%w: %s", ErrRealmUnknownRealm, snapshot.Binding.DefinitionID)
	}
	if definition.Version != snapshot.Binding.DefinitionVersion {
		return exploration.DungeonDefinition{}, fmt.Errorf("%w: %s changed from %s to %s",
			ErrRealmUnknownRealm, definition.ID, snapshot.Binding.DefinitionVersion, definition.Version)
	}
	return definition, nil
}

// realmRequireInside refuses an action that assumes the character is in the
// realm.
func realmRequireInside(snapshot exploration.DungeonInstanceSnapshot) error {
	if snapshot.Presence != exploration.InstanceInside {
		return fmt.Errorf("%w: the character is outside", ErrRealmNotPresent)
	}
	return nil
}

// realmActorOps writes what the realm action changed in the story's own state.
//
// The item write is the one that matters: a reward that only existed in the
// instance snapshot would be a treasure the panel shows and the character does
// not hold. Leaving writes the place the character came from, captured on the
// entering record, so the main story resumes where the realm was entered rather
// than at a place nobody authored.
func realmActorOps(projection *storyBranchProjection, definition exploration.DungeonDefinition, instance exploration.DungeonInstanceSnapshot, reward *exploration.InstanceReward, origin string) []ActorStateOp {
	ops := make([]ActorStateOp, 0, 2)
	if reward != nil {
		if entry, ok := realmRewardPackEntry(projection, definition, *reward); ok {
			ops = append(ops, ActorStateOp{
				Op:         "set",
				ActorID:    DefaultActorID,
				FieldID:    normalizeActorStateFieldName(xiuxianFieldImportantItems),
				Value:      entry,
				Reason:     "由秘境所得结算写入行囊，不由叙述改写。",
				SourceKind: StateOpSourceRealm,
				SourceID:   reward.ID,
			})
		}
	}
	if instance.Presence == exploration.InstanceOutside && strings.TrimSpace(origin) != "" {
		ops = append(ops, ActorStateOp{
			Op:         "set",
			ActorID:    DefaultStoryContextActorID,
			FieldID:    normalizeActorStateFieldName(storySceneLocationField),
			Value:      origin,
			Reason:     "由秘境离开结算写回进入前的位置，不由叙述改写。",
			SourceKind: StateOpSourceRealm,
			SourceID:   instance.Binding.InstanceID,
		})
	}
	return ops
}

// realmRewardPackEntry builds the whole 重要物品 map after the reward is added.
//
// The rules layer writes whole maps rather than patches for this field, because
// the pack is one record: a partial write would have to know the merge rule, and
// the merge rule belongs to the story's own item format. A stackable reward adds
// to the count it already has; equipment is one real object, so a second copy
// under the same key is left as the first.
//
// The 来历 is written from the realm and the room the copy was taken out of,
// because that is the real source and the instance snapshot is what proves it.
// Keeping whatever wording an earlier copy carried would let a relic found in a
// cave read as something the character bought.
func realmRewardPackEntry(projection *storyBranchProjection, definition exploration.DungeonDefinition, reward exploration.InstanceReward) (map[string]any, bool) {
	item, ok := storyItemCatalog(projection.cultivationRules()).Definition(reward.RefID)
	if !ok {
		// A reward naming an unregistered definition would be a treasure with no
		// published numbers. The caller refuses before this point, but a record
		// replayed from an older catalogue must not invent one either.
		return nil, false
	}
	var state map[string]any
	if projection != nil {
		state = projection.State
	}
	pack, _ := xiuxianEquipmentEntry(state, DefaultActorID, xiuxianFieldImportantItems)
	next := make(map[string]any, len(pack)+1)
	for key, value := range pack {
		next[key] = value
	}
	name := reward.RefID
	quantity := int(reward.Quantity)
	if quantity <= 0 {
		quantity = 1
	}
	if item.Kind == cultivation.KindImplement || item.Kind == cultivation.KindArmor {
		quantity = 1
	}
	existing, held := next[name].(map[string]any)
	if held {
		if recorded := xiuxianEntryText(existing, "定义"); recorded == "" || recorded == item.ID {
			if item.Kind != cultivation.KindImplement && item.Kind != cultivation.KindArmor {
				quantity += xiuxianCarriedQuantity(existing)
			}
		}
	}
	next[name] = xiuxianItemPackEntryWithFlavor(
		item,
		name,
		quantity,
		xiuxianEntryText(existing, "描述"),
		realmRewardProvenance(definition, reward),
	)
	return next, true
}

// realmRewardProvenance names the room a reward was taken out of.
//
// The room is the whole honest answer: it is where the character really stood
// when the grant was committed, and the content reference is the authored record
// that ties the reward to it. The realm itself has no name field — its Theme is a
// sentence, and realmDisplayName returns the entry room rather than the realm —
// so writing one in would be inventing a place name the world pack never wrote.
func realmRewardProvenance(definition exploration.DungeonDefinition, reward exploration.InstanceReward) string {
	for _, reference := range definition.ContentRefs {
		if reference.ID != reward.ID {
			continue
		}
		for _, area := range definition.Areas {
			if area.ID == reference.AreaID && strings.TrimSpace(area.Name) != "" {
				return fmt.Sprintf("在%s取得。", area.Name)
			}
		}
		break
	}
	return "在此处取得。"
}

// realmCreatePermit builds the trusted permit a new instance is created under.
func realmCreatePermit(definition exploration.DungeonDefinition, scope exploration.DungeonScope, instanceID, sourceRef string) (exploration.InstanceActionPermit, error) {
	hash, err := exploration.DungeonDefinitionContentHash(definition)
	if err != nil {
		return exploration.InstanceActionPermit{}, err
	}
	return exploration.InstanceActionPermit{
		Binding: exploration.InstancePermitBinding{
			InstanceID: instanceID, WorldID: scope.WorldID, BranchID: scope.BranchID,
			DefinitionHash: hash, Revision: 0,
		},
		Decision: exploration.DecisionAllowed, Action: exploration.ActionEnter,
		TargetID: definition.EntryAreaIDs[0], SourceRef: sourceRef,
	}, nil
}

// realmActionContext builds the trusted context one action runs under.
//
// The permit is issued here rather than accepted from the caller: the product
// layer is what knows the character is really at the area the action names, and
// the rules layer checks that the permit it was handed matches the instance and
// revision it is actually acting on.
func realmActionContext(instance exploration.DungeonInstanceSnapshot, scope exploration.DungeonScope, action exploration.InstanceActionKind, targetID, fromAreaID, toAreaID, sourceRef string) (exploration.InstanceActionContext, error) {
	return exploration.InstanceActionContext{
		ExpectedRevision: instance.Revision,
		Scope:            scope,
		Permit: exploration.InstanceActionPermit{
			Binding: exploration.InstancePermitBinding{
				InstanceID: instance.Binding.InstanceID, WorldID: scope.WorldID, BranchID: scope.BranchID,
				DefinitionHash: instance.DefinitionHash, Revision: instance.Revision,
			},
			Decision: exploration.DecisionAllowed, Action: action,
			TargetID: targetID, FromAreaID: fromAreaID, ToAreaID: toAreaID, SourceRef: sourceRef,
		},
		Battle:   exploration.BattleNone,
		Blocking: exploration.BlockingNone,
	}, nil
}

// realmOpeningDiscoveries is what the character knows on the way in.
//
// The entry area and the realm's declared objectives are known from the brief the
// player acted on; nothing else is. §11.2 keeps hidden content hidden, so the
// inner rooms and the rewards are not seeded here.
func realmOpeningDiscoveries(definition exploration.DungeonDefinition) []exploration.InstanceDiscovery {
	discoveries := []exploration.InstanceDiscovery{
		{Kind: exploration.DiscoveryArea, ID: definition.EntryAreaIDs[0], SourceRef: "realm.enter"},
	}
	if definition.ExploreRules != nil {
		return discoveries
	}
	for _, objective := range definition.Objectives {
		discoveries = append(discoveries, exploration.InstanceDiscovery{
			Kind: exploration.DiscoveryObjective, ID: objective.ID, SourceRef: "realm.enter",
		})
	}
	return discoveries
}

// realmRecordedContentRef keeps the reward identity on the record only for the
// action that takes a reward.
//
// A move or an exit that carried a content reference would read as a grant to a
// later reader, so the other actions store nothing rather than echoing the
// request.
func realmRecordedContentRef(action, contentRefID string) string {
	if action != RealmActionReward {
		return ""
	}
	return strings.TrimSpace(contentRefID)
}

// realmRecordForCommand finds the action a previous attempt committed.
func realmRecordForCommand(records []StoryEventRecord, branchID, commandID, commandHash string, content *WorldContent) (RealmResult, bool, error) {
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeRealmRecord {
			continue
		}
		var event RealmRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return RealmResult{}, false, fmt.Errorf("decode committed realm record: %w", err)
		}
		if event.BranchID != branchID || strings.TrimSpace(event.CommandID) != commandID {
			continue
		}
		if strings.TrimSpace(event.CommandHash) != commandHash {
			return RealmResult{}, false, fmt.Errorf("%w: command_id=%q existing_hash=%q requested_hash=%q",
				ErrRealmCommandConflict, commandID, event.CommandHash, commandHash)
		}
		// The replay answer carries the same view the first attempt reported, so a
		// repeated click shows the room and the reward it already committed rather
		// than an empty realm.
		result := RealmResult{
			EventID: event.ID, InstanceID: event.Instance.Binding.InstanceID,
			RealmID: event.RealmID, Action: event.Action,
			Instance: event.Instance, Replayed: true, Rewards: append([]exploration.InstanceReward(nil), event.Rewards...),
		}
		if event.ClockBefore != nil && event.ClockAfter != nil {
			result.DurationSeconds = event.ClockAfter.Seconds - event.ClockBefore.Seconds
		}
		if definition, ok := worldRealmDefinition(content, event.RealmID); ok && definition.Version == event.RealmVersion {
			scope := exploration.DungeonScope{
				WorldID:  event.Instance.Binding.WorldID,
				BranchID: event.Instance.Binding.BranchID,
			}
			if view, diagnostics := exploration.ProjectDungeonInstanceView(definition, scope, event.Instance); len(diagnostics) == 0 {
				result.View = view
			}
		}
		if event.ContentRefID != "" {
			for _, granted := range event.Instance.GrantedRewards {
				if granted.ID != event.ContentRefID {
					continue
				}
				reward := granted
				result.Reward = &reward
				break
			}
		}
		return result, true, nil
	}
	return RealmResult{}, false, nil
}

// applyRealmRecord merges a side record into the branch projection.
//
// The stored snapshot is re-validated against the definition and the rules rather
// than trusted, so a corrupt row cannot hand the branch a realm state the rules
// would not produce.
func applyRealmRecord(branch *storyBranchProjection, event RealmRecordEvent) error {
	definition, ok := worldRealmDefinition(branch.worldContent(), event.RealmID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrRealmUnknownRealm, event.RealmID)
	}
	if definition.Version != event.RealmVersion {
		return fmt.Errorf("%w: %s version %s", ErrRealmUnknownRealm, event.RealmID, event.RealmVersion)
	}
	scope := exploration.DungeonScope{
		WorldID:  event.Instance.Binding.WorldID,
		BranchID: event.Instance.Binding.BranchID,
	}
	if diagnostics := exploration.ValidateDungeonInstance(definition, scope, event.Instance); len(diagnostics) > 0 {
		return fmt.Errorf("%w: %s", exploration.ErrInvalidInstance, diagnostics[0].Message)
	}
	if err := applyRealmClock(branch, event); err != nil {
		return err
	}
	if branch.Realms == nil {
		branch.Realms = map[string]RealmInstanceState{}
	}
	prior := branch.Realms[event.Instance.Binding.InstanceID]
	origin := event.OriginLocation
	if strings.TrimSpace(origin) == "" {
		origin = prior.OriginLocation
	}
	branch.Realms[event.Instance.Binding.InstanceID] = RealmInstanceState{
		Snapshot:       cloneRealmInstance(event.Instance),
		OriginLocation: origin,
		ExploreSources: realmExploreSources(prior, event),
	}
	return nil
}

// validateRealmRecordEvent rejects a record that could not have been honestly
// attributed.
func validateRealmRecordEvent(event RealmRecordEvent) error {
	if strings.TrimSpace(event.BranchID) == "" {
		return fmt.Errorf("秘境记录缺少 branch_id / a realm record requires a branch")
	}
	if strings.TrimSpace(event.RealmID) == "" {
		return fmt.Errorf("秘境记录缺少 realm_id / a realm record requires a realm")
	}
	if strings.TrimSpace(event.Instance.Binding.InstanceID) == "" {
		return fmt.Errorf("秘境记录缺少实例 / a realm record requires an instance")
	}
	switch event.Action {
	case RealmActionEnter, RealmActionMove, RealmActionReward, RealmActionExit, RealmActionReenter, RealmActionSurvey, RealmActionPrepare, RealmActionApproach, RealmActionResolve:
	default:
		return fmt.Errorf("秘境记录的动作未知 / unknown realm action %q", event.Action)
	}
	return nil
}

// cloneRealmInstances copies the realm map so one branch's instance cannot be
// mutated through an alias into another.
func cloneRealmInstances(realms map[string]RealmInstanceState) map[string]RealmInstanceState {
	if len(realms) == 0 {
		return nil
	}
	cloned := make(map[string]RealmInstanceState, len(realms))
	for id, state := range realms {
		cloned[id] = RealmInstanceState{
			Snapshot:       cloneRealmInstance(state.Snapshot),
			OriginLocation: state.OriginLocation,
			ExploreSources: cloneExploreSources(state.ExploreSources),
		}
	}
	return cloned
}

// forkRealmInstances copies a parent's realms onto a new branch.
//
// A realm snapshot is the one checkpoint that carries its trusted scope inside
// the fact itself: the binding names the branch the instance belongs to, and
// every action validates the binding against the branch acting. Copying the
// parent's snapshot unchanged made a fork inherit an instance bound to the
// parent, so every move, reward and exit on the fork was refused with
// "instance belongs to a different trusted branch scope" — reproduced live on a
// branch forked from a turn taken inside the realm. The fork's copy is the
// fork's own, exactly as the journey, task and event checkpoints are, so the
// binding is re-pointed at the branch receiving it. Nothing else about the
// instance changes: the same rooms, discoveries, rewards and revision.
func forkRealmInstances(realms map[string]RealmInstanceState, branchID string) map[string]RealmInstanceState {
	cloned := cloneRealmInstances(realms)
	for id, state := range cloned {
		state.Snapshot.Binding.BranchID = branchID
		cloned[id] = state
	}
	return cloned
}

// cloneRealmInstance deep-copies the slices a snapshot carries.
func cloneRealmInstance(snapshot exploration.DungeonInstanceSnapshot) exploration.DungeonInstanceSnapshot {
	snapshot.Discoveries = append([]exploration.InstanceDiscovery(nil), snapshot.Discoveries...)
	snapshot.CompletedObjectives = append([]exploration.SourcedProgress(nil), snapshot.CompletedObjectives...)
	snapshot.ResolvedChallenges = append([]exploration.SourcedProgress(nil), snapshot.ResolvedChallenges...)
	snapshot.GrantedRewards = append([]exploration.InstanceReward(nil), snapshot.GrantedRewards...)
	if snapshot.ExploreState != nil {
		v := *snapshot.ExploreState
		v.UsedApproachIDs = append([]string(nil), v.UsedApproachIDs...)
		v.Decisions = append([]exploration.ExploreDecision(nil), v.Decisions...)
		snapshot.ExploreState = &v
	}
	return snapshot
}

// realmWorldID is the trusted world scope a branch's realms belong to.
//
// The story has one world; a branch is the unit that can differ. Using the story
// id keeps two stories' instances from sharing a scope even when they name the
// same realm.
func realmWorldID(meta StoryMeta, branchID string) string {
	if id := strings.TrimSpace(meta.StoryID); id != "" {
		return id
	}
	return "story." + branchID
}

// RealmStateView is one realm as the panel reads it.
type RealmStateView struct {
	NameKey        string `json:"name_key,omitempty"`
	DescriptionKey string `json:"description_key,omitempty"`
	InstanceID     string `json:"instance_id"`
	RealmID        string `json:"realm_id"`
	// Name and Theme are the definition's own wording, so the panel never
	// invents a title.
	Name  string `json:"name"`
	Theme string `json:"theme"`
	// Presence is "inside" or "outside"; the panel offers different actions.
	Presence string `json:"presence"`
	// View is the player-known projection: only what this character has really
	// discovered.
	View exploration.DungeonInstanceView `json:"view"`
	// OriginLocation is where the character entered from.
	OriginLocation string `json:"origin_location,omitempty"`
	// Rewards are the declared rewards in the room the character is standing in
	// that have not been taken yet. They are resolved here rather than in the
	// panel, because the definition is the authority on what the realm holds and
	// an interface copy would drift.
	Rewards []RealmRewardOption `json:"rewards"`
	// Granted are the rewards this instance has already handed over, in the same
	// wording, so a taken reward is reported as taken rather than as available.
	Granted []RealmGrantedReward `json:"granted"`
	// Ways are the doors out of the room the character is standing in.
	//
	// View.Connections cannot serve: a projection lists a connection only after
	// both of its ends are known, and the far end of a door is by definition not
	// known until it is walked. Without this list nothing tells the narrator a
	// connection_id, so realm_action=move cannot be submitted at all — the first
	// live run entered the outer hall and then had no door to name.
	Ways    []RealmWay               `json:"ways"`
	Explore *exploration.ExploreView `json:"explore,omitempty"`
}

// RealmWay is one visible door out of the current room.
type RealmWay struct {
	ConnectionID string `json:"connection_id"`
	// DestinationAreaID and DestinationName are the far side as the definition
	// names it. A door is visible from the room it opens off; what is behind it
	// is still a fact the character learns by walking through.
	DestinationAreaID  string `json:"destination_area_id"`
	DestinationName    string `json:"destination_name"`
	DestinationNameKey string `json:"destination_name_key,omitempty"`
}

// RealmRewardOption is one reward the character may take right now.
type RealmRewardOption struct {
	// ContentRefID is the definition's own reference, which is what a take names.
	ContentRefID string `json:"content_ref_id"`
	RefID        string `json:"ref_id"`
	// NameKey is the catalogue's localisation key, so the panel never invents a
	// name for an item the catalogue already names.
	NameKey  string `json:"name_key"`
	Quantity int64  `json:"quantity"`
}

// RealmGrantedReward is one reward this instance has already handed over.
type RealmGrantedReward struct {
	Name         string `json:"name,omitempty"`
	ContentRefID string `json:"content_ref_id"`
	RefID        string `json:"ref_id"`
	NameKey      string `json:"name_key"`
	Quantity     int64  `json:"quantity"`
}

// RealmState is one branch's realms plus the authored realm the player can
// still enter.
type RealmState struct {
	Cooperations    []CooperationProjectView `json:"cooperations"`
	ClockSeconds    int64                    `json:"clock_seconds"`
	CurrentLocation string                   `json:"current_location,omitempty"`
	Instances       []RealmStateView         `json:"instances"`
	// Available lists the authored realms, each with its declared entry area.
	Available []RealmOption `json:"available"`
}

// RealmOption is one authored realm as the entry offers it.
type RealmOption struct {
	NameKey        string `json:"name_key,omitempty"`
	DescriptionKey string `json:"description_key,omitempty"`
	RealmID        string `json:"realm_id"`
	RealmVersion   string `json:"realm_version"`
	Name           string `json:"name"`
	Theme          string `json:"theme"`
	EntryAreaID    string `json:"entry_area_id"`
}

// RealmStateFor reads the branch's realms without writing anything.
//
// Opening the panel costs no world time and changes no state, for the same
// reason opening the journey entry does not.
func (s *Store) RealmStateFor(storyID, branchID string) (RealmState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return RealmState{}, err
	}
	defer release()
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return RealmState{}, err
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return RealmState{}, err
	}
	meta := normalizeStoryMeta(handle.projection.Meta)
	return realmStateFrom(meta, projection), nil
}

// realmStateFrom builds the realm view from a projection the caller has already
// read.
//
// It is split out so the main story's World Actions page can report the same
// realms from one journal read. The known-instance half is what the character has
// really seen; the available half is filtered by the same eligibility predicate
// the entry uses, so the page never names a realm the commit would refuse.
func realmStateFrom(meta StoryMeta, projection *storyBranchProjection) RealmState {
	state := RealmState{
		Cooperations:    cooperationProjectViews(projection, meta),
		ClockSeconds:    projection.clockSeconds(),
		CurrentLocation: storySceneText(projection.State, storySceneLocationField),
		Instances:       []RealmStateView{},
		Available:       []RealmOption{},
	}
	ids := make([]string, 0, len(projection.Realms))
	for id := range projection.Realms {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		instance := projection.Realms[id]
		definition, ok := worldRealmDefinition(worldContentForMeta(meta), instance.Snapshot.Binding.DefinitionID)
		if !ok {
			continue
		}
		scope := exploration.DungeonScope{
			WorldID:  instance.Snapshot.Binding.WorldID,
			BranchID: instance.Snapshot.Binding.BranchID,
		}
		view, diagnostics := exploration.ProjectDungeonInstanceView(definition, scope, instance.Snapshot)
		if len(diagnostics) > 0 {
			continue
		}
		state.Instances = append(state.Instances, RealmStateView{
			InstanceID: id, RealmID: definition.ID,
			Name: realmDisplayName(definition), Theme: definition.Theme,
			NameKey: realmNameKey(definition), DescriptionKey: realmDescriptionKey(definition),
			Presence:       string(instance.Snapshot.Presence),
			View:           view,
			OriginLocation: instance.OriginLocation,
			Rewards: realmRewardOptions(cultivationRulesForMeta(meta),
				definition, instance.Snapshot),
			Granted: realmGrantedRewards(instance.Snapshot, actorStateSystemFromSnapshot(meta.ActorStateSchema, StoryDirectorActorStateSystem{})),
			Ways:    realmWaysFrom(definition, instance.Snapshot),
			Explore: realmExploreView(meta, projection, definition, scope, instance.Snapshot),
		})
	}
	inside := false
	for _, instance := range projection.Realms {
		if instance.Snapshot.Presence == exploration.InstanceInside {
			inside = true
			break
		}
	}
	for _, definition := range worldRealmDefinitions(worldContentForMeta(meta)) {
		if inside {
			continue
		}
		if len(definition.EntryAreaIDs) == 0 {
			continue
		}
		// §11.2: the realm is offered only where it can really be entered, and
		// only from this story's own world revision. The commit asks the same
		// question, so the entry never shows a realm it would refuse.
		if !realmDefinitionEligible(meta, definition, projection.State) {
			continue
		}
		state.Available = append(state.Available, RealmOption{
			RealmID: definition.ID, RealmVersion: definition.Version,
			Name: realmDisplayName(definition), Theme: definition.Theme,
			NameKey: realmNameKey(definition), DescriptionKey: realmDescriptionKey(definition),
			EntryAreaID: definition.EntryAreaIDs[0],
		})
	}
	return state
}

// realmConnectionTo names the visible door from one room to another.
//
// A player names where they are going; the connection id is the routing fact the
// rules need. It returns "" when no single visible connection joins the two, so
// the caller refuses rather than picks one.
func realmConnectionTo(definition exploration.DungeonDefinition, fromAreaID, toAreaID string) string {
	if toAreaID == "" {
		return ""
	}
	found := ""
	for _, connection := range definition.Connections {
		if !connection.PlayerVisible {
			continue
		}
		matches := connection.FromAreaID == fromAreaID && connection.ToAreaID == toAreaID
		if connection.Bidirectional {
			matches = matches || (connection.ToAreaID == fromAreaID && connection.FromAreaID == toAreaID)
		}
		if !matches {
			continue
		}
		if found != "" {
			return ""
		}
		found = connection.ID
	}
	return found
}

// realmWaysFrom is the doors out of the room the character stands in.
//
// A door is visible from the side it opens off: the definition marks its
// connections PlayerVisible, and the room the character is in is known to them
// by standing there. Only the far side's name is added, not its contents — the
// projection still withholds the area until it is walked into.
func realmWaysFrom(definition exploration.DungeonDefinition, snapshot exploration.DungeonInstanceSnapshot) []RealmWay {
	ways := []RealmWay{}
	if snapshot.Presence != exploration.InstanceInside {
		return ways
	}
	names := make(map[string]string, len(definition.Areas))
	nameKeys := make(map[string]string, len(definition.Areas))
	for _, area := range definition.Areas {
		names[area.ID] = area.Name
		nameKeys[area.ID] = area.NameKey
	}
	for _, connection := range definition.Connections {
		if !realmConnectionKnown(definition, snapshot, connection.ID) {
			continue
		}
		var destination string
		switch {
		case connection.FromAreaID == snapshot.CurrentAreaID:
			destination = connection.ToAreaID
		case connection.Bidirectional && connection.ToAreaID == snapshot.CurrentAreaID:
			destination = connection.FromAreaID
		default:
			continue
		}
		ways = append(ways, RealmWay{
			ConnectionID:       connection.ID,
			DestinationAreaID:  destination,
			DestinationName:    names[destination],
			DestinationNameKey: nameKeys[destination],
		})
	}
	sort.Slice(ways, func(i, j int) bool { return ways[i].ConnectionID < ways[j].ConnectionID })
	return ways
}

// realmRewardOptions is what the character may take from the room they are
// standing in.
//
// It is empty when they are outside, because a reward is taken from inside, and
// it excludes anything already granted, because the realm does not grow a second
// copy. The definition is read here rather than by the panel so an interface
// cannot offer a reward the rules would refuse.
func realmRewardOptions(cultivationRules *cultivation.Rules, definition exploration.DungeonDefinition, snapshot exploration.DungeonInstanceSnapshot) []RealmRewardOption {
	options := []RealmRewardOption{}
	if definition.ExploreRules != nil {
		return options
	}
	if snapshot.Presence != exploration.InstanceInside {
		return options
	}
	taken := make(map[string]bool, len(snapshot.GrantedRewards))
	for _, granted := range snapshot.GrantedRewards {
		taken[granted.ID] = true
	}
	for _, reference := range definition.ContentRefs {
		if reference.Kind != "reward" || reference.AreaID != snapshot.CurrentAreaID || taken[reference.ID] {
			continue
		}
		options = append(options, RealmRewardOption{
			ContentRefID: reference.ID, RefID: reference.ReferenceID,
			NameKey: StoryItemNameKey(cultivationRules,
				reference.ReferenceID), Quantity: reference.Quantity,
		})
	}
	return options
}

// realmGrantedRewards is what this instance has already handed over, in the same
// wording the options use.
func realmGrantedRewards(snapshot exploration.DungeonInstanceSnapshot, system StoryDirectorActorStateSystem) []RealmGrantedReward {
	granted := []RealmGrantedReward{}
	for _, reward := range snapshot.GrantedRewards {
		name := ""
		if system.AdventureRules != nil {
			for _, item := range system.AdventureRules.Items {
				if item.ID == reward.RefID {
					name = item.Name
					break
				}
			}
		}
		granted = append(granted, RealmGrantedReward{
			ContentRefID: reward.ID, RefID: reward.RefID, Name: name,
			NameKey: StoryItemNameKey(system.CultivationRules,
				reward.RefID), Quantity: reward.Quantity,
		})
	}
	return granted
}

// realmDisplayName is the realm's own name: its entry area's wording when the
// definition names one, because that is what the player first sees.
func realmDisplayName(definition exploration.DungeonDefinition) string {
	for _, area := range definition.Areas {
		if len(definition.EntryAreaIDs) > 0 && area.ID == definition.EntryAreaIDs[0] {
			return area.Name
		}
	}
	return definition.ID
}

// StoryItemNameKey is the catalogue's localisation key for one registered item.
//
// The rules layer names the definition; the wording belongs to the interface, so
// a caller that has to report a granted item gets the key rather than an invented
// name. An unregistered reference reads as empty, which is the honest answer for
// a reward the catalogue does not hold.
func StoryItemNameKey(cultivationRules *cultivation.Rules, refID string) string {
	definition, ok := storyItemCatalog(cultivationRules).Definition(strings.TrimSpace(refID))
	if !ok {
		return ""
	}
	return definition.NameKey
}
