package npcgrowth

import "ta/internal/cultivation"

func calculateRecovery(input Input, intervalHash string) (Candidate, error) {
	if input.BreakthroughIntent != nil {
		return Candidate{}, diagnostic("breakthrough_during_recovery", "$.breakthrough_intent", "A breakthrough attempt cannot be submitted during a recovery activity.")
	}
	if len(input.Interval.Segments) != 1 {
		return Candidate{}, diagnostic("recovery_interval_shape", "$.interval.segments", "A recovery interval must describe one continuous safe activity.")
	}
	segment := input.Interval.Segments[0]
	mode := cultivation.RecoveryMode("")
	switch segment.Activity {
	case ActivityHealthRest:
		mode = cultivation.RecoveryHealthRest
	case ActivitySafeMeditation:
		mode = cultivation.RecoverySafeMeditation
	default:
		return Candidate{}, diagnostic("unsupported_recovery_activity", "$.interval.segments[0].activity", "The requested recovery activity is unsupported.")
	}
	if !sourceRefValid(segment.RecoveryActivityID) || !sourceRefValid(segment.RecoverySourceRef) {
		return Candidate{}, diagnostic("recovery_activity_source_required", "$.interval.segments[0].recovery_activity_id", "Recovery needs a stable activity identity and source.")
	}
	if segment.SupportUse != nil {
		return Candidate{}, diagnostic("support_during_recovery", "$.interval.segments[0].support_use", "A cultivation-support dose is only used during actual training.")
	}
	if segment.Conditions.CurrentMana < 0 || segment.Conditions.MaximumMana <= 0 || segment.Conditions.CurrentMana > segment.Conditions.MaximumMana {
		return Candidate{}, diagnostic("invalid_recovery_mana", "$.interval.segments[0].conditions.current_mana", "Recovery requires the actor's current mana and maximum from the same snapshot.")
	}
	pools, ok := snapshotPools(input.State)
	if !ok {
		return Candidate{}, diagnostic("missing_actor_pools", "$.state.resources", "Recovery needs the actor's current health and mana snapshot.")
	}
	if segment.Conditions.MaximumHealth != pools.HealthMaximum || segment.Conditions.MaximumMana != pools.ManaMaximum ||
		segment.Conditions.CurrentHealth != pools.Health || segment.Conditions.CurrentMana != pools.Mana {
		return Candidate{}, diagnostic("recovery_pool_snapshot_mismatch", "$.interval.segments[0].conditions", "Recovery conditions must match the actor's current pools and formal maxima.")
	}

	recovery := input.State.Recovery
	if recovery == nil || recovery.ID != segment.RecoveryActivityID || recovery.Mode != string(mode) {
		if segment.RecoveryEligibilityAtStart == "" || segment.RecoveryEligibilityAtStart != segment.RecoveryEligibilityNow {
			return Candidate{}, diagnostic("recovery_eligibility_required", "$.interval.segments[0].recovery_eligibility_at_start", "A new recovery activity must remain safe and qualified from its start.")
		}
		recovery = &RecoveryActivity{
			ID: segment.RecoveryActivityID, SourceRef: segment.RecoverySourceRef, Mode: string(mode),
			EligibilityAtStart: segment.RecoveryEligibilityAtStart, StartedAtSeconds: segment.From,
			StartHealth: pools.Health, StartMana: pools.Mana,
			StartHealthMaximum: pools.HealthMaximum, StartManaMaximum: pools.ManaMaximum,
		}
	} else if recovery.SourceRef != segment.RecoverySourceRef || recovery.EligibilityAtStart != segment.RecoveryEligibilityAtStart ||
		recovery.StartHealthMaximum != segment.Conditions.MaximumHealth || recovery.StartManaMaximum != segment.Conditions.MaximumMana {
		return Candidate{}, diagnostic("recovery_activity_identity_conflict", "$.interval.segments[0].recovery_activity_id", "A continuing recovery must keep its original source and safety qualification.")
	}
	if recovery.SettledSeconds < 0 || recovery.StartedAtSeconds < 0 || recovery.StartedAtSeconds > segment.From ||
		recovery.StartedAtSeconds > segment.From-recovery.SettledSeconds {
		return Candidate{}, diagnostic("recovery_time_reversal", "$.state.recovery", "The stored safe activity waterline does not match this completed interval.")
	}
	if recovery.StartedAtSeconds+recovery.SettledSeconds != segment.From {
		return Candidate{}, diagnostic("recovery_activity_gap", "$.state.recovery", "A paused recovery must begin a new sourced safe activity.")
	}
	startAttrs := cultivation.Attributes{
		Health: recovery.StartHealth, HealthMax: recovery.StartHealthMaximum,
		Mana: recovery.StartMana, ManaMax: recovery.StartManaMaximum,
	}
	currentAttrs := cultivation.Attributes{
		Health: pools.Health, HealthMax: pools.HealthMaximum,
		Mana: pools.Mana, ManaMax: pools.ManaMaximum,
	}
	through := segment.To
	if input.State.Breakthrough != nil && input.State.Breakthrough.RecoveryUntilSeconds > segment.From && through > input.State.Breakthrough.RecoveryUntilSeconds {
		through = input.State.Breakthrough.RecoveryUntilSeconds
	}
	target := recovery.SettledSeconds + through - segment.From
	if target < recovery.SettledSeconds {
		return Candidate{}, diagnostic("recovery_time_overflow", "$.interval.to_seconds", "The cumulative recovery duration overflows.")
	}
	planned, err := cultivation.PlanSafeRecovery(cultivation.RecoveryRequest{Rules: input.State.Cultivation.Rules(),
		ActivityID: recovery.ID, Mode: mode,
		EligibilityAtStart: cultivation.RecoveryEligibility(recovery.EligibilityAtStart),
		EligibilityNow:     cultivation.RecoveryEligibility(segment.RecoveryEligibilityNow),
		StartAttributes:    startAttrs, CurrentAttributes: currentAttrs,
		SettledSeconds: recovery.SettledSeconds, TargetSeconds: target,
	})
	if err != nil {
		return Candidate{}, diagnostic("recovery_refused", "$.interval.segments[0]", "The NPC cannot recover under the current safety, pool, or activity conditions.")
	}
	if mode == cultivation.RecoveryHealthRest {
		pools.Health = planned.CandidatePool
	} else {
		pools.Mana = planned.CandidatePool
	}
	updated := input.State
	updated.Resources = &pools
	if updated.Breakthrough != nil {
		bt := *updated.Breakthrough
		bt.CurrentHealth, bt.CurrentMana = pools.Health, pools.Mana
		if bt.RecoveryUntilSeconds > 0 && through >= bt.RecoveryUntilSeconds {
			bt.RecoveryUntilSeconds = 0
		}
		updated.Breakthrough = &bt
	}
	updated.Recovery = &RecoveryActivity{
		ID: recovery.ID, SourceRef: recovery.SourceRef, Mode: recovery.Mode,
		EligibilityAtStart: recovery.EligibilityAtStart, StartedAtSeconds: recovery.StartedAtSeconds,
		SettledSeconds: target, StartHealth: recovery.StartHealth, StartMana: recovery.StartMana,
		StartHealthMaximum: recovery.StartHealthMaximum, StartManaMaximum: recovery.StartManaMaximum,
	}
	candidate := Candidate{
		Status: StatusApplied, ActorID: input.ActorID, IntervalID: input.Interval.ID,
		IntervalHash: intervalHash, ExpectedRevision: input.State.Revision,
		NextState: updated, ConsumedThrough: through, Segments: []SegmentResult{}, Recovery: &planned,
	}
	if through < input.Interval.To {
		candidate.RemainingIntent = remainingIntent(input.Interval, segmentIndexAt(input.Interval, through), through, "Breakthrough recovery ended; continue the remaining safe activity in a new interval.")
	}
	return commitBreakthroughProgress(input, intervalHash, candidate, updated, through)
}

func snapshotPools(state Snapshot) (ActorPools, bool) {
	if state.Resources != nil {
		return *state.Resources, true
	}
	if state.Breakthrough == nil {
		return ActorPools{}, false
	}
	base := cultivation.BaseAttributes(state.Cultivation)
	return ActorPools{
		Health: state.Breakthrough.CurrentHealth, HealthMaximum: base.HealthMax,
		Mana: state.Breakthrough.CurrentMana, ManaMaximum: base.ManaMax,
	}, true
}
