package interactive

import (
	"fmt"
	"math"
)

const adventureRoundSeconds int64 = 20

func adventureDeadline(now, delay int64) (int64, error) {
	if now < 0 || delay < 0 || now > math.MaxInt64-delay {
		return 0, fmt.Errorf("formal adventure clock overflows")
	}
	return now + delay, nil
}

func refreshAdventureShieldItem(a *AdventureActor, item AdventureItem, now int64) bool {
	ready, present := a.ItemReadyAt[item.ID]
	if !present || item.Effect != "shield" || ready > now {
		return false
	}
	delete(a.ItemReadyAt, item.ID)
	if a.Inventory[item.ID] > 0 && a.ItemCharges[item.ID] == 0 {
		if a.ItemCharges == nil {
			a.ItemCharges = map[string]int{}
		}
		a.ItemCharges[item.ID] = item.MaxCharges
	}
	return true
}

func refreshAdventureShields(r AdventureRules, a *AdventureActor, now int64) bool {
	changed := false
	for id := range a.ItemReadyAt {
		for _, item := range r.Items {
			if item.ID == id {
				if refreshAdventureShieldItem(a, item, now) {
					changed = true
				}
				break
			}
		}
	}
	return changed
}

func adventureEquippedShield(a *AdventureActor, items map[string]AdventureItem) (AdventureItem, int, bool) {
	id := a.Equipment["person:armor"]
	item, ok := items[id]
	if !ok || item.Effect != "shield" || a.Inventory[id] < 1 {
		return AdventureItem{}, 0, false
	}
	remaining, present := a.ItemCharges[id]
	if !present {
		remaining = item.MaxCharges
	}
	return item, remaining, remaining > 0
}

func activateAdventureShieldBoost(a *AdventureActor, items map[string]AdventureItem, required string, now int64) error {
	item, _, ready := adventureEquippedShield(a, items)
	if !ready || item.ID != required || a.ItemReadyAt[item.ID] > now {
		return fmt.Errorf("shield overcharge requires its equipped charged bracelet outside cooldown")
	}
	until, err := adventureDeadline(now, adventureRoundSeconds+120)
	if err != nil {
		return err
	}
	if a.ItemReadyAt == nil {
		a.ItemReadyAt = map[string]int64{}
	}
	a.ItemReadyAt[item.ID] = until
	return nil
}

// A charge is spent only on an actual laser hit. Other harm is not converted
// into laser damage just because the character owns a bracelet.
func absorbAdventureLaser(a *AdventureActor, items map[string]AdventureItem, incoming int, now int64, boost int) (int, error) {
	if incoming < 1 {
		return incoming, nil
	}
	impact, err := adventureDeadline(now, adventureRoundSeconds)
	if err != nil {
		return 0, err
	}
	item, _, _ := adventureEquippedShield(a, items)
	if boost == 0 {
		refreshAdventureShieldItem(a, item, impact)
	}
	item, remaining, ready := adventureEquippedShield(a, items)
	if !ready || a.ItemReadyAt[item.ID] > impact && boost == 0 {
		return incoming, nil
	}
	peak := item.Power * (100 + boost) / 100
	remaining--
	if a.ItemCharges == nil {
		a.ItemCharges = map[string]int{}
	}
	a.ItemCharges[item.ID] = remaining
	if remaining == 0 {
		until, err := adventureDeadline(impact, item.CooldownSeconds)
		if err != nil {
			return 0, err
		}
		if a.ItemReadyAt == nil {
			a.ItemReadyAt = map[string]int64{}
		}
		a.ItemReadyAt[item.ID] = max(a.ItemReadyAt[item.ID], until)
		a.Injury = min(100, a.Injury+1)
	}
	return max(0, incoming-peak), nil
}

func absorbAdventureEnemyLaser(r AdventureRules, a *AdventureActor, items map[string]AdventureItem, force int, now int64) (int, error) {
	b := a.Battle
	enemy := AdventureActor{Inventory: b.EnemyInventory, Equipment: b.EnemyEquipment, ItemCharges: b.EnemyItemCharges, ItemReadyAt: b.EnemyItemReadyAt, Injury: b.EnemyInjury}
	refreshAdventureShields(r, &enemy, now)
	remaining, err := absorbAdventureLaser(&enemy, items, force, now, 0)
	b.EnemyItemCharges = enemy.ItemCharges
	b.EnemyItemReadyAt = enemy.ItemReadyAt
	b.EnemyInjury = enemy.Injury
	return remaining, err
}
