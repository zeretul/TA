package cultivation

import (
	"fmt"
	"math"
)

func (r *Rules) trainingScale(m TrainingModifiers) float64 {
	growth := m.GrowthCoefficient
	if growth <= 0 {
		growth = ruleScale(r.Training.Quality, m.Quality)
	}
	health := 0.0
	for _, band := range r.Training.Health {
		if m.HealthFraction >= band.Minimum {
			health = band.Scale
			break
		}
	}
	scale := ruleScale(r.Training.Talent, m.Talent) * growth * ruleScale(r.Training.Environment, m.Environment) * ruleScale(r.Training.Resource, m.Resource) * health
	if scale <= 0 {
		return 0
	}
	return clampFloat(scale, r.Training.MinimumScale, r.Training.MaximumScale)
}

func (r *Rules) breakthroughChance(target BreakthroughTarget, m BreakthroughModifiers) (float64, bool) {
	row, ok := r.Breakthrough.Targets[target]
	if !ok {
		return 0, false
	}
	chance := row.BaseChance + r.Breakthrough.TalentBonus[m.Talent] + r.Breakthrough.QualityBonus[m.Quality]
	if m.AuxiliaryMedicine {
		chance += row.MedicineBonus
	}
	if m.StableSite {
		chance += r.Breakthrough.SiteBonus
	}
	if m.QualifiedGuardian {
		chance += r.Breakthrough.GuardianBonus
	}
	return clampFloat(chance, r.Breakthrough.MinimumChance, r.Breakthrough.MaximumChance), true
}

func (r *Rules) breakthroughRequirement(c Cultivation, accumulated float64, attrs Attributes, target BreakthroughTarget, convalescing bool) BreakthroughRequirement {
	row, ok := r.Breakthrough.Targets[target]
	stage, known := r.stage(c)
	if known && stage.ProgressRequired <= 0 {
		return BreakthroughRequirement{Reasons: []string{"当前阶段不积累修炼进度"}}
	}
	if !ok || !known {
		return BreakthroughRequirement{Reasons: []string{"目标境界未实现"}}
	}
	if accumulated < stage.ProgressRequired {
		return BreakthroughRequirement{Reasons: []string{"积累尚未达到门槛"}}
	}
	if convalescing {
		return BreakthroughRequirement{Reasons: []string{"上次突破失败后的恢复尚未结束"}}
	}
	if StageKey(c) != row.From {
		return BreakthroughRequirement{Reasons: []string{"必须处于前一大境界的最后阶段"}}
	}
	if attrs.HealthMax <= 0 || float64(attrs.Health) < r.Breakthrough.MinimumHealth*float64(attrs.HealthMax) {
		return BreakthroughRequirement{Reasons: []string{fmt.Sprintf("当前生命低于上限的%g%%", 100*r.Breakthrough.MinimumHealth)}}
	}
	if attrs.ManaMax <= 0 || float64(attrs.Mana) < r.Breakthrough.MinimumMana*float64(attrs.ManaMax) {
		return BreakthroughRequirement{Reasons: []string{fmt.Sprintf("当前灵力低于上限的%g%%", 100*r.Breakthrough.MinimumMana)}}
	}
	return BreakthroughRequirement{Eligible: true, ManaCost: int(ceilNonNegative(r.Breakthrough.ManaCost * float64(attrs.ManaMax))), Days: row.Days}
}

func (r *Rules) resolveBreakthrough(c Cultivation, accumulated float64, attrs Attributes, target BreakthroughTarget, m BreakthroughModifiers, roll float64) (BreakthroughOutcome, bool) {
	chance, ok := r.breakthroughChance(target, m)
	if !ok {
		return BreakthroughOutcome{}, false
	}
	row := r.Breakthrough.Targets[target]
	if roll < chance {
		next, ok := ParseStageKey(row.To)
		if !ok {
			return BreakthroughOutcome{}, false
		}
		next = next.WithRules(r)
		base := BaseAttributes(next)
		return BreakthroughOutcome{Succeeded: true, Cultivation: next, Health: minInt(attrs.Health, base.HealthMax), Mana: minInt(attrs.Mana, base.ManaMax)}, true
	}
	stage, ok := r.stage(c)
	if !ok {
		return BreakthroughOutcome{}, false
	}
	return BreakthroughOutcome{Cultivation: c, Accumulated: math.Max(0, accumulated-r.Breakthrough.FailureProgressLoss*stage.ProgressRequired), Health: maxInt(0, attrs.Health-int(roundHalfUp(r.Breakthrough.FailureHealthLoss*float64(attrs.HealthMax)))), Mana: attrs.Mana, RecoveryDays: row.FailureRecoveryDays}, true
}

func ruleScale[T ~string](values map[T]float64, key T) float64 {
	if value, ok := values[key]; ok {
		return value
	}
	return 1
}
