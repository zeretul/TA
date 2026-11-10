package interactive

import "testing"

// This is a market-accounting fixture, not a claim of earned travel between
// merchants. Start with the real smith's skill, tools and purse, then earn the
// money, buy every ingredient, craft and resell through the actual reducer.
func TestFantasyStarterCraftHasSelfUseValueWithoutResaleArbitrage(t *testing.T) {
	for _, recipeID := range []string{"sg-recipe-blade", "sg-recipe-arrows"} {
		t.Run(recipeID, func(t *testing.T) {
			r, a := fantasyOrigin(t, "sg-start-smith")
			items := map[string]AdventureItem{}
			for _, item := range r.Items {
				items[item.ID] = item
			}
			var recipe AdventureRecipe
			for _, candidate := range r.Recipes {
				if candidate.ID == recipeID {
					recipe = candidate
				}
			}
			if recipe.ID == "" {
				t.Fatal("missing starter recipe")
			}
			materialCost := 0
			for id, count := range recipe.Materials {
				materialCost += items[id].Price * count
			}
			retail := items[recipe.OutputID].Price * recipe.OutputCount
			resale := (items[recipe.OutputID].Price / 2) * recipe.OutputCount
			if materialCost >= retail {
				t.Errorf("making the ordinary starter item has no material saving: materials=%d retail=%d", materialCost, retail)
			}
			if materialCost <= resale {
				t.Errorf("starter recipe creates a buy/craft/sell arbitrage: materials=%d resale=%d", materialCost, resale)
			}
			apply := func(action AdventureAction) AdventureOutcome {
				t.Helper()
				next, out, err := ResolveAdventureAction(r, a, action, recipeID)
				if err != nil {
					t.Fatalf("%+v: %v", action, err)
				}
				a = next
				return out
			}
			apply(AdventureAction{Action: "work", MarketID: "sg-market-23", Hours: 2})
			beforeCurrency := a.Currency
			for id, count := range recipe.Materials {
				apply(AdventureAction{Action: "buy", MarketID: "sg-market-23", ItemID: id, Quantity: count})
			}
			beforeOutput := a.Inventory[recipe.OutputID]
			out := apply(AdventureAction{Action: "craft", RecipeID: recipeID, Hours: recipe.Hours})
			if out.DurationSeconds != int64(recipe.Hours)*3600 || a.Inventory[recipe.OutputID] != beforeOutput+recipe.OutputCount || a.Inventory[recipe.RequiredTool] != 1 {
				t.Fatal("craft lost its actual duration, output or reusable tool")
			}
			for id := range recipe.Materials {
				if a.Inventory[id] != 0 {
					t.Fatal("purchased material was not actually spent", id)
				}
			}
			if a.Equipment["person:weapon"] == recipe.OutputID {
				apply(AdventureAction{Action: "unequip", ItemID: recipe.OutputID})
			}
			marketID := "sg-market-23"
			if recipe.OutputID == "sg-arrow" {
				marketID = "sg-market-13"
			}
			apply(AdventureAction{Action: "sell", MarketID: marketID, ItemID: recipe.OutputID, Quantity: recipe.OutputCount})
			if a.Currency != beforeCurrency-materialCost+resale || a.Inventory[recipe.OutputID] != beforeOutput {
				t.Fatal("formal sale differs from the reviewed material/resale accounting")
			}
			t.Logf("materials=%d retail=%d resale=%d craft_hours=%d self_use_saving=%d resale_margin=%d", materialCost, retail, resale, recipe.Hours, retail-materialCost, resale-materialCost)
		})
	}
}
