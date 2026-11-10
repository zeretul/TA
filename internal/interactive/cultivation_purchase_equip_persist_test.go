package interactive

import (
	"testing"

	"ta/internal/cultivation"
)

// These two regressions are the inventory-conservation defects found in the
// 0.5.6 package, run through the story's own turn transaction rather than the
// rules helpers alone. What the player paid for has to still be there after the
// turn is saved and the game is reopened.
//
// 《自制-装备栏与法宝御使》§2 fixes the slot count as "how many may be worn at
// once", not "how many may be kept", and 《自制-灵石经济与简单交易》§10 requires an
// accepted payment to hand over the goods it charged for. Neither rule is
// satisfied by a number that only looks right before the save.

// paidCopyStory opens a story whose protagonist already holds the given purse.
//
// The realm and the purse arrive through the initial actor, which is the one
// place a starting fact is written without a settlement having produced it.
// Extra initial actors are added to the same story so a test can start with a
// second tracked character already holding something.
func paidCopyStory(t *testing.T, purse float64, extra ...ActorStateInitialActor) (store *Store, storyID, root string) {
	t.Helper()
	system := xiuxianTestSystem()
	for i := range system.InitialActors {
		if system.InitialActors[i].ID == DefaultActorID {
			system.InitialActors[i].State = map[string]any{
				xiuxianFieldRealm:    cultivation.StageKey(cultivation.Qi(6)),
				xiuxianFieldCurrency: purse,
			}
		}
	}
	// Extra characters are how a story starts with someone else already holding
	// something. A transfer needs a real giver, and inventing one after the save
	// would be writing the state the test is supposed to observe.
	system.InitialActors = append(system.InitialActors, extra...)
	root = t.TempDir()
	store = NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "行囊与钱袋", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "进了坊市", Narrative: "坊市里人声嘈杂，柜台后摆着一排阵盘。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID, root
}

// commitPaidTurn appends one turn whose modules the rules settle.
func commitPaidTurn(t *testing.T, store *Store, storyID, user, narrative string, result *TurnResult) {
	t.Helper()
	result.Choices = testTurnChoices()
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: user, Narrative: narrative, TurnResult: result,
	}); err != nil {
		t.Fatalf("%s: %v", user, err)
	}
}

// carriedInSnapshot reads one copy count out of a saved branch.
func carriedInSnapshot(t *testing.T, store *Store, storyID, name string) int {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	pack, _ := xiuxianEquipmentEntry(snapshot.State, DefaultActorID, xiuxianFieldImportantItems)
	entry, ok := pack[name].(map[string]any)
	if !ok {
		return 0
	}
	return xiuxianCarriedQuantity(entry)
}

// purseInSnapshot reads the saved purse of the protagonist.
func purseInSnapshot(t *testing.T, store *Store, storyID string) int {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return xiuxianStoredInt(snapshot.State, DefaultActorID, xiuxianFieldCurrency)
}

// TestThreePaidFormationsSurviveEquipUnequipAndReload is the 0.5.6 defect: three
// 聚灵阵盘 were bought, one was worn, and putting it back left a single copy in
// the pack. The player paid for three; the pack owes them three, before and
// after the save.
func TestThreePaidFormationsSurviveEquipUnequipAndReload(t *testing.T) {
	store, storyID, root := paidCopyStory(t, 1000)

	commitPaidTurn(t, store, storyID, "买下三个阵盘", "你数出三份灵石，柜台后推来三只阵盘。", &TurnResult{
		Trade: &TurnTradeInput{
			Merchant: "坊市阵法铺",
			Buys:     []TurnTradeBuy{{DefID: "formation.attack.middle", Name: "聚灵阵盘", Quantity: 3}},
		},
	})
	if got := carriedInSnapshot(t, store, storyID, "聚灵阵盘"); got != 3 {
		t.Fatalf("after paying for three: pack holds %d, want 3", got)
	}
	if got, want := purseInSnapshot(t, store, storyID), 1000-3*50; got != want {
		t.Fatalf("after paying for three: purse = %d, want %d", got, want)
	}

	commitPaidTurn(t, store, storyID, "布下一只阵盘", "你把其中一只阵盘摆在脚边，余下两只收回行囊。", &TurnResult{
		Items: &TurnItemsInput{Equip: []TurnItemEquip{{InstanceID: "聚灵阵盘"}}},
	})
	if got := carriedInSnapshot(t, store, storyID, "聚灵阵盘"); got != 2 {
		t.Fatalf("one worn, %d left in the pack; want 2", got)
	}

	commitPaidTurn(t, store, storyID, "收起阵盘", "你撤去阵势，把那只阵盘也收回行囊。", &TurnResult{
		Items: &TurnItemsInput{Unequip: []TurnItemUnequip{{Slot: xiuxianFieldFormation}}},
	})
	if got := carriedInSnapshot(t, store, storyID, "聚灵阵盘"); got != 3 {
		t.Fatalf("after putting it back: pack holds %d, want the three that were paid for", got)
	}
	if got, want := purseInSnapshot(t, store, storyID), 1000-3*50; got != want {
		t.Fatalf("wearing and putting back a copy changed the purse: %d, want %d", got, want)
	}

	// The save is what the player comes back to. Reading the same branch through a
	// freshly opened store is the only check that the counts above are on disk
	// rather than in the process that wrote them.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	t.Cleanup(func() { _ = reopened.Close() })
	if got := carriedInSnapshot(t, reopened, storyID, "聚灵阵盘"); got != 3 {
		t.Fatalf("after reopening: pack holds %d, want 3", got)
	}
	if got, want := purseInSnapshot(t, reopened, storyID), 1000-3*50; got != want {
		t.Fatalf("after reopening: purse = %d, want %d", got, want)
	}
}

// TestTwoPaidSwordsBuyTwoSwords is the other 0.5.6 defect: two purchases of the
// same 青锋剑 charged 40 灵石 and left one sword. An accepted payment either
// delivers the copy or is refused before any coin moves.
func TestTwoPaidSwordsBuyTwoSwords(t *testing.T) {
	store, storyID, root := paidCopyStory(t, 1000)

	for turn, narrative := range []string{
		"你挑了一柄青锋剑，数出二十枚灵石。",
		"你觉得一柄不够用，又挑了同样的一柄。",
	} {
		commitPaidTurn(t, store, storyID, "买一柄青锋剑", narrative, &TurnResult{
			Trade: &TurnTradeInput{
				Merchant: "坊市兵器铺",
				Buys:     []TurnTradeBuy{{DefID: "implement.lower.common", Name: "青锋剑"}},
			},
		})
		if got := carriedInSnapshot(t, store, storyID, "青锋剑"); got != turn+1 {
			t.Fatalf("after purchase %d: pack holds %d swords, want %d", turn+1, got, turn+1)
		}
	}

	paid := 1000 - purseInSnapshot(t, store, storyID)
	held := carriedInSnapshot(t, store, storyID, "青锋剑")
	if paid != held*20 {
		t.Fatalf("paid %d 灵石 but holds %d swords at 20 each", paid, held)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	t.Cleanup(func() { _ = reopened.Close() })
	if got := carriedInSnapshot(t, reopened, storyID, "青锋剑"); got != 2 {
		t.Fatalf("after reopening: pack holds %d swords, want the two that were paid for", got)
	}
	if got, want := purseInSnapshot(t, reopened, storyID), 1000-2*20; got != want {
		t.Fatalf("after reopening: purse = %d, want %d", got, want)
	}
}

// TestTheSecondUniqueCopyIsRefusedBeforeAnyCoinMoves is the guard beside the
// stacking rule: a 独特之物 is one object in the world, so a second purchase is
// refused while the purse is still whole rather than charged for a copy the
// pack cannot hold.
func TestTheSecondUniqueCopyIsRefusedBeforeAnyCoinMoves(t *testing.T) {
	store, storyID, _ := paidCopyStory(t, 1000)

	commitPaidTurn(t, store, storyID, "买下那柄孤剑", "铺主说这剑世上只此一柄，你付了五百灵石。", &TurnResult{
		Trade: &TurnTradeInput{
			Merchant: "坊市兵器铺",
			Buys:     []TurnTradeBuy{{DefID: "implement.top.unique", Name: "孤锋"}},
		},
	})
	if got, want := purseInSnapshot(t, store, storyID), 1000-500; got != want {
		t.Fatalf("first purchase: purse = %d, want %d", got, want)
	}

	commitPaidTurn(t, store, storyID, "再问一柄", "你又问铺主还有没有第二柄，他说没有。", &TurnResult{
		Trade: &TurnTradeInput{
			Merchant: "坊市兵器铺",
			Buys:     []TurnTradeBuy{{DefID: "implement.top.unique", Name: "孤锋"}},
		},
	})
	if got := carriedInSnapshot(t, store, storyID, "孤锋"); got != 1 {
		t.Fatalf("the unique copy is held %d times, want 1", got)
	}
	if got, want := purseInSnapshot(t, store, storyID), 1000-500; got != want {
		t.Fatalf("a refused second unique purchase still charged: purse = %d, want %d", got, want)
	}
}

// actorFieldInSnapshot reads one numeric field of one saved actor.
func actorFieldInSnapshot(t *testing.T, store *Store, storyID, actorID, field string) int {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return xiuxianStoredInt(snapshot.State, actorID, field)
}

// TestTheSameFormationNamedTwiceChargesOneDeployment is the duplicate-request
// half of §10's "one copy, one effect". Naming one 阵盘 twice in a submission
// describes one placement, so it must be paid for once: raising it twice used to
// charge the activation cost twice for the single formation that actually stood.
func TestTheSameFormationNamedTwiceChargesOneDeployment(t *testing.T) {
	store, storyID, _ := paidCopyStory(t, 1000)

	commitPaidTurn(t, store, storyID, "买下三个阵盘", "你数出三份灵石，柜台后推来三只阵盘。", &TurnResult{
		Trade: &TurnTradeInput{
			Merchant: "坊市阵法铺",
			Buys:     []TurnTradeBuy{{DefID: "formation.attack.middle", Name: "聚灵阵盘", Quantity: 3}},
		},
	})
	manaBefore := actorFieldInSnapshot(t, store, storyID, DefaultActorID, xiuxianFieldMana)

	commitPaidTurn(t, store, storyID, "布下阵盘", "你把一只阵盘摆在脚边，另两只收回行囊。", &TurnResult{
		Items: &TurnItemsInput{Equip: []TurnItemEquip{
			{InstanceID: "聚灵阵盘"},
			{InstanceID: "聚灵阵盘"},
		}},
	})

	if got := carriedInSnapshot(t, store, storyID, "聚灵阵盘"); got != 2 {
		t.Fatalf("one worn, %d left in the pack; want 2", got)
	}
	// 炼气六层的灵力上限是 160，启动一只阵盘付 ceil(0.10 × 160) = 16。
	if got, want := actorFieldInSnapshot(t, store, storyID, DefaultActorID, xiuxianFieldMana), manaBefore-16; got != want {
		t.Fatalf("mana after naming one formation twice = %d, want %d (one activation, not two)", got, want)
	}
}

// TestAFreeAcquisitionKeepsTheCopiesAlreadyInThePack is the other conservation
// hole: an acquisition that names a copy already held must not rewrite the stack
// with its own count. A pack holding three 阵盘 came out of one free acquisition
// holding a single copy, so two paid-for objects vanished.
func TestAFreeAcquisitionKeepsTheCopiesAlreadyInThePack(t *testing.T) {
	store, storyID, root := paidCopyStory(t, 1000)

	commitPaidTurn(t, store, storyID, "买下三个阵盘", "你数出三份灵石，柜台后推来三只阵盘。", &TurnResult{
		Trade: &TurnTradeInput{
			Merchant: "坊市阵法铺",
			Buys:     []TurnTradeBuy{{DefID: "formation.attack.middle", Name: "聚灵阵盘", Quantity: 3}},
		},
	})
	if got := carriedInSnapshot(t, store, storyID, "聚灵阵盘"); got != 3 {
		t.Fatalf("after paying for three: pack holds %d, want 3", got)
	}

	commitPaidTurn(t, store, storyID, "收起同款阵盘", "你把散落在蒲团边的那只同款阵盘也捡了起来。", &TurnResult{
		Items: &TurnItemsInput{Acquire: []TurnItemAcquire{
			{DefID: "formation.attack.middle", Name: "聚灵阵盘", Source: "蒲团边捡到"},
		}},
	})
	if got := carriedInSnapshot(t, store, storyID, "聚灵阵盘"); got != 3 {
		t.Fatalf("a free acquisition changed the stack: pack holds %d, want the three that were paid for", got)
	}
	if got, want := purseInSnapshot(t, store, storyID), 1000-3*50; got != want {
		t.Fatalf("a free acquisition changed the purse: %d, want %d", got, want)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	t.Cleanup(func() { _ = reopened.Close() })
	if got := carriedInSnapshot(t, reopened, storyID, "聚灵阵盘"); got != 3 {
		t.Fatalf("after reopening: pack holds %d, want 3", got)
	}
}

// TestATransferJoinsTheCopiesTheRecipientAlreadyHolds is §7's "one real object,
// one holder" seen from the receiving side. The player already has two of the
// same sword; a third handed over by a tracked character joins them. Refusing
// used to force the player to invent a second name for an identical blade just
// to accept it.
func TestATransferJoinsTheCopiesTheRecipientAlreadyHolds(t *testing.T) {
	store, storyID, root := paidCopyStory(t, 1000, ActorStateInitialActor{
		ID: "断道客", Name: "断道客", TemplateID: ActorStateOpponentTemplateID, Role: "opponent",
		State: map[string]any{
			xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(6)),
			xiuxianFieldImportantItems: map[string]any{
				"青锋剑": map[string]any{"定义": "implement.middle.rare", "数量": float64(1)},
			},
		},
	})

	for turn, narrative := range []string{
		"你挑了一柄青锋剑，数出六十枚灵石。",
		"你又挑了同样的一柄，付了六十枚灵石。",
	} {
		commitPaidTurn(t, store, storyID, "买一柄青锋剑", narrative, &TurnResult{
			Trade: &TurnTradeInput{
				Merchant: "坊市兵器铺",
				Buys:     []TurnTradeBuy{{DefID: "implement.middle.rare", Name: "青锋剑"}},
			},
		})
		if got := carriedInSnapshot(t, store, storyID, "青锋剑"); got != turn+1 {
			t.Fatalf("after purchase %d: pack holds %d swords, want %d", turn+1, got, turn+1)
		}
	}

	commitPaidTurn(t, store, storyID, "接过断道客的剑", "断道客把手里那柄青锋剑递过来，你收进行囊。", &TurnResult{
		Items: &TurnItemsInput{Acquire: []TurnItemAcquire{
			{DefID: "implement.middle.rare", Name: "青锋剑", From: "断道客", Source: "断道客递来"},
		}},
	})
	if got := carriedInSnapshot(t, store, storyID, "青锋剑"); got != 3 {
		t.Fatalf("after the transfer: pack holds %d swords, want the two bought plus the one handed over", got)
	}
	if got, want := purseInSnapshot(t, store, storyID), 1000-2*60; got != want {
		t.Fatalf("a transfer moved coin: purse = %d, want %d", got, want)
	}
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if pack, held := xiuxianEquipmentEntry(snapshot.State, "断道客", xiuxianFieldImportantItems); held {
		if entry, still := pack["青锋剑"].(map[string]any); still && xiuxianCarriedQuantity(entry) > 0 {
			t.Fatalf("the donor still holds the sword it handed over: %+v", entry)
		}
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	t.Cleanup(func() { _ = reopened.Close() })
	if got := carriedInSnapshot(t, reopened, storyID, "青锋剑"); got != 3 {
		t.Fatalf("after reopening: pack holds %d swords, want 3", got)
	}
	if got, want := purseInSnapshot(t, reopened, storyID), 1000-2*60; got != want {
		t.Fatalf("after reopening: purse = %d, want %d", got, want)
	}
}

// TestTwoPaidArmoursBuyTwoArmours is the same rule as the swords, on the 护具
// slot: armour is one object per slot, not one object per pack key.
func TestTwoPaidArmoursBuyTwoArmours(t *testing.T) {
	store, storyID, _ := paidCopyStory(t, 1000)

	for turn, narrative := range []string{
		"你挑了一件玄铁甲，付了六十枚灵石。",
		"你想着替换着穿，又买了一件同样的玄铁甲。",
	} {
		commitPaidTurn(t, store, storyID, "买一件玄铁甲", narrative, &TurnResult{
			Trade: &TurnTradeInput{
				Merchant: "坊市法器铺",
				Buys:     []TurnTradeBuy{{DefID: "armor.middle.rare", Name: "玄铁甲"}},
			},
		})
		if got := carriedInSnapshot(t, store, storyID, "玄铁甲"); got != turn+1 {
			t.Fatalf("after purchase %d: pack holds %d, want %d", turn+1, got, turn+1)
		}
	}
	if got, want := purseInSnapshot(t, store, storyID), 1000-2*60; got != want {
		t.Fatalf("purse = %d, want %d", got, want)
	}
}

// TestSellingPartOfAStackKeepsTheRestAndPays is the sell side of conservation:
// one copy leaves the pack, the rest stay, and the merchant's own cash pays for
// exactly the one that moved.
func TestSellingPartOfAStackKeepsTheRestAndPays(t *testing.T) {
	store, storyID, root := paidCopyStory(t, 1000)

	commitPaidTurn(t, store, storyID, "买下三柄青锋剑", "你把柜台上的三柄青锋剑一并买下。", &TurnResult{
		Trade: &TurnTradeInput{
			Merchant: "坊市兵器铺",
			Buys:     []TurnTradeBuy{{DefID: "implement.lower.common", Name: "青锋剑", Quantity: 3}},
		},
	})
	if got := carriedInSnapshot(t, store, storyID, "青锋剑"); got != 3 {
		t.Fatalf("after paying for three: pack holds %d, want 3", got)
	}

	commitPaidTurn(t, store, storyID, "卖掉一柄", "你留下两柄，把多余的那柄卖回铺子。", &TurnResult{
		Trade: &TurnTradeInput{
			Merchant: "坊市兵器铺",
			Sells:    []TurnTradeSell{{InstanceID: "青锋剑", Quantity: 1}},
		},
	})
	if got := carriedInSnapshot(t, store, storyID, "青锋剑"); got != 2 {
		t.Fatalf("after selling one: pack holds %d, want 2", got)
	}
	// 回售价是售价的 60%：20 × 0.6 = 12。
	if got, want := purseInSnapshot(t, store, storyID), 1000-3*20+12; got != want {
		t.Fatalf("purse = %d, want %d", got, want)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	t.Cleanup(func() { _ = reopened.Close() })
	if got := carriedInSnapshot(t, reopened, storyID, "青锋剑"); got != 2 {
		t.Fatalf("after reopening: pack holds %d, want 2", got)
	}
	if got, want := purseInSnapshot(t, reopened, storyID), 1000-3*20+12; got != want {
		t.Fatalf("after reopening: purse = %d, want %d", got, want)
	}
}
