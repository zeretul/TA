package interactive

import (
	"math"
	"reflect"
	"ta/internal/cultivation"
	"ta/internal/storytime"
	"testing"
)

func iter4MethodsState(t *testing.T, stage string) map[string]any {
	t.Helper()
	records := map[string]any{}
	for _, id := range []string{"method.basic_breathing", "method.warm_origin"} {
		d, _ := cultivation.TechniqueByID(id)
		records[d.Name] = map[string]any{"名称": d.Name, "定义": d.ID, "版本": d.Version, "掌握程度": "已掌握", "已修习分钟": d.StandardLearningMinutes, "运转": "运转", "来历": "规则测试既有完整传承"}
	}
	return xiuxianTestState(t, stage, map[string]any{xiuxianFieldTechniques: records})
}
func TestIter4CultivationUsesChosenMethodNotHighest(t *testing.T) {
	state := iter4MethodsState(t, "qi_6")
	system := xiuxianTestSystem()
	_, low, ok := advanceXiuxianCultivation(state, system, DefaultActorID, TurnCultivationInput{MethodID: "method.basic_breathing", Days: 1}, xiuxianFormalFacts{}, b1Identity("method-low", 1), "low")
	if !ok || !low.Trained {
		t.Fatalf("low %+v", low)
	}
	_, high, ok := advanceXiuxianCultivation(state, system, DefaultActorID, TurnCultivationInput{MethodID: "method.warm_origin", Days: 1}, xiuxianFormalFacts{}, b1Identity("method-high", 1), "high")
	if !ok || !high.Trained {
		t.Fatalf("high %+v", high)
	}
	if math.Abs(high.RealmProgressAfter/low.RealmProgressAfter-1.15) > 1e-9 {
		t.Fatalf("selection ignored or coefficient duplicated: low=%v high=%v", low.RealmProgressAfter, high.RealmProgressAfter)
	}
	_, invalid, _ := advanceXiuxianCultivation(state, system, DefaultActorID, TurnCultivationInput{MethodID: "method.unlearned", Days: 1}, xiuxianFormalFacts{}, b1Identity("method-invalid", 1), "invalid")
	if invalid.Trained || invalid.ConsumedDays != 0 {
		t.Fatalf("unlearned method spent time or grew: %+v", invalid)
	}
}
func TestIter4MethodCannotSponsorUnsupportedBreakthrough(t *testing.T) {
	state := iter4MethodsState(t, "foundation_late")
	stage, _ := cultivation.ParseStageKey("foundation_late")
	required, _, _ := cultivation.ProgressRequired(stage)
	applyActorStateOp(state, ActorStateOp{Op: "set", ActorID: DefaultActorID, FieldID: xiuxianFieldRealmProgress, Value: required})
	if got := cultivationMethodSelectionDiagnostic(cultivation.ReferenceRules(),
		state, &TurnCultivationInput{MethodID: "method.warm_origin", AttemptBreakthrough: "core"}); got == nil {
		t.Fatal("preview accepted method beyond its frozen ceiling")
	}
	_, result, _ := advanceXiuxianCultivation(state, xiuxianTestSystem(), DefaultActorID, TurnCultivationInput{MethodID: "method.warm_origin", AttemptBreakthrough: "core"}, xiuxianFormalFacts{}, b1Identity("method-ceiling", 1), "ceiling")
	if result.BreakthroughEligible || result.BreakthroughRolled || result.BreakthroughDays != 0 || len(result.BreakthroughReasons) == 0 {
		t.Fatalf("foundation-only method sponsored core breakthrough: %+v", result)
	}
}

func TestIter4ChosenMethodSurvivesInterruptedRetryColdAndFork(t *testing.T) {
	state := iter4MethodsState(t, "qi_6")
	system := xiuxianTestSystem()
	system.InitialActors = []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: map[string]any{xiuxianFieldRealm: "qi_6", xiuxianFieldTechniques: actorStateFieldValue(state, DefaultActorID, xiuxianFieldTechniques)}}}
	root := t.TempDir()
	store := NewStore(root)
	defer store.Close()
	story, err := store.CreateStory(CreateStoryRequest{Title: "真实功法选择", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	id := story.ID
	if _, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "准备修炼", Narrative: "隔离自动测试起点", TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
		t.Fatal(err)
	}
	initial, err := store.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	req := CultivationRequest{BranchID: "main", ActorID: DefaultActorID, Interval: CultivationInterval{Value: 2, Unit: CultivationUnitDay}, Conditions: &CultivationConditions{MethodID: "method.basic_breathing"}, CommandID: "chosen-part", CommandHash: "chosen-part", Nodes: []storytime.Node{{ID: "actual-stop", Kind: storytime.DecisionNode, At: 86400, RequiresResponse: true}}}
	first := cultivationTestApply(t, store, id, req)
	if first.Completed || first.Elapsed != 86400 || first.Activity.Conditions.MethodID != "method.basic_breathing" {
		t.Fatalf("wrong interrupted selection: %+v", first)
	}
	replay := cultivationTestApply(t, store, id, req)
	if !replay.Replayed || replay.EventID != first.EventID {
		t.Fatal("retry did not reuse method interval")
	}
	beforeFork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: initial.CurrentTurn.ID, Title: "before chosen method interval"})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "停下处理眼前事情", Narrative: "实际中断后的检查点。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	inheritedFork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: checkpoint.ID, Title: "after chosen method interval"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cold := NewStore(root)
	defer cold.Close()
	_, acts, err := cold.CultivationState(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if acts[first.ActivityID].Conditions.MethodID != "method.basic_breathing" {
		t.Fatal("chosen method lost on cold read")
	}
	resume := CultivationRequest{BranchID: "main", ActorID: DefaultActorID, ActivityID: first.ActivityID, CommandID: "chosen-resume", CommandHash: "chosen-resume", Nodes: []storytime.Node{{ID: "actual-stop", Kind: storytime.DecisionNode, At: 86400, RequiresResponse: true, Resolved: true}}}
	second := cultivationTestApply(t, cold, id, resume)
	if !second.Completed || second.Elapsed != 86400 || second.Advance.MethodID != "method.basic_breathing" || math.Abs(second.Advance.RealmProgressAfter-2*first.Advance.RealmProgressAfter) > 1e-9 {
		t.Fatalf("resume changed method or charged duplicate time: %+v", second)
	}
	preClock, preActs, err := cold.CultivationState(id, beforeFork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preClock.Seconds != 0 || len(preActs) != 0 {
		t.Fatal("pre-interval fork inherited future cultivation")
	}
	inheritedClock, inheritedActs, err := cold.CultivationState(id, inheritedFork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inheritedClock.Seconds != 86400 || inheritedActs[first.ActivityID].Conditions.MethodID != "method.basic_breathing" || inheritedActs[first.ActivityID].Activity.CompletedEffective != 86400 {
		t.Fatal("checkpoint did not preserve original method and partial progress")
	}
	before, err := cold.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cold.ApplyCultivationInterval(id, CultivationRequest{BranchID: "main", ActorID: DefaultActorID, Interval: CultivationInterval{Value: 1, Unit: CultivationUnitDay}, Conditions: &CultivationConditions{MethodID: "unknown"}, CommandID: "bad-method", CommandHash: "bad-method"}); err == nil {
		t.Fatal("unknown method accepted")
	}
	if _, _, err = cold.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "try unknown method", Narrative: "invalid synthetic fixture", TurnResult: &TurnResult{Choices: testTurnChoices(), Cultivation: &TurnCultivationInput{MethodID: "unknown", Days: 1}}}); err == nil {
		t.Fatal("turn accepted unknown method")
	}
	after, err := cold.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.State, after.State) || SnapshotClockSeconds(before) != SnapshotClockSeconds(after) || before.CurrentTurn.ID != after.CurrentTurn.ID {
		t.Fatal("rejected method mutated state/time/turn")
	}
}
