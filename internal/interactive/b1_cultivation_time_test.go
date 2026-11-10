package interactive

import (
	"fmt"
	"testing"

	"ta/internal/cultivation"
)

// This file is the minimal reproduction of the four cultivation defects the
// independent review confirmed by source reading. Each test states the design
// rule it checks and fails on the current code; the repair that follows must make
// them pass without changing the rules' own rates.
//
// R1 — a main-story turn that trains does not move the branch's formal clock.
// R2 — a client-declared bonus is accepted with no real resource behind it.
// R3 — a character already at a major-realm bottleneck still spends the whole
//      named interval instead of stopping at the boundary.
// R4 — a major breakthrough that the rules price at seven days is adjudicated in
//      whatever interval the caller named.

// b1TurnTrain commits one main-story turn that trains the protagonist for the
// given number of days and returns the branch's formal clock afterwards.
func b1TurnTrain(t *testing.T, store *Store, storyID string, commandID string, input TurnCultivationInput) StoryClock {
	t.Helper()
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main",
		User:     "闭关修炼",
		Narrative: "主角在洞府中盘膝坐下，依功法运转周天。",
		TurnResult: &TurnResult{
			Choices:     testTurnChoices(),
			Cultivation: &input,
		},
	}); err != nil {
		t.Fatal(err)
	}
	clock, _, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return clock
}

// TestR1MainStoryTrainingAdvancesTheFormalClock is the review's first defect.
//
// §五 and the time design require the interval the rules integrated and the
// interval the story's formal clock advanced by to be one number. A turn that
// trains thirty days must therefore leave the branch thirty days later.
func TestR1MainStoryTrainingAdvancesTheFormalClock(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)

	before := cultivationTestStoredFloat(t, store, storyID, "main", xiuxianFieldRealmProgress)
	clock := b1TurnTrain(t, store, storyID, "cmd-r1", TurnCultivationInput{Days: 30})
	after := cultivationTestStoredFloat(t, store, storyID, "main", xiuxianFieldRealmProgress)

	if after <= before {
		t.Fatalf("the turn must make progress: %v -> %v", before, after)
	}
	const wantSeconds = int64(30 * 24 * 60 * 60)
	if clock.Seconds != wantSeconds {
		t.Fatalf("a turn that trained 30 days must leave the formal clock 30 days later: clock=%d want=%d (progress moved %v -> %v)",
			clock.Seconds, wantSeconds, before, after)
	}
}

// TestR2DeclaredSupportWithoutARealResourceIsNotCredited is the review's second
// defect.
//
// A character who holds no auxiliary resource and stands in no qualified
// spiritual site cannot train as though they did. The design allows the intent to
// arrive from the model; the bonus truth has to come from the character's own
// committed facts, so the unbacked claim must not raise the rate.
func TestR2DeclaredSupportWithoutARealResourceIsNotCredited(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)

	clock := b1TurnTrain(t, store, storyID, "cmd-r2", TurnCultivationInput{
		Days:        30,
		Resource:    string(cultivation.SupportStrong),
		Environment: string(cultivation.EnvironmentStable),
	})
	_ = clock
	gained := cultivationTestStoredFloat(t, store, storyID, "main", xiuxianFieldRealmProgress)

	// The ordinary rate at qi_6 is one rule month's 120 points. The claimed
	// strong support and stable site would make it 210, which the review observed.
	const ordinaryMonth = 120.0
	if gained > ordinaryMonth {
		t.Fatalf("an unbacked support claim must not raise the training rate: gained=%v want at most %v", gained, ordinaryMonth)
	}
	if gained <= 0 {
		t.Fatalf("the character still trained under ordinary conditions: gained=%v", gained)
	}
}

// TestR3ABottleneckStopsTheIntervalInsteadOfSpendingIt is the review's third
// defect.
//
// At qi_13 with the threshold already met, ordinary training can no longer move
// the character: only an authorised major breakthrough can. The interval must
// stop at that real boundary and report itself incomplete, so the remaining
// intent is preserved for the player's next decision.
func TestR3ABottleneckStopsTheIntervalInsteadOfSpendingIt(t *testing.T) {
	store, storyID := cultivationTestStory(t, map[string]any{
		xiuxianFieldRealm:         cultivation.StageKey(cultivation.Qi(cultivation.QiLayers)),
		xiuxianFieldRealmProgress: 16900.0,
	})

	result := cultivationTestApply(t, store, storyID, CultivationRequest{
		Interval:  CultivationInterval{Value: 1, Unit: CultivationUnitRuleYear},
		CommandID: "cmd-r3", CommandHash: "h-r3",
	})

	const fullYear = int64(DaysPerRuleYear * 24 * 60 * 60)
	if result.Elapsed >= fullYear {
		t.Fatalf("a year at a real bottleneck must not be spent whole: elapsed=%d full=%d", result.Elapsed, fullYear)
	}
	if result.Completed {
		t.Fatalf("stopping at a bottleneck is not completion: %#v", result)
	}
	if !result.Advance.StoppedAtBoundary {
		t.Fatalf("the rules must report the boundary that stopped it: %#v", result.Advance)
	}
	if result.ClockAfter.Seconds >= fullYear {
		t.Fatalf("the formal clock must not claim the whole year: clock=%d", result.ClockAfter.Seconds)
	}
}

// TestR4ABreakthroughCostsTheDaysTheRulesRequire is the review's fourth defect.
//
// 筑基 is a seven-day attempt. A player who names one hour cannot have the
// attempt resolved inside it; the rules' own duration is the floor, and the
// committed interval must report the time actually spent.
func TestR4ABreakthroughCostsTheDaysTheRulesRequire(t *testing.T) {
	store, storyID := cultivationTestStory(t, map[string]any{
		xiuxianFieldRealm:         cultivation.StageKey(cultivation.Qi(cultivation.QiLayers)),
		xiuxianFieldRealmProgress: 16900.0,
	})

	result := cultivationTestApply(t, store, storyID, CultivationRequest{
		Interval:            CultivationInterval{Value: 1, Unit: CultivationUnitHour},
		Conditions:          &CultivationConditions{AttemptBreakthrough: string(cultivation.BreakthroughFoundation)},
		CommandID:           "cmd-r4",
		CommandHash:         "h-r4",
	})

	if !result.Advance.BreakthroughEligible {
		t.Fatalf("the fixture is meant to be a legal attempt: %#v", result.Advance.BreakthroughReasons)
	}
	const requiredSeconds = int64(7 * 24 * 60 * 60)
	if result.Elapsed < requiredSeconds {
		t.Fatalf("a 筑基 attempt is seven days: elapsed=%d want at least %d", result.Elapsed, requiredSeconds)
	}
	if result.ClockAfter.Seconds < requiredSeconds {
		t.Fatalf("the formal clock must carry the attempt's real duration: clock=%d", result.ClockAfter.Seconds)
	}
}

// b1BreakthroughFixture places the protagonist on the last refining layer with
// the threshold met and a whole body, which is what §7.1 requires before an
// attempt. The pools are set to the layer's real maxima rather than to numbers
// the test invented.
func b1BreakthroughFixture(t *testing.T) map[string]any {
	t.Helper()
	required, _, ok := cultivation.ProgressRequired(cultivation.Qi(cultivation.QiLayers))
	if !ok {
		t.Fatal("the last refining layer must accumulate progress")
	}
	full := cultivation.CharacterState{Cultivation: cultivation.Qi(cultivation.QiLayers)}.Attributes()
	return map[string]any{
		xiuxianFieldRealm:         cultivation.StageKey(cultivation.Qi(cultivation.QiLayers)),
		xiuxianFieldRealmProgress: required,
		xiuxianFieldHealth:        float64(full.HealthMax),
		xiuxianFieldMana:          float64(full.ManaMax),
	}
}

// b1Identity builds the full commit identity for one attempt cycle. The command
// ID is distinct per cycle because the store matches a turn against the input
// accepted under the same command, and two attempts sharing one command would be
// read as a conflicting replay of the first.
func b1Identity(operationID string, cycle int) DomainCommitIdentity {
	return DomainCommitIdentity{
		CommandID:   fmt.Sprintf("cmd-%s-%d", operationID, cycle),
		OperationID: operationID,
		Cycle:       cycle,
	}
}

// b1FailingCycle finds an identity whose deterministic draw fails a foundation
// attempt, so the failure branch is exercised without reaching into the rules for
// a roll.
func b1FailingCycle(t *testing.T) DomainCommitIdentity {
	t.Helper()
	for cycle := 1; cycle <= 400; cycle++ {
		identity := b1Identity("op-b1", cycle)
		if cultivationBreakthroughRoll(identity, DefaultActorID) >= 0.30 {
			return identity
		}
	}
	t.Fatal("no failing draw found in the searched range")
	return DomainCommitIdentity{}
}

// b1CommitInput stages the canonical player input a turn under this identity
// consumes.
//
// A turn carrying an Agent command identity must consume the player input
// accepted under the same identity; without it the store refuses the commit.
// The text is the player's own line, which is also what the turn records.
func b1CommitInput(t *testing.T, store *Store, storyID string, identity DomainCommitIdentity, text string) DomainCommitIdentity {
	t.Helper()
	if identity.CommandID == "" {
		t.Fatal("a turn identity needs a command ID")
	}
	intent, err := NewPlayerInputIntent(identity, "main", text)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(storyID, intent); err != nil {
		t.Fatal(err)
	}
	return identity
}

// b1TurnCultivate commits one cultivation turn under the given identity and
// returns the turn and the branch's formal clock afterwards.
func b1TurnCultivate(t *testing.T, store *Store, storyID string, identity DomainCommitIdentity, input TurnCultivationInput) (TurnEvent, StoryClock) {
	t.Helper()
	const user = "尝试突破"
	identity = b1CommitInput(t, store, storyID, identity, user)
	turn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:         "main",
		User:             user,
		Narrative:        "主角凝神静气，准备冲击瓶颈。",
		TurnResult:       &TurnResult{Choices: testTurnChoices(), Cultivation: &input},
		AgentCommandID:   identity.CommandID,
		AgentOperationID: identity.OperationID,
		AgentCycle:       identity.Cycle,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock, _, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return turn, clock
}

// TestB1TurnPathBreakthroughCostsTheRulesDays is R4 reached through the main
// story rather than through the interval entry point.
//
// The review reproduced a foundation attempt that the rules price at seven days
// being adjudicated inside a one-hour turn. Both paths run the same advance, so
// both must leave the formal clock seven days later, and the turn's own record
// is what carries that interval.
func TestB1TurnPathBreakthroughCostsTheRulesDays(t *testing.T) {
	store, storyID := cultivationTestStory(t, b1BreakthroughFixture(t))
	identity := b1FailingCycle(t)

	turn, clock := b1TurnCultivate(t, store, storyID, identity, TurnCultivationInput{
		AttemptBreakthrough: string(cultivation.BreakthroughFoundation),
	})

	const requiredSeconds = int64(7 * 24 * 60 * 60)
	if clock.Seconds < requiredSeconds {
		t.Fatalf("a turn that attempts foundation must spend the rules' seven days: clock=%d want at least %d", clock.Seconds, requiredSeconds)
	}
	_, activities, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	activity, ok := activities["clt-"+turn.ID]
	if !ok {
		t.Fatalf("the turn must publish the interval it spent: %#v", activities)
	}
	if activity.ActorID != DefaultActorID {
		t.Fatalf("the interval must be attributed to the protagonist: %#v", activity)
	}
}

// TestB1RerollCountsTheIntervalOnce is the "one interval, one submission" rule
// under a reroll.
//
// A reroll is a second reading of the same events, not a rewind of the calendar.
// Re-deriving the advance must replace the recorded growth while leaving the
// branch at exactly the instant the turn already reached; charging the interval
// again would let a player bank time by rerolling, and refunding it would take
// away time the world already spent.
func TestB1RerollCountsTheIntervalOnce(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)

	resolution, err := resolveTurnRulesWithSeed(storyID, "main", initialStoryState(), sampleTurnCheckRequest(), 7)
	if err != nil {
		t.Fatal(err)
	}
	input := TurnCultivationInput{Days: 30}
	turn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:         "main",
		User:             "闭关一月",
		Narrative:        "主角闭关一月，依功法运转周天。",
		RuleResolution: &resolution,
		TurnResult:     &TurnResult{Choices: testTurnChoices(), Cultivation: &input},
	})
	if err != nil {
		t.Fatal(err)
	}
	before, _, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	const wantSeconds = int64(30 * 24 * 60 * 60)
	if before.Seconds != wantSeconds {
		t.Fatalf("the turn must have spent its thirty days before the reroll: clock=%d want=%d", before.Seconds, wantSeconds)
	}

	if _, err := store.RerollRuleResolution(storyID, resolution.ID, RuleResolutionRerollRequest{TurnID: turn.ID}); err != nil {
		t.Fatalf("RerollRuleResolution failed: %v", err)
	}
	after, activities, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.Seconds != wantSeconds {
		t.Fatalf("a reroll must not spend the interval twice: clock=%d want=%d", after.Seconds, wantSeconds)
	}
	if len(activities) != 1 {
		t.Fatalf("a reroll must replace the turn's interval, not add a second one: %#v", activities)
	}
	kept, ok := activities["clt-"+turn.ID]
	if !ok {
		t.Fatalf("the rerolled turn must keep one interval of its own: %#v", activities)
	}
	// The reroll must genuinely republish the record rather than leave the
	// original in place: the replacement is a second event, and the projection
	// walks it. A reroll that silently dropped the record would also show one
	// activity, so the event count is what tells the two apart.
	if got := int64(kept.Activity.CompletedEffective); got != 30*24*60*60 {
		t.Fatalf("the rerolled interval must still be the turn's thirty days: got=%d %#v", got, kept)
	}
	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	var cultivationRecords int
	for _, record := range records {
		if record.Envelope.Type == StoryEventTypeCultivationRecord {
			cultivationRecords++
		}
	}
	if cultivationRecords != 2 {
		t.Fatalf("the reroll must publish a replacement record beside the superseded one: count=%d", cultivationRecords)
	}
}

// TestB1AFailedBreakthroughBlocksTheNextAttempt is §7.1's precondition that no
// unresolved breakthrough recovery may still be running.
//
// §7.2 prices a failed attempt's recovery at seven days for foundation. Inside
// that window a second attempt is not legal, and the refusal has to come from
// the committed record rather than from the model remembering it.
func TestB1AFailedBreakthroughBlocksTheNextAttempt(t *testing.T) {
	store, storyID := cultivationTestStory(t, b1BreakthroughFixture(t))
	identity := b1FailingCycle(t)

	first, firstClock := b1TurnCultivate(t, store, storyID, identity, TurnCultivationInput{
		AttemptBreakthrough: string(cultivation.BreakthroughFoundation),
	})
	_, activities, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	activity, ok := activities["clt-"+first.ID]
	if !ok {
		t.Fatalf("the failed attempt must publish its interval: %#v", activities)
	}
	if activity.ConvalescenceUntil <= 0 {
		t.Fatalf("a failed breakthrough must record its recovery deadline: %#v", activity)
	}
	// §7.2 prices a foundation failure's recovery at seven days from the end of
	// the attempt.
	const recoverySeconds = int64(7 * 24 * 60 * 60)
	if want := firstClock.Seconds + recoverySeconds; activity.ConvalescenceUntil != want {
		t.Fatalf("the recovery must run seven days from the attempt: deadline=%d want=%d", activity.ConvalescenceUntil, want)
	}

	second := b1Identity(identity.OperationID, identity.Cycle+1)
	secondTurn, clock := b1TurnCultivate(t, store, storyID, second, TurnCultivationInput{
		AttemptBreakthrough: string(cultivation.BreakthroughFoundation),
	})
	if clock.Seconds >= activity.ConvalescenceUntil {
		t.Fatalf("the fixture meant to attempt inside the recovery window: clock=%d deadline=%d", clock.Seconds, activity.ConvalescenceUntil)
	}
	_, after, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	// §7.1's "缺条件不掷骰、不消耗" reaches the time axis too: an attempt the
	// preconditions refuse spends no interval at all, so the refused turn
	// publishes no record and leaves the clock exactly where it stood.
	if _, ok := after["clt-"+secondTurn.ID]; ok {
		t.Fatalf("a refused attempt must not publish an interval: %#v", after)
	}
	if len(after) != 1 {
		t.Fatalf("a refused attempt must not add an activity: %#v", after)
	}
	if clock.Seconds != firstClock.Seconds {
		t.Fatalf("a refused attempt must not spend time: clock=%d want=%d", clock.Seconds, firstClock.Seconds)
	}
}
