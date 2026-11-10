package interactive

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

// Separates raw burst damage from canceling a reply. It deliberately stages
// mastery/gear and uses real generation registration, never an acquisition claim.
func TestAdventureControlIsolationReport(t *testing.T) {
	path := os.Getenv("TA_CONTROL_ISOLATION_REPORT")
	if path == "" {
		t.Skip("set TA_CONTROL_ISOLATION_REPORT")
	}
	before := balanceSourceHashes(t)
	source, err := os.ReadFile("../storypack/assets/xinghai-689.json")
	if err != nil {
		t.Fatal(err)
	}
	rows := []map[string]any{}
	for _, target := range []struct {
		id   string
		rank int
	}{{"xinghai-p02", 44}, {"xinghai-p05", 49}, {"xinghai-p08", 49}, {"xinghai-advanced-trial-7", 51}} {
		for _, mastery := range []int{6, 10} {
			for _, equipped := range []bool{false, true} {
				for _, control := range []bool{false, true} {
					r, a := npcTacticsBalanceFixture(t, "xinghai-689", target.id, target.rank, "xinghai-psychic-a")
					a.Skills["xinghai-psychic-a"] = 2
					if !equipped {
						a.Equipment = map[string]string{}
					}
					effects := []AdventureEffect{{Kind: "attack", Power: 100}}
					if control {
						effects = append(effects, AdventureEffect{Kind: "control", Power: 1})
					}
					d := AdventureGeneratedDefinition{Category: "skill", TemplateID: "xinghai-psychic-a", Name: fmt.Sprintf("Isolated Burst %t", control), Source: "Refine the actual learned foundation to isolate damage and control", Design: &AdventureGeneratedDesign{Effects: effects}}
					battle := a.Battle
					a.Battle = nil
					var work AdventureOutcome
					a, work, err = ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &d, Hours: 96}, "isolation-register")
					if err != nil {
						t.Fatal(err)
					}
					id, _ := d.id()
					a.Skills[id] = mastery
					a.Battle = battle
					catalog, err := adventureRulesForActor(r, a, id)
					if err != nil {
						t.Fatal(err)
					}
					var actual AdventureSkill
					for _, s := range catalog.Skills {
						if s.ID == id {
							actual = s
						}
					}
					if actual.ID == "" || !reflect.DeepEqual(actual.Effects, effects) {
						t.Fatal("generated comparison did not retain its declared effects")
					}
					start := a
					steps := []map[string]any{}
					for round := 0; round < 30 && a.Battle.Status == "active"; round++ {
						prior := a
						in := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: id}
						next, _, e := ResolveAdventureActionAt(r, prior, in, fmt.Sprintf("control-isolation-%d", round), int64(round*20))
						rejected := ""
						if e != nil {
							rejected = e.Error()
							if !reflect.DeepEqual(next, prior) {
								t.Fatal("refused comparison mutated its input")
							}
							in.Choice, in.SkillID = "attack", ""
							next, _, e = ResolveAdventureActionAt(r, prior, in, fmt.Sprintf("control-isolation-%d", round), int64(round*20))
						}
						if e != nil {
							t.Fatal(e)
						}
						steps = append(steps, map[string]any{"round": round + 1, "requested_skill": id, "accepted_choice": in.Choice, "rejection": rejected, "player_control_before": prior.Control, "player_health_before": prior.Health, "player_health_after": next.Health, "player_energy_spent": prior.Energy - next.Energy, "enemy_health_before": prior.Battle.Health, "enemy_health_after": next.Battle.Health, "enemy_last_action": next.Battle.EnemyLastAction, "enemy_last_skill": next.Battle.EnemyLastSkillID, "enemy_energy_spent": prior.Battle.EnemyEnergy - next.Battle.EnemyEnergy, "enemy_control_after": next.Battle.EnemyControl, "enemy_control_recovery_after": next.Battle.EnemyControlRecovery})
						a = next
					}
					rows = append(rows, map[string]any{"target": target.id, "rank": target.rank, "mastery": mastery, "equipped": equipped, "with_control": control, "effects": actual.Effects, "cost": actual.Cost, "required_rank": actual.RequiredRank, "training_hours_base": actual.TrainingHours, "actual_registration_hours": work.DurationSeconds / 3600, "player_equipment": start.Equipment, "enemy_equipment": start.Battle.EnemyEquipment, "enemy_max_health": start.Battle.MaxHealth, "status": a.Battle.Status, "rounds": len(steps), "player_health": a.Health, "max_health": a.MaxHealth, "enemy_health": a.Battle.Health, "energy_spent": start.Energy - a.Energy, "steps": steps})
				}
			}
		}
	}
	current, err := os.ReadFile("../storypack/assets/xinghai-689.json")
	if err != nil || !reflect.DeepEqual(source, current) || !reflect.DeepEqual(before, balanceSourceHashes(t)) {
		t.Fatal("comparison source changed during execution")
	}
	data, err := json.MarshalIndent(map[string]any{"format": 1, "assumptions": []string{"Three actual same-rank named people and one stage-seven trial; authored current bodies, mastery and equipment are retained.", "Player foundation mastery two permits actual generation registration; derived mastery six/ten and gear are staged grants, not natural acquisition proofs.", "Pure attack100 and attack100/control1 retain their own validated costs, rank gates and registration/study hours. Neither is forced to a fictitious equal price. Control1 retains its real small additional damage, so the paired raw attack component, not total damage, is identical.", "Repeated generated skill with ordinary fallback, no replenishment or optimal policy search. Player-first fatal attacks have no enemy reply under the existing rule. Active at 30 rounds remains unresolved."}, "source_sha256": before, "author_sha256": fmt.Sprintf("%x", sha256.Sum256(source)), "rows": rows}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d damage/control isolation cases", len(rows))
}
