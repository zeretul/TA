package interactive

import (
	"fmt"
	"strings"
	interactivestate "ta/internal/interactive/state"
	"time"
)

func (s *Store) AppendStateDelta(storyID string, req AppendStateDeltaRequest) (StateDeltaEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return StateDeltaEvent{}, err
	}
	defer releaseStory()
	if len(req.Ops) == 0 && len(req.ActorOps) == 0 {
		return StateDeltaEvent{}, fmt.Errorf("状态变化不能为空")
	}

	meta, lines, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return StateDeltaEvent{}, err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	if _, ok := meta.Branches[branchID]; !ok {
		return StateDeltaEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	parentID := strings.TrimSpace(req.ParentID)
	if parentID == "" {
		parentID = latestLogicalTurnID(meta, lines, branchID)
	}
	if parentID == "" {
		return StateDeltaEvent{}, fmt.Errorf("状态变化缺少所属回合")
	}
	if err := requireLatestLogicalTurn(meta, lines, branchID, parentID); err != nil {
		return StateDeltaEvent{}, err
	}
	ops := normalizeStateOps(req.Ops)
	actorOps := normalizeActorStateOps(req.ActorOps)
	if len(ops) == 0 && len(actorOps) == 0 {
		return StateDeltaEvent{}, fmt.Errorf("状态变化不能为空")
	}
	for _, op := range ops {
		if err := validateStateOp(op); err != nil {
			return StateDeltaEvent{}, err
		}
	}
	for _, op := range actorOps {
		if err := validateActorStateOp(op); err != nil {
			return StateDeltaEvent{}, err
		}
	}
	projected, err := projectStoryEventOverlays(lines)
	if err != nil {
		return StateDeltaEvent{}, err
	}
	record, ok := eventsByID(projected)[parentID]
	if !ok || record.Envelope.Type != StoryEventTypeTurn {
		return StateDeltaEvent{}, fmt.Errorf("状态变化所属回合不存在: %s", parentID)
	}
	var turn TurnEvent
	if err := mapToStruct(record.Raw, &turn); err != nil {
		return StateDeltaEvent{}, err
	}
	nextOps := append([]interactivestate.Op(nil), ops...)
	nextActorOps := append([]ActorStateOp(nil), actorOps...)
	if turn.StateDelta != nil {
		nextOps = append(append([]interactivestate.Op(nil), turn.StateDelta.Ops...), nextOps...)
		nextActorOps = append(append([]ActorStateOp(nil), turn.StateDelta.ActorOps...), nextActorOps...)
	}
	delta := newStateDeltaWithActorOps(nextOps, nextActorOps)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	revision := TurnStateRevisedEvent{
		V: schemaVersion, Type: StoryEventTypeTurnStateRevised, ID: newID("tsr"),
		ParentID: parentID, BranchID: branchID, Ts: now, TurnID: parentID,
		StateDelta: &delta, StateStatus: "ready", Reason: "state_settled",
	}
	event := newStateDeltaEventWithActorOps(revision.ID, parentID, branchID, now, ops, actorOps)
	// A settlement written outside a turn is dated to where the branch stands, so
	// an Actor it touches is not left looking older or newer than it is.
	if atSeconds, atEvent := stateDeltaStamp(s.branchClockSecondsLocked(storyID, branchID), revision.ID); atEvent != "" && len(actorOps) > 0 {
		delta.WrittenAtSeconds = atSeconds
		delta.WrittenAtEvent = atEvent
		event.WrittenAtSeconds = atSeconds
		event.WrittenAtEvent = atEvent
	}
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, revision); err != nil {
		return StateDeltaEvent{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return StateDeltaEvent{}, err
	}
	return event, nil
}

// branchClockSecondsLocked reads where the branch stands on its formal clock.
//
// A settlement that is not part of a turn still writes Actors, and a record it
// touched must not read as older or newer than the story. The caller already
// holds the store lock.
func (s *Store) branchClockSecondsLocked(storyID, branchID string) int64 {
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return 0
	}
	return projection.clockSeconds()
}

func (s *Store) MarkStateFailed(storyID string, req MarkStateFailedRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return err
	}
	defer releaseStory()

	meta, lines, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	if _, ok := meta.Branches[branchID]; !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	parentID := strings.TrimSpace(req.ParentID)
	if parentID == "" {
		parentID = latestLogicalTurnID(meta, lines, branchID)
	}
	if parentID == "" {
		return fmt.Errorf("状态失败标记缺少所属回合")
	}
	if err := requireLatestLogicalTurn(meta, lines, branchID, parentID); err != nil {
		return err
	}
	errText := strings.TrimSpace(req.Error)
	if errText == "" {
		errText = "状态生成失败"
	}
	projected, err := projectStoryEventOverlays(lines)
	if err != nil {
		return err
	}
	record, ok := eventsByID(projected)[parentID]
	if !ok || record.Envelope.Type != StoryEventTypeTurn {
		return fmt.Errorf("状态失败标记所属回合不存在: %s", parentID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	revision := TurnStateRevisedEvent{
		V: schemaVersion, Type: StoryEventTypeTurnStateRevised, ID: newID("tsr"),
		ParentID: parentID, BranchID: branchID, Ts: now, TurnID: parentID,
		StateStatus: "failed", StateError: errText, Reason: "state_failed",
	}
	if err := s.appendStoryTransactionLocked(storyID, meta, revision); err != nil {
		return err
	}
	return s.syncStorySummaryLocked(storyID)
}

func (s *Store) RerollRuleResolution(storyID, resolutionID string, req RuleResolutionRerollRequest) (RuleResolution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return RuleResolution{}, err
	}
	defer releaseStory()

	resolutionID = strings.TrimSpace(resolutionID)
	if resolutionID == "" {
		return RuleResolution{}, fmt.Errorf("规则结算 ID 不能为空")
	}
	meta, lines, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return RuleResolution{}, err
	}
	branchID, branch, err := resolveBranch(meta, req.BranchID)
	if err != nil {
		return RuleResolution{}, err
	}
	turnID := strings.TrimSpace(req.TurnID)
	if turnID == "" {
		turnID = latestLogicalTurnID(meta, lines, branchID)
	}
	if err := requireLatestLogicalTurn(meta, lines, branchID, turnID); err != nil {
		return RuleResolution{}, err
	}
	projected, err := projectStoryEventOverlays(lines)
	if err != nil {
		return RuleResolution{}, err
	}
	path, _, err := walkEventPath(branch.Head, eventsByID(projected))
	if err != nil {
		return RuleResolution{}, err
	}
	var target TurnEvent
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			continue
		}
		if turn.ID != turnID {
			continue
		}
		if turn.RuleResolution != nil && turn.RuleResolution.ID == resolutionID {
			target = turn
			break
		}
	}
	if target.ID == "" {
		return RuleResolution{}, fmt.Errorf("当前分支路径中未找到规则结算: %s", resolutionID)
	}
	request := NormalizeTurnCheckRequest(target.RuleResolution.Request)
	branchProjection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return RuleResolution{}, err
	}
	state := cloneStoryState(branchProjection.StateBeforeLatest)
	director := s.storyDirectorForMeta(meta)
	actorState := actorStateSystemFromSnapshot(meta.ActorStateSchema, director.ActorState)
	applyLegacyActorStateAliases(state, meta.ActorStateSchema)
	next, err := ResolveTurnRulesWithStorySettings(storyID, branchID, state, director, meta.CheckSettings, request)
	if err != nil {
		return RuleResolution{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	next.CreatedAt = now
	next.ID = newID("rr")
	// Rule consumption applies its operations to the live state in place, so the
	// projection base is snapshotted before it runs and the rerolled turn's full
	// operation set is replayed onto that snapshot. Settling against the live map
	// instead would apply the new rule operations a second time.
	settlementBase := cloneStoryState(state)
	ruleOps, ruleActorOps := applyRuleStateConsumptionV2(state, actorState, target.ID, &next, director.Strategy.RuleStateConsumptionMode)
	terminalOutcome := terminalOutcomeFromRuleResolution(next, target.ID, target.Narrative)
	existingOps := []interactivestate.Op{}
	existingActorOps := []ActorStateOp{}
	if target.StateDelta != nil {
		existingOps = append(existingOps, target.StateDelta.Ops...)
		existingActorOps = append(existingActorOps, target.StateDelta.ActorOps...)
	}
	nextOps := append(removeRuleResolutionStateOps(existingOps, target.RuleResolution.ID), ruleOps...)
	nextActorOps := append(removeRuleResolutionActorOps(existingActorOps, target.RuleResolution.ID), ruleActorOps...)
	// The settlement the original turn committed is derived from the rules
	// position that turn ended at, so a reroll invalidates it the same way it
	// invalidates the rule ops it was derived from. Keeping the old numbers
	// beside the new resolution would leave the panel showing attributes that
	// belong to a stage the rerolled turn no longer reaches.
	nextActorOps = removeCultivationSettlementActorOps(nextActorOps)
	// The authorised cultivation interval was applied to the position the original
	// rule outcome left the character in, so it is invalidated by the reroll for
	// the same reason the settlement is: re-deriving it from the replacement
	// outcome is the only way the stage the panel shows is one the rerolled turn
	// actually reaches. The draw itself is bound to the command identity, so the
	// re-derivation re-uses the same die rather than rolling a second time.
	nextActorOps = removeCultivationAdvanceActorOps(nextActorOps)
	// A rest interval is invalidated by the reroll for the same reason: its pool
	// and meridian writes were settled against the position the superseded
	// outcome left the character in. The interval itself is not persisted on the
	// Turn — TurnResult's modules are rules-owned writes and deliberately absent
	// from the stored Turn — so it is re-derived from the side record below,
	// which is where the spent time really lives.
	nextActorOps = removeSafeRecoveryActorOps(nextActorOps)
	// A craft is invalidated by the reroll for the same reason a rest is: its pack
	// write was settled against the position the superseded outcome left the
	// character in, and keeping it beside the new resolution would leave a product
	// in the pack that the rerolled turn no longer produces. The method and the
	// interval are not persisted on the Turn either, so the side record is again
	// the only honest source — see the block below.
	nextActorOps = removeCraftActorOps(nextActorOps)
	// A rerolled turn's own cultivation module is not persisted — TurnResult's
	// cultivation, skills, techniques, items and trade are committed as
	// rules-owned writes and deliberately absent from the stored Turn — so the
	// interval cannot be re-read from the turn. The side record is where the
	// interval really lives, and it is the only honest source for it.
	//
	// The growth is therefore re-derived from that record's conditions and the
	// branch position the record started from. The time is not: the world already
	// spent it, and a reroll is a second reading of the same events rather than a
	// rewind of the calendar.
	var rerollClock *CultivationRecordEvent
	rerollSource, hasRerollSource := cultivationRecordForTurn(path, branchID, target.ID)
	if hasRerollSource && rerollSource.Record != nil {
		rerollBase := cultivationRerollProjection(branchProjection, target.ID, rerollSource)
		advanceBase := cloneStoryState(settlementBase)
		for _, op := range nextOps {
			applyStateOp(advanceBase, op)
		}
		for _, op := range nextActorOps {
			applyActorStateOp(advanceBase, op)
		}
		actorID := normalizeStatePanelActorID(rerollSource.Record.Activity.ActorID)
		if actorID == "" {
			actorID = DefaultActorID
		}
		elapsed := rerollSource.Record.ClockAfter.Seconds - rerollSource.Record.ClockBefore.Seconds
		days := float64(elapsed) / (24 * 60 * 60)
		input := rerollSource.Record.Activity.Conditions.inputFor(days)
		if advanceOps, advance, ok := advanceXiuxianCultivation(advanceBase, actorState, actorID, input, xiuxianFormalFactsFromBranch(rerollBase, actorID), turnIdentity(target), target.ID); ok {
			nextActorOps = append(nextActorOps, advanceOps...)
			// The reroll replaces the rule outcome the turn's clock record was
			// derived from, so that record's growth must be re-derived with it. The
			// replacement keeps the original window and carries only the new
			// outcome, and it reuses the superseded activity's ID so the projection
			// overwrites it rather than keeping both.
			if replaced := cultivationRerollClockRecord(rerollBase, actorState, target, advance, rerollSource, now); replaced != nil {
				rerollClock = replaced
			}
		}
	}
	// Learned abilities, item acquisition and trade are independent committed
	// facts. Their transient module inputs are deliberately absent from a stored
	// TurnResult (json:"-"). In that canonical case retain the accepted ActorOps;
	// removing them would erase paid goods and progress while retaining elapsed
	// time. Only replace these ops when the matching input really exists.
	// A learned technique is not a consequence of the rule outcome being rerolled:
	// the same learning re-applied to the replacement position is idempotent, and
	// discarding it would let a reroll silently take away a technique the story
	// already had the character earn.
	if target.TurnResult != nil && target.TurnResult.Skills != nil && !target.TurnResult.Skills.Empty() {
		nextActorOps = removeSkillLearningActorOps(nextActorOps)
		learningBase := cloneStoryState(settlementBase)
		for _, op := range nextOps {
			applyStateOp(learningBase, op)
		}
		for _, op := range nextActorOps {
			applyActorStateOp(learningBase, op)
		}
		learningActorID := normalizeStatePanelActorID(target.TurnResult.Skills.ActorID)
		if learningActorID == "" {
			learningActorID = DefaultActorID
		}
		if learningOps, _, ok := xiuxianSkillLearningOps(learningBase, actorState, learningActorID, *target.TurnResult.Skills, target.ID); ok {
			nextActorOps = append(nextActorOps, learningOps...)
		}
	}
	// Methods are re-derived like skills: mastery is a fact about the character,
	// and re-applying the same module to the replacement position is idempotent.
	if target.TurnResult != nil && target.TurnResult.Techniques != nil && !target.TurnResult.Techniques.Empty() {
		nextActorOps = removeTechniqueLearningActorOps(nextActorOps)
		techniqueBase := cloneStoryState(settlementBase)
		for _, op := range nextOps {
			applyStateOp(techniqueBase, op)
		}
		for _, op := range nextActorOps {
			applyActorStateOp(techniqueBase, op)
		}
		techniqueActorID := normalizeStatePanelActorID(target.TurnResult.Techniques.ActorID)
		if techniqueActorID == "" {
			techniqueActorID = DefaultActorID
		}
		if techniqueOps, _, ok := xiuxianTechniqueOps(techniqueBase, actorState, techniqueActorID, *target.TurnResult.Techniques, target.ID); ok {
			nextActorOps = append(nextActorOps, techniqueOps...)
		}
	}
	// Items are re-derived like skills and methods: what the character obtained or
	// equipped is a fact about them, and re-applying the same module to the
	// replacement position is idempotent.
	if target.TurnResult != nil && target.TurnResult.Items != nil && !target.TurnResult.Items.Empty() {
		nextActorOps = removeItemChangeActorOps(nextActorOps)
		itemBase := cloneStoryState(settlementBase)
		for _, op := range nextOps {
			applyStateOp(itemBase, op)
		}
		for _, op := range nextActorOps {
			applyActorStateOp(itemBase, op)
		}
		itemActorID := normalizeStatePanelActorID(target.TurnResult.Items.ActorID)
		if itemActorID == "" {
			itemActorID = DefaultActorID
		}
		if itemOps, _, ok := xiuxianItemOps(itemBase, actorState, itemActorID, withoutPaidAcquisitions(*target.TurnResult.Items, target.TurnResult.Trade), target.ID); ok {
			nextActorOps = append(nextActorOps, itemOps...)
		}
	}
	// Trade is re-derived the same way, and after items because a copy obtained
	// this turn can be the copy that is sold. Both the purse write and the pack
	// write carry the trade source kind, so dropping them together is what keeps
	// a rerolled turn from deducting twice or transferring goods twice.
	if target.TurnResult != nil && target.TurnResult.Trade != nil && !target.TurnResult.Trade.Empty() {
		nextActorOps = removeTradeActorOps(nextActorOps)
		tradeBase := cloneStoryState(settlementBase)
		for _, op := range nextOps {
			applyStateOp(tradeBase, op)
		}
		for _, op := range nextActorOps {
			applyActorStateOp(tradeBase, op)
		}
		tradeActorID := normalizeStatePanelActorID(target.TurnResult.Trade.ActorID)
		if tradeActorID == "" {
			tradeActorID = DefaultActorID
		}
		if tradeOps, _, ok := xiuxianTradeOps(tradeBase, actorState, tradeActorID, *target.TurnResult.Trade, target.ID); ok {
			nextActorOps = append(nextActorOps, tradeOps...)
		}
	}
	// A rest interval is re-derived from its side record, which is the only place
	// the spent time lives: TurnResult's rest module is a rules-owned write and is
	// deliberately absent from the stored Turn. The interval's pool and meridian
	// writes are recomputed against the replacement position, and the record is
	// republished with the superseded window so the calendar is not spent twice.
	var rerollRecovery *RecoveryRecordEvent
	recoverySource, hasRecoverySource := recoveryRecordForTurn(path, branchID, target.ID)
	if hasRecoverySource && recoverySource.Record != nil {
		recoveryBase := recoveryRerollProjection(branchProjection, target.ID, recoverySource)
		restBase := cloneStoryState(settlementBase)
		for _, op := range nextOps {
			applyStateOp(restBase, op)
		}
		for _, op := range nextActorOps {
			applyActorStateOp(restBase, op)
		}
		actorID := normalizeStatePanelActorID(recoverySource.Record.Activity.ActorID)
		if actorID == "" {
			actorID = DefaultActorID
		}
		if outcome, ok := rerollTurnRest(recoveryBase, actorState, restBase, storyID, branchID, actorID, recoverySource, target.ID); ok {
			nextActorOps = append(nextActorOps, outcome.ops...)
			rerollRecovery = recoveryRerollRecord(recoveryBase, target, outcome, recoverySource, now)
		}
	}
	// A craft is re-derived from its side record for the same reason a rest is:
	// the module's method and interval are deliberately absent from the stored
	// Turn. The batch is recomputed against the position the replacement turn ends
	// at, and the record is republished with the superseded window so the calendar
	// is not spent twice. The draw is bound to the turn identity, so a reroll reads
	// the same die rather than rolling a second one.
	var rerollCraft *CraftRecordEvent
	craftSource, craftSourceID, hasCraftSource := craftRecordForTurn(path, branchID, target.ID)
	if hasCraftSource {
		craftBase := cloneStoryState(settlementBase)
		for _, op := range nextOps {
			applyStateOp(craftBase, op)
		}
		for _, op := range nextActorOps {
			applyActorStateOp(craftBase, op)
		}
		if craftOps := craftRerollActorOps(actorState, craftBase, target, craftSource); len(craftOps) > 0 {
			nextActorOps = append(nextActorOps, craftOps...)
			// Re-derive the record from the same pre-craft state, not the writes
			// just calculated (which already reserve inputs and advance progress).
			rerollCraft = craftRerollRecord(actorState, craftBase, target, craftSource, craftSourceID, now)
		}
	}
	nextActorOps = append(nextActorOps, settleXiuxianTurnState(settlementBase, actorState, nextOps, nextActorOps, target.ID)...)
	for _, op := range nextOps {
		if err := validateStateOp(op); err != nil {
			return RuleResolution{}, err
		}
	}
	for _, op := range nextActorOps {
		if err := validateActorStateOp(op); err != nil {
			return RuleResolution{}, err
		}
	}
	revision := TurnStateRevisedEvent{
		V: schemaVersion, Type: StoryEventTypeTurnStateRevised, ID: newID("tsr"),
		ParentID: target.ID, BranchID: branchID, Ts: now, TurnID: target.ID,
		RuleResolution: &next, Reason: "rule_rerolled",
	}
	if len(nextOps) > 0 || len(nextActorOps) > 0 {
		delta := newStateDeltaWithActorOps(nextOps, nextActorOps)
		revision.StateDelta = &delta
		revision.StateStatus = "ready"
	} else {
		revision.ClearStateDelta = true
		revision.StateStatus = "pending"
	}
	// A reroll republishes the same window rather than spending a new one, so the
	// branch already stands at the instant the replacement ends at. Stamping the
	// replacement with it keeps an Actor it touched from reading as older than the
	// story it belongs to.
	rerollStampSeconds, rerollStampEvent := stateDeltaStamp(branchProjection.clockSeconds(), revision.ID)
	if len(nextActorOps) > 0 && rerollStampEvent != "" && revision.StateDelta != nil {
		revision.StateDelta.WrittenAtSeconds = rerollStampSeconds
		revision.StateDelta.WrittenAtEvent = rerollStampEvent
	}
	if terminalOutcome != nil {
		revision.TerminalOutcome = terminalOutcome
	} else {
		revision.ClearTerminalOutcome = true
	}
	meta.UpdatedAt = now
	events := []any{revision}
	headID := branch.Head
	if rerollClock != nil {
		events = append(events, *rerollClock)
		headID = rerollClock.ID
		// The superseded interval's expiries were chained onto the record this one
		// replaces, so they leave the branch's path with it. Re-emitting the pass
		// from the replacement's own window puts them back. The task map is rewound
		// to the position before that interval first, because the live projection
		// already carries the expiry and expiring it again would produce nothing.
		//
		// The growth watermark is rewound the same way and for the same reason: the
		// live projection already reports this window as settled, so the rules would
		// answer 重复 and the branch would lose the growth along with the record it
		// replaced.
		base := *branchProjection
		base.Quests = intervalRewindQuests(branchProjection, rerollClock.ClockBefore, rerollClock.ClockAfter)
		base.NPCGrowth = intervalRewindNPCGrowth(branchProjection, branchID, rerollClock.ClockBefore, rerollClock.ClockAfter)
		closed, closedHead := storyIntervalEvents(&base, actorState, branchID, headID, now, target.ID, rerollClock.Activity.ActorID, rerollClock.ClockBefore, rerollClock.ClockAfter)
		events = append(events, closed...)
		headID = closedHead
	}
	if rerollRecovery != nil {
		// The republished rest record is chained onto the record it replaces so the
		// projection overwrites one activity rather than keeping both.
		events = append(events, *rerollRecovery)
		headID = rerollRecovery.ID
		base := *branchProjection
		base.Quests = intervalRewindQuests(branchProjection, rerollRecovery.ClockBefore, rerollRecovery.ClockAfter)
		base.NPCGrowth = intervalRewindNPCGrowth(branchProjection, branchID, rerollRecovery.ClockBefore, rerollRecovery.ClockAfter)
		closed, closedHead := storyIntervalEvents(&base, actorState, branchID, headID, now, target.ID, rerollRecovery.Activity.ActorID, rerollRecovery.ClockBefore, rerollRecovery.ClockAfter)
		events = append(events, closed...)
		headID = closedHead
	}
	if rerollCraft != nil {
		// The republished craft record is chained onto the record it replaces so the
		// projection overwrites one batch rather than keeping both.
		events = append(events, *rerollCraft)
		headID = rerollCraft.ID
		base := *branchProjection
		base.Quests = intervalRewindQuests(branchProjection, rerollCraft.ClockBefore, rerollCraft.ClockAfter)
		base.NPCGrowth = intervalRewindNPCGrowth(branchProjection, branchID, rerollCraft.ClockBefore, rerollCraft.ClockAfter)
		closed, closedHead := storyIntervalEvents(&base, actorState, branchID, headID, now, target.ID, "", rerollCraft.ClockBefore, rerollCraft.ClockAfter)
		events = append(events, closed...)
		headID = closedHead
	}
	if headID != branch.Head {
		nextBranch := branch
		nextBranch.Head = headID
		meta.Branches[branchID] = nextBranch
	}
	if err := s.appendStoryTransactionLocked(storyID, meta, events...); err != nil {
		return RuleResolution{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return RuleResolution{}, err
	}
	return next, nil
}
