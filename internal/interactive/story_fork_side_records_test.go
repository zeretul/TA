package interactive

import (
	"testing"

	"ta/internal/cultivation"
)

// These tests cover C-NEXT-14: a side record a Turn publishes is parented *after*
// the Turn rather than to it, so the ancestor cut at the Turn excludes it by
// envelope. F15 taught the character projection to attribute such a record to the
// Turn it names; the combat and story-time projections still matched by envelope
// alone, which means a fork of a turn silently lost the training, rest, walk or
// battle that turn really committed.
//
// The rule is the same one the character records use, with one addition: a record
// whose TurnID names an ancestor travels with it, and a record that names no turn
// keeps matching by its own position. That second half is what preserves
// TestRecoveryForkOfTheRestsTurnInheritsNoneOfIt — a rest taken from the light
// entry after a turn names no turn, and a fork cut at that turn still sits before
// it.

// forkTestTurn commits one turn and returns the turn's own event id.
func forkTestTurn(t *testing.T, store *Store, storyID string, result *TurnResult) string {
	t.Helper()
	turn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "继续赶路", Narrative: "主角在洞府中静坐，任时间过去。",
		TurnResult: result,
	})
	if err != nil {
		t.Fatal(err)
	}
	return turn.ID
}

// TestAForkOfATrainingTurnInheritsTheTrainingItCommitted is the failure the
// ancestor rule produced: the cultivation record is parented after the Turn, so
// cutting at the Turn dropped the interval while the pool write stayed in the
// Turn's own delta. The fork kept the growth and lost the time.
func TestAForkOfATrainingTurnInheritsTheTrainingItCommitted(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)
	turnID := forkTestTurn(t, store, storyID, &TurnResult{
		Choices: testTurnChoices(),
		Cultivation: &TurnCultivationInput{
			Days: 30,
		},
	})
	mainClock, mainActivities, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if mainClock.Seconds <= 0 {
		t.Fatalf("the training turn must spend formal time on main, got %d", mainClock.Seconds)
	}
	if len(mainActivities) == 0 {
		t.Fatal("the training turn must leave an activity on main")
	}

	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: turnID, Title: "同一日的另一条路"})
	if err != nil {
		t.Fatal(err)
	}
	forkClock, forkActivities, err := store.CultivationState(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The turn committed this interval, so the fork of that turn contains it.
	if forkClock.Seconds != mainClock.Seconds {
		t.Fatalf("a fork of the training turn must carry the instant that turn spent: got %d, want %d",
			forkClock.Seconds, mainClock.Seconds)
	}
	if len(forkActivities) != len(mainActivities) {
		t.Fatalf("a fork of the training turn must carry its activity: got %d, want %d",
			len(forkActivities), len(mainActivities))
	}
}

// TestAForkOfARestingTurnInheritsTheRestItCommitted is the same rule for a rest
// the player asked for in prose. The rest record carries the turn's id, so the
// fork must keep both the restored activity and the day it spent.
func TestAForkOfARestingTurnInheritsTheRestItCommitted(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))
	turnID := forkTestTurn(t, store, storyID, &TurnResult{
		Choices: testTurnChoices(),
		Rest: &TurnRestInput{
			Mode:            string(cultivation.RecoveryHealthRest),
			DurationSeconds: 24 * 60 * 60,
		},
	})
	mainClock, mainActivities, err := store.RecoveryState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if mainClock.Seconds != 24*60*60 {
		t.Fatalf("the resting turn must spend the day on main, got %d", mainClock.Seconds)
	}
	if len(mainActivities) != 1 {
		t.Fatalf("the resting turn must leave one activity on main: %#v", mainActivities)
	}

	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: turnID, Title: "养伤的另一条路"})
	if err != nil {
		t.Fatal(err)
	}
	forkClock, forkActivities, err := store.RecoveryState(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if forkClock.Seconds != mainClock.Seconds {
		t.Fatalf("a fork of the resting turn must carry the day it spent: got %d, want %d",
			forkClock.Seconds, mainClock.Seconds)
	}
	if len(forkActivities) != len(mainActivities) {
		t.Fatalf("a fork of the resting turn must carry its activity: got %d, want %d",
			len(forkActivities), len(mainActivities))
	}
}

// TestAForkOfABattleTurnInheritsTheBattleItStarted covers the combat projection.
//
// The battle is opened from the window *after* the turn's own clock record moved
// the branch head past that turn, which is the real order: a training turn
// publishes its side record first, and the fight starts from the position that
// turn left. So the battle's parent is the clock record, not the Turn, and a fork
// cut at the Turn reaches it only by the turn it names.
//
// A later turn is committed before the fork so the live-branch override cannot
// answer for the ancestor scan: the fork is of an older turn, which is exactly the
// case the override deliberately declines.
func TestAForkOfABattleTurnInheritsTheBattleItStarted(t *testing.T) {
	store, storyID := combatTestStory(t)
	turnID := forkTestTurn(t, store, storyID, &TurnResult{
		Choices:     testTurnChoices(),
		Cultivation: &TurnCultivationInput{Days: 1},
	})
	state := combatTestState(t, store, storyID)
	system := combatTestSystemFromStore(t, store, storyID)
	session, outcome, err := ResolveCombatExchange(state, system, "cbt-fork", "cmd-fork-battle", combatTestSpecs(), nil, map[string]cultivation.CombatAction{
		DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCombatRecord(storyID, AppendCombatRecordRequest{
		BranchID: "main", TurnID: turnID, WorldTime: "第1日 辰时",
		Session: session, Outcome: outcome,
		CommandID: "cmd-fork-battle", CommandHash: "hash-fork-battle",
	}); err != nil {
		t.Fatal(err)
	}
	mainProjection := combatTestProjection(t, store, storyID)
	if _, ok := mainProjection.Combats["cbt-fork"]; !ok {
		t.Fatalf("the battle must be live on main: %#v", mainProjection.Combats)
	}

	forkTestTurn(t, store, storyID, &TurnResult{Choices: testTurnChoices()})
	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: turnID, Title: "战斗的另一条路"})
	if err != nil {
		t.Fatal(err)
	}
	forked := combatTestProjectionForBranch(t, store, storyID, fork.ID)
	if _, ok := forked.Combats["cbt-fork"]; !ok {
		t.Fatalf("a fork of the turn the battle names must inherit it: %#v", forked.Combats)
	}
}

// TestAForkOfAnEarlierTurnStillDropsALaterTurnsSideRecords is the guard on the
// new rule: attribution travels only with the turn the record names. A training
// interval committed by a *later* turn must not appear on a fork cut before it,
// or the fork would spend time it never lived.
func TestAForkOfAnEarlierTurnStillDropsALaterTurnsSideRecords(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)
	firstTurnID := combatTestLatestTurnID(t, store, storyID)
	forkTestTurn(t, store, storyID, &TurnResult{
		Choices:     testTurnChoices(),
		Cultivation: &TurnCultivationInput{Days: 30},
	})

	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: firstTurnID, Title: "修炼之前"})
	if err != nil {
		t.Fatal(err)
	}
	forkClock, forkActivities, err := store.CultivationState(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if forkClock.Seconds != 0 {
		t.Fatalf("a fork cut before the training turn must not spend its time: got %d", forkClock.Seconds)
	}
	if len(forkActivities) != 0 {
		t.Fatalf("a fork cut before the training turn must not carry its activity: %#v", forkActivities)
	}
}
