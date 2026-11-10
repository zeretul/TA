package cultivation

// These are original game templates, not attributed to a canonical character.
// Each is learnable at the preceding realm's last stage, so its own minimum
// never makes the breakthrough it supports impossible. Mastery and circulation
// still require the existing paid learning and preparation paths.
var spiritRealmTechniques = append([]TechniqueDef{
	{
		ID: "method.core_continuation", Version: 1, Name: "凝丹养元法",
		DefinitionDetails: DefinitionDetails{QualityGrade: QualityBlue},
		Quality:           QualityGood, Rarity: RarityUncommon, Circulation: CirculationNeutral,
		MinimumStage: "foundation_late", SupportsThrough: "core_late",
		GrowthCoefficient: 1.15, BreakthroughBonus: 0.05, StandardLearningMinutes: 43200,
		Specialty: map[AttributeName]float64{AttrDefense: 0.10, AttrAttack: -0.05}, QualityAttack: 0.05,
	},
	{
		ID: "method.nascent_continuation", Version: 1, Name: "婴元观照法",
		DefinitionDetails: DefinitionDetails{QualityGrade: QualityPurple},
		Quality:           QualityAdvanced, Rarity: RarityRare, Circulation: CirculationMindFocus,
		MinimumStage: "core_late", SupportsThrough: "nascent_late",
		GrowthCoefficient: 1.30, BreakthroughBonus: 0.10, StandardLearningMinutes: 86400,
		Specialty: map[AttributeName]float64{AttrPerception: 0.30, AttrManaMax: 0.10, AttrAttack: -0.05, AttrDefense: -0.05}, QualityAttack: 0.10,
	},
	{
		ID: "method.spirit_continuation", Version: 1, Name: "化神凝元法",
		DefinitionDetails: DefinitionDetails{QualityGrade: QualityPurple},
		Quality:           QualityAdvanced, Rarity: RarityRare, Circulation: CirculationNeutral,
		MinimumStage: "nascent_late", SupportsThrough: "spirit_late",
		GrowthCoefficient: 1.30, BreakthroughBonus: 0.10, StandardLearningMinutes: 129600,
		Specialty: map[AttributeName]float64{AttrHealthMax: 0.10, AttrDefense: 0.20, AttrAttack: -0.10, AttrAgility: -0.05}, QualityAttack: 0.10,
	},
	{
		ID: "method.void_unification", RequiredManualID: "manual.spirit.void", Version: 1, Name: "五行炼虚诀",
		DefinitionDetails: DefinitionDetails{QualityGrade: QualityOrange},
		Quality:           QualityAdvanced, Rarity: RarityRare, Circulation: CirculationNeutral,
		MinimumStage: "spirit_late", SupportsThrough: "void_late",
		GrowthCoefficient: 1.30, BreakthroughBonus: 0.10, StandardLearningMinutes: 259200,
		Specialty: map[AttributeName]float64{AttrAttack: 0.10, AttrDefense: -0.20, AttrHealthMax: -0.05}, QualityAttack: 0.10,
	},
	{
		ID: "method.integration_return", RequiredManualID: "manual.spirit.integration", Version: 1, Name: "合元归真法",
		DefinitionDetails: DefinitionDetails{QualityGrade: QualityOrange},
		Quality:           QualityAdvanced, Rarity: RarityRare, Circulation: CirculationBodyTraining,
		MinimumStage: "void_late", SupportsThrough: "integration_late",
		GrowthCoefficient: 1.30, BreakthroughBonus: 0.10, StandardLearningMinutes: 518400,
		Specialty: map[AttributeName]float64{AttrHealthMax: 0.10, AttrDefense: 0.20, AttrAttack: -0.10, AttrAgility: -0.05}, QualityAttack: 0.10,
	},
	{
		ID: "method.mahayana_insight", RequiredManualID: "manual.spirit.mahayana", Version: 1, Name: "大乘参玄经",
		DefinitionDetails: DefinitionDetails{QualityGrade: QualityRed},
		Quality:           QualityAdvanced, Rarity: RarityRare, Circulation: CirculationMindFocus,
		MinimumStage: "integration_late", SupportsThrough: "mahayana_late",
		GrowthCoefficient: 1.30, BreakthroughBonus: 0.10, StandardLearningMinutes: 1036800,
		Specialty: map[AttributeName]float64{AttrPerception: 0.30, AttrManaMax: 0.10, AttrAttack: -0.05, AttrDefense: -0.05}, QualityAttack: 0.10,
	},
}, realmBuildTechniques...)
