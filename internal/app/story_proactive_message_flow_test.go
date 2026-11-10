package app

import (
	"testing"

	"ta/config"
	"ta/internal/character"
	"ta/internal/interactive"
)

// This file closes 第5项's remaining gap. The rules half already proves each
// piece of an incoming message — the legal channel, the frozen send time, the
// deadline arithmetic, the content-hashed identity — but those proofs live in
// three separate fixtures, and most of them write through
// AppendCharacterFacts, which production never calls. What a player actually
// meets is one turn whose prose says a character wrote, read back through the
// character window, after real world time has passed, retried, and reopened.
//
// So this is one chain, not a fourth isolated case: a person the story has met
// writes on their own initiative through the turn's own character_facts module;
// the window shows the message arriving on the 传音 channel because they are not
// in the scene; a seclusion runs past the hour they named; the window then reads
// it as lapsed while keeping every word; the same command identity replayed
// delivers nothing a second time; and a reopened store tells the same story.

const (
	proactiveMessageText      = "东畦的药苗我留了一份，三日内来取，过了就给别人了。"
	proactiveMessageBasis     = "主角今日替她查过东畦的病株"
	proactiveMessageDeadline  = "三日内"
	proactiveMessageWorldTime = "第3日 酉时"
)

// proactiveMessageIdentity is the command identity the message turn is
// committed under. Reusing it is what a retry looks like to the store.
func proactiveMessageIdentity() interactive.DomainCommitIdentity {
	return interactive.DomainCommitIdentity{
		CommandID:   "cmd-proactive-message",
		OperationID: "op-proactive-message",
		Cycle:       1,
	}
}

// sendProactiveMessage commits the turn in which 慕沛灵 writes. She has already
// been met in the duty turn, which is what makes the channel legal; this turn
// leaves her out of the scene, which is what makes it a 传音 rather than a line
// spoken in front of the protagonist.
func (f sectDuty) sendProactiveMessage(t *testing.T, identity interactive.DomainCommitIdentity) interactive.TurnEvent {
	t.Helper()
	const user = "我把东畦查完，回房歇下。"
	intent, err := interactive.NewPlayerInputIntent(identity, "main", user)
	if err != nil {
		t.Fatalf("stage the player input: %v", err)
	}
	if _, err := f.store.CommitPlayerInput(f.story.ID, intent); err != nil {
		t.Fatalf("commit the player input: %v", err)
	}
	turn, _, err := f.store.AppendTurnWithState(f.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: user,
		Narrative: "东畦的病株查完了，你把结果报给慕沛灵，回房歇下。夜里，一张传音符贴到窗上。",
		ActorOps: []interactive.ActorStateOp{
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前时间",
				Value: proactiveMessageWorldTime, Reason: "本回合确认的时间",
			},
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "在场角色",
				Value: []any{interactive.DefaultActorID}, Reason: "本回合确认的在场角色",
			},
		},
		TurnResult: &interactive.TurnResult{
			Choices: []string{"连夜去取", "等天亮了再说", "回一句问她急不急"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Messages: []interactive.TurnCharacterMessageInput{{
					CharacterID: "mu_peiling",
					Text:        proactiveMessageText,
					Basis:       proactiveMessageBasis,
					Deadline:    proactiveMessageDeadline,
				}},
			},
		},
		AgentCommandID:   identity.CommandID,
		AgentOperationID: identity.OperationID,
		AgentCycle:       identity.Cycle,
	})
	if err != nil {
		t.Fatalf("commit the turn that carries the message: %v", err)
	}
	return turn
}

// characterWindowApp is the app the window reads through. It is the same
// composition the server builds, minus the parts the window does not touch.
func characterWindowApp(f sectDuty) *App {
	return &App{
		workspace:   f.workspace,
		interactive: f.store,
		cfg:         &config.Config{DenovaDir: f.dataRoot, Workspace: f.workspace},
	}
}

// proactiveWindowMessage returns the one message the window reports from a
// character.
func proactiveWindowMessage(t *testing.T, window CharacterWindow, characterID string) CharacterWindowMessage {
	t.Helper()
	var found []CharacterWindowMessage
	for _, message := range window.Messages {
		if message.CharacterID == characterID {
			found = append(found, message)
		}
	}
	if len(found) != 1 {
		t.Fatalf("the window reports %d messages from %s, want exactly one: %+v", len(found), characterID, window.Messages)
	}
	return found[0]
}

// TestOneProactiveMessageArrivesLapsesAndSurvivesARetry is 第5项's chain from
// the player's side.
func TestOneProactiveMessageArrivesLapsesAndSurvivesARetry(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	identity := proactiveMessageIdentity()
	turn := fixture.sendProactiveMessage(t, identity)

	// --- 送达：the message is committed with the turn that caused it ----------
	state, err := fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	messages := state.MessagesFor("mu_peiling")
	if len(messages) != 1 {
		t.Fatalf("the turn's message did not reach the journal: %+v", state.Messages)
	}
	message := messages[0]
	if message.WorldTime != proactiveMessageWorldTime {
		t.Fatalf("send time = %q, want the turn's own %q", message.WorldTime, proactiveMessageWorldTime)
	}
	if message.Deadline != proactiveMessageDeadline {
		t.Fatalf("deadline = %q, want the words the character used", message.Deadline)
	}
	// The story froze a calendar at creation, so 三日内 is a real instant on the
	// branch's own clock rather than an unbound label.
	if message.DeadlineSeconds <= 0 {
		t.Fatalf("the deadline was not placed on the story clock: %+v", message)
	}
	if message.BasisTurnID != turn.ID {
		t.Fatalf("the message is not pinned to the turn that caused it: %+v", message)
	}
	if message.Status != character.MessageUnread {
		t.Fatalf("a delivered message starts %q, want unread", message.Status)
	}

	// --- 合法渠道：she is not in the scene, so the window calls it 传音 --------
	window, err := characterWindowApp(fixture).CharacterWindow(CharacterWindowRequest{
		StoryID: fixture.story.ID, BranchID: "main", CharacterID: "mu_peiling",
	})
	if err != nil {
		t.Fatalf("read the character window: %v", err)
	}
	var row CharacterWindowCharacter
	foundRow := false
	for _, candidate := range window.Characters {
		if candidate.ID == "mu_peiling" {
			row, foundRow = candidate, true
		}
	}
	if !foundRow {
		t.Fatalf("the window does not list the person who wrote: %+v", window.Characters)
	}
	if row.Present {
		t.Fatal("the sender is reported as standing in the scene, so the window would show a spoken line")
	}
	if row.Channel != string(character.ChannelMessage) {
		t.Fatalf("channel = %q, want %q for someone who is not present", row.Channel, character.ChannelMessage)
	}

	// --- 送达读回：still live, and the words are hers ------------------------
	arrived := proactiveWindowMessage(t, window, "mu_peiling")
	if arrived.Lapsed {
		t.Fatal("a message inside its hour is reported as lapsed")
	}
	if arrived.Text != proactiveMessageText || arrived.Basis != proactiveMessageBasis {
		t.Fatalf("the window rewrote the message: %+v", arrived)
	}
	if arrived.Name != "慕沛灵" {
		t.Fatalf("the window names the sender %q, want the person the story met", arrived.Name)
	}
	// Receiving it accepts nothing: the thread the duty opened is still open and
	// no new one appeared.
	if len(state.Matters) != 1 || state.Matters[0].Status != character.MatterOpen {
		t.Fatalf("the message settled a thread by arriving: %+v", state.Matters)
	}

	// --- 失效：five days of real world time run past 三日内 -------------------
	secludeFor(t, fixture, 5*24*60*60)

	lapsedWindow, err := characterWindowApp(fixture).CharacterWindow(CharacterWindowRequest{
		StoryID: fixture.story.ID, BranchID: "main", CharacterID: "mu_peiling",
	})
	if err != nil {
		t.Fatalf("read the window after the seclusion: %v", err)
	}
	lapsed := proactiveWindowMessage(t, lapsedWindow, "mu_peiling")
	if !lapsed.Lapsed {
		t.Fatalf("the hour the letter named passed and the window still offers it: %+v", lapsed)
	}
	// The words, the hour it was written and the limit it named all survive: a
	// lapsed letter is old news, not a deleted one.
	if lapsed.Text != proactiveMessageText {
		t.Fatalf("a lapsed letter lost its words: %q", lapsed.Text)
	}
	if lapsed.WorldTime != proactiveMessageWorldTime {
		t.Fatalf("a lapsed letter was re-stamped to %q, want its original %q", lapsed.WorldTime, proactiveMessageWorldTime)
	}
	if lapsed.Deadline != proactiveMessageDeadline {
		t.Fatalf("a lapsed letter lost the limit it named: %q", lapsed.Deadline)
	}
	if lapsed.Status != string(character.MessageUnread) {
		t.Fatalf("lapsing changed the message's status to %q; it is still something the player has not dealt with", lapsed.Status)
	}

	// --- 重试去重：the same command identity delivers nothing new ------------
	retried := fixture.sendProactiveMessage(t, identity)
	if retried.ID != turn.ID {
		t.Fatalf("a retry committed a second turn: %q vs %q", retried.ID, turn.ID)
	}
	retriedState, err := fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the registry after the retry: %v", err)
	}
	if got := retriedState.MessagesFor("mu_peiling"); len(got) != 1 {
		t.Fatalf("a retried turn delivered the message twice: %+v", got)
	}

	// --- 冷重开：the journal, not the process, holds all of it ----------------
	if err := fixture.store.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}
	reopenedStore := interactive.NewStore(fixture.workspace)
	t.Cleanup(func() { _ = reopenedStore.Close() })
	fixture.store = reopenedStore

	reopenedWindow, err := characterWindowApp(fixture).CharacterWindow(CharacterWindowRequest{
		StoryID: fixture.story.ID, BranchID: "main", CharacterID: "mu_peiling",
	})
	if err != nil {
		t.Fatalf("read the window after reopening: %v", err)
	}
	afterRestart := proactiveWindowMessage(t, reopenedWindow, "mu_peiling")
	if !afterRestart.Lapsed {
		t.Fatal("reopening forgot that the hour had passed")
	}
	if afterRestart.Text != proactiveMessageText ||
		afterRestart.WorldTime != proactiveMessageWorldTime ||
		afterRestart.Deadline != proactiveMessageDeadline ||
		afterRestart.Status != lapsed.Status {
		t.Fatalf("a restart changed the message: %+v", afterRestart)
	}
	// The pending list keeps it too: an unhandled letter is still unhandled after
	// a restart, and it is still marked as no longer offering anything.
	pending := 0
	for _, message := range reopenedWindow.Pending {
		if message.CharacterID == "mu_peiling" {
			pending++
			if !message.Lapsed {
				t.Fatalf("the reopened pending list offers a lapsed letter: %+v", message)
			}
		}
	}
	if pending != 1 {
		t.Fatalf("the reopened pending list holds %d letters from her, want 1", pending)
	}
}
