package interactive

import (
	"errors"
	"strings"
	"testing"

	"ta/internal/quest"
)

// These tests are 自制-事件与任务 §9 as it happens during play: a task is offered,
// the player takes it on, a committed fact advances it, and settling it pays once.
//
// The rules package already proves the objective arithmetic. What these tests add
// is that the same facts are reachable from the story's one journal: the tick is
// written by the commit that really produced the fact, the payment lands in the
// same transaction, a retry pays nothing twice, and an offer the player never
// accepted does not advance behind their back.

// questTestStory builds a story standing at the route's origin, which is where
// the personal task's second objective can really be reached from.
func questTestStory(t *testing.T) (*Store, string) {
	t.Helper()
	system := journeyTestSystem("越国坊市外围散修人家")
	for index := range system.InitialActors {
		if system.InitialActors[index].ID == DefaultStoryContextActorID {
			system.InitialActors[index].State[storyScenePresentField] = []any{"su-qian"}
		}
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "事务", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "听人说起", Narrative: "主角在散修人家门前听人提起坊市里托付的一件小事。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{
			Profiles: []TurnCharacterProfileInput{{CharacterID: "su-qian", Name: "苏浅", SourceKey: "npc_su_qian"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// questTestApply settles one task action and fails the test if the store refuses
// it.
func questTestApply(t *testing.T, store *Store, storyID string, req QuestRequest) QuestResult {
	t.Helper()
	if req.BranchID == "" {
		req.BranchID = "main"
	}
	// Settlement fixtures must first establish the offer. Discovery tests call
	// the public Store methods directly, without this fixture preparation.
	if req.Action == QuestActionAccept {
		state, err := store.QuestStateFor(storyID, req.BranchID)
		if err != nil {
			t.Fatal(err)
		}
		known := false
		for _, task := range state.Tasks {
			if task.TaskID == req.TaskID {
				known = true
			}
		}
		if !known {
			questTestKnownOffer(t, store, storyID, req.TaskID)
		}
	}
	result, err := store.ApplyQuest(storyID, req)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// questTestKnownOffer records a real source in the deadline/settlement fixtures
// that previously accepted a private card solely by ID. It uses normal commits.
func questTestKnownOffer(t *testing.T, store *Store, storyID, taskID string) {
	questTestSource(t, store, storyID)
	if _, err := store.ApplyQuest(storyID, QuestRequest{BranchID: "main", TaskID: taskID, Action: QuestActionOffer}); err != nil {
		t.Fatal(err)
	}
}

func questTestSource(t *testing.T, store *Store, storyID string) {
	t.Helper()
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "询问差事", Narrative: "在散修聚落听到求药消息，苏浅也当面说起了待交接的药材。",
		ActorOps: []ActorStateOp{
			{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storySceneLocationField, Value: "越国坊市外围散修人家"},
			{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{"su-qian"}},
		},
		TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{
			Profiles: []TurnCharacterProfileInput{{CharacterID: "su-qian", Name: "苏浅", SourceKey: "npc_su_qian"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

// questTestState reads the branch's tasks through the projection the entry uses.
func questTestState(t *testing.T, store *Store, storyID string) QuestState {
	t.Helper()
	state, err := store.QuestStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return state
}

// questTestTask finds one task view by id.
func questTestTask(t *testing.T, state QuestState, taskID string) QuestStateView {
	t.Helper()
	for _, task := range state.Tasks {
		if task.TaskID == taskID {
			return task
		}
	}
	t.Fatalf("task %s must be in play: %#v", taskID, state.Tasks)
	return QuestStateView{}
}

// questTestRecordCount counts the task records on the branch's journal.
func questTestRecordCount(t *testing.T, store *Store, storyID string) int {
	t.Helper()
	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, record := range records {
		if record.Envelope.Type == StoryEventTypeQuestRecord {
			count++
		}
	}
	return count
}

// questTestCurrency reads the committed spirit-stone balance.
func questTestCurrency(t *testing.T, store *Store, storyID string) int64 {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return int64(xiuxianStoredInt(snapshot.State, DefaultActorID, xiuxianFieldCurrency))
}

// TestAnOfferIsNotAnAcceptance is §4.4's boundary. Offering a task records it as
// offered and does not tick anything, even if the player has already done the
// work.
func TestAnOfferIsNotAnAcceptance(t *testing.T) {
	store, storyID := questTestStory(t)

	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskHerbHandover, Action: QuestActionOffer,
		CommandID: "cmd-offer", CommandHash: "h-offer",
	})
	view := questTestTask(t, questTestState(t, store, storyID), quest.TaskHerbHandover)
	if view.Status != string(quest.StatusOffered) {
		t.Fatalf("an offer must stay offered: %q", view.Status)
	}
	if view.Settled {
		t.Fatal("an offer must not pay")
	}
}

// TestAnUnacceptedTaskDoesNotAdvanceOnARealFact is the second half of §4.4: doing
// the thing does not accept the task on the player's behalf.
func TestAnUnacceptedTaskDoesNotAdvanceOnARealFact(t *testing.T) {
	store, storyID := questTestStory(t)
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskSuQianMaterials, Action: QuestActionOffer,
		CommandID: "cmd-offer", CommandHash: "h-offer",
	})

	// Really walk to the market, which is the personal task's second objective.
	journeyTestApply(t, store, storyID, JourneyRequest{
		RouteID: "route_036:reverse", Arrive: true,
		CommandID: "cmd-walk", CommandHash: "h-walk",
	})

	view := questTestTask(t, questTestState(t, store, storyID), quest.TaskSuQianMaterials)
	if view.Status != string(quest.StatusOffered) {
		t.Fatalf("an offer the player never took on must stay an offer: %q", view.Status)
	}
	for _, objective := range view.Objectives {
		if objective.Completed {
			t.Fatalf("nothing may tick before acceptance: %#v", objective)
		}
	}
}

// TestAnAcceptedTaskAdvancesOnACommittedFact is the loop's core: accepting the
// task and really arriving is what ticks the objective, and the tick lands on the
// journal as its own record.
func TestAnAcceptedTaskAdvancesOnACommittedFact(t *testing.T) {
	store, storyID := questTestStory(t)
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskSuQianMaterials, Action: QuestActionOffer,
		CommandID: "cmd-offer", CommandHash: "h-offer",
	})
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskSuQianMaterials, Action: QuestActionAccept,
		WorldTime: "第三日", CommandID: "cmd-accept", CommandHash: "h-accept",
	})
	before := questTestRecordCount(t, store, storyID)

	journeyTestApply(t, store, storyID, JourneyRequest{
		RouteID: "route_036:reverse", Arrive: true,
		CommandID: "cmd-walk", CommandHash: "h-walk",
	})

	if after := questTestRecordCount(t, store, storyID); after != before+1 {
		t.Fatalf("the arrival must tick the task exactly once: before=%d after=%d", before, after)
	}
	view := questTestTask(t, questTestState(t, store, storyID), quest.TaskSuQianMaterials)
	var source *QuestObjectiveView
	for index := range view.Objectives {
		if view.Objectives[index].ID == "quest.su_qian_materials.source" {
			source = &view.Objectives[index]
		}
	}
	if source == nil || !source.Completed {
		t.Fatalf("arriving at the market must complete the source objective: %#v", view.Objectives)
	}
	// The other objective is untouched: a task does not complete on half its work.
	if view.Status != string(quest.StatusAccepted) {
		t.Fatalf("a half-done task must stay accepted: %q", view.Status)
	}
}

// TestAcceptingCreditsWorkAlreadyDone is §4.4's other half, and the live defect
// this test was written for.
//
// The section keeps acceptance the player's decision, so doing the work must not
// take the task on by itself. It also says a character who acted before
// accepting still gets the real result — 不要求补点"领取" — so accepting after
// the work has to credit it. Without that, the live story's errand (herb already
// granted, offer still standing) could only be accepted into a task whose single
// objective the world had already satisfied and which could never tick again.
func TestAcceptingCreditsWorkAlreadyDone(t *testing.T) {
	store, storyID := questTestStory(t)

	// Really take the herb out of the cave first, with no task in play.
	realmTestApply(t, store, storyID, RealmRequest{
		RealmID: "realm.gu_xiu_dong_fu", Action: RealmActionEnter,
		CommandID: "cmd-enter", CommandHash: "h-enter",
	})
	realmTestApply(t, store, storyID, RealmRequest{
		RealmID: "realm.gu_xiu_dong_fu", Action: RealmActionMove,
		ConnectionID: "realm.hall_to_plot", DestinationAreaID: "realm.herb_plot",
		CommandID: "cmd-move", CommandHash: "h-move",
	})
	realmTestApply(t, store, storyID, RealmRequest{
		RealmID: "realm.gu_xiu_dong_fu", Action: RealmActionReward,
		ContentRefID: "realm.herb_ref",
		CommandID:    "cmd-reward", CommandHash: "h-reward",
	})

	// Only now is the errand raised and taken on.
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskHerbHandover, Action: QuestActionOffer,
		CommandID: "cmd-offer", CommandHash: "h-offer",
	})
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskHerbHandover, Action: QuestActionAccept,
		WorldTime: "第一日", CommandID: "cmd-accept", CommandHash: "h-accept",
	})

	view := questTestTask(t, questTestState(t, store, storyID), quest.TaskHerbHandover)
	if len(view.Objectives) != 1 || !view.Objectives[0].Completed {
		t.Fatalf("the herb really taken before acceptance must be credited: %#v", view.Objectives)
	}
	if view.Status != string(quest.StatusCompleted) {
		t.Fatalf("crediting the only objective must complete the errand: %q", view.Status)
	}
	if view.Settled {
		t.Fatal("crediting work must not pay; settle is still the only payment")
	}

	// Settling pays the agreed amount exactly once, so the credit did not
	// double-count anything.
	before := questTestCurrency(t, store, storyID)
	settled := questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskHerbHandover, Action: QuestActionSettle,
		CommandID: "cmd-settle", CommandHash: "h-settle",
	})
	if len(settled.Rewards) != 1 || settled.Rewards[0].Quantity != 20 {
		t.Fatalf("the agreed payment must be handed over once: %#v", settled.Rewards)
	}
	if after := questTestCurrency(t, store, storyID); after != before+20 {
		t.Fatalf("settling must add the agreed payment once: before=%d after=%d", before, after)
	}
}

// TestAcceptingCreditsOnlyFactsThisBranchHas keeps the credit from reading a
// fork's parent or another branch's work.
func TestAcceptingCreditsOnlyFactsThisBranchHas(t *testing.T) {
	store, storyID := questTestStory(t)
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskSuQianMaterials, Action: QuestActionAccept,
		CommandID: "cmd-accept", CommandHash: "h-accept",
	})

	view := questTestTask(t, questTestState(t, store, storyID), quest.TaskSuQianMaterials)
	for _, objective := range view.Objectives {
		if objective.Completed {
			t.Fatalf("a fact this branch never produced must not tick: %#v", objective)
		}
	}
	if view.Status != string(quest.StatusAccepted) {
		t.Fatalf("nothing was done yet, so the task stays open: %q", view.Status)
	}
}

// TestATaskCannotCompleteWithWorkOpen keeps a completion honest.
func TestATaskCannotCompleteWithWorkOpen(t *testing.T) {

	store, storyID := questTestStory(t)
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskSuQianMaterials, Action: QuestActionOffer,
		CommandID: "cmd-offer", CommandHash: "h-offer",
	})
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskSuQianMaterials, Action: QuestActionAccept,
		CommandID: "cmd-accept", CommandHash: "h-accept",
	})

	_, err := store.ApplyQuest(storyID, QuestRequest{
		BranchID: "main", TaskID: quest.TaskSuQianMaterials, Action: QuestActionComplete,
		CommandID: "cmd-complete", CommandHash: "h-complete",
	})
	if err == nil || !errors.Is(err, ErrQuestObjectiveUnmet) {
		t.Fatalf("completing with an open objective must be refused: %v", err)
	}
}

// TestSettlingPaysOnceAndOnlyOnce is §4.7: the agreed reward is handed over in the
// same transaction as the settled flag, and a second attempt pays nothing.
func TestSettlingPaysOnceAndOnlyOnce(t *testing.T) {
	store, storyID := questTestStory(t)
	before := questTestCurrency(t, store, storyID)

	// The errand's only objective is the realm reward, and the story starts at
	// the realm's entrance, so it is taken on the spot: the one authored route
	// runs outward only, and a realm cannot be entered from the market it leads
	// to.
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskHerbHandover, Action: QuestActionOffer,
		CommandID: "cmd-offer2", CommandHash: "h-offer2",
	})
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskHerbHandover, Action: QuestActionAccept,
		CommandID: "cmd-accept2", CommandHash: "h-accept2",
	})
	realmTestApply(t, store, storyID, RealmRequest{
		RealmID: "realm.gu_xiu_dong_fu", Action: RealmActionEnter,
		CommandID: "cmd-enter", CommandHash: "h-enter",
	})
	realmTestApply(t, store, storyID, RealmRequest{
		RealmID: "realm.gu_xiu_dong_fu", Action: RealmActionMove,
		ConnectionID: "realm.hall_to_plot", DestinationAreaID: "realm.herb_plot",
		CommandID: "cmd-move", CommandHash: "h-move",
	})
	realmTestApply(t, store, storyID, RealmRequest{
		RealmID: "realm.gu_xiu_dong_fu", Action: RealmActionReward,
		ContentRefID: "realm.herb_ref",
		CommandID:    "cmd-reward", CommandHash: "h-reward",
	})

	view := questTestTask(t, questTestState(t, store, storyID), quest.TaskHerbHandover)
	if view.Status != string(quest.StatusCompleted) {
		t.Fatalf("taking the declared reward must complete the errand: %q", view.Status)
	}

	settled := questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskHerbHandover, Action: QuestActionSettle,
		WorldTime: "第三日", CommandID: "cmd-settle", CommandHash: "h-settle",
	})
	if len(settled.Rewards) != 1 || settled.Rewards[0].Quantity != 20 {
		t.Fatalf("settling must pay the authored reward: %#v", settled.Rewards)
	}
	if !settled.Instance.SettledRewards {
		t.Fatal("settling must record that it paid")
	}
	if after := questTestCurrency(t, store, storyID); after != before+20 {
		t.Fatalf("the payment must reach the purse: before=%d after=%d", before, after)
	}

	// A second settlement is refused, and a retry under the same command returns
	// the first answer rather than paying again.
	if _, err := store.ApplyQuest(storyID, QuestRequest{
		BranchID: "main", TaskID: quest.TaskHerbHandover, Action: QuestActionSettle,
		CommandID: "cmd-settle-again", CommandHash: "h-settle-again",
	}); err == nil || !errors.Is(err, ErrQuestAlreadySettled) {
		t.Fatalf("a settled task must not pay twice: %v", err)
	}
	replay := questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskHerbHandover, Action: QuestActionSettle,
		CommandID: "cmd-settle", CommandHash: "h-settle",
	})
	if !replay.Replayed {
		t.Fatal("a repeated command must report itself as replayed")
	}
	if after := questTestCurrency(t, store, storyID); after != before+20 {
		t.Fatalf("a replay must not pay again: before=%d after=%d", before+20, after)
	}
}

// TestACommandReusedForADifferentActionIsRefused keeps idempotency from answering
// the wrong question.
func TestACommandReusedForADifferentActionIsRefused(t *testing.T) {
	store, storyID := questTestStory(t)
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskHerbHandover, Action: QuestActionOffer,
		CommandID: "cmd-one", CommandHash: "h-one",
	})
	_, err := store.ApplyQuest(storyID, QuestRequest{
		BranchID: "main", TaskID: quest.TaskHerbHandover, Action: QuestActionAccept,
		CommandID: "cmd-one", CommandHash: "h-two",
	})
	if err == nil || !errors.Is(err, ErrQuestCommandConflict) {
		t.Fatalf("reusing a command id for another action must be refused: %v", err)
	}
}

// TestAbandoningATaskStopsItAdvancing keeps an abandoned task from ticking on
// later facts.
func TestAbandoningATaskStopsItAdvancing(t *testing.T) {
	store, storyID := questTestStory(t)
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskSuQianMaterials, Action: QuestActionOffer,
		CommandID: "cmd-offer", CommandHash: "h-offer",
	})
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskSuQianMaterials, Action: QuestActionAccept,
		CommandID: "cmd-accept", CommandHash: "h-accept",
	})
	questTestApply(t, store, storyID, QuestRequest{
		TaskID: quest.TaskSuQianMaterials, Action: QuestActionAbandon,
		CommandID: "cmd-abandon", CommandHash: "h-abandon",
	})
	before := questTestRecordCount(t, store, storyID)

	journeyTestApply(t, store, storyID, JourneyRequest{
		RouteID: "route_036:reverse", Arrive: true,
		CommandID: "cmd-walk", CommandHash: "h-walk",
	})

	if after := questTestRecordCount(t, store, storyID); after != before {
		t.Fatalf("an abandoned task must not advance: before=%d after=%d", before, after)
	}
}

// TestATaskSurvivesARestart is the side record's own test: the task is rebuilt
// from the journal, so a reopened store reads the same state.
func TestATaskSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	system := journeyTestSystem("越国坊市外围散修人家")
	store := NewStore(dir)
	story, err := store.CreateStory(CreateStoryRequest{Title: "事务", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "听人说起", Narrative: "主角在散修人家门前听人提起坊市里托付的一件小事。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	questTestSource(t, store, story.ID)
	if _, err := store.ApplyQuest(story.ID, QuestRequest{
		BranchID: "main", TaskID: quest.TaskSuQianMaterials, Action: QuestActionOffer,
		CommandID: "cmd-offer", CommandHash: "h-offer",
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
	view := questTestTask(t, state, quest.TaskSuQianMaterials)
	if view.Status != string(quest.StatusOffered) {
		t.Fatalf("a reopened store must read the same task: %q", view.Status)
	}
	if !strings.Contains(view.Title, "药材") {
		t.Fatalf("the task must carry its authored wording: %q", view.Title)
	}
}
