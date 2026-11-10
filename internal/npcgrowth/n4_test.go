package npcgrowth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"ta/internal/cultivation"
)

func TestN4RealSupportDoseExpiresAndFallsBackToOrdinaryRate(t *testing.T) {
	input := sample()
	const day = int64(24 * 60 * 60)
	input.Interval.ID = "time:n4-support-40-days"
	input.Interval.To = input.Interval.From + 40*day
	input.Interval.Segments[0].To = input.Interval.To
	input.State.SupportDoses = []SupportDose{{
		InstanceID: "item-stack:npc.test:support", DefinitionID: "pill.support.cultivation.lower",
		Quantity: 1, SourceRef: "story:item:npc.test:support",
	}}
	input.Interval.Segments[0].SupportUse = &SupportUse{InstanceID: "item-stack:npc.test:support", SourceRef: "story:use:support-dose:1"}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeInput(bytes.NewReader(encoded))
	if err != nil || !reflect.DeepEqual(decoded, input) {
		t.Fatalf("strict JSON did not preserve the sourced dose and actor snapshot: err=%v", err)
	}

	candidate, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidate.NextState.SupportDoses) != 0 || candidate.NextState.ActiveSupport != nil {
		t.Fatalf("one real dose was not consumed once and expired at its registered end: %+v", candidate.NextState)
	}
	if len(candidate.Segments) != 2 {
		t.Fatalf("the item effect did not create its real expiry boundary: %+v", candidate.Segments)
	}
	if candidate.Segments[0].To != input.Interval.From+int64(cultivation.DaysPerRuleMonth)*day || candidate.Segments[0].SupportResource != string(cultivation.SupportNormal) {
		t.Fatalf("supported training did not end at the registered month: %+v", candidate.Segments[0])
	}
	if candidate.Segments[1].From != candidate.Segments[0].To || candidate.Segments[1].SupportResource != string(cultivation.SupportNone) {
		t.Fatalf("ordinary speed did not resume at expiry: %+v", candidate.Segments[1])
	}
	ordinary := input
	ordinary.Interval.Segments = append([]Segment(nil), input.Interval.Segments...)
	ordinary.State.SupportDoses = nil
	ordinary.Interval.Segments[0].SupportUse = nil
	plain, err := Calculate(ordinary)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Gained <= plain.Gained {
		t.Fatalf("registered temporary support gave no real increase: supported=%v ordinary=%v", candidate.Gained, plain.Gained)
	}
	committed := input
	committed.State, committed.BaseRevision = candidate.NextState, candidate.NextState.Revision
	retry, err := Calculate(committed)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Status != StatusDuplicate || len(retry.NextState.SupportDoses) != 0 {
		t.Fatalf("a readback retry consumed or restored a dose: %+v", retry)
	}
}

func TestN4SafeRecoveryUsesCumulativeFormalRestAndMeditationRules(t *testing.T) {
	const day = int64(24 * 60 * 60)
	input := sample()
	base := cultivation.BaseAttributes(input.State.Cultivation)
	input.State.Resources = &ActorPools{Health: 10, HealthMaximum: base.HealthMax, Mana: 10, ManaMaximum: base.ManaMax}
	input.Interval.To = input.Interval.From + 3*day
	input.Interval.Segments[0].To = input.Interval.To
	input.Interval.Segments[0].Activity = ActivityHealthRest
	input.Interval.Segments[0].RecoveryActivityID = "activity:npc.test:rest:1"
	input.Interval.Segments[0].RecoverySourceRef = "npc-plan:npc.test:health-rest:1"
	input.Interval.Segments[0].RecoveryEligibilityAtStart = string(cultivation.RecoveryQualified)
	input.Interval.Segments[0].RecoveryEligibilityNow = string(cultivation.RecoveryQualified)
	input.Interval.Segments[0].Conditions.CurrentHealth = 10
	input.Interval.Segments[0].Conditions.MaximumHealth = base.HealthMax
	input.Interval.Segments[0].Conditions.CurrentMana = 10
	input.Interval.Segments[0].Conditions.MaximumMana = base.ManaMax

	first, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.NextState.Resources.Health <= 10 || first.NextState.Resources.Health >= base.HealthMax {
		t.Fatalf("health rest failed to restore a partial pool at its formal rate: %+v", first.NextState.Resources)
	}
	secondInput := input
	secondInput.Interval.Segments = append([]Segment(nil), input.Interval.Segments...)
	secondInput.State, secondInput.BaseRevision = first.NextState, first.NextState.Revision
	secondInput.Interval.ID = "time:n4-health-rest-continuation"
	secondInput.Interval.From = first.NextState.SettledThroughSeconds
	secondInput.Interval.To = secondInput.Interval.From + 7*day
	secondInput.Interval.Segments[0].From = secondInput.Interval.From
	secondInput.Interval.Segments[0].To = secondInput.Interval.To
	secondInput.Interval.Segments[0].Conditions.CurrentHealth = first.NextState.Resources.Health
	secondInput.Interval.Segments[0].Conditions.CurrentMana = first.NextState.Resources.Mana
	second, err := Calculate(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	oneShot := input
	oneShot.Interval.ID = "time:n4-health-rest-one-shot"
	oneShot.Interval.To = input.Interval.From + 10*day
	oneShot.Interval.Segments[0].To = oneShot.Interval.To
	oneShotResult, err := Calculate(oneShot)
	if err != nil {
		t.Fatal(err)
	}
	if second.NextState.Resources.Health != oneShotResult.NextState.Resources.Health {
		t.Fatalf("small rest intervals lost cumulative precision: split=%d one-shot=%d", second.NextState.Resources.Health, oneShotResult.NextState.Resources.Health)
	}

	meditation := secondInput
	meditation.State, meditation.BaseRevision = second.NextState, second.NextState.Revision
	meditation.Interval.ID = "time:n4-safe-meditation"
	meditation.Interval.From = second.NextState.SettledThroughSeconds
	meditation.Interval.To = meditation.Interval.From + 3600
	meditation.Interval.Segments[0].From = meditation.Interval.From
	meditation.Interval.Segments[0].To = meditation.Interval.To
	meditation.Interval.Segments[0].Activity = ActivitySafeMeditation
	meditation.Interval.Segments[0].RecoveryActivityID = "activity:npc.test:meditation:1"
	meditation.Interval.Segments[0].RecoverySourceRef = "npc-plan:npc.test:meditation:1"
	meditation.Interval.Segments[0].Conditions.CurrentHealth = second.NextState.Resources.Health
	meditation.Interval.Segments[0].Conditions.CurrentMana = second.NextState.Resources.Mana
	beforeHealth := meditation.State.Resources.Health
	meditated, err := Calculate(meditation)
	if err != nil {
		t.Fatal(err)
	}
	if meditated.NextState.Resources.Mana <= 10 || meditated.NextState.Resources.Health != beforeHealth {
		t.Fatalf("safe meditation did not recover mana alone: %+v", meditated.NextState.Resources)
	}
}

func TestN4UnsafeRecoveryAndUnsupportedOrStackedDosesAreRefused(t *testing.T) {
	const day = int64(24 * 60 * 60)
	input := sample()
	base := cultivation.BaseAttributes(input.State.Cultivation)
	input.State.Breakthrough = &BreakthroughState{CurrentHealth: 50, CurrentMana: 30}
	input.Interval.To = input.Interval.From + day
	segment := &input.Interval.Segments[0]
	segment.To, segment.Activity = input.Interval.To, ActivityHealthRest
	segment.RecoveryActivityID, segment.RecoverySourceRef = "activity:rest", "plan:rest"
	segment.RecoveryEligibilityAtStart = string(cultivation.RecoveryQualified)
	segment.RecoveryEligibilityNow = string(cultivation.RecoveryInDanger)
	segment.Conditions.CurrentHealth, segment.Conditions.MaximumHealth = 50, base.HealthMax
	segment.Conditions.CurrentMana, segment.Conditions.MaximumMana = 30, base.ManaMax
	if _, err := Calculate(input); err == nil {
		t.Fatal("unsafe recovery was accepted")
	}

	input = sample()
	input.State.SupportDoses = []SupportDose{{InstanceID: "dose", DefinitionID: "pill.support.cultivation.lower", Quantity: 1, SourceRef: "inventory:dose"}}
	input.State.ActiveSupport = &ActiveSupport{InstanceID: "active", DefinitionID: "pill.support.cultivation.lower", SourceRef: "use:active", StartedAtSeconds: input.Interval.From, EndsAtSeconds: input.Interval.From + 30*day, Strength: string(cultivation.SupportNormal)}
	input.Interval.Segments[0].SupportUse = &SupportUse{InstanceID: "dose", SourceRef: "use:stacked"}
	if _, err := Calculate(input); err == nil {
		t.Fatal("a second dose stacked over an active support effect")
	}

	input = sample()
	input.State.SupportDoses = []SupportDose{{InstanceID: "dose", DefinitionID: "pill.support.cultivation.lower", Quantity: 1, SourceRef: "inventory:dose"}}
	input.Interval.Segments[0].SupportUse = &SupportUse{InstanceID: "dose", SourceRef: "use:no-effective-time"}
	input.Interval.Segments[0].Conditions.CurrentHealth = 20
	if _, err := Calculate(input); err == nil {
		t.Fatal("a dose was consumed when the actor had no effective cultivation time")
	}
}

func TestN4UnusedFutureDoseIsNotConsumedAfterTheNPCReachesItsBoundary(t *testing.T) {
	const day = int64(24 * 60 * 60)
	input := sample()
	input.State.Cultivation = cultivation.Qi(cultivation.QiLayers)
	required, _, ok := cultivation.ProgressRequired(input.State.Cultivation)
	if !ok {
		t.Fatal("fixed realm has no growth threshold")
	}
	input.State.Accumulated = required - 1
	input.State.SupportDoses = []SupportDose{{InstanceID: "dose", DefinitionID: "pill.support.cultivation.lower", Quantity: 1, SourceRef: "inventory:dose"}}
	input.Interval.ID = "time:n4-stop-before-next-dose"
	input.Interval.To = input.Interval.From + 2*day
	first := input.Interval.Segments[0]
	first.To = input.Interval.From + day
	second := first
	second.From, second.To = first.To, input.Interval.To
	second.SupportUse = &SupportUse{InstanceID: "dose", SourceRef: "use:future-dose"}
	input.Interval.Segments = []Segment{first, second}
	result, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusAtBoundary || result.ConsumedThrough >= second.From || len(result.NextState.SupportDoses) != 1 || result.NextState.SupportDoses[0].Quantity != 1 {
		t.Fatalf("an unused later dose was charged after reaching the major realm boundary: %+v", result)
	}
	if result.RemainingIntent == nil || len(result.RemainingIntent.Segments) != 2 || result.RemainingIntent.Segments[0].SupportUse != nil || result.RemainingIntent.Segments[1].SupportUse == nil {
		t.Fatalf("continuation must clear the started segment's use and preserve the future dose use: %+v", result.RemainingIntent)
	}
}

func TestN4BoundaryContinuationDoesNotRepeatDoseUse(t *testing.T) {
	const day = int64(24 * 60 * 60)
	input := sample()
	input.State.Cultivation = cultivation.Qi(cultivation.QiLayers)
	required, _, ok := cultivation.ProgressRequired(input.State.Cultivation)
	if !ok {
		t.Fatal("fixed realm has no growth threshold")
	}
	input.State.Accumulated = required - 1
	input.State.SupportDoses = []SupportDose{{InstanceID: "dose", DefinitionID: "pill.support.cultivation.lower", Quantity: 1, SourceRef: "inventory:dose"}}
	input.Interval.ID = "time:n4-dose-use-before-boundary"
	input.Interval.To = input.Interval.From + day
	input.Interval.Segments[0].To = input.Interval.To
	input.Interval.Segments[0].SupportUse = &SupportUse{InstanceID: "dose", SourceRef: "use:before-boundary"}
	result, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusAtBoundary || result.RemainingIntent == nil || len(result.RemainingIntent.Segments) != 1 {
		t.Fatalf("expected a remaining intent after consuming growth up to the boundary: %+v", result)
	}
	if result.RemainingIntent.Segments[0].SupportUse != nil {
		t.Fatalf("the already executed dose use remained in the continuation: %+v", result.RemainingIntent.Segments[0])
	}
}

func TestN4MixedRecoveryIntervalsAreRejectedBeforeAnyActivityIsSettled(t *testing.T) {
	const day = int64(24 * 60 * 60)
	base := sample()
	base.State.Resources = &ActorPools{Health: 40, HealthMaximum: 200, Mana: 24, ManaMaximum: 160}
	first := base.Interval.Segments[0]
	first.To = base.Interval.From + day
	second := first
	second.From, second.To = first.To, first.To+day
	second.Activity = ActivityHealthRest
	second.RecoveryActivityID, second.RecoverySourceRef = "activity:rest", "plan:rest"
	second.RecoveryEligibilityAtStart, second.RecoveryEligibilityNow = string(cultivation.RecoveryQualified), string(cultivation.RecoveryQualified)
	first.Conditions.CurrentHealth, first.Conditions.MaximumHealth = base.State.Resources.Health, base.State.Resources.HealthMaximum
	first.Conditions.CurrentMana, first.Conditions.MaximumMana = base.State.Resources.Mana, base.State.Resources.ManaMaximum
	second.Conditions = first.Conditions

	cases := []struct {
		name       string
		activities [2]Activity
	}{
		{name: "training then rest", activities: [2]Activity{ActivityOrdinaryTraining, ActivityHealthRest}},
		{name: "rest then training", activities: [2]Activity{ActivityHealthRest, ActivityOrdinaryTraining}},
		{name: "rest then meditation", activities: [2]Activity{ActivityHealthRest, ActivitySafeMeditation}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := base
			input.Interval.ID = "time:n4-mixed-recovery-" + test.name
			input.Interval.To = second.To
			segments := []Segment{first, second}
			segments[0].Activity, segments[1].Activity = test.activities[0], test.activities[1]
			input.Interval.Segments = segments
			candidate, err := Calculate(input)
			requireDiagnostic(t, err, "mixed_recovery_interval")
			if candidate.Status != "" || candidate.HasStateChange || candidate.ConsumedThrough != 0 {
				t.Fatalf("mixed interval returned a partial candidate: %+v", candidate)
			}
		})
	}
}

func TestN4TrainingConditionsMustMatchTheSameActorResourceSnapshot(t *testing.T) {
	input := sample()
	input.State.Resources = &ActorPools{Health: 0, HealthMaximum: 100, Mana: 20, ManaMaximum: 100}
	input.State.SupportDoses = []SupportDose{{InstanceID: "dose", DefinitionID: "pill.support.cultivation.lower", Quantity: 1, SourceRef: "inventory:dose"}}
	input.Interval.Segments[0].SupportUse = &SupportUse{InstanceID: "dose", SourceRef: "use:zero-health"}
	candidate, err := Calculate(input)
	requireDiagnostic(t, err, "actor_pool_condition_mismatch")
	if candidate.Status != "" || candidate.HasStateChange || candidate.ConsumedThrough != 0 {
		t.Fatalf("contradictory health conditions returned a candidate: %+v", candidate)
	}
}

func TestN4FailedBreakthroughRecoversThenCultivatesAndFormsANewIntent(t *testing.T) {
	const day = int64(24 * 60 * 60)
	input := breakthroughSample(t)
	ready, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	input.BreakthroughIntent.RandomReceipt = receiptFor(input, ready.Breakthrough.ConditionFingerprint, 0.99)
	failure, err := Calculate(input)
	if err != nil {
		t.Fatal(err)
	}
	if failure.Status != StatusBreakthroughFailed || failure.NextState.Breakthrough.RecoveryUntilSeconds <= failure.ConsumedThrough {
		t.Fatalf("synthetic attempt did not create its formal recovery period: %+v", failure.Breakthrough)
	}

	state := failure.NextState
	current := state.Breakthrough
	base := cultivation.BaseAttributes(state.Cultivation)
	meditation := sample()
	meditation.State, meditation.BaseRevision = state, state.Revision
	meditation.Interval.ID = "time:n4-post-failure-meditation"
	meditation.Interval.From = state.SettledThroughSeconds
	meditation.Interval.To = meditation.Interval.From + 6*60*60
	meditation.Interval.Segments[0].From, meditation.Interval.Segments[0].To = meditation.Interval.From, meditation.Interval.To
	meditation.Interval.Segments[0].Activity = ActivitySafeMeditation
	meditation.Interval.Segments[0].RecoveryActivityID = "activity:npc.test:after-failure:meditation"
	meditation.Interval.Segments[0].RecoverySourceRef = "npc-plan:npc.test:after-failure:meditation"
	meditation.Interval.Segments[0].RecoveryEligibilityAtStart = string(cultivation.RecoveryQualified)
	meditation.Interval.Segments[0].RecoveryEligibilityNow = string(cultivation.RecoveryQualified)
	meditation.Interval.Segments[0].Conditions.CurrentHealth = current.CurrentHealth
	meditation.Interval.Segments[0].Conditions.MaximumHealth = base.HealthMax
	meditation.Interval.Segments[0].Conditions.CurrentMana = current.CurrentMana
	meditation.Interval.Segments[0].Conditions.MaximumMana = base.ManaMax
	meditated, err := Calculate(meditation)
	if err != nil {
		t.Fatal(err)
	}
	if meditated.NextState.Resources == nil || meditated.NextState.Resources.Mana <= current.CurrentMana {
		t.Fatalf("safe meditation did not update the same actor snapshot after failure: %+v", meditated.NextState)
	}

	state = meditated.NextState
	cooldownEnd := failure.NextState.Breakthrough.RecoveryUntilSeconds
	rest := sample()
	rest.State, rest.BaseRevision = state, state.Revision
	rest.Interval.ID = "time:n4-post-failure-health-rest"
	rest.Interval.From = state.SettledThroughSeconds
	rest.Interval.To = cooldownEnd + day
	rest.Interval.Segments[0].From, rest.Interval.Segments[0].To = rest.Interval.From, rest.Interval.To
	rest.Interval.Segments[0].Activity = ActivityHealthRest
	rest.Interval.Segments[0].RecoveryActivityID = "activity:npc.test:after-failure:rest"
	rest.Interval.Segments[0].RecoverySourceRef = "npc-plan:npc.test:after-failure:rest"
	rest.Interval.Segments[0].RecoveryEligibilityAtStart = string(cultivation.RecoveryQualified)
	rest.Interval.Segments[0].RecoveryEligibilityNow = string(cultivation.RecoveryQualified)
	rest.Interval.Segments[0].Conditions.CurrentHealth = state.Resources.Health
	rest.Interval.Segments[0].Conditions.MaximumHealth = state.Resources.HealthMaximum
	rest.Interval.Segments[0].Conditions.CurrentMana = state.Resources.Mana
	rest.Interval.Segments[0].Conditions.MaximumMana = state.Resources.ManaMaximum
	restedThroughCooldown, err := Calculate(rest)
	if err != nil {
		t.Fatal(err)
	}
	if restedThroughCooldown.ConsumedThrough != cooldownEnd || restedThroughCooldown.RemainingIntent == nil || restedThroughCooldown.NextState.Breakthrough.RecoveryUntilSeconds != 0 {
		t.Fatalf("health rest did not stop at the actual failed-attempt recovery end: %+v", restedThroughCooldown)
	}

	state = restedThroughCooldown.NextState
	continuedRest := rest
	continuedRest.State, continuedRest.BaseRevision = state, state.Revision
	continuedRest.Interval.ID = "time:n4-rest-after-cooldown"
	continuedRest.Interval.From = state.SettledThroughSeconds
	continuedRest.Interval.To = continuedRest.Interval.From + 20*day
	continuedRest.Interval.Segments = append([]Segment(nil), rest.Interval.Segments...)
	continuedRest.Interval.Segments[0].From, continuedRest.Interval.Segments[0].To = continuedRest.Interval.From, continuedRest.Interval.To
	continuedRest.Interval.Segments[0].Conditions.CurrentHealth = state.Resources.Health
	continuedRest.Interval.Segments[0].Conditions.CurrentMana = state.Resources.Mana
	recovered, err := Calculate(continuedRest)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.NextState.Resources.Health != recovered.NextState.Resources.HealthMaximum {
		t.Fatalf("health rest did not restore the actor's health pool: %+v", recovered.NextState.Resources)
	}

	state = recovered.NextState
	finalMeditation := meditation
	finalMeditation.State, finalMeditation.BaseRevision = state, state.Revision
	finalMeditation.Interval.ID = "time:n4-safe-meditation-before-retry"
	finalMeditation.Interval.From = state.SettledThroughSeconds
	finalMeditation.Interval.To = finalMeditation.Interval.From + 3600
	finalMeditation.Interval.Segments = append([]Segment(nil), meditation.Interval.Segments...)
	finalMeditation.Interval.Segments[0].From, finalMeditation.Interval.Segments[0].To = finalMeditation.Interval.From, finalMeditation.Interval.To
	finalMeditation.Interval.Segments[0].RecoveryActivityID = "activity:npc.test:after-failure:final-meditation"
	finalMeditation.Interval.Segments[0].RecoverySourceRef = "npc-plan:npc.test:after-failure:final-meditation"
	finalMeditation.Interval.Segments[0].Conditions.CurrentHealth = state.Resources.Health
	finalMeditation.Interval.Segments[0].Conditions.CurrentMana = state.Resources.Mana
	finalMeditationResult, err := Calculate(finalMeditation)
	if err != nil {
		t.Fatal(err)
	}
	if finalMeditationResult.NextState.Resources.Mana < (finalMeditationResult.NextState.Resources.ManaMaximum*80+99)/100 {
		t.Fatalf("safe meditation did not reach the formal breakthrough mana floor: %+v", finalMeditationResult.NextState.Resources)
	}

	state = finalMeditationResult.NextState
	var trained Candidate
	for year := 0; year < 40; year++ {
		practice := sample()
		practice.State, practice.BaseRevision = state, state.Revision
		practice.Interval.ID = fmt.Sprintf("time:n4-training-after-recovery-%d", year)
		practice.Interval.From = state.SettledThroughSeconds
		practice.Interval.To = practice.Interval.From + 365*day
		practice.Interval.Segments[0].From, practice.Interval.Segments[0].To = practice.Interval.From, practice.Interval.To
		practice.Interval.Segments[0].Conditions.CurrentHealth = state.Resources.Health
		practice.Interval.Segments[0].Conditions.MaximumHealth = state.Resources.HealthMaximum
		trained, err = Calculate(practice)
		if err != nil {
			t.Fatal(err)
		}
		state = trained.NextState
		if trained.Status == StatusAtBoundary {
			break
		}
	}
	if trained.Status != StatusAtBoundary || trained.Gained <= 0 {
		t.Fatalf("post-recovery ordinary cultivation did not reach the real stage boundary: %+v", trained)
	}

	intent := sample()
	intent.State, intent.BaseRevision = trained.NextState, trained.NextState.Revision
	intent.Interval.ID = "time:n4-next-breakthrough-intent"
	intent.Interval.From = trained.NextState.SettledThroughSeconds
	intent.Interval.To = intent.Interval.From + 7*day
	intent.Interval.Segments[0].From, intent.Interval.Segments[0].To = intent.Interval.From, intent.Interval.To
	intent.Interval.Segments[0].Activity = ActivityBreakthroughPrep
	intent.Interval.Segments[0].Conditions.CurrentHealth = intent.State.Resources.Health
	intent.Interval.Segments[0].Conditions.MaximumHealth = intent.State.Resources.HealthMaximum
	intent.Interval.Segments[0].Conditions.CurrentMana = intent.State.Resources.Mana
	intent.Interval.Segments[0].Conditions.MaximumMana = intent.State.Resources.ManaMaximum
	intent.BreakthroughIntent = &BreakthroughIntent{
		AttemptID: "attempt:npc.test:foundation:after-recovery", SourceRef: "npc-plan:npc.test:foundation:after-recovery",
		Target: cultivation.BreakthroughFoundation, TechniqueSupportsTarget: true,
		TechniqueSourceRef: "technique:basic", ApplicabilitySourceRef: "world-rule:qi-to-foundation",
	}
	newReady, err := Calculate(intent)
	if err != nil {
		t.Fatal(err)
	}
	if newReady.Status != StatusBreakthroughReady || newReady.Breakthrough.AttemptID != intent.BreakthroughIntent.AttemptID {
		t.Fatalf("recovered NPC could not form a fresh eligible attempt: %+v", newReady.Breakthrough)
	}
}
