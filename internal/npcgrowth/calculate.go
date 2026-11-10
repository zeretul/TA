package npcgrowth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"ta/internal/cultivation"
)

// Calculate returns a deterministic growth candidate. It never persists it;
// Story must atomically commit NextState with the source event.
func Calculate(input Input) (Candidate, error) {
	if err := validate(input); err != nil {
		return Candidate{}, err
	}
	if err := validateSupportState(input.State); err != nil {
		return Candidate{}, err
	}
	encoded, err := json.Marshal(input.Interval)
	if err != nil {
		return Candidate{}, diagnostic("interval_hash_failed", "$.interval", "Could not fingerprint the interval.")
	}
	sum := sha256.Sum256(encoded)
	intervalHash := hex.EncodeToString(sum[:])

	if input.Interval.ID == input.State.LastIntervalID {
		if intervalHash != input.State.LastIntervalHash {
			return Candidate{}, diagnostic("interval_identity_conflict", "$.interval.id", "This interval identity was already used with different content.")
		}
		if err := validateSettledIntervalRequest(input); err != nil {
			return Candidate{}, err
		}
		return Candidate{
			Status: StatusDuplicate, ActorID: input.ActorID, IntervalID: input.Interval.ID,
			IntervalHash: intervalHash, ExpectedRevision: input.State.Revision, NextState: input.State,
			ConsumedThrough: input.State.SettledThroughSeconds, Segments: []SegmentResult{},
		}, nil
	}
	if input.BaseRevision != input.State.Revision {
		return Candidate{}, diagnostic("stale_revision", "$.base_revision", "The request was built from an older actor revision.")
	}
	if input.Interval.From < input.State.SettledThroughSeconds {
		return Candidate{}, diagnostic("overlapping_interval", "$.interval.from_seconds", "The interval overlaps time already settled.")
	}
	if input.Interval.From > input.State.SettledThroughSeconds {
		return Candidate{}, diagnostic("interval_gap", "$.interval.from_seconds", "The interval must begin at the settlement watermark.")
	}
	hasRecoveryActivity := false
	for _, segment := range input.Interval.Segments {
		if segment.Activity == ActivityHealthRest || segment.Activity == ActivitySafeMeditation {
			hasRecoveryActivity = true
			break
		}
	}
	if hasRecoveryActivity && len(input.Interval.Segments) != 1 {
		return Candidate{}, diagnostic("mixed_recovery_interval", "$.interval.segments", "Recovery must be submitted as one continuous single-activity interval.")
	}
	if err := validateIntervalResources(input); err != nil {
		return Candidate{}, err
	}
	if hasRecoveryActivity {
		return calculateRecovery(input, intervalHash)
	}
	if input.State.Revision == math.MaxUint64 {
		return Candidate{}, diagnostic("revision_overflow", "$.state.revision", "The actor revision cannot be incremented.")
	}
	if input.State.Breakthrough == nil && input.State.Resources != nil && input.BreakthroughIntent != nil {
		input.State.Breakthrough = &BreakthroughState{CurrentHealth: input.State.Resources.Health, CurrentMana: input.State.Resources.Mana}
	}
	if (input.State.Breakthrough != nil && (input.State.Breakthrough.Pending != nil || input.State.Breakthrough.RecoveryUntilSeconds > input.Interval.From)) || input.BreakthroughIntent != nil {
		return calculateBreakthrough(input, intervalHash)
	}
	candidate := Candidate{
		Status: StatusApplied, ActorID: input.ActorID, IntervalID: input.Interval.ID,
		IntervalHash: intervalHash, ExpectedRevision: input.State.Revision,
		NextState: input.State, ConsumedThrough: input.Interval.From, Segments: []SegmentResult{},
	}
	current := input.State
	for index, segment := range input.Interval.Segments {
		if atMajorBoundary(current.Cultivation, current.Accumulated) {
			candidate.Status = StatusAtBoundary
			candidate.NextState = current
			candidate.HasStateChange = candidate.ConsumedThrough > input.Interval.From
			candidate.RemainingIntent = remainingIntentWithoutPastSupport(input.Interval, index, candidate.ConsumedThrough, "A major-realm decision is required before growth can continue.")
			break
		}
		if segment.SupportUse != nil {
			var err error
			current, err = prepareSupport(current, segment)
			if err != nil {
				return Candidate{}, err
			}
		}
		current.Recovery = nil
		stopped := false
		for _, segment := range splitSegmentAtSupportExpiry(current, segment) {
			if current.ActiveSupport != nil && current.ActiveSupport.EndsAtSeconds <= segment.From {
				current.ActiveSupport = nil
			}
			segment.SupportUse = nil
			segment.Conditions.Resource = string(supportResourceAt(current, segment.From))
			duration := segment.To - segment.From
			rate := activityTenthsPerWorldSecond(segment.Activity)
			effectiveTenths, ok := checkedMultiply(duration, rate)
			if !ok {
				return Candidate{}, diagnostic("effective_time_overflow", "$.interval.segments", "The effective training duration overflows.")
			}
			modifiers := trainingModifiers(segment.Conditions)
			progress := advance(current, effectiveTenths, modifiers)
			if !finite(progress.Accumulated) || !finite(progress.Gained) {
				return Candidate{}, diagnostic("progress_overflow", "$.state.accumulated", "The cultivation result is not finite.")
			}

			usedSeconds := duration
			hitBoundary := atMajorBoundary(progress.Cultivation, progress.Accumulated)
			if hitBoundary {
				low, high := int64(1), duration
				for low < high {
					mid := low + (high-low)/2
					trialTenths, valid := checkedMultiply(mid, rate)
					if !valid {
						return Candidate{}, diagnostic("effective_time_overflow", "$.interval.segments", "The effective training duration overflows.")
					}
					trial := advance(current, trialTenths, modifiers)
					if atMajorBoundary(trial.Cultivation, trial.Accumulated) {
						high = mid
					} else {
						low = mid + 1
					}
				}
				usedSeconds = low
				usedTenths, _ := checkedMultiply(usedSeconds, rate)
				progress = advance(current, usedTenths, modifiers)
				required, _, _ := cultivation.ProgressRequired(progress.Cultivation)
				if progress.Accumulated > required {
					progress.Gained -= progress.Accumulated - required
				}
				progress.Accumulated = required
				candidate.Status = StatusAtBoundary
			}

			usedTenths, _ := checkedMultiply(usedSeconds, rate)
			if candidate.EffectiveSeconds10 > math.MaxInt64-usedTenths {
				return Candidate{}, diagnostic("effective_time_overflow", "$.interval.segments", "The combined effective training duration overflows.")
			}
			candidate.EffectiveSeconds10 += usedTenths
			candidate.Gained += progress.Gained
			candidate.StagesAdvanced += progress.StagesAdvanced
			current.Cultivation, current.Accumulated = progress.Cultivation, progress.Accumulated
			cursor := segment.From + usedSeconds
			candidate.ConsumedThrough = cursor
			candidate.Segments = append(candidate.Segments, SegmentResult{
				From: segment.From, To: cursor, EffectiveSeconds10: usedTenths,
				Cultivation: current.Cultivation, Accumulated: current.Accumulated,
				Gained: progress.Gained, StagesAdvanced: progress.StagesAdvanced,
				SupportResource: segment.Conditions.Resource,
			})
			if hitBoundary {
				if current.ActiveSupport != nil && current.ActiveSupport.EndsAtSeconds <= cursor {
					current.ActiveSupport = nil
				}
				candidate.NextState = current
				candidate.HasStateChange = cursor > input.Interval.From
				candidate.RemainingIntent = remainingIntentWithoutPastSupport(input.Interval, index, cursor, "A major-realm decision is required before growth can continue.")
				stopped = true
				break
			}
			if current.ActiveSupport != nil && current.ActiveSupport.EndsAtSeconds <= segment.To {
				current.ActiveSupport = nil
			}
			current.SettledThroughSeconds = segment.To
			candidate.ConsumedThrough = segment.To
		}
		if stopped {
			break
		}
	}

	if candidate.Status == StatusAtBoundary && !candidate.HasStateChange {
		candidate.NextState = input.State
		return candidate, nil
	}
	current.SettledThroughSeconds = candidate.ConsumedThrough
	current.LastIntervalID, current.LastIntervalHash = input.Interval.ID, intervalHash
	current.Revision = input.State.Revision + 1
	candidate.NextState, candidate.HasStateChange = current, true
	return candidate, nil
}

func validate(input Input) error {
	var issues []Diagnostic
	add := func(code, path, message string) {
		issues = append(issues, Diagnostic{Code: code, Path: path, Message: message})
	}
	if input.SchemaVersion != InputVersion {
		add("unsupported_schema", "$.schema_version", "The input schema version is unsupported.")
	}
	if !sourceRefValid(input.ActorID) {
		add("invalid_actor_id", "$.actor_id", "A sourced actor identifier is required.")
	}
	if !sourceRefValid(input.ActorSource) {
		add("missing_actor_source", "$.actor_source_ref", "A source for the actor state is required.")
	}
	if !sourceRefValid(input.State.SourceRef) {
		add("missing_state_source", "$.state.source_ref", "A source for the formal actor snapshot is required.")
	}
	if strings.TrimSpace(input.Interval.ID) == "" || len(input.Interval.ID) > maxSourceRefLength {
		add("invalid_interval_id", "$.interval.id", "A stable interval identity is required.")
	}
	if !sourceRefValid(input.Interval.SourceRef) {
		add("missing_interval_source", "$.interval.source_ref", "A source for the completed world interval is required.")
	}
	if input.State.SettledThroughSeconds < 0 || input.Interval.From < 0 || input.Interval.To <= input.Interval.From {
		add("invalid_interval", "$.interval", "The completed interval must have a positive, non-negative range.")
	}
	if !finite(input.State.Accumulated) || input.State.Accumulated < 0 {
		add("invalid_accumulated", "$.state.accumulated", "Accumulated cultivation progress must be finite and non-negative.")
	}
	if !validCultivation(input.State.Cultivation, input.State.Accumulated) {
		add("invalid_cultivation", "$.state.cultivation", "The cultivation state or its accumulated progress is unsupported.")
	}
	if input.State.LastIntervalID == "" && input.State.LastIntervalHash != "" || input.State.LastIntervalID != "" && input.State.LastIntervalHash == "" {
		add("invalid_last_interval", "$.state.last_interval_id", "The last interval identity and hash must both be empty or both be present.")
	}
	if err := validateActorPools(input.State); err != nil {
		if diagnosticErr, ok := err.(*DiagnosticError); ok {
			issues = append(issues, diagnosticErr.Diagnostics...)
		}
	}
	if err := validateBreakthroughInput(input); err != nil {
		if diagnosticErr, ok := err.(*DiagnosticError); ok {
			issues = append(issues, diagnosticErr.Diagnostics...)
		} else {
			add("invalid_breakthrough_state", "$.state.breakthrough", "The breakthrough state is invalid.")
		}
	}
	if len(input.Interval.Segments) == 0 {
		add("missing_segments", "$.interval.segments", "At least one activity segment is required.")
	}
	cursor := input.Interval.From
	for i, segment := range input.Interval.Segments {
		path := fmt.Sprintf("$.interval.segments[%d]", i)
		if segment.From != cursor || segment.To <= segment.From {
			add("unordered_segments", path, "Segments must be positive, ordered, contiguous, and non-overlapping.")
		}
		if !sourceRefValid(segment.ActivitySourceRef) {
			add("missing_activity_source", path+".activity_source_ref", "A source for the legal activity arrangement is required.")
		}
		if !sourceRefValid(segment.Conditions.SourceRef) {
			add("missing_conditions_source", path+".conditions.source_ref", "A source for training conditions is required.")
		}
		if segment.Activity == ActivityTravelOpportunity && !sourceRefValid(segment.OpportunitySourceRef) {
			add("missing_training_opportunity", path+".opportunity_source_ref", "Travel earns training time only with a sourced valid opportunity.")
		}
		if segment.Activity == ActivityBreakthroughPrep {
			pending := input.State.Breakthrough != nil && input.State.Breakthrough.Pending != nil
			if input.BreakthroughIntent == nil && !pending {
				add("missing_breakthrough_intent", path+".activity", "A breakthrough preparation interval requires an NPC intent.")
			}
		} else if segment.Activity != ActivityFocusedSeclusion && segment.Activity != ActivityOrdinaryTraining && segment.Activity != ActivityTravelOpportunity && segment.Activity != ActivityTravelNoWindow && segment.Activity != ActivityInjuryPrevents && segment.Activity != ActivityOccupied && segment.Activity != ActivityHealthRest && segment.Activity != ActivitySafeMeditation {
			add("unsupported_activity", path+".activity", "The activity arrangement is unsupported.")
		}
		if segment.SupportUse != nil && activityTenthsPerWorldSecond(segment.Activity) == 0 {
			add("support_without_training", path+".support_use", "A cultivation-support dose can only be used during a real training window.")
		}
		if segment.To > segment.From {
			cursor = segment.To
		}
		if segment.Conditions.Resource != string(cultivation.SupportNone) {
			add("unsupported_resource_effect", path+".conditions.support_resource", "Auxiliary resource effects require a consumption path not implemented in this candidate flow.")
		}
		if !validConditions(segment.Conditions) {
			add("invalid_conditions", path+".conditions", "Training conditions must use supported, sourced values.")
		}
	}
	if cursor != input.Interval.To {
		add("segment_range_mismatch", "$.interval.segments", "Segments must cover the requested interval exactly.")
	}
	if len(issues) > 0 {
		return &DiagnosticError{Diagnostics: issues}
	}
	return nil
}

func validCultivation(c cultivation.Cultivation, accumulated float64) bool {
	required, _, ok := cultivation.ProgressRequired(c)
	if c.Realm == cultivation.RealmMortal {
		return c.Stage == "" && c.Layer == 0 && accumulated == 0
	}
	if !ok || required < 0 || accumulated > required {
		return false
	}
	if c.Realm == cultivation.RealmQi {
		return c.Stage == "" && c.Layer >= 1 && c.Layer <= cultivation.QiLayers && (accumulated < required || c.Layer == cultivation.QiLayers)
	}
	return c.Layer == 0 && (c.Stage == cultivation.StageEarly || c.Stage == cultivation.StageMiddle || c.Stage == cultivation.StageLate) && (accumulated < required || c.Stage == cultivation.StageLate)
}

func validateActorPools(state Snapshot) error {
	if state.Resources == nil {
		return nil
	}
	pools := state.Resources
	if pools.HealthMaximum <= 0 || pools.ManaMaximum <= 0 || pools.Health < 0 || pools.Health > pools.HealthMaximum || pools.Mana < 0 || pools.Mana > pools.ManaMaximum {
		return diagnostic("invalid_actor_pools", "$.state.resources", "Current health and mana must be within their formal maxima.")
	}
	if state.Breakthrough != nil && (state.Breakthrough.CurrentHealth != pools.Health || state.Breakthrough.CurrentMana != pools.Mana) {
		return diagnostic("actor_pool_snapshot_mismatch", "$.state.resources", "Actor resources and breakthrough pools must be the same current snapshot.")
	}
	return nil
}

func validateIntervalResources(input Input) error {
	if input.State.Resources == nil {
		return nil
	}
	for index, segment := range input.Interval.Segments {
		if segment.Conditions.CurrentHealth != input.State.Resources.Health || segment.Conditions.MaximumHealth != input.State.Resources.HealthMaximum {
			return diagnostic("actor_pool_condition_mismatch", fmt.Sprintf("$.interval.segments[%d].conditions.current_health", index), "Training conditions must match the actor's current health snapshot and maximum.")
		}
	}
	return nil
}

func atMajorBoundary(c cultivation.Cultivation, accumulated float64) bool {
	if c.Realm == cultivation.RealmQi && c.Layer == cultivation.QiLayers || c.Stage == cultivation.StageLate {
		required, _, ok := cultivation.ProgressRequired(c)
		return ok && accumulated >= required
	}
	return false
}

func validConditions(c Conditions) bool {
	if !sourceRefValid(c.SourceRef) || !sourceRefValid(c.TechniqueRef) || c.MaximumHealth <= 0 || c.CurrentHealth < 0 || c.CurrentHealth > c.MaximumHealth {
		return false
	}
	if !finite(c.GrowthCoefficient) || c.GrowthCoefficient < 0 {
		return false
	}
	switch c.Talent {
	case string(cultivation.TalentPoor), string(cultivation.TalentNormal), string(cultivation.TalentGood), string(cultivation.TalentExcellent), string(cultivation.TalentExceptional):
	default:
		return false
	}
	switch c.Technique {
	case string(cultivation.QualityOrdinary), string(cultivation.QualityGood), string(cultivation.QualityAdvanced):
	default:
		return false
	}
	switch c.Environment {
	case string(cultivation.EnvironmentPoor), string(cultivation.EnvironmentOrdinary), string(cultivation.EnvironmentStable):
	default:
		return false
	}
	if c.Resource != "" && c.Resource != string(cultivation.SupportNone) {
		return false
	}
	return true
}

func activityTenthsPerWorldSecond(activity Activity) int64 {
	switch activity {
	case ActivityFocusedSeclusion:
		return 10
	case ActivityOrdinaryTraining:
		return 5
	case ActivityTravelOpportunity:
		return 2
	default:
		return 0
	}
}

func trainingModifiers(c Conditions) cultivation.TrainingModifiers {
	return cultivation.TrainingModifiers{
		Talent: cultivation.Talent(c.Talent), Quality: cultivation.TechniqueQuality(c.Technique),
		Environment: cultivation.Environment(c.Environment), Resource: cultivation.SupportResource(c.Resource),
		HealthFraction:    float64(c.CurrentHealth) / float64(c.MaximumHealth),
		GrowthCoefficient: c.GrowthCoefficient,
	}
}

func advance(state Snapshot, effectiveSeconds10 int64, modifiers cultivation.TrainingModifiers) cultivation.Progress {
	days := float64(effectiveSeconds10) / (10 * 86400)
	return cultivation.AdvanceCultivationFrom(state.Cultivation, state.Accumulated, days, modifiers)
}

func checkedMultiply(value, factor int64) (int64, bool) {
	if value < 0 || factor < 0 || factor != 0 && value > math.MaxInt64/factor {
		return 0, false
	}
	return value * factor, true
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func sourceRefValid(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && trimmed == value && len(value) <= maxSourceRefLength
}

func remainingIntent(interval Interval, segmentIndex int, from int64, reason string) *RemainingIntent {
	left := make([]Segment, 0, len(interval.Segments)-segmentIndex)
	for i := segmentIndex; i < len(interval.Segments); i++ {
		segment := interval.Segments[i]
		if segment.To <= from {
			continue
		}
		if segment.From < from {
			segment.From = from
		}
		left = append(left, segment)
	}
	if len(left) == 0 {
		return nil
	}
	return &RemainingIntent{From: from, To: interval.To, Reason: reason, Segments: left}
}
