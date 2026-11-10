package interactive

import (
	"testing"

	"ta/internal/cultivation"
)

// This file is the minimal reproduction of the second half of the B2 repair:
// an auxiliary cultivation resource must really exist, must really be deducted,
// and the rate must drop back when the stock runs out.
//
// 规则 §6.2 line 232: "辅助资源必须实际存在并按内容合同扣除，耗尽后从那个时间点换
// 速率，不能一颗丹药永久获得辅助倍数。此表确定辅助强度，具体丹药数量与有效持续时间
// 在物品数据中定义。"
//
// R2 already proved that a *claimed* support with no stock behind it earns the
// ordinary rate. These tests prove the other direction: real stock earns the
// supported rate for exactly as long as it pays for, is deducted from 重要物品,
// and the unsupported remainder is ordinary training.

// b2TestSystem is the fixture every test here shares.
//
// It cannot be combatTestSystem: that helper builds the protagonist from the
// panel and state fields alone, which leaves 重要物品 outside the schema, so a
// seeded stock would never reach the state the advance reads. It cannot be
// xiuxianTestSystem either: the preset's default actors carry no realm, and a
// character with no realm has nothing to train. The protagonist template is the
// full preset one, which declares the carried record, and the realm is seeded
// the way combatTestSystem seeds it.
func b2TestSystem() StoryDirectorActorStateSystem {
	spec := xiuxianActorStatePresetSpec()
	return StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			protagonistStateTemplate(cultivation.ReferenceRules(),
				spec),
			storyContextStateTemplate(spec),
		},
		InitialActors: []ActorStateInitialActor{
			{
				ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID,
				State: map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(6))},
			},
			{ID: DefaultStoryContextActorID, Name: "故事状态", TemplateID: ActorStateStoryContextTemplateID},
		},
	}
}

// b2SupportStory builds a story whose protagonist holds the given count of the
// first-version cultivation-support dose.
//
// The stock is seeded through the initial actor, the one place a story fact
// arrives without a settlement having produced it, and the record uses the same
// shape items.acquire writes: the stable definition id plus a count.
func b2SupportStory(t *testing.T, quantity int) (*Store, string) {
	t.Helper()
	return b2SupportStoryAt(t, map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"聚灵散": map[string]any{
				"定义": "pill.support.cultivation.lower",
				"数量": float64(quantity),
			},
		},
	}, "")
}

// b2SupportStoryAt is b2SupportStory with an optional scene location, which is
// where the environment reader looks.
func b2SupportStoryAt(t *testing.T, initial map[string]any, location string) (*Store, string) {
	t.Helper()
	system := b2TestSystem()
	for index := range system.InitialActors {
		switch system.InitialActors[index].ID {
		case DefaultActorID:
			if system.InitialActors[index].State == nil {
				system.InitialActors[index].State = map[string]any{}
			}
			for field, value := range initial {
				system.InitialActors[index].State[field] = value
			}
		case DefaultStoryContextActorID:
			if location == "" {
				continue
			}
			if system.InitialActors[index].State == nil {
				system.InitialActors[index].State = map[string]any{}
			}
			system.InitialActors[index].State[storySceneLocationField] = location
		}
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "修炼时长", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角在洞府中盘膝坐下，准备长修。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// b2SupportStock reads the protagonist's committed count of the support dose.
func b2SupportStock(t *testing.T, store *Store, storyID string) int {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	record, ok := xiuxianEquipmentEntry(snapshot.State, DefaultActorID, xiuxianFieldImportantItems)
	if !ok {
		return 0
	}
	entry, ok := record["聚灵散"].(map[string]any)
	if !ok {
		return 0
	}
	return xiuxianCarriedQuantity(entry)
}

// TestB2SupportIsCreditedOnlyWhileTheStockPaysForIt is §6.2 line 232's first
// half: the resource exists, so the rate really rises.
//
// One dose supports one rule month, so a thirty-day interval is covered in full
// and the gain must be the ordinary 120 × 1.20 = 144.
func TestB2SupportIsCreditedOnlyWhileTheStockPaysForIt(t *testing.T) {
	store, storyID := b2SupportStory(t, 1)

	b1TurnTrain(t, store, storyID, "cmd-b2-supported", TurnCultivationInput{Days: 30})
	gained := cultivationTestStoredFloat(t, store, storyID, "main", xiuxianFieldRealmProgress)

	const supportedMonth = 144.0
	if diff := gained - supportedMonth; diff > 0.01 || diff < -0.01 {
		t.Fatalf("one real 普通辅助 dose must scale one rule month's 120 by 1.20: gained=%v want=%v", gained, supportedMonth)
	}
	if stock := b2SupportStock(t, store, storyID); stock != 0 {
		t.Fatalf("the dose that paid for the month must be deducted: stock=%d", stock)
	}
}

// TestB2TheRateChangesWhenTheStockRunsOut is the rule's "耗尽后从那个时间点换速率".
//
// Two doses cover sixty days, so a ninety-day interval is sixty days supported
// and thirty days ordinary: 2 × 144 + 120 = 408. A single pill granting a
// permanent multiplier would instead give 3 × 144 = 432, and no deduction at all.
func TestB2TheRateChangesWhenTheStockRunsOut(t *testing.T) {
	store, storyID := b2SupportStory(t, 2)

	b1TurnTrain(t, store, storyID, "cmd-b2-exhaust", TurnCultivationInput{Days: 90})
	gained := cultivationTestStoredFloat(t, store, storyID, "main", xiuxianFieldRealmProgress)

	const want = 2*144.0 + 120.0
	if diff := gained - want; diff > 0.01 || diff < -0.01 {
		t.Fatalf("the rate must switch at the instant the stock runs out: gained=%v want=%v", gained, want)
	}
	if stock := b2SupportStock(t, store, storyID); stock != 0 {
		t.Fatalf("both doses must be deducted: stock=%d", stock)
	}
}

// TestB2ASeclusionLongerThanTheStockIsNotStopped is the other side of the same
// rule: running out of a pill continues the seclusion at the ordinary rate
// rather than refusing the interval.
//
// It is also the control for the deduction: a second interval finds no stock and
// is trained at the ordinary rate.
func TestB2ASeclusionLongerThanTheStockIsNotStopped(t *testing.T) {
	store, storyID := b2SupportStory(t, 1)

	b1TurnTrain(t, store, storyID, "cmd-b2-first", TurnCultivationInput{Days: 30})
	first := cultivationTestStoredFloat(t, store, storyID, "main", xiuxianFieldRealmProgress)

	b1TurnTrain(t, store, storyID, "cmd-b2-second", TurnCultivationInput{Days: 30})
	second := cultivationTestStoredFloat(t, store, storyID, "main", xiuxianFieldRealmProgress)

	if diff := first - 144.0; diff > 0.01 || diff < -0.01 {
		t.Fatalf("the first month must be supported: %v", first)
	}
	if diff := (second - first) - 120.0; diff > 0.01 || diff < -0.01 {
		t.Fatalf("with the stock gone the next month must be ordinary: %v -> %v", first, second)
	}
}

// TestB2AStrengthenedDoseIsNotSpentByAWeakerClaim is the boundary between the
// claim and the stock.
//
// The model may say strong_support, but the only registered dose is 普通辅助, so
// the strength comes from the definition: 144, not 168. The dose is still
// deducted, because it really was consumed.
func TestB2AStrengthenedDoseIsNotSpentByAWeakerClaim(t *testing.T) {
	store, storyID := b2SupportStory(t, 1)

	b1TurnTrain(t, store, storyID, "cmd-b2-claim", TurnCultivationInput{
		Days:     30,
		Resource: string(cultivation.SupportStrong),
	})
	gained := cultivationTestStoredFloat(t, store, storyID, "main", xiuxianFieldRealmProgress)

	if diff := gained - 144.0; diff > 0.01 || diff < -0.01 {
		t.Fatalf("the registered dose's own strength decides the rate, not the claim: gained=%v", gained)
	}
}

// TestB2AStableSiteStillNeedsNoStock keeps the two modifiers apart: the place is
// read from the committed scene, so a qualified site really does scale the rate
// with no auxiliary stock at all.
//
// It is here because the support reader replaced the one zero-argument bonus
// function, and the environment must not have been folded into it.
func TestB2AStableSiteStillNeedsNoStock(t *testing.T) {
	store, storyID := b2SupportStoryAt(t, map[string]any{
		xiuxianFieldImportantItems: map[string]any{},
	}, "青崖洞府灵眼")

	b1TurnTrain(t, store, storyID, "cmd-b2-site", TurnCultivationInput{Days: 30})
	gained := cultivationTestStoredFloat(t, store, storyID, "main", xiuxianFieldRealmProgress)

	const stableMonth = 150.0
	if diff := gained - stableMonth; diff > 0.01 || diff < -0.01 {
		t.Fatalf("a committed spiritual site must scale the ordinary month by 1.25: gained=%v want=%v", gained, stableMonth)
	}
}

// TestB2SupportPillIsRegisteredAndValidated proves the catalogue really holds
// the dose the reader prices, and that an incomplete support definition is
// refused rather than becoming a permanent multiplier.
func TestB2SupportPillIsRegisteredAndValidated(t *testing.T) {
	definition, ok := storyItemCatalog(cultivation.ReferenceRules()).Definition("pill.support.cultivation.lower")
	if !ok {
		t.Fatal("the first-version catalogue must register the cultivation-support dose")
	}
	if definition.Pill == nil {
		t.Fatal("the support dose must carry a pill payload")
	}
	strength, ok := definition.Pill.SupportStrengthOf()
	if !ok || strength != cultivation.SupportNormal {
		t.Fatalf("the support dose must publish its §6.2 strength: %v %v", strength, ok)
	}
	if days := definition.Pill.SupportDuration(); days != float64(cultivation.DaysPerRuleMonth) {
		t.Fatalf("the support dose must publish its own duration: %v", days)
	}

	incomplete := cultivation.ItemDef{
		ID: "pill.support.incomplete", Kind: cultivation.KindPill,
		Grade: cultivation.GradeLower, Rarity: cultivation.RarityCommon,
		Pill: &cultivation.PillDef{Purpose: cultivation.PillSupportCultivation},
	}
	if err := incomplete.Validate(); err == nil {
		t.Fatal("a support dose with no strength or duration is the permanent multiplier the rule forbids")
	}
}
