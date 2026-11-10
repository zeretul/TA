package interactive

import (
	"reflect"
	"ta/internal/character"
	"testing"
)

func enemyTacticsFixture(t *testing.T, family string) (AdventureRules, AdventureActor) {
	t.Helper()
	r := AdventureRules{Family: family, Currency: "coin", StartingSkills: []string{"strike", "heal", "guard", "energy", "cure"}, Skills: []AdventureSkill{
		{ID: "strike", Name: "Strike", Kind: "attack", Scale: "person", Power: 15, Cost: 10, RequiredRank: 1, TrainingHours: 2},
		{ID: "heal", Name: "Heal", Kind: "heal", Scale: "person", Power: 30, Cost: 12, RequiredRank: 1, TrainingHours: 2},
		{ID: "guard", Name: "Guard", Kind: "guard", Scale: "person", Power: 20, Cost: 8, RequiredRank: 1, TrainingHours: 2},
		{ID: "energy", Name: "Recover", Kind: "energy", Scale: "person", Power: 40, Cost: 0, RequiredRank: 1, TrainingHours: 2},
		{ID: "cure", Name: "Cure", Kind: "antidote", Scale: "person", Cost: 5, RequiredRank: 1, TrainingHours: 2},
	}, Items: []AdventureItem{
		{ID: "ammo", Name: "Ammo", Kind: "material", Scale: "person", Acquisition: "market", Price: 1},
		{ID: "weapon", Name: "Weapon", Kind: "weapon", Scale: "person", Acquisition: "market", Price: 20, Power: 10, Materials: map[string]int{"ammo": 1}},
	}, Opponents: []AdventureOpponent{{ID: "rival", Name: "Rival", Scale: "person", Location: "arena", Health: 100, Attack: 20, Rank: 1, SkillIDs: []string{"strike", "heal"}, WeaponID: "weapon", Inventory: map[string]int{"ammo": 3}}}}
	a, _, err := ResolveAdventureAction(r, NewAdventureActor(r), AdventureAction{Action: "battle_start", OpponentID: "rival"}, "start")
	if err != nil {
		t.Fatal(err)
	}
	return r, a
}

func TestAdventureEnemySelectsRecoveryWithoutFreeAttackOrAmmunition(t *testing.T) {
	for _, family := range []string{"wuxia", "space"} {
		t.Run(family, func(t *testing.T) {
			r, a := enemyTacticsFixture(t, family)
			a.Battle.Health = 20
			healed, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard"}, "heal")
			if err != nil {
				t.Fatal(err)
			}
			if healed.Battle.Health != 50 || healed.Health != a.Health || healed.Battle.EnemyEnergy != 88 || healed.Battle.EnemyInventory["ammo"] != 3 {
				t.Fatalf("recovery mixed with a free shot: %+v", healed)
			}
			healthy := a
			battle := *a.Battle
			healthy.Battle = &battle
			healthy.Battle.Health = healthy.Battle.MaxHealth
			attacked, _, err := ResolveAdventureAction(r, healthy, AdventureAction{Action: "battle_round", Choice: "guard"}, "attack")
			if err != nil || attacked.Health >= healthy.Health || attacked.Battle.EnemyInventory["ammo"] != 2 || attacked.Battle.EnemyEnergy != 90 {
				t.Fatalf("healthy enemy did not attack with a real cost: %+v / %v", attacked, err)
			}
			if a.Battle.Health != 20 || a.Battle.EnemyInventory["ammo"] != 3 {
				t.Fatal("decision mutated original state")
			}
		})
	}
}

func TestAdventureEnemyGuardTurnsAsideOnlyThisRoundsForce(t *testing.T) {
	r, a := enemyTacticsFixture(t, "wuxia")
	r.Opponents[0].SkillIDs = []string{"guard"}
	r.Skills[2].Effects = []AdventureEffect{{Kind: "guard", Power: 20}, {Kind: "guard", Power: 20}}
	a.Battle.Health = 50
	a.Rank = 10 // Enough observed force makes this complete guard worthwhile.
	guarded, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "guard")
	if err != nil || guarded.Battle.Health != 50 || guarded.Health != a.Health || guarded.Battle.EnemyInventory["ammo"] != 3 {
		t.Fatalf("guard healed prior wounds or dealt a free blow: %+v / %v", guarded, err)
	}
}

func TestAdventureEnemyReservesSkillMaterialsAndWeaponAmmoTogether(t *testing.T) {
	r, a := enemyTacticsFixture(t, "space")
	r.Opponents[0].SkillIDs = []string{"strike"}
	r.Skills[0].Materials = map[string]int{"ammo": 2}
	paid, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard"}, "paid")
	if err != nil || paid.Battle.EnemyInventory["ammo"] != 0 || paid.Battle.EnemyEnergy != 90 {
		t.Fatalf("complete price not paid once: %+v / %v", paid, err)
	}
	a.Battle.EnemyInventory["ammo"] = 2
	short, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard"}, "short")
	if err != nil || short.Battle.EnemyInventory["ammo"] != 1 || short.Battle.EnemyEnergy != 100 {
		t.Fatalf("short stock paid an impossible skill: %+v / %v", short, err)
	}
}

func TestAdventureEnemyUsesActualLearnedGrowthAndSkipsWorkSkills(t *testing.T) {
	r, a := enemyTacticsFixture(t, "space")
	r.Skills = append(r.Skills, AdventureSkill{ID: "work", Name: "Engineering", Kind: "engineering", Scale: "person", Power: 100, RequiredRank: 1, TrainingHours: 1})
	r.StartingSkills = append(r.StartingSkills, "work")
	npc := NewAdventureActor(r)
	npc.Skills = map[string]int{"guard": 1, "work": 1}
	npc.Health = 25 // Complete protection is urgent after this hit.
	// A real person replaces the anonymous encounter's owned equipment as
	// well as its repertoire; setting only an ID is not a binding operation.
	bindAdventureEncounter(a.Battle, "rival-person", npc, r)
	// The authored strike and heal are no longer owned; the newly learned guard
	// is not on that old list but must become a real combat option.
	guarded, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "grown")
	if err != nil || guarded.Health != a.Health || guarded.Battle.Health != a.Battle.Health || guarded.Battle.EnemyEnergy != 92 {
		t.Fatalf("NPC's current repertoire was ignored: %+v / %v", guarded, err)
	}
}

func TestAdventureEnemyChoiceIsStableAndDoesNotReadHiddenPlayerSkills(t *testing.T) {
	r, a := enemyTacticsFixture(t, "space")
	first := chooseAdventureEnemyTechnique(r, &a, r.Opponents[0], r.Items[1], 10, 0)
	r.Skills[0], r.Skills[1] = r.Skills[1], r.Skills[0]
	a.Skills = map[string]int{"cure": 10}
	a.Energy = 0
	next := chooseAdventureEnemyTechnique(r, &a, r.Opponents[0], r.Items[1], 10, 0)
	if !reflect.DeepEqual(first, next) {
		t.Fatalf("hidden player data or ordering changed choice: %+v -> %+v", first, next)
	}
}

func TestAdventureLearnedNPCTacticPersistsThroughCommitRetryBranchAndColdReplay(t *testing.T) {
	r := adventureTestRules()
	r.Opponents[0].CharacterSourceKey = "person.rival"
	r.Opponents[0].SkillIDs = []string{"strike"}
	npc := NewAdventureActor(r)
	npc.Skills = map[string]int{"strike": 1}
	npc.Inventory = map[string]int{"book": 1}
	delete(r.StartingItems, "book") // This fixture's single teaching book belongs to the NPC.
	r.NPCs = []AdventureNPC{{CharacterSourceKey: "person.rival", Actor: npc, Plan: AdventureNPCPlan{Action: "learn", SkillID: "advanced", TrainingPercent: 100, RankLimit: 1}}}
	store, id := adventureTestStoryWithRules(t, r)
	defer func() { _ = store.Close() }()
	_, err := store.AppendCharacterFacts(id, AppendCharacterFactsRequest{BranchID: "main", WorldTime: "第1日", Scene: character.Scene{Location: "船坞", Present: []string{"rival"}}, Profiles: []character.Profile{{ID: "rival", Name: "Rival", SourceKey: "person.rival", Revision: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "rest two hours", Narrative: "Both continue their activities.", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "rest two hours", Adventure: &AdventureAction{Action: "rest", Hours: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "迎战", Narrative: "The rival approaches.", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "迎战", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "rival"}}}})
	if err != nil {
		t.Fatal(err)
	}
	window, err := store.AdventureWindow(id, "main")
	if err != nil || window.Actor.Battle.EnemySkillRanks["advanced"] != 1 {
		t.Fatalf("study did not reach battle: %+v / %v", window.Actor.Battle, err)
	}
	chosen := chooseAdventureEnemyTechnique(window.Rules, &window.Actor, r.Opponents[0], AdventureItem{}, 0, 0)
	if chosen.Skill.ID != "advanced" {
		t.Fatalf("new learned tactic not chosen: %+v", chosen)
	}
	input := AdventureAction{Action: "battle_round", Choice: "guard"}
	first, err := store.CommitAdventureAction(id, "main", "grown-round", window.Head, input)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := store.CommitAdventureAction(id, "main", "grown-round", window.Head, input)
	if err != nil || !reflect.DeepEqual(first, retry) {
		t.Fatal("retry changed NPC costs", err)
	}
	if first.Actor.Health != window.Actor.Health-11 {
		t.Fatalf("learned technique was not actually resolved: %d -> %d", window.Actor.Health, first.Actor.Health)
	}
	fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: window.Head, Title: "Before response"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.AdventureWindow(id, fork.ID)
	if err != nil || before.Actor.Battle.EnemyEnergy != window.Actor.Battle.EnemyEnergy {
		t.Fatal("enemy cost leaked to branch", err)
	}
	root := store.Root()
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(root)
	cold, err := store.AdventureWindow(id, "main")
	if err != nil || !reflect.DeepEqual(cold.Actor, first.Actor) {
		t.Fatal("cold replay changed enemy tactic settlement", err)
	}
}

func TestAdventureWeaponTechniqueKeepsItsLaserTypeAgainstActualEnemyShield(t *testing.T) {
	r, a := enemyTacticsFixture(t, "space")
	r.Items[1].AttackKind = "laser"
	r.Items = append(r.Items, AdventureItem{ID: "shield", Name: "Shield", Kind: "armor", Scale: "person", Acquisition: "reward", Effect: "shield", Power: 15, MaxCharges: 2, CooldownSeconds: 60})
	a.Inventory["weapon"], a.Inventory["ammo"] = 1, 3
	a.Equipment["person:weapon"] = "weapon"
	a.Battle.EnemyInventory["shield"] = 1
	a.Battle.EnemyEquipment = map[string]string{"person:armor": "shield"}
	next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "strike"}, "laser-technique")
	if err != nil {
		t.Fatal(err)
	}
	if next.Battle.EnemyItemCharges["shield"] != 1 || next.Inventory["ammo"] != 2 {
		t.Fatalf("technique bypassed a real laser shield or paid the wrong ammunition: %+v", next)
	}
}

func TestAdventureObservedEnemyTechniqueClearsWhenNoReplyOccurred(t *testing.T) {
	for _, reason := range []string{"defeated", "controlled", "surrender"} {
		t.Run(reason, func(t *testing.T) {
			r, a := enemyTacticsFixture(t, "wuxia")
			a.Battle.EnemyLastSkillID = "heal"
			choice := "guard"
			switch reason {
			case "defeated":
				a.Battle.Health = 1
				choice = "attack"
			case "controlled":
				a.Battle.EnemyControl = 1
			case "surrender":
				choice = "surrender"
			}
			next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: choice}, "no-reply")
			if err != nil || next.Battle.EnemyLastSkillID != "" || next.Battle.EnemyEnergy != a.Battle.EnemyEnergy {
				t.Fatalf("old or unperformed technique was reported or charged: %+v / %v", next.Battle, err)
			}
		})
	}
}

func TestAdventureEnemyComputationAidRequiresItsOwnEquippedCopy(t *testing.T) {
	r := adventureTestRules()
	a := NewAdventureActor(r)
	a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "deploy", ItemID: "ship"}, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "frigate"}, "battle")
	if err != nil {
		t.Fatal(err)
	}
	a.Battle.EnemyActorID = "captain"
	a.Battle.EnemySkillRanks = map[string]int{"ship-gun": 1}
	a.Battle.EnemyInventory = map[string]int{"chip": 1}
	plain, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard"}, "plain")
	if err != nil {
		t.Fatal(err)
	}
	a.Battle.EnemyEquipment = map[string]string{"ship:tool": "chip"}
	assisted, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard"}, "assisted")
	if err != nil {
		t.Fatal(err)
	}
	remaining := func(actor AdventureActor) int { asset := actor.Assets["ship"]; return asset.Health + asset.Shield }
	if remaining(assisted) >= remaining(plain) || assisted.Battle.EnemyEnergy != plain.Battle.EnemyEnergy || assisted.Battle.EnemyLastSkillID != "ship-gun" {
		t.Fatal("owned equipped computation aid did not change the real response at the same skill price")
	}
}

func TestAdventureEnemySkipsTechniquesWithNoNetBenefit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   string
		power  int
		cost   int
		energy int
	}{
		{name: "full health", kind: "heal", power: 30, cost: 12, energy: 100},
		{name: "full energy", kind: "energy", power: 40, energy: 100},
		{name: "no incoming force", kind: "guard", power: 20, cost: 8, energy: 100},
		{name: "no poison", kind: "antidote", cost: 5, energy: 100},
		{name: "no mental attack", kind: "mental_guard", power: 80, cost: 5, energy: 100},
		{name: "nothing to reflect", kind: "reflect", power: 50, cost: 5, energy: 100},
		{name: "recovery only repays itself", kind: "energy", power: 20, cost: 20, energy: 50},
		{name: "recovery loses energy", kind: "energy", power: 10, cost: 20, energy: 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, a := enemyTacticsFixture(t, "wuxia")
			r.Skills = append(r.Skills, AdventureSkill{ID: "only-technique", Name: "Only technique", Kind: "guard", Effects: []AdventureEffect{{Kind: tc.kind, Power: tc.power}}, Scale: "person", Power: tc.power, Cost: tc.cost, RequiredRank: 1, TrainingHours: 1})
			r.Opponents[0].SkillIDs = []string{"only-technique"}
			r.StartingSkills = append(r.StartingSkills, "only-technique")
			a.Battle.EnemyEnergy = tc.energy
			a.Battle.EnemyActorID = "actual-rival"
			a.Battle.EnemySkillRanks = map[string]int{"only-technique": 1}
			a.Battle.EnemyEquipment = map[string]string{"person:weapon": "weapon"}
			a.Battle.EnemyInventory["weapon"] = 1
			// The player applies no force. An already healthy opponent must
			// still make progress instead of endlessly wasting a response.
			for round := 0; round < 2; round++ {
				next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard"}, "idle-support")
				if err != nil {
					t.Fatal(err)
				}
				if next.Health >= a.Health || next.Battle.EnemyEnergy != tc.energy || next.Battle.EnemyLastSkillID != "" || next.Battle.EnemyInventory["ammo"] != a.Battle.EnemyInventory["ammo"]-1 {
					t.Fatalf("ineffective skill replaced ordinary attack: health %d -> %d; battle %+v", a.Health, next.Health, next.Battle)
				}
				a = next
			}
		})
	}
}

func TestAdventureEnemyStillRecoversWhenGainOutweighsImmediateAttack(t *testing.T) {
	r, a := enemyTacticsFixture(t, "space")
	r.Opponents[0].SkillIDs = []string{"energy"}
	r.Skills[3].Cost = 10
	r.Skills[3].Power = 50
	a.Battle.EnemyEnergy = 50
	next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard"}, "useful-energy")
	if err != nil || next.Battle.EnemyEnergy != 90 || next.Health != a.Health || next.Battle.EnemyLastSkillID != "energy" || next.Battle.EnemyInventory["ammo"] != 3 {
		t.Fatalf("useful recovery did not pay and recover real energy: %+v / %v", next, err)
	}
}

func TestAdventureEnemyDoesNotDefendItselfToDeathAgainstWeakerAttacks(t *testing.T) {
	r, a := enemyTacticsFixture(t, "wuxia")
	r.Opponents[0].SkillIDs = []string{"strike", "guard"}
	r.Skills[0].Power = 80
	r.Skills[2].Power = 70
	// A partial guard (70 of 100 force) used to score140, beating a110
	// counterstrike, although it could only postpone death without replying.
	chosen := chooseAdventureEnemyTechnique(r, &a, r.Opponents[0], r.Items[1], 100, 0)
	if chosen.Skill.ID != "strike" {
		t.Fatalf("partial defense displaced a stronger counterattack: %+v", chosen)
	}
}

func TestAdventureEnemyRecoversUrgentlyNotForEveryScratch(t *testing.T) {
	r, a := enemyTacticsFixture(t, "wuxia")
	a.Battle.Health = 75
	chosen := chooseAdventureEnemyTechnique(r, &a, r.Opponents[0], r.Items[1], 0, 0)
	if chosen.Skill.ID != "strike" {
		t.Fatalf("minor wounds displaced an available attack: %+v", chosen)
	}
	a.Battle.Health = 20
	chosen = chooseAdventureEnemyTechnique(r, &a, r.Opponents[0], r.Items[1], 0, 0)
	if chosen.Skill.ID != "heal" {
		t.Fatalf("urgent affordable recovery was lost: %+v", chosen)
	}
}

func TestAdventureEnemyComparesSupportWithOrdinaryAttack(t *testing.T) {
	r, a := enemyTacticsFixture(t, "wuxia")
	r.Opponents[0].SkillIDs = []string{"guard"}
	chosen := chooseAdventureEnemyTechnique(r, &a, r.Opponents[0], r.Items[1], 5, 0)
	if chosen.Skill.ID != "" {
		t.Fatalf("negligible defense displaced ordinary attack: %+v", chosen)
	}
	a.Battle.Health = 10
	chosen = chooseAdventureEnemyTechnique(r, &a, r.Opponents[0], r.Items[1], 20, 0)
	if chosen.Skill.ID != "guard" {
		t.Fatalf("complete urgent protection was lost: %+v", chosen)
	}
}
