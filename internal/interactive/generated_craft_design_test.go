package interactive

import (
	"math"
	"reflect"
	"testing"

	"ta/internal/cultivation"
)

func generatedBonus(value float64) *float64 { return &value }

func TestGeneratedCultivationCraftProducesPricedStrongerEquipment(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianSkillTestState(t, "qi_7", nil)
	request := TurnSkillLearning{TemplateID: "craft.smithing.iron_sword", Name: "雨纹炼剑法", Source: "研读完整炼器注本", Description: "剑身细纹凝住雨光。", CraftDesign: &GeneratedCraftDesign{OutputTemplateID: "implement.middle.rare", ProductName: "雨纹剑", Item: GeneratedItemDesign{MainBonus: generatedBonus(.12), QualityGrade: cultivation.QualityPurple}}}
	ops, result, _ := xiuxianSkillLearningOps(state, system, DefaultActorID, TurnSkillLearningInput{Skills: []TurnSkillLearning{request}}, "design")
	if len(result.Refused) > 0 || len(result.CraftMethods) != 1 {
		t.Fatalf("registration failed: %+v", result)
	}
	registered := appliedXiuxianState(t, state, ops)
	method := result.CraftMethods[0]
	if method.MinimumStage != "qi_4" || method.MinutesPerBatch != 2160 || len(method.Inputs) != 1 || method.Inputs[0].Quantity != 9 {
		t.Fatalf("strong product not priced/qualified: %+v", method)
	}
	productID := method.Outputs[0].DefID
	product, ok := storyItemCatalog(cultivation.ReferenceRules(),
		registered).Definition(productID)
	if !ok || product.Implement.AttackBonus != .12 || product.BasePrice != 60 || product.QualityGrade != cultivation.QualityPurple || product.ApplicableStage != "qi_4" {
		t.Fatalf("invalid generated item: %+v", product)
	}
	if _, ok := storyItemCatalog(cultivation.ReferenceRules(),
		state).Definition(productID); ok {
		t.Fatal("product leaked before registration")
	}
	// Registration is not production. Only the actual inventory consumer settles a batch.
	items, acquired, supported := xiuxianItemOps(registered, system, DefaultActorID, TurnItemsInput{Acquire: []TurnItemAcquire{{DefID: "material.ore.iron", Name: "铁矿", Quantity: 9, Source: "实际矿料"}}}, "materials")
	if !supported || len(acquired.Refused) > 0 || len(acquired.Acquired) != 1 {
		t.Fatalf("material grant: %+v supported=%v", acquired, supported)
	}
	ready := appliedXiuxianState(t, registered, items)
	batch, err := cultivation.PlanCraftBatch(storyCraftRegistryFromState(system, ready), cultivation.CraftRequest{MethodID: method.ID, Minutes: method.MinutesPerBatch, Stage: cultivation.Qi(7), Held: craftHeldFromState(cultivation.ReferenceRules(),
		ready, DefaultActorID), Draw: .9})
	if err != nil {
		t.Fatal(err)
	}
	craftOps, ok := craftActorOps(cultivation.ReferenceRules(),
		ready, DefaultActorID, batch, method, "make")
	if !ok {
		t.Fatal("generated output has no inventory consumer")
	}
	made := appliedXiuxianState(t, ready, craftOps)
	held, _ := xiuxianEquipmentEntry(made, DefaultActorID, xiuxianFieldImportantItems)
	if xiuxianCarriedQuantity(held["铁矿"].(map[string]any)) != 0 || xiuxianCarriedQuantity(held[productID].(map[string]any)) != 1 {
		t.Fatalf("craft did not debit/create real quantities: %+v", held)
	}
	equipOps, equipped, _ := xiuxianItemOps(made, system, DefaultActorID, TurnItemsInput{Equip: []TurnItemEquip{{InstanceID: productID}}}, "equip")
	if len(equipped.Refused) > 0 || len(equipped.Equipped) != 1 {
		t.Fatalf("new item cannot be equipped: %+v", equipped)
	}
	after := appliedXiuxianState(t, made, equipOps)
	if xiuxianImplementOperatingCost(cultivation.ReferenceRules(),
		after, DefaultActorID) != product.OperatingManaCost(cultivation.Qi(7)) {
		t.Fatal("generated operating cost not used")
	}
	if len(GeneratedDefinitionArtJobs(registered, "ink", system)) != 2 {
		t.Fatal("method and product should each retain optional artwork")
	}
}

func TestGeneratedCultivationCraftRejectsFreeOrUnsupportedEffects(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, "qi_7", nil)
	good := TurnSkillLearning{TemplateID: "craft.smithing.iron_sword", Name: "新剑法", Source: "完整炼器法", CraftDesign: &GeneratedCraftDesign{OutputTemplateID: "implement.middle.rare", ProductName: "新剑"}}
	cases := []struct {
		name   string
		change func(*GeneratedCraftDesign)
	}{
		{"over budget", func(d *GeneratedCraftDesign) { d.Item.MainBonus = generatedBonus(.99) }},
		{"not finite", func(d *GeneratedCraftDesign) { d.Item.MainBonus = generatedBonus(math.NaN()) }},
		{"free operation", func(d *GeneratedCraftDesign) {
			d.OutputTemplateID = "implement.upper.rare.effect"
			d.Item.ManaFraction = generatedBonus(0)
		}},
		{"missing materials", func(d *GeneratedCraftDesign) {
			d.Inputs = []cultivation.CraftInput{{DefID: "material.ore.iron", Quantity: 1}}
		}},
		{"wrong profession", func(d *GeneratedCraftDesign) { d.OutputTemplateID = "pill.restore.health.lower" }},
		{"unique output", func(d *GeneratedCraftDesign) { d.OutputTemplateID = "implement.top.unique" }},
		{"unconsumed effect", func(d *GeneratedCraftDesign) { d.Item.PillPurpose = "restore_mana" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := *good.CraftDesign
			tc.change(&d)
			request := good
			request.CraftDesign = &d
			ops, result, _ := xiuxianSkillLearningOps(state, system, DefaultActorID, TurnSkillLearningInput{Skills: []TurnSkillLearning{request}}, "reject")
			if len(result.Refused) != 1 || len(ops) != 0 {
				t.Fatalf("invalid design mutated state: %+v %+v", result, ops)
			}
		})
	}
	low := xiuxianSkillTestState(t, "qi_1", nil)
	ops, result, _ := xiuxianSkillLearningOps(low, system, DefaultActorID, TurnSkillLearningInput{Skills: []TurnSkillLearning{good}}, "low")
	if len(result.Refused) != 1 || len(ops) != 0 {
		t.Fatal("advanced crafting bypassed stage gate")
	}
}

func TestGeneratedCultivationFormationChangesRealAttribute(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, "qi_7", nil)
	request := TurnSkillLearning{TemplateID: "craft.formation.gathered_edge", Name: "收锋护阵法", Source: "研习完整聚锋阵图后改推守势", CraftDesign: &GeneratedCraftDesign{OutputTemplateID: "formation.attack.middle", ProductName: "收锋护阵", Item: GeneratedItemDesign{Attribute: "defense", MainBonus: generatedBonus(.09)}}}
	ops, result, _ := xiuxianSkillLearningOps(state, system, DefaultActorID, TurnSkillLearningInput{Skills: []TurnSkillLearning{request}}, "formation")
	if len(result.Refused) > 0 {
		t.Fatalf("formation: %+v", result)
	}
	after := appliedXiuxianState(t, state, ops)
	def, ok := storyItemCatalog(cultivation.ReferenceRules(),
		after).Definition(result.CraftMethods[0].Outputs[0].DefID)
	if !ok || def.Formation.AttackBonus != 0 || def.Formation.DefenseBonus != .09 {
		t.Fatalf("formation remained cosmetic: %+v", def)
	}
	// Cold projection uses the same checked proposal rather than a cached object.
	replay := appliedXiuxianState(t, state, ops)
	if !reflect.DeepEqual(storyItemCatalog(cultivation.ReferenceRules(),
		after).Definitions(), storyItemCatalog(cultivation.ReferenceRules(),
		replay).Definitions()) {
		t.Fatal("proposal replay differs")
	}
	request.CraftDesign.Item.MainBonus = generatedBonus(.11)
	if _, result, _ := xiuxianSkillLearningOps(state, system, DefaultActorID, TurnSkillLearningInput{Skills: []TurnSkillLearning{request}}, "over"); len(result.Refused) != 1 {
		t.Fatal("formation exceeded native defense budget")
	}
}

// Synthetic integration: real trade/craft commits and canonical Story replay,
// with no live model request or player data.
func TestGeneratedCraftDesignStoryTradeClockRetryForkAndReplay(t *testing.T) {
	method, _ := cultivation.NewCraftRegistryWithBuiltins().Method("craft.smithing.iron_sword")
	method.ID = "world.fixture_forge"
	method.Source = "完整炼器传授"
	method.FailureCost = "consume_inputs"
	method.LossChance = 0
	store, storyID, root := talismanStory(t, method)
	initial, _ := store.Snapshot(storyID, "main")
	old, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: initial.CurrentTurn.ID, Title: "尚未研究"})
	if err != nil {
		t.Fatal(err)
	}
	request := TurnSkillLearning{TemplateID: method.ID, Name: "重叠剑纹法", Source: "在完整基础法上研究剑纹", Description: "雨光似的纹路附在刃上。", CraftDesign: &GeneratedCraftDesign{OutputTemplateID: "implement.middle.rare", ProductName: "叠纹剑", Item: GeneratedItemDesign{MainBonus: generatedBonus(.12), QualityGrade: cultivation.QualityPurple}}}
	talismanTurn(t, store, storyID, "main", "登记新的剑纹方法", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{request}}})
	registered, _ := store.Snapshot(storyID, "main")
	methodID, _ := generatedContentID("craft", method.ID, request.Name)
	derived, ok := storyCraftRegistryFromState(registered.ActorStateSchema.System, registered.State).Method(methodID)
	if !ok {
		t.Fatal("new method did not survive formal registration")
	}
	outputID := derived.Outputs[0].DefID
	if held, _ := xiuxianEquipmentEntry(registered.State, DefaultActorID, xiuxianFieldImportantItems); xiuxianCarriedQuantityMap(held, outputID) != 0 {
		t.Fatal("registration manufactured product")
	}
	result := &TurnResult{Trade: &TurnTradeInput{Merchant: "铁匠铺", Buys: []TurnTradeBuy{{DefID: "material.ore.iron", Name: "铁矿", Quantity: 9}}}, Craft: &TurnCraftInput{MethodID: methodID, DurationSeconds: derived.MinutesPerBatch * 60}}
	madeTurn := talismanTurn(t, store, storyID, "main", "买九份铁矿并制作一把叠纹剑", result)
	made, _ := store.Snapshot(storyID, "main")
	if SnapshotClockSeconds(made)-SnapshotClockSeconds(registered) != derived.MinutesPerBatch*60 {
		t.Fatal("craft time absent or doubled")
	}
	held, _ := xiuxianEquipmentEntry(made.State, DefaultActorID, xiuxianFieldImportantItems)
	if xiuxianCarriedQuantityMap(held, "铁矿") != 0 || xiuxianCarriedQuantityMap(held, outputID) != 1 {
		t.Fatalf("formal inventory mismatch: %+v", held)
	}
	details, err := store.CultivationEntryDetails(storyID, "main", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(details.Items) != 1 || details.Items[0].Name != "叠纹剑" || details.Items[0].QualityGrade != cultivation.QualityPurple {
		t.Fatalf("generated product lost display identity: %+v", details.Items)
	}
	talismanTurn(t, store, storyID, "main", "买九份铁矿并制作一把叠纹剑", result)
	retried, _ := store.Snapshot(storyID, "main")
	if !reflect.DeepEqual(made.State, retried.State) || made.CurrentTurn.ID != retried.CurrentTurn.ID {
		t.Fatal("same committed command paid or produced twice")
	}
	retained, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: madeTurn.ID, Title: "保留制成品"})
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	reopened := NewStore(root)
	defer reopened.Close()
	replay, err := reopened.Snapshot(storyID, "main")
	if err != nil || !reflect.DeepEqual(made.State, replay.State) {
		t.Fatalf("cold craft replay: %v", err)
	}
	earlier, _ := reopened.Snapshot(storyID, old.ID)
	if _, ok := storyItemCatalog(cultivation.ReferenceRules(),
		earlier.State).Definition(outputID); ok {
		t.Fatal("new definition leaked into earlier branch")
	}
	later, _ := reopened.Snapshot(storyID, retained.ID)
	if held, _ := xiuxianEquipmentEntry(later.State, DefaultActorID, xiuxianFieldImportantItems); xiuxianCarriedQuantityMap(held, outputID) != 1 {
		t.Fatal("later branch lost actual crafted item")
	}
}

func xiuxianCarriedQuantityMap(record map[string]any, key string) int {
	entry, _ := record[key].(map[string]any)
	if entry == nil {
		return 0
	}
	return xiuxianCarriedQuantity(entry)
}
