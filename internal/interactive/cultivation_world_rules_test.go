package interactive

import (
	"testing"

	"ta/internal/cultivation"
)

func TestCultivationWorldRulesDriveSavedStateTrainingAndTrade(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	profiles := []*cultivation.Rules{cultivation.ReferenceRules(), cultivation.ReferenceRules()}
	row := profiles[1].Stages["qi_1"]
	row.Base.HealthMax *= 2
	row.Base.Attack *= 3
	row.MonthlyProgress *= 2
	profiles[1].Stages["qi_1"] = row
	profiles[1].Economy.MerchantCash = 1234
	for i := range profiles[1].Items {
		if profiles[1].Items[i].ID == "material.ore.iron" {
			profiles[1].Items[i].BasePrice *= 3
		}
	}
	var ids []string
	for i, name := range []string{"cultivation-a", "cultivation-b"} {
		system := combatTestSystem()
		system.CultivationRules = profiles[i]
		system.WorldContent = &WorldContent{PackID: name}
		for j := range system.InitialActors {
			if system.InitialActors[j].ID == DefaultActorID {
				initial := system.InitialActors[j].State
				initial[xiuxianFieldRealm] = "qi_1"
				initial[xiuxianFieldHealth] = float64(profiles[i].Stages["qi_1"].Base.HealthMax)
				initial[xiuxianFieldMana] = float64(profiles[i].Stages["qi_1"].Base.ManaMax)
			}
		}
		story, err := store.CreateStory(CreateStoryRequest{Title: name, ActorState: &system, PackBinding: &StoryPackBinding{PackID: name, Scope: "pack:" + name}})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, story.ID)
		if _, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "入定", Narrative: "在安全的静室内休整。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
			t.Fatal(err)
		}
		cultivationTestApply(t, store, story.ID, CultivationRequest{CommandID: "grow", CommandHash: "grow", Interval: CultivationInterval{Value: 1, Unit: CultivationUnitDay}})
	}
	progress := []float64{}
	prices := []int64{}
	// Reopen from the canonical journal, after changing the original author input.
	profiles[1].Stages["qi_1"] = cultivation.StageRule{}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(root)
	for i, id := range ids {
		ctx, err := store.StoryContext(id, "main")
		if err != nil {
			t.Fatal(err)
		}
		maximum := xiuxianStoredInt(ctx.Snapshot.State, DefaultActorID, xiuxianFieldHealthMax)
		if maximum != 100*(i+1) {
			t.Fatalf("world %d health=%d", i, maximum)
		}
		progress = append(progress, xiuxianStoredFloat(ctx.Snapshot.State, DefaultActorID, xiuxianFieldRealmProgress))
		quote, err := QuoteTrade(StoryCultivationRules(ctx.Meta), ctx.Snapshot.State, TradeQuoteRequest{Merchant: "merchant", DefID: "material.ore.iron", Direction: "buy", Quantity: 1})
		if err != nil {
			t.Fatal(err)
		}
		prices = append(prices, quote.UnitPrice)
		if i == 1 && quote.MerchantCash != 1234 {
			t.Fatalf("merchant cash came from another world: %d", quote.MerchantCash)
		}
	}
	if progress[0] <= 0 || progress[1] != 2*progress[0] {
		t.Fatalf("training not world scoped: %v", progress)
	}
	if prices[0] <= 0 || prices[1] != 3*prices[0] {
		t.Fatalf("prices not world scoped: %v", prices)
	}
}

func TestExplicitCultivationCatalogDoesNotBorrowMissingDefinitions(t *testing.T) {
	rules := cultivation.ReferenceRules()
	rules.Items = nil
	rules.Skills = nil
	rules.Techniques = nil
	rules.CraftMethods = nil
	if _, ok := storyItemCatalog(rules).Definition("material.ore.iron"); ok {
		t.Fatal("missing item borrowed from reference catalog")
	}
	if len(storySkillRegistry(rules).Definitions()) != 0 {
		t.Fatal("empty skill catalog inherited defaults")
	}
	if _, ok := resolveTechnique(rules, "method.basic_breathing", 0); ok {
		t.Fatal("missing technique borrowed from reference catalog")
	}
	if len(storyCraftRegistry(StoryDirectorActorStateSystem{CultivationRules: rules}).Methods()) != 0 {
		t.Fatal("empty craft catalog inherited defaults")
	}
}
