package interactive

import (
	"fmt"
	"testing"
)

func TestFantasyEveryOriginUsesItsOwnedFirstMethod(t *testing.T) {
	// Combat placement is isolated. All resources, knowledge and equipment come
	// from the real origin; this test makes no claim of earned cross-map travel.
	for origin, skill := range map[string]string{"road": "blade", "learner": "spark", "scout": "bow", "smith": "blade", "carer": "prayer", "grove": "thorn", "runner": "knife"} {
		t.Run(origin, func(t *testing.T) {
			r, a := fantasyOrigin(t, "sg-start-"+origin)
			next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "sg-practice"}, "origin-practice")
			if err != nil {
				t.Fatal(err)
			}
			next, _, err = ResolveAdventureAction(r, next, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "sg-" + skill}, "origin-method")
			if err != nil {
				t.Fatal(err)
			}
			if next.Energy >= a.Energy {
				t.Fatal("owned first method did not consume effort")
			}
		})
	}
}

func TestFantasyEveryRecipeConsumesItsAuthoredMaterials(t *testing.T) {
	r, _ := fantasyOrigin(t, "sg-start-smith")
	for _, recipe := range r.Recipes {
		t.Run(recipe.ID, func(t *testing.T) {
			// Deliberately qualified, fully provisioned fixture, not earned progression.
			_, a := fantasyOrigin(t, "sg-start-smith")
			a.Rank = 30
			a.Skills = map[string]int{recipe.SkillID: 10}
			a.Inventory = map[string]int{}
			a.Equipment = map[string]string{}
			for id, n := range recipe.Materials {
				a.Inventory[id] = n
			}
			if recipe.RequiredTool != "" {
				a.Inventory[recipe.RequiredTool] = 1
			}
			beforeOutput := a.Inventory[recipe.OutputID]
			result, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "craft", RecipeID: recipe.ID, Hours: recipe.Hours}, "recipe-"+recipe.ID)
			if err != nil {
				t.Fatal(err)
			}
			if result.Inventory[recipe.OutputID] != beforeOutput+recipe.OutputCount {
				t.Fatal("output quantity changed")
			}
			for id := range recipe.Materials {
				if result.Inventory[id] != 0 {
					t.Fatal("ingredient not spent: " + id)
				}
			}
			if recipe.RequiredTool != "" && result.Inventory[recipe.RequiredTool] != 1 {
				t.Fatal("reusable tool consumed")
			}
		})
	}
}

func TestFantasyEveryMethodHasAnActualLearnCommand(t *testing.T) {
	r, _ := fantasyOrigin(t, "sg-start-road")
	for _, skill := range r.Skills {
		t.Run(skill.ID, func(t *testing.T) {
			// A rank-qualified learner with the exact authored source and prerequisite.
			_, a := fantasyOrigin(t, "sg-start-road")
			a.Rank = 30
			a.Skills = map[string]int{}
			a.Inventory = map[string]int{skill.RequiredItem: 1}
			a.Equipment = map[string]string{}
			if skill.Prerequisite != "" {
				a.Skills[skill.Prerequisite] = 1
			}
			if skill.RequiredTool != "" {
				a.Inventory[skill.RequiredTool] = 1
			}
			result, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "learn", SkillID: skill.ID, Hours: skill.TrainingHours}, fmt.Sprintf("learn-%s", skill.ID))
			if err != nil {
				t.Fatal(err)
			}
			if result.Skills[skill.ID] != 1 {
				t.Fatal("source did not lead to learned knowledge")
			}
		})
	}
}

// A fully qualified synthetic opponent isolates each skill's actual consumer.
// This is not a campaign, a claim of earned mastery, or 44 natural AI battles.
func TestFantasyEveryCombatMethodUsesRealResources(t *testing.T) {
	r, _ := fantasyOrigin(t, "sg-start-road")
	for _, skill := range r.Skills {
		if skill.Kind == "social" || skill.Kind == "engineering" {
			continue
		}
		t.Run(skill.ID, func(t *testing.T) {
			_, a := fantasyOrigin(t, "sg-start-road")
			a.Rank = 30
			a.MaxHealth = 1000
			a.Health = 500
			a.MaxEnergy = 1000
			a.Energy = 400
			a.Skills = map[string]int{skill.ID: 1}
			a.Inventory = map[string]int{}
			a.Equipment = map[string]string{}
			if skill.RequiredTool != "" {
				a.Inventory[skill.RequiredTool] = 1
			}
			for item, n := range skill.Materials {
				a.Inventory[item] = n + 8
			}
			weaponID := "sg-travel-blade"
			for _, item := range r.Items {
				if item.Kind == "weapon" && (skill.RequiredWeapon != "" && item.ID == skill.RequiredWeapon || skill.RequiredWeaponClass != "" && item.WeaponClass == skill.RequiredWeaponClass) {
					weaponID = item.ID
					break
				}
			}
			a.Inventory[weaponID] = 1
			a.Equipment["person:weapon"] = weaponID
			for _, item := range r.Items {
				if item.ID == weaponID {
					for material, n := range item.Materials {
						a.Inventory[material] += n + 8
					}
				}
			}
			if skill.Kind == "antidote" {
				a.Poison = 3
			}
			fixture := r
			fixture.Opponents = []AdventureOpponent{{ID: "fantasy-qualified-method-test", Name: "Synthetic consenting opponent", Location: "渡鸦桥镇", Scale: "person", Rank: 1, Health: 10000, Attack: 1, Defense: 1, Energy: 100, Lethal: false}}
			next, _, err := ResolveAdventureAction(fixture, a, AdventureAction{Action: "battle_start", OpponentID: fixture.Opponents[0].ID}, "method-start")
			if err != nil {
				t.Fatal(err)
			}
			before := next
			next, _, err = ResolveAdventureAction(fixture, next, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: skill.ID}, "method-use")
			if err != nil {
				t.Fatal("learned method has no usable formal consumer", err)
			}
			expected := before.Energy - skill.Cost
			if skill.Kind == "energy" {
				expected = min(before.MaxEnergy, expected+skill.Power)
			}
			if next.Energy != expected {
				t.Fatalf("effort cost/recovery mismatch: before=%d got=%d expected=%d", before.Energy, next.Energy, expected)
			}
			for item, n := range skill.Materials {
				if before.Inventory[item]-next.Inventory[item] < n {
					t.Fatal("method did not consume its material", item, n)
				}
			}
			if skill.RequiredTool != "" && next.Inventory[skill.RequiredTool] != 1 {
				t.Fatal("reusable tool was consumed")
			}
			if skill.Kind == "heal" && next.Health <= before.Health {
				t.Fatal("heal had no real health effect")
			}
			if skill.Kind == "antidote" && next.Poison >= before.Poison {
				t.Fatal("antidote did not remove poison")
			}
			if (skill.Kind == "attack" || skill.Kind == "poison") && next.Battle.Health >= before.Battle.Health {
				t.Fatal("offensive method had no real effect")
			}
		})
	}
}
