package interactive

import (
	"math"
	"testing"

	"ta/internal/cultivation"
)

// A settlement commits as a side record parented after the Turn it names, and
// writes its progress as a delta parented to that record. Cutting a fork at the
// Turn keeps the side record — belongsToStoryAncestorPath attributes it to the
// Turn — so the growth watermark advances, but the delta carries no Turn of its
// own and the envelope test dropped it. The fork then held a watermark already
// paid for with a progress of zero: the half-applied commit the checkpoint
// design forbids, and the reason a fork of a settled Turn showed 0 on every NPC.
//
// The path that matters is the one play takes: a Turn that settles the interval.
// The window path (ApplySafeRecovery with no TurnID) names no Turn at all and is
// off the path for both halves, which the second test pins.
func TestForkOfASettledTurnKeepsNPCProgress(t *testing.T) {
	store, storyID := npcGrowthTestStory(t)
	turnID := forkTestTurn(t, store, storyID, &TurnResult{
		Choices: testTurnChoices(),
		Rest: &TurnRestInput{
			Mode:            string(cultivation.RecoverySafeMeditation),
			DurationSeconds: 3600,
		},
	})

	main := npcGrowthTestProgress(t, store, storyID, "rival")
	if math.Abs(main) < 1e-9 {
		t.Fatalf("the interval should have banked fractional progress, got %v", main)
	}

	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: turnID, Title: "结算后分支"})
	if err != nil {
		t.Fatal(err)
	}
	forked, err := store.Snapshot(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := actorStateNumber(actorStateFieldValue(forked.State, "rival", xiuxianFieldRealmProgress))
	if math.Abs(got-main) > 1e-10 {
		t.Fatalf("fork lost the settled progress: main=%v branch=%v", main, got)
	}
}

// The guard: a settlement taken from the window after a Turn names no Turn, so a
// fork cut at that Turn sits before it and must keep neither the watermark nor
// the progress. Widening attribution to "the record is just past the cut" would
// hand the fork an interval it never lived.
func TestForkOfTheTurnBeforeAWindowRestKeepsNoneOfIt(t *testing.T) {
	store, storyID := npcGrowthTestStory(t)
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	turnID := snapshot.Turns[len(snapshot.Turns)-1].ID

	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 3600, "fork-window-rest")
	if math.Abs(npcGrowthTestProgress(t, store, storyID, "rival")) < 1e-9 {
		t.Fatal("the window rest should have banked fractional progress on main")
	}

	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: turnID, Title: "窗口休整之前"})
	if err != nil {
		t.Fatal(err)
	}
	forked, err := store.Snapshot(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := actorStateNumber(actorStateFieldValue(forked.State, "rival", xiuxianFieldRealmProgress))
	if math.Abs(got) > 1e-10 {
		t.Fatalf("a fork cut before the window rest must not inherit its progress, got %v", got)
	}
}
