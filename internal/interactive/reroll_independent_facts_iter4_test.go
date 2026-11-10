package interactive

import (
	"reflect"
	"testing"
)

func TestIter4RerollRetainsIndependentLearningItemsAndTrade(t *testing.T) {
	store, id, root := talismanStory(t)
	initial, err := store.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := resolveTurnRulesWithSeed(id, "main", initialStoryState(), sampleTurnCheckRequest(), 7)
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "带上新剑买符纸，学画护符并研习温元诀一小时", Narrative: "合成自动规则回归：独立能力、物品与交易同时保存。", RuleResolution: &resolution, TurnResult: &TurnResult{Choices: testTurnChoices(),
		Skills:     &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: "craft.talisman.ward", Source: "符师正式传授"}}},
		Techniques: &TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: "method.warm_origin", Source: "师承完整注本", Minutes: 60}}},
		Items:      &TurnItemsInput{Acquire: []TurnItemAcquire{{TemplateID: "implement.lower.common", Name: "雨痕剑", Source: "铁匠交付"}}},
		Trade:      &TurnTradeInput{Merchant: "符铺", Buys: []TurnTradeBuy{{DefID: "material.talisman.paper", Name: "符纸", Quantity: 2}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	old, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: initial.CurrentTurn.ID, Title: "独立事实之前"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(root)
	defer store.Close()
	// The persisted TurnResult deliberately does not contain transient module inputs.
	cold, err := store.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if cold.CurrentTurn.TurnResult == nil || cold.CurrentTurn.TurnResult.Skills != nil || cold.CurrentTurn.TurnResult.Techniques != nil || cold.CurrentTurn.TurnResult.Items != nil || cold.CurrentTurn.TurnResult.Trade != nil {
		t.Fatal("fixture must reproduce canonical persisted input shape")
	}
	current := resolution.ID
	for attempt := 0; attempt < 2; attempt++ {
		next, err := store.RerollRuleResolution(id, current, RuleResolutionRerollRequest{BranchID: "main", TurnID: turn.ID})
		if err != nil {
			t.Fatal(err)
		}
		current = next.ID
		after, err := store.Snapshot(id, "main")
		if err != nil {
			t.Fatal(err)
		}
		for _, actorID := range []string{DefaultActorID, DefaultStoryContextActorID} {
			for _, field := range []string{xiuxianFieldLearnedSkills, xiuxianFieldTechniques, xiuxianFieldImportantItems, xiuxianFieldCurrency, xiuxianFieldSkillRegistry} {
				want := actorStateFieldValue(before.State, actorID, field)
				got := actorStateFieldValue(after.State, actorID, field)
				if !reflect.DeepEqual(want, got) {
					t.Errorf("reroll removed or repeated independent %s/%s: before=%+v after=%+v", actorID, field, want, got)
				}
			}
		}
		if SnapshotClockSeconds(after) != SnapshotClockSeconds(before) {
			t.Fatal("reroll charged study time again")
		}
	}
	sibling, err := store.Snapshot(id, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(actorStateFieldValue(sibling.State, DefaultActorID, xiuxianFieldLearnedSkills), actorStateFieldValue(before.State, DefaultActorID, xiuxianFieldLearnedSkills)) {
		t.Fatal("pre-fact branch unexpectedly inherited learned method")
	}
}
