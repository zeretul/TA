package interactive

import (
	"reflect"
	"strings"
	"testing"
)

func mentalAttackFixture(t *testing.T) (AdventureRules, AdventureActor) {
	t.Helper()
	r, a := enemyTacticsFixture(t, "space")
	extra := []AdventureSkill{
		{ID: "mind", Name: "Mental pulse", Kind: "attack", Scale: "person", Cost: 8, RequiredRank: 1, TrainingHours: 6, Effects: []AdventureEffect{{Kind: "mental_attack", Power: 14}}},
		{ID: "ward", Name: "Mental ward", Kind: "guard", Scale: "person", Cost: 6, RequiredRank: 1, TrainingHours: 6, Effects: []AdventureEffect{{Kind: "mental_guard", Power: 50}}},
		{ID: "reflection", Name: "Reflection", Kind: "guard", Scale: "person", Cost: 6, RequiredRank: 1, TrainingHours: 6, Effects: []AdventureEffect{{Kind: "reflect", Power: 90}}},
	}
	r.Skills = append(r.Skills, extra...)
	for _, s := range extra {
		r.StartingSkills = append(r.StartingSkills, s.ID)
		a.Skills[s.ID] = 1
	}
	r.Opponents[0].SkillIDs = []string{"mind", "ward", "guard", "reflection"}
	return r, a
}

func TestAdventureMentalAttackSeparatesWeaponsArmorAndRealWard(t *testing.T) {
	for _, tc := range []struct {
		choice string
		damage int
	}{{"ceasefire", 14}, {"skill:guard", 14}, {"skill:reflection", 14}, {"skill:ward", 7}} {
		t.Run(tc.choice, func(t *testing.T) {
			r, a := mentalAttackFixture(t)
			r.Items[1].Power = 100
			a.Inventory["weapon"], a.Inventory["ammo"] = 1, 3
			a.Equipment["person:weapon"] = "weapon"
			a.Battle.Defense = 1000
			next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "mind", enemyChoice: tc.choice}, "mental-components")
			if err != nil {
				t.Fatal(err)
			}
			if next.Battle.Health != a.Battle.Health-tc.damage || next.Energy != a.Energy-8 || next.Inventory["ammo"] != 3 || next.Health != a.Health || next.Control != 0 || next.MentalCorruption != 0 {
				t.Fatalf("mental attack inherited physical power/mitigation or free side effect: %+v", next)
			}
			if tc.choice == "skill:ward" && next.Battle.EnemyEnergy != a.Battle.EnemyEnergy-6 {
				t.Fatal("mental ward was free")
			}
		})
	}
}

func TestAdventureMentalAttackEnemyIsSymmetricAndDoesNotBorrowPhysicalMitigation(t *testing.T) {
	for _, tc := range []struct {
		choice, skill string
		damage        int
	}{{"guard", "", 14}, {"skill", "reflection", 14}, {"skill", "ward", 7}} {
		t.Run(tc.choice+tc.skill, func(t *testing.T) {
			r, a := mentalAttackFixture(t)
			r.Items = append(r.Items, AdventureItem{ID: "heavy-armor", Name: "Heavy armor", Kind: "armor", Scale: "person", Power: 100, Price: 50, Acquisition: "market"})
			a.Inventory["heavy-armor"] = 1
			a.Equipment["person:armor"] = "heavy-armor"
			next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: tc.choice, SkillID: tc.skill, enemyChoice: "skill:mind"}, "enemy-mental")
			if err != nil {
				t.Fatal(err)
			}
			if next.Health != a.Health-tc.damage || next.Battle.Health != a.Battle.Health || next.Battle.EnemyEnergy != a.Battle.EnemyEnergy-8 || next.Battle.EnemyInventory["ammo"] != a.Battle.EnemyInventory["ammo"] || next.Control != 0 || next.MentalCorruption != 0 {
				t.Fatalf("mental response gained ammo/physical mitigation/side effects: %+v", next)
			}
		})
	}
}

func TestAdventureMentalAttackCompositeGuardDoesNotBuyFreeStance(t *testing.T) {
	r, a := mentalAttackFixture(t)
	plain, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "mind", enemyChoice: "ordinary"}, "plain-mind")
	if err != nil {
		t.Fatal(err)
	}
	for i := range r.Skills {
		if r.Skills[i].ID == "mind" {
			r.Skills[i].Effects = append(r.Skills[i].Effects, AdventureEffect{Kind: "guard", Power: 1})
		}
	}
	mixed, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "mind", enemyChoice: "ordinary"}, "mind-plus-guard")
	if err != nil {
		t.Fatal(err)
	}
	if mixed.Health != plain.Health+1 || mixed.Battle.Health != plain.Battle.Health {
		t.Fatalf("one-point mental combo received a full defensive stance: plain=%d combo=%d", plain.Health, mixed.Health)
	}
}

func TestAdventureMentalAttackWardCapAndLearnedMasteryAreReal(t *testing.T) {
	r, a := mentalAttackFixture(t)
	a.Skills["mind"] = 3
	a.Battle.EnemySkillRanks = map[string]int{"ward": 3}
	for i := range r.Skills {
		if r.Skills[i].ID == "ward" {
			r.Skills[i].Effects = append(r.Skills[i].Effects, AdventureEffect{Kind: "mental_guard", Power: 90})
		}
	}
	next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "mind", enemyChoice: "skill:ward"}, "capped-ward")
	if err != nil {
		t.Fatal(err)
	}
	if next.Battle.Health != a.Battle.Health-5 || next.Energy != a.Energy-8 || next.Battle.EnemyEnergy != a.Battle.EnemyEnergy-6 {
		t.Fatalf("ward stacked past 90%% or lost real mastery/cost: %+v", next)
	}
}

func TestAdventureMentalAttackRejectsFreeOrCrossScaleAndGeneratedEscalation(t *testing.T) {
	r, a := mentalAttackFixture(t)
	for _, mode := range []string{"free", "ship", "wuxia"} {
		t.Run(mode, func(t *testing.T) {
			copyRules, _ := mentalAttackFixture(t)
			switch mode {
			case "free":
				copyRules.Skills[len(copyRules.Skills)-3].Cost = 0
			case "ship":
				copyRules.Skills[len(copyRules.Skills)-3].Scale = "ship"
			case "wuxia":
				copyRules.Family = "wuxia"
			}
			if copyRules.Validate() == nil {
				t.Fatal("invalid mental capability accepted")
			}
		})
	}
	var mind AdventureSkill
	for _, s := range r.Skills {
		if s.ID == "mind" {
			mind = s
		}
	}
	if _, ok := adventurePlainSkillEffects(mind); ok {
		t.Fatal("mental effect entered unrestricted cheap recomposition")
	}
	if _, err := adventureDesignEffectBudget([]AdventureEffect{{Kind: "mental_attack", Power: 100}}, "person"); err == nil {
		t.Fatal("generated physical template invented a mental effect")
	}
	if _, _, err := deriveAdventureGeneratedSkill(r, mind, AdventureGeneratedDefinition{}); err != nil {
		t.Fatal("exact frozen mental variant was rejected", err)
	}
	// An actual learned skill cannot turn a registered space capability into
	// remote attacks on a crew through the vessel's hull.
	fleetRules, fleet := personalFleetFixture(t, 33, "xinghai-space-breach", 6)
	fleetRules.Skills = append(fleetRules.Skills, mind)
	fleetRules.StartingSkills = append(fleetRules.StartingSkills, mind.ID)
	fleet.Skills[mind.ID] = 1
	fleet = startPersonalFleet(t, fleetRules, fleet, "xinghai-raider-patrol", "xinghai-space-breach")
	before := fleet
	next, _, err := ResolveAdventureAction(fleetRules, fleet, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "mind"}, "no-remote-crew")
	if err == nil || !reflect.DeepEqual(next, before) {
		t.Fatal("mental skill crossed the hull boundary or spent a rejected action", err)
	}
	found := false
	for _, c := range adventureEnemyDecisionCandidates(r, &a) {
		if c.ID == "skill:mind" {
			found = strings.Contains(c.Description, "精神冲击")
		}
	}
	if !found {
		t.Fatal("legal owned mental ability was absent from actual model menu")
	}
}

func TestAdventureMentalAttackSmallHitsAndDuplicateWardsRespectPublishedCap(t *testing.T) {
	if got := adventureMentalDamage(6, 90); got != 1 {
		t.Fatalf("90 percent became total immunity: %d", got)
	}
	b := &AdventureBattle{Scale: "person", Health: 100, MaxHealth: 100, EnemyEnergy: 100, EnemyMaxEnergy: 100}
	single, ok := adventureEnemyTechniqueScore(b, []AdventureEffect{{Kind: "mental_guard", Power: 50}}, 1, 0, 42, 6, true)
	duplicate, valid := adventureEnemyTechniqueScore(b, []AdventureEffect{{Kind: "mental_guard", Power: 50}, {Kind: "mental_guard", Power: 50}}, 1, 0, 42, 6, true)
	if !ok || !valid || single != duplicate || single != 21 {
		t.Fatalf("duplicate ward inflated tactical utility: single=%d duplicate=%d", single, duplicate)
	}
	for _, playerAttacks := range []bool{false, true} {
		r, a := mentalAttackFixture(t)
		for i := range r.Skills {
			if r.Skills[i].ID == "mind" {
				r.Skills[i].Effects[0].Power = 6
			}
			if r.Skills[i].ID == "ward" {
				r.Skills[i].Effects[0].Power = 90
			}
		}
		action := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "ward", enemyChoice: "skill:mind"}
		if playerAttacks {
			action.SkillID, action.enemyChoice = "mind", "skill:ward"
		}
		next, _, err := ResolveAdventureAction(r, a, action, "symmetric-tiny-mental")
		if err != nil {
			t.Fatal(err)
		}
		if playerAttacks {
			if next.Battle.Health != a.Battle.Health-1 || next.Health != a.Health || next.Energy != a.Energy-8 || next.Battle.EnemyEnergy != a.Battle.EnemyEnergy-6 {
				t.Fatal("player tiny mental hit exceeded the paid ward cap")
			}
		} else if next.Health != a.Health-1 || next.Battle.Health != a.Battle.Health || next.Energy != a.Energy-6 || next.Battle.EnemyEnergy != a.Battle.EnemyEnergy-8 {
			t.Fatal("enemy tiny mental hit exceeded the paid ward cap")
		}
	}
}

func TestAdventureMentalAttackCannotTreatRegisteredMechanismsAsMinds(t *testing.T) {
	r := authoredAdventureRules(t, "xinghai-689")
	a, err := ApplyAdventureStartingLoadouts(r, NewAdventureActor(r), "xinghai-class-psychic")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"xinghai-sparring", "xinghai-electronic-node"} {
		t.Run(id, func(t *testing.T) {
			started, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: id}, "mindless-start")
			if e != nil {
				t.Fatal(e)
			}
			if !started.Battle.Mindless {
				t.Fatal("authored mechanism did not freeze its actual target condition")
			}
			next, _, e := ResolveAdventureAction(r, started, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-starter-mental-pulse"}, "no-machine-mind")
			if e == nil || !reflect.DeepEqual(next, started) {
				t.Fatal("mental attack hit a mindless target or spent the invalid action")
			}
			if _, _, e = ResolveAdventureAction(r, started, AdventureAction{Action: "battle_round", Choice: "attack"}, "ordinary-training"); e != nil {
				t.Fatal("a mindless target blocked legitimate physical training", e)
			}
			changed := *started.Battle
			changed.Mindless = false
			started.Battle = &changed
			if ValidateAdventureActor(r, started) == nil {
				t.Fatal("saved battle could remove its frozen mindless condition")
			}
		})
	}
}

func TestAdventureMentalAttackPublishedWardCapAlsoAppliesToRegisteredStorm(t *testing.T) {
	r, a := spaceMentalFixture(t)
	started, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "xinghai-e06"}, "registered-storm")
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := ResolveAdventureAction(r, started, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s19", enemyChoice: "ordinary"}, "nineteen-percent-ward")
	if err != nil {
		t.Fatal(err)
	}
	// Real author values: 50 incoming, 19% ward at mastery one. Integer
	// mitigation must not become 20%; this is the existing registered source.
	if got.Health != started.Health-41 {
		t.Fatalf("published 19%% ward exceeded its percentage on registered 50-point storm: damage=%d, want41; player costs=%d", started.Health-got.Health, started.Energy-got.Energy)
	}
}
