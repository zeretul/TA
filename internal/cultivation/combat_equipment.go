package cultivation

// driveTreasure enables a registered numeric effect when combat actually uses
// it: attack on a strike, defence when targeted, perception during pursuit or
// escape. The cost is paid once per exchange, even against multiple opponents.
// Merely wearing it, resting, swapping it in or naming an unsupported special
// effect does not cast another ability. Insufficient mana leaves the ordinary
// action available without the optional treasure bonus.
func (c *Combatant) driveTreasure(attribute AttributeName) int {
	spent := 0
	changed := false
	for i := range c.Equipment {
		copy := &c.Equipment[i]
		if copy.Slot == EquipmentSlotWeapon {
			continue
		}
		if copy.Driven || copy.SwappedThisExchange || (ItemDef{ApplicableStage: copy.ApplicableStage}).UsableBy(c.Cultivation) != nil {
			continue
		}
		used := false
		for _, modifier := range copy.Modifiers {
			if modifier.Attribute == attribute && modifier.Value != 0 {
				used = true
			}
		}
		if !used {
			continue
		}
		cost := maxInt(0, copy.OperatingManaCost)
		if cost > c.Attributes.Mana {
			continue
		}
		c.Attributes.Mana -= cost
		copy.Driven = true
		spent += cost
		changed = true
	}
	if changed {
		c.recomputeAttributes()
	}
	return spent
}
