package interactive

import (
	"strings"
	"testing"

	"ta/internal/character"
)

func characterTestStory(t *testing.T) (*Store, string) {
	t.Helper()
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "人物记录"})
	if err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

func characterTestProfiles() []character.Profile {
	return []character.Profile{
		{ID: "guardsman", Name: "陈九", KnownAs: "北门守卒", Role: "北门守卒", Revision: 1,
			Personality: []string{"谨慎"}, SpeechStyle: "短句"},
		{ID: "steward", Name: "管事", Role: "驿馆管事", Revision: 1},
	}
}

// TestCharacterFactsSurviveAJournalRebuild is the single-save guarantee: memory
// is a projection of the story journal, so closing and reopening the store
// reproduces it rather than losing it.
func TestCharacterFactsSurviveAJournalRebuild(t *testing.T) {
	store, storyID := characterTestStory(t)
	projected, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Location: "北门", Present: []string{"guardsman"}},
		Profiles: characterTestProfiles(),
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{
				{CharacterID: "guardsman", Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, Text: "文牒。"},
			},
			Knowledge: []character.KnowledgeDraft{
				{CharacterID: "guardsman", Statement: "主角要进城寻人。", Nature: character.NatureTold,
					Audience: character.AudienceSingle, LearnedFrom: "protagonist"},
			},
			Matters: []character.MatterDraft{
				{CharacterID: "guardsman", Kind: character.MatterPermission, Status: character.MatterOpen,
					Statement: "守卒允许主角今夜入城。"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Exchanges) != 1 || len(projected.Knowledge) != 1 || len(projected.Matters) != 1 {
		t.Fatalf("projection produced %+v", projected)
	}

	// A fresh store over the same directory reads the same journal.
	reopened := NewStore(store.root)
	state, err := reopened.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.CharacterIDs()) != 2 {
		t.Fatalf("characters after reopen = %v, want two", state.CharacterIDs())
	}
	profile, ok := state.Profile("guardsman")
	if !ok || profile.KnownAs != "北门守卒" {
		t.Fatalf("profile after reopen = %+v ok=%v", profile, ok)
	}
	if len(state.Exchanges) != 1 || state.Exchanges[0].Text != "文牒。" {
		t.Fatalf("exchanges after reopen = %+v", state.Exchanges)
	}
	if len(state.Knowledge) != 1 || len(state.Matters) != 1 {
		t.Fatalf("memory after reopen: knowledge=%d matters=%d", len(state.Knowledge), len(state.Matters))
	}
	if open := character.OpenMatters("guardsman", state.Matters); len(open) != 1 {
		t.Fatalf("open matters after reopen = %+v", open)
	}
}

// TestCharacterRecordsDoNotConsumeStoryTurns keeps the window from advancing the
// story. A conversation is a side record: it changes what the next turn knows
// without becoming a turn itself.
func TestCharacterRecordsDoNotConsumeStoryTurns(t *testing.T) {
	store, storyID := characterTestStory(t)
	before, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3; index++ {
		if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
			BranchID: "main", WorldTime: "第3日 辰时",
			Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
			Profiles: characterTestProfiles(),
			Facts: character.TurnCharacterFacts{
				Exchanges: []character.ExchangeDraft{{CharacterID: "guardsman", Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate, Text: "在吗"}},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	after, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.TurnCount != before.TurnCount {
		t.Fatalf("character records advanced the turn count: %d -> %d", before.TurnCount, after.TurnCount)
	}
	if len(after.Turns) != len(before.Turns) {
		t.Fatalf("character records created story turns: %d -> %d", len(before.Turns), len(after.Turns))
	}
	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Exchanges) != 3 {
		t.Fatalf("window exchanges recorded = %d, want 3", len(state.Exchanges))
	}
	for _, exchange := range state.Exchanges {
		if exchange.TurnID != "" {
			t.Fatalf("a window exchange claimed a story turn: %+v", exchange)
		}
	}
}

// TestCharacterMemoryIsBranchScoped is the isolation requirement: two branches
// of one story must not share conversations, knowledge or unfinished business.
func TestCharacterMemoryIsBranchScoped(t *testing.T) {
	store, storyID := characterTestStory(t)
	turn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "进城", Narrative: "主角来到北门。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", TurnID: turn.ID, WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
		Profiles: characterTestProfiles(),
		Facts: character.TurnCharacterFacts{
			Matters: []character.MatterDraft{{CharacterID: "guardsman", Kind: character.MatterPromise, Status: character.MatterOpen, Statement: "守卒答应通传。"}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: turn.ID, Title: "支线"})
	if err != nil {
		t.Fatal(err)
	}

	mainState, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	forkState, err := store.CharacterState(storyID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The fork inherits what the parent knew at that turn.
	if len(forkState.CharacterIDs()) != 2 {
		t.Fatalf("fork did not inherit the cast: %v", forkState.CharacterIDs())
	}
	if len(character.OpenMatters("guardsman", forkState.Matters)) != 1 {
		t.Fatalf("fork did not inherit the open matter: %+v", forkState.Matters)
	}

	// A conversation held on the fork must not appear on main.
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: branch.ID, WorldTime: "第4日",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{{CharacterID: "guardsman", Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate, Text: "支线专属的悄悄话"}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	mainAfter, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, exchange := range mainAfter.Exchanges {
		if exchange.Text == "支线专属的悄悄话" {
			t.Fatal("a fork's private conversation leaked into the main branch")
		}
	}
	forkAfter, err := store.CharacterState(storyID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, exchange := range forkAfter.Exchanges {
		if exchange.Text == "支线专属的悄悄话" {
			found = true
		}
	}
	if !found {
		t.Fatal("the fork did not record its own conversation")
	}
	_ = mainState
}

// TestCharacterMessagesKeepTheirOriginalTime covers the incoming-message rules:
// a message is caused rather than scheduled, and its send time is frozen so
// reading it later still reports when it was sent.
func TestCharacterMessagesKeepTheirOriginalTime(t *testing.T) {
	store, storyID := characterTestStory(t)
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 酉时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
		Profiles: characterTestProfiles(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendCharacterMessages(storyID, "main", []character.Message{{
		ID: "msg-1", CharacterID: "guardsman", Text: "今夜北门有变。",
		Basis: "主角与他有未了的约定", WorldTime: "第3日 酉时", Deadline: "第6日",
	}}); err != nil {
		t.Fatal(err)
	}
	// A message with no cause is refused rather than delivered.
	if err := store.AppendCharacterMessages(storyID, "main", []character.Message{{
		ID: "msg-2", CharacterID: "guardsman", Text: "在吗", WorldTime: "第3日 酉时",
	}}); err == nil {
		t.Fatal("a message with no stated basis was delivered")
	}

	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	messages := state.MessagesFor("guardsman")
	if len(messages) != 1 || messages[0].WorldTime != "第3日 酉时" {
		t.Fatalf("messages = %+v", messages)
	}
	if messages[0].Lapsed("第4日") {
		t.Fatal("a message lapsed before its deadline")
	}
	if !messages[0].Lapsed("第7日") {
		t.Fatal("a message did not lapse after its deadline")
	}
	// Reading advances nothing in the world.
	if err := state.MarkMessage("msg-1", character.MessageRead, "第4日"); err != nil {
		t.Fatal(err)
	}
	if state.Messages[0].WorldTime != "第3日 酉时" {
		t.Fatalf("reading rewrote the send time: %q", state.Messages[0].WorldTime)
	}
}

// TestCharacterAppendIsIdempotentOnCommandID is the no-duplicate guarantee for
// the window: a repeated click, a lost response or a recovery re-read carries
// the same command ID, and the second attempt must return what the first one
// committed rather than writing the exchange twice.
func TestCharacterAppendIsIdempotentOnCommandID(t *testing.T) {
	store, storyID := characterTestStory(t)
	request := AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
		Profiles: characterTestProfiles(),
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{
				{CharacterID: "guardsman", Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate, Text: "今夜能进城吗"},
				{CharacterID: "guardsman", Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate, Text: "看你的文牒。"},
			},
		},
		CommandID: "cmd-1", CommandHash: "sha256:aaa",
	}
	first, err := store.AppendCharacterFacts(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AppendCharacterFacts(storyID, request)
	if err != nil {
		t.Fatalf("a retry under the same command ID failed: %v", err)
	}
	if len(second.Exchanges) != len(first.Exchanges) {
		t.Fatalf("retry returned %d exchanges, want %d", len(second.Exchanges), len(first.Exchanges))
	}
	for index := range first.Exchanges {
		if second.Exchanges[index].ID != first.Exchanges[index].ID {
			t.Fatalf("retry minted a new exchange: %q then %q", first.Exchanges[index].ID, second.Exchanges[index].ID)
		}
	}

	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Exchanges) != 2 {
		t.Fatalf("the journal holds %d exchanges after a retry, want 2", len(state.Exchanges))
	}
	// The replay read is what the app layer checks before generating, so it must
	// find the same records.
	replayed, found, err := store.CharacterExchangesForCommand(storyID, "main", "cmd-1", "sha256:aaa")
	if err != nil || !found {
		t.Fatalf("command lookup: found=%v err=%v", found, err)
	}
	if len(replayed.Exchanges) != 2 {
		t.Fatalf("command lookup returned %d exchanges", len(replayed.Exchanges))
	}
}

// TestCharacterCommandIDIsDurableAcrossRestart is the recovery case: the retry
// usually arrives at a different process than the one that committed, so the
// answer has to come from the journal rather than from memory.
func TestCharacterCommandIDIsDurableAcrossRestart(t *testing.T) {
	store, storyID := characterTestStory(t)
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
		Profiles: characterTestProfiles(),
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{{CharacterID: "guardsman", Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate, Text: "文牒。"}},
		},
		CommandID: "cmd-restart", CommandHash: "sha256:bbb",
	}); err != nil {
		t.Fatal(err)
	}

	restarted := NewStore(store.root)
	committed, found, err := restarted.CharacterExchangesForCommand(storyID, "main", "cmd-restart", "sha256:bbb")
	if err != nil {
		t.Fatal(err)
	}
	if !found || len(committed.Exchanges) != 1 || committed.Exchanges[0].Text != "文牒。" {
		t.Fatalf("after restart: found=%v exchanges=%+v", found, committed.Exchanges)
	}
	// Replaying through the write path must also be a read, not a second write.
	if _, err := restarted.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{{CharacterID: "guardsman", Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate, Text: "文牒。"}},
		},
		CommandID: "cmd-restart", CommandHash: "sha256:bbb",
	}); err != nil {
		t.Fatal(err)
	}
	state, err := restarted.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Exchanges) != 1 {
		t.Fatalf("a post-restart retry duplicated the exchange: %d records", len(state.Exchanges))
	}
}

// TestCharacterCommandIDConflictIsRefused stops one identity from standing for
// two different actions. Silently replaying the wrong reply would be worse than
// an error the caller can see.
func TestCharacterCommandIDConflictIsRefused(t *testing.T) {
	store, storyID := characterTestStory(t)
	base := AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
		Profiles: characterTestProfiles(),
		CommandID: "cmd-2",
	}
	base.Facts = character.TurnCharacterFacts{
		Exchanges: []character.ExchangeDraft{{CharacterID: "guardsman", Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate, Text: "第一次问的话"}},
	}
	base.CommandHash = "sha256:first"
	if _, err := store.AppendCharacterFacts(storyID, base); err != nil {
		t.Fatal(err)
	}

	reused := base
	reused.CommandHash = "sha256:second"
	reused.Facts = character.TurnCharacterFacts{
		Exchanges: []character.ExchangeDraft{{CharacterID: "guardsman", Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate, Text: "完全不同的一句话"}},
	}
	if _, err := store.AppendCharacterFacts(storyID, reused); err == nil {
		t.Fatal("one command ID was accepted for two different actions")
	}
	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Exchanges) != 1 || state.Exchanges[0].Text != "第一次问的话" {
		t.Fatalf("the conflicting write landed: %+v", state.Exchanges)
	}
}

// TestCharacterAppendRejectsUnknownCharacter keeps identity honest across the
// store boundary: a character must be registered before they can speak.
func TestCharacterAppendRejectsUnknownCharacter(t *testing.T) {
	store, storyID := characterTestStory(t)
	_, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene: character.Scene{NodeID: "gate", Present: []string{"stranger"}},
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{{CharacterID: "stranger", Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, Text: "…"}},
		},
	})
	if err == nil {
		t.Fatal("an unregistered character was allowed to speak")
	}
	state, stateErr := store.CharacterState(storyID, "main")
	if stateErr != nil {
		t.Fatal(stateErr)
	}
	if len(state.Exchanges) != 0 {
		t.Fatalf("a rejected record was written anyway: %+v", state.Exchanges)
	}
}

// TestCharacterMessageLifecycleDeliversOnceAndNeverAccepts is the whole inbound
// message path driven through the real store rather than left to whatever the
// model happens to volunteer on an acceptance run.
//
// It constructs the invitation the design describes — a formal appointment with
// contact conditions and an explicit deadline — delivers it, and then walks
// every state the player can put it in: unread, read, ignored, handled. Each
// step is checked for the two properties the whole feature rests on: the send
// time is frozen at commit and never rewritten, and no player action on the
// message ever accepts what it proposed. A message that has lapsed must keep its
// original time and its basis while no longer being executable.
func TestCharacterMessageLifecycleDeliversOnceAndNeverAccepts(t *testing.T) {
	store, storyID := characterTestStory(t)
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 酉时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
		Profiles: characterTestProfiles(),
	}); err != nil {
		t.Fatal(err)
	}

	// A formal appointment: who, where, and by when. The deadline is the part a
	// later world time has to be able to expire.
	invitation := character.Message{
		ID: "msg-invite", CharacterID: "guardsman",
		Text:  "三日后北门换防，我在门内等你，过时不候。",
		Basis: "主角此前与他约定在北门再会", WorldTime: "第3日 酉时", Deadline: "第6日",
	}
	if err := store.AppendCharacterMessages(storyID, "main", []character.Message{invitation}); err != nil {
		t.Fatal(err)
	}
	// The same delivery replayed — a retried commit or a recovery — is one record.
	if err := store.AppendCharacterMessages(storyID, "main", []character.Message{invitation}); err != nil {
		t.Fatal(err)
	}
	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	messages := state.MessagesFor("guardsman")
	if len(messages) != 1 {
		t.Fatalf("the invitation was delivered %d times, want 1", len(messages))
	}
	delivered := messages[0]
	if delivered.Status != character.MessageUnread {
		t.Fatalf("a delivered message started %q, want unread", delivered.Status)
	}
	if delivered.WorldTime != "第3日 酉时" || delivered.Basis != invitation.Basis {
		t.Fatalf("delivery rewrote the message: %+v", delivered)
	}
	// Receiving it accepts nothing: an invitation is not an agreement, and no
	// matter may exist until the player actually replies.
	if len(state.Matters) != 0 {
		t.Fatalf("receiving a message created a matter on the player's behalf: %+v", state.Matters)
	}

	// Reading is the player's own state and advances nothing.
	if _, err := store.MarkCharacterMessage(storyID, "main", "guardsman", "msg-invite", character.MessageRead, "第4日 辰时"); err != nil {
		t.Fatal(err)
	}
	afterRead, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	read := afterRead.MessagesFor("guardsman")[0]
	if read.Status != character.MessageRead || read.WorldTime != "第3日 酉时" {
		t.Fatalf("reading changed more than the status: %+v", read)
	}
	if len(afterRead.Matters) != 0 {
		t.Fatalf("reading accepted the invitation: %+v", afterRead.Matters)
	}
	// A read message is still pending: the player has seen it, not answered it.
	if pending := character.PendingMessages(afterRead.Messages, "第4日 辰时"); len(pending) != 1 {
		t.Fatalf("a read but unanswered invitation left the pending list: %+v", pending)
	}

	// Ignoring is a player decision that leaves the message a real event. The
	// character still sent it and the record must still say so.
	if _, err := store.MarkCharacterMessage(storyID, "main", "guardsman", "msg-invite", character.MessageIgnored, "第4日 辰时"); err != nil {
		t.Fatal(err)
	}
	afterIgnore, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	ignored := afterIgnore.MessagesFor("guardsman")[0]
	if ignored.Status != character.MessageIgnored {
		t.Fatalf("ignoring did not stick: %+v", ignored)
	}
	if ignored.WorldTime != "第3日 酉时" || ignored.Text == "" || ignored.Basis == "" {
		t.Fatalf("ignoring erased the message rather than dismissing it: %+v", ignored)
	}
	if len(afterIgnore.Matters) != 0 {
		t.Fatalf("ignoring accepted the invitation: %+v", afterIgnore.Matters)
	}
	// An ignored message leaves the pending hint, since the player has dealt with
	// it by dismissing it.
	if pending := character.PendingMessages(afterIgnore.Messages, "第4日 辰时"); len(pending) != 0 {
		t.Fatalf("an ignored message still asked for attention: %+v", pending)
	}
	// The deadline is measured against the story's clock: still live on the
	// deadline, expired after it.
	if ignored.Lapsed("第6日") {
		t.Fatal("the invitation lapsed on its own deadline day")
	}
	if !ignored.Lapsed("第7日") {
		t.Fatal("the invitation did not lapse after its deadline")
	}

	// A restart reproduces the whole history, statuses included, from the journal
	// alone. This is what makes the player's own progress durable.
	reopened := NewStore(store.Root())
	rebuilt, err := reopened.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	rebuiltMessages := rebuilt.MessagesFor("guardsman")
	if len(rebuiltMessages) != 1 {
		t.Fatalf("the invitation did not survive a restart: %+v", rebuiltMessages)
	}
	if rebuiltMessages[0].Status != character.MessageIgnored || rebuiltMessages[0].WorldTime != "第3日 酉时" {
		t.Fatalf("restart lost the message state: %+v", rebuiltMessages[0])
	}

	// Replying is what actually accepts an arrangement, and it goes through the
	// committed exchange path rather than the message status. The reply commits a
	// matter; the message becomes handled.
	if _, err := reopened.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第7日 卯时",
		Scene: character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{
				{CharacterID: "guardsman", Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate, Text: "我来迟了，改日再约。"},
				{CharacterID: "guardsman", Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate, Text: "那就改日。"},
			},
			Matters: []character.MatterDraft{{
				CharacterID: "guardsman", Kind: "promise", Status: "open",
				Statement: "双方约定改日再约北门。", Counterparty: "protagonist", Deadline: "第12日",
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.MarkCharacterMessage(storyID, "main", "guardsman", "msg-invite", character.MessageHandled, "第7日 卯时"); err != nil {
		t.Fatal(err)
	}
	final, err := reopened.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := final.MessagesFor("guardsman")[0]; got.Status != character.MessageHandled || got.WorldTime != "第3日 酉时" {
		t.Fatalf("handling the message changed its recorded time: %+v", got)
	}
	if len(character.OpenMatters("guardsman", final.Matters)) != 1 {
		t.Fatalf("the reply did not commit an arrangement: %+v", final.Matters)
	}
}

// TestLapsedMatterLosesItsForceButNotItsHistory is the "a lapsed invitation
// cannot resume execution" rule at the level the model actually reads.
//
// A matter's deadline is the only thing that makes an old arrangement stop
// binding. If the prepared context lists it as unresolved without saying it has
// expired, the character is free to honour an invitation the world already let
// go. What is asserted here is the split: the lapsed thread is still present,
// because the character remembers it, but it is presented as no longer in force
// rather than under the heading that says to treat it as still binding.
func TestLapsedMatterLosesItsForceButNotItsHistory(t *testing.T) {
	matters := []character.Matter{{
		ID: "mat-1", StoryID: "story", BranchID: "main", CharacterID: "guardsman",
		Kind: character.MatterPromise, Status: character.MatterOpen,
		Statement: "守卒答应三日后在北门放主角入城。", Counterparty: "protagonist",
		Deadline: "第6日", WorldTime: "第3日 酉时", SourceEventID: "evt-1",
	}}
	profile := character.Profile{ID: "guardsman", Name: "陈九", Role: "北门守卒"}

	// Before the deadline the promise is live and carries its binding guidance.
	live := character.Prepare(character.PrepareInput{
		StoryID: "story", Profile: profile,
		Scene:    character.Scene{WorldTime: "第5日 辰时"},
		Matters:  matters,
		Revision: character.IdentityRevision{BranchID: "main"},
	})
	if !strings.Contains(live.Interactions, "守卒答应三日后在北门放主角入城。") {
		t.Fatalf("a live promise is missing from the prepared context: %q", live.Interactions)
	}
	if strings.Contains(live.Interactions, "Lapsed") {
		t.Fatalf("a live promise was presented as lapsed: %q", live.Interactions)
	}
	if !strings.Contains(live.Circumstances, "still in force") {
		t.Fatalf("a live promise was not marked as binding: %q", live.Circumstances)
	}

	// After it, the promise is still remembered but no longer binds.
	expired := character.Prepare(character.PrepareInput{
		StoryID: "story", Profile: profile,
		Scene:    character.Scene{WorldTime: "第9日 辰时"},
		Matters:  matters,
		Revision: character.IdentityRevision{BranchID: "main"},
	})
	if !strings.Contains(expired.Interactions, "守卒答应三日后在北门放主角入城。") {
		t.Fatalf("a lapsed promise vanished instead of being remembered: %q", expired.Interactions)
	}
	if !strings.Contains(expired.Interactions, "no longer in force") {
		t.Fatalf("a lapsed promise was not marked as spent: %q", expired.Interactions)
	}
	if strings.Contains(expired.Circumstances, "still in force") {
		t.Fatalf("a lapsed promise was still described as binding: %q", expired.Circumstances)
	}
	// The distinction has to reach the narrator too, not only the character.
	cast := character.CastMarkdown(character.StoryCast(
		character.State{Profiles: []character.Profile{profile}, Matters: matters},
		character.Scene{WorldTime: "第9日 辰时", Present: []string{"guardsman"}},
		"第9日 辰时",
	), nil, 0)
	if !strings.Contains(cast, "no longer bind") || !strings.Contains(cast, "lapsed at 第6日") {
		t.Fatalf("the narrator was not told the promise had lapsed: %q", cast)
	}
}

// TestCastBlockNeverMintsAProtagonistID is the root cause of a whole class of
// rejected turns. The cast block is the model's only source of character ids,
// and it rendered the protagonist's own lines as a speaker named "Protagonist".
// A model reading that block reasonably concludes that the protagonist is a
// character with a name to copy, and files the protagonist's half of a
// conversation under character_id "protagonist".
//
// That draft is then rejected — Project refuses a character nobody registered —
// and because the character module is validated as a whole, one such line
// discards every exchange, matter, belief and message the turn established. In
// the acceptance run this happened on 3 of 11 turns, and no turn ever committed
// a message.
func TestCastBlockNeverMintsAProtagonistID(t *testing.T) {
	profile := character.Profile{ID: "hejing", StoryID: "story", Name: "贺敬", Revision: 1}
	exchanges := []character.Exchange{
		{
			ID: "xch_1", StoryID: "story", BranchID: "main", CharacterID: "hejing",
			Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate,
			Text: "日落之前我都在。", WorldTime: "第3日 午后",
		},
		{
			ID: "xch_2", StoryID: "story", BranchID: "main", CharacterID: "hejing",
			Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate,
			Text: "名字对了，钱当场数。", WorldTime: "第3日 午后",
		},
	}
	cast := character.CastMarkdown(
		character.StoryCast(character.State{Profiles: []character.Profile{profile}},
			character.Scene{WorldTime: "第3日 午后", Present: []string{"hejing"}}, "第3日 午后"),
		exchanges, 0,
	)

	// The block must not offer a bare "Protagonist" as a speaker label: that is
	// the shape the model copied into character_id.
	if strings.Contains(cast, "] Protagonist:") {
		t.Fatalf("the cast block presents the protagonist as a speaker to name:\n%s", cast)
	}
	// The player's line must still be visible and attributable to the character
	// it was spoken to, or the conversation the model reads is missing half.
	if !strings.Contains(cast, "日落之前我都在。") {
		t.Fatalf("the protagonist's own line was dropped from the conversation:\n%s", cast)
	}
	if !strings.Contains(cast, "hejing") {
		t.Fatalf("the protagonist's line was not attributed to the character it was spoken to:\n%s", cast)
	}
	// And the rule has to be stated, because a model that never sees the trap
	// cannot avoid it.
	if !strings.Contains(cast, "has no character_id") {
		t.Fatalf("the cast block does not say the protagonist is not a character:\n%s", cast)
	}
}

// TestAffinitySurvivesARebuildAndSettlesOnce is F11's single-save guarantee at
// the layer that owns the journal. The value a character holds must be readable
// after closing and reopening the store, and the same fact committed twice must
// still count once — the projection that settles it has to see the branch's own
// history, not just this turn's drafts.
func TestAffinitySurvivesARebuildAndSettlesOnce(t *testing.T) {
	store, storyID := characterTestStory(t)
	request := AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
		Profiles: characterTestProfiles(),
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{
				{CharacterID: "guardsman", Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate, Text: "你娘的药我送去了。"},
				{CharacterID: "guardsman", Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate, Text: "……记下了。"},
			},
			Affinities: []character.AffinityDraft{{
				CharacterID: "guardsman", Kind: character.AffinityMeaningful, Delta: 6,
				Basis: "主角替他把药送到了家里",
			}},
		},
	}
	projected, err := store.AppendCharacterFacts(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Affinities) != 1 {
		t.Fatalf("affinity records = %d, want 1", len(projected.Affinities))
	}
	if projected.Affinities[0].After != 6 {
		t.Fatalf("affinity after = %d, want 6", projected.Affinities[0].After)
	}

	// A fresh store over the same journal reproduces the value.
	reopened := NewStore(store.root)
	state, err := reopened.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := state.AffinityFor("guardsman"); value != 6 {
		t.Fatalf("affinity after reopen = %d, want 6", value)
	}

	// The same fact again, in a later exchange, adds nothing. This is the rule
	// the design states as "不重复刷同一礼物".
	repeat := request
	repeat.WorldTime = "第4日"
	repeat.Facts = character.TurnCharacterFacts{
		Exchanges: []character.ExchangeDraft{
			{CharacterID: "guardsman", Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate, Text: "上次那药，你还满意吧？"},
		},
		Affinities: []character.AffinityDraft{{
			CharacterID: "guardsman", Kind: character.AffinityMeaningful, Delta: 6,
			Basis: "主角替他把药送到了家里",
		}},
	}
	again, err := reopened.AppendCharacterFacts(storyID, repeat)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Affinities) != 0 {
		t.Fatalf("a repeated fact was scored again: %+v", again.Affinities)
	}
	after, err := reopened.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := after.AffinityFor("guardsman"); value != 6 {
		t.Fatalf("affinity after a repeat = %d, want 6", value)
	}
}

// TestAffinityChangeReachesTheCharacterOwnContext is the design's "不同阶段的态度":
// the number has to be something the character behaves from, not a badge in the
// window. This reads the same prepared context the character model is given.
func TestAffinityChangeReachesTheCharacterOwnContext(t *testing.T) {
	store, storyID := characterTestStory(t)
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
		Profiles: characterTestProfiles(),
		Facts: character.TurnCharacterFacts{
			Affinities: []character.AffinityDraft{{
				CharacterID: "guardsman", Kind: character.AffinityMajor, Delta: 12,
				Basis: "主角在北门替他挡了一刀",
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := state.Profile("guardsman")
	if !ok {
		t.Fatal("the guardsman was not registered")
	}
	value, band := state.AffinityFor("guardsman")
	prepared := character.Prepare(character.PrepareInput{
		StoryID: storyID, Profile: profile,
		Revision:     character.IdentityRevision{BranchID: "main", ProfileRevision: profile.Revision},
		Scene:        character.Scene{NodeID: "north-gate", WorldTime: "第3日 辰时"},
		Affinity:     value,
		AffinityBand: band,
		Question:     "你为什么要替我挡那一刀？",
	})
	if !strings.Contains(prepared.Circumstances, "12") {
		t.Fatalf("the character's own context does not carry the value:\n%s", prepared.Circumstances)
	}
	if !strings.Contains(prepared.Circumstances, "not a command") {
		t.Fatalf("the value is presented without saying it is a reference:\n%s", prepared.Circumstances)
	}
}
