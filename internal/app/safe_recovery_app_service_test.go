package app

import (
	"errors"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// These tests cover the rest entry's own contract: what the read reports before
// anything is committed, what the write accepts, and which of the two answers a
// retry gets.
//
// The store layer beneath already has its own suite for the rules, the journal
// record and the branch isolation. What it cannot reach is what this layer
// decides — that an empty branch means the current one, that opening the entry
// costs no world time, that a refusal reaches the player as a reason rather than
// as a bare error, and that the wording is the natural one §三.2 asks for rather
// than a calendar stamp. Those are asserted here and not restated.

// recoveryTestApp builds a story whose protagonist carries a real recorded wound,
// so a rest has something to restore. The wound is written at creation rather
// than after the first turn, which is the same fact arriving by a shorter route.
func recoveryTestApp(t *testing.T, health int) (*App, string) {
	t.Helper()
	system := combatTestSystem(t)
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != interactive.DefaultActorID {
			continue
		}
		state := system.InitialActors[index].State
		if state == nil {
			state = map[string]any{}
		}
		state[combatTestHealthField] = health
		system.InitialActors[index].State = state
	}
	return combatTestAppWithSystem(t, system)
}

// TestSafeRecoveryEntryReadsWithoutSpendingTime is §三.3's "不打开窗口即恢复".
//
// The entry is read before the player decides anything, so reading it must not
// move the clock, restore a pool or leave an activity behind. A window that
// healed on open would let a player rest by opening and closing it.
func TestSafeRecoveryEntryReadsWithoutSpendingTime(t *testing.T) {
	application, storyID := recoveryTestApp(t, 37)

	view, err := application.SafeRecoveryStateFor(storyID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.ClockSeconds != 0 {
		t.Fatalf("a story that has never rested must read as being at its origin, got %d", view.ClockSeconds)
	}
	if view.Health != 37 || view.HealthMax != 200 {
		t.Fatalf("the entry must show the live pools, got %d/%d", view.Health, view.HealthMax)
	}
	if view.Mana != 160 || view.ManaMax != 160 {
		t.Fatalf("the entry must show the mana pool, got %d/%d", view.Mana, view.ManaMax)
	}
	if view.Eligibility != string(cultivation.RecoveryQualified) {
		t.Fatalf("a wounded but unhurried character must be qualified, got %q", view.Eligibility)
	}
	if view.Refusal != "" {
		t.Fatalf("a qualified character must carry no refusal, got %q", view.Refusal)
	}
	if len(view.Activities) != 0 {
		t.Fatalf("a story that has never rested must have no activity, got %#v", view.Activities)
	}
	if view.ActorID != interactive.DefaultActorID {
		t.Fatalf("an empty actor must resolve to the protagonist, got %q", view.ActorID)
	}

	// Reading twice must give the same answer: the read is pure.
	again, err := application.SafeRecoveryStateFor(storyID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if again.ClockSeconds != 0 || again.Health != 37 || len(again.Activities) != 0 {
		t.Fatalf("reading the entry must not change anything: %+v", again)
	}
}

// TestApplySafeRecoverySettlesAndDescribesTheInterval covers the write end of
// §三.6's "指定时长后明确普通行动直接推进": the interval the player named is the
// interval settled, and it comes back described naturally rather than as a
// fabricated date.
func TestApplySafeRecoverySettlesAndDescribesTheInterval(t *testing.T) {
	application, storyID := recoveryTestApp(t, 37)

	result, err := application.ApplySafeRecovery(SafeRecoveryAppRequest{
		StoryID: storyID, BranchID: "main", Mode: string(cultivation.RecoveryHealthRest),
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ElapsedSeconds != 24*60*60 {
		t.Fatalf("the settled interval must be the one asked for, got %d", result.ElapsedSeconds)
	}
	if result.Elapsed != "1日" {
		t.Fatalf("the wording must be natural rather than a calendar stamp, got %q", result.Elapsed)
	}
	if result.PoolBefore != 37 || result.PoolAfter != 47 || result.PoolMaximum != 200 {
		t.Fatalf("the entry must report the pool around the interval: %+v", result)
	}
	if result.ClockSeconds != 24*60*60 {
		t.Fatalf("the branch's formal instant must advance by the interval, got %d", result.ClockSeconds)
	}
	if result.ActorID != interactive.DefaultActorID {
		t.Fatalf("an empty actor must resolve to the protagonist, got %q", result.ActorID)
	}
	if result.Replayed {
		t.Fatal("a first attempt is not a replay")
	}

	// And the read agrees with the write, which is what makes the entry usable
	// after a rest rather than only before one.
	view, err := application.SafeRecoveryStateFor(storyID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.Health != 47 || view.ClockSeconds != 24*60*60 {
		t.Fatalf("the entry must reflect the committed rest: %+v", view)
	}
	if len(view.Activities) != 1 {
		t.Fatalf("the entry must offer the rest that was taken: %#v", view.Activities)
	}
	// A rest that reached its target is finished, not resumable: offering it back
	// would let the same interval be earned twice.
	if view.Activities[0].Status != "completed" || view.Activities[0].Remaining != 0 || view.Activities[0].Resumable {
		t.Fatalf("a rest that reached its target must read as finished: %#v", view.Activities[0])
	}
}

// TestApplySafeRecoveryRetryIsAReplay covers the app layer's half of §三.5's
// "同一活动同目标重试增量0": the caller sees the same committed interval rather
// than a second one.
func TestApplySafeRecoveryRetryIsAReplay(t *testing.T) {
	application, storyID := recoveryTestApp(t, 37)

	first, err := application.ApplySafeRecovery(SafeRecoveryAppRequest{
		StoryID: storyID, Mode: string(cultivation.RecoveryHealthRest),
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-retry",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.ApplySafeRecovery(SafeRecoveryAppRequest{
		StoryID: storyID, Mode: string(cultivation.RecoveryHealthRest),
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-retry",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed {
		t.Fatal("a retry must be reported as a replay")
	}
	if second.EventID != first.EventID || second.PoolAfter != first.PoolAfter {
		t.Fatalf("a retry must return the committed interval: %+v vs %+v", second, first)
	}

	view, err := application.SafeRecoveryStateFor(storyID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.Health != 47 {
		t.Fatalf("a retry must not recover a second time, got health %d", view.Health)
	}
}

// TestApplySafeRecoveryRefusesOneIdentityForTwoIntervals is the reason the
// fingerprint is derived here rather than sent by the caller: a client that
// reused one command id for a genuinely different rest would otherwise be handed
// the first interval as if it were the second.
//
// The interval is part of the identity, so the same id with a different length is
// a conflict rather than a replay.
func TestApplySafeRecoveryRefusesOneIdentityForTwoIntervals(t *testing.T) {
	application, storyID := recoveryTestApp(t, 37)

	if _, err := application.ApplySafeRecovery(SafeRecoveryAppRequest{
		StoryID: storyID, Mode: string(cultivation.RecoveryHealthRest),
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-one-identity",
	}); err != nil {
		t.Fatal(err)
	}
	// The same identity, a different interval. Answering with the committed day
	// would tell the player their hour of rest restored a day's health.
	if _, err := application.ApplySafeRecovery(SafeRecoveryAppRequest{
		StoryID: storyID, Mode: string(cultivation.RecoveryHealthRest),
		DurationSeconds: 60 * 60, CommandID: "cmd-one-identity",
	}); !errors.Is(err, interactive.ErrRecoveryCommandConflict) {
		t.Fatalf("one command id must not carry two different intervals, got %v", err)
	}
	// And the first interval still stands rather than being replaced.
	view, err := application.SafeRecoveryStateFor(storyID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.Health != 47 {
		t.Fatalf("the committed rest must survive the refused reuse, got health %d", view.Health)
	}
}

// TestApplySafeRecoveryIdentityIgnoresTheBranchSpelling pins the other half of
// the fingerprint: an empty branch and the explicit current branch name the same
// place, so a retry that spells it differently is still the same command rather
// than a conflict the player cannot act on.
func TestApplySafeRecoveryIdentityIgnoresTheBranchSpelling(t *testing.T) {
	application, storyID := recoveryTestApp(t, 37)

	if _, err := application.ApplySafeRecovery(SafeRecoveryAppRequest{
		StoryID: storyID, Mode: string(cultivation.RecoveryHealthRest),
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-branch-spelling",
	}); err != nil {
		t.Fatal(err)
	}
	replay, err := application.ApplySafeRecovery(SafeRecoveryAppRequest{
		StoryID: storyID, BranchID: "main", Mode: string(cultivation.RecoveryHealthRest),
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-branch-spelling",
	})
	if err != nil {
		t.Fatalf("naming the current branch must not turn a retry into a conflict: %v", err)
	}
	if !replay.Replayed {
		t.Fatal("the second call is a retry of the same interval and must read as a replay")
	}
}

// TestSafeRecoveryEntryExplainsARefusal is §三.6's "中英文错误清楚": an
// unavailable rest reaches the player as a reason about the world, not as a bare
// error and not as the character's misfortune.
func TestSafeRecoveryEntryExplainsARefusal(t *testing.T) {
	// A live battle on this branch.
	application, storyID := recoveryTestApp(t, 37)
	combatTestExchangeApp(t, application, storyID)

	view, err := application.SafeRecoveryStateFor(storyID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.Eligibility != string(cultivation.RecoveryInBattle) {
		t.Fatalf("a live battle must be reported as the reason, got %q", view.Eligibility)
	}
	if view.Refusal == "" {
		t.Fatal("an unavailable rest must carry a reason the player can read")
	}
	if view.Refusal != interactive.DescribeRecoveryEligibilityRefusal(cultivation.RecoveryInBattle) {
		t.Fatalf("the entry must carry the real refusal, got %q", view.Refusal)
	}

	// The write refuses the same way, so the entry's promise and the commit's
	// answer cannot disagree.
	if _, err := application.ApplySafeRecovery(SafeRecoveryAppRequest{
		StoryID: storyID, Mode: string(cultivation.RecoveryHealthRest),
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-blocked",
	}); !errors.Is(err, cultivation.ErrRecoveryIneligible) {
		t.Fatalf("a live battle must refuse the rest, got %v", err)
	}
}

// combatTestExchangeApp opens and resolves one real battle round through the
// app's own battle entry, which is how the rest entry sees a fight that is
// genuinely live rather than a flag someone passed in.
func combatTestExchangeApp(t *testing.T, application *App, storyID string) {
	t.Helper()
	if _, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-app-fight", Combatants: combatTestSpecs(),
	}); err != nil {
		t.Fatal(err)
	}
}

// TestApplySafeRecoveryRejectsUnknownInput covers the entry's boundary: a mode
// the rules do not define, a story that is not there, and a branch that is not
// there are all refused rather than settled against a guess.
func TestApplySafeRecoveryRejectsUnknownInput(t *testing.T) {
	application, storyID := recoveryTestApp(t, 37)

	if _, err := application.ApplySafeRecovery(SafeRecoveryAppRequest{
		StoryID: storyID, Mode: "sleep", DurationSeconds: 60, CommandID: "cmd-bad-mode",
	}); !errors.Is(err, cultivation.ErrRecoveryUnknownMode) {
		t.Fatalf("an undefined mode must be refused, got %v", err)
	}
	if _, err := application.ApplySafeRecovery(SafeRecoveryAppRequest{
		StoryID: "st_missing", Mode: string(cultivation.RecoveryHealthRest),
		DurationSeconds: 60, CommandID: "cmd-bad-story",
	}); err == nil {
		t.Fatal("an unknown story must be refused")
	}
	if _, err := application.SafeRecoveryStateFor(storyID, "branch-missing", ""); err == nil {
		t.Fatal("an unknown branch must be refused")
	}
}

// TestApplySafeRecoveryReportsTheMeridianEffect is §四's acceptance seen from the
// entry the player actually uses.
//
// The store layer beneath asserts the rule itself. What this adds is that the
// answer the rest window receives carries the meridian numbers rather than only
// the pool: the stage shows what the interval did to the character without
// re-deriving it, and the residual clearing is reported as the same interval's
// effect rather than as an unmentioned side effect.
func TestApplySafeRecoveryReportsTheMeridianEffect(t *testing.T) {
	// The protagonist carries the secret blade's residual: floor 60 at 85 burden,
	// with health at 75% of the maximum so the technique's 60% requirement holds.
	system := combatTestSystem(t)
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != interactive.DefaultActorID {
			continue
		}
		state := system.InitialActors[index].State
		if state == nil {
			state = map[string]any{}
		}
		state[combatTestHealthField] = 150
		state[combatTestManaField] = 40
		state[combatTestBurdenFloorField] = 60
		state["经脉负担"] = 85
		system.InitialActors[index].State = state
	}
	application, storyID := combatTestAppWithSystem(t, system)

	result, err := application.ApplySafeRecovery(SafeRecoveryAppRequest{
		StoryID: storyID, BranchID: "main", Mode: string(cultivation.RecoverySafeMeditation),
		DurationSeconds: 60 * 60, CommandID: "cmd-meridian",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.ResidualCleared {
		t.Fatalf("an hour of safe meditation at 75%% health must clear the residual: %+v", result)
	}
	if result.BurdenBefore != 85 || result.BurdenAfter != 0 || result.BurdenFloorAfter != 0 {
		t.Fatalf("the entry must report the meridian numbers around the interval: %+v", result)
	}
	// §四 forbids paying a second interval for the clearing.
	if result.ElapsedSeconds != 60*60 || result.ClockSeconds != 60*60 {
		t.Fatalf("clearing must not advance a second interval: %+v", result)
	}

	// A retry reports the same facts off the record rather than re-deriving them
	// from conditions that may have moved.
	replayed, err := application.ApplySafeRecovery(SafeRecoveryAppRequest{
		StoryID: storyID, BranchID: "main", Mode: string(cultivation.RecoverySafeMeditation),
		DurationSeconds: 60 * 60, CommandID: "cmd-meridian",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || !replayed.ResidualCleared || replayed.BurdenBefore != 85 || replayed.BurdenAfter != 0 {
		t.Fatalf("a replay must report what the first attempt committed: %+v", replayed)
	}
}

// TestSafeRecoveryEntryRefusesWithoutAWorkspace is the same boundary the battle
// window has: a nil app has no store to read, and saying so is better than
// reporting an empty rest.
func TestSafeRecoveryEntryRefusesWithoutAWorkspace(t *testing.T) {
	var application *App
	if _, err := application.SafeRecoveryStateFor("st_1", "main", ""); !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("a nil app must refuse the read, got %v", err)
	}
	if _, err := application.ApplySafeRecovery(SafeRecoveryAppRequest{StoryID: "st_1"}); !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("a nil app must refuse the write, got %v", err)
	}
	empty := &App{}
	if _, err := empty.SafeRecoveryStateFor("st_1", "main", ""); !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("an app with no store must refuse the read, got %v", err)
	}
}
