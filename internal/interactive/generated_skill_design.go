package interactive

import (
	"fmt"
	"maps"
	"math"

	"ta/internal/cultivation"
)

// GeneratedSkillDesign reallocates an approved combat form's working budget.
// Targeting, casting, preparation, focus, residual injury and recovery continue
// to use the native consumers. No prose-only effect can enter this contract.
type GeneratedSkillDesign struct {
	Power          *float64                 `json:"power,omitempty" jsonschema_description:"攻击倍率提案，正值且不超过模板1.5倍；护盾模板不填。增强须同步增加耗灵、负担与研习时间。"`
	ManaFraction   *float64                 `json:"mana_fraction,omitempty" jsonschema_description:"施展消耗的基础灵力比例，至少2%且覆盖实际效果预算。"`
	Burden         *int                     `json:"burden,omitempty" jsonschema_description:"施展经脉负担，0至100且不得低于效果预算。"`
	Targets        *int                     `json:"targets,omitempty" jsonschema_description:"攻击实际最大目标数，1至3；更多目标会提高成本，不表示当前一定有目标。"`
	HitOffset      *float64                 `json:"hit_offset,omitempty" jsonschema_description:"实际命中修正，-0.15至0.15；命中提升也计入成本。"`
	IgnoreDefense  *float64                 `json:"ignore_defense,omitempty" jsonschema_description:"穿透防御比例，0至0.20；保留本题材穿透上限并计入成本。"`
	ShieldFraction *float64                 `json:"shield_fraction,omitempty" jsonschema_description:"护盾模板可调整护盾占基础生命的比例，上限35%，攻击模板不填。"`
	Duration       *int                     `json:"duration,omitempty" jsonschema_description:"护盾持续轮数，1至6，计入真实效果预算。"`
	QualityGrade   cultivation.QualityGrade `json:"quality_grade,omitempty" jsonschema_description:"六色展示品级，不提高实际规则预算。"`
}

func generatedSkillBudget(s cultivation.SkillDef) float64 {
	if s.ShieldBaseHealthFraction > 0 {
		return s.ShieldBaseHealthFraction * float64(max(1, s.DurationExchanges))
	}
	return s.Power * float64(s.Targets) * (1 + max(0, s.HitOffset)*2) / (1 - s.IgnoreDefense)
}

func deriveGeneratedSkill(base cultivation.SkillDef, d GeneratedSkillDesign) (cultivation.SkillDef, error) {
	v := base
	for _, p := range []*float64{d.Power, d.ManaFraction, d.HitOffset, d.IgnoreDefense, d.ShieldFraction} {
		if p != nil && (math.IsNaN(*p) || math.IsInf(*p, 0)) {
			return v, fmt.Errorf("generated skill values must be finite")
		}
	}
	if d.QualityGrade != "" {
		if !d.QualityGrade.Valid() {
			return v, fmt.Errorf("invalid generated skill quality")
		}
		v.QualityGrade = d.QualityGrade
	}
	if base.ShieldBaseHealthFraction > 0 {
		if d.Power != nil || d.Targets != nil || d.HitOffset != nil || d.IgnoreDefense != nil {
			return v, fmt.Errorf("shield design cannot claim attack-only properties")
		}
		if d.ShieldFraction != nil {
			v.ShieldBaseHealthFraction = *d.ShieldFraction
		}
		if d.Duration != nil {
			v.DurationExchanges = *d.Duration
		}
		if v.ShieldBaseHealthFraction <= 0 || v.ShieldBaseHealthFraction > .35 || v.DurationExchanges < 1 || v.DurationExchanges > 6 {
			return v, fmt.Errorf("generated shield exceeds size or duration budget")
		}
	} else {
		if d.ShieldFraction != nil || d.Duration != nil {
			return v, fmt.Errorf("attack design cannot add an unregistered shield")
		}
		if d.Power != nil {
			v.Power = *d.Power
		}
		if d.Targets != nil {
			v.Targets = *d.Targets
		}
		if d.HitOffset != nil {
			v.HitOffset = *d.HitOffset
		}
		if d.IgnoreDefense != nil {
			v.IgnoreDefense = *d.IgnoreDefense
		}
		if v.Power <= 0 || v.Power > base.Power*1.5 || v.Targets < 1 || v.Targets > 3 || v.HitOffset < -0.15 || v.HitOffset > .15 || v.IgnoreDefense < 0 || v.IgnoreDefense > .2 {
			return v, fmt.Errorf("generated attack exceeds supported power, reach or accuracy budget")
		}
	}
	baseline := generatedSkillBudget(base)
	if baseline <= 0 {
		return v, fmt.Errorf("skill has no supported design budget")
	}
	ratio := generatedSkillBudget(v) / baseline
	if ratio > 4 {
		return v, fmt.Errorf("generated skill exceeds fourfold working budget")
	}
	minimumMana := max(.02, base.ManaFraction*ratio)
	minimumBurden := int(math.Ceil(float64(base.Burden)*ratio - 1e-9))
	if base.LeavesResidual {
		minimumBurden = max(minimumBurden, base.Burden)
	}
	v.ManaFraction = minimumMana
	v.Burden = minimumBurden
	if d.ManaFraction != nil {
		v.ManaFraction = *d.ManaFraction
	}
	if d.Burden != nil {
		v.Burden = *d.Burden
	}
	if v.ManaFraction+1e-9 < minimumMana || v.Burden < minimumBurden {
		return v, fmt.Errorf("generated skill does not pay its mana or meridian budget")
	}
	return v, cultivation.ValidateSkillDefinition(v)
}

// GeneratedTechniqueDesign redistributes a cultivation method's native
// specialty budget while retaining its grade, penalties, growth and gates.
type GeneratedTechniqueDesign struct {
	Specialty    map[cultivation.AttributeName]float64 `json:"specialty" jsonschema_description:"六项已有属性的实际专长分配，使用attack/defense/agility/perception/health_max/mana_max。正收益总预算不能超过模板，负面代价不能减少；各项仍受原生上限限制。"`
	QualityGrade cultivation.QualityGrade              `json:"quality_grade,omitempty" jsonschema_description:"六色展示品级，不增加修炼倍率或突破加成。"`
}

func deriveGeneratedTechnique(base cultivation.TechniqueDef, d GeneratedTechniqueDesign) (cultivation.TechniqueDef, error) {
	if d.QualityGrade != "" {
		if !d.QualityGrade.Valid() {
			return base, fmt.Errorf("invalid generated technique quality")
		}
		base.QualityGrade = d.QualityGrade
	}
	if d.Specialty == nil || len(d.Specialty) > 6 {
		return base, fmt.Errorf("generated technique needs a bounded specialty allocation")
	}
	budget := func(profile map[cultivation.AttributeName]float64) (float64, float64, error) {
		positive, negative := 0.0, 0.0
		for attribute, value := range profile {
			cap, ok := cultivation.TechniqueSourceCap(attribute)
			if !ok || math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > cap {
				return 0, 0, fmt.Errorf("generated specialty exceeds a native attribute cap")
			}
			if value > 0 {
				positive += value / cap
			} else {
				negative -= value / cap
			}
		}
		return positive, negative, nil
	}
	oldPositive, oldNegative, err := budget(base.Specialty)
	if err != nil {
		return base, err
	}
	positive, negative, err := budget(d.Specialty)
	if err != nil {
		return base, err
	}
	if positive > oldPositive+1e-9 || negative+1e-9 < oldNegative {
		return base, fmt.Errorf("generated technique increases its gains or removes its costs")
	}
	base.Specialty = maps.Clone(d.Specialty)
	return base, cultivation.ValidateTechniqueDefinition(base)
}

func cloneGeneratedSkillDesign(d *GeneratedSkillDesign) *GeneratedSkillDesign {
	if d == nil {
		return nil
	}
	v := *d
	v.Power = cloneGeneratedFloat(d.Power)
	v.ManaFraction = cloneGeneratedFloat(d.ManaFraction)
	v.HitOffset = cloneGeneratedFloat(d.HitOffset)
	v.IgnoreDefense = cloneGeneratedFloat(d.IgnoreDefense)
	v.ShieldFraction = cloneGeneratedFloat(d.ShieldFraction)
	if d.Burden != nil {
		n := *d.Burden
		v.Burden = &n
	}
	if d.Targets != nil {
		n := *d.Targets
		v.Targets = &n
	}
	if d.Duration != nil {
		n := *d.Duration
		v.Duration = &n
	}
	return &v
}

func cloneGeneratedFloat(p *float64) *float64 {
	if p == nil {
		return nil
	}
	n := *p
	return &n
}
