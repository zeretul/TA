package interactive

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"ta/internal/cultivation"
	"ta/internal/exploration"
	"ta/internal/npcgrowth"
	"ta/internal/publiquevent"
	"ta/internal/quest"
	"ta/internal/storytime"
)

// This file is §五's player-specified cultivation interval.
//
// The cultivation engine already existed and already had the properties the
// design asks for: small-stage transitions happen automatically, the accumulator
// is carried so one long interval equals several short ones, and a major
// breakthrough is never rolled without an explicit authorisation. What was
// missing was a way for the player to name an interval at all. Cultivation could
// only ride inside a story Turn, and a Turn does not move the branch's formal
// clock, so "the training result agrees with the formal time" could not hold:
// a character could cultivate for a rule year and the story's one time axis
// would not have noticed.
//
// So this file adds a second kind of side record beside the recovery's. It
// carries one authorised cultivation interval — the activity, the interval it
// really settled and the rules position that interval reached — without creating
// a Turn and without consuming a turn number.
//
// Three properties are deliberate.
//
//   - There is no second clock and no second formula. §五 requires the interval
//     to reuse AdvanceCultivationFrom and its multiplier budget, so the advance
//     is produced by advanceXiuxianCultivation, the same function the turn path
//     calls. The record advances the branch's one clock by exactly the interval
//     the rules were paid for, so §五's "有效时段只计一次" is structural rather
//     than a rule someone has to remember.
//
//   - The activity is the idempotency, exactly as it is for a rest. A retry on
//     the same command adds nothing, and a segment cut short at a real node
//     followed by a continuation counts only the remainder. §五's "可按原活动
//     恢复，不重新加时/扣资源" is that requirement.
//
//   - The record and the advance are one transaction. An interval either exists
//     in the journal with the position it reached or does not exist at all, so a
//     rebuilt branch can never show a stage whose training time was never spent.

// StateOpSourceCultivationInterval marks an Actor State operation the cultivation
// interval produced outside a Turn.
//
// The writes themselves carry StateOpSourceCultivationAdvance, because they come
// from the same advance the turn path uses and a reader must not have to know
// which entry point ran. This constant exists so a caller can tell the two entry
// points apart when it needs to, and it is deliberately not used as an op source:
// one kind of write should have one source kind.
const StateOpSourceCultivationInterval = "cultivation_interval"

// DaysPerRuleYear is twelve rule months. §五 permits the rule month and rule year
// as duration units — the design states 30 and 360 — but only for converting an
// interval the player already named. Neither is ever used as a calendar.
const DaysPerRuleYear = 12 * cultivation.DaysPerRuleMonth

// The units a player may name an interval in. They are the units the design
// names: 小时, 日, 月 and 年.
const (
	CultivationUnitHour      = "hour"
	CultivationUnitDay       = "day"
	CultivationUnitRuleMonth = "rule_month"
	CultivationUnitRuleYear  = "rule_year"
)

// CultivationInterval is an interval in the player's own unit.
//
// It exists so the rule month and the rule year are converted to the one formal
// second axis in exactly one place, next to the constants that define them.
// Converting in the interface instead would put 30 and 360 in a language that
// cannot see the rules, and the two could then drift apart.
type CultivationInterval struct {
	// Value is how many of Unit the player asked for. It must be positive and
	// finite; a fractional value is allowed so half a month is expressible.
	Value float64 `json:"value"`
	// Unit is one of hour, day, rule_month or rule_year.
	Unit string `json:"unit"`
}

// Seconds converts the interval to the branch's formal axis.
//
// §五 is explicit that the rule month and the rule year are units of account
// rather than a calendar, so this multiplies by their stated length and does
// nothing else: it does not pick a starting date, does not know how long a month
// is in any world, and does not touch the display anchor.
func (interval CultivationInterval) Seconds(profiles ...*cultivation.Rules) (int64, error) {
	if math.IsNaN(interval.Value) || math.IsInf(interval.Value, 0) || interval.Value <= 0 {
		return 0, fmt.Errorf("%w: %v", ErrCultivationMissingDuration, interval.Value)
	}
	monthDays := float64(cultivation.DaysPerRuleMonth)
	if len(profiles) > 0 && profiles[0] != nil {
		monthDays = profiles[0].Training.DaysPerMonth
	}
	var secondsPerUnit float64
	switch strings.ToLower(strings.TrimSpace(interval.Unit)) {
	case CultivationUnitHour:
		secondsPerUnit = 60 * 60
	case CultivationUnitDay:
		secondsPerUnit = 24 * 60 * 60
	case CultivationUnitRuleMonth:
		secondsPerUnit = monthDays * 24 * 60 * 60
	case CultivationUnitRuleYear:
		secondsPerUnit = monthDays * 12 * 24 * 60 * 60
	default:
		return 0, fmt.Errorf("%w: %q", ErrCultivationUnknownUnit, interval.Unit)
	}
	seconds := interval.Value * secondsPerUnit
	if seconds > math.MaxInt64/4 {
		// The only refusal here is arithmetic. §五's rules impose no maximum
		// seclusion length — the design says so in as many words — so this is a
		// guard against an interval the axis cannot hold, not a limit on play.
		return 0, fmt.Errorf("%w: %v %s is too long for the story clock", ErrCultivationMissingDuration, interval.Value, interval.Unit)
	}
	return int64(math.Round(seconds)), nil
}

// CultivationConditions are the real conditions the interval is spent under.
//
// They are a struct rather than loose fields because a continuation has to be
// able to tell "the player named a new place" from "the player named nothing".
// §五's "沿原活动剩余目标与新条件续接" requires the first to be possible and the
// second to inherit, and a bare bool cannot express the difference.
type CultivationConditions struct {
	MethodID string `json:"method_id,omitempty"`
	// Environment is the spiritual quality of the place.
	Environment string `json:"environment,omitempty"`
	// Resource is the strength of the auxiliary resources actually consumed.
	Resource string `json:"resource,omitempty"`
	// AttemptBreakthrough is the realm the player authorised an attempt to enter.
	// Empty means no attempt was authorised, and the rules then do not roll.
	AttemptBreakthrough string `json:"attempt_breakthrough,omitempty"`
	// AuxiliaryMedicine, StableSite and QualifiedGuardian are the real conditions
	// of that attempt.
	AuxiliaryMedicine bool `json:"auxiliary_medicine,omitempty"`
	StableSite        bool `json:"stable_site,omitempty"`
	QualifiedGuardian bool `json:"qualified_guardian,omitempty"`
}

// inputFor builds the engine's own input from these conditions and an interval.
//
// The conversion to days happens here and nowhere else, so the interval the
// clock advanced by and the interval the rules integrated are the same number.
func (conditions CultivationConditions) inputFor(days float64) TurnCultivationInput {
	return TurnCultivationInput{
		MethodID:            conditions.MethodID,
		Days:                days,
		Environment:         conditions.Environment,
		Resource:            conditions.Resource,
		AttemptBreakthrough: conditions.AttemptBreakthrough,
		AuxiliaryMedicine:   conditions.AuxiliaryMedicine,
		StableSite:          conditions.StableSite,
		QualifiedGuardian:   conditions.QualifiedGuardian,
	}
}

// CultivationActivity is one cultivation activity as it stands on a branch.
//
// It mirrors RecoveryActivity, and for the same reasons: the activity's origin
// instant and cumulative settled seconds are what make a repeat, a continuation
// and a restart read the same number, and the safety finding is recorded at the
// origin so a change of safety starts a new segment rather than extending one.
type CultivationActivity struct {
	ID      string `json:"id"`
	ActorID string `json:"actor_id"`
	// Activity is the storytime activity: its origin instant, the cumulative
	// effective seconds it has settled and the seconds it is aiming for.
	Activity storytime.Activity `json:"activity"`
	// Conditions are the training conditions the segment runs under.
	Conditions CultivationConditions `json:"conditions"`
	// Eligibility is the safety finding the activity started under. §五 reuses the
	// character's existing cultivation qualification, and a change of safety — a
	// battle starting, a wound that needs treatment — has to start a new segment
	// rather than extend this one.
	Eligibility cultivation.RecoveryEligibility `json:"eligibility"`
	// ClockSeconds is the formal instant the activity's origin sits at.
	ClockSeconds int64 `json:"clock_seconds"`
	// ConvalescenceUntil is the formal instant a failed major breakthrough's
	// recovery ends, or zero when there is none.
	//
	// §7.1 lists "不存在尚未解除的突破恢复状态" among the preconditions for a new
	// attempt, so the recovery has to be a fact a later attempt can read rather
	// than a number the narration remembers. It is stored on the activity the
	// failed attempt produced, which means it travels with the same checkpoint a
	// fork or a rewind already restores: a branch that rewinds past the failure
	// inherits the activity from before it and with it no outstanding recovery.
	ConvalescenceUntil int64 `json:"convalescence_until,omitempty"`
}

// Validate rejects an activity the arithmetic cannot honestly continue.
func (activity CultivationActivity) Validate() error {
	if strings.TrimSpace(activity.ID) == "" {
		return fmt.Errorf("修炼活动缺少 id / a cultivation activity requires an id")
	}
	if normalizeStatePanelActorID(activity.ActorID) == "" {
		return fmt.Errorf("修炼活动缺少 actor_id / a cultivation activity requires an actor")
	}
	if !validRecoveryEligibilityValue(activity.Eligibility) {
		return cultivation.ErrRecoveryUnknownEligibility
	}
	if activity.ClockSeconds < 0 {
		return storytime.ErrNegativeTime
	}
	if activity.ConvalescenceUntil < 0 {
		return storytime.ErrNegativeTime
	}
	return activity.Activity.Validate()
}

// CultivationRecordEvent carries one settled cultivation interval.
type CultivationRecordEvent struct {
	V        int    `json:"v"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	BranchID string `json:"branch_id"`
	Ts       string `json:"ts"`
	// TurnID is the main-story turn this interval belongs to. It is empty when the
	// interval was settled from the cultivation entry rather than inside a turn,
	// which is a legitimate way to spend time without a turn.
	TurnID string `json:"turn_id,omitempty"`
	// Cursor is the canonical commit position this record was projected at.
	Cursor string `json:"cursor,omitempty"`
	// CommandID is the caller-owned idempotency key this record was written under,
	// and CommandHash fingerprints the content behind it. Together they let a
	// retried commit find what the first attempt settled instead of granting the
	// same interval a second time.
	CommandID   string `json:"command_id,omitempty"`
	CommandHash string `json:"command_hash,omitempty"`
	// WorldTime is the story's own display text for this moment. It is a label;
	// every calculation uses ClockAfter, which is the formal axis.
	WorldTime string `json:"world_time,omitempty"`
	// Activity is the whole activity after the interval: its origin, its cumulative
	// settled seconds and its target.
	Activity CultivationActivity `json:"activity"`
	// Advance is what the rules did with the interval: the stage reached, the
	// stages crossed, whether an authorised attempt was made and how it went. It
	// is kept so a reopened entry can show what happened without re-deriving it
	// from a position that has moved on since.
	Advance CultivationAdvanceResult `json:"advance"`
	// ClockBefore and ClockAfter are the branch's formal time axis around this
	// interval. ClockAfter - ClockBefore.Seconds is the in-fiction time the
	// interval really consumed, and the interval owns no other clock.
	ClockBefore StoryClock `json:"clock_before"`
	ClockAfter  StoryClock `json:"clock_after"`
}

// CultivationRequest is one player request to spend an interval cultivating.
//
// The caller supplies the interval and the real conditions. It does not supply
// the resulting stage or progress: those are the rules' answer, and a client that
// could send them would be deciding its own cultivation.
type CultivationRequest struct {
	BranchID string
	TurnID   string
	// WorldTime is the story's display text for this moment. Empty leaves the
	// caller's scene text alone.
	WorldTime string
	ActorID   string
	// Interval is the in-fiction interval the player named. It must be positive
	// for a new activity.
	Interval CultivationInterval
	// Conditions are the real training conditions. On a continuation a nil value
	// inherits the activity's recorded conditions, which is what makes an
	// uninterrupted interval one interval rather than several.
	Conditions *CultivationConditions
	// ActivityID continues an activity that was cut short at a real node. Empty
	// starts a new one. §五's "可按原活动恢复，不重新加时" is this path: a
	// continuation aims at the activity's own target and settles only the
	// remainder.
	ActivityID string
	// DeclaredEligibility is the caller's own safety finding, such as a
	// creator-marked condition the store cannot derive. It may only make the
	// store's derivation stricter, never weaker.
	DeclaredEligibility cultivation.RecoveryEligibility
	// Nodes are the registered boundaries this interval must stop at, such as a
	// decision the player has to make or a real condition change. §五 requires the
	// interval to commit what already happened and stop at such a node rather than
	// run past it. A node already handled is passed back with Resolved set.
	Nodes       []storytime.Node
	CommandID   string
	CommandHash string
}

// CultivationResult is what a committed interval reports back.
type CultivationResult struct {
	EventID    string
	ActivityID string
	ActorID    string
	Activity   CultivationActivity
	Advance    CultivationAdvanceResult
	ClockAfter StoryClock
	// Elapsed is the in-fiction seconds this commit really spent.
	Elapsed int64
	// Days is the same interval expressed in the rules' own unit. It is the number
	// the advance integrated, so a caller can see the two agree.
	Days float64
	// Eligibility is the safety finding the interval was admitted under.
	Eligibility cultivation.RecoveryEligibility
	// SettlementOps are the Actor State writes this commit produced.
	SettlementOps []ActorStateOp
	// Completed reports whether the activity reached its target. A false value
	// means a real node stopped it and the caller must resume it explicitly.
	Completed bool
	// PendingNodeIDs are the registered boundaries that stopped this interval.
	PendingNodeIDs []string
	// Replayed reports that this result came from an interval already committed
	// under the same command, so nothing was added.
	Replayed bool
	// ElapsedDescription is the natural wording §三.2 allows for the interval.
	ElapsedDescription string
}

// ErrCultivationCommandConflict means a command ID was reused for different
// content. Refusing is the only honest answer: accepting would either lose the
// first interval or grant the second one under a false identity.
var ErrCultivationCommandConflict = errors.New("cultivation command identity conflict")

// ErrCultivationUnknownActivity means a continuation named an activity this
// branch does not have.
var ErrCultivationUnknownActivity = errors.New("cultivation activity is not on this branch")

// ErrCultivationUnknownActor means the rules do not track the named character.
var ErrCultivationUnknownActor = errors.New("cultivation actor is not tracked by the cultivation rules")

// ErrCultivationMissingDuration means a new activity arrived without a usable
// interval.
var ErrCultivationMissingDuration = errors.New("a new cultivation activity requires a positive interval")

// ErrCultivationUnknownUnit means the interval was named in a unit the rules do
// not define.
var ErrCultivationUnknownUnit = errors.New("cultivation interval unit is unsupported")

// ErrCultivationActivityFinished means a continuation named an activity that
// already reached its target. More training is a new interval, not an extension
// of a finished one.
var ErrCultivationActivityFinished = errors.New("cultivation activity already reached its target")

// ApplyCultivationInterval settles one player-named cultivation interval and
// commits it as a side record plus its advance in a single journal transaction.
//
// The append is idempotent on CommandID. A retry after a lost response, a
// repeated click or a reopened entry returns what the first attempt committed, so
// the same interval is never paid for twice.
func (s *Store) ApplyCultivationInterval(storyID string, req CultivationRequest) (CultivationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return CultivationResult{}, err
	}
	defer releaseStory()

	meta, records, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return CultivationResult{}, err
	}
	return s.applyCultivationIntervalLocked(storyID, meta, records, req)
}

// applyCultivationIntervalLocked is the commit itself, for callers that already
// hold the store lock and have already read the branch.
//
// It exists for the same reason the recovery's does: the lock is not reentrant,
// and a caller that reads the branch, derives the interval from it and then
// commits has to do all three under one hold, or another write could advance the
// position between the read and the write.
func (s *Store) applyCultivationIntervalLocked(storyID string, meta StoryMeta, records []StoryEventRecord, req CultivationRequest) (CultivationResult, error) {
	commandID := strings.TrimSpace(req.CommandID)
	commandHash := strings.TrimSpace(req.CommandHash)
	if commandID != "" && commandHash == "" {
		return CultivationResult{}, fmt.Errorf("%w: command_id=%q requires a command hash", ErrCultivationCommandConflict, commandID)
	}

	branchID := strings.TrimSpace(req.BranchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return CultivationResult{}, fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}
	if commandID != "" {
		committed, found, err := cultivationRecordForCommand(records, branchID, commandID, commandHash)
		if err != nil {
			return CultivationResult{}, err
		}
		if found {
			// The first attempt already settled this exact interval. Returning its
			// result makes the retry a read rather than a second grant.
			return committed, nil
		}
	}
	if branchIsTerminal(records, branch.Head) {
		return CultivationResult{}, branchTerminalError()
	}

	actorID := normalizeStatePanelActorID(req.ActorID)
	if actorID == "" {
		return CultivationResult{}, fmt.Errorf("%w: missing actor", ErrCultivationUnknownActor)
	}

	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return CultivationResult{}, err
	}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState)
	if !ActorStateSystemIsXiuxian(system) {
		return CultivationResult{}, fmt.Errorf("%w: story is not on the cultivation rules", ErrCultivationUnknownActor)
	}

	clock, err := branchStoryClock(projection, actorID, system)
	if err != nil {
		return CultivationResult{}, err
	}
	// §五's "复用已有修炼资格" is this call. The safety finding is the story's own
	// — a live battle, a real danger, a character who needs treatment — and it is
	// deliberately not the rules' body-condition scale: a merely wounded character
	// still trains, slowly, which is the design's own distinction.
	eligibility, err := recoveryEligibilityFor(projection, actorID, req.DeclaredEligibility)
	if err != nil {
		return CultivationResult{}, err
	}

	activity, err := s.planCultivationActivity(storyID, branchID, projection, actorID, req, clock, eligibility)
	if err != nil {
		return CultivationResult{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	eventID := newID("cul")
	// The plan is what advances the activity: it settles the interval the activity
	// still owes, which is the whole target for a new activity and only the
	// remainder for a continuation. §五's "可按原活动恢复，不重新加时" is this call.
	//
	// The stop points come from the branch's committed facts and are merged in
	// beside anything the caller sent, so a caller cannot resolve a stage out of
	// the way and a caller that sends nothing still stops at it.
	nodes := mergeCultivationNodes(cultivationStopNodesWithin(projection, clock.Instant(), int64(activity.Activity.RequiredEffective)-int64(activity.Activity.CompletedEffective), lifespanBoundary(meta, projection.State, actorID)), req.Nodes)
	plan, err := storytime.PlanActivity(activity.Activity, clock.Instant(), nodes)
	if err != nil {
		return CultivationResult{}, err
	}
	named := int64(plan.WorldElapsed)
	// The interval the rules are asked to integrate is the interval the activity
	// planned. What they actually spend is their own answer, and the clock moves
	// by that: a character standing at a major-realm bottleneck cannot train the
	// rest of the year away, and a 筑基 attempt costs the seven days the rules
	// price it at whatever interval the player named.
	advanceOps, advance, ok := advanceXiuxianCultivation(
		projection.State, system, actorID,
		activity.Conditions.inputFor(float64(named)/(24*60*60)),
		xiuxianFormalFactsFromBranch(projection, actorID),
		cultivationIntervalIdentity(commandID, activity.ID), eventID,
	)
	if !ok {
		return CultivationResult{}, fmt.Errorf("%w: %s", ErrCultivationUnknownActor, actorID)
	}
	if advance.RefusalReason != "" {
		return CultivationResult{}, fmt.Errorf("%s", advance.RefusalReason)
	}
	if advance.MethodID != "" {
		activity.Conditions.MethodID = advance.MethodID
	}
	// The rules' answer replaces the named interval, in both directions.
	//
	// A character standing at a major-realm bottleneck cannot train the rest of
	// the year away, so the interval shrinks to what the rules really spent. An
	// authorised 筑基 attempt is seven days whatever interval the player named, so
	// it can also grow: the attempt is an operation with its own duration, not a
	// stretch of training the player chose the length of. In that case the named
	// training interval was spent in full and the attempt's remaining days are
	// the operation's own.
	elapsed := int64(math.Round(advance.ConsumedDays * 24 * 60 * 60))
	if elapsed < 0 {
		elapsed = 0
	}
	if breakthroughSeconds := int64(math.Round(advance.BreakthroughDays * 24 * 60 * 60)); breakthroughSeconds > elapsed {
		elapsed = breakthroughSeconds
	}
	days := float64(elapsed) / (24 * 60 * 60)

	// The advance writes the stored position; the settlement then derives every
	// displayed number from the stage key it wrote. Running both here is what keeps
	// the panel, the battle and the interval reading one set of numbers.
	settlementOps := advanceOps
	if len(advanceOps) > 0 {
		projected := cloneStoryState(projection.State)
		for _, op := range advanceOps {
			applyActorStateOp(projected, op)
		}
		settlementOps = append(settlementOps, settleXiuxianStoryState(projected, system, eventID)...)
	}
	settlementOps = normalizeActorStateOps(settlementOps)

	clockAfter, err := AdvanceStoryClock(clock, elapsed)
	if err != nil {
		return CultivationResult{}, err
	}
	activity.Activity = settleCultivationCandidate(plan.Candidate, named, elapsed)
	activity.ClockSeconds = clockAfter.Seconds
	// §7.1's "不存在尚未解除的突破恢复状态" needs the failure's recovery to be a fact
	// a later attempt can read. The activity carries it, so a retry, a restart and
	// a fork all see the same deadline without a second ledger.
	if advance.BreakthroughRolled && !advance.BreakthroughSucceeded && advance.BreakthroughRecoveryDays > 0 {
		recovery := int64(math.Round(advance.BreakthroughRecoveryDays * 24 * 60 * 60))
		activity.ConvalescenceUntil = clockAfter.Seconds + recovery
	}

	record := CultivationRecordEvent{
		V: schemaVersion, Type: StoryEventTypeCultivationRecord, ID: eventID,
		ParentID: branch.Head, BranchID: branchID, Ts: now,
		TurnID: strings.TrimSpace(req.TurnID), Cursor: eventID,
		CommandID: commandID, CommandHash: commandHash,
		WorldTime: strings.TrimSpace(req.WorldTime),
		Activity:  activity, Advance: advance,
		ClockBefore: clock, ClockAfter: clockAfter,
	}

	headID := eventID
	newEvents := []any{record}
	if len(settlementOps) > 0 {
		for _, op := range settlementOps {
			if err := validateActorStateOp(op); err != nil {
				return CultivationResult{}, err
			}
		}
		// The advance is a separate event parented to the record, for the same
		// reason the recovery's pool write is: an event whose ID is also its parent
		// closes the branch's parent chain into a cycle.
		deltaID := newID("sd")
		newEvents = append(newEvents, newStateDeltaEventWithActorOps(deltaID, eventID, branchID, now, nil, settlementOps))
		headID = deltaID
	}
	// The interval this stretch really spent closes every registered matter whose
	// instant it crossed, not only the activity that was being trained. The
	// activity's own clock record is already written above; these records hang off
	// it and carry no clock of their own, so the branch's instant stays the one the
	// training settled.
	closed, headID := storyIntervalEvents(projection, system, branchID, headID, now, "", actorID, clock, clockAfter)
	newEvents = append(newEvents, closed...)

	branch.Head = headID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, newEvents...); err != nil {
		return CultivationResult{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return CultivationResult{}, err
	}
	return CultivationResult{
		EventID: eventID, ActivityID: activity.ID, ActorID: actorID,
		Activity: activity, Advance: advance, ClockAfter: clockAfter,
		Elapsed: elapsed, Days: days,
		Eligibility: eligibility, SettlementOps: settlementOps,
		Completed:          activity.Activity.Status == storytime.ActivityCompleted,
		PendingNodeIDs:     append([]string{}, activity.Activity.PendingNodeIDs...),
		ElapsedDescription: DescribeElapsed(elapsed),
	}, nil
}

// settleCultivationCandidate rewrites the planned activity to the interval the
// rules really spent.
//
// PlanActivity settles the whole interval the activity owes; the rules may spend
// less of it, because a major-realm bottleneck turns the rest into a decision
// rather than into training. The activity has to carry that truth, or the next
// continuation would aim at a target that includes time the character never sat
// through, and a reopened entry would claim a settled interval nobody paid.
//
// planned is the increment this call planned, not the activity's whole span: a
// continuation that resumed a half-settled activity plans only its remainder, and
// shortening by the span would make every continuation look incomplete.
//
// A shortened interval is a real node: §五's "可按原活动恢复" is the player
// deciding what to do at the bottleneck, and storytime requires a paused activity
// to name the node that stopped it. The node is synthetic — the stop is the rule
// table's own boundary, not a client claim — which is why it is named for the
// position rather than for a caller-supplied id.
func settleCultivationCandidate(candidate storytime.Activity, planned, elapsed int64) storytime.Activity {
	if elapsed >= planned {
		return candidate
	}
	if candidate.Mode != storytime.EffectiveDuration {
		return candidate
	}
	// The planned candidate counted the whole interval as settled. The rules spent
	// less, so the cumulative figure gives the shortfall back and the settled point
	// follows it: for an effective-duration activity the settled point is the
	// origin plus the effective time really accumulated.
	completed := candidate.CompletedEffective - storytime.Duration(planned-elapsed)
	if completed < 0 {
		completed = 0
	}
	if completed >= candidate.RequiredEffective {
		// The target was met before the boundary; the boundary is then not what
		// stopped the interval and the planned candidate stands.
		return candidate
	}
	settled, err := candidate.StartedAt.Add(completed)
	if err != nil {
		return candidate
	}
	candidate.CompletedEffective = completed
	candidate.LastSettledAt = settled
	candidate.Status = storytime.ActivityPaused
	candidate.PendingNodeIDs = []string{cultivationBoundaryNodeID}
	return candidate
}

// cultivationBoundaryNodeID names the rule table's own training boundary as the
// node that paused an interval.
const cultivationBoundaryNodeID = "rule:training_boundary"

// cultivationStopNodePrefix marks a stop the backend derived from a committed
// fact rather than one the caller supplied.
const cultivationStopNodePrefix = "story:"

// cultivationStopNodes collects the boundaries a training interval really has to
// stop at, from the branch's own committed facts.
//
// §B1 asks for the stop points to come from the backend's formal facts and
// forbids trusting a caller's own nodes/resolved list to skip them. Everything
// here is read off the projection: the event definitions are the pack's, the
// participation record is committed, and the clock is the branch's own axis. A
// caller that sends no nodes therefore still stops where the story really does,
// and a caller that sends a resolved list cannot make an event disappear.
//
// Only boundaries that already have a real formal instant are collected. An
// accepted task's agreed deadline is one: it is stamped from the committed clock
// when the task is taken, so it is a moment the world really reaches rather than
// prose. A character's lifespan is the other, once the story has frozen a
// calendar and the pack stated an age: boundary carries that instant, and nil
// means the story has no answerable lifespan rather than an infinite one.
func cultivationStopNodes(branch *storyBranchProjection, from, to storytime.Instant, boundary *storyLifespanBoundary) []storytime.Node {
	if branch == nil || to <= from {
		return nil
	}
	nodes := make([]storytime.Node, 0, 4)
	for eventID, instance := range branch.PublicEvents {
		definition, ok := worldEventDefinition(branch.worldContent(), eventID)
		if !ok || definition.Version != instance.EventVersion {
			// A definition that changed underneath is not re-read under new
			// meaning: the instance is pinned to the version it was raised against.
			continue
		}
		if instance.Withdrawn {
			// The character already stepped back from this situation. Its later
			// stages are not a decision they owe.
			continue
		}
		for _, stage := range definition.Stages {
			at := storytime.Instant(stage.OpenAfterSeconds)
			if at <= from || at >= to {
				continue
			}
			if int64(at) <= instance.JoinedAtSeconds {
				// The character already stood at or past this stage when they
				// joined, so it is not a change they would learn of later.
				continue
			}
			nodes = append(nodes, storytime.Node{
				ID: cultivationStopNodePrefix + eventID + ":" + stage.ID,
				At: at,
				// A new stage of an event the character already took part in is a
				// change in what they can do about it, which the design's stop
				// table lists beside a real resource change: the interval settles
				// up to the moment the situation moved and the player decides from
				// there.
				Kind:              storytime.ResourceNode,
				PublicDescription: stage.Title,
			})
		}
	}
	// An accepted task's agreed instant is a decision point the same way an
	// event's stage is: training past it does not merely delay the errand, it ends
	// the agreement. The node is derived from the instance's own stamped instant,
	// so a task taken without a limit contributes nothing and an old save whose
	// acceptance predates deadlines stays untouched.
	for _, instance := range branch.Quests {
		if instance.Status != quest.StatusAccepted {
			continue
		}
		deadline, has := instance.Deadline()
		if !has {
			continue
		}
		at := storytime.Instant(deadline)
		if at <= from || at >= to {
			continue
		}
		// The label is the task's own authored title, so the warning names the
		// matter the way the player took it on rather than by an internal id. A
		// definition that no longer matches the instance is skipped, because a
		// task the pack has moved past is not one the player can be told about.
		definition, ok := worldQuestDefinition(branch.worldContent(), instance.TaskID)
		if !ok || definition.Version != instance.TaskVersion {
			continue
		}
		nodes = append(nodes, storytime.Node{
			ID: cultivationStopNodePrefix + instance.TaskID + ":deadline",
			At: at,
			// Crossing the instant ends the agreement rather than costing a
			// resource, but the stop table's decision-point kinds are what the
			// planner understands, and the player's decision — stop now or let the
			// promise run out — is the same shape as any other interruption.
			Kind:              storytime.ResourceNode,
			PublicDescription: "「" + definition.Title + "」的约定到期",
		})
	}
	// The protagonist's own lifespan edge is the third kind, and it is the only
	// one that is a decision rather than a resource change: §4 asks for the
	// situation to be put in front of the player before the interval runs past
	// it, so the stop is where they choose to stop or continue.
	if node, ok := lifespanStopNode(boundary, from, to); ok {
		nodes = append(nodes, node)
	}
	return nodes
}

// pausedCultivationActivity returns the activity a further training turn
// continues, if the actor left one unfinished.
//
// §7 step 4 saves the unfinished activity before the choice is narrated, and
// step 5 continues it afterwards. The activity is found on the branch rather than
// named by the caller: the model has no way to know the id, and a caller that
// could name one could resume an activity that is not theirs.
//
// The latest origin wins, because that is the intent most recently interrupted.
// A character can only be at one real boundary at a time, so there is normally
// exactly one candidate.
func pausedCultivationActivity(branch *storyBranchProjection, actorID string) (CultivationActivity, bool) {
	if branch == nil {
		return CultivationActivity{}, false
	}
	normalized := normalizeStatePanelActorID(actorID)
	var best CultivationActivity
	found := false
	for _, activity := range branch.Cultivations {
		if normalized != "" && normalizeStatePanelActorID(activity.ActorID) != normalized {
			continue
		}
		switch activity.Activity.Status {
		case storytime.ActivityPaused, storytime.ActivityAwaitingDecision:
		default:
			continue
		}
		if activityRemainingSeconds(activity) <= 0 {
			continue
		}
		if found && activity.ClockSeconds <= best.ClockSeconds {
			continue
		}
		best, found = activity, true
	}
	return best, found
}

// activityRemainingSeconds is how much of the activity's own target is still
// owed. It is the one figure a continuation may still settle.
func activityRemainingSeconds(activity CultivationActivity) int64 {
	remaining := int64(activity.Activity.RequiredEffective) - int64(activity.Activity.CompletedEffective)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// continuePausedCultivation folds a further training turn into the unfinished
// activity instead of opening a new one.
//
// §7 step 5 continues along the original activity's remaining target, and the
// turn is that continuation whenever the actor left one paused. The named days
// are the training the player asked for in *this* turn, so the segment planned is
// the shorter of the two: a player who named ten years but only owes two days of
// the interrupted seclusion trains those two days, and one who names a day keeps
// the rest of the activity paused for later.
//
// The conditions come from the new turn. §7 step 5 says the continuation takes
// the new conditions, and the equipment, the site and the support stock are all
// read from the committed state, so nothing is carried that the character no
// longer really has.
//
// The activity resumes at the branch's current instant, which is where the story
// stood when the interruption ended. The interval between the pause and this turn
// is not training: the character was doing whatever stopped them.
func continuePausedCultivation(
	branch *storyBranchProjection,
	activity CultivationActivity,
	input *TurnCultivationInput,
) *TurnCultivationInput {
	if branch == nil || input == nil {
		return nil
	}
	remaining := activityRemainingSeconds(activity)
	if remaining <= 0 {
		return nil
	}
	named := int64(math.Round(input.Days * 24 * 60 * 60))
	if named <= 0 || named > remaining {
		named = remaining
	}
	continued := *input
	// Continuing an interval does not silently replace its selected method with
	// whichever active method now has the highest coefficient. An explicit new
	// choice is allowed and is checked against the current committed abilities.
	if strings.TrimSpace(continued.MethodID) == "" {
		continued.MethodID = activity.Conditions.MethodID
	}
	continued.Days = float64(named) / (24 * 60 * 60)
	return &continued
}

// cultivationStopNodesWithin is cultivationStopNodes for an interval of the
// given length starting at from.
func cultivationStopNodesWithin(branch *storyBranchProjection, from storytime.Instant, named int64, boundary *storyLifespanBoundary) []storytime.Node {
	if branch == nil || named <= 0 {
		return nil
	}
	to, err := from.Add(storytime.Duration(named))
	if err != nil {
		return nil
	}
	return cultivationStopNodes(branch, from, to, boundary)
}

// mergeCultivationNodes keeps a caller's nodes but never lets them replace the
// backend's own.
//
// The interval entry point accepts a node list because the caller may know about
// a decision the backend does not model. It may not, however, submit a resolved
// node under the backend's namespace: that would be the "客户端自报nodes/resolved
// 而跳过事项" §B1 forbids, and it would let one request walk a training interval
// past a stage that really arrived.
func mergeCultivationNodes(derived, caller []storytime.Node) []storytime.Node {
	if len(derived) == 0 {
		return caller
	}
	merged := make([]storytime.Node, 0, len(derived)+len(caller))
	seen := make(map[string]struct{}, len(derived))
	for _, node := range derived {
		seen[node.ID] = struct{}{}
		merged = append(merged, node)
	}
	for _, node := range caller {
		if _, clash := seen[node.ID]; clash {
			continue
		}
		if strings.HasPrefix(node.ID, cultivationStopNodePrefix) {
			continue
		}
		merged = append(merged, node)
	}
	return merged
}

// cultivationBoundaryWithin returns the first committed boundary a training
// interval would run past.
//
// §7 step 3 settles only up to the node that really affects the calculation or
// the player's decision, and §B1 requires those nodes to come from the backend's
// own facts. The nodes are collected from the branch, never from the caller, so a
// request that sends no nodes still stops where the story really does and one
// that sends a resolved list cannot walk past a stage that is still owed.
func cultivationBoundaryWithin(branch *storyBranchProjection, from storytime.Instant, named int64, boundary *storyLifespanBoundary) (storytime.Node, bool) {
	nodes := cultivationStopNodesWithin(branch, from, named, boundary)
	if len(nodes) == 0 {
		return storytime.Node{}, false
	}
	earliest := nodes[0]
	for _, node := range nodes[1:] {
		if node.At < earliest.At {
			earliest = node
		}
	}
	return earliest, true
}

// turnCultivationPlan is the interval a training turn really asks the rules to
// integrate.
//
// It exists because three separate things narrow the interval before the advance
// runs: the activity a boundary paused, a committed boundary inside the named
// stretch, and the rules' own major-realm bottleneck (which only the advance
// itself can see). The first two have to be decided before the advance, so the
// turn does not train past a moment the story stops at.
type turnCultivationPlan struct {
	// input is the interval handed to the rules, already capped by the paused
	// activity's remainder and by the first committed boundary.
	input TurnCultivationInput
	// continued is the paused activity this turn continues, nil for a new one.
	continued *CultivationActivity
	// named is the interval the player asked for in this turn, in seconds, before
	// a boundary cut it. A paused activity keeps this as its target, so the
	// remainder is exactly what the player still wants.
	named int64
	// boundary names the committed stop that cut the interval; empty when the
	// interval ran its named length.
	boundary string
}

// planTurnCultivation decides the interval a training turn really settles.
//
// boundary is the protagonist's resolved lifespan edge, or nil when the story
// cannot answer that question. It is passed in rather than derived here because
// deriving it needs the story's frozen calendar and the protagonist's established
// age, and neither lives on the branch projection.
func planTurnCultivation(branch *storyBranchProjection, actorID string, input *TurnCultivationInput, boundary *storyLifespanBoundary) turnCultivationPlan {
	plan := turnCultivationPlan{}
	if branch == nil || input == nil {
		return plan
	}
	plan.input = *input
	// A turn that authorises a major breakthrough is that operation, not more of
	// the interrupted seclusion. Folding it into the paused activity would count
	// the attempt's own days against the training remainder and then republish the
	// remainder as if nothing had happened.
	if strings.TrimSpace(input.AttemptBreakthrough) == "" {
		if paused, ok := pausedCultivationActivity(branch, actorID); ok {
			if continued := continuePausedCultivation(branch, paused, input); continued != nil {
				plan.input = *continued
				plan.continued = &paused
			}
		}
	}
	plan.named = int64(math.Round(plan.input.Days * 24 * 60 * 60))
	if plan.named <= 0 {
		return plan
	}
	from := storytime.Instant(branch.clockSeconds())
	node, ok := cultivationBoundaryWithin(branch, from, plan.named, boundary)
	if !ok {
		return plan
	}
	cut := int64(node.At) - int64(from)
	if cut <= 0 || cut >= plan.named {
		return plan
	}
	// The turn trains up to the boundary and no further; the rest of the named
	// interval stays owed on the activity the record publishes.
	plan.input.Days = float64(cut) / (24 * 60 * 60)
	plan.boundary = node.ID
	return plan
}

// cultivationPauseNodeIDs names what the paused interval is still waiting on.
//
// The rule table's own bottleneck and a committed event stage are different
// reasons to stop, and an interval can meet both: it trains up to the stage, then
// the stage's own moment arrives before the training target does. Both are listed
// so the player sees the situation they are actually in.
func cultivationPauseNodeIDs(plan turnCultivationPlan, stoppedByRules bool) []string {
	ids := make([]string, 0, 2)
	if plan.boundary != "" {
		ids = append(ids, plan.boundary)
	}
	if stoppedByRules || len(ids) == 0 {
		ids = append(ids, cultivationBoundaryNodeID)
	}
	return ids
}

// turnCultivationClockRecord publishes the interval a training turn really spent.
//
// A Turn carries no clock: its projection cannot move one, so a turn that trained
// would grow the character while the world stood still — the defect this record
// exists to close. The record is a side record like the interval entry's, parented
// after whatever the turn left at the branch head, so it folds exactly once and a
// regenerated turn's record no longer matches the path.
//
// It carries no operations. The turn's own StateDelta already holds everything the
// advance wrote, and writing the position twice would let a replay apply it twice.
//
// The activity is completed rather than paused: a turn's interval is over when the
// turn ends. A character stopped at a major-realm bottleneck spent only the part
// before the boundary, which is why the elapsed figure comes from the advance
// rather than from the interval the model named.
//
// A boundary the player must decide about is the one exception. §7 step 4 asks the
// story to save the occurred interval *and* the unfinished activity before it
// narrates the choice, so the turn publishes a paused activity that still owes the
// rest of the interval the player named. Without it a seclusion of ten years at a
// full threshold would leave no record of the intent at all, and the player's next
// line would be read as a brand new request rather than as a continuation.
//
// The paused record is deliberately narrow: it needs a real named interval and a
// boundary the rules themselves reported. A refused breakthrough attempt reaches
// neither — the advance consumes nothing and reports no boundary — so it still
// publishes nothing and spends no time.
func turnCultivationClockRecord(
	projection *storyBranchProjection,
	parentID, storyID, branchID string,
	turn TurnEvent,
	advance CultivationAdvanceResult,
	system StoryDirectorActorStateSystem,
	plan turnCultivationPlan,
	now string,
) *CultivationRecordEvent {
	// Training progress and clock movement are different figures. A turn that both
	// trains and attempts a breakthrough moves the clock by the longer of the two,
	// but only the training part is progress on the training activity.
	consumed := int64(math.Round(advance.ConsumedDays * 24 * 60 * 60))
	elapsed := consumed
	if breakthrough := int64(math.Round(advance.BreakthroughDays * 24 * 60 * 60)); breakthrough > elapsed {
		elapsed = breakthrough
	}
	// The target is the interval the player named before any boundary cut it, so
	// the remainder the activity keeps is exactly the training still owed.
	named := plan.named
	if named <= 0 {
		named = int64(math.Round(advance.Days * 24 * 60 * 60))
	}
	// A pause is only honest when a pure training turn really stopped short of the
	// interval the player named, either at the rules' own bottleneck or at a
	// committed event stage. A turn that rolled a breakthrough is a record of that
	// attempt, and a refused attempt has Days == 0 and consumes nothing: neither has
	// a training remainder to preserve.
	paused := (advance.StoppedAtBoundary || plan.boundary != "") && !advance.BreakthroughRolled && named > 0 && consumed < named
	if elapsed <= 0 && !paused {
		return nil
	}
	clock, err := branchStoryClock(projection, advance.ActorID, system)
	if err != nil {
		return nil
	}
	clockAfter, err := AdvanceStoryClock(clock, elapsed)
	if err != nil {
		return nil
	}
	instant := clock.Instant()
	settled, err := instant.Add(storytime.Duration(elapsed))
	if err != nil {
		return nil
	}
	activityID := "clt-" + turn.ID
	startedAt := instant
	conditions := cultivationConditionsFromInput(turn.TurnResult.Cultivation)
	if advance.MethodID != "" {
		conditions.MethodID = advance.MethodID
	}
	continued := plan.continued
	if continued != nil {
		// The continuation inherits the interrupted activity's identity and origin,
		// so it overwrites that activity rather than adding a second one and the
		// branch reads one uninterrupted intent. Only the target stays the same:
		// the progress is cumulative across the whole interruption.
		activityID = continued.ID
		startedAt = continued.Activity.StartedAt
	}
	activity := CultivationActivity{
		ID:      activityID,
		ActorID: advance.ActorID,
		Activity: storytime.Activity{
			ID: activityID, StoryID: storyID, BranchID: branchID,
			SourceID: advance.ActorID, Mode: storytime.EffectiveDuration,
			Status:    storytime.ActivityCompleted,
			StartedAt: startedAt, LastSettledAt: settled,
			RequiredEffective:  storytime.Duration(elapsed),
			CompletedEffective: storytime.Duration(elapsed),
		},
		Conditions:   conditions,
		Eligibility:  cultivation.RecoveryQualified,
		ClockSeconds: clock.Seconds,
	}
	if continued != nil {
		progress := int64(continued.Activity.CompletedEffective) + consumed
		target := int64(continued.Activity.RequiredEffective)
		if progress > target {
			progress = target
		}
		activity.Activity.RequiredEffective = storytime.Duration(target)
		activity.Activity.CompletedEffective = storytime.Duration(progress)
		if progress >= target {
			activity.Activity.Status = storytime.ActivityCompleted
			activity.Activity.PendingNodeIDs = nil
		} else {
			// The player named less than the activity still owed, so the rest stays
			// paused and owed exactly as before.
			activity.Activity.Status = storytime.ActivityPaused
			activity.Activity.PendingNodeIDs = cultivationPauseNodeIDs(plan, advance.StoppedAtBoundary)
		}
		activity.ClockSeconds = continued.ClockSeconds
	}
	if paused && continued == nil {
		// The activity keeps the whole named interval as its target and the training
		// it really did as its progress, so a continuation owes exactly the remainder.
		activity.Activity.Status = storytime.ActivityPaused
		activity.Activity.RequiredEffective = storytime.Duration(named)
		activity.Activity.CompletedEffective = storytime.Duration(consumed)
		activity.Activity.PendingNodeIDs = cultivationPauseNodeIDs(plan, advance.StoppedAtBoundary)
		// Nothing was spent, so the activity has not moved off its own origin. Its
		// last settled instant stays at the start rather than at the settled target,
		// which is the only instant the rules can honestly claim: the remainder is a
		// decision, not time that passed.
		if consumed == 0 {
			activity.Activity.LastSettledAt = instant
		}
	}
	// A turn that failed a breakthrough carries the same recovery an interval
	// entry would, so §7.1's precondition holds whichever path made the attempt.
	if advance.BreakthroughRolled && !advance.BreakthroughSucceeded && advance.BreakthroughRecoveryDays > 0 {
		activity.ConvalescenceUntil = clockAfter.Seconds + int64(math.Round(advance.BreakthroughRecoveryDays*24*60*60))
	}
	return &CultivationRecordEvent{
		V: schemaVersion, Type: StoryEventTypeCultivationRecord, ID: newID("cul"),
		ParentID: parentID, BranchID: branchID, Ts: now,
		TurnID: turn.ID, Cursor: "",
		CommandID: strings.TrimSpace(turn.AgentCommandID), CommandHash: strings.TrimSpace(turn.AgentCommitHash),
		Activity: activity, Advance: advance,
		ClockBefore: clock, ClockAfter: clockAfter,
	}
}

// cultivationRerollSource is the clock record a turn already committed, plus the
// event ID that record sits at.
//
// The ID is carried beside the decoded body because the body is a payload: the
// event's own identity lives on the envelope, and a replacement has to be
// chained onto it to fold.
type cultivationRerollSource struct {
	Record *CultivationRecordEvent
	ID     string
}

// cultivationRecordForTurn returns the clock record one turn committed.
//
// A reroll re-reads a turn's outcome, so it needs the interval that turn really
// spent. The record is the only place that interval is stored, and it is read
// from the branch path rather than from a process-local index so the answer
// survives a restart.
func cultivationRecordForTurn(path []StoryEventRecord, branchID, turnID string) (cultivationRerollSource, bool) {
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeCultivationRecord {
			continue
		}
		var event CultivationRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			continue
		}
		if event.BranchID != branchID || strings.TrimSpace(event.TurnID) != turnID {
			continue
		}
		return cultivationRerollSource{Record: &event, ID: record.Envelope.ID}, true
	}
	return cultivationRerollSource{}, false
}

// cultivationRerollProjection is the branch as it stood before the turn a reroll
// replaces.
//
// Re-deriving that turn's advance from the live projection would read the turn's
// own record back as a precondition of itself: a failed breakthrough's
// convalescence deadline would refuse the very attempt being re-derived, and the
// clock would already stand after the interval. Dropping the turn's own activity
// and rewinding to the clock it started from gives the re-derivation the
// position it originally ran against.
func cultivationRerollProjection(branch *storyBranchProjection, turnID string, source cultivationRerollSource) *storyBranchProjection {
	if branch == nil {
		return nil
	}
	clone := *branch
	clone.Cultivations = cloneCultivationActivities(branch.Cultivations)
	delete(clone.Cultivations, "clt-"+strings.TrimSpace(turnID))
	if source.Record != nil && source.Record.ClockBefore.Validate() == nil {
		clock := source.Record.ClockBefore
		clone.Clock = &clock
	}
	return &clone
}

// cultivationRerollClockRecord republishes a rerolled turn's clock record.
//
// A reroll replaces the rule outcome the record's advance was derived from, so
// the recorded growth has to be re-derived with it. What it does not do is
// re-spend the interval: the turn's authorised time already happened in the
// world, and a reroll is a second reading of the same events rather than a
// rewind of the calendar. When the superseded turn left a record, the
// replacement keeps that record's window verbatim and carries only the new
// outcome. When it left none, the turn spent no time, and none is charged here
// either.
//
// The replacement is chained onto whatever the branch head stands on at that
// point — the superseded record, or the turn itself — because that is the only
// position from which the projection's path rule folds it. It also reuses the
// superseded activity's ID, so the projection overwrites the old activity
// rather than keeping both and reading the old convalescence deadline.
func cultivationRerollClockRecord(
	projection *storyBranchProjection,
	system StoryDirectorActorStateSystem,
	turn TurnEvent,
	advance CultivationAdvanceResult,
	source cultivationRerollSource,
	now string,
) *CultivationRecordEvent {
	var clockBefore, clockAfter StoryClock
	var activity CultivationActivity
	parentID := turn.ID
	if source.Record != nil && source.Record.ClockBefore.Validate() == nil && source.Record.ClockAfter.Validate() == nil {
		clockBefore, clockAfter = source.Record.ClockBefore, source.Record.ClockAfter
		activity = source.Record.Activity
		parentID = source.ID
	} else {
		// No record to inherit: this reroll is the first reading that gives the
		// turn a spent interval, so it is charged now, exactly once.
		elapsed := int64(math.Round(advance.ConsumedDays * 24 * 60 * 60))
		if breakthrough := int64(math.Round(advance.BreakthroughDays * 24 * 60 * 60)); breakthrough > elapsed {
			elapsed = breakthrough
		}
		if elapsed <= 0 {
			return nil
		}
		clock, err := branchStoryClock(projection, advance.ActorID, system)
		if err != nil {
			return nil
		}
		clockAfter, err = AdvanceStoryClock(clock, elapsed)
		if err != nil {
			return nil
		}
		clockBefore = clock
		instant := clock.Instant()
		settled, err := instant.Add(storytime.Duration(elapsed))
		if err != nil {
			return nil
		}
		activityID := "clt-" + turn.ID
		activity = CultivationActivity{
			ID:      activityID,
			ActorID: advance.ActorID,
			Activity: storytime.Activity{
				ID: activityID, BranchID: turn.BranchID,
				SourceID: advance.ActorID, Mode: storytime.EffectiveDuration,
				Status:    storytime.ActivityCompleted,
				StartedAt: instant, LastSettledAt: settled,
				RequiredEffective:  storytime.Duration(elapsed),
				CompletedEffective: storytime.Duration(elapsed),
			},
			Eligibility: cultivation.RecoveryQualified,
		}
	}
	activity.ActorID = advance.ActorID
	activity.Conditions = cultivationConditionsFromInput(turn.TurnResult.Cultivation)
	if advance.MethodID != "" {
		activity.Conditions.MethodID = advance.MethodID
	}
	activity.ClockSeconds = clockAfter.Seconds
	// The deadline follows the re-derived outcome rather than being copied: a
	// reroll that turns a failure into a success must not leave the branch
	// convalescing from a wound that no longer happened.
	activity.ConvalescenceUntil = 0
	if advance.BreakthroughRolled && !advance.BreakthroughSucceeded && advance.BreakthroughRecoveryDays > 0 {
		activity.ConvalescenceUntil = clockAfter.Seconds + int64(math.Round(advance.BreakthroughRecoveryDays*24*60*60))
	}
	return &CultivationRecordEvent{
		V: schemaVersion, Type: StoryEventTypeCultivationRecord, ID: newID("cul"),
		ParentID: parentID, BranchID: turn.BranchID, Ts: now,
		TurnID: turn.ID, Cursor: "",
		CommandID: strings.TrimSpace(turn.AgentCommandID), CommandHash: strings.TrimSpace(turn.AgentCommitHash),
		Activity: activity, Advance: advance,
		ClockBefore: clockBefore, ClockAfter: clockAfter,
	}
}

// cultivationConditionsFromInput records the conditions a turn trained under, so
// a reopened entry reports the same intent the turn carried.
func cultivationConditionsFromInput(input *TurnCultivationInput) CultivationConditions {
	if input == nil {
		return CultivationConditions{}
	}
	return CultivationConditions{
		MethodID:            input.MethodID,
		Environment:         input.Environment,
		Resource:            input.Resource,
		AttemptBreakthrough: input.AttemptBreakthrough,
		AuxiliaryMedicine:   input.AuxiliaryMedicine,
		StableSite:          input.StableSite,
		QualifiedGuardian:   input.QualifiedGuardian,
	}
}

// The draw is derived from it rather than taken fresh, so a retried or refreshed
// request re-uses the same draw instead of re-rolling. §五's "不重抽已确定突破
// 结果" is that property. A continuation is a different command and therefore a
// genuinely new attempt, which is what a second authorised attempt is.
func cultivationIntervalIdentity(commandID, activityID string) DomainCommitIdentity {
	return DomainCommitIdentity{
		CommandID:   strings.TrimSpace(commandID),
		OperationID: strings.TrimSpace(activityID),
		Cycle:       1,
	}
}

// planCultivationActivity resolves the activity this request settles against.
//
// A new activity takes its origin from the branch's formal clock and its target
// from the named interval. A continuation resumes an activity that a real node
// stopped and aims at that activity's own target, which is what makes a partial
// interval followed by a continuation count only the remainder.
func (s *Store) planCultivationActivity(storyID, branchID string, projection *storyBranchProjection, actorID string, req CultivationRequest, clock StoryClock, eligibility cultivation.RecoveryEligibility) (CultivationActivity, error) {
	if activityID := strings.TrimSpace(req.ActivityID); activityID != "" {
		existing, found := projection.Cultivations[activityID]
		if !found {
			return CultivationActivity{}, fmt.Errorf("%w: %s", ErrCultivationUnknownActivity, activityID)
		}
		if normalizeStatePanelActorID(existing.ActorID) != actorID {
			return CultivationActivity{}, fmt.Errorf("%w: activity %s belongs to %s", ErrCultivationUnknownActivity, existing.ID, existing.ActorID)
		}
		if existing.Eligibility != eligibility {
			// A change of safety condition starts a new segment rather than
			// extending this one. Extending would let a stretch spent in battle be
			// spliced onto one spent in seclusion and counted as continuous.
			return CultivationActivity{}, fmt.Errorf("%w: %s -> %s", cultivation.ErrRecoveryEligibilityChanged, existing.Eligibility, eligibility)
		}
		switch existing.Activity.Status {
		case storytime.ActivityPaused, storytime.ActivityAwaitingDecision:
		default:
			// A finished interval is finished. Extending it would let one request
			// earn two full intervals, so more training is a new activity instead.
			return CultivationActivity{}, fmt.Errorf("%w: activity %s is %s", ErrCultivationActivityFinished, existing.ID, existing.Activity.Status)
		}
		// §五's "沿原活动剩余目标与新条件续接": a continuation keeps the target and
		// takes the new conditions when the player named them, and otherwise keeps
		// the ones the segment started under.
		conditions := existing.Conditions
		if req.Conditions != nil {
			conditions = *req.Conditions
			if strings.TrimSpace(conditions.MethodID) == "" {
				conditions.MethodID = existing.Conditions.MethodID
			}
		}
		// Resuming goes through the storytime rules' own resume rather than
		// rewinding the activity's settled point to where it paused. The interval
		// between the pause and this call is not training: the character was doing
		// whatever stopped them.
		resumed, err := storytime.ResumeActivity(existing.Activity, clock.Instant())
		if err != nil {
			return CultivationActivity{}, err
		}
		activity := existing
		activity.Activity = resumed
		activity.Conditions = conditions
		activity.ClockSeconds = clock.Seconds
		return activity, nil
	}

	seconds, err := req.Interval.Seconds(projection.cultivationRules())
	if err != nil {
		return CultivationActivity{}, err
	}
	if eligibility != cultivation.RecoveryQualified {
		return CultivationActivity{}, fmt.Errorf("%w: %s", cultivation.ErrRecoveryIneligible, eligibility)
	}
	conditions := CultivationConditions{}
	if req.Conditions != nil {
		conditions = *req.Conditions
	}
	activityID := newID("clt")
	instant := clock.Instant()
	activity := storytime.Activity{
		ID: activityID, StoryID: strings.TrimSpace(storyID), BranchID: branchID,
		SourceID: actorID, Mode: storytime.EffectiveDuration, Status: storytime.ActivityRunning,
		StartedAt: instant, LastSettledAt: instant,
		RequiredEffective: storytime.Duration(seconds),
	}
	return CultivationActivity{
		ID: activityID, ActorID: actorID,
		Activity: activity, Conditions: conditions, Eligibility: eligibility,
		ClockSeconds: clock.Seconds,
	}, nil
}

// cultivationRecordForCommand looks for the interval a previous attempt
// committed under this command ID. It scans the branch's own recent records
// rather than trusting a process-local index, so the answer survives a restart.
//
// A found command ID with a different hash is a conflict: the caller reused one
// identity for two different intervals, and either answer would be wrong.
func cultivationRecordForCommand(records []StoryEventRecord, branchID, commandID, commandHash string) (CultivationResult, bool, error) {
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeCultivationRecord {
			continue
		}
		var event CultivationRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return CultivationResult{}, false, fmt.Errorf("decode committed cultivation record: %w", err)
		}
		if event.BranchID != branchID || strings.TrimSpace(event.CommandID) != commandID {
			continue
		}
		if strings.TrimSpace(event.CommandHash) != commandHash {
			return CultivationResult{}, false, fmt.Errorf("%w: command_id=%q existing_hash=%q requested_hash=%q", ErrCultivationCommandConflict, commandID, event.CommandHash, commandHash)
		}
		elapsed := event.ClockAfter.Seconds - event.ClockBefore.Seconds
		return CultivationResult{
			EventID: event.ID, ActivityID: event.Activity.ID, ActorID: event.Activity.ActorID,
			Activity: event.Activity, Advance: event.Advance,
			ClockAfter:         event.ClockAfter,
			Elapsed:            elapsed,
			Days:               float64(elapsed) / (24 * 60 * 60),
			Eligibility:        event.Activity.Eligibility,
			Completed:          event.Activity.Activity.Status == storytime.ActivityCompleted,
			PendingNodeIDs:     append([]string{}, event.Activity.Activity.PendingNodeIDs...),
			Replayed:           true,
			ElapsedDescription: DescribeElapsed(elapsed),
		}, true, nil
	}
	return CultivationResult{}, false, nil
}

// applyCultivationRecord merges a side record into the branch projection. It is
// the only place a cultivation activity and the formal clock are written during a
// rebuild, so a replay reproduces the time and the activity that were live when
// the record was committed.
func applyCultivationRecord(branch *storyBranchProjection, event CultivationRecordEvent) error {
	if err := event.Activity.Validate(); err != nil {
		return err
	}
	if err := event.ClockAfter.Validate(); err != nil {
		return err
	}
	if event.ClockBefore.Validate() != nil {
		return fmt.Errorf("修炼记录缺少合法的起始时刻 / a cultivation record needs a valid clock before it")
	}
	// The clock only moves forward. A record that appears to move it backwards is
	// a replayed fork or a corrupt row, and accepting it would let a branch's
	// formal time run in reverse.
	if event.ClockAfter.Seconds < branch.clockSeconds() {
		return fmt.Errorf("%w: cultivation record moves the story clock backwards", storytime.ErrTimeReversal)
	}
	if branch.Cultivations == nil {
		branch.Cultivations = map[string]CultivationActivity{}
	}
	branch.Cultivations[event.Activity.ID] = event.Activity
	clock := event.ClockAfter
	branch.Clock = &clock
	return nil
}

// validateCultivationRecordEvent rejects a record that could not have been
// honestly attributed. A malformed row is an error rather than a silent drop,
// because a dropped record is an interval the character never spent.
func validateCultivationRecordEvent(event CultivationRecordEvent) error {
	if strings.TrimSpace(event.BranchID) == "" {
		return fmt.Errorf("修炼记录缺少 branch_id / a cultivation record requires a branch")
	}
	return event.Activity.Validate()
}

// cloneCultivationActivities copies the activity map so one branch's cultivation
// cannot be mutated through an alias into another.
func cloneCultivationActivities(activities map[string]CultivationActivity) map[string]CultivationActivity {
	if len(activities) == 0 {
		return nil
	}
	cloned := make(map[string]CultivationActivity, len(activities))
	for id, activity := range activities {
		cloned[id] = activity
	}
	return cloned
}

// cultivationConvalescing reports whether a failed breakthrough's recovery is
// still running at the branch's current instant.
//
// §7.1 makes "不存在尚未解除的突破恢复状态" a precondition for a new attempt, and
// §7.2 prices the recovery per target realm. The recovery is written onto the
// activity the failed attempt produced, so this reader asks the same committed
// record a replay, a restart and a fork all inherit: a branch that rewound past
// the failure carries the earlier activity and with it no outstanding recovery.
//
// The latest deadline wins. Two failures cannot overlap, because the second is
// refused while the first is running, but reading the maximum rather than the
// first match keeps the answer right for a hand-edited journal too.
func cultivationConvalescing(branch *storyBranchProjection, actorID string) bool {
	if branch == nil {
		return false
	}
	now := branch.clockSeconds()
	normalized := normalizeStatePanelActorID(actorID)
	for _, activity := range branch.Cultivations {
		if activity.ConvalescenceUntil <= now {
			continue
		}
		if normalized != "" && normalizeStatePanelActorID(activity.ActorID) != normalized {
			continue
		}
		return true
	}
	return false
}

// storyTimeCheckpoint is a branch's formal time and the activities that spent it.
//
// The recovery, the cultivation interval and the journey are three kinds of
// activity on one axis, so they are rebuilt together. Folding only the
// recoveries would hand a fork an instant that predates a cultivation interval
// the same fork inherited the position from — time the character spent that the
// branch would then deny.
type storyTimeCheckpoint struct {
	LocalAffairs []LocalAffairState
	Clock        *StoryClock
	Recoveries   map[string]RecoveryActivity
	Cultivations map[string]CultivationActivity
	Journeys     map[string]exploration.JourneySnapshot
	Realms       map[string]RealmInstanceState
	Quests       map[string]quest.Instance
	PublicEvents map[string]publiquevent.Instance
	// NPCGrowth is each non-protagonist actor's growth watermark along the same
	// path. It travels with the clock because it is measured in that clock: a
	// fork that inherited the instant but not the watermark would pay for the
	// same stretch of life twice, or refuse the next one as overlapping.
	NPCGrowth map[string]npcgrowth.Snapshot
}

// storyTimeFromRecords rebuilds the formal clock and both kinds of activity
// along a branch path.
//
// It replaces the recovery-only rebuild and takes the same input, so a rewound
// branch can never keep a later instant than the one its position was restored
// to. The clock is the latest one any record on the path established, which is
// what makes time monotonic along the path rather than dependent on record order.
func storyTimeFromRecords(path []StoryEventRecord, metas ...StoryMeta) (storyTimeCheckpoint, error) {
	branch := &storyBranchProjection{}
	if len(metas) > 0 {
		branch.bindWorld(metas[0])
	}
	for _, record := range path {
		switch record.Envelope.Type {
		case StoryEventTypeStateDelta:
			var delta StateDeltaEvent
			if err := mapToStruct(record.Raw, &delta); err != nil {
				return storyTimeCheckpoint{}, err
			}
			if delta.ClockAdvance != nil {
				if err := applyTurnClockAdvance(branch, TurnEvent{User: delta.ClockAdvance.SourceText, ClockAdvance: delta.ClockAdvance}); err != nil {
					return storyTimeCheckpoint{}, err
				}
			}
		case StoryEventTypeTurn:
			var turn TurnEvent
			if err := mapToStruct(record.Raw, &turn); err != nil {
				return storyTimeCheckpoint{}, err
			}
			if err := applyTurnClockAdvance(branch, turn); err != nil {
				return storyTimeCheckpoint{}, err
			}
		case StoryEventTypeRecoveryRecord:
			var event RecoveryRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return storyTimeCheckpoint{}, err
			}
			if err := applyRecoveryRecord(branch, event); err != nil {
				return storyTimeCheckpoint{}, err
			}
		case StoryEventTypeCultivationRecord:
			var event CultivationRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return storyTimeCheckpoint{}, err
			}
			if err := applyCultivationRecord(branch, event); err != nil {
				return storyTimeCheckpoint{}, err
			}
		case StoryEventTypeJourneyRecord:
			var event JourneyRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return storyTimeCheckpoint{}, err
			}
			if err := applyJourneyRecord(branch, event); err != nil {
				return storyTimeCheckpoint{}, err
			}
		case StoryEventTypeRealmRecord:
			var event RealmRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return storyTimeCheckpoint{}, err
			}
			if err := applyRealmRecord(branch, event); err != nil {
				return storyTimeCheckpoint{}, err
			}
		case StoryEventTypeQuestRecord:
			var event QuestRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return storyTimeCheckpoint{}, err
			}
			if err := applyQuestRecord(branch, event); err != nil {
				return storyTimeCheckpoint{}, err
			}
		case StoryEventTypePublicEventRecord:
			var event PublicEventRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return storyTimeCheckpoint{}, err
			}
			if err := applyPublicEventRecord(branch, event); err != nil {
				return storyTimeCheckpoint{}, err
			}
		case StoryEventTypeNPCGrowthRecord:
			var event NPCGrowthRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return storyTimeCheckpoint{}, err
			}
			if err := applyNPCGrowthRecord(branch, event); err != nil {
				return storyTimeCheckpoint{}, err
			}
		case StoryEventTypeLocalAffairRecord:
			var event LocalAffairRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return storyTimeCheckpoint{}, err
			}
			if err := applyLocalAffairRecord(branch, event); err != nil {
				return storyTimeCheckpoint{}, err
			}
		case StoryEventTypeCooperationRecord:
			var event CooperationRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return storyTimeCheckpoint{}, err
			}
			if err := applyCooperationRecordClock(branch, event); err != nil {
				return storyTimeCheckpoint{}, err
			}
		case StoryEventTypeCraftRecord:
			var event CraftRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return storyTimeCheckpoint{}, err
			}
			if err := applyCraftRecord(branch, event); err != nil {
				return storyTimeCheckpoint{}, err
			}
		}
	}
	return storyTimeCheckpoint{
		LocalAffairs: cloneLocalAffairs(branch.LocalAffairs),
		Clock:        cloneStoryClock(branch.Clock),
		Recoveries:   cloneRecoveryActivities(branch.Recoveries),
		Cultivations: cloneCultivationActivities(branch.Cultivations),
		Journeys:     cloneJourneySnapshots(branch.Journeys),
		Realms:       cloneRealmInstances(branch.Realms),
		Quests:       cloneQuestInstances(branch.Quests),
		PublicEvents: clonePublicEventInstances(branch.PublicEvents),
		NPCGrowth:    cloneNPCGrowthSnapshots(branch.NPCGrowth),
	}, nil
}

// storyTimeForStoryAncestorsLocked rebuilds the formal clock and both kinds of
// activity over a set of ancestor records.
//
// It mirrors combatsForStoryAncestorsLocked and takes the same input, so a fork
// of an older turn restores the instant and the activities that belonged to that
// turn rather than the ones live now.
//
// Every record it folds is a Turn's own side record: a training interval, a rest,
// a walk, a task decision, a participation, a realm action, an NPC's growth or a
// craft batch. The Turn publishes each of them *after* itself, so an ancestor cut
// at the Turn excludes them by envelope while the Turn's own pool writes stay in.
// They therefore travel with the turn they name — see belongsToStoryAncestorPath.
// Without that, a fork of a training turn kept the growth and lost the time, and
// a fork of a walk kept the arrival and lost the road.
func (s *Store) storyTimeForStoryAncestorsLocked(handle *storyJournalHandle, ancestorIDs map[string]bool) (storyTimeCheckpoint, error) {
	branch := &storyBranchProjection{}
	branch.bindWorld(handle.projection.Meta)
	if err := scanStoryEventsLocked(handle, func(record StoryEventRecord) error {
		switch record.Envelope.Type {
		case StoryEventTypeStateDelta:
			// Independent adventure battle exchanges publish time on their state
			// delta, not a narrative Turn. Only ancestor deltas belong in this cut.
			if !ancestorIDs[record.Envelope.ID] {
				return nil
			}
			var delta StateDeltaEvent
			if err := mapToStruct(record.Raw, &delta); err != nil {
				return err
			}
			if delta.ClockAdvance != nil {
				return applyTurnClockAdvance(branch, TurnEvent{User: delta.ClockAdvance.SourceText, ClockAdvance: delta.ClockAdvance})
			}
		case StoryEventTypeTurn:
			if !ancestorIDs[record.Envelope.ID] {
				return nil
			}
			var turn TurnEvent
			if err := mapToStruct(record.Raw, &turn); err != nil {
				return err
			}
			return applyTurnClockAdvance(branch, turn)
		case StoryEventTypeRecoveryRecord:
			var event RecoveryRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return err
			}
			if !belongsToStoryAncestorPath(ancestorIDs, record.Envelope.ID, event.TurnID) {
				return nil
			}
			return applyRecoveryRecord(branch, event)
		case StoryEventTypeCultivationRecord:
			var event CultivationRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return err
			}
			if !belongsToStoryAncestorPath(ancestorIDs, record.Envelope.ID, event.TurnID) {
				return nil
			}
			return applyCultivationRecord(branch, event)
		case StoryEventTypeJourneyRecord:
			var event JourneyRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return err
			}
			if !belongsToStoryAncestorPath(ancestorIDs, record.Envelope.ID, event.TurnID) {
				return nil
			}
			return applyJourneyRecord(branch, event)
		case StoryEventTypeRealmRecord:
			var event RealmRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return err
			}
			if !belongsToStoryAncestorPath(ancestorIDs, record.Envelope.ID, event.TurnID) {
				return nil
			}
			return applyRealmRecord(branch, event)
		case StoryEventTypeQuestRecord:
			var event QuestRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return err
			}
			if !belongsToStoryAncestorPath(ancestorIDs, record.Envelope.ID, event.TurnID) {
				return nil
			}
			return applyQuestRecord(branch, event)
		case StoryEventTypePublicEventRecord:
			var event PublicEventRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return err
			}
			if !belongsToStoryAncestorPath(ancestorIDs, record.Envelope.ID, event.TurnID) {
				return nil
			}
			return applyPublicEventRecord(branch, event)
		case StoryEventTypeNPCGrowthRecord:
			var event NPCGrowthRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return err
			}
			if !belongsToStoryAncestorPath(ancestorIDs, record.Envelope.ID, event.TurnID) {
				return nil
			}
			return applyNPCGrowthRecord(branch, event)
		case StoryEventTypeLocalAffairRecord:
			var event LocalAffairRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return err
			}
			if !belongsToStoryAncestorPath(ancestorIDs, record.Envelope.ID, event.TurnID) {
				return nil
			}
			return applyLocalAffairRecord(branch, event)
		case StoryEventTypeCooperationRecord:
			var event CooperationRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return err
			}
			if !belongsToStoryAncestorPath(ancestorIDs, record.Envelope.ID, event.TurnID) {
				return nil
			}
			return applyCooperationRecordClock(branch, event)
		case StoryEventTypeCraftRecord:
			var event CraftRecordEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				return err
			}
			if !belongsToStoryAncestorPath(ancestorIDs, record.Envelope.ID, event.TurnID) {
				return nil
			}
			return applyCraftRecord(branch, event)
		}
		return nil
	}); err != nil {
		return storyTimeCheckpoint{}, err
	}
	return storyTimeCheckpoint{
		LocalAffairs: cloneLocalAffairs(branch.LocalAffairs),
		Clock:        cloneStoryClock(branch.Clock),
		Recoveries:   cloneRecoveryActivities(branch.Recoveries),
		Cultivations: cloneCultivationActivities(branch.Cultivations),
		Journeys:     cloneJourneySnapshots(branch.Journeys),
		Realms:       cloneRealmInstances(branch.Realms),
		Quests:       cloneQuestInstances(branch.Quests),
		PublicEvents: clonePublicEventInstances(branch.PublicEvents),
		NPCGrowth:    cloneNPCGrowthSnapshots(branch.NPCGrowth),
	}, nil
}

// CultivationState reads one branch's formal clock and cultivation activities.
//
// It is the read a cultivation entry opens on: a window that was closed and
// reopened has to find the same activity at the same settled interval, and a
// branch that never cultivated has to read as having no activity rather than as
// an error.
func (s *Store) CultivationState(storyID, branchID string) (StoryClock, map[string]CultivationActivity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return StoryClock{}, nil, err
	}
	defer release()
	if _, err := s.refreshStoryJournalLocked(storyID, true); err != nil {
		return StoryClock{}, nil, err
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return StoryClock{}, nil, err
	}
	if projection.Clock == nil {
		return NewStoryClock(storySceneText(projection.State, storySceneTimeField)), nil, nil
	}
	activities := make(map[string]CultivationActivity, len(projection.Cultivations))
	for id, activity := range projection.Cultivations {
		activities[id] = activity
	}
	return *projection.Clock, activities, nil
}

// CultivationPosition is one character's live position on the cultivation rules'
// own axis, read without writing anything.
//
// The cultivation entry needs it before it can offer an interval: §五 has the
// player name a duration and the rules decide the outcome, so the entry can only
// show what an interval would reach by reading the committed position rather than
// by guessing at it or by asking a model. Realm is the same stage key the advance
// result reports, so the two can be compared without a translation step.
type CultivationPosition struct {
	ActorID string
	Realm   string
	// Progress is the banked position toward the next small stage.
	Progress float64
	// StoppedAtBoundary reports that the character is at the top of the
	// implemented stage table, so ordinary training can no longer advance them and
	// only an authorised major breakthrough would.
	StoppedAtBoundary bool
}

// CultivationPosition reads one character's committed stage and banked progress.
//
// An unwritten stage reads as the rules' own starting point rather than as an
// error, exactly as the settlement path reads it: a story that has not written
// the field yet is at the default the schema declares.
func (s *Store) CultivationPosition(storyID, branchID, actorID string) (CultivationPosition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return CultivationPosition{}, err
	}
	defer release()
	if _, err := s.refreshStoryJournalLocked(storyID, true); err != nil {
		return CultivationPosition{}, err
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return CultivationPosition{}, err
	}
	resolved := normalizeRecoveryActorID(actorID)
	realm := xiuxianStoredStageKey(projection.State, resolved)
	// The stage key is the same one the advance result reports, and the boundary
	// finding comes from the rules rather than from comparing key text: a stage
	// table that grows a realm must not need this reader edited too.
	atBoundary := false
	if parsed, ok := cultivation.ParseStageKey(realm, projection.cultivationRules()); ok {
		// The boundary is the rules' own answer, not a comparison of stage text:
		// the last position before a major realm stops ordinary training exactly
		// as the terminal stage does, and only the rules know where those are.
		atBoundary = cultivation.AtTrainingBoundary(
			parsed,
			xiuxianStoredFloat(projection.State, resolved, xiuxianFieldRealmProgress),
		)
	}
	return CultivationPosition{
		ActorID:           resolved,
		Realm:             realm,
		Progress:          xiuxianStoredFloat(projection.State, resolved, xiuxianFieldRealmProgress),
		StoppedAtBoundary: atBoundary,
	}, nil
}
