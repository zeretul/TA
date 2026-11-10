package interactive

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

func resolveAdventureRound(r AdventureRules, a *AdventureActor, in AdventureAction, seed string, out *AdventureOutcome, nowSeconds int64) error {
	b := a.Battle
	playerScale := AdventurePlayerBattleScale(b)
	personalSpace, personalShip := AdventurePersonalSpaceCombat(r, *a)
	b.EnemyLastSkillID = ""
	b.EnemyLastAction = ""
	// An accepted already-published offer ends the encounter before upkeep.
	if in.Choice == "accept_ceasefire" {
		if !b.EnemyCeasefireOffer {
			return fmt.Errorf("there is no committed ceasefire offer to accept")
		}
		b.PlayerCeasefireOffer = true
		b.Status = "standoff"
		b.Round++
		out.DurationSeconds = adventureRoundSeconds
		out.Message = "双方接受停战，交锋不分胜负；已有伤势和消耗保留。"
		finishAdventureEncounter(a)
		return nil
	}
	playerHealthBefore := a.Health
	playerMovement, enemyMovement := 0, 0
	playerWasControlled := a.Control > 0
	enemyAction := ""

	if in.Choice == "surrender" {
		if (r.Family != "wuxia" && r.Family != "fantasy") || b.Scale != "person" || b.Lethal {
			return fmt.Errorf("surrender requires a supported nonlethal person encounter")
		}
		// Conceding is a recorded loss, not a victory, a reward, or a healing
		// action. It must not be silently converted into an involuntary attack.
		b.Status = "defeat"
		b.Round++
		out.DurationSeconds = adventureRoundSeconds
		out.Message = "你认输了，本次非致命交锋结束；已有伤势保留，没有战利奖励。"
		finishAdventureEncounter(a)
		return nil
	}
	enemyHealthBefore := b.Health
	attack := max(1, 10+a.Rank*3-a.Injury/10)
	defense := a.Rank
	weapon := AdventureItem{}
	guard := false
	items := map[string]AdventureItem{}
	for _, v := range r.Items {
		items[v.ID] = v
	}
	for _, slot := range []string{"weapon", "armor"} {
		if v, ok := items[a.Equipment[playerScale+":"+slot]]; ok {
			if slot == "weapon" {
				attack += v.Power
				weapon = v
			} else {
				if v.Effect != "shield" {
					defense += v.Power
				}
			}
		}
	}
	enemyDefense := b.Defense
	if armor := adventureEnemyEquippedItem(b, items, "armor"); armor.Effect != "shield" {
		enemyDefense += armor.Power
	}
	if personalShip {
		enemyDefense += b.ShipCombat.HullThreshold
		defense += personalSpace.Protection
	}
	asset := a.Assets[b.AssetID]
	if playerScale != "person" {
		if asset.Health <= 0 || asset.Fuel < 1 || asset.DisabledUntil > nowSeconds {
			// Older journals may have an active encounter with no operable asset.
			// Explicit withdrawal records a loss, not free escape or recovery.
			if in.Choice == "escape" {
				b.Status = "defeat"
				b.Round++
				out.DurationSeconds = adventureRoundSeconds
				out.Message = "载具已无法继续行动，本次交锋以败退结束；损耗与补给仍需处理。"
				finishAdventureEncounter(a)
				return nil
			}
			return fmt.Errorf("asset cannot operate")
		}
		asset.Fuel--
		attack += items[b.AssetID].Power
	}
	if personalShip {
		if a.Energy < personalSpace.EnergyPerRound {
			if in.Choice != "escape" {
				return fmt.Errorf("personal deep-space combat energy is exhausted")
			}
			b.Status = "defeat"
			b.Round++
			out.DurationSeconds = adventureRoundSeconds
			out.Message = "深空作战能力已中断，未确认安全脱离；损耗保留，没有战利。"
			finishAdventureEncounter(a)
			return nil
		}
		a.Energy -= personalSpace.EnergyPerRound
	}
	draw := sha256.Sum256([]byte(seed))
	damage := 0
	mentalDamage := 0
	reflectPercent := 0
	mentalGuardPercent := 0
	shieldBoost := 0
	outgoingKind := "physical"
	sealedTarget := ""
	controlResisted := false
	rushWasBlocked := a.RushBlocked > 0
	mentalShockWasPresent := a.MentalShock > 0
	if a.Control > 0 {
		a.Control--
		a.ControlRecovery = max(a.ControlRecovery, 1)
		if !a.SoftLimbs {
			in.Choice = "guard"
		}
	}
	switch in.Choice {
	case "attack":
		if a.MentalShock > 0 {
			return fmt.Errorf("mental shock blocks physical offense; defend, recover or retreat")
		}
		if a.SoftLimbs {
			return fmt.Errorf("softened limbs permit only verbal standoff or recovery")
		}
		if err := consumeAdventureAmmunition(a.Inventory, a.Equipment, weapon.Materials); err != nil {
			return err
		}
		if weapon.AttackKind != "" {
			outgoingKind = weapon.AttackKind
		}
		damage = max(0, attack+personalSpace.HullPower-enemyDefense)
	case "guard":
		if a.SoftLimbs {
			return fmt.Errorf("softened limbs permit only verbal standoff or recovery")
		}
		guard = true
	case "escape", "pursue":
		if a.SoftLimbs {
			return fmt.Errorf("softened limbs cannot make a physical escape")
		}
		var err error
		playerMovement, err = adventureMovementPower(r, a, false, in.SkillID, true)
		if err != nil {
			return err
		}
		b.PlayerCeasefireOffer = false
	case "ceasefire":
		b.PlayerCeasefireOffer = true
	case "talk":
		if !a.SoftLimbs && !b.EnemySoftLimbs {
			return fmt.Errorf("verbal standoff requires an actual softened participant")
		}
		b.Status = "standoff"
	case "listen":
		if !b.MentalAttack {
			return fmt.Errorf("there is no registered mental signal to listen to")
		}
		a.MentalExposureID = b.OpponentID
		a.MentalCorruption = min(100, a.MentalCorruption+b.MentalCorruptionPower)
		a.MentalShock = 1
		a.Injury = min(100, a.Injury+5)
	case "skill":
		var s AdventureSkill
		for _, v := range r.Skills {
			if v.ID == in.SkillID {
				s = v
			}
		}
		targetKey := b.OpponentID + "/" + s.ID
		if b.CharacterSourceKey != "" {
			targetKey = b.CharacterSourceKey + "/" + s.ID
		}
		if s.OncePerTarget && a.UsedTargetSkills[targetKey] {
			return fmt.Errorf("skill already used against this target")
		}
		level := a.Skills[s.ID]
		if s.ID == "" || level < 1 || a.Rank < s.RequiredRank || (s.Scale != playerScale && !AdventureElectronicTargetInRange(s, b)) || a.Energy < s.Cost {
			return fmt.Errorf("skill is unlearned, wrong scale, unqualified or unaffordable")
		}
		if s.Rush && adventureRushUnavailable(*a) {
			return fmt.Errorf("rush movement is blocked by sealing or the current fog")
		}
		selfHarm, selfInjury := adventureBacklashCost(*a, s.Backlash)
		if selfHarm >= a.Health {
			return fmt.Errorf("full-force self-injury exceeds surviving health")
		}
		if !adventureSkillWeaponAvailable(r, s, a.Inventory, a.Equipment) {
			return fmt.Errorf("skill requires its actual equipped weapon")
		}
		if s.RequiredTool != "" && AdventureOwnedItemCount(*a, s.RequiredTool) < 1 {
			return fmt.Errorf("skill tool is not owned")
		}
		for id, n := range s.Materials {
			if a.Inventory[id] < n {
				return fmt.Errorf("skill materials are insufficient")
			}
			for _, equipped := range a.Equipment {
				if equipped == id {
					return fmt.Errorf("equipped item cannot be consumed as skill material")
				}
			}
		}
		for id, n := range s.Materials {
			a.Inventory[id] -= n
		}
		a.Energy -= s.Cost
		a.Health -= selfHarm
		a.Injury = min(100, a.Injury+selfInjury)
		if s.OncePerTarget {
			if a.UsedTargetSkills == nil {
				a.UsedTargetSkills = map[string]bool{}
			}
			a.UsedTargetSkills[targetKey] = true
		}
		multiplier := 1
		if playerScale != "person" {
			if item, ok := items[a.Equipment[playerScale+":tool"]]; ok && item.Effect == "compute" {
				multiplier = item.Multiplier
			}
		}
		effects := s.Effects
		if len(effects) == 0 {
			effects = []AdventureEffect{{Kind: s.Kind, Power: s.Power}}
		}
		if in.TargetID != "" && in.TargetID != "self" && in.TargetID != "opponent" {
			return fmt.Errorf("invalid skill target")
		}
		if in.TargetID == "opponent" && len(s.EnemyEffects) > 0 {
			effects = s.EnemyEffects
		}
		for _, conditional := range s.ConditionalEffects {
			if AdventureOwnedItemCount(*a, conditional.RequiredItem) > 0 {
				effects = append(append([]AdventureEffect{}, effects...), conditional.Effects...)
			}
		}
		if !adventurePersonalShipEffectsAllowed(b, effects) {
			return fmt.Errorf("personal technique does not affect ship hulls or fleet systems")
		}
		if a.SoftLimbs && !adventureSoftRecovery(s, effects) {
			return fmt.Errorf("softened limbs cannot perform physical defense or tool techniques")
		}
		if a.MentalShock > 0 && adventureEffectsOffensive(effects) {
			return fmt.Errorf("mental shock blocks offensive techniques")
		}
		if adventureEffectsSeal(effects) {
			sealedTarget = b.OpponentID
			if b.CharacterSourceKey != "" {
				sealedTarget = b.CharacterSourceKey
			}
			if a.LastSealTarget == sealedTarget {
				return fmt.Errorf("cannot seal the same target on consecutive exchanges")
			}
		}
		usesWeapon := false
		for _, effect := range effects {
			if effect.Kind == "attack" {
				usesWeapon = true
			}
		}
		if usesWeapon {
			if err := consumeAdventureAmmunition(a.Inventory, a.Equipment, weapon.Materials); err != nil {
				return err
			}
			if weapon.AttackKind != "" {
				outgoingKind = weapon.AttackKind
			}
		}
		structuralForce := personalSpace.HullPower
		for _, effect := range effects {
			power := effect.Power * level * multiplier
			switch effect.Kind {
			case "attack":
				damage += max(0, attack+power+structuralForce-enemyDefense)
				structuralForce = 0
			case "mental_attack":
				if playerScale != "person" || b.Scale != "person" || b.Mindless || b.ElectronicDistanceMeters > 0 {
					return fmt.Errorf("mental attack requires a participating conscious target, not a mindless mechanism or hull")
				}
				// Concentration does not borrow a weapon, ammunition or hull force.
				mentalDamage += power
			case "heal":
				if playerScale != "person" {
					return fmt.Errorf("healing requires person scale")
				}
				a.Health = min(a.MaxHealth, a.Health+power)
			case "recoil":
				a.Health = max(1, a.Health-max(1, a.MaxHealth*effect.Power/100))
			case "energy":
				a.Energy = min(a.MaxEnergy, a.Energy+power)
			case "antidote":
				a.Poison = 0
			case "guard", "pilot":
				// The full defensive stance spends the whole primary action.
				// Adding a one-point guard to an attack must not buy a free
				// two-thirds reduction on every incoming blow.
				guard = guard || !adventureEffectsOffensive(effects)
				defense += power
			case "control":
				if !applyAdventureHardControl(&b.EnemyControl, &b.EnemyControlRecovery) {
					controlResisted = true
				}
				damage += max(0, power/2-enemyDefense)
			case "reflect":
				if b.Scale != "person" {
					return fmt.Errorf("force reflection requires a person-scale exchange")
				}
				reflectPercent = max(reflectPercent, min(90, power))
			case "electronic_disrupt":
				if !AdventureElectronicTargetInRange(s, b) {
					return fmt.Errorf("electronic disruption requires a registered node inside its range")
				}
				if !applyAdventureHardControl(&b.EnemyControl, &b.EnemyControlRecovery) {
					controlResisted = true
				}
			case "overclock":
				if err := overclockAdventureShip(r, a, &asset, items[b.AssetID], effect.Power, seed, nowSeconds); err != nil {
					return err
				}
			case "shield_boost":
				if !b.MentalAttack && b.AttackKind == "laser" && playerScale == "person" {
					if err := activateAdventureShieldBoost(a, items, s.RequiredTool, nowSeconds); err != nil {
						return err
					}
					shieldBoost = effect.Power
				} else {
					return fmt.Errorf("shield overcharge requires a registered laser threat")
				}
			case "mental_guard":
				mentalGuardPercent = max(mentalGuardPercent, min(90, power))
			case "drain":
				if b.Scale != "person" {
					return fmt.Errorf("inner-energy transfer requires a person")
				}
				drainAdventureEnemy(r, a, s, power)
			case "seal":
				if b.Scale != "person" {
					return fmt.Errorf("acupoint sealing requires a person")
				}
				b.EnemyRushBlocked = 1
			case "poison":
				if b.Scale != "person" {
					return fmt.Errorf("poison cannot affect machines or ships")
				}
				b.EnemyPoison = max(b.EnemyPoison, power/3)
			case "repair":
				if playerScale == "person" {
					return fmt.Errorf("repair requires an asset")
				}
				asset.Health = min(asset.MaxHealth, asset.Health+power)
			case "social", "engineering":
				return fmt.Errorf("social skills are used by task checks, not combat damage")
			default:
				return fmt.Errorf("unsupported combat effect")
			}
		}

	case "item":
		i, ok := items[in.ItemID]
		if !ok || a.Inventory[i.ID] < 1 || i.Kind != "consumable" && !(i.Kind == "tool" && i.Effect == "mental_restore") || i.Scale != playerScale {
			return fmt.Errorf("battle item unavailable or wrong scale")
		}
		if a.MentalShock > 0 && (i.Effect == "attack" || i.Effect == "poison" || i.Effect == "soften") {
			return fmt.Errorf("mental shock blocks offensive item use")
		}
		switch i.Effect {
		case "mental_restore":
			if err := restoreAdventureMentalState(r, a, i); err != nil {
				return err
			}
		case "soften":
			if b.Scale != "person" {
				return fmt.Errorf("softening agent requires a person target")
			}
			if a.SoftLimbs {
				return fmt.Errorf("softened limbs cannot deliver another dose")
			}
			b.EnemySoftLimbs = true
		case "attack":
			if a.SoftLimbs {
				return fmt.Errorf("softened limbs cannot attack")
			}
			damage = max(0, i.Power-enemyDefense)
		case "heal":
			if playerScale == "person" {
				a.Health = min(a.MaxHealth, a.Health+i.Power)
			} else {
				asset.Health = min(asset.MaxHealth, asset.Health+i.Power)
			}
		case "energy":
			a.Energy = min(a.MaxEnergy, a.Energy+i.Power)
		case "antidote":
			a.Poison = 0
			a.SoftLimbs = false
			cureAdventureFog(r, a, i.ID)
		case "poison":
			if a.SoftLimbs {
				return fmt.Errorf("softened limbs cannot deliver poison")
			}
			if b.Scale != "person" {
				return fmt.Errorf("poison cannot affect assets")
			}
			b.EnemyPoison = max(b.EnemyPoison, i.Power)
		default:
			return fmt.Errorf("item has no active battle effect")
		}
		if i.Effect != "mental_restore" {
			a.Inventory[i.ID]--
		}
	default:
		return fmt.Errorf("unknown battle choice")
	}
	if rushWasBlocked {
		a.RushBlocked = max(0, a.RushBlocked-1)
	}
	if mentalShockWasPresent && in.Choice != "listen" {
		a.MentalShock = 0
	}
	a.LastSealTarget = sealedTarget
	if in.Choice != "ceasefire" {
		b.PlayerCeasefireOffer = false
	}
	b.Round++
	out.DurationSeconds = adventureRoundSeconds
	if b.Status == "active" {
		if outgoingKind == "laser" {
			var err error
			damage, err = absorbAdventureEnemyLaser(r, a, items, damage, nowSeconds)
			if err != nil {
				return err
			}
		}
		b.Health = max(0, b.Health-damage-mentalDamage-b.EnemyPoison)
		settleAdventureVictory(r, a)
		handled, movement, action, maneuverErr := adventureEnemyNonStrike(r, a, in.enemyChoice)
		if maneuverErr != nil {
			return maneuverErr
		}
		enemyMovement, enemyAction = movement, action
		if b.Status == "victory" {
			// The paid victory has no reply.
		} else if handled {
			// Moving or negotiating spends the enemy's reply without a free attack.
		} else if b.EnemySoftLimbs {
			enemyAction = "softened"
			// The actual consumed dose leaves this participant speaking only.
			// It is not a damaging poison and does not create victory or rewards.
		} else if b.EnemyControl > 0 {
			enemyAction = "controlled"
			b.EnemyControl--
			b.EnemyControlRecovery = max(b.EnemyControlRecovery, 1)
		} else {
			b.EnemyCeasefireOffer = false
			b.EnemyLastAction = "attack"
			strike, err := adventureEnemyStrike(r, a, items, damage, mentalDamage, in.enemyChoice)
			if b.EnemyLastSkillID != "" {
				b.EnemyLastAction = "skill"
			}
			enemyPower := strike.Power
			if personalShip {
				enemyPower = adventureShipIncomingForce(enemyPower, b.ShipCombat, personalSpace)
			}
			if err != nil {
				return err
			}
			incoming := 0
			if enemyPower > 0 {
				incoming = max(0, enemyPower-defense)
				if b.MentalAttack {
					incoming = enemyPower
				}
			}
			if reflectPercent > 0 && incoming > 0 && !b.MentalAttack {
				if b.EnemyRank > a.Rank+1 {
					a.Injury = min(100, a.Injury+max(1, incoming/10))
					incoming += max(1, incoming/2)
				} else {
					reflected := incoming * reflectPercent / 100
					incoming -= reflected
					b.Health = max(0, b.Health-reflected)
				}
			}
			incoming = applyAdventureMentalThreat(a, mentalGuardPercent, incoming)
			if guard && incoming > 0 && !b.MentalAttack {
				incoming = incoming / 3
			}
			if playerScale == "person" && !b.MentalAttack && strike.Kind == "laser" {
				incoming, err = absorbAdventureLaser(a, items, incoming, nowSeconds, shieldBoost)
				if err != nil {
					return err
				}
			}
			// Only an actually used mental ward mitigates this separate component.
			// It has no corruption, stun, reflection or physical-armor side effect.
			incoming += adventureMentalDamage(strike.MentalPower, mentalGuardPercent)
			if playerScale == "person" {
				a.Health = max(0, a.Health-incoming)
				a.Injury = min(100, a.Injury+incoming/5)
			} else {
				absorbed := min(asset.Shield, incoming)
				asset.Shield -= absorbed
				asset.Health = max(0, asset.Health-(incoming-absorbed))
			}
		}
	}
	a.ControlRecovery = max(0, a.ControlRecovery-1)
	b.EnemyControlRecovery = max(0, b.EnemyControlRecovery-1)
	if b.EnemyRushBlocked > 0 {
		b.EnemyRushBlocked--
	}
	playerMovementAction := in.Choice
	if playerWasControlled {
		playerMovementAction = "controlled"
	}
	settleAdventureDisengagement(r, a, playerMovementAction, enemyAction, playerMovement, enemyMovement)
	if playerScale != "person" {
		a.Assets[b.AssetID] = asset
		if asset.Health == 0 {
			b.Status = "defeat"
		} else if asset.Fuel == 0 && b.Status == "active" {
			// The last powered round still resolves normally. If it did not win
			// or escape, exhaustion ends the encounter without a reward/softlock.
			b.Status = "defeat"
		}
	}
	if personalShip && a.Energy < personalSpace.EnergyPerRound && b.Status == "active" {
		b.Status = "defeat"
	}
	if playerScale == "person" && a.Poison > 0 && b.Status != "victory" {
		a.Health = max(0, a.Health-a.Poison)
	}
	if a.Health == 0 {
		// A unique owned escape device is consumed even when its deterministic
		// survival roll fails. Retrying a command cannot reroll this result.
		ids := make([]string, 0, len(a.Inventory))
		for id := range a.Inventory {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			i := items[id]
			if i.Effect == "revive" && a.Inventory[id] > 0 && a.Equipment[i.Scale+":tool"] == id {
				a.Inventory[id]--
				delete(a.Equipment, i.Scale+":tool")
				if int(draw[1])%100 < i.Chance {
					a.Health = max(1, a.MaxHealth/4)
					b.Status = "escaped"
				}
				break
			}
		}
		if a.Health == 0 {
			b.Status = "defeat"
		}
	}
	settleAdventureVictory(r, a)
	if b.Status == "active" && a.Health == playerHealthBefore && b.Health == enemyHealthBefore && damage+mentalDamage == 0 {
		b.NoProgressRounds = min(1000, b.NoProgressRounds+1)
	} else {
		b.NoProgressRounds = 0
	}
	out.Message = fmt.Sprintf("第%d轮：造成%d伤害，敌方剩余%d。", b.Round, max(0, enemyHealthBefore-b.Health), b.Health)
	if controlResisted {
		out.Message += " 敌方已从上轮控制中恢复，本轮未再次失去行动。"
	}
	if b.Status == "standoff" {
		out.Message += " 双方停止交锋，不分胜负，伤势和已付消耗保留。"
	}
	if b.Status == "enemy_escaped" {
		out.Message += " 对手已经脱离，未能追上；没有胜利奖励。"
	}
	if in.Choice == "escape" && b.Status == "active" {
		out.Message += fmt.Sprintf(" 撤离准备%d%%，对方仍在拦截。", b.PlayerRetreatProgress)
	}
	if b.EnemyCeasefireOffer && b.Status == "active" {
		out.Message += " 对方已提出停战，是否接受由你决定。"
	}
	if personalShip && b.Status == "defeat" && a.Energy < personalSpace.EnergyPerRound {
		out.Message += " 深空作战能力已中断，未确认安全脱离；损耗保留，没有战利。"
	}
	finishAdventureEncounter(a)
	return nil
}

// Consumption is checked completely before deductions. The caller resolves on a
// detached actor, so a later invalid effect also rolls back the entire action.
func consumeAdventureAmmunition(inventory map[string]int, equipment map[string]string, materials map[string]int) error {
	for id, n := range materials {
		if inventory[id] < n {
			return fmt.Errorf("weapon ammunition is insufficient")
		}
		for _, equipped := range equipment {
			if equipped == id {
				return fmt.Errorf("equipped item cannot be consumed as ammunition")
			}
		}
	}
	for id, n := range materials {
		inventory[id] -= n
	}
	return nil
}
