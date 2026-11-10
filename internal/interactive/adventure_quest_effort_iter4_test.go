package interactive

import (
	"encoding/json"
	"reflect"
	"testing"

	"ta/internal/quest"
)

// Isolated qualified fixtures exercise method consumers. They do not claim that
// resources were earned; the separate native resource chains prove acquisition.
func questEffortFixtureIter4(t *testing.T) (AdventureRules, AdventureActor, quest.Definition) {
	t.Helper()
	r := authoredQuestRules(t, "xinghai-689")
	a := NewAdventureActor(r)
	a.Rank = 7
	a.Skills["xinghai-s28"] = 1
	a.Skills["xinghai-s30"] = 1
	a.Inventory["xinghai-i19"] = 1
	a.Inventory["xinghai-frigate"] = 1
	for _, item := range r.Items {
		if item.ID == "xinghai-frigate" {
			a.Assets[item.ID] = AdventureAsset{Health: item.Durability, MaxHealth: item.Durability, Shield: item.Power, Fuel: 1}
		}
	}
	a.Equipment["ship:asset"] = "xinghai-frigate"
	d, ok := quest.DefinitionByID("xinghai-a18")
	if !ok {
		t.Fatal("missing authored task")
	}
	return r, a, d
}

func questEffortProjectionIter4(a AdventureActor) *storyBranchProjection {
	state := map[string]any{}
	for _, op := range adventureActorOps(a, "fixture") {
		setPath(state, actorStateFieldPath(op.ActorID, op.FieldID), op.Value)
	}
	setPath(state, actorStateFieldPath(DefaultStoryContextActorID, storySceneLocationField), journeyPlaceLabel("xinghai-l21"))
	return &storyBranchProjection{State: state}
}

func TestAdventureQuestRealMethodCostsIter4(t *testing.T) {
	for _, suffix := range []string{"c1", "c2", "c3"} {
		t.Run(suffix, func(t *testing.T) {
			r, a, d := questEffortFixtureIter4(t)
			p := questEffortProjectionIter4(a)
			prior := quest.Instance{TaskID: d.ID, TaskVersion: d.Version, Status: quest.StatusAccepted}
			before, _ := json.Marshal(p.State)
			plan, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, prior, true, QuestRequest{ChoiceID: d.ID + "." + suffix})
			if err != nil {
				t.Fatal(err)
			}
			got, err := AdventureActorFromState(intervalProjectionAfterOps(p, plan.Ops).State, DefaultActorID)
			if err != nil {
				t.Fatal(err)
			}
			cost, fuel := 0, 1
			if suffix == "c1" {
				cost = 6
			}
			if suffix == "c2" {
				fuel = 0
			}
			if got.Currency-a.Currency != 50000 || got.Energy != a.Energy-cost || got.Assets["xinghai-frigate"].Fuel != fuel || got.Inventory["xinghai-i19"] != 1 || !reflect.DeepEqual(got.Defeated, a.Defeated) || plan.ClockAfter.Seconds-plan.ClockBefore.Seconds != 1800 {
				t.Fatalf("method did not settle exact money/effort/fuel/time together: %+v", got)
			}
			unchanged, _ := json.Marshal(p.State)
			if string(before) != string(unchanged) {
				t.Fatal("planning mutated source")
			}
		})
	}
}

func TestAdventureQuestInvalidMethodsAreAtomicIter4(t *testing.T) {
	for _, tc := range []struct {
		name, choice string
		change       func(*AdventureActor)
	}{
		{"unlearned_breaching", "c1", func(a *AdventureActor) { delete(a.Skills, "xinghai-s28") }},
		{"missing_cutter", "c1", func(a *AdventureActor) { delete(a.Inventory, "xinghai-i19") }},
		{"insufficient_effort", "c1", func(a *AdventureActor) { a.Energy = 5 }},
		{"no_ship", "c2", func(a *AdventureActor) {
			delete(a.Assets, "xinghai-frigate")
			delete(a.Inventory, "xinghai-frigate")
			delete(a.Equipment, "ship:asset")
		}},
		{"withdrawn_ship", "c2", func(a *AdventureActor) { delete(a.Equipment, "ship:asset") }},
		{"empty_fuel", "c2", func(a *AdventureActor) { x := a.Assets["xinghai-frigate"]; x.Fuel = 0; a.Assets["xinghai-frigate"] = x }},
		{"destroyed_ship", "c2", func(a *AdventureActor) {
			x := a.Assets["xinghai-frigate"]
			x.Health = 0
			a.Assets["xinghai-frigate"] = x
		}},
		{"disabled_ship", "c2", func(a *AdventureActor) {
			x := a.Assets["xinghai-frigate"]
			x.DisabledUntil = 3600
			a.Assets["xinghai-frigate"] = x
		}},
		{"unlearned_negotiation", "c3", func(a *AdventureActor) { delete(a.Skills, "xinghai-s30") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, a, d := questEffortFixtureIter4(t)
			tc.change(&a)
			p := questEffortProjectionIter4(a)
			before, _ := json.Marshal(p.State)
			prior := quest.Instance{TaskID: d.ID, TaskVersion: d.Version, Status: quest.StatusAccepted}
			if _, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, prior, true, QuestRequest{ChoiceID: d.ID + "." + tc.choice}); err == nil {
				t.Fatal("invalid method accepted")
			}
			unchanged, _ := json.Marshal(p.State)
			if string(before) != string(unchanged) {
				t.Fatal("refusal changed money, item, body, or clock")
			}
		})
	}
}

func TestAdventureQuestGunneryFuelDurableRetryAndBranchIter4(t *testing.T) {
	r, a, _ := questEffortFixtureIter4(t)
	values := map[string]any{}
	for _, op := range adventureActorOps(a, "fixture") {
		values[op.FieldID] = op.Value
	}
	location := journeyPlaceLabel("xinghai-l21")
	system := StoryDirectorActorStateSystem{WorldContent: authoredWorldContent(t, "xinghai-689"), AdventureRules: &r, Templates: []ActorStateTemplate{{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(r)}, {ID: DefaultStoryContextActorID, Name: "场景", Fields: []ActorStateField{{Name: storySceneLocationField, Type: "string", Default: location}}}}, InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: values}, {ID: DefaultStoryContextActorID, Name: "场景", TemplateID: DefaultStoryContextActorID, State: map[string]any{storySceneLocationField: location}}}}
	dir := t.TempDir()
	s := NewStore(dir)
	t.Cleanup(func() { _ = s.Close() })
	story, err := s.CreateStory(CreateStoryRequest{Title: "Isolated gunnery consumer fixture", ActorState: &system, PackBinding: &StoryPackBinding{PackID: "xinghai-689", ContentVersion: 1, Scope: "xinghai-689"}})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := s.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "观察", Narrative: "确认所处位置", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := s.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "未使用舰炮"})
	if err != nil {
		t.Fatal(err)
	}
	questApplyOn(t, s, story.ID, "main", "xinghai-a18", QuestActionAccept, "", "accept")
	req := QuestRequest{BranchID: "main", TaskID: "xinghai-a18", Action: QuestActionChoose, ChoiceID: "xinghai-a18.c2", CommandID: "gunnery", CommandHash: "gunnery"}
	first, err := s.ApplyQuest(story.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	repeat, err := s.ApplyQuest(story.ID, req)
	repeat.Replayed = false
	if err != nil || first.EventID != repeat.EventID || !reflect.DeepEqual(first.Instance, repeat.Instance) {
		t.Fatalf("repeat is not original receipt: %v", err)
	}
	read := func(branch string) AdventureWindowState {
		t.Helper()
		state, err := s.AdventureWindow(story.ID, branch)
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	main, other := read("main"), read(fork.ID)
	if main.Actor.Assets["xinghai-frigate"].Fuel != 0 || main.Actor.Currency-a.Currency != 50000 || main.ClockSeconds != 1800 || other.Actor.Assets["xinghai-frigate"].Fuel != 1 || other.Actor.Currency != a.Currency || other.ClockSeconds != 0 {
		t.Fatal("durable settlement leaked to branch or spent twice")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = NewStore(dir)
	if !reflect.DeepEqual(main, read("main")) || !reflect.DeepEqual(other, read(fork.ID)) {
		t.Fatal("cold replay changed exact settlement")
	}
	req.CommandID = "cannot repeat completed work"
	if _, err = s.ApplyQuest(story.ID, req); err == nil {
		t.Fatal("completed task paid again")
	}
	if !reflect.DeepEqual(main, read("main")) {
		t.Fatal("repeated stage changed state")
	}
}

func TestAdventureQuestFailureStillSpendsRealEffortIter4(t *testing.T) {
	r, a, d := questEffortFixtureIter4(t)
	d.Choices[0].Difficulty = 100
	p := questEffortProjectionIter4(a)
	prior := quest.Instance{TaskID: d.ID, TaskVersion: d.Version, Status: quest.StatusAccepted}
	plan, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, prior, true, QuestRequest{ChoiceID: d.Choices[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	got, err := AdventureActorFromState(intervalProjectionAfterOps(p, plan.Ops).State, DefaultActorID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Energy != a.Energy-6 || got.Currency != a.Currency || plan.ClockAfter.Seconds != 1800 || plan.Instance.Choices[d.Choices[0].ObjectiveID].Outcome != "failure" {
		t.Fatal("failed method did not spend its real effort and interval exactly once")
	}
	for _, id := range []string{"missing-method", "xinghai-s06"} {
		d.Choices[0].UseSkillID = id
		a.Skills[id] = 1
		p = questEffortProjectionIter4(a)
		if _, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, prior, true, QuestRequest{ChoiceID: d.Choices[0].ID}); err == nil {
			t.Fatalf("accepted unknown or combat method %s", id)
		}
	}
}
