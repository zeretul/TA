package cultivation

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestAuthoredCraftCapacityLeavesRoomForFrozenBuiltins(t *testing.T) {
	builtins := NewCraftRegistryWithBuiltins().Methods()
	base, _ := NewCraftRegistryWithBuiltins().Method("craft.talisman.ward")
	base.Source = "author"
	base.FailureCost = "consume_inputs"
	methods := make([]CraftMethodDef, 64-len(builtins))
	for i := range methods {
		methods[i] = base
		methods[i].ID = fmt.Sprintf("custom.%d", i)
	}
	if err := ValidateCraftMethods(methods, AuthoredCraftCatalog); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCraftMethods(append(builtins, methods...), FrozenCraftCatalog); err != nil {
		t.Fatal("accepted world cannot freeze", err)
	}
	base.ID = "custom.overflow"
	if ValidateCraftMethods(append(methods, base), AuthoredCraftCatalog) == nil {
		t.Fatal("oversize author catalog accepted")
	}
}

func TestCraftRegistryDoesNotShareMutableRecipeSlices(t *testing.T) {
	first := NewCraftRegistryWithBuiltins()
	definition, _ := first.Method("craft.talisman.ward")
	definition.Inputs[0].Quantity = 99
	stored, _ := first.Method(definition.ID)
	next, _ := NewCraftRegistryWithBuiltins().Method(definition.ID)
	if stored.Inputs[0].Quantity != 1 || next.Inputs[0].Quantity != 1 {
		t.Fatal("editing a retrieved method mutated its registry or another world's builtins")
	}
}

func TestAuthoredCraftValidatesAndEnforcesToolsAndLocation(t *testing.T) {
	method, _ := NewCraftRegistryWithBuiltins().Method("craft.talisman.ward")
	method.ID = "custom.ward"
	method.Source = "teacher manuscript"
	method.FailureCost = "consume_inputs"
	method.LossChance = 0
	method.RequiredTools = []string{"implement.lower.common"}
	method.RequiredLocation = "workbench"
	if err := ValidateCraftMethods([]CraftMethodDef{method}, AuthoredCraftCatalog); err != nil {
		t.Fatal(err)
	}
	registry := NewCraftRegistry()
	_ = registry.Register(method)
	request := CraftRequest{MethodID: method.ID, Stage: Qi(6), Minutes: 120, Location: "workbench", Draw: .5, Held: []CraftHeld{{DefID: "material.talisman.paper", InstanceID: "paper", Quantity: 1}, {DefID: "material.talisman.ink", InstanceID: "ink", Quantity: 1}}}
	if _, err := PlanCraftBatch(registry, request); err == nil {
		t.Fatal("missing tool accepted")
	}
	request.Held = append(request.Held, CraftHeld{DefID: "implement.lower.common", InstanceID: "tool", Quantity: 1})
	request.Location = "street"
	if _, err := PlanCraftBatch(registry, request); err == nil {
		t.Fatal("wrong committed location accepted")
	}
	request.Location = "workbench"
	batch, err := PlanCraftBatch(registry, request)
	if err != nil || len(batch.Products) != 1 || batch.Products[0].Quantity != 1 || len(batch.Consumed) != 2 {
		t.Fatalf("real craft: %+v %v", batch, err)
	}
	for _, spent := range batch.Consumed {
		if spent.InstanceID == "tool" {
			t.Fatal("tool was consumed")
		}
	}
	invalid := method
	invalid.Outputs = []CraftOutput{{DefID: "invented.effect", Quantity: 1, Chance: 1}}
	if ValidateCraftMethods([]CraftMethodDef{invalid}, AuthoredCraftCatalog) == nil {
		t.Fatal("invented effect accepted")
	}
	invalid = method
	invalid.Inputs = append(append([]CraftInput(nil), method.Inputs...), method.Inputs[0])
	if ValidateCraftMethods([]CraftMethodDef{invalid}, AuthoredCraftCatalog) == nil {
		t.Fatal("duplicate material could overspend a stack")
	}
	if err := json.Unmarshal([]byte(`{"id":"invented","effect":{"damage":999}}`), &invalid); err == nil {
		t.Fatal("invented effect field silently ignored")
	}
}
