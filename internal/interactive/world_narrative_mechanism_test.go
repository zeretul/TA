package interactive

import (
	interactivestate "ta/internal/interactive/state"
	"testing"
)

// This supplies the kind of result an AI infers from an authored oath rule.
// It verifies the existing submission/journal boundary, not model reasoning.
// There is deliberately no oath calculator or world-name dispatch in production.
func TestCustomSingleMechanismUsesExistingStateSubmissionAndColdFork(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	defer func() { _ = store.Close() }()
	ids := []string{}
	for _, packID := range []string{"custom-oath-island", "custom-memory-room"} {
		system := StoryDirectorActorStateSystem{WorldContent: &WorldContent{PackID: packID}, Templates: []ActorStateTemplate{{ID: DefaultActorID, Fields: []ActorStateField{{Name: "特殊状态", Type: "string", Default: "尚未发生", Description: "依照本世界规则和实际经历记录已确认的变化。"}}}}, InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, TemplateID: DefaultActorID}}}
		story, err := store.CreateStory(CreateStoryRequest{Title: packID, ActorState: &system, PackBinding: &StoryPackBinding{PackID: packID, Scope: packID}})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, story.ID)
		if _, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "观察", Narrative: "尚未作出决定。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
			t.Fatal(err)
		}
	}
	before, err := store.Snapshot(ids[0], "main")
	if err != nil {
		t.Fatal(err)
	}
	fork, err := store.CreateBranch(ids[0], CreateBranchRequest{ParentEventID: before.CurrentTurn.ID, Title: "尚未立誓"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(ids[0], AppendTurnWithStateRequest{BranchID: "main", User: "我自愿立下约定", Narrative: "见证人在场，你说完约定，手背浮现约定的印记。", TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{Op: interactivestate.Replace, Path: "/protagonist/特殊状态", Value: "约定已立；见证人在场；手背出现印记"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(root)
	for _, tc := range []struct{ id, branch, want string }{{ids[0], "main", "约定已立；见证人在场；手背出现印记"}, {ids[0], fork.ID, "尚未发生"}, {ids[1], "main", "尚未发生"}} {
		snap, err := store.Snapshot(tc.id, tc.branch)
		if err != nil {
			t.Fatal(err)
		}
		actors := snap.State[actorStateRoot].(map[string]any)
		actor := actors[DefaultActorID].(map[string]any)
		state := actor["state"].(map[string]any)
		if state["特殊状态"] != tc.want {
			t.Fatalf("world or branch contaminated: %v", state)
		}
		if ActorStateSystemIsXiuxian(snap.ActorStateSchema.System) || snap.ActorStateSchema.System.AdventureRules != nil {
			t.Fatal("single-mechanism world inherited unrelated numeric systems")
		}
		routes, err := store.JourneyOptionsFor(tc.id, tc.branch)
		if err != nil || len(routes.Routes) != 0 {
			t.Fatalf("custom world inherited routes: %v %v", routes, err)
		}
	}
}
