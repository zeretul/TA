package interactive

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/quest"
)

func echoTestProjection(t *testing.T) (*storyBranchProjection, quest.Definition, quest.Instance) {
	t.Helper()
	d, ok := quest.DefinitionByID("xinghai-a01")
	if !ok {
		t.Fatal("authored source missing")
	}
	c := &d.Choices[0]
	c.Success = "private method that was never disclosed"
	c.SuccessReaction = &quest.EchoReaction{CharacterSourceKey: "echo_person", Statement: "下次我愿先和你核对实物，再商量修理办法。"}
	i := quest.Instance{TaskID: d.ID, TaskVersion: d.Version, Status: quest.StatusCompleted, CompletedObjectiveIDs: []string{c.ObjectiveID}, SourceRefs: map[string]string{c.ObjectiveID: "choice:" + c.ID + ":success"}, Choices: map[string]quest.ChoiceResult{c.ObjectiveID: {ChoiceID: c.ID, Outcome: "success", AtSeconds: 10, EchoDueSeconds: 10 + c.EchoAfterSeconds, EchoResolved: true, EchoEncounterRequired: true}}}
	p := &storyBranchProjection{State: map[string]any{}, Quests: map[string]quest.Instance{d.ID: i}, Clock: &StoryClock{Version: StoryClockVersion, Seconds: 10 + c.EchoAfterSeconds}, Characters: &character.State{Version: character.StateVersion, Profiles: []character.Profile{{ID: "person", StoryID: "story", Name: "本人", SourceKey: "echo_person", Revision: 1}}}}
	applyActorStateOp(p.State, ActorStateOp{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{"person"}})
	return p, d, i
}

func TestQuestEchoAcknowledgementNeedsMaturityAndActualPerson(t *testing.T) {
	p, d, i := echoTestProjection(t)
	request := QuestRequest{TaskID: d.ID, Action: QuestActionFollowUp, ChoiceID: d.Choices[0].ID}
	before, _ := json.Marshal(p)
	good, err := planQuestFollowUp(p, d, i, true, request)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("planning mutated canonical facts")
	}
	if len(good.Ops) != 0 || len(good.Rewards) != 0 || good.ClockAfter != nil {
		t.Fatal("follow-up paid, moved or aged the player")
	}
	r := good.Instance.Choices[d.Choices[0].ObjectiveID]
	if r.EchoAcknowledgedBy != "person" || r.EchoAcknowledgedAtSeconds != p.Clock.Seconds {
		t.Fatal("actual encounter missing")
	}
	for _, test := range []struct {
		name  string
		alter func(*storyBranchProjection, *quest.Instance)
	}{
		{"early", func(p *storyBranchProjection, i *quest.Instance) { p.Clock.Seconds-- }},
		{"not_matured", func(p *storyBranchProjection, i *quest.Instance) {
			r := i.Choices[d.Choices[0].ObjectiveID]
			r.EchoResolved = false
			i.Choices[d.Choices[0].ObjectiveID] = r
		}},
		{"absent", func(p *storyBranchProjection, i *quest.Instance) {
			applyActorStateOp(p.State, ActorStateOp{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{}})
		}},
		{"wrong_person", func(p *storyBranchProjection, i *quest.Instance) { p.Characters.Profiles[0].SourceKey = "someone_else" }},
		{"unconscious", func(p *storyBranchProjection, i *quest.Instance) {
			applyActorStateOp(p.State, ActorStateOp{Op: "set", ActorID: "person", FieldID: xiuxianFieldHealth, Value: float64(0)})
		}},
		{"occupied", func(p *storyBranchProjection, i *quest.Instance) {
			applyActorStateOp(p.State, ActorStateOp{Op: "set", ActorID: "person", FieldID: xiuxianFieldArrangement, Value: xiuxianArrangementTravel})
		}},
		{"already_known", func(p *storyBranchProjection, i *quest.Instance) { *i = good.Instance }},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, d, i := echoTestProjection(t)
			test.alter(p, &i)
			if _, err := planQuestFollowUp(p, d, i, true, request); err == nil {
				t.Fatal("impossible acknowledgement accepted")
			}
		})
	}
	event := questEchoCharacterEvent("main", "source-quest", "now", "turn", good)
	if err := validateCharacterRecordEvent(*event); err != nil {
		t.Fatal(err)
	}
	if err := applyCharacterRecord(p, *event); err != nil {
		t.Fatal(err)
	}
	if len(p.Characters.Matters) != 1 || p.Characters.Matters[0].Kind != character.MatterAttitude || len(p.Characters.Knowledge) != 1 || p.Characters.Knowledge[0].Nature != character.NatureExperienced {
		t.Fatal("source knowledge and directed attitude were not committed")
	}
	if strings.Contains(p.Characters.Knowledge[0].Statement, "private method") || p.Characters.Knowledge[0].LearnedFrom != "" {
		t.Fatal("asking about an old result fabricated a confession of a private method")
	}
	if character.HolderCanKnow(p.Characters.Knowledge[0], "bystander") {
		t.Fatal("another person learned a private report")
	}
	if err := applyCharacterRecord(p, *event); err != nil {
		t.Fatal(err)
	}
	if len(p.Characters.Matters) != 1 || len(p.Characters.Knowledge) != 1 {
		t.Fatal("replay duplicated facts")
	}
}

func TestQuestEchoMaturityDoesNotRevealThePersonalResponse(t *testing.T) {
	p, d, i := echoTestProjection(t)
	c := d.Choices[0]
	view := questStateView(cultivation.ReferenceRules(),
		d, i, *p.Clock)
	if len(view.Consequences) != 1 || view.Choices[0].SuccessEcho != "" || view.Choices[0].SuccessReaction != nil {
		t.Fatal("maturity leaked a remote person's response")
	}
	if questEchoGateOpen(p, &quest.EchoGate{TaskID: d.ID, ChoiceID: c.ID, Outcome: "success", RequiresAcknowledged: true}) {
		t.Fatal("unheard opportunity opened")
	}
	plan, err := planQuestFollowUp(p, d, i, true, QuestRequest{ChoiceID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	p.Quests[d.ID] = plan.Instance
	if !questEchoGateOpen(p, &quest.EchoGate{TaskID: d.ID, ChoiceID: c.ID, Outcome: "success", RequiresAcknowledged: true}) {
		t.Fatal("the actual encounter has no gameplay consumer")
	}
	view = questStateView(cultivation.ReferenceRules(),
		d, plan.Instance, *p.Clock)
	if len(view.Consequences) != 2 {
		t.Fatal("known result did not become visible")
	}
}

func TestQuestEchoIntervalsUseDueOrderAndKeepSourceProjectionImmutable(t *testing.T) {
	p, _, i := echoTestProjection(t)
	p.Quests = map[string]quest.Instance{}
	for taskID, due := range map[string]int64{"z-earlier": 20, "a-later": 40, "m-same": 20} {
		next := cloneQuestInstance(i)
		next.TaskID = taskID
		for objective, r := range next.Choices {
			r.EchoResolved = false
			r.EchoDueSeconds = due
			next.Choices[objective] = r
		}
		p.Quests[taskID] = next
	}
	before := cloneQuestInstances(p.Quests)
	events, head := questEchoEvents(p, "main", "turn", "parent", "now", 40)
	if len(events) != 3 || !reflect.DeepEqual(before, p.Quests) {
		t.Fatal("wrong closure or mutated projection")
	}
	want := []string{"m-same", "z-earlier", "a-later"}
	parent := "parent"
	for n, value := range events {
		event := value.(QuestRecordEvent)
		if event.TaskID != want[n] || event.ParentID != parent {
			t.Fatalf("not time ordered: %+v", event)
		}
		parent = event.ID
	}
	if head != parent {
		t.Fatal("broken event chain")
	}
	for _, value := range events {
		event := value.(QuestRecordEvent)
		p.Quests[event.TaskID] = event.Instance
	}
	again, _ := questEchoEvents(p, "main", "turn", head, "now", 100)
	if len(again) != 0 {
		t.Fatal("a later interval repeated an old maturity")
	}
}

func TestQuestCompletionEchoNeedsRealCompletedSourceAndRecipient(t *testing.T) {
	p, _, _ := echoTestProjection(t)
	d, _ := quest.DefinitionByID(quest.TaskSuQianMaterials)
	p.Characters.Profiles[0].SourceKey = d.CompletionEcho.Reaction.CharacterSourceKey
	i := quest.Instance{TaskID: d.ID, TaskVersion: d.Version, Status: quest.StatusCompleted, SettledRewards: true, SourceRefs: map[string]string{}, CompletionEcho: &quest.EchoResult{AtSeconds: 0, DueSeconds: d.CompletionEcho.AfterSeconds, Resolved: true}}
	for _, objective := range d.Objectives {
		i.CompletedObjectiveIDs = append(i.CompletedObjectiveIDs, objective.ID)
		i.SourceRefs[objective.ID] = "real-source:" + objective.ID
	}
	p.Clock.Seconds = d.CompletionEcho.AfterSeconds
	result, err := planQuestFollowUp(p, d, i, true, QuestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	p.Quests[d.ID] = result.Instance
	if !questEchoGateOpen(p, &quest.EchoGate{TaskID: d.ID, Outcome: "success", RequiresAcknowledged: true}) {
		t.Fatal("ordinary completion had no later consumer")
	}
	delete(i.SourceRefs, d.Objectives[0].ID)
	if _, err := planQuestFollowUp(p, d, i, true, QuestRequest{}); err == nil {
		t.Fatal("a claimed completion fabricated source evidence")
	}
}

func TestSuQianCompletionEchoSurvivesJournalAndDoesNotCreateFreeRewards(t *testing.T) {
	s, id := questTestStory(t)
	questTestApply(t, s, id, QuestRequest{TaskID: quest.TaskSuQianMaterials, Action: QuestActionAccept})
	entered := realmTestApply(t, s, id, RealmRequest{RealmID: "realm.gu_xiu_dong_fu", Action: RealmActionEnter})
	realmTestApply(t, s, id, RealmRequest{InstanceID: entered.InstanceID, Action: RealmActionMove, ConnectionID: "realm.hall_to_plot", DestinationAreaID: "realm.herb_plot"})
	realmTestApply(t, s, id, RealmRequest{InstanceID: entered.InstanceID, Action: RealmActionReward, ContentRefID: "realm.herb_ref"})
	realmTestApply(t, s, id, RealmRequest{InstanceID: entered.InstanceID, Action: RealmActionMove, ConnectionID: "realm.hall_to_plot", DestinationAreaID: "realm.outer_hall"})
	// The ordinary realm route owns exit eligibility; this is not a teleport.
	realmTestApply(t, s, id, RealmRequest{InstanceID: entered.InstanceID, Action: RealmActionMove, ConnectionID: "realm.hall_to_formation", DestinationAreaID: "realm.formation_room"})
	realmTestApply(t, s, id, RealmRequest{InstanceID: entered.InstanceID, Action: RealmActionMove, ConnectionID: "realm.formation_to_alchemy", DestinationAreaID: realmTestAlchemy})
	realmTestApply(t, s, id, RealmRequest{InstanceID: entered.InstanceID, Action: RealmActionMove, ConnectionID: "realm.alchemy_to_passage", DestinationAreaID: realmTestPassage})
	realmTestApply(t, s, id, RealmRequest{InstanceID: entered.InstanceID, Action: RealmActionExit})
	journeyTestApply(t, s, id, JourneyRequest{RouteID: "route_036:reverse", Arrive: true})
	// Re-establish the known person's presence after travel, without giving items.
	if _, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "在坊市与苏浅碰面", Narrative: "你在坊市见到苏浅，准备说明记录。", ActorOps: []ActorStateOp{{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{"su-qian"}}}, TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
		t.Fatal(err)
	}
	paid := questTestApply(t, s, id, QuestRequest{TaskID: quest.TaskSuQianMaterials, Action: QuestActionSettle})
	if paid.Instance.CompletionEcho == nil {
		t.Fatal("ordinary completion did not retain its own source clock")
	}
	beforeCurrency := questTestCurrency(t, s, id)
	due := paid.Instance.CompletionEcho.DueSeconds - paid.Instance.CompletionEcho.AtSeconds
	questWait(t, s, id, "main", due-1)
	request := QuestRequest{BranchID: "main", TaskID: quest.TaskSuQianMaterials, Action: QuestActionFollowUp, CommandID: "actual-revisit", CommandHash: "actual-revisit"}
	if _, err := s.ApplyQuest(id, request); err == nil {
		t.Fatal("premature completion echo accepted")
	}
	questWait(t, s, id, "main", 1)
	before, err := s.CharacterState(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Matters) != 0 {
		t.Fatal("time alone made a personal response known")
	}
	first, err := s.ApplyQuest(id, request)
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.CharacterState(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Matters) != 1 || len(after.Knowledge) != 1 || questTestCurrency(t, s, id) != beforeCurrency {
		t.Fatal("follow-up lost its person or fabricated another reward")
	}
	replay, err := s.ApplyQuest(id, request)
	if err != nil || !replay.Replayed || replay.EventID != first.EventID {
		t.Fatalf("retry did not replay: %v", err)
	}
	again, err := s.CharacterState(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, again) {
		t.Fatal("retry repeated a personal consequence")
	}
	state := questTestState(t, s, id)
	view := questTestTask(t, state, quest.TaskSuQianMaterials)
	if len(view.Consequences) != 1 || view.CompletionEcho.AcknowledgedBy != "su-qian" {
		t.Fatal("known completion not projected")
	}

	// The new contract concerns a second journey. The source journey has already
	// been paid and must not satisfy this freshly agreed visit a second time.
	later := "quest.su_qian_return_source"
	questApplyOn(t, s, id, "main", later, QuestActionOffer, "", "offer-second-visit")
	accepted := questApplyOn(t, s, id, "main", later, QuestActionAccept, "", "accept-second-visit")
	if len(accepted.Instance.CompletedObjectiveIDs) != 0 || accepted.Instance.Status != quest.StatusAccepted {
		t.Fatal("old source visit instantly completed the new agreement")
	}
	if _, err := s.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: later, Action: QuestActionSettle}); err == nil {
		t.Fatal("old journey produced a new payment")
	}
	journeyTestApply(t, s, id, JourneyRequest{RouteID: "route_036:forward", Arrive: true, CommandID: "return-from-market", CommandHash: "return-from-market"})
	journeyTestApply(t, s, id, JourneyRequest{RouteID: "route_036:reverse", Arrive: true, CommandID: "visit-market-again", CommandHash: "visit-market-again"})
	finished := questApplyOn(t, s, id, "main", later, QuestActionSettle, "", "pay-second-visit")
	if !finished.Instance.SettledRewards || questTestCurrency(t, s, id) != beforeCurrency+10 {
		t.Fatal("real second journey did not satisfy its own agreement")
	}
}

func TestQuestEchoMainTurnKeepsAcknowledgementAndFactsOnSameTurn(t *testing.T) {
	p, d, i := echoTestProjection(t)
	input := &TurnWorldActionInput{Action: TurnWorldActionQuest, TaskID: d.ID, QuestAction: QuestActionFollowUp, QuestChoiceID: d.Choices[0].ID}
	planned, err := planQuestFollowUp(p, d, i, true, QuestRequest{ChoiceID: input.QuestChoiceID})
	if err != nil {
		t.Fatal(err)
	}
	events, head, err := turnWorldSettlementEvents(p, StoryDirectorActorStateSystem{}, "main", "turn-state", "now", "actual-turn", input, turnWorldPlan{action: TurnWorldActionQuest, quest: &planned})
	if err != nil || len(events) != 2 {
		t.Fatalf("atomic follow-up: %v %+v", err, events)
	}
	receipt := events[0].(QuestRecordEvent)
	facts := events[1].(CharacterRecordEvent)
	if receipt.TurnID != "actual-turn" || facts.TurnID != receipt.TurnID || receipt.ParentID != "turn-state" || facts.ParentID != receipt.ID || head != facts.ID {
		t.Fatalf("source and acknowledgement lost their main turn: receipt=%+v facts=%+v", receipt, facts)
	}
}

func TestQuestEchoEncounterForkAndRegenerationKeepKnowledgeLocal(t *testing.T) {
	s, id, _, dir := authoredQuestStory(t, "xinghai-689", "朱伯利外环47号减压长廊", 1000, map[string]int{"xinghai-s25": 1})
	questApplyOn(t, s, id, "main", "xinghai-a01", QuestActionAccept, "", "take")
	questApplyOn(t, s, id, "main", "xinghai-a01", QuestActionChoose, "xinghai-a01.c1", "work")
	d, _ := quest.DefinitionByID("xinghai-a01")
	questWait(t, s, id, "main", d.Choices[0].EchoAfterSeconds)
	before, err := s.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	earlier, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: before.CurrentTurn.ID, Title: "Before hearing back"})
	if err != nil {
		t.Fatal(err)
	}
	questTestFollowUp(t, s, id, "main", d.ID, d.Choices[0].ID)
	after, err := s.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	later, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: after.CurrentTurn.ID, Title: "After hearing back"})
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		branch string
		count  int
	}{{earlier.ID, 0}, {later.ID, 1}, {"main", 1}} {
		people, err := s.CharacterState(id, check.branch)
		if err != nil || len(people.Matters) != check.count || len(people.Knowledge) != check.count {
			t.Fatalf("branch knowledge leaked: %s %+v %v", check.branch, people, err)
		}
	}
	window, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", ExpectedParentID: &window.Head, ReplaceTurnID: after.CurrentTurn.ID, User: "暂时不问旧事，聊聊附近的天气", Narrative: "你们只谈了眼前的天气，没有核实旧事。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	cold := NewStore(dir)
	defer cold.Close()
	main, err := cold.CharacterState(id, "main")
	if err != nil || len(main.Matters) != 0 || len(main.Knowledge) != 0 {
		t.Fatalf("replaced encounter left ghost facts: %+v %v", main, err)
	}
	state, err := cold.QuestStateFor(id, "main")
	if err != nil || questAvailable(state, "xinghai-a01-echo-1") {
		t.Fatal("replaced encounter retained an unheard opportunity")
	}
	kept, err := cold.CharacterState(id, later.ID)
	if err != nil || len(kept.Matters) != 1 || len(kept.Knowledge) != 1 {
		t.Fatal("main regeneration rewrote the alternate branch")
	}
}

func TestQuestEchoRegenerationRetainsPersonEstablishedBetweenTurns(t *testing.T) {
	s, id, _, _ := authoredQuestStory(t, "xinghai-689", "朱伯利外环47号减压长廊", 1000, map[string]int{"xinghai-s25": 1})
	questApplyOn(t, s, id, "main", "xinghai-a01", QuestActionAccept, "", "take")
	questApplyOn(t, s, id, "main", "xinghai-a01", QuestActionChoose, "xinghai-a01.c1", "work")
	d, _ := quest.DefinitionByID("xinghai-a01")
	questWait(t, s, id, "main", d.Choices[0].EchoAfterSeconds)
	partner := "window-contact"
	if _, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "回到当事人面前", Narrative: "你来到当事人面前。", ActorOps: []ActorStateOp{{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{partner}}}, TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendCharacterFacts(id, AppendCharacterFactsRequest{BranchID: "main", WorldTime: "当前", Profiles: []character.Profile{{ID: partner, Name: "蒙托", SourceKey: "xinghai-p28"}}, Scene: character.Scene{Present: []string{partner}}, Facts: character.TurnCharacterFacts{Exchanges: []character.ExchangeDraft{{CharacterID: partner, Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate, Text: "我就在这里，慢慢说。"}}}}); err != nil {
		t.Fatal(err)
	}
	request := AppendTurnWithStateRequest{BranchID: "main", User: "核实上次的交付", Narrative: "他查看了原先的记录，说明愿意讨论一次新合作。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: TurnWorldActionQuest, TaskID: d.ID, QuestAction: QuestActionFollowUp, QuestChoiceID: d.Choices[0].ID}}}
	first, _, err := s.AppendTurnWithState(id, request)
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.TurnWorldView(id, "main", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := view.Characters.Profile(partner); !ok || len(view.Characters.Exchanges) != 1 || len(view.Characters.Matters) != 0 {
		t.Fatal("dry-run missed the exact pre-turn window conversation")
	}
	window, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	request.ReplaceTurnID, request.ExpectedParentID = first.ID, &window.Head
	if _, _, err = s.AppendTurnWithState(id, request); err != nil {
		t.Fatalf("replacement lost its actual recipient: %v", err)
	}
	state, err := s.CharacterState(id, "main")
	if err != nil || len(state.Exchanges) != 1 || len(state.Matters) != 1 || len(state.Knowledge) != 1 {
		t.Fatal("replacement lost old speech or repeated its response")
	}
}

func TestQuestEchoActualRetrySupersedesUnheardFailureWithoutErasingHistory(t *testing.T) {
	rules := authoredQuestRules(t, "xinghai-689")
	actor := NewAdventureActor(rules)
	actor.Currency = 500
	p := &storyBranchProjection{State: map[string]any{}, Clock: &StoryClock{Version: StoryClockVersion}, Quests: map[string]quest.Instance{}}
	writeActor := func() {
		for _, op := range adventureActorOps(actor, "") {
			applyActorStateOp(p.State, op)
		}
	}
	writeActor()
	applyActorStateOp(p.State, ActorStateOp{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storySceneLocationField, Value: "老锚机械酒吧与二手船材集市"})
	d, _ := quest.DefinitionByID("xinghai-a02")
	c := &d.Choices[0]
	c.SuccessReaction = &quest.EchoReaction{CharacterSourceKey: "test-person", Statement: "新交付已核验，可以再谈合作。"}
	c.FailureReaction = &quest.EchoReaction{CharacterSourceKey: "test-person", Statement: "先把失败的交付补妥，再谈合作。"}
	prior := quest.Instance{TaskID: d.ID, TaskVersion: d.Version, Status: quest.StatusAccepted}
	req := QuestRequest{TaskID: d.ID, Action: QuestActionChoose, ChoiceID: c.ID}
	failed, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &rules}, d, prior, true, req)
	if err != nil {
		t.Fatal(err)
	}
	r := failed.Instance.Choices[c.ObjectiveID]
	if r.Outcome != "failure" || !r.EchoEncounterRequired {
		t.Fatal("fixture lacks a pending personal failure response")
	}
	r.EchoResolved = true
	failed.Instance.Choices[c.ObjectiveID] = r
	p.Clock.Seconds = r.EchoDueSeconds
	actor.Skills[c.SkillID] = 1
	writeActor()
	recovered, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &rules}, d, failed.Instance, true, req)
	if err != nil {
		t.Fatal(err)
	}
	got := recovered.Instance.Choices[c.ObjectiveID]
	if got.Outcome != "success" || got.EchoResolved || got.Previous == nil || !reflect.DeepEqual(*got.Previous, r) {
		t.Fatal("actual remedy lost the failure history or reused its old response")
	}
	if len(questFollowUps(p, d, recovered.Instance)) != 0 || len(questConsequences(d, recovered.Instance)) != 1 {
		t.Fatal("superseded failure kept a live response or leaked the new one")
	}
	if failed.Instance.Choices[c.ObjectiveID].EchoAcknowledgedBy != "" {
		t.Fatal("retry fabricated a failure conversation")
	}
}

func TestQuestParticipantAcceptsUninitializedNativeTemplateButNotIncapacitatedActor(t *testing.T) {
	p, _, _ := echoTestProjection(t)
	values := map[string]any{}
	for _, f := range AdventureNPCStateFields(AdventureRules{Family: "fantasy"}) {
		values[f.Name] = f.Default
	}
	actors := p.State[actorStateRoot].(map[string]any)
	actors["person"] = map[string]any{"id": "person", "template_id": AdventureNPCTemplateID, "state": values}
	if _, ok := questEchoPartner(p, "echo_person"); !ok {
		t.Fatal("a newly established native NPC was treated as dead before its first clock interval")
	}
	values[xiuxianFieldArrangement] = xiuxianArrangementOccupied
	if _, ok := questEchoPartner(p, "echo_person"); ok {
		t.Fatal("busy participant accepted")
	}
	values[xiuxianFieldArrangement] = "照常"
	values[xiuxianFieldHealthMax] = 100
	if _, ok := questEchoPartner(p, "echo_person"); ok {
		t.Fatal("real zero health accepted as an uninitialized template")
	}
	alive := AdventureActor{Family: "fantasy", Rank: 1, Health: 100, MaxHealth: 100}
	for _, op := range adventureActorOps(alive, "fixture") {
		op.ActorID = "person"
		applyActorStateOp(p.State, op)
	}
	if _, ok := questEchoPartner(p, "echo_person"); !ok {
		t.Fatal("living participant rejected")
	}
	alive.Health = 0
	for _, op := range adventureActorOps(alive, "fixture") {
		op.ActorID = "person"
		applyActorStateOp(p.State, op)
	}
	if _, ok := questEchoPartner(p, "echo_person"); ok {
		t.Fatal("dead participant accepted")
	}
}
