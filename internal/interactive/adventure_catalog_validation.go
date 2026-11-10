package interactive

import (
	"encoding/json"
	"fmt"
)

func (r AdventureRules) Validate() error {
	if (r.Family != "wuxia" && r.Family != "space" && r.Family != "fantasy") || r.Currency == "" || r.StartingCurrency < 0 || r.StartingCurrency > 1000000 {
		return fmt.Errorf("invalid adventure family or currency")
	}
	if !r.generatedCatalog && (len(r.Skills) > 256 || len(r.Items) > 512 || len(r.Recipes) > 256 || len(r.Opponents) > 256) {
		return fmt.Errorf("adventure catalog exceeds supported capacity")
	}
	if err := validateAdventureProgression(r); err != nil {
		return err
	}
	skills, items, ids := map[string]bool{}, map[string]bool{}, map[string]bool{}
	validScale := func(s string) bool { return s == "person" || (r.Family == "space" && (s == "machine" || s == "ship")) }
	for _, s := range r.Skills {
		if err := validateAdventureSpaceCombatSkill(r, s); err != nil {
			return err
		}
		if err := s.DefinitionDetails.Validate(); err != nil {
			return fmt.Errorf("skill metadata %s: %w", s.ID, err)
		}
		if s.RangeMeters < 0 || s.RangeMeters > 10000 || s.ID == "" || s.Name == "" || skills[s.ID] || !validScale(s.Scale) || s.Power < 0 || s.Power > 100 || s.Cost < 0 || s.Cost > 100 || s.RequiredRank < 1 || s.RequiredRank > AdventureMaxRank(r) || s.TrainingHours < 1 || s.TrainingHours > 100000 {
			return fmt.Errorf("invalid adventure skill %s", s.ID)
		}
		switch s.Kind {
		case "attack", "heal", "guard", "control", "poison", "repair", "engineering", "pilot", "social", "energy", "antidote":
		default:
			return fmt.Errorf("unsupported adventure skill kind %s", s.Kind)
		}
		for _, e := range s.Effects {
			if e.Kind == "mental_attack" && ((r.Family != "space" && r.Family != "fantasy") || s.Scale != "person" || s.Cost < 1 || e.Power < 1) {
				return fmt.Errorf("mental attack requires a paid personal space or fantasy skill %s", s.ID)
			}
			if e.Kind == "electronic_disrupt" && (r.Family != "space" || s.Scale != "person" || s.RangeMeters < 1 || len(s.Effects) != 1) {
				return fmt.Errorf("invalid electronic disruption %s", s.ID)
			}
			if e.Kind == "overclock" && (r.Family != "space" || s.Scale != "ship" || s.Kind != "pilot" || e.Power < 1) {
				return fmt.Errorf("invalid overclock skill %s", s.ID)
			}
			switch e.Kind {
			case "attack", "heal", "energy", "guard", "control", "poison", "antidote", "repair", "seal", "reflect", "drain", "mental_attack", "mental_guard", "shield_boost", "overclock", "electronic_disrupt":
			default:
				return fmt.Errorf("invalid skill effect %s", s.ID)
			}
			if e.Power < 0 || e.Power > 100 {
				return fmt.Errorf("skill effect out of range")
			}
		}
		if len(s.EnemyEffects) > 4 {
			return fmt.Errorf("too many alternate enemy effects %s", s.ID)
		}
		for _, effect := range s.EnemyEffects {
			if effect.Kind != "seal" || effect.Power < 1 || effect.Power > 100 || s.Scale != "person" {
				return fmt.Errorf("invalid enemy targeting effect %s", s.ID)
			}
		}
		skills[s.ID] = true
	}
	for _, s := range r.Skills {
		if err := validateAdventureBacklash(s, skills); err != nil {
			return err
		}
		for _, id := range s.DrainCounterSkills {
			if !skills[id] {
				return fmt.Errorf("unknown drain counter skill %s", id)
			}
		}
		if s.Prerequisite != "" && (!skills[s.Prerequisite] || s.Prerequisite == s.ID) {
			return fmt.Errorf("invalid skill prerequisite %s", s.ID)
		}
	}
	// A cycle has no learnable first skill and is rejected at pack validation.
	prerequisites := map[string]string{}
	for _, skill := range r.Skills {
		prerequisites[skill.ID] = skill.Prerequisite
	}
	for _, skill := range r.Skills {
		seen := map[string]bool{}
		for id := skill.ID; id != ""; id = prerequisites[id] {
			if seen[id] {
				return fmt.Errorf("cyclic skill prerequisite %s", skill.ID)
			}
			seen[id] = true
		}
	}
	for _, i := range r.Items {
		if i.WeaponClass != "" && (!adventureWeaponClassValid(i.WeaponClass) || i.Kind != "weapon" || i.Scale != "person") {
			return fmt.Errorf("invalid authored weapon class %s", i.ID)
		}
		if i.Mobility < 0 || i.Mobility > 100 {
			return fmt.Errorf("invalid item mobility %s", i.ID)
		}
		if err := i.DefinitionDetails.Validate(); err != nil {
			return fmt.Errorf("item metadata %s: %w", i.ID, err)
		}
		if i.ID == "" || i.Name == "" || items[i.ID] || !validScale(i.Scale) || i.Price < 0 || i.Price > 100000000 || i.Power < 0 || i.Power > 100 || i.Durability < 0 || i.Durability > 1000000 || i.Chance < 0 || i.Chance > 100 || i.Multiplier < 0 || i.Multiplier > 4 {
			return fmt.Errorf("invalid adventure item %s", i.ID)
		}
		switch i.Kind {
		case "weapon", "armor", "consumable", "material", "tool", "machine", "ship":
		default:
			return fmt.Errorf("unsupported adventure item kind %s", i.Kind)
		}
		switch i.Effect {
		case "", "heal", "energy", "poison", "revive", "compute", "antidote", "attack", "soften", "mental_restore", "shield":
		default:
			return fmt.Errorf("unsupported adventure effect %s", i.Effect)
		}
		if i.RequiredSkill != "" && !skills[i.RequiredSkill] {
			return fmt.Errorf("unknown item skill %s", i.ID)
		}
		if (i.Kind == "ship" || i.Kind == "machine") && (r.Family != "space" || i.Scale != i.Kind || i.Durability < 1) {
			return fmt.Errorf("invalid asset %s", i.ID)
		}
		if i.SupportOnly && i.Kind != "machine" && i.Kind != "ship" {
			return fmt.Errorf("support role requires an asset %s", i.ID)
		}
		if i.RadioDevice && (r.Family != "space" || i.Kind != "tool") {
			return fmt.Errorf("radio capability requires a space communication tool %s", i.ID)
		}
		if (i.Effect == "revive" && i.Chance < 1) || (i.Effect == "compute" && i.Multiplier < 1) {
			return fmt.Errorf("missing special effect parameter %s", i.ID)
		}
		if i.AttackKind != "" && (i.AttackKind != "laser" || i.Kind != "weapon") {
			return fmt.Errorf("invalid weapon attack kind %s", i.ID)
		}
		if i.CooldownSeconds < 0 || i.CooldownSeconds > 86400 || i.Effect == "shield" && (i.Kind != "armor" || i.Scale != "person" || i.MaxCharges < 1 || i.Power < 1 || i.CooldownSeconds < 1) {
			return fmt.Errorf("invalid shield definition %s", i.ID)
		}
		items[i.ID] = true
		if i.MaxCharges < 0 || i.MaxCharges > 20 || i.Effect == "mental_restore" && (i.Kind != "tool" || i.MaxCharges < 1 || i.Power < 1) {
			return fmt.Errorf("invalid finite-use item %s", i.ID)
		}
	}
	if err := validateAdventureFogAreas(r, items); err != nil {
		return err
	}
	starters := map[string]bool{}
	for _, id := range r.StartingSkills {
		starters[id] = true
	}
	for _, skill := range r.Skills {
		if skill.RequiredWeaponClass != "" {
			if !adventureWeaponClassValid(skill.RequiredWeaponClass) || skill.Scale != "person" {
				return fmt.Errorf("invalid skill weapon class: %s", skill.ID)
			}
			matched := false
			for _, item := range r.Items {
				matched = matched || item.WeaponClass == skill.RequiredWeaponClass && item.Kind == "weapon" && item.Scale == skill.Scale && (skill.RequiredWeapon == "" || item.ID == skill.RequiredWeapon)
			}
			if !matched {
				return fmt.Errorf("skill weapon class has no matching registered weapon: %s", skill.ID)
			}
		}
		if skill.RequiredWeapon != "" {
			matched := false
			for _, item := range r.Items {
				matched = matched || item.ID == skill.RequiredWeapon && item.Kind == "weapon" && item.Scale == skill.Scale
			}
			if !matched {
				return fmt.Errorf("skill requires a registered same-scale weapon: %s", skill.ID)
			}
		}
		if skill.RequiredTool != "" && !items[skill.RequiredTool] {
			return fmt.Errorf("unknown skill tool %s", skill.ID)
		}
		for id, n := range skill.Materials {
			if !items[id] || n < 1 || n > 1000 {
				return fmt.Errorf("invalid skill material %s", id)
			}
		}
		for _, conditional := range skill.ConditionalEffects {
			if !items[conditional.RequiredItem] || len(conditional.Effects) == 0 || len(conditional.Effects) > 4 {
				return fmt.Errorf("invalid conditional skill source")
			}
			for _, effect := range conditional.Effects {
				switch effect.Kind {
				case "attack", "heal", "energy", "guard", "control", "poison", "antidote", "repair", "recoil":
				default:
					return fmt.Errorf("unsupported conditional effect")
				}
				if effect.Power < 0 || effect.Power > 100 {
					return fmt.Errorf("conditional effect out of range")
				}
			}
		}
		if !starters[skill.ID] && !items[skill.RequiredItem] && !(skill.GeneratedFrom != "" && skills[skill.GeneratedFrom] && skill.GeneratedFrom != skill.ID) {
			return fmt.Errorf("skill learning source missing: %s", skill.ID)
		}
	}
	for _, item := range r.Items {
		if len(item.Materials) > 0 && item.Kind != "weapon" {
			return fmt.Errorf("only weapons may consume ammunition")
		}
		for id, n := range item.Materials {
			if !items[id] || id == item.ID || n < 1 || n > 1000 {
				return fmt.Errorf("invalid weapon ammunition %s", item.ID)
			}
		}
		if item.Effect == "attack" && item.Kind != "consumable" {
			return fmt.Errorf("attack item must be consumable")
		}
		switch item.Acquisition {
		case "market", "reward", "craft":
		default:
			return fmt.Errorf("invalid acquisition %s", item.ID)
		}
		if item.Unique && item.Acquisition == "market" {
			return fmt.Errorf("unique item cannot be a market commodity: %s", item.ID)
		}
	}
	if r.RepairMaterialID != "" && !items[r.RepairMaterialID] {
		return fmt.Errorf("invalid repair material")
	}
	marketIDs := map[string]bool{}
	for _, market := range r.Markets {
		if market.ID == "" || market.Location == "" || marketIDs[market.ID] {
			return fmt.Errorf("invalid market")
		}
		if work := market.WorkService; work != nil {
			if err := work.validate(skills); err != nil {
				return fmt.Errorf("work service %s: %w", market.ID, err)
			}
			if r.Family == "wuxia" && work.SkillRank > 4 {
				return fmt.Errorf("work service mastery exceeds its world ceiling: %s", market.ID)
			}
		}

		if dock := market.DockService; dock != nil {
			if r.Family != "space" || dock.Scale != "ship" || !skills[dock.RequiredSkill] || dock.PricePerHour < 1 || dock.PricePerHour > 1000000 || dock.RepairPerHour < 1 || dock.RepairPerHour > 10000 || r.RepairMaterialID == "" {
				return fmt.Errorf("invalid dock service %s", market.ID)
			}
		}
		marketIDs[market.ID] = true
		for _, id := range market.Items {
			if !items[id] {
				return fmt.Errorf("unknown market item %s", id)
			}
		}
	}
	for _, v := range r.Recipes {
		if v.RequiredTool != "" && !items[v.RequiredTool] {
			return fmt.Errorf("unknown recipe tool %s", v.ID)
		}
		if v.ID == "" || ids[v.ID] || !skills[v.SkillID] || v.SkillRank < 1 || v.SkillRank > 10 || v.Hours < 1 || v.Hours > 100000 || !items[v.OutputID] || v.OutputCount < 1 || v.OutputCount > 100 || len(v.Materials) == 0 {
			return fmt.Errorf("invalid adventure recipe %s", v.ID)
		}
		if v.FuelCost < 0 || v.FuelCost > 100 || (v.RequiredAsset == "" && v.FuelCost != 0) {
			return fmt.Errorf("invalid recipe fuel cost %s", v.ID)
		}
		if v.RequiredAsset != "" {
			found := false
			for _, item := range r.Items {
				found = found || (item.ID == v.RequiredAsset && (item.Kind == "machine" || item.Kind == "ship"))
			}
			if !found || v.FuelCost < 1 {
				return fmt.Errorf("invalid recipe working asset %s", v.ID)
			}
		}
		for id, n := range v.Materials {
			if !items[id] || n < 1 || n > 10000 {
				return fmt.Errorf("invalid recipe material %s", id)
			}
		}
		ids[v.ID] = true
	}
	ids = map[string]bool{}
	for _, v := range r.Opponents {
		if v.RequiresPresence {
			known := false
			for _, npc := range r.NPCs {
				known = known || npc.CharacterSourceKey == v.CharacterSourceKey
			}
			if v.CharacterSourceKey == "" || !known {
				return fmt.Errorf("named encounter requires an authored NPC %s", v.ID)
			}
		}
		for id, rank := range v.SkillRanks {
			listed := false
			for _, learned := range v.SkillIDs {
				listed = listed || learned == id
			}
			if !listed || !skills[id] || rank < 1 || rank > AdventureSkillMasteryCap(r) {
				return fmt.Errorf("invalid authored opponent skill mastery %s/%s", v.ID, id)
			}
		}
		if err := validateAdventureShipCombat(v.ShipCombat); err != nil {
			return err
		}
		if v.ShipCombat != nil && (r.Family != "space" || v.Scale != "ship" || v.CharacterSourceKey != "") {
			return fmt.Errorf("ship combat profile requires an unbound vessel or formation %s", v.ID)
		}
		if v.ElectronicDistanceMeters < 0 || v.ElectronicDistanceMeters > 10000 || (v.ElectronicDistanceMeters > 0 && v.Scale == "ship") {
			return fmt.Errorf("invalid electronic encounter range %s", v.ID)
		}
		if v.AttackKind != "" && v.AttackKind != "physical" && v.AttackKind != "laser" && v.AttackKind != "mental" || v.MentalAttack && v.AttackKind != "" && v.AttackKind != "mental" {
			return fmt.Errorf("invalid opponent attack kind %s", v.ID)
		}

		if v.MentalCorruptionPower < 0 || v.MentalCorruptionPower > 100 || v.MentalAttack && (r.Family != "space" || v.Scale != "person") || !v.MentalAttack && v.MentalCorruptionPower > 0 {
			return fmt.Errorf("invalid mental threat %s", v.ID)
		}
		if v.Rank < 0 || v.Rank > AdventureMaxRank(r) || v.Energy < 0 || v.Energy > 1000 || v.ID == "" || v.Location == "" || ids[v.ID] || !validScale(v.Scale) || v.Health < 1 || v.Health > 1000000 || v.Attack < 1 || v.Attack > 10000 || v.Defense < 0 || v.Defense > 10000 || v.RewardCurrency < 0 || v.RewardCurrency > 1000000 || v.RewardExperience < 0 || v.RewardExperience > 100000 {
			return fmt.Errorf("invalid adventure opponent %s", v.ID)
		}
		if v.WeaponID != "" {
			found := false
			for _, item := range r.Items {
				if item.ID == v.WeaponID && item.Kind == "weapon" && item.Scale == v.Scale {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("invalid opponent weapon %s", v.ID)
			}
		}
		for id, n := range v.Inventory {
			if !items[id] || n < 1 || n > 10000 {
				return fmt.Errorf("invalid opponent inventory %s", id)
			}
		}
		for _, id := range v.SkillIDs {
			if !skills[id] {
				return fmt.Errorf("unknown opponent skill %s", id)
			}
		}
		ids[v.ID] = true
	}
	for _, id := range r.StartingSkills {
		if !skills[id] {
			return fmt.Errorf("unknown starting skill %s", id)
		}
	}
	for id, n := range r.StartingItems {
		if !items[id] || n < 1 || n > 10000 {
			return fmt.Errorf("unknown starting item %s", id)
		}
	}
	if err := validateAdventureStartingLoadouts(r); err != nil {
		return err
	}
	if err := validateAdventureNPCs(r); err != nil {
		return err
	}
	// Story-local definitions are checked above with the same mechanics, but
	// must not consume the frozen pack or model-context capacities.
	if r.generatedCatalog {
		return adventureFrozenRules(r).Validate()
	}
	full, err := json.Marshal(r)
	if err != nil || len(full) > 4*1024*1024 {
		return fmt.Errorf("adventure catalog exceeds the 4 MiB frozen display capacity")
	}
	// The bounded mechanical prefix now includes period-specific NPC repertoires,
	// carried equipment and named meetings; display prose remains excluded.
	encoded, err := json.Marshal(adventureMechanicalRules(r))
	if err != nil || len(encoded) > 128*1024 {
		return fmt.Errorf("adventure catalog exceeds the 128 KiB context capacity")
	}
	return nil
}
