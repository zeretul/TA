package interactive

import (
	"sort"
	"testing"
)

// Lists every frozen person, including noncombatants without an encounter.
// Listing a snapshot does not establish their presence or create a fight entry.
func adventureNPCBalanceSnapshots(t *testing.T, r AdventureRules) []map[string]any {
	t.Helper()
	var rows []map[string]any
	items := map[string]AdventureItem{}
	for _, item := range r.Items {
		items[item.ID] = item
	}
	for _, npc := range r.NPCs {
		actor := npc.Actor
		stage := ""
		if current := AdventureProgressionStageFor(r, actor.Rank); current != nil {
			stage = current.Name
		}
		var encounters []map[string]any
		for _, op := range r.Opponents {
			if op.CharacterSourceKey != npc.CharacterSourceKey {
				continue
			}
			b := AdventureBattle{OpponentID: op.ID, Scale: op.Scale, Attack: op.Attack, Defense: op.Defense}
			bindAdventureEncounter(&b, "balance-snapshot", actor, r)
			weapon := adventureEnemyEquippedItem(&b, items, "weapon")
			armor := adventureEnemyEquippedItem(&b, items, "armor")
			defense := b.Defense
			if armor.Effect != "shield" {
				defense += armor.Power
			}
			target := NewAdventureActor(r)
			target.Rank = actor.Rank
			target.MaxHealth = 100 + 20*(actor.Rank-1)
			target.Health = target.MaxHealth
			target.MaxEnergy = 100 + 10*(actor.Rank-1)
			target.Energy = target.MaxEnergy
			target.Battle = &b
			// These are separate initial observations, not a forecast shown to the player.
			idle := chooseAdventureEnemyTechnique(r, &target, op, weapon, 0, 0)
			pressure := chooseAdventureEnemyTechnique(r, &target, op, weapon, max(1, 10+3*actor.Rank-defense), 0)
			encounters = append(encounters, map[string]any{"id": op.ID, "name": op.Name, "scale": op.Scale, "requires_presence": op.RequiresPresence, "base_attack": b.Attack, "base_defense": b.Defense, "owned_weapon": weapon.ID, "owned_weapon_power": weapon.Power, "owned_armor": armor.ID, "owned_armor_power": armor.Power, "effective_ordinary_attack": max(1, b.Attack-b.EnemyInjury/10) + weapon.Power, "effective_defense": defense, "choice_without_incoming": idle.Skill.ID, "choice_under_plain_attack": pressure.Skill.ID})
		}
		skillIDs := make([]string, 0, len(actor.Skills))
		for id := range actor.Skills {
			skillIDs = append(skillIDs, id)
		}
		sort.Strings(skillIDs)
		rows = append(rows, map[string]any{"family": r.Family, "source_key": npc.CharacterSourceKey, "rank": actor.Rank, "stage": stage, "current_actor": actor, "known_skill_ids": skillIDs, "encounters": encounters})
	}
	return rows
}

func TestNPCBalanceReportPreservesPeriodSnapshotsAndNamedRanks(t *testing.T) {
	for _, pack := range []string{"lanchuan-qunxia", "xinghai-689"} {
		r := authoredAdventureRules(t, pack)
		rows := adventureNPCBalanceSnapshots(t, r)
		if len(rows) != 60 {
			t.Fatalf("%s: expected all sixty frozen people", pack)
		}
		for _, row := range rows {
			actor := row["current_actor"].(AdventureActor)
			if row["rank"] != actor.Rank || actor.Health > actor.MaxHealth {
				t.Fatal("snapshot did not preserve current body")
			}
			if r.Family == "space" && actor.Rank >= 51 {
				t.Fatal("opening person was silently made future super-divine")
			}
			for _, enc := range row["encounters"].([]map[string]any) {
				if enc["requires_presence"] != true {
					t.Fatal("named report entry bypassed required presence")
				}
			}
		}
	}
}
