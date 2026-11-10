package cultivation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// UnmarshalJSON refuses invented effect fields rather than silently accepting
// a recipe whose promised effect is not part of the registered item consumer.
func (m *CraftMethodDef) UnmarshalJSON(data []byte) error {
	type definition CraftMethodDef
	var value definition
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*m = CraftMethodDef(value)
	return nil
}

type CraftCatalogMode string

const (
	AuthoredCraftCatalog CraftCatalogMode = "authored"
	FrozenCraftCatalog   CraftCatalogMode = "frozen"
)

// ValidateCraftMethods bounds an authored catalog and resolves every reference
// against the existing item engine. Frozen catalogs may include builtins.
func ValidateCraftMethods(methods []CraftMethodDef, mode CraftCatalogMode) error {
	catalog := NewItemCatalogWithFirstVersion()
	builtins := NewCraftRegistryWithBuiltins()
	limit := 64
	switch mode {
	case AuthoredCraftCatalog:
		limit -= len(builtins.IDs())
	case FrozenCraftCatalog:
	default:
		return fmt.Errorf("craft catalog: unknown validation mode")
	}
	if len(methods) > limit {
		return fmt.Errorf("craft catalog: maximum %d methods", limit)
	}
	registry := NewCraftRegistry()
	seen := map[string]bool{}
	for _, method := range methods {
		if seen[method.ID] {
			return fmt.Errorf("craft catalog: duplicate method %q", method.ID)
		}
		seen[method.ID] = true
		if err := registry.Register(method); err != nil {
			return err
		}
		_, builtin := builtins.Method(method.ID)
		if mode == AuthoredCraftCatalog && builtin {
			return fmt.Errorf("craft catalog: builtin method cannot be replaced")
		}
		if !builtin {
			if strings.TrimSpace(method.Source) == "" || strings.TrimSpace(method.NameKey) == "" || !method.RequiresLearning || method.FailureCost != "consume_inputs" {
				return fmt.Errorf("craft catalog: authored method requires source, name, learning and explicit failure cost")
			}
		}
		if method.MinutesPerBatch > 525600 {
			return fmt.Errorf("craft catalog: batch duration exceeds one year")
		}
		if len(method.Source) > 2048 || len(method.NameKey) > 256 || len(method.RequiredLocation) > 256 || len(method.Inputs) > 16 || len(method.Outputs) > 16 || len(method.RequiredTools) > 16 {
			return fmt.Errorf("craft catalog: definition exceeds context limits")
		}
		for _, input := range method.Inputs {
			if input.Quantity > 1000000 {
				return fmt.Errorf("craft catalog: material quantity exceeds limit")
			}
			def, ok := catalog.Definition(input.DefID)
			if !ok || def.Kind != KindMaterial {
				return fmt.Errorf("%w: %s", ErrCraftUnknownMaterial, input.DefID)
			}
		}
		for _, output := range method.Outputs {
			if output.Quantity > 1000000 {
				return fmt.Errorf("craft catalog: output quantity exceeds limit")
			}
			def, ok := catalog.Definition(output.DefID)
			if !ok || def.Kind == KindMaterial {
				return fmt.Errorf("%w: %s", ErrCraftUnknownOutput, output.DefID)
			}
		}
		for _, tool := range method.RequiredTools {
			if _, ok := catalog.Definition(tool); !ok {
				return fmt.Errorf("craft catalog: unknown tool %q", tool)
			}
			for _, input := range method.Inputs {
				if input.DefID == tool {
					return fmt.Errorf("craft catalog: tool cannot also be consumed")
				}
			}
		}
	}
	return nil
}
