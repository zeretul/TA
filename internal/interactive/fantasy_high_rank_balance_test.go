package interactive

import (
	"fmt"
	"testing"
)

// Explicitly provisioned combat fixtures isolate rank, learned mastery and owned
// armor. They are not a claim that this gear or mastery was naturally acquired.
// The player repeats one paid method, guarding only when actually controlled;
// there is no forced enemy choice, healing, replenishment or free fallback.
func TestFantasyHighRankCountersNeedPreparation(t *testing.T) {
	for _, tc := range []struct {
		source, method, armor, want string
		rank, mastery               int
	}{
		{"sg-p07", "sg-seal", "sg-courtguard", "defeat", 18, 10},
		{"sg-p56", "sg-seal", "sg-courtguard", "defeat", 18, 10},
		{"sg-p07", "sg-seal", "sg-courtguard", "defeat", 24, 10},
		{"sg-p56", "sg-seal", "sg-courtguard", "defeat", 24, 10},
		{"sg-p07", "sg-seal", "sg-courtguard", "victory", 25, 10},
		{"sg-p07", "sg-seal", "sg-padded", "defeat", 27, 10},
		{"sg-p07", "sg-seal", "sg-padded", "victory", 28, 10},
		{"sg-p56", "sg-seal", "sg-courtguard", "defeat", 28, 10},
		{"sg-p56", "sg-seal", "sg-courtguard", "victory", 29, 10},
		{"sg-p07", "sg-seal", "sg-padded", "victory", 30, 10},
		{"sg-p07", "sg-seal", "sg-courtguard", "victory", 30, 10},
		{"sg-p56", "sg-seal", "sg-padded", "defeat", 30, 10},
		{"sg-p56", "sg-seal", "sg-courtguard", "victory", 30, 10},
		{"sg-p56", "sg-banner", "sg-padded", "victory", 30, 3},
		{"sg-p56", "sg-banner", "sg-courtguard", "victory", 30, 5},
		{"sg-p56", "sg-banner", "sg-courtguard", "defeat", 24, 5},
	} {
		name := fmt.Sprintf("%s/rank%d/mastery%d/%s/%s", tc.source, tc.rank, tc.mastery, tc.armor, tc.method)
		t.Run(name, func(t *testing.T) {
			r, _ := fantasyOrigin(t, "sg-start-road")
			a := NewAdventureActor(r)
			a.Rank = tc.rank
			a.MaxHealth, a.MaxEnergy = 100+20*(tc.rank-1), 100+10*(tc.rank-1)
			a.Health, a.Energy = a.MaxHealth, a.MaxEnergy
			var method AdventureSkill
			for _, s := range r.Skills {
				if s.ID == tc.method {
					method = s
				}
			}
			if method.ID == "" || method.RequiredRank > tc.rank {
				t.Fatal("fixture requested an unqualified method")
			}
			a.Skills[method.ID] = tc.mastery
			if method.Prerequisite != "" {
				a.Skills[method.Prerequisite] = tc.mastery
			}
			if method.RequiredTool != "" {
				a.Inventory[method.RequiredTool] = 1
			}
			for id := range method.Materials {
				a.Inventory[id] = 10
			}
			weapon := "sg-travel-blade"
			if method.ID == "sg-seal" {
				weapon = "sg-focus-staff"
			}
			a.Inventory[weapon], a.Inventory[tc.armor] = 1, 1
			a.Equipment["person:weapon"], a.Equipment["person:armor"] = weapon, tc.armor
			if err := ValidateAdventureActor(r, a); err != nil {
				t.Fatal(err)
			}
			started, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "sg-encounter-" + tc.source[3:]}, "prepared-start")
			if err != nil {
				t.Fatal(err)
			}
			a = started
			bound := false
			for _, n := range r.NPCs {
				if n.CharacterSourceKey == tc.source {
					bindAdventureEncounter(a.Battle, tc.source, n.Actor, r)
					bound = true
				}
			}
			if !bound {
				t.Fatal("fixture did not bind the actual authored person")
			}
			spent, casts, rounds := 0, 0, 0
			for a.Battle.Status == "active" && rounds < 40 {
				in := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: method.ID}
				if a.Control > 0 {
					in.Choice, in.SkillID = "guard", ""
				}
				next, _, e := ResolveAdventureAction(r, a, in, fmt.Sprintf("prepared-%d", rounds))
				if e != nil {
					t.Fatal("prepared method became unaffordable or invalid", e)
				}
				if in.Choice == "skill" {
					casts++
				}
				spent += a.Energy - next.Energy
				a = next
				rounds++
			}
			if a.Battle.Status != tc.want || spent != casts*method.Cost {
				t.Fatalf("rank/gear/counter boundary changed: want=%s got=%s hp=%d enemy=%d spent=%d casts=%d", tc.want, a.Battle.Status, a.Health, a.Battle.Health, spent, casts)
			}
			for id, n := range method.Materials {
				if a.Inventory[id] != 10-casts*n {
					t.Fatal("prepared tactic ignored its finite reagents")
				}
			}
			t.Logf("result=%s rounds=%d hp=%d enemy_hp=%d energy_spent=%d casts=%d", a.Battle.Status, rounds, a.Health, a.Battle.Health, spent, casts)
		})
	}
}
