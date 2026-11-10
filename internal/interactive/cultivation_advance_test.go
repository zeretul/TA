package interactive

import (
	"math"
	"testing"

	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

func advanceTestIdentity() DomainCommitIdentity {
	return DomainCommitIdentity{CommandID: "cmd-1", OperationID: "op-1", Cycle: 1}
}

func TestXiuxianAdvanceMovesTheStageTheRulesTableWouldReach(t *testing.T) {
	system := xiuxianTestSystem()
	// A qi-refining layer 1 character in an ordinary place with ordinary aptitude.
	// The rules fix layer 1's threshold at 100 points and its monthly rate at 20, so
	// a rule month of 30 days is one fifth of the layer. The check is that the stored
	// stage and the banked progress moved to what AdvanceCultivationFrom says they
	// should, rather than to numbers this test computed independently.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(1)), nil)
	input := TurnCultivationInput{Days: cultivation.DaysPerRuleMonth}

	ops, result, ok := advanceXiuxianCultivation(state, system, "protagonist", input, xiuxianFormalFacts{}, advanceTestIdentity(), "ev1")
	if !ok {
		t.Fatal("a tracked character must advance")
	}
	if !result.Trained {
		t.Fatal("a rule month at layer 1 must gain progress")
	}
	if result.StagesAdvanced != 0 {
		t.Fatalf("one rule month at layer 1 is a fifth of the layer, advanced %d stages", result.StagesAdvanced)
	}

	// The oracle: the same call the writer makes, evaluated independently. The
	// character has no stored pool, so the body condition is read at full health —
	// the same default the settlement applies.
	character, err := xiuxianCharacterFromState(cultivation.ReferenceRules(),
		state, "protagonist")
	if err != nil {
		t.Fatal(err)
	}
	attributes := character.Attributes()
	progress := cultivation.AdvanceCultivationFrom(character.Cultivation, character.Accumulated, cultivation.DaysPerRuleMonth, cultivation.TrainingModifiers{
		Talent: cultivation.TalentNormal, Quality: "", Environment: cultivation.EnvironmentOrdinary,
		Resource: cultivation.SupportNone, HealthFraction: healthFraction(attributes.HealthMax, attributes.HealthMax),
	})
	want := cultivation.StageKey(progress.Cultivation)
	if result.RealmAfter != want {
		t.Fatalf("realm after = %q, want %q", result.RealmAfter, want)
	}
	settled := appliedXiuxianState(t, state, ops)
	if got, _ := actorStateFieldValue(settled, "protagonist", xiuxianFieldRealm).(string); got != want {
		t.Fatalf("stored realm = %q, want %q", got, want)
	}
	banked, _ := actorStateNumber(actorStateFieldValue(settled, "protagonist", xiuxianFieldRealmProgress))
	if math.Abs(banked-progress.Accumulated) > 1e-9 {
		t.Fatalf("stored banked progress = %v, want %v", banked, progress.Accumulated)
	}
}

// A character whose pools have never been written must still train.
//
// The pools are written by the settlement, which runs after the advance in the same
// commit, so on the turn a character is first advanced their 生命 does not exist
// yet. Reading that absence as zero health would make the body-condition scale
// zero and stop them from ever training.
func TestXiuxianAdvanceTrainsACharacterWithNoStoredPools(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(2)), nil)
	if _, ok := actorStateFieldValue(state, "protagonist", xiuxianFieldHealth).(float64); ok {
		t.Fatal("this fixture is meant to leave 生命 unwritten")
	}
	_, result, ok := advanceXiuxianCultivation(state, system, "protagonist", TurnCultivationInput{Days: 365}, xiuxianFormalFacts{}, advanceTestIdentity(), "ev1")
	if !ok {
		t.Fatal("a tracked character must advance")
	}
	if !result.Trained {
		t.Fatal("an unwritten 生命 is a whole body, not a dead one")
	}
}

// A long interval and the same interval split into pieces must agree. This is the
// design's stated consistency requirement for §六.2, and it is the reason the
// accumulator is carried rather than rounded per call.
func TestXiuxianAdvanceSplitsIntoTheSameResultAsOneLongInterval(t *testing.T) {
	system := xiuxianTestSystem()
	start := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(3)), nil)

	_, whole, ok := advanceXiuxianCultivation(start, system, "protagonist", TurnCultivationInput{Days: 400}, xiuxianFormalFacts{}, advanceTestIdentity(), "ev1")
	if !ok {
		t.Fatal("a tracked character must advance")
	}

	// The same 400 days, delivered as four separate commands. Each is a distinct
	// command identity, which is what a real player issuing four intervals would
	// produce.
	split := start
	accumulatedDays := 0.0
	for index := 0; index < 4; index++ {
		state := cloneActorStateRoot(split)
		identity := DomainCommitIdentity{CommandID: "cmd-split", OperationID: "op-split", Cycle: index + 1}
		ops, _, ok := advanceXiuxianCultivation(state, system, "protagonist", TurnCultivationInput{Days: 100}, xiuxianFormalFacts{}, identity, "ev1")
		if !ok {
			t.Fatal("a tracked character must advance")
		}
		split = appliedXiuxianState(t, state, ops)
		accumulatedDays += 100
	}
	if accumulatedDays != 400 {
		t.Fatalf("split interval totalled %v days, want 400", accumulatedDays)
	}

	if got, _ := actorStateFieldValue(split, "protagonist", xiuxianFieldRealm).(string); got != whole.RealmAfter {
		t.Fatalf("split realm = %q, one long interval = %q", got, whole.RealmAfter)
	}
	// The banked progress must agree too, not only the stage: a per-call rounding
	// of the accumulator would leave the two at the same stage but a different
	// distance into the next one.
	wholeProgress, _ := actorStateNumber(actorStateFieldValue(
		appliedXiuxianState(t, start, mustAdvanceDays(t, start, system, advanceTestIdentity(), 400)),
		"protagonist", xiuxianFieldRealmProgress))
	splitProgress, _ := actorStateNumber(actorStateFieldValue(split, "protagonist", xiuxianFieldRealmProgress))
	if math.Abs(wholeProgress-splitProgress) > 1e-9 {
		t.Fatalf("banked progress: split = %v, one long interval = %v", splitProgress, wholeProgress)
	}
}

func mustAdvanceDays(t *testing.T, state map[string]any, system StoryDirectorActorStateSystem, identity DomainCommitIdentity, days float64) []ActorStateOp {
	t.Helper()
	ops, _, ok := advanceXiuxianCultivation(state, system, "protagonist", TurnCultivationInput{Days: days}, xiuxianFormalFacts{}, identity, "ev1")
	if !ok {
		t.Fatal("a tracked character must advance")
	}
	return ops
}

// A badly injured character does not cultivate normally: the design treats a body
// condition of zero as a prohibition, not a slow rate.
func TestXiuxianAdvanceGainsNothingWhileTooInjuredToCultivate(t *testing.T) {
	system := xiuxianTestSystem()
	// Layer 6 is 200 health. 30 health is 15%, below the 30% threshold, so the
	// body condition scale is zero and time passes without progress.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldHealth: float64(30),
		xiuxianFieldMana:   float64(160),
	})
	_, result, ok := advanceXiuxianCultivation(state, system, "protagonist", TurnCultivationInput{Days: 365}, xiuxianFormalFacts{}, advanceTestIdentity(), "ev1")
	if !ok {
		t.Fatal("a tracked character must advance")
	}
	if result.Trained {
		t.Fatalf("a character at 15%% health must gain no progress, gained %v", result.StagesAdvanced)
	}
	if result.StagesAdvanced != 0 {
		t.Fatalf("stages advanced = %d, want 0", result.StagesAdvanced)
	}
}

// The design requires an explicit authorisation for a major breakthrough, and a
// missing precondition must produce no roll and no cost rather than a failed
// attempt.
func TestXiuxianBreakthroughIsNotRolledWithoutEligibility(t *testing.T) {
	system := xiuxianTestSystem()
	// Layer 3 is far below the last refining layer, so the attempt is not made.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(3)), nil)
	_, result, ok := advanceXiuxianCultivation(state, system, "protagonist", TurnCultivationInput{
		AttemptBreakthrough: string(cultivation.BreakthroughFoundation),
	}, xiuxianFormalFacts{}, advanceTestIdentity(), "ev1")
	if !ok {
		t.Fatal("a tracked character must advance")
	}
	if !result.BreakthroughRequested {
		t.Fatal("an authorised attempt must be recorded as requested")
	}
	if result.BreakthroughEligible || result.BreakthroughRolled {
		t.Fatalf("an ineligible attempt must not roll: eligible=%v rolled=%v", result.BreakthroughEligible, result.BreakthroughRolled)
	}
	if len(result.BreakthroughReasons) == 0 {
		t.Fatal("an ineligible attempt must report which precondition failed")
	}
}

// The same command must not re-roll. This is the design's 同一命令不重掷, and it is
// why ResolveBreakthrough takes the draw as a caller-supplied value.
func TestXiuxianBreakthroughDrawIsBoundToTheCommandIdentity(t *testing.T) {
	identity := advanceTestIdentity()
	first := cultivationBreakthroughRoll(identity, "protagonist")
	for attempt := 0; attempt < 8; attempt++ {
		if again := cultivationBreakthroughRoll(identity, "protagonist"); again != first {
			t.Fatalf("the same command re-rolled: %v then %v", first, again)
		}
	}
	other := DomainCommitIdentity{CommandID: "cmd-2", OperationID: "op-1", Cycle: 1}
	if cultivationBreakthroughRoll(other, "protagonist") == first {
		t.Fatal("a different command must draw independently")
	}
	if first < 0 || first >= 1 {
		t.Fatalf("draw %v is outside [0,1)", first)
	}
}

// An eligible attempt is rolled, and a success enters the target realm while
// keeping the current pools rather than refilling them. The design is explicit
// that a breakthrough does not hand back full health and mana.
func TestXiuxianBreakthroughSucceedsWithoutRefillingThePools(t *testing.T) {
	system := xiuxianTestSystem()
	// The last refining layer, at full progress, is what foundation requires.
	required, _, _ := cultivation.ProgressRequired(cultivation.Qi(cultivation.QiLayers))
	stage := cultivation.StageKey(cultivation.Qi(cultivation.QiLayers))
	// The attempt requires a healthy body — the rules refuse it below 90% of the
	// maximum — so the character starts whole, at the layer's real maximum.
	full := cultivation.CharacterState{Cultivation: cultivation.Qi(cultivation.QiLayers)}.Attributes()
	startHealth := full.HealthMax
	startMana := full.ManaMax
	state := xiuxianTestState(t, stage, map[string]any{
		xiuxianFieldRealmProgress: required,
		xiuxianFieldHealth:        float64(startHealth),
		xiuxianFieldMana:          float64(startMana),
	})

	// Search for a command identity whose deterministic draw succeeds, so the test
	// exercises the success path without reaching into the rules for a roll.
	succeeded := false
	for cycle := 1; cycle <= 200 && !succeeded; cycle++ {
		identity := DomainCommitIdentity{CommandID: "cmd-bt", OperationID: "op-bt", Cycle: cycle}
		roll := cultivationBreakthroughRoll(identity, "protagonist")
		if roll >= 0.30 {
			continue
		}
		_, result, ok := advanceXiuxianCultivation(state, system, "protagonist", TurnCultivationInput{
			AttemptBreakthrough: string(cultivation.BreakthroughFoundation),
		}, xiuxianFormalFacts{}, identity, "ev1")
		if !ok || !result.BreakthroughRolled {
			t.Fatalf("an eligible attempt must roll: %+v", result)
		}
		if !result.BreakthroughSucceeded {
			t.Fatalf("draw %v was below the chance %v but the attempt failed", roll, result.BreakthroughChance)
		}
		settled := appliedXiuxianState(t, state, mustAdvanceOps(t, state, system, identity))
		if got, _ := actorStateFieldValue(settled, "protagonist", xiuxianFieldRealm).(string); got != cultivation.StageKey(cultivation.Cultivation{Realm: cultivation.RealmFoundation, Stage: cultivation.StageEarly}) {
			t.Fatalf("realm after a successful breakthrough = %q", got)
		}
		// A breakthrough is not a free rest: the pools it leaves are the ones the
		// attempt cost, not the new realm's maxima. The new maxima are the
		// settlement's to derive, so the check is that this writer did not raise
		// the pools to them.
		health, _ := actorStateNumber(actorStateFieldValue(settled, "protagonist", xiuxianFieldHealth))
		if health > float64(startHealth) {
			t.Fatalf("a breakthrough must not refill health, got %v from %v", health, startHealth)
		}
		mana, _ := actorStateNumber(actorStateFieldValue(settled, "protagonist", xiuxianFieldMana))
		if mana >= float64(startMana) {
			t.Fatalf("a breakthrough must cost mana, got %v from %v", mana, startMana)
		}
		succeeded = true
	}
	if !succeeded {
		t.Skip("no command identity in the search window drew a success")
	}
}

func mustAdvanceOps(t *testing.T, state map[string]any, system StoryDirectorActorStateSystem, identity DomainCommitIdentity) []ActorStateOp {
	t.Helper()
	ops, _, ok := advanceXiuxianCultivation(state, system, "protagonist", TurnCultivationInput{
		AttemptBreakthrough: string(cultivation.BreakthroughFoundation),
	}, xiuxianFormalFacts{}, identity, "ev1")
	if !ok {
		t.Fatal("a tracked character must advance")
	}
	return ops
}

// The model must not be able to reword a character's cultivation. This is the
// design's central structural requirement for §六.1, and it is enforced rather
// than merely discouraged.
func TestXiuxianSettledFieldsAreRefusedToTheModel(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(3)), nil)
	for _, fieldID := range []string{
		xiuxianFieldRealm, xiuxianFieldRealmProgress, xiuxianFieldBurden, xiuxianFieldBurdenFloor,
		xiuxianFieldAttack, xiuxianFieldDefense, xiuxianFieldAgility, xiuxianFieldPerception,
		xiuxianFieldHealthMax, xiuxianFieldManaMax,
	} {
		updates := []interactivestate.Update{{
			Op:    interactivestate.Replace,
			Path:  interactivestate.FormatPath([]string{"protagonist", fieldID}),
			Value: float64(9999),
		}}
		if fieldID == xiuxianFieldRealm {
			updates[0].Value = cultivation.StageKey(cultivation.Qi(13))
		}
		if _, err := CompileTurnStateUpdates(system, state, updates, TurnStateUpdateCompileOptions{}); err == nil {
			t.Errorf("%s was accepted from the model, want a refusal", fieldID)
		}
	}
}

// A real wound is a fact the narrative does record. The two current pools are
// therefore deliberately outside the settled set, and the settlement's job there
// is to clamp rather than to author.
func TestXiuxianCurrentPoolsRemainWritableByTheModel(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)
	updates := []interactivestate.Update{{
		Op:    interactivestate.Replace,
		Path:  interactivestate.FormatPath([]string{"protagonist", xiuxianFieldHealth}),
		Value: float64(37),
	}}
	if _, err := CompileTurnStateUpdates(system, state, updates, TurnStateUpdateCompileOptions{}); err != nil {
		t.Fatalf("a recorded wound must stay writable, got %v", err)
	}
}

// A different genre using the same field wording must be unaffected: the
// settlement's ownership is a property of the cultivation system, not of a name.
func TestXiuxianOwnershipDoesNotLeakIntoAnotherGenre(t *testing.T) {
	// A template with the same field names but not the rules' stage option set is
	// correctly not the cultivation system.
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Name: "主角",
		Fields: []ActorStateField{
			{ID: "当前境界", Name: "当前境界", Type: "enum", Options: []string{"一阶", "二阶"}},
			{ID: "攻击", Name: "攻击", Type: "number"},
		},
	}}}
	if ActorStateSystemIsXiuxian(system) {
		t.Fatal("a template without the rules' stage set must not be treated as the cultivation system")
	}
	state := map[string]any{"actors": map[string]any{"protagonist": map[string]any{
		"id": "protagonist", "template_id": "protagonist",
		"state": map[string]any{"当前境界": "一阶", "攻击": float64(10)},
	}}}
	updates := []interactivestate.Update{{
		Op:    interactivestate.Replace,
		Path:  interactivestate.FormatPath([]string{"protagonist", "攻击"}),
		Value: float64(99),
	}}
	if _, err := CompileTurnStateUpdates(system, state, updates, TurnStateUpdateCompileOptions{}); err != nil {
		t.Fatalf("another genre's own field must remain writable, got %v", err)
	}
}

// The settlement's own writes and the rules' advance writes are not model writes,
// so they must survive enforcement. This is what stops the fix from freezing
// cultivation entirely: the refusal lives on the model's path into the state, and
// the rules' ops are applied by the commit rather than compiled from a submission.
func TestXiuxianRulesWritesAreNotRefused(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)
	for _, sourceKind := range []string{StateOpSourceCultivationSettlement, StateOpSourceCultivationAdvance} {
		op := ActorStateOp{
			Op: "set", ActorID: "protagonist", FieldID: xiuxianFieldAttack,
			Value: float64(41), SourceKind: sourceKind, SourceID: "cultivation",
		}
		if err := validateActorStateOp(op); err != nil {
			t.Fatalf("%s write rejected: %v", sourceKind, err)
		}
		working := cloneActorStateRoot(state)
		applyActorStateOp(working, op)
		if got, _ := actorStateNumber(actorStateFieldValue(working, "protagonist", xiuxianFieldAttack)); got != 41 {
			t.Fatalf("%s write stored %v, want 41", sourceKind, got)
		}
	}
	// And every field the settlement owns must be reported as owned under this
	// system, or the enforcement above has a hole.
	for fieldID := range xiuxianSettledFields() {
		if !IsXiuxianSettledField(system, fieldID) {
			t.Errorf("%s is settled but not reported as owned", fieldID)
		}
	}
	// And the derived-field list must be a subset of the settled list, or a field
	// could be derivable and model-writable at once.
	for fieldID := range xiuxianDerivedFields() {
		if !xiuxianSettledFields()[fieldID] {
			t.Errorf("%s is derived but not settled", fieldID)
		}
	}
}
