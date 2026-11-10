package interactive

import (
	"fmt"
	"reflect"
	"ta/internal/cultivation"
	"testing"
)

func TestIter4FourArtsRequireRealLearningBeforeProduction(t *testing.T) {
	for _, method := range cultivation.NewCraftRegistryWithBuiltins().Methods() {
		t.Run(string(method.Art), func(t *testing.T) {
			if !method.RequiresLearning {
				t.Fatal("new default method grants unearned craft ability")
			}
			store, id, root := talismanStory(t)
			before, _ := store.Snapshot(id, "main")
			var acquire []TurnItemAcquire
			for _, input := range method.Inputs {
				acquire = append(acquire, TurnItemAcquire{DefID: input.DefID, Quantity: input.Quantity * 2, Source: "师傅交来的两批实物材料"})
			}
			talismanTurn(t, store, id, "main", "收到两批材料", &TurnResult{Items: &TurnItemsInput{Acquire: acquire}})
			ready, _ := store.Snapshot(id, "main")
			talismanTurn(t, store, id, "main", "未学方法尝试制作", &TurnResult{Craft: &TurnCraftInput{MethodID: method.ID, DurationSeconds: method.MinutesPerBatch * 120}})
			refused, _ := store.Snapshot(id, "main")
			if SnapshotClockSeconds(refused) != SnapshotClockSeconds(ready) || !reflect.DeepEqual(actorStateFieldValue(ready.State, DefaultActorID, xiuxianFieldImportantItems), actorStateFieldValue(refused.State, DefaultActorID, xiuxianFieldImportantItems)) {
				t.Fatal("unlearned craft spent materials or time")
			}
			talismanTurn(t, store, id, "main", "在师傅指导下真正学会方法", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: method.ID, Source: "师傅当面传授并核过操作"}}}})
			learned, _ := store.Snapshot(id, "main")
			if !craftMethodKnown(learned.State, DefaultActorID, method) || SnapshotClockSeconds(learned) != SnapshotClockSeconds(before) {
				t.Fatal("formal craft learning absent or invented skill days")
			}
			var identity DomainCommitIdentity
			for n := 1; n < 1000; n++ {
				identity = b1Identity(fmt.Sprintf("arts-%s-%d", method.ID, n), 1)
				if craftDraw(identity, "", method.ID) >= method.LossChance {
					break
				}
			}
			identity = b1CommitInput(t, store, id, identity, "按已学方法制作两批")
			crafted, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "按已学方法制作两批", Narrative: "按方法完成两批制作。", TurnResult: &TurnResult{Choices: testTurnChoices(), Craft: &TurnCraftInput{MethodID: method.ID, DurationSeconds: method.MinutesPerBatch * 120}}, AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle})
			if err != nil {
				t.Fatal(err)
			}
			product := method.Outputs[0].DefID
			afterCraft, _ := store.Snapshot(id, "main")
			if SnapshotClockSeconds(afterCraft)-SnapshotClockSeconds(learned) != method.MinutesPerBatch*120 || savedCarriedQuantity(t, store, id, DefaultActorID, product) != 2 {
				t.Fatalf("craft output/time not real: %s", product)
			}
			fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: crafted.ID, Title: "保留两份成品"})
			if err != nil {
				t.Fatal(err)
			}
			switch method.Art {
			case cultivation.CraftArtAlchemy:
				_, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "整理擦伤", Narrative: "主角手臂有伤。", ActorOps: []ActorStateOp{{Op: "set", ActorID: DefaultActorID, FieldID: xiuxianFieldHealth, Value: float64(100)}}, TurnResult: &TurnResult{Choices: testTurnChoices()}})
				if err != nil {
					t.Fatal(err)
				}
				talismanTurn(t, store, id, "main", "服下自己炼的一枚丹药", &TurnResult{Items: &TurnItemsInput{Use: []TurnItemUse{{InstanceID: product}}}})
				used, _ := store.Snapshot(id, "main")
				if xiuxianStoredInt(used.State, DefaultActorID, xiuxianFieldHealth) <= 100 || savedCarriedQuantity(t, store, id, DefaultActorID, product) != 1 {
					t.Fatal("crafted pill did not heal and consume")
				}
			case cultivation.CraftArtTalisman:
				session := openCarryingBattle(t, store, id)
				next, out := playDoseRound(t, store, id, session, "iter4-made-ward", product)
				if !findCombatStep(t, out, DefaultActorID).ItemApplied || next.CombatantByID(DefaultActorID).Shield.Amount <= 0 || savedCarriedQuantity(t, store, id, DefaultActorID, product) != 1 {
					t.Fatal("crafted ward had no battle effect")
				}
			default:
				talismanTurn(t, store, id, "main", "使用自己制作的器物", &TurnResult{Items: &TurnItemsInput{Equip: []TurnItemEquip{{InstanceID: product}}}})
				equipped, _ := store.Snapshot(id, "main")
				if xiuxianStoredInt(equipped.State, DefaultActorID, xiuxianFieldAttack) <= xiuxianStoredInt(afterCraft.State, DefaultActorID, xiuxianFieldAttack) {
					t.Fatal("crafted equipment did not affect actual attack")
				}
				slot := xiuxianFieldImplement
				if method.Art == cultivation.CraftArtFormation {
					slot = xiuxianFieldFormation
					if xiuxianStoredInt(equipped.State, DefaultActorID, xiuxianFieldMana) >= xiuxianStoredInt(afterCraft.State, DefaultActorID, xiuxianFieldMana) {
						t.Fatal("formation activation was free")
					}
				}
				talismanTurn(t, store, id, "main", "收起制作器物", &TurnResult{Items: &TurnItemsInput{Unequip: []TurnItemUnequip{{Slot: slot}}}})
			}
			preSale, _ := store.Snapshot(id, "main")
			purse := xiuxianStoredInt(preSale.State, DefaultActorID, xiuxianFieldCurrency)
			talismanTurn(t, store, id, "main", "出售一份制成品", &TurnResult{Trade: &TurnTradeInput{Merchant: "坊市作坊", Sells: []TurnTradeSell{{InstanceID: product, Quantity: 1}}}})
			final, _ := store.Snapshot(id, "main")
			if xiuxianStoredInt(final.State, DefaultActorID, xiuxianFieldCurrency) <= purse {
				t.Fatal("crafted product did not pay real currency")
			}
			_ = store.Close()
			store = NewStore(root)
			defer store.Close()
			cold, err := store.Snapshot(id, "main")
			if err != nil || !reflect.DeepEqual(cold.State, final.State) {
				t.Fatalf("four-art cold replay changed: %v", err)
			}
			unspent, _ := store.Snapshot(id, fork.ID)
			pack, _ := xiuxianEquipmentEntry(unspent.State, DefaultActorID, xiuxianFieldImportantItems)
			entry, _ := pack[product].(map[string]any)
			if xiuxianCarriedQuantity(entry) != 2 {
				t.Fatal("use/sale leaked into earlier branch")
			}
		})
	}
}
