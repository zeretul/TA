package interactive

import (
	"errors"
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/quest"
)

// These tests cover 自制-并行事项与到期事件 §五's second half as play reaches it:
// an interval of committed formal time runs past a message's hour and past an
// agreed task deadline, and the two must answer differently.
//
// A task that ran out closes. A message is not an agreement, so nothing about it
// closes: it keeps the hour it was written and the words it used, and the one
// thing that changes is that what it offered can no longer be taken up. The
// earlier suite proved the task half; what these add is the message half, and the
// boundary between them.
//
// Every fixture here carries a story calendar, which is what the production entry
// freezes for a new story. Without one an absolute label such as 第6日 酉时 is not
// placed on the clock at all, so a test that skipped it would prove the interval
// form only and leave the absolute form unproven.

// messageTestStory is the same story the rest and cultivation suites build, with
// the calendar a new story really gets.
func messageTestStory(t *testing.T) (*Store, string) {
	t.Helper()
	system := combatTestSystem()
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	calendar := DefaultStoryCalendar()
	story, err := store.CreateStory(CreateStoryRequest{Title: "来讯与期限", ActorState: &system, Calendar: &calendar})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角在山洞中停下。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// messageTestRegister registers the two people the fixture's cast needs. A
// message must come from someone the story has met, so the sender is registered
// before anything is delivered.
func messageTestRegister(t *testing.T, store *Store, storyID string) {
	t.Helper()
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 酉时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
		Profiles: characterTestProfiles(),
	}); err != nil {
		t.Fatal(err)
	}
}

// messageTestDeliver writes one incoming message and fails the test if it is
// refused.
func messageTestDeliver(t *testing.T, store *Store, storyID string, message character.Message) {
	t.Helper()
	if err := store.AppendCharacterMessages(storyID, "main", []character.Message{message}); err != nil {
		t.Fatalf("deliver %s: %v", message.ID, err)
	}
}

// messageTestMessages reads the sender's messages off the branch.
func messageTestMessages(t *testing.T, store *Store, storyID, characterID string) []character.Message {
	t.Helper()
	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return state.MessagesFor(characterID)
}

// messageTestBoundMessage returns the one message with the given id.
func messageTestBoundMessage(t *testing.T, store *Store, storyID, characterID, messageID string) character.Message {
	t.Helper()
	for _, message := range messageTestMessages(t, store, storyID, characterID) {
		if message.ID == messageID {
			return message
		}
	}
	t.Fatalf("message %s is not on the branch: %+v", messageID, messageTestMessages(t, store, storyID, characterID))
	return character.Message{}
}

// messageTestRest commits one interval of committed formal time. It is the same
// activity a player's 闭关 becomes, so the clock moves through the path the game
// really uses rather than by writing a number into the fixture.
func messageTestRest(t *testing.T, store *Store, storyID string, seconds int64, commandID string) {
	t.Helper()
	if _, err := store.ApplySafeRecovery(storyID, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoveryHealthRest,
		DurationSeconds: seconds, CommandID: commandID, CommandHash: commandID + "-hash",
	}); err != nil {
		t.Fatalf("commit the interval %s: %v", commandID, err)
	}
}

// TestAMessageInsideItsHourStillStands is the first of the three results the
// design asks for: an interval that ends before the hour the letter named leaves
// the letter live and open.
func TestAMessageInsideItsHourStillStands(t *testing.T) {
	store, storyID := messageTestStory(t)
	messageTestRegister(t, store, storyID)
	messageTestDeliver(t, store, storyID, character.Message{
		ID: "msg-invite", CharacterID: "guardsman",
		Text:  "三日后北门换防，我在门内等你，过时不候。",
		Basis: "主角此前与他约定在北门再会", WorldTime: "第3日 酉时", Deadline: "三日内",
	})

	// One day against a three-day invitation.
	messageTestRest(t, store, storyID, 24*60*60, "cmd-rest-1")

	invitation := messageTestBoundMessage(t, store, storyID, "guardsman", "msg-invite")
	if invitation.DeadlineSeconds <= 0 {
		t.Fatalf("the interval deadline was not placed on the clock: %+v", invitation)
	}
	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if clock := SnapshotClockSeconds(mustSnapshot(t, store, storyID)); clock <= 0 {
		t.Fatalf("the interval did not move the formal clock: %d", clock)
	} else if invitation.LapsedAt("第4日", clock) {
		t.Fatalf("a letter inside its hour was marked lapsed: %+v", invitation)
	}
	if pending := character.PendingMessages(state.Messages, "第4日"); len(pending) != 1 {
		t.Fatalf("a live invitation left the pending list: %+v", pending)
	}
}

// TestAMessageWhoseHourPassedKeepsItsWordsAndStopsOffering is the second result.
// The interval crosses the hour, and the letter is still there afterwards — with
// its own send time, its own words and its own deadline — but what it proposed is
// no longer on offer.
func TestAMessageWhoseHourPassedKeepsItsWordsAndStopsOffering(t *testing.T) {
	store, storyID := messageTestStory(t)
	messageTestRegister(t, store, storyID)
	messageTestDeliver(t, store, storyID, character.Message{
		ID: "msg-invite", CharacterID: "guardsman",
		Text:  "三日后北门换防，我在门内等你，过时不候。",
		Basis: "主角此前与他约定在北门再会", WorldTime: "第3日 酉时", Deadline: "三日内",
	})

	// Five days against a three-day invitation.
	messageTestRest(t, store, storyID, 5*24*60*60, "cmd-rest-1")

	invitation := messageTestBoundMessage(t, store, storyID, "guardsman", "msg-invite")
	clock := SnapshotClockSeconds(mustSnapshot(t, store, storyID))
	if !invitation.LapsedAt("第8日", clock) {
		t.Fatalf("an invitation whose hour passed must read as lapsed: %+v", invitation)
	}
	// The record itself is not rewritten by the passing of its hour.
	if invitation.WorldTime != "第3日 酉时" || invitation.Deadline != "三日内" {
		t.Fatalf("the interval rewrote the letter: %+v", invitation)
	}
	if invitation.Text != "三日后北门换防，我在门内等你，过时不候。" {
		t.Fatalf("the interval rewrote the words: %q", invitation.Text)
	}
	if invitation.Basis != "主角此前与他约定在北门再会" {
		t.Fatalf("the interval erased the reason the letter exists: %q", invitation.Basis)
	}
	// It stays in the player's list rather than vanishing: the story still has to
	// be able to say that someone wrote and was not answered.
	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if pending := character.PendingMessages(state.Messages, "第8日"); len(pending) != 1 {
		t.Fatalf("a lapsed but unanswered letter must stay readable: %+v", pending)
	}
}

// TestAnExpiredInvitationCannotBeAcceptedOrPaidOut is the rule the design states
// in one sentence: 有期限的邀请到期后不能继续以有效邀请接受或交付重复奖励.
//
// The task side of the same arrangement is where that is enforced. A completed
// task whose acceptance ran out keeps its claim only when it was finished inside
// the agreed time, and an unfinished one cannot be finished afterwards.
func TestAnExpiredInvitationCannotBeAcceptedOrPaidOut(t *testing.T) {
	store, storyID := recoveryTestStory(t)
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskSuQianMaterials, Action: QuestActionAccept,
		CommandID: "cmd-accept", CommandHash: "h-accept",
	})

	// Three days of rest against the task's two-day agreement.
	recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 3*24*60*60, "cmd-rest")

	if _, err := store.ApplyQuest(storyID, QuestRequest{
		BranchID: "main", TaskID: quest.TaskSuQianMaterials, Action: QuestActionComplete,
		CommandID: "cmd-complete", CommandHash: "h-complete",
	}); !errors.Is(err, ErrQuestExpired) {
		t.Fatalf("an expired acceptance must not complete: %v", err)
	}
	if _, err := store.ApplyQuest(storyID, QuestRequest{
		BranchID: "main", TaskID: quest.TaskSuQianMaterials, Action: QuestActionSettle,
		CommandID: "cmd-settle", CommandHash: "h-settle",
	}); !errors.Is(err, ErrQuestExpired) {
		t.Fatalf("an expired acceptance must not pay out: %v", err)
	}
	// Neither refusal changed the task's standing.
	if view := questTestTask(t, questTestState(t, store, storyID), quest.TaskSuQianMaterials); view.Status != string(quest.StatusExpired) {
		t.Fatalf("a refused claim changed the task: %q", view.Status)
	}
}

// TestAPlainGreetingKeepsItsHourAndGetsNoCountdown is the third result, and the
// rule that keeps a friendly note from becoming a timer: a message with no real
// deadline is never lapsed, and it keeps the hour it was written at however far
// the story moves on.
func TestAPlainGreetingKeepsItsHourAndGetsNoCountdown(t *testing.T) {
	store, storyID := messageTestStory(t)
	messageTestRegister(t, store, storyID)
	messageTestDeliver(t, store, storyID, character.Message{
		ID: "msg-hello", CharacterID: "guardsman",
		Text:  "这几日北门风大，夜里添件衣裳。",
		Basis: "主角上次离城时说过要回来", WorldTime: "第3日 酉时",
	})

	messageTestRest(t, store, storyID, 30*24*60*60, "cmd-rest-1")

	greeting := messageTestBoundMessage(t, store, storyID, "guardsman", "msg-hello")
	if greeting.DeadlineSeconds != 0 {
		t.Fatalf("a greeting with no deadline was given an instant: %+v", greeting)
	}
	clock := SnapshotClockSeconds(mustSnapshot(t, store, storyID))
	if greeting.LapsedAt("第33日", clock) {
		t.Fatalf("a greeting with no deadline lapsed: %+v", greeting)
	}
	if greeting.WorldTime != "第3日 酉时" {
		t.Fatalf("the interval rewrote the send time: %q", greeting.WorldTime)
	}
	if greeting.Text != "这几日北门风大，夜里添件衣裳。" {
		t.Fatalf("the interval rewrote the words: %q", greeting.Text)
	}
}

// TestAMessageIsDeliveredOnceAcrossARetry covers the retry half of the
// acceptance list. A message has no command id, so its identity is what makes it
// one record: the same letter submitted again — a retried commit, a recovery
// re-read — is the same letter.
func TestAMessageIsDeliveredOnceAcrossARetry(t *testing.T) {
	store, storyID := messageTestStory(t)
	messageTestRegister(t, store, storyID)
	invitation := character.Message{
		ID: "msg-invite", CharacterID: "guardsman",
		Text:  "三日后北门换防，我在门内等你，过时不候。",
		Basis: "主角此前与他约定在北门再会", WorldTime: "第3日 酉时", Deadline: "三日内",
	}
	messageTestDeliver(t, store, storyID, invitation)
	messageTestDeliver(t, store, storyID, invitation)

	if messages := messageTestMessages(t, store, storyID, "guardsman"); len(messages) != 1 {
		t.Fatalf("the letter was delivered %d times, want 1: %+v", len(messages), messages)
	}
}

// TestAMessageSubmittedThroughTheWindowIsKept is the save guarantee for the path
// the character window writes through. A message submitted beside a window
// conversation must reach the journal: the caller is handed the projection, so a
// record that dropped it would show the letter to the caller and lose it from the
// story.
func TestAMessageSubmittedThroughTheWindowIsKept(t *testing.T) {
	store, storyID := characterTestStory(t)
	request := AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 酉时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
		Profiles: characterTestProfiles(),
		Facts: character.TurnCharacterFacts{
			Messages: []character.MessageDraft{{
				CharacterID: "guardsman", Text: "今夜北门有变，你若要走，趁早。",
				Basis: "主角今夜在城里落脚", Deadline: "三日内",
			}},
		},
		CommandID: "cmd-window", CommandHash: "sha256:window",
	}
	projected, err := store.AppendCharacterFacts(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Messages) != 1 {
		t.Fatalf("the projection did not produce the message: %+v", projected.Messages)
	}
	stored := messageTestMessages(t, store, storyID, "guardsman")
	if len(stored) != 1 || stored[0].Text != "今夜北门有变，你若要走，趁早。" {
		t.Fatalf("the message was projected but not kept: %+v", stored)
	}
	// A retry under the same command returns the same message rather than a second.
	replayed, err := store.AppendCharacterFacts(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed.Messages) != 1 || replayed.Messages[0].ID != projected.Messages[0].ID {
		t.Fatalf("the retry did not return the committed message: %+v", replayed.Messages)
	}
	if stored := messageTestMessages(t, store, storyID, "guardsman"); len(stored) != 1 {
		t.Fatalf("the retry delivered the message twice: %+v", stored)
	}
}

// TestAMessageAndItsStatusSurviveAReopen is the save rule. The letter, the hour
// it was written at and what the player did with it all come back from the
// journal alone.
func TestAMessageAndItsStatusSurviveAReopen(t *testing.T) {
	dir := t.TempDir()
	system := combatTestSystem()
	calendar := DefaultStoryCalendar()
	store := NewStore(dir)
	story, err := store.CreateStory(CreateStoryRequest{Title: "重开", ActorState: &system, Calendar: &calendar})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角在山洞中停下。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	messageTestRegister(t, store, story.ID)
	messageTestDeliver(t, store, story.ID, character.Message{
		ID: "msg-invite", CharacterID: "guardsman",
		Text:  "三日后北门换防，我在门内等你，过时不候。",
		Basis: "主角此前与他约定在北门再会", WorldTime: "第3日 酉时", Deadline: "三日内",
	})
	messageTestRest(t, store, story.ID, 5*24*60*60, "cmd-rest-1")
	if _, err := store.MarkCharacterMessage(story.ID, "main", "guardsman", "msg-invite", character.MessageRead, "第8日 辰时"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := NewStore(dir)
	t.Cleanup(func() { _ = reopened.Close() })
	messages := messageTestMessages(t, reopened, story.ID, "guardsman")
	if len(messages) != 1 {
		t.Fatalf("the letter did not survive the restart: %+v", messages)
	}
	kept := messages[0]
	if kept.WorldTime != "第3日 酉时" || kept.Deadline != "三日内" || kept.Status != character.MessageRead {
		t.Fatalf("the reopened record lost something: %+v", kept)
	}
	clock := SnapshotClockSeconds(mustSnapshot(t, reopened, story.ID))
	if !kept.LapsedAt("第8日", clock) {
		t.Fatalf("the reopened store must still read the hour as passed: %+v", kept)
	}
}

// TestAForkBeforeTheMessageDoesNotInheritIt is the branch half of the acceptance
// list. A fork cut at the turn before the letter was written never saw it, and
// the fork does not touch main's own copy.
//
// The letter is written between two turns, so the turn it must be excluded from
// is not the branch's latest one. Forking the latest turn is the other case and
// deliberately inherits the window: a creator revising after the last turn keeps
// what the window settled. What must not happen is a fork of an older turn
// carrying a letter that arrived after it.
func TestAForkBeforeTheMessageDoesNotInheritIt(t *testing.T) {
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	system := combatTestSystem()
	calendar := DefaultStoryCalendar()
	story, err := store.CreateStory(CreateStoryRequest{Title: "分支", ActorState: &system, Calendar: &calendar})
	if err != nil {
		t.Fatal(err)
	}
	// The cast is registered first, so the fork below can inherit the person and
	// still have no letter. Without this the two would be indistinguishable and a
	// fork that dropped both would pass.
	messageTestRegister(t, store, story.ID)
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "进城", Narrative: "主角来到北门。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	})
	if err != nil {
		t.Fatal(err)
	}
	messageTestDeliver(t, store, story.ID, character.Message{
		ID: "msg-invite", CharacterID: "guardsman",
		Text:  "三日后北门换防，我在门内等你，过时不候。",
		Basis: "主角此前与他约定在北门再会", WorldTime: "第3日 酉时", Deadline: "三日内",
	})
	// One more turn, so the turn below is no longer the branch's latest and the
	// fork is the older-turn case rather than the live-window one.
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "我在城里等消息。", Narrative: "你在客栈住了下来。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}

	before, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "来讯之前"})
	if err != nil {
		t.Fatal(err)
	}
	beforeState, err := store.CharacterState(story.ID, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(beforeState.MessagesFor("guardsman")) != 0 {
		t.Fatalf("a fork cut before the letter inherited it: %+v", beforeState.MessagesFor("guardsman"))
	}
	// The cast is still there: the fork predates the letter, not the person.
	if len(beforeState.CharacterIDs()) == 0 {
		t.Fatal("the fork lost the cast it forked from")
	}

	// Main keeps its own copy, and the fork's silence did not reach it.
	if messages := messageTestMessages(t, store, story.ID, "guardsman"); len(messages) != 1 {
		t.Fatalf("the fork changed main's letters: %+v", messages)
	}
}

// mustSnapshot reads the branch snapshot the tests measure the clock through.
func mustSnapshot(t *testing.T, store *Store, storyID string) Snapshot {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
