package interactive

import (
	"testing"

	"ta/internal/publiquevent"
	"ta/internal/storytime"
)

// This file is the remainder of §B1's acceptance flow:
//
//	本批验收一条正常"输入闭关→实际时间和成长→瓶颈停下→明确突破→结果回正文→恢复"的
//	流程，再覆盖同请求/双击/丢响应不重复、合法新分段等价、分叉/回退不串。
//
// The normal flow is covered piecewise: b1_cultivation_time_test.go has the
// interval, the bottleneck and the breakthrough; b2_stop_node_test.go has the
// boundary, the saved remainder and the continuation. This file covers the two
// remaining acceptance clauses.
//
// "同请求/双击/丢响应不重复" is the same request arriving twice, whether the
// player pressed the button twice or the first response never reached them. The
// turn identity already makes the Turn and its delta idempotent
// (TestAppendTurnWithStateIsIdempotentByDurableAgentIdentity); what has to hold
// here is that the cultivation side record folds with it, because a second
// record would hand out the interval a second time.
//
// "分叉/回退不串" is the paused intent staying with the branch that really holds
// it. A fork cut before the training must not inherit it, a continuation on one
// branch must not move the other, and a rewind past the training must drop the
// intent along with the time it spent.

// b1TurnTrainUnderIdentity commits one training turn under a durable Agent
// identity, which is what makes a repeat of the same request detectable.
func b1TurnTrainUnderIdentity(t *testing.T, store *Store, storyID string, identity DomainCommitIdentity, text string, input TurnCultivationInput) (TurnEvent, StoryClock) {
	t.Helper()
	identity = b1CommitInput(t, store, storyID, identity, text)
	turn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:         "main",
		User:             text,
		Narrative:        "主角在洞府中盘膝坐下，依功法运转周天。",
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

// b1RecordCount counts the journal records of one type on the main branch.
func b1RecordCount(t *testing.T, store *Store, storyID, eventType string) int {
	t.Helper()
	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, record := range records {
		if record.Envelope.Type == eventType {
			count++
		}
	}
	return count
}

// b1OpeningTurnID returns the story's first committed turn, which is the fork
// point that sits before everything the test itself does.
func b1OpeningTurnID(t *testing.T, store *Store, storyID string) string {
	t.Helper()
	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Envelope.Type == StoryEventTypeTurn {
			return record.Envelope.ID
		}
	}
	t.Fatal("the story must have an opening turn")
	return ""
}

// TestB1ARepeatedTrainingRequestSettlesOnce is "同请求/双击/丢响应不重复" on the
// turn path.
//
// A player who presses send twice, or whose first response is lost and retried,
// submits one training turn. The retry must find that turn rather than train the
// same interval again: the branch stays at the same instant, the growth is
// unchanged, and the journal still holds one cultivation record for it.
func TestB1ARepeatedTrainingRequestSettlesOnce(t *testing.T) {
	store, storyID := cultivationTestStory(t, nil)
	identity := b1Identity("op-b1-repeat", 1)
	input := TurnCultivationInput{Days: 30}

	first, firstClock := b1TurnTrainUnderIdentity(t, store, storyID, identity, "闭关一月", input)
	firstProgress := cultivationTestStoredFloat(t, store, storyID, "main", xiuxianFieldRealmProgress)
	_, firstActivities, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(firstActivities) != 1 {
		t.Fatalf("one training turn publishes one interval: %#v", firstActivities)
	}
	if got := b1RecordCount(t, store, storyID, StoryEventTypeCultivationRecord); got != 1 {
		t.Fatalf("one training turn publishes one record: count=%d", got)
	}

	// The identical request again, as a double click or a retry after a lost
	// response.
	second, secondClock := b1TurnTrainUnderIdentity(t, store, storyID, identity, "闭关一月", input)

	if second.ID != first.ID {
		t.Fatalf("a repeated request must return the turn it already committed: %q vs %q", second.ID, first.ID)
	}
	if secondClock.Seconds != firstClock.Seconds {
		t.Fatalf("a repeated request must not spend the interval twice: clock=%d want=%d", secondClock.Seconds, firstClock.Seconds)
	}
	if got := cultivationTestStoredFloat(t, store, storyID, "main", xiuxianFieldRealmProgress); got != firstProgress {
		t.Fatalf("a repeated request must not train twice: progress=%v want=%v", got, firstProgress)
	}
	_, secondActivities, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(secondActivities) != 1 {
		t.Fatalf("a repeated request must not add a second interval: %#v", secondActivities)
	}
	if got := b1RecordCount(t, store, storyID, StoryEventTypeCultivationRecord); got != 1 {
		t.Fatalf("a repeated request must not publish a second record: count=%d", got)
	}
}

// TestB1AForkAndARewindDoNotShareThePausedIntent is "分叉/回退不串".
//
// The paused intent belongs to the branch that reached the boundary. A fork cut
// before the training must not inherit it or the time it spent, continuing on
// main must not move the fork, and rewinding main past the training must take
// the intent back with the interval it belongs to.
func TestB1AForkAndARewindDoNotShareThePausedIntent(t *testing.T) {
	store, storyID := b2StopStoryAt(t, nil)
	publicEventTestApply(t, store, storyID, PublicEventRequest{
		Action:              PublicEventActionJoin,
		ParticipationAction: string(publiquevent.ActionObserve),
	})
	openingTurnID := b1OpeningTurnID(t, store, storyID)

	const rushStage = int64(6 * 60 * 60)
	training, clock := b1TurnTrainUnderIdentity(t, store, storyID, b1Identity("op-b1-fork", 1), "闭关一日", TurnCultivationInput{Days: 1})
	if clock.Seconds != rushStage {
		t.Fatalf("the training must stop at the event stage: clock=%d want=%d", clock.Seconds, rushStage)
	}
	mainBefore := b2StopActivities(t, store, storyID)
	if len(mainBefore) != 1 {
		t.Fatalf("the stopped interval must be published on main: %#v", mainBefore)
	}

	// The fork sits before the training, so it holds neither the intent nor the
	// time.
	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: openingTurnID, Title: "修炼之前"})
	if err != nil {
		t.Fatal(err)
	}
	forkClock, forkActivities, err := store.CultivationState(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(forkActivities) != 0 {
		t.Fatalf("a fork cut before the training must not inherit its paused intent: %#v", forkActivities)
	}
	if forkClock.Seconds != 0 {
		t.Fatalf("a fork cut before the training must not inherit its spent time: clock=%d", forkClock.Seconds)
	}

	// Continuing on main settles the remainder there and nowhere else.
	b1TurnTrain(t, store, storyID, "cmd-b1-fork-cont", TurnCultivationInput{Days: 1})
	mainAfter := b2StopActivities(t, store, storyID)
	if len(mainAfter) != 1 {
		t.Fatalf("the continuation keeps one intent on main: %#v", mainAfter)
	}
	for _, activity := range mainAfter {
		if activity.Activity.Status != storytime.ActivityCompleted {
			t.Fatalf("the continuation must finish the day it was owed: %#v", activity.Activity)
		}
	}
	_, forkAfter, err := store.CultivationState(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(forkAfter) != 0 {
		t.Fatalf("a continuation on main must not reach the fork: %#v", forkAfter)
	}
	forkClockAfter, _, err := store.CultivationState(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if forkClockAfter.Seconds != forkClock.Seconds {
		t.Fatalf("a continuation on main must not move the fork: clock=%d want=%d", forkClockAfter.Seconds, forkClock.Seconds)
	}

	// Rewinding main past the training takes the intent back with the interval
	// that produced it.
	if err := store.RewindToTurnParent(storyID, RewindTurnRequest{BranchID: "main", TurnID: training.ID}); err != nil {
		t.Fatal(err)
	}
	rewoundClock, rewoundActivities, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(rewoundActivities) != 0 {
		t.Fatalf("a rewind past the training must drop the intent: %#v", rewoundActivities)
	}
	if rewoundClock.Seconds != 0 {
		t.Fatalf("a rewind past the training must drop the time it spent: clock=%d", rewoundClock.Seconds)
	}
}
