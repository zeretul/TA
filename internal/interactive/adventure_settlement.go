package interactive

import (
	"encoding/json"
	"fmt"
)

// ResolveAdventureAction performs an atomic deterministic transition. It never
// mutates its arguments, so preview, rejection, retry and commit use one rule.
func ResolveAdventureAction(r AdventureRules, original AdventureActor, in AdventureAction, seed string) (AdventureActor, AdventureOutcome, error) {
	return ResolveAdventureActionAt(r, original, in, seed, 0)
}

// ResolveAdventureActionAt receives formal Story time from the Store, never
// from model/player action fields or the host wall clock.
func ResolveAdventureActionAt(r AdventureRules, original AdventureActor, in AdventureAction, seed string, nowSeconds int64) (AdventureActor, AdventureOutcome, error) {
	if nowSeconds < 0 {
		return original, AdventureOutcome{}, fmt.Errorf("invalid formal adventure clock")
	}
	var catalogErr error
	r, catalogErr = adventureRulesForActor(r, original, in.SkillID, in.ItemID, in.RecipeID)
	if catalogErr != nil {
		return original, AdventureOutcome{}, catalogErr
	}
	if err := r.Validate(); err != nil {
		return original, AdventureOutcome{}, err
	}
	if err := ValidateAdventureActor(r, original); err != nil {
		return original, AdventureOutcome{}, err
	}
	b, err := json.Marshal(original)
	if err != nil {
		return original, AdventureOutcome{}, err
	}
	var a AdventureActor
	if err = json.Unmarshal(b, &a); err != nil {
		return original, AdventureOutcome{}, err
	}
	refreshAdventureShields(r, &a, nowSeconds)
	a.Family = r.Family
	if a.Skills == nil {
		a.Skills = map[string]int{}
	}
	if a.Training == nil {
		a.Training = map[string]int{}
	}
	if a.Inventory == nil {
		a.Inventory = map[string]int{}
	}
	if a.Equipment == nil {
		a.Equipment = map[string]string{}
	}
	if a.Assets == nil {
		a.Assets = map[string]AdventureAsset{}
	}
	if a.Acquired == nil {
		a.Acquired = map[string]bool{}
	}
	for id, n := range a.Inventory {
		if n > 0 {
			a.Acquired[id] = true
		}
	}
	if a.Defeated == nil {
		a.Defeated = map[string]bool{}
	}
	fail := func(message string) (AdventureActor, AdventureOutcome, error) {
		return original, AdventureOutcome{}, fmt.Errorf("adventure action refused: %s", message)
	}
	if a.Rank < 1 || a.Rank > AdventureMaxRank(r) || a.MaxHealth < 1 || a.MaxEnergy < 1 || a.Health < 0 || a.Health > a.MaxHealth || a.Energy < 0 || a.Energy > a.MaxEnergy || a.Currency < 0 || a.Injury < 0 || a.Injury > 100 {
		return fail("invalid actor state")
	}
	if a.Battle != nil && a.Battle.Status == "active" && in.Action != "battle_round" {
		return fail("finish the active battle in its window first")
	}
	if a.Health == 0 && a.Battle != nil && a.Battle.Lethal {
		return fail("the lethal outcome is terminal")
	}
	if a.Health == 0 && in.Action != "rest" && in.Action != "use" {
		return fail("the actor is incapacitated")
	}
	var out AdventureOutcome
	skills, items := map[string]AdventureSkill{}, map[string]AdventureItem{}
	for _, v := range r.Skills {
		skills[v.ID] = v
	}
	for _, v := range r.Items {
		items[v.ID] = v
	}
	skill, hasSkill := skills[in.SkillID]
	item, hasItem := items[in.ItemID]
	quantity := in.Quantity
	if quantity == 0 {
		quantity = 1
	}
	if quantity < 1 || quantity > 100 {
		return fail("quantity must be between 1 and 100")
	}
	if in.Hours < 0 || in.Hours > 100000 {
		return fail("hours must be between 0 and 100000")
	}
	switch in.Action {
	case "work":
		var err error
		out, err = resolveAdventureWork(r, &a, in)
		if err != nil {
			return fail(err.Error())
		}
	case "generate":
		if in.Generate == nil {
			return fail("generated content is missing")
		}
		definition := cloneAdventureGeneratedDefinition(*in.Generate)
		if definition.Design != nil && definition.Category == "craft" && definition.Design.ProductTemplateID == "" {
			design := *definition.Design
			m, err := adventureGenerationMethod(r, definition.TemplateID)
			if err != nil {
				return fail(err.Error())
			}
			design.ProductTemplateID = m.OutputID
			definition.Design = &design
		}
		alreadyRegistered := false
		generatedID, _ := definition.id()
		for _, existing := range a.GeneratedDefinitions {
			id, _ := existing.id()
			alreadyRegistered = alreadyRegistered || id == generatedID
		}
		if !alreadyRegistered {
			elapsed, err := settleAdventureGenerationWork(r, &a, definition, in.Hours, nowSeconds)
			if err != nil {
				return fail(err.Error())
			}
			out.DurationSeconds = elapsed
		}
		if err := generateAdventureContent(r, &a, definition); err != nil {
			return fail(err.Error())
		}
		out.Message = "新内容已经登记到本局。"
	case "refine":
		var err error
		out, err = refineAdventureFoundation(r, &a, in)
		if err != nil {
			return fail(err.Error())
		}
	case "learn", "train":
		if !hasSkill || in.Hours < 1 || a.Rank < skill.RequiredRank || (skill.Prerequisite != "" && a.Skills[skill.Prerequisite] < 1) {
			return fail("skill prerequisites or training interval not met")
		}
		if a.Skills[skill.ID] == 0 && (skill.RequiredItem == "" || AdventureOwnedItemCount(a, skill.RequiredItem) < 1) {
			return fail("a committed learning source is required")
		}
		if in.Action == "train" && a.Skills[skill.ID] == 0 {
			return fail("the skill has not been learned")
		}
		if a.Injury >= 50 {
			return fail("severe injury prevents training")
		}
		level := a.Skills[skill.ID]
		cap := AdventureSkillMasteryCap(r)
		if level >= cap {
			return fail("skill has reached its mastery ceiling")
		}
		// Only productive study consumes time or earns XP. Preserve partial
		// progress, but never turn post-mastery idle hours into free ranks.
		remaining := -a.Training[skill.ID]
		for next := level + 1; next <= cap; next++ {
			remaining += skill.TrainingHours * next
		}
		hours := min(in.Hours, max(0, remaining))
		a.Training[skill.ID] += hours
		for level < cap && a.Training[skill.ID] >= skill.TrainingHours*(level+1) {
			a.Training[skill.ID] -= skill.TrainingHours * (level + 1)
			level++
		}
		a.Skills[skill.ID] = level
		a.Experience += hours
		out.DurationSeconds = int64(hours) * 3600
		out.Message = fmt.Sprintf("%s研习%d小时，掌握程度%d。", skill.Name, hours, level)
	case "rest":
		if in.Hours < 1 {
			return fail("rest requires a positive interval")
		}
		a.Injury = max(0, a.Injury-in.Hours*2)
		a.Health = min(a.MaxHealth, a.Health+in.Hours*10)
		a.Energy = min(a.MaxEnergy, a.Energy+in.Hours*20)
		out.DurationSeconds = int64(in.Hours) * 3600
		out.Message = "休养已恢复体力并缓解伤势。"
	case "buy":
		if item.Acquisition != "market" || item.Unique || !adventureMarketOffers(r, in.MarketID, item.ID) {
			return fail("this market does not sell the item")
		}
		if !hasItem || item.Price < 1 || a.Currency < item.Price*quantity {
			return fail("item unavailable or insufficient currency")
		}
		a.Currency -= adventureTradeUnitPrice(item, "buy") * quantity
		a.Inventory[item.ID] += quantity
		out.Message = fmt.Sprintf("购得%s×%d。", item.Name, quantity)
	case "sell":
		if item.Unique || !adventureMarketOffers(r, in.MarketID, AdventureItemTemplateID(a, item.ID)) {
			return fail("this market does not buy the item")
		}
		if !hasItem || a.Inventory[item.ID] < quantity || item.Price < 1 {
			return fail("sale requires owned tradable stock")
		}
		for _, id := range a.Equipment {
			if id == item.ID {
				return fail("unequip or withdraw the item before selling")
			}
		}
		if a.Inventory[item.ID] == quantity {
			delete(a.Assets, item.ID)
			delete(a.ItemCharges, item.ID)
			delete(a.ItemReadyAt, item.ID)
		}
		a.Inventory[item.ID] -= quantity
		a.Currency += adventureTradeUnitPrice(item, "sell") * quantity
		out.Message = fmt.Sprintf("售出%s×%d。", item.Name, quantity)
	case "equip":
		if a.SoftLimbs && item.Kind == "weapon" {
			return fail("softened limbs cannot draw a weapon")
		}
		if !hasItem || a.Inventory[item.ID] < 1 || (item.RequiredSkill != "" && a.Skills[item.RequiredSkill] < 1) {
			return fail("equipment or qualification missing")
		}
		if item.Kind != "weapon" && item.Kind != "armor" && item.Kind != "tool" {
			return fail("item has no equipment slot")
		}
		a.Equipment[item.Scale+":"+item.Kind] = item.ID
		out.Message = "装备已更换。"
	case "unequip":
		if !hasItem {
			return fail("unknown equipment")
		}
		found := false
		for slot, id := range a.Equipment {
			if id == item.ID && slot != item.Scale+":asset" {
				delete(a.Equipment, slot)
				found = true
			}
		}
		if !found {
			return fail("item is not equipped")
		}
		out.Message = "装备已卸下。"
	case "withdraw":
		if !hasItem {
			return fail("unknown asset")
		}
		if _, ok := a.Assets[item.ID]; !ok || a.Equipment[item.Scale+":asset"] != item.ID {
			return fail("asset is not deployed")
		}
		delete(a.Equipment, item.Scale+":asset")
		out.Message = "机械资产已退出部署，保留实际耐久与燃料。"
	case "use":
		if !hasItem || a.Inventory[item.ID] < 1 || item.Kind != "consumable" && !(item.Kind == "tool" && item.Effect == "mental_restore") {
			return fail("consumable is not owned")
		}
		switch item.Effect {
		case "mental_restore":
			if err := restoreAdventureMentalState(r, &a, item); err != nil {
				return fail(err.Error())
			}
		case "heal":
			a.Health = min(a.MaxHealth, a.Health+item.Power)
			a.Injury = max(0, a.Injury-item.Power/5)
		case "antidote":
			a.Poison = 0
			a.SoftLimbs = false
			cureAdventureFog(r, &a, item.ID)
		case "soften":
			a.SoftLimbs = true
		case "energy":
			a.Energy = min(a.MaxEnergy, a.Energy+item.Power)
		default:
			return fail("this item is used by its battle or equipment effect")
		}
		if item.Effect != "mental_restore" {
			a.Inventory[item.ID]--
		}
		out.Message = "物品已消耗并生效。"
	case "craft":
		var recipe AdventureRecipe
		for _, v := range r.Recipes {
			if v.ID == in.RecipeID {
				recipe = v
			}
		}
		work, continuing := a.Crafting[recipe.ID]
		if continuing {
			if in.Quantity > 0 && in.Quantity != work.Quantity {
				return fail("unfinished craft quantity differs")
			}
			quantity = work.Quantity
		}
		if recipe.ID == "" || a.Skills[recipe.SkillID] < recipe.SkillRank || in.Hours < 1 {
			return fail("recipe, skill qualification or interval missing")
		}
		if recipe.RequiredTool != "" && AdventureOwnedItemCount(a, recipe.RequiredTool) < 1 {
			return fail("recipe requires its owned reusable tool")
		}
		if items[recipe.OutputID].Unique {
			owner, ownershipErr := adventureUniqueItemOwner(r, nil, recipe.OutputID)
			if ownershipErr != nil || owner != "" {
				return fail("unique recipe output is reserved to an actual owner")
			}
		}
		if items[recipe.OutputID].Unique && (a.Acquired[recipe.OutputID] || quantity*recipe.OutputCount > 1) {
			return fail("unique recipe output already acquired or repeated")
		}
		if items[recipe.OutputID].Unique {
			for pendingID := range a.Crafting {
				for _, other := range r.Recipes {
					if pendingID != recipe.ID && other.ID == pendingID && other.OutputID == recipe.OutputID {
						return fail("unique recipe output already in progress")
					}
				}
			}
		}
		requiredFuel := recipe.FuelCost * quantity
		if continuing {
			requiredFuel = 0
		}
		workingAssetID := adventureDeployedAssetID(a, recipe.RequiredAsset)
		if recipe.RequiredAsset != "" {
			asset, owned := a.Assets[workingAssetID]
			if !owned || asset.Health < 1 || asset.Fuel < requiredFuel || asset.DisabledUntil > nowSeconds {
				return fail("recipe requires its owned deployed powered working asset")
			}
		}
		if !continuing {
			for id, n := range recipe.Materials {
				for _, equipped := range a.Equipment {
					if equipped == id {
						return fail("equipped items cannot be crafting materials")
					}
				}
				if _, ok := a.Assets[id]; ok {
					return fail("operational assets cannot be crafting materials")
				}
				if a.Inventory[id] < n*quantity {
					return fail("craft materials are insufficient")
				}
			}
			for id, n := range recipe.Materials {
				a.Inventory[id] -= n * quantity
			}
			if recipe.RequiredAsset != "" {
				asset := a.Assets[workingAssetID]
				asset.Fuel -= recipe.FuelCost * quantity
				a.Assets[workingAssetID] = asset
			}
		}
		used := min(in.Hours, recipe.Hours*quantity-work.WorkedHours)
		work.Quantity = quantity
		work.WorkedHours += used
		out.DurationSeconds = int64(used) * 3600
		if work.WorkedHours < recipe.Hours*quantity {
			if a.Crafting == nil {
				a.Crafting = map[string]AdventureCraftWork{}
			}
			a.Crafting[recipe.ID] = work
			out.Message = fmt.Sprintf("%s已投入%d/%d小时，进度已保存；本批材料已投入，尚无成品。", recipe.Name, work.WorkedHours, recipe.Hours*quantity)
			break
		}
		delete(a.Crafting, recipe.ID)
		a.Inventory[recipe.OutputID] += recipe.OutputCount * quantity
		a.Experience += recipe.Hours * quantity
		out.Message = fmt.Sprintf("完成%s，共%d件。", recipe.Name, recipe.OutputCount*quantity)
	case "deploy":
		if !hasItem || (item.Kind != "machine" && item.Kind != "ship") || a.Inventory[item.ID] < 1 || (item.RequiredSkill != "" && a.Skills[item.RequiredSkill] < 1) {
			return fail("asset or operator qualification missing")
		}
		if _, ok := a.Assets[item.ID]; !ok {
			a.Assets[item.ID] = AdventureAsset{Health: item.Durability, MaxHealth: item.Durability, Shield: item.Power, Fuel: 100}
		}
		if a.Assets[item.ID].DisabledUntil > nowSeconds {
			return fail("asset power is still disabled by overclock")
		}
		a.Equipment[item.Scale+":asset"] = item.ID
		out.Message = "机械资产已部署。"
	case "repair":
		if in.MarketID != "" {
			if err := repairAdventureAtDock(r, &a, in, items, &out); err != nil {
				return fail(err.Error())
			}
			break
		}
		asset, ok := a.Assets[in.TargetID]
		if !ok || !hasSkill || skill.Kind != "repair" || skill.Scale != items[in.TargetID].Scale || a.Skills[skill.ID] < 1 || in.Hours < 1 {
			return fail("repair requires an owned asset and learned repair skill")
		}
		if r.RepairMaterialID == "" || a.Inventory[r.RepairMaterialID] < in.Hours {
			return fail("repair materials are missing")
		}
		a.Inventory[r.RepairMaterialID] -= in.Hours
		asset.Health = min(asset.MaxHealth, asset.Health+in.Hours*skill.Power*a.Skills[skill.ID])
		asset.Fuel = min(100, asset.Fuel+in.Hours*10)
		a.Assets[in.TargetID] = asset
		out.DurationSeconds = int64(in.Hours) * 3600
		out.Message = "维修与补给已完成。"
	case "battle_start":
		var enemy AdventureOpponent
		for _, v := range r.Opponents {
			if v.ID == in.OpponentID {
				enemy = v
			}
		}
		if enemy.ID == "" {
			return fail("opponent or combat scale is invalid")
		}
		if len(r.FogAreas) > 0 {
			setAdventureFogLocation(r, &a, enemy.Location)
		}
		if a.Defeated[enemy.ID] || (enemy.CharacterSourceKey != "" && a.Defeated[enemy.CharacterSourceKey]) {
			return fail("this opponent has already been defeated on this branch")
		}
		playerScale := in.Scale
		if playerScale == "" {
			playerScale = enemy.Scale
		}
		personalSkill := ""
		if playerScale != enemy.Scale {
			if r.Family != "space" || playerScale != "person" || enemy.Scale != "ship" || enemy.ShipCombat == nil || !hasSkill || skill.SpaceCombat == nil || skill.Scale != "person" || a.Skills[skill.ID] < 1 || a.Rank < skill.RequiredRank || a.Energy < skill.SpaceCombat.EnergyPerRound {
				return fail("mixed-scale combat requires a learned deep-space capability and registered ship threat")
			}
			personalSkill = skill.ID
		}
		assetID := ""
		if playerScale != "person" {
			assetID = a.Equipment[playerScale+":asset"]
			if items[assetID].SupportOnly {
				return fail("support-only assets cannot enter combat")
			}
			asset, ok := a.Assets[assetID]
			if !ok || asset.Health == 0 || asset.Fuel < 1 || asset.DisabledUntil > nowSeconds {
				return fail("a functioning deployed asset is required")
			}
		}
		enemyEnergy := enemy.Energy
		if enemyEnergy == 0 {
			enemyEnergy = 100
		}
		enemyInventory := map[string]int{}
		for id, n := range enemy.Inventory {
			enemyInventory[id] = n
		}
		a.Battle = &AdventureBattle{ID: seed, EnemyRank: max(1, enemy.Rank), EnemyMaxEnergy: enemyEnergy, EnemyInventory: enemyInventory, CharacterSourceKey: enemy.CharacterSourceKey, Lethal: enemy.Lethal, EnemyEnergy: enemyEnergy, Scale: enemy.Scale, PlayerScale: playerScale, PersonalCombatSkillID: personalSkill, OpponentID: enemy.ID, OpponentName: enemy.Name, Health: enemy.Health, MaxHealth: enemy.Health, Attack: enemy.Attack, Defense: enemy.Defense, Status: "active", AssetID: assetID}
		if enemy.CharacterSourceKey == "" && enemy.WeaponID != "" {
			// An anonymous author's explicit wielded weapon is part of this
			// encounter, not merely an attack bonus. Named people instead bind
			// their actual owned loadout and must never inherit fallback gear.
			a.Battle.EnemyInventory[enemy.WeaponID] = max(1, a.Battle.EnemyInventory[enemy.WeaponID])
			a.Battle.EnemyEquipment = map[string]string{enemy.Scale + ":weapon": enemy.WeaponID}
		}
		if enemy.ShipCombat != nil {
			profile := *enemy.ShipCombat
			a.Battle.ShipCombat = &profile
		}
		if len(enemy.SkillRanks) > 0 {
			a.Battle.EnemySkillRanks = map[string]int{}
			for id, rank := range enemy.SkillRanks {
				a.Battle.EnemySkillRanks[id] = rank
			}
		}
		a.Battle.ElectronicDistanceMeters = enemy.ElectronicDistanceMeters
		a.Battle.AttackKind = enemy.AttackKind
		if a.Battle.AttackKind == "" {
			a.Battle.AttackKind = "physical"
		}
		if enemy.MentalAttack {
			a.Battle.AttackKind = "mental"
		}
		a.Battle.MentalAttack = enemy.MentalAttack
		a.Battle.Mindless = enemy.Mindless
		a.Battle.MentalCorruptionPower = enemy.MentalCorruptionPower
		out.Message = "交战已开始，请在战斗窗口选择行动。"
	case "battle_round":
		if a.Battle == nil || a.Battle.Status != "active" {
			return fail("no active battle")
		}
		if err := resolveAdventureRound(r, &a, in, seed, &out, nowSeconds); err != nil {
			return fail(err.Error())
		}
	default:
		return fail("unknown action")
	}
	advanceAdventureRank(r, &a)
	end, clockErr := adventureDeadline(nowSeconds, out.DurationSeconds)
	if clockErr != nil {
		return fail(clockErr.Error())
	}
	refreshAdventureShields(r, &a, end)
	if a.Currency > 1000000000 {
		return fail("currency capacity exceeded")
	}
	for id, n := range a.Inventory {
		if n > 0 {
			a.Acquired[id] = true
		}
	}
	if err := ValidateAdventureActor(r, a); err != nil {
		return fail(err.Error())
	}
	return a, out, nil
}

func adventureMarketOffers(r AdventureRules, marketID, itemID string) bool {
	for _, market := range r.Markets {
		if market.ID == marketID {
			for _, id := range market.Items {
				if id == itemID {
					return true
				}
			}
		}
	}
	return false
}
