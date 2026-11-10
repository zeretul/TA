package interactive

import (
	"errors"
	"testing"

	"ta/internal/character"
	interactivestate "ta/internal/interactive/state"
)

func TestCommittedPlayerLetterIsARebuildableOpenMatter(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{
		Title: "驿路来信",
		PackBinding: &StoryPackBinding{
			PackID: "letter-world", Scope: PackScopeFromID("letter-world"),
			Gameplay: &StoryGameplayProfile{Communication: "letter", LetterDeliverySeconds: 24 * 60 * 60},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "初次见到驿丞", Narrative: "柳四守在驿站柜台后。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{
			Profiles: []TurnCharacterProfileInput{{CharacterID: "liu-si", Name: "柳四", Age: 42, Gender: "female"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "写信给柳四", Narrative: "你把信交给驿卒带走。",
		PlayerLetter: &PlayerLetterIntent{CharacterID: "liu-si", Body: "请替我留一间房。"},
		TurnResult:   &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	assertOpenLetter := func(reader *Store) {
		t.Helper()
		state, err := reader.CharacterState(story.ID, "main")
		if err != nil {
			t.Fatal(err)
		}
		if len(state.Matters) != 1 {
			t.Fatalf("letter matter count = %d, want one: %+v", len(state.Matters), state.Matters)
		}
		matter := state.Matters[0]
		if matter.CharacterID != "liu-si" || matter.Kind != character.MatterAction || matter.Status != character.MatterOpen || matter.Letter == nil {
			t.Fatalf("committed letter facts = %+v", matter)
		}
		if matter.Letter.Body != "请替我留一间房。" || matter.Letter.SentAtSeconds != 0 || matter.Letter.EarliestDeliverySeconds != 24*60*60 {
			t.Fatalf("letter clock/body = %+v", matter.Letter)
		}
	}
	assertOpenLetter(store)
	assertOpenLetter(NewStore(workspace))
	for _, bypass := range []struct {
		name  string
		facts *TurnCharacterFactsInput
	}{
		{
			name: "instant incoming reply",
			facts: &TurnCharacterFactsInput{Messages: []TurnCharacterMessageInput{{
				CharacterID: "liu-si", Text: "房间留好了。", Basis: "读到了来信。",
			}}},
		},
		{
			name: "remote private exchange",
			facts: &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{
				CharacterID: "liu-si", Speaker: "character", Channel: "private", Text: "房间留好了。",
			}}},
		},
	} {
		t.Run(bypass.name, func(t *testing.T) {
			_, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
				BranchID: "main", User: "查看是否回信", Narrative: "驿站暂时没有新消息。",
				TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: bypass.facts},
			})
			if err == nil {
				t.Fatal("letter-only story accepted an immediate remote reply")
			}
		})
	}
}

func TestExplicitWaitAfterPlayerLetterAdvancesFormalClock(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	story, err := store.CreateStory(CreateStoryRequest{
		Title: "等候回信",
		PackBinding: &StoryPackBinding{
			PackID: "letter-wait-world", Scope: PackScopeFromID("letter-wait-world"),
			Gameplay: &StoryGameplayProfile{
				StatusTemplate: "world-actions-v1", Growth: "none", Combat: "none",
				Communication: "letter", LetterDeliverySeconds: 24 * 60 * 60,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "遇见柳四", Narrative: "柳四守在驿站柜台后。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			StateUpdates: []interactivestate.Update{{
				Op: interactivestate.Replace, Path: "/story/当前时间", Value: "雾港清晨",
			}},
			CharacterFacts: &TurnCharacterFactsInput{
				Profiles: []TurnCharacterProfileInput{{CharacterID: "liu-si", Name: "柳四", Age: 42, Gender: "female"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sent, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "写信给柳四", Narrative: "你把信交给驿卒带走。",
		PlayerLetter: &PlayerLetterIntent{CharacterID: "liu-si", Body: "请替我留一间房。"},
		TurnResult:   &TurnResult{Choices: testTurnChoices()},
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeWait, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: sent.ID, Title: "等待前"})
	if err != nil {
		t.Fatal(err)
	}
	letter, err := playerLetterMatterForTest(t, store, story.ID, "main", "liu-si")
	if err != nil {
		t.Fatal(err)
	}
	if got := SnapshotClockSeconds(mustSnapshot(t, store, story.ID)); got != 0 {
		t.Fatalf("sending a letter alone must not spend time: clock=%d", got)
	}
	identity := DomainCommitIdentity{CommandID: "wait-one-day", OperationID: "wait-run", Cycle: 1}
	playerInput, err := NewPlayerInputIntent(identity, "main", "我在驿站等一天")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, playerInput); err != nil {
		t.Fatal(err)
	}
	wait := AppendTurnWithStateRequest{
		BranchID: "main", User: playerInput.Text, Narrative: "早春次日近午，春汛水势暂缓。",
		AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle,
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			StateUpdates: []interactivestate.Update{{
				Op: interactivestate.Replace, Path: "/story/当前时间", Value: "雾港次日清晨",
			}},
			World: &TurnWorldActionInput{
				Action: TurnWorldActionWait, DurationSeconds: 24 * 60 * 60, SourceText: "等一天",
			},
		},
	}
	waited, _, err := store.AppendTurnWithState(story.ID, wait)
	if err != nil {
		t.Fatalf("commit the explicit one-day wait: %v", err)
	}
	if waited.ClockAdvance == nil || waited.ClockAdvance.DurationSeconds != 24*60*60 {
		t.Fatalf("wait turn did not persist its backend clock evidence: %+v", waited.ClockAdvance)
	}
	if waited.ClockAdvance.Before.Anchor != "雾港清晨" {
		t.Fatalf("wait clock origin must use the scene at turn start, got %q", waited.ClockAdvance.Before.Anchor)
	}
	retried, _, err := store.AppendTurnWithState(story.ID, wait)
	if err != nil || retried.ID != waited.ID {
		t.Fatalf("wait retry = %q, %v; want original turn %q", retried.ID, err, waited.ID)
	}
	if got := SnapshotClockSeconds(mustSnapshot(t, store, story.ID)); got != 24*60*60 {
		t.Fatalf("the next-day story turn must advance its formal clock by one day: got=%d want=%d", got, 24*60*60)
	}
	if clock, _, err := store.JourneyState(story.ID, "main"); err != nil || clock.Anchor != "雾港清晨" {
		t.Fatalf("wait clock anchor=%q error=%v, want the scene at turn start", clock.Anchor, err)
	}
	if _, _, err := store.JourneyState(story.ID, "main"); err != nil {
		t.Fatal(err)
	}
	if got := SnapshotClockSeconds(mustSnapshot(t, store, story.ID)); got != 24*60*60 {
		t.Fatalf("reading story time must not spend it: %d", got)
	}
	afterWait, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: waited.ID, Title: "等候一天后"})
	if err != nil {
		t.Fatal(err)
	}
	for _, branch := range []struct {
		id   string
		want int64
	}{{beforeWait.ID, 0}, {afterWait.ID, 24 * 60 * 60}} {
		clock, _, err := store.JourneyState(story.ID, branch.id)
		if err != nil || clock.Seconds != branch.want {
			t.Fatalf("branch %q clock=%+v error=%v, want %d", branch.id, clock, err, branch.want)
		}
	}
	replyIdentity := DomainCommitIdentity{CommandID: "letter-reply", OperationID: "reply-run", Cycle: 1}
	replyInput, err := NewPlayerInputIntent(replyIdentity, "main", "收取驿站回信")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, replyInput); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: replyInput.Text, Narrative: "驿丞将柳四的回信交给你。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{
			Messages: []TurnCharacterMessageInput{{CharacterID: "liu-si", Text: "房间留好了。", Basis: "已收到来信。"}},
		}},
	}); err == nil {
		t.Fatal("a letter-world incoming message bypassed the formal settled-matter reply path")
	}
	response := &TurnCharacterFactsInput{Matters: []TurnCharacterMatterInput{{
		CharacterID: "liu-si", Kind: string(character.MatterAction), Status: string(character.MatterSettled),
		Statement: "柳四收到了来信并回信，已替你留好房间。", Counterparty: DefaultActorID, Resolves: letter.ID,
	}}}
	reply := AppendTurnWithStateRequest{
		BranchID: "main", User: replyInput.Text, Narrative: "驿丞将柳四的回信交给你。",
		AgentCommandID: replyIdentity.CommandID, AgentOperationID: replyIdentity.OperationID, AgentCycle: replyIdentity.Cycle,
		TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: response},
	}
	if _, _, err := store.AppendTurnWithState(story.ID, reply); err != nil {
		t.Fatalf("commit a recipient reply after formal arrival: %v", err)
	}
	assertReply := func(reader *Store) {
		t.Helper()
		state, err := reader.CharacterState(story.ID, "main")
		if err != nil || len(state.Matters) != 2 {
			t.Fatalf("letter reply projection matters=%+v error=%v", state.Matters, err)
		}
		var delivered, answer *character.Matter
		for index := range state.Matters {
			matter := &state.Matters[index]
			if matter.Letter != nil {
				delivered = matter
			} else if matter.SettledBy == letter.ID {
				answer = matter
			}
		}
		if delivered == nil || delivered.Status != character.MatterSettled || delivered.SettledBy == "" {
			t.Fatalf("delivered original letter was not settled: %+v", delivered)
		}
		if answer == nil || answer.Statement != "柳四收到了来信并回信，已替你留好房间。" {
			t.Fatalf("recipient reply was not retained: %+v", answer)
		}
	}
	assertReply(store)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(dir)
	defer store.Close()
	assertReply(store)
	for _, branch := range []struct {
		id   string
		want int64
	}{{beforeWait.ID, 0}, {afterWait.ID, 24 * 60 * 60}} {
		clock, _, err := store.JourneyState(story.ID, branch.id)
		if err != nil || clock.Seconds != branch.want {
			t.Fatalf("reopened branch %q clock=%+v error=%v, want %d", branch.id, clock, err, branch.want)
		}
	}
	if got := SnapshotClockSeconds(mustSnapshot(t, store, story.ID)); got != 24*60*60 {
		t.Fatalf("cold reopen lost the wait interval or double-counted a retry: %d", got)
	}
}

func TestPlayerLetterMatterFollowsOnlyItsStoryBranch(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title: "驿路支线",
		PackBinding: &StoryPackBinding{
			PackID: "letter-branch-world", Scope: PackScopeFromID("letter-branch-world"),
			Gameplay: &StoryGameplayProfile{Communication: "letter", LetterDeliverySeconds: 24 * 60 * 60},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	introduced, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "遇见柳四", Narrative: "柳四守着驿站。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{
			Profiles: []TurnCharacterProfileInput{{CharacterID: "liu-si", Name: "柳四", Age: 42, Gender: "female"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeLetter, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: introduced.ID, Title: "未寄信"})
	if err != nil {
		t.Fatal(err)
	}
	sent, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "写信给柳四", Narrative: "信交给驿卒带走。",
		PlayerLetter: &PlayerLetterIntent{CharacterID: "liu-si", Body: "请替我留一间房。"},
		TurnResult:   &TurnResult{Choices: testTurnChoices()},
	})
	if err != nil {
		t.Fatal(err)
	}
	afterLetter, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: sent.ID, Title: "信已寄出"})
	if err != nil {
		t.Fatal(err)
	}
	for _, branch := range []struct {
		id   string
		want int
	}{{"main", 1}, {beforeLetter.ID, 0}, {afterLetter.ID, 1}} {
		state, err := store.CharacterState(story.ID, branch.id)
		if err != nil {
			t.Fatal(err)
		}
		letters := 0
		for _, matter := range state.Matters {
			if matter.Letter != nil {
				letters++
			}
		}
		if letters != branch.want {
			t.Errorf("branch %q has %d letter matters, want %d", branch.id, letters, branch.want)
		}
	}
}

func TestLetterCanOnlySettleForItsRecipientAfterItsFormalArrival(t *testing.T) {
	existing := []character.Matter{{
		ID: "mat_letter", CharacterID: "liu-si", Kind: character.MatterAction, Status: character.MatterOpen,
		Letter: &character.LetterDelivery{Body: "留房", SentAtSeconds: 10, EarliestDeliverySeconds: 100},
	}}
	newSettlement := func(characterID string) *TurnCharacterFactsInput {
		return &TurnCharacterFactsInput{Matters: []TurnCharacterMatterInput{{
			CharacterID: characterID, Kind: string(character.MatterAction), Status: string(character.MatterSettled),
			Statement: "驿丞读到来信并留了房。", Resolves: "mat_letter",
		}}}
	}
	if diagnostic := validateLetterMatterSettlements(newSettlement("liu-si"), existing, 99); diagnostic == nil {
		t.Fatal("the letter was settled before its arrival time")
	}
	if diagnostic := validateLetterMatterSettlements(newSettlement("someone-else"), existing, 100); diagnostic == nil {
		t.Fatal("a different character settled the letter")
	}
	if diagnostic := validateLetterMatterSettlements(newSettlement("liu-si"), existing, 100); diagnostic != nil {
		t.Fatalf("the original recipient could not settle an arrived letter: %+v", diagnostic)
	}
	duplicate := newSettlement("liu-si")
	duplicate.Matters = append(duplicate.Matters, duplicate.Matters[0])
	if diagnostic := validateLetterMatterSettlements(duplicate, existing, 100); diagnostic == nil {
		t.Fatal("two settlements for one letter were accepted in the same turn")
	}
}

func TestLetterIntentParticipatesInStoreCommitIdentity(t *testing.T) {
	base := AppendTurnWithStateRequest{BranchID: "main", User: "写信", Narrative: "你交出信。"}
	first := base
	first.PlayerLetter = &PlayerLetterIntent{CharacterID: "liu-si", Body: "留下房间。"}
	second := base
	second.PlayerLetter = &PlayerLetterIntent{CharacterID: "liu-si", Body: "取消房间。"}
	firstHash, err := agentTurnRequestHash(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := agentTurnRequestHash(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash == secondHash {
		t.Fatal("changing the letter body left the store commit identity unchanged")
	}
}

func TestLetterWorldRejectsImmediateChannelsButAllowsSceneSpeech(t *testing.T) {
	for _, facts := range []*TurnCharacterFactsInput{
		{Exchanges: []TurnCharacterExchangeInput{{CharacterID: "liu-si", Speaker: "character", Channel: "private", Text: "回信"}}},
		{Exchanges: []TurnCharacterExchangeInput{{CharacterID: "liu-si", Speaker: "character", Channel: "message", Text: "回信"}}},
		{Messages: []TurnCharacterMessageInput{{CharacterID: "liu-si", Text: "回信", Basis: "已经读信"}}},
	} {
		if diagnostic := validateLetterWorldCharacterChannels(facts, "letter"); diagnostic == nil {
			t.Fatalf("letter world accepted immediate channel facts: %+v", facts)
		}
	}
	if diagnostic := validateLetterWorldCharacterChannels(&TurnCharacterFactsInput{
		Exchanges: []TurnCharacterExchangeInput{{CharacterID: "liu-si", Speaker: "character", Channel: "scene", Text: "我看过你的信了。"}},
	}, "letter"); diagnostic != nil {
		t.Fatalf("people together could not speak in person: %+v", diagnostic)
	}
}

func TestPlayerLetterDeliveryReplySurvivesRetryReopenAndBranch(t *testing.T) {
	dir := t.TempDir()
	system := combatTestSystem()
	calendar := DefaultStoryCalendar()
	store := NewStore(dir)
	story, err := store.CreateStory(CreateStoryRequest{
		Title: "寄信与回信", ActorState: &system, Calendar: &calendar,
		PackBinding: &StoryPackBinding{
			PackID: "wuxia-letter", Scope: PackScopeFromID("wuxia-letter"),
			Gameplay: &StoryGameplayProfile{
				StatusTemplate: "world-actions-v1", Growth: "none", Combat: "none",
				Communication: "letter", LetterDeliverySeconds: 24 * 60 * 60,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	opening, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "在驿站遇见柳四", Narrative: "柳四守在驿站柜台后。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{
			Profiles: []TurnCharacterProfileInput{{CharacterID: "liu-si", Name: "柳四", Age: 42, Gender: "female"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeSend, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: opening.ID, Title: "寄信前"})
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "letter-send", OperationID: "letter-run", Cycle: 1}
	input, err := NewPlayerInputIntent(identity, "main", "请替我留一间房。")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, input); err != nil {
		t.Fatal(err)
	}
	send := AppendTurnWithStateRequest{
		BranchID: "main", User: input.Text, Narrative: "你把信交给驿卒带走。",
		AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle,
		PlayerLetter: &PlayerLetterIntent{CharacterID: "liu-si", Body: input.Text},
		TurnResult:   &TurnResult{Choices: testTurnChoices()},
	}
	beforeSendSnapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	expectedSentAt := SnapshotClockSeconds(beforeSendSnapshot)
	sent, _, err := store.AppendTurnWithState(story.ID, send)
	if err != nil {
		t.Fatal(err)
	}
	retried, _, err := store.AppendTurnWithState(story.ID, send)
	if err != nil || retried.ID != sent.ID {
		t.Fatalf("sending-letter retry = %q, %v; want original turn %q", retried.ID, err, sent.ID)
	}
	acceptanceBranch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: sent.ID, Title: "回信验收隔离支线"})
	if err != nil {
		t.Fatal(err)
	}
	letter, err := playerLetterMatterForTest(t, store, story.ID, acceptanceBranch.ID, "liu-si")
	if err != nil {
		t.Fatal(err)
	}
	if letter.Letter.SentAtSeconds != expectedSentAt || letter.Letter.EarliestDeliverySeconds != expectedSentAt+24*60*60 {
		t.Fatalf("letter delivery threshold was not stamped from the submitted branch clock: letter=%+v clock=%d", letter.Letter, expectedSentAt)
	}

	for _, bypass := range []*TurnCharacterFactsInput{
		{Messages: []TurnCharacterMessageInput{{CharacterID: "liu-si", Text: "房间留好了。", Basis: "读到了来信。"}}},
		{Exchanges: []TurnCharacterExchangeInput{{CharacterID: "liu-si", Speaker: "character", Channel: "private", Text: "房间留好了。"}}},
	} {
		_, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
			BranchID: acceptanceBranch.ID, User: "询问回信", Narrative: "你看了看驿站。",
			TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: bypass},
		})
		if err == nil {
			t.Fatal("an immediate message or remote private exchange bypassed letter delivery")
		}
	}
	early := &TurnCharacterFactsInput{Matters: []TurnCharacterMatterInput{{
		CharacterID: "liu-si", Kind: string(character.MatterAction), Status: string(character.MatterSettled),
		Statement: "柳四收到了来信，并回信说已留好房间。", Counterparty: DefaultActorID, Resolves: letter.ID,
	}}}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: acceptanceBranch.ID, User: "问驿站有没有回信", Narrative: "驿丞说信已收到。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: early},
	}); err == nil {
		t.Fatal("the letter was answered before formal time reached its arrival threshold")
	}
	waitIdentity := DomainCommitIdentity{CommandID: "letter-branch-wait-once", OperationID: "letter-branch-wait-run", Cycle: 1}
	waitInput, err := NewPlayerInputIntent(waitIdentity, acceptanceBranch.ID, "我在驿站等一天")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, waitInput); err != nil {
		t.Fatal(err)
	}
	wait := AppendTurnWithStateRequest{
		BranchID: acceptanceBranch.ID, User: waitInput.Text, Narrative: "一日过去，驿站终于等到送回的信。",
		AgentCommandID: waitIdentity.CommandID, AgentOperationID: waitIdentity.OperationID, AgentCycle: waitIdentity.Cycle,
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			World:   &TurnWorldActionInput{Action: TurnWorldActionWait, DurationSeconds: 24 * 60 * 60, SourceText: "等一天"},
		},
	}
	waited, _, err := store.AppendTurnWithState(story.ID, wait)
	if err != nil {
		t.Fatalf("commit the first wait on a branch created through the store API: %v", err)
	}
	if got := SnapshotClockSeconds(mustSnapshotOnBranch(t, store, story.ID, acceptanceBranch.ID)); got != 24*60*60 {
		t.Fatalf("isolated wait branch clock=%d, want one day", got)
	}
	clock, _, err := store.JourneyState(story.ID, acceptanceBranch.ID)
	if err != nil || clock.Seconds != 24*60*60 {
		t.Fatalf("journey API clock=%+v error=%v, want the same one-day clock as the snapshot", clock, err)
	}
	if got := SnapshotClockSeconds(mustSnapshotOnBranch(t, store, story.ID, "main")); got != 0 {
		t.Fatalf("formal branch wait changed its parent branch clock to %d", got)
	}
	if retried, _, err := store.AppendTurnWithState(story.ID, wait); err != nil || retried.ID != waited.ID {
		t.Fatalf("isolated wait retry = %q, %v; want original turn %q", retried.ID, err, waited.ID)
	}
	for _, branchID := range []string{"main", beforeSend.ID} {
		if got := SnapshotClockSeconds(mustSnapshotOnBranch(t, store, story.ID, branchID)); got != 0 {
			t.Fatalf("wait leaked into sibling branch %q: clock=%d", branchID, got)
		}
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: acceptanceBranch.ID, User: "收取驿站回信", Narrative: "驿丞将柳四的回信交给你。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{
			Messages: []TurnCharacterMessageInput{{CharacterID: "liu-si", Text: "房间已替你留好。", Basis: "来信已送到。"}},
		}},
	}); err == nil {
		t.Fatal("a letter-world incoming message bypassed the formal settled-matter reply path after arrival")
	}
	arrivedSnapshot, err := store.Snapshot(story.ID, acceptanceBranch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := SnapshotClockSeconds(arrivedSnapshot); got < letter.Letter.EarliestDeliverySeconds {
		t.Fatalf("formal wait left clock %d below letter threshold %d", got, letter.Letter.EarliestDeliverySeconds)
	}
	response := &TurnCharacterFactsInput{Matters: []TurnCharacterMatterInput{{
		CharacterID: "liu-si", Kind: string(character.MatterAction), Status: string(character.MatterSettled),
		Statement: "柳四收到来信后回信：房间已替你留好。", Counterparty: DefaultActorID, Resolves: letter.ID,
		LetterReply: &character.LetterReply{Body: "房间已替你留好。"},
	}}}
	replyIdentity := DomainCommitIdentity{CommandID: "letter-reply", OperationID: "reply-run", Cycle: 1}
	replyInput, err := NewPlayerInputIntent(replyIdentity, acceptanceBranch.ID, "收取驿站回信")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, replyInput); err != nil {
		t.Fatal(err)
	}
	reply := AppendTurnWithStateRequest{
		BranchID: acceptanceBranch.ID, User: replyInput.Text, Narrative: "驿丞将柳四的回信交给你。",
		AgentCommandID: replyIdentity.CommandID, AgentOperationID: replyIdentity.OperationID, AgentCycle: replyIdentity.Cycle,
		TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: response},
	}
	replyTurn, _, err := store.AppendTurnWithState(story.ID, reply)
	if err != nil {
		t.Fatalf("commit the arrived letter and response: %v", err)
	}

	assertReplyState := func(reader *Store) {
		t.Helper()
		state, err := reader.CharacterState(story.ID, acceptanceBranch.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(state.Matters) != 2 {
			t.Fatalf("letter conversation matters = %+v", state.Matters)
		}
		var sentLetter, receivedReply *character.Matter
		for index := range state.Matters {
			matter := &state.Matters[index]
			if matter.Letter != nil {
				sentLetter = matter
			} else if matter.SettledBy != "" {
				receivedReply = matter
			}
		}
		if sentLetter == nil || sentLetter.Status != character.MatterSettled || sentLetter.SettledBy == "" {
			t.Fatalf("sent letter did not record delivery/settlement: %+v", sentLetter)
		}
		if receivedReply == nil || receivedReply.Statement != "柳四收到来信后回信：房间已替你留好。" || receivedReply.SettledBy != sentLetter.ID || receivedReply.LetterReply == nil || receivedReply.LetterReply.Body != "房间已替你留好。" {
			t.Fatalf("the reply is not a durable recipient matter: %+v", receivedReply)
		}
		beforeState, err := reader.CharacterState(story.ID, beforeSend.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(beforeState.Matters) != 0 {
			t.Fatalf("pre-letter branch inherited a later letter: %+v", beforeState.Matters)
		}
		parentState, err := reader.CharacterState(story.ID, "main")
		if err != nil {
			t.Fatal(err)
		}
		if len(parentState.Matters) != 1 || parentState.Matters[0].Status != character.MatterOpen {
			t.Fatalf("parent branch inherited a later reply: %+v", parentState.Matters)
		}
	}
	assertReplyState(store)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(dir)
	defer store.Close()
	assertReplyState(store)
	replayed, _, err := store.AppendTurnWithState(story.ID, reply)
	if err != nil || replayed.ID != replyTurn.ID {
		t.Fatalf("reply retry = %q, %v; want original turn %q", replayed.ID, err, replyTurn.ID)
	}
	state, err := store.CharacterState(story.ID, acceptanceBranch.ID)
	if err != nil || len(state.Matters) != 2 {
		t.Fatalf("reply retry duplicated or lost letter facts: matters=%+v error=%v", state.Matters, err)
	}
	if got := SnapshotClockSeconds(mustSnapshotOnBranch(t, store, story.ID, acceptanceBranch.ID)); got != 24*60*60 {
		t.Fatalf("cold reopen changed formal branch clock: %d", got)
	}
	clock, _, err = store.JourneyState(story.ID, acceptanceBranch.ID)
	if err != nil || clock.Seconds != SnapshotClockSeconds(mustSnapshotOnBranch(t, store, story.ID, acceptanceBranch.ID)) {
		t.Fatalf("cold reopen journey/snapshot clocks diverged: journey=%+v error=%v", clock, err)
	}
}

func mustSnapshotOnBranch(t *testing.T, store *Store, storyID, branchID string) Snapshot {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func playerLetterMatterForTest(t *testing.T, store *Store, storyID, branchID, characterID string) (character.Matter, error) {
	t.Helper()
	state, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return character.Matter{}, err
	}
	for _, matter := range state.Matters {
		if matter.CharacterID == characterID && matter.Letter != nil {
			return matter, nil
		}
	}
	return character.Matter{}, errors.New("committed letter matter is missing")
}
