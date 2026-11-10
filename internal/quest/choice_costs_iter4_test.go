package quest

import "testing"

func TestChoiceEffortAndFuelBoundsIter4(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Choice)
	}{
		{"negative_fuel", func(c *Choice) { c.AssetFuelCost = -1 }},
		{"excessive_fuel", func(c *Choice) { c.AssetFuelCost = 101 }},
		{"fuel_without_asset", func(c *Choice) { c.AssetFuelCost = 1; c.RequiredAsset = "" }},
		{"invalid_method_id", func(c *Choice) { c.UseSkillID = "not an id" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := DefinitionByID("xinghai-a18")
			tc.change(&d.Choices[1])
			if len(ValidateDefinition(d)) == 0 {
				t.Fatal("invalid method cost accepted")
			}
		})
	}
	d, _ := DefinitionByID("xinghai-a18")
	if len(ValidateDefinition(d)) != 0 {
		t.Fatal("authored bounded method invalid")
	}
}
