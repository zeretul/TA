package app

import (
	"encoding/json"
	"testing"

	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

func TestRadioContactRejectsOwnedNontransmittingSourceEquipment(t *testing.T) {
	pack, e := storypack.NewLibrary(t.TempDir()).Get("xinghai-689")
	if e != nil {
		t.Fatal(e)
	}
	rules := pack.Story.Gameplay.AdventureRules
	for _, id := range []string{"xinghai-signal-processor", "xinghai-i17", "xinghai-i49", "xinghai-radio", "legacy-radio"} {
		t.Run(id, func(t *testing.T) {
			deviceID := id
			if id == "legacy-radio" {
				deviceID = "xinghai-radio"
				for index := range rules.Items {
					if rules.Items[index].ID == deviceID {
						rules.Items[index].RadioDevice = false
					}
				}
			}
			actor := interactive.NewAdventureActor(*rules)
			actor.Inventory[deviceID] = 1
			encoded, _ := json.Marshal(actor)
			var value map[string]any
			_ = json.Unmarshal(encoded, &value)
			ctx := interactive.StoryContext{Meta: interactive.StoryMeta{PackBinding: &interactive.StoryPackBinding{PackID: "xinghai-689", Gameplay: &interactive.StoryGameplayProfile{Communication: "radio"}}}, Snapshot: interactive.Snapshot{ActorStateSchema: &interactive.ActorStateSchemaSnapshot{System: interactive.StoryDirectorActorStateSystem{AdventureRules: rules}}, State: map[string]any{"actors": map[string]any{"protagonist": map[string]any{"state": map[string]any{interactive.AdventureStateField: value, "重要物品": map[string]any{deviceID: 1}}}}}}}
			state := character.State{Profiles: []character.Profile{{ID: "remote", RadioContact: &character.RadioContact{DeviceItemID: deviceID, Channel: "actually-agreed", Status: "connected"}}}}
			if got := radioContactAvailable(ctx, state, "remote"); got != (deviceID == "xinghai-radio") {
				t.Fatalf("owned %s transmitting=%v", id, got)
			}
		})
	}
}
