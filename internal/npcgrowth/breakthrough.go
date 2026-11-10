package npcgrowth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"
	"strings"

	"ta/internal/cultivation"
)

const (
	StatusBreakthroughReady      Status = "breakthrough_ready"
	StatusBreakthroughPreparing  Status = "breakthrough_preparing"
	StatusBreakthroughSucceeded  Status = "breakthrough_succeeded"
	StatusBreakthroughFailed     Status = "breakthrough_failed"
	StatusPreparationInterrupted Status = "breakthrough_preparation_interrupted"
	StatusBreakthroughRecovering Status = "breakthrough_recovering"
	StatusRecoveryEnded          Status = "breakthrough_recovery_ended"
)

// BreakthroughState is the portion of the canonical actor snapshot needed to
// resume one NPC breakthrough attempt without a second persistence store.
type BreakthroughState struct {
	CurrentHealth        int                      `json:"current_health"`
	CurrentMana          int                      `json:"current_mana"`
	RecoveryUntilSeconds int64                    `json:"recovery_until_seconds"`
	Pending              *BreakthroughPreparation `json:"pending,omitempty"`
	LastAttempt          *CompletedBreakthrough   `json:"last_attempt,omitempty"`
}

// BreakthroughIntent is the NPC's own sourced decision to make one attempt.
// It does not represent player approval.
type BreakthroughIntent struct {
	AttemptID               string                         `json:"attempt_id"`
	SourceRef               string                         `json:"source_ref"`
	Target                  cultivation.BreakthroughTarget `json:"target"`
	TechniqueSupportsTarget bool                           `json:"technique_supports_target"`
	TechniqueSourceRef      string                         `json:"technique_source_ref"`
	ApplicabilitySourceRef  string                         `json:"applicability_source_ref"`
	RandomReceipt           *RandomReceipt                 `json:"random_receipt,omitempty"`
}

// RandomReceipt is supplied by the caller and bound to one attempt and its
// sourced conditions. This package never creates entropy.
type RandomReceipt struct {
	AttemptID            string  `json:"attempt_id"`
	SourceRef            string  `json:"source_ref"`
	Roll                 float64 `json:"roll"`
	ConditionFingerprint string  `json:"condition_fingerprint"`
}

// BreakthroughPreparation preserves the paid start and original conditions
// across completed time intervals. The original mana is kept so retries do
// not charge again or re-run the 80% eligibility check after payment.
type BreakthroughPreparation struct {
	Intent             BreakthroughIntent `json:"intent"`
	Conditions         Conditions         `json:"conditions"`
	StartedAtSeconds   int64              `json:"started_at_seconds"`
	CompletesAtSeconds int64              `json:"completes_at_seconds"`
	StartHealth        int                `json:"start_health"`
	StartMana          int                `json:"start_mana"`
	ManaCost           int                `json:"mana_cost"`
}

// CompletedBreakthrough is the most recent attempt identity retained with the
// authoritative snapshot so a changed retry cannot pay or roll again.
type CompletedBreakthrough struct {
	Intent               BreakthroughIntent `json:"intent"`
	Conditions           Conditions         `json:"conditions"`
	StartedAtSeconds     int64              `json:"started_at_seconds"`
	CompletedAtSeconds   int64              `json:"completed_at_seconds"`
	Status               Status             `json:"status"`
	ConditionFingerprint string             `json:"condition_fingerprint"`
}

// BreakthroughCandidate describes one non-persistent attempt step or result.
type BreakthroughCandidate struct {
	Status               Status                           `json:"status"`
	AttemptID            string                           `json:"attempt_id"`
	Target               cultivation.BreakthroughTarget   `json:"target"`
	ConditionFingerprint string                           `json:"condition_fingerprint,omitempty"`
	Chance               float64                          `json:"chance,omitempty"`
	ManaCost             int                              `json:"mana_cost,omitempty"`
	PreparationSeconds   int64                            `json:"preparation_seconds,omitempty"`
	StartedAtSeconds     int64                            `json:"started_at_seconds,omitempty"`
	CompletesAtSeconds   int64                            `json:"completes_at_seconds,omitempty"`
	RecoveryUntilSeconds int64                            `json:"recovery_until_seconds,omitempty"`
	Outcome              *cultivation.BreakthroughOutcome `json:"outcome,omitempty"`
}

type breakthroughCheck struct {
	requirement cultivation.BreakthroughRequirement
	modifiers   cultivation.BreakthroughModifiers
	chance      float64
	fingerprint string
	seconds     int64
}

type breakthroughFingerprintInput struct {
	ActorID                 string
	IntentSource            string
	Target                  cultivation.BreakthroughTarget
	TechniqueSupportsTarget bool
	TechniqueSourceRef      string
	ApplicabilitySourceRef  string
	Cultivation             cultivation.Cultivation
	Accumulated             float64
	CurrentHealth           int
	CurrentMana             int
	HealthMax               int
	ManaMax                 int
	Conditions              Conditions
}

func validBreakthroughTarget(target cultivation.BreakthroughTarget) bool {
	switch target {
	case cultivation.BreakthroughFoundation, cultivation.BreakthroughCore,
		cultivation.BreakthroughNascent, cultivation.BreakthroughSpirit,
		cultivation.BreakthroughVoid, cultivation.BreakthroughIntegration, cultivation.BreakthroughMahayana:
		return true
	default:
		return false
	}
}

func validateBreakthroughInput(input Input) error {
	if input.BreakthroughIntent != nil {
		if err := validateBreakthroughIntent(*input.BreakthroughIntent, false); err != nil {
			return err
		}
	}
	state := input.State.Breakthrough
	if state == nil {
		if input.BreakthroughIntent != nil && input.State.Resources == nil {
			return diagnostic("missing_breakthrough_state", "$.state.breakthrough", "Current health and mana are required for a breakthrough attempt.")
		}
		return nil
	}
	if state.CurrentHealth < 0 || state.CurrentMana < 0 || state.RecoveryUntilSeconds < 0 {
		return diagnostic("invalid_breakthrough_state", "$.state.breakthrough", "Breakthrough pools and recovery time must be non-negative.")
	}
	base := cultivation.BaseAttributes(input.State.Cultivation)
	if state.CurrentHealth > base.HealthMax || state.CurrentMana > base.ManaMax {
		return diagnostic("invalid_breakthrough_state", "$.state.breakthrough", "Current health or mana exceeds the formal stage maximum.")
	}
	if state.Pending != nil {
		pending := state.Pending
		if err := validateBreakthroughIntent(pending.Intent, true); err != nil {
			return diagnostic("invalid_pending_attempt", "$.state.breakthrough.pending.intent", "The stored breakthrough receipt is invalid.")
		}
		if !validConditions(pending.Conditions) || pending.Conditions.Resource != string(cultivation.SupportNone) ||
			pending.StartedAtSeconds < 0 || pending.CompletesAtSeconds <= pending.StartedAtSeconds ||
			pending.StartHealth < 0 || pending.StartMana < 0 || pending.StartHealth > base.HealthMax ||
			pending.StartMana > base.ManaMax || pending.ManaCost < 0 || pending.StartMana < pending.ManaCost {
			return diagnostic("invalid_pending_attempt", "$.state.breakthrough.pending", "The stored breakthrough preparation is incomplete or inconsistent.")
		}
		if err := validatePendingPreparation(input, pending); err != nil {
			return err
		}
		if state.RecoveryUntilSeconds != 0 {
			return diagnostic("invalid_breakthrough_state", "$.state.breakthrough.recovery_until_seconds", "An active preparation cannot also be in recovery.")
		}
	}
	if state.LastAttempt != nil {
		last := state.LastAttempt
		if err := validateBreakthroughIntent(last.Intent, true); err != nil || !validConditions(last.Conditions) ||
			last.StartedAtSeconds < 0 || last.CompletedAtSeconds < last.StartedAtSeconds ||
			last.ConditionFingerprint != last.Intent.RandomReceipt.ConditionFingerprint ||
			(last.Status != StatusBreakthroughSucceeded && last.Status != StatusBreakthroughFailed && last.Status != StatusPreparationInterrupted) {
			return diagnostic("invalid_last_attempt", "$.state.breakthrough.last_attempt", "The stored breakthrough attempt is incomplete or inconsistent.")
		}
		if state.RecoveryUntilSeconds > 0 && last.Status != StatusBreakthroughFailed {
			return diagnostic("invalid_breakthrough_state", "$.state.breakthrough.recovery_until_seconds", "Only a failed attempt can impose breakthrough recovery.")
		}
	}
	return nil
}

func validatePendingPreparation(input Input, pending *BreakthroughPreparation) error {
	if input.State.Breakthrough == nil || pending == nil {
		return diagnostic("invalid_pending_attempt", "$.state.breakthrough.pending", "The stored breakthrough preparation is missing its actor state.")
	}

	startState := input.State
	startBreakthrough := *input.State.Breakthrough
	startBreakthrough.CurrentHealth = pending.StartHealth
	startBreakthrough.CurrentMana = pending.StartMana
	startBreakthrough.Pending = nil
	startState.Breakthrough = &startBreakthrough
	startIntent := pending.Intent
	startInput := input
	startInput.State = startState
	startInput.BreakthroughIntent = &startIntent
	startInput.Interval.From = pending.StartedAtSeconds

	check, err := checkBreakthrough(startInput, pending.Conditions)
	if err != nil {
		return diagnostic("invalid_pending_attempt", "$.state.breakthrough.pending", "The stored preparation no longer matches formal breakthrough eligibility.")
	}
	if pending.StartedAtSeconds > math.MaxInt64-check.seconds {
		return diagnostic("invalid_pending_attempt", "$.state.breakthrough.pending.completes_at_seconds", "The stored preparation completion time overflows.")
	}
	if pending.ManaCost != check.requirement.ManaCost || pending.CompletesAtSeconds != pending.StartedAtSeconds+check.seconds {
		return diagnostic("invalid_pending_attempt", "$.state.breakthrough.pending", "The stored preparation fee or duration does not match the formal breakthrough rules.")
	}
	if input.State.SettledThroughSeconds <= pending.StartedAtSeconds || input.State.SettledThroughSeconds >= pending.CompletesAtSeconds {
		return diagnostic("invalid_pending_attempt", "$.state.settled_through_seconds", "The settlement watermark must fall strictly inside the stored preparation interval.")
	}
	if pending.Intent.RandomReceipt == nil || pending.Intent.RandomReceipt.ConditionFingerprint != check.fingerprint {
		return diagnostic("invalid_pending_attempt", "$.state.breakthrough.pending.intent.random_receipt.condition_fingerprint", "The stored receipt does not match the preparation's original conditions.")
	}
	return nil
}

func validateBreakthroughIntent(intent BreakthroughIntent, receiptRequired bool) error {
	if !intent.TechniqueSupportsTarget {
		return diagnostic("breakthrough_technique_unsupported", "$.breakthrough_intent.technique_supports_target", "The active technique is not verified for this breakthrough target.")
	}
	if !sourceRefValid(intent.AttemptID) || !sourceRefValid(intent.SourceRef) || !validBreakthroughTarget(intent.Target) ||
		!sourceRefValid(intent.TechniqueSourceRef) || !sourceRefValid(intent.ApplicabilitySourceRef) {
		return diagnostic("invalid_breakthrough_intent", "$.breakthrough_intent", "A stable NPC attempt identity, supported target, technique, and sourced applicability are required.")
	}
	if intent.RandomReceipt == nil {
		if receiptRequired {
			return diagnostic("missing_random_receipt", "$.breakthrough_intent.random_receipt", "A random receipt is required for a prepared or completed attempt.")
		}
		return nil
	}
	receipt := intent.RandomReceipt
	if receipt.AttemptID != intent.AttemptID || !sourceRefValid(receipt.SourceRef) || !finite(receipt.Roll) || receipt.Roll < 0 || receipt.Roll >= 1 {
		return diagnostic("invalid_random_receipt", "$.breakthrough_intent.random_receipt", "The random receipt identity or roll is invalid.")
	}
	decoded, err := hex.DecodeString(receipt.ConditionFingerprint)
	if err != nil || len(decoded) != sha256.Size || strings.ToLower(receipt.ConditionFingerprint) != receipt.ConditionFingerprint {
		return diagnostic("invalid_random_receipt", "$.breakthrough_intent.random_receipt.condition_fingerprint", "The condition fingerprint must be a lowercase SHA-256 value.")
	}
	return nil
}

func checkBreakthrough(input Input, conditions Conditions) (breakthroughCheck, error) {
	if input.State.Breakthrough == nil {
		return breakthroughCheck{}, diagnostic("missing_breakthrough_state", "$.state.breakthrough", "Current health and mana are required for a breakthrough attempt.")
	}
	base := cultivation.BaseAttributes(input.State.Cultivation)
	if conditions.MaximumHealth != base.HealthMax || conditions.CurrentHealth != input.State.Breakthrough.CurrentHealth {
		return breakthroughCheck{}, diagnostic("unsupported_breakthrough_attributes", "$.interval.segments[0].conditions", "Breakthrough health must match the formal base attributes and sourced current health.")
	}
	intent := input.BreakthroughIntent
	if !intent.TechniqueSupportsTarget || intent.TechniqueSourceRef != conditions.TechniqueRef || !sourceRefValid(intent.ApplicabilitySourceRef) {
		return breakthroughCheck{}, diagnostic("breakthrough_technique_unsupported", "$.breakthrough_intent", "The active technique must have sourced proof that it supports the requested target.")
	}
	attrs := base
	attrs.Health = input.State.Breakthrough.CurrentHealth
	attrs.Mana = input.State.Breakthrough.CurrentMana
	convalescing := input.State.Breakthrough != nil && input.State.Breakthrough.RecoveryUntilSeconds > input.Interval.From
	requirement := cultivation.CanAttemptBreakthrough(input.State.Cultivation, input.State.Accumulated, attrs, input.BreakthroughIntent.Target, convalescing)
	if !requirement.Eligible {
		return breakthroughCheck{}, diagnostic("breakthrough_ineligible", "$.breakthrough_intent", "The NPC does not meet the formal breakthrough requirements.")
	}
	modifiers := cultivation.BreakthroughModifiers{
		Talent:  cultivation.Talent(conditions.Talent),
		Quality: cultivation.TechniqueQuality(conditions.Technique),
	}
	chance, ok := cultivation.BreakthroughChance(input.BreakthroughIntent.Target, modifiers, input.State.Cultivation.Rules())
	if !ok {
		return breakthroughCheck{}, diagnostic("unsupported_breakthrough_target", "$.breakthrough_intent.target", "The breakthrough target is not implemented.")
	}
	seconds, ok := ruleDaysToSeconds(requirement.Days)
	if !ok {
		return breakthroughCheck{}, diagnostic("invalid_preparation_duration", "$.breakthrough_intent.target", "The formal preparation duration cannot be represented in world seconds.")
	}
	if input.Interval.From > math.MaxInt64-seconds {
		return breakthroughCheck{}, diagnostic("time_overflow", "$.interval.from_seconds", "The breakthrough completion time overflows.")
	}
	fingerprint, err := conditionFingerprint(input.ActorID, input.State, *input.BreakthroughIntent, conditions)
	if err != nil {
		return breakthroughCheck{}, err
	}
	return breakthroughCheck{requirement: requirement, modifiers: modifiers, chance: chance, fingerprint: fingerprint, seconds: seconds}, nil
}

func conditionFingerprint(actorID string, state Snapshot, intent BreakthroughIntent, conditions Conditions) (string, error) {
	if state.Breakthrough == nil {
		return "", diagnostic("missing_breakthrough_state", "$.state.breakthrough", "Current health and mana are required for a breakthrough attempt.")
	}
	base := cultivation.BaseAttributes(state.Cultivation)
	payload := breakthroughFingerprintInput{
		ActorID: actorID, IntentSource: intent.SourceRef, Target: intent.Target,
		Cultivation: state.Cultivation, Accumulated: state.Accumulated,
		CurrentHealth: state.Breakthrough.CurrentHealth, CurrentMana: state.Breakthrough.CurrentMana,
		HealthMax: base.HealthMax, ManaMax: base.ManaMax, Conditions: conditions,
		TechniqueSupportsTarget: intent.TechniqueSupportsTarget,
		TechniqueSourceRef:      intent.TechniqueSourceRef,
		ApplicabilitySourceRef:  intent.ApplicabilitySourceRef,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", diagnostic("fingerprint_failed", "$.breakthrough_intent", "The sourced breakthrough conditions could not be fingerprinted.")
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func ruleDaysToSeconds(days float64) (int64, bool) {
	if !finite(days) || days <= 0 {
		return 0, false
	}
	seconds := days * 86400
	if !finite(seconds) || seconds >= float64(math.MaxInt64) || math.Trunc(seconds) != seconds {
		return 0, false
	}
	return int64(seconds), true
}

func sameIntent(left, right BreakthroughIntent) bool {
	return reflect.DeepEqual(left, right)
}

func sameConditions(left, right Conditions) bool {
	return left == right
}

func attemptModifiers(conditions Conditions) cultivation.BreakthroughModifiers {
	return cultivation.BreakthroughModifiers{
		Talent:  cultivation.Talent(conditions.Talent),
		Quality: cultivation.TechniqueQuality(conditions.Technique),
	}
}

func validateSettledIntervalRequest(input Input) error {
	state := input.State.Breakthrough
	intent := input.BreakthroughIntent
	if state != nil && state.Pending != nil && intent != nil {
		if !sameIntent(*intent, state.Pending.Intent) {
			return diagnostic("attempt_identity_conflict", "$.breakthrough_intent", "A pending preparation can only continue with its original attempt identity and receipt.")
		}
		return nil
	}
	if intent == nil {
		return nil
	}
	if state != nil && state.LastAttempt != nil && intent.AttemptID == state.LastAttempt.Intent.AttemptID {
		if !sameIntent(*intent, state.LastAttempt.Intent) || !sameConditions(input.Interval.Segments[0].Conditions, state.LastAttempt.Conditions) {
			return diagnostic("attempt_identity_conflict", "$.breakthrough_intent", "A completed attempt identity cannot be reused with changed target, roll, or conditions.")
		}
		return nil
	}
	return diagnostic("attempt_interval_already_settled", "$.breakthrough_intent", "This world interval was already settled before this attempt was requested.")
}

func calculateBreakthrough(input Input, intervalHash string) (Candidate, error) {
	candidate := Candidate{
		Status: StatusApplied, ActorID: input.ActorID, IntervalID: input.Interval.ID,
		IntervalHash: intervalHash, ExpectedRevision: input.State.Revision,
		NextState: input.State, ConsumedThrough: input.Interval.From,
		Segments: []SegmentResult{},
	}
	state := input.State.Breakthrough
	if state == nil {
		return Candidate{}, diagnostic("missing_breakthrough_state", "$.state.breakthrough", "Current health and mana are required for a breakthrough attempt.")
	}
	if state.Pending != nil {
		return continueBreakthrough(input, intervalHash, state.Pending)
	}
	if state.RecoveryUntilSeconds > input.Interval.From {
		if input.BreakthroughIntent != nil && input.BreakthroughIntent.RandomReceipt != nil {
			return Candidate{}, diagnostic("breakthrough_in_recovery", "$.breakthrough_intent.random_receipt", "Do not draw or submit a random receipt while breakthrough recovery is active.")
		}
		return settleRecoveryInterval(input, intervalHash)
	}
	if input.BreakthroughIntent == nil {
		return Candidate{}, diagnostic("missing_breakthrough_intent", "$.breakthrough_intent", "An NPC-sourced breakthrough intent is required.")
	}
	intent := *input.BreakthroughIntent
	conditions := input.Interval.Segments[0].Conditions
	if state.LastAttempt != nil && state.LastAttempt.Intent.AttemptID == intent.AttemptID {
		last := state.LastAttempt
		if !sameIntent(intent, last.Intent) || !sameConditions(conditions, last.Conditions) {
			return Candidate{}, diagnostic("attempt_identity_conflict", "$.breakthrough_intent", "A completed attempt identity cannot be reused with changed target, roll, or conditions.")
		}
		candidate.Status = StatusDuplicate
		candidate.Breakthrough = &BreakthroughCandidate{Status: StatusDuplicate, AttemptID: intent.AttemptID, Target: intent.Target}
		return candidate, nil
	}
	if input.Interval.Segments[0].Activity != ActivityBreakthroughPrep {
		return Candidate{}, diagnostic("breakthrough_activity_required", "$.interval.segments[0].activity", "A breakthrough attempt must occupy an explicit preparation segment.")
	}
	check, err := checkBreakthrough(input, conditions)
	if err != nil {
		return Candidate{}, err
	}
	if intent.RandomReceipt == nil {
		candidate.Status = StatusBreakthroughReady
		candidate.Breakthrough = &BreakthroughCandidate{
			Status: StatusBreakthroughReady, AttemptID: intent.AttemptID, Target: intent.Target,
			ConditionFingerprint: check.fingerprint, Chance: check.chance,
			ManaCost: check.requirement.ManaCost, PreparationSeconds: check.seconds,
			StartedAtSeconds: input.Interval.From, CompletesAtSeconds: input.Interval.From + check.seconds,
		}
		return candidate, nil
	}
	if intent.RandomReceipt.ConditionFingerprint != check.fingerprint {
		return Candidate{}, diagnostic("attempt_condition_conflict", "$.breakthrough_intent.random_receipt.condition_fingerprint", "The random receipt is bound to different breakthrough conditions.")
	}
	if check.requirement.ManaCost > state.CurrentMana {
		return Candidate{}, diagnostic("breakthrough_mana_cost", "$.state.breakthrough.current_mana", "The eligible breakthrough mana cost exceeds the current pool.")
	}
	completesAt := input.Interval.From + check.seconds
	pending := &BreakthroughPreparation{
		Intent: intent, Conditions: conditions,
		StartedAtSeconds: input.Interval.From, CompletesAtSeconds: completesAt,
		StartHealth: state.CurrentHealth, StartMana: state.CurrentMana,
		ManaCost: check.requirement.ManaCost,
	}
	next := input.State
	breakthrough := *state
	breakthrough.CurrentMana -= check.requirement.ManaCost
	breakthrough.RecoveryUntilSeconds = 0
	breakthrough.Pending = pending
	next.Breakthrough = &breakthrough
	if next.Resources != nil {
		pools := *next.Resources
		pools.Health, pools.Mana = breakthrough.CurrentHealth, breakthrough.CurrentMana
		next.Resources = &pools
	}
	return advanceBreakthroughPreparation(input, intervalHash, next, pending, check.chance)
}

func continueBreakthrough(input Input, intervalHash string, pending *BreakthroughPreparation) (Candidate, error) {
	if input.BreakthroughIntent != nil && !sameIntent(*input.BreakthroughIntent, pending.Intent) {
		return Candidate{}, diagnostic("attempt_identity_conflict", "$.breakthrough_intent", "A pending preparation can only continue with its original attempt identity and receipt.")
	}
	state := input.State.Breakthrough
	if state.CurrentHealth != pending.StartHealth || state.CurrentMana != pending.StartMana-pending.ManaCost {
		return interruptPreparation(input, intervalHash, input.State, pending, input.Interval.From, 0)
	}
	basis := input.State
	basisBreakthrough := *state
	basisBreakthrough.CurrentHealth = pending.StartHealth
	basisBreakthrough.CurrentMana = pending.StartMana
	basis.Breakthrough = &basisBreakthrough
	fingerprint, err := conditionFingerprint(input.ActorID, basis, pending.Intent, pending.Conditions)
	if err != nil {
		return Candidate{}, err
	}
	if fingerprint != pending.Intent.RandomReceipt.ConditionFingerprint {
		return Candidate{}, diagnostic("pending_attempt_state_conflict", "$.state", "The cultivation position or accumulated progress changed during breakthrough preparation.")
	}
	chance, ok := cultivation.BreakthroughChance(pending.Intent.Target, attemptModifiers(pending.Conditions), input.State.Cultivation.Rules())
	if !ok {
		return Candidate{}, diagnostic("unsupported_breakthrough_target", "$.state.breakthrough.pending.intent.target", "The stored breakthrough target is not implemented.")
	}
	if input.Interval.From < pending.StartedAtSeconds || input.Interval.From >= pending.CompletesAtSeconds {
		return Candidate{}, diagnostic("invalid_pending_attempt_time", "$.interval.from_seconds", "The pending preparation interval is outside its stored start and completion times.")
	}
	return advanceBreakthroughPreparation(input, intervalHash, input.State, pending, chance)
}

func advanceBreakthroughPreparation(input Input, intervalHash string, state Snapshot, pending *BreakthroughPreparation, chance float64) (Candidate, error) {
	candidate := Candidate{
		Status: StatusBreakthroughPreparing, ActorID: input.ActorID, IntervalID: input.Interval.ID,
		IntervalHash: intervalHash, ExpectedRevision: input.State.Revision,
		NextState: state, ConsumedThrough: input.Interval.From, Segments: []SegmentResult{},
		Breakthrough: &BreakthroughCandidate{
			Status: StatusBreakthroughPreparing, AttemptID: pending.Intent.AttemptID,
			Target: pending.Intent.Target, ConditionFingerprint: pending.Intent.RandomReceipt.ConditionFingerprint,
			Chance: chance, ManaCost: pending.ManaCost, PreparationSeconds: pending.CompletesAtSeconds - pending.StartedAtSeconds,
			StartedAtSeconds: pending.StartedAtSeconds, CompletesAtSeconds: pending.CompletesAtSeconds,
		},
	}
	for index, segment := range input.Interval.Segments {
		if segment.From >= pending.CompletesAtSeconds {
			return completeBreakthrough(input, intervalHash, state, pending, chance, pending.CompletesAtSeconds, index)
		}
		if segment.Activity != ActivityBreakthroughPrep || !sameConditions(segment.Conditions, pending.Conditions) {
			return interruptPreparation(input, intervalHash, state, pending, segment.From, index)
		}
		if segment.To >= pending.CompletesAtSeconds {
			return completeBreakthrough(input, intervalHash, state, pending, chance, pending.CompletesAtSeconds, index)
		}
	}
	return commitBreakthroughProgress(input, intervalHash, candidate, state, input.Interval.To)
}

func completeBreakthrough(input Input, intervalHash string, state Snapshot, pending *BreakthroughPreparation, chance float64, completedAt int64, segmentIndex int) (Candidate, error) {
	attrs := cultivation.BaseAttributes(state.Cultivation)
	attrs.Health = pending.StartHealth
	attrs.Mana = pending.StartMana - pending.ManaCost
	outcome, ok := cultivation.ResolveBreakthrough(
		state.Cultivation, state.Accumulated, attrs, pending.Intent.Target,
		attemptModifiers(pending.Conditions), pending.Intent.RandomReceipt.Roll,
	)
	if !ok {
		return Candidate{}, diagnostic("unsupported_breakthrough_target", "$.breakthrough_intent.target", "The breakthrough target is not implemented.")
	}
	next := state
	next.Cultivation, next.Accumulated = outcome.Cultivation, outcome.Accumulated
	bt := *state.Breakthrough
	bt.Pending = nil
	bt.CurrentHealth, bt.CurrentMana = outcome.Health, outcome.Mana
	status := StatusBreakthroughFailed
	recoveryUntil := int64(0)
	if outcome.Succeeded {
		status = StatusBreakthroughSucceeded
	} else {
		recoverySeconds, valid := ruleDaysToSeconds(outcome.RecoveryDays)
		if !valid || completedAt > math.MaxInt64-recoverySeconds {
			return Candidate{}, diagnostic("recovery_time_overflow", "$.state.breakthrough.recovery_until_seconds", "The formal recovery end time overflows.")
		}
		recoveryUntil = completedAt + recoverySeconds
		bt.RecoveryUntilSeconds = recoveryUntil
	}
	bt.LastAttempt = &CompletedBreakthrough{
		Intent: pending.Intent, Conditions: pending.Conditions,
		StartedAtSeconds: pending.StartedAtSeconds, CompletedAtSeconds: completedAt,
		Status: status, ConditionFingerprint: pending.Intent.RandomReceipt.ConditionFingerprint,
	}
	next.Breakthrough = &bt
	if next.Resources != nil {
		pools := *next.Resources
		pools.Health, pools.Mana = bt.CurrentHealth, bt.CurrentMana
		next.Resources = &pools
	}
	candidate := Candidate{
		Status: status, ActorID: input.ActorID, IntervalID: input.Interval.ID,
		IntervalHash: intervalHash, ExpectedRevision: input.State.Revision,
		NextState: next, ConsumedThrough: completedAt, Segments: []SegmentResult{},
		Breakthrough: &BreakthroughCandidate{
			Status: status, AttemptID: pending.Intent.AttemptID, Target: pending.Intent.Target,
			ConditionFingerprint: pending.Intent.RandomReceipt.ConditionFingerprint,
			Chance:               chance, ManaCost: pending.ManaCost,
			PreparationSeconds: completedAt - pending.StartedAtSeconds,
			StartedAtSeconds:   pending.StartedAtSeconds, CompletesAtSeconds: completedAt,
			RecoveryUntilSeconds: recoveryUntil, Outcome: &outcome,
		},
	}
	if completedAt < input.Interval.To {
		candidate.RemainingIntent = remainingIntent(input.Interval, segmentIndex, completedAt, "The single requested breakthrough attempt completed; settle the remaining world interval separately.")
	}
	return commitBreakthroughProgress(input, intervalHash, candidate, next, completedAt)
}

func interruptPreparation(input Input, intervalHash string, state Snapshot, pending *BreakthroughPreparation, at int64, segmentIndex int) (Candidate, error) {
	next := state
	bt := *state.Breakthrough
	bt.Pending = nil
	bt.LastAttempt = &CompletedBreakthrough{
		Intent: pending.Intent, Conditions: pending.Conditions,
		StartedAtSeconds: pending.StartedAtSeconds, CompletedAtSeconds: at,
		Status: StatusPreparationInterrupted, ConditionFingerprint: pending.Intent.RandomReceipt.ConditionFingerprint,
	}
	next.Breakthrough = &bt
	candidate := Candidate{
		Status: StatusPreparationInterrupted, ActorID: input.ActorID, IntervalID: input.Interval.ID,
		IntervalHash: intervalHash, ExpectedRevision: input.State.Revision,
		NextState: next, ConsumedThrough: at, Segments: []SegmentResult{},
		Breakthrough: &BreakthroughCandidate{
			Status: StatusPreparationInterrupted, AttemptID: pending.Intent.AttemptID,
			Target: pending.Intent.Target, ConditionFingerprint: pending.Intent.RandomReceipt.ConditionFingerprint,
			ManaCost: pending.ManaCost, StartedAtSeconds: pending.StartedAtSeconds,
			CompletesAtSeconds: pending.CompletesAtSeconds,
		},
		RemainingIntent: remainingIntent(input.Interval, segmentIndex, at, "Preparation conditions changed; the attempt was interrupted without a roll."),
	}
	return commitBreakthroughProgress(input, intervalHash, candidate, next, at)
}

func settleRecoveryInterval(input Input, intervalHash string) (Candidate, error) {
	state := input.State
	bt := *state.Breakthrough
	stopAt := minInt64(input.Interval.To, bt.RecoveryUntilSeconds)
	ended := stopAt == bt.RecoveryUntilSeconds
	if ended {
		bt.RecoveryUntilSeconds = 0
	}
	state.Breakthrough = &bt
	status := StatusBreakthroughRecovering
	if ended {
		status = StatusRecoveryEnded
	}
	candidate := Candidate{
		Status: status, ActorID: input.ActorID, IntervalID: input.Interval.ID,
		IntervalHash: intervalHash, ExpectedRevision: input.State.Revision,
		NextState: state, ConsumedThrough: stopAt, Segments: []SegmentResult{},
		Breakthrough: &BreakthroughCandidate{Status: status, RecoveryUntilSeconds: input.State.Breakthrough.RecoveryUntilSeconds},
	}
	if stopAt < input.Interval.To {
		candidate.RemainingIntent = remainingIntent(input.Interval, segmentIndexAt(input.Interval, stopAt), stopAt, "Breakthrough recovery ended; recheck conditions and obtain a fresh NPC intent before another attempt.")
	}
	return commitBreakthroughProgress(input, intervalHash, candidate, state, stopAt)
}

func commitBreakthroughProgress(input Input, intervalHash string, candidate Candidate, state Snapshot, through int64) (Candidate, error) {
	if input.State.Revision == math.MaxUint64 {
		return Candidate{}, diagnostic("revision_overflow", "$.state.revision", "The actor revision cannot be incremented.")
	}
	state.SettledThroughSeconds = through
	state.LastIntervalID, state.LastIntervalHash = input.Interval.ID, intervalHash
	state.Revision = input.State.Revision + 1
	candidate.NextState = state
	candidate.ConsumedThrough = through
	candidate.HasStateChange = true
	return candidate, nil
}

func segmentIndexAt(interval Interval, at int64) int {
	for index, segment := range interval.Segments {
		if at < segment.To {
			return index
		}
	}
	return len(interval.Segments)
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
