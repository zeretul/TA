package interactive

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"ta/internal/cultivation"
	"ta/internal/exploration"
	"ta/internal/quest"
)

// This file makes an ordinary task a real, once-only fact on the branch.
//
// 自制-事件与任务 asks for one loop: a task is discovered, the player may accept
// or refuse it, doing the thing really advances it, and finishing it pays once.
// The properties that make that honest are all in the commit below.
//
//   - An offer is not an acceptance. §4.4 keeps those apart, so discovering a
//     task and even doing the work does not silently accept it on the player's
//     behalf.
//
//   - Objectives are ticked by committed facts, never by prose. A task names
//     the realm reward, journey arrival or battle result that satisfies it, and
//     only the commit that really produced that fact may tick it. The narrative
//     afterwards describes what was decided.
//
//   - Payment is one fact. §4.7 requires an agreed reward to be settled once;
//     the settled flag lives on the instance and the item and currency writes
//     are committed in the same transaction, so walking past the giver again
//     cannot pay twice.
//
// It is a side record for the same reason a journey is: accepting a task or
// ticking an objective is not a story turn, and making it one would let a
// window open advance the world.

// StateOpSourceQuest marks an Actor State operation a committed task settlement
// produced.
const StateOpSourceQuest = "quest"

// StoryEventTypeQuestRecord is a side record carrying one task instance after an
// action.
type StoryEventTypeQuestRecordMarker = struct{}

// QuestRecordEvent is one task action — offered, accepted, advanced, completed,
// abandoned or paid — together with the whole instance after it.
//
// The whole instance is stored rather than a patch for the same reason a realm
// action stores its whole snapshot: a replay that reaches this record produces
// exactly the state that was live, and a record that disagrees with the rules is
// refused rather than trusted.
type QuestRecordEvent struct {
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
	// TaskID and TaskVersion name the authored definition the instance is bound
	// to, so a definition that changed underneath is detected rather than
	// silently re-read.
	TaskID      string `json:"task_id"`
	TaskVersion string `json:"task_version"`
	// Action is the operation this record settled.
	Action string `json:"action"`
	// FactRef names the committed fact this action was driven by, empty for an
	// action the player took directly. It is stored so a tick can be traced back
	// to the thing that caused it.
	FactRef string `json:"fact_ref,omitempty"`
	// Instance is the whole task after this action.
	Instance quest.Instance `json:"instance"`
	// Rewards are what the settlement handed over, empty for any other action.
	// They are repeated on the record so a replay reports the same payment.
	Rewards     []quest.Reward `json:"rewards,omitempty"`
	ClockBefore *StoryClock    `json:"clock_before,omitempty"`
	ClockAfter  *StoryClock    `json:"clock_after,omitempty"`
}

// The task actions one record may settle.
const (
	QuestActionOffer    = "offer"
	QuestActionChoose   = "choose"
	QuestActionEcho     = "echo"
	QuestActionFollowUp = "follow_up"
	QuestActionAccept   = "accept"
	QuestActionAbandon  = "abandon"
	QuestActionComplete = "complete"
	QuestActionSettle   = "settle"
	// QuestActionExpire records that formal time passed an agreed deadline. It
	// is not a player action: no request may submit it, and it is written only by
	// the interval pass that owns the committed clock.
	QuestActionExpire = "expire"
)

var (
	// ErrQuestUnknownTask means the named task is not authored.
	ErrQuestUnknownTask = errors.New("task is not registered")
	// ErrQuestNotHere means the task belongs to another world or another cast.
	ErrQuestNotHere = errors.New("task is not part of this story's world")
	// ErrQuestNotAccepted means an action requires the player to have taken the
	// task on.
	ErrQuestNotAccepted = errors.New("task is not accepted")
	// ErrQuestAlreadyOffered means the task is already in play on this branch.
	ErrQuestAlreadyOffered = errors.New("task is already offered")
	// ErrQuestAlreadySettled means the task already paid.
	ErrQuestAlreadySettled = errors.New("task is already settled")
	// ErrQuestCommandConflict means a command ID was reused for different
	// content.
	ErrQuestCommandConflict = errors.New("task command identity conflict")
	// ErrQuestUnknownAction means a request named an action this module does not
	// settle.
	ErrQuestUnknownAction = errors.New("task action is not supported")
	// ErrQuestObjectiveUnmet means the task cannot be completed yet.
	ErrQuestObjectiveUnmet = errors.New("task objective is not satisfied")
	// ErrQuestExpired means the agreed deadline passed before the work finished.
	// It is a world fact rather than a technical error: the opportunity closed.
	ErrQuestExpired = errors.New("task deadline has passed")
)

// QuestRequest is one player request to act on a task.
type QuestRequest struct {
	BranchID  string
	TurnID    string
	WorldTime string
	// TaskID names the authored task. Required to offer and accept.
	TaskID   string
	ChoiceID string
	// Action is offer, accept, abandon, complete or settle.
	Action string
	// CommandID and CommandHash identify one attempt.
	CommandID   string
	CommandHash string
}

// QuestResult is what a committed task action reports back.
type QuestResult struct {
	EventID  string
	TaskID   string
	Action   string
	Instance quest.Instance
	// Rewards is what this action paid, if it settled.
	Rewards []quest.Reward
	// SettlementOps are the Actor State writes this commit produced.
	SettlementOps []ActorStateOp
	Replayed      bool
}

// ApplyQuest settles one task action and commits it as a side record plus its
// payment in a single journal transaction.
func (s *Store) ApplyQuest(storyID string, req QuestRequest) (QuestResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return QuestResult{}, err
	}
	defer releaseStory()
	meta, records, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return QuestResult{}, err
	}
	return s.applyQuestLocked(storyID, meta, records, req)
}

// questPlannedAction is one task decision the rules have already accepted.
//
// It carries the definition the decision was made against and the instance the
// decision produces, so a caller can commit the record inside its own
// transaction. Nothing here has been written yet.
type questPlannedAction struct {
	Echo        *questEchoAcknowledgement
	Definition  quest.Definition
	Action      string
	Instance    quest.Instance
	Rewards     []quest.Reward
	Ops         []ActorStateOp
	ClockBefore *StoryClock
	ClockAfter  *StoryClock
}

// planQuestAction decides one task action against a branch projection.
//
// ApplyQuest calls it so the entry and a main-story turn cannot disagree about
// what accepting, completing or settling a task does; the turn calls it so a
// player who writes 接下这桩差事 gets the same instance, the same objective
// check and the same reward-once rule as the entry, rather than a scene that
// says a task was taken while nothing was recorded.
func planQuestAction(projection *storyBranchProjection, system StoryDirectorActorStateSystem, req QuestRequest) (questPlannedAction, error) {
	action := strings.TrimSpace(req.Action)
	taskID := strings.TrimSpace(req.TaskID)
	prior, exists := questPriorInstance(projection, taskID)
	if taskID == "" && exists {
		taskID = prior.TaskID
	}
	definition, found := worldQuestDefinition(projection.worldContent(), taskID)
	if !found {
		return questPlannedAction{}, fmt.Errorf("%w: %s", ErrQuestUnknownTask, taskID)
	}
	if exists && prior.TaskVersion != definition.Version {
		return questPlannedAction{}, fmt.Errorf("%w: %s changed from %s to %s",
			ErrQuestUnknownTask, taskID, prior.TaskVersion, definition.Version)
	}

	instance := prior
	var rewards []quest.Reward
	// now is the branch's committed formal instant, read the same way the
	// projection reads it. Acceptance stamps the agreed deadline from this number
	// and nothing else: the story's display text is not arithmetic.
	now := int64(0)
	if projection != nil {
		now = projection.clockSeconds()
	}
	switch action {
	case QuestActionOffer:
		if exists && instance.Status != quest.StatusOffered {
			// Offering a task already in play would reset it, which is how a
			// completed task would be made available again.
			return questPlannedAction{}, fmt.Errorf("%w: %s is already %s", ErrQuestNotAccepted, taskID, instance.Status)
		}
		if exists {
			// A task already on offer is not offered a second time: a duplicate
			// record would say the world changed when nothing did.
			return questPlannedAction{}, fmt.Errorf("%w: %s", ErrQuestAlreadyOffered, taskID)
		}
		instance = quest.Instance{
			TaskID: taskID, TaskVersion: definition.Version, Status: quest.StatusOffered,
			CompletedObjectiveIDs: []string{},
		}
	case QuestActionAccept:
		if !exists {
			// A player who says 接下这桩差事 has been shown the task in the scene
			// and is taking it in the same breath; there is no separate moment in
			// which they were merely offered it. Requiring an offer record first
			// was an artifact of the old two-click window and made the natural
			// sentence impossible to settle, so accepting a task that is authored,
			// eligible for this story and not yet in play creates it directly.
			//
			// The offer action still exists and still records its own instance:
			// a scene that raises a task without the player taking it yet is a
			// real distinction, and it stays one.
			instance = quest.Instance{
				TaskID: taskID, TaskVersion: definition.Version, Status: quest.StatusAccepted,
				CompletedObjectiveIDs: []string{}, AcceptedAt: strings.TrimSpace(req.WorldTime),
			}
			instance = quest.StampDeadline(definition, instance, now)
			if !definition.FreshFactsOnly {
				instance = questCreditCommittedFacts(definition, instance, projection, now)
			}
			break
		}
		if instance.Status != quest.StatusOffered {
			return questPlannedAction{}, fmt.Errorf("%w: %s is %s", ErrQuestNotAccepted, taskID, instance.Status)
		}
		instance.Status = quest.StatusAccepted
		instance.AcceptedAt = strings.TrimSpace(req.WorldTime)
		instance = quest.StampDeadline(definition, instance, now)
		if !definition.FreshFactsOnly {
			instance = questCreditCommittedFacts(definition, instance, projection, now)
		}
	case QuestActionFollowUp:
		return planQuestFollowUp(projection, definition, instance, exists, req)
	case QuestActionChoose:
		return planQuestChoice(projection, system, definition, instance, exists, req)
	case QuestActionAbandon:
		if !exists {
			return questPlannedAction{}, fmt.Errorf("%w: %s was not offered", ErrQuestNotAccepted, taskID)
		}
		if instance.Status == quest.StatusCompleted || instance.Status == quest.StatusAbandoned || instance.Status == quest.StatusExpired {
			return questPlannedAction{}, fmt.Errorf("%w: %s is %s", ErrQuestNotAccepted, taskID, instance.Status)
		}
		instance.Status = quest.StatusAbandoned
	case QuestActionComplete:
		if !exists {
			return questPlannedAction{}, fmt.Errorf("%w: %s was not offered", ErrQuestNotAccepted, taskID)
		}
		// The hour is asked about before the standing is. An acceptance that ran
		// out is still an acceptance the player made, and answering it with "not
		// accepted" told them they had never taken the work on — the opposite of
		// what happened, and a different thing to do about it. §五 says the
		// opportunity closes rather than the promise quietly succeeding, so the
		// refusal names the passed hour.
		if instance.Status == quest.StatusExpired || instance.ExpiredAt(now) {
			return questPlannedAction{}, fmt.Errorf("%w: %s", ErrQuestExpired, taskID)
		}
		if instance.Status != quest.StatusAccepted {
			return questPlannedAction{}, fmt.Errorf("%w: %s is %s", ErrQuestNotAccepted, taskID, instance.Status)
		}
		if !definition.IsComplete(instance.CompletedObjectiveIDs) {
			missing := definition.MissingObjectives(instance.CompletedObjectiveIDs)
			return questPlannedAction{}, fmt.Errorf("%w: %s still open", ErrQuestObjectiveUnmet, strings.Join(missing, ", "))
		}
		instance.Status = quest.StatusCompleted
	case QuestActionSettle:
		if !exists {
			return questPlannedAction{}, fmt.Errorf("%w: %s was not offered", ErrQuestNotAccepted, taskID)
		}
		if instance.SettledRewards {
			return questPlannedAction{}, fmt.Errorf("%w: %s already paid", ErrQuestAlreadySettled, taskID)
		}
		if instance.Status == quest.StatusExpired || instance.ExpiredAt(now) {
			// §五's "期限内已完成目标但尚未领报酬" case is the completed task that
			// expired later, and it keeps its claim. An unfinished acceptance that
			// ran out does not.
			if instance.Status != quest.StatusCompleted {
				return questPlannedAction{}, fmt.Errorf("%w: %s", ErrQuestExpired, taskID)
			}
		}
		if !definition.IsComplete(instance.CompletedObjectiveIDs) {
			missing := definition.MissingObjectives(instance.CompletedObjectiveIDs)
			return questPlannedAction{}, fmt.Errorf("%w: %s still open", ErrQuestObjectiveUnmet, strings.Join(missing, ", "))
		}
		if err := spiritQuestReviewReady(projection.State, taskID, projection.worldContent()); err != nil {
			return questPlannedAction{}, err
		}
		if definition.CompletionEcho != nil {
			if _, ok := questEchoPartner(projection, definition.CompletionEcho.Reaction.CharacterSourceKey); !ok {
				return questPlannedAction{}, fmt.Errorf("%w: the handover recipient is not available in this scene", quest.ErrInvalidState)
			}
			due, err := adventureDeadline(now, definition.CompletionEcho.AfterSeconds)
			if err != nil {
				return questPlannedAction{}, err
			}
			instance.CompletionEcho = &quest.EchoResult{AtSeconds: now, DueSeconds: due}
		}
		instance.Status = quest.StatusCompleted
		instance.SettledRewards = true
		instance.SettledAt = strings.TrimSpace(req.WorldTime)
		rewards = questEarnedRewards(definition, instance)
	default:
		return questPlannedAction{}, fmt.Errorf("%w: %q", ErrQuestUnknownAction, action)
	}

	if diagnostics := quest.ValidateInstance(definition, instance); len(diagnostics) > 0 {
		return questPlannedAction{}, fmt.Errorf("%w: %s", quest.ErrInvalidState, diagnostics[0].Message)
	}
	ops, err := questSettlementOps(projection, system, definition, rewards)
	if err != nil {
		return questPlannedAction{}, err
	}
	for _, op := range ops {
		if err := validateActorStateOp(op); err != nil {
			return questPlannedAction{}, err
		}
	}
	return questPlannedAction{Definition: definition, Action: action, Instance: instance, Rewards: rewards, Ops: ops}, nil
}

// questSettlementEvents turns a planned task decision into the events a commit
// writes: the record, then its reward write.
func questRecordEvent(branchID, parentID, now string, req QuestRequest, plan questPlannedAction, eventID string) QuestRecordEvent {
	return QuestRecordEvent{
		V: schemaVersion, Type: StoryEventTypeQuestRecord, ID: eventID,
		ParentID: parentID, BranchID: branchID, Ts: now,
		TurnID: strings.TrimSpace(req.TurnID), Cursor: eventID,
		CommandID: strings.TrimSpace(req.CommandID), CommandHash: strings.TrimSpace(req.CommandHash),
		WorldTime:   strings.TrimSpace(req.WorldTime),
		TaskID:      plan.Definition.ID,
		TaskVersion: plan.Definition.Version,
		Action:      plan.Action,
		Instance:    plan.Instance,
		Rewards:     plan.Rewards,
		ClockBefore: plan.ClockBefore, ClockAfter: plan.ClockAfter,
	}
}

// questSettlementEvents is the record plus its reward write, for callers that
// commit the task entry on its own.
func questSettlementEvents(branchID, parentID, now string, req QuestRequest, plan questPlannedAction, eventID string) ([]any, string) {
	record := questRecordEvent(branchID, parentID, now, req, plan, eventID)
	headID := eventID
	newEvents := []any{record}
	if echo := questEchoCharacterEvent(branchID, eventID, now, req.TurnID, plan); echo != nil {
		newEvents = append(newEvents, *echo)
		headID = echo.ID
	}
	if len(plan.Ops) > 0 {
		deltaID := newID("sd")
		newEvents = append(newEvents, newStateDeltaEventWithActorOps(deltaID, headID, branchID, now, nil, plan.Ops))
		headID = deltaID
	}
	return newEvents, headID
}

// applyQuestLocked is the commit itself.
func (s *Store) applyQuestLocked(storyID string, meta StoryMeta, records []StoryEventRecord, req QuestRequest) (QuestResult, error) {
	commandID := strings.TrimSpace(req.CommandID)
	commandHash := strings.TrimSpace(req.CommandHash)
	if commandID != "" && commandHash == "" {
		return QuestResult{}, fmt.Errorf("%w: command_id=%q requires a command hash", ErrQuestCommandConflict, commandID)
	}
	branchID := strings.TrimSpace(req.BranchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return QuestResult{}, fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}
	if commandID != "" {
		committed, found, err := questRecordForCommand(records, branchID, commandID, commandHash)
		if err != nil {
			return QuestResult{}, err
		}
		if found {
			committed.Replayed = true
			return committed, nil
		}
	}
	if branchIsTerminal(records, branch.Head) {
		return QuestResult{}, branchTerminalError()
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return QuestResult{}, err
	}

	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState)
	plan, err := planQuestAction(projection, system, req)
	if err != nil {
		return QuestResult{}, err
	}
	// §4.10: a task is raised in a world and by a person this story really has.
	// The entry asks the same question before offering it, so a task the player
	// was never shown cannot be accepted by naming its id.
	if !questDefinitionEligible(meta, plan.Definition) || !questActionDiscovered(projection, plan.Definition, plan.Action) {
		return QuestResult{}, fmt.Errorf("%w: %s is not part of this story's world", ErrQuestNotHere, plan.Definition.ID)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	eventID := newID("qstrec")
	newEvents, headID := questSettlementEvents(branchID, branch.Head, now, req, plan, eventID)
	if plan.ClockAfter != nil {
		view := questProjectionAfter(projection, plan)
		closed, next := storyIntervalEvents(view, system, branchID, headID, now, req.TurnID, "", *plan.ClockBefore, *plan.ClockAfter)
		newEvents = append(newEvents, closed...)
		headID = next
	}

	branch.Head = headID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, newEvents...); err != nil {
		return QuestResult{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return QuestResult{}, err
	}
	return QuestResult{
		EventID: eventID, TaskID: plan.Definition.ID, Action: plan.Action,
		Instance: plan.Instance, Rewards: plan.Rewards, SettlementOps: plan.Ops,
	}, nil
}

// questPriorInstance finds the task instance a request acts on.
func questPriorInstance(projection *storyBranchProjection, taskID string) (quest.Instance, bool) {
	if projection == nil {
		return quest.Instance{}, false
	}
	if id := strings.TrimSpace(taskID); id != "" {
		instance, ok := projection.Quests[id]
		return instance, ok
	}
	return quest.Instance{}, false
}

// questSettlementOps writes what a settlement really paid.
//
// Currency and items are separate writes because they are separate facts: the
// purse moves and the pack moves, and a payment that only did one of them would
// be half a reward. An unregistered item is refused rather than written, because
// a payment naming an entry the catalogue does not hold would be a reward with
// no published numbers.
//
// The provenance is written here rather than left to the narration. §4.2 of
// 自制-装备与物品品级 requires an item's actual source to be confirmed at the
// moment it is handed over, and this module is the one that knows it: the reward
// came from this task's settlement. Reusing the copy's old 来历 would have left
// a task-awarded sword reading as "bought at the market" if the model had reused
// a name already in the pack.
func questSettlementOps(projection *storyBranchProjection, system StoryDirectorActorStateSystem, definition quest.Definition, rewards []quest.Reward) ([]ActorStateOp, error) {
	if len(rewards) == 0 {
		return nil, nil
	}
	if system.AdventureRules != nil {
		return adventureQuestRewardOps(projection, system, definition, rewards)
	}
	provenance := questRewardProvenance(definition)
	var state map[string]any
	if projection != nil {
		state = projection.State
	}
	var ops []ActorStateOp
	var currency int64
	currencyChanged := false
	pack, _ := xiuxianEquipmentEntry(state, DefaultActorID, xiuxianFieldImportantItems)
	next := make(map[string]any, len(pack)+1)
	for key, value := range pack {
		next[key] = value
	}
	packChanged := false
	for _, reward := range rewards {
		switch reward.Kind {
		case quest.RewardCurrency:
			currency += reward.Quantity
			currencyChanged = true
		case quest.RewardItem:
			item, ok := storyItemCatalog(system.CultivationRules).Definition(reward.RefID)
			if !ok {
				return nil, fmt.Errorf("%w: reward %s is not a registered item", quest.ErrInvalidTask, reward.RefID)
			}
			quantity := int(reward.Quantity)
			if quantity <= 0 {
				quantity = 1
			}
			if item.Kind == cultivation.KindImplement || item.Kind == cultivation.KindArmor {
				quantity = 1
			}
			// 独特 is one object in the world, not a quality tier, and the
			// acquisition path refuses a second copy under any name. A reward is
			// authored content, so it may pay the first one; paying a second would
			// put two of the same blade in one pack, which is the pair that rule
			// exists to prevent. The copy already held is the reward's whole
			// intent, so nothing is written rather than a duplicate appearing.
			if item.Rarity == cultivation.RarityUnique {
				if _, held := xiuxianPackHeldDefinition(next, item.ID); held {
					continue
				}
				if _, held := xiuxianHeldDefinition(state, DefaultActorID, item.ID); held {
					continue
				}
			}
			existing, held := next[item.ID].(map[string]any)
			if held {
				if item.Kind != cultivation.KindImplement && item.Kind != cultivation.KindArmor {
					quantity += xiuxianCarriedQuantity(existing)
				}
			}
			next[item.ID] = xiuxianItemPackEntryWithFlavor(
				item,
				item.ID,
				quantity,
				xiuxianEntryText(existing, "描述"),
				provenance,
			)
			packChanged = true
		}
	}
	if currencyChanged {
		balance := int64(xiuxianStoredInt(state, DefaultActorID, xiuxianFieldCurrency)) + currency
		ops = append(ops, ActorStateOp{
			Op:         "set",
			ActorID:    DefaultActorID,
			FieldID:    normalizeActorStateFieldName(xiuxianFieldCurrency),
			Value:      float64(balance),
			Reason:     "由任务报酬结算写入余额，不由叙述改写。",
			SourceKind: StateOpSourceQuest,
			SourceID:   "quest.reward",
		})
	}
	if packChanged {
		ops = append(ops, ActorStateOp{
			Op:         "set",
			ActorID:    DefaultActorID,
			FieldID:    normalizeActorStateFieldName(xiuxianFieldImportantItems),
			Value:      next,
			Reason:     "由任务报酬结算写入行囊，不由叙述改写。",
			SourceKind: StateOpSourceQuest,
			SourceID:   "quest.reward",
		})
	}
	return ops, nil
}

// questRewardProvenance states where a task's item rewards came from, in the
// story's own words.
//
// The giver's id is deliberately not written here: an id is a stable identity,
// not a name the player reads, and putting "npc_su_qian" in a journal entry
// would be engineering wording leaking into the story. The task's own title is
// authored prose and says enough for the copy to have a real origin.
func questRewardProvenance(definition quest.Definition) string {
	if title := strings.TrimSpace(definition.Title); title != "" {
		return fmt.Sprintf("任务「%s」的约定报酬。", title)
	}
	return "任务约定报酬。"
}

// questRecordForCommand finds the action a previous attempt committed.
func questRecordForCommand(records []StoryEventRecord, branchID, commandID, commandHash string) (QuestResult, bool, error) {
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeQuestRecord {
			continue
		}
		var event QuestRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return QuestResult{}, false, fmt.Errorf("decode committed task record: %w", err)
		}
		if event.BranchID != branchID || strings.TrimSpace(event.CommandID) != commandID {
			continue
		}
		if strings.TrimSpace(event.CommandHash) != commandHash {
			return QuestResult{}, false, fmt.Errorf("%w: command_id=%q existing_hash=%q requested_hash=%q",
				ErrQuestCommandConflict, commandID, event.CommandHash, commandHash)
		}
		return QuestResult{
			EventID: event.ID, TaskID: event.TaskID, Action: event.Action,
			Instance: event.Instance, Rewards: event.Rewards, Replayed: true,
		}, true, nil
	}
	return QuestResult{}, false, nil
}

// applyQuestRecord merges a side record into the branch projection.
func applyQuestRecord(branch *storyBranchProjection, event QuestRecordEvent) error {
	definition, ok := worldQuestDefinition(branch.worldContent(), event.TaskID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrQuestUnknownTask, event.TaskID)
	}
	if definition.Version != event.TaskVersion {
		return fmt.Errorf("%w: %s version %s", ErrQuestUnknownTask, event.TaskID, event.TaskVersion)
	}
	if diagnostics := quest.ValidateInstance(definition, event.Instance); len(diagnostics) > 0 {
		return fmt.Errorf("%w: %s", quest.ErrInvalidState, diagnostics[0].Message)
	}
	if branch.Quests == nil {
		branch.Quests = map[string]quest.Instance{}
	}
	if err := applyQuestClock(branch, event); err != nil {
		return err
	}
	branch.Quests[event.TaskID] = cloneQuestInstance(event.Instance)
	return nil
}

// validateQuestRecordEvent rejects a record that could not have been honestly
// attributed.
func validateQuestRecordEvent(event QuestRecordEvent) error {
	if strings.TrimSpace(event.BranchID) == "" {
		return fmt.Errorf("任务记录缺少 branch_id / a task record requires a branch")
	}
	if strings.TrimSpace(event.TaskID) == "" {
		return fmt.Errorf("任务记录缺少 task_id / a task record requires a task")
	}
	switch event.Action {
	case QuestActionOffer, QuestActionAccept, QuestActionAbandon, QuestActionComplete, QuestActionSettle, QuestActionExpire, QuestActionChoose, QuestActionEcho, QuestActionFollowUp:
	default:
		return fmt.Errorf("任务记录的动作未知 / unknown task action %q", event.Action)
	}
	return nil
}

// cloneQuestInstances copies the task map so one branch's instance cannot be
// mutated through an alias into another.
func cloneQuestInstances(quests map[string]quest.Instance) map[string]quest.Instance {
	if len(quests) == 0 {
		return nil
	}
	cloned := make(map[string]quest.Instance, len(quests))
	for id, instance := range quests {
		cloned[id] = cloneQuestInstance(instance)
	}
	return cloned
}

func cloneQuestInstance(instance quest.Instance) quest.Instance {
	if instance.CompletionEcho != nil {
		copy := *instance.CompletionEcho
		instance.CompletionEcho = &copy
	}
	if instance.Choices != nil {
		choices := make(map[string]quest.ChoiceResult, len(instance.Choices))
		for id, c := range instance.Choices {
			choices[id] = cloneQuestChoiceResult(c)
		}
		instance.Choices = choices
	}
	instance.CompletedObjectiveIDs = append([]string(nil), instance.CompletedObjectiveIDs...)
	if instance.SourceRefs != nil {
		refs := make(map[string]string, len(instance.SourceRefs))
		for key, value := range instance.SourceRefs {
			refs[key] = value
		}
		instance.SourceRefs = refs
	}
	return instance
}

// questFactFromRealmReward turns a realm grant into the fact a task objective
// can be satisfied by.
func questFactFromRealmReward(reward exploration.InstanceReward) quest.Fact {
	return quest.Fact{Kind: quest.EvidenceRealmReward, RefID: strings.TrimSpace(reward.ID)}
}

// questFactFromJourneyArrival turns a completed journey into its fact.
func questFactFromJourneyArrival(locationID string) quest.Fact {
	return quest.Fact{Kind: quest.EvidenceJourneyArrival, RefID: strings.TrimSpace(locationID)}
}

// questFactFromCombatOutcome turns a settled battle into its fact.
func questFactFromCombatOutcome(result cultivation.CombatResult) quest.Fact {
	return quest.Fact{Kind: quest.EvidenceCombatOutcome, RefID: string(result)}
}

// questCommittedFact is one fact the branch has really produced, with the ref
// that traces a tick back to the record it came from.
type questCommittedFact struct {
	Fact quest.Fact
	Ref  string
}

// questCommittedFacts lists the facts this branch has already committed, in a
// stable order.
//
// It reads the same projection the rest of the turn reads, so nothing here is a
// second record: a realm grant, a finished journey and a settled battle are
// facts the journal already wrote, and this only says which task objectives
// they answer.
func questCommittedFacts(projection *storyBranchProjection) []questCommittedFact {
	if projection == nil {
		return nil
	}
	var facts []questCommittedFact

	instanceIDs := make([]string, 0, len(projection.Realms))
	for id := range projection.Realms {
		instanceIDs = append(instanceIDs, id)
	}
	sort.Strings(instanceIDs)
	for _, instanceID := range instanceIDs {
		for _, granted := range projection.Realms[instanceID].Snapshot.GrantedRewards {
			facts = append(facts, questCommittedFact{
				Fact: questFactFromRealmReward(granted),
				Ref:  "realm:" + instanceID + ":" + granted.ID,
			})
		}
	}

	journeyIDs := make([]string, 0, len(projection.Journeys))
	for id := range projection.Journeys {
		journeyIDs = append(journeyIDs, id)
	}
	sort.Strings(journeyIDs)
	for _, journeyID := range journeyIDs {
		snapshot := projection.Journeys[journeyID]
		if snapshot.TotalMinutes <= 0 || snapshot.ElapsedMinutes < snapshot.TotalMinutes {
			continue
		}
		facts = append(facts, questCommittedFact{
			Fact: questFactFromJourneyArrival(snapshot.DestinationLocationID),
			Ref:  "journey:" + journeyID,
		})
	}

	sessionIDs := make([]string, 0, len(projection.Combats))
	for id := range projection.Combats {
		sessionIDs = append(sessionIDs, id)
	}
	sort.Strings(sessionIDs)
	for _, sessionID := range sessionIDs {
		session := projection.Combats[sessionID]
		if !session.Ended || session.Result == cultivation.ResultUnresolved {
			continue
		}
		facts = append(facts, questCommittedFact{
			Fact: questFactFromCombatOutcome(session.Result),
			Ref:  "combat:" + sessionID,
		})
	}
	return facts
}

// questCreditCommittedFacts ticks the objectives this branch has already
// satisfied, at the instant the task is taken on.
//
// §4.4 keeps acceptance the player's decision, so doing the work does not take
// the task on by itself. The same section also says a character who acted
// without having accepted still gets the real result — 不要求补点"领取". Those
// two rules contradict each other unless acceptance looks back once: ApplyFact
// only advances an accepted instance, so a player who carried the herb out of
// the cave before saying yes would accept into a task whose only objective the
// world had already satisfied and which could never tick again.
//
// The credit is a committed fact of this branch, not a report of one, and the
// acceptance instant is the instant the deadline was stamped from, so a task
// taken after its authored window still runs its full agreed time. Payment is
// untouched: settle remains the only thing that pays, and it pays once.
func questCreditCommittedFacts(definition quest.Definition, instance quest.Instance, projection *storyBranchProjection, atSeconds int64) quest.Instance {
	if projection == nil || instance.Status != quest.StatusAccepted {
		return instance
	}
	for _, fact := range questCommittedFacts(projection) {
		instance, _ = instance.ApplyFact(definition, fact.Fact, fact.Ref, atSeconds)
	}
	return instance
}

// questAdvanceEvents is the records an already-committed fact ticks.
//
// It is called from the commit that produced the fact, so the tick and the fact
// land in one transaction: a journey that arrived and a task that advanced
// cannot be separated by a crash, and a retry cannot tick twice.
//
// Only accepted tasks advance. An offer the player never took on stays an offer
// even if they happen to have done the work, because §4.4 keeps acceptance the
// player's decision.
//
// atSeconds is the committed instant the fact happened at, so a fact that lands
// after an agreed deadline ticks nothing: the work was real, but it was not done
// in time.
//
// turnID names the Turn whose action produced this fact, and it is carried onto
// the tick. A tick is parented after whatever the action left at the branch head,
// so an ancestor cut at that Turn excludes it by envelope; without the Turn id it
// would also fail to travel with the Turn it belongs to, and a fork would restore
// a battle's outcome but not the objective it completed.
func questAdvanceEvents(projection *storyBranchProjection, branchID, turnID string, fact quest.Fact, sourceRef, parentID, now string, atSeconds int64) ([]any, map[string]quest.Instance, error) {
	if projection == nil || len(projection.Quests) == 0 {
		return nil, nil, nil
	}
	taskIDs := make([]string, 0, len(projection.Quests))
	for id := range projection.Quests {
		taskIDs = append(taskIDs, id)
	}
	sort.Strings(taskIDs)
	var events []any
	updated := map[string]quest.Instance{}
	head := parentID
	for _, taskID := range taskIDs {
		instance := projection.Quests[taskID]
		if instance.Status != quest.StatusAccepted {
			continue
		}
		definition, ok := worldQuestDefinition(projection.worldContent(), instance.TaskID)
		if !ok || definition.Version != instance.TaskVersion {
			continue
		}
		next, added := instance.ApplyFact(definition, fact, sourceRef, atSeconds)
		if len(added) == 0 {
			continue
		}
		eventID := newID("qstrec")
		events = append(events, QuestRecordEvent{
			V: schemaVersion, Type: StoryEventTypeQuestRecord, ID: eventID,
			ParentID: head, BranchID: branchID, Ts: now,
			TurnID:      strings.TrimSpace(turnID),
			Cursor:      eventID,
			TaskID:      definition.ID,
			TaskVersion: definition.Version,
			Action:      QuestActionComplete,
			FactRef:     sourceRef,
			Instance:    next,
		})
		head = eventID
		updated[taskID] = next
	}
	return events, updated, nil
}

// questDeadlineEvents is the records one interval of committed time closes.
//
// §五 of 自制-并行事项与到期事件 asks the interval to settle every registered
// matter whose node it crossed, not only the one the player was looking at. This
// is that pass for tasks: each accepted instance whose agreed instant is now in
// the past is written as one expiry record, chained after the interval's own
// record. Nothing else changes — no objective is unticked and no reward is
// clawed back — because expiry closes an opportunity rather than rewriting what
// happened.
//
// It reads fromSeconds and toSeconds rather than only the later one so a
// deadline that was already past before this interval is not written a second
// time; the instance would have been expired by the interval that crossed it.
//
// turnID names the Turn whose interval crossed the deadline. The expiry is
// parented after that Turn's own clock record, so it needs the Turn id to travel
// with the Turn on a fork — otherwise a fork would restore the interval that
// spent the time but not the deadline it closed.
func questDeadlineEvents(projection *storyBranchProjection, branchID, turnID, parentID, now string, fromSeconds, toSeconds int64) []any {
	if projection == nil || len(projection.Quests) == 0 || toSeconds <= fromSeconds {
		return nil
	}
	taskIDs := make([]string, 0, len(projection.Quests))
	for id := range projection.Quests {
		taskIDs = append(taskIDs, id)
	}
	sort.Strings(taskIDs)
	var events []any
	head := parentID
	for _, taskID := range taskIDs {
		instance := projection.Quests[taskID]
		if instance.Status != quest.StatusAccepted {
			continue
		}
		definition, ok := worldQuestDefinition(projection.worldContent(), instance.TaskID)
		if !ok || definition.Version != instance.TaskVersion {
			continue
		}
		deadline, has := instance.Deadline()
		if !has || deadline <= fromSeconds || deadline > toSeconds {
			// A limit outside this interval is another interval's node. Writing it
			// here would claim the world changed at a moment it did not.
			continue
		}
		next, expired := instance.Expire(toSeconds)
		if !expired {
			continue
		}
		eventID := newID("qstrec")
		events = append(events, QuestRecordEvent{
			V: schemaVersion, Type: StoryEventTypeQuestRecord, ID: eventID,
			ParentID: head, BranchID: branchID, Ts: now,
			TurnID:      strings.TrimSpace(turnID),
			Cursor:      eventID,
			TaskID:      definition.ID,
			TaskVersion: definition.Version,
			Action:      QuestActionExpire,
			Instance:    next,
		})
		head = eventID
	}
	return events
}

// QuestStateView is one task as the panel reads it.
type QuestStateView struct {
	CompletionEcho *quest.EchoResult `json:"completion_echo,omitempty"`
	// FollowUps are narrator-only actions for a present source, not disclosed outcomes.
	FollowUps    []QuestFollowUp               `json:"-"`
	Choices      []quest.Choice                `json:"choices,omitempty"`
	Resolutions  map[string]quest.ChoiceResult `json:"resolutions,omitempty"`
	Consequences []string                      `json:"consequences,omitempty"`
	TaskID       string                        `json:"task_id"`
	TaskVersion  string                        `json:"task_version"`
	Kind         string                        `json:"kind"`
	Title        string                        `json:"title"`
	Brief        string                        `json:"brief"`
	Giver        string                        `json:"giver,omitempty"`
	Status       string                        `json:"status"`
	// Objectives are the task's own wording plus whether each is satisfied, so
	// the panel never invents a goal or a tick.
	Objectives []QuestObjectiveView `json:"objectives"`
	// Rewards are what the task promises, stated before acceptance.
	Rewards []QuestRewardView `json:"rewards"`
	// Settled reports that the payment was really handed over.
	Settled    bool   `json:"settled"`
	AcceptedAt string `json:"accepted_at,omitempty"`
	SettledAt  string `json:"settled_at,omitempty"`
	// DeadlineSeconds is the absolute formal instant the agreement runs to, zero
	// when none was agreed. It is the same number the arithmetic expires on, so
	// the panel cannot count down to a moment the rules would not honour.
	DeadlineSeconds int64 `json:"deadline_seconds,omitempty"`
	// DeadlineText is the same instant in the story's own wording, empty when no
	// deadline was agreed. §五 forbids inventing a countdown where there is none,
	// so an empty value is the honest answer and the panel says 未约定期限.
	DeadlineText string `json:"deadline_text,omitempty"`
	// RemainingSeconds is the committed time left at the instant this view was
	// read. It is derived, not stored, and is negative once the instant passed.
	RemainingSeconds int64 `json:"remaining_seconds,omitempty"`
	// Expired reports that the deadline passed before the task was completed.
	Expired bool `json:"expired,omitempty"`
}

// QuestObjectiveView is one objective and its state.
type QuestObjectiveView struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Completed   bool   `json:"completed"`
	// SourceRef is the committed fact that satisfied it, empty while open.
	SourceRef string `json:"source_ref,omitempty"`
}

// QuestRewardView is one promised payment in the story's own wording.
type QuestRewardView struct {
	Kind     string `json:"kind"`
	RefID    string `json:"ref_id"`
	NameKey  string `json:"name_key,omitempty"`
	Quantity int64  `json:"quantity"`
}

// QuestState is one branch's tasks plus the authored tasks it could still take
// on.
type QuestState struct {
	CurrentLocation string           `json:"current_location,omitempty"`
	Tasks           []QuestStateView `json:"tasks"`
	// Available are opportunities the player knows through an authored public source.
	Available []QuestOption `json:"available"`
	// OfferCandidates are narrator-only possibilities from an encountered giver.
	// They are never serialized to the player API as already-known opportunities.
	OfferCandidates []QuestOption `json:"-"`
}

// QuestOption is one authored task the entry can offer.
type QuestOption struct {
	Choices     []quest.Choice `json:"choices,omitempty"`
	TaskID      string         `json:"task_id"`
	TaskVersion string         `json:"task_version"`
	Kind        string         `json:"kind"`
	Title       string         `json:"title"`
	Brief       string         `json:"brief"`
	Giver       string         `json:"giver,omitempty"`
	// DeadlineSeconds is the interval the offer runs once accepted, zero when
	// none is agreed. It is the authored duration rather than an instant,
	// because the limit starts at acceptance and the player has not accepted yet.
	DeadlineSeconds int64 `json:"deadline_seconds,omitempty"`
}

// QuestStateFor reads the branch's tasks without writing anything.
func (s *Store) QuestStateFor(storyID, branchID string) (QuestState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return QuestState{}, err
	}
	defer release()
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return QuestState{}, err
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return QuestState{}, err
	}
	meta := normalizeStoryMeta(handle.projection.Meta)
	clock, err := branchStoryClock(projection, DefaultStoryContextActorID,
		actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState))
	if err != nil {
		return QuestState{}, err
	}
	return questStateFrom(meta, projection, clock), nil
}

// questStateFrom builds the task view from a projection the caller has already
// read.
//
// It is split out so the main story's World Actions page can report the same
// tasks from one journal read, with the same eligibility answer the entry and
// the commit use. A second copy of this logic would be a second answer to "may
// this task be offered here", which is exactly what §4.10 forbids.
func questStateFrom(meta StoryMeta, projection *storyBranchProjection, clock StoryClock) QuestState {
	state := QuestState{
		CurrentLocation: storySceneText(projection.State, storySceneLocationField),
		Tasks:           []QuestStateView{},
		Available:       []QuestOption{},
	}
	ids := make([]string, 0, len(projection.Quests))
	for id := range projection.Quests {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		instance := projection.Quests[id]
		definition, ok := worldQuestDefinition(worldContentForMeta(meta), instance.TaskID)
		if !ok {
			continue
		}
		view := questStateView(cultivationRulesForMeta(meta),
			definition, instance, clock)
		view.FollowUps = questFollowUps(projection, definition, instance)
		state.Tasks = append(state.Tasks, view)
	}
	for _, definition := range worldQuestDefinitions(worldContentForMeta(meta)) {
		if _, taken := projection.Quests[definition.ID]; taken {
			continue
		}
		// The commit asks the same question, so a task shown here is one that can
		// really be accepted.
		if !questDefinitionEligible(meta, definition) || !questMembershipGateOpen(projection, definition) || !questEchoGateOpen(projection, definition.RequiresEcho) {
			continue
		}
		if questPubliclyKnown(projection, definition) {
			state.Available = append(state.Available, questOption(definition))
		} else if questGiverPresent(projection, definition) {
			state.OfferCandidates = append(state.OfferCandidates, questOption(definition))
		}
	}
	return state
}

// questStateView renders one task from its authored wording and its real state.
//
// clock is the branch's committed instant. It is only used to say how much time
// is left on an agreed limit; the task's own stored deadline stays the number
// the rules expire on, so opening the panel cannot change anything.
func questStateView(cultivationRules *cultivation.Rules, definition quest.Definition, instance quest.Instance, clock StoryClock) QuestStateView {
	completed := make(map[string]bool, len(instance.CompletedObjectiveIDs))
	for _, id := range instance.CompletedObjectiveIDs {
		completed[id] = true
	}
	objectives := make([]QuestObjectiveView, 0, len(definition.Objectives))
	for _, objective := range definition.Objectives {
		objectives = append(objectives, QuestObjectiveView{
			ID: objective.ID, Description: objective.Description,
			Completed: completed[objective.ID],
			SourceRef: instance.SourceRefs[objective.ID],
		})
	}
	rewards := make([]QuestRewardView, 0, len(definition.Rewards))
	for _, reward := range definition.Rewards {
		view := QuestRewardView{Kind: string(reward.Kind), RefID: reward.RefID, Quantity: reward.Quantity}
		if reward.Kind == quest.RewardItem {
			view.NameKey = StoryItemNameKey(cultivationRules,
				reward.RefID)
		}
		rewards = append(rewards, view)
	}
	view := QuestStateView{
		Choices: quest.VisibleChoices(definition, instance.Choices), Resolutions: cloneQuestInstance(instance).Choices, Consequences: questConsequences(definition, instance), CompletionEcho: cloneQuestInstance(instance).CompletionEcho,
		TaskID: definition.ID, TaskVersion: definition.Version,
		Kind: string(definition.Kind), Title: definition.Title, Brief: definition.Brief,
		Giver: definition.Giver, Status: string(instance.Status),
		Objectives: objectives, Rewards: rewards,
		Settled:    instance.SettledRewards,
		AcceptedAt: instance.AcceptedAt, SettledAt: instance.SettledAt,
	}
	if deadline, has := instance.Deadline(); has {
		view.DeadlineSeconds = deadline
		view.RemainingSeconds = deadline - clock.Seconds
		// The text is derived from the same instant the arithmetic uses, so a
		// reader and the rules cannot disagree about how long is left.
		switch {
		case deadline <= clock.Seconds:
			view.DeadlineText = "已到期"
		default:
			if elapsed := DescribeElapsed(deadline - clock.Seconds); elapsed != "" {
				view.DeadlineText = "约 " + elapsed + " 后"
			}
		}
	}
	view.Expired = instance.Status == quest.StatusExpired ||
		(instance.Status == quest.StatusAccepted && instance.ExpiredAt(clock.Seconds))
	return view
}

func cloneQuestChoiceResult(c quest.ChoiceResult) quest.ChoiceResult {
	if c.Previous != nil {
		previous := cloneQuestChoiceResult(*c.Previous)
		c.Previous = &previous
	}
	return c
}
