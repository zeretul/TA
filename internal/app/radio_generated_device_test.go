package app

import (
	"encoding/json"
	"testing"

	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// This is an isolated ownership/qualification regression, not natural AI play.
// The fixture supplies one real radio, then uses the ordinary adaptation rule.
func TestAdventureRadioRequiresCanonicalOwnedGeneratedDevice(t *testing.T) {
	pack, err := storypack.NewLibrary(t.TempDir()).Get("xinghai-689")
	if err != nil {
		t.Fatal(err)
	}
	rules := *pack.Story.Gameplay.AdventureRules
	actor := interactive.NewAdventureActor(rules)
	actor.Inventory["xinghai-radio"] = 1
	definition := interactive.AdventureGeneratedDefinition{Category: "equipment", TemplateID: "xinghai-radio", Name: "防尘便携无线电", Source: "为持有的无线电加上防尘外壳"}
	adapted, _, err := interactive.ResolveAdventureAction(rules, actor, interactive.AdventureAction{Action: "generate", Generate: &definition}, "radio-adaptation")
	if err != nil {
		t.Fatal(err)
	}
	generatedID := ""
	originalIDs := map[string]bool{}
	for _, item := range rules.Items {
		originalIDs[item.ID] = true
	}
	for id, quantity := range adapted.Inventory {
		if quantity > 0 && !originalIDs[id] {
			generatedID = id
		}
	}
	if generatedID == "" || adapted.Inventory["xinghai-radio"] != 0 {
		t.Fatal("adaptation did not replace the owned radio")
	}
	contextFor := func(a interactive.AdventureActor) interactive.StoryContext {
		encoded, marshalErr := json.Marshal(a)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		var canonical map[string]any
		if unmarshalErr := json.Unmarshal(encoded, &canonical); unmarshalErr != nil {
			t.Fatal(unmarshalErr)
		}
		return interactive.StoryContext{
			Meta: interactive.StoryMeta{PackBinding: &interactive.StoryPackBinding{PackID: pack.ID, Gameplay: &interactive.StoryGameplayProfile{Communication: "radio"}}},
			Snapshot: interactive.Snapshot{
				ActorStateSchema: &interactive.ActorStateSchemaSnapshot{System: interactive.StoryDirectorActorStateSystem{AdventureRules: &rules}},
				State: map[string]any{"actors": map[string]any{interactive.DefaultActorID: map[string]any{"state": map[string]any{
					interactive.AdventureStateField: canonical,
					// Free-form descriptive possessions must never override canonical stock.
					"重要物品": map[string]any{"xinghai-radio": 1, generatedID: 1},
				}}}},
			},
		}
	}
	for _, tc := range []struct {
		name, device, status, channel string
		owned, want                   bool
	}{
		{"original contact survives physical adaptation", "xinghai-radio", "connected", "agreed", true, true},
		{"generated device can be explicitly registered", generatedID, "connected", "agreed", true, true},
		{"offline contact stays offline", generatedID, "offline", "agreed", true, false},
		{"hardware grants no channel", generatedID, "connected", "", true, false},
		{"lost original cannot use descriptive inventory", "xinghai-radio", "connected", "agreed", false, false},
		{"lost adaptation cannot use descriptive inventory", generatedID, "connected", "agreed", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := adapted
			a.Inventory = map[string]int{}
			for id, quantity := range adapted.Inventory {
				a.Inventory[id] = quantity
			}
			if !tc.owned {
				a.Inventory[generatedID] = 0
			}
			state := character.State{Profiles: []character.Profile{{ID: "remote", RadioContact: &character.RadioContact{DeviceItemID: tc.device, Channel: tc.channel, Status: tc.status}}}}
			if got := radioContactAvailable(contextFor(a), state, "remote"); got != tc.want {
				t.Fatalf("available=%v, want %v", got, tc.want)
			}
		})
	}
}
