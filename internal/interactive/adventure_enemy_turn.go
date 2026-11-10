package interactive

// One authored NPC response, including its own inventory and energy costs.
// The round orchestrator owns incoming-force mitigation and final outcomes.
type adventureStrike struct {
	Power       int
	MentalPower int
	Kind        string
}

func adventureEnemyStrike(r AdventureRules, a *AdventureActor, items map[string]AdventureItem, outgoingForce, outgoingMental int, choice string) (adventureStrike, error) {
	b := a.Battle
	basePower := max(1, b.Attack-b.EnemyInjury/10)
	attackKind := b.AttackKind
	opponent := AdventureOpponent{}
	for _, candidate := range r.Opponents {
		if candidate.ID == b.OpponentID {
			opponent = candidate
			break
		}
	}
	weaponID := opponent.WeaponID
	if owned := b.EnemyEquipment[b.Scale+":weapon"]; owned != "" || b.EnemyActorID != "" {
		weaponID = owned
	}
	weapon := items[weaponID]
	if b.EnemyActorID != "" || b.EnemyEquipment[b.Scale+":weapon"] != "" {
		weapon = adventureEnemyEquippedItem(b, items, "weapon")
	}
	chosen := chooseAdventureEnemyTechnique(r, a, opponent, weapon, outgoingForce, outgoingMental)
	if selected, legal := adventureEnemyTechniqueForChoice(r, a, weapon, choice); choice != "" && legal {
		chosen = selected
	}
	skill, effects := chosen.Skill, chosen.Effects
	usesWeapon := skill.ID == "" || adventureEnemyUsesWeapon(effects)
	enemyPower, mentalPower, mentalWard := 0, 0, 0
	if usesWeapon {
		enemyPower = basePower + weapon.Power
		if err := consumeAdventureAmmunition(b.EnemyInventory, b.EnemyEquipment, weapon.Materials); err != nil {
			// An ordinary attack can fall back to an unarmed strike. A selected
			// technique has already reserved its complete materials and ammunition.
			enemyPower = max(1, 5+b.EnemyRank*3)
			attackKind = "physical"
		} else if weapon.AttackKind != "" {
			attackKind = weapon.AttackKind
		}
	}
	b.EnemyLastSeal = false
	if skill.ID == "" {
		return adventureStrike{Power: enemyPower, Kind: attackKind}, nil
	}
	selfHarm, selfInjury := adventureBacklashCost(AdventureActor{MaxHealth: b.MaxHealth, Skills: b.EnemySkillRanks}, skill.Backlash)
	for id, n := range skill.Materials {
		b.EnemyInventory[id] -= n
	}
	b.EnemyEnergy -= skill.Cost
	b.EnemyLastSkillID = skill.ID
	b.Health -= selfHarm
	b.EnemyInjury = min(100, b.EnemyInjury+selfInjury)
	remainingForce := outgoingForce
	for _, effect := range effects {
		power := effect.Power * adventureEnemySkillMultiplier(b, skill.ID, items)
		switch effect.Kind {
		case "attack":
			enemyPower += power
		case "reflect":
			if outgoingForce > 0 {
				if a.Rank > b.EnemyRank+1 {
					b.Health = max(0, b.Health-max(1, outgoingForce/2))
					b.EnemyInjury = min(100, b.EnemyInjury+max(1, outgoingForce/10))
				} else {
					reflected := remainingForce * min(90, power) / 100
					remainingForce -= reflected
					b.Health = min(b.MaxHealth, b.Health+reflected)
					a.Health = max(0, a.Health-reflected)
					a.Injury = min(100, a.Injury+reflected/5)
				}
			}
		case "electronic_disrupt":
			applyAdventureHardControl(&a.Control, &a.ControlRecovery)
		case "control":
			applyAdventureHardControl(&a.Control, &a.ControlRecovery)
			enemyPower += max(1, power/2)
		case "mental_attack":
			mentalPower += power
		case "mental_guard":
			mentalWard = max(mentalWard, min(90, power))
		case "drain":
			drainAdventurePlayer(a, skill, power)
		case "poison":
			a.Poison = max(a.Poison, power/3)
		case "seal":
			a.RushBlocked = 1
			b.EnemyLastSeal = true
		case "heal", "repair":
			b.Health = min(b.MaxHealth, b.Health+power)
		case "guard", "pilot":
			// A defensive action can turn aside actual force, never heal old
			// wounds or erase poison. It no longer grants a free ordinary strike.
			blocked := min(remainingForce, power)
			remainingForce -= blocked
			b.Health = min(b.MaxHealth, b.Health+blocked)
		case "energy":
			b.EnemyEnergy = min(b.EnemyMaxEnergy, b.EnemyEnergy+power)
		case "antidote":
			b.EnemyPoison = 0
		case "recoil":
			b.Health = max(1, b.Health-max(1, b.MaxHealth*effect.Power/100))
		}
	}
	if outgoingMental > 0 && mentalWard > 0 {
		blocked := outgoingMental - adventureMentalDamage(outgoingMental, mentalWard)
		b.Health = min(b.MaxHealth, b.Health+blocked)
	}
	return adventureStrike{Power: enemyPower, MentalPower: mentalPower, Kind: attackKind}, nil
}

// A snapshot slot is not ownership. Only a genuinely owned, matching piece
// contributes equipment power; bound NPCs never inherit an authored fallback.
func adventureEnemyEquippedItem(b *AdventureBattle, items map[string]AdventureItem, kind string) AdventureItem {
	item := items[b.EnemyEquipment[b.Scale+":"+kind]]
	if item.ID == "" || item.Kind != kind || item.Scale != b.Scale || b.EnemyInventory[item.ID] < 1 {
		return AdventureItem{}
	}
	return item
}
