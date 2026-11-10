package interactive

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ta/internal/cultivation"
)

// Synthetic Store integration, not natural AI play or evidence of natural acquisition.
func TestCraftPartialFailurePreservesProductsAcrossRetryReloadAndFork(t *testing.T) {
	store, id := f16CraftStory(t, 6)
	before, err := store.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: before.CurrentTurn.ID, Title: "制作前"})
	if err != nil {
		t.Fatal(err)
	}
	identity := b1CommitInput(t, store, id, f16UnluckyIdentity(t), "制作三批回春丹")
	input := &TurnCraftInput{MethodID: "craft.alchemy.spring_return", DurationSeconds: 3 * 360 * 60}
	outcome, refusal := planTurnCraftCommit(f16SubmissionSystem(t, store, id), DefaultActorID, before.State, input, identity, "")
	if refusal != "" {
		t.Fatal(refusal)
	}
	if reason := craftLossRewriteReason(cultivation.ReferenceRules(),
		outcome); !strings.Contains(reason, "2") || strings.Contains(reason, "没有产出任何成品") {
		t.Errorf("partial loss feedback discarded successful batches: %s", reason)
	}
	prepared, receipt := f16CraftSubmission(t, store, id, identity, input.DurationSeconds)
	receiptJSON, _ := json.Marshal(receipt)
	if !receipt.NarrativeRewriteRequired || receipt.ModuleStatus.Craft != TurnSubmissionModuleAccepted || prepared.TurnResult().Craft == nil || !strings.Contains(receipt.NarrativeRewriteReason, "2批成功") || strings.Contains(string(receiptJSON), "nothing was produced") {
		t.Errorf("partial loss receipt must retain the craft and describe its real yield: %s", receiptJSON)
	}
	req := AppendTurnWithStateRequest{BranchID: "main", User: "制作三批回春丹", Narrative: "前两炉成丹，最后一炉损失。", TurnResult: &TurnResult{Choices: testTurnChoices(), Craft: input}, AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle}
	turn, _, err := store.AppendTurnWithState(id, req)
	if err != nil {
		t.Fatal(err)
	}
	pack := f16StoredPack(t, store, id)
	herb, _ := pack["青萝草"].(map[string]any)
	pills, _ := pack["pill.restore.health.lower"].(map[string]any)
	if xiuxianCarriedQuantity(herb) != 0 || xiuxianCarriedQuantity(pills) != 2 {
		t.Errorf("three paid batches with one failure must retain two products: %#v", pack)
	}
	clock, _, err := store.CultivationState(id, "main")
	if err != nil || clock.Seconds != input.DurationSeconds {
		t.Fatalf("all performed batches must pay time: %+v %v", clock, err)
	}
	committed, _ := store.Snapshot(id, "main")
	store.mu.Lock()
	_, records, readErr := store.readStoryRecentLocked(id, "main")
	store.mu.Unlock()
	if readErr != nil {
		t.Fatal(readErr)
	}
	var recorded CraftRecordEvent
	for _, record := range records {
		if record.Envelope.Type == StoryEventTypeCraftRecord {
			if err := mapToStruct(record.Raw, &recorded); err != nil {
				t.Fatal(err)
			}
		}
	}
	if recorded.TurnID != turn.ID || recorded.Batches != 3 || recorded.FailedBatches != 1 || len(recorded.Products) != 1 || recorded.Products[0].Quantity != 2 {
		t.Fatalf("journal did not retain the mixed outcome: %+v", recorded)
	}
	retried, _, err := store.AppendTurnWithState(id, req)
	if err != nil || retried.ID != turn.ID {
		t.Fatalf("same command did not replay: %v", err)
	}
	if b1RecordCount(t, store, id, StoryEventTypeCraftRecord) != 1 {
		t.Fatal("retry added a second craft")
	}
	root := store.Root()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	defer reopened.Close()
	cold, err := reopened.Snapshot(id, "main")
	if err != nil || !reflect.DeepEqual(cold.State, committed.State) {
		t.Fatalf("cold replay changed craft: %v", err)
	}
	retried, _, err = reopened.AppendTurnWithState(id, req)
	if err != nil || retried.ID != turn.ID {
		t.Fatalf("cold retry did not return receipt: %v", err)
	}
	coldAfter, _ := reopened.Snapshot(id, "main")
	if !reflect.DeepEqual(coldAfter.State, cold.State) || b1RecordCount(t, reopened, id, StoryEventTypeCraftRecord) != 1 {
		t.Fatal("cold retry recharged or rerolled craft")
	}
	coldClock, _, err := reopened.CultivationState(id, "main")
	if err != nil || coldClock.Seconds != input.DurationSeconds {
		t.Fatalf("cold retry changed the paid time: %+v %v", coldClock, err)
	}
	old, err := reopened.Snapshot(id, fork.ID)
	if err != nil || !reflect.DeepEqual(old.State, before.State) {
		t.Fatalf("future craft leaked to prior branch: %v", err)
	}
	oldClock, _, err := reopened.CultivationState(id, fork.ID)
	if err != nil || oldClock.Seconds != 0 {
		t.Fatalf("future craft time leaked to prior branch: %+v %v", oldClock, err)
	}
}

func TestCraftToolRejectsAbsentInvalidAndUnownedCopies(t *testing.T) {
	method, _ := cultivation.NewCraftRegistryWithBuiltins().Method("craft.talisman.ward")
	method.ID, method.Source, method.FailureCost = "world.tool_ward", "符师传授", "consume_inputs"
	method.RequiredTools = []string{"implement.lower.common"}
	method.LossChance = 0
	registry := cultivation.NewCraftRegistry()
	if err := registry.Register(method); err != nil {
		t.Fatal(err)
	}
	for name, tool := range map[string]map[string]any{
		"missing":            nil,
		"unknown definition": {"定义": "imaginary.knife", "数量": float64(1)},
		"zero quantity":      {"定义": "implement.lower.common", "数量": float64(0)},
		"negative quantity":  {"定义": "implement.lower.common", "数量": float64(-1)},
		"name alone":         {"名称": "implement.lower.common", "数量": float64(1)},
	} {
		t.Run(name, func(t *testing.T) {
			state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)
			player := state[actorStateRoot].(map[string]any)[DefaultActorID].(map[string]any)["state"].(map[string]any)
			player[xiuxianFieldImportantItems] = map[string]any{
				"paper": map[string]any{"定义": "material.talisman.paper", "数量": float64(1)},
				"ink":   map[string]any{"定义": "material.talisman.ink", "数量": float64(1)},
				"knife": tool,
			}
			before, _ := json.Marshal(state)
			batch, err := cultivation.PlanCraftBatch(registry, cultivation.CraftRequest{MethodID: method.ID, Minutes: method.MinutesPerBatch, Stage: cultivation.Qi(6), Held: craftHeldFromState(cultivation.ReferenceRules(),
				state, DefaultActorID), Draw: 0.5})
			if err == nil || batch.ElapsedMinutes != 0 || len(batch.Consumed) != 0 || len(batch.Products) != 0 {
				t.Fatalf("invalid tool reached crafting: %+v %v", batch, err)
			}
			after, _ := json.Marshal(state)
			if string(before) != string(after) {
				t.Fatal("tool refusal mutated inventory")
			}
		})
	}
}

func TestCraftNonMaterialToolQualifiesWithoutBeingConsumed(t *testing.T) {
	for _, equipped := range []bool{false, true} {
		name := "carried"
		if equipped {
			name = "equipped"
		}
		t.Run(name, func(t *testing.T) {
			method, _ := cultivation.NewCraftRegistryWithBuiltins().Method("craft.talisman.ward")
			method.ID, method.Source, method.FailureCost = "world.tool_ward", "符师传授", "consume_inputs"
			method.LossChance = 0
			method.RequiredTools = []string{"implement.lower.common"}
			if err := cultivation.ValidateCraftMethods([]cultivation.CraftMethodDef{method}, cultivation.AuthoredCraftCatalog); err != nil {
				t.Fatal(err)
			}
			store, id, root := talismanStory(t, method)
			talismanTurn(t, store, id, "main", "学法备料", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: method.ID, Source: "符师传授"}}}, Trade: &TurnTradeInput{Merchant: "符铺", Buys: []TurnTradeBuy{{DefID: "material.talisman.paper", Name: "符纸", Quantity: 2}, {DefID: "material.talisman.ink", Name: "灵墨", Quantity: 2}}}})
			craft := &TurnCraftInput{MethodID: method.ID, DurationSeconds: method.MinutesPerBatch * 60}
			before, _ := store.Snapshot(id, "main")
			out, refusal := planTurnCraftCommit(f16SubmissionSystem(t, store, id), DefaultActorID, before.State, craft, b1Identity("no-tool", 1), "")
			if refusal == "" || out.elapsed != 0 || len(out.ops) != 0 {
				t.Fatal("missing tool was not refused without charging")
			}
			items := &TurnItemsInput{Acquire: []TurnItemAcquire{{DefID: "implement.lower.common", Name: "符师的小刀", Source: "符师借出刻符工具"}}}
			if equipped {
				items.Equip = []TurnItemEquip{{InstanceID: "符师的小刀"}}
			}
			talismanTurn(t, store, id, "main", "借用刻符小刀", &TurnResult{Items: items})
			before, _ = store.Snapshot(id, "main")
			identity := b1CommitInput(t, store, id, b1Identity("tool-craft-"+name, 1), "用小刀制符")
			req := AppendTurnWithStateRequest{BranchID: "main", User: "用小刀制符", Narrative: "借小刀刻符，符纸与灵墨各用一份。", TurnResult: &TurnResult{Choices: testTurnChoices(), Craft: craft}, AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle}
			turn, _, err := store.AppendTurnWithState(id, req)
			if err != nil {
				t.Fatal(err)
			}
			pack := f16StoredPack(t, store, id)
			ward, _ := pack["talisman.ward.lower"].(map[string]any)
			if ward == nil || xiuxianCarriedQuantity(ward) != 1 {
				t.Errorf("owned non-material tool never reached craft consumer: %#v", pack)
			}
			for _, key := range []string{"符纸", "灵墨"} {
				entry, _ := pack[key].(map[string]any)
				if xiuxianCarriedQuantity(entry) != 1 {
					t.Errorf("wrong actual material cost: %s=%#v", key, entry)
				}
			}
			after, _ := store.Snapshot(id, "main")
			if equipped {
				want, _ := xiuxianEquipmentEntry(before.State, DefaultActorID, xiuxianFieldImplement)
				got, _ := xiuxianEquipmentEntry(after.State, DefaultActorID, xiuxianFieldImplement)
				if !reflect.DeepEqual(want, got) {
					t.Fatal("craft consumed or changed equipped tool")
				}
			} else if tool, _ := pack["符师的小刀"].(map[string]any); xiuxianCarriedQuantity(tool) != 1 {
				t.Fatal("craft consumed its non-material tool")
			}
			clock, _, err := store.CultivationState(id, "main")
			if err != nil || clock.Seconds != craft.DurationSeconds {
				t.Errorf("tool craft did not spend real time: %+v %v", clock, err)
			}
			_ = store.Close()
			reopened := NewStore(root)
			defer reopened.Close()
			retry, _, err := reopened.AppendTurnWithState(id, req)
			if err != nil || retry.ID != turn.ID {
				t.Fatalf("tool craft cold retry: %v", err)
			}
			cold, _ := reopened.Snapshot(id, "main")
			if !reflect.DeepEqual(cold.State, after.State) {
				t.Fatal("tool craft changed on reopen/retry")
			}
		})
	}
}
