package interactive

import (
	"encoding/json"
	"reflect"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/exploration"
)

// All actors in this file are explicitly qualified rule fixtures. The tests use
// real Story commits and authored routes/costs, but are not natural-AI evidence.
func realmExploreStory(t *testing.T, pack, location string, items map[string]int, skills map[string]int) (*Store, string, string) {
	t.Helper()
	var system StoryDirectorActorStateSystem
	if pack == "fanren-jie" {
		system = journeyTestSystem(location)
		for i := range system.InitialActors {
			if system.InitialActors[i].ID != DefaultActorID {
				continue
			}
			actor := system.InitialActors[i].State
			if actor == nil {
				actor = map[string]any{}
				system.InitialActors[i].State = actor
			}
			actor[xiuxianFieldRealm] = cultivation.StageKey(cultivation.Qi(6))
			held := map[string]any{}
			for id, n := range items {
				held[id] = map[string]any{"定义": id, "数量": float64(n)}
			}
			actor[xiuxianFieldImportantItems] = held
		}
	} else {
		r := authoredQuestRules(t, pack)
		a := NewAdventureActor(r)
		for id, n := range items {
			a.Inventory[id] = n
		}
		for id, n := range skills {
			a.Skills[id] = n
			// Learned skills remain usable after the teaching source is gone.
			// The source is a learning prerequisite, not an invocation tool.
			for _, skill := range r.Skills {
				if skill.ID == id && skill.RequiredItem != "" {
					delete(a.Inventory, skill.RequiredItem)
				}
			}
		}
		values := map[string]any{}
		for _, op := range adventureActorOps(a, "fixture") {
			values[op.FieldID] = op.Value
		}
		system = StoryDirectorActorStateSystem{WorldContent: r.world, AdventureRules: &r, Templates: []ActorStateTemplate{
			{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(r)},
			{ID: DefaultStoryContextActorID, Name: "场景", Fields: []ActorStateField{{Name: storySceneLocationField, Type: "string", Default: location}}},
		}, InitialActors: []ActorStateInitialActor{
			{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: values},
			{ID: DefaultStoryContextActorID, Name: "场景", TemplateID: DefaultStoryContextActorID, State: map[string]any{storySceneLocationField: location}},
		}}
	}
	system.WorldContent = authoredWorldContent(t, pack)
	dir := t.TempDir()
	s := NewStore(dir)
	t.Cleanup(func() { _ = s.Close() })
	story, err := s.CreateStory(CreateStoryRequest{Title: "Exploration rule fixture", ActorState: &system, PackBinding: &StoryPackBinding{PackID: pack, ContentVersion: 3, Scope: pack}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "观察", Narrative: "确认所在位置与所携物品。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
		t.Fatal(err)
	}
	return s, story.ID, dir
}

func realmExploreViewAt(t *testing.T, s *Store, id, branch string) RealmStateView {
	t.Helper()
	state, err := s.RealmStateFor(id, branch)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Instances) != 1 || state.Instances[0].Explore == nil {
		t.Fatalf("expected one explorable place: %+v", state)
	}
	return state.Instances[0]
}

func realmExploreRequest(instance, realm, action, approach, command string) RealmRequest {
	return RealmRequest{BranchID: "main", InstanceID: instance, RealmID: realm, Action: action, ApproachID: approach, CommandID: command, CommandHash: ContentHashOf([]string{instance, realm, action, approach, command})}
}

func realmExploreDo(t *testing.T, s *Store, id, instance, realm, action, method string) RealmResult {
	t.Helper()
	return realmTestApply(t, s, id, realmExploreRequest(instance, realm, action, realm+"."+method, method))
}

func realmExploreMove(t *testing.T, s *Store, id, instance, realm, area, command string) RealmResult {
	t.Helper()
	return realmTestApply(t, s, id, RealmRequest{InstanceID: instance, Action: RealmActionMove, DestinationAreaID: realm + "." + area, CommandID: command, CommandHash: command})
}

func realmExploreQuantity(t *testing.T, s *Store, id, branch, pack, item string) int {
	t.Helper()
	if pack != "fanren-jie" {
		w, err := s.AdventureWindow(id, branch)
		if err != nil {
			t.Fatal(err)
		}
		return w.Actor.Inventory[item]
	}
	n := 0
	for key, value := range realmTestHeldItems(t, s, id, branch) {
		entry, _ := value.(map[string]any)
		if key == item || entry["定义"] == item {
			n += xiuxianCarriedQuantity(entry)
		}
	}
	return n
}

func TestRealmExploreRealCostsAndIdempotency(t *testing.T) {
	cases := []struct {
		name, pack, realm, location, method, item, skill string
		consume, energy                                  int
	}{
		{"fanren ward", "fanren-jie", exploration.RealmMountainHerbCave, realmTestOrigin, "ward", "talisman.ward.lower", "", 1, 0},
		{"wuxia rope held", "lanchuan-qunxia", exploration.RealmStrandedTideBoat, "潮信渡", "rope", "lanchuan-i28", "", 0, 0},
		{"wuxia frozen skill", "lanchuan-qunxia", exploration.RealmStrandedTideBoat, "潮信渡", "boatcraft", "", "lanchuan-s34", 0, 6},
		{"space wire and frozen skill", "xinghai-689", exploration.RealmShaftServiceBay, "废弃47号排污减压井盲端", "repair", "xinghai-i34", "xinghai-s02", 1, 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, skills := map[string]int{}, map[string]int{}
			if tc.item != "" {
				items[tc.item] = 2
			}
			if tc.skill != "" {
				skills[tc.skill] = 1
			}
			s, id, _ := realmExploreStory(t, tc.pack, tc.location, items, skills)
			entered := realmTestApply(t, s, id, realmExploreRequest("", tc.realm, RealmActionEnter, "", "enter"))
			energy := 0
			if tc.pack != "fanren-jie" {
				w, err := s.AdventureWindow(id, "main")
				if err != nil {
					t.Fatal(err)
				}
				energy = w.Actor.Energy
			}
			beforeClock := worldFlowClock(t, s, id).Seconds
			req := realmExploreRequest(entered.InstanceID, tc.realm, "approach", tc.realm+"."+tc.method, "method")
			first := realmTestApply(t, s, id, req)
			if first.DurationSeconds != 600 || worldFlowClock(t, s, id).Seconds-beforeClock != 600 {
				t.Fatalf("method time not committed exactly once: %+v", first)
			}
			if first.Instance.ExploreState == nil || first.Instance.ExploreState.Pressure != 0 {
				t.Fatalf("prepared method spent unexpected pressure: %+v", first.Instance.ExploreState)
			}
			if tc.item != "" && realmExploreQuantity(t, s, id, "main", tc.pack, tc.item) != 2-tc.consume {
				t.Fatal("method did not charge the real inventory, or consumed a held-only tool")
			}
			if tc.pack != "fanren-jie" {
				w, _ := s.AdventureWindow(id, "main")
				if w.Actor.Energy != energy-tc.energy {
					t.Fatalf("frozen skill cost must be %d, energy %d -> %d", tc.energy, energy, w.Actor.Energy)
				}
			}
			before, err := s.Snapshot(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			retry := realmTestApply(t, s, id, req)
			after, err := s.Snapshot(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			if !retry.Replayed || retry.EventID != first.EventID || !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(before.Clock, after.Clock) {
				t.Fatal("same command charged time or resources twice")
			}
			req.CommandID, req.CommandHash = "method-again", "method-again"
			if _, err := s.ApplyRealm(id, req); err == nil {
				t.Fatal("a different command repeated an already completed method")
			}
			req.CommandID, req.CommandHash = "method", "different-content"
			if _, err := s.ApplyRealm(id, req); err == nil {
				t.Fatal("a command identity accepted different content")
			}
		})
	}
}

func TestRealmExploreMissingResourcesRefuseAtomically(t *testing.T) {
	for _, missing := range []string{"skill", "wire", "energy"} {
		t.Run(missing, func(t *testing.T) {
			r := authoredQuestRules(t, "xinghai-689")
			a := NewAdventureActor(r)
			a.Skills["xinghai-s02"], a.Inventory["xinghai-i34"] = 1, 1
			switch missing {
			case "skill":
				delete(a.Skills, "xinghai-s02")
			case "wire":
				delete(a.Inventory, "xinghai-i34")
			case "energy":
				a.Energy = 5
			}
			s, id := adventureFixtureStoryWithActor(t, r, a, "废弃47号排污减压井盲端")
			t.Cleanup(func() { _ = s.Close() })
			entered := realmTestApply(t, s, id, realmExploreRequest("", exploration.RealmShaftServiceBay, RealmActionEnter, "", "enter"))
			before, err := s.Snapshot(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.ApplyRealm(id, realmExploreRequest(entered.InstanceID, exploration.RealmShaftServiceBay, "approach", exploration.RealmShaftServiceBay+".repair", "repair"))
			if err == nil {
				t.Fatalf("repair accepted absent %s", missing)
			}
			after, err := s.Snapshot(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(before.Clock, after.Clock) || realmTestRecordCount(t, s, id, "main") != 1 {
				t.Fatal("rejected method partially charged the actor or journal")
			}
		})
	}
}

func TestRealmExploreFormalTurnClockReturnForkAndColdReplay(t *testing.T) {
	realm := exploration.RealmShaftServiceBay
	s, id, dir := realmExploreStory(t, "xinghai-689", "朱伯利外环47号减压长廊", map[string]int{"xinghai-i34": 1}, map[string]int{"xinghai-s02": 1})
	worldFlowTurn(t, s, id, "去47号废井", "沿正式道路前往废井。", TurnWorldActionInput{Action: TurnWorldActionJourney, RouteID: "xinghai-route-l01-l05", Arrive: true})
	if worldFlowClock(t, s, id).Seconds != 900 {
		t.Fatal("the authored approach road must cost fifteen minutes")
	}
	worldFlowTurn(t, s, id, "进入检修舱", "进入舱门。", TurnWorldActionInput{Action: TurnWorldActionRealm, RealmID: realm, RealmAction: RealmActionEnter})
	entered := realmExploreViewAt(t, s, id, "main")
	worldFlowTurn(t, s, id, "用合金丝飞线抢修后通过", "按学过的手法抢修。", TurnWorldActionInput{Action: TurnWorldActionRealm, InstanceID: entered.InstanceID, RealmAction: "approach", ApproachID: realm + ".repair"})
	if worldFlowClock(t, s, id).Seconds != 1500 || realmExploreQuantity(t, s, id, "main", "xinghai-689", "xinghai-i34") != 0 {
		t.Fatal("formal turn failed to settle exactly one method cost and interval")
	}
	realmExploreMove(t, s, id, entered.InstanceID, realm, "core", "core")
	anchor, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "停下来决定", Narrative: "先看清电源间。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: anchor.ID, Title: "拆料分支"})
	if err != nil {
		t.Fatal(err)
	}
	req := realmExploreRequest(entered.InstanceID, realm, "resolve", realm+".strip", "strip")
	req.BranchID = fork.ID
	strip := realmTestApply(t, s, id, req)
	if len(strip.Rewards) != 1 || realmExploreQuantity(t, s, id, fork.ID, "xinghai-689", "xinghai-i34") != 2 {
		t.Fatal("fork strip did not grant its own real material")
	}
	before := worldFlowClock(t, s, id).Seconds
	worldFlowTurn(t, s, id, "保电完成离线诊断", "保住辅助供电，留下诊断继续运行。", TurnWorldActionInput{Action: TurnWorldActionRealm, InstanceID: entered.InstanceID, RealmAction: "resolve", ApproachID: realm + ".preserve"})
	view := realmExploreViewAt(t, s, id, "main")
	due := view.Explore.ReturnDueSeconds
	if worldFlowClock(t, s, id).Seconds-before != 1200 || due != before+1200+3600 {
		t.Fatalf("return must be measured from completion on the only clock: before=%d due=%d", before, due)
	}
	if realmExploreQuantity(t, s, id, "main", "xinghai-689", "xinghai-i34") != 0 {
		t.Fatal("preserving incorrectly paid the mutually exclusive stripping reward")
	}
	req = realmExploreRequest(entered.InstanceID, realm, "approach", realm+".return", "return-too-soon")
	if _, err = s.ApplyRealm(id, req); err == nil {
		t.Fatal("return route opened before formal time elapsed")
	}
	req = realmExploreRequest(entered.InstanceID, realm, "resolve", realm+".preserve", "fork-preserve")
	req.BranchID = fork.ID
	if _, err = s.ApplyRealm(id, req); err == nil {
		t.Fatal("stripped fork was allowed to preserve the same core")
	}
	parentBefore, _ := s.Snapshot(id, "main")
	beforeReadCount := realmTestRecordCount(t, s, id, "main")
	for i := 0; i < 3; i++ {
		_ = realmExploreViewAt(t, s, id, "main")
	}
	parentAfter, _ := s.Snapshot(id, "main")
	if !reflect.DeepEqual(parentBefore.State, parentAfter.State) || !reflect.DeepEqual(parentBefore.Clock, parentAfter.Clock) || parentBefore.ContextRevision != parentAfter.ContextRevision || !reflect.DeepEqual(parentBefore.Graph, parentAfter.Graph) || realmTestRecordCount(t, s, id, "main") != beforeReadCount {
		t.Fatal("read-only projection advanced clock or changed inventory")
	}
	questWait(t, s, id, "main", due-worldFlowClock(t, s, id).Seconds-1)
	if realmExploreViewAt(t, s, id, "main").Explore.ReturnAvailable {
		t.Fatal("return opened one second early")
	}
	questWait(t, s, id, "main", 1)
	if !realmExploreViewAt(t, s, id, "main").Explore.ReturnAvailable {
		t.Fatal("return did not open at its exact deadline")
	}
	if realmExploreViewAt(t, s, id, fork.ID).Explore.ReturnDueSeconds != 0 {
		t.Fatal("fork inherited a later parent decision or clock")
	}
	preserved, _ := s.RealmStateFor(id, "main")
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(dir)
	defer reopened.Close()
	restored, err := reopened.RealmStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(preserved, restored) {
		t.Fatal("cold reopen changed known paths, deadline or decisions")
	}
	realmExploreDo(t, reopened, id, entered.InstanceID, realm, "approach", "return")
	realmExploreMove(t, reopened, id, entered.InstanceID, realm, "side", "side")
	cache := realmExploreDo(t, reopened, id, entered.InstanceID, realm, "resolve", "cache")
	if len(cache.Rewards) != 1 || realmExploreQuantity(t, reopened, id, "main", "xinghai-689", "xinghai-i37") != 1 {
		t.Fatal("return cache did not land in native inventory exactly once")
	}
	if realmExploreQuantity(t, reopened, id, fork.ID, "xinghai-689", "xinghai-i37") != 0 {
		t.Fatal("parent return cache leaked to fork")
	}
	retry := realmExploreDo(t, reopened, id, entered.InstanceID, realm, "resolve", "cache")
	if !retry.Replayed || !reflect.DeepEqual(cache.Rewards, retry.Rewards) {
		t.Fatal("reward retry lost the committed result")
	}
}

func TestRealmExploreSetbackKeepsEarlierLootAndSafeRetreat(t *testing.T) {
	realm := exploration.RealmMountainHerbCave
	s, id, _ := realmExploreStory(t, "fanren-jie", "公共坊市", nil, nil)
	worldFlowTurn(t, s, id, "去坊市外围", "沿山路前往外围。", TurnWorldActionInput{Action: TurnWorldActionJourney, RouteID: "route_036:forward", Arrive: true})
	entered := realmTestApply(t, s, id, realmExploreRequest("", realm, RealmActionEnter, "", "enter"))
	outer := realmExploreDo(t, s, id, entered.InstanceID, realm, "resolve", "outer")
	if len(outer.Rewards) != 1 || realmExploreQuantity(t, s, id, "main", "fanren-jie", "material.herb.green_vine") != 1 {
		t.Fatalf("outer loot must be exactly one in the real pack: rewards=%+v pack=%+v", outer.Rewards, realmTestHeldItems(t, s, id, "main"))
	}
	realmExploreDo(t, s, id, entered.InstanceID, realm, "survey", "survey")
	realmExploreDo(t, s, id, entered.InstanceID, realm, "approach", "slow")
	realmExploreMove(t, s, id, entered.InstanceID, realm, "core", "core")
	view := realmExploreViewAt(t, s, id, "main")
	warned := false
	for _, method := range view.Explore.Approaches {
		if method.ID == realm+".preserve" {
			warned = method.CausesSetback
		}
	}
	if !warned {
		t.Fatal("the offered choice failed to disclose the pressure-limit setback")
	}
	result := realmExploreDo(t, s, id, entered.InstanceID, realm, "resolve", "preserve")
	if !result.Instance.ExploreState.Setback || result.Instance.ExploreState.Pressure != 4 || result.Instance.ExploreState.ReturnDueSeconds != 0 || len(result.Rewards) != 0 {
		t.Fatalf("pressure limit was presented as core success: %+v", result)
	}
	realmExploreMove(t, s, id, entered.InstanceID, realm, "entry", "retreat")
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, realm, RealmActionExit, "", "exit"))
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, realm, RealmActionReenter, "", "reenter"))
	if view = realmExploreViewAt(t, s, id, "main"); view.Explore.Pressure != 4 || !view.Explore.Setback {
		t.Fatal("safe retreat/re-entry reset the place's pressure or setback")
	}
	if realmExploreQuantity(t, s, id, "main", "fanren-jie", "material.herb.green_vine") != 1 {
		t.Fatal("setback removed loot already carried away")
	}
	residue := realmExploreDo(t, s, id, entered.InstanceID, realm, "resolve", "residue")
	if len(residue.Rewards) != 1 || realmExploreQuantity(t, s, id, "main", "fanren-jie", "material.stone.ward") != 1 {
		t.Fatal("setback residue was not a finite real reward")
	}
	req := realmExploreRequest(entered.InstanceID, realm, "resolve", realm+".residue", "residue-again")
	if _, err := s.ApplyRealm(id, req); err == nil {
		t.Fatal("setback residue can be farmed under different commands")
	}
}

func TestRealmExploreSameTurnAcquireCannotInventEligibility(t *testing.T) {
	realm := exploration.RealmMountainHerbCave
	s, id, _ := realmExploreStory(t, "fanren-jie", realmTestOrigin, nil, nil)
	entered := realmTestApply(t, s, id, realmExploreRequest("", realm, RealmActionEnter, "", "enter"))
	before, err := s.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "用护身符穿过去", Narrative: "忽然找到一张护身符并用它开路。", TurnResult: &TurnResult{
		Choices: testTurnChoices(),
		Items:   &TurnItemsInput{Acquire: []TurnItemAcquire{{DefID: "talisman.ward.lower", Quantity: 1, Source: "临时声称在脚边捡到"}}},
		World:   &TurnWorldActionInput{Action: TurnWorldActionRealm, InstanceID: entered.InstanceID, RealmAction: "approach", ApproachID: realm + ".ward"},
	}})
	if err == nil {
		t.Fatal("a turn manufactured its own consumable eligibility through Acquire")
	}
	after, readErr := s.Snapshot(id, "main")
	if readErr != nil {
		t.Fatal(readErr)
	}
	a, _ := json.Marshal(before.State)
	b, _ := json.Marshal(after.State)
	if string(a) != string(b) || !reflect.DeepEqual(before.Clock, after.Clock) || realmTestRecordCount(t, s, id, "main") != 1 {
		t.Fatal("refused self-qualification partially changed inventory, time or realm")
	}
}
