package interactive

import (
	"encoding/json"
	"os"
	"reflect"
	"ta/internal/agents/conversationjournal"
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/exploration"
)

const cooperationFlowPartner = "cooperation-partner"

// Explicit rule actors and encountered profiles are test fixtures, not natural
// NPC acquisition or AI consent. All cooperation below uses the formal turn API.
func cooperationFanrenFlowStory(t *testing.T) (*Store, string) {
	t.Helper()
	system := journeyTestSystem(realmTestOrigin)
	system.WorldContent = authoredWorldContent(t, "fanren-jie")
	for i := range system.InitialActors {
		a := &system.InitialActors[i]
		if a.State == nil {
			a.State = map[string]any{}
		}
		if a.ID == DefaultActorID {
			a.State[xiuxianFieldRealm] = cultivation.StageKey(cultivation.Qi(6))
		}
		if a.ID == DefaultStoryContextActorID {
			a.State[storyScenePresentField] = []any{cooperationFlowPartner}
		}
	}
	system.InitialActors = append(system.InitialActors, ActorStateInitialActor{
		ID: cooperationFlowPartner, Name: "赵灵", TemplateID: ActorStateImportantCharacterTemplateID,
		State: map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(1)), xiuxianFieldArrangement: "未定"},
	})
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "Cooperation formal rule fixture", ActorState: &system,
		PackBinding: &StoryPackBinding{PackID: "fanren-jie", ContentVersion: 3, Scope: "fanren-jie"}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "观察", Narrative: "在坊市外围记录所见。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AppendCharacterFacts(story.ID, AppendCharacterFactsRequest{BranchID: "main", WorldTime: "当前", Profiles: []character.Profile{{ID: cooperationFlowPartner, Name: "赵灵", SourceKey: "npc_zhao_ling", Revision: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

func cooperationFlowRequest(t *testing.T, store *Store, storyID, projectID, instanceID, command, action, decision string, share bool, refs []string) AppendTurnWithStateRequest {
	return cooperationFlowRequestForBranch(t, store, storyID, "main", projectID, instanceID, command, action, decision, share, refs)
}

func cooperationFlowRequestForBranch(t *testing.T, store *Store, storyID, branch, projectID, instanceID, command, action, decision string, share bool, refs []string) AppendTurnWithStateRequest {
	t.Helper()
	text := "我把这次亲查的记录给你看；若你愿意，就一起核对。"
	if !share {
		text = "我只问你是否愿意帮忙，暂不分享调查记录。"
	}
	if action == "work" {
		text = "照已经明确答应的分工，现在一起工作。"
		if share {
			text = "我把亲查的记录交给你看，按已经明确答应的分工一起工作。"
		}
	}
	if action == "report" {
		text = "我只把选中的这一项实际结果告诉你。"
	}
	identity := b1Identity(command, 1)
	intent, err := NewPlayerInputIntent(identity, branch, text)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CommitPlayerInput(storyID, intent); err != nil {
		t.Fatal(err)
	}
	result := &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{
		Action: TurnWorldActionCooperation, SourceText: text,
		Cooperation: &CooperationActionInput{ProjectID: projectID, InstanceID: instanceID, Action: action, ShareDiscovery: share, ReportSourceRefs: refs},
	}}
	if decision != "" {
		result.CharacterFacts = &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{
			CharacterID: cooperationFlowPartner, Speaker: string(character.SpeakerCharacter), Channel: string(character.ChannelScene), AddressedTo: DefaultActorID,
			Text:                "我对这一次核对的答复是：" + decision,
			CooperationDecision: &character.CooperationDecision{ProjectID: projectID, RealmInstanceID: instanceID, Decision: decision},
		}}}
	}
	return AppendTurnWithStateRequest{BranchID: branch, User: text, Narrative: "你们按实际答复处理这一次具体分工。", TurnResult: result,
		AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle}
}

func TestCooperationFlowFanrenFormalConsentWorkUseReport(t *testing.T) {
	store, storyID := cooperationFanrenFlowStory(t)
	const realm = exploration.RealmMountainHerbCave
	const project = "coop.herbcave.notes"
	entered := realmTestApply(t, store, storyID, realmExploreRequest("", realm, RealmActionEnter, "", "enter"))
	realmExploreDo(t, store, storyID, entered.InstanceID, realm, RealmActionSurvey, "survey")
	realmTestApply(t, store, storyID, realmExploreRequest(entered.InstanceID, realm, RealmActionExit, "", "exit"))
	before, _ := store.Snapshot(storyID, "main")
	req := cooperationFlowRequest(t, store, storyID, project, entered.InstanceID, "fanren-coop-work", "work_if_agreed", character.CooperationAccept, true, nil)
	turn, _, err := store.AppendTurnWithState(storyID, req)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := store.Snapshot(storyID, "main")
	state, err := store.CharacterState(storyID, "main")
	if err != nil || len(state.Matters) != 1 || state.Matters[0].Cooperation == nil || state.Matters[0].Cooperation.Phase != character.CooperationWorked {
		t.Fatalf("formal response did not produce work: %#v %v", state, err)
	}
	if SnapshotClockSeconds(after)-SnapshotClockSeconds(before) != 600 || len(state.Knowledge) != 1 || state.Knowledge[0].Nature != character.NatureTold || len(state.Affinities) != 0 {
		t.Fatalf("wrong shared work time/knowledge/reward: before=%#v after=%#v state=%#v", before.Clock, after.Clock, state)
	}
	consentFound := false
	for _, exchange := range state.Exchanges {
		if exchange.ID == state.Matters[0].Cooperation.ConsentExchangeID && exchange.SourceEventID != "" && exchange.TurnID == turn.ID && exchange.CooperationDecision != nil {
			consentFound = true
		}
	}
	if !consentFound {
		t.Fatal("same-turn consent did not bind the actual persisted reply")
	}
	retried, _, err := store.AppendTurnWithState(storyID, req)
	if err != nil || retried.ID != turn.ID {
		t.Fatalf("same command failed to replay: %v", err)
	}
	retrySnapshot, _ := store.Snapshot(storyID, "main")
	if !reflect.DeepEqual(retrySnapshot.State, after.State) || !reflect.DeepEqual(retrySnapshot.Clock, after.Clock) {
		t.Fatal("work retry changed resources or time")
	}
	store = cooperationFlowCold(t, store, storyID)
	cold, _ := store.Snapshot(storyID, "main")
	coldChars, _ := store.CharacterState(storyID, "main")
	if !reflect.DeepEqual(cold.State, after.State) || !reflect.DeepEqual(cold.Clock, after.Clock) || !reflect.DeepEqual(coldChars, state) {
		t.Fatal("cold work replay changed formal facts")
	}
	retried, _, err = store.AppendTurnWithState(storyID, req)
	if err != nil || retried.ID != turn.ID {
		t.Fatalf("cold command retry: %v", err)
	}
	repeated := cooperationFlowRequest(t, store, storyID, project, entered.InstanceID, "already-paid-new-command", "work", "", false, nil)
	if _, _, err := store.AppendTurnWithState(storyID, repeated); err != nil {
		t.Fatal(err)
	}
	repeatedSnapshot, _ := store.Snapshot(storyID, "main")
	repeatedChars, _ := store.CharacterState(storyID, "main")
	if !reflect.DeepEqual(repeatedSnapshot.State, cold.State) || !reflect.DeepEqual(repeatedSnapshot.Clock, cold.Clock) || !reflect.DeepEqual(repeatedChars, coldChars) {
		t.Fatal("new command repeated already-paid work or its knowledge")
	}
	realmTestApply(t, store, storyID, realmExploreRequest(entered.InstanceID, realm, RealmActionEnter, "", "reenter"))
	pressure := realmExploreViewAt(t, store, storyID, "main").Explore.Pressure
	used := realmExploreDo(t, store, storyID, entered.InstanceID, realm, RealmActionApproach, "assisted_slow")
	if used.Instance.ExploreState.Pressure != pressure || used.DurationSeconds != 1200 {
		t.Fatal("assisted slow did not consume its registered time/zero added pressure")
	}
	state, _ = store.CharacterState(storyID, "main")
	if state.Matters[0].Cooperation.Phase != character.CooperationUsed || state.Matters[0].Cooperation.UsedBy != used.EventID || len(state.Knowledge) != 1 {
		t.Fatal("actual approach did not consume the plan, or prematurely told NPC the outcome")
	}
	realmExploreMove(t, store, storyID, entered.InstanceID, realm, "core", "core")
	preserved := realmExploreDo(t, store, storyID, entered.InstanceID, realm, RealmActionResolve, "preserve")
	realmExploreMove(t, store, storyID, entered.InstanceID, realm, "entry", "return-to-entry")
	realmTestApply(t, store, storyID, realmExploreRequest(entered.InstanceID, realm, RealmActionExit, "", "report-exit"))
	beforeReport, _ := store.Snapshot(storyID, "main")
	reportReq := cooperationFlowRequest(t, store, storyID, project, entered.InstanceID, "fanren-coop-report", "report", "", false, []string{"disposition:" + preserved.EventID})
	if _, _, err := store.AppendTurnWithState(storyID, reportReq); err != nil {
		t.Fatal(err)
	}
	afterReport, _ := store.Snapshot(storyID, "main")
	state, _ = store.CharacterState(storyID, "main")
	if len(state.Knowledge) != 2 || len(state.Affinities) != 0 || state.Matters[0].Cooperation.Phase != character.CooperationReported || !reflect.DeepEqual(state.Matters[0].Cooperation.ReportSourceRefs, reportReq.TurnResult.World.Cooperation.ReportSourceRefs) || SnapshotClockSeconds(beforeReport) != SnapshotClockSeconds(afterReport) {
		t.Fatalf("report duplicated rewards/time or shared unselected facts: %#v", state)
	}
}

func cooperationAdventureFlowStory(t *testing.T, projectID string, wire, energy int) (*Store, string, CooperationProjectDefinition) {
	return cooperationAdventureFlowStoryOptions(t, projectID, wire, energy, true)
}

func cooperationAdventureFlowStoryOptions(t *testing.T, projectID string, wire, energy int, establishedActor bool) (*Store, string, CooperationProjectDefinition) {
	t.Helper()
	d, ok := cooperationProject(projectID)
	if !ok {
		t.Fatal("missing authored cooperation project")
	}
	def, _ := exploration.RealmDefinition(d.RealmID)
	rules := authoredQuestRules(t, def.PackageID)
	player := NewAdventureActor(rules)
	if d.PlayerItemID != "" {
		delete(player.Inventory, d.PlayerItemID)
		if wire > 0 {
			player.Inventory[d.PlayerItemID] = wire
		}
	}
	var npc AdventureActor
	for _, candidate := range rules.NPCs {
		if candidate.CharacterSourceKey == d.PartnerSourceKey {
			npc = candidate.Actor
			npc.SourceKey = candidate.CharacterSourceKey
		}
	}
	if npc.SourceKey == "" {
		t.Fatal("missing actual frozen NPC definition")
	}
	if energy >= 0 {
		npc.Energy = energy
	}
	values := func(actor AdventureActor) map[string]any {
		state := map[string]any{}
		for _, op := range adventureActorOps(actor, "explicit-rule-fixture") {
			state[op.FieldID] = op.Value
		}
		return state
	}
	system := StoryDirectorActorStateSystem{WorldContent: rules.world, AdventureRules: &rules,
		Templates: []ActorStateTemplate{
			{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(rules)},
			{ID: AdventureNPCTemplateID, Name: "已认识人物", Fields: AdventureNPCStateFields(rules)},
			{ID: DefaultStoryContextActorID, Name: "场景", Fields: []ActorStateField{
				{Name: storySceneLocationField, Type: "string", Default: journeyPlaceLabel(d.LocationID)},
				listStateField("scene.present_actors", storyScenePresentField, "Present people", "当前场景"),
			}},
		}, InitialActors: []ActorStateInitialActor{
			{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: values(player)},
			{ID: cooperationFlowPartner, Name: d.PartnerSourceKey, TemplateID: AdventureNPCTemplateID, State: values(npc)},
			{ID: DefaultStoryContextActorID, Name: "场景", TemplateID: DefaultStoryContextActorID, State: map[string]any{
				storySceneLocationField: journeyPlaceLabel(d.LocationID), storyScenePresentField: []any{cooperationFlowPartner},
			}},
		}}
	if !establishedActor {
		system.InitialActors = append(system.InitialActors[:1], system.InitialActors[2:]...)
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "Authored NPC cooperation rule fixture", ActorState: &system,
		PackBinding: &StoryPackBinding{PackID: def.PackageID, ContentVersion: 3, Scope: def.PackageID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "确认所在位置", Narrative: "你在外部地点停下。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
		t.Fatal(err)
	}
	if establishedActor {
		if _, err := store.AppendCharacterFacts(story.ID, AppendCharacterFactsRequest{BranchID: "main", WorldTime: "当前", Profiles: []character.Profile{{ID: cooperationFlowPartner, Name: d.PartnerSourceKey, SourceKey: d.PartnerSourceKey, Revision: 1}}}); err != nil {
			t.Fatal(err)
		}
	}
	return store, story.ID, d
}

func cooperationFlowPresence(t *testing.T, store *Store, id string, present bool) {
	t.Helper()
	list := []any{}
	if present {
		list = append(list, cooperationFlowPartner)
	}
	if _, err := store.AppendStateDelta(id, AppendStateDeltaRequest{BranchID: "main", ActorOps: []ActorStateOp{{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: list}}}); err != nil {
		t.Fatal(err)
	}
}

func cooperationFlowSpaceJourney(t *testing.T, store *Store, id string, toBay bool, command string) {
	t.Helper()
	route := "xinghai-route-l05-l01"
	want := "xinghai-l01"
	if toBay {
		route, want = "xinghai-route-l01-l05", "xinghai-l05"
		cooperationFlowPresence(t, store, id, false)
	}
	before := worldFlowClock(t, store, id).Seconds
	journeyTestApply(t, store, id, JourneyRequest{BranchID: "main", RouteID: route, Arrive: true, CommandID: command, CommandHash: command})
	after, _ := store.Snapshot(id, "main")
	location, _ := storyLocationID(after.State)
	if location != want || SnapshotClockSeconds(after)-before != 900 || npcGrowthSharesScene(after.State, cooperationFlowPartner) {
		t.Fatalf("journey skipped its formal route or brought the NPC along: place=%s seconds=%d", location, SnapshotClockSeconds(after)-before)
	}
	if !toBay {
		cooperationFlowPresence(t, store, id, true)
	}
}

func TestCooperationFlowAdventureRealParticipantCostsAndReport(t *testing.T) {
	for _, projectID := range []string{"coop.tideboat.current", "coop.servicebay.markers"} {
		t.Run(projectID, func(t *testing.T) {
			store, storyID, d := cooperationAdventureFlowStory(t, projectID, 2, -1)
			space := d.PlayerItemID != ""
			if space {
				cooperationFlowSpaceJourney(t, store, storyID, true, "walk-to-bay-for-survey")
			}
			entered := realmTestApply(t, store, storyID, realmExploreRequest("", d.RealmID, RealmActionEnter, "", "enter"))
			realmExploreDo(t, store, storyID, entered.InstanceID, d.RealmID, RealmActionSurvey, "survey")
			realmTestApply(t, store, storyID, realmExploreRequest(entered.InstanceID, d.RealmID, RealmActionExit, "", "exit"))
			if space {
				cooperationFlowSpaceJourney(t, store, storyID, false, "walk-back-to-silva")
			}
			before, _ := store.Snapshot(storyID, "main")
			beforeNPC, _ := AdventureActorFromState(before.State, cooperationFlowPartner)
			beforePlayer, _ := AdventureActorFromState(before.State, DefaultActorID)
			req := cooperationFlowRequest(t, store, storyID, d.ID, entered.InstanceID, "cooperation-work", "work_if_agreed", character.CooperationAccept, true, nil)
			if _, _, err := store.AppendTurnWithState(storyID, req); err != nil {
				t.Fatal(err)
			}
			after, _ := store.Snapshot(storyID, "main")
			afterNPC, _ := AdventureActorFromState(after.State, cooperationFlowPartner)
			afterPlayer, _ := AdventureActorFromState(after.State, DefaultActorID)
			beforeNPC.Energy -= 6
			if space {
				beforePlayer.Inventory[d.PlayerItemID]--
			}
			if !cooperationFlowActorEqual(beforeNPC, afterNPC) || !cooperationFlowActorEqual(beforePlayer, afterPlayer) || SnapshotClockSeconds(after)-SnapshotClockSeconds(before) != 600 {
				t.Fatalf("work charged the wrong owner or participant growth/refill offset the cost:\nNPC want=%#v got=%#v\nplayer want=%#v got=%#v\ntime=%d", beforeNPC, afterNPC, beforePlayer, afterPlayer, SnapshotClockSeconds(after)-SnapshotClockSeconds(before))
			}
			if space {
				cooperationFlowSpaceJourney(t, store, storyID, true, "walk-to-bay-use-plan")
			}
			realmTestApply(t, store, storyID, realmExploreRequest(entered.InstanceID, d.RealmID, RealmActionEnter, "", "reenter"))
			pressure := realmExploreViewAt(t, store, storyID, "main").Explore.Pressure
			used := realmExploreDo(t, store, storyID, entered.InstanceID, d.RealmID, RealmActionApproach, "assisted_slow")
			if used.Instance.ExploreState.Pressure != pressure || used.DurationSeconds != 1200 {
				t.Fatal("assisted approach did not use the existing method consumer")
			}
			realmExploreMove(t, store, storyID, entered.InstanceID, d.RealmID, "core", "core")
			resolved := realmExploreDo(t, store, storyID, entered.InstanceID, d.RealmID, RealmActionResolve, "strip")
			realmExploreMove(t, store, storyID, entered.InstanceID, d.RealmID, "entry", "back-to-entry")
			realmTestApply(t, store, storyID, realmExploreRequest(entered.InstanceID, d.RealmID, RealmActionExit, "", "exit-after-strip"))
			if space {
				cooperationFlowSpaceJourney(t, store, storyID, false, "walk-back-to-report")
			}
			beforeReport, _ := store.Snapshot(storyID, "main")
			reportReq := cooperationFlowRequest(t, store, storyID, d.ID, entered.InstanceID, "cooperation-report", "report", "", false, []string{"disposition:" + resolved.EventID})
			if _, _, err := store.AppendTurnWithState(storyID, reportReq); err != nil {
				t.Fatal(err)
			}
			afterReport, _ := store.Snapshot(storyID, "main")
			state, _ := store.CharacterState(storyID, "main")
			if len(state.Matters) != 1 || state.Matters[0].Cooperation.UsedBy != used.EventID || len(state.Knowledge) != 2 || len(state.Affinities) != 0 || SnapshotClockSeconds(beforeReport) != SnapshotClockSeconds(afterReport) {
				t.Fatalf("report leaked unselected knowledge or changed costs: %#v", state)
			}
			playerBefore, _ := AdventureActorFromState(beforeReport.State, DefaultActorID)
			playerAfter, _ := AdventureActorFromState(afterReport.State, DefaultActorID)
			if !reflect.DeepEqual(playerBefore, playerAfter) {
				t.Fatal("report awarded new resources or changed the player's state")
			}
		})
	}
}

// The reward consumer allocates Acquired even on a pure cost. Treat nil and
// empty only for this bookkeeping map; every actual resource remains compared.
func cooperationFlowActorEqual(a, b AdventureActor) bool {
	if a.Acquired == nil {
		a.Acquired = map[string]bool{}
	}
	if b.Acquired == nil {
		b.Acquired = map[string]bool{}
	}
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

func cooperationFlowCold(t *testing.T, store *Store, storyID string) *Store {
	t.Helper()
	root, path := store.Root(), store.storyPath(storyID)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(conversationjournal.SidecarPath(path)); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	t.Cleanup(func() { _ = reopened.Close() })
	return reopened
}

func TestCooperationFlowForkInheritsOnlyCheckpointPlanAndUsesIndependently(t *testing.T) {
	store, id := cooperationFanrenFlowStory(t)
	d, _ := cooperationProject("coop.herbcave.notes")
	instance := cooperationFlowSurveyExit(t, store, id, d)
	// Standalone window actions have no TurnID. Commit a real turn after them
	// before choosing a historical turn checkpoint for the pre-work branch.
	if _, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "记下调查后离开", Narrative: "你在外面收好调查记录。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
		t.Fatal(err)
	}
	before, _ := store.Snapshot(id, "main")
	beforeFork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: before.Turns[len(before.Turns)-1].ID, Title: "Before cooperation"})
	if err != nil {
		t.Fatal(err)
	}
	req := cooperationFlowRequest(t, store, id, d.ID, instance, "branch-source-work", "work_if_agreed", character.CooperationAccept, true, nil)
	work, _, err := store.AppendTurnWithState(id, req)
	if err != nil {
		t.Fatal(err)
	}
	sourceChars, _ := store.CharacterState(id, "main")
	afterFork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: work.ID, Title: "Inherit completed work"})
	if err != nil {
		t.Fatal(err)
	}
	empty, _ := store.CharacterState(id, beforeFork.ID)
	if len(empty.Matters) != 0 || len(empty.Knowledge) != 0 {
		t.Fatal("a parallel earlier branch learned later work")
	}
	inherited, _ := store.CharacterState(id, afterFork.ID)
	if len(inherited.Matters) != 1 || inherited.Matters[0].BranchID != afterFork.ID || inherited.Matters[0].Cooperation.WorkSourceRef != sourceChars.Matters[0].Cooperation.WorkSourceRef || inherited.Matters[0].Cooperation.ConsentExchangeID != sourceChars.Matters[0].Cooperation.ConsentExchangeID {
		t.Fatal("explicit fork did not preserve paid work and real sources")
	}
	apply := func(branch, action, command string) RealmResult {
		request := realmExploreRequest(instance, d.RealmID, action, "", command)
		request.BranchID = branch
		if action == RealmActionApproach {
			request.ApproachID = d.assistedApproachID()
		}
		result, err := store.ApplyRealm(id, request)
		if err != nil {
			t.Fatalf("%s branch=%s: %v", command, branch, err)
		}
		return result
	}
	// Earlier fork remains a solo route: the later main-branch plan cannot authorize it.
	apply(beforeFork.ID, RealmActionEnter, "before-enter")
	denied := realmExploreRequest(instance, d.RealmID, RealmActionApproach, d.assistedApproachID(), "before-cannot-use")
	denied.BranchID = beforeFork.ID
	if _, err := store.ApplyRealm(id, denied); err == nil {
		t.Fatal("earlier branch used future work")
	}
	apply(afterFork.ID, RealmActionEnter, "after-enter")
	forkBefore, _ := store.Snapshot(id, afterFork.ID)
	used := apply(afterFork.ID, RealmActionApproach, "after-use")
	forkAfter, _ := store.Snapshot(id, afterFork.ID)
	if SnapshotClockSeconds(forkAfter)-SnapshotClockSeconds(forkBefore) != 1200 {
		t.Fatal("inherited plan charged work a second time")
	}
	actual, _ := store.CharacterState(id, afterFork.ID)
	main, _ := store.CharacterState(id, "main")
	if actual.Matters[0].Cooperation.Phase != character.CooperationUsed || actual.Matters[0].Cooperation.UsedBy != used.EventID || !reflect.DeepEqual(main, sourceChars) {
		t.Fatal("branch plan use changed main or did not bind actual realm source")
	}
	anchor, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: afterFork.ID, User: "记下已用走法", Narrative: "你记住刚才的换位。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	usedFork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: anchor.ID, Title: "After plan use"})
	if err != nil {
		t.Fatal(err)
	}
	usedState, _ := store.CharacterState(id, usedFork.ID)
	if usedState.Matters[0].Cooperation.Phase != character.CooperationUsed || usedState.Matters[0].Cooperation.UsedBy != used.EventID {
		t.Fatal("fork reset an already spent plan")
	}
	denied = realmExploreRequest(instance, d.RealmID, RealmActionApproach, d.assistedApproachID(), "used-cannot-repeat")
	denied.BranchID = usedFork.ID
	if _, err := store.ApplyRealm(id, denied); err == nil {
		t.Fatal("already-used inherited plan was usable again")
	}
	store = cooperationFlowCold(t, store, id)
	for _, branch := range []string{"main", beforeFork.ID, afterFork.ID, usedFork.ID} {
		chars, err := store.CharacterState(id, branch)
		if err != nil {
			t.Fatal(err)
		}
		expected := sourceChars
		switch branch {
		case beforeFork.ID:
			expected = empty
		case afterFork.ID:
			expected = actual
		case usedFork.ID:
			expected = usedState
		}
		if !reflect.DeepEqual(chars, expected) {
			t.Fatalf("cold fork %s changed cooperation facts", branch)
		}
	}
}
