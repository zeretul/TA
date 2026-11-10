package interactive

import (
	"reflect"
	"testing"
)

func TestGeneratedAdventureEquipmentRetainsTemplateConsumers(t *testing.T) {
	t.Run("sale does not authorize buying invented stock", func(t *testing.T) {
		r := adventureTestRules()
		a := NewAdventureActor(r)
		a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "buy", MarketID: "dock", ItemID: "blade"}, "buy")
		if err != nil {
			t.Fatal(err)
		}
		d := AdventureGeneratedDefinition{Category: "equipment", TemplateID: "blade", Name: "潮纹刀", Source: "以所购刀改制"}
		a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &d}, "adapt")
		if err != nil {
			t.Fatal(err)
		}
		id, _ := d.id()
		if _, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "buy", MarketID: "dock", ItemID: id}, "buy-copy"); err == nil {
			t.Fatal("market invented stock of a Story-local adaptation")
		}
		next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "sell", MarketID: "dock", ItemID: id}, "sell-adaptation")
		if err != nil || next.Inventory[id] != 0 || next.Currency != 90 {
			t.Fatalf("adapted weapon lost ordinary resale: %v %+v", err, next)
		}
	})
	t.Run("learning source", func(t *testing.T) {
		r := adventureTestRules()
		r.Items[1].Unique, r.Items[1].Kind = false, "tool"
		a := NewAdventureActor(r)
		d := AdventureGeneratedDefinition{Category: "equipment", TemplateID: "book", Name: "自注格斗手册", Source: "在实际持有的手册中加注"}
		a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &d}, "annotate")
		if err != nil {
			t.Fatal(err)
		}
		next, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "learn", SkillID: "advanced", Hours: 2}, "learn")
		if err != nil || next.Skills["advanced"] != 1 || out.DurationSeconds != 7200 {
			t.Fatalf("adapted source lost its learning consumer: %v", err)
		}
	})
	t.Run("combat tool and conditional effect", func(t *testing.T) {
		r := adventureTestRules()
		r.Items = append(r.Items, AdventureItem{ID: "needles", Name: "Needles", Kind: "tool", Scale: "person", Acquisition: "market", Price: 5})
		r.StartingItems["needles"] = 1
		r.Skills[0].RequiredTool = "needles"
		r.Skills[0].ConditionalEffects = []AdventureConditionalEffects{{RequiredItem: "needles", Effects: []AdventureEffect{{Kind: "energy", Power: 3}}}}
		a := NewAdventureActor(r)
		d := AdventureGeneratedDefinition{Category: "equipment", TemplateID: "needles", Name: "绣柄银针", Source: "改制手中银针"}
		a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &d}, "adapt")
		if err != nil {
			t.Fatal(err)
		}
		a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "rival"}, "start")
		if err != nil {
			t.Fatal(err)
		}
		next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "strike"}, "round")
		if err != nil || next.Energy != a.Energy-10+3 || next.Battle.Health >= a.Battle.Health {
			t.Fatalf("adapted tool lost battle effect or actual energy cost: %v %+v", err, next)
		}
	})
}

func TestGeneratedAdventureWithdrawnAssetPreservesWearAndWorkingIdentity(t *testing.T) {
	r := adventureTestRules()
	r.Items = append(r.Items, AdventureItem{ID: "workshop", Name: "Workshop", Kind: "machine", Scale: "machine", Durability: 100, Power: 10, Acquisition: "market", Price: 50, RequiredSkill: "engineering", SupportOnly: true})
	r.StartingItems["workshop"] = 1
	r.Recipes[0].RequiredAsset, r.Recipes[0].FuelCost = "workshop", 10
	a := NewAdventureActor(r)
	a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "deploy", ItemID: "workshop"}, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately damaged rule fixture: adaptation must not refill or heal it.
	worn := AdventureAsset{Health: 31, MaxHealth: 80, Shield: 2, Fuel: 11, PermanentDamage: 20, DisabledUntil: 100}
	a.Assets["workshop"] = worn
	a, _, err = ResolveAdventureActionAt(r, a, AdventureAction{Action: "withdraw", ItemID: "workshop"}, "withdraw", 50)
	if err != nil {
		t.Fatal(err)
	}
	s, storyID := adventureFixtureStoryWithActor(t, r, a, "船坞")
	defer s.Close()
	before, _ := s.AdventureWindow(storyID, "main")
	d := AdventureGeneratedDefinition{Category: "equipment", TemplateID: "workshop", Name: "潮纹作业台", Source: "改制已经撤回的作业台"}
	_, _, err = s.AppendTurnWithState(storyID, AppendTurnWithStateRequest{BranchID: "main", User: "改制作业台", Narrative: "你调整了作业台的外壳。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "改制作业台", Adventure: &AdventureAction{Action: "generate", Generate: &d}}}})
	if err != nil {
		t.Fatal(err)
	}
	window, err := s.AdventureWindow(storyID, "main")
	id, _ := d.id()
	if err != nil || window.Actor.Assets[id] != worn || window.Actor.Inventory[id] != 1 || window.Actor.Inventory["workshop"] != 0 {
		t.Fatalf("adaptation lost worn asset facts: %v %+v", err, window.Actor)
	}
	if _, exists := window.Actor.Assets["workshop"]; exists {
		t.Fatal("adaptation duplicated the worn asset")
	}
	a = window.Actor
	if _, _, err = ResolveAdventureActionAt(r, a, AdventureAction{Action: "deploy", ItemID: id}, "too-early", 50); err == nil {
		t.Fatal("adaptation cleared the engine lock")
	}
	a, _, err = ResolveAdventureActionAt(r, a, AdventureAction{Action: "deploy", ItemID: id}, "deploy-adapted", 100)
	if err != nil {
		t.Fatal(err)
	}
	beforeMaterials := a.Inventory["scrap"]
	crafted, out, err := ResolveAdventureActionAt(r, a, AdventureAction{Action: "craft", RecipeID: "make-blade", Hours: 2}, "craft-adapted", 100)
	if err != nil || crafted.Assets[id].Fuel != 1 || crafted.Assets[id].Health != worn.Health || crafted.Inventory["scrap"] != beforeMaterials-3 || crafted.Inventory["blade"] != 1 || out.DurationSeconds != 7200 {
		t.Fatalf("adapted working asset lost recipe consumer/costs: %v %+v", err, crafted)
	}
	retry, _, err := ResolveAdventureActionAt(r, crafted, AdventureAction{Action: "generate", Generate: &d}, "retry-adaptation", 100)
	if err != nil || !reflect.DeepEqual(crafted, retry) {
		t.Fatalf("retry refilled generated asset: %v", err)
	}
	root := s.Root()
	_ = s.Close()
	s = NewStore(root)
	defer s.Close()
	cold, err := s.AdventureWindow(storyID, "main")
	if err != nil || !reflect.DeepEqual(window.Actor, cold.Actor) {
		t.Fatalf("cold adaptation changed: %v", err)
	}
	branch, err := s.CreateBranch(storyID, CreateBranchRequest{ParentEventID: before.Head, Title: "Before adaptation"})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := s.AdventureWindow(storyID, branch.ID)
	if err != nil || fork.Actor.Assets["workshop"] != worn || fork.Actor.Inventory[id] != 0 {
		t.Fatalf("adaptation leaked to old branch: %v %+v", err, fork.Actor)
	}
}

func TestGeneratedWuxiaAntidoteKeepsItsFrozenFogTreatment(t *testing.T) {
	r := authoredAdventureRules(t, "lanchuan-qunxia")
	a := NewAdventureActor(r)
	a.Inventory["lanchuan-i21"] = 1 // Isolated rule fixture, not earned stock.
	d := AdventureGeneratedDefinition{Category: "equipment", TemplateID: "lanchuan-i21", Name: "随身辟瘴丸", Source: "调整已有药丸的携带包衣"}
	a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &d}, "adapt-antidote")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := d.id()
	setAdventureFogLocation(r, &a, "雾锁沟")
	a.Poison = 10
	got, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "use", ItemID: id}, "treat-fog")
	if err != nil || got.Inventory[id] != 0 || got.Poison != 0 || !got.FogTreated || adventureRushUnavailable(got) {
		t.Fatalf("adapted actual dose lost its registered treatment: %v %+v", err, got)
	}
	if _, _, err = ResolveAdventureAction(r, got, AdventureAction{Action: "use", ItemID: id}, "repeat-dose"); err == nil {
		t.Fatal("definition alone supplied another dose")
	}
}
