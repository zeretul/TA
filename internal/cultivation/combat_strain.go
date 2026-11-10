package cultivation

// StrainReduction uses the strongest circulating strain benefit, not an
// unbounded sum of learned methods. Merely knowing a method grants nothing:
// CharacterState.Techniques contains only the currently circulating set.
func (s CharacterState) StrainReduction() float64 {
	best := 0.0
	for _, method := range s.Techniques {
		best = max(best, method.StrainReduction)
	}
	return clampFloat(best, 0, .20)
}

// CanPerformWithStrain prices routine casts and applies circulating strain
// relief before the shared stage, focus, mana and capacity checks.
func CanPerformWithStrain(skill SkillDef, c Cultivation, attrs Attributes, burden BurdenState, manaReduction, strainReduction float64, hasRequiredImplement bool) CastCheck {
	cost := skill.Burden
	// A quick, light attack is easy to repeat. A meridian
	// injury makes even these forms strenuous. Use the authored power and
	// cost so stronger variants derived from the same templates still strain.
	if burden.Residual == 0 && cost <= 22 && skill.Power <= 1.60 && skill.Targets <= 1 &&
		!skill.IsPrepared() && !skill.LeavesResidual && skill.ShieldBaseHealthFraction == 0 {
		cost = 0
	}
	if cost > 0 {
		cost = maxInt(1, int(ceilNonNegative(float64(cost)*(1-clampFloat(strainReduction, 0, .20)))))
	}
	return canPerformWithBurden(skill, c, attrs, burden, manaReduction, hasRequiredImplement, cost)
}
