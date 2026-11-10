package cultivation

// CastPlan is the one resource price used by declaration checks, NPC choice,
// previews and resolution. Optional powered armor is included only when its
// strain benefit can be paid for; it shares the same once-per-exchange drive
// as its defensive use.
type CastPlan struct {
	Check          CastCheck
	WeaponManaCost int
	ArmorManaCost  int
	DriveArmor     bool
}

func CastPlanFor(actor Combatant, skill SkillDef, additionalManaReduction float64) CastPlan {
	weaponCost := maxInt(0, actor.drivingCost())
	price := func(strain float64, armorCost int) CastCheck {
		check := CanPerformWithStrain(skill, actor.Cultivation, actor.Attributes, actor.Burden, actor.ManaReduction+additionalManaReduction, strain, actor.HasImplement)
		// Preserve the real reason for a stage/focus/capacity refusal. The complete
		// mana figure is still useful to a preview even when capacity is too low.
		check.ManaCost += weaponCost + armorCost
		if check.Allowed && check.ManaCost > actor.Attributes.Mana {
			check.Allowed = false
			check.ReasonKey = ReasonInsufficientMana
		}
		return check
	}
	plan := CastPlan{Check: price(actor.StrainReduction, 0), WeaponManaCost: weaponCost}
	armor, ok := actor.EquipmentInSlot(EquipmentSlotGarment)
	if !ok || plan.Check.BurdenCost <= 0 || armor.StrainReduction <= 0 || armor.SwappedThisExchange || (ItemDef{ApplicableStage: armor.ApplicableStage}).UsableBy(actor.Cultivation) != nil {
		return plan
	}
	reduction := clampFloat(actor.StrainReduction+armor.StrainReduction, 0, .20)
	if reduction <= clampFloat(actor.StrainReduction, 0, .20) {
		return plan
	}
	cost := 0
	if !armor.Driven {
		cost = maxInt(0, armor.OperatingManaCost)
	}
	aided := price(reduction, cost)
	if aided.Allowed && aided.BurdenCost < plan.Check.BurdenCost {
		return CastPlan{Check: aided, WeaponManaCost: weaponCost, ArmorManaCost: cost, DriveArmor: true}
	}
	// An unaffordable optional effect never blocks a cast that works without it.
	// If the base is impossible too, its existing diagnostic stays correctable.
	return plan
}

func (c *Combatant) applyCastArmor(plan CastPlan) {
	if !plan.DriveArmor {
		return
	}
	for i := range c.Equipment {
		if c.Equipment[i].Slot == EquipmentSlotGarment {
			c.Equipment[i].Driven = true
			c.recomputeAttributes()
			return
		}
	}
}

func (c Combatant) weaponDefensePierce() int {
	weapon, ok := c.EquipmentInSlot(EquipmentSlotWeapon)
	if !ok || weapon.SwappedThisExchange || (ItemDef{ApplicableStage: weapon.ApplicableStage}).UsableBy(c.Cultivation) != nil {
		return 0
	}
	return maxInt(0, weapon.DefensePierce)
}

// NextExchangeCastPlanFor is for choices and UI between exchanges. A robe paid
// in the recorded previous round is not prepaid for the next one.
func NextExchangeCastPlanFor(actor Combatant, skill SkillDef, additionalManaReduction float64) CastPlan {
	return CastPlanFor(actor.atExchangeStart(), skill, additionalManaReduction)
}

func (c Combatant) atExchangeStart() Combatant {
	c.Equipment = append([]EquipmentCopy(nil), c.Equipment...)
	refresh := false
	for i := range c.Equipment {
		copy := &c.Equipment[i]
		copy.SwappedThisExchange = false
		if copy.Slot != EquipmentSlotWeapon && copy.OperatingManaCost > 0 {
			copy.Driven = false
			refresh = true
		}
	}
	if refresh {
		c.recomputeAttributes()
	}
	return c
}
