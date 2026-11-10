package storypack

import (
	"encoding/json"
	"strings"
	"ta/internal/interactive"
	"testing"
)

func TestNamedNPCEncounterHomeHintMustHaveAnAuthoredSource(t *testing.T) {
	for _, original := range adventureContentPacks(t) {
		if err := original.Validate(); err != nil {
			t.Fatal(err)
		}
		// Change a deep copy of a single named encounter, not global builtins.
		p := original
		data, err := json.Marshal(p.Story.Gameplay.AdventureRules)
		if err != nil {
			t.Fatal(err)
		}
		var rules interactive.AdventureRules
		if err := json.Unmarshal(data, &rules); err != nil {
			t.Fatal(err)
		}
		p.Story.Gameplay.AdventureRules = &rules
		if err := p.Validate(); err != nil {
			t.Fatal(err)
		}
		for i := range p.Story.Gameplay.AdventureRules.Opponents {
			enemy := &p.Story.Gameplay.AdventureRules.Opponents[i]
			if !enemy.RequiresPresence {
				continue
			}
			enemy.Location = "an invented unregistered home"
			if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "adventure opponent references unknown location") {
				t.Fatalf("%s did not reject the unsourced named home for its location: %v", p.ID, err)
			}
			break
		}
	}
}
