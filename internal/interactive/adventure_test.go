package interactive

import (
	"encoding/json"
	"reflect"
	interactivestate "ta/internal/interactive/state"
	"testing"
)

func adventureTestRules() AdventureRules {
	return AdventureRules{Family: "space", Currency: "伊纳", StartingCurrency: 100, StartingSkills: []string{"strike", "repair", "engineering", "ship-gun"}, StartingItems: map[string]int{"scrap": 10, "book": 1, "ship": 1}, RepairMaterialID: "scrap",
		Skills:    []AdventureSkill{{ID: "strike", Name: "格斗", Kind: "attack", Scale: "person", Power: 15, Cost: 10, RequiredRank: 1, TrainingHours: 2}, {ID: "advanced", Name: "进阶格斗", Kind: "attack", Scale: "person", Power: 25, Cost: 10, RequiredRank: 1, TrainingHours: 2, Prerequisite: "strike", RequiredItem: "book"}, {ID: "repair", Name: "修理", Kind: "repair", Scale: "ship", Power: 10, RequiredRank: 1, TrainingHours: 2}, {ID: "engineering", Name: "制造", Kind: "engineering", Scale: "machine", Power: 10, RequiredRank: 1, TrainingHours: 2}, {ID: "ship-gun", Name: "舰炮", Kind: "attack", Scale: "ship", Power: 10, Cost: 5, RequiredRank: 1, TrainingHours: 2}},
		Items:     []AdventureItem{{ID: "scrap", Name: "零件", Kind: "material", Scale: "person", Price: 2, Acquisition: "market"}, {ID: "book", Name: "手册", Kind: "material", Scale: "person", Acquisition: "reward", Unique: true}, {ID: "blade", Name: "刀", Kind: "weapon", Scale: "person", Power: 10, Price: 20, Acquisition: "market"}, {ID: "ship", Name: "舰船", Kind: "ship", Scale: "ship", Durability: 100, Power: 10, Acquisition: "reward", Unique: true}, {ID: "chip", Name: "芯片", Kind: "tool", Scale: "ship", Effect: "compute", Multiplier: 4, Acquisition: "reward", Unique: true}, {ID: "amulet", Name: "护符", Kind: "tool", Scale: "person", Effect: "revive", Chance: 100, Acquisition: "reward", Unique: true}, {ID: "antidote", Name: "解毒剂", Kind: "consumable", Scale: "person", Effect: "antidote", Acquisition: "market", Price: 5}},
		Markets:   []AdventureMarket{{ID: "dock", Name: "船坞", Location: "船坞", Items: []string{"scrap", "blade", "antidote"}}},
		Recipes:   []AdventureRecipe{{ID: "make-blade", Name: "制作刀", SkillID: "engineering", SkillRank: 1, Hours: 2, Materials: map[string]int{"scrap": 3}, OutputID: "blade", OutputCount: 1}},
		Opponents: []AdventureOpponent{{ID: "rival", Location: "船坞", Name: "对手", Scale: "person", Health: 60, Attack: 10, Defense: 2, RewardExperience: 30, RewardCurrency: 10}, {ID: "frigate", Location: "船坞", Name: "敌舰", Scale: "ship", Health: 200, Attack: 25, Defense: 5}, {ID: "killer", Location: "船坞", Name: "强敌", Scale: "person", Health: 500, Attack: 1000}}}
}

func TestAdventureTrainingCraftTradeAtomicity(t *testing.T) {
	r := adventureTestRules()
	a := NewAdventureActor(r)
	next, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "learn", SkillID: "advanced", Hours: 2}, "learn")
	if err != nil {
		t.Fatal(err)
	}
	if next.Skills["advanced"] != 1 || out.DurationSeconds != 7200 || a.Skills["advanced"] != 0 {
		t.Fatalf("learning was not isolated: %+v", next)
	}
	delete(a.Inventory, "book")
	_, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "learn", SkillID: "advanced", Hours: 2}, "forged")
	if err == nil {
		t.Fatal("learned without a committed source")
	}
	a = next
	before, _ := json.Marshal(a)
	_, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "craft", RecipeID: "make-blade", Hours: 2, Quantity: 5}, "short")
	after, _ := json.Marshal(a)
	if err == nil || string(before) != string(after) {
		t.Fatal("refused craft mutated its input")
	}
	a, out, err = ResolveAdventureAction(r, a, AdventureAction{Action: "craft", RecipeID: "make-blade", Hours: 2}, "craft")
	if err != nil || a.Inventory["blade"] != 1 || a.Inventory["scrap"] != 7 || out.DurationSeconds != 7200 {
		t.Fatalf("craft: %+v %v", a, err)
	}
	a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "equip", ItemID: "blade"}, "equip")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "sell", ItemID: "blade", MarketID: "dock"}, "sell-equipped")
	if err == nil {
		t.Fatal("sold equipped weapon")
	}
	a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "unequip", ItemID: "blade"}, "unequip")
	if err != nil {
		t.Fatal(err)
	}
	a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "sell", ItemID: "blade", MarketID: "dock"}, "sell")
	if err != nil || a.Currency != 110 {
		t.Fatalf("sale: %+v %v", a, err)
	}
	_, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "buy", ItemID: "ship", MarketID: "dock"}, "unique")
	if err == nil {
		t.Fatal("bought unique reward")
	}
}

func TestAdventureAssetScaleRepairAndCompute(t *testing.T) {
	r := adventureTestRules()
	a := NewAdventureActor(r)
	a.Inventory["chip"] = 1
	a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "deploy", ItemID: "ship"}, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "equip", ItemID: "chip"}, "chip")
	if err != nil {
		t.Fatal(err)
	}
	a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "frigate"}, "start")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "strike"}, "wrong-scale")
	if err == nil {
		t.Fatal("person skill affected ship combat")
	}
	before := a.Battle.Health
	a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "ship-gun"}, "round")
	if err != nil {
		t.Fatal(err)
	}
	if before-a.Battle.Health != 58 || a.Assets["ship"].Fuel != 99 || a.Health != 100 {
		t.Fatalf("ship effect, fuel or body isolation failed: %+v", a)
	}
	a.Battle.Status = "escaped"
	asset := a.Assets["ship"]
	asset.Health = 30
	a.Assets["ship"] = asset
	a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "repair", TargetID: "ship", SkillID: "repair", Hours: 2}, "repair")
	if err != nil || a.Inventory["scrap"] != 8 || a.Assets["ship"].Health != 50 {
		t.Fatalf("repair: %+v %v", a, err)
	}
}

func TestAdventureSurvivalPoisonAndCatalogRejection(t *testing.T) {
	r := adventureTestRules()
	a := NewAdventureActor(r)
	a.Inventory["amulet"] = 1
	a.Inventory["antidote"] = 1
	a.Poison = 10
	a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "use", ItemID: "antidote"}, "cure")
	if err != nil || a.Poison != 0 || a.Health != 100 {
		t.Fatalf("antidote did not cure only poison: %+v %v", a, err)
	}
	a, _, _ = ResolveAdventureAction(r, a, AdventureAction{Action: "equip", ItemID: "amulet"}, "equip")
	a, _, _ = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "killer"}, "fight")
	a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "survive")
	if err != nil || a.Health != 25 || a.Battle.Status != "escaped" || a.Inventory["amulet"] != 0 || a.Equipment["person:tool"] != "" {
		t.Fatalf("survival: %+v %v", a, err)
	}
	r.Skills[0].Prerequisite = "advanced"
	if r.Validate() == nil {
		t.Fatal("accepted prerequisite cycle")
	}
}

func adventureTestStory(t *testing.T) (*Store, string) {
	t.Helper()
	return adventureTestStoryWithRules(t, adventureTestRules())
}

func adventureTestStoryWithRules(t *testing.T, r AdventureRules) (*Store, string) {
	t.Helper()
	system := StoryDirectorActorStateSystem{WorldContent: r.world, AdventureRules: &r, Templates: []ActorStateTemplate{{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(r)}}}
	initial := map[string]any{}
	for _, field := range AdventureStateFields(r) {
		initial[field.Name] = field.Default
	}
	system.Templates = append(system.Templates, ActorStateTemplate{ID: DefaultStoryContextActorID, Name: "场景", Fields: []ActorStateField{{Name: storySceneLocationField, Type: "string", Default: "船坞"}}})
	if len(r.NPCs) > 0 {
		system.Templates = append(system.Templates, ActorStateTemplate{ID: AdventureNPCTemplateID, Name: "人物", Fields: AdventureNPCStateFields(r)})
	}
	system.InitialActors = []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: initial}}
	system.InitialActors = append(system.InitialActors, ActorStateInitialActor{ID: DefaultStoryContextActorID, Name: "场景", TemplateID: DefaultStoryContextActorID, State: map[string]any{storySceneLocationField: "船坞"}})
	s := NewStore(t.TempDir())
	story, err := s.CreateStory(CreateStoryRequest{Title: "星海测试", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "开始", Narrative: "整备完毕。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	return s, story.ID
}

func TestAdventureStoryClockBattleRetryAndReload(t *testing.T) {
	s, id := adventureTestStory(t)
	_, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "学习两小时", Narrative: "练习格斗。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "学习两小时", Adventure: &AdventureAction{Action: "train", SkillID: "strike", Hours: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "迎战", Narrative: "对手上前。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "迎战", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "rival"}}}})
	if err != nil {
		t.Fatal(err)
	}
	window, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	in := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "strike"}
	first, err := s.CommitAdventureAction(id, "main", "round-1", window.Head, in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CommitAdventureAction(id, "main", "round-1", window.Head, in)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("idempotency: %v", err)
	}
	if _, err := s.CommitAdventureAction(id, "main", "round-2", window.Head, in); err == nil {
		t.Fatal("stale window committed")
	}
	reopened := NewStore(s.Root())
	restored, err := reopened.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Actor, restored.Actor) {
		t.Fatal("cold replay changed battle")
	}
	snapshot, err := reopened.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Clock == nil || snapshot.Clock.Seconds != 7220 {
		t.Fatalf("clock was not persisted: %+v", snapshot.Clock)
	}
	fork, err := reopened.CreateBranch(id, CreateBranchRequest{ParentEventID: window.Head, Title: "交手前"})
	if err != nil {
		t.Fatal(err)
	}
	forkWindow, err := reopened.AdventureWindow(id, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if forkWindow.Actor.Battle.Round != 0 || forkWindow.Actor.Health != 100 {
		t.Fatalf("fork inherited later battle state: %+v", forkWindow.Actor)
	}
}

func TestAdventureProtectsOnlyOwnedStateAndFrozenSchema(t *testing.T) {
	s, id := adventureTestStory(t)
	snapshot, err := s.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	system := snapshot.ActorStateSchema.System
	for _, path := range []string{"/protagonist/历练状态", "/protagonist/历练状态/currency", "/protagonist/生命", "/protagonist/钱财"} {
		if _, err := CompileTurnStateUpdates(system, snapshot.State, []interactivestate.Update{{Op: interactivestate.Replace, Path: path, Value: 999}}, TurnStateUpdateCompileOptions{}); err == nil {
			t.Fatalf("accepted model write %s", path)
		}
	}
	_, _, err = ApplyActorStateSchemaAdaptation(system, StoryDirectorTRPGSystem{}, ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{Op: "fields", TemplateID: DefaultActorID, FieldOps: []ActorStateFieldSchemaOp{{Op: "remove", FieldID: AdventureStateField}}}}})
	if err == nil {
		t.Fatal("schema adaptation removed the canonical rules field")
	}
	general := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{ID: DefaultActorID, Fields: []ActorStateField{{Name: "钱财", Type: "number", Default: 0}}}}}
	state := map[string]any{"actors": map[string]any{DefaultActorID: map[string]any{"template_id": DefaultActorID, "state": map[string]any{"钱财": 0}}}}
	if _, err := CompileTurnStateUpdates(general, state, []interactivestate.Update{{Op: interactivestate.Replace, Path: "/protagonist/钱财", Value: 20}}, TurnStateUpdateCompileOptions{}); err != nil {
		t.Fatalf("adventure locked an unrelated world: %v", err)
	}
	r := adventureTestRules()
	frozen := FreezeActorStateSchema(StoryDirectorActorStateSystem{AdventureRules: &r}, false)
	r.Items[0].Price = 999
	if frozen.System.AdventureRules.Items[0].Price == 999 {
		t.Fatal("frozen catalog aliases the mutable author catalog")
	}
}

func TestAdventureSceneAndUniqueRewardGates(t *testing.T) {
	r := adventureTestRules()
	state := map[string]any{"actors": map[string]any{DefaultStoryContextActorID: map[string]any{"state": map[string]any{storySceneLocationField: "外太空"}}}}
	if validateAdventureScene(r, state, AdventureAction{Action: "buy", ItemID: "blade", MarketID: "dock"}) == nil {
		t.Fatal("bought from a remote market")
	}
	if validateAdventureScene(r, state, AdventureAction{Action: "battle_start", OpponentID: "rival"}) == nil {
		t.Fatal("summoned a remote opponent")
	}
	a := NewAdventureActor(r)
	state["actors"].(map[string]any)[DefaultActorID] = map[string]any{"state": map[string]any{AdventureStateField: a}}
	if _, err := AdventureRewardOps(state, StoryDirectorActorStateSystem{AdventureRules: &r}, 0, map[string]int{"book": 1}, 0, "repeat"); err == nil {
		t.Fatal("granted duplicate unique book")
	}
}

func TestAdventureDefeatRecoveryAndLethalTerminalAreDistinct(t *testing.T) {
	for _, lethal := range []bool{false, true} {
		t.Run(map[bool]string{false: "defeat", true: "terminal"}[lethal], func(t *testing.T) {
			r := adventureTestRules()
			r.Opponents[2].Lethal = lethal
			s, id := adventureTestStoryWithRules(t, r)
			_, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "迎战强敌", Narrative: "强敌拔刀。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "迎战强敌", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "killer"}}}})
			if err != nil {
				t.Fatal(err)
			}
			window, err := s.AdventureWindow(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			result, err := s.CommitAdventureAction(id, "main", "loss", window.Head, AdventureAction{Action: "battle_round", Choice: "attack"})
			if err != nil {
				t.Fatal(err)
			}
			if result.Actor.Health != 0 || result.Actor.Battle.Status != "defeat" || (result.TerminalOutcome != nil) != lethal {
				t.Fatalf("defeat classification: %+v", result)
			}
			s = NewStore(s.Root())
			_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "休养十小时", Narrative: "慢慢休养。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "休养十小时", Adventure: &AdventureAction{Action: "rest", Hours: 10}}}})
			if lethal && err == nil {
				t.Fatal("terminal branch resumed through rest")
			}
			if !lethal && err != nil {
				t.Fatalf("ordinary defeat softlocked: %v", err)
			}
			fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: window.Head, Title: "致命回合前"})
			if err != nil {
				t.Fatal(err)
			}
			before, err := s.AdventureWindow(id, fork.ID)
			if err != nil || before.Actor.Health != 100 || before.Actor.Battle.Round != 0 {
				t.Fatalf("branch did not restore before defeat: %+v %v", before, err)
			}
		})
	}
}

func TestAdventureAmmunitionGrenadeAndSkillMaterialAtomicity(t *testing.T) {
	r := adventureTestRules()
	r.Items = append(r.Items, AdventureItem{ID: "grenade", Name: "Grenade", Kind: "consumable", Scale: "person", Effect: "attack", Power: 41, Acquisition: "market", Price: 20})
	for i := range r.Items {
		if r.Items[i].ID == "blade" {
			r.Items[i].Materials = map[string]int{"scrap": 2}
		}
	}
	r.Skills[0].Materials = map[string]int{"scrap": 1}
	a := NewAdventureActor(r)
	a.Inventory["blade"] = 1
	a.Inventory["grenade"] = 1
	a.Equipment["person:weapon"] = "blade"
	a.Inventory["scrap"] = 2
	a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "rival"}, "start")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(a)
	_, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "strike"}, "short-ammo")
	unchanged, _ := json.Marshal(a)
	if err == nil || string(before) != string(unchanged) {
		t.Fatal("partial material consumption on refused skill")
	}
	next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "ammo")
	if err != nil || next.Inventory["scrap"] != 0 {
		t.Fatalf("ammunition not consumed: %v", err)
	}
	_, _, err = ResolveAdventureAction(r, next, AdventureAction{Action: "battle_round", Choice: "attack"}, "empty")
	if err == nil {
		t.Fatal("empty firearm attacked")
	}
	next, _, err = ResolveAdventureAction(r, next, AdventureAction{Action: "battle_round", Choice: "item", ItemID: "grenade"}, "grenade")
	if err != nil || next.Inventory["grenade"] != 0 || next.Battle.Status != "victory" {
		t.Fatalf("grenade did not resolve once: %+v %v", next, err)
	}
}

func TestAuthoredSpaceAssetsRequireSpecificOperatorSkills(t *testing.T) {
	r := authoredQuestRules(t, "xinghai-689")
	expected := map[string]string{"xinghai-i09": "xinghai-s03", "xinghai-i10": "xinghai-s26", "xinghai-frigate": "xinghai-s32", "xinghai-drone": "xinghai-s07"}
	for _, item := range r.Items {
		skill, ok := expected[item.ID]
		if !ok {
			continue
		}
		if item.RequiredSkill != skill {
			t.Fatalf("%s operator qualification = %q, want %q", item.ID, item.RequiredSkill, skill)
		}
		a := NewAdventureActor(r)
		a.Inventory[item.ID] = 1
		a.Skills = map[string]int{}
		if _, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "deploy", ItemID: item.ID}, "unqualified"); err == nil {
			t.Fatalf("unqualified owner deployed %s", item.ID)
		}
		a.Skills[skill] = 1
		for _, definition := range r.Skills {
			if definition.ID == skill {
				a.Rank = definition.RequiredRank
			}
		}
		deployed, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "deploy", ItemID: item.ID}, "qualified")
		if err != nil {
			t.Fatal(err)
		}
		if deployed.Equipment[item.Scale+":asset"] != item.ID {
			t.Fatal("qualified asset not deployed")
		}
	}
}
