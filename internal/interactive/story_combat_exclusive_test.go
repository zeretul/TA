package interactive

import (
	"errors"
	"testing"

	"ta/internal/cultivation"
)

// These tests cover R1: one person may take part in only one unfinished battle on
// a branch, and an older battle may not write its snapshot over a settlement a
// later battle really produced.

// combatTestStoryWithSecondEnemy is combatTestStory with a third tracked
// character, so two different battles can be opened against one protagonist.
func combatTestStoryWithSecondEnemy(t *testing.T) (*Store, string) {
	t.Helper()
	spec := xiuxianActorStatePresetSpec()
	fields := append([]ActorStateField{}, spec.PanelFields...)
	fields = append(fields, spec.StateFields...)
	fields = append(fields, xiuxianLearnedSkillsField())
	stage := cultivation.StageKey(cultivation.Qi(6))
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{
		{ID: DefaultActorID, Name: "主角", Fields: fields},
		{ID: "rival", Name: "对手", Fields: append([]ActorStateField{}, fields...)},
		{ID: "rival2", Name: "另一个对手", Fields: append([]ActorStateField{}, fields...)},
	}}
	system.InitialActors = []ActorStateInitialActor{
		{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: map[string]any{xiuxianFieldRealm: stage}},
		{ID: "rival", Name: "对手", TemplateID: "rival", State: map[string]any{xiuxianFieldRealm: stage}},
		{ID: "rival2", Name: "另一个对手", TemplateID: "rival2", State: map[string]any{xiuxianFieldRealm: stage}},
	}
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "战斗记录", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角与两名对手对峙。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

func combatTestProtagonistHealth(t *testing.T, store *Store, storyID string) float64 {
	t.Helper()
	state := combatTestState(t, store, storyID)
	value, ok := actorStateNumber(actorStateFieldValue(state, DefaultActorID, xiuxianFieldHealth))
	if !ok {
		t.Fatal("the protagonist must have a settled health pool")
	}
	return value
}

func combatTestProtagonistMana(t *testing.T, store *Store, storyID string) float64 {
	t.Helper()
	state := combatTestState(t, store, storyID)
	value, ok := actorStateNumber(actorStateFieldValue(state, DefaultActorID, xiuxianFieldMana))
	if !ok {
		t.Fatal("the protagonist must have a settled mana pool")
	}
	return value
}

// combatTestAppendBattle writes one battle record straight through the side
// record path, bypassing the opening guard.
//
// It is how a journal written before the guard is reproduced: the store's own
// opening path now refuses a second fight, but a save from an older build can
// already hold two, and the round path has to cope with that.
func combatTestAppendBattle(t *testing.T, store *Store, storyID, sessionID string, specs []CombatantSpec, commandID, hash string) cultivation.CombatSession {
	t.Helper()
	state := combatTestState(t, store, storyID)
	system := combatTestSystemFromStore(t, store, storyID)
	session, outcome, err := ResolveCombatExchange(state, system, sessionID, commandID, specs, nil, map[string]cultivation.CombatAction{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCombatRecord(storyID, AppendCombatRecordRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Session: session, Outcome: outcome,
		CommandID: commandID, CommandHash: hash,
	}); err != nil {
		t.Fatal(err)
	}
	return session
}

// combatTestAppendRound advances a live battle one exchange with the opposition
// deciding for itself.
func combatTestAppendRound(t *testing.T, store *Store, storyID string, live cultivation.CombatSession, commandID, hash string) cultivation.CombatSession {
	t.Helper()
	state := combatTestState(t, store, storyID)
	system := combatTestSystemFromStore(t, store, storyID)
	session, outcome, err := ResolveCombatExchange(state, system, live.ID, commandID, nil, &live, battleActions(live, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCombatRecord(storyID, AppendCombatRecordRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Session: session, Outcome: outcome,
		CommandID: commandID, CommandHash: hash,
	}); err != nil {
		t.Fatal(err)
	}
	return session
}

// TestStartBattleRefusesASecondBattleForTheSameActor is R1's first half: one
// person fights one battle at a time, so a second one is refused before anything
// is resolved or committed.
func TestStartBattleRefusesASecondBattleForTheSameActor(t *testing.T) {
	store, storyID := combatTestStoryWithSecondEnemy(t)

	first, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open-a", CommandHash: "hash-open-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	healthBefore := combatTestProtagonistHealth(t, store, storyID)
	recordsBefore := combatTestRecordCount(t, store, storyID)

	_, err = store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: []CombatantSpec{
			{ActorID: DefaultActorID, Side: cultivation.SidePlayer, RouteExists: true},
			{ActorID: "rival2", Side: cultivation.SideEnemy, RouteExists: true, Retreat: cultivation.RetreatSurvival},
		},
		CommandID: "cmd-open-b", CommandHash: "hash-open-b",
	})
	if !errors.Is(err, ErrCombatActorAlreadyFighting) {
		t.Fatalf("a second battle for the same actor must be refused, got %v", err)
	}
	var conflict *CombatActorConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("the refusal must name the battle holding the actor: %#v", err)
	}
	if conflict.SessionID != first.Session.ID {
		t.Fatalf("the refusal must name the live battle: got %q want %q", conflict.SessionID, first.Session.ID)
	}
	if len(conflict.Actors) != 1 || conflict.Actors[0] != DefaultActorID {
		t.Fatalf("the refusal must name the shared actor: %#v", conflict.Actors)
	}
	if health := combatTestProtagonistHealth(t, store, storyID); health != healthBefore {
		t.Fatalf("a refused opening must not settle a round: health %v -> %v", healthBefore, health)
	}
	if count := combatTestRecordCount(t, store, storyID); count != recordsBefore {
		t.Fatalf("a refused opening must not commit a record: %d -> %d", recordsBefore, count)
	}
	sessions, err := store.CombatSessions(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("only the first battle may be live: %#v", sessions)
	}
}

// TestStartBattleReopensTheSameEncounter is R1's other half: opening the same
// encounter again returns the battle already running rather than refusing the
// player entry to the fight they are in.
func TestStartBattleReopensTheSameEncounter(t *testing.T) {
	store, storyID := combatTestStory(t)

	first, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open-a", CommandHash: "hash-open-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	healthBefore := combatTestProtagonistHealth(t, store, storyID)

	reopened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open-again", CommandHash: "hash-open-again",
	})
	if err != nil {
		t.Fatalf("reopening the same encounter must not fail: %v", err)
	}
	if reopened.Session.ID != first.Session.ID {
		t.Fatalf("reopening must address the live battle: got %q want %q", reopened.Session.ID, first.Session.ID)
	}
	if reopened.Settled {
		t.Fatal("reopening must not settle another round")
	}
	if reopened.EventID != "" {
		t.Fatalf("reopening must not commit a record, got %q", reopened.EventID)
	}
	if health := combatTestProtagonistHealth(t, store, storyID); health != healthBefore {
		t.Fatalf("reopening must not change the pools: %v -> %v", healthBefore, health)
	}
	if count := combatTestRecordCount(t, store, storyID); count != 1 {
		t.Fatalf("reopening must not commit a second record, got %d", count)
	}
}

// TestResolveBattleRoundRefusesABattleSupersededByALaterSettlement is the
// regression for the reported 89→148: a journal written before the guard can
// already hold two live battles for one person, and continuing the older one must
// not write its snapshot over the newer fight's settlement.
func TestResolveBattleRoundRefusesABattleSupersededByALaterSettlement(t *testing.T) {
	store, storyID := combatTestStoryWithSecondEnemy(t)

	older := combatTestAppendBattle(t, store, storyID, "cbt-older", combatTestSpecs(), "cmd-older", "hash-older")
	olderSnapshot := combatTestProtagonistHealth(t, store, storyID)

	newerSpecs := []CombatantSpec{
		{ActorID: DefaultActorID, Side: cultivation.SidePlayer, RouteExists: true},
		{ActorID: "rival2", Side: cultivation.SideEnemy, RouteExists: true, Retreat: cultivation.RetreatSurvival},
	}
	newer := combatTestAppendBattle(t, store, storyID, "cbt-newer", newerSpecs, "cmd-newer", "hash-newer")

	// Fight the newer battle until the protagonist is hurt more than the older
	// battle's snapshot records. That gap is exactly what the stale round would
	// have written back.
	for round := 0; round < 6 && combatTestProtagonistHealth(t, store, storyID) >= olderSnapshot; round++ {
		newer = combatTestAppendRound(t, store, storyID, newer, commandIDFor(100+round), "hash-newer-round")
	}
	wounded := combatTestProtagonistHealth(t, store, storyID)
	if wounded >= olderSnapshot {
		t.Fatalf("the newer battle must wound the protagonist below the older snapshot: older=%v now=%v", olderSnapshot, wounded)
	}
	recordsBefore := combatTestRecordCount(t, store, storyID)

	// Returning to the older fight and striking must not restore the old pool.
	_, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: older.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
		CommandID: "cmd-older-round", CommandHash: "hash-older-round",
	})
	if !errors.Is(err, ErrCombatStaleBattle) {
		t.Fatalf("continuing a superseded battle must be refused, got %v", err)
	}
	var conflict *CombatActorConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("the refusal must name the superseding battle: %#v", err)
	}
	if conflict.SessionID != newer.ID {
		t.Fatalf("the refusal must name the battle that settled last: got %q want %q", conflict.SessionID, newer.ID)
	}
	if health := combatTestProtagonistHealth(t, store, storyID); health != wounded {
		t.Fatalf("a refused round must not restore the old pools: %v -> %v", wounded, health)
	}
	if count := combatTestRecordCount(t, store, storyID); count != recordsBefore {
		t.Fatalf("a refused round must not commit a record: %d -> %d", recordsBefore, count)
	}

	// The battle that did settle last is still playable.
	if _, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: newer.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival2"},
		},
		CommandID: "cmd-newer-next", CommandHash: "hash-newer-next",
	}); err != nil {
		t.Fatalf("the live battle must still be playable: %v", err)
	}
}

// TestStartBattleLetsAFreedActorFightAgain keeps the guard from becoming a
// permanent ban: a battle that has reached a result holds nobody.
func TestStartBattleLetsAFreedActorFightAgain(t *testing.T) {
	store, storyID := combatTestStoryWithSecondEnemy(t)

	ended := combatTestAppendBattle(t, store, storyID, "cbt-done", combatTestSpecs(), "cmd-done", "hash-done")
	ended.Ended = true
	ended.Result = cultivation.ResultPlayerWon
	if _, err := store.AppendCombatRecord(storyID, AppendCombatRecordRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Session: ended, Outcome: cultivation.ExchangeOutcome{Round: ended.Round, Ended: true, Result: cultivation.ResultPlayerWon},
		CommandID: "cmd-done-end", CommandHash: "hash-done-end",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 午时",
		Combatants: []CombatantSpec{
			{ActorID: DefaultActorID, Side: cultivation.SidePlayer, RouteExists: true},
			{ActorID: "rival2", Side: cultivation.SideEnemy, RouteExists: true, Retreat: cultivation.RetreatSurvival},
		},
		CommandID: "cmd-rematch", CommandHash: "hash-rematch",
	}); err != nil {
		t.Fatalf("a finished battle must not keep its actors out of a new one: %v", err)
	}
}

// TestCombatBattlesOnTwoBranchesStayApart keeps the guard branch-local: the same
// person may be fighting in two branches, because only one of them is the branch
// being played.
func TestCombatBattlesOnTwoBranchesStayApart(t *testing.T) {
	store, storyID := combatTestStoryWithSecondEnemy(t)

	branch, err := store.CreateBranch(storyID, CreateBranchRequest{
		ParentEventID: combatTestLatestTurnID(t, store, storyID),
		Title:         "另一条路",
	})
	if err != nil {
		t.Fatal(err)
	}

	onMain, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时", Combatants: combatTestSpecs(),
		CommandID: "cmd-main", CommandHash: "hash-main",
	})
	if err != nil {
		t.Fatal(err)
	}
	onFork, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: branch.ID, WorldTime: "第1日 辰时", Combatants: combatTestSpecs(),
		CommandID: "cmd-fork", CommandHash: "hash-fork",
	})
	if err != nil {
		t.Fatalf("a branch must be able to fight its own battle: %v", err)
	}
	if onFork.Session.ID == onMain.Session.ID {
		t.Fatal("two branches must not share one battle")
	}
	for _, target := range []struct{ branchID, sessionID string }{
		{"main", onMain.Session.ID},
		{branch.ID, onFork.Session.ID},
	} {
		if _, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
			BranchID: target.branchID, SessionID: target.sessionID,
			Actions: map[string]cultivation.CombatAction{
				DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
			},
			CommandID: "cmd-" + target.branchID + "-round", CommandHash: "hash-round",
		}); err != nil {
			t.Fatalf("branch %s must advance its own battle: %v", target.branchID, err)
		}
	}
}
