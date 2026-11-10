package interactive

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"ta/internal/cultivation"
	"ta/internal/storytime"
)

// TurnRestInput is the model-facing rest module of one story turn.
//
// It is the structured form of a player command like "在客栈休养一天" or "就地调息
// 半个时辰". §三 of the interface boundary requires rest and regulation to happen
// through main-story narration and action, so the turn itself has to be able to
// spend the interval the prose describes; without this module the panel would be
// the only way to rest, and removing it would leave "养伤一天" as free text with no
// formal result.
//
// The module carries the interval and the one pool it is meant to restore. It
// carries no resulting pool, no eligibility and no maximum: whether the character
// may rest at all, and how much the interval restores, are the rules' answers,
// read from committed state. That is the same split TurnCultivationInput makes.
type TurnRestInput struct {
	// ActorID is who rests. Omit for the protagonist.
	ActorID string `json:"actor_id,omitempty" jsonschema_description:"The character who rests or regulates. Omit for the protagonist."`
	// Mode names the one pool this interval restores. Rest never also
	// meditates, and meditation never heals health.
	Mode string `json:"mode,omitempty" jsonschema:"enum=health_rest,enum=safe_meditation" jsonschema_description:"health_rest for ordinary rest that restores health; safe_meditation for safe regulation that restores mana. Rest never also meditates and meditation never heals health, so one turn carries one mode."`
	// DurationSeconds is the real in-fiction interval the player asked for.
	DurationSeconds int64 `json:"duration_seconds,omitempty" jsonschema_description:"Real in-fiction seconds of rest or regulation this turn, from the player's own command. Never invent a duration the player did not ask for."`
}

// Empty reports whether the module carries nothing to apply.
func (input TurnRestInput) Empty() bool {
	return input.DurationSeconds <= 0 && strings.TrimSpace(input.Mode) == ""
}

// turnRestPoolOwner names the one pool a rest module in this turn restores.
//
// It answers the question the state_changes compiler asks before the turn is
// committed: which field the rules are about to author, and for whom. It is
// deliberately narrower than the module's own acceptance test — a mode the
// rules do not know, or an interval of no length, names nobody, because such a
// module will be rejected and the pool must stay writable by the prose.
func turnRestPoolOwner(input *TurnRestInput) (string, string) {
	if input == nil || input.DurationSeconds <= 0 {
		return "", ""
	}
	fieldID, err := recoveryPoolField(cultivation.RecoveryMode(strings.TrimSpace(input.Mode)))
	if err != nil {
		return "", ""
	}
	actorID := normalizeStatePanelActorID(input.ActorID)
	if actorID == "" {
		actorID = DefaultActorID
	}
	return actorID, fieldID
}

// cloneTurnRestInput copies a rest module so an accepted one can be retained in a
// draft without the caller's value being aliased.
func cloneTurnRestInput(input *TurnRestInput) *TurnRestInput {
	if input == nil {
		return nil
	}
	cloned := *input
	return &cloned
}

// validateTurnRestModule rejects a rest module the rules could not act on.
//
// It checks shape, not outcome. Whether the character is in a state to rest is a
// domain answer read from committed facts, and it is answered against the branch
// while the turn commits rather than guessed here.
func validateTurnRestModule(input *TurnRestInput) *TurnSubmissionDiagnostic {
	if input == nil || input.Empty() {
		return nil
	}
	mode := cultivation.RecoveryMode(strings.TrimSpace(input.Mode))
	if !validRecoveryMode(mode) {
		return newTurnSubmissionDiagnostic(
			TurnSubmissionModuleRest, nil, TurnSubmissionDiagnosticInvalidModule,
			"/rest/mode", "health_rest or safe_meditation", strings.TrimSpace(input.Mode),
			"rest.mode must name one pool the rules restore: health_rest or safe_meditation.",
		)
	}
	if input.DurationSeconds <= 0 {
		return newTurnSubmissionDiagnostic(
			TurnSubmissionModuleRest, nil, TurnSubmissionDiagnosticInvalidModule,
			"/rest/duration_seconds", "a positive number of in-fiction seconds", fmt.Sprint(input.DurationSeconds),
			"rest.duration_seconds must be the positive interval the player actually asked to spend.",
		)
	}
	return nil
}

// pausedRecoveryActivity returns the interrupted rest of one mode this actor left
// unfinished, if any.
//
// A rest a real boundary stopped stays on the branch with its remainder owed, and
// the next rest of the same mode continues it rather than opening a second
// activity. The latest origin wins, because that is the intent most recently
// interrupted.
func pausedRecoveryActivity(branch *storyBranchProjection, actorID string, mode cultivation.RecoveryMode) (RecoveryActivity, bool) {
	if branch == nil {
		return RecoveryActivity{}, false
	}
	normalized := normalizeStatePanelActorID(actorID)
	var best RecoveryActivity
	found := false
	for _, activity := range branch.Recoveries {
		if normalizeStatePanelActorID(activity.ActorID) != normalized || activity.Mode != mode {
			continue
		}
		switch activity.Activity.Status {
		case storytime.ActivityPaused, storytime.ActivityAwaitingDecision:
		default:
			continue
		}
		if int64(activity.Activity.RequiredEffective)-int64(activity.Activity.CompletedEffective) <= 0 {
			continue
		}
		if found && activity.ClockSeconds <= best.ClockSeconds {
			continue
		}
		best, found = activity, true
	}
	return best, found
}

// turnRestPlan is the rest this turn really settles.
type turnRestPlan struct {
	input TurnRestInput
	mode  cultivation.RecoveryMode
	// activity is the activity the request settles against, already resumed when
	// it continues one.
	activity RecoveryActivity
	// continued is the interrupted activity this turn continues, nil for a new
	// one.
	continued *RecoveryActivity
	// nodes are the committed boundaries the interval stops at.
	nodes []storytime.Node
}

// turnRestActivityID names the activity a turn's rest settles against.
//
// It is derived from the turn rather than minted at random so a rerolled turn
// re-publishes the same activity instead of leaving the superseded one behind on
// the branch. A continuation keeps its own id, which is what makes the activity
// read as one uninterrupted intent.
func turnRestActivityID(turnID string) string {
	return "rcv-" + strings.TrimSpace(turnID)
}

// planTurnRest resolves the activity a rest turn settles against.
//
// A continuation only applies when this turn asks for at least what the
// interrupted rest still owes. Naming less is a shorter rest of its own: folding
// it into the paused activity would spend time the player did not ask for, and
// PlanActivity settles an activity's whole remainder once it is resumed.
func planTurnRest(
	storyID, branchID string,
	branch *storyBranchProjection,
	system StoryDirectorActorStateSystem,
	actorID string,
	input *TurnRestInput,
	clock StoryClock,
	eligibility cultivation.RecoveryEligibility,
	boundary *storyLifespanBoundary,
	turnID string,
) (turnRestPlan, error) {
	plan := turnRestPlan{}
	if branch == nil || input == nil {
		return plan, nil
	}
	mode := cultivation.RecoveryMode(strings.TrimSpace(input.Mode))
	if !validRecoveryMode(mode) {
		return plan, cultivation.ErrRecoveryUnknownMode
	}
	plan.input = *input
	plan.mode = mode
	request := SafeRecoveryRequest{
		BranchID:        branchID,
		ActorID:         actorID,
		Mode:            mode,
		DurationSeconds: input.DurationSeconds,
	}
	// A named activity continues; an empty one starts fresh. The turn's own
	// activity is renamed below rather than named here, because a non-empty
	// ActivityID means "resume the activity that already exists".
	if paused, ok := pausedRecoveryActivity(branch, actorID, mode); ok {
		remaining := int64(paused.Activity.RequiredEffective) - int64(paused.Activity.CompletedEffective)
		if remaining > 0 && input.DurationSeconds >= remaining {
			request.ActivityID = paused.ID
			continued := paused
			plan.continued = &continued
		}
	}
	activity, err := planRecoveryActivity(storyID, branchID, branch, system, actorID, request, clock, eligibility)
	if err != nil {
		return turnRestPlan{}, err
	}
	if plan.continued == nil {
		// A fresh interval carries the turn's own id so a rerolled turn
		// re-publishes the same activity instead of leaving the superseded one
		// on the branch.
		activity.ID = turnRestActivityID(turnID)
		activity.Activity.ID = activity.ID
	}
	// A rest is time, so it stops at the same committed boundaries a training
	// interval does: a public event stage that arrives mid-rest changes what the
	// character can do about it, and the interval settles up to that moment.
	plan.activity = activity
	plan.nodes = cultivationStopNodesWithin(branch, clock.Instant(), input.DurationSeconds, boundary)
	return plan, nil
}

// recoveryViewAt returns the branch projection as it stands after the turn's own
// operations.
//
// A rest this turn really starts from the position the turn left the character
// in — a wound written by this turn's prose is already there — so the eligibility
// finding, the origin pools and the meridian rule are all read from that position
// rather than from the one the turn began at.
func recoveryViewAt(branch *storyBranchProjection, state map[string]any) *storyBranchProjection {
	if branch == nil {
		return nil
	}
	view := *branch
	view.State = state
	return &view
}

// turnRestOutcome is one settled rest interval, ready to be published.
type turnRestOutcome struct {
	plan            turnRestPlan
	candidate       cultivation.RecoveryCandidate
	settled         storytime.Activity
	elapsed         int64
	ops             []ActorStateOp
	residualCleared bool
	burdenBefore    cultivation.BurdenState
	burdenAfter     cultivation.BurdenState
}

// planTurnRestCommit settles this turn's rest against the state the turn's own
// operations produced.
//
// It reports false when the rules refuse the interval — the character is in
// battle, down, or the pools moved under a paused activity. A refusal leaves the
// turn itself committed: the prose and the other modules are already valid, and
// the rest simply does not happen, exactly as an advance the rules refuse does
// not move a character's stage.
func planTurnRestCommit(
	branch *storyBranchProjection,
	system StoryDirectorActorStateSystem,
	state map[string]any,
	meta StoryMeta,
	storyID, branchID, actorID string,
	input *TurnRestInput,
	turnID string,
) (turnRestOutcome, bool) {
	view := recoveryViewAt(branch, state)
	clock, err := branchStoryClock(view, actorID, system)
	if err != nil {
		slog.WarnContext(context.Background(), fmt.Sprintf("[interactive-story] rest interval refused: clock unreadable story_id=%s actor=%s err=%v", storyID, actorID, err))
		return turnRestOutcome{}, false
	}
	eligibility, err := recoveryEligibilityFor(view, actorID, "")
	if err != nil {
		slog.WarnContext(context.Background(), fmt.Sprintf("[interactive-story] rest interval refused: eligibility unreadable story_id=%s actor=%s err=%v", storyID, actorID, err))
		return turnRestOutcome{}, false
	}
	plan, err := planTurnRest(storyID, branchID, view, system, actorID, input, clock, eligibility, lifespanBoundary(meta, state, actorID), turnID)
	if err != nil {
		slog.WarnContext(context.Background(), fmt.Sprintf("[interactive-story] rest interval refused story_id=%s actor=%s mode=%s err=%v", storyID, actorID, input.Mode, err))
		return turnRestOutcome{}, false
	}
	// The plan is what advances the activity: it settles the interval the activity
	// still owes, which is the whole target for a new activity and only the
	// remainder for a continuation. It is also what cuts the interval at a
	// committed boundary, so a rest never runs past a decision the player owes.
	planned, err := storytime.PlanActivity(plan.activity.Activity, clock.Instant(), plan.nodes)
	if err != nil {
		slog.WarnContext(context.Background(), fmt.Sprintf("[interactive-story] rest interval refused: plan failed story_id=%s actor=%s err=%v", storyID, actorID, err))
		return turnRestOutcome{}, false
	}
	candidate, err := cultivation.PlanSafeRecovery(cultivation.RecoveryRequest{Rules: system.CultivationRules,
		ActivityID:         plan.activity.Activity.ID,
		Mode:               plan.mode,
		EligibilityAtStart: plan.activity.Eligibility,
		EligibilityNow:     eligibility,
		StartAttributes:    plan.activity.Origin,
		CurrentAttributes:  recoveryCurrentAttributes(view, actorID),
		SettledSeconds:     int64(plan.activity.Activity.CompletedEffective),
		TargetSeconds:      int64(planned.Candidate.CompletedEffective),
	})
	if err != nil {
		slog.WarnContext(context.Background(), fmt.Sprintf("[interactive-story] rest interval refused: rules declined story_id=%s actor=%s err=%v", storyID, actorID, err))
		return turnRestOutcome{}, false
	}
	// The pool write is the only thing this module adds to the turn's delta. The
	// settlement that follows re-derives the maxima and the four attributes on top
	// of it, so the panel, the battle and the rest keep reading one set of
	// numbers. It is the pool write alone, without the settlement the standalone
	// entry runs for itself, because the turn's single settlement pass runs after
	// every module.
	poolOps, err := recoveryPoolWriteOps(state, system, actorID, candidate, turnID)
	if err != nil {
		slog.WarnContext(context.Background(), fmt.Sprintf("[interactive-story] rest interval refused: pool write failed story_id=%s actor=%s err=%v", storyID, actorID, err))
		return turnRestOutcome{}, false
	}
	ops := xiuxianChangedOps(state, actorID, poolOps)
	// The interval the rules were paid for is the interval the story clock
	// advances by, and it is also the interval the meridian rule relieves.
	elapsed := int64(planned.WorldElapsed)
	burdenBefore, burdenAfter, cleared, err := planMeditationBurden(view, actorID, plan.mode, elapsed, eligibility)
	if err != nil {
		slog.WarnContext(context.Background(), fmt.Sprintf("[interactive-story] rest interval refused: meridian rule failed story_id=%s actor=%s err=%v", storyID, actorID, err))
		return turnRestOutcome{}, false
	}
	if burdenAfter.Current != burdenBefore.Current || burdenAfter.Residual != burdenBefore.Residual {
		burdenOps, err := recoveryBurdenOps(state, system, actorID, burdenBefore, burdenAfter, cleared, turnID)
		if err != nil {
			slog.WarnContext(context.Background(), fmt.Sprintf("[interactive-story] rest interval refused: burden write failed story_id=%s actor=%s err=%v", storyID, actorID, err))
			return turnRestOutcome{}, false
		}
		ops = append(ops, burdenOps...)
	}
	return turnRestOutcome{
		plan: plan, candidate: candidate, settled: planned.Candidate,
		elapsed: elapsed, ops: ops, residualCleared: cleared,
		burdenBefore: burdenBefore, burdenAfter: burdenAfter,
	}, true
}

// turnRecoveryRecord publishes the interval a rest turn really spent.
//
// A Turn carries no clock: its projection cannot move one, so a rest would
// restore a pool while the world stood still. The record is a side record
// parented after whatever the turn left at the branch head, so it folds exactly
// once and a regenerated turn's record no longer matches the path.
//
// It carries no operations. The turn's own StateDelta already holds the pool and
// meridian writes, and writing them twice would let a replay apply them twice.
func turnRecoveryRecord(
	projection *storyBranchProjection,
	parentID, branchID string,
	turn TurnEvent,
	outcome turnRestOutcome,
	system StoryDirectorActorStateSystem,
	now string,
) *RecoveryRecordEvent {
	if outcome.elapsed <= 0 {
		return nil
	}
	actorID := normalizeStatePanelActorID(outcome.plan.activity.ActorID)
	clock, err := branchStoryClock(projection, actorID, system)
	if err != nil {
		return nil
	}
	clockAfter, err := AdvanceStoryClock(clock, outcome.elapsed)
	if err != nil {
		return nil
	}
	activity := outcome.plan.activity
	activity.Activity = outcome.settled
	activity.ClockSeconds = clockAfter.Seconds
	return &RecoveryRecordEvent{
		V: schemaVersion, Type: StoryEventTypeRecoveryRecord, ID: newID("rec"),
		ParentID: parentID, BranchID: branchID, Ts: now,
		TurnID: turn.ID, Cursor: "",
		Activity: activity, Candidate: outcome.candidate,
		ClockBefore: clock, ClockAfter: clockAfter,
		ResidualCleared:  outcome.residualCleared,
		BurdenBefore:     outcome.burdenBefore.Current,
		BurdenAfter:      outcome.burdenAfter.Current,
		BurdenFloorAfter: outcome.burdenAfter.Residual,
	}
}

// rerollTurnRest re-derives a rerolled turn's rest against the replacement
// position.
//
// The interval is fixed: the world already spent those seconds, and a reroll is a
// second reading of the same events rather than a rewind of the calendar. What is
// re-derived is what the interval restored, because the rule outcome the rest was
// settled against has been replaced. The replacement position already carries
// everything that happened before this turn, so only this turn's own segment is
// projected from it — re-projecting the activity's whole cumulative total would
// count the earlier segments twice.
func rerollTurnRest(
	branch *storyBranchProjection,
	system StoryDirectorActorStateSystem,
	state map[string]any,
	storyID, branchID, actorID string,
	source recoveryRerollSource,
	turnID string,
) (turnRestOutcome, bool) {
	if source.Record == nil {
		return turnRestOutcome{}, false
	}
	record := *source.Record
	view := recoveryViewAt(branch, state)
	eligibility, err := recoveryEligibilityFor(view, actorID, "")
	if err != nil {
		return turnRestOutcome{}, false
	}
	spent := record.Candidate.ToElapsedSeconds - record.Candidate.FromElapsedSeconds
	if spent <= 0 {
		return turnRestOutcome{}, false
	}
	// The replacement position already carries the turn's own state writes, so it
	// is the origin the interval is re-settled from. Reading the pre-turn state
	// would add this interval's restoration a second time on top of the one the
	// superseded write already made.
	origin := recoveryCurrentAttributes(view, actorID)
	candidate, err := cultivation.PlanSafeRecovery(cultivation.RecoveryRequest{Rules: system.CultivationRules,
		ActivityID:         record.Activity.Activity.ID,
		Mode:               record.Activity.Mode,
		EligibilityAtStart: eligibility,
		EligibilityNow:     eligibility,
		StartAttributes:    origin,
		CurrentAttributes:  origin,
		SettledSeconds:     0,
		TargetSeconds:      spent,
	})
	if err != nil {
		return turnRestOutcome{}, false
	}
	poolOps, err := recoveryPoolWriteOps(state, system, actorID, candidate, turnID)
	if err != nil {
		return turnRestOutcome{}, false
	}
	ops := xiuxianChangedOps(state, actorID, poolOps)
	burdenBefore, burdenAfter, cleared, err := planMeditationBurden(view, actorID, record.Activity.Mode, spent, eligibility)
	if err != nil {
		return turnRestOutcome{}, false
	}
	if burdenAfter.Current != burdenBefore.Current || burdenAfter.Residual != burdenBefore.Residual {
		burdenOps, err := recoveryBurdenOps(state, system, actorID, burdenBefore, burdenAfter, cleared, turnID)
		if err != nil {
			return turnRestOutcome{}, false
		}
		ops = append(ops, burdenOps...)
	}
	return turnRestOutcome{
		plan: turnRestPlan{
			input:    TurnRestInput{ActorID: actorID, Mode: string(record.Activity.Mode), DurationSeconds: spent},
			mode:     record.Activity.Mode,
			activity: record.Activity,
		},
		candidate: candidate, settled: record.Activity.Activity,
		elapsed: spent, ops: ops, residualCleared: cleared,
		burdenBefore: burdenBefore, burdenAfter: burdenAfter,
	}, true
}

// removeSafeRecoveryActorOps drops a previous rest interval's writes.
//
// It mirrors removeCultivationSettlementActorOps for the recovery's own source
// kind. A reroll has to discard them because they were settled against the
// position the replaced rule outcome left the character in; leaving them beside
// the re-derived numbers would apply one rest twice.
func removeSafeRecoveryActorOps(ops []ActorStateOp) []ActorStateOp {
	if len(ops) == 0 {
		return nil
	}
	out := make([]ActorStateOp, 0, len(ops))
	for _, op := range ops {
		if op.SourceKind == StateOpSourceSafeRecovery {
			continue
		}
		out = append(out, op)
	}
	return normalizeActorStateOps(out)
}

// recoveryRerollSource is the rest record a turn already committed, plus the
// event ID that record sits at.
//
// The ID is carried beside the decoded body because the body is a payload: the
// event's own identity lives on the envelope, and a replacement has to be
// chained onto it to fold.
type recoveryRerollSource struct {
	Record *RecoveryRecordEvent
	ID     string
}

// recoveryRecordForTurn finds the rest interval a turn committed, if it rested.
//
// It scans the branch's own path rather than a process-local index, so the answer
// survives a restart: the journal is what remembers.
func recoveryRecordForTurn(path []StoryEventRecord, branchID, turnID string) (recoveryRerollSource, bool) {
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeRecoveryRecord {
			continue
		}
		var event RecoveryRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			continue
		}
		if event.BranchID != branchID || strings.TrimSpace(event.TurnID) != turnID {
			continue
		}
		return recoveryRerollSource{Record: &event, ID: record.Envelope.ID}, true
	}
	return recoveryRerollSource{}, false
}

// recoveryRerollProjection rewinds a branch to the instant before a rest interval.
//
// A reroll re-reads the same events against a replacement rule outcome, so the
// interval is planned again from the moment it began rather than from the moment
// it ended. The superseded activity is dropped from the projection so the replan
// starts a fresh segment instead of continuing one it just settled.
func recoveryRerollProjection(branch *storyBranchProjection, turnID string, source recoveryRerollSource) *storyBranchProjection {
	if branch == nil {
		return nil
	}
	clone := *branch
	clone.Recoveries = cloneRecoveryActivities(branch.Recoveries)
	delete(clone.Recoveries, turnRestActivityID(turnID))
	if source.Record != nil && source.Record.ClockBefore.Validate() == nil {
		clock := source.Record.ClockBefore
		clone.Clock = &clock
	}
	return &clone
}

// recoveryRerollRecord republishes a rerolled turn's rest interval.
//
// A reroll replaces the rule outcome the interval was settled against, so the
// pool and meridian writes have to be re-derived with it. What it does not do is
// re-spend the time: the world already spent those hours, and a reroll is a
// second reading of the same events rather than a rewind of the calendar. The
// replacement therefore keeps the superseded record's window verbatim and
// carries only the new outcome, chained onto the record it replaces so the
// projection overwrites one activity instead of keeping both.
func recoveryRerollRecord(
	projection *storyBranchProjection,
	turn TurnEvent,
	outcome turnRestOutcome,
	source recoveryRerollSource,
	now string,
) *RecoveryRecordEvent {
	if source.Record == nil || source.Record.ClockBefore.Validate() != nil || source.Record.ClockAfter.Validate() != nil {
		return nil
	}
	activity := source.Record.Activity
	activity.Activity = outcome.settled
	activity.ClockSeconds = source.Record.ClockAfter.Seconds
	return &RecoveryRecordEvent{
		V: schemaVersion, Type: StoryEventTypeRecoveryRecord, ID: newID("rec"),
		ParentID: source.ID, BranchID: turn.BranchID, Ts: now,
		TurnID: turn.ID, Cursor: "",
		Activity: activity, Candidate: outcome.candidate,
		ClockBefore: source.Record.ClockBefore, ClockAfter: source.Record.ClockAfter,
		ResidualCleared:  outcome.residualCleared,
		BurdenBefore:     outcome.burdenBefore.Current,
		BurdenAfter:      outcome.burdenAfter.Current,
		BurdenFloorAfter: outcome.burdenAfter.Residual,
	}
}
