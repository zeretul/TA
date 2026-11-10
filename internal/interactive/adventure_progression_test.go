package interactive

import (
	"reflect"
	"testing"
)

func TestAdventureExtendedProgressionIsReachableWithPaidNativeActions(t *testing.T) {
	for _, pack := range []string{"lanchuan-qunxia", "xinghai-689"} {
		t.Run(pack, func(t *testing.T) {
			r := authoredAdventureRules(t, pack)
			if err := r.Validate(); err != nil {
				t.Fatal(err)
			}
			a := NewAdventureActor(r)
			var elapsed int64
			action := func(in AdventureAction) {
				t.Helper()
				next, out, err := ResolveAdventureAction(r, a, in, "paid-progression")
				if err != nil {
					t.Fatalf("%+v at rank %d: %v", in, a.Rank, err)
				}
				a = next
				elapsed += out.DurationSeconds
			}
			// Funding is earned through an authored market's ordinary work, never
			// injected XP, currency, inventory or an unlimited battle reward.
			market := r.Markets[0]
			fund := func(amount int) {
				for a.Currency < amount {
					if a.Energy < market.WorkService.EnergyCost {
						action(AdventureAction{Action: "rest", Hours: (a.MaxEnergy - a.Energy + 19) / 20})
					}
					hours := a.Energy / market.WorkService.EnergyCost * market.WorkService.Hours
					action(AdventureAction{Action: "work", MarketID: market.ID, Hours: hours})
				}
			}
			visited := map[string]bool{}
			for a.Rank < AdventureMaxRank(r) {
				stage := AdventureProgressionStageFor(r, a.Rank)
				visited[stage.ID] = true
				id := stage.PracticeSkill
				if id == "" {
					id = r.StartingSkills[0]
				}
				if a.Skills[id] < stage.PracticeMastery {
					before := a
					if _, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "refine", SkillID: id, Hours: 1}, "unlearned"); err == nil {
						t.Fatal("refinement bypassed foundation mastery")
					}
					if !reflect.DeepEqual(a, before) {
						t.Fatal("refused refinement mutated input")
					}
					var skill AdventureSkill
					for _, s := range r.Skills {
						if s.ID == id {
							skill = s
						}
					}
					for _, item := range r.Items {
						if item.ID == skill.RequiredItem {
							fund(item.Price)
						}
					}
					action(AdventureAction{Action: "buy", MarketID: market.ID, ItemID: skill.RequiredItem})
					action(AdventureAction{Action: "learn", SkillID: id, Hours: skill.TrainingHours * 3})
				}
				limit := AdventureMaxRank(r)
				for _, next := range r.Progression.Stages {
					if next.MinRank > a.Rank {
						limit = next.MinRank
						break
					}
				}
				xp := -a.Experience
				for rank := a.Rank; rank < limit; rank++ {
					xp += AdventureRankExperience(r, rank)
				}
				fund((xp + r.Progression.ExperiencePerHour - 1) / r.Progression.ExperiencePerHour * r.Progression.CurrencyPerHour)
				action(AdventureAction{Action: "refine", SkillID: id, Hours: 100000})
				if a.Rank != limit {
					t.Fatalf("refinement did not stop at the next stage: %d vs %d", a.Rank, limit)
				}
			}
			if len(visited) != len(r.Progression.Stages) || a.Experience != 0 || a.MaxHealth != 100+20*(a.Rank-1) {
				t.Fatalf("incomplete growth: %+v", a)
			}
			if _, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "refine", Hours: 1}, "ceiling"); err == nil {
				t.Fatal("refinement exceeded ceiling")
			}
			// A ceiling is not a forced ending; ordinary life remains playable.
			action(AdventureAction{Action: "rest", Hours: 1})
			t.Logf("paid native path: rank=%d stages=%d elapsed_hours=%d currency=%d", a.Rank, len(visited), elapsed/3600, a.Currency)
		})
	}
}

func TestAdventureRefinementIsSegmentInvariantAndCannotBorrowSupplies(t *testing.T) {
	r := authoredAdventureRules(t, "xinghai-689")
	a := NewAdventureActor(r)
	a.Currency = 14
	in := AdventureAction{Action: "refine", SkillID: r.StartingSkills[0], Hours: 100000}
	long, out, err := ResolveAdventureAction(r, a, in, "batch")
	if err != nil {
		t.Fatal(err)
	}
	segmented := a
	for i := 0; i < 7; i++ {
		segmented, _, err = ResolveAdventureAction(r, segmented, AdventureAction{Action: "refine", SkillID: in.SkillID, Hours: 1}, "hour")
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(long, segmented) || out.DurationSeconds != 7*3600 || long.Currency != 0 {
		t.Fatal("funding or segmentation changed progression")
	}
	if _, _, err := ResolveAdventureAction(r, long, in, "empty"); err == nil {
		t.Fatal("refinement borrowed money")
	}
	r.Progression = nil
	if AdventureMaxRank(r) != 10 || AdventureProgressionStageFor(r, 10) != nil {
		t.Fatal("legacy rules were silently upgraded")
	}
	if _, err := refineAdventureFoundation(r, &a, in); err == nil {
		t.Fatal("legacy world gained new actions")
	}
}

func TestAdventureRefinementCommitsClockAndRestoresBranches(t *testing.T) {
	r := authoredAdventureRules(t, "lanchuan-qunxia")
	a := NewAdventureActor(r)
	store, id := adventureFixtureStoryWithActor(t, r, a, r.Markets[0].Location)
	_, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "做两小时短工", Narrative: "你做完两个小时的码头短工，收下工钱。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "做两小时短工", Adventure: &AdventureAction{Action: "work", MarketID: r.Markets[0].ID, Hours: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	start, err := store.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: start.Head, Title: "before refinement"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "深修两小时", Narrative: "你用两个小时整顿基础。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "深修两小时", Adventure: &AdventureAction{Action: "refine", SkillID: r.StartingSkills[0], Hours: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if current.ClockSeconds-start.ClockSeconds != 7200 || current.Actor.Currency != start.Actor.Currency-2 || current.Actor.Experience != start.Actor.Experience+32 {
		t.Fatal("refinement did not settle formal clock and costs")
	}
	old, err := store.AdventureWindow(id, fork.ID)
	if err != nil || !reflect.DeepEqual(old.Actor, start.Actor) {
		t.Fatalf("branch changed: %v", err)
	}
	root := store.Root()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cold := NewStore(root)
	defer cold.Close()
	restored, err := cold.AdventureWindow(id, "main")
	if err != nil || !reflect.DeepEqual(restored.Actor, current.Actor) {
		t.Fatalf("cold replay lost progression: %v", err)
	}
}

func TestAdventureAlternativeFoundationAndHighStageGeneratedSkills(t *testing.T) {
	r := authoredAdventureRules(t, "lanchuan-qunxia")
	a := NewAdventureActor(r)
	stage := r.Progression.Stages[len(r.Progression.Stages)-1]
	a.Rank, a.Currency = stage.MinRank, 100
	alternative := stage.PracticeAlternatives[0]
	a.Skills[alternative] = stage.PracticeMastery
	next, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "refine", SkillID: alternative, Hours: 1}, "alternate")
	if err != nil || next.Experience != a.Experience+r.Progression.ExperiencePerHour || out.DurationSeconds != 3600 {
		t.Fatalf("alternative did not grant actual refinement: %v", err)
	}
	for _, skill := range r.Skills {
		if skill.ID == "lanchuan-advanced-strike-7" {
			a.Skills[skill.ID] = 2
			generated, _, err := deriveAdventureGeneratedSkill(r, skill, AdventureGeneratedDefinition{Category: "skill", Design: &AdventureGeneratedDesign{Power: &skill.Power}})
			if err != nil || generated.RequiredRank <= 10 {
				t.Fatalf("high-stage generation still has a legacy ten-rank cap: %v", err)
			}
		}
	}
}

func TestAdventureLegacyCatalogRetainsItsActualTenRankCeiling(t *testing.T) {
	r := adventureTestRules()
	a := NewAdventureActor(r)
	a.Rank = 10
	a.Experience = 2000
	if _, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "rest", Hours: 1}, "old-save"); err != nil {
		t.Fatalf("legacy save stopped working: %v", err)
	}
	if _, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "refine", SkillID: "strike", Hours: 1}, "old-refine"); err == nil {
		t.Fatal("legacy catalog silently gained extended growth")
	}
	a.Rank = 11
	if ValidateAdventureActor(r, a) == nil {
		t.Fatal("legacy catalog accepted an invented extra rank")
	}
}

// Qualified resource fixtures isolate the generation/equipment consumer. The
// separate full progression test earns all growth funds through native work.
func TestAdventureHighStageEquipmentCanBeCraftedAndAdapted(t *testing.T) {
	for _, pack := range []string{"lanchuan-qunxia", "xinghai-689"} {
		for _, kind := range []string{"weapon", "armor"} {
			t.Run(pack+"/"+kind, func(t *testing.T) {
				r := authoredAdventureRules(t, pack)
				stage := r.Progression.Stages[len(r.Progression.Stages)-1]
				var recipe AdventureRecipe
				var item AdventureItem
				for _, candidate := range r.Recipes {
					for _, product := range r.Items {
						if candidate.OutputID == product.ID && product.RequiredSkill == stage.PracticeSkill && product.Kind == kind {
							recipe, item = candidate, product
						}
					}
				}
				if recipe.ID == "" {
					t.Fatal("high-stage equipment has no real manufacturing foundation")
				}
				a := NewAdventureActor(r)
				a.Rank = stage.MinRank
				a.Skills[recipe.SkillID] = min(AdventureSkillMasteryCap(r), recipe.SkillRank+1)
				a.Skills[stage.PracticeSkill] = 2
				for id, count := range recipe.Materials {
					a.Inventory[id] = count * 5
				}
				made, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "craft", RecipeID: recipe.ID, Hours: recipe.Hours}, "craft-high")
				if err != nil || made.Inventory[item.ID] != 1 || out.DurationSeconds != int64(recipe.Hours)*3600 {
					t.Fatalf("high-stage manufacture failed: %v", err)
				}
				power := item.Power + 1
				definition := AdventureGeneratedDefinition{Category: "equipment", TemplateID: item.ID, Name: "细调过的装备", Source: "按已掌握工艺细调实物", Design: &AdventureGeneratedDesign{Power: &power, MethodID: recipe.ID}}
				next, out, err := ResolveAdventureAction(r, made, AdventureAction{Action: "generate", Generate: &definition, Hours: recipe.Hours * 2}, "adapt-high")
				if err != nil {
					t.Fatal(err)
				}
				id, _ := definition.id()
				if next.Inventory[item.ID] != 0 || next.Inventory[id] != 1 || out.DurationSeconds != int64(recipe.Hours*2)*3600 {
					t.Fatal("adaptation skipped the real object or work")
				}
				for material := range recipe.Materials {
					if next.Inventory[material] != 0 {
						t.Fatal("adaptation did not consume the stronger budget")
					}
				}
				equipped, _, err := ResolveAdventureAction(r, next, AdventureAction{Action: "equip", ItemID: id}, "equip-high")
				if err != nil || equipped.Equipment["person:"+kind] != id {
					t.Fatalf("generated high-stage equipment has no consumer: %v", err)
				}
			})
		}
	}
}
