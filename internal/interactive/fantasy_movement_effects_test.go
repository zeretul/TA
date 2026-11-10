package interactive

import "testing"

// These fixtures use unmodified frozen world rules, actual anonymous opponents,
// origin equipment and each movement method's minimum authored rank. Only the
// qualifying rank, prerequisite and owned manual/material are provisioned.
// Learning, escape and pursuit execute their real deterministic consumers; this
// is not an earned journey, Store encounter, natural AI or browser claim.
// Not compiled or executed by the independent reviewer.
func TestFantasyThreeMovementMethodsChangeEscapeAndPursuit(t *testing.T) {
	for _, tc := range []struct {
		id                                                               string
		plainEscape, skilledEscape, plainEnemyEscape, skilledEnemyEscape int
	}{
		{"sg-quietstep", 15, 51, 100, 69},
		{"sg-smoketurn", 15, 75, 100, 45},
		{"sg-unseenroad", 36, 100, 84, 0},
	} {
		t.Run(tc.id, func(t *testing.T) {
			r, a := fantasyOrigin(t, "sg-start-road")
			var skill AdventureSkill
			for _, s := range r.Skills {
				if s.ID == tc.id {
					skill = s
				}
			}
			if skill.ID == "" || skill.Kind != "pilot" || skill.Mobility <= 0 {
				t.Fatal("missing actual movement method")
			}
			a.Rank = skill.RequiredRank
			if skill.Prerequisite != "" {
				a.Skills[skill.Prerequisite] = 1
			}
			a.Inventory[skill.RequiredItem] = 1
			if skill.RequiredTool != "" {
				a.Inventory[skill.RequiredTool] = 1
			}
			for id, n := range skill.Materials {
				a.Inventory[id] = n
			}
			var err error
			a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "learn", SkillID: skill.ID, Hours: skill.TrainingHours}, "movement-learn")
			if err != nil || a.Skills[skill.ID] != 1 || a.Rank != skill.RequiredRank {
				t.Fatalf("minimum-rank actual learning failed: %v", err)
			}

			for _, kind := range []string{"escape", "pursue"} {
				t.Run(kind, func(t *testing.T) {
					opponent, enemyChoice := "sg-crypt-warden", "ordinary"
					if kind == "pursue" {
						opponent, enemyChoice = "sg-op23", "escape:sg-smoketurn"
					}
					start, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: opponent}, "movement-start")
					if err != nil {
						t.Fatal(err)
					}
					plain, _, err := ResolveAdventureAction(r, start, AdventureAction{Action: "battle_round", Choice: kind, enemyChoice: enemyChoice}, "movement-plain")
					if err != nil {
						t.Fatal(err)
					}
					paid, _, err := ResolveAdventureAction(r, start, AdventureAction{Action: "battle_round", Choice: kind, SkillID: skill.ID, enemyChoice: enemyChoice}, "movement-paid")
					if err != nil {
						t.Fatal(err)
					}
					if plain.Energy != start.Energy || paid.Energy != start.Energy-skill.Cost {
						t.Fatal("movement effort was missing or charged twice")
					}
					for id, n := range skill.Materials {
						if plain.Inventory[id] != start.Inventory[id] || paid.Inventory[id] != start.Inventory[id]-n {
							t.Fatal("movement did not consume only its actual material", id)
						}
					}
					if paid.Currency != start.Currency || paid.Experience != start.Experience || paid.Defeated[opponent] || paid.Battle.Health != start.Battle.Health {
						t.Fatal("movement minted victory, rewards or a free strike")
					}
					if kind == "escape" {
						if plain.Battle.PlayerRetreatProgress != tc.plainEscape || paid.Battle.PlayerRetreatProgress != tc.skilledEscape || paid.Battle.PlayerRetreatProgress <= plain.Battle.PlayerRetreatProgress {
							t.Fatalf("mobility did not improve actual escape: plain=%d skilled=%d", plain.Battle.PlayerRetreatProgress, paid.Battle.PlayerRetreatProgress)
						}
						if (paid.Battle.Status == "escaped") != (tc.skilledEscape == 100) {
							t.Fatal("escape completion disagreed with actual progress")
						}
					} else {
						if plain.Battle.EnemyRetreatProgress != tc.plainEnemyEscape || paid.Battle.EnemyRetreatProgress != tc.skilledEnemyEscape || paid.Battle.EnemyRetreatProgress >= plain.Battle.EnemyRetreatProgress {
							t.Fatalf("mobility did not improve actual interception: plain=%d skilled=%d", plain.Battle.EnemyRetreatProgress, paid.Battle.EnemyRetreatProgress)
						}
						if paid.Battle.Status != "active" || paid.Health != start.Health || paid.Battle.EnemyEnergy != start.Battle.EnemyEnergy-8 || paid.Battle.EnemyInventory["sg-smoke"] != start.Battle.EnemyInventory["sg-smoke"]-1 || paid.Battle.EnemyLastSkillID != "sg-smoketurn" {
							t.Fatal("authored opponent did not pay its actual escape cost or received a free attack")
						}
					}
				})
			}
		})
	}
}
