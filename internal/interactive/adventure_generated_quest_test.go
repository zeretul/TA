package interactive

import (
	"encoding/json"
	"reflect"
	"testing"

	"ta/internal/quest"
)

// These are isolated rule fixtures, not AI playthroughs or earned resources.
func TestGeneratedAdventureQuestConsumesRealItemsAndDeployedAssets(t *testing.T) {
	r := adventureTestRules()
	r.Items = append(r.Items,
		AdventureItem{ID: "workshop", Name: "Workshop", Kind: "machine", Scale: "machine", Durability: 100, Acquisition: "craft"},
		AdventureItem{ID: "permit", Name: "Permit", Kind: "tool", Scale: "person", Acquisition: "reward"})
	r.StartingItems["workshop"], r.StartingItems["permit"], r.StartingItems["antidote"] = 1, 1, 1
	a := NewAdventureActor(r)
	generated := map[string]string{}
	for _, template := range []string{"workshop", "permit", "antidote"} {
		d := AdventureGeneratedDefinition{Category: "equipment", TemplateID: template, Name: "Adapted " + template, Source: "Adaptation of a held fixture object"}
		var err error
		a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &d}, "adapt-"+template)
		if err != nil {
			t.Fatal(err)
		}
		generated[template], _ = d.id()
	}
	a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "deploy", ItemID: generated["workshop"]}, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	state := map[string]any{"actors": map[string]any{DefaultActorID: map[string]any{"state": map[string]any{AdventureStateField: a}}}}
	p := &storyBranchProjection{State: state}
	system := StoryDirectorActorStateSystem{AdventureRules: &r}
	d := quest.Definition{ID: "fixture", Version: "1", Objectives: []quest.Objective{{ID: "repair"}}, Choices: []quest.Choice{{ID: "prepare", ObjectiveID: "repair", RequiredAsset: "workshop", RequiredItems: map[string]int{"permit": 1}, ItemCosts: map[string]int{"antidote": 1}, SkillID: "engineering", Difficulty: 1, DurationSeconds: 3600, EchoAfterSeconds: 3600}}}
	prior := quest.Instance{TaskID: d.ID, TaskVersion: d.Version, Status: quest.StatusAccepted}
	original, _ := json.Marshal(state)
	plan, err := planQuestChoice(p, system, d, prior, true, QuestRequest{ChoiceID: "prepare"})
	if err != nil {
		t.Fatal(err)
	}
	var committed AdventureActor
	for _, op := range plan.Ops {
		if op.FieldID == AdventureStateField {
			committed, err = AdventureActorFromState(map[string]any{"actors": map[string]any{DefaultActorID: map[string]any{"state": map[string]any{AdventureStateField: op.Value}}}}, DefaultActorID)
		}
	}
	if err != nil || committed.Inventory[generated["antidote"]] != 0 || committed.Inventory[generated["permit"]] != 1 || committed.Assets[generated["workshop"]] != a.Assets[generated["workshop"]] || plan.ClockAfter.Seconds-plan.ClockBefore.Seconds != 3600 {
		t.Fatalf("task did not consume the actual adaptation atomically: %v %+v", err, committed)
	}
	unchanged, _ := json.Marshal(p.State)
	if !reflect.DeepEqual(original, unchanged) || a.Inventory[generated["antidote"]] != 1 {
		t.Fatal("planning changed source branch")
	}
	// Holding the generated definition alone is never ownership or deployment.
	delete(a.Equipment, "machine:asset")
	state["actors"].(map[string]any)[DefaultActorID].(map[string]any)["state"].(map[string]any)[AdventureStateField] = a
	if _, err = planQuestChoice(p, system, d, prior, true, QuestRequest{ChoiceID: "prepare"}); err == nil {
		t.Fatal("task accepted a withdrawn generated asset")
	}
}

func TestGeneratedAdventureItemCostsRejectUnavailableCopies(t *testing.T) {
	r := adventureTestRules()
	r.StartingItems["blade"] = 1
	a := NewAdventureActor(r)
	d := AdventureGeneratedDefinition{Category: "equipment", TemplateID: "blade", Name: "Adapted blade", Source: "Held fixture blade"}
	a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &d}, "adapt")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := d.id()
	for _, costs := range []map[string]int{{"blade": 2}, {"blade": -1}, {"missing": 1}} {
		if _, err = adventureItemCosts(a, costs); err == nil {
			t.Fatalf("accepted invalid costs %+v", costs)
		}
	}
	a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "equip", ItemID: id}, "equip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adventureItemCosts(a, map[string]int{"blade": 1}); err == nil {
		t.Fatal("spent equipped adaptation as a task cost")
	}
	if _, err = adventureItemCosts(a, map[string]int{"ship": 1}); err != nil {
		t.Fatalf("undeployed owned hull should remain an ordinary held copy: %v", err)
	}
	a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "deploy", ItemID: "ship"}, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adventureItemCosts(a, map[string]int{"ship": 1}); err == nil {
		t.Fatal("spent a tracked operational asset")
	}
}
