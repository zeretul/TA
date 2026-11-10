package app

import (
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// These tests cover the two recovery gaps the delivery review reproduced, both
// of which only appear after something has already gone wrong: a round settled
// whose prose was never written, and an earlier round filled in after the battle
// has moved on. The ordinary path passes without either of them, which is why
// they are asserted here rather than left to the happy-path suite.

// combatNarrationTestFirstRound settles one exchange and returns the round's
// event ID together with the battle as it stood when that round committed.
func combatNarrationTestFirstRound(t *testing.T, application *App, storyID string) (interactive.BattleRoundOutcome, cultivation.CombatSession) {
	t.Helper()
	opened, err := application.interactive.StartBattle(storyID, interactive.BattleStartRequest{
		BranchID: "main", CommandID: "narration-open", CommandHash: "narration-hash-open",
		Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	settled, err := application.interactive.ResolveBattleRound(storyID, interactive.BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		CommandID: "narration-first", CommandHash: "narration-hash-first",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
			"rival":                    {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: interactive.DefaultActorID},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// The session the round committed is the one its own record carries; a later
	// round mutates the projection but never this snapshot.
	historical := settled.Session
	return settled, historical
}

// TestCommittedCombatRoundReadsTheSessionAsOfThatRound covers the first gap: a
// round filled in after the battle has moved on.
//
// committedCombatRound used to pair the historical outcome with the branch's
// latest session. The pools a later round settled are already on that session, so
// the passage written for round 1 was told the numbers round 2 produced — the
// prose described an exchange the journal does not contain, and every later round
// silently rewrote the earlier round's account. Session.Round is the *next*
// exchange index, so the round's own session is the only correct source; it must
// never be compared against outcome.Round.
func TestCommittedCombatRoundReadsTheSessionAsOfThatRound(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	first, historical := combatNarrationTestFirstRound(t, application, storyID)

	// A second round moves the pools. This is what makes the two sessions
	// distinguishable: without it the defect is invisible.
	second, err := application.interactive.ResolveBattleRound(storyID, interactive.BattleRoundRequest{
		BranchID: "main", SessionID: first.Session.ID,
		CommandID: "narration-second", CommandHash: "narration-hash-second",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
			"rival":                    {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: interactive.DefaultActorID},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Session.Round <= historical.Round {
		t.Fatalf("the fixture must advance the battle: %d -> %d", historical.Round, second.Session.Round)
	}

	session, outcome, err := committedCombatRound(storyID, "main", first.Session.ID, first.EventID, application.interactive)
	if err != nil {
		t.Fatal(err)
	}
	if session.Round != historical.Round {
		t.Fatalf("round %d must be described from its own session (next exchange %d), but got the latest session (next exchange %d)",
			outcome.Round, historical.Round, session.Round)
	}
	// The pools are the user-visible consequence: the facts block the model reads
	// must carry the numbers that round actually used.
	before := combatNarrationFacts(outcome, historical, character.State{})
	after := combatNarrationFacts(outcome, session, character.State{})
	if before != after {
		t.Fatalf("the passage for round %d would be written from the wrong pools:\n--- as committed ---\n%s\n--- as read ---\n%s", outcome.Round, before, after)
	}
}

// TestCombatWindowOffersRecoveryForACommittedRoundWithoutProse covers the second
// gap: the retry entry has to come back after a restart.
//
// NarrationError was only ever set inline by PlayCombatRound, so it lived in the
// response of the request that failed and nowhere else. A process interrupted
// after the round was committed and before its passage left nothing for a later
// read to find, and the window showed a round with no prose and no way to ask for
// one. The flag is now derived from the durable fact — a committed round whose
// passage is absent — so a plain read recovers the entry, and reading must never
// itself call the model: this test runs with no runtime config, so a generation
// attempt here would fail rather than pass silently.
func TestCombatWindowOffersRecoveryForACommittedRoundWithoutProse(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	first, _ := combatNarrationTestFirstRound(t, application, storyID)

	window, err := application.CombatWindow(CombatWindowRequest{
		StoryID: storyID, BranchID: "main", SessionID: first.Session.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if window.LastRound == nil || window.LastRound.Narration != "" {
		t.Fatalf("the fixture must leave a settled round without prose: %#v", window.LastRound)
	}
	if window.NarrationError == "" {
		t.Fatal("a committed round with no prose must offer the retry entry; the frontend only exposes it when narration_error is set")
	}
	if window.LastRound.EventID != first.EventID {
		t.Fatalf("the recovery must point at the round that is missing its prose: %s vs %s", window.LastRound.EventID, first.EventID)
	}

	// Process recovery: the entry is a fact about the journal, so it must survive
	// the store being closed and reopened rather than having been remembered.
	root := application.workspace
	if err := application.interactive.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := interactive.NewStore(root)
	t.Cleanup(func() { _ = reopened.Close() })
	restarted := &App{workspace: root, interactive: reopened}

	afterRestart, err := restarted.CombatWindow(CombatWindowRequest{
		StoryID: storyID, BranchID: "main", SessionID: first.Session.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if afterRestart.NarrationError == "" {
		t.Fatal("the retry entry was lost on restart; it must be read back from the journal, not remembered by the process")
	}
}

// TestFillingInAnEarlierRoundsProseAdvancesNothing is the §3 C01 boundary stated
// as an assertion: filling in a passage must only fill in a passage.
//
// It must not settle the round again, spend another dose, or move the battle on.
// A player whose round 1 lost its prose, who then played round 2, must be able to
// write round 1's passage without either rewinding or re-playing anything — so
// the round index, the pools and the committed history all have to come back
// unchanged.
func TestFillingInAnEarlierRoundsProseAdvancesNothing(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	first, _ := combatNarrationTestFirstRound(t, application, storyID)

	second, err := application.interactive.ResolveBattleRound(storyID, interactive.BattleRoundRequest{
		BranchID: "main", SessionID: first.Session.ID,
		CommandID: "narration-second", CommandHash: "narration-hash-second",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
			"rival":                    {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: interactive.DefaultActorID},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	before, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID, BranchID: "main", SessionID: first.Session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if before.NarrationError == "" {
		t.Fatal("round 1 must still be missing its prose at this point")
	}

	if _, err := application.interactive.AppendCombatNarration(storyID, interactive.AppendCombatNarrationRequest{
		BranchID: "main", SessionID: first.Session.ID, RoundEventID: first.EventID,
		Narrative: "第一合的经过，补写在此。", StyleID: "source-drama",
		CommandID: "narration-late", CommandHash: "narration-late-hash",
	}); err != nil {
		t.Fatal(err)
	}

	after, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID, BranchID: "main", SessionID: first.Session.ID})
	if err != nil {
		t.Fatal(err)
	}
	// The battle must not have moved: the round the window is about to resolve is
	// the same one it was about to resolve before the passage was written.
	if after.Round != before.Round {
		t.Fatalf("filling in an earlier round moved the battle on: round %d -> %d", before.Round, after.Round)
	}
	if after.Ended != before.Ended || after.Result != before.Result {
		t.Fatalf("filling in an earlier round changed the result: %#v -> %#v", before, after)
	}
	if len(after.History) != len(before.History) || len(after.History) != 2 {
		t.Fatalf("filling in an earlier round changed the history: %d -> %d", len(before.History), len(after.History))
	}
	for index := range after.History {
		if after.History[index].EventID != before.History[index].EventID {
			t.Fatalf("filling in an earlier round rewrote history at %d: %s -> %s",
				index, before.History[index].EventID, after.History[index].EventID)
		}
	}
	// And the passage landed on the round that was missing it, not on the newest.
	if got := after.History[0].Narration; got != "第一合的经过，补写在此。" {
		t.Fatalf("the passage did not land on round 1: %q", got)
	}
	if got := after.History[1].Narration; got == "第一合的经过，补写在此。" {
		t.Fatal("the passage was written onto the wrong round")
	}
	if after.LastRound == nil || after.LastRound.Narration != "" {
		t.Fatalf("the newest round must still have no passage of its own: %#v", after.LastRound)
	}
	// The fixture has to have genuinely moved past round 1, or "nothing advanced"
	// would hold for the trivial reason that nothing had advanced to begin with.
	if second.Session.Round <= first.Session.Round {
		t.Fatalf("the fixture did not advance the battle: %d -> %d", first.Session.Round, second.Session.Round)
	}
	if after.Round != second.Session.Round {
		t.Fatalf("the battle must stand where round 2 left it (next exchange %d), not at %d", second.Session.Round, after.Round)
	}
}

// TestCombatWindowClearsTheRecoveryEntryOnceTheProseIsCommitted is the other half
// of the same fact: the entry must disappear when there is nothing left to write,
// or a player is offered a retry for a passage they already have.
func TestCombatWindowClearsTheRecoveryEntryOnceTheProseIsCommitted(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	first, _ := combatNarrationTestFirstRound(t, application, storyID)

	if _, err := application.interactive.AppendCombatNarration(storyID, interactive.AppendCombatNarrationRequest{
		BranchID: "main", SessionID: first.Session.ID, RoundEventID: first.EventID,
		Narrative: "刀锋擦过石屑，你退半步稳住气息。", StyleID: "source-drama",
		CommandID: "narration-commit", CommandHash: "narration-commit-hash",
	}); err != nil {
		t.Fatal(err)
	}

	window, err := application.CombatWindow(CombatWindowRequest{
		StoryID: storyID, BranchID: "main", SessionID: first.Session.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if window.LastRound == nil || strings.TrimSpace(window.LastRound.Narration) == "" {
		t.Fatalf("the committed passage must be shown: %#v", window.LastRound)
	}
	if window.NarrationError != "" {
		t.Fatalf("a round that has its prose must not offer a retry: %q", window.NarrationError)
	}
}
