package interactive

import (
	"reflect"
	"testing"
)

// Synthetic Store integration; natural model play is checked separately.
func TestCraftWorkResumesAfterReloadWithoutDoublePayment(t *testing.T) {
	store, id := f16CraftStory(t, 2)
	before, _ := store.Snapshot(id, "main")
	identity := b1CommitInput(t, store, id, b1Identity("partial-craft", 1), "先炼三个小时，停下来")
	input := &TurnCraftInput{MethodID: "craft.alchemy.spring_return", DurationSeconds: 3 * 3600}
	outcome, refusal := planTurnCraftCommit(f16SubmissionSystem(t, store, id), DefaultActorID, before.State, input, identity, "")
	if refusal != "" || outcome.elapsed != 3*3600 || len(outcome.batch.Products) != 0 {
		t.Fatalf("work must start without issuing a product: %s %+v", refusal, outcome)
	}
	request := AppendTurnWithStateRequest{BranchID: "main", User: "先炼三个小时，停下来", Narrative: "先停在半途，稍后续做。", TurnResult: &TurnResult{Choices: testTurnChoices(), Craft: input}, AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle}
	turn, _, err := store.AppendTurnWithState(id, request)
	if err != nil {
		t.Fatal(err)
	}
	partial, _ := store.Snapshot(id, "main")
	fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: partial.CurrentTurn.ID, Title: "半途分支"})
	if err != nil {
		t.Fatal(err)
	}
	root := store.Root()
	store.Close()
	store = NewStore(root)
	defer store.Close()
	cold, err := store.Snapshot(id, "main")
	if err != nil || !reflect.DeepEqual(cold.State, partial.State) {
		t.Fatalf("work lost after reopen: %v", err)
	}
	replay, _, err := store.AppendTurnWithState(id, request)
	if err != nil || replay.ID != turn.ID {
		t.Fatalf("retry did not reuse original commit: %v", err)
	}
	resumedID := b1CommitInput(t, store, id, b1Identity("resume-craft", 1), "继续剩下三个小时")
	request.AgentCommandID = resumedID.CommandID
	request.AgentOperationID, request.AgentCycle = resumedID.OperationID, resumedID.Cycle
	request.User = "继续剩下三个小时"
	request.Narrative = "把剩余工序做完。"
	if _, _, err = store.AppendTurnWithState(id, request); err != nil {
		t.Fatal(err)
	}
	final, _ := store.Snapshot(id, "main")
	pack, _ := xiuxianEquipmentEntry(final.State, DefaultActorID, xiuxianFieldImportantItems)
	herb, _ := pack["青萝草"].(map[string]any)
	if xiuxianCarriedQuantity(herb) != 0 {
		t.Fatalf("inputs not paid exactly once: %+v", pack)
	}
	clock, _, err := store.CultivationState(id, "main")
	if err != nil || clock.Seconds != 6*3600 {
		t.Fatalf("wrong elapsed time: %+v %v", clock, err)
	}
	unchanged, _ := store.Snapshot(id, fork.ID)
	if !reflect.DeepEqual(unchanged.State, partial.State) {
		t.Fatal("main completion altered the paused branch")
	}
}
