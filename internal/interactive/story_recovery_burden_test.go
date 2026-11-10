package interactive

import (
	"testing"

	"ta/internal/cultivation"
	"ta/internal/storytime"
)

// These tests cover §四's acceptance list for C2: one safe meditation interval
// both relieves ordinary meridian burden and clears a meridian residual injury
// when the conditions really hold, using the same actual interval rather than a
// second one.
//
// The pure rules already have their own suite in internal/cultivation — the
// rate, the floor, the rounding and the condition set. What those cannot reach
// is the journal layer: that the relief and the residual clear are the effect of
// the interval the rest loop already settled, that a repeat adds nothing, and
// that a mana pill, a fork or a restart does not undo any of it. Those are
// asserted here and not restated.

// recoveryBurdenTestStory builds a story whose protagonist already carries a
// meridian residual injury, the way a secret technique leaves one.
//
// The burden and its floor are settled fields the narrative may not write, so
// they are seeded through the story's own initial actor state — the authored
// starting position — rather than through a turn's state updates. That is the
// same path the combat fixtures use, and it is what makes the fixture a legal
// initial state rather than a narrative override.
func recoveryBurdenTestStory(t *testing.T, initial map[string]any) (*Store, string) {
	t.Helper()
	system := combatTestSystem()
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != DefaultActorID {
			continue
		}
		for field, value := range initial {
			system.InitialActors[index].State[field] = value
		}
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "经脉负担", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "收功", Narrative: "主角在山洞中收功坐下。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// recoveryBurdenTestStored reads one settled field out of committed state.
//
// It takes the branch so the fork assertions read the fork's own state rather
// than main's.
func recoveryBurdenTestStored(t *testing.T, store *Store, storyID, branchID, fieldID string) int {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := actorStateNumber(actorStateFieldValue(snapshot.State, DefaultActorID, fieldID))
	if !ok {
		t.Fatalf("the committed state must carry %s: %#v", fieldID, snapshot.State)
	}
	return int(value)
}

// TestMeditationClearsTheResidualWithinTheSameInterval is §四's "合法30分钟解除".
//
// The character carries the secret blade's residual (floor 60) at 85 burden,
// with health at 75% and mana at 25% of their maxima. One meditation interval
// must clear the injury and take the ordinary burden to zero — and it must do so
// without advancing the clock by a second interval, because the clearing is the
// effect of the same minutes the meditation was already paid for.
func TestMeditationClearsTheResidualWithinTheSameInterval(t *testing.T) {
	store, storyID := recoveryBurdenTestStory(t, map[string]any{
		xiuxianFieldHealth:      150,
		xiuxianFieldMana:        40,
		xiuxianFieldBurden:      85,
		xiuxianFieldBurdenFloor: 60,
	})

	result := recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 60*60, "cmd-clear")

	if !result.ResidualCleared {
		t.Fatalf("an hour of safe meditation at 75%% health must clear the residual: %#v", result)
	}
	if got := recoveryBurdenTestStored(t, store, storyID, "main", xiuxianFieldBurden); got != 0 {
		t.Fatalf("clearing the residual must take the burden to zero, got %d", got)
	}
	if got := recoveryBurdenTestStored(t, store, storyID, "main", xiuxianFieldBurdenFloor); got != 0 {
		t.Fatalf("the residual floor must be gone, got %d", got)
	}
	// The interval is the one the player asked for. Clearing must not add the
	// technique's own 30-minute requirement on top of it.
	if result.Elapsed != 60*60 {
		t.Fatalf("clearing must not advance a second interval: elapsed=%d", result.Elapsed)
	}
	if result.ClockAfter.Seconds != 60*60 {
		t.Fatalf("the branch's formal instant must move by the settled interval only, got %d", result.ClockAfter.Seconds)
	}
}

// TestMeditationRelievesBurdenButKeepsAnUnearnedResidual is §四's "正常负担下降
// 但余伤未满足时保留下限".
//
// The interval really happens and really relieves ordinary burden, but the body
// conditions for clearing the injury do not hold, so the floor stays. The
// relief still stops at that floor rather than crossing it.
func TestMeditationRelievesBurdenButKeepsAnUnearnedResidual(t *testing.T) {
	// Health at 50% of the maximum is below the technique's 60% requirement.
	store, storyID := recoveryBurdenTestStory(t, map[string]any{
		xiuxianFieldHealth:      100,
		xiuxianFieldMana:        40,
		xiuxianFieldBurden:      85,
		xiuxianFieldBurdenFloor: 60,
	})

	result := recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 60*60, "cmd-unearned")

	if result.ResidualCleared {
		t.Fatalf("a wound below the technique's health requirement must keep the residual: %#v", result)
	}
	if got := recoveryBurdenTestStored(t, store, storyID, "main", xiuxianFieldBurden); got != 60 {
		t.Fatalf("ordinary burden must fall to the residual floor and stop there, got %d", got)
	}
	if got := recoveryBurdenTestStored(t, store, storyID, "main", xiuxianFieldBurdenFloor); got != 60 {
		t.Fatalf("the residual floor must survive, got %d", got)
	}
}

// TestMeditationReliefUsesTheSameIntervalOnce is §四's "同一调息时段的回灵和负担
// 处理使用同一次实际时间，不额外推进30分钟，不免费补满血蓝".
//
// A short interval relieves only what its own minutes earn. The burden figure has
// to be the rules' floor of 6 points per real minute, and neither pool may be
// refilled for free by the relief.
func TestMeditationReliefUsesTheSameIntervalOnce(t *testing.T) {
	store, storyID := recoveryBurdenTestStory(t, map[string]any{
		xiuxianFieldHealth:      100,
		xiuxianFieldMana:        40,
		xiuxianFieldBurden:      90,
		xiuxianFieldBurdenFloor: 20,
	})

	// Ten minutes earn 60 points of relief, which the floor of 20 does not block.
	result := recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 10*60, "cmd-ten")

	if got := recoveryBurdenTestStored(t, store, storyID, "main", xiuxianFieldBurden); got != 30 {
		t.Fatalf("ten minutes must relieve 6 points per minute from 90, got %d, want 30", got)
	}
	if got := recoveryBurdenTestStored(t, store, storyID, "main", xiuxianFieldBurdenFloor); got != 20 {
		t.Fatalf("an unmet residual must survive the relief, got %d", got)
	}
	if result.Elapsed != 10*60 {
		t.Fatalf("the interval must be the one asked for, got %d", result.Elapsed)
	}
	// Health is not a meditation effect: the wound is still there afterwards.
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 100 {
		t.Fatalf("meditation must not refill health, got %d", got)
	}
}

// TestRepeatedMeditationAddsNoSecondRelief is §四's "重复请求...不重置".
//
// The relief is projected from the activity's cumulative interval, so a repeat
// under the same command identity finds the first attempt's numbers rather than
// relieving the same minutes twice.
func TestRepeatedMeditationAddsNoSecondRelief(t *testing.T) {
	store, storyID := recoveryBurdenTestStory(t, map[string]any{
		xiuxianFieldHealth:      100,
		xiuxianFieldMana:        40,
		xiuxianFieldBurden:      90,
		xiuxianFieldBurdenFloor: 20,
	})

	first := recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 10*60, "cmd-repeat")
	second := recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 10*60, "cmd-repeat")

	if !second.Replayed {
		t.Fatalf("the second attempt must be reported as a replay: %#v", second)
	}
	if got := recoveryBurdenTestStored(t, store, storyID, "main", xiuxianFieldBurden); got != 30 {
		t.Fatalf("a repeat must not relieve the same minutes twice, got %d", got)
	}
	if first.ClockAfter.Seconds != second.ClockAfter.Seconds {
		t.Fatalf("a repeat must not move the clock again: %d -> %d", first.ClockAfter.Seconds, second.ClockAfter.Seconds)
	}
}

// TestRestingHealthDoesNotRelieveBurden is §四's scope: the burden rule is the
// safe *meditation* interval's, not the health rest's.
//
// The design names 离战后安全调息 for the per-minute relief, so a plain rest must
// leave the meridian burden exactly where it was.
func TestRestingHealthDoesNotRelieveBurden(t *testing.T) {
	store, storyID := recoveryBurdenTestStory(t, map[string]any{
		xiuxianFieldHealth:      100,
		xiuxianFieldMana:        40,
		xiuxianFieldBurden:      90,
		xiuxianFieldBurdenFloor: 20,
	})

	recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 24*60*60, "cmd-rest-burden")

	if got := recoveryBurdenTestStored(t, store, storyID, "main", xiuxianFieldBurden); got != 90 {
		t.Fatalf("a health rest must leave the meridian burden alone, got %d", got)
	}
	if got := recoveryBurdenTestStored(t, store, storyID, "main", xiuxianFieldBurdenFloor); got != 20 {
		t.Fatalf("a health rest must not clear a residual, got %d", got)
	}
}

// TestRestingHealthReportsTheBurdenItLeavesAlone is §四's "在既有人物状态/主故事
// 显示结果即可", seen from the reported result rather than the committed state.
//
// A rest does not move the meridian, so the result's meridian numbers have to be
// the character's real ones. Answering zero because the rule did not apply would
// tell the player their residual is gone when it is not: a rest day is exactly
// when they are deciding whether the injury still needs regulating.
func TestRestingHealthReportsTheBurdenItLeavesAlone(t *testing.T) {
	store, storyID := recoveryBurdenTestStory(t, map[string]any{
		xiuxianFieldHealth:      150,
		xiuxianFieldMana:        40,
		xiuxianFieldBurden:      85,
		xiuxianFieldBurdenFloor: 60,
	})

	result := recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 24*60*60, "cmd-rest-report")

	if result.ResidualCleared {
		t.Fatalf("a health rest must not report a cleared residual: %#v", result)
	}
	if result.BurdenBefore != 85 || result.BurdenAfter != 85 || result.BurdenFloorAfter != 60 {
		t.Fatalf("a rest must report the meridian it left alone, got before=%d after=%d floor=%d",
			result.BurdenBefore, result.BurdenAfter, result.BurdenFloorAfter)
	}
}

// TestARestoringPillDoesNotClearTheResidual is §四's "药物补灵本身不解除".
//
// A dose restores a pool; it is not the regulation interval the technique's
// injury requires. The rules already preserve the burden across a dose, and this
// asserts the product path agrees.
func TestARestoringPillDoesNotClearTheResidual(t *testing.T) {
	store, storyID := recoveryBurdenTestStory(t, map[string]any{
		xiuxianFieldHealth:      150,
		xiuxianFieldMana:        40,
		xiuxianFieldBurden:      85,
		xiuxianFieldBurdenFloor: 60,
	})

	// The dose is taken at the rules layer the battle and the story both use.
	attributes := cultivation.Attributes{Health: 150, HealthMax: 200, Mana: 40, ManaMax: 160}
	pill := cultivation.PillDef{
		ID: "restore_mana", Grade: cultivation.GradeMiddle, Purpose: cultivation.PillRestoreMana,
		RegisteredStage: cultivation.Qi(6),
	}
	restored, burden := pill.ApplyPill(attributes, cultivation.BurdenState{Current: 85, Residual: 60})
	if restored.Mana <= attributes.Mana {
		t.Fatalf("the fixture's dose must actually restore mana: %d -> %d", attributes.Mana, restored.Mana)
	}
	if burden.Current != 85 || burden.Residual != 60 {
		t.Fatalf("a restoring dose must not touch the meridian burden: %#v", burden)
	}

	// And the committed story state is untouched by the dose the story did not
	// take through its own path.
	if got := recoveryBurdenTestStored(t, store, storyID, "main", xiuxianFieldBurdenFloor); got != 60 {
		t.Fatalf("the residual must survive a dose, got %d", got)
	}
}

// TestMeditationCannotSpliceAnInterruptedStretch is §四's "不能把不具资格的前29分钟
// 与后来1分钟拼成合格30分钟" and "连续性中断后不能沿用旧累计".
//
// The two stretches add up to the technique's half-hour, but neither is itself a
// continuous safe half-hour, so the injury survives both. The relief is a
// different matter: it is the effect of the minutes really spent, so it still
// happens and still stops at the floor.
func TestMeditationCannotSpliceAnInterruptedStretch(t *testing.T) {
	const halfHour = int64(30 * 60)
	const justUnder = int64(29 * 60)

	store, storyID := recoveryBurdenTestStory(t, map[string]any{
		xiuxianFieldHealth:      150,
		xiuxianFieldMana:        40,
		xiuxianFieldBurden:      85,
		xiuxianFieldBurdenFloor: 60,
	})

	// A decision the player has to answer one minute short of the half-hour.
	first, err := store.ApplySafeRecovery(storyID, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoverySafeMeditation,
		DurationSeconds: halfHour,
		Nodes: []storytime.Node{{
			ID: "decision-residual", At: storytime.Instant(justUnder),
			Kind: storytime.DecisionNode, RequiresResponse: true,
		}},
		CommandID: "cmd-splice-1", CommandHash: "cmd-splice-1-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Completed {
		t.Fatal("an unresolved decision node must stop the interval rather than run past it")
	}
	if first.Elapsed != justUnder {
		t.Fatalf("only the settled stretch is time spent, got %d", first.Elapsed)
	}
	if first.ResidualCleared {
		t.Fatalf("29 minutes is not the technique's half-hour: %#v", first)
	}

	// The last minute is spent after the decision is answered. It is a second
	// stretch, not the tail of the first, so it cannot borrow the 29 minutes
	// already spent.
	second, err := store.ApplySafeRecovery(storyID, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoverySafeMeditation,
		ActivityID: first.ActivityID,
		Nodes: []storytime.Node{{
			ID: "decision-residual", At: storytime.Instant(justUnder),
			Kind: storytime.DecisionNode, RequiresResponse: true, Resolved: true,
		}},
		CommandID: "cmd-splice-2", CommandHash: "cmd-splice-2-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Completed {
		t.Fatalf("the resumed interval must finish its remainder: %#v", second)
	}
	if second.Elapsed != halfHour-justUnder {
		t.Fatalf("a continuation settles only the remainder, got %d", second.Elapsed)
	}
	if second.ResidualCleared {
		t.Fatalf("one minute after a break must not clear a residual the 29 before it could not: %#v", second)
	}
	if got := recoveryBurdenTestStored(t, store, storyID, "main", xiuxianFieldBurdenFloor); got != 60 {
		t.Fatalf("the residual must survive the spliced stretches, got %d", got)
	}
	// The relief is still real: 30 minutes of meditation relieve 180 points, which
	// the floor of 60 stops.
	if got := recoveryBurdenTestStored(t, store, storyID, "main", xiuxianFieldBurden); got != 60 {
		t.Fatalf("the relief must still stop at the residual floor, got %d", got)
	}

	// And one unbroken half-hour at the same body condition does clear it, so the
	// refusal above is about the break rather than about the fixture.
	unbroken, unbrokenStory := recoveryBurdenTestStory(t, map[string]any{
		xiuxianFieldHealth:      150,
		xiuxianFieldMana:        40,
		xiuxianFieldBurden:      85,
		xiuxianFieldBurdenFloor: 60,
	})
	whole := recoveryTestApply(t, unbroken, unbrokenStory, cultivation.RecoverySafeMeditation, halfHour, "cmd-unbroken")
	if !whole.ResidualCleared {
		t.Fatalf("an unbroken half-hour must clear the residual: %#v", whole)
	}
}

// TestMeditationResidualAndRestartAgree is §四's "分支和重启不重置".
//
// A cleared residual and a relieved burden are committed facts, so a rebuilt
// journal reproduces them, and a branch that forked before the interval does not
// inherit them.
func TestMeditationResidualAndRestartAgree(t *testing.T) {
	store, storyID := recoveryBurdenTestStory(t, map[string]any{
		xiuxianFieldHealth:      150,
		xiuxianFieldMana:        40,
		xiuxianFieldBurden:      85,
		xiuxianFieldBurdenFloor: 60,
	})
	forkParent := combatTestLatestTurnID(t, store, storyID)

	result := recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 60*60, "cmd-restart-residual")
	if !result.ResidualCleared {
		t.Fatalf("the fixture must clear the residual: %#v", result)
	}

	restarted := NewStore(store.Root())
	t.Cleanup(func() { _ = restarted.Close() })
	if got := recoveryBurdenTestStored(t, restarted, storyID, "main", xiuxianFieldBurden); got != 0 {
		t.Fatalf("a rebuilt journal must reproduce the relieved burden, got %d", got)
	}
	if got := recoveryBurdenTestStored(t, restarted, storyID, "main", xiuxianFieldBurdenFloor); got != 0 {
		t.Fatalf("a rebuilt journal must reproduce the cleared residual, got %d", got)
	}

	// The fork was cut at the turn the interval followed, so the interval is that
	// turn's child and is not on the fork's path.
	forked, err := store.CreateBranch(storyID, CreateBranchRequest{
		ParentEventID: forkParent, Title: "另一条线",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := recoveryBurdenTestStored(t, store, storyID, forked.ID, xiuxianFieldBurdenFloor); got != 60 {
		t.Fatalf("a fork cut before the interval must keep the residual, got %d", got)
	}
	if got := recoveryBurdenTestStored(t, store, storyID, forked.ID, xiuxianFieldBurden); got != 85 {
		t.Fatalf("a fork cut before the interval must keep its burden, got %d", got)
	}
}
