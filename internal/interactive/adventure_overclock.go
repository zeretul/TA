package interactive

import (
	"crypto/sha256"
	"fmt"
)

// Rejection sampling avoids modulo bias and remains replayable from command
// identity. Different named draws cannot accidentally couple accident/escape.
func adventurePercentile(seed, purpose string) int {
	for counter := 0; ; counter++ {
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%d", seed, purpose, counter)))
		for _, v := range digest {
			if v < 200 {
				return int(v) % 100
			}
		}
	}
}

func overclockAdventureShip(r AdventureRules, a *AdventureActor, asset *AdventureAsset, item AdventureItem, thrustBonus int, seed string, now int64) error {
	if AdventurePlayerBattleScale(a.Battle) != "ship" || item.Kind != "ship" || asset.Health < 1 || asset.DisabledUntil > now {
		return fmt.Errorf("overclock requires a working owned deployed ship")
	}
	impact, err := adventureDeadline(now, adventureRoundSeconds)
	if err != nil {
		return err
	}
	until, err := adventureDeadline(impact, 3*3600)
	if err != nil {
		return err
	}
	// The sleeve is permanently worn: ordinary service restores only the reduced
	// capacity. Ten percent is a bounded catalog calibration, not a new ship.
	wear := max(1, item.Durability/10)
	if asset.MaxHealth <= wear {
		return fmt.Errorf("engine sleeve cannot survive another full overclock")
	}
	asset.PermanentDamage += wear
	asset.MaxHealth = item.Durability - asset.PermanentDamage
	asset.Health = min(asset.Health, asset.MaxHealth)
	asset.DisabledUntil = max(asset.DisabledUntil, until)
	asset.Fuel = 0
	a.Energy = 0
	if adventurePercentile(seed, "engine-melt") < 10 {
		asset.Health = 0
		a.Battle.Status = "defeat"
		return nil
	}
	// Emergency thrust is opposed by the actual pursuer, not personal rank.
	movement := min(100, adventureMovementBase(r, a, false)+max(0, thrustBonus))
	interception := adventureMovementBase(r, a, true)
	chance := max(5, min(95, 50+2*(movement-interception)))
	if adventurePercentile(seed, "overclock-escape") < chance {
		a.Battle.Status = "escaped"
	} else {
		// The attempt has exhausted the propulsion system. This is an honest lost
		// encounter, never an active battle requiring impossible fuel or free escape.
		a.Battle.Status = "defeat"
	}
	return nil
}

func repairAdventureAtDock(r AdventureRules, a *AdventureActor, in AdventureAction, items map[string]AdventureItem, out *AdventureOutcome) error {
	var dock *AdventureDockService
	for _, m := range r.Markets {
		if m.ID == in.MarketID {
			dock = m.DockService
		}
	}
	asset, ok := a.Assets[in.TargetID]
	if dock == nil || !ok || a.Inventory[in.TargetID] < 1 || items[in.TargetID].Scale != dock.Scale || a.Skills[dock.RequiredSkill] < 1 || in.SkillID != "" || in.Hours < 1 {
		return fmt.Errorf("dock service requires its registered owned ship and engineer qualification")
	}
	cost := dock.PricePerHour * in.Hours
	if a.Currency < cost || a.Inventory[r.RepairMaterialID] < in.Hours {
		return fmt.Errorf("dock service funds or repair materials are insufficient")
	}
	a.Currency -= cost
	a.Inventory[r.RepairMaterialID] -= in.Hours
	asset.Health = min(asset.MaxHealth, asset.Health+dock.RepairPerHour*in.Hours)
	asset.Fuel = min(100, asset.Fuel+10*in.Hours)
	a.Assets[in.TargetID] = asset
	out.DurationSeconds = int64(in.Hours) * 3600
	out.Message = "船坞已按工时收取费用与材料，完成检修补给；动力套筒的永久磨损仍在。"
	return nil
}
