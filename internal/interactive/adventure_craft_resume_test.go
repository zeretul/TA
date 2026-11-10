package interactive

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAdventureCraftResumesWithPaidMaterialsAndNoEarlyReward(t *testing.T) {
	for _, family := range []string{"wuxia", "space", "fantasy"} {
		t.Run(family, func(t *testing.T) {
			r := adventureTestRules()
			r.Family = family
			if family != "space" {
				r = generatedWuxiaRules()
				r.Family = family
			}
			a := NewAdventureActor(r)
			a.Inventory["scrap"] = 3
			before, _ := json.Marshal(a)
			partial, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "craft", RecipeID: "make-blade", Hours: 1}, "first")
			if err != nil {
				t.Fatal(err)
			}
			if partial.Inventory["scrap"] != 0 || partial.Inventory["blade"] != 0 || partial.Experience != a.Experience || out.DurationSeconds != 3600 || partial.Crafting["make-blade"].WorkedHours != 1 {
				t.Fatalf("invalid partial result: %+v %+v", partial, out)
			}
			if b, _ := json.Marshal(a); string(before) != string(b) {
				t.Fatal("preview mutated input")
			}
			bytes, _ := json.Marshal(partial)
			var restored AdventureActor
			if err = json.Unmarshal(bytes, &restored); err != nil {
				t.Fatal(err)
			}
			if _, _, err = ResolveAdventureAction(r, restored, AdventureAction{Action: "sell", ItemID: "blade", MarketID: "dock"}, "sell"); err == nil {
				t.Fatal("sold unfinished product")
			}
			if _, _, err = ResolveAdventureAction(r, restored, AdventureAction{Action: "craft", RecipeID: "make-blade", Hours: 1, Quantity: 2}, "changed"); err == nil {
				t.Fatal("changed quantity of paid batch")
			}
			final, out, err := ResolveAdventureAction(r, restored, AdventureAction{Action: "craft", RecipeID: "make-blade", Hours: 8}, "finish")
			if err != nil {
				t.Fatal(err)
			}
			if final.Inventory["scrap"] != 0 || final.Inventory["blade"] != 1 || out.DurationSeconds != 3600 || final.Experience != a.Experience+2 || len(final.Crafting) != 0 {
				t.Fatalf("invalid completion: %+v %+v", final, out)
			}
			again, _, err := ResolveAdventureAction(r, final, AdventureAction{Action: "craft", RecipeID: "make-blade", Hours: 1}, "no-stock")
			if err == nil || !reflect.DeepEqual(again, final) {
				t.Fatal("started an unpaid new batch")
			}
		})
	}
}
