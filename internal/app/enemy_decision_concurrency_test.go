package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

func TestEnemyDecisionConcurrentCombatRetryCallsModelOnce(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	entered := make(chan struct{})
	releaseResponse := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseResponse) }) }
	defer release()
	var calls atomic.Int32
	application.cfg = enemyDecisionTestConfig(t, application.workspace, func(request enemyDecisionTestRequest) string {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-releaseResponse
		return `{"candidate_id":"rest"}`
	})
	opened, err := application.PlayCombatRound(CombatRoundRequest{StoryID: storyID, CommandID: "concurrent-open", Combatants: combatTestSpecs(), SkipNarration: true})
	if err != nil {
		t.Fatal(err)
	}
	request := CombatRoundRequest{StoryID: storyID, SessionID: opened.SessionID, CommandID: "concurrent-round", SkipNarration: true, Actions: map[string]cultivation.CombatAction{interactive.DefaultActorID: {Kind: cultivation.ActionRest}}}
	const callers = 4
	done, eventIDs := make(chan error, callers), make(chan string, callers)
	play := func() error {
		window, err := application.PlayCombatRound(request)
		if err == nil && window.LastRound != nil {
			eventIDs <- window.LastRound.EventID
		}
		return err
	}
	runAppErrorTestGoroutine(done, "first enemy decision", play)
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first model call did not start")
	}
	for i := 1; i < callers; i++ {
		runAppErrorTestGoroutine(done, "concurrent enemy decision", play)
	}
	// Keep the first HTTP response pending while duplicate requests enter. This
	// is a bounded concurrency test wait, not a model runtime deadline.
	time.Sleep(30 * time.Millisecond)
	release()
	for i := 0; i < callers; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("concurrent decision did not finish")
		}
	}
	if calls.Load() != 1 || len(eventIDs) != callers {
		t.Fatalf("concurrent retries repeated model or missed receipt: calls = %d, receipts = %d", calls.Load(), len(eventIDs))
	}
	first := <-eventIDs
	for len(eventIDs) > 0 {
		if got := <-eventIDs; got != first {
			t.Fatalf("concurrent retries committed different rounds: %s vs %s", first, got)
		}
	}
	application.mu.RLock()
	remaining := len(application.enemyDecisionFlights)
	application.mu.RUnlock()
	if remaining != 0 {
		t.Fatal("completed decision gates were retained")
	}
}

func TestEnemyDecisionCommandWaitCanCancelAndReleasesEntries(t *testing.T) {
	application, storyID := combatTestApp(t)
	release, err := application.acquireEnemyDecisionCommand(context.Background(), application.interactive, storyID, "same-command")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	runAppErrorTestGoroutine(done, "canceled decision waiter", func() error {
		unlock, err := application.acquireEnemyDecisionCommand(ctx, application.interactive, storyID, "same-command")
		if unlock != nil {
			unlock()
		}
		return err
	})
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled waiter acquired decision: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("decision wait ignored cancellation")
	}
	// An unrelated command is not blocked by the active first command.
	otherRelease, err := application.acquireEnemyDecisionCommand(context.Background(), application.interactive, storyID, "other-command")
	if err != nil {
		t.Fatal(err)
	}
	otherRelease()
	release()
	application.mu.RLock()
	remaining := len(application.enemyDecisionFlights)
	application.mu.RUnlock()
	if remaining != 0 {
		t.Fatalf("decision gate leaked %d entries", remaining)
	}
}
