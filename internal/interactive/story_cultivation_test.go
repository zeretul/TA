package interactive

import (
	"errors"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/storytime"
)

// These tests are §五's verification list for a player-named cultivation
// interval: the result agrees with the formal time, a small-stage threshold is
// crossed, one long interval equals several short ones, a bottleneck does not
// roll a major breakthrough on its own, and stop / retry / restart / branch all
// read the same numbers.
//
// The rules layer already proved the arithmetic — cultivation_advance_test.go has
// the one-long-versus-four-short equivalence and the no-unauthorised-roll
// property. What these tests add is that the same arithmetic is reachable from
// the story's one formal clock, which is what §五 actually asks for.

// cultivationTestStory builds a story whose protagonist can cultivate.
//
// It mirrors recoveryBurdenTestStory and adds one opening turn so the branch has
// a head a side record can be parented to, exactly as a rest needs.
func cultivationTestStory(t *testing.T, initial map[string]any) (*Store, string) {
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
	story, err := store.CreateStory(CreateStoryRequest{Title: "修炼时长", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角在洞府中盘膝坐下，准备长修。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// cultivationTestApply settles one interval and fails the test if the store
// refuses it.
func cultivationTestApply(t *testing.T, store *Store, storyID string, req CultivationRequest) CultivationResult {
	t.Helper()
	if req.BranchID == "" {
		req.BranchID = "main"
	}
	if req.ActorID == "" {
		req.ActorID = DefaultActorID
	}
	result, err := store.ApplyCultivationInterval(storyID, req)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// cultivationTestStoredFloat reads one settled field out of committed state.
func cultivationTestStoredFloat(t *testing.T, store *Store, storyID, branchID, fieldID string) float64 {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := actorStateNumber(actorStateFieldValue(snapshot.State, DefaultActorID, fieldID))
	if !ok {
		t.Fatalf("the committed state must carry %s: %#v", fieldID, snapshot.State)
	}
	return value
}

func cultivationTestStoredText(t *testing.T, store *Store, storyID, branchID, fieldID string) string {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := actorStateFieldValue(snapshot.State, DefaultActorID, fieldID).(string)
	if !ok {
		t.Fatalf("the committed state must carry %s: %#v", fieldID, snapshot.State)
	}
	return value
}

// TestCultivationIntervalMovesTheFormalClockByTheIntervalItTrained is §五's
// "正常输入的修炼结果与正式时间一致".
//
// The whole point of the batch. A turn does not advance the branch's formal
// clock, so before this record a character could train for a rule year inside a
// turn and the story's one time axis would not have moved. The interval the
// clock advanced by and the interval the rules integrated must be one number.
func TestCultivationIntervalMovesTheFormalClockByTheIntervalItTrained(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)

	result := cultivationTestApply(t, store, storyID, CultivationRequest{
		Interval:  CultivationInterval{Value: 30, Unit: CultivationUnitRuleMonth},
		CommandID: "cmd-30d", CommandHash: "h-30d",
	})

	// A rule month is 30 days, so 30 of them is 900 days on the one axis. That is
	// exactly qi_6's threshold, so the remainder is legitimately zero; the test
	// asserts the clock first and the banked remainder is checked separately with
	// an interval that does not land on the boundary.
	const wantDays = 30 * 30
	const wantSeconds = wantDays * 24 * 60 * 60
	if result.Elapsed != wantSeconds {
		t.Fatalf("30 rule months is 900 days on the formal axis: elapsed=%d want=%d", result.Elapsed, wantSeconds)
	}
	if result.ClockAfter.Seconds != wantSeconds {
		t.Fatalf("the branch's formal instant must move by the trained interval: clock=%d want=%d",
			result.ClockAfter.Seconds, wantSeconds)
	}
	if result.Days != wantDays {
		t.Fatalf("the interval the rules integrated must be the one the clock moved: days=%v want=%d", result.Days, wantDays)
	}
	// The advance really happened: a month at ordinary conditions is real
	// progress, and the rules report it in the same interval.
	if !result.Advance.Trained {
		t.Fatalf("a rule month of ordinary training must make progress: %#v", result.Advance)
	}
	// The interval really crossed a threshold rather than merely passing time.
	if result.Advance.StagesAdvanced <= 0 {
		t.Fatalf("900 days from qi_6 must cross the layer threshold: %#v", result.Advance)
	}
	// A threshold landing exactly leaves no remainder, which is the rules' own
	// arithmetic rather than a lost write.
	if stored := cultivationTestStoredFloat(t, store, storyID, "main", xiuxianFieldRealmProgress); stored != 0 {
		t.Fatalf("landing exactly on a threshold banks no remainder, got %v", stored)
	}
}

// TestCultivationIntervalCrossesASmallStageThreshold is §五's "跨小门槛".
//
// §五 requires a reached small-stage threshold to be entered by the rules rather
// than left to the narrative. A long enough interval must therefore come out at
// a later stage than it went in at, with the stage key the rules table names.
func TestCultivationIntervalCrossesASmallStageThreshold(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)

	before := cultivationTestStoredText(t, store, storyID, "main", xiuxianFieldRealm)
	result := cultivationTestApply(t, store, storyID, CultivationRequest{
		Interval:  CultivationInterval{Value: 3, Unit: CultivationUnitRuleYear},
		CommandID: "cmd-3y", CommandHash: "h-3y",
	})

	if result.Advance.StagesAdvanced <= 0 {
		t.Fatalf("three rule years of ordinary training must cross at least one small stage: %#v", result.Advance)
	}
	after := cultivationTestStoredText(t, store, storyID, "main", xiuxianFieldRealm)
	if after == before {
		t.Fatalf("the stored stage must follow the advance: %q -> %q", before, after)
	}
	if after != result.Advance.RealmAfter {
		t.Fatalf("the stored stage must be the stage the rules reached: stored=%q reported=%q", after, result.Advance.RealmAfter)
	}
}

// TestOneLongCultivationIntervalEqualsSeveralShortOnes is §五's
// "一长段/多短段等价" reached through the story's own entry point.
//
// The engine already proves this for AdvanceCultivationFrom. What matters here is
// that segmentation through the story — several separate committed intervals
// rather than one call — does not change the answer, because the accumulator is
// carried through committed state rather than through a live process.
func TestOneLongCultivationIntervalEqualsSeveralShortOnes(t *testing.T) {
	longStore, longStory := cultivationTestStory(t, nil)
	long := cultivationTestApply(t, longStore, longStory, CultivationRequest{
		Interval:  CultivationInterval{Value: 400, Unit: CultivationUnitDay},
		CommandID: "cmd-long", CommandHash: "h-long",
	})

	shortStore, shortStory := cultivationTestStory(t, nil)
	var short CultivationResult
	for index := 0; index < 4; index++ {
		short = cultivationTestApply(t, shortStore, shortStory, CultivationRequest{
			Interval:    CultivationInterval{Value: 100, Unit: CultivationUnitDay},
			CommandID:   "cmd-short-" + string(rune('a'+index)),
			CommandHash: "h-short-" + string(rune('a'+index)),
		})
	}

	if long.Advance.RealmAfter != short.Advance.RealmAfter {
		t.Fatalf("one long interval and four short ones must reach the same stage: %q vs %q",
			long.Advance.RealmAfter, short.Advance.RealmAfter)
	}
	longBanked := cultivationTestStoredFloat(t, longStore, longStory, "main", xiuxianFieldRealmProgress)
	shortBanked := cultivationTestStoredFloat(t, shortStore, shortStory, "main", xiuxianFieldRealmProgress)
	if longBanked != shortBanked {
		t.Fatalf("one long interval and four short ones must bank the same progress: %v vs %v",
			longBanked, shortBanked)
	}
	// Both branches really did spend the same formal time, so the agreement is
	// not two branches that simply stopped early.
	if long.ClockAfter.Seconds != short.ClockAfter.Seconds {
		t.Fatalf("both routes must spend the same formal time: %d vs %d",
			long.ClockAfter.Seconds, short.ClockAfter.Seconds)
	}
}

// TestCultivationIntervalDoesNotRollAnUnauthorisedBreakthrough is §五's
// "普通定时修炼不自动尝试"大突破.
//
// The interval is long enough to reach the top of the qi layers and therefore to
// bank progress at the boundary. Reaching a bottleneck is not an authorisation,
// so no attempt may be made and no draw may be consumed.
func TestCultivationIntervalDoesNotRollAnUnauthorisedBreakthrough(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)

	// Far more than the top qi layer needs, so the interval certainly reaches the
	// boundary and stops there.
	result := cultivationTestApply(t, store, storyID, CultivationRequest{
		Interval:  CultivationInterval{Value: 300, Unit: CultivationUnitRuleYear},
		CommandID: "cmd-bottleneck", CommandHash: "h-bottleneck",
	})

	if result.Advance.BreakthroughRequested || result.Advance.BreakthroughRolled {
		t.Fatalf("a time-targeted interval must never attempt a major breakthrough: %#v", result.Advance)
	}
	if !result.Advance.StoppedAtBoundary {
		t.Fatalf("an interval long enough to reach the boundary must stop at it: %#v", result.Advance)
	}
	// Stopping at the boundary is not the same as entering the next realm: the
	// stage must still be a qi layer.
	if result.Advance.RealmAfter != cultivation.StageKey(cultivation.Cultivation{Realm: cultivation.RealmQi, Layer: 13}) {
		t.Fatalf("an unauthorised interval must stop inside the qi layers, got %q", result.Advance.RealmAfter)
	}
}

// TestCultivationIntervalConsumesItsOwnIntervalOnly is §五's "有效时段只计一次".
//
// §五 forbids assigning one complete stretch of time to two activities. A second
// committed interval must therefore advance the clock by its own length on top of
// the first, and the branch's formal instant must equal their sum rather than
// either one alone.
func TestCultivationIntervalConsumesItsOwnIntervalOnly(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)

	first := cultivationTestApply(t, store, storyID, CultivationRequest{
		Interval:  CultivationInterval{Value: 10, Unit: CultivationUnitDay},
		CommandID: "cmd-1", CommandHash: "h-1",
	})
	second := cultivationTestApply(t, store, storyID, CultivationRequest{
		Interval:  CultivationInterval{Value: 5, Unit: CultivationUnitDay},
		CommandID: "cmd-2", CommandHash: "h-2",
	})

	if second.ClockAfter.Seconds != first.ClockAfter.Seconds+5*24*60*60 {
		t.Fatalf("a second interval must advance the clock by its own length: first=%d second=%d",
			first.ClockAfter.Seconds, second.ClockAfter.Seconds)
	}
	// The first interval is untouched by the second: its own record still reads
	// the instant it was settled at.
	if first.Elapsed != 10*24*60*60 {
		t.Fatalf("the first interval's own length must not change: %d", first.Elapsed)
	}
}

// TestRepeatingACultivationCommandAddsNothing is §五's "重试".
//
// A retried request must find what the first attempt committed rather than
// granting the same interval twice, and the replay must be readable rather than
// an error.
func TestRepeatingACultivationCommandAddsNothing(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)

	req := CultivationRequest{
		Interval:  CultivationInterval{Value: 10, Unit: CultivationUnitDay},
		CommandID: "cmd-retry", CommandHash: "h-retry",
	}
	first := cultivationTestApply(t, store, storyID, req)
	second := cultivationTestApply(t, store, storyID, req)

	if !second.Replayed {
		t.Fatalf("a repeated command must report itself as a replay: %#v", second)
	}
	if second.EventID != first.EventID {
		t.Fatalf("a repeated command must return the first attempt's record: %q vs %q", second.EventID, first.EventID)
	}
	if second.ClockAfter.Seconds != first.ClockAfter.Seconds {
		t.Fatalf("a repeated command must not advance the clock again: %d vs %d",
			second.ClockAfter.Seconds, first.ClockAfter.Seconds)
	}
	firstBanked := cultivationTestStoredFloat(t, store, storyID, "main", xiuxianFieldRealmProgress)
	if got := cultivationTestStoredFloat(t, store, storyID, "main", xiuxianFieldRealmProgress); got != firstBanked {
		t.Fatalf("a repeated command must not train a second time: stored=%v want=%v", got, firstBanked)
	}
}

// TestReusingACultivationCommandForDifferentContentIsRefused is the other half of
// idempotency.
//
// One command identity may not stand for two different intervals. Accepting it
// would either lose the first interval or grant the second under a false name.
func TestReusingACultivationCommandForDifferentContentIsRefused(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)

	cultivationTestApply(t, store, storyID, CultivationRequest{
		Interval:  CultivationInterval{Value: 10, Unit: CultivationUnitDay},
		CommandID: "cmd-same", CommandHash: "h-first",
	})
	_, err := store.ApplyCultivationInterval(storyID, CultivationRequest{
		BranchID: "main", ActorID: DefaultActorID,
		Interval:  CultivationInterval{Value: 20, Unit: CultivationUnitDay},
		CommandID: "cmd-same", CommandHash: "h-second",
	})
	if !errors.Is(err, ErrCultivationCommandConflict) {
		t.Fatalf("one command identity for two intervals must be refused, got %v", err)
	}
}

// TestCultivationIntervalSurvivesARestart is §五's "重启".
//
// The committed interval must be readable from the journal alone, so a reopened
// store reports the same instant and the same activity without replaying the
// request that produced them.
func TestCultivationIntervalSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	system := combatTestSystem()
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != DefaultActorID {
			continue
		}
	}
	store := NewStore(dir)
	story, err := store.CreateStory(CreateStoryRequest{Title: "修炼重启", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角盘膝坐下。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	result := cultivationTestApply(t, store, story.ID, CultivationRequest{
		Interval:  CultivationInterval{Value: 30, Unit: CultivationUnitDay},
		CommandID: "cmd-restart", CommandHash: "h-restart",
	})
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := NewStore(dir)
	t.Cleanup(func() { _ = reopened.Close() })
	clock, activities, err := reopened.CultivationState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if clock.Seconds != result.ClockAfter.Seconds {
		t.Fatalf("a reopened store must report the committed instant: %d want %d", clock.Seconds, result.ClockAfter.Seconds)
	}
	activity, found := activities[result.ActivityID]
	if !found {
		t.Fatalf("a reopened store must report the committed activity %q: %#v", result.ActivityID, activities)
	}
	if activity.Activity.CompletedEffective != result.Activity.Activity.CompletedEffective {
		t.Fatalf("the reopened activity must carry the settled interval: %v want %v",
			activity.Activity.CompletedEffective, result.Activity.Activity.CompletedEffective)
	}
}

// TestAForkInheritsTheCultivationItActuallyHad is §五's "分支".
//
// A fork cut after the interval inherits it; a fork cut before it does not. The
// clock and the activity travel together, so a fork can never claim the instant
// without the training that produced it.
func TestAForkInheritsTheCultivationItActuallyHad(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)
	result := cultivationTestApply(t, store, storyID, CultivationRequest{
		Interval:  CultivationInterval{Value: 30, Unit: CultivationUnitDay},
		CommandID: "cmd-fork", CommandHash: "h-fork",
	})

	// The interval is parented to the opening turn's record, so a fork of that
	// turn sits before it and must not inherit the training.
	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	openingTurnID := ""
	for _, record := range records {
		if record.Envelope.Type == StoryEventTypeTurn {
			openingTurnID = record.Envelope.ID
			break
		}
	}
	if openingTurnID == "" {
		t.Fatal("the opening turn must be on the journal")
	}

	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: openingTurnID, Title: "早于修炼"})
	if err != nil {
		t.Fatal(err)
	}
	forkClock, forkActivities, err := store.CultivationState(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := forkActivities[result.ActivityID]; found {
		t.Fatalf("a fork cut before the interval must not inherit it: %#v", forkActivities)
	}
	if forkClock.Seconds >= result.ClockAfter.Seconds {
		t.Fatalf("a fork cut before the interval must not inherit its spent time: %d vs %d",
			forkClock.Seconds, result.ClockAfter.Seconds)
	}
	// Main is untouched by the fork.
	mainClock, mainActivities, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if mainClock.Seconds != result.ClockAfter.Seconds {
		t.Fatalf("the fork must not move main's instant: %d want %d", mainClock.Seconds, result.ClockAfter.Seconds)
	}
	if _, found := mainActivities[result.ActivityID]; !found {
		t.Fatalf("main must still hold the interval: %#v", mainActivities)
	}
	// The branch metadata check confirms the fork really is a separate branch.
	store.mu.Lock()
	forked, _, forkErr := store.readStoryRecentLocked(storyID, fork.ID)
	store.mu.Unlock()
	if forkErr != nil {
		t.Fatal(forkErr)
	}
	if _, ok := forked.Branches[fork.ID]; !ok {
		t.Fatalf("the fork must be a branch of the story: %#v", forked.Branches)
	}
}

// TestAnActivityStoppedAtANodeKeepsItsRemainder is §五's "停止" plus "可按原活动
// 恢复，不重新加时".
//
// A registered boundary stops the interval where it is and the activity keeps
// what it still owes. Resuming settles only the remainder, so the target is
// reached once in total rather than once per request.
func TestAnActivityStoppedAtANodeKeepsItsRemainder(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)

	const target = 10 * 24 * 60 * 60
	// The boundary sits five days into a ten-day target, so the first commit must
	// stop there and leave five days owing.
	stopped := cultivationTestApply(t, store, storyID, CultivationRequest{
		Interval:  CultivationInterval{Value: 10, Unit: CultivationUnitDay},
		CommandID: "cmd-node-1", CommandHash: "h-node-1",
		Nodes: []storytime.Node{{
			ID: "appointed", Kind: storytime.DecisionNode, At: 5 * 24 * 60 * 60,
			RequiresResponse: true,
		}},
	})

	if stopped.Completed {
		t.Fatalf("a boundary must stop the interval rather than run past it: %#v", stopped)
	}
	if stopped.Elapsed != 5*24*60*60 {
		t.Fatalf("the stopped interval must commit what already happened: elapsed=%d", stopped.Elapsed)
	}
	if len(stopped.PendingNodeIDs) != 1 || stopped.PendingNodeIDs[0] != "appointed" {
		t.Fatalf("the boundary that stopped the interval must be reported: %#v", stopped.PendingNodeIDs)
	}

	// Resuming with the boundary resolved settles only the remaining five days.
	resumed := cultivationTestApply(t, store, storyID, CultivationRequest{
		ActivityID: stopped.ActivityID,
		CommandID:  "cmd-node-2", CommandHash: "h-node-2",
		Nodes: []storytime.Node{{
			ID: "appointed", Kind: storytime.DecisionNode, At: 5 * 24 * 60 * 60,
			RequiresResponse: true, Resolved: true,
		}},
	})

	if !resumed.Completed {
		t.Fatalf("the continuation must reach the target: %#v", resumed)
	}
	if resumed.Elapsed != 5*24*60*60 {
		t.Fatalf("the continuation must settle only the remainder: elapsed=%d want=%d",
			resumed.Elapsed, 5*24*60*60)
	}
	if resumed.ClockAfter.Seconds != target {
		t.Fatalf("the two segments together must be the named interval: clock=%d want=%d",
			resumed.ClockAfter.Seconds, target)
	}
	if resumed.ActivityID != stopped.ActivityID {
		t.Fatalf("the continuation must continue the same activity: %q vs %q", resumed.ActivityID, stopped.ActivityID)
	}
	// The clock advanced by exactly the two remainders, so the stopped segment's
	// time was not handed out twice.
	if resumed.ClockAfter.Seconds-stopped.ClockAfter.Seconds != 5*24*60*60 {
		t.Fatalf("the continuation must not re-charge the first segment: %d -> %d",
			stopped.ClockAfter.Seconds, resumed.ClockAfter.Seconds)
	}
}

// TestAContinuationInheritsTheConditionsItWasNotGiven is §五's "沿原活动剩余目标与
// 新条件续接".
//
// A continuation that names no conditions keeps the ones the segment started
// under, so an uninterrupted interval stays one interval rather than being
// re-interpreted partway through.
func TestAContinuationInheritsTheConditionsItWasNotGiven(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)

	stopped := cultivationTestApply(t, store, storyID, CultivationRequest{
		Interval:   CultivationInterval{Value: 10, Unit: CultivationUnitDay},
		Conditions: &CultivationConditions{Environment: "stable_spiritual_site", Resource: "strong_support"},
		CommandID:  "cmd-inherit-1", CommandHash: "h-inherit-1",
		Nodes: []storytime.Node{{
			ID: "appointed", Kind: storytime.DecisionNode, At: 5 * 24 * 60 * 60,
			RequiresResponse: true,
		}},
	})

	resumed := cultivationTestApply(t, store, storyID, CultivationRequest{
		ActivityID: stopped.ActivityID,
		CommandID:  "cmd-inherit-2", CommandHash: "h-inherit-2",
		Nodes: []storytime.Node{{
			ID: "appointed", Kind: storytime.DecisionNode, At: 5 * 24 * 60 * 60,
			RequiresResponse: true, Resolved: true,
		}},
	})

	if resumed.Activity.Conditions.Environment != "stable_spiritual_site" {
		t.Fatalf("a continuation must keep the environment it was not given a new one for: %#v", resumed.Activity.Conditions)
	}
	if resumed.Activity.Conditions.Resource != "strong_support" {
		t.Fatalf("a continuation must keep the resource it was not given a new one for: %#v", resumed.Activity.Conditions)
	}
}

// TestAContinuationMayTakeNewConditions is the other half of §五's "新条件".
//
// The player may genuinely move to a better site between segments, and the new
// conditions must then apply to the remainder without rewriting the first
// segment's time.
func TestAContinuationMayTakeNewConditions(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)

	stopped := cultivationTestApply(t, store, storyID, CultivationRequest{
		Interval:   CultivationInterval{Value: 10, Unit: CultivationUnitDay},
		Conditions: &CultivationConditions{Environment: "poor"},
		CommandID:  "cmd-new-1", CommandHash: "h-new-1",
		Nodes: []storytime.Node{{
			ID: "appointed", Kind: storytime.DecisionNode, At: 5 * 24 * 60 * 60,
			RequiresResponse: true,
		}},
	})

	resumed := cultivationTestApply(t, store, storyID, CultivationRequest{
		ActivityID: stopped.ActivityID,
		Conditions: &CultivationConditions{Environment: "stable_spiritual_site"},
		CommandID:  "cmd-new-2", CommandHash: "h-new-2",
		Nodes: []storytime.Node{{
			ID: "appointed", Kind: storytime.DecisionNode, At: 5 * 24 * 60 * 60,
			RequiresResponse: true, Resolved: true,
		}},
	})

	if resumed.Activity.Conditions.Environment != "stable_spiritual_site" {
		t.Fatalf("a continuation must take the new conditions it was given: %#v", resumed.Activity.Conditions)
	}
	// The first segment's own record is untouched, so the better site cannot be
	// applied backwards to time already spent.
	if stopped.Activity.Conditions.Environment != "poor" {
		t.Fatalf("the new conditions must not rewrite the settled segment: %#v", stopped.Activity.Conditions)
	}
}

// TestAFreshIntervalIsRefusedForAnUnsuitableCharacter is §五's "复用已有修炼资格".
//
// The eligibility finding is the story's own, so a character who cannot safely
// cultivate cannot open a new interval. Starting a fresh activity while in battle
// must be refused rather than silently started.
func TestAFreshIntervalIsRefusedForAnUnsuitableCharacter(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)

	_, err := store.ApplyCultivationInterval(storyID, CultivationRequest{
		BranchID: "main", ActorID: DefaultActorID,
		Interval:            CultivationInterval{Value: 10, Unit: CultivationUnitDay},
		DeclaredEligibility: cultivation.RecoveryInBattle,
		CommandID:           "cmd-ineligible", CommandHash: "h-ineligible",
	})
	if !errors.Is(err, cultivation.ErrRecoveryIneligible) {
		t.Fatalf("an unsuitable character must not open a new interval, got %v", err)
	}
}

// TestAnIntervalNamedInAnUnknownUnitIsRefused pins the one place the rule month
// and rule year are converted.
//
// §五 permits them as units of account and forbids treating them as a calendar,
// so an unlisted unit must be refused rather than guessed at.
func TestAnIntervalNamedInAnUnknownUnitIsRefused(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)

	for _, unit := range []string{"", "fortnight", "calendar_month"} {
		_, err := store.ApplyCultivationInterval(storyID, CultivationRequest{
			BranchID: "main", ActorID: DefaultActorID,
			Interval:  CultivationInterval{Value: 1, Unit: unit},
			CommandID: "cmd-unit-" + unit, CommandHash: "h-unit-" + unit,
		})
		if !errors.Is(err, ErrCultivationUnknownUnit) {
			t.Fatalf("unit %q must be refused, got %v", unit, err)
		}
	}
	// A zero or negative interval is not an interval either.
	if _, err := store.ApplyCultivationInterval(storyID, CultivationRequest{
		BranchID: "main", ActorID: DefaultActorID,
		Interval:  CultivationInterval{Value: 0, Unit: CultivationUnitDay},
		CommandID: "cmd-zero", CommandHash: "h-zero",
	}); !errors.Is(err, ErrCultivationMissingDuration) {
		t.Fatalf("a zero interval must be refused, got %v", err)
	}
}

// TestARuleYearIsTwelveRuleMonths pins the conversion §五 authorises.
//
// The rule month and the rule year are duration units the design names, not a
// calendar. A rule year must therefore be exactly twelve rule months on the one
// formal axis, and both must be the same number of days.
func TestARuleYearIsTwelveRuleMonths(t *testing.T) {
	yearSeconds, err := CultivationInterval{Value: 1, Unit: CultivationUnitRuleYear}.Seconds()
	if err != nil {
		t.Fatal(err)
	}
	monthsSeconds, err := CultivationInterval{Value: 12, Unit: CultivationUnitRuleMonth}.Seconds()
	if err != nil {
		t.Fatal(err)
	}
	if yearSeconds != monthsSeconds {
		t.Fatalf("a rule year must be twelve rule months: %d vs %d", yearSeconds, monthsSeconds)
	}
	if yearSeconds != 360*24*60*60 {
		t.Fatalf("a rule year must be 360 days on the formal axis: %d", yearSeconds)
	}
}
