package interactive

import (
	"testing"

	"ta/internal/character"
	"ta/internal/exploration"
)

func TestFactionMembershipRouteIsNotAnInventedAccessPermission(t *testing.T) {
	ctx, _ := membershipRuleFixture()
	p := &storyBranchProjection{Characters: &character.State{}, PackCatalog: ctx.Catalog}
	for _, status := range []string{"", "pending", "formal", "temporary", "left"} {
		p.Characters.FactionMemberships = []character.FactionMembership{{FactionID: "yue.huangfeng", Status: status}}
		if got, want := membershipRouteAvailable(p, huangfengEntryRoute), status == "formal"; got != want {
			t.Fatalf("%s gate=%v", status, got)
		}
		if !membershipRouteAvailable(p, huangfengExitRoute) || !membershipRouteAvailable(p, huangfengMarketRoute) {
			t.Fatal("standing traps actor inside or blocks public road")
		}
	}
}
func TestFactionMembershipTaskConsumesRealJourneyAfterLeaving(t *testing.T) {
	store, id := membershipStoreFixture(t)
	// Place the previously established source at the registered interior, not a
	// free-text teleport used to satisfy the task's destination.
	if _, err := store.AppendStateDelta(id, AppendStateDeltaRequest{BranchID: "main", ActorOps: []ActorStateOp{{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storySceneLocationField, Value: "黄枫谷山门内"}}}); err != nil {
		t.Fatal(err)
	}
	taskID := "quest.huangfeng_member_market_visit"
	if _, err := store.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: taskID, Action: QuestActionOffer}); err == nil {
		t.Fatal("nonmember received a member-only commission")
	}
	ctx, facts := membershipRuleFixture()
	if _, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: ctx.PlayerInput, Narrative: "掌门接纳入谷。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: facts}}); err != nil {
		t.Fatal(err)
	}
	questTestApply(t, store, id, QuestRequest{TaskID: taskID, Action: QuestActionOffer})
	questTestApply(t, store, id, QuestRequest{TaskID: taskID, Action: QuestActionAccept})
	if _, err := store.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: taskID, Action: QuestActionComplete}); err == nil {
		t.Fatal("dialogue alone completed actual arrival")
	}
	// Withdrawal revokes gate access and new offers, not an accepted commitment.
	leave := &TurnCharacterFactsInput{Memberships: []TurnFactionMembershipInput{{FactionID: "yue.huangfeng", Status: "left", Statement: "主角主动退出，仍愿履行已接的小事", SourceText: "我退出黄枫谷。"}}}
	if _, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "我退出黄枫谷。", Narrative: "主角说明退出。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: leave}}); err != nil {
		t.Fatal(err)
	}
	exit := journeyTestApply(t, store, id, JourneyRequest{RouteID: huangfengExitRoute, Arrive: true, CommandID: "membership-exit", CommandHash: "membership-exit"})
	if exit.AddedMinutes != 30 || !exit.Arrived {
		t.Fatalf("gate exit not formal travel: %+v", exit)
	}
	if _, err := store.ApplyJourney(id, JourneyRequest{BranchID: "main", RouteID: huangfengEntryRoute, Arrive: true, CommandID: "membership-illegal-return", CommandHash: "membership-illegal-return"}); err == nil {
		t.Fatal("left member reentered restricted gate")
	}
	arrived := journeyTestApply(t, store, id, JourneyRequest{RouteID: huangfengMarketRoute, Arrive: true, CommandID: "membership-market", CommandHash: "membership-market"})
	if arrived.AddedMinutes != 2880 || !arrived.Arrived || journeyTestLocation(t, store, id, "main") != "公共坊市" {
		t.Fatalf("market task did not consume real travel: %+v", arrived)
	}
	if task := questTestTask(t, questTestState(t, store, id), taskID); task.Status != "completed" {
		t.Fatalf("arrival did not satisfy real objective: %+v", task)
	}
	questTestApply(t, store, id, QuestRequest{TaskID: taskID, Action: QuestActionSettle})
}

func TestFactionMembershipRoutePreservesOldCatalogAndCommittedPassage(t *testing.T) {
	ctx, _ := membershipRuleFixture()
	p := &storyBranchProjection{PackCatalog: ctx.Catalog, Characters: &character.State{FactionMemberships: []character.FactionMembership{{FactionID: "yue.huangfeng", Status: "formal"}}}}
	p.PackCatalog.ContentVersion = 29
	if membershipRouteAvailable(p, huangfengMarketRoute) || membershipRouteAvailable(p, huangfengEntryRoute) {
		t.Fatal("new route changed the older frozen revision")
	}
	p.PackCatalog.ContentVersion = 30
	p.Characters.FactionMemberships[0].Status = "left"
	p.Journeys = map[string]exploration.JourneySnapshot{"existing": {RouteID: huangfengEntryRoute, ElapsedMinutes: 10, TotalMinutes: 30}}
	if !membershipRouteAvailable(p, huangfengEntryRoute) {
		t.Fatal("withdrawal stranded an already permitted journey midway")
	}
	p.Journeys["existing"] = exploration.JourneySnapshot{RouteID: huangfengEntryRoute, ElapsedMinutes: 30, TotalMinutes: 30}
	if membershipRouteAvailable(p, huangfengEntryRoute) {
		t.Fatal("completed historical passage grants a new entry")
	}
}
func TestFactionMembershipRouteUsesSameFrozenCatalogDuringSubmission(t *testing.T) {
	store, id := membershipStoreFixture(t)
	if _, err := store.AppendStateDelta(id, AppendStateDeltaRequest{BranchID: "main", ActorOps: []ActorStateOp{{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storySceneLocationField, Value: "黄枫谷山门内"}}}); err != nil {
		t.Fatal(err)
	}
	ctx, err := store.StoryContext(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.TurnWorldView(id, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	validation := TurnSubmissionContext{CurrentState: ctx.Snapshot.State, Meta: ctx.Meta, BranchID: "main", ActorState: actorStateSystemFromSnapshot(ctx.Meta.ActorStateSchema, StoryDirectorActorStateSystem{}), WorldView: &view}
	if _, err := dryRunTurnWorldPlan(validation, &TurnWorldActionInput{Action: TurnWorldActionJourney, RouteID: huangfengExitRoute, Arrive: true}, nil); err != nil {
		t.Fatalf("submission lost frozen route source: %v", err)
	}
}

func TestFactionMembershipTemporaryCannotBecomePermanentGatePermission(t *testing.T) {
	ctx, _ := membershipRuleFixture()
	p := &storyBranchProjection{PackCatalog: ctx.Catalog, Characters: &character.State{FactionMemberships: []character.FactionMembership{{FactionID: "yue.huangfeng", Status: "temporary", Statement: "只暂列一日"}}}}
	if membershipRouteAvailable(p, huangfengEntryRoute) {
		t.Fatal("unbounded temporary record became permanent gate permission")
	}
}
