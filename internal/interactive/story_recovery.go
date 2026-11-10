package interactive

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"ta/internal/cultivation"
	"ta/internal/storytime"
)

// StateOpSourceSafeRecovery marks an Actor State operation the off-battle safe
// recovery rules produced.
//
// It is its own source kind for the same reason the combat settlement has one: a
// reader of the journal can tell a pool a rest restored from a pool a narrative
// described, and a later re-settlement can discard its own previous writes
// without touching anything else.
const StateOpSourceSafeRecovery = "safe_recovery"

// StoryEventTypeRecoveryRecord is a side record on the canonical story journal.
// It carries one safe rest or meditation interval — the activity, the interval it
// really settled and the pool that interval restored — without creating a story
// Turn and without consuming a turn number.
//
// Three properties are deliberate.
//
//   - There is no second world and no second clock. §三.1 forbids recovery from
//     advancing a time of its own, so the record carries the branch's formal
//     clock both before and after, and the interval it moved is exactly the
//     interval the rules were paid for. The recovery does not own a time; it
//     spends the story's one time.
//
//   - The activity is the idempotency. §三.5 requires that a retry on the same
//     activity and target add nothing, and that a partial interval followed by a
//     continuation count only the remainder. Both fall out of the activity's
//     cumulative effective seconds being the same number PlanSafeRecovery
//     projects its rate from, rather than being enforced by a second ledger.
//
//   - The record and the pool write are one transaction. A rest either exists in
//     the journal with its restored pool or does not exist at all. §三.5's "失败
//     不半扣半补" is that requirement: a record without its settlement would be a
//     day that passed to nobody, and a settlement without its record would be
//     health with no rest behind it.
type RecoveryRecordEvent struct {
	V        int    `json:"v"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	BranchID string `json:"branch_id"`
	Ts       string `json:"ts"`
	// TurnID is the main-story turn this interval belongs to. It is empty when
	// the interval was settled from the light rest entry rather than inside a
	// turn, which is a legitimate way to spend time without a turn.
	TurnID string `json:"turn_id,omitempty"`
	// Cursor is the canonical commit position this record was projected at.
	Cursor string `json:"cursor,omitempty"`
	// CommandID is the caller-owned idempotency key this record was written
	// under, and CommandHash fingerprints the content behind it. Together they
	// let a retried commit find what the first attempt settled instead of
	// granting the same rest a second time.
	CommandID   string `json:"command_id,omitempty"`
	CommandHash string `json:"command_hash,omitempty"`
	// WorldTime is the story's own display text for this moment. It is a label;
	// every calculation uses ClockAfter, which is the formal axis.
	WorldTime string `json:"world_time,omitempty"`
	// Activity is the whole activity after the interval: its origin, its
	// cumulative settled seconds and its target. It is what makes a repeat, a
	// continuation and a restart all read the same number.
	Activity RecoveryActivity `json:"activity"`
	// Candidate is the interval this record settled, kept so a reopened panel can
	// show what just happened without re-deriving it from the rules.
	Candidate cultivation.RecoveryCandidate `json:"candidate"`
	// ClockBefore and ClockAfter are the branch's formal time axis around this
	// interval. ClockAfter - ClockBefore.Seconds is the in-fiction time the rest
	// really consumed, and the recovery owns no other clock.
	ClockBefore StoryClock `json:"clock_before"`
	ClockAfter  StoryClock `json:"clock_after"`
	// ResidualCleared reports that this interval was also the safe regulation a
	// meridian residual injury required. §四 makes the clearing the effect of the
	// same minutes, so the fact is recorded with them rather than recomputed on a
	// replay from conditions that may have moved since.
	ResidualCleared bool `json:"residual_cleared,omitempty"`
	// BurdenBefore, BurdenAfter and BurdenFloorAfter are the meridian burden
	// around this interval. They are kept so a replayed answer reports the relief
	// the rules really applied instead of a zero the replay cannot derive.
	BurdenBefore     int `json:"burden_before,omitempty"`
	BurdenAfter      int `json:"burden_after,omitempty"`
	BurdenFloorAfter int `json:"burden_floor_after,omitempty"`
}

// RecoveryActivity is one safe recovery activity as it stands on a branch.
//
// Origin carries the pools the activity started from. They cannot be recovered
// from the live state afterwards — the live state already has the recovery
// applied — and PlanSafeRecovery projects its cumulative rate from exactly those
// numbers, so losing them would silently reset the rate.
type RecoveryActivity struct {
	ID      string                   `json:"id"`
	ActorID string                   `json:"actor_id"`
	Mode    cultivation.RecoveryMode `json:"mode"`
	// Activity is the storytime activity: its origin instant, the cumulative
	// effective seconds it has settled and the seconds it is aiming for.
	Activity storytime.Activity `json:"activity"`
	// Origin is the pool snapshot at the activity's origin.
	Origin cultivation.Attributes `json:"origin"`
	// Eligibility is the safety finding the activity started under. §三.5
	// requires a change of safety to start a new segment rather than extend this
	// one, so it is recorded at the origin and compared on every continuation.
	Eligibility cultivation.RecoveryEligibility `json:"eligibility"`
	// ClockSeconds is the formal instant the activity's origin sits at.
	ClockSeconds int64 `json:"clock_seconds"`
}

// Validate rejects an activity the arithmetic cannot honestly continue.
func (activity RecoveryActivity) Validate() error {
	if strings.TrimSpace(activity.ID) == "" {
		return fmt.Errorf("恢复活动缺少 id / a recovery activity requires an id")
	}
	if normalizeStatePanelActorID(activity.ActorID) == "" {
		return fmt.Errorf("恢复活动缺少 actor_id / a recovery activity requires an actor")
	}
	if !validRecoveryMode(activity.Mode) {
		return cultivation.ErrRecoveryUnknownMode
	}
	if !validRecoveryEligibilityValue(activity.Eligibility) {
		return cultivation.ErrRecoveryUnknownEligibility
	}
	if activity.ClockSeconds < 0 {
		return storytime.ErrNegativeTime
	}
	return activity.Activity.Validate()
}

func validRecoveryMode(mode cultivation.RecoveryMode) bool {
	switch mode {
	case cultivation.RecoveryHealthRest, cultivation.RecoverySafeMeditation:
		return true
	default:
		return false
	}
}

func validRecoveryEligibilityValue(eligibility cultivation.RecoveryEligibility) bool {
	switch eligibility {
	case cultivation.RecoveryQualified, cultivation.RecoveryInBattle, cultivation.RecoveryInDanger,
		cultivation.RecoveryDead, cultivation.RecoveryNeedsSpecialTreatment:
		return true
	default:
		return false
	}
}

// SafeRecoveryRequest is one player request to spend an interval resting or
// regulating.
//
// The caller supplies the interval and, optionally, its own safety finding. It
// does not supply the resulting pool: §三.4 is explicit that eligibility comes
// from formal state and the scene, and a client that could send safe=true would
// be deciding its own recovery.
type SafeRecoveryRequest struct {
	BranchID string
	TurnID   string
	// WorldTime is the story's display text for this moment. Empty leaves the
	// caller's scene text alone.
	WorldTime string
	ActorID   string
	Mode      cultivation.RecoveryMode
	// DurationSeconds is the in-fiction interval the player asked to spend. It
	// must be positive for a new activity.
	DurationSeconds int64
	// ActivityID continues an activity that was cut short at a real node. Empty
	// starts a new one. §三.5's "部分完成再继续只算剩余" is this path: a
	// continuation aims at the activity's own target and settles only the
	// remainder.
	ActivityID string
	// DeclaredEligibility is the caller's own safety finding, such as a
	// creator-marked seal the store cannot derive. It may only make the store's
	// derivation stricter, never weaker, so it cannot be used to claim safety.
	DeclaredEligibility cultivation.RecoveryEligibility
	// Nodes are the registered boundaries this interval must stop at, such as a
	// decision the player has to make. §三.6 asks the ordinary action to advance
	// directly and the prose to stop naturally at a node that really needs a
	// choice, so the boundaries are the caller's to register and the store's to
	// respect. A node already handled is passed back with Resolved set.
	Nodes       []storytime.Node
	CommandID   string
	CommandHash string
}

// SafeRecoveryResult is what a committed interval reports back.
type SafeRecoveryResult struct {
	EventID     string
	ActivityID  string
	ActorID     string
	Mode        cultivation.RecoveryMode
	Activity    RecoveryActivity
	Candidate   cultivation.RecoveryCandidate
	ClockAfter  StoryClock
	Elapsed     int64
	Eligibility cultivation.RecoveryEligibility
	// SettlementOps are the Actor State writes this commit produced, so the
	// caller can report what moved without re-deriving it.
	SettlementOps []ActorStateOp
	// Completed reports whether the activity reached its target. A false value
	// means a real node stopped it and the caller must resume it explicitly.
	Completed bool
	// PendingNodeIDs are the registered boundaries that stopped this interval, so
	// the caller can say what needs a decision rather than presenting a short rest
	// as the whole one.
	PendingNodeIDs []string
	// Replayed reports that this result came from an interval already committed
	// under the same command. The caller can then say that nothing was added
	// rather than presenting a second recovery that never happened.
	Replayed bool
	// ElapsedDescription is the natural wording §三.2 allows for the interval,
	// such as 一日, rather than a fabricated precise calendar stamp.
	ElapsedDescription string
	// ResidualCleared reports that this interval was also the safe regulation a
	// meridian residual injury required, so the injury is gone. §四 makes the
	// clearing the effect of the same interval rather than a second one, so a
	// caller can say what happened without re-deriving the conditions.
	ResidualCleared bool
	// BurdenBefore and BurdenAfter are the meridian burden around the interval,
	// and BurdenFloorAfter is the residual floor left afterwards. They are
	// reported so the panel can show the relief the rules really applied instead
	// of a number it recomputed for itself.
	BurdenBefore     int
	BurdenAfter      int
	BurdenFloorAfter int
}

// ErrRecoveryCommandConflict means a command ID was reused for different
// content. Refusing is the only honest answer: accepting would either lose the
// first interval or grant the second one under a false identity, and either way
// the character's rest would be a lie.
var ErrRecoveryCommandConflict = errors.New("recovery command identity conflict")

// ErrRecoveryUnknownActivity means a continuation named an activity this branch
// does not have. Guessing a replacement would apply the remainder to a different
// rest, so it is refused.
var ErrRecoveryUnknownActivity = errors.New("recovery activity is not on this branch")

// ErrRecoveryUnknownActor means the rules do not track the named character.
// Writing pools into an untracked actor would invent fields their template never
// declared.
var ErrRecoveryUnknownActor = errors.New("recovery actor is not tracked by the cultivation rules")

// ErrRecoveryMissingDuration means a new activity arrived without an interval.
var ErrRecoveryMissingDuration = errors.New("a new safe recovery activity requires a positive duration")

// ErrRecoveryActivityFinished means a continuation named an activity that already
// reached its target. More rest is a new rest, not an extension of a finished
// one, and silently extending would let one request earn two full recoveries.
var ErrRecoveryActivityFinished = errors.New("recovery activity already reached its target")

// ApplySafeRecovery settles one safe rest or meditation interval and commits it
// as a side record plus its pool write in a single journal transaction.
//
// The append is idempotent on CommandID. A retry after a lost response, a
// repeated click or a recovery re-read returns what the first attempt committed,
// so the same interval is never paid for twice.
func (s *Store) ApplySafeRecovery(storyID string, req SafeRecoveryRequest) (SafeRecoveryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return SafeRecoveryResult{}, err
	}
	defer releaseStory()

	meta, records, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return SafeRecoveryResult{}, err
	}
	return s.applySafeRecoveryLocked(storyID, meta, records, req)
}

// applySafeRecoveryLocked is the commit itself, for callers that already hold the
// store lock and have already read the branch.
//
// It exists because the lock is not reentrant: a caller that reads the branch,
// derives the interval from it and then commits has to do all three under one
// hold, or another write could advance the pools between the read and the write
// and the recovery would be projected from numbers that no longer hold.
func (s *Store) applySafeRecoveryLocked(storyID string, meta StoryMeta, records []StoryEventRecord, req SafeRecoveryRequest) (SafeRecoveryResult, error) {
	commandID := strings.TrimSpace(req.CommandID)
	commandHash := strings.TrimSpace(req.CommandHash)
	if commandID != "" && commandHash == "" {
		return SafeRecoveryResult{}, fmt.Errorf("%w: command_id=%q requires a command hash", ErrRecoveryCommandConflict, commandID)
	}

	branchID := strings.TrimSpace(req.BranchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return SafeRecoveryResult{}, fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}
	if commandID != "" {
		committed, found, err := recoveryRecordForCommand(records, branchID, commandID, commandHash)
		if err != nil {
			return SafeRecoveryResult{}, err
		}
		if found {
			// The first attempt already settled this exact interval. Returning its
			// result makes the retry a read rather than a second grant.
			return committed, nil
		}
	}
	if branchIsTerminal(records, branch.Head) {
		return SafeRecoveryResult{}, branchTerminalError()
	}

	actorID := normalizeStatePanelActorID(req.ActorID)
	if actorID == "" {
		return SafeRecoveryResult{}, fmt.Errorf("%w: missing actor", ErrRecoveryUnknownActor)
	}
	if !validRecoveryMode(req.Mode) {
		return SafeRecoveryResult{}, cultivation.ErrRecoveryUnknownMode
	}

	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return SafeRecoveryResult{}, err
	}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState)
	if !ActorStateSystemIsXiuxian(system) {
		return SafeRecoveryResult{}, fmt.Errorf("%w: story is not on the cultivation rules", ErrRecoveryUnknownActor)
	}

	clock, err := branchStoryClock(projection, actorID, system)
	if err != nil {
		return SafeRecoveryResult{}, err
	}
	eligibility, err := recoveryEligibilityFor(projection, actorID, req.DeclaredEligibility)
	if err != nil {
		return SafeRecoveryResult{}, err
	}

	activity, err := planRecoveryActivity(storyID, branchID, projection, system, actorID, req, clock, eligibility)
	if err != nil {
		return SafeRecoveryResult{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	eventID := newID("rec")
	// The plan is what advances the activity: it settles the interval the
	// activity still owes, which is the whole target for a new activity and only
	// the remainder for a continuation. §三.5's "部分完成再继续只算剩余" is this
	// call, not a second ledger of partial rests.
	plan, err := storytime.PlanActivity(activity.Activity, clock.Instant(), req.Nodes)
	if err != nil {
		return SafeRecoveryResult{}, err
	}
	settledSeconds := int64(plan.Candidate.CompletedEffective)
	candidate, err := cultivation.PlanSafeRecovery(cultivation.RecoveryRequest{Rules: system.CultivationRules,
		ActivityID:         activity.Activity.ID,
		Mode:               activity.Mode,
		EligibilityAtStart: activity.Eligibility,
		EligibilityNow:     eligibility,
		StartAttributes:    activity.Origin,
		CurrentAttributes:  recoveryCurrentAttributes(projection, actorID),
		SettledSeconds:     int64(activity.Activity.CompletedEffective),
		TargetSeconds:      settledSeconds,
	})
	if err != nil {
		return SafeRecoveryResult{}, err
	}

	settlementOps, err := recoverySettlementOps(projection.State, system, actorID, candidate, eventID)
	if err != nil {
		return SafeRecoveryResult{}, err
	}

	// The interval the rules were paid for is the interval the story clock
	// advances by. §三.1 forbids recovery from owning a time of its own, so the
	// two numbers are one number: there is no path here that moves the clock
	// without settling the pool, or settles the pool without moving the clock.
	elapsed := int64(plan.WorldElapsed)

	// §四's meridian rule rides on that same interval: the minutes a meditation
	// already paid for are the minutes that relieve ordinary burden and, when the
	// body conditions really hold, clear a residual injury. It adds no time of its
	// own, which is why it is planned after the interval is known and before the
	// clock is advanced.
	burdenBefore, burdenAfter, residualCleared, err := planMeditationBurden(
		projection, actorID, activity.Mode, elapsed, eligibility)
	if err != nil {
		return SafeRecoveryResult{}, err
	}
	if burdenAfter.Current != burdenBefore.Current || burdenAfter.Residual != burdenBefore.Residual {
		burdenOps, err := recoveryBurdenOps(projection.State, system, actorID, burdenBefore, burdenAfter, residualCleared, eventID)
		if err != nil {
			return SafeRecoveryResult{}, err
		}
		settlementOps = append(settlementOps, burdenOps...)
	}

	clockAfter, err := AdvanceStoryClock(clock, elapsed)
	if err != nil {
		return SafeRecoveryResult{}, err
	}
	activity.Activity = plan.Candidate
	activity.ClockSeconds = clockAfter.Seconds

	record := RecoveryRecordEvent{
		V: schemaVersion, Type: StoryEventTypeRecoveryRecord, ID: eventID,
		ParentID: branch.Head, BranchID: branchID, Ts: now,
		TurnID: strings.TrimSpace(req.TurnID), Cursor: eventID,
		CommandID: commandID, CommandHash: commandHash,
		WorldTime: strings.TrimSpace(req.WorldTime),
		Activity:  activity, Candidate: candidate,
		ClockBefore: clock, ClockAfter: clockAfter,
		ResidualCleared:  residualCleared,
		BurdenBefore:     burdenBefore.Current,
		BurdenAfter:      burdenAfter.Current,
		BurdenFloorAfter: burdenAfter.Residual,
	}

	headID := eventID
	newEvents := []any{record}
	if len(settlementOps) > 0 {
		for _, op := range settlementOps {
			if err := validateActorStateOp(op); err != nil {
				return SafeRecoveryResult{}, err
			}
		}
		// The pool write is a separate event parented to the record, so the branch
		// head advances through the interval and then through its consequence, and
		// the next turn is parented after both. It carries its own identity: an
		// event whose ID is also its parent closes the branch's parent chain into a
		// cycle, and every later walk of that chain would never reach an ancestor.
		deltaID := newID("sd")
		newEvents = append(newEvents, newStateDeltaEventWithActorOps(deltaID, eventID, branchID, now, nil, settlementOps))
		headID = deltaID
	}
	// A rest is still time passing, so every registered matter whose instant the
	// rest crossed closes here. The recovery record already carries the interval,
	// and these records carry no clock, so the branch keeps the instant the rest
	// settled.
	closed, headID := storyIntervalEvents(projection, system, branchID, headID, now, "", "", clock, clockAfter)
	newEvents = append(newEvents, closed...)

	branch.Head = headID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, newEvents...); err != nil {
		return SafeRecoveryResult{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return SafeRecoveryResult{}, err
	}
	return SafeRecoveryResult{
		EventID: eventID, ActivityID: activity.Activity.ID, ActorID: actorID, Mode: activity.Mode,
		Activity: activity, Candidate: candidate, ClockAfter: clockAfter,
		Elapsed:     elapsed,
		Eligibility: eligibility, SettlementOps: settlementOps,
		Completed:          activity.Activity.Status == storytime.ActivityCompleted,
		PendingNodeIDs:     append([]string{}, activity.Activity.PendingNodeIDs...),
		ElapsedDescription: DescribeElapsed(elapsed),
		ResidualCleared:    residualCleared,
		BurdenBefore:       burdenBefore.Current,
		BurdenAfter:        burdenAfter.Current,
		BurdenFloorAfter:   burdenAfter.Residual,
	}, nil
}

// planRecoveryActivity resolves the activity this request settles against.
//
// A new activity takes its origin from the branch's formal clock and its target
// from the requested interval. A continuation resumes an activity that a real
// node stopped and aims at that activity's own target, which is what makes a
// partial interval followed by a continuation count only the remainder.
func planRecoveryActivity(storyID, branchID string, projection *storyBranchProjection, system StoryDirectorActorStateSystem, actorID string, req SafeRecoveryRequest, clock StoryClock, eligibility cultivation.RecoveryEligibility) (RecoveryActivity, error) {
	origin := recoveryCurrentAttributes(projection, actorID)

	if activityID := strings.TrimSpace(req.ActivityID); activityID != "" {
		existing, found := projection.Recoveries[activityID]
		if !found {
			return RecoveryActivity{}, fmt.Errorf("%w: %s", ErrRecoveryUnknownActivity, activityID)
		}
		if normalizeStatePanelActorID(existing.ActorID) != actorID {
			return RecoveryActivity{}, fmt.Errorf("%w: activity %s belongs to %s", ErrRecoveryUnknownActivity, existing.ID, existing.ActorID)
		}
		if existing.Mode != req.Mode {
			return RecoveryActivity{}, fmt.Errorf("%w: activity %s is %s", cultivation.ErrRecoveryUnknownMode, existing.ID, existing.Mode)
		}
		if existing.Eligibility != eligibility {
			// §三.5 requires a change of safety condition to start a new segment
			// rather than extend this one. Extending would let an ineligible
			// stretch be spliced onto an eligible one and counted as continuous.
			return RecoveryActivity{}, fmt.Errorf("%w: %s -> %s", cultivation.ErrRecoveryEligibilityChanged, existing.Eligibility, eligibility)
		}
		switch existing.Activity.Status {
		case storytime.ActivityPaused, storytime.ActivityAwaitingDecision:
		default:
			// A finished rest is finished. Extending it would let one request earn
			// two full recoveries, so more rest is a new activity instead.
			return RecoveryActivity{}, fmt.Errorf("%w: activity %s is %s", ErrRecoveryActivityFinished, existing.ID, existing.Activity.Status)
		}
		// The pools must still match what the origin plus the settled interval
		// predicts. A change since then is a real condition change — a wound taken,
		// a maximum moved — and §三.5 requires it to start a new segment rather
		// than extend this one.
		if _, err := cultivation.PlanSafeRecovery(cultivation.RecoveryRequest{Rules: system.CultivationRules,
			ActivityID:         existing.Activity.ID,
			Mode:               existing.Mode,
			EligibilityAtStart: existing.Eligibility,
			EligibilityNow:     eligibility,
			StartAttributes:    existing.Origin,
			CurrentAttributes:  origin,
			SettledSeconds:     int64(existing.Activity.CompletedEffective),
			TargetSeconds:      int64(existing.Activity.CompletedEffective),
		}); err != nil {
			return RecoveryActivity{}, err
		}
		// Resuming goes through the storytime rules' own resume rather than
		// rewinding the activity's settled point to where it paused. The interval
		// between the pause and this call is not rest: the character was doing
		// whatever stopped them. storytime.ResumeActivity moves LastSettledAt to
		// now and keeps the effective target, so the activity still owes only its
		// remainder and the unearned gap can never be spliced on. §三.5's "部分完成
		// 再继续只算剩余" and its refusal to count an ineligible stretch as
		// continuous are both this call.
		resumed, err := storytime.ResumeActivity(existing.Activity, clock.Instant())
		if err != nil {
			return RecoveryActivity{}, err
		}
		activity := existing
		activity.Activity = resumed
		activity.ClockSeconds = clock.Seconds
		return activity, nil
	}

	if req.DurationSeconds <= 0 {
		return RecoveryActivity{}, fmt.Errorf("%w: %d", ErrRecoveryMissingDuration, req.DurationSeconds)
	}
	if eligibility != cultivation.RecoveryQualified {
		return RecoveryActivity{}, fmt.Errorf("%w: %s", cultivation.ErrRecoveryIneligible, eligibility)
	}
	activityID := newID("rcv")
	instant := clock.Instant()
	activity := storytime.Activity{
		ID: activityID, StoryID: strings.TrimSpace(storyID), BranchID: branchID,
		SourceID: actorID, Mode: storytime.EffectiveDuration, Status: storytime.ActivityRunning,
		StartedAt: instant, LastSettledAt: instant,
		RequiredEffective: storytime.Duration(req.DurationSeconds),
	}
	return RecoveryActivity{
		ID: activityID, ActorID: actorID, Mode: req.Mode,
		Activity: activity, Origin: origin, Eligibility: eligibility,
		ClockSeconds: clock.Seconds,
	}, nil
}

// recoveryRecordForCommand looks for the interval a previous attempt committed
// under this command ID. It scans the branch's own recent records rather than
// trusting a process-local index, so the answer survives a restart: the journal
// is what remembers, not the process.
//
// A found command ID with a different hash is a conflict. The caller reused one
// identity for two different intervals, and either answer would be wrong.
func recoveryRecordForCommand(records []StoryEventRecord, branchID, commandID, commandHash string) (SafeRecoveryResult, bool, error) {
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeRecoveryRecord {
			continue
		}
		var event RecoveryRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return SafeRecoveryResult{}, false, fmt.Errorf("decode committed recovery record: %w", err)
		}
		if event.BranchID != branchID || strings.TrimSpace(event.CommandID) != commandID {
			continue
		}
		if strings.TrimSpace(event.CommandHash) != commandHash {
			return SafeRecoveryResult{}, false, fmt.Errorf("%w: command_id=%q existing_hash=%q requested_hash=%q", ErrRecoveryCommandConflict, commandID, event.CommandHash, commandHash)
		}
		return SafeRecoveryResult{
			EventID: event.ID, ActivityID: event.Activity.ID, ActorID: event.Activity.ActorID,
			Mode: event.Activity.Mode, Activity: event.Activity, Candidate: event.Candidate,
			ClockAfter:         event.ClockAfter,
			Elapsed:            event.Candidate.ToElapsedSeconds - event.Candidate.FromElapsedSeconds,
			Eligibility:        event.Activity.Eligibility,
			Completed:          event.Activity.Activity.Status == storytime.ActivityCompleted,
			PendingNodeIDs:     append([]string{}, event.Activity.Activity.PendingNodeIDs...),
			Replayed:           true,
			ElapsedDescription: DescribeElapsed(event.Candidate.ToElapsedSeconds - event.Candidate.FromElapsedSeconds),
			// The meridian facts come off the record rather than being re-derived:
			// the conditions that allowed the clearing may have moved since, and a
			// replay has to report what the first attempt committed.
			ResidualCleared:  event.ResidualCleared,
			BurdenBefore:     event.BurdenBefore,
			BurdenAfter:      event.BurdenAfter,
			BurdenFloorAfter: event.BurdenFloorAfter,
		}, true, nil
	}
	return SafeRecoveryResult{}, false, nil
}

// applyRecoveryRecord merges a side record into the branch projection. It is the
// only place a recovery activity and the formal clock are written during a
// rebuild, so a replay reproduces the time and the activity that were live when
// the record was committed.
func applyRecoveryRecord(branch *storyBranchProjection, event RecoveryRecordEvent) error {
	if err := event.Activity.Validate(); err != nil {
		return err
	}
	if err := event.ClockAfter.Validate(); err != nil {
		return err
	}
	if event.ClockBefore.Validate() != nil {
		return fmt.Errorf("恢复记录缺少合法的起始时刻 / a recovery record needs a valid clock before it")
	}
	// The clock only moves forward. A record that appears to move it backwards is
	// a replayed fork or a corrupt row, and accepting it would let a branch's
	// formal time run in reverse.
	if event.ClockAfter.Seconds < branch.clockSeconds() {
		return fmt.Errorf("%w: recovery record moves the story clock backwards", storytime.ErrTimeReversal)
	}
	if branch.Recoveries == nil {
		branch.Recoveries = map[string]RecoveryActivity{}
	}
	branch.Recoveries[event.Activity.ID] = event.Activity
	clock := event.ClockAfter
	branch.Clock = &clock
	return nil
}

// validateRecoveryRecordEvent rejects a record that could not have been honestly
// attributed. A malformed row is an error rather than a silent drop, because a
// dropped record is a rest the character never took.
func validateRecoveryRecordEvent(event RecoveryRecordEvent) error {
	if strings.TrimSpace(event.BranchID) == "" {
		return fmt.Errorf("恢复记录缺少 branch_id / a recovery record requires a branch")
	}
	return event.Activity.Validate()
}

// cloneRecoveryActivities copies the activity map so one branch's recovery
// cannot be mutated through an alias into another.
func cloneRecoveryActivities(activities map[string]RecoveryActivity) map[string]RecoveryActivity {
	if len(activities) == 0 {
		return nil
	}
	cloned := make(map[string]RecoveryActivity, len(activities))
	for id, activity := range activities {
		cloned[id] = activity
	}
	return cloned
}
