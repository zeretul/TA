package npcgrowth

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"ta/internal/cultivation"
)

func sample() Input {
	return Input{
		SchemaVersion: InputVersion, ActorID: "npc.test", ActorSource: "story:actor:1", BaseRevision: 4,
		State: Snapshot{
			SourceRef: "story:actor:1", Revision: 4,
			Cultivation: cultivation.Qi(6), Accumulated: 125.5, SettledThroughSeconds: 1000,
		},
		Interval: Interval{
			ID: "time:1000-2593000", SourceRef: "story-time:event:1", From: 1000, To: 2593000,
			Segments: []Segment{{
				From: 1000, To: 2593000, Activity: ActivityOrdinaryTraining,
				ActivitySourceRef: "plan:npc:test", Conditions: Conditions{
					SourceRef: "state:npc:test", Talent: string(cultivation.TalentNormal),
					Technique: string(cultivation.QualityOrdinary), TechniqueRef: "technique:basic", GrowthCoefficient: 1,
					Environment: string(cultivation.EnvironmentOrdinary), Resource: string(cultivation.SupportNone),
					CurrentHealth: 100, MaximumHealth: 100,
				},
			}},
		},
	}
}

func breakthroughSample(t *testing.T) Input {
	t.Helper()
	input := sample()
	cultivationState := cultivation.Qi(cultivation.QiLayers)
	required, _, ok := cultivation.ProgressRequired(cultivationState)
	if !ok {
		t.Fatal("fixed breakthrough test stage has no progress requirement")
	}
	base := cultivation.BaseAttributes(cultivationState)
	input.State.Cultivation = cultivationState
	input.State.Accumulated = required
	input.State.Breakthrough = &BreakthroughState{CurrentHealth: base.HealthMax, CurrentMana: base.ManaMax}
	input.Interval.ID = "time:breakthrough:1000-865000"
	input.Interval.To = input.Interval.From + 10*86400
	segment := &input.Interval.Segments[0]
	segment.To = input.Interval.To
	segment.Activity = ActivityBreakthroughPrep
	segment.Conditions.CurrentHealth = base.HealthMax
	segment.Conditions.MaximumHealth = base.HealthMax
	input.BreakthroughIntent = &BreakthroughIntent{
		AttemptID: "attempt:npc.test:foundation:1", SourceRef: "npc-plan:npc.test:attempt-1",
		Target: cultivation.BreakthroughFoundation, TechniqueSupportsTarget: true,
		TechniqueSourceRef: "technique:basic", ApplicabilitySourceRef: "world-rule:qi-to-foundation",
	}
	return input
}

func receiptFor(input Input, fingerprint string, roll float64) *RandomReceipt {
	return &RandomReceipt{
		AttemptID: input.BreakthroughIntent.AttemptID, SourceRef: "random-receipt:" + input.BreakthroughIntent.AttemptID,
		Roll: roll, ConditionFingerprint: fingerprint,
	}
}

func requireDiagnostic(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected diagnostic %q", code)
	}
	diagnosticErr, ok := err.(*DiagnosticError)
	if !ok || len(diagnosticErr.Diagnostics) == 0 || diagnosticErr.Diagnostics[0].Code != code {
		t.Fatalf("got error %v, want diagnostic %q", err, code)
	}
}

func TestUsesPublicCultivationAPIAndCarriesAccumulation(t *testing.T) {
	input := sample()
	got, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	modifiers := cultivation.TrainingModifiers{
		Talent: cultivation.TalentNormal, Quality: cultivation.QualityOrdinary,
		Environment: cultivation.EnvironmentOrdinary, Resource: cultivation.SupportNone, HealthFraction: 1, GrowthCoefficient: 1,
	}
	want := cultivation.AdvanceCultivationFrom(input.State.Cultivation, input.State.Accumulated, 15, modifiers)
	if got.NextState.Cultivation != want.Cultivation || math.Abs(got.NextState.Accumulated-want.Accumulated) > 1e-9 {
		t.Fatalf("candidate does not match public cultivation API: got %+v, want %+v", got.NextState, want)
	}
	if got.NextState.SettledThroughSeconds != input.Interval.To || !got.HasStateChange {
		t.Fatalf("candidate did not advance its proposed watermark: %+v", got)
	}
}

func TestSmallStageTransitionUsesRemainingEffectiveTime(t *testing.T) {
	input := sample()
	input.State.Accumulated = 3599
	input.Interval.Segments[0].Activity = ActivityFocusedSeclusion
	input.Interval.Segments[0].To = input.Interval.From + 86400*2
	input.Interval.To = input.Interval.Segments[0].To
	got, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.NextState.Cultivation != cultivation.Qi(7) || got.StagesAdvanced != 1 {
		t.Fatalf("small-stage threshold did not advance: %+v", got)
	}
}

func TestLongAndSplitIntervalsMatch(t *testing.T) {
	long, err := Calculate(sample())
	if err != nil {
		t.Fatal(err)
	}
	firstInput := sample()
	firstInput.Interval.ID, firstInput.Interval.To = "time:first-half", 1297000
	firstInput.Interval.Segments[0].To = firstInput.Interval.To
	first, err := Calculate(firstInput)
	if err != nil {
		t.Fatal(err)
	}
	secondInput := firstInput
	secondInput.State, secondInput.BaseRevision = first.NextState, first.NextState.Revision
	secondInput.Interval = Interval{
		ID: "time:second-half", SourceRef: "story-time:event:2", From: 1297000, To: 2593000,
		Segments: []Segment{{
			From: 1297000, To: 2593000, Activity: ActivityOrdinaryTraining,
			ActivitySourceRef: firstInput.Interval.Segments[0].ActivitySourceRef,
			Conditions:        firstInput.Interval.Segments[0].Conditions,
		}},
	}
	splitSecond, err := Calculate(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if long.NextState.Cultivation != splitSecond.NextState.Cultivation || math.Abs(long.NextState.Accumulated-splitSecond.NextState.Accumulated) > 1e-8 {
		t.Fatalf("long and split interval differ: long=%+v split=%+v", long.NextState, splitSecond.NextState)
	}
	if math.Abs(long.Gained-(first.Gained+splitSecond.Gained)) > 1e-8 {
		t.Fatalf("gained progress differs across split: long=%v split=%v", long.Gained, first.Gained+splitSecond.Gained)
	}
}

func TestDuplicateAndConflict(t *testing.T) {
	first, err := Calculate(sample())
	if err != nil {
		t.Fatal(err)
	}
	retry := sample()
	retry.State, retry.BaseRevision = first.NextState, first.NextState.Revision
	duplicate, err := Calculate(retry)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Status != StatusDuplicate || duplicate.Gained != 0 || duplicate.NextState.Accumulated != first.NextState.Accumulated {
		t.Fatalf("retry gained progress: %+v", duplicate)
	}
	retry.Interval.Segments[0].Activity = ActivityFocusedSeclusion
	if _, err := Calculate(retry); err == nil {
		t.Fatal("changed content under same identity was accepted")
	}
}

func TestCandidateNextStateCanBeRetriedThroughJSON(t *testing.T) {
	first, err := Calculate(sample())
	if err != nil {
		t.Fatal(err)
	}
	candidateJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var returned Candidate
	if err := json.Unmarshal(candidateJSON, &returned); err != nil {
		t.Fatal(err)
	}

	retry := sample()
	retry.State = returned.NextState
	retry.BaseRevision = retry.State.Revision
	retryJSON, err := json.Marshal(retry)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeInput(bytes.NewReader(retryJSON))
	if err != nil {
		t.Fatalf("candidate next_state could not be reused as strict input: %v; JSON=%s", err, retryJSON)
	}
	duplicate, err := Calculate(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Status != StatusDuplicate || duplicate.Gained != 0 || !reflect.DeepEqual(duplicate.NextState, first.NextState) {
		t.Fatalf("same interval was not a no-op retry: %+v", duplicate)
	}
}

func TestSnapshotJSONIncludesZeroCultivationFields(t *testing.T) {
	cases := []struct {
		name        string
		cultivation cultivation.Cultivation
		accumulated float64
	}{
		{name: "qi", cultivation: cultivation.Qi(6), accumulated: 125.5},
		{name: "major realm", cultivation: cultivation.Cultivation{Realm: cultivation.RealmFoundation, Stage: cultivation.StageEarly}},
		{name: "mortal zero value", cultivation: cultivation.Mortal()},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := sample()
			input.State.Cultivation = test.cultivation
			input.State.Accumulated = test.accumulated
			encoded, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := DecodeInput(bytes.NewReader(encoded))
			if err != nil {
				t.Fatalf("snapshot did not round-trip through strict input: %v; JSON=%s", err, encoded)
			}
			if decoded.State.Cultivation != test.cultivation || decoded.State.Accumulated != test.accumulated {
				t.Fatalf("snapshot changed during round-trip: got %+v, want %+v", decoded.State.Cultivation, test.cultivation)
			}
		})
	}
}

func TestNewIntervalContinuesFromJSONRoundTrippedCandidate(t *testing.T) {
	first, err := Calculate(sample())
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var restored Candidate
	if err := json.Unmarshal(firstJSON, &restored); err != nil {
		t.Fatal(err)
	}
	input := sample()
	input.State = restored.NextState
	input.BaseRevision = input.State.Revision
	input.Interval.ID = "time:continuation"
	input.Interval.SourceRef = "story-time:event:2"
	input.Interval.From = input.State.SettledThroughSeconds
	input.Interval.To = input.Interval.From + 86400
	input.Interval.Segments[0].From = input.Interval.From
	input.Interval.Segments[0].To = input.Interval.To

	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeInput(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("new interval could not continue from candidate JSON: %v", err)
	}
	continued, err := Calculate(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if continued.Status != StatusApplied || continued.NextState.Revision != restored.NextState.Revision+1 || continued.NextState.LastIntervalID != input.Interval.ID {
		t.Fatalf("new interval did not continue from the committed candidate snapshot: %+v", continued)
	}
}

func TestBottleneckReturnsUnconsumedIntent(t *testing.T) {
	input := sample()
	input.State.Cultivation, input.State.Accumulated = cultivation.Qi(cultivation.QiLayers), 16899
	input.Interval.To, input.Interval.ID = input.Interval.From+86400*10, "time:bottleneck"
	input.Interval.Segments[0].To = input.Interval.To
	got, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusAtBoundary || got.RemainingIntent == nil || got.ConsumedThrough >= input.Interval.To {
		t.Fatalf("bottleneck did not return remaining time: %+v", got)
	}
	if got.NextState.Cultivation != input.State.Cultivation || got.NextState.Accumulated != 16900 {
		t.Fatalf("bottleneck crossed without a breakthrough: %+v", got.NextState)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var restored Candidate
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.RemainingIntent, got.RemainingIntent) {
		t.Fatalf("remaining intent changed across JSON round-trip: got %+v, want %+v", restored.RemainingIntent, got.RemainingIntent)
	}
}

func TestAllocationsAndNoWindow(t *testing.T) {
	for _, item := range []struct {
		activity Activity
		source   string
		want     int64
	}{
		{ActivityFocusedSeclusion, "", 25920000},
		{ActivityOrdinaryTraining, "", 12960000},
		{ActivityTravelOpportunity, "rest-window:1", 5184000},
		{ActivityTravelNoWindow, "", 0},
		{ActivityInjuryPrevents, "", 0},
	} {
		input := sample()
		input.Interval.Segments[0].Activity = item.activity
		input.Interval.Segments[0].OpportunitySourceRef = item.source
		result, err := Calculate(input)
		if err != nil {
			t.Fatal(err)
		}
		if result.EffectiveSeconds10 != item.want {
			t.Fatalf("%s: got %d, want %d", item.activity, result.EffectiveSeconds10, item.want)
		}
		if item.want == 0 && result.Gained != 0 {
			t.Fatalf("%s unexpectedly grew", item.activity)
		}
	}
}

func TestTravelOpportunityNeedsSourceAndResourcesAreUnsupported(t *testing.T) {
	input := sample()
	input.Interval.Segments[0].Activity = ActivityTravelOpportunity
	if _, err := Calculate(input); err == nil {
		t.Fatal("travel without a sourced opportunity was accepted")
	}
	input = sample()
	input.Interval.Segments[0].Conditions.Resource = string(cultivation.SupportNormal)
	if _, err := Calculate(input); err == nil {
		t.Fatal("unconsumed support resource was accepted")
	}
}

func TestRejectsStaleOverlappingAndGappedIntervals(t *testing.T) {
	input := sample()
	input.BaseRevision--
	if _, err := Calculate(input); err == nil {
		t.Fatal("stale revision was accepted")
	}
	input = sample()
	input.Interval.From++
	if _, err := Calculate(input); err == nil {
		t.Fatal("interval gap was accepted")
	}
	input = sample()
	input.State.SettledThroughSeconds++
	if _, err := Calculate(input); err == nil {
		t.Fatal("overlapping interval was accepted")
	}
}

func TestStrictDecodeRejectsMalformedInput(t *testing.T) {
	valid, err := json.Marshal(sample())
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(valid, &root); err != nil {
		t.Fatal(err)
	}
	root["state"] = json.RawMessage("null")
	nullState, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(valid, &root); err != nil {
		t.Fatal(err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(root["state"], &state); err != nil {
		t.Fatal(err)
	}
	var cultivationState map[string]json.RawMessage
	if err := json.Unmarshal(state["cultivation"], &cultivationState); err != nil {
		t.Fatal(err)
	}
	cultivationState["stage"] = json.RawMessage("null")
	state["cultivation"], err = json.Marshal(cultivationState)
	if err != nil {
		t.Fatal(err)
	}
	root["state"], err = json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	nullStage, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, text string }{
		{"unknown field", strings.Replace(string(valid), "\"actor_id\":", "\"unexpected\":true,\"actor_id\":", 1)},
		{"null state", string(nullState)},
		{"null cultivation stage", string(nullStage)},
		{"trailing value", string(valid) + " {}"},
		{"overflow", strings.Replace(string(valid), "\"accumulated\":125.5", "\"accumulated\":1e999", 1)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeInput(bytes.NewBufferString(test.text)); err == nil {
				t.Fatal("invalid input was accepted")
			}
		})
	}
}

func TestBreakthroughPreflightUsesNPCIntentAndRejectsIneligibleWithoutCost(t *testing.T) {
	input := breakthroughSample(t)
	ready, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	base := cultivation.BaseAttributes(input.State.Cultivation)
	requirement := cultivation.CanAttemptBreakthrough(input.State.Cultivation, input.State.Accumulated, base, cultivation.BreakthroughFoundation, false)
	chance, ok := cultivation.BreakthroughChance(cultivation.BreakthroughFoundation, cultivation.BreakthroughModifiers{
		Talent: cultivation.TalentNormal, Quality: cultivation.QualityOrdinary,
	})
	if !ok || !requirement.Eligible || ready.Status != StatusBreakthroughReady || ready.HasStateChange || ready.ConsumedThrough != input.Interval.From {
		t.Fatalf("autonomous NPC preflight did not remain a pure eligibility check: %+v", ready)
	}
	if ready.Breakthrough.ManaCost != requirement.ManaCost || ready.Breakthrough.PreparationSeconds != 7*86400 || math.Abs(ready.Breakthrough.Chance-chance) > 1e-12 {
		t.Fatalf("candidate did not use the formal breakthrough API: got %+v req=%+v chance=%v", ready.Breakthrough, requirement, chance)
	}
	if ready.NextState.Breakthrough.CurrentMana != input.State.Breakthrough.CurrentMana || input.BreakthroughIntent.RandomReceipt != nil {
		t.Fatalf("preflight spent mana or supplied a roll: %+v", ready)
	}

	ineligible := breakthroughSample(t)
	ineligible.State.Breakthrough.CurrentHealth = cultivation.BaseAttributes(ineligible.State.Cultivation).HealthMax * 89 / 100
	ineligible.Interval.Segments[0].Conditions.CurrentHealth = ineligible.State.Breakthrough.CurrentHealth
	_, err = Calculate(ineligible)
	requireDiagnostic(t, err, "breakthrough_ineligible")
	if ineligible.BreakthroughIntent.RandomReceipt != nil || ineligible.State.Breakthrough.CurrentMana != base.ManaMax {
		t.Fatal("ineligible NPC received a roll or spent mana")
	}

	unsupported := breakthroughSample(t)
	unsupported.BreakthroughIntent.TechniqueSupportsTarget = false
	_, err = Calculate(unsupported)
	requireDiagnostic(t, err, "breakthrough_technique_unsupported")

	unsupported = breakthroughSample(t)
	unsupported.Interval.Segments[0].Conditions.Resource = "qi_pill"
	_, err = Calculate(unsupported)
	requireDiagnostic(t, err, "unsupported_resource_effect")
}

func TestBreakthroughPreparationChargesOnceAndRestoresFromJSON(t *testing.T) {
	input := breakthroughSample(t)
	input.Interval.To = input.Interval.From + 3600
	input.Interval.Segments[0].To = input.Interval.To
	ready, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	input.BreakthroughIntent.RandomReceipt = receiptFor(input, ready.Breakthrough.ConditionFingerprint, 0.42)
	prepared, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	requirement := cultivation.CanAttemptBreakthrough(input.State.Cultivation, input.State.Accumulated, cultivation.BaseAttributes(input.State.Cultivation), cultivation.BreakthroughFoundation, false)
	if prepared.Status != StatusBreakthroughPreparing || prepared.NextState.Breakthrough.Pending == nil || prepared.NextState.Breakthrough.CurrentMana != input.State.Breakthrough.CurrentMana-requirement.ManaCost {
		t.Fatalf("short interval did not preserve an exactly-paid preparation: %+v", prepared)
	}
	if prepared.NextState.Breakthrough.Pending.CompletesAtSeconds != input.Interval.From+7*86400 || prepared.ConsumedThrough != input.Interval.To {
		t.Fatalf("short interval received a premature breakthrough: %+v", prepared)
	}

	retry := input
	retry.State, retry.BaseRevision = prepared.NextState, prepared.NextState.Revision
	duplicate, err := Calculate(retry)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Status != StatusDuplicate || duplicate.NextState.Breakthrough.CurrentMana != prepared.NextState.Breakthrough.CurrentMana {
		t.Fatalf("same interval retry charged or prepared again: %+v", duplicate)
	}

	encoded, err := json.Marshal(prepared)
	if err != nil {
		t.Fatal(err)
	}
	var restored Candidate
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	continuation := input
	continuation.State = restored.NextState
	continuation.BaseRevision = restored.NextState.Revision
	continuation.BreakthroughIntent = nil
	continuation.Interval.ID = "time:breakthrough:continued"
	continuation.Interval.From = restored.NextState.SettledThroughSeconds
	continuation.Interval.To = continuation.Interval.From + 86400
	continuation.Interval.Segments[0].From = continuation.Interval.From
	continuation.Interval.Segments[0].To = continuation.Interval.To
	requestJSON, err := json.Marshal(continuation)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeInput(bytes.NewReader(requestJSON))
	if err != nil {
		t.Fatalf("pending preparation did not survive strict JSON recovery: %v", err)
	}
	continued, err := Calculate(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if continued.Status != StatusBreakthroughPreparing || continued.NextState.Breakthrough.CurrentMana != prepared.NextState.Breakthrough.CurrentMana || continued.NextState.Breakthrough.Pending.StartedAtSeconds != input.Interval.From {
		t.Fatalf("JSON continuation reset start conditions or paid twice: %+v", continued)
	}
}

func TestOneLongBreakthroughIntervalMatchesSplitPreparationAndStopsOnce(t *testing.T) {
	longInput := breakthroughSample(t)
	ready, err := Calculate(longInput)
	if err != nil {
		t.Fatal(err)
	}
	longInput.BreakthroughIntent.RandomReceipt = receiptFor(longInput, ready.Breakthrough.ConditionFingerprint, 0.99)
	long, err := Calculate(longInput)
	if err != nil {
		t.Fatal(err)
	}
	if long.Status != StatusBreakthroughFailed || long.ConsumedThrough != longInput.Interval.From+7*86400 || long.RemainingIntent == nil {
		t.Fatalf("one long interval did not stop after one attempt at its actual completion: %+v", long)
	}

	splitInput := breakthroughSample(t)
	splitInput.Interval.To = splitInput.Interval.From + 86400
	splitInput.Interval.Segments[0].To = splitInput.Interval.To
	splitReady, err := Calculate(splitInput)
	if err != nil {
		t.Fatal(err)
	}
	splitInput.BreakthroughIntent.RandomReceipt = receiptFor(splitInput, splitReady.Breakthrough.ConditionFingerprint, 0.99)
	first, err := Calculate(splitInput)
	if err != nil {
		t.Fatal(err)
	}
	secondInput := splitInput
	secondInput.State, secondInput.BaseRevision = first.NextState, first.NextState.Revision
	secondInput.BreakthroughIntent = nil
	secondInput.Interval.ID = "time:breakthrough:second-part"
	secondInput.Interval.From = first.NextState.SettledThroughSeconds
	secondInput.Interval.To = longInput.Interval.To
	secondInput.Interval.Segments[0].From = secondInput.Interval.From
	secondInput.Interval.Segments[0].To = secondInput.Interval.To
	second, err := Calculate(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != long.Status || second.ConsumedThrough != long.ConsumedThrough || second.RemainingIntent == nil ||
		second.NextState.Cultivation != long.NextState.Cultivation || second.NextState.Accumulated != long.NextState.Accumulated ||
		second.NextState.Breakthrough.CurrentHealth != long.NextState.Breakthrough.CurrentHealth ||
		second.NextState.Breakthrough.CurrentMana != long.NextState.Breakthrough.CurrentMana ||
		second.NextState.Breakthrough.RecoveryUntilSeconds != long.NextState.Breakthrough.RecoveryUntilSeconds {
		t.Fatalf("long and split preparation differ: long=%+v split=%+v", long, second)
	}
	if second.NextState.Breakthrough.LastAttempt == nil || second.NextState.Breakthrough.Pending != nil {
		t.Fatalf("split preparation did not store its single completed attempt: %+v", second.NextState.Breakthrough)
	}
}

func TestBreakthroughSuccessCarriesPoolsAndRecoveryRechecksEligibility(t *testing.T) {
	successInput := breakthroughSample(t)
	ready, err := Calculate(successInput)
	if err != nil {
		t.Fatal(err)
	}
	successInput.BreakthroughIntent.RandomReceipt = receiptFor(successInput, ready.Breakthrough.ConditionFingerprint, 0)
	success, err := Calculate(successInput)
	if err != nil {
		t.Fatal(err)
	}
	if success.Status != StatusBreakthroughSucceeded || success.NextState.Cultivation != (cultivation.Cultivation{Realm: cultivation.RealmFoundation, Stage: cultivation.StageEarly}) || success.NextState.Accumulated != 0 {
		t.Fatalf("formal success result was not applied: %+v", success)
	}
	if success.NextState.Breakthrough.CurrentHealth != successInput.State.Breakthrough.CurrentHealth ||
		success.NextState.Breakthrough.CurrentMana != successInput.State.Breakthrough.CurrentMana-ready.Breakthrough.ManaCost ||
		success.NextState.Breakthrough.RecoveryUntilSeconds != 0 {
		t.Fatalf("success refilled current pools or imposed recovery: %+v", success.NextState.Breakthrough)
	}

	failureInput := breakthroughSample(t)
	failureReady, err := Calculate(failureInput)
	if err != nil {
		t.Fatal(err)
	}
	failureInput.BreakthroughIntent.RandomReceipt = receiptFor(failureInput, failureReady.Breakthrough.ConditionFingerprint, 0.99)
	failure, err := Calculate(failureInput)
	if err != nil {
		t.Fatal(err)
	}
	apiAttrs := cultivation.BaseAttributes(failureInput.State.Cultivation)
	apiAttrs.Mana -= failureReady.Breakthrough.ManaCost
	wantFailure, ok := cultivation.ResolveBreakthrough(failureInput.State.Cultivation, failureInput.State.Accumulated, apiAttrs,
		cultivation.BreakthroughFoundation, cultivation.BreakthroughModifiers{Talent: cultivation.TalentNormal, Quality: cultivation.QualityOrdinary}, 0.99)
	if !ok || failure.NextState.Accumulated != wantFailure.Accumulated || failure.NextState.Breakthrough.CurrentHealth != wantFailure.Health {
		t.Fatalf("failure did not use formal current-health and progress penalties: got=%+v want=%+v", failure.NextState, wantFailure)
	}
	if failure.NextState.Breakthrough.RecoveryUntilSeconds != failure.ConsumedThrough+int64(wantFailure.RecoveryDays*86400) {
		t.Fatalf("recovery did not begin at the actual attempt completion: %+v", failure.NextState.Breakthrough)
	}

	recovery := breakthroughSample(t)
	recovery.State = failure.NextState
	recovery.BaseRevision = failure.NextState.Revision
	recovery.BreakthroughIntent.AttemptID = "attempt:npc.test:foundation:2"
	recovery.BreakthroughIntent.SourceRef = "npc-plan:npc.test:attempt-2"
	recovery.BreakthroughIntent.RandomReceipt = nil
	recovery.Interval.ID = "time:recovery"
	recovery.Interval.From = failure.NextState.SettledThroughSeconds
	recovery.Interval.To = recovery.Interval.From + 10*86400
	recovery.Interval.Segments[0].From = recovery.Interval.From
	recovery.Interval.Segments[0].To = recovery.Interval.To
	recovery.Interval.Segments[0].Activity = ActivityOrdinaryTraining
	recovery.Interval.Segments[0].Conditions.CurrentHealth = failure.NextState.Breakthrough.CurrentHealth
	recovery.Interval.Segments[0].Conditions.MaximumHealth = apiAttrs.HealthMax
	recoveryResult, err := Calculate(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if recoveryResult.Status != StatusRecoveryEnded || recoveryResult.ConsumedThrough != failure.NextState.Breakthrough.RecoveryUntilSeconds || recoveryResult.RemainingIntent == nil {
		t.Fatalf("recovery did not stop at its real completion boundary: %+v", recoveryResult)
	}
	if recoveryResult.NextState.Breakthrough.CurrentHealth != failure.NextState.Breakthrough.CurrentHealth || recoveryResult.NextState.Breakthrough.CurrentMana != failure.NextState.Breakthrough.CurrentMana {
		t.Fatal("recovery interval replenished health or mana without a sourced recovery action")
	}

	newAttempt := recovery
	newAttempt.State, newAttempt.BaseRevision = recoveryResult.NextState, recoveryResult.NextState.Revision
	newAttempt.Interval.ID = "time:after-recovery"
	newAttempt.Interval.From = recoveryResult.NextState.SettledThroughSeconds
	newAttempt.Interval.To = newAttempt.Interval.From + 86400
	newAttempt.Interval.Segments[0].From = newAttempt.Interval.From
	newAttempt.Interval.Segments[0].To = newAttempt.Interval.To
	newAttempt.Interval.Segments[0].Activity = ActivityBreakthroughPrep
	_, err = Calculate(newAttempt)
	requireDiagnostic(t, err, "breakthrough_ineligible")
}

func TestBreakthroughAttemptIdentityCannotChangeDuringPreparation(t *testing.T) {
	input := breakthroughSample(t)
	input.Interval.To = input.Interval.From + 3600
	input.Interval.Segments[0].To = input.Interval.To
	ready, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	input.BreakthroughIntent.RandomReceipt = receiptFor(input, ready.Breakthrough.ConditionFingerprint, 0.42)
	prepared, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	changed := input
	changed.State, changed.BaseRevision = prepared.NextState, prepared.NextState.Revision
	changed.Interval.ID = "time:breakthrough:changed-roll"
	changed.Interval.From = prepared.NextState.SettledThroughSeconds
	changed.Interval.To = changed.Interval.From + 3600
	changed.Interval.Segments[0].From = changed.Interval.From
	changed.Interval.Segments[0].To = changed.Interval.To
	changedIntent := *input.BreakthroughIntent
	changedReceipt := *input.BreakthroughIntent.RandomReceipt
	changedReceipt.Roll = 0.1
	changedIntent.RandomReceipt = &changedReceipt
	changed.BreakthroughIntent = &changedIntent
	_, err = Calculate(changed)
	requireDiagnostic(t, err, "attempt_identity_conflict")
	changed.BreakthroughIntent.AttemptID = "attempt:npc.test:foundation:other"
	changedReceipt.AttemptID = changed.BreakthroughIntent.AttemptID
	changed.BreakthroughIntent.RandomReceipt = &changedReceipt
	_, err = Calculate(changed)
	requireDiagnostic(t, err, "attempt_identity_conflict")
}

func TestBreakthroughPreparationStopsAtConditionBoundary(t *testing.T) {
	input := breakthroughSample(t)
	input.Interval.To = input.Interval.From + 86400
	input.Interval.Segments[0].To = input.Interval.To
	ready, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	input.BreakthroughIntent.RandomReceipt = receiptFor(input, ready.Breakthrough.ConditionFingerprint, 0.42)
	prepared, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	boundary := prepared.NextState.SettledThroughSeconds + 4*3600
	changedConditions := prepared.NextState.Breakthrough.Pending.Conditions
	changedConditions.CurrentHealth -= 40
	interval := Input{
		SchemaVersion: InputVersion, ActorID: input.ActorID, ActorSource: input.ActorSource,
		BaseRevision: prepared.NextState.Revision, State: prepared.NextState,
		Interval: Interval{
			ID: "time:condition-boundary", SourceRef: "story-time:condition-change",
			From: prepared.NextState.SettledThroughSeconds, To: boundary + 3600,
			Segments: []Segment{
				{From: prepared.NextState.SettledThroughSeconds, To: boundary, Activity: ActivityBreakthroughPrep, ActivitySourceRef: "plan:attempt", Conditions: prepared.NextState.Breakthrough.Pending.Conditions},
				{From: boundary, To: boundary + 3600, Activity: ActivityBreakthroughPrep, ActivitySourceRef: "plan:attempt", Conditions: changedConditions},
			},
		},
	}
	interrupted, err := Calculate(interval)
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.Status != StatusPreparationInterrupted || interrupted.ConsumedThrough != boundary || interrupted.RemainingIntent == nil || interrupted.Breakthrough.Outcome != nil {
		t.Fatalf("changed conditions were calculated past the true boundary: %+v", interrupted)
	}
	if interrupted.NextState.Breakthrough.Pending != nil || interrupted.NextState.Breakthrough.CurrentMana != prepared.NextState.Breakthrough.CurrentMana {
		t.Fatalf("interrupted preparation was not safely retained as a paid, non-rolling result: %+v", interrupted.NextState.Breakthrough)
	}
}

func TestPendingPreparationRejectsInconsistentRestoredDurationFeeAndWatermark(t *testing.T) {
	makeRestoredInput := func(t *testing.T) (Input, Candidate) {
		t.Helper()
		input := breakthroughSample(t)
		input.Interval.To = input.Interval.From + 3600
		input.Interval.Segments[0].To = input.Interval.To
		ready, err := Calculate(input)
		if err != nil {
			t.Fatal(err)
		}
		input.BreakthroughIntent.RandomReceipt = receiptFor(input, ready.Breakthrough.ConditionFingerprint, 0)
		prepared, err := Calculate(input)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(prepared.NextState)
		if err != nil {
			t.Fatal(err)
		}
		var restored Snapshot
		if err := json.Unmarshal(encoded, &restored); err != nil {
			t.Fatal(err)
		}
		continued := input
		continued.State = restored
		continued.BaseRevision = restored.Revision
		continued.BreakthroughIntent = nil
		continued.Interval.ID = "time:breakthrough:restored-next"
		continued.Interval.From = restored.SettledThroughSeconds
		continued.Interval.To = continued.Interval.From + 3600
		continued.Interval.Segments[0].From = continued.Interval.From
		continued.Interval.Segments[0].To = continued.Interval.To
		return continued, prepared
	}

	t.Run("shortened formal duration", func(t *testing.T) {
		input, prepared := makeRestoredInput(t)
		input.State.Breakthrough.Pending.CompletesAtSeconds = prepared.NextState.Breakthrough.Pending.StartedAtSeconds + 7200
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeInput(bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		_, err = Calculate(decoded)
		requireDiagnostic(t, err, "invalid_pending_attempt")
	})

	t.Run("forged preparation fee", func(t *testing.T) {
		input, _ := makeRestoredInput(t)
		input.State.Breakthrough.Pending.ManaCost--
		_, err := Calculate(input)
		requireDiagnostic(t, err, "invalid_pending_attempt")
	})

	t.Run("forged original resource snapshot", func(t *testing.T) {
		input, _ := makeRestoredInput(t)
		input.State.Breakthrough.Pending.StartMana = cultivation.BaseAttributes(input.State.Cultivation).ManaMax + 1
		_, err := Calculate(input)
		requireDiagnostic(t, err, "invalid_pending_attempt")
	})

	t.Run("watermark outside preparation", func(t *testing.T) {
		input, prepared := makeRestoredInput(t)
		input.State.SettledThroughSeconds = prepared.NextState.Breakthrough.Pending.CompletesAtSeconds
		input.Interval.From = input.State.SettledThroughSeconds
		input.Interval.To = input.Interval.From + 3600
		input.Interval.Segments[0].From = input.Interval.From
		input.Interval.Segments[0].To = input.Interval.To
		_, err := Calculate(input)
		requireDiagnostic(t, err, "invalid_pending_attempt")
	})
}

func TestRestoredPreparationCompletesAtFormalSevenDayBoundaryWithoutRecharging(t *testing.T) {
	input := breakthroughSample(t)
	input.Interval.To = input.Interval.From + 3600
	input.Interval.Segments[0].To = input.Interval.To
	ready, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	input.BreakthroughIntent.RandomReceipt = receiptFor(input, ready.Breakthrough.ConditionFingerprint, 0)
	prepared, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	continuation := input
	continuation.State = prepared.NextState
	continuation.BaseRevision = prepared.NextState.Revision
	continuation.BreakthroughIntent = nil
	continuation.Interval.ID = "time:breakthrough:exact-completion"
	continuation.Interval.From = prepared.NextState.SettledThroughSeconds
	continuation.Interval.To = prepared.NextState.Breakthrough.Pending.CompletesAtSeconds
	continuation.Interval.Segments[0].From = continuation.Interval.From
	continuation.Interval.Segments[0].To = continuation.Interval.To
	completed, err := Calculate(continuation)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusBreakthroughSucceeded || completed.ConsumedThrough != input.Interval.From+7*86400 ||
		completed.NextState.Breakthrough.CurrentMana != input.State.Breakthrough.CurrentMana-ready.Breakthrough.ManaCost ||
		completed.Breakthrough.ManaCost != ready.Breakthrough.ManaCost {
		t.Fatalf("restored preparation did not complete at the formal seven-day boundary with one fee: %+v", completed)
	}
}

// A spirit beast's registered method carries its own growth coefficient, and the
// design states the resulting rate in concrete terms: a healthy beast training a
// whole rule month at refining layer L banks 12×L progress under a 0.60 method.
// That number is the contract the spirit-beast chain is accepted against, so it
// is checked against the public entry point rather than re-derived from the
// formula, and the same month split in two must bank the same total.
func TestSpiritBeastMethodBanksTwelvePerLayerPerRuleMonth(t *testing.T) {
	const coefficient = 0.60
	for _, layer := range []int{1, 2, 6} {
		month := func() Input {
			input := sample()
			input.State.Cultivation = cultivation.Qi(layer)
			input.State.Accumulated = 0
			start := input.State.SettledThroughSeconds
			input.Interval = Interval{
				ID: "time:beast-month", SourceRef: "story-time:beast", From: start, To: start + cultivation.DaysPerRuleMonth*86400,
				Segments: []Segment{{
					From: start, To: start + cultivation.DaysPerRuleMonth*86400, Activity: ActivityFocusedSeclusion,
					ActivitySourceRef: "plan:beast:seclusion", Conditions: Conditions{
						SourceRef: "state:beast", Talent: string(cultivation.TalentNormal),
						Technique: string(cultivation.QualityOrdinary), TechniqueRef: "method.spirit_beast_breathing",
						GrowthCoefficient: coefficient,
						Environment:       string(cultivation.EnvironmentOrdinary), Resource: string(cultivation.SupportNone),
						CurrentHealth: 100, MaximumHealth: 100,
					},
				}},
			}
			return input
		}
		want := 12.0 * float64(layer)

		whole, err := Calculate(month())
		if err != nil {
			t.Fatalf("layer %d whole month: %v", layer, err)
		}
		if math.Abs(whole.Gained-want) > 1e-9 {
			t.Fatalf("layer %d banked %v for one focused rule month, want %v", layer, whole.Gained, want)
		}

		half := month()
		halfSeconds := half.Interval.From + int64(cultivation.DaysPerRuleMonth)*86400/2
		monthEnd := half.Interval.To
		half.Interval.ID, half.Interval.To = "time:beast-half", halfSeconds
		half.Interval.Segments[0].To = halfSeconds
		first, err := Calculate(half)
		if err != nil {
			t.Fatalf("layer %d first half: %v", layer, err)
		}
		second := half
		second.State, second.BaseRevision = first.NextState, first.NextState.Revision
		second.Interval = Interval{
			ID: "time:beast-half-2", SourceRef: "story-time:beast", From: halfSeconds, To: monthEnd,
			Segments: []Segment{{
				From: halfSeconds, To: monthEnd, Activity: ActivityFocusedSeclusion,
				ActivitySourceRef: "plan:beast:seclusion", Conditions: half.Interval.Segments[0].Conditions,
			}},
		}
		rest, err := Calculate(second)
		if err != nil {
			t.Fatalf("layer %d second half: %v", layer, err)
		}
		if math.Abs((first.Gained+rest.Gained)-whole.Gained) > 1e-9 {
			t.Fatalf("layer %d split month banked %v, whole month banked %v", layer, first.Gained+rest.Gained, whole.Gained)
		}
		if math.Abs(rest.NextState.Accumulated-whole.NextState.Accumulated) > 1e-9 {
			t.Fatalf("layer %d split month accumulated %v, whole month accumulated %v", layer, rest.NextState.Accumulated, whole.NextState.Accumulated)
		}
	}
}
