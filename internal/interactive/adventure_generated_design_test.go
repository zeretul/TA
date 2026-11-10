package interactive

import (
	"encoding/json"
	"reflect"
	"testing"

	"ta/internal/cultivation"
)

func generatedPower(value int) *int { return &value }

func TestGeneratedCraftDesignProducesStrongerOwnedWeapon(t *testing.T) {
	for _, family := range []string{"space", "wuxia"} {
		t.Run(family, func(t *testing.T) {
			r := adventureTestRules()
			if family == "wuxia" {
				r = generatedWuxiaRules()
			}
			a := NewAdventureActor(r)
			a.Rank = 2
			a.Skills["engineering"] = 2
			a.Inventory["scrap"] = 30
			d := AdventureGeneratedDefinition{Category: "craft", TemplateID: "make-blade", Name: "叠锻法", Source: "研究已有锻造法", Description: "反复锻打以强化刃口。", QualityGrade: cultivation.QualityBlue, Design: &AdventureGeneratedDesign{Power: generatedPower(20), ProductName: "叠锻刀"}}
			next, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &d}, "register")
			if err != nil {
				t.Fatal(err)
			}
			if out.DurationSeconds != 0 || next.Inventory["scrap"] != 30 {
				t.Fatal("registration spent resources or manufactured an item")
			}
			saved := next.GeneratedDefinitions[0]
			productID := adventureGeneratedProductID(saved)
			recipeID, _ := d.id()
			if next.Inventory[productID] != 0 {
				t.Fatal("method registration granted stock")
			}
			before, _ := json.Marshal(next)
			partial, progress, err := ResolveAdventureAction(r, next, AdventureAction{Action: "craft", RecipeID: recipeID, Hours: 3}, "short")
			if err != nil || partial.Inventory[productID] != 0 || partial.Crafting[recipeID].WorkedHours != 3 || progress.DurationSeconds != 10800 {
				t.Fatal("partial generated craft must reserve materials and retain progress")
			}
			if b, _ := json.Marshal(next); string(b) != string(before) {
				t.Fatal("refusal changed caller")
			}
			next, out, err = ResolveAdventureAction(r, partial, AdventureAction{Action: "craft", RecipeID: recipeID, Hours: 1}, "make")
			if err != nil {
				t.Fatal(err)
			}
			if next.Inventory["scrap"] != 18 || next.Inventory[productID] != 1 || out.DurationSeconds != 3600 {
				t.Fatalf("real craft cost/output: %+v %+v", next.Inventory, out)
			}
			catalog, err := adventureRulesForActor(r, next)
			if err != nil {
				t.Fatal(err)
			}
			var product AdventureItem
			for _, i := range catalog.Items {
				if i.ID == productID {
					product = i
				}
			}
			if product.Power != 20 || product.Name != "叠锻刀" || product.QualityGrade != cultivation.QualityBlue || product.Description != d.Description {
				t.Fatalf("derived product missing: %+v", product)
			}
			if AdventureItemTemplateID(next, productID) != "blade" || AdventureOwnedItemCount(next, "blade") != 1 {
				t.Fatal("crafted product lost original physical capabilities")
			}
			next, _, err = ResolveAdventureAction(r, next, AdventureAction{Action: "equip", ItemID: productID}, "equip")
			if err != nil {
				t.Fatal(err)
			}
			next, _, err = ResolveAdventureAction(r, next, AdventureAction{Action: "battle_start", OpponentID: "rival"}, "start")
			if err != nil {
				t.Fatal(err)
			}
			beforeHealth := next.Battle.Health
			next, _, err = ResolveAdventureAction(r, next, AdventureAction{Action: "battle_round", Choice: "attack"}, "strike")
			if err != nil {
				t.Fatal(err)
			}
			if beforeHealth-next.Battle.Health != 34 {
				t.Fatalf("strong weapon not consumed: dealt %d", beforeHealth-next.Battle.Health)
			}
		})
	}
}

func generatedWuxiaRules() AdventureRules {
	r := adventureTestRules()
	r.Family = "wuxia"
	r.Skills = []AdventureSkill{{ID: "strike", Name: "基础拳", Kind: "attack", Scale: "person", Power: 15, Cost: 10, RequiredRank: 1, TrainingHours: 2}, {ID: "engineering", Name: "锻造", Kind: "repair", Scale: "person", Power: 10, RequiredRank: 1, TrainingHours: 2}}
	r.StartingSkills = []string{"strike", "engineering"}
	r.StartingItems = map[string]int{"scrap": 10}
	r.Items = r.Items[:3]
	r.Markets[0].Items = []string{"scrap", "blade"}
	r.Opponents = r.Opponents[:1]
	return r
}

func TestGeneratedEquipmentDesignChargesAndRetriesOnce(t *testing.T) {
	r := adventureTestRules()
	a := NewAdventureActor(r)
	a.Skills["engineering"] = 2
	a.Inventory["blade"] = 1
	a.Inventory["scrap"] = 20
	d := AdventureGeneratedDefinition{Category: "equipment", TemplateID: "blade", Name: "重锻刀", Source: "以随身刀改制", Design: &AdventureGeneratedDesign{Power: generatedPower(20), MethodID: "make-blade"}}
	in := AdventureAction{Action: "generate", Generate: &d, Hours: 4}
	next, out, err := ResolveAdventureAction(r, a, in, "adapt")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := d.id()
	if next.Inventory["blade"] != 0 || next.Inventory[id] != 1 || next.Inventory["scrap"] != 8 || out.DurationSeconds != 14400 {
		t.Fatalf("adaptation costs: %+v %+v", next.Inventory, out)
	}
	retry, retryOut, err := ResolveAdventureAction(r, next, in, "retry")
	if err != nil || !reflect.DeepEqual(retry, next) || retryOut.DurationSeconds != 0 {
		t.Fatalf("retry spent again: %+v %v", retryOut, err)
	}
	changed := d
	design := *d.Design
	design.Power = generatedPower(30)
	changed.Design = &design
	if _, _, err = ResolveAdventureAction(r, next, AdventureAction{Action: "generate", Generate: &changed, Hours: 6}, "replace"); err == nil {
		t.Fatal("changed existing mechanics under the same identity")
	}
	poor := a
	poor.Inventory = map[string]int{"blade": 1, "scrap": 1}
	if _, _, err = ResolveAdventureAction(r, poor, in, "poor"); err == nil {
		t.Fatal("crafted without materials")
	}
	if poor.Inventory["blade"] != 1 || poor.Inventory["scrap"] != 1 {
		t.Fatal("refusal consumed source")
	}
}

func TestGeneratedCompositeSkillHasRealEffectsAndCost(t *testing.T) {
	r := adventureTestRules()
	r.Skills = append(r.Skills, AdventureSkill{ID: "defend", Name: "防御", Kind: "guard", Scale: "person", Power: 8, Cost: 5, RequiredRank: 1, TrainingHours: 2})
	r.StartingSkills = append(r.StartingSkills, "defend")
	a := NewAdventureActor(r)
	a.Rank = 2
	a.Skills["strike"] = 2
	d := AdventureGeneratedDefinition{Category: "skill", TemplateID: "strike", Name: "攻守一式", Source: "把已学拳法与防御结合", Design: &AdventureGeneratedDesign{Effects: []AdventureEffect{{Kind: "attack", Power: 12}, {Kind: "guard", Power: 6}}, Foundations: []string{"defend"}}}
	next, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &d, Hours: 4}, "combine")
	if err != nil {
		t.Fatal(err)
	}
	if out.DurationSeconds != 14400 {
		t.Fatal("study time did not settle")
	}
	id, _ := d.id()
	catalog, _ := adventureRulesForActor(r, next)
	var skill AdventureSkill
	for _, s := range catalog.Skills {
		if s.ID == id {
			skill = s
		}
	}
	if skill.Cost != 24 || skill.RequiredRank != 2 || len(skill.Effects) != 2 {
		t.Fatalf("wrong composed budget: %+v", skill)
	}
	next, _, err = ResolveAdventureAction(r, next, AdventureAction{Action: "battle_start", OpponentID: "rival"}, "start")
	if err != nil {
		t.Fatal(err)
	}
	health, energy, enemy := next.Health, next.Energy, next.Battle.Health
	next, _, err = ResolveAdventureAction(r, next, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: id}, "round")
	if err != nil {
		t.Fatal(err)
	}
	if next.Energy != energy-24 || next.Battle.Health >= enemy || health-next.Health > 2 {
		t.Fatalf("effect/cost consumers: %+v", next)
	}
	d.Design.Cost = generatedPower(0)
	if _, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &d, Hours: 4}, "free"); err == nil {
		t.Fatal("accepted unpriced extra effects")
	}
	d.Design.Cost = nil
	d.Design.Effects = []AdventureEffect{{Kind: "revive", Power: 50}}
	if _, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &d, Hours: 4}, "unsupported"); err == nil {
		t.Fatal("accepted unimplemented composition")
	}
}

func TestGeneratedDesignStoryClockForkAndColdReplay(t *testing.T) {
	r := adventureTestRules()
	store, storyID := adventureTestStoryWithRules(t, r)
	defer store.Close()
	before, _ := store.Snapshot(storyID, "main")
	branch, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: before.CurrentTurn.ID, Title: "生成前"})
	if err != nil {
		t.Fatal(err)
	}
	d := AdventureGeneratedDefinition{Category: "skill", TemplateID: "strike", Name: "省力拳", Source: "对已学拳法调整发力", Design: &AdventureGeneratedDesign{Power: generatedPower(12), Cost: generatedPower(8)}}
	_, _, err = store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{BranchID: "main", User: "投入两小时推演省力拳", Narrative: "你练成更省力的一式。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "投入两小时推演省力拳", Adventure: &AdventureAction{Action: "generate", Generate: &d, Hours: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.AdventureWindow(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	old, err := store.AdventureWindow(storyID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := d.id()
	if current.Actor.Skills[id] != 1 || old.Actor.Skills[id] != 0 {
		t.Fatal("branch isolation failed")
	}
	snap, _ := store.Snapshot(storyID, "main")
	if SnapshotClockSeconds(snap)-SnapshotClockSeconds(before) != 7200 {
		t.Fatal("generated study did not advance the formal clock by two hours")
	}
	if reflect.DeepEqual(snap.State, before.State) {
		t.Fatal("formal generation did not commit")
	}
	root := store.Root()
	store.Close()
	reopened := NewStore(root)
	defer reopened.Close()
	cold, err := reopened.AdventureWindow(storyID, "main")
	if err != nil || !reflect.DeepEqual(current, cold) {
		t.Fatalf("cold replay changed mechanics: %v", err)
	}
}
