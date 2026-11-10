package interactive

import (
	"testing"

	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

// This file is F17: one real creature met in the story, living through the same
// rules as anyone else.
//
// 《自制-灵兽与御兽》 asks for a creature with its own identity, stage, body and
// place, whose growth comes from its own species and parameters (§2), whose
// wounds are real and survive being recalled (§3, §4), and which is never both a
// living companion and a corpse whose materials were taken (§4).
//
// Most of that already exists: an Actor carries the creature's own stage and
// numbers, NPC growth settles it from its own arrangement, the battle settles
// and persists its wounds, and the archive is the formal inactive marker. What
// the tests below fix is that these paths agree about one creature — that a
// meeting is not a fight, that a wound is not undone by being called back, and
// that an archived creature is neither a companion nor prey.

// f17BeastStory builds a story on the full xiuxian preset, so a creature can be
// created under any of its real templates and priced by its own stage.
func f17BeastStory(t *testing.T) (*Store, string) {
	t.Helper()
	spec := xiuxianActorStatePresetSpec()
	system := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			protagonistStateTemplate(cultivation.ReferenceRules(),
				spec),
			storyContextStateTemplate(spec),
			importantCharacterStateTemplate(cultivation.ReferenceRules(),
				spec),
			opponentStateTemplate(cultivation.ReferenceRules(),
				spec),
			worldEntitiesStateTemplate(spec),
		},
		InitialActors: []ActorStateInitialActor{
			{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: map[string]any{
				xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(6)),
			}},
			{ID: DefaultStoryContextActorID, Name: "故事状态", TemplateID: ActorStateStoryContextTemplateID},
			{ID: DefaultWorldEntitiesActorID, Name: "世界实体", TemplateID: ActorStateWorldEntitiesTemplateID},
		},
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "灵兽", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入林", Narrative: "主角沿溪入林。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// f17BeastSystem returns the story's frozen actor-state schema as the rules read
// it, which is the system the commit itself uses.
func f17BeastSystem(t *testing.T, store *Store, storyID string) StoryDirectorActorStateSystem {
	t.Helper()
	store.mu.Lock()
	meta, _, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	return actorStateSystemFromSnapshot(meta.ActorStateSchema, store.storyDirectorForMeta(meta).ActorState)
}

// f17MeetCreature commits the turn in which the creature first appears. It is
// created on the template the rules will track it under; the state it carries is
// its own species' position, never the protagonist's.
func f17MeetCreature(t *testing.T, store *Store, storyID, name, templateID string, stage cultivation.Cultivation, extra map[string]any) {
	t.Helper()
	state := map[string]any{xiuxianFieldRealm: cultivation.StageKey(stage)}
	for key, value := range extra {
		state[key] = value
	}
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "遇见" + name,
		Narrative: "溪石后伏着一只" + name + "，没有立刻扑上来。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{
			Op:   interactivestate.Create,
			Path: "/" + name,
			Value: map[string]any{
				"template_id": templateID,
				"name":        name,
				"state":       state,
			},
		}}},
	}); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
}

// f17Present settles who the scene holds. The list is the same one the character
// window, the encounter and NPC growth read.
func f17Present(t *testing.T, store *Store, storyID string, ids ...string) {
	t.Helper()
	values := make([]any, 0, len(ids))
	for _, id := range ids {
		values = append(values, id)
	}
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "同处",
		Narrative: "溪谷里只剩这几道气息。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{
			Op:    interactivestate.Replace,
			Path:  "/" + DefaultStoryContextActorID + "/" + storyScenePresentField,
			Value: values,
		}}},
	}); err != nil {
		t.Fatalf("present: %v", err)
	}
}

// f17DeclareEngagement writes the scene's own statement that the creature is
// attacking. It is the fact that starts a fight; presence alone is not one.
func f17DeclareEngagement(t *testing.T, store *Store, storyID, actorID, basis string) {
	t.Helper()
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "应战",
		Narrative: actorID + "扑了上来。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{
			Op:    interactivestate.Replace,
			Path:  "/" + DefaultStoryContextActorID + "/" + storySceneEngagementField,
			Value: map[string]any{actorID: basis},
		}}},
	}); err != nil {
		t.Fatalf("engagement: %v", err)
	}
}

func f17BeastHealth(t *testing.T, store *Store, storyID, actorID string) int {
	t.Helper()
	state := combatTestState(t, store, storyID)
	value, ok := actorStateNumber(actorStateFieldValue(state, actorID, xiuxianFieldHealth))
	if !ok {
		t.Fatalf("%s must carry a current 生命: %#v", actorID, state)
	}
	return int(value)
}

func f17BeastStage(t *testing.T, store *Store, storyID, actorID string) string {
	t.Helper()
	state := combatTestState(t, store, storyID)
	stage, _ := actorStateFieldValue(state, actorID, xiuxianFieldRealm).(string)
	return stage
}

func f17BeastProgress(t *testing.T, store *Store, storyID, actorID string) float64 {
	t.Helper()
	state := combatTestState(t, store, storyID)
	value, _ := actorStateNumber(actorStateFieldValue(state, actorID, xiuxianFieldRealmProgress))
	return value
}

func f17CombatRecordCount(t *testing.T, store *Store, storyID string) int {
	t.Helper()
	return b1RecordCount(t, store, storyID, StoryEventTypeCombatRecord)
}

// TestF17AMeetingIsNotAFightOrATaming keeps §1's 遇见不等于收服 and §2's
// 不以主人等级自动缩放 in one place: the creature the scene holds is its own
// actor with its own stage, and standing there is not attacking.
func TestF17AMeetingIsNotAFightOrATaming(t *testing.T) {
	store, storyID := f17BeastStory(t)
	f17MeetCreature(t, store, storyID, "青焰狐", ActorStateImportantCharacterTemplateID, cultivation.Qi(2), nil)
	f17Present(t, store, storyID, "主角", "青焰狐")

	if stage := f17BeastStage(t, store, storyID, "青焰狐"); stage != cultivation.StageKey(cultivation.Qi(2)) {
		t.Fatalf("the creature's stage is its own, not the protagonist's: %q", stage)
	}
	encounter, err := store.CombatEncounter(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(encounter.Combatants) != 1 || encounter.Combatants[0].ActorID != DefaultActorID {
		t.Fatalf("a creature merely present is not a combatant: %#v", encounter.Combatants)
	}
	if encounter.ReasonKey != combatEncounterNoEnemy {
		t.Fatalf("the encounter must report that nobody is fighting, got %q", encounter.ReasonKey)
	}
	if len(encounter.Bystanders) != 1 || encounter.Bystanders[0] != "青焰狐" {
		t.Fatalf("the creature is present, not hostile: %#v", encounter.Bystanders)
	}
	// Meeting it granted the protagonist nothing: no bond record, no control,
	// no item. The creature is a fact in the world and nothing more.
	state := combatTestState(t, store, storyID)
	for _, field := range []string{"对主角好感度", "关系"} {
		if value := actorStateFieldValue(state, "青焰狐", field); value != nil {
			if number, ok := actorStateNumber(value); ok && number != 0 {
				t.Fatalf("a meeting must not move %s: %#v", field, value)
			}
			if object, ok := value.(map[string]any); ok && len(object) != 0 {
				t.Fatalf("a meeting must not create %s: %#v", field, value)
			}
		}
	}
}

// TestF17BTheCreatureGrowsOnItsOwnStage is §2's 成长须由物种与适用阶段定义 and
// §3's 不把主人修炼收益无条件复制过去.
func TestF17BTheCreatureGrowsOnItsOwnStage(t *testing.T) {
	store, storyID := f17BeastStory(t)
	f17MeetCreature(t, store, storyID, "青焰狐", ActorStateImportantCharacterTemplateID, cultivation.Qi(2), nil)
	f17Present(t, store, storyID, "主角", "青焰狐")

	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 30*24*60*60, "cmd-f17-grow")

	if got := f17BeastProgress(t, store, storyID, "青焰狐"); got <= 0 {
		t.Fatalf("a real interval must train the creature through its own arrangement, got %v", got)
	}
	if stage := f17BeastStage(t, store, storyID, "青焰狐"); stage != cultivation.StageKey(cultivation.Qi(2)) {
		t.Fatalf("30 days must not lift a creature to its owner's stage: %q", stage)
	}
	if stage := f17BeastStage(t, store, storyID, DefaultActorID); stage != cultivation.StageKey(cultivation.Qi(6)) {
		t.Fatalf("the protagonist's own stage is untouched: %q", stage)
	}
	records := npcGrowthTestRecords(t, store, storyID)
	found := false
	for _, record := range records {
		if record.Candidate.ActorID != "青焰狐" {
			continue
		}
		found = true
		if record.Candidate.Status != "applied" {
			t.Fatalf("the creature's interval must settle: %#v", record.Candidate)
		}
		if cultivation.StageKey(record.Candidate.NextState.Cultivation) == cultivation.StageKey(cultivation.Qi(6)) {
			t.Fatalf("the creature was paid at the protagonist's stage: %#v", record.Candidate.NextState.Cultivation)
		}
	}
	if !found {
		t.Fatal("the creature shares the scene, so its interval must be settled")
	}
}

// TestF17CAWoundSurvivesRecallAndReopen is §3's 不能通过反复收回／召出清除伤势
// and §4's 战斗…伤势真实.
func TestF17CAWoundSurvivesRecallAndReopen(t *testing.T) {
	store, storyID := f17BeastStory(t)
	f17MeetCreature(t, store, storyID, "青焰狐", ActorStateImportantCharacterTemplateID, cultivation.Qi(2), nil)
	f17Present(t, store, storyID, "主角", "青焰狐")
	f17DeclareEngagement(t, store, storyID, "青焰狐", "扑上来咬")

	before := f17BeastHealth(t, store, storyID, "青焰狐")
	state := combatTestState(t, store, storyID)
	system := f17BeastSystem(t, store, storyID)
	encounter := CombatEncounterFromState(state, system)
	session, outcome, err := ResolveCombatExchange(state, system, "cbt-f17", "cmd-f17-fight", encounter.Combatants, nil, map[string]cultivation.CombatAction{
		DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "青焰狐"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCombatRecord(storyID, AppendCombatRecordRequest{
		BranchID: "main", WorldTime: "第2日",
		Session: session, Outcome: outcome,
		CommandID: "cmd-f17-fight", CommandHash: "hash-f17",
	}); err != nil {
		t.Fatal(err)
	}
	wounded := f17BeastHealth(t, store, storyID, "青焰狐")
	if wounded >= before {
		t.Fatalf("the exchange must wound the creature: %d -> %d", before, wounded)
	}

	// The save is the record. A reopen reads the same number.
	reopened := NewStore(store.root)
	defer reopened.Close()
	if got := f17BeastHealth(t, reopened, storyID, "青焰狐"); got != wounded {
		t.Fatalf("a reopen must not heal the creature: %d -> %d", wounded, got)
	}

	// Calling it off the field and back is a lifecycle move, not a healing.
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "收进灵兽袋",
		Narrative: "主角把青焰狐收回袋中。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{
			Op: interactivestate.Archive, Path: "/青焰狐", Value: map[string]any{"reason": "收回灵兽袋"},
		}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "放出来",
		Narrative: "主角又把它放出来。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{
			{Op: interactivestate.Restore, Path: "/青焰狐", Value: map[string]any{"reason": "重新放出"}},
			{Op: interactivestate.Replace, Path: "/" + DefaultStoryContextActorID + "/" + storyScenePresentField, Value: []any{"主角", "青焰狐"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := f17BeastHealth(t, store, storyID, "青焰狐"); got != wounded {
		t.Fatalf("recalling and re-summoning must not clear the wound: %d -> %d", wounded, got)
	}

	// Nor does resting. A rest interval settles the protagonist's pools; the
	// creature's body is its own, and an ordinary growth leg writes no pool.
	//
	// The fight has to be over first: §三.4 refuses ordinary safe recovery while a
	// battle is open, and that refusal is about the protagonist, not the creature.
	ended := session
	ended.Ended, ended.Result = true, cultivation.ResultTruce
	if _, err := store.AppendCombatRecord(storyID, AppendCombatRecordRequest{
		BranchID: "main", WorldTime: "第2日",
		Session: ended, Outcome: cultivation.ExchangeOutcome{Round: ended.Round, Ended: true, Result: cultivation.ResultTruce},
		CommandID: "cmd-f17-part", CommandHash: "hash-f17-part",
	}); err != nil {
		t.Fatal(err)
	}
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 24*60*60, "cmd-f17-rest")
	if got := f17BeastHealth(t, store, storyID, "青焰狐"); got != wounded {
		t.Fatalf("the protagonist's rest must not heal the creature: %d -> %d", wounded, got)
	}
}

// TestF17DAnArchivedCreatureIsNeitherCompanionNorPrey is §4's 不能把同一只生灵
// 既作为活着的伙伴、又作为已经死亡并领取材料的猎物.
func TestF17DAnArchivedCreatureIsNeitherCompanionNorPrey(t *testing.T) {
	store, storyID := f17BeastStory(t)
	f17MeetCreature(t, store, storyID, "青焰狐", ActorStateImportantCharacterTemplateID, cultivation.Qi(2), nil)
	f17Present(t, store, storyID, "主角", "青焰狐")

	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "收尸",
		Narrative: "青焰狐死在雪里，主角取走它的妖丹。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{
			Op: interactivestate.Archive, Path: "/青焰狐", Value: map[string]any{"reason": "已死，妖丹被取走"},
		}}},
	}); err != nil {
		t.Fatal(err)
	}

	state := combatTestState(t, store, storyID)
	if !actorIsArchived(state, "青焰狐") {
		t.Fatal("the creature must be archived")
	}
	for _, present := range storySceneFieldList(storyContextActorState(state), storyScenePresentField) {
		if normalizeStatePanelActorID(present) == "青焰狐" {
			t.Fatalf("a dead creature must leave 在场角色: %#v", storySceneFieldList(storyContextActorState(state), storyScenePresentField))
		}
	}
	encounter, err := store.CombatEncounter(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, combatant := range encounter.Combatants {
		if combatant.ActorID == "青焰狐" {
			t.Fatalf("a dead creature cannot be a combatant: %#v", encounter.Combatants)
		}
	}

	// An inactive creature does not keep training. Presence in the archive is the
	// formal "not in the story" marker, so its interval is not paid.
	progressBefore := f17BeastProgress(t, store, storyID, "青焰狐")
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 30*24*60*60, "cmd-f17-dead")
	if got := f17BeastProgress(t, store, storyID, "青焰狐"); got != progressBefore {
		t.Fatalf("an archived creature must not accumulate growth: %v -> %v", progressBefore, got)
	}
	for _, record := range npcGrowthTestRecords(t, store, storyID) {
		if record.Candidate.ActorID == "青焰狐" && record.ClockAfter.Seconds > record.ClockBefore.Seconds {
			t.Fatalf("an archived creature must not be settled: %#v", record.Candidate)
		}
	}
}

// TestF17ERetryAndForkDoNotSettleTheFightTwice is §6's 同一行为幂等，分支各自发展
// read against one creature's body.
func TestF17ERetryAndForkDoNotSettleTheFightTwice(t *testing.T) {
	store, storyID := f17BeastStory(t)
	f17MeetCreature(t, store, storyID, "青焰狐", ActorStateImportantCharacterTemplateID, cultivation.Qi(2), nil)
	f17Present(t, store, storyID, "主角", "青焰狐")

	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	beforeFight := snapshot.CurrentTurn.ID

	f17DeclareEngagement(t, store, storyID, "青焰狐", "扑上来咬")
	state := combatTestState(t, store, storyID)
	system := f17BeastSystem(t, store, storyID)
	encounter := CombatEncounterFromState(state, system)
	session, outcome, err := ResolveCombatExchange(state, system, "cbt-f17", "cmd-f17-once", encounter.Combatants, nil, map[string]cultivation.CombatAction{
		DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "青焰狐"},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := AppendCombatRecordRequest{
		BranchID: "main", WorldTime: "第2日",
		Session: session, Outcome: outcome,
		CommandID: "cmd-f17-once", CommandHash: "hash-f17",
	}
	if _, err := store.AppendCombatRecord(storyID, request); err != nil {
		t.Fatal(err)
	}
	wounded := f17BeastHealth(t, store, storyID, "青焰狐")
	if _, err := store.AppendCombatRecord(storyID, request); err != nil {
		t.Fatalf("a retry of the same command must be a read: %v", err)
	}
	if got := f17BeastHealth(t, store, storyID, "青焰狐"); got != wounded {
		t.Fatalf("a retry must not wound the creature twice: %d -> %d", wounded, got)
	}
	if got := f17CombatRecordCount(t, store, storyID); got != 1 {
		t.Fatalf("one command is one record, got %d", got)
	}

	// A branch cut before the fight never had it, so the creature there is whole.
	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: beforeFight, Title: "未交手"})
	if err != nil {
		t.Fatal(err)
	}
	forkSnapshot, err := store.Snapshot(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := actorStateNumber(actorStateFieldValue(forkSnapshot.State, "青焰狐", xiuxianFieldHealth))
	if !ok || int(value) == wounded {
		t.Fatalf("a fork cut before the fight must not inherit its wound: %#v", actorStateFieldValue(forkSnapshot.State, "青焰狐", xiuxianFieldHealth))
	}
}
