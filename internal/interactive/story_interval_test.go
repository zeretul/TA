package interactive

import (
	"testing"

	"ta/internal/cultivation"
	"ta/internal/quest"
)

// These tests are 自制-并行事项与到期事件 §五 as it happens during play: an
// interval of committed formal time settles every registered matter whose
// instant it crossed, whether or not the player was looking at it.
//
// The rules layer already proves the deadline arithmetic. What these tests add
// is that the pass runs from a commit and only from a commit: a rest or a
// training stretch really writes the expiry onto the branch, the journal really
// carries it, an open-ended matter beside it is untouched, and a reopened store
// reads the same result.

// intervalTestExpiryRecords returns the expiry records on the branch's journal,
// in journal order.
func intervalTestExpiryRecords(t *testing.T, store *Store, storyID string) []QuestRecordEvent {
	t.Helper()
	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	var expired []QuestRecordEvent
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeQuestRecord {
			continue
		}
		var event QuestRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			t.Fatal(err)
		}
		if event.Action == QuestActionExpire {
			expired = append(expired, event)
		}
	}
	return expired
}

// TestARestClosesAnAgreedDeadline is §五's core: the interval the rest really
// spent is what expires the promise, and the expiry is a record on the branch
// rather than a status the panel computed while reading.
//
// Nothing reads the task between acceptance and the rest, which is the
// unfollowed case: attention is not what keeps an agreement alive.
func TestARestClosesAnAgreedDeadline(t *testing.T) {
	store, storyID := recoveryTestStory(t)
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskSuQianMaterials, Action: QuestActionAccept,
		CommandID: "cmd-accept", CommandHash: "h-accept",
	})

	// Three days of rest against a two-day agreement. The deadline instant is
	// inside the interval, so the rest crosses it.
	recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 3*24*60*60, "cmd-rest")

	view := questTestTask(t, questTestState(t, store, storyID), quest.TaskSuQianMaterials)
	if view.Status != string(quest.StatusExpired) {
		t.Fatalf("the crossed deadline must expire the task: %q", view.Status)
	}
	if !view.Expired {
		t.Fatal("the panel must read the expiry as expired")
	}
	expired := intervalTestExpiryRecords(t, store, storyID)
	if len(expired) != 1 || expired[0].TaskID != quest.TaskSuQianMaterials {
		t.Fatalf("the interval must write exactly one expiry record: %#v", expired)
	}
	if expired[0].Instance.Status != quest.StatusExpired {
		t.Fatalf("the record must carry the expired instance: %#v", expired[0].Instance)
	}
}

// TestAnOpenEndedErrandSurvivesTheSameInterval is the A/B pair the design asks
// the first batch to demonstrate: A has no agreed limit, B does, and the same
// interval closes only B.
func TestAnOpenEndedErrandSurvivesTheSameInterval(t *testing.T) {
	store, storyID := recoveryTestStory(t)
	for _, taskID := range []string{quest.TaskHerbHandover, quest.TaskSuQianMaterials} {
		questTestApply(t, store, storyID, QuestRequest{
			TaskID: taskID, Action: QuestActionAccept,
			CommandID: "cmd-accept-" + taskID, CommandHash: "h-accept-" + taskID,
		})
	}

	recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 3*24*60*60, "cmd-rest")

	state := questTestState(t, store, storyID)
	open := questTestTask(t, state, quest.TaskHerbHandover)
	if open.Status != string(quest.StatusAccepted) {
		t.Fatalf("an errand with no agreed limit must stay open: %q", open.Status)
	}
	if open.Expired {
		t.Fatal("an errand with no agreed limit must never read as expired")
	}
	closed := questTestTask(t, state, quest.TaskSuQianMaterials)
	if closed.Status != string(quest.StatusExpired) {
		t.Fatalf("the limited matter must close: %q", closed.Status)
	}
	expired := intervalTestExpiryRecords(t, store, storyID)
	if len(expired) != 1 || expired[0].TaskID != quest.TaskSuQianMaterials {
		t.Fatalf("only the limited matter may be written: %#v", expired)
	}
}

// TestAnIntervalThatEndsBeforeTheDeadlineChangesNothing keeps the pass from
// closing a matter early. A one-day rest against a two-day agreement leaves it
// in play.
func TestAnIntervalThatEndsBeforeTheDeadlineChangesNothing(t *testing.T) {
	store, storyID := recoveryTestStory(t)
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskSuQianMaterials, Action: QuestActionAccept,
		CommandID: "cmd-accept", CommandHash: "h-accept",
	})

	recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 24*60*60, "cmd-rest")

	view := questTestTask(t, questTestState(t, store, storyID), quest.TaskSuQianMaterials)
	if view.Status != string(quest.StatusAccepted) {
		t.Fatalf("an interval that ends before the instant must not expire it: %q", view.Status)
	}
	if len(intervalTestExpiryRecords(t, store, storyID)) != 0 {
		t.Fatal("no expiry may be written before the instant is crossed")
	}
}

// TestADeadlineIsWrittenOnceAcrossManyIntervals keeps later intervals from
// repeating an expiry an earlier one already committed.
func TestADeadlineIsWrittenOnceAcrossManyIntervals(t *testing.T) {
	store, storyID := recoveryTestStory(t)
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskSuQianMaterials, Action: QuestActionAccept,
		CommandID: "cmd-accept", CommandHash: "h-accept",
	})

	// Day one is inside the limit; the next two cross it; a further day has
	// nothing left to close.
	recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 24*60*60, "cmd-rest-1")
	recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 2*24*60*60, "cmd-rest-2")
	recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 24*60*60, "cmd-rest-3")

	expired := intervalTestExpiryRecords(t, store, storyID)
	if len(expired) != 1 {
		t.Fatalf("the expiry must be written once, not once per interval: %#v", expired)
	}
	if view := questTestTask(t, questTestState(t, store, storyID), quest.TaskSuQianMaterials); view.Status != string(quest.StatusExpired) {
		t.Fatalf("the task must stay expired: %q", view.Status)
	}
}

// TestATrainingIntervalStopsAtAnAgreedDeadline is §五's warning rule as the
// planner applies it: a long seclusion pauses at the moment the promise runs
// out instead of training past it, and the pause is a real node rather than a
// sentence the narrator invented.
func TestATrainingIntervalStopsAtAnAgreedDeadline(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskSuQianMaterials, Action: QuestActionAccept,
		CommandID: "cmd-accept", CommandHash: "h-accept",
	})

	// Thirty days named against a two-day agreement.
	result := cultivationTestApply(t, store, storyID, CultivationRequest{
		Interval:  CultivationInterval{Value: 30, Unit: CultivationUnitDay},
		CommandID: "cmd-train", CommandHash: "h-train",
	})

	if result.Completed {
		t.Fatal("the training must stop at the deadline rather than run through it")
	}
	if result.Elapsed != 2*24*60*60 {
		t.Fatalf("the interval must stop at the agreed instant: elapsed=%d", result.Elapsed)
	}
	want := cultivationStopNodePrefix + quest.TaskSuQianMaterials + ":deadline"
	found := false
	for _, id := range result.PendingNodeIDs {
		if id == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("the deadline must be the boundary that stopped training: %#v", result.PendingNodeIDs)
	}
	if view := questTestTask(t, questTestState(t, store, storyID), quest.TaskSuQianMaterials); view.Status != string(quest.StatusExpired) {
		t.Fatalf("the paused instant must also close the promise: %q", view.Status)
	}
	if len(intervalTestExpiryRecords(t, store, storyID)) != 1 {
		t.Fatal("the pause must write the expiry once")
	}
}

// TestAnExpiredMatterSurvivesARestart is the save rule: the expiry is on the
// journal, so a reopened store reads the same closed state.
func TestAnExpiredMatterSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	system := combatTestSystem()
	store := NewStore(dir)
	story, err := store.CreateStory(CreateStoryRequest{Title: "到期", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角在山洞中停下。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	questTestKnownOffer(t, store, story.ID, quest.TaskSuQianMaterials)
	if _, err := store.ApplyQuest(story.ID, QuestRequest{
		BranchID: "main", TaskID: quest.TaskSuQianMaterials, Action: QuestActionAccept,
		CommandID: "cmd-accept", CommandHash: "h-accept",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplySafeRecovery(story.ID, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoveryHealthRest,
		DurationSeconds: 3 * 24 * 60 * 60,
		CommandID:       "cmd-rest", CommandHash: "h-rest",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := NewStore(dir)
	t.Cleanup(func() { _ = reopened.Close() })
	state, err := reopened.QuestStateFor(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if view := questTestTask(t, state, quest.TaskSuQianMaterials); view.Status != string(quest.StatusExpired) {
		t.Fatalf("the reopened store must read the expiry: %q", view.Status)
	}
	if len(intervalTestExpiryRecords(t, reopened, story.ID)) != 1 {
		t.Fatal("the expiry record must survive the restart")
	}
}
