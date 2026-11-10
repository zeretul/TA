package interactive

import "fmt"

// ValidateAdventureActor checks authored origins and restored canonical values.
// It accepts different lawful origins, but never unknown skills or phantom items.
func ValidateAdventureActor(r AdventureRules, a AdventureActor) error {
	var err error
	r, err = adventureRulesForActor(r, a)
	if err != nil {
		return err
	}
	if err := validateAdventureBattleScales(r, a); err != nil {
		return err
	}
	if a.ControlRecovery < 0 || a.ControlRecovery > 2 || a.RushBlocked < 0 || a.RushBlocked > 1 || len(a.LastSealTarget) > 256 || a.FogTreated && a.FogAreaID == "" {
		return fmt.Errorf("invalid encounter condition")
	}
	if a.FogAreaID != "" {
		known := false
		for _, area := range r.FogAreas {
			known = known || area.ID == a.FogAreaID
		}
		if !known {
			return fmt.Errorf("unknown fog exposure")
		}
	}
	if a.Battle != nil && (a.Battle.EnemyControlRecovery < 0 || a.Battle.EnemyControlRecovery > 2 || a.Battle.ElectronicDistanceMeters < 0 || a.Battle.ElectronicDistanceMeters > 10000 || a.Battle.EnemyRushBlocked < 0 || a.Battle.EnemyRushBlocked > 1 || a.Battle.EnemyInjury < 0 || a.Battle.EnemyInjury > 100) {
		return fmt.Errorf("invalid enemy rush condition")
	}
	if a.Battle != nil && (a.Battle.PlayerRetreatProgress < 0 || a.Battle.PlayerRetreatProgress > 100 || a.Battle.EnemyRetreatProgress < 0 || a.Battle.EnemyRetreatProgress > 100 || a.Battle.NoProgressRounds < 0 || a.Battle.NoProgressRounds > 1000) {
		return fmt.Errorf("invalid disengagement state")
	}
	if err := validateAdventureOpeningHazards(a.OpeningHazards); err != nil {
		return err
	}
	if a.Family != "" && a.Family != r.Family {
		return fmt.Errorf("adventure actor belongs to a different family")
	}
	if a.Rank < 1 || a.Rank > AdventureMaxRank(r) || a.Experience < 0 || a.Experience > 100000000 || a.MaxHealth < 1 || a.MaxHealth > 1000000 || a.Health < 0 || a.Health > a.MaxHealth || a.MaxEnergy < 1 || a.MaxEnergy > 1000000 || a.Energy < 0 || a.Energy > a.MaxEnergy || a.Injury < 0 || a.Injury > 100 || a.Currency < 0 || a.Currency > 1000000000 {
		return fmt.Errorf("adventure actor values out of range")
	}
	skills := map[string]AdventureSkill{}
	for _, v := range r.Skills {
		skills[v.ID] = v
	}
	items := map[string]AdventureItem{}
	for _, v := range r.Items {
		items[v.ID] = v
	}
	if a.MentalCorruption < 0 || a.MentalCorruption > 100 || a.MentalShock < 0 || a.MentalShock > 1 || a.MentalCorruption > 0 && a.MentalExposureID == "" || a.MentalExposureID != "" && !adventureMentalSource(r, a.MentalExposureID) {
		return fmt.Errorf("invalid registered mental condition")
	}
	for id, remaining := range a.ItemCharges {
		item, ok := items[id]
		if !ok || item.MaxCharges < 1 || a.Inventory[id] < 1 || remaining < 0 || remaining > item.MaxCharges {
			return fmt.Errorf("invalid item charge state %s", id)
		}
	}
	for id, ready := range a.ItemReadyAt {
		item, ok := items[id]
		if !ok || item.Effect != "shield" || a.Inventory[id] < 1 || ready <= 0 {
			return fmt.Errorf("invalid shield cooldown %s", id)
		}
	}
	cap := 10
	if r.Family == "wuxia" {
		cap = 4
	}
	for id, level := range a.Skills {
		skill, ok := skills[id]
		if !ok || level < 0 || level > cap || (level > 0 && skill.RequiredRank > a.Rank) {
			return fmt.Errorf("invalid learned skill %s", id)
		}
	}
	for id, n := range a.Training {
		if _, ok := skills[id]; !ok || n < 0 || n > 100000000 {
			return fmt.Errorf("invalid skill progress %s", id)
		}
	}
	for id, work := range a.Crafting {
		valid := false
		for _, recipe := range r.Recipes {
			if recipe.ID == id && work.Quantity >= 1 && work.Quantity <= 100 && work.WorkedHours > 0 && work.WorkedHours < recipe.Hours*work.Quantity {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid unfinished craft %s", id)
		}
	}
	for id, n := range a.Inventory {
		item, ok := items[id]
		if !ok || n < 0 || n > 1000000 || (item.Unique && n > 1) {
			return fmt.Errorf("invalid item ownership %s", id)
		}
	}
	for slot, id := range a.Equipment {
		item, ok := items[id]
		if !ok || a.Inventory[id] < 1 {
			return fmt.Errorf("unowned equipped item %s", id)
		}
		kind := item.Kind
		if kind == "machine" || kind == "ship" {
			kind = "asset"
		}
		if slot != item.Scale+":"+kind {
			return fmt.Errorf("invalid equipment slot %s", slot)
		}
	}
	for id, asset := range a.Assets {
		item, ok := items[id]
		if !ok || (item.Kind != "machine" && item.Kind != "ship") || a.Inventory[id] < 1 || asset.PermanentDamage < 0 || asset.PermanentDamage >= item.Durability || asset.DisabledUntil < 0 || asset.MaxHealth != item.Durability-asset.PermanentDamage || asset.Health < 0 || asset.Health > asset.MaxHealth || asset.Fuel < 0 || asset.Fuel > 100 || asset.Shield < 0 || asset.Shield > item.Power {
			return fmt.Errorf("invalid owned asset %s", id)
		}
	}
	return nil
}
