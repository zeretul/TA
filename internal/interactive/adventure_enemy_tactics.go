package interactive

// An opponent uses only its authored repertoire until it is bound to an actual
// NPC. Once bound, that person's current learned skills become the source of
// truth, so later study changes options rather than only increasing statistics.
type adventureEnemyTechnique struct {
	Skill   AdventureSkill
	Effects []AdventureEffect
	Score   int
}

func chooseAdventureEnemyTechnique(r AdventureRules, a *AdventureActor, opponent AdventureOpponent, weapon AdventureItem, outgoingForce, outgoingMental int) adventureEnemyTechnique {
	items := map[string]AdventureItem{}
	for _, item := range r.Items {
		items[item.ID] = item
	}
	b := a.Battle
	// A response must improve on the available ordinary attack, not merely
	// have a positive nominal benefit. In particular, repeated partial guards
	// must not make a dangerous opponent pass every turn until it dies.
	ordinaryPower := max(1, b.Attack-b.EnemyInjury/10) + weapon.Power
	for id, quantity := range weapon.Materials {
		if b.EnemyInventory[id] < quantity {
			ordinaryPower = max(1, 5+b.EnemyRank*3)
			break
		}
	}
	best := adventureEnemyTechnique{Score: ordinaryPower}
	for _, option := range adventureLegalEnemyTechniques(r, a, weapon) {
		skill, effects := option.Skill, option.Effects
		selfHarm, _ := adventureBacklashCost(AdventureActor{MaxHealth: b.MaxHealth, Skills: b.EnemySkillRanks}, skill.Backlash)
		score, supported := adventureEnemyTechniqueScore(b, effects, adventureEnemySkillMultiplier(b, skill.ID, items), outgoingForce, outgoingMental, skill.Cost, a.ControlRecovery == 0)
		if !supported {
			continue
		}
		if adventureEnemyUsesWeapon(effects) {
			score += weapon.Power
		}
		score -= selfHarm
		// Ordinary attack remains the fallback. A support action that does
		// nothing must not consume a turn, energy, or materials indefinitely.
		if score <= 0 {
			continue
		}
		if score > best.Score || score == best.Score && best.Skill.ID != "" && (skill.Cost < best.Skill.Cost || skill.Cost == best.Skill.Cost && skill.ID < best.Skill.ID) {
			best = adventureEnemyTechnique{Skill: skill, Effects: effects, Score: score}
		}
	}
	return best
}

// Only already observable pressure and the opponent's own state enter the
// utility estimate. The player's hidden skills, inventory and pending command
// are not inspected, so acquiring a counter does not trigger clairvoyant play.
func adventureEnemyTechniqueScore(b *AdventureBattle, effects []AdventureEffect, level, outgoingForce, outgoingMental, energyCost int, controlAvailable bool) (int, bool) {
	score := 0
	mentalWard := 0
	energy := b.EnemyEnergy - energyCost
	recoversEnergy := false
	for _, effect := range effects {
		power := effect.Power * level
		switch effect.Kind {
		case "attack":
			score += max(1, b.Attack-b.EnemyInjury/10) + power
		case "heal":
			if b.Scale != "person" {
				return 0, false
			}
			recovered := min(b.MaxHealth-b.Health, power)
			score += recovered
			if b.Health*2 <= b.MaxHealth {
				score += recovered
			}
		case "repair":
			if b.Scale == "person" {
				return 0, false
			}
			recovered := min(b.MaxHealth-b.Health, power)
			score += recovered
			if b.Health*2 <= b.MaxHealth {
				score += recovered
			}
		case "energy":
			recoversEnergy = true
			energy = min(b.EnemyMaxEnergy, energy+power)
		case "antidote":
			score += b.EnemyPoison * 3
		case "reflect":
			if b.Scale != "person" {
				return 0, false
			}
			score += outgoingForce * min(90, power) / 50
		case "guard", "pilot":
			blocked := min(outgoingForce, power)
			score += blocked
			// Complete protection can buy a critical turn. Partial mitigation
			// is useful, but is not worth twice its actual prevented damage.
			if power >= outgoingForce && b.Health <= outgoingForce*2 {
				score += blocked
			}
		case "poison":
			if b.Scale != "person" {
				return 0, false
			}
			score += max(0, power/3) * 2
		case "control":
			score += max(1, power/2)
			if controlAvailable {
				score += max(1, b.Attack)
			}
		case "electronic_disrupt":
			if controlAvailable {
				score += max(1, b.Attack)
			}
		case "seal", "drain":
			score += power
		case "mental_attack":
			if b.Scale != "person" {
				return 0, false
			}
			score += power
		case "mental_guard":
			mentalWard = max(mentalWard, min(90, power))
		case "recoil":
			score -= b.MaxHealth * effect.Power / 100
		default:
			// Noncombat work and unsupported special actions must not cause a
			// whole player round to fail just because an NPC learned them.
			return 0, false
		}
	}
	blockedMental := outgoingMental - adventureMentalDamage(outgoingMental, mentalWard)
	score += blockedMental
	if b.Health <= outgoingMental*2 {
		score += blockedMental
	}
	if recoversEnergy {
		// Recovery must create a net gain after paying for the skill itself.
		score += energy - b.EnemyEnergy
	}
	return max(0, score), true
}

func adventureEnemyUsesWeapon(effects []AdventureEffect) bool {
	for _, effect := range effects {
		if effect.Kind == "attack" {
			return true
		}
	}
	return false
}

// A real equipped computation aid scales a mounted skill just as it does for
// the protagonist. Owning an unequipped aid is not enough.
func adventureEnemySkillMultiplier(b *AdventureBattle, skillID string, items map[string]AdventureItem) int {
	level := max(1, b.EnemySkillRanks[skillID])
	if b.Scale != "person" {
		item := items[b.EnemyEquipment[b.Scale+":tool"]]
		if item.Effect == "compute" && b.EnemyInventory[item.ID] > 0 {
			level *= item.Multiplier
		}
	}
	return level
}
