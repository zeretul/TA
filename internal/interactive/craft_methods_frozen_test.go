package interactive

import (
	"encoding/json"
	"strings"
	"ta/internal/cultivation"
	"testing"
)

func TestAuthoredCraftFrozenLearningPreviewCommitForkAndReload(t *testing.T) {
	method, _ := cultivation.NewCraftRegistryWithBuiltins().Method("craft.talisman.ward")
	method.ID = "world.ward"
	method.Source = "local teacher"
	method.Version = 3
	method.FailureCost = "consume_inputs"
	method.LossChance = 0
	store, id, root := talismanStory(t, method)
	system := f16SubmissionSystem(t, store, id)
	method.Inputs[0].Quantity = 99
	method.Version = 4
	frozen, ok := storyCraftRegistry(system).Method(method.ID)
	if !ok || frozen.Version != 3 || frozen.Inputs[0].Quantity != 1 {
		t.Fatalf("author edit changed frozen method: %+v", frozen)
	}
	other, otherID, _ := talismanStory(t)
	if _, ok := storyCraftRegistry(f16SubmissionSystem(t, other, otherID)).Method(method.ID); ok {
		t.Fatal("method leaked to another world")
	}
	input := DecodeInteractiveTurnSubmissionInput(`{"skills":{"skills":[{"def_id":"world.ward","version":3,"source":"师父当面传授"}]},"trade":{"merchant":"符铺","buys":[{"def_id":"material.talisman.paper","name":"符纸","quantity":2},{"def_id":"material.talisman.ink","name":"符墨","quantity":2}]},"craft":{"method_id":"world.ward","duration_seconds":7200}}`)
	identity := b1Identity("custom-craft", 1)
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{ActorState: system, CurrentState: combatTestState(t, store, id), ChoiceCount: 5, CraftIdentity: identity}, nil, input)
	if receipt.ModuleStatus.Craft != TurnSubmissionModuleAccepted || receipt.NarrativeRewriteRequired {
		t.Fatalf("preview refused registered method: %+v", receipt)
	}
	identity = b1CommitInput(t, store, id, identity, "学制新符")
	result := prepared.TurnResult()
	result.Choices = testTurnChoices()
	turn, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "学制新符", Narrative: "制成新符。", TurnResult: &result, AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle})
	if err != nil {
		t.Fatal(err)
	}
	state := combatTestState(t, store, id)
	if !craftMethodKnown(state, DefaultActorID, frozen) || !strings.Contains(CraftMethodsContext(system, state, DefaultActorID, StoryContextMaxBytes), "world.ward") {
		t.Fatal("learning/context use a different registry")
	}
	store.mu.Lock()
	_, records, readErr := store.readStoryRecentLocked(id, "main")
	store.mu.Unlock()
	if readErr != nil {
		t.Fatal(readErr)
	}
	var source CraftRecordEvent
	for _, record := range records {
		if record.Envelope.Type == StoryEventTypeCraftRecord {
			if err := mapToStruct(record.Raw, &source); err != nil {
				t.Fatal(err)
			}
		}
	}
	if source.MethodVersion != 3 {
		t.Fatal("craft event lost frozen method version")
	}
	if ops := craftRerollActorOps(system, state, turn, source); len(ops) == 0 {
		t.Fatal("reroll cannot read frozen custom method")
	}
	if rerolled := craftRerollRecord(system, state, turn, source, source.ID, source.Ts); rerolled == nil || rerolled.MethodVersion != 3 || rerolled.ClockAfter != source.ClockAfter {
		t.Fatal("reroll changed frozen method or paid clock twice")
	}
	wrongVersion := source
	wrongVersion.MethodVersion = 4
	if len(craftRerollActorOps(system, state, turn, wrongVersion)) != 0 {
		t.Fatal("reroll accepted a different method version")
	}
	branch, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: turn.ID, Title: "方法分支"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	defer reopened.Close()
	for _, bid := range []string{"main", branch.ID} {
		before, err := reopened.Snapshot(id, bid)
		if err != nil {
			t.Fatal(err)
		}
		learned := craftMethodKnown(before.State, DefaultActorID, frozen)
		if !learned {
			t.Fatal("reopen/fork lost versioned learning")
		}
		talismanTurn(t, reopened, id, bid, "再次制作"+bid, &TurnResult{Craft: &TurnCraftInput{MethodID: frozen.ID, DurationSeconds: 7200}})
		after, err := reopened.Snapshot(id, bid)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(after.State)
		if !strings.Contains(string(data), "talisman.ward.lower") {
			t.Fatal("restored method could not produce real item")
		}
		beforeJSON, _ := json.Marshal(before.State)
		if string(beforeJSON) == string(data) {
			t.Fatal("restored craft made no state change")
		}
	}
}
