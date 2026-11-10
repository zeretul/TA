package interactive

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Load the actual authored rule catalog rather than a conveniently permissive
// synthetic receiver. Campaign tests separately prove honest acquisition.
func authoredAdventureRules(t *testing.T, packID string) AdventureRules {
	t.Helper()
	b, err := os.ReadFile("../storypack/assets/" + packID + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var pack struct {
		Story struct {
			Gameplay struct {
				Rules AdventureRules `json:"adventure_rules"`
			} `json:"gameplay"`
		} `json:"story"`
	}
	if err := json.Unmarshal(b, &pack); err != nil {
		t.Fatal(err)
	}
	pack.Story.Gameplay.Rules.world = authoredWorldContent(t, packID)
	return pack.Story.Gameplay.Rules
}

func TestAdventureSignalProcessingConsumesActualSourceRequirements(t *testing.T) {
	r := authoredAdventureRules(t, "xinghai-689")
	a := NewAdventureActor(r)
	// This bounded solver fixture is deliberately separate from the earned Story
	// campaign. It isolates each prerequisite and observes exact atomic rejection.
	a.Rank = 4
	a.Skills["xinghai-s29"] = 1
	a.Currency = 2000
	apply := func(in AdventureAction) {
		t.Helper()
		n, _, e := ResolveAdventureAction(r, a, in, "signal-setup")
		if e != nil {
			t.Fatal(e)
		}
		a = n
	}
	apply(AdventureAction{Action: "buy", MarketID: "xinghai-market-l21", ItemID: "xinghai-signal-processor"})
	apply(AdventureAction{Action: "buy", MarketID: "xinghai-market-l21", ItemID: "xinghai-hopping-recording", Quantity: 2})
	apply(AdventureAction{Action: "deploy", ItemID: "xinghai-signal-processor"})
	input := AdventureAction{Action: "craft", RecipeID: "xinghai-decode-hopping", Hours: 4}
	rejected := func(name string, edit func(*AdventureActor), in AdventureAction) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			b, _ := json.Marshal(a)
			var candidate AdventureActor
			_ = json.Unmarshal(b, &candidate)
			edit(&candidate)
			before, _ := json.Marshal(candidate)
			got, out, e := ResolveAdventureAction(r, candidate, in, "signal-refuse")
			after, _ := json.Marshal(candidate)
			if e == nil || !reflect.DeepEqual(got, candidate) || string(before) != string(after) || out.DurationSeconds != 0 {
				t.Fatalf("refusal was not atomic: %v %+v", e, out)
			}
		})
	}
	rejected("unlearned", func(a *AdventureActor) { delete(a.Skills, "xinghai-s29") }, input)
	rejected("unowned", func(a *AdventureActor) {
		delete(a.Inventory, "xinghai-signal-processor")
		delete(a.Assets, "xinghai-signal-processor")
		delete(a.Equipment, "machine:asset")
	}, input)
	rejected("portable radio is not the processor", func(a *AdventureActor) {
		delete(a.Inventory, "xinghai-signal-processor")
		delete(a.Assets, "xinghai-signal-processor")
		delete(a.Equipment, "machine:asset")
		a.Inventory["xinghai-radio"] = 1
	}, input)
	rejected("undeployed", func(a *AdventureActor) { delete(a.Equipment, "machine:asset") }, input)
	rejected("destroyed", func(a *AdventureActor) {
		s := a.Assets["xinghai-signal-processor"]
		s.Health = 0
		a.Assets["xinghai-signal-processor"] = s
	}, input)
	rejected("unpowered", func(a *AdventureActor) {
		s := a.Assets["xinghai-signal-processor"]
		s.Fuel = 9
		a.Assets["xinghai-signal-processor"] = s
	}, input)
	rejected("power lock", func(a *AdventureActor) {
		s := a.Assets["xinghai-signal-processor"]
		s.DisabledUntil = 3600
		a.Assets["xinghai-signal-processor"] = s
	}, input)
	rejected("no recording", func(a *AdventureActor) { delete(a.Inventory, "xinghai-hopping-recording") }, input)
	partial, partialOut, err := ResolveAdventureAction(r, a, AdventureAction{Action: "craft", RecipeID: input.RecipeID, Hours: 3}, "listen-part")
	if err != nil || partialOut.DurationSeconds != 10800 || partial.Inventory["xinghai-signal-fragments"] != 0 || partial.Inventory["xinghai-hopping-recording"] != 1 || partial.Assets["xinghai-signal-processor"].Fuel != a.Assets["xinghai-signal-processor"].Fuel-10 {
		t.Fatalf("partial listening did not reserve inputs: %v %+v", err, partialOut)
	}
	completed, completeOut, err := ResolveAdventureAction(r, partial, AdventureAction{Action: "craft", RecipeID: input.RecipeID, Hours: 1}, "listen-finish")
	if err != nil || completeOut.DurationSeconds != 3600 || completed.Inventory["xinghai-signal-fragments"] != 1 || completed.Inventory["xinghai-hopping-recording"] != 1 || completed.Assets["xinghai-signal-processor"].Fuel != partial.Assets["xinghai-signal-processor"].Fuel {
		t.Fatalf("resume charged twice or lost result: %v %+v", err, completeOut)
	}
	rejected("quantum unsupported", func(a *AdventureActor) {}, AdventureAction{Action: "craft", RecipeID: "xinghai-decode-quantum", Hours: 4})
	rejected("not a combat carrier", func(a *AdventureActor) {}, AdventureAction{Action: "battle_start", OpponentID: "xinghai-e05"})
	before := a.Assets["xinghai-signal-processor"]
	got, out, e := ResolveAdventureAction(r, a, input, "signal-success")
	if e != nil {
		t.Fatal(e)
	}
	if got.Inventory["xinghai-hopping-recording"] != 1 || got.Inventory["xinghai-signal-fragments"] != 1 || got.Assets["xinghai-signal-processor"].Fuel != before.Fuel-10 || out.DurationSeconds != 4*3600 {
		t.Fatalf("wrong actual processing: %+v %+v", got, out)
	}
	if !reflect.DeepEqual(got.Skills, a.Skills) || !reflect.DeepEqual(got.Equipment, a.Equipment) {
		t.Fatal("processing granted capabilities or changed ownership")
	}
}
