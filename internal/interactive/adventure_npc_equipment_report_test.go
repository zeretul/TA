package interactive

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// Two controlled one-round probes isolate owned NPC equipment while leaving
// their current rank, body, learned mastery and inventory unchanged. The player
// has deliberately enlarged pools so this is not a victory/acquisition claim.
func TestNPCPeriodEquipmentAblationReport(t *testing.T) {
	path := os.Getenv("TA_NPC_GEAR_REPORT")
	if path == "" {
		t.Skip("set TA_NPC_GEAR_REPORT")
	}
	var rows []map[string]any
	hashes := map[string]string{}
	for _, pack := range []string{"lanchuan-qunxia", "xinghai-689"} {
		source, err := os.ReadFile("../storypack/assets/" + pack + ".json")
		if err != nil {
			t.Fatal(err)
		}
		hashes[pack] = fmt.Sprintf("%x", sha256.Sum256(source))
		r := authoredAdventureRules(t, pack)
		items := map[string]AdventureItem{}
		for _, item := range r.Items {
			items[item.ID] = item
		}
		for _, op := range r.Opponents {
			if !op.RequiresPresence {
				continue
			}
			var person AdventureActor
			for _, npc := range r.NPCs {
				if npc.CharacterSourceKey == op.CharacterSourceKey {
					person = npc.Actor
					break
				}
			}
			if person.Rank == 0 {
				t.Fatal("missing actual named person")
			}
			for _, policy := range []string{"guard", "stage_balanced_attack"} {
				for _, loadout := range []string{"owned_equipment", "unequipped_only"} {
					raw, _ := json.Marshal(person)
					var npc AdventureActor
					if err := json.Unmarshal(raw, &npc); err != nil {
						t.Fatal(err)
					}
					if loadout == "unequipped_only" {
						npc.Equipment = map[string]string{}
					}
					a := NewAdventureActor(r)
					a.Rank = person.Rank
					a.Health = 10000
					a.MaxHealth = 10000
					a.Energy = 1000
					a.MaxEnergy = 1000
					input := AdventureAction{Action: "battle_round", Choice: "guard"}
					prefix := "xinghai"
					if r.Family == "wuxia" {
						prefix = "lanchuan"
					}
					for i, stage := range r.Progression.Stages {
						if i > 0 && stage.MinRank <= a.Rank {
							skillID := prefix + "-advanced-balance-" + strconv.Itoa(i)
							a.Skills[skillID] = AdventureSkillMasteryCap(r)
							if policy != "guard" {
								input.Choice = "skill"
								input.SkillID = skillID
							}
							for slot, kind := range map[string]string{"person:weapon": "weapon", "person:armor": "armor"} {
								id := prefix + "-advanced-" + kind + "-" + strconv.Itoa(i)
								a.Inventory[id] = 1
								a.Equipment[slot] = id
								if required := items[id].RequiredSkill; required != "" {
									a.Skills[required] = 1
								}
								for material := range items[id].Materials {
									a.Inventory[material] = 100
								}
							}
						}
					}
					if policy != "guard" && input.SkillID == "" {
						input.Choice = "attack"
					}
					started, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: op.ID}, "npc-equipment-probe")
					if err != nil {
						t.Fatal(err)
					}
					bindAdventureEncounter(started.Battle, "period-person", npc, r)
					next, _, err := ResolveAdventureActionAt(r, started, input, "npc-equipment-probe", 0)
					if err != nil {
						t.Fatal(err)
					}
					spent := map[string]int{}
					for id, quantity := range person.Inventory {
						if n := quantity - next.Battle.EnemyInventory[id]; n > 0 {
							spent[id] = n
						}
					}
					if skill := next.Battle.EnemyLastSkillID; skill != "" && person.Skills[skill] <= 0 {
						t.Fatal("ablation invented an enemy skill")
					}
					rows = append(rows, map[string]any{"family": r.Family, "source_key": op.CharacterSourceKey, "name": op.Name, "rank": person.Rank, "policy": policy, "player_skill": input.SkillID, "player_mastery": AdventureSkillMasteryCap(r), "npc_loadout": loadout, "player_health_lost": a.Health - next.Health, "npc_health_lost": person.Health - next.Battle.Health, "npc_skill": next.Battle.EnemyLastSkillID, "npc_energy_spent": person.Energy - next.Battle.EnemyEnergy, "npc_materials_spent": spent, "status": next.Battle.Status})
				}
			}
		}
		after, err := os.ReadFile("../storypack/assets/" + pack + ".json")
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(after)) != hashes[pack] {
			t.Fatal("catalog changed during equipment probe")
		}
	}
	data, err := json.MarshalIndent(map[string]any{"format": 1, "assumption": "One-round ablation only. Player pools explicitly staged at 10000 health/1000 energy; ranks and current-stage gear/learned mastery stay identical between paired runs. NPC body/skills/inventory are real frozen period data. Unequipped means carried gear is not worn; ownership is not removed. This does not establish presence or grant loot.", "input_sha256": hashes, "rows": rows}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d owned-equipment ablations", len(rows))
}
