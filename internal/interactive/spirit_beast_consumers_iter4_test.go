package interactive

import (
	"errors"
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

func iter4CompanionBattleStory(t *testing.T) (*Store, string, AppendTurnWithStateRequest) {
	t.Helper()
	store, storyID := f17FlowStory(t)
	f17MeetCreature(t, store, storyID, f17FlowBeast, ActorStateImportantCharacterTemplateID, cultivation.Qi(6), map[string]any{
		xiuxianFieldLearnedSkills: map[string]any{"狐焰": map[string]any{"定义": "fire_bolt", "版本": 1, "掌握程度": "入门"}},
	})
	f17MeetCreature(t, store, storyID, "山狼", ActorStateOpponentTemplateID, cultivation.Qi(6), nil)
	_, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "在场相识", Narrative: "青焰狐伏在你身边，山狼在对岸。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{DefaultActorID, f17FlowBeast, "山狼"}}},
			CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{CharacterID: f17FlowBeast, Name: f17FlowBeast, Lifeform: PackCatalogLifeformSpiritBeast, CultivationMethodID: spiritBeastGrowthMethod}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "请青焰狐帮我抵挡山狼", Narrative: "青焰狐回应了邀请，与你一起挡住扑来的山狼。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{Op: interactivestate.Replace, Path: "/story/本次交战", Value: map[string]any{
			"山狼": "扑向主角", f17FlowBeast: map[string]any{"side": "player", "basis": "本人接受本场助战", "target_id": "山狼", "source_exchange_index": 1},
		}}}, CharacterFacts: &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{CharacterID: f17FlowBeast, Speaker: "character", Channel: "scene", AddressedTo: DefaultActorID, Text: "（青焰狐回应你的邀请，站到你身侧迎击山狼。）", ParticipationDecision: &character.ParticipationDecision{Kind: character.ParticipationCombat, TargetID: "山狼", Decision: character.ParticipationAccept}}}}},
	}
}

func TestIter4CompanionConsentRejectsUnownedOrRemoteResponses(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*testing.T, *Store, string, *AppendTurnWithStateRequest)
	}{
		{"missing response", func(_ *testing.T, _ *Store, _ string, request *AppendTurnWithStateRequest) {
			request.TurnResult.CharacterFacts = nil
		}},
		{"no decision", func(_ *testing.T, _ *Store, _ string, request *AppendTurnWithStateRequest) {
			request.TurnResult.CharacterFacts.Exchanges[0].ParticipationDecision = nil
		}},
		{"refusal", func(_ *testing.T, _ *Store, _ string, request *AppendTurnWithStateRequest) {
			request.TurnResult.CharacterFacts.Exchanges[0].Text = "（摇头，退回石后。）"
			request.TurnResult.CharacterFacts.Exchanges[0].ParticipationDecision.Decision = character.ParticipationRefuse
		}},
		{"later refusal", func(_ *testing.T, _ *Store, _ string, request *AppendTurnWithStateRequest) {
			x := request.TurnResult.CharacterFacts.Exchanges[0]
			x.ParticipationDecision = &character.ParticipationDecision{Kind: character.ParticipationCombat, TargetID: "山狼", Decision: character.ParticipationRefuse}
			x.Text = "（又摇头退开，拒绝参与。）"
			request.TurnResult.CharacterFacts.Exchanges = append(request.TurnResult.CharacterFacts.Exchanges, x)
		}},
		{"player command", func(_ *testing.T, _ *Store, _ string, request *AppendTurnWithStateRequest) {
			request.TurnResult.CharacterFacts.Exchanges[0].Speaker = "player"
		}},
		{"other speaker", func(_ *testing.T, _ *Store, _ string, request *AppendTurnWithStateRequest) {
			request.TurnResult.CharacterFacts.Exchanges[0].CharacterID = "山狼"
		}},
		{"private response", func(_ *testing.T, _ *Store, _ string, request *AppendTurnWithStateRequest) {
			request.TurnResult.CharacterFacts.Exchanges[0].Channel = "private"
		}},
		{"undirected response", func(_ *testing.T, _ *Store, _ string, request *AppendTurnWithStateRequest) {
			request.TurnResult.CharacterFacts.Exchanges[0].AddressedTo = ""
		}},
		{"remote creature", func(t *testing.T, store *Store, id string, request *AppendTurnWithStateRequest) {
			f17Present(t, store, id, DefaultActorID, "山狼")
			request.TurnResult.StateUpdates = append(request.TurnResult.StateUpdates, interactivestate.Update{Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{DefaultActorID, f17FlowBeast, "山狼"}})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, storyID, request := iter4CompanionBattleStory(t)
			test.edit(t, store, storyID, &request)
			before, err := store.Snapshot(storyID, "main")
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.AppendTurnWithState(storyID, request); err == nil {
				t.Fatal("an unsupported companion declaration was committed")
			}
			after, err := store.Snapshot(storyID, "main")
			if err != nil || after.CurrentTurn.ID != before.CurrentTurn.ID {
				t.Fatalf("refused participation partially committed: err=%v", err)
			}
		})
	}
}

func TestIter4CompanionUsesOwnSkillOnceAndKeepsCostAcrossRetryReopenAndFork(t *testing.T) {
	store, storyID, request := iter4CompanionBattleStory(t)
	before, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(storyID, request); err != nil {
		t.Fatal(err)
	}
	encounter, err := store.CombatEncounter(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	opened, err := store.StartBattle(storyID, BattleStartRequest{BranchID: "main", Combatants: encounter.Combatants, CommandID: "companion-open", CommandHash: "companion-open"})
	if err != nil {
		t.Fatal(err)
	}
	beastBefore := opened.Session.CombatantByID(f17FlowBeast).Attributes
	if _, err := store.ResolveBattleRound(storyID, BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, CommandID: "missing-player", CommandHash: "missing-player"}); !errors.Is(err, ErrCombatPlayerActionRequired) {
		t.Fatalf("the autonomous companion must not authorize the player's omitted action: %v", err)
	}
	command := combatFatalityCommandForHit(opened.Session, "山狼", f17FlowBeast)
	round := BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, Actions: map[string]cultivation.CombatAction{
		DefaultActorID: {Kind: cultivation.ActionRest}, "山狼": {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: f17FlowBeast},
	}, CommandID: command, CommandHash: command}
	played, err := store.ResolveBattleRound(storyID, round)
	if err != nil {
		t.Fatal(err)
	}
	beastAfter := played.Session.CombatantByID(f17FlowBeast).Attributes
	steps := 0
	for _, step := range played.Outcome.Steps {
		if step.ActorID == f17FlowBeast {
			steps++
			if step.SkillID != "fire_bolt" || step.TargetID != "山狼" || step.ActionRejected || step.Cast.ManaCost == 0 {
				t.Fatalf("the creature did not use its own affordable skill against the enemy: %+v", step)
			}
		}
	}
	if steps != 1 || beastAfter.Mana >= beastBefore.Mana || beastAfter.Health >= beastBefore.Health {
		t.Fatalf("the creature must act once, pay its own mana and retain its wound: steps=%d before=%+v after=%+v", steps, beastBefore, beastAfter)
	}
	retry, err := store.ResolveBattleRound(storyID, round)
	if err != nil || retry.EventID != played.EventID || retry.Session.CombatantByID(f17FlowBeast).Attributes != beastAfter {
		t.Fatalf("a retry re-settled the creature: err=%v", err)
	}
	branch, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: before.CurrentTurn.ID, Title: "未接受助战"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(store.root)
	t.Cleanup(func() { _ = reopened.Close() })
	sessions, err := reopened.CombatSessions(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	stored := sessions[opened.Session.ID]
	if stored.CombatantByID(f17FlowBeast).Attributes != beastAfter {
		t.Fatal("a cold reopen lost the companion's consumed mana or wound")
	}
	prior, err := reopened.Snapshot(storyID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := actorStateNumber(actorStateFieldValue(prior.State, f17FlowBeast, xiuxianFieldMana)); int(got) != beastBefore.Mana {
		t.Fatalf("the earlier branch inherited the creature's future cost: %v", got)
	}
	if combatCompanionEligible(prior.State, f17BeastSystem(t, reopened, storyID), f17FlowBeast) {
		t.Fatal("the earlier branch inherited the creature's future consent")
	}
}

func TestIter4FirstMeetingCanRegisterAndAcceptAssistanceInTheSameTurn(t *testing.T) {
	store, storyID := f17FlowStory(t)
	_, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "向刚遇见的灵狐求助挡住山狼", Narrative: "灵狐回过头，以低鸣回应你的求助，站到你身侧迎击山狼。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{
			xiuxianCreateUpdate(f17FlowBeast, map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(2))}),
			xiuxianCreateUpdate("山狼", map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(2))}),
			{Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{DefaultActorID, f17FlowBeast, "山狼"}},
			{Op: interactivestate.Replace, Path: "/story/本次交战", Value: map[string]any{"山狼": "扑向主角", f17FlowBeast: map[string]any{"side": "player", "basis": "首次相遇时本人接受求助", "target_id": "山狼", "source_exchange_index": 1}}},
		}, CharacterFacts: &TurnCharacterFactsInput{
			Profiles:  []TurnCharacterProfileInput{{CharacterID: f17FlowBeast, Name: f17FlowBeast, Lifeform: PackCatalogLifeformSpiritBeast, CultivationMethodID: spiritBeastGrowthMethod}},
			Exchanges: []TurnCharacterExchangeInput{{CharacterID: f17FlowBeast, Speaker: "character", Channel: "scene", AddressedTo: DefaultActorID, Text: "（低鸣回应求助，主动并肩迎击。）", ParticipationDecision: &character.ParticipationDecision{Kind: character.ParticipationCombat, TargetID: "山狼", Decision: character.ParticipationAccept}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !combatCompanionEligible(combatTestState(t, store, storyID), f17BeastSystem(t, store, storyID), f17FlowBeast) {
		t.Fatal("a real same-turn first meeting imposed an unnecessary extra interaction")
	}
}

func TestIter4ExplicitRosterCannotConscriptAnUnwillingCreature(t *testing.T) {
	store, storyID, _ := iter4CompanionBattleStory(t)
	_, err := store.StartBattle(storyID, BattleStartRequest{BranchID: "main", CommandID: "conscript", CommandHash: "conscript",
		Combatants: []CombatantSpec{{ActorID: DefaultActorID, Side: cultivation.SidePlayer}, {ActorID: f17FlowBeast, Side: cultivation.SidePlayer}, {ActorID: "山狼", Side: cultivation.SideEnemy}},
	})
	if err == nil {
		t.Fatal("an explicit roster bypassed the creature's missing agreement")
	}
}

func TestIter4CompanionRecallKeepsItsWoundsAndDoesNotRejoinTheOldBattle(t *testing.T) {
	store, storyID, request := iter4CompanionBattleStory(t)
	if _, _, err := store.AppendTurnWithState(storyID, request); err != nil {
		t.Fatal(err)
	}
	encounter, _ := store.CombatEncounter(storyID, "main")
	opened, err := store.StartBattle(storyID, BattleStartRequest{BranchID: "main", Combatants: encounter.Combatants, CommandID: "recall-open", CommandHash: "recall-open"})
	if err != nil {
		t.Fatal(err)
	}
	command := combatFatalityCommandForHit(opened.Session, "山狼", f17FlowBeast)
	first, err := store.ResolveBattleRound(storyID, BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, CommandID: command, CommandHash: command,
		Actions: map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionRest}, "山狼": {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: f17FlowBeast}},
	})
	if err != nil {
		t.Fatal(err)
	}
	beast := first.Session.CombatantByID(f17FlowBeast).Attributes
	_, _, err = store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{BranchID: "main", User: "让它退下", Narrative: "青焰狐退出战场。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{Op: interactivestate.Archive, Path: "/" + f17FlowBeast, Value: map[string]any{"reason": "本人退出战场"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"recalled", "restored"} {
		if command == "restored" {
			_, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{BranchID: "main", User: "确认它还在身边", Narrative: "它回到旁边，伤势没有变化。",
				TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{
					{Op: interactivestate.Restore, Path: "/" + f17FlowBeast, Value: map[string]any{"reason": "返回旁观"}},
					{Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{DefaultActorID, f17FlowBeast, "山狼"}},
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		round, err := store.ResolveBattleRound(storyID, BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, CommandID: command, CommandHash: command,
			Actions: map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionRest}, "山狼": {Kind: cultivation.ActionRest}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := round.Session.CombatantByID(f17FlowBeast); !got.Fled || got.Attributes != beast {
			t.Fatalf("%s changed or reactivated the departed creature: %+v", command, got)
		}
		for _, step := range round.Outcome.Steps {
			if step.ActorID == f17FlowBeast {
				t.Fatalf("%s let the departed creature act again: %+v", command, step)
			}
		}
	}
}

func TestIter4LaterCompanionRefusalRequiresRemovingItsBattleDeclaration(t *testing.T) {
	store, storyID, request := iter4CompanionBattleStory(t)
	if _, _, err := store.AppendTurnWithState(storyID, request); err != nil {
		t.Fatal(err)
	}
	encounter, _ := store.CombatEncounter(storyID, "main")
	opened, err := store.StartBattle(storyID, BattleStartRequest{BranchID: "main", Combatants: encounter.Combatants, CommandID: "refuse-open", CommandHash: "refuse-open"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveBattleRound(storyID, BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, CommandID: "refuse-first", CommandHash: "refuse-first", Actions: map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionRest}, "山狼": {Kind: cultivation.ActionRest}}}); err != nil {
		t.Fatal(err)
	}
	refusal := AppendTurnWithStateRequest{BranchID: "main", User: "问它还愿不愿继续", Narrative: "它摇头退开，不再愿意助战。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{CharacterID: f17FlowBeast, Speaker: "character", Channel: "scene", AddressedTo: DefaultActorID, Text: "（摇头，退到旁边。）", ParticipationDecision: &character.ParticipationDecision{Kind: character.ParticipationCombat, TargetID: "山狼", Decision: character.ParticipationRefuse}}}}}}
	if _, _, err := store.AppendTurnWithState(storyID, refusal); err == nil {
		t.Fatal("a later explicit refusal kept an active battle declaration")
	}
	refusal.TurnResult.StateUpdates = []interactivestate.Update{{Op: interactivestate.Replace, Path: "/story/本次交战", Value: map[string]any{"山狼": "仍在交战"}}}
	if _, _, err := store.AppendTurnWithState(storyID, refusal); err != nil {
		t.Fatalf("removing the contradicted declaration must permit the refusal: %v", err)
	}
	played, err := store.ResolveBattleRound(storyID, BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, CommandID: "after-refusal", CommandHash: "after-refusal", Actions: map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionRest}, "山狼": {Kind: cultivation.ActionRest}}})
	if err != nil {
		t.Fatal(err)
	}
	if !played.Session.CombatantByID(f17FlowBeast).Fled {
		t.Fatal("the withdrawn creature kept fighting from the old session")
	}
}

func TestIter4SpiritBeastConsentsToOneBattleOnPlayersSide(t *testing.T) {
	store, storyID := f17FlowStory(t)
	f17FlowMeet(t, store, storyID)
	f17MeetCreature(t, store, storyID, "山狼", ActorStateOpponentTemplateID, cultivation.Qi(2), nil)
	_, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "请青焰狐一起挡住山狼", Narrative: "青焰狐回应你的请求，站到你身侧迎击扑来的山狼。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{
			{Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{DefaultActorID, f17FlowBeast, "山狼"}},
			{Op: interactivestate.Replace, Path: "/story/本次交战", Value: map[string]any{
				"山狼":         "扑向主角",
				f17FlowBeast: map[string]any{"side": "player", "basis": "本人同意迎击山狼", "target_id": "山狼", "source_exchange_index": 1},
			}},
		}, CharacterFacts: &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{
			CharacterID: f17FlowBeast, Speaker: "character", Channel: "scene", AddressedTo: DefaultActorID,
			Text: "（青焰狐应声低鸣，站到你身侧，准备迎击山狼。）", ParticipationDecision: &character.ParticipationDecision{Kind: character.ParticipationCombat, TargetID: "山狼", Decision: character.ParticipationAccept},
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	encounter, err := store.CombatEncounter(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, actor := range encounter.Combatants {
		if actor.ActorID == f17FlowBeast {
			if actor.Side != cultivation.SidePlayer {
				t.Fatalf("the creature's accepted assistance became an enemy: %+v", actor)
			}
			return
		}
	}
	t.Fatalf("the creature's accepted assistance has no combat consumer: %+v", encounter)
}

func TestIter4ArchivedCreatureCannotKeepActingInLiveBattle(t *testing.T) {
	store, storyID := combatTestStory(t)
	t.Cleanup(func() { _ = store.Close() })
	opened, err := store.StartBattle(storyID, BattleStartRequest{BranchID: "main", Combatants: combatTestSpecs(), CommandID: "archive-open", CommandHash: "archive-open"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "让它退出交战", Narrative: "这只生灵退出场地，不再参与交战。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{Op: interactivestate.Archive, Path: "/rival", Value: map[string]any{"reason": "退出场地"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	played, err := store.ResolveBattleRound(storyID, BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionRest}}, CommandID: "after-archive", CommandHash: "after-archive"})
	if err != nil {
		return // Refusing a stale participation roster is also safe.
	}
	for _, step := range played.Outcome.Steps {
		if step.ActorID == "rival" && !step.ActionRejected {
			t.Fatalf("an archived creature continued acting from the stale battle roster: %+v", step)
		}
	}
}

func TestIter4FormalFatalityCannotBeUndoneByNarrativeRestore(t *testing.T) {
	_, store, storyID, _, _ := combatFatalityTestStory(t, "rival")
	t.Cleanup(func() { _ = store.Close() })
	opened, err := store.StartBattle(storyID, BattleStartRequest{BranchID: "main", Combatants: combatTestSpecs(), CommandID: "dead-open", CommandHash: "dead-open"})
	if err != nil {
		t.Fatal(err)
	}
	commandID := combatFatalityCommandForHit(opened.Session, DefaultActorID, "rival")
	played, err := store.ResolveBattleRound(storyID, BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, PlayerIntent: PlayerCombatIntentTakeLife,
		Actions:   map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"}, "rival": {Kind: cultivation.ActionRest}},
		CommandID: commandID, CommandHash: commandID})
	if err != nil || len(played.Fatalities) != 1 {
		t.Fatalf("the fixture must settle a real fatality: result=%+v err=%v", played, err)
	}
	_, _, err = store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{BranchID: "main", User: "再叫它过来", Narrative: "它又重新参与故事。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{Op: interactivestate.Restore, Path: "/rival", Value: map[string]any{"reason": "重新出现"}}}},
	})
	if err == nil && !actorIsArchived(combatTestState(t, store, storyID), "rival") {
		t.Fatal("a generic narrative restore removed the actor's rule-settled death")
	}
}

func TestIter4JourneyDoesNotTakeEveryBystanderAlong(t *testing.T) {
	store, storyID := journeyTestStory(t, "越国坊市外围散修人家")
	_, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{BranchID: "main", User: "出门前看了一眼小兽", Narrative: "小兽留在门前，并没有同行。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{
			xiuxianCreateUpdate("小兽", map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(1))}),
			{Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{DefaultActorID, "小兽"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	journeyTestApply(t, store, storyID, JourneyRequest{RouteID: "route_036:reverse", Arrive: true, CommandID: "alone-to-market", CommandHash: "alone-to-market"})
	for _, id := range StorySceneFromState(combatTestState(t, store, storyID), "").Present {
		if id == "小兽" {
			t.Fatal("an ordinary journey carried a non-travelling creature to the destination")
		}
	}
}

func TestIter4CompanionRefusalUsesNormalizedSpeakerAndChannel(t *testing.T) {
	store, storyID, request := iter4CompanionBattleStory(t)
	if _, _, err := store.AppendTurnWithState(storyID, request); err != nil {
		t.Fatal(err)
	}
	_, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{BranchID: "main", User: "再问它是否愿意", Narrative: "它明确摇头，拒绝继续迎击山狼。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{CharacterID: f17FlowBeast, Speaker: " character ", Channel: " scene ", AddressedTo: DefaultActorID, Text: "（摇头拒绝继续迎击。）", ParticipationDecision: &character.ParticipationDecision{Kind: character.ParticipationCombat, TargetID: "山狼", Decision: character.ParticipationRefuse}}}}}})
	if err == nil {
		t.Fatal("a normalized valid refusal bypassed withdrawal of the old combat declaration")
	}
}
