package cultivation

import (
	"errors"
	"strings"
	"testing"
)

// This file is the rules half of F16: what one batch costs, yields and refuses.
//
// §2 of 《自制-修仙技艺》V0.1 fixes the order these answers are read in —
// qualification, then time, then materials — and §5 says a batch that has not
// formally begun has not spent anything. The ordering is what the tests below
// observe, because it is what the player is told.

func craftTestHeld(defID string, quantity int) []CraftHeld {
	return []CraftHeld{{InstanceID: "青萝草", DefID: defID, Quantity: quantity}}
}

// TestPlanCraftBatchChargesOnlyWholeBatches is §2's 未正式开始不先扣成本: an
// interval short of one batch starts nothing, and a longer one spends only the
// batches it really paid for.
func TestPlanCraftBatchChargesOnlyWholeBatches(t *testing.T) {
	registry := NewCraftRegistryWithBuiltins()
	method, ok := registry.Method("craft.alchemy.spring_return")
	if !ok {
		t.Fatal("the shipped method must be registered")
	}
	request := CraftRequest{
		MethodID: method.ID,
		Stage:    Qi(6),
		Held:     craftTestHeld("material.herb.green_vine", 6),
		Draw:     0.5,
	}

	short := request
	short.Minutes = method.MinutesPerBatch - 1
	if _, err := PlanCraftBatch(registry, short); !errors.Is(err, ErrCraftDurationTooShort) {
		t.Fatalf("an interval short of a batch must start none, got %v", err)
	}

	exact := request
	exact.Minutes = method.MinutesPerBatch
	batch, err := PlanCraftBatch(registry, exact)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Batches != 1 || batch.ElapsedMinutes != method.MinutesPerBatch {
		t.Fatalf("one batch worth of time must settle one batch: %#v", batch)
	}

	// Two and a half batches' worth of time pays for two, and the half is not
	// charged.
	long := request
	long.Minutes = method.MinutesPerBatch*5/2 + 7
	batch, err = PlanCraftBatch(registry, long)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Batches != 2 || batch.ElapsedMinutes != method.MinutesPerBatch*2 {
		t.Fatalf("a partial batch must not be started or charged: %#v", batch)
	}
	for _, consumed := range batch.Consumed {
		if consumed.DefID != "material.herb.green_vine" || consumed.Quantity != 4 {
			t.Fatalf("two batches must spend two herbs each: %#v", batch.Consumed)
		}
	}
	for _, product := range batch.Products {
		if product.DefID != "pill.restore.health.lower" || product.Quantity != 2 {
			t.Fatalf("two batches must yield two pills: %#v", batch.Products)
		}
	}
}

// TestPlanCraftBatchStopsAtWhatTheMaterialsPayFor is §5's 同一批次只结算一次 read
// against real stock: time offered is not the same question as material held.
func TestPlanCraftBatchStopsAtWhatTheMaterialsPayFor(t *testing.T) {
	registry := NewCraftRegistryWithBuiltins()
	method, _ := registry.Method("craft.alchemy.spring_return")

	batch, err := PlanCraftBatch(registry, CraftRequest{
		MethodID: method.ID,
		Minutes:  method.MinutesPerBatch * 10,
		Stage:    Qi(6),
		Held:     craftTestHeld("material.herb.green_vine", 5),
		Draw:     0.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Five herbs at two per batch pays for two batches, not ten.
	if batch.Batches != 2 || batch.ElapsedMinutes != method.MinutesPerBatch*2 {
		t.Fatalf("the run must stop where the stock does: %#v", batch)
	}

	_, err = PlanCraftBatch(registry, CraftRequest{
		MethodID: method.ID,
		Minutes:  method.MinutesPerBatch,
		Stage:    Qi(6),
		Held:     craftTestHeld("material.herb.green_vine", 1),
		Draw:     0.5,
	})
	if !errors.Is(err, ErrCraftMissingMaterial) {
		t.Fatalf("one herb cannot pay for a two-herb batch, got %v", err)
	}
}

// TestPlanCraftBatchReadsTheOrderThePlayerIsTold keeps the refusal order honest:
// a character who does not qualify hears about the stage, not about herbs.
func TestPlanCraftBatchReadsTheOrderThePlayerIsTold(t *testing.T) {
	registry := NewCraftRegistryWithBuiltins()
	method, _ := registry.Method("craft.alchemy.spring_return")

	_, err := PlanCraftBatch(registry, CraftRequest{
		MethodID: method.ID,
		Minutes:  method.MinutesPerBatch,
		Stage:    Mortal(),
		Held:     nil,
		Draw:     0.5,
	})
	if !errors.Is(err, ErrCraftStageTooLow) {
		t.Fatalf("a mortal short of everything must be told about the stage first, got %v", err)
	}

	_, err = PlanCraftBatch(registry, CraftRequest{
		MethodID: "craft.alchemy.moon_dew",
		Minutes:  method.MinutesPerBatch,
		Stage:    Qi(6),
		Held:     craftTestHeld("material.herb.green_vine", 2),
		Draw:     0.5,
	})
	if !errors.Is(err, ErrCraftUnknownMethod) {
		t.Fatalf("an unregistered method must reach no output, got %v", err)
	}
}

// TestPlanCraftBatchLosesAtMostOneBatchPerRun is §2's stated loss read once for
// the whole run: a ten-batch commission cannot roll ten times.
func TestPlanCraftBatchLosesAtMostOneBatchPerRun(t *testing.T) {
	registry := NewCraftRegistryWithBuiltins()
	method, _ := registry.Method("craft.alchemy.spring_return")

	lost, err := PlanCraftBatch(registry, CraftRequest{
		MethodID: method.ID,
		Minutes:  method.MinutesPerBatch * 10,
		Stage:    Qi(6),
		Held:     craftTestHeld("material.herb.green_vine", 20),
		Draw:     method.LossChance / 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if lost.FailedBatches != 1 || len(lost.Products) != 1 || lost.Products[0].Quantity != 9 {
		t.Fatalf("only the last batch may be lost, preserving nine products: %#v", lost)
	}
	// The materials and the time are still spent: the work really happened.
	if lost.Batches != 10 || lost.ElapsedMinutes != method.MinutesPerBatch*10 {
		t.Fatalf("a lost run still costs what it did: %#v", lost)
	}

	kept, err := PlanCraftBatch(registry, CraftRequest{
		MethodID: method.ID,
		Minutes:  method.MinutesPerBatch * 10,
		Stage:    Qi(6),
		Held:     craftTestHeld("material.herb.green_vine", 20),
		Draw:     method.LossChance,
	})
	if err != nil {
		t.Fatal(err)
	}
	if kept.FailedBatches != 0 || len(kept.Products) != 1 || kept.Products[0].Quantity != 10 {
		t.Fatalf("a draw outside the band must yield every batch: %#v", kept)
	}
}

// TestCraftMethodValidateRefusesAnUnrunnableRecipe guards the table itself: a
// method whose chances do not add to one would silently drop or double batches.
func TestCraftMethodValidateRefusesAnUnrunnableRecipe(t *testing.T) {
	base := CraftMethodDef{
		ID: "craft.test", Version: 1, Art: CraftArtAlchemy, MinimumStage: "qi_1", MinutesPerBatch: 60,
		Inputs:  []CraftInput{{DefID: "material.herb.green_vine", Quantity: 1}},
		Outputs: []CraftOutput{{DefID: "pill.restore.health.lower", Quantity: 1, Chance: 1}},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("the baseline must be runnable: %v", err)
	}

	short := base
	short.Outputs = []CraftOutput{{DefID: "pill.restore.health.lower", Quantity: 1, Chance: 0.5}}
	if err := short.Validate(); err == nil {
		t.Fatal("output chances that fall short of one must be refused")
	}

	always := base
	always.LossChance = 1
	if err := always.Validate(); err == nil {
		t.Fatal("a method that always fails must be refused")
	}

	staged := base
	staged.MinimumStage = "immortal"
	if err := staged.Validate(); err == nil {
		t.Fatal("an unknown stage must be refused")
	}
}

// TestEveryShippedArtReachesAUsableProduct is the acceptance line behind
// 《自制-修仙技艺》§6's 能学、能做、能使用、能交易及能恢复 read once per art.
//
// The rule the tests here enforce is not "there are three recipes". It is that
// every method the table ships consumes a material the catalogue really prices,
// takes real time, needs a stage the rules can read, and yields a definition the
// rest of the game already settles. A recipe whose product nothing can equip,
// drink or trade would pass a table check and still be unplayable, which §6
// explicitly refuses to call done.
func TestEveryShippedArtReachesAUsableProduct(t *testing.T) {
	catalog := NewItemCatalogWithFirstVersion()

	arts := map[CraftArt]bool{}
	for _, method := range builtinCraftMethods {
		arts[method.Art] = true
		if err := method.Validate(); err != nil {
			t.Fatalf("shipped method %s is not runnable: %v", method.ID, err)
		}
		for _, input := range method.Inputs {
			definition, ok := catalog.Definition(input.DefID)
			if !ok {
				t.Fatalf("method %s consumes %s, which the catalogue does not hold", method.ID, input.DefID)
			}
			if definition.Kind != KindMaterial {
				t.Fatalf("method %s consumes %s, whose kind is %s rather than material", method.ID, input.DefID, definition.Kind)
			}
			if definition.BasePrice <= 0 {
				// An unpriced input cannot be bought, so the art's materials could
				// never be obtained through the economy the design reuses.
				t.Fatalf("method %s consumes %s, which publishes no price", method.ID, input.DefID)
			}
		}
		for _, output := range method.Outputs {
			definition, ok := catalog.Definition(output.DefID)
			if !ok {
				t.Fatalf("method %s yields %s, which the catalogue does not hold", method.ID, output.DefID)
			}
			if definition.BasePrice <= 0 {
				// §6 requires the product to be tradeable, and the trade module
				// prices from the definition. A product with no price could be made
				// but never sold.
				t.Fatalf("method %s yields %s, which publishes no price and so cannot be traded", method.ID, output.DefID)
			}
			// 能使用: the product must be something a real path consumes. A pill is
			// drunk, equipment is equipped, a formation is deployed; anything else
			// would be a product the game can only describe.
			usable := definition.Kind == KindPill || definition.Kind == KindTalisman || definition.Equippable()
			if !usable {
				t.Fatalf("method %s yields %s, a %s that no path can use", method.ID, output.DefID, definition.Kind)
			}
			// 能学: the method's floor and the product's floor must agree, or the
			// art would let a character make what they cannot operate.
			if strings.TrimSpace(definition.ApplicableStage) != "" && definition.ApplicableStage != method.MinimumStage {
				t.Fatalf("method %s needs %s but yields %s, whose floor is %s",
					method.ID, method.MinimumStage, output.DefID, definition.ApplicableStage)
			}
		}
	}
	// Every art must reach an actual consumer rather than a recipe-only entry.
	for _, art := range []CraftArt{CraftArtAlchemy, CraftArtSmithing, CraftArtFormation, CraftArtTalisman} {
		if !arts[art] {
			t.Fatalf("art %s ships no method", art)
		}
	}
}

// TestTheSmithingAndFormationMethodsPriceTheirOwnBatches keeps §2's per-method
// cost honest: the three arts do not share one interval, one material or one
// loss rate merely because they share one arithmetic.
func TestTheSmithingAndFormationMethodsPriceTheirOwnBatches(t *testing.T) {
	registry := NewCraftRegistryWithBuiltins()
	alchemy, _ := registry.Method("craft.alchemy.spring_return")
	smithing, ok := registry.Method("craft.smithing.iron_sword")
	if !ok {
		t.Fatal("the smithing method must be registered")
	}
	formation, ok := registry.Method("craft.formation.gathered_edge")
	if !ok {
		t.Fatal("the formation method must be registered")
	}
	if smithing.Art != CraftArtSmithing || formation.Art != CraftArtFormation {
		t.Fatalf("the arts are mislabelled: %s, %s", smithing.Art, formation.Art)
	}
	if smithing.MinutesPerBatch == alchemy.MinutesPerBatch || formation.MinutesPerBatch == alchemy.MinutesPerBatch {
		t.Fatal("each method must state its own interval rather than inherit one")
	}
	if smithing.Inputs[0].DefID == alchemy.Inputs[0].DefID {
		t.Fatal("each method must consume its own material")
	}

	// The formation's floor is the array's own qi_4, so a qi_1 character is told
	// about the stage rather than about the stone.
	if _, err := PlanCraftBatch(registry, CraftRequest{
		MethodID: formation.ID,
		Minutes:  formation.MinutesPerBatch,
		Stage:    Qi(1),
		Held:     craftTestHeld("material.stone.ward", 4),
		Draw:     0.5,
	}); !errors.Is(err, ErrCraftStageTooLow) {
		t.Fatalf("a qi_1 character cannot cut a qi_4 array, got %v", err)
	}

	// A qualified character working the smithing method really yields the
	// catalogue's white blade, and the ore is really spent.
	batch, err := PlanCraftBatch(registry, CraftRequest{
		MethodID: smithing.ID,
		Minutes:  smithing.MinutesPerBatch,
		Stage:    Qi(6),
		Held:     craftTestHeld("material.ore.iron", 3),
		Draw:     smithing.LossChance,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Products) != 1 || batch.Products[0].DefID != "implement.lower.common" {
		t.Fatalf("the forge must yield the registered blade: %#v", batch.Products)
	}
	if len(batch.Consumed) != 1 || batch.Consumed[0].DefID != "material.ore.iron" || batch.Consumed[0].Quantity != 3 {
		t.Fatalf("the forge must spend its own ore: %#v", batch.Consumed)
	}
}

func TestCraftConsumesSplitMaterialStacksByInstance(t *testing.T) {
	batch, err := PlanCraftBatch(NewCraftRegistryWithBuiltins(), CraftRequest{MethodID: "craft.talisman.ward", Minutes: 240, Stage: Qi(1), Draw: .5, Held: []CraftHeld{
		{InstanceID: "paper-a", DefID: "material.talisman.paper", Quantity: 1},
		{InstanceID: "paper-b", DefID: "material.talisman.paper", Quantity: 1},
		{InstanceID: "ink", DefID: "material.talisman.ink", Quantity: 2},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Consumed) != 3 || batch.Consumed[0].InstanceID != "paper-a" || batch.Consumed[0].Quantity != 1 || batch.Consumed[1].InstanceID != "paper-b" || batch.Consumed[1].Quantity != 1 || batch.Consumed[2].Quantity != 2 {
		t.Fatalf("split stock deduction = %+v", batch.Consumed)
	}
}
