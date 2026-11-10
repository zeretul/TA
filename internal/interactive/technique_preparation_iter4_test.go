package interactive

import (
	"reflect"
	"ta/internal/cultivation"
	"ta/internal/quest"
	"testing"
)

func iter4TechniqueBranch(state map[string]any) *storyBranchProjection {
	c := NewStoryClock("")
	return &storyBranchProjection{State: state, Clock: &c}
}
func iter4ApplyTechnique(t *testing.T, branch *storyBranchProjection, system StoryDirectorActorStateSystem, input TurnTechniqueInput) turnTechniqueOutcome {
	t.Helper()
	out, err := planTurnTechniqueCommit(branch, StoryMeta{}, system, branch.State, &input, "prepare-test", "按本次意图安全运功")
	if err != nil {
		t.Fatal(err)
	}
	branch.State = appliedXiuxianState(t, branch.State, out.ops)
	if out.clock != nil {
		branch.Clock = &out.clock.After
	}
	return out
}

func TestIter4TechniquePreparationSafetyRecoveryAndAtomicity(t *testing.T) {
	system := xiuxianTechniqueTestSystem()
	initial := masteredTechniqueState(t, "qi_6", "method.warm_origin", "method.cold_spring", "method.origin_guard")
	fields := f16ActorFields(initial)
	fields[xiuxianFieldHealth] = float64(100)
	fields[xiuxianFieldMana] = float64(20)
	fields[xiuxianFieldBurden] = float64(60)
	branch := iter4TechniqueBranch(initial)
	start := iter4ApplyTechnique(t, branch, system, TurnTechniqueInput{Circulate: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}, {DefID: "method.origin_guard"}}})
	if start.clock.DurationSeconds != 600 || len(start.result.Started) != 2 {
		t.Fatalf("compatible set pays once: %+v", start)
	}
	if xiuxianStoredInt(branch.State, DefaultActorID, xiuxianFieldMana) != 20 {
		t.Fatal("normal preparation granted meditation recovery")
	}
	repeat := iter4ApplyTechnique(t, branch, system, TurnTechniqueInput{Circulate: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}}})
	if repeat.clock != nil {
		t.Fatal("already-active method charged preparation again")
	}
	before := cloneStoryState(branch.State)
	out, err := planTurnTechniqueCommit(branch, StoryMeta{}, system, branch.State, &TurnTechniqueInput{Stop: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}}, Circulate: []TurnTechniqueSwitch{{DefID: "method.cold_spring"}}}, "warm-cold", "安全调息换寒法")
	if err != nil || out.clock == nil || out.clock.DurationSeconds != 3600 {
		t.Fatalf("switch: %+v %v", out.clock, err)
	}
	if !reflect.DeepEqual(before, branch.State) {
		t.Fatal("planning mutated the committed state")
	}
	after := appliedXiuxianState(t, branch.State, out.ops)
	expected := 20 + recoveryCurrentAttributes(&storyBranchProjection{State: before}, DefaultActorID).ManaMax/10
	if xiuxianStoredInt(after, DefaultActorID, xiuxianFieldMana) != expected || xiuxianStoredInt(after, DefaultActorID, xiuxianFieldHealth) != 100 || xiuxianStoredInt(after, DefaultActorID, xiuxianFieldBurden) != 0 {
		t.Fatalf("switch recovery not existing one-hour rules: %+v", f16ActorFields(after))
	}
	branch.Combats = map[string]cultivation.CombatSession{"active": {Combatants: []cultivation.Combatant{{ID: DefaultActorID, Attributes: cultivation.Attributes{Health: 100}}}}}
	if _, err := planTurnTechniqueCommit(branch, StoryMeta{}, system, branch.State, &TurnTechniqueInput{Stop: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}}, Circulate: []TurnTechniqueSwitch{{DefID: "method.cold_spring"}}}, "fight", "战斗中换法"); err == nil {
		t.Fatal("battle allowed a passive-set swap")
	}
	branch.Combats = nil
	failed := iter4ApplyTechnique(t, branch, system, TurnTechniqueInput{Stop: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}}, Circulate: []TurnTechniqueSwitch{{DefID: "method.clear_mind"}}})
	if len(failed.result.Refused) == 0 || len(failed.result.Stopped) > 0 || failed.clock != nil {
		t.Fatal("failed replacement stopped old method or claimed success")
	}
}

func TestIter4TechniquePreparationStopsAtBoundaryAndSameTemplateDoesNotStack(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianSkillTestState(t, "qi_6", nil)
	ops, _, _ := xiuxianTechniqueOps(state, system, DefaultActorID, TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: "method.warm_origin", Source: "完整温元诀", Minutes: 14400}, {TemplateID: "method.warm_origin", Name: "松烟温元诀", Source: "完整同效温元诀注本", Minutes: 14400}}}, "past")
	state = appliedXiuxianState(t, state, ops)
	id, _ := generatedContentID("technique", "method.warm_origin", "松烟温元诀")
	branch := iter4TechniqueBranch(state)
	branch.Quests = map[string]quest.Instance{"limit": {TaskID: quest.TaskSuQianMaterials, TaskVersion: "1.0", Status: quest.StatusAccepted, DeadlineSeconds: 300}}
	input := TurnTechniqueInput{Circulate: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}, {DefID: id}}}
	interrupted := iter4ApplyTechnique(t, branch, system, input)
	if interrupted.clock == nil || interrupted.clock.DurationSeconds != 300 || len(interrupted.result.Started) != 0 || len(interrupted.result.Refused) == 0 || len(circulatingTechniques(cultivation.ReferenceRules(),
		mustReadTechniques(t, branch.State), branch.State)) != 0 {
		t.Fatalf("interrupted preparation claimed activation: %+v", interrupted)
	}
	branch.Quests = nil
	out := iter4ApplyTechnique(t, branch, system, input)
	if out.clock.DurationSeconds != 600 {
		t.Fatal("restart did not pay one complete preparation")
	}
	active := circulatingTechniques(cultivation.ReferenceRules(),
		mustReadTechniques(t, branch.State), branch.State)
	one, _ := cultivation.TechniqueByID("method.warm_origin")
	got := cultivation.TechniqueModifiers(active)
	want := cultivation.TechniqueModifiers([]cultivation.TechniqueDef{one})
	if len(got) != len(want) {
		t.Fatalf("same-source generated variant stacked effects: %+v %+v", got, want)
	}
	for i := range got {
		if got[i].Attribute != want[i].Attribute || got[i].Value != want[i].Value || got[i].Kind != want[i].Kind {
			t.Fatalf("same-template numbers stacked: %+v %+v", got, want)
		}
	}
}

func TestIter4TechniqueStoppedMethodPaidRecoveryPersists(t *testing.T) {
	store, id, root := talismanStory(t)
	talismanTurn(t, store, id, "main", "按完整注本学会两种功法", &TurnResult{Techniques: &TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: "method.warm_origin", Source: "完整温元诀注本", Minutes: 14400}, {DefID: "method.cold_spring", Source: "完整寒泉诀注本", Minutes: 14400}}}})
	talismanTurn(t, store, id, "main", "安全运转温法", &TurnResult{Techniques: &TurnTechniqueInput{Circulate: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}}}})
	beforeStop, _ := store.Snapshot(id, "main")
	talismanTurn(t, store, id, "main", "停止温法", &TurnResult{Techniques: &TurnTechniqueInput{Stop: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}}}})
	stopped, _ := store.Snapshot(id, "main")
	if SnapshotClockSeconds(stopped) != SnapshotClockSeconds(beforeStop) {
		t.Fatal("pure stop charged time")
	}
	fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: stopped.CurrentTurn.ID, Title: "停法未调息"})
	if err != nil {
		t.Fatal(err)
	}
	talismanTurn(t, store, id, "main", "安全调息一个小时", &TurnResult{Rest: &TurnRestInput{Mode: "safe_meditation", DurationSeconds: 3600}})
	paid, _ := store.Snapshot(id, "main")
	_ = store.Close()
	store = NewStore(root)
	defer store.Close()
	view, err := store.TurnWorldView(id, "main", "")
	if err != nil || len(view.Recoveries) == 0 {
		t.Fatalf("paid recovery missing from dry-run view: %v", err)
	}
	started := talismanTurn(t, store, id, "main", "启用寒法", &TurnResult{Techniques: &TurnTechniqueInput{Circulate: []TurnTechniqueSwitch{{DefID: "method.cold_spring"}}}})
	final, _ := store.Snapshot(id, "main")
	if SnapshotClockSeconds(final) != SnapshotClockSeconds(paid) {
		t.Fatal("previously paid sixty minutes were charged twice")
	}
	retry, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: started.User, Narrative: started.Narrative, TurnResult: &TurnResult{Choices: testTurnChoices(), Techniques: &TurnTechniqueInput{Circulate: []TurnTechniqueSwitch{{DefID: "method.cold_spring"}}}}, AgentCommandID: started.AgentCommandID, AgentOperationID: started.AgentOperationID, AgentCycle: started.AgentCycle})
	if err != nil || retry.ID != started.ID {
		t.Fatalf("retry was not durable: %v", err)
	}
	talismanTurn(t, store, id, fork.ID, "未调息直接换寒法", &TurnResult{Techniques: &TurnTechniqueInput{Circulate: []TurnTechniqueSwitch{{DefID: "method.cold_spring"}}}})
	other, _ := store.Snapshot(id, fork.ID)
	if SnapshotClockSeconds(other)-SnapshotClockSeconds(stopped) != 3600 {
		t.Fatal("stop in a prior turn bypassed warm/cold preparation")
	}
}
