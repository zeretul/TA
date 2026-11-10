package interactive

import (
	"reflect"
	"testing"

	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

func TestIter4FatalityRestoreGuardSurvivesReplayAndRespectsFork(t *testing.T) {
	root, store, storyID, _, safeTurnID := combatFatalityTestStory(t, "rival")
	opened, err := store.StartBattle(storyID, BattleStartRequest{BranchID: "main", Combatants: combatTestSpecs(), CommandID: "immutable-death-open", CommandHash: "immutable-death-open"})
	if err != nil {
		t.Fatal(err)
	}
	commandID := combatFatalityCommandForHit(opened.Session, DefaultActorID, "rival")
	played, err := store.ResolveBattleRound(storyID, BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, PlayerIntent: PlayerCombatIntentTakeLife, Actions: map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"}, "rival": {Kind: cultivation.ActionRest}}, CommandID: commandID, CommandHash: commandID})
	if err != nil || len(played.Fatalities) != 1 {
		t.Fatalf("fatality: %+v %v", played, err)
	}
	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: safeTurnID, Title: "before fatal result"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	defer reopened.Close()
	before, err := reopened.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	archived, ok := actorArchiveRecordFromState(before.State, "rival")
	if !ok || archived.DeathEventID != played.EventID {
		t.Fatalf("missing authoritative death provenance: %+v", archived)
	}
	req := AppendTurnWithStateRequest{BranchID: "main", User: "it returns", Narrative: "It is alive again.", TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{
		{Op: interactivestate.Restore, Path: "/rival", Value: map[string]any{"reason": "called back"}},
		{Op: interactivestate.Replace, Path: "/rival/生命值", Value: 100},
	}}}
	for attempt := 0; attempt < 2; attempt++ {
		if _, _, err := reopened.AppendTurnWithState(storyID, req); err == nil {
			t.Fatal("ordinary restore must not erase a settled death")
		}
		after, err := reopened.Snapshot(storyID, "main")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before.State, after.State) || before.CurrentTurn.ID != after.CurrentTurn.ID {
			t.Fatal("rejected restore committed partial fields or a new turn")
		}
	}
	sibling, err := reopened.Snapshot(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if actorIsArchived(sibling.State, "rival") {
		t.Fatal("pre-death fork inherited the later death")
	}
}
