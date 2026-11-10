package interactive

import "fmt"

func adventureMentalSource(r AdventureRules, id string) bool {
	for _, enemy := range r.Opponents {
		if enemy.ID == id && enemy.MentalAttack {
			return true
		}
	}
	return false
}

func restoreAdventureMentalState(r AdventureRules, a *AdventureActor, item AdventureItem) error {
	if a.MentalCorruption < 1 || !adventureMentalSource(r, a.MentalExposureID) {
		return fmt.Errorf("mental recovery requires a registered actual corruption source")
	}
	remaining, present := a.ItemCharges[item.ID]
	if !present {
		remaining = item.MaxCharges
	}
	if remaining < 1 {
		return fmt.Errorf("the item has no remaining charges")
	}
	if a.ItemCharges == nil {
		a.ItemCharges = map[string]int{}
	}
	remaining--
	if remaining == 0 {
		delete(a.ItemCharges, item.ID)
		a.Inventory[item.ID]--
	} else {
		a.ItemCharges[item.ID] = remaining
	}
	a.MentalCorruption = max(0, a.MentalCorruption-item.Power)
	a.MentalShock = 0
	if a.MentalCorruption == 0 {
		a.MentalExposureID = ""
	}
	return nil
}

func applyAdventureMentalThreat(a *AdventureActor, guardPercent, incoming int) int {
	b := a.Battle
	if !b.MentalAttack || incoming < 1 {
		return incoming
	}
	if guardPercent > 0 && b.EnemyRank <= a.Rank+1 {
		a.MentalShock = 0
		a.Control = 0
		return adventureMentalDamage(incoming, guardPercent)
	}
	a.MentalExposureID = b.OpponentID
	a.MentalCorruption = min(100, a.MentalCorruption+b.MentalCorruptionPower)
	a.MentalShock = 1
	if guardPercent > 0 {
		a.Injury = min(100, a.Injury+5)
	}
	return incoming
}

// Integer mitigation never exceeds the published cap. A ward does not become
// complete immunity merely because a small remaining hit would round to zero.
func adventureMentalDamage(power, wardPercent int) int {
	power = max(0, power)
	return power - power*min(90, max(0, wardPercent))/100
}
