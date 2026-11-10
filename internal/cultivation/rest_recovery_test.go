package cultivation

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func recoveryRequest(mode RecoveryMode, start, current Attributes, settled, target int64) RecoveryRequest {
	return RecoveryRequest{
		ActivityID: "rest-activity-1", Mode: mode,
		EligibilityAtStart: RecoveryQualified, EligibilityNow: RecoveryQualified,
		StartAttributes: start, CurrentAttributes: current,
		SettledSeconds: settled, TargetSeconds: target,
	}
}

func TestSafeRecoveryUsesRestAndMeditationRates(t *testing.T) {
	health := Attributes{Health: 40, HealthMax: 100, Mana: 30, ManaMax: 80}
	candidate, err := PlanSafeRecovery(recoveryRequest(RecoveryHealthRest, health, health, 0, 24*60*60))
	if err != nil {
		t.Fatal(err)
	}
	if candidate.CandidatePool != 45 || candidate.Increment != 5 || candidate.CumulativeRecovered != 5 {
		t.Fatalf("daily health recovery = %+v, want pool 45 and increment 5", candidate)
	}
	mana := Attributes{Health: 50, HealthMax: 100, Mana: 80, ManaMax: 100}
	candidate, err = PlanSafeRecovery(recoveryRequest(RecoverySafeMeditation, mana, mana, 0, 60*60))
	if err != nil {
		t.Fatal(err)
	}
	if candidate.CandidatePool != 90 || candidate.Increment != 10 {
		t.Fatalf("hourly mana recovery = %+v, want pool 90 and increment 10", candidate)
	}
}

func TestRecoveryCumulativeRoundingAndReplay(t *testing.T) {
	start := Attributes{Health: 20, HealthMax: 81, Mana: 7, ManaMax: 80}
	oneShot, err := PlanSafeRecovery(recoveryRequest(RecoveryHealthRest, start, start, 0, 24*60*60))
	if err != nil {
		t.Fatal(err)
	}
	if oneShot.CandidatePool != 24 || oneShot.Increment != 4 {
		t.Fatalf("one-day fractional recovery = %+v, want cumulative floor 4", oneShot)
	}

	current := start
	var total int
	const minute = int64(60)
	for elapsed := minute; elapsed <= 24*60*60; elapsed += minute {
		request := recoveryRequest(RecoveryHealthRest, start, current, elapsed-minute, elapsed)
		part, err := PlanSafeRecovery(request)
		if err != nil {
			t.Fatalf("elapsed %d: %v", elapsed, err)
		}
		total += part.Increment
		current.Health = part.CandidatePool
	}
	if total != oneShot.Increment || current.Health != oneShot.CandidatePool {
		t.Fatalf("minute segments recovered %d to %d, one-shot recovered %d to %d", total, current.Health, oneShot.Increment, oneShot.CandidatePool)
	}
	replay := recoveryRequest(RecoveryHealthRest, start, current, 24*60*60, 24*60*60)
	candidate, err := PlanSafeRecovery(replay)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Increment != 0 || candidate.CandidatePool != current.Health {
		t.Fatalf("same-target replay = %+v, want zero increment", candidate)
	}
}

func TestMeditationKeepsSecondPrecisionAcrossSegments(t *testing.T) {
	start := Attributes{Health: 60, HealthMax: 100, Mana: 0, ManaMax: 160}
	oneShot, err := PlanSafeRecovery(recoveryRequest(RecoverySafeMeditation, start, start, 0, 60*60))
	if err != nil {
		t.Fatal(err)
	}
	current := start
	var total int
	for second := int64(1); second <= 60*60; second++ {
		part, err := PlanSafeRecovery(recoveryRequest(RecoverySafeMeditation, start, current, second-1, second))
		if err != nil {
			t.Fatalf("second %d: %v", second, err)
		}
		total += part.Increment
		current.Mana = part.CandidatePool
	}
	if total != 16 || current.Mana != oneShot.CandidatePool || oneShot.CandidatePool != 16 {
		t.Fatalf("one-shot=%+v segmented total=%d current=%d, want 16", oneShot, total, current.Mana)
	}
}

func TestRecoveryCapsAtMaximumAndAllowsQualifiedZeroHealth(t *testing.T) {
	start := Attributes{Health: 195, HealthMax: 200, Mana: 3, ManaMax: 40}
	candidate, err := PlanSafeRecovery(recoveryRequest(RecoveryHealthRest, start, start, 0, 10*24*60*60))
	if err != nil {
		t.Fatal(err)
	}
	if candidate.CandidatePool != 200 || candidate.Increment != 5 {
		t.Fatalf("health exceeded or missed its cap: %+v", candidate)
	}

	incapacitated := Attributes{Health: 0, HealthMax: 100, Mana: 0, ManaMax: 80}
	candidate, err = PlanSafeRecovery(recoveryRequest(RecoveryHealthRest, incapacitated, incapacitated, 0, 24*60*60))
	if err != nil {
		t.Fatalf("qualified zero-health survivor was rejected: %v", err)
	}
	if candidate.CandidatePool != 5 {
		t.Fatalf("qualified zero-health survivor recovered to %d, want 5 without a death decision", candidate.CandidatePool)
	}
}

func TestRecoveryRejectsEligibilityChangesAndUnsafeStates(t *testing.T) {
	attrs := Attributes{Health: 50, HealthMax: 100, Mana: 30, ManaMax: 80}
	for _, state := range []RecoveryEligibility{RecoveryInBattle, RecoveryInDanger, RecoveryDead, RecoveryNeedsSpecialTreatment} {
		t.Run(string(state), func(t *testing.T) {
			request := recoveryRequest(RecoveryHealthRest, attrs, attrs, 0, 86400)
			request.EligibilityAtStart = state
			request.EligibilityNow = state
			if _, err := PlanSafeRecovery(request); !errors.Is(err, ErrRecoveryIneligible) {
				t.Fatalf("eligibility %q error = %v", state, err)
			}
		})
	}
	changed := recoveryRequest(RecoverySafeMeditation, attrs, attrs, 0, 3600)
	changed.EligibilityNow = RecoveryInDanger
	if _, err := PlanSafeRecovery(changed); !errors.Is(err, ErrRecoveryEligibilityChanged) {
		t.Fatalf("changed qualification error = %v", err)
	}
}

func TestRecoveryRequiresNewSegmentWhenPoolMaximumChanges(t *testing.T) {
	start := Attributes{Health: 50, HealthMax: 100, Mana: 30, ManaMax: 80}
	current := start
	current.HealthMax = 120
	request := recoveryRequest(RecoveryHealthRest, start, current, 0, 86400)
	if _, err := PlanSafeRecovery(request); !errors.Is(err, ErrRecoveryMaximumChanged) {
		t.Fatalf("changed maximum error = %v", err)
	}
}

func TestRecoveryRejectsInvalidAndInconsistentSnapshots(t *testing.T) {
	attrs := Attributes{Health: 50, HealthMax: 100, Mana: 30, ManaMax: 80}
	cases := []struct {
		name string
		edit func(*RecoveryRequest)
		want error
	}{
		{name: "negative-duration", edit: func(r *RecoveryRequest) { r.TargetSeconds = -1 }, want: ErrRecoveryNegativeValue},
		{name: "reversed-target", edit: func(r *RecoveryRequest) { r.SettledSeconds = 3600; r.TargetSeconds = 60 }, want: ErrRecoveryTimeReversal},
		{name: "missing-maximum", edit: func(r *RecoveryRequest) { r.StartAttributes.HealthMax = 0; r.CurrentAttributes.HealthMax = 0 }, want: ErrRecoveryMissingMaximum},
		{name: "pool-over-start-maximum", edit: func(r *RecoveryRequest) { r.StartAttributes.Health = 101 }, want: ErrRecoveryPoolOutOfRange},
		{name: "pool-over-current-maximum", edit: func(r *RecoveryRequest) { r.CurrentAttributes.Health = 101 }, want: ErrRecoveryPoolOutOfRange},
		{name: "unaccounted-pool-change", edit: func(r *RecoveryRequest) {
			r.SettledSeconds = 86400
			r.TargetSeconds = 86400
			r.CurrentAttributes.Health = 56
		}, want: ErrRecoverySnapshotMismatch},
		{name: "missing-activity-id", edit: func(r *RecoveryRequest) { r.ActivityID = " " }, want: ErrRecoveryMissingActivityID},
		{name: "unknown-mode", edit: func(r *RecoveryRequest) { r.Mode = "unknown" }, want: ErrRecoveryUnknownMode},
		{name: "unknown-eligibility", edit: func(r *RecoveryRequest) { r.EligibilityNow = "unknown" }, want: ErrRecoveryUnknownEligibility},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := recoveryRequest(RecoveryHealthRest, attrs, attrs, 0, 60)
			tc.edit(&request)
			if _, err := PlanSafeRecovery(request); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRecoveryUsesExactIntegerArithmeticAtLargestInputs(t *testing.T) {
	maximum := math.MaxInt
	start := Attributes{Health: 0, HealthMax: maximum, Mana: 0, ManaMax: 80}
	candidate, err := PlanSafeRecovery(recoveryRequest(RecoveryHealthRest, start, start, 0, math.MaxInt64))
	if err != nil {
		t.Fatal(err)
	}
	if candidate.CandidatePool != maximum || candidate.Increment != maximum {
		t.Fatalf("large exact projection = %+v, want capped pool %d", candidate, maximum)
	}
}

func TestRecoveryReturnsOnlySelectedPoolAndDoesNotMutateSnapshots(t *testing.T) {
	start := Attributes{Health: 40, HealthMax: 100, Mana: 20, ManaMax: 80, Attack: 11, Defense: 7}
	current := start
	before := current
	candidate, err := PlanSafeRecovery(recoveryRequest(RecoveryHealthRest, start, current, 0, 86400))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current, before) {
		t.Fatalf("recovery mutated input attributes: before=%+v after=%+v", before, current)
	}
	if candidate.CandidatePool != 45 || candidate.Mode != RecoveryHealthRest {
		t.Fatalf("candidate changed an unselected pool or field: %+v", candidate)
	}
}
