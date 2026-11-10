package cultivation

import (
	"fmt"
	"reflect"
	"ta/internal/economy"
)

func (r *Rules) validateParameters() error {
	order := map[string]int{}
	for i, k := range r.StageOrder {
		order[k] = i
	}
	lives := map[Realm]LifespanRule{}
	for k, s := range r.Stages {
		if s.Next != "" && order[s.Next] <= order[k] {
			return fmt.Errorf("cultivation stage %q does not advance", k)
		}
		if s.Base.ManaMax < 0 || s.Base.Attack < 0 || s.Base.Defense < 0 || s.Base.Agility < 0 || s.Base.Perception < 0 || s.Base.HealthMax > 1_000_000_000 || s.Base.ManaMax > 1_000_000_000 {
			return fmt.Errorf("invalid base attributes at %q", k)
		}
		if s.Lifespan.Years < 0 || (!s.Lifespan.NoFixedLimit && s.Lifespan.Years == 0) {
			return fmt.Errorf("invalid lifespan at %q", k)
		}
		c, _ := ParseStageKey(k)
		if prior, ok := lives[c.Realm]; ok && prior != s.Lifespan {
			return fmt.Errorf("inconsistent realm lifespan at %q", k)
		}
		lives[c.Realm] = s.Lifespan
	}
	if r.Training.DaysPerMonth > 10000 || r.Training.MaximumScale > 10000 {
		return fmt.Errorf("invalid training scale")
	}
	reference := ReferenceRules()
	if !validRuleMap(r.Training.Talent, reference.Training.Talent, 0, 1000) || !validRuleMap(r.Training.Quality, reference.Training.Quality, 0, 1000) || !validRuleMap(r.Training.Environment, reference.Training.Environment, 0, 1000) || !validRuleMap(r.Training.Resource, reference.Training.Resource, 0, 1000) {
		return fmt.Errorf("incomplete training factors")
	}
	if !validRuleMap(r.Breakthrough.TalentBonus, reference.Breakthrough.TalentBonus, -1, 1) || !validRuleMap(r.Breakthrough.QualityBonus, reference.Breakthrough.QualityBonus, -1, 1) {
		return fmt.Errorf("incomplete breakthrough factors")
	}
	if len(r.Training.Health) == 0 {
		return fmt.Errorf("missing training health bands")
	}
	last := 2.0
	for _, b := range r.Training.Health {
		if b.Minimum < 0 || b.Minimum > 1 || b.Minimum >= last || b.Scale < 0 || b.Scale > 1000 {
			return fmt.Errorf("invalid training health band")
		}
		last = b.Minimum
	}
	b := r.Breakthrough
	for _, v := range []float64{b.MinimumChance, b.MaximumChance, b.MinimumHealth, b.MinimumMana, b.ManaCost, b.FailureHealthLoss, b.FailureProgressLoss} {
		if v < 0 || v > 1 {
			return fmt.Errorf("invalid breakthrough fraction")
		}
	}
	if b.MinimumChance > b.MaximumChance {
		return fmt.Errorf("inverted breakthrough chance bounds")
	}
	for _, v := range b.Targets {
		if order[v.To] <= order[v.From] || v.MedicineBonus < 0 || v.MedicineBonus > 1 {
			return fmt.Errorf("invalid breakthrough transition")
		}
	}
	if !validRuleMap(r.PositiveCaps, reference.PositiveCaps, 0, 1000) || !validRuleMap(r.NegativeCaps, reference.NegativeCaps, -1, 1000) {
		return fmt.Errorf("invalid attribute caps")
	}
	for _, mode := range []RecoveryMode{RecoveryHealthRest, RecoverySafeMeditation} {
		if r.RecoverySeconds[mode] <= 0 {
			return fmt.Errorf("invalid recovery duration %q", mode)
		}
	}
	e := r.Economy
	c := e.Candidate
	if e.CurrencyLabel == "" || e.UnitID == "" || c.CurrencyID == "" || c.AmountLimit <= 0 || c.AmountLimit > economy.MaxAmount || c.QuantityLimit <= 0 || c.QuantityLimit > economy.MaxQuantity || e.MerchantCash < 0 || e.MerchantCash > c.AmountLimit || e.MarketPermille <= 0 || e.MarketPermille > 100000 || e.MerchantBuybackPermille < 0 || e.MerchantBuybackPermille > 1000 {
		return fmt.Errorf("invalid cultivation economy")
	}
	ids := map[string]bool{}
	for _, d := range r.Techniques {
		if ids[d.ID] {
			return fmt.Errorf("duplicate technique %q", d.ID)
		}
		ids[d.ID] = true
	}
	ids = map[string]bool{}
	for _, t := range r.Talents {
		if t.ID == "" || ids[t.ID] || t.Value < -1 || t.Value > 1000 {
			return fmt.Errorf("invalid talent %q", t.ID)
		}
		if _, ok := r.PositiveCaps[t.Attribute]; !ok {
			return fmt.Errorf("unknown talent attribute %q", t.Attribute)
		}
		ids[t.ID] = true
	}
	skills := map[string]SkillDef{}
	for _, s := range r.Skills {
		skills[s.ID] = s
	}
	ids = map[string]bool{}
	for _, t := range r.SkillTemplates {
		d, ok := skills[t.Definition.ID]
		if !ok || ids[d.ID] || !reflect.DeepEqual(d, t.Definition) {
			return fmt.Errorf("invalid skill template %q", t.Definition.ID)
		}
		ids[d.ID] = true
	}
	return nil
}

func validRuleMap[T ~string](values, required map[T]float64, min, max float64) bool {
	for k := range required {
		if _, ok := values[k]; !ok {
			return false
		}
	}
	for _, v := range values {
		if v < min || v > max {
			return false
		}
	}
	return true
}
