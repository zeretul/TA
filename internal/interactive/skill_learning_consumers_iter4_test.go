package interactive

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/quest"
)

func iter4SkillRecord(t *testing.T, state map[string]any, id string) map[string]any {
	t.Helper()
	records, _ := xiuxianEquipmentEntry(state, DefaultActorID, xiuxianFieldLearnedSkills)
	_, record := skillLearningRecord(records, id)
	if record == nil {
		t.Fatalf("skill %s has no owned learning record: %#v", id, records)
	}
	return record
}

// All learning below goes through the formal Story commit and its clock. Only
// the failed half-learned cast uses a disposable exchange from that saved state.
func TestIter4SkillRegistrationStudyBattleRetryColdReplayAndBranches(t *testing.T) {
	store, storyID, root := talismanStory(t)
	before, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	old, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: before.CurrentTurn.ID, Title: "未取得技能"})
	if err != nil {
		t.Fatal(err)
	}
	const name = "青焰弹"
	const otherName = "赤霞弹"
	id := cultivation.GeneratedSkillID("fire_bolt", name)
	otherID := cultivation.GeneratedSkillID("fire_bolt", otherName)
	talismanTurn(t, store, storyID, "main", "拿到两份完整火术注本", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{
		{TemplateID: "fire_bolt", Name: name, Source: "青焰火术完整注本", Description: "一线青焰凝在指尖。"},
		{TemplateID: "fire_bolt", Name: otherName, Source: "赤霞火术完整注本", Description: "霞光裹住细小的火核。"},
	}}})
	registered, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	registry := storySkillRegistryFromState(cultivation.ReferenceRules(),
		registered.State)
	def, exists := registry.Current(id)
	if !exists || def.Name != name {
		t.Fatalf("the new skill definition was not registered: %+v", def)
	}
	required := skillStudyMinutes(def, registered.State, DefaultActorID)
	if required <= 0 {
		t.Fatal("the registered skill must have a real study duration")
	}
	for _, skillID := range []string{id, otherID} {
		record := iter4SkillRecord(t, registered.State, skillID)
		if skillRecordMastered(record) || skillRecordStudyMinutes(record) != 0 || learnedSkillsFromState(registered.State, DefaultActorID, registry).Knows(skillID) {
			t.Fatalf("zero-minute registration granted mastery: %#v", record)
		}
	}
	if SnapshotClockSeconds(registered) != SnapshotClockSeconds(before) {
		t.Fatal("definition registration charged study time")
	}

	// An independently mastered art must survive learning the next art, even
	// though both have the same frozen numeric template.
	talismanTurn(t, store, storyID, "main", "先学会赤霞弹", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: otherID, Minutes: required}}}})
	first := talismanTurn(t, store, storyID, "main", "研习青焰弹前半程", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: id, Minutes: required / 2}}}})
	half, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	halfRecord := iter4SkillRecord(t, half.State, id)
	if skillRecordMastered(halfRecord) || skillRecordStudyMinutes(halfRecord) != required/2 || !skillRecordMastered(iter4SkillRecord(t, half.State, otherID)) {
		t.Fatalf("partial study lost progress, granted mastery or erased another art: %#v", halfRecord)
	}
	if got := SnapshotClockSeconds(half) - SnapshotClockSeconds(before); got != int64(required*90) {
		t.Fatalf("the formal clock omitted or duplicated study: got %d want %d", got, int64(required*90))
	}
	system := combatTestSystemFromStore(t, store, storyID)
	_, _, err = ResolveCombatExchange(half.State, system, "half-skill", "half-cast", combatTestSpecs(), nil,
		map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: cultivation.SkillDef{ID: id}, TargetID: "rival"}})
	if !errors.Is(err, ErrCombatTechniqueNotLearned) {
		t.Fatalf("a half-studied art reached the casting engine: %v", err)
	}
	details, err := store.CultivationEntryDetails(storyID, "main", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	seenHalf := false
	for _, detail := range details.Skills {
		if detail.ID == id {
			seenHalf = true
			if detail.Mastered || detail.Castable || detail.LearningMinutes != required/2 || detail.RequiredLearningMinutes != required || detail.ReasonKey != "entryDetails.learningIncomplete" {
				t.Fatalf("the owned-detail consumer presented unfinished study as usable: %+v", detail)
			}
		}
	}
	if !seenHalf {
		t.Fatal("owned details hid a real partly learned art")
	}
	halfBranch, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: first.ID, Title: "只学半程"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	coldHalf, err := store.Snapshot(storyID, "main")
	if err != nil || !reflect.DeepEqual(coldHalf.State, half.State) || SnapshotClockSeconds(coldHalf) != SnapshotClockSeconds(half) {
		t.Fatalf("cold reopen lost partial skill progress or its clock: %v", err)
	}

	// Offering a full duration after half is already paid must charge only the
	// remaining half. Replaying the same accepted command must charge nothing.
	const user = "继续研习青焰弹直到掌握"
	identity := b1CommitInput(t, store, storyID, b1Identity("iter4-skill-finish", 1), user)
	request := AppendTurnWithStateRequest{BranchID: "main", User: user, Narrative: "你把余下的运气与凝焰步骤练熟。",
		AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle,
		TurnResult: &TurnResult{Choices: testTurnChoices(), Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: id, Minutes: required}}}}}
	completed, _, err := store.AppendTurnWithState(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	retry, _, err := store.AppendTurnWithState(storyID, request)
	if err != nil || retry.ID != completed.ID {
		t.Fatalf("the completed study command was not idempotent: %v", err)
	}
	mastered, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, skillID := range []string{id, otherID} {
		record := iter4SkillRecord(t, mastered.State, skillID)
		if !skillRecordMastered(record) || skillRecordStudyMinutes(record) != required || !learnedSkillsFromState(mastered.State, DefaultActorID, storySkillRegistryFromState(cultivation.ReferenceRules(),
			mastered.State)).Knows(skillID) {
			t.Fatalf("completed art is missing from its real casting set: %#v", record)
		}
	}
	if got := SnapshotClockSeconds(mastered) - SnapshotClockSeconds(half); got != int64(required*30) {
		t.Fatalf("completion charged the offered duration rather than remaining study: %d", got)
	}

	opened, err := store.StartBattle(storyID, BattleStartRequest{BranchID: "main", Combatants: combatTestSpecs(), CommandID: "iter4-skill-open", CommandHash: "iter4-skill-open"})
	if err != nil {
		t.Fatal(err)
	}
	command := combatFatalityCommandForHit(opened.Session, DefaultActorID, "rival")
	cast := BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, CommandID: command, CommandHash: command,
		Actions: map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: cultivation.SkillDef{ID: id}, TargetID: "rival"}, "rival": {Kind: cultivation.ActionRest}}}
	played, err := store.ResolveBattleRound(storyID, cast)
	if err != nil {
		t.Fatal(err)
	}
	step := findCombatStep(t, played.Outcome, DefaultActorID)
	if step.SkillID != id || step.ActionRejected || step.Cast.ManaCost <= 0 || step.Resolution.HealthLost <= 0 || played.Session.CombatantByID(DefaultActorID).Attributes.Mana >= opened.Session.CombatantByID(DefaultActorID).Attributes.Mana {
		t.Fatalf("mastery did not reach real battle cost and damage: %+v", step)
	}
	castRetry, err := store.ResolveBattleRound(storyID, cast)
	if err != nil || castRetry.EventID != played.EventID || !reflect.DeepEqual(castRetry.Session, played.Session) {
		t.Fatalf("the battle retry charged the learned art twice: %v", err)
	}
	final, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	retry, _, err = store.AppendTurnWithState(storyID, request)
	if err != nil || retry.ID != completed.ID {
		t.Fatalf("a cold study retry did not return its original result: %v", err)
	}
	cold, err := store.Snapshot(storyID, "main")
	if err != nil || !reflect.DeepEqual(cold.State, final.State) || SnapshotClockSeconds(cold) != SnapshotClockSeconds(final) {
		t.Fatalf("cold replay changed mastered skills, paid mana, damage or time: %v", err)
	}
	prior, err := store.Snapshot(storyID, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := storySkillRegistryFromState(cultivation.ReferenceRules(),
		prior.State).Current(id); found || SnapshotClockSeconds(prior) != SnapshotClockSeconds(before) {
		t.Fatal("the earlier branch inherited a future generated definition or study time")
	}
	forked, err := store.Snapshot(storyID, halfBranch.ID)
	if err != nil || !reflect.DeepEqual(forked.State, half.State) || SnapshotClockSeconds(forked) != SnapshotClockSeconds(half) {
		t.Fatalf("the halfway branch inherited future mastery or combat consumption: %v", err)
	}
}

func TestIter4SkillStudyRejectsOverlappingRestAtomically(t *testing.T) {
	store, storyID, _ := talismanStory(t)
	before, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{BranchID: "main", User: "同一小时研习又调息", Narrative: "试图把一小时重复结算。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{TemplateID: "fire_bolt", Name: "同段火术", Source: "完整注本", Minutes: 60}}}, Rest: &TurnRestInput{Mode: "safe_meditation", DurationSeconds: 3600}}})
	if err == nil {
		t.Fatal("the same interval was accepted as both study and rest")
	}
	after, err := store.Snapshot(storyID, "main")
	if err != nil || !reflect.DeepEqual(after.State, before.State) || SnapshotClockSeconds(after) != SnapshotClockSeconds(before) {
		t.Fatalf("rejected overlapping study left a partial definition, learning or clock write: %v", err)
	}
}

func TestIter4SkillStudyRejectsInvalidDurations(t *testing.T) {
	for _, minutes := range []float64{-1, math.NaN(), math.Inf(1), float64(math.MaxInt64)} {
		if validateTurnSkillLearningModule(&TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: "fire_bolt", Source: "完整注本", Minutes: minutes}}}) == nil {
			t.Fatalf("invalid study duration admitted: %v", minutes)
		}
	}
}

func TestIter4SkillStudySequentialEntriesDoNotMultiplyMasteryOrTime(t *testing.T) {
	store, storyID, _ := talismanStory(t)
	const name = "顺序青焰弹"
	id := cultivation.GeneratedSkillID("fire_bolt", name)
	talismanTurn(t, store, storyID, "main", "取得完整青焰注本", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{TemplateID: "fire_bolt", Name: name, Source: "完整青焰注本"}}}})
	before, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	def, _ := storySkillRegistryFromState(cultivation.ReferenceRules(),
		before.State).Current(id)
	required := skillStudyMinutes(def, before.State, DefaultActorID)
	talismanTurn(t, store, storyID, "main", "前后两段研习同门技能", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: id, Minutes: required / 4}, {DefID: id, Minutes: required / 4}}}})
	half, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	record := iter4SkillRecord(t, half.State, id)
	if skillRecordMastered(record) || skillRecordStudyMinutes(record) != required/2 || SnapshotClockSeconds(half)-SnapshotClockSeconds(before) != int64(required*30) {
		t.Fatalf("duplicate entries did not form one sequential interval: record=%#v clock=%d", record, SnapshotClockSeconds(half))
	}
	talismanTurn(t, store, storyID, "main", "学完剩余部分后不再重复研习", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: id, Minutes: required}, {DefID: id, Minutes: required}}}})
	final, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	record = iter4SkillRecord(t, final.State, id)
	if !skillRecordMastered(record) || skillRecordStudyMinutes(record) != required || SnapshotClockSeconds(final)-SnapshotClockSeconds(before) != int64(required*60) {
		t.Fatalf("repeated complete-study entries charged more than remaining work: record=%#v clock=%d", record, SnapshotClockSeconds(final))
	}
}

func TestIter4SkillStudyStopsAtExistingDeadlineWithoutGrantingFutureProgress(t *testing.T) {
	store, storyID, _ := talismanStory(t)
	const name = "赴约前青焰弹"
	id := cultivation.GeneratedSkillID("fire_bolt", name)
	talismanTurn(t, store, storyID, "main", "取得可研习的完整注本", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{TemplateID: "fire_bolt", Name: name, Source: "完整青焰注本"}}}})
	before, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	clock := NewStoryClock("")
	branch := &storyBranchProjection{State: before.State, Clock: &clock, Quests: map[string]quest.Instance{"existing-deadline": {TaskID: quest.TaskSuQianMaterials, TaskVersion: "1.0", Status: quest.StatusAccepted, DeadlineSeconds: 600}}}
	out, err := planTurnSkillCommit(branch, StoryMeta{}, combatTestSystemFromStore(t, store, storyID), before.State, &TurnSkillLearningInput{Skills: []TurnSkillLearning{
		{DefID: "shield", Minutes: 120}, // An unowned source cannot become free study or consume the deadline.
		{DefID: id, Minutes: 30}, {DefID: id, Minutes: 30},
	}}, "deadline-study", "赴约前尽量研习")
	if err != nil {
		t.Fatal(err)
	}
	if out.clock == nil || out.clock.DurationSeconds != 600 || len(out.result.Learned) != 0 {
		t.Fatalf("study crossed an existing deadline or granted premature mastery: %+v", out)
	}
	after := appliedXiuxianState(t, before.State, out.ops)
	record := iter4SkillRecord(t, after, id)
	if skillRecordMastered(record) || skillRecordStudyMinutes(record) != 10 {
		t.Fatalf("interrupted sequential study credited unperformed work: %#v", record)
	}
	if learnedSkillsFromState(after, DefaultActorID, storySkillRegistryFromState(cultivation.ReferenceRules(),
		after)).Knows("shield") || skillRecordStudyMinutes(iter4SkillRecord(t, before.State, id)) != 0 {
		t.Fatal("deadline planning granted an unowned art or mutated the committed snapshot")
	}
}
