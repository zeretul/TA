package app

import (
	"errors"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// These tests cover the cultivation entry's own contract: what the read reports
// before anything is committed, what the write accepts, and which of the two
// answers a retry gets.
//
// The store layer beneath already has its own suite for the rules, the journal
// record and the branch isolation. What it cannot reach is what this layer
// decides — that an empty branch means the current one, that opening the entry
// costs no world time, that an interval named in the player's own unit arrives at
// the rules as that unit, and that a refusal reaches the player as a reason rather
// than as a bare error.

// TestCultivationEntryReadsWithoutSpendingTime is §五's read half: the entry is
// read before the player names an interval, so reading it must not move the clock,
// advance the character or leave an activity behind. An entry that trained on open
// would let a player cultivate by opening and closing it.
func TestCultivationEntryReadsWithoutSpendingTime(t *testing.T) {
	application, storyID := recoveryTestApp(t, 200)

	view, err := application.CultivationStateFor(storyID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.ClockSeconds != 0 {
		t.Fatalf("a story that has never cultivated must read as being at its origin, got %d", view.ClockSeconds)
	}
	if view.Realm != "qi_6" {
		t.Fatalf("the entry must show the live stage, got %q", view.Realm)
	}
	if view.RealmProgress != 0 {
		t.Fatalf("a character who has not banked progress must read as zero, got %v", view.RealmProgress)
	}
	if view.Health != 200 || view.HealthMax != 200 {
		t.Fatalf("the entry must show the live pools, got %d/%d", view.Health, view.HealthMax)
	}
	if view.Eligibility != string(cultivation.RecoveryQualified) {
		t.Fatalf("an unhurried character must be qualified to cultivate, got %q", view.Eligibility)
	}
	if view.Refusal != "" {
		t.Fatalf("a qualified character must carry no refusal, got %q", view.Refusal)
	}
	if len(view.Activities) != 0 {
		t.Fatalf("a story that has never cultivated must have no activity, got %#v", view.Activities)
	}
	if view.ActorID != interactive.DefaultActorID {
		t.Fatalf("an empty actor must resolve to the protagonist, got %q", view.ActorID)
	}

	// Reading twice must give the same answer: the read is pure.
	again, err := application.CultivationStateFor(storyID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if again.ClockSeconds != 0 || again.Realm != "qi_6" || len(again.Activities) != 0 {
		t.Fatalf("reading the entry must not change anything: %+v", again)
	}
}

// TestApplyCultivationIntervalAgreesWithTheFormalClock is §五's central
// acceptance: "正常输入的修炼结果与正式时间一致".
//
// Thirty rule months is 900 days, which is exactly the interval the rules need to
// carry a character from qi_6 across one small-stage threshold. The result and the
// branch's own clock have to say the same thing, because they are the same
// interval: a story where a character trained for thirty months and the world
// calendar did not move is the gap this closes.
func TestApplyCultivationIntervalAgreesWithTheFormalClock(t *testing.T) {
	application, storyID := recoveryTestApp(t, 200)

	const wantDays = 30 * 30 // thirty rule months of thirty days
	result, err := application.ApplyCultivationInterval(CultivationAppRequest{
		StoryID: storyID, BranchID: "main",
		Value: 30, Unit: interactive.CultivationUnitRuleMonth,
		CommandID: "cmd-app-cultivate",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ElapsedSeconds != wantDays*24*60*60 {
		t.Fatalf("the settled interval must be the one asked for, got %d", result.ElapsedSeconds)
	}
	if result.ClockSeconds != result.ElapsedSeconds {
		t.Fatalf("the training result and the formal time must be the same interval: clock=%d elapsed=%d",
			result.ClockSeconds, result.ElapsedSeconds)
	}
	if result.Days != float64(wantDays) {
		t.Fatalf("the interval in the rules' own unit must be the one integrated, got %v", result.Days)
	}
	if result.RealmBefore != "qi_6" || result.RealmAfter != "qi_7" {
		t.Fatalf("thirty rule months must cross one small stage: %q -> %q", result.RealmBefore, result.RealmAfter)
	}
	if result.StagesAdvanced != 1 {
		t.Fatalf("the entry must report the small stage it crossed, got %d", result.StagesAdvanced)
	}
	// Landing exactly on the threshold banks no remainder, so the write is
	// legitimately dropped as a no-op and the position reads as zero.
	if result.RealmProgress != 0 {
		t.Fatalf("an interval landing on the threshold banks no remainder, got %v", result.RealmProgress)
	}
	// §五: a time-targeted interval never attempts a major breakthrough on its own.
	if result.BreakthroughRequested || result.BreakthroughRolled {
		t.Fatalf("ordinary training must not attempt a breakthrough: %+v", result)
	}
	if !result.Completed || result.ActivityStatus != "completed" {
		t.Fatalf("an interval with no boundaries must run to its target: %+v", result)
	}
	if result.Replayed {
		t.Fatal("a first attempt is not a replay")
	}

	// And the read agrees with the write, which is what makes the entry usable
	// after an interval rather than only before one.
	view, err := application.CultivationStateFor(storyID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.Realm != "qi_7" || view.ClockSeconds != wantDays*24*60*60 {
		t.Fatalf("the entry must reflect the committed interval: %+v", view)
	}
	if len(view.Activities) != 1 {
		t.Fatalf("the entry must offer the interval that was taken: %#v", view.Activities)
	}
	// An interval that reached its target is finished, not resumable: offering it
	// back would let the same training time be spent twice.
	if view.Activities[0].Status != "completed" || view.Activities[0].Remaining != 0 || view.Activities[0].Resumable {
		t.Fatalf("an interval that reached its target must read as finished: %#v", view.Activities[0])
	}
}

// TestApplyCultivationIntervalRetryIsAReplay covers the entry's half of §五's
// "重试": the caller sees the same committed interval rather than training a
// second time.
func TestApplyCultivationIntervalRetryIsAReplay(t *testing.T) {
	application, storyID := recoveryTestApp(t, 200)

	request := CultivationAppRequest{
		StoryID: storyID, Value: 30, Unit: interactive.CultivationUnitRuleMonth,
		CommandID: "cmd-cultivate-retry",
	}
	first, err := application.ApplyCultivationInterval(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.ApplyCultivationInterval(request)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed {
		t.Fatal("a retry must be reported as a replay")
	}
	if second.EventID != first.EventID || second.ClockSeconds != first.ClockSeconds || second.RealmAfter != first.RealmAfter {
		t.Fatalf("a retry must return the committed interval: %+v vs %+v", second, first)
	}

	view, err := application.CultivationStateFor(storyID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.Realm != "qi_7" || view.ClockSeconds != 30*30*24*60*60 {
		t.Fatalf("a retry must not train a second interval: %+v", view)
	}
}

// TestApplyCultivationIntervalRefusesOneIdentityForTwoIntervals is the reason the
// fingerprint is derived here rather than sent by the caller: a client that reused
// one command id for a genuinely different interval would otherwise be handed the
// first interval as if it were the second.
func TestApplyCultivationIntervalRefusesOneIdentityForTwoIntervals(t *testing.T) {
	application, storyID := recoveryTestApp(t, 200)

	if _, err := application.ApplyCultivationInterval(CultivationAppRequest{
		StoryID: storyID, Value: 30, Unit: interactive.CultivationUnitRuleMonth,
		CommandID: "cmd-one-cultivation-identity",
	}); err != nil {
		t.Fatal(err)
	}
	// The same identity, a different interval. Answering with the committed thirty
	// months would tell the player their hour of training crossed a stage.
	if _, err := application.ApplyCultivationInterval(CultivationAppRequest{
		StoryID: storyID, Value: 1, Unit: interactive.CultivationUnitHour,
		CommandID: "cmd-one-cultivation-identity",
	}); !errors.Is(err, interactive.ErrCultivationCommandConflict) {
		t.Fatalf("one command id must not carry two different intervals, got %v", err)
	}
	// And the first interval still stands rather than being replaced.
	view, err := application.CultivationStateFor(storyID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.Realm != "qi_7" {
		t.Fatalf("the committed interval must survive the refused reuse, got %q", view.Realm)
	}
}

// TestApplyCultivationIntervalIdentityIgnoresTheBranchSpelling pins the other half
// of the fingerprint: an empty branch and the explicit current branch name the
// same place, so a retry that spells it differently is still the same command
// rather than a conflict the player cannot act on.
func TestApplyCultivationIntervalIdentityIgnoresTheBranchSpelling(t *testing.T) {
	application, storyID := recoveryTestApp(t, 200)

	if _, err := application.ApplyCultivationInterval(CultivationAppRequest{
		StoryID: storyID, Value: 30, Unit: interactive.CultivationUnitRuleMonth,
		CommandID: "cmd-branch-spelling-cultivation",
	}); err != nil {
		t.Fatal(err)
	}
	replay, err := application.ApplyCultivationInterval(CultivationAppRequest{
		StoryID: storyID, BranchID: "main", Value: 30, Unit: interactive.CultivationUnitRuleMonth,
		CommandID: "cmd-branch-spelling-cultivation",
	})
	if err != nil {
		t.Fatalf("naming the current branch must not turn a retry into a conflict: %v", err)
	}
	if !replay.Replayed {
		t.Fatal("the second call is a retry of the same interval and must read as a replay")
	}
}

// TestApplyCultivationIntervalRefusesAnUnknownUnit is §五's unit boundary: the
// interval is named in the player's own terms, and a term the rules do not define
// is refused rather than guessed at. §五 permits the rule month and the rule year
// as units of account and forbids treating them as a calendar, so an invented unit
// like "calendar_month" must not silently become one.
func TestApplyCultivationIntervalRefusesAnUnknownUnit(t *testing.T) {
	application, storyID := recoveryTestApp(t, 200)

	for _, unit := range []string{"fortnight", "calendar_month", "season"} {
		if _, err := application.ApplyCultivationInterval(CultivationAppRequest{
			StoryID: storyID, Value: 1, Unit: unit, CommandID: "cmd-bad-unit-" + unit,
		}); !errors.Is(err, interactive.ErrCultivationUnknownUnit) {
			t.Fatalf("the unit %q must be refused, got %v", unit, err)
		}
	}
	if _, err := application.ApplyCultivationInterval(CultivationAppRequest{
		StoryID: storyID, Value: 0, Unit: interactive.CultivationUnitDay, CommandID: "cmd-zero-interval",
	}); !errors.Is(err, interactive.ErrCultivationMissingDuration) {
		t.Fatalf("a zero interval must be refused, got %v", err)
	}
	// Nothing was committed by any of the refusals.
	view, err := application.CultivationStateFor(storyID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.ClockSeconds != 0 || view.Realm != "qi_6" || len(view.Activities) != 0 {
		t.Fatalf("a refused interval must leave the branch untouched: %+v", view)
	}
}

// TestCultivationEntryExplainsARefusal is §三.6's "中英文错误清楚" applied to the
// cultivation entry: an unavailable interval reaches the player as a reason about
// the world, not as a bare error and not as the character's misfortune.
func TestCultivationEntryExplainsARefusal(t *testing.T) {
	application, storyID := recoveryTestApp(t, 200)
	combatTestExchangeApp(t, application, storyID)

	view, err := application.CultivationStateFor(storyID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.Eligibility != string(cultivation.RecoveryInBattle) {
		t.Fatalf("a live battle must be reported as the reason, got %q", view.Eligibility)
	}
	if view.Refusal == "" {
		t.Fatal("an unavailable interval must carry a reason the player can read")
	}
	if view.Refusal != interactive.DescribeRecoveryEligibilityRefusal(cultivation.RecoveryInBattle) {
		t.Fatalf("the entry must carry the real refusal, got %q", view.Refusal)
	}

	// The write refuses the same way, so the entry's promise and the commit's
	// answer cannot disagree.
	if _, err := application.ApplyCultivationInterval(CultivationAppRequest{
		StoryID: storyID, Value: 30, Unit: interactive.CultivationUnitRuleMonth,
		CommandID: "cmd-blocked-cultivation",
	}); !errors.Is(err, cultivation.ErrRecoveryIneligible) {
		t.Fatalf("a live battle must refuse the interval, got %v", err)
	}
}

// TestCultivationEntryRefusesWithoutAWorkspace is the same boundary the rest entry
// has: a nil app has no store to read, and saying so is better than reporting an
// empty position.
func TestCultivationEntryRefusesWithoutAWorkspace(t *testing.T) {
	var application *App
	if _, err := application.CultivationStateFor("st_1", "main", ""); !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("a nil app must refuse the read, got %v", err)
	}
	if _, err := application.ApplyCultivationInterval(CultivationAppRequest{StoryID: "st_1"}); !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("a nil app must refuse the write, got %v", err)
	}
	empty := &App{}
	if _, err := empty.CultivationStateFor("st_1", "main", ""); !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("an app with no store must refuse the read, got %v", err)
	}
}
