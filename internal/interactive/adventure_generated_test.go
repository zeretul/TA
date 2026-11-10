package interactive

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestGeneratedAdventureContentKeepsCostsAndOwnedStock(t *testing.T) {
	r := adventureTestRules()
	r.NPCs = []AdventureNPC{{CharacterSourceKey: "fixture/mechanic", Actor: NewAdventureActor(r), Plan: AdventureNPCPlan{Action: "rest", RankLimit: 2}}}
	r.NPCs[0].Actor.Inventory = map[string]int{} // Unrelated NPC does not copy player-owned relics.
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	a := NewAdventureActor(r)
	// Solver fixture earns the ordinary blade through the real market consumer.
	a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "buy", MarketID: "dock", ItemID: "blade"}, "buy")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []AdventureGeneratedDefinition{
		{Category: "skill", TemplateID: "strike", Name: "回风拳", Source: "从已学格斗中领悟", Description: "拳势像风绕过船桅。"},
		{Category: "equipment", TemplateID: "blade", Name: "潮纹刀", Source: "以随身刀改制", Description: "刀柄添了止滑的潮纹。"},
		{Category: "craft", TemplateID: "make-blade", Name: "旧船锻刀法", Source: "船坞师傅传授"},
	} {
		before, _ := json.Marshal(a)
		next, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &d}, "generate-"+d.Category)
		if err != nil || out.DurationSeconds != 0 {
			t.Fatalf("generate: %+v %v", out, err)
		}
		if original, _ := json.Marshal(a); !reflect.DeepEqual(original, before) {
			t.Fatal("solver mutated original branch")
		}
		id, _ := d.id()
		catalog, err := adventureRulesForActor(r, next)
		if err != nil {
			t.Fatal(err)
		}
		switch d.Category {
		case "skill":
			if next.Skills[id] != 1 {
				t.Fatal("generated skill was not learned")
			}
		case "equipment":
			if next.Inventory["blade"] != 0 || next.Inventory[id] != 1 {
				t.Fatal("adaptation duplicated held stock")
			}
			next, _, err = ResolveAdventureAction(catalog, next, AdventureAction{Action: "equip", ItemID: id}, "equip-new")
			if err != nil {
				t.Fatal(err)
			}
		case "craft":
			prevMaterials := next.Inventory["scrap"]
			next, _, err = ResolveAdventureAction(catalog, next, AdventureAction{Action: "craft", RecipeID: id, Hours: 2}, "craft-new")
			if err != nil || next.Inventory["scrap"] >= prevMaterials || next.Inventory["blade"] < 1 {
				t.Fatalf("generated recipe did not settle actual materials: %v", err)
			}
		}
		// Re-proposing the same definition never re-grants or refills it.
		retry, _, err := ResolveAdventureAction(r, next, AdventureAction{Action: "generate", Generate: &d}, "retry")
		if err != nil || !reflect.DeepEqual(retry, next) {
			t.Fatalf("generation retry changed state: %v", err)
		}
		a = next
	}
	bad := AdventureGeneratedDefinition{Category: "equipment", TemplateID: "ship", Name: "第二艘船", Source: "空想"}
	if _, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &bad}, "bad"); err == nil {
		t.Fatal("unique ship copied")
	}
}

func TestGeneratedAdventureStoryCommitAndColdReplay(t *testing.T) {
	r := adventureTestRules()
	store, id := adventureTestStoryWithRules(t, r)
	defer store.Close()
	before, _ := store.Snapshot(id, "main")
	branch, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: before.CurrentTurn.ID, Title: "旧拳法"})
	if err != nil {
		t.Fatal(err)
	}
	d := AdventureGeneratedDefinition{Category: "skill", TemplateID: "strike", Name: "船桅拳", Source: "从格斗的基础中自创", Description: "顺着船桅摇动的节奏出拳。"}
	turn, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "自创一式船桅拳", Narrative: "你在甲板上试出一种新的发力。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "自创一式船桅拳", Adventure: &AdventureAction{Action: "generate", Generate: &d}}}})
	if err != nil {
		t.Fatal(err)
	}
	window, err := store.AdventureWindow(id, "main")
	newID, _ := d.id()
	if err != nil || window.Actor.Skills[newID] != 1 {
		t.Fatalf("generated skill unavailable after formal commit: %v", err)
	}
	old, err := store.AdventureWindow(id, branch.ID)
	if err != nil || old.Actor.Skills[newID] != 0 {
		t.Fatal("generated skill leaked to pre-generation branch")
	}
	state, _ := store.Snapshot(id, "main")
	jobs := GeneratedDefinitionArtJobs(state.State, "ink", state.ActorStateSchema.System)
	if len(jobs) != 1 || jobs[0].DefinitionID != newID {
		t.Fatalf("generated adventure art not discovered: %+v", jobs)
	}
	if queued, err := store.TransitionDefinitionArt(id, "main", turn.ID, jobs[0], "", "queued", nil); err != nil || !queued {
		t.Fatalf("generated adventure art admission: %v %v", queued, err)
	}
	root := store.Root()
	_ = store.Close()
	reopened := NewStore(root)
	defer reopened.Close()
	cold, err := reopened.AdventureWindow(id, "main")
	if err != nil || !reflect.DeepEqual(window.Actor, cold.Actor) || !reflect.DeepEqual(window.Rules, cold.Rules) {
		t.Fatalf("generated adventure cold replay changed: %v", err)
	}
}
