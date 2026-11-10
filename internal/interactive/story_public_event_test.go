package interactive

import (
	"errors"
	"strings"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/publiquevent"
)

// These tests are 自制-事件与任务 §4.4, §4.5 and §6 as they happen during play: a
// public situation that several parties care about moves in stages with the
// branch's own committed time, the player can take part or stand aside, and what
// they are told depends on where they stood and when.
//
// The rules package already proves the stage arithmetic and that a secret never
// leaves it. What these tests add is everything the journal layer decides: the
// stage is read off the branch clock rather than stored, participation and its
// payment commit once, a fork cut earlier sees the earlier stage, and a character
// outside the region is not told about the event at all.

const publicEventTestMarket = "公共坊市"

// publicEventTestSystem places the protagonist in the market on the cultivation
// rules, so the branch has both a location the event is known in and real pools a
// long rest can move the clock with.
func publicEventTestSystem(location string) StoryDirectorActorStateSystem {
	system := xiuxianTestSystem()
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != DefaultStoryContextActorID {
			continue
		}
		if system.InitialActors[index].State == nil {
			system.InitialActors[index].State = map[string]any{}
		}
		system.InitialActors[index].State[storySceneLocationField] = location
	}
	return system
}

// publicEventTestCombatSystem is the same cast with a real realm, so the pools
// have maxima and a long rest can move the branch clock.
func publicEventTestCombatSystem(location string) StoryDirectorActorStateSystem {
	system := publicEventTestSystem(location)
	stage := cultivation.StageKey(cultivation.Qi(6))
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != DefaultActorID {
			continue
		}
		if system.InitialActors[index].State == nil {
			system.InitialActors[index].State = map[string]any{}
		}
		system.InitialActors[index].State[xiuxianFieldRealm] = stage
	}
	return system
}

// publicEventTestStory builds a story whose scene places the protagonist in the
// market, which is the one region the authored event happens in.
func publicEventTestStory(t *testing.T) (*Store, string) {
	t.Helper()
	return publicEventTestStoryAt(t, publicEventTestMarket)
}

// publicEventTestStoryAt builds the same story at an arbitrary registered place.
func publicEventTestStoryAt(t *testing.T, location string) (*Store, string) {
	t.Helper()
	return publicEventTestStoryOn(t, publicEventTestSystem(location))
}

// publicEventTestStoryOn builds the story on a caller-chosen cast.
func publicEventTestStoryOn(t *testing.T, system StoryDirectorActorStateSystem) (*Store, string) {
	t.Helper()
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "公共事件", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "打听", Narrative: "主角在坊市里听说太岳山那边有些动静。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// publicEventTestApply settles one participation action and fails the test if the
// store refuses it.
func publicEventTestApply(t *testing.T, store *Store, storyID string, req PublicEventRequest) PublicEventResult {
	t.Helper()
	if req.BranchID == "" {
		req.BranchID = "main"
	}
	if req.EventID == "" {
		req.EventID = publiquevent.PublicEventIDTaiyueTide
	}
	result, err := store.ApplyPublicEvent(storyID, req)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// publicEventTestState reads the entry's own view.
func publicEventTestState(t *testing.T, store *Store, storyID, branchID string) PublicEventState {
	t.Helper()
	if branchID == "" {
		branchID = "main"
	}
	state, err := store.PublicEventStateFor(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

// publicEventTestView finds the authored event in the entry's view.
func publicEventTestView(t *testing.T, state PublicEventState) PublicEventView {
	t.Helper()
	for _, event := range state.Events {
		if event.EventID == publiquevent.PublicEventIDTaiyueTide {
			return event
		}
	}
	t.Fatalf("the authored event must be known in the market: %#v", state.Events)
	return PublicEventView{}
}

// publicEventTestRecordCount counts the participation records on the journal.
func publicEventTestRecordCount(t *testing.T, store *Store, storyID string) int {
	t.Helper()
	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, record := range records {
		if record.Envelope.Type == StoryEventTypePublicEventRecord {
			count++
		}
	}
	return count
}

// publicEventTestCurrency reads the committed spirit-stone balance.
func publicEventTestCurrency(t *testing.T, store *Store, storyID string) int64 {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return int64(xiuxianStoredInt(snapshot.State, DefaultActorID, xiuxianFieldCurrency))
}

// TestTheStageComesFromTheClockNotFromAWrite is §4.4's separation as a fact
// about the journal: reading the entry moves nothing, and the stage the entry
// reports is the one the branch's committed instant puts the event in.
func TestTheStageComesFromTheClockNotFromAWrite(t *testing.T) {
	store, storyID := publicEventTestStory(t)

	before := publicEventTestState(t, store, storyID, "")
	view := publicEventTestView(t, before)
	// A branch that never spent formal time reports zero seconds, and the event's
	// first stage opens at zero, so the rumour is genuinely public from the start.
	if before.ClockSeconds != 0 {
		t.Fatalf("a fresh branch has spent no formal time: %d", before.ClockSeconds)
	}
	if view.CurrentStageID != "taiyue.rumor" {
		t.Fatalf("the first stage must be current at zero: %q", view.CurrentStageID)
	}
	if view.Closed {
		t.Fatal("the event cannot be closed before its final stage")
	}

	// Reading again must not have written anything or moved the clock.
	after := publicEventTestState(t, store, storyID, "")
	if after.ClockSeconds != before.ClockSeconds {
		t.Fatalf("opening the entry must not move the clock: %d -> %d", before.ClockSeconds, after.ClockSeconds)
	}
	if count := publicEventTestRecordCount(t, store, storyID); count != 0 {
		t.Fatalf("opening the entry must not write a record: %d", count)
	}
}

// TestJoiningRecordsTheStageTheClockWasIn is the participation's whole content:
// one record, the action that was really chosen, the stage that was really
// current, and the payment that action really promises.
func TestJoiningRecordsTheStageTheClockWasIn(t *testing.T) {
	store, storyID := publicEventTestStory(t)

	result := publicEventTestApply(t, store, storyID, PublicEventRequest{
		Action:              PublicEventActionJoin,
		ParticipationAction: string(publiquevent.ActionHelp),
		CommandID:           "cmd-join", CommandHash: "h-join",
	})
	if result.Instance.JoinedStage != "taiyue.rumor" {
		t.Fatalf("the join must record the stage the clock was in: %q", result.Instance.JoinedStage)
	}
	if result.Instance.Action != publiquevent.ActionHelp {
		t.Fatalf("the join must record the chosen action: %q", result.Instance.Action)
	}
	if count := publicEventTestRecordCount(t, store, storyID); count != 1 {
		t.Fatalf("one join is one record: %d", count)
	}

	view := publicEventTestView(t, publicEventTestState(t, store, storyID, ""))
	if !view.Joined || view.Participation != string(publiquevent.ActionHelp) {
		t.Fatalf("the entry must report the participation: %#v", view)
	}
	if view.JoinedStage != "taiyue.rumor" {
		t.Fatalf("the entry must report the stage it was made at: %q", view.JoinedStage)
	}
}

// TestTheHelpRewardIsSettledOnce is §4.7's single payment: the join and its
// purse write commit in one transaction, and a retry under the same command ID
// returns the first answer without paying again.
func TestTheHelpRewardIsSettledOnce(t *testing.T) {
	store, storyID := publicEventTestStory(t)
	before := publicEventTestCurrency(t, store, storyID)

	first := publicEventTestApply(t, store, storyID, PublicEventRequest{
		Action:              PublicEventActionJoin,
		ParticipationAction: string(publiquevent.ActionHelp),
		CommandID:           "cmd-help", CommandHash: "h-help",
	})
	definition, ok := publiquevent.DefinitionByID(publiquevent.PublicEventIDTaiyueTide)
	if !ok {
		t.Fatal("the authored event must be registered")
	}
	action, ok := definition.ActionByKind(publiquevent.ActionHelp)
	if !ok || action.Reward == nil {
		t.Fatal("helping must pay something, or this test proves nothing")
	}
	if len(first.Rewards) != 1 || first.Rewards[0].Quantity != action.Reward.Quantity {
		t.Fatalf("the join must report what it really pays: %#v", first.Rewards)
	}
	after := publicEventTestCurrency(t, store, storyID)
	if after != before+action.Reward.Quantity {
		t.Fatalf("the purse must move by the reward exactly once: %d -> %d", before, after)
	}

	retry, err := store.ApplyPublicEvent(storyID, PublicEventRequest{
		BranchID: "main", EventID: publiquevent.PublicEventIDTaiyueTide,
		Action:              PublicEventActionJoin,
		ParticipationAction: string(publiquevent.ActionHelp),
		CommandID:           "cmd-help", CommandHash: "h-help",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !retry.Replayed {
		t.Fatal("a retry under the same command must be answered from the first attempt")
	}
	if again := publicEventTestCurrency(t, store, storyID); again != after {
		t.Fatalf("a retry must not pay twice: %d -> %d", after, again)
	}
	if count := publicEventTestRecordCount(t, store, storyID); count != 1 {
		t.Fatalf("a retry must not add a record: %d", count)
	}
}

// TestASecondParticipationIsRefused keeps a participation a single fact.
func TestASecondParticipationIsRefused(t *testing.T) {
	store, storyID := publicEventTestStory(t)
	publicEventTestApply(t, store, storyID, PublicEventRequest{
		Action: PublicEventActionJoin, ParticipationAction: string(publiquevent.ActionObserve),
		CommandID: "cmd-first", CommandHash: "h-first",
	})

	_, err := store.ApplyPublicEvent(storyID, PublicEventRequest{
		BranchID: "main", EventID: publiquevent.PublicEventIDTaiyueTide,
		Action: PublicEventActionJoin, ParticipationAction: string(publiquevent.ActionInvestigate),
		CommandID: "cmd-second", CommandHash: "h-second",
	})
	if !errors.Is(err, ErrPublicEventAlreadyJoined) {
		t.Fatalf("a second participation must be refused: %v", err)
	}
	if count := publicEventTestRecordCount(t, store, storyID); count != 1 {
		t.Fatalf("a refused second participation must write nothing: %d", count)
	}
}

// TestWithdrawingKeepsTheParticipation is §4.5's "参与与公共局势分离": stepping
// back is a fact about the character, not a reset of the event.
func TestWithdrawingKeepsTheParticipation(t *testing.T) {
	store, storyID := publicEventTestStory(t)
	publicEventTestApply(t, store, storyID, PublicEventRequest{
		Action: PublicEventActionJoin, ParticipationAction: string(publiquevent.ActionHelp),
		CommandID: "cmd-join", CommandHash: "h-join",
	})

	withdrawn := publicEventTestApply(t, store, storyID, PublicEventRequest{
		Action: PublicEventActionWithdraw,
		CommandID: "cmd-withdraw", CommandHash: "h-withdraw",
	})
	if !withdrawn.Instance.Withdrawn {
		t.Fatal("a withdrawal must be recorded on the participation")
	}
	if len(withdrawn.Rewards) != 0 {
		t.Fatalf("a withdrawal pays nothing: %#v", withdrawn.Rewards)
	}
	view := publicEventTestView(t, publicEventTestState(t, store, storyID, ""))
	if !view.Joined || !view.Withdrawn {
		t.Fatalf("the entry must still show that they took part and then stepped back: %#v", view)
	}
	// The event itself did not change: the same stage is still current.
	if view.CurrentStageID != "taiyue.rumor" {
		t.Fatalf("a withdrawal must not move the event: %q", view.CurrentStageID)
	}
}

// TestWithdrawingAnEventNeverJoinedIsRefused keeps a withdrawal honest.
func TestWithdrawingAnEventNeverJoinedIsRefused(t *testing.T) {
	store, storyID := publicEventTestStory(t)

	_, err := store.ApplyPublicEvent(storyID, PublicEventRequest{
		BranchID: "main", EventID: publiquevent.PublicEventIDTaiyueTide,
		Action: PublicEventActionWithdraw, CommandID: "cmd-w", CommandHash: "h-w",
	})
	if !errors.Is(err, ErrPublicEventNotJoined) {
		t.Fatalf("a withdrawal without a participation must be refused: %v", err)
	}
}

// TestAnEventOutsideTheRegionIsNotKnown is §6's knowledge boundary: the event is
// really happening, but a character who is not in the market has not been told.
func TestAnEventOutsideTheRegionIsNotKnown(t *testing.T) {
	// The outskirts are a real registered place and not the market, so a story
	// that opens there is outside the event's region.
	store, storyID := publicEventTestStoryAt(t, "越国坊市外围散修人家")

	state := publicEventTestState(t, store, storyID, "")
	if len(state.Events) != 0 {
		t.Fatalf("a character outside the region must not be shown the event: %#v", state.Events)
	}
	// Joining it is refused for the same reason, not merely hidden.
	_, err := store.ApplyPublicEvent(storyID, PublicEventRequest{
		BranchID: "main", EventID: publiquevent.PublicEventIDTaiyueTide,
		Action: PublicEventActionJoin, ParticipationAction: string(publiquevent.ActionObserve),
		CommandID: "cmd-join", CommandHash: "h-join",
	})
	if !errors.Is(err, ErrPublicEventNotHere) {
		t.Fatalf("joining an event the character never heard of must be refused: %v", err)
	}
}

// TestAForkSeesTheStageOfItsOwnInstant is the fork rule that makes the event
// honest across branches: a fork cut before time was spent sees the earlier
// stage and does not inherit the participation.
func TestAForkSeesTheStageOfItsOwnInstant(t *testing.T) {
	store, storyID := publicEventTestStoryOn(t, publicEventTestCombatSystem(publicEventTestMarket))
	openingTurnID := journeyTestOpeningTurnID(t, store, storyID)

	// Spend enough formal time to reach the second stage through a real rest.
	if _, err := store.ApplySafeRecovery(storyID, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoverySafeMeditation,
		DurationSeconds: 7 * 60 * 60,
		CommandID:       "cmd-long", CommandHash: "h-long",
	}); err != nil {
		t.Fatal(err)
	}
	mainView := publicEventTestView(t, publicEventTestState(t, store, storyID, ""))
	if mainView.CurrentStageID != "taiyue.rush" {
		t.Fatalf("seven hours must reach the second stage: %q", mainView.CurrentStageID)
	}

	publicEventTestApply(t, store, storyID, PublicEventRequest{
		Action: PublicEventActionJoin, ParticipationAction: string(publiquevent.ActionObserve),
		CommandID: "cmd-join", CommandHash: "h-join",
	})

	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: openingTurnID, Title: "早于参与"})
	if err != nil {
		t.Fatal(err)
	}
	forkState := publicEventTestState(t, store, storyID, fork.ID)
	if forkState.ClockSeconds != 0 {
		t.Fatalf("a fork cut before the rest must not inherit its time: %d", forkState.ClockSeconds)
	}
	forkView := publicEventTestView(t, forkState)
	if forkView.CurrentStageID != "taiyue.rumor" {
		t.Fatalf("a fork cut earlier must see the earlier stage: %q", forkView.CurrentStageID)
	}
	if forkView.Joined {
		t.Fatalf("a fork cut before the participation must not inherit it: %#v", forkView)
	}
	// Main is untouched by the fork.
	if joined := publicEventTestView(t, publicEventTestState(t, store, storyID, "")); !joined.Joined {
		t.Fatal("main must still hold the participation")
	}
}

// TestAParticipantOnlyKnowsTheStageTheyWereAt is §6's filtering at the view: the
// participant lines of a stage belong to the character who was present at it,
// and a character who joined later does not learn them retroactively.
func TestAParticipantOnlyKnowsTheStageTheyWereAt(t *testing.T) {
	store, storyID := publicEventTestStory(t)
	publicEventTestApply(t, store, storyID, PublicEventRequest{
		Action: PublicEventActionJoin, ParticipationAction: string(publiquevent.ActionHelp),
		CommandID: "cmd-join", CommandHash: "h-join",
	})

	definition, _ := publiquevent.DefinitionByID(publiquevent.PublicEventIDTaiyueTide)
	view := publicEventTestView(t, publicEventTestState(t, store, storyID, ""))
	joined, _ := definition.StageByID(view.JoinedStage)
	for _, stage := range view.Stages {
		for _, line := range stage.Lines {
			if strings.TrimSpace(line) == "" {
				t.Fatalf("a stage line must never be blank: %#v", stage)
			}
			// A secret motive is the one thing the view must never carry.
			for _, participant := range definition.Participants {
				if participant.SecretMotive != "" && line == participant.SecretMotive {
					t.Fatalf("a secret motive must never reach the view: %q", line)
				}
			}
		}
		// Only the joined stage carries participant lines; every other stage
		// carries its public summary alone.
		if stage.ID != joined.ID && len(stage.Lines) != 1 {
			t.Fatalf("a stage the character was not at must show only its summary: %#v", stage)
		}
	}
}

// TestTheEntryOffersEveryAuthoredAction keeps the buttons honest: the panel never
// invents a choice, so what it offers is exactly what the rules accept.
func TestTheEntryOffersEveryAuthoredAction(t *testing.T) {
	store, storyID := publicEventTestStory(t)
	definition, ok := publiquevent.DefinitionByID(publiquevent.PublicEventIDTaiyueTide)
	if !ok {
		t.Fatal("the authored event must be registered")
	}
	view := publicEventTestView(t, publicEventTestState(t, store, storyID, ""))
	if len(view.Actions) != len(definition.Actions) {
		t.Fatalf("the entry must offer every authored action: %d vs %d", len(view.Actions), len(definition.Actions))
	}
	for index, action := range definition.Actions {
		offered := view.Actions[index]
		if offered.Kind != string(action.Kind) || offered.Label != action.Label {
			t.Fatalf("the entry must use the authored wording: %#v vs %#v", offered, action)
		}
		if (action.Reward == nil) != (offered.Reward == nil) {
			t.Fatalf("the entry must show exactly the real reward: %#v", offered)
		}
	}
	if len(view.Participants) != len(definition.Participants) {
		t.Fatalf("the region's participants must be listed: %d", len(view.Participants))
	}
}
