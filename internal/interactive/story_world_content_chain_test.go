package interactive

import (
	"reflect"
	"testing"

	"ta/internal/quest"
)

// These fixtures deliberately seed preparation. They verify authored consumers,
// not natural acquisition, NPC improvisation, or a complete campaign playthrough.
func worldContentPreparedStory(t *testing.T, pack, location string, skills map[string]int, items map[string]int) (*Store, string, string) {
	t.Helper()
	r := authoredQuestRules(t, pack)
	r.StartingCurrency = 1000
	a := NewAdventureActor(r)
	for skill, rank := range skills {
		a.Skills[skill] = rank
	}
	for item, quantity := range items {
		a.Inventory[item] = quantity
	}
	initial := map[string]any{}
	for _, op := range adventureActorOps(a, "") {
		initial[op.FieldID] = op.Value
	}
	place := journeyPlaceLabel(location)
	system := StoryDirectorActorStateSystem{WorldContent: r.world, AdventureRules: &r, Templates: []ActorStateTemplate{{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(r)}, {ID: DefaultStoryContextActorID, Name: "场景", Fields: []ActorStateField{{Name: storySceneLocationField, Type: "string", Default: place}}}}, InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: initial}, {ID: DefaultStoryContextActorID, Name: "场景", TemplateID: DefaultStoryContextActorID, State: map[string]any{storySceneLocationField: place}}}}
	dir := t.TempDir()
	s := NewStore(dir)
	t.Cleanup(func() { _ = s.Close() })
	story, err := s.CreateStory(CreateStoryRequest{Title: "地方悬念规则切片", ActorState: &system, PackBinding: &StoryPackBinding{PackID: pack, ContentVersion: 4, Scope: pack}})
	if err != nil {
		t.Fatal(err)
	}
	id := story.ID
	if _, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "留意当地消息", Narrative: "在当地看到这一次事件的公开线索。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
		t.Fatal(err)
	}
	return s, id, dir
}

func worldContentActor(t *testing.T, s *Store, id, branch string) AdventureActor {
	t.Helper()
	snapshot, err := s.Snapshot(id, branch)
	if err != nil {
		t.Fatal(err)
	}
	a, err := AdventureActorFromState(snapshot.State, DefaultActorID)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func worldContentRejectWithoutCharge(t *testing.T, s *Store, id, branch, task, choice string) {
	t.Helper()
	before, err := s.Snapshot(id, branch)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ApplyQuest(id, QuestRequest{BranchID: branch, TaskID: task, Action: QuestActionChoose, ChoiceID: choice})
	if err == nil {
		t.Fatal("unearned authored stage was accepted")
	}
	after, err := s.Snapshot(id, branch)
	if err != nil || !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(before.Clock, after.Clock) {
		t.Fatal("rejected stage changed resources or time", err)
	}
}

func TestWorldContentTideLampStagesTravelAndDistinctResolutions(t *testing.T) {
	const task = "lanchuan-tide-lamp"
	s, id, dir := worldContentPreparedStory(t, "lanchuan-qunxia", "lanchuan-l04", map[string]int{"lanchuan-s36": 1}, map[string]int{"lanchuan-steel": 1})
	state, err := s.QuestStateFor(id, "main")
	if err != nil || !questAvailable(state, task) {
		t.Fatal("local source did not expose its authored opportunity", err)
	}
	questApplyOn(t, s, id, "main", task, QuestActionAccept, "", "take")
	worldContentRejectWithoutCharge(t, s, id, "main", task, task+".repair")
	// Technical observation really needs the learned skill and hooked rope.
	worldContentRejectWithoutCharge(t, s, id, "main", task, task+".survey")
	observed := questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".watch", "watch")
	if observed.Instance.Status != quest.StatusAccepted || worldContentActor(t, s, id, "main").Currency != 920 {
		t.Fatal("paid observation did not remain partial or charge its actual fee")
	}
	worldContentRejectWithoutCharge(t, s, id, "main", task, task+".compare")
	for _, route := range []string{"lanchuan-route-l04-l01", "lanchuan-route-l01-l02", "lanchuan-route-l02-l05"} {
		if result, err := s.ApplyJourney(id, JourneyRequest{BranchID: "main", RouteID: route, Arrive: true}); err != nil || !result.Arrived {
			t.Fatal("real route to the cliff did not arrive", route, err)
		}
	}
	comparison := questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".compare", "compare")
	if comparison.Instance.Status != quest.StatusAccepted {
		t.Fatal("finding the cause falsely completed the repair")
	}
	decision, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "看看已经查明的原因，再决定怎么处理", Narrative: "漏壶误差已经查明，修复和警戒是不同的安排。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: decision.ID, Title: "只留警戒牌"})
	if err != nil {
		t.Fatal(err)
	}
	before := worldContentActor(t, s, id, "main")
	repaired := questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".repair", "repair")
	marked := questApplyOn(t, s, id, fork.ID, task, QuestActionChoose, task+".mark", "mark")
	if repaired.Instance.Status != quest.StatusCompleted || marked.Instance.Status != quest.StatusCompleted {
		t.Fatal("a complete evidence-and-response chain remained unfinished")
	}
	main := worldContentActor(t, s, id, "main")
	other := worldContentActor(t, s, id, fork.ID)
	if main.Currency != before.Currency+160 || main.Inventory["lanchuan-steel"] != 0 || main.Energy != before.Energy-6 {
		t.Fatalf("repair did not consume real preparation and pay once: before=%+v after=%+v", before, main)
	}
	if other.Currency != before.Currency-40 || other.Inventory["lanchuan-steel"] != 1 || other.Energy != before.Energy {
		t.Fatal("warning path secretly repaired equipment or charged the other branch")
	}
	worldContentRejectWithoutCharge(t, s, id, "main", task, task+".mark")
	retry := questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".repair", "repair")
	if !retry.Replayed || retry.EventID != repaired.EventID || !reflect.DeepEqual(main, worldContentActor(t, s, id, "main")) {
		t.Fatal("retry refreshed the repair or reward")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	cold := NewStore(dir)
	defer cold.Close()
	if !reflect.DeepEqual(main, worldContentActor(t, cold, id, "main")) || !reflect.DeepEqual(other, worldContentActor(t, cold, id, fork.ID)) {
		t.Fatal("cold load lost a distinct local consequence")
	}
	t.Log("paid observation -> actual multi-leg journey -> mechanical explanation -> repair or warning; costs, completed stages, retry and cold branches verified")
}

func TestWorldContentRepeatingBeaconPreservationAndRecovery(t *testing.T) {
	const task = "xinghai-repeating-beacon"
	s, id, dir := worldContentPreparedStory(t, "xinghai-689", "xinghai-l21", map[string]int{"xinghai-s02": 1}, map[string]int{"xinghai-i37": 1})
	questApplyOn(t, s, id, "main", task, QuestActionAccept, "", "take")
	worldContentRejectWithoutCharge(t, s, id, "main", task, task+".repair")
	worldContentRejectWithoutCharge(t, s, id, "main", task, task+".measure")
	questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".listen", "timing")
	worldContentRejectWithoutCharge(t, s, id, "main", task, task+".trace")
	trip, err := s.ApplyJourney(id, JourneyRequest{BranchID: "main", RouteID: "xinghai-route-l21-l22", Arrive: true})
	if err != nil || !trip.Arrived || trip.ElapsedSeconds != 3600 {
		t.Fatal("outpost arrival was not a paid formal route", err)
	}
	questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".trace", "trace")
	decision, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "原因弄清了，考虑接下来怎样处理", Narrative: "缓存误回放的来源已查明，中继尚未修复。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: decision.ID, Title: "保留记录，隔离缓存"})
	if err != nil {
		t.Fatal(err)
	}
	before := worldContentActor(t, s, id, "main")
	repaired := questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".repair", "repair")
	recovered := questApplyOn(t, s, id, fork.ID, task, QuestActionChoose, task+".isolate", "isolate")
	main := worldContentActor(t, s, id, "main")
	other := worldContentActor(t, s, id, fork.ID)
	if repaired.Instance.Status != quest.StatusCompleted || recovered.Instance.Status != quest.StatusCompleted || main.Currency != 1140 || other.Currency != 880 {
		t.Fatalf("different settlements did not produce actual balances: repair=%d recovery=%d", main.Currency, other.Currency)
	}
	if main.Inventory["xinghai-i37"] != 0 || other.Inventory["xinghai-i37"] != 1 || main.Energy != before.Energy-6 || other.Inventory["xinghai-i38"] != before.Inventory["xinghai-i38"]+1 || main.Inventory["xinghai-i38"] != before.Inventory["xinghai-i38"] {
		t.Fatal("preservation and salvage lost their resource difference")
	}
	worldContentRejectWithoutCharge(t, s, id, fork.ID, task, task+".repair")
	replay := questApplyOn(t, s, id, fork.ID, task, QuestActionChoose, task+".isolate", "isolate")
	if !replay.Replayed || replay.EventID != recovered.EventID || !reflect.DeepEqual(other, worldContentActor(t, s, id, fork.ID)) {
		t.Fatal("replayed recovery duplicated its capacitor")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	cold := NewStore(dir)
	defer cold.Close()
	if !reflect.DeepEqual(main, worldContentActor(t, cold, id, "main")) || !reflect.DeepEqual(other, worldContentActor(t, cold, id, fork.ID)) {
		t.Fatal("cold load changed the authored outcome")
	}
	t.Log("two-station timing -> actual outpost travel -> stale-cache explanation -> preserve relay or recover one capacitor; no duplicate salvage")
}

func TestWorldContentRevisionKeepsOldWorkWithoutInjectingNewAffairsIntoOldStories(t *testing.T) {
	for _, tc := range []struct {
		pack, oldTask, newTask string
		previous, current      int
	}{
		{"fanren-jie", quest.TaskHerbHandover, "quest.su_qian_return_source", 29, 30},
		{"lanchuan-qunxia", "lanchuan-training-s34", "lanchuan-tide-lamp", 3, 4},
		{"xinghai-689", "xinghai-training-s03", "xinghai-repeating-beacon", 3, 4},
	} {
		t.Run(tc.pack, func(t *testing.T) {
			old, ok := quest.DefinitionByID(tc.oldTask)
			if !ok {
				t.Fatal("existing task is missing")
			}
			fresh, ok := quest.DefinitionByID(tc.newTask)
			if !ok {
				t.Fatal("new task is missing")
			}
			for _, revision := range []int{tc.previous, tc.current} {
				content := &WorldContent{PackID: tc.pack, Quests: []quest.Definition{old}}
				if revision == tc.current {
					content.Quests = append(content.Quests, fresh)
				}
				meta := StoryMeta{PackBinding: &StoryPackBinding{PackID: tc.pack, ContentVersion: revision}, ActorStateSchema: &ActorStateSchemaSnapshot{System: StoryDirectorActorStateSystem{WorldContent: content}}}
				if _, ok := StoryQuestDefinition(meta, old.ID); !ok {
					t.Fatal("old work disappeared")
				}
				if _, ok := StoryQuestDefinition(meta, fresh.ID); ok != (revision == tc.current) {
					t.Fatal("new affair crossed frozen content boundary")
				}
			}

		})
	}
}

func TestWorldContentTechnicalSurveysUseOwnedToolsLearnedSkillsAndRealEffort(t *testing.T) {
	for _, tc := range []struct{ pack, place, task, method, skill, tool string }{
		{"lanchuan-qunxia", "lanchuan-l04", "lanchuan-tide-lamp", "survey", "lanchuan-s34", "lanchuan-i28"},
		{"xinghai-689", "xinghai-l21", "xinghai-repeating-beacon", "measure", "xinghai-s32", "xinghai-i20"},
	} {
		for _, hasTool := range []bool{false, true} {
			t.Run(tc.task+"/"+map[bool]string{false: "missing-tool", true: "prepared"}[hasTool], func(t *testing.T) {
				items := map[string]int{}
				if hasTool {
					items[tc.tool] = 1
				}
				s, id, _ := worldContentPreparedStory(t, tc.pack, tc.place, map[string]int{tc.skill: 1}, items)
				questApplyOn(t, s, id, "main", tc.task, QuestActionAccept, "", "take")
				if !hasTool {
					worldContentRejectWithoutCharge(t, s, id, "main", tc.task, tc.task+"."+tc.method)
					return
				}
				before := worldContentActor(t, s, id, "main")
				result := questApplyOn(t, s, id, "main", tc.task, QuestActionChoose, tc.task+"."+tc.method, "survey")
				after := worldContentActor(t, s, id, "main")
				if result.Instance.Status != quest.StatusAccepted || after.Energy != before.Energy-6 || after.Inventory[tc.tool] != 1 || after.Currency != before.Currency {
					t.Fatal("technical survey did not use effort while preserving its tool")
				}
			})
		}
	}
}
