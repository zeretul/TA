package interactive

import "fmt"

// Backlash is the skill's own full-force price, distinct from energy and from
// harm an opponent causes. Foundation mitigation never removes all self-harm.
type AdventureSkillBacklash struct {
	HealthPercent          int      `json:"health_percent"`
	Injury                 int      `json:"injury"`
	FoundationSkills       []string `json:"foundation_skills,omitempty"`
	FoundationRank         int      `json:"foundation_rank,omitempty"`
	MitigatedHealthPercent int      `json:"mitigated_health_percent,omitempty"`
	MitigatedInjury        int      `json:"mitigated_injury,omitempty"`
}

func validateAdventureBacklash(s AdventureSkill, skills map[string]bool) error {
	b := s.Backlash
	if b == nil {
		return nil
	}
	if b.HealthPercent < 1 || b.HealthPercent > 50 || b.Injury < 1 || b.Injury > 50 || len(b.FoundationSkills) > 8 {
		return fmt.Errorf("invalid full-force backlash %s", s.ID)
	}
	if len(b.FoundationSkills) == 0 {
		if b.FoundationRank != 0 || b.MitigatedHealthPercent != 0 || b.MitigatedInjury != 0 {
			return fmt.Errorf("foundation mitigation requires an actual foundation %s", s.ID)
		}
	} else if b.FoundationRank < 1 || b.FoundationRank > 10 || b.MitigatedHealthPercent < 1 || b.MitigatedHealthPercent > b.HealthPercent || b.MitigatedInjury < 1 || b.MitigatedInjury > b.Injury {
		return fmt.Errorf("invalid foundation mitigation %s", s.ID)
	}
	for _, id := range b.FoundationSkills {
		if !skills[id] {
			return fmt.Errorf("unknown protective foundation %s", id)
		}
	}
	return nil
}

func adventureBacklashCost(a AdventureActor, b *AdventureSkillBacklash) (int, int) {
	if b == nil {
		return 0, 0
	}
	percent, injury := b.HealthPercent, b.Injury
	for _, id := range b.FoundationSkills {
		if a.Skills[id] >= b.FoundationRank {
			percent, injury = b.MitigatedHealthPercent, b.MitigatedInjury
			break
		}
	}
	return max(1, a.MaxHealth*percent/100), injury
}

func adventureEnemyKnows(r AdventureRules, b *AdventureBattle, id string) bool {
	if b.EnemyActorID != "" {
		return b.EnemySkillRanks[id] > 0
	}
	for _, enemy := range r.Opponents {
		if enemy.ID == b.OpponentID {
			for _, skillID := range enemy.SkillIDs {
				if skillID == id {
					return true
				}
			}
		}
	}
	return false
}

func drainAdventureEnemy(r AdventureRules, a *AdventureActor, s AdventureSkill, power int) {
	b := a.Battle
	for _, id := range s.DrainCounterSkills {
		if adventureEnemyKnows(r, b, id) {
			a.Health = max(0, a.Health-max(1, a.MaxHealth/10))
			a.Injury = min(100, a.Injury+10)
			return
		}
	}
	amount := max(0, min(power, b.EnemyEnergy, a.MaxEnergy-a.Energy))
	b.EnemyEnergy -= amount
	a.Energy += amount
	if amount > 0 {
		a.Injury = min(100, a.Injury+max(1, amount/20))
	}
}

func drainAdventurePlayer(a *AdventureActor, s AdventureSkill, power int) {
	b := a.Battle
	for _, id := range s.DrainCounterSkills {
		if a.Skills[id] > 0 {
			b.Health = max(0, b.Health-max(1, b.MaxHealth/10))
			b.EnemyInjury = min(100, b.EnemyInjury+10)
			return
		}
	}
	amount := max(0, min(power, a.Energy, b.EnemyMaxEnergy-b.EnemyEnergy))
	a.Energy -= amount
	b.EnemyEnergy += amount
	if amount > 0 {
		b.EnemyInjury = min(100, b.EnemyInjury+max(1, amount/20))
	}
}

func settleAdventureVictory(r AdventureRules, a *AdventureActor) {
	b := a.Battle
	if b.Status != "active" || b.Health > 0 || a.Health <= 0 {
		return
	}
	b.Status = "victory"
	a.Defeated[b.OpponentID] = true
	if b.CharacterSourceKey != "" {
		a.Defeated[b.CharacterSourceKey] = true
	}
	for _, enemy := range r.Opponents {
		if enemy.ID == b.OpponentID {
			a.Currency += enemy.RewardCurrency
			a.Experience += enemy.RewardExperience
		}
	}
}
