package interactive

import (
	"strings"
	"testing"

	"ta/internal/cultivation"
)

// These tests cover the P1 the fixed package's journal proved: an NPC the story
// explicitly trained was then re-priced by the automatic growth pass over the
// same real interval, and the second write overwrote the first.
//
// The journal (story st_dma6l93ci4xof83ae739, command r02-npc-interval-001) shows
// one transaction doing all of this in order:
//
//	Turn ev_dma70n6rpffca02e7093's cultivation_advance   -> progress 16900
//	cultivation_record cul_dma70n6x5vyg1f8807b9         -> clock 0..172800000
//	npc_growth_record npg_dma70n6y6r70a57b5e21          -> same 0..172800000
//	state_delta sd_dma70n6ytivcc6330b6c                 -> progress 8666.666666666666
//
// Two settlements priced one actor's one interval and the later one won. That is
// not a display or history-cache effect: the canonical state ends at the growth
// pass's number.
//
// The rule these tests pin: one actor's one real interval has one settlement
// owner. An interval the story already settled through the cultivation rules is
// not re-priced by the automatic pass; every other NPC still grows on its own
// conditions, and the remainder of an interval nobody claimed is still paid.

// npcGrowthTwoRivalsStory builds a story with the protagonist and two
// non-protagonist cultivators, so a test can tell "skipped the named actor" from
// "stopped all growth".
func npcGrowthTwoRivalsStory(t *testing.T) (*Store, string) {
	t.Helper()
	system := combatTestSystem()
	fields := append([]ActorStateField{}, system.Templates[0].Fields...)
	system.Templates = append(system.Templates, ActorStateTemplate{ID: "rival-b", Name: "第二个同门", Fields: fields})
	system.InitialActors = append(system.InitialActors, ActorStateInitialActor{
		ID: "rival-b", Name: "第二个同门", TemplateID: "rival-b",
		State: map[string]any{xiuxianFieldRealm: system.InitialActors[1].State[xiuxianFieldRealm]},
	})
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "NPC显式修炼", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角在山门中停下，两位同门在旁练剑。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// TestAnExplicitlyTrainedNPCIsNotRepricedByTheAutomaticPass is the P1
// reproduction.
//
// The story names the rival and trains them for thirty days. The cultivation
// record settles that interval at the rules' own answer for the rival's
// conditions; the automatic growth pass must then leave that same interval alone,
// because it is the same actor's same stretch of real time and a second pricing
// is a second, contradictory grant.
func TestAnExplicitlyTrainedNPCIsNotRepricedByTheAutomaticPass(t *testing.T) {
	store, storyID := npcGrowthTwoRivalsStory(t)

	result := cultivationTestApply(t, store, storyID, CultivationRequest{
		BranchID:  "main",
		ActorID:   "rival",
		Interval:  CultivationInterval{Value: 30, Unit: CultivationUnitDay},
		CommandID: "cmd-npc-explicit", CommandHash: "hash-npc-explicit",
	})
	if result.Advance.ConsumedDays <= 0 {
		t.Fatalf("the explicit interval must really train the rival: %+v", result.Advance)
	}

	// The rules' own figure for that interval, as the cultivation commit recorded
	// it. The automatic pass must not have replaced it.
	explicit := result.Advance.RealmProgressAfter
	stored := npcGrowthTestProgress(t, store, storyID, "rival")
	if !sameFloat(explicit, stored) {
		t.Fatalf("the automatic pass re-priced an interval the cultivation rules already settled: "+
			"cultivation wrote %v, stored %v", explicit, stored)
	}

	// And the disagreement must not be hidden in the records either: a growth
	// record covering this interval is a second settlement of it.
	for _, record := range npcGrowthTestRecords(t, store, storyID) {
		if record.Candidate.ActorID != "rival" {
			continue
		}
		if record.ClockBefore.Seconds < result.ClockAfter.Seconds && record.ClockAfter.Seconds > result.ClockAfter.Seconds-result.Elapsed {
			t.Fatalf("the automatic pass wrote a growth record over the explicitly settled interval: %+v", record)
		}
	}
}

// TestAnExplicitlyTrainedNPCLeavesTheOtherNPCsGrowing is the other half of the
// rule: skipping the actor the story already settled must not skip the rest.
// Suppressing all NPC growth would "fix" the conflict by removing the feature.
func TestAnExplicitlyTrainedNPCLeavesTheOtherNPCsGrowing(t *testing.T) {
	store, storyID := npcGrowthTwoRivalsStory(t)

	before := npcGrowthTestProgress(t, store, storyID, "rival-b")
	cultivationTestApply(t, store, storyID, CultivationRequest{
		BranchID:  "main",
		ActorID:   "rival",
		Interval:  CultivationInterval{Value: 30, Unit: CultivationUnitDay},
		CommandID: "cmd-npc-explicit-2", CommandHash: "hash-npc-explicit-2",
	})
	after := npcGrowthTestProgress(t, store, storyID, "rival-b")
	if after <= before {
		t.Fatalf("an NPC the explicit interval did not name must still grow: %v -> %v", before, after)
	}
}

// TestAnUnclaimedRemainderIsStillPaid covers the interval arithmetic: the
// explicit settlement owns only what it really spent. A later window that starts
// where it ended is nobody's yet and must be paid.
func TestAnUnclaimedRemainderIsStillPaid(t *testing.T) {
	store, storyID := npcGrowthTwoRivalsStory(t)
	cultivationTestApply(t, store, storyID, CultivationRequest{
		BranchID:  "main",
		ActorID:   "rival",
		Interval:  CultivationInterval{Value: 10, Unit: CultivationUnitDay},
		CommandID: "cmd-npc-rem-1", CommandHash: "hash-npc-rem-1",
	})
	afterExplicit := npcGrowthTestProgress(t, store, storyID, "rival")

	// A rest spends another thirty days on the branch clock. The rival is not in
	// it explicitly, so the automatic pass owns this window.
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 30*24*60*60, "cmd-npc-rem-rest")
	afterRest := npcGrowthTestProgress(t, store, storyID, "rival")
	if afterRest <= afterExplicit {
		t.Fatalf("a window the explicit interval did not cover must still be paid: %v -> %v",
			afterExplicit, afterRest)
	}
}

// TestAnExplicitlySettledIntervalIsNotPaidAgainOnRetry keeps the retry rule: the
// same command returns the first answer, and the automatic pass has not written a
// competing figure that a retry would then re-read.
func TestAnExplicitlySettledIntervalIsNotPaidAgainOnRetry(t *testing.T) {
	store, storyID := npcGrowthTwoRivalsStory(t)
	req := CultivationRequest{
		BranchID:  "main",
		ActorID:   "rival",
		Interval:  CultivationInterval{Value: 30, Unit: CultivationUnitDay},
		CommandID: "cmd-npc-retry", CommandHash: "hash-npc-retry",
	}
	first := cultivationTestApply(t, store, storyID, req)
	firstStored := npcGrowthTestProgress(t, store, storyID, "rival")

	replayed := cultivationTestApply(t, store, storyID, req)
	if !replayed.Replayed {
		t.Fatal("the same command must read back the committed interval rather than settle it again")
	}
	if replayed.EventID != first.EventID {
		t.Fatalf("the retry must name the committed interval: %s vs %s", replayed.EventID, first.EventID)
	}
	if stored := npcGrowthTestProgress(t, store, storyID, "rival"); !sameFloat(stored, firstStored) {
		t.Fatalf("a retry must not move the stored progress: %v -> %v", firstStored, stored)
	}
}

// sameFloat compares two progress figures at the rules' own precision. The
// growth rules integrate days as float64, so an exact comparison would fail on
// the last bit for reasons that have nothing to do with this rule.
func sameFloat(a, b float64) bool {
	const epsilon = 1e-6
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= epsilon
}

// TestTheExplicitIntervalSurvivesAReopen is the durability half. The settlement
// is a journal record, not a process-local figure, so closing the store and
// opening the same directory again must read the same numbers — and must not
// settle the same seconds a second time while doing it.
func TestTheExplicitIntervalSurvivesAReopen(t *testing.T) {
	system := combatTestSystem()
	fields := append([]ActorStateField{}, system.Templates[0].Fields...)
	system.Templates = append(system.Templates, ActorStateTemplate{ID: "rival-b", Name: "第二个同门", Fields: fields})
	system.InitialActors = append(system.InitialActors, ActorStateInitialActor{
		ID: "rival-b", Name: "第二个同门", TemplateID: "rival-b",
		State: map[string]any{xiuxianFieldRealm: system.InitialActors[1].State[xiuxianFieldRealm]},
	})
	dir := t.TempDir()

	store := NewStore(dir)
	story, err := store.CreateStory(CreateStoryRequest{Title: "NPC重开", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角在山门中停下，两位同门在旁练剑。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	result := cultivationTestApply(t, store, story.ID, CultivationRequest{
		BranchID:  "main",
		ActorID:   "rival",
		Interval:  CultivationInterval{Value: 30, Unit: CultivationUnitDay},
		CommandID: "cmd-npc-reopen", CommandHash: "hash-npc-reopen",
	})
	wantProgress := npcGrowthTestProgress(t, store, story.ID, "rival")
	wantRealm := npcGrowthTestRealm(t, store, story.ID, "rival")
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := NewStore(dir)
	t.Cleanup(func() { _ = reopened.Close() })
	if got := npcGrowthTestProgress(t, reopened, story.ID, "rival"); !sameFloat(got, wantProgress) {
		t.Fatalf("a reopen must read the committed progress: got %v, want %v", got, wantProgress)
	}
	if got := npcGrowthTestRealm(t, reopened, story.ID, "rival"); got != wantRealm {
		t.Fatalf("a reopen must read the committed realm: got %s, want %s", got, wantRealm)
	}
	// The interval is already settled; presenting the same command again must
	// read it back rather than pay for the same thirty days.
	replayed := cultivationTestApply(t, reopened, story.ID, CultivationRequest{
		BranchID:  "main",
		ActorID:   "rival",
		Interval:  CultivationInterval{Value: 30, Unit: CultivationUnitDay},
		CommandID: "cmd-npc-reopen", CommandHash: "hash-npc-reopen",
	})
	if !replayed.Replayed || replayed.EventID != result.EventID {
		t.Fatalf("a reopen must recognise the committed command: replayed=%v id=%s want=%s",
			replayed.Replayed, replayed.EventID, result.EventID)
	}
	if got := npcGrowthTestProgress(t, reopened, story.ID, "rival"); !sameFloat(got, wantProgress) {
		t.Fatalf("a replay after reopen must not move the progress: %v -> %v", wantProgress, got)
	}
}

// TestAnExplicitBreakthroughReadsItsRealVerdict is the "accepted is not success"
// half of the N12 finding.
//
// module_status=accepted only says the submission was taken. It says nothing
// about whether the NPC met the rules' preconditions or whether the attempt
// succeeded. So this test trains a rival who stands at the qi threshold through
// the same explicit command the journal used, asks for a 筑基 attempt, and then
// reads what the rules actually decided: the eligibility finding, the resolved
// outcome, the pools the attempt really moved, and the recovery a failure
// imposes. Each must agree between the returned result, the committed record and
// the stored snapshot.
func TestAnExplicitBreakthroughReadsItsRealVerdict(t *testing.T) {
	store, storyID := npcGrowthTestBoundaryStory(t)

	result := cultivationTestApply(t, store, storyID, CultivationRequest{
		BranchID: "main",
		ActorID:  "rival",
		Interval: CultivationInterval{Value: 1, Unit: CultivationUnitDay},
		Conditions: &CultivationConditions{
			AttemptBreakthrough: string(cultivation.BreakthroughFoundation),
		},
		CommandID: "cmd-npc-breakthrough", CommandHash: "hash-npc-breakthrough",
	})
	if !result.Advance.BreakthroughRequested {
		t.Fatal("the request must reach the rules as an authorised attempt")
	}
	// The verdict is the rules' own, not the submission's. An ineligible
	// character must be told why and must not be rolled for.
	if !result.Advance.BreakthroughEligible {
		if len(result.Advance.BreakthroughReasons) == 0 {
			t.Fatal("an ineligible attempt must report the precondition that failed")
		}
		if result.Advance.BreakthroughRolled {
			t.Fatal("an ineligible attempt must not be rolled")
		}
		return
	}
	if !result.Advance.BreakthroughRolled {
		t.Fatal("an eligible attempt must be resolved, not left pending")
	}

	// The record is the durable half: reopening or reloading must read the same
	// verdict the caller got back.
	var committed *CultivationRecordEvent
	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeCultivationRecord {
			continue
		}
		var event CultivationRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(event.CommandID) == "cmd-npc-breakthrough" {
			committed = &event
		}
	}
	if committed == nil {
		t.Fatal("the attempt must be committed as a cultivation record")
	}
	if committed.Advance.BreakthroughSucceeded != result.Advance.BreakthroughSucceeded {
		t.Fatalf("the record and the result must agree on the verdict: record=%v result=%v",
			committed.Advance.BreakthroughSucceeded, result.Advance.BreakthroughSucceeded)
	}
	if committed.Advance.RealmAfter != result.Advance.RealmAfter {
		t.Fatalf("the record and the result must agree on the realm: %s vs %s",
			committed.Advance.RealmAfter, result.Advance.RealmAfter)
	}

	// The snapshot is the third witness. Whatever the rules decided, the stored
	// stage and the stored progress must be that decision rather than a figure
	// from another pass.
	if realm := npcGrowthTestRealm(t, store, storyID, "rival"); realm != result.Advance.RealmAfter {
		t.Fatalf("the stored realm must be what the rules answered: stored=%s answered=%s",
			realm, result.Advance.RealmAfter)
	}
	if progress := npcGrowthTestProgress(t, store, storyID, "rival"); !sameFloat(progress, result.Advance.RealmProgressAfter) {
		t.Fatalf("the stored progress must be what the rules answered: stored=%v answered=%v",
			progress, result.Advance.RealmProgressAfter)
	}

	if result.Advance.BreakthroughSucceeded {
		// Success is a new realm with nothing banked toward the next one.
		want := cultivation.StageKey(cultivation.Cultivation{Realm: cultivation.RealmFoundation, Stage: cultivation.StageEarly})
		if result.Advance.RealmAfter != want {
			t.Fatalf("a successful 筑基 attempt must enter the new realm: %s", result.Advance.RealmAfter)
		}
	} else if result.Advance.BreakthroughRecoveryDays <= 0 {
		t.Fatal("a failed attempt must impose the rules' recovery period")
	}
}

// TestAnIneligibleExplicitAttemptIsRefusedWithAReason is the other side: a
// character who has not met the preconditions must get an explainable refusal,
// not a silent success and not a fabricated one.
func TestAnIneligibleExplicitAttemptIsRefusedWithAReason(t *testing.T) {
	// The ordinary story starts its rival at a low stage with no accumulated
	// progress, so the threshold is not met.
	store, storyID := npcGrowthTwoRivalsStory(t)

	result := cultivationTestApply(t, store, storyID, CultivationRequest{
		BranchID: "main",
		ActorID:  "rival",
		Interval: CultivationInterval{Value: 1, Unit: CultivationUnitDay},
		Conditions: &CultivationConditions{
			AttemptBreakthrough: string(cultivation.BreakthroughFoundation),
		},
		CommandID: "cmd-npc-ineligible", CommandHash: "hash-npc-ineligible",
	})
	if !result.Advance.BreakthroughRequested {
		t.Fatal("the request must reach the rules even when the character cannot meet it")
	}
	if result.Advance.BreakthroughEligible {
		t.Fatalf("a character below the threshold must not be eligible: %+v", result.Advance)
	}
	if len(result.Advance.BreakthroughReasons) == 0 {
		t.Fatal("the refusal must name the precondition that failed")
	}
	if result.Advance.BreakthroughRolled {
		t.Fatal("a refused attempt must not be rolled")
	}
	// And the refusal must not have moved the character into the target realm.
	if realm := npcGrowthTestRealm(t, store, storyID, "rival"); realm == cultivation.StageKey(cultivation.Cultivation{Realm: cultivation.RealmFoundation, Stage: cultivation.StageEarly}) {
		t.Fatalf("a refused attempt must not enter the target realm: %s", realm)
	}
}
