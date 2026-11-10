package interactive

import (
	"math"
	"reflect"
	"testing"

	"ta/internal/cultivation"
)

// This is a deterministic Store-level regression, not model gameplay.
func TestIter4TechniqueStudyAdvancesFormalClock(t *testing.T) {
	store, storyID, _ := talismanStory(t)
	before, _, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	talismanTurn(t, store, storyID, "main", "研习温元诀六小时", &TurnResult{
		Techniques: &TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: "method.warm_origin", Source: "师父授业", Minutes: 360}}},
	})
	after, _, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.Seconds-before.Seconds != 360*60 {
		t.Fatalf("six hours of accepted technique study advanced formal clock by %d seconds, want 21600", after.Seconds-before.Seconds)
	}
}

func TestIter4GeneratedTechniqueStudyUseColdReplayAndBranches(t *testing.T) {
	store, storyID, root := talismanStory(t)
	before, _ := store.Snapshot(storyID, "main")
	old, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: before.CurrentTurn.ID, Title: "研习前"})
	if err != nil {
		t.Fatal(err)
	}
	id, _ := generatedContentID("technique", "method.warm_origin", "松烟温元法")
	talismanTurn(t, store, storyID, "main", "取得松烟温元法", &TurnResult{Techniques: &TurnTechniqueInput{Learn: []TurnTechniqueLearning{{TemplateID: "method.warm_origin", Name: "松烟温元法", Source: "师父授下的注本", Description: "暖意沿息流转。"}}}})
	registered, _ := store.Snapshot(storyID, "main")
	def, ok := resolveTechnique(cultivation.ReferenceRules(),
		id, 1, registered.State)
	base, _ := cultivation.TechniqueByID("method.warm_origin")
	if !ok || def.Name != "松烟温元法" || def.StandardLearningMinutes != base.StandardLearningMinutes || def.Circulation != base.Circulation {
		t.Fatalf("generated method lost its frozen mechanics: %+v", def)
	}
	if len(circulatingTechniques(cultivation.ReferenceRules(),
		mustReadTechniques(t, registered.State), registered.State)) != 0 || SnapshotClockSeconds(registered) != 0 {
		t.Fatal("registration granted mastery or elapsed time")
	}
	half := float64(base.StandardLearningMinutes) / 2
	first := talismanTurn(t, store, storyID, "main", "研习注本前半程", &TurnResult{Techniques: &TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: id, Minutes: half}}}})
	halfBranch, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: first.ID, Title: "半程"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(root)
	defer store.Close()
	talismanTurn(t, store, storyID, "main", "继续完成注本研习", &TurnResult{Techniques: &TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: id, Minutes: half}}, Circulate: []TurnTechniqueSwitch{{DefID: id}}}})
	final, _ := store.Snapshot(storyID, "main")
	active := circulatingTechniques(cultivation.ReferenceRules(),
		mustReadTechniques(t, final.State), final.State)
	if len(active) != 1 || active[0].ID != id || SnapshotClockSeconds(final) != int64(base.StandardLearningMinutes*60+600) {
		t.Fatalf("study/use failed: %+v clock=%d", active, SnapshotClockSeconds(final))
	}
	if xiuxianStoredInt(final.State, DefaultActorID, xiuxianFieldAttack) <= xiuxianStoredInt(before.State, DefaultActorID, xiuxianFieldAttack) {
		t.Fatal("generated circulating method did not change real attack")
	}
	details, err := store.CultivationEntryDetails(storyID, "main", "en-US")
	if err != nil || len(details.Techniques) != 1 || details.Techniques[0].Name != "松烟温元法" || !details.Techniques[0].Active {
		t.Fatalf("generated details unavailable: %+v %v", details, err)
	}
	for bid, wantMinutes := range map[string]float64{old.ID: 0, halfBranch.ID: half} {
		got, err := store.Snapshot(storyID, bid)
		if err != nil {
			t.Fatal(err)
		}
		records, _ := readTechniqueRecords(got.State, DefaultActorID)
		if wantMinutes == 0 && len(records) != 0 {
			t.Fatal("definition leaked before registration")
		}
		for _, record := range records {
			if record.Minutes != wantMinutes || record.Mastered || record.Active {
				t.Fatalf("future learning leaked to branch: %+v", record)
			}
		}
	}
	if jobs := GeneratedDefinitionArtJobs(final.State, "ink"); len(jobs) != 1 || jobs[0].Category != "technique" || jobs[0].DefinitionID != id {
		t.Fatalf("optional art lost new method: %+v", jobs)
	}
	_ = store.Close()
	store = NewStore(root)
	defer store.Close()
	cold, err := store.Snapshot(storyID, "main")
	if err != nil || !reflect.DeepEqual(cold.State, final.State) || SnapshotClockSeconds(cold) != SnapshotClockSeconds(final) {
		t.Fatalf("cold method replay changed: %v", err)
	}
}

func TestIter4TechniqueRejectsConflictingTimeAndInvalidDuration(t *testing.T) {
	store, id, _ := talismanStory(t)
	result := &TurnResult{Choices: testTurnChoices(), Techniques: &TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: "method.warm_origin", Source: "完整温元诀注本", Minutes: 60}}}, Rest: &TurnRestInput{Mode: "safe_meditation", DurationSeconds: 3600}}
	before, _ := store.Snapshot(id, "main")
	if _, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "同一小时研习又调息", Narrative: "重复计算时间。", TurnResult: result}); err == nil {
		t.Fatal("overlapping intervals were accepted")
	}
	after, _ := store.Snapshot(id, "main")
	if !reflect.DeepEqual(before.State, after.State) || SnapshotClockSeconds(before) != SnapshotClockSeconds(after) {
		t.Fatal("rejected interval changed state")
	}
	for _, minutes := range []float64{math.Inf(1), math.NaN(), float64(math.MaxInt64)} {
		if validateTurnTechniqueModule(&TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: "method.warm_origin", Minutes: minutes}}}) == nil {
			t.Fatal("invalid study duration admitted")
		}
	}
}

func TestIter4GeneratedTechniqueNamesDoNotReplaceOtherDefinitions(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianSkillTestState(t, "qi_6", nil)
	ops, result, _ := xiuxianTechniqueOps(state, system, DefaultActorID, TurnTechniqueInput{Learn: []TurnTechniqueLearning{
		{TemplateID: "method.warm_origin", Name: "同名功法", Source: "温息注本"},
		{TemplateID: "method.cold_spring", Name: "同名功法", Source: "寒息注本"},
	}}, "named")
	if len(result.Refused) != 0 {
		t.Fatalf("valid names refused: %+v", result)
	}
	next := appliedXiuxianState(t, state, ops)
	records, _ := readTechniqueRecords(next, DefaultActorID)
	if len(records) != 2 {
		t.Fatalf("same name overwrote another definition: %+v", records)
	}
}

func TestIter4TechniqueCirculationPaysPreparationOnce(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianSkillTestState(t, "qi_6", nil)
	ops, _, _ := xiuxianTechniqueOps(state, system, DefaultActorID, TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: "method.warm_origin", Source: "完整温元诀注本", Minutes: 14400}, {DefID: "method.cold_spring", Source: "完整寒泉诀注本", Minutes: 14400}}}, "past")
	state = appliedXiuxianState(t, state, ops)
	branch := &storyBranchProjection{State: state, Clock: func() *StoryClock { c := NewStoryClock(""); return &c }()}
	out, err := planTurnTechniqueCommit(branch, StoryMeta{}, system, state, &TurnTechniqueInput{Circulate: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}}}, "start", "安全运功")
	if err != nil || out.clock == nil || out.clock.DurationSeconds != 600 {
		t.Fatalf("start preparation not paid: %+v %v", out.clock, err)
	}
	state = appliedXiuxianState(t, state, out.ops)
	branch.State = state
	branch.Clock = &out.clock.After
	out, err = planTurnTechniqueCommit(branch, StoryMeta{}, system, state, &TurnTechniqueInput{Stop: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}}, Circulate: []TurnTechniqueSwitch{{DefID: "method.cold_spring"}}}, "switch", "先停温法，安全调息换寒法")
	if err != nil || out.clock == nil || out.clock.DurationSeconds != 3600 {
		t.Fatalf("warm/cold switch not one hour: %+v %v", out.clock, err)
	}
}
