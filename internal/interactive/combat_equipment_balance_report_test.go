package interactive

import (
	"strings"
	"ta/internal/cultivation"
	"testing"
)

// These are sourced loadouts against real realm/armor profiles, not adjusted
// attack values. It records damage before health truncation and actual payment.
func ratedEquipmentBalanceRows(t *testing.T) []map[string]any {
	t.Helper()
	var rows []map[string]any
	catalog := cultivation.NewItemCatalogWithFirstVersion()
	defs := map[string]cultivation.ItemDef{}
	for _, d := range catalog.Definitions() {
		defs[d.ID] = d
	}
	realms := []cultivation.Realm{cultivation.RealmFoundation, cultivation.RealmCore, cultivation.RealmNascent, cultivation.RealmSpirit, cultivation.RealmVoid, cultivation.RealmIntegration, cultivation.RealmMahayana}

	for i, realm := range realms {
		stage := cultivation.Cultivation{Realm: realm, Stage: cultivation.StageEarly}
		strike, _ := cultivation.SkillByID(string(realm) + "_pierce")
		lower := cultivation.Qi(13)
		if i > 0 {
			lower = cultivation.Cultivation{Realm: realms[i-1], Stage: cultivation.StageLate}
		}
		targets := map[string]cultivation.Cultivation{"lower_realm": lower, "same_stage": stage, "late_armored": {Realm: realm, Stage: cultivation.StageLate}}
		if i+1 < len(realms) {
			targets["next_realm"] = cultivation.Cultivation{Realm: realms[i+1], Stage: cultivation.StageEarly}
		}
		for _, profile := range []string{"old_unique", "current_rated", "foundation_rated"} {
			id := "implement.top.unique"
			if profile == "current_rated" {
				id = "equipment." + string(realm) + "_blade"
			}
			if profile == "foundation_rated" {
				id = "equipment.foundation_blade"
			}
			def, ok := defs[id]
			if !ok {
				t.Fatal(id)
			}
			mod := cultivation.EquipmentModifier(id, cultivation.AttrAttack, def.Implement.AttackBonus)
			for _, targetName := range []string{"lower_realm", "same_stage", "late_armored", "next_realm"} {
				targetStage, exists := targets[targetName]
				if !exists {
					continue
				}
				hero := cultivation.Combatant{ID: "hero", Side: cultivation.SidePlayer, Cultivation: stage, HasImplement: true, Skills: []cultivation.SkillDef{strike}, Attributes: cultivation.AggregateAttributes(cultivation.BaseAttributes(stage), []cultivation.AttributeModifier{mod}), Equipment: []cultivation.EquipmentCopy{{InstanceID: id, DefID: id, Slot: cultivation.EquipmentSlotWeapon, ApplicableStage: def.ApplicableStage, OperatingManaCost: def.OperatingManaCost(stage), DefensePierce: def.DefensePierceAmount(stage), Modifiers: []cultivation.AttributeModifier{mod}}}}
				foe := cultivation.Combatant{ID: "foe", Side: cultivation.SideEnemy, Cultivation: targetStage, Attributes: cultivation.BaseAttributes(targetStage)}
				if targetName == "late_armored" {
					armor := cultivation.AttributeModifier{Kind: cultivation.SourceArmor, Attribute: cultivation.AttrDefense, Value: .35}
					foe.Equipment = []cultivation.EquipmentCopy{{Slot: cultivation.EquipmentSlotGarment, Modifiers: []cultivation.AttributeModifier{armor}}}
					foe.Attributes = cultivation.AggregateAttributes(foe.Attributes, []cultivation.AttributeModifier{armor})
				}
				before := hero.Attributes.Mana
				next, out := cultivation.ResolveExchange(cultivation.CombatSession{ID: "rated-loadout", Round: 1, Combatants: []cultivation.Combatant{hero, foe}}, map[string]cultivation.CombatAction{"hero": {Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "foe"}, "foe": {Kind: cultivation.ActionRest}}, func(purpose, _, _ string) float64 {
					if purpose == "hit" {
						return 0
					}
					return .5
				})
				found := false
				for _, step := range out.Steps {
					if step.ActorID == "hero" && step.Kind == cultivation.ActionTechnique {
						if step.ActionRejected || !step.Resolution.Hit {
							t.Fatalf("%s/%s/%s refused: %+v", realm, profile, targetName, step)
						}
						rows = append(rows, map[string]any{"stage": cultivation.StageKey(stage), "profile": profile, "weapon": id, "base_price": def.BasePrice, "skill": strike.ID, "weapon_mana": def.OperatingManaCost(stage), "target": targetName, "target_stage": cultivation.StageKey(targetStage), "target_defense": foe.Attributes.Defense, "attack": hero.Attributes.Attack, "damage": step.Resolution.Damage, "mana_spent": before - next.CombatantByID("hero").Attributes.Mana, "rated_pierce": def.DefensePierceAmount(stage)})
						found = true
					}
				}
				if !found {
					t.Fatal("missing rated loadout action")
				}
			}
		}
	}
	return rows
}

func TestRatedEquipmentHasRealAdvantagesAndCostsInsteadOfStrictDominance(t *testing.T) {
	rows := ratedEquipmentBalanceRows(t)
	index := map[string]map[string]any{}
	for _, row := range rows {
		index[row["stage"].(string)+"/"+row["profile"].(string)+"/"+row["target"].(string)] = row
	}
	for key, newRow := range index {
		if !strings.Contains(key, "/current_rated/") {
			continue
		}
		old := index[strings.Replace(key, "/current_rated/", "/old_unique/", 1)]
		if newRow["mana_spent"].(int) <= old["mana_spent"].(int) || old["weapon_mana"].(int) != 0 {
			t.Fatalf("rated effect did not pay its tradeoff: %s", key)
		}
		if newRow["target"] == "lower_realm" && newRow["damage"].(int) >= old["damage"].(int) {
			t.Fatalf("old unique lost its low-defense niche: %s", key)
		}
		if newRow["target"] == "next_realm" || newRow["stage"] == "mahayana_early" && newRow["target"] == "late_armored" {
			if newRow["damage"].(int) <= old["damage"].(int) {
				t.Fatalf("rated weapon still strictly dominated against its high-defense niche: %s: new=%v old=%v", key, newRow["damage"], old["damage"])
			}
		}
	}
}
