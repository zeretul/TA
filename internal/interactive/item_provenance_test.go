package interactive

import (
	"strings"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/exploration"
	"ta/internal/quest"
)

// §4.1 and §4.2 of 自制-装备与物品品级 separate three things that used to be one
// sentence in the narration:
//
//   - 稀有度 is where a thing can be got, not a quality tier. 独特 therefore means
//     one object in the world, and the only honest ways to hold it are to be handed
//     it by whoever really has it or to be awarded it by authored content.
//   - 来历 is the copy's real source, and a module that hands something over is the
//     one that knows it. Leaving it to the narration let a task-awarded sword read
//     as something bought at a market.
//   - 同一件实物不能记成两份, checked against the state as this submission has
//     written it, not only as it was committed before.
//
// These tests pin each of the three at the boundary where it can be broken.

// TestAFreeAcquisitionOfAUniqueCopyIsRefused is the first half of §4.1's 独特
// rule: a model that writes items.acquire without naming a real holder would
// otherwise mint a second 顶品独特 blade out of a sentence.
func TestAFreeAcquisitionOfAUniqueCopyIsRefused(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Cultivation{Realm: cultivation.RealmFoundation, Stage: cultivation.StageEarly}), nil)

	ops, result, ok := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "implement.top.unique", Name: "斩仙剑"}},
	}, "ev-unique-free")
	if !ok {
		t.Fatal("the module must accept a tracked character")
	}
	if len(ops) != 0 {
		t.Fatalf("a free unique acquisition wrote state: %+v", ops)
	}
	if len(result.Acquired) != 0 || len(result.Refused) != 1 {
		t.Fatalf("acquired = %+v refused = %+v, want one refusal", result.Acquired, result.Refused)
	}
	if !strings.Contains(result.Refused[0].Reason, "独特") {
		t.Errorf("refusal = %q, want it to say the thing is one of a kind", result.Refused[0].Reason)
	}
}

// TestAUniqueCopyHandedOverByItsHolderIsAccepted is the other half: the same
// acquisition is legal once it names the character who really has the blade,
// because that is a transfer of one existing object rather than a second one.
func TestAUniqueCopyHandedOverByItsHolderIsAccepted(t *testing.T) {
	system := xiuxianTestSystem()
	state := swordOnBothSidesState(t)
	donor := state[actorStateRoot].(map[string]any)["断道客"].(map[string]any)["state"].(map[string]any)
	donor[xiuxianFieldImplement] = map[string]any{
		"名称": "斩仙剑", "定义": "implement.top.unique",
		"适用阶段": cultivation.StageKey(cultivation.Cultivation{Realm: cultivation.RealmFoundation, Stage: cultivation.StageEarly}),
	}

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "implement.top.unique", Name: "斩仙剑", From: "断道客"}},
	}, "ev-unique-transfer")
	if len(result.Acquired) != 1 || len(result.Refused) != 0 {
		t.Fatalf("acquired = %+v refused = %+v, want the transfer accepted", result.Acquired, result.Refused)
	}
	after := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(after, "protagonist", xiuxianFieldImportantItems)
	if _, held := pack["斩仙剑"]; !held {
		t.Fatalf("the transferred blade is not in the recipient's pack: %+v", pack)
	}
	// The donor no longer holds it: one real object, one holder.
	if _, still := xiuxianEquipmentEntry(after, "断道客", xiuxianFieldImplement); still {
		if held, _ := xiuxianEquipmentEntry(after, "断道客", xiuxianFieldImplement); len(held) > 0 {
			t.Fatalf("the donor still holds the blade after handing it over: %+v", held)
		}
	}
}

// TestTheSameUniqueCopyCannotBeAcquiredTwiceUnderTwoNames is the second half of
// §4.1's 独特 rule applied inside one submission. A lookup that only read the
// committed state would let two acquisitions in the same turn each pass.
func TestTheSameUniqueCopyCannotBeAcquiredTwiceUnderTwoNames(t *testing.T) {
	system := xiuxianTestSystem()
	state := swordOnBothSidesState(t)
	donor := state[actorStateRoot].(map[string]any)["断道客"].(map[string]any)["state"].(map[string]any)
	donor[xiuxianFieldImportantItems] = map[string]any{
		"斩仙剑": map[string]any{"定义": "implement.top.unique", "数量": float64(1)},
		"断水刀": map[string]any{"定义": "implement.top.unique", "数量": float64(1)},
	}

	_, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{
			{DefID: "implement.top.unique", Name: "斩仙剑", From: "断道客"},
			{DefID: "implement.top.unique", Name: "断水刀", From: "断道客"},
		},
	}, "ev-unique-twice")
	if len(result.Acquired) != 1 {
		t.Fatalf("acquired = %+v, want exactly the first copy", result.Acquired)
	}
	if len(result.Refused) != 1 {
		t.Fatalf("refused = %+v, want the second name refused", result.Refused)
	}
	if !strings.Contains(result.Refused[0].Reason, "斩仙剑") {
		t.Errorf("refusal = %q, want it to name the entry already held", result.Refused[0].Reason)
	}
}

// TestAUniqueCopyAlreadyInThePackIsNotAcquiredAgain covers the committed-state
// half of the same check: the character already holds the definition under one
// name, so a later turn cannot record a second object under another.
func TestAUniqueCopyAlreadyInThePackIsNotAcquiredAgain(t *testing.T) {
	system := xiuxianTestSystem()
	state := swordOnBothSidesState(t)
	player := state[actorStateRoot].(map[string]any)["protagonist"].(map[string]any)["state"].(map[string]any)
	player[xiuxianFieldImportantItems] = map[string]any{
		"斩仙剑": map[string]any{"定义": "implement.top.unique", "数量": float64(1)},
	}
	donor := state[actorStateRoot].(map[string]any)["断道客"].(map[string]any)["state"].(map[string]any)
	donor[xiuxianFieldImportantItems] = map[string]any{
		"断水刀": map[string]any{"定义": "implement.top.unique", "数量": float64(1)},
	}

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "implement.top.unique", Name: "断水刀", From: "断道客"}},
	}, "ev-unique-held")
	if len(ops) != 0 || len(result.Acquired) != 0 || len(result.Refused) != 1 {
		t.Fatalf("a second unique copy was written: ops=%+v acquired=%+v refused=%+v", ops, result.Acquired, result.Refused)
	}
}

// TestATaskRewardRecordsItsOwnProvenance is §4.2 at the settlement: the copy a
// task pays for says where it came from, and that wording is the task's, not
// whatever an earlier copy under the same name happened to carry.
func TestATaskRewardRecordsItsOwnProvenance(t *testing.T) {
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)
	player := state[actorStateRoot].(map[string]any)["protagonist"].(map[string]any)["state"].(map[string]any)
	// An earlier copy of the same definition, bought at the market under the name
	// the narration used. It keeps its own entry: the module writes rewards under
	// the definition's identity so a replayed settlement cannot restyle a copy the
	// narration already recorded.
	player[xiuxianFieldImportantItems] = map[string]any{
		"回春丹": map[string]any{
			"定义": "pill.restore.health.lower", "数量": float64(2), "来历": "坊市里买的",
		},
	}
	projection := &storyBranchProjection{State: state}
	definition := quest.Definition{ID: "quest.test_provenance", Title: "药圃里的封口药瓶"}

	ops, err := questSettlementOps(projection, StoryDirectorActorStateSystem{}, definition, []quest.Reward{
		{Kind: quest.RewardItem, RefID: "pill.restore.health.lower", Quantity: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	after := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(after, DefaultActorID, xiuxianFieldImportantItems)
	entry, _ := pack["pill.restore.health.lower"].(map[string]any)
	if got := xiuxianEntryText(entry, "来历"); got != "任务「药圃里的封口药瓶」的约定报酬。" {
		t.Fatalf("来历 = %q, want the task's own wording", got)
	}
	if got := xiuxianCarriedQuantity(entry); got != 1 {
		t.Fatalf("quantity = %d, want the authored one", got)
	}
	// The copy the narration already recorded is untouched, wording and count.
	bought, _ := pack["回春丹"].(map[string]any)
	if got := xiuxianEntryText(bought, "来历"); got != "坊市里买的" {
		t.Fatalf("the purchased copy's 来历 was rewritten to %q", got)
	}
	if got := xiuxianCarriedQuantity(bought); got != 2 {
		t.Fatalf("the purchased copy's count changed to %d", got)
	}
}

// TestARealmRewardNamesTheRoomItWasTakenFrom is the same rule for a declared
// realm reward. The room is the honest answer: it is where the character really
// stood, and the definition is what says so.
func TestARealmRewardNamesTheRoomItWasTakenFrom(t *testing.T) {
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)
	projection := &storyBranchProjection{State: state}
	definition, ok := exploration.RealmDefinition(exploration.RealmGuXiuDongFu)
	if !ok {
		t.Fatal("the authored realm must be registered")
	}

	ops := realmActorOps(projection, definition, exploration.DungeonInstanceSnapshot{}, &exploration.InstanceReward{
		ID: "realm.herb_ref", RefID: "pill.restore.health.lower", Quantity: 1,
	}, "")
	after := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(after, DefaultActorID, xiuxianFieldImportantItems)
	entry, _ := pack["pill.restore.health.lower"].(map[string]any)
	if got := xiuxianEntryText(entry, "来历"); got != "在药圃取得。" {
		t.Fatalf("来历 = %q, want the room the reward was declared in", got)
	}
}

// TestACraftedProductSaysItWasMade is §4.2 for the one source that is not a
// find or a gift: the character made it, and a journal entry that said otherwise
// would read as though the pill had been bought.
func TestACraftedProductSaysItWasMade(t *testing.T) {
	if got := craftProductProvenance(); got != "自行炼制所得。" {
		t.Fatalf("craft provenance = %q", got)
	}
}

// A task reward is authored content, so it may legitimately pay a unique copy.
// It may not pay a second one: 独特 means one object in the world, and the
// settlement writes into the same pack the acquisition path refuses to duplicate
// in. Paying under the definition id while the character already holds the same
// definition under the narration's name would put two 顶品独特 blades in one
// pack, which is exactly the pair the acquisition rules exist to prevent.
func TestATaskRewardCannotPayASecondUniqueCopy(t *testing.T) {
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Cultivation{Realm: cultivation.RealmFoundation, Stage: cultivation.StageEarly}), nil)
	player := state[actorStateRoot].(map[string]any)["protagonist"].(map[string]any)["state"].(map[string]any)
	player[xiuxianFieldImportantItems] = map[string]any{
		"断水刀": map[string]any{"定义": "implement.top.unique", "数量": float64(1)},
	}
	projection := &storyBranchProjection{State: state}
	definition := quest.Definition{ID: "quest.test_unique_reward", Title: "断道客的托付"}

	ops, err := questSettlementOps(projection, StoryDirectorActorStateSystem{}, definition, []quest.Reward{
		{Kind: quest.RewardItem, RefID: "implement.top.unique", Quantity: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	after := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(after, DefaultActorID, xiuxianFieldImportantItems)
	copies := 0
	for _, value := range pack {
		entry, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if xiuxianEntryText(entry, "定义") == "implement.top.unique" {
			copies++
		}
	}
	if copies != 1 {
		t.Fatalf("the pack holds %d copies of a unique blade, want the one already there", copies)
	}
}
