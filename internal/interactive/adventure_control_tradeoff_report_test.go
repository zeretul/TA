package interactive

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// Limited policy comparison, not a universal best-strategy search. Learned
// mastery and operating inventory are explicit staged grants; the generated
// variant must nevertheless pass the real registration/derivation consumer.
func TestAdventureControlTradeoffReport(t *testing.T) {
	path := os.Getenv("TA_CONTROL_REPORT")
	if path == "" {
		t.Skip("set TA_CONTROL_REPORT")
	}
	var rows []map[string]any
	for _, target := range []struct {
		source string
		rank   int
	}{{"xinghai-p02", 44}, {"xinghai-p05", 49}, {"xinghai-p08", 49}, {"xinghai-advanced-trial-6", 41}, {"xinghai-advanced-trial-7", 51}} {
		for _, policy := range []string{"strike", "psychic", "psychic_then_strike", "generated", "generated_then_strike"} {
			if target.rank < 42 && (policy == "generated" || policy == "generated_then_strike") {
				continue
			}
			r, a := npcTacticsBalanceFixture(t, "xinghai-689", target.source, target.rank, "xinghai-psychic-a")
			finisher := "xinghai-advanced-strike-6"
			if target.rank >= 51 {
				finisher = "xinghai-advanced-strike-7"
			}
			a.Skills[finisher] = 10
			opener := "xinghai-psychic-a"
			cost := 62
			studyHours := 0
			if policy == "generated" || policy == "generated_then_strike" {
				d := AdventureGeneratedDefinition{Category: "skill", TemplateID: "xinghai-psychic-a", Name: "Focused Psychic Burst", Source: "Combine the already learned psychic attack", Design: &AdventureGeneratedDesign{Effects: []AdventureEffect{{Kind: "attack", Power: 100}, {Kind: "control", Power: 1}}}}
				battle := a.Battle
				a.Battle = nil
				var out AdventureOutcome
				var err error
				a, out, err = ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &d, Hours: 96}, "control-tradeoff-register")
				if err != nil {
					t.Fatal(err)
				}
				studyHours = int(out.DurationSeconds / 3600)
				a.Battle = battle
				opener, _ = d.id()
				a.Skills[opener] = 10
				catalog, err := adventureRulesForActor(r, a)
				if err != nil {
					t.Fatal(err)
				}
				for _, s := range catalog.Skills {
					if s.ID == opener {
						cost = s.Cost
					}
				}
			}
			start := a
			replies := 0
			rounds := 0
			for rounds < 30 && a.Battle.Status == "active" {
				skill := opener
				if policy == "strike" || rounds > 0 && (policy == "psychic_then_strike" || policy == "generated_then_strike") {
					skill = finisher
				}
				next, _, err := ResolveAdventureActionAt(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: skill}, fmt.Sprintf("control-tradeoff-%d", rounds), int64(rounds*20))
				if err != nil {
					next, _, err = ResolveAdventureActionAt(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, fmt.Sprintf("control-tradeoff-%d", rounds), int64(rounds*20))
				}
				if err != nil {
					t.Fatal(err)
				}
				if next.Battle.EnemyLastSkillID != "" {
					replies++
				}
				a = next
				rounds++
			}
			rows = append(rows, map[string]any{"target": target.source, "rank": target.rank, "policy": policy, "opener": opener, "finisher": finisher, "opener_cost": cost, "generated_registration_hours": studyHours, "mastery": 10, "rounds": rounds, "status": a.Battle.Status, "player_health": a.Health, "max_health": a.MaxHealth, "enemy_health": a.Battle.Health, "enemy_max_health": start.Battle.MaxHealth, "energy_spent": start.Energy - a.Energy, "observed_skill_replies": replies})
		}
	}
	raw, err := json.MarshalIndent(map[string]any{"assumptions": "Staged mastered skills and current-stage equipment, actual bound NPC snapshots. Generated variant uses the real registration consumer; subsequent mastery10 is an explicit test grant, not free natural mastery. Two policies mix a control opener with strongest available authored strike. No optimal-play claim.", "rows": rows}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(raw, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d control tradeoff cases", len(rows))
}
