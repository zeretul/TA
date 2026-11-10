package cultivation

import (
	"encoding/json"
	"fmt"
)

// Rules is immutable authored data. A story freezes it and supplies it explicitly
// to its calculators; no process-wide current world or mutable global override exists.
type Rules struct {
	SkillTemplates  []SkillTemplate           `json:"skill_templates"`
	Economy         EconomyRules              `json:"economy"`
	RecoverySeconds map[RecoveryMode]int64    `json:"recovery_seconds"`
	Stages          map[string]StageRule      `json:"stages"`
	StageOrder      []string                  `json:"stage_order"`
	Training        TrainingRules             `json:"training"`
	Breakthrough    BreakthroughRules         `json:"breakthrough"`
	Items           []ItemDef                 `json:"items"`
	Skills          []SkillDef                `json:"skills"`
	Techniques      []TechniqueDef            `json:"techniques"`
	CraftMethods    []CraftMethodDef          `json:"craft_methods"`
	Talents         []InnateTalent            `json:"talents"`
	PositiveCaps    map[AttributeName]float64 `json:"positive_caps"`
	NegativeCaps    map[AttributeName]float64 `json:"negative_caps"`
}

type StageRule struct {
	Base             Attributes   `json:"base"`
	ProgressRequired float64      `json:"progress_required"`
	MonthlyProgress  float64      `json:"monthly_progress"`
	Next             string       `json:"next,omitempty"`
	Terminal         bool         `json:"terminal,omitempty"`
	Lifespan         LifespanRule `json:"lifespan"`
}

type HealthTrainingBand struct {
	Minimum float64 `json:"minimum"`
	Scale   float64 `json:"scale"`
}

type TrainingRules struct {
	DaysPerMonth float64                      `json:"days_per_month"`
	Talent       map[Talent]float64           `json:"talent"`
	Quality      map[TechniqueQuality]float64 `json:"quality"`
	Environment  map[Environment]float64      `json:"environment"`
	Resource     map[SupportResource]float64  `json:"resource"`
	Health       []HealthTrainingBand         `json:"health"`
	MinimumScale float64                      `json:"minimum_scale"`
	MaximumScale float64                      `json:"maximum_scale"`
}

type BreakthroughTargetRule struct {
	From                string  `json:"from"`
	To                  string  `json:"to"`
	BaseChance          float64 `json:"base_chance"`
	MedicineBonus       float64 `json:"medicine_bonus"`
	Days                float64 `json:"days"`
	FailureRecoveryDays float64 `json:"failure_recovery_days"`
}

type BreakthroughRules struct {
	Targets             map[BreakthroughTarget]BreakthroughTargetRule `json:"targets"`
	TalentBonus         map[Talent]float64                            `json:"talent_bonus"`
	QualityBonus        map[TechniqueQuality]float64                  `json:"quality_bonus"`
	SiteBonus           float64                                       `json:"site_bonus"`
	GuardianBonus       float64                                       `json:"guardian_bonus"`
	MinimumChance       float64                                       `json:"minimum_chance"`
	MaximumChance       float64                                       `json:"maximum_chance"`
	MinimumHealth       float64                                       `json:"minimum_health"`
	MinimumMana         float64                                       `json:"minimum_mana"`
	ManaCost            float64                                       `json:"mana_cost"`
	FailureHealthLoss   float64                                       `json:"failure_health_loss"`
	FailureProgressLoss float64                                       `json:"failure_progress_loss"`
}

func (rules *Rules) Validate() error {
	if rules == nil {
		return nil
	}
	if _, err := json.Marshal(rules); err != nil {
		return fmt.Errorf("invalid cultivation numeric data: %w", err)
	}
	if len(rules.Stages) == 0 || len(rules.StageOrder) != len(rules.Stages) || rules.Training.DaysPerMonth <= 0 || rules.Training.MinimumScale < 0 || rules.Training.MaximumScale < rules.Training.MinimumScale {
		return fmt.Errorf("invalid cultivation progression table")
	}
	seen := map[string]bool{}
	for _, key := range rules.StageOrder {
		row, exists := rules.Stages[key]
		_, known := ParseStageKey(key)
		if !exists || !known || seen[key] || row.Base.HealthMax <= 0 || row.MonthlyProgress < 0 || row.ProgressRequired < 0 {
			return fmt.Errorf("invalid cultivation stage %q", key)
		}
		seen[key] = true
		if row.Next != "" {
			if _, ok := rules.Stages[row.Next]; !ok {
				return fmt.Errorf("missing next cultivation stage %q", row.Next)
			}
		}
	}
	for target, row := range rules.Breakthrough.Targets {
		if _, ok := rules.Stages[row.From]; !ok {
			return fmt.Errorf("invalid breakthrough origin %q", target)
		}
		if _, ok := rules.Stages[row.To]; !ok {
			return fmt.Errorf("invalid breakthrough destination %q", target)
		}
		if row.Days <= 0 || row.BaseChance < 0 || row.BaseChance > 1 || row.FailureRecoveryDays < 0 {
			return fmt.Errorf("invalid breakthrough parameters %q", target)
		}
	}
	if err := rules.validateParameters(); err != nil {
		return err
	}
	items := NewItemCatalog()
	for _, d := range rules.Items {
		if err := items.Register(d); err != nil {
			return err
		}
	}
	skills := NewSkillRegistry()
	for _, d := range rules.Skills {
		if err := skills.Register(d); err != nil {
			return err
		}
	}
	for _, d := range rules.Techniques {
		if err := ValidateTechniqueDefinition(d); err != nil {
			return err
		}
	}
	return ValidateCraftMethods(rules.CraftMethods, FrozenCraftCatalog)
}

func CloneRules(rules *Rules) *Rules {
	if rules == nil {
		return nil
	}
	data, err := json.Marshal(rules)
	if err != nil {
		panic(err)
	}
	var copy Rules
	if err := json.Unmarshal(data, &copy); err != nil {
		panic(err)
	}
	return &copy
}

// WithRules binds a position to the caller's immutable rules for this operation.
// The pointer is runtime-only; the containing story owns the serialized rule snapshot.
func (c Cultivation) WithRules(rules *Rules) Cultivation { c.rules = rules; return c }

func (rules *Rules) stage(c Cultivation) (StageRule, bool) {
	if rules == nil {
		return StageRule{}, false
	}
	row, ok := rules.Stages[StageKey(c)]
	return row, ok
}
