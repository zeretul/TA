package quest

import (
	"encoding/json"
	"testing"
)

// This candidate requires handovers.patch. It has not been compiled or run by
// the independent reviewer. Each case isolates the new optional schema only.
func TestChoiceItemHandoversRequireExplicitBoundedCosts(t *testing.T) {
	original, ok := DefinitionByID("sg-crypt-inquiry")
	if !ok {
		t.Fatal("missing authored investigation")
	}
	fresh := func(t *testing.T) (Definition, *Choice) {
		t.Helper()
		raw, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		var d Definition
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Fatal(err)
		}
		for i := range d.Choices {
			if d.Choices[i].ID == "sg-crypt-inquiry.stage3.c1" {
				return d, &d.Choices[i]
			}
		}
		t.Fatal("missing handover choice")
		return Definition{}, nil
	}
	for _, tc := range []struct {
		name  string
		valid bool
		alter func(*Choice)
	}{
		{"authored_return", true, func(c *Choice) {}},
		{"legacy_empty", true, func(c *Choice) { c.ItemHandovers = nil }},
		{"explicit_subset", true, func(c *Choice) { c.ItemCosts["sg-oath-key"] = 2 }},
		{"split_receivers", true, func(c *Choice) {
			c.ItemCosts["sg-oath-key"] = 2
			c.ItemHandovers = append(c.ItemHandovers, ItemHandover{CharacterSourceKey: "sg-p31", Items: map[string]int{"sg-oath-key": 1}})
		}},
		{"blank_source", false, func(c *Choice) { c.ItemHandovers[0].CharacterSourceKey = "" }},
		{"empty_items", false, func(c *Choice) { c.ItemHandovers[0].Items = nil }},
		{"zero_quantity", false, func(c *Choice) { c.ItemHandovers[0].Items["sg-oath-key"] = 0 }},
		{"negative_quantity", false, func(c *Choice) { c.ItemHandovers[0].Items["sg-oath-key"] = -1 }},
		{"uncosted_item", false, func(c *Choice) { c.ItemHandovers[0].Items = map[string]int{"sg-cloth": 1} }},
		{"too_many_items", false, func(c *Choice) { c.ItemHandovers[0].Items["sg-oath-key"] = 2 }},
		{"duplicate_recipient", false, func(c *Choice) { c.ItemHandovers = append(c.ItemHandovers, c.ItemHandovers[0]) }},
		{"combined_overdraft", false, func(c *Choice) {
			c.ItemHandovers = append(c.ItemHandovers, ItemHandover{CharacterSourceKey: "sg-p31", Items: map[string]int{"sg-oath-key": 1}})
		}},
		{"reward_remints_handover", false, func(c *Choice) {
			c.Rewards = append(c.Rewards, Reward{Kind: RewardItem, RefID: "sg-oath-key", Quantity: 1})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, c := fresh(t)
			tc.alter(c)
			diagnostics := ValidateDefinition(d)
			if (len(diagnostics) == 0) != tc.valid {
				t.Fatalf("valid=%v diagnostics=%+v", tc.valid, diagnostics)
			}
		})
	}
}
