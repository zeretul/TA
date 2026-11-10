package cultivation

import "math"

// Equipment effect budgets are explicit portions of the unchanged rarity
// allowance. They do not create a second attack/defense multiplier.
const equipmentSpecialBudget = 0.03

func (d ItemDef) validateRatedEquipmentEffects() error {
	pierce, strain := 0, 0.0
	if d.Implement != nil {
		pierce = d.Implement.DefensePierce
	}
	if d.Armor != nil {
		strain = d.Armor.StrainReduction
	}
	if pierce < 0 || math.IsNaN(strain) || math.IsInf(strain, 0) || strain < 0 || strain > .15 {
		return ErrItemOverBudget
	}
	active := pierce > 0 || strain > 0
	if !active {
		if d.EffectStage != "" || d.RaritySpentOnSpecialEffect != 0 {
			return ErrItemBudgetCountedTwice
		}
		return nil
	}
	stage, ok := ParseStageKey(d.EffectStage)
	if !ok {
		return ErrItemUnknownStage
	}
	minimum, ok := ParseStageKey(d.ApplicableStage)
	if !ok || minimum.Realm != stage.Realm || !stageAtLeast(stage, d.ApplicableStage) {
		return ErrItemUnknownStage
	}
	budget, ok := rarityExtraBudget[d.Rarity]
	spend := d.RaritySpentOnSpecialEffect
	if !ok || math.IsNaN(spend) || math.IsInf(spend, 0) || spend < equipmentSpecialBudget ||
		math.IsNaN(d.RaritySpentOnMainStat) || math.IsInf(d.RaritySpentOnMainStat, 0) || d.RaritySpentOnMainStat < 0 ||
		spend+d.RaritySpentOnMainStat > budget+1e-9 {
		return ErrItemBudgetCountedTwice
	}
	if pierce > int(float64(BaseAttributes(stage).Defense)*.5) {
		return ErrItemOverBudget
	}
	if d.OperatingManaFraction() <= 0 || math.IsNaN(d.OperatingManaFraction()) || math.IsInf(d.OperatingManaFraction(), 0) {
		return ErrItemOverBudget
	}
	if (pierce > 0 && d.Kind != KindImplement) || (strain > 0 && d.Kind != KindArmor) {
		return ErrItemPayloadMismatch
	}
	return nil
}

// DefensePierceAmount is the weapon's fixed rated penetration. It does not grow
// with a stronger owner. Combat additionally shares the existing 20% ignore cap
// with the chosen skill and charges the weapon before using this amount.
func (d ItemDef) DefensePierceAmount(user Cultivation) int {
	if d.Kind != KindImplement || d.Implement == nil || d.UsableBy(user) != nil {
		return 0
	}
	return d.Implement.DefensePierce
}

// EquipmentStrainReduction is the robe's rated meridian support. A low-realm
// robe cannot keep its whole support on a much stronger circulation. The engine
// must pay this exchange's robe cost and combine it under the shared 20% cap.
func (d ItemDef) EquipmentStrainReduction(user Cultivation) float64 {
	if d.Kind != KindArmor || d.Armor == nil || d.Armor.StrainReduction <= 0 || d.UsableBy(user) != nil {
		return 0
	}
	rated, ok := ParseStageKey(d.EffectStage)
	if !ok || BaseAttributes(user).HealthMax <= 0 {
		return 0
	}
	scale := math.Min(1, float64(BaseAttributes(rated).HealthMax)/float64(BaseAttributes(user).HealthMax))
	return d.Armor.StrainReduction * scale
}
