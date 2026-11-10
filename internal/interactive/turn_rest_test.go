package interactive

import (
	"strings"
	"testing"

	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

// B3 removed the player-facing rest and meditation panel. The formal service and
// the recovery records stay: the panel is gone, but the main story still has to
// settle a rest the player asks for in ordinary prose. These tests check the
// replacement path — a turn that carries a rest module — rather than the panel,
// so the removal cannot be mistaken for a repair that only hides an entry.

// turnRestTestTurn commits one main-story turn whose only module is the rest.
func turnRestTestTurn(t *testing.T, store *Store, storyID string, input TurnRestInput) StoryClock {
	t.Helper()
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:   "main",
		User:       "在客栈休养",
		Narrative:  "主角在客栈房中合衣躺下，把这一日交给睡眠。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), Rest: &input},
	}); err != nil {
		t.Fatal(err)
	}
	clock, _, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return clock
}

// TestTurnRestSettlesThroughTheOrdinaryTurnPath is the acceptance flow the
// removal has to preserve: "养伤一天" arrives as normal story input, the rules
// decide how much a day restores, and the turn both restores the pool and spends
// the day. The restored number and the advanced clock have to reach the save, not
// just the prose.
func TestTurnRestSettlesThroughTheOrdinaryTurnPath(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))

	clock := turnRestTestTurn(t, store, storyID, TurnRestInput{
		Mode:            string(cultivation.RecoveryHealthRest),
		DurationSeconds: 24 * 60 * 60,
	})

	// Qi-refining layer 6 fixes health at 200 and rest restores a twentieth of the
	// maximum per day, so the day must land on 47 in the committed state.
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 47 {
		t.Fatalf("a day of rest in the main story must restore the pool to 47, got %d", got)
	}
	if clock.Seconds != 24*60*60 {
		t.Fatalf("the turn's rest must be the interval the branch clock spends, got %d", clock.Seconds)
	}
	if got := recoveryTestRecordCount(t, store, storyID); got != 1 {
		t.Fatalf("the turn must leave exactly one recovery record, got %d", got)
	}
}

// TestTurnRestMeditationRestoresManaNotHealth checks the two modes stay distinct.
// A turn that meditates must not also heal, because a single turn carries one
// mode and the rules price each separately.
func TestTurnRestMeditationRestoresManaNotHealth(t *testing.T) {
	store, storyID := recoveryTestStory(t,
		recoveryTestPool(xiuxianFieldHealth, 37),
		recoveryTestPool(xiuxianFieldMana, 0),
	)

	clock := turnRestTestTurn(t, store, storyID, TurnRestInput{
		Mode:            string(cultivation.RecoverySafeMeditation),
		DurationSeconds: 60 * 60,
	})

	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldMana); got != 16 {
		t.Fatalf("an hour of meditation must restore a tenth of the mana maximum, got %d", got)
	}
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 37 {
		t.Fatalf("meditation must not heal health, got %d", got)
	}
	if clock.Seconds != 60*60 {
		t.Fatalf("the meditation interval must be the time spent, got %d", clock.Seconds)
	}
}

// TestTurnRestWithoutADurationDoesNothing stops a turn from resting for a
// duration nobody asked for. The model must not invent an interval to make the
// prose consistent.
func TestTurnRestWithoutADurationDoesNothing(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))

	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:   "main",
		User:       "坐着发呆",
		Narrative:  "主角坐了一会儿。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), Rest: &TurnRestInput{Mode: string(cultivation.RecoveryHealthRest)}},
	}); err != nil {
		t.Fatal(err)
	}

	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 37 {
		t.Fatalf("a rest with no duration must not restore anything, got %d", got)
	}
	if got := recoveryTestRecordCount(t, store, storyID); got != 0 {
		t.Fatalf("a rest with no duration must leave no recovery record, got %d", got)
	}
}

// TestTurnRestAndCultivationAreMutuallyExclusive stops one turn from advancing
// the branch clock twice from the same starting instant. Both modules write a
// clock record from ClockBefore; the second would overwrite the first and the
// save would claim only one interval while the prose spent two.
func TestTurnRestAndCultivationAreMutuallyExclusive(t *testing.T) {
	system, state := turnSubmissionTestState()
	choices := testTurnChoices()

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState:   system,
		CurrentState: state,
		ChoiceCount:  5,
	}, nil, TurnSubmissionInput{
		Choices:     &choices,
		Cultivation: &TurnCultivationInput{Days: 1},
		Rest: &TurnRestInput{
			Mode:            string(cultivation.RecoveryHealthRest),
			DurationSeconds: 24 * 60 * 60,
		},
	})

	if receipt.Ready {
		t.Fatal("a turn that both trains and rests must not be accepted as ready")
	}
	if prepared.TurnResult().Rest != nil {
		t.Fatalf("the conflicting rest must be dropped, got %#v", prepared.TurnResult().Rest)
	}
	if prepared.TurnResult().Cultivation == nil {
		t.Fatal("the training interval was accepted first and must be kept")
	}
	if receipt.ModuleStatus.Rest != TurnSubmissionModuleRejected {
		t.Fatalf("the conflict must be reported against the rest module: %#v", receipt.ModuleStatus)
	}
}

// TestTurnRestRefusesTheModelsOwnPoolWrite is the boundary the removal must not
// weaken: the rules decide what an interval restores.
//
// A real run produced a day's rest in which the model also wrote 生命 150. The
// rules priced the day at +5, so the save landed on 155 — the model's number plus
// the rules'. The design forbids the model authoring resources, and it forbids it
// here for the same reason it forbids it for 灵石: the number would otherwise be
// whatever the prose claimed.
func TestTurnRestRefusesTheModelsOwnPoolWrite(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))

	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "在客栈养伤一天",
		Narrative: "主角在客栈房中躺了一整天，醒来时气力稍复。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			StateUpdates: []interactivestate.Update{
				recoveryTestPool(xiuxianFieldHealth, 150),
			},
			Rest: &TurnRestInput{
				Mode:            string(cultivation.RecoveryHealthRest),
				DurationSeconds: 24 * 60 * 60,
			},
		},
	}); err == nil {
		t.Fatal("a turn that rests must refuse the model's own write to the pool it restores")
	} else if !strings.Contains(err.Error(), xiuxianFieldHealth) {
		t.Fatalf("the refusal must name the field it refused, got %v", err)
	}
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 37 {
		t.Fatalf("a refused turn must leave the pool untouched, got %d", got)
	}
}

// TestTurnRestStillLetsTheProseRecordAWound checks the other half of the same
// boundary. 生命 and 灵力 are deliberately writable by the narrative so a wound or
// an expenditure can be recorded; only the pool this turn's rest is restoring is
// the rules' answer. A rest that heals must not cost the story the ability to say
// someone was hurt somewhere else.
func TestTurnRestStillLetsTheProseRecordAWound(t *testing.T) {
	store, storyID := recoveryTestStory(t,
		recoveryTestPool(xiuxianFieldHealth, 37),
		recoveryTestPool(xiuxianFieldMana, 100),
	)

	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "调息一个时辰，强压伤势",
		Narrative: "主角盘膝调息，压住翻涌的伤势，灵力回了些，人却更虚了。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			StateUpdates: []interactivestate.Update{
				recoveryTestPool(xiuxianFieldHealth, 30),
			},
			Rest: &TurnRestInput{
				Mode:            string(cultivation.RecoverySafeMeditation),
				DurationSeconds: 60 * 60,
			},
		},
	}); err != nil {
		t.Fatalf("a wound to the pool the rest does not restore must still be recordable: %v", err)
	}

	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 30 {
		t.Fatalf("the wound the prose recorded must reach the save, got %d", got)
	}
	// The rules own the other pool: meditation restores a tenth of the maximum on
	// top of the prose, not the number the prose might have preferred.
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldMana); got != 116 {
		t.Fatalf("meditation must restore a tenth of the mana maximum, got %d", got)
	}
}

// TestTurnRestRefusesTheModelsOwnPoolWriteWithoutTheRestModule covers the failure
// the guard above does not reach.
//
// A real run answered "我谁也不见，什么也不做，就在客栈房里躺满一整天，只是养伤"
// by omitting the rest module entirely and writing 生命 delta +60 in
// state_changes. The guard for a submitted rest never ran, the day spent no clock,
// and the save claimed a recovery the rules never priced. Prose may record a
// wound, because the pool falling is a fact; it may not record a recovery, because
// the rate belongs to the rules and a turn that heals without an interval has not
// spent the time the prose says it spent.
func TestTurnRestRefusesTheModelsOwnPoolWriteWithoutTheRestModule(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))

	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我就在客栈房里躺满一整天，只是养伤。",
		Narrative: "主角在客栈房中躺了一整天，醒来时伤口已经收口。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			StateUpdates: []interactivestate.Update{
				{Op: interactivestate.Delta, Path: "/protagonist/" + xiuxianFieldHealth, Value: float64(60)},
			},
		},
	}); err == nil {
		t.Fatal("a turn must refuse prose that restores a pool without the rules' interval")
	} else if !strings.Contains(err.Error(), xiuxianFieldHealth) {
		t.Fatalf("the refusal must name the field it refused, got %v", err)
	}
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 37 {
		t.Fatalf("a refused turn must leave the pool untouched, got %d", got)
	}
}

// TestTurnStillLetsProseLowerAPool is the half that must keep working: a wound
// recorded in prose is a real fact, and the settlement's job for the pools is to
// clamp them rather than to author them.
func TestTurnStillLetsProseLowerAPool(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 120))

	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我硬挨了那一刀。",
		Narrative: "刀锋从左肋擦过，主角踉跄后退，血很快浸透了半幅衣襟。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			StateUpdates: []interactivestate.Update{
				{Op: interactivestate.Delta, Path: "/protagonist/" + xiuxianFieldHealth, Value: float64(-40)},
			},
		},
	}); err != nil {
		t.Fatalf("a wound the prose records must still reach the save: %v", err)
	}

	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 80 {
		t.Fatalf("the wound must lower the pool to 80, got %d", got)
	}
}

// TestRerollTurnRestDoesNotSpendTheIntervalTwice checks the regeneration path.
//
// The world already spent the interval when the turn committed. Re-running the
// rules for a new narration must recompute what that interval restored, not
// restore a second interval on top and not move the clock again.
func TestRerollTurnRestDoesNotSpendTheIntervalTwice(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))
	resolution, err := resolveTurnRulesWithSeed(storyID, "main", initialStoryState(), sampleTurnCheckRequest(), 7)
	if err != nil {
		t.Fatal(err)
	}
	input := TurnRestInput{
		Mode:            string(cultivation.RecoveryHealthRest),
		DurationSeconds: 24 * 60 * 60,
	}
	turn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:       "main",
		User:           "在客栈休养一天",
		Narrative:      "主角在客栈房中睡了一整天。",
		RuleResolution: &resolution,
		TurnResult:     &TurnResult{Choices: testTurnChoices(), Rest: &input},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 47 {
		t.Fatalf("the turn must have restored the pool before the reroll, got %d", got)
	}

	if _, err := store.RerollRuleResolution(storyID, resolution.ID, RuleResolutionRerollRequest{TurnID: turn.ID}); err != nil {
		t.Fatalf("RerollRuleResolution failed: %v", err)
	}

	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 47 {
		t.Fatalf("a reroll must reproduce the same restored pool, got %d", got)
	}
	clock, _, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if clock.Seconds != 24*60*60 {
		t.Fatalf("a reroll must not spend the interval again, got clock %d", clock.Seconds)
	}
	// A reroll publishes a replacement record beside the superseded one, exactly
	// as a rerolled training interval does: the second event is what shows the
	// record was genuinely re-derived rather than left in place.
	if got := recoveryTestRecordCount(t, store, storyID); got != 2 {
		t.Fatalf("the reroll must publish a replacement record beside the superseded one, got %d", got)
	}
	// One activity, though: the replacement reuses the superseded activity's ID,
	// so the branch does not carry a second convalescence.
	store.mu.Lock()
	projection, err := store.storyBranchProjectionLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Recoveries) != 1 {
		t.Fatalf("the reroll must keep one interval, got %#v", projection.Recoveries)
	}
}
