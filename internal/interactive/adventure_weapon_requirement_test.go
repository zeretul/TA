package interactive

import (
	"reflect"
	"testing"
)

func weaponRequirementFixture(t *testing.T) (AdventureRules, AdventureActor) {
	t.Helper()
	r, a := enemyTacticsFixture(t, "space")
	r.Skills[0].RequiredWeapon = "weapon"
	r.Items = append(r.Items, AdventureItem{ID: "blade", Name: "Blade", Kind: "weapon", Scale: "person", Power: 10, Price: 20, Acquisition: "market"})
	a.Inventory["weapon"], a.Inventory["blade"], a.Inventory["ammo"] = 1, 1, 3
	a.Equipment["person:weapon"] = "weapon"
	return r, a
}

func TestAdventureRequiredWeaponUsesActualEquippedOwnedSourceAndAmmunition(t *testing.T) {
	for _, mode := range []string{"equipped", "bag-only", "different-weapon", "unowned", "empty-ammo", "generated-variant"} {
		t.Run(mode, func(t *testing.T) {
			r, a := weaponRequirementFixture(t)
			switch mode {
			case "bag-only":
				delete(a.Equipment, "person:weapon")
			case "different-weapon":
				a.Equipment["person:weapon"] = "blade"
			case "unowned":
				a.Inventory["weapon"] = 0
			case "empty-ammo":
				a.Inventory["ammo"] = 0
			case "generated-variant":
				item := r.Items[1]
				item.ID = "local-weapon"
				item.GeneratedFrom = "weapon"
				r.Items = append(r.Items, item)
				a.Inventory[item.ID] = 1
				a.Equipment["person:weapon"] = item.ID
			}
			before := a
			result, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "strike", enemyChoice: "ceasefire"}, "bound-weapon")
			if mode == "equipped" || mode == "generated-variant" {
				if err != nil || result.Inventory["ammo"] != 2 || result.Energy != a.Energy-10 {
					t.Fatalf("actual gun/variant failed or skipped ammunition: %v %+v", err, result)
				}
			} else if err == nil || !reflect.DeepEqual(result, before) {
				t.Fatalf("wrong, missing or empty gun was accepted/spent a round: %s %v", mode, err)
			}
		})
	}
}

func TestAdventureRequiredWeaponFiltersEnemyLegalMenuAndActualSelectedMove(t *testing.T) {
	for _, equipped := range []bool{false, true} {
		r, a := weaponRequirementFixture(t)
		a.Battle.EnemyActorID = "actual-rival"
		a.Battle.EnemySkillRanks = map[string]int{"strike": 1}
		a.Battle.EnemyInventory = map[string]int{"weapon": 1, "blade": 1, "ammo": 3}
		a.Battle.EnemyEquipment = map[string]string{"person:weapon": "blade"}
		if equipped {
			a.Battle.EnemyEquipment["person:weapon"] = "weapon"
		}
		offered := false
		for _, c := range adventureEnemyDecisionCandidates(r, &a) {
			offered = offered || c.ID == "skill:strike"
		}
		if offered != equipped {
			t.Fatal("model menu treated bag possession as equipped weapon")
		}
		next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "skill:strike"}, "enemy-weapon")
		if err != nil {
			t.Fatal(err)
		}
		if equipped {
			if next.Battle.EnemyLastSkillID != "strike" || next.Battle.EnemyInventory["ammo"] != 2 || next.Battle.EnemyEnergy != a.Battle.EnemyEnergy-10 {
				t.Fatal("bound NPC gun did not pay true costs")
			}
		} else if next.Battle.EnemyLastSkillID != "" || next.Battle.EnemyInventory["ammo"] != 3 || next.Battle.EnemyEnergy != a.Battle.EnemyEnergy {
			t.Fatal("invalid selected gun skill bypassed validation")
		}
	}
}

func TestAdventureRequiredWeaponCatalogRejectsWrongTargetAndGenerationRetainsGate(t *testing.T) {
	r, _ := weaponRequirementFixture(t)
	for _, id := range []string{"missing", "ammo"} {
		r.Skills[0].RequiredWeapon = id
		if r.Validate() == nil {
			t.Fatal("unregistered/non-weapon requirement was accepted")
		}
	}
	r.Skills[0].RequiredWeapon = "weapon"
	power := 20
	out, _, err := deriveAdventureGeneratedSkill(r, r.Skills[0], AdventureGeneratedDefinition{Design: &AdventureGeneratedDesign{Power: &power}})
	if err != nil || out.RequiredWeapon != "weapon" {
		t.Fatalf("legitimate generated skill dropped its weapon condition: %+v %v", out, err)
	}
	_, _, err = deriveAdventureGeneratedSkill(r, r.Skills[2], AdventureGeneratedDefinition{Design: &AdventureGeneratedDesign{Foundations: []string{"strike"}, Effects: []AdventureEffect{{Kind: "attack", Power: 15}, {Kind: "guard", Power: 5}}}})
	if err == nil {
		t.Fatal("a weapon-bound foundation was laundered into an unbound skill")
	}
}

func TestAdventureRequiredWeaponAlsoGatesActiveMovement(t *testing.T) {
	r, a := weaponRequirementFixture(t)
	r.Skills[0].Mobility = 50
	a.Equipment["person:weapon"] = "blade"
	next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "escape", SkillID: "strike"}, "wrong-movement-tool")
	if err == nil || !reflect.DeepEqual(next, a) {
		t.Fatal("movement path bypassed the actual equipped weapon condition")
	}
}

func TestAdventureRequiredWeaponClassAllowsRealUpgradesNotAnotherWeaponKind(t *testing.T) {
	for _, mode := range []string{"same-class-upgrade", "wrong-class", "unclassified", "mixed-conflict"} {
		t.Run(mode, func(t *testing.T) {
			r, a := weaponRequirementFixture(t)
			r.Skills[0].RequiredWeapon = ""
			r.Skills[0].RequiredWeaponClass = "blade"
			r.Items[2].WeaponClass = "blade"
			upgraded := r.Items[2]
			upgraded.ID = "better-blade"
			upgraded.Power = 30
			switch mode {
			case "wrong-class":
				upgraded.WeaponClass = "sword"
			case "unclassified":
				upgraded.WeaponClass = ""
			case "mixed-conflict":
				r.Skills[0].RequiredWeapon = "weapon"
				r.Items[1].WeaponClass = "blade"
			}
			r.Items = append(r.Items, upgraded)
			a.Inventory[upgraded.ID] = 1
			a.Equipment["person:weapon"] = upgraded.ID
			next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "strike", enemyChoice: "ceasefire"}, "weapon-class")
			if mode == "same-class-upgrade" {
				if err != nil || next.Battle.Health >= a.Battle.Health || next.Inventory["ammo"] != 3 {
					t.Fatal("real same-class upgrade was blocked or borrowed gun ammunition", err)
				}
			} else if err == nil || !reflect.DeepEqual(next, a) {
				t.Fatal("a weapon name or conflicting exact identity granted the wrong class", err)
			}
		})
	}
}

func TestAdventureRequiredWeaponClassSurvivesBothGeneratedDefinitions(t *testing.T) {
	r, a := weaponRequirementFixture(t)
	r.Items[2].WeaponClass = "blade"
	base := r.Skills[0]
	base.RequiredWeapon = ""
	base.RequiredWeaponClass = "blade"
	power := 20
	skill, _, err := deriveAdventureGeneratedSkill(r, base, AdventureGeneratedDefinition{Design: &AdventureGeneratedDesign{Power: &power}})
	if err != nil || skill.RequiredWeaponClass != "blade" {
		t.Fatal("generated skill erased its actual weapon class", err)
	}
	item, _, err := deriveAdventureGeneratedItem(r, r.Items[2], AdventureRecipe{}, AdventureGeneratedDefinition{})
	if err != nil || item.WeaponClass != "blade" {
		t.Fatal("generated weapon erased its source class", err)
	}
	item.ID = "local-class-blade"
	item.GeneratedFrom = r.Items[2].ID
	r.Items = append(r.Items, item)
	a.Inventory[item.ID] = 1
	a.Equipment["person:weapon"] = item.ID
	if !adventureSkillWeaponAvailable(r, skill, a.Inventory, a.Equipment) {
		t.Fatal("real generated compatible weapon did not satisfy class")
	}
}

func TestAdventureRequiredWeaponClassCatalogRejectsInventedOrEmptyClass(t *testing.T) {
	r, _ := weaponRequirementFixture(t)
	r.Skills[0].RequiredWeapon = ""
	r.Skills[0].RequiredWeaponClass = "sword"
	if r.Validate() == nil {
		t.Fatal("unregistered weapon class had no actual matching object")
	}
	r.Items[2].WeaponClass = "sword"
	if err := r.Validate(); err != nil {
		t.Fatal("matching registered class was rejected", err)
	}
	r.Items[2].WeaponClass = "all-weapons"
	if r.Validate() == nil {
		t.Fatal("unimplemented class bypassed the explicit vocabulary")
	}
}

func TestAdventureRequiredWeaponAuthoredAnonymousLoadoutReachesRealConsumer(t *testing.T) {
	r, _ := weaponRequirementFixture(t)
	r.Opponents[0].SkillIDs = []string{"strike"}
	// This is an actual accepted authored opponent, not a forged active battle.
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	a, _, err := ResolveAdventureAction(r, NewAdventureActor(r), AdventureAction{Action: "battle_start", OpponentID: "rival"}, "declared-weapon-start")
	if err != nil {
		t.Fatal(err)
	}
	offered := false
	for _, c := range adventureEnemyDecisionCandidates(r, &a) {
		offered = offered || c.ID == "skill:strike"
	}
	next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "skill:strike"}, "declared-weapon-reply")
	if err != nil {
		t.Fatal(err)
	}
	if !offered || next.Battle.EnemyLastSkillID != "strike" || next.Battle.EnemyEnergy != a.Battle.EnemyEnergy-10 || next.Battle.EnemyInventory["ammo"] != 2 {
		t.Fatalf("accepted authored WeaponID did not reach required weapon consumer: offered=%v last=%q energy=%d ammo=%d equipment=%v inventory=%v", offered, next.Battle.EnemyLastSkillID, next.Battle.EnemyEnergy, next.Battle.EnemyInventory["ammo"], a.Battle.EnemyEquipment, a.Battle.EnemyInventory)
	}
}
