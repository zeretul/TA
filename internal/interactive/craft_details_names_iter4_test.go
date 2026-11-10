package interactive

import (
	"reflect"
	"ta/internal/cultivation"
	"testing"
)

func TestLearnedCultivationCraftDetailsIncludeNonConsumedToolNames(t *testing.T) {
	method, _ := cultivation.NewCraftRegistryWithBuiltins().Method("craft.talisman.ward")
	method.ID, method.Source, method.FailureCost = "world.tool_details", "符师传授", "consume_inputs"
	method.RequiredTools = []string{"implement.lower.common"}
	store, storyID, _ := talismanStory(t, method)
	input := DecodeInteractiveTurnSubmissionInput(`{"skills":{"skills":[{"def_id":"world.tool_details","version":1,"source":"符师当面传授"}]}}`)
	talismanTurn(t, store, storyID, "main", "学习符法", &TurnResult{Skills: input.Skills})
	before, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	details, err := store.CultivationEntryDetails(storyID, "main", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(details.CraftMethods) != 1 {
		t.Fatalf("methods = %+v", details.CraftMethods)
	}
	entry := details.CraftMethods[0]
	tool, _ := cultivation.NewItemCatalogWithFirstVersion().Definition("implement.lower.common")
	if entry.MaterialNames[tool.ID] != tool.NameKey {
		t.Fatalf("non-consumed tool name missing: %+v", entry.MaterialNames)
	}
	if len(entry.MaterialNames) != len(method.Inputs)+len(method.Outputs)+1 {
		t.Fatalf("unreferenced catalog names leaked: %+v", entry.MaterialNames)
	}
	after, err := store.Snapshot(storyID, "main")
	if err != nil || !reflect.DeepEqual(before.State, after.State) || before.CurrentTurn.ID != after.CurrentTurn.ID {
		t.Fatal("details changed canonical state", err)
	}
}

func TestPartialSkillDetailsShowStudyWithoutGrantingCasting(t *testing.T) {
	store, storyID, _ := talismanStory(t)
	talismanTurn(t, store, storyID, "main", "研习松针火一分钟", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{TemplateID: "fire_bolt", Name: "松针火", Source: "合成测试中的符师示范", Minutes: 1}}}})
	view, err := store.CultivationEntryDetails(storyID, "main", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Skills) != 1 {
		t.Fatalf("partial study disappeared from details: %+v", view.Skills)
	}
	skill := view.Skills[0]
	if skill.Name != "松针火" {
		t.Fatalf("generated identity leaked as a display name: %s", skill.Name)
	}
	if skill.Mastered || skill.Castable || skill.LearningMinutes != 1 || skill.RequiredLearningMinutes <= 1 || skill.ReasonKey != "entryDetails.learningIncomplete" {
		t.Fatalf("incorrect study projection: %+v", skill)
	}
	talismanTurn(t, store, storyID, "main", "继续研习松针火", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: skill.ID, Source: "持续研习", Minutes: 10080}}}})
	view, err = store.CultivationEntryDetails(storyID, "main", "zh-CN")
	if err != nil || len(view.Skills) != 1 || !view.Skills[0].Mastered || view.Skills[0].ReasonKey == "entryDetails.learningIncomplete" {
		t.Fatalf("completed study stayed blocked: %+v %v", view.Skills, err)
	}
}
