package interactive

import "testing"

// A gift and payment commit through the real turn transaction. Reload, retries
// and branching must keep both sides of each transfer together.
func TestItemGiftAndNPCPaymentSurviveRetryReloadAndBranch(t *testing.T) {
	system := xiuxianTestSystem()
	for i := range system.InitialActors {
		if system.InitialActors[i].ID == DefaultActorID {
			system.InitialActors[i].State = map[string]any{xiuxianFieldRealm: "qi_7", xiuxianFieldCurrency: float64(20)}
		}
	}
	system.InitialActors = append(system.InitialActors, ActorStateInitialActor{
		ID: "giver", Name: "赠药人", TemplateID: ActorStateImportantCharacterTemplateID,
		State: map[string]any{xiuxianFieldRealm: "qi_7", xiuxianFieldCurrency: float64(6),
			xiuxianFieldImportantItems: map[string]any{"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(5)}},
		},
	})
	root := t.TempDir()
	store := NewStore(root)
	defer func() { _ = store.Close() }()
	story, err := store.CreateStory(CreateStoryRequest{Title: "赠药与还款", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	opening, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "见面", Narrative: "赠药人打开药瓶。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	identity := b1CommitInput(t, store, story.ID, b1Identity("gift-payment", 1), "接过一颗药和四枚灵石")
	req := AppendTurnWithStateRequest{BranchID: "main", User: "接过一颗药和四枚灵石", Narrative: "对方递来一颗药，又还了四枚灵石。",
		AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle,
		TurnResult: &TurnResult{Choices: testTurnChoices(),
			Items: &TurnItemsInput{Acquire: []TurnItemAcquire{{DefID: "pill.restore.health.lower", Name: "回春丹", From: "giver", Quantity: 1}}},
			Trade: &TurnTradeInput{Payments: []TurnTradePayment{{From: "giver", To: DefaultActorID, Amount: 4}}},
		},
	}
	first, _, err := store.AppendTurnWithState(story.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := store.AppendTurnWithState(story.ID, req)
	if err != nil || again.ID != first.ID {
		t.Fatalf("retry did not return the original turn: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(root)
	assertState := func(branch string, playerCoin, donorCoin, playerDose, donorDose int) {
		t.Helper()
		snapshot, err := store.Snapshot(story.ID, branch)
		if err != nil {
			t.Fatal(err)
		}
		for actor, want := range map[string]int{DefaultActorID: playerCoin, "giver": donorCoin} {
			if got := xiuxianStoredInt(snapshot.State, actor, xiuxianFieldCurrency); got != want {
				t.Fatalf("%s %s coin=%d, want %d", branch, actor, got, want)
			}
		}
		for actor, want := range map[string]int{DefaultActorID: playerDose, "giver": donorDose} {
			pack, _ := xiuxianEquipmentEntry(snapshot.State, actor, xiuxianFieldImportantItems)
			got := 0
			if entry, ok := pack["回春丹"].(map[string]any); ok {
				got = xiuxianCarriedQuantity(entry)
			}
			if got != want {
				t.Fatalf("%s %s doses=%d, want %d", branch, actor, got, want)
			}
		}
	}
	assertState("main", 24, 2, 1, 4)
	before, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: opening.ID, Title: "赠送之前"})
	if err != nil {
		t.Fatal(err)
	}
	assertState(before.ID, 20, 6, 0, 5)
	after, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: first.ID, Title: "赠送之后"})
	if err != nil {
		t.Fatal(err)
	}
	assertState(after.ID, 24, 2, 1, 4)
}
