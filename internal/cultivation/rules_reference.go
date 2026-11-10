package cultivation

// ReferenceRules exports the existing calculator's reference values for explicit
// template creation and equivalence tests. Running stories supply their own Rules.
func ReferenceRules() *Rules {
	r := &Rules{SkillTemplates: SkillTemplates(), Economy: ReferenceEconomyRules(), RecoverySeconds: map[RecoveryMode]int64{RecoveryHealthRest: 20 * 24 * 60 * 60, RecoverySafeMeditation: 10 * 60 * 60}, Stages: map[string]StageRule{}, StageOrder: StageKeys(), Items: NewItemCatalogWithFirstVersion().Definitions(), Skills: NewSkillRegistryWithBuiltins().Definitions(), Techniques: TechniqueTable(), CraftMethods: NewCraftRegistryWithBuiltins().Methods(), Talents: InnateTalents(), PositiveCaps: map[AttributeName]float64{}, NegativeCaps: map[AttributeName]float64{}}
	for _, key := range r.StageOrder {
		c, _ := ParseStageKey(key)
		required, monthly, _ := ProgressRequired(c)
		life, _ := LifespanRuleFor(c.Realm)
		row := StageRule{Base: BaseAttributes(c), ProgressRequired: required, MonthlyProgress: monthly, Lifespan: life, Terminal: AtTerminalStage(c)}
		if next, ok := nextStage(c); ok {
			row.Next = StageKey(next)
		}
		r.Stages[key] = row
	}
	r.Training = TrainingRules{DaysPerMonth: DaysPerRuleMonth, Talent: map[Talent]float64{}, Quality: map[TechniqueQuality]float64{}, Environment: map[Environment]float64{}, Resource: map[SupportResource]float64{}, Health: []HealthTrainingBand{{.9, 1}, {.6, .8}, {.3, .4}}, MinimumScale: .2, MaximumScale: 4}
	for _, v := range []Talent{TalentPoor, TalentNormal, TalentGood, TalentExcellent, TalentExceptional} {
		r.Training.Talent[v] = talentScale(v)
	}
	for _, v := range []TechniqueQuality{QualityOrdinary, QualityGood, QualityAdvanced} {
		r.Training.Quality[v] = qualityScale(v)
	}
	for _, v := range []Environment{EnvironmentPoor, EnvironmentOrdinary, EnvironmentStable} {
		r.Training.Environment[v] = environmentScale(v)
	}
	for _, v := range []SupportResource{SupportNone, SupportNormal, SupportStrong} {
		r.Training.Resource[v] = resourceScale(v)
	}
	r.Breakthrough = BreakthroughRules{Targets: map[BreakthroughTarget]BreakthroughTargetRule{}, TalentBonus: map[Talent]float64{TalentPoor: -.1, TalentNormal: 0, TalentGood: .08, TalentExcellent: .15, TalentExceptional: .2}, QualityBonus: map[TechniqueQuality]float64{QualityOrdinary: 0, QualityGood: .05, QualityAdvanced: .1}, SiteBonus: .05, GuardianBonus: .05, MinimumChance: .05, MaximumChance: .9, MinimumHealth: .9, MinimumMana: .8, ManaCost: .8, FailureHealthLoss: .1, FailureProgressLoss: .1}
	for target, chance := range breakthroughBaseChance {
		row := BreakthroughTargetRule{To: StageKey(enteredRealm(target)), BaseChance: chance, MedicineBonus: breakthroughMedicineBonus[target], Days: breakthroughDays[target], FailureRecoveryDays: breakthroughFailureRecoveryDays[target]}
		for _, key := range r.StageOrder {
			c, _ := ParseStageKey(key)
			if atLastStageBefore(c, target) {
				row.From = key
			}
		}
		r.Breakthrough.Targets[target] = row
	}
	for _, a := range []AttributeName{AttrHealthMax, AttrManaMax, AttrAttack, AttrDefense, AttrAgility, AttrPerception} {
		r.PositiveCaps[a] = positiveCap(a)
		r.NegativeCaps[a] = negativeCap(a)
	}
	return r
}
