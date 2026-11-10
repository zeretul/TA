package interactive

import (
	"testing"

	"ta/internal/character"
	interactivestate "ta/internal/interactive/state"
)

// An arrangement carried into the next turn arrives as one message: the stage
// joins the settled words and the player's own main-line choice with a newline
// and sends them as a single request. This file is the backend half of that
// contract.
//
// What has to be true is not "the text survived" but "the turn is one turn". A
// message with two lines is still one player action: one Turn is appended, the
// formal clock advances once, and an arrangement the prose discharged is settled
// once. A retry after a lost response asks for the receipt it already has rather
// than spending the same interval and settling the same thread a second time.
//
// Every byte here is synthetic and lives in t.TempDir().

const (
	arrangementAgreed  = "天亮前我来渡口接你。"
	arrangementChoice  = "在船边等到天亮"
	arrangementMessage = arrangementAgreed + "\n" + arrangementChoice
)

// arrangementStore stands a story at a known scene time with one person already
// established and one arrangement already open between them, which is exactly
// the state the character window leaves behind when the player presses
// 加入下轮行动.
func arrangementStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "渡口约定"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "在渡口见到陈婆婆", Narrative: "陈婆婆守在石阶上，篮子搁在脚边。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			StateUpdates: []interactivestate.Update{{
				Op: interactivestate.Replace, Path: "/story/当前时间", Value: "卯时",
			}},
			CharacterFacts: &TurnCharacterFactsInput{
				Profiles: []TurnCharacterProfileInput{{
					CharacterID: "陈婆婆", Name: "陈婆婆", Age: 63, Gender: "female", Lifeform: "human",
				}},
				Matters: []TurnCharacterMatterInput{{
					CharacterID: "陈婆婆", Kind: string(character.MatterAction), Status: string(character.MatterOpen),
					Statement: "陈婆婆托沈舟天亮前来渡口接她，尚未得他应承。",
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	state, err := store.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Matters) != 1 {
		t.Fatalf("the arrangement must start as one open thread: %+v", state.Matters)
	}
	return store, story.ID, state.Matters[0].ID
}

// arrangementTurn commits the joined message as one player request: the
// arrangement's own words, then the main-line choice the player picked beside
// it. The wait is what makes "only once" measurable — it is the turn's one
// formal interval.
func arrangementTurn(t *testing.T, store *Store, storyID, commandID, agreedID string) AppendTurnWithStateRequest {
	t.Helper()
	identity := DomainCommitIdentity{CommandID: commandID, OperationID: commandID + "-run", Cycle: 1}
	input, err := NewPlayerInputIntent(identity, "main", arrangementMessage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(storyID, input); err != nil {
		t.Fatal(err)
	}
	return AppendTurnWithStateRequest{
		BranchID: "main", User: input.Text,
		Narrative:      "沈舟应下天亮前到渡口，转身把船绳在桩上又绕了一道，就在船边等着。",
		AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle,
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			World: &TurnWorldActionInput{
				Action: TurnWorldActionWait, DurationSeconds: 6 * 60 * 60, SourceText: arrangementChoice,
			},
			CharacterFacts: &TurnCharacterFactsInput{Matters: []TurnCharacterMatterInput{{
				CharacterID: "陈婆婆", Kind: string(character.MatterAction), Status: string(character.MatterSettled),
				Statement: "沈舟应下天亮前到渡口接陈婆婆。", Resolves: agreedID,
			}}},
		},
	}
}

func TestOneSendCarriesAnArrangementAndAChoiceAsOneTurn(t *testing.T) {
	store, storyID, agreedID := arrangementStore(t)
	before := mustSnapshot(t, store, storyID)
	beforeMatters := len(mustCharacterMatters(t, store, storyID))

	request := arrangementTurn(t, store, storyID, "arrangement-one", agreedID)
	turn, _, err := store.AppendTurnWithState(storyID, request)
	if err != nil {
		t.Fatalf("the arrangement and the main-line choice must commit as one turn: %v", err)
	}

	after := mustSnapshot(t, store, storyID)
	if after.TurnCount != before.TurnCount+1 {
		t.Fatalf("one send is one turn: before=%d after=%d", before.TurnCount, after.TurnCount)
	}
	// The player's own message is carried whole. A newline inside it is where the
	// stage joined the arrangement to the choice, not a second action.
	if turn.User != arrangementMessage {
		t.Fatalf("the turn did not carry the composed message verbatim: %q", turn.User)
	}
	if turn.ClockAdvance == nil || turn.ClockAdvance.DurationSeconds != 6*60*60 {
		t.Fatalf("the wait the choice asked for must be the turn's one interval: %+v", turn.ClockAdvance)
	}
	if got := SnapshotClockSeconds(after); got != 6*60*60 {
		t.Fatalf("the joined message spent formal time more than once: clock=%d", got)
	}

	// The arrangement the prose discharged is settled, and settling it added the
	// one record that says so rather than a second copy of the arrangement.
	matters := mustCharacterMatters(t, store, storyID)
	if len(matters) != beforeMatters+1 {
		t.Fatalf("settling the arrangement must add exactly one record: before=%d after=%d %+v",
			beforeMatters, len(matters), matters)
	}
	settled, ok := matterByID(matters, agreedID)
	if !ok {
		t.Fatalf("the arrangement the turn resolved is gone: %+v", matters)
	}
	if settled.Status != character.MatterSettled {
		t.Fatalf("the arrangement the prose discharged is still open: %+v", settled)
	}
	open := 0
	for _, matter := range matters {
		if matter.Status == character.MatterOpen {
			open++
		}
	}
	if open != 0 {
		t.Fatalf("one send must not leave a second open copy of the same arrangement: %+v", matters)
	}
}

func TestRetryingTheSameSendNeitherSpendsTimeNorSettlesAgain(t *testing.T) {
	store, storyID, agreedID := arrangementStore(t)
	request := arrangementTurn(t, store, storyID, "arrangement-retry", agreedID)
	first, _, err := store.AppendTurnWithState(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	afterFirst := mustSnapshot(t, store, storyID)
	mattersAfterFirst := mustCharacterMatters(t, store, storyID)

	// The same command arriving again is the client asking for the receipt it
	// lost, not a second action: a retry that duplicated the turn would spend the
	// same six hours twice and settle the arrangement twice.
	retried, _, err := store.AppendTurnWithState(storyID, request)
	if err != nil {
		t.Fatalf("a retry of an already committed send must return its receipt: %v", err)
	}
	if retried.ID != first.ID {
		t.Fatalf("the retry produced a second turn: first=%q retried=%q", first.ID, retried.ID)
	}
	afterRetry := mustSnapshot(t, store, storyID)
	if afterRetry.TurnCount != afterFirst.TurnCount {
		t.Fatalf("the retry appended another turn: before=%d after=%d", afterFirst.TurnCount, afterRetry.TurnCount)
	}
	if got := SnapshotClockSeconds(afterRetry); got != 6*60*60 {
		t.Fatalf("the retry spent the interval again: clock=%d", got)
	}
	if matters := mustCharacterMatters(t, store, storyID); len(matters) != len(mattersAfterFirst) {
		t.Fatalf("the retry settled the arrangement a second time: before=%d after=%d %+v",
			len(mattersAfterFirst), len(matters), matters)
	}

	// A cold reader replays the same journal, so the answer must not depend on the
	// process that wrote it.
	reopened := NewStore(store.Root())
	t.Cleanup(func() { _ = reopened.Close() })
	if got := SnapshotClockSeconds(mustSnapshot(t, reopened, storyID)); got != 6*60*60 {
		t.Fatalf("a cold replay of the same send read a different clock: %d", got)
	}
	if matters := mustCharacterMatters(t, reopened, storyID); len(matters) != len(mattersAfterFirst) {
		t.Fatalf("a cold replay read a different set of matters: %+v", matters)
	}
}

// TestTheSameCommandWithDifferentWordsIsRefused is the other half of retrying:
// reusing a command id for a different message is a different action, and
// accepting it would either lose the first turn or write the second under a
// false identity.
func TestTheSameCommandWithDifferentWordsIsRefused(t *testing.T) {
	store, storyID, agreedID := arrangementStore(t)
	request := arrangementTurn(t, store, storyID, "arrangement-conflict", agreedID)
	if _, _, err := store.AppendTurnWithState(storyID, request); err != nil {
		t.Fatal(err)
	}
	changed := request
	changed.Narrative = "沈舟改了主意，天亮前没有去渡口。"
	if _, _, err := store.AppendTurnWithState(storyID, changed); err == nil {
		t.Fatal("the same command id with different content was accepted as a retry")
	}
	if after := mustSnapshot(t, store, storyID); after.TurnCount != 2 {
		t.Fatalf("a refused conflicting retry still appended a turn: count=%d", after.TurnCount)
	}
}

func mustCharacterMatters(t *testing.T, store *Store, storyID string) []character.Matter {
	t.Helper()
	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return state.Matters
}

func matterByID(matters []character.Matter, id string) (character.Matter, bool) {
	for _, matter := range matters {
		if matter.ID == id {
			return matter, true
		}
	}
	return character.Matter{}, false
}
