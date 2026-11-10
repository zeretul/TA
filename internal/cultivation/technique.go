package cultivation

import "math"

// This file implements the cultivation-method (功法) rules of
// 《自制-功法与技能内容》 §4: a method is a definition with a circulation model,
// a specialty profile, a quality correction and a real learning cost.
//
// Three facts are kept apart, because the design insists on it: a manual (秘籍)
// existing, a method being registered, and a character actually having mastered
// it. Nothing here grants a method to anyone — this file only answers what a
// method *would* do once the story has established that it was learned.
//
// The specialty numbers deliberately do NOT go through the §3.1 specialisation
// template in modifier.go. That template is the single-technique shortcut used
// by characters whose state carries only free text; a structured method carries
// its own published percentages and several methods may circulate at once. Both
// paths end in the same AggregateAttributes call, so there is one budget.

// CirculationModel is the way a method moves qi. Compatibility is a property of
// the models, not of the methods' names: two 良好 methods are not automatically
// compatible, and two different methods with the same model are.
type CirculationModel string

const (
	CirculationNeutral      CirculationModel = "neutral"
	CirculationWarm         CirculationModel = "warm"
	CirculationCold         CirculationModel = "cold"
	CirculationBodyTraining CirculationModel = "body_training"
	CirculationMindFocus    CirculationModel = "mind_focus"
)

// CirculationModels lists every model the rules support, in display order.
var CirculationModels = []CirculationModel{
	CirculationNeutral,
	CirculationWarm,
	CirculationCold,
	CirculationBodyTraining,
	CirculationMindFocus,
}

// TraitSplitChannels is the innate trait that lets a character sustain warm and
// cold circulation at the same time. It is the only exception to the one
// conflicting pair in the compatibility matrix.
const TraitSplitChannels = "trait.split_channels"

// TechniqueDef is one registered cultivation method.
//
// Percentages are signed fractions: +0.10 is a 10% increase, −0.20 a 20%
// decrease. They are fractions rather than percentage points so they can enter
// the attribute budget without a conversion step that could be applied twice.
type TechniqueDef struct {
	// RequiredManualID is a held complete source, not mastery or a free unlock.
	RequiredManualID string `json:"required_manual_id,omitempty"`
	DefinitionDetails
	ID      string `json:"id,omitempty"`
	Version int    `json:"version,omitempty"`
	Name    string `json:"name,omitempty"`
	// Quality is the 普通/良好/高阶 grade. It drives the once-only attack
	// correction and the growth coefficient, which are separate budgets.
	Quality TechniqueQuality `json:"quality,omitempty"`
	// Rarity is acquisition difficulty, using the same vocabulary as items.
	Rarity      ItemRarity       `json:"rarity,omitempty"`
	Circulation CirculationModel `json:"circulation,omitempty"`
	// MinimumStage is the canonical stage key at which the method may be
	// learned. A method is not learnable below it, whatever the prose says.
	MinimumStage string `json:"minimum_stage,omitempty"`
	// SupportsThrough is the canonical stage key past which the method no
	// longer carries a cultivator forward. It does not remove mastery.
	SupportsThrough string `json:"supports_through,omitempty"`
	// GrowthCoefficient scales training progress while this method is the one
	// actually being trained. Only one method's coefficient applies to a given
	// training segment.
	GrowthCoefficient float64 `json:"growth_coefficient,omitempty"`
	// BreakthroughBonus is the additive chance correction this method grants
	// when it is the selected method for a breakthrough attempt.
	BreakthroughBonus float64 `json:"breakthrough_bonus,omitempty"`
	// Specialty is the method's per-attribute profile. Positive and negative
	// entries are budgeted separately; see TechniqueModifiers.
	Specialty map[AttributeName]float64 `json:"specialty,omitempty"`
	// QualityAttack is the once-per-active-set attack correction.
	QualityAttack float64 `json:"quality_attack,omitempty"`
	// MPCostReduction reduces a technique's mana cost while circulating.
	MPCostReduction float64 `json:"mp_cost_reduction,omitempty"`
	// StrainReduction reduces burden accumulation while circulating.
	StrainReduction float64 `json:"strain_reduction,omitempty"`
	// StandardLearningMinutes is the time a normal-aptitude cultivator needs.
	StandardLearningMinutes int `json:"standard_learning_minutes,omitempty"`
	// LearnableSkillRefs names skills this method makes learnable. Learning the
	// method does not grant them; it only makes them available.
	LearnableSkillRefs []string `json:"learnable_skill_refs,omitempty"`
}

// techniqueSourceCap is the published §4.3 ceiling on the sum of one category's
// specialty contribution, applied before the global §3.4 caps.
//
// It is a ceiling on the method category alone. Equipment, pills and formations
// each keep their own budget; the global cap then bounds the combination.
var techniqueSourceCap = map[AttributeName]float64{
	AttrAttack:     0.20,
	AttrDefense:    0.20,
	AttrAgility:    0.25,
	AttrPerception: 0.30,
	AttrHealthMax:  0.10,
	AttrManaMax:    0.10,
}

// TechniqueSourceCap returns the published ceiling for one attribute.
func TechniqueSourceCap(attribute AttributeName) (float64, bool) {
	cap, ok := techniqueSourceCap[attribute]
	return cap, ok
}

// qualityRank orders technique grades for replacement. An unknown grade is
// rejected by validation, so this only has to be total on the supported set.
func qualityRank(q TechniqueQuality) int {
	switch q {
	case QualityOrdinary:
		return 0
	case QualityGood:
		return 1
	case QualityAdvanced:
		return 2
	}
	return -1
}

// IsCirculationModel reports whether the text names a supported model.
func IsCirculationModel(model CirculationModel) bool {
	_, ok := circulationRank(model)
	return ok
}

// circulationRank gives each model a stable position so the compatibility
// lookup does not depend on map iteration order.
func circulationRank(model CirculationModel) (int, bool) {
	for index, candidate := range CirculationModels {
		if candidate == model {
			return index, true
		}
	}
	return 0, false
}

// CheckCirculationCompatibility reports whether two models may be sustained at
// the same time.
//
// The published matrix has exactly one conflicting pair: warm with cold. Every
// other combination is compatible, including two different methods that share a
// model. The warm/cold pair is permitted only for a character who both holds
// the split-channel trait and has reached Foundation — one condition is not
// enough.
//
// Ownership is never a conflict. A character may know both methods; only
// running them together is refused.
func CheckCirculationCompatibility(a, b CirculationModel, realm Realm, hasSplitChannels bool) error {
	first, ok := circulationRank(a)
	if !ok {
		return ErrTechniqueUnknownCirculation
	}
	second, ok := circulationRank(b)
	if !ok {
		return ErrTechniqueUnknownCirculation
	}
	if !isWarmColdPair(first, second) {
		return nil
	}
	realmPosition := realmIndex(realm)
	if hasSplitChannels && realmPosition >= realmIndex(RealmFoundation) {
		return nil
	}
	return ErrTechniqueCirculationConflict
}

// isWarmColdPair reports whether the two ranks are warm and cold in either
// order. Keeping it a named helper means the single conflicting pair is stated
// once rather than inferred from index arithmetic at each call site.
func isWarmColdPair(first, second int) bool {
	warmRank, _ := circulationRank(CirculationWarm)
	coldRank, _ := circulationRank(CirculationCold)
	return (first == warmRank && second == coldRank) || (first == coldRank && second == warmRank)
}

// TechniqueModifiers reduces a set of simultaneously circulating methods to the
// modifiers that actually apply.
//
// §4.3 is not "add everything up": within the method category a specialty
// positive is the highest one per attribute, and a specialty negative is the
// strongest one per attribute. Only the best quality correction across the
// whole set applies, once. The result therefore carries at most one positive
// and one negative per attribute, and the caller can hand it straight to
// AggregateAttributes where the global caps still apply on top.
//
// The input order is irrelevant: ties are broken by definition ID so a replay
// produces the same set every time.
func TechniqueModifiers(active []TechniqueDef) []AttributeModifier {
	if len(active) == 0 {
		return nil
	}
	ordered := make([]TechniqueDef, len(active))
	copy(ordered, active)
	sortTechniques(ordered)

	positive := map[AttributeName]AttributeModifier{}
	negative := map[AttributeName]AttributeModifier{}
	var quality *AttributeModifier

	for _, technique := range ordered {
		for _, attribute := range AllAttributes {
			value, ok := technique.Specialty[attribute]
			if !ok || value == 0 {
				continue
			}
			candidate := AttributeModifier{
				ID:        "technique:" + technique.ID + ":" + string(attribute),
				Kind:      SourceTechniqueSpecialisation,
				Attribute: attribute,
				Value:     value,
			}
			if value > 0 {
				if current, seen := positive[attribute]; !seen || value > current.Value {
					positive[attribute] = candidate
				}
				continue
			}
			if current, seen := negative[attribute]; !seen || value < current.Value {
				negative[attribute] = candidate
			}
		}

		if technique.QualityAttack == 0 {
			continue
		}
		candidate := AttributeModifier{
			ID:        "technique_quality:" + technique.ID,
			Kind:      SourceTechniqueQuality,
			Attribute: AttrAttack,
			Value:     technique.QualityAttack,
		}
		if quality == nil || betterQualityCorrection(*quality, candidate) {
			quality = &candidate
		}
	}

	modifiers := make([]AttributeModifier, 0, len(AllAttributes)*2+1)
	for _, attribute := range AllAttributes {
		ceiling := techniqueSourceCap[attribute]
		// The ceiling bounds the category's positive total, not each entry: a
		// specialty bonus and the quality correction are one budget, and taking
		// the best of each separately would let 20% + 10% through a 20% ceiling.
		// The stronger specialty entry is kept whole and the correction gives way
		// to it, so the player still sees which source was cut rather than a
		// silently rescaled number.
		positiveEntry, hasPositive := positive[attribute]
		correction := 0.0
		if quality != nil && quality.Attribute == attribute {
			correction = quality.Value
		}
		if hasPositive {
			allowed := math.Max(0, ceiling-positiveEntry.Value)
			modifiers = append(modifiers, positiveEntry)
			if correction > 0 && correction > allowed {
				correction = allowed
			}
		} else if correction > ceiling {
			correction = ceiling
		}
		if correction > 0 {
			entry := *quality
			entry.Value = correction
			modifiers = append(modifiers, entry)
		}
		if modifier, ok := negative[attribute]; ok {
			modifiers = append(modifiers, modifier)
		}
	}
	return modifiers
}

// betterQualityCorrection reports whether candidate should replace current.
// The highest correction wins; ties are already resolved by the sorted input,
// so an equal value keeps the earlier (lower-ID) definition.
func betterQualityCorrection(current, candidate AttributeModifier) bool {
	return candidate.Value > current.Value
}

// sortTechniques orders by ID so aggregation is deterministic.
func sortTechniques(techniques []TechniqueDef) {
	for i := 1; i < len(techniques); i++ {
		for j := i; j > 0 && techniques[j].ID < techniques[j-1].ID; j-- {
			techniques[j], techniques[j-1] = techniques[j-1], techniques[j]
		}
	}
}

// LearningProgress returns how much of a method has been learned, as a fraction
// clamped to 1.
//
// The published formula is actual learning minutes × aptitude ÷ the method's
// standard minutes. Elapsed time counts continuously, including ordinary rest,
// so six one-hour blocks equal one six-hour block. The same minutes never
// produce realm progress: learning a method and cultivating are separate
// budgets.
func LearningProgress(actualMinutes float64, aptitude Talent, standardMinutes int, profiles ...*Rules) float64 {
	required := LearningMinutesToComplete(aptitude, standardMinutes, profiles...)
	if required <= 0 {
		return 0
	}
	if actualMinutes <= 0 {
		return 0
	}
	return math.Min(1, actualMinutes/required)
}

// LearningMinutesToComplete returns the elapsed minutes a character of the given
// aptitude needs for one method.
//
// A standard of zero or less is treated as unlearnable rather than instantly
// learned, so a malformed definition cannot hand out a method for free.
func LearningMinutesToComplete(aptitude Talent, standardMinutes int, profiles ...*Rules) float64 {
	if standardMinutes <= 0 {
		return 0
	}
	scale := talentScale(aptitude)
	if len(profiles) > 0 && profiles[0] != nil {
		scale = ruleScale(profiles[0].Training.Talent, aptitude)
	}
	if scale <= 0 {
		return 0
	}
	return float64(standardMinutes) / scale
}

// TechniqueReplaces reports whether next may take over from current as the
// trained or circulating method.
//
// §4.6: a higher grade may replace a lower one, but only once the new method has
// actually been mastered and the character is eligible for it. The old method is
// not destroyed — it stays mastered and may be circulated again later — so this
// function decides an ordering, not a deletion. Everything else (realm, layer
// progress, learned skills, history, traits, inventory) is untouched by a
// replacement and is deliberately not a parameter here.
func TechniqueReplaces(current, next TechniqueDef, realm Cultivation) error {
	if next.ID == "" {
		return ErrTechniqueMissingID
	}
	if qualityRank(next.Quality) <= qualityRank(current.Quality) {
		return ErrTechniqueNotHigherGrade
	}
	if !stageAtLeast(realm, next.MinimumStage) {
		return ErrTechniqueStageTooLow
	}
	return nil
}

// stageAtLeast reports whether current is at or above the canonical minimum
// stage. An unreadable minimum is treated as unreachable: a definition that
// cannot state its own requirement must not be assumed to have none.
func stageAtLeast(current Cultivation, minimumKey string) bool {
	minimum, ok := ParseStageKey(minimumKey)
	if !ok {
		return false
	}
	currentRank, minimumRank := realmIndex(current.Realm), realmIndex(minimum.Realm)
	if currentRank < 0 || minimumRank < 0 {
		return false
	}
	if currentRank != minimumRank {
		return currentRank > minimumRank
	}
	if current.Realm == RealmQi {
		return current.Layer >= minimum.Layer
	}
	if current.Realm == RealmMortal {
		return true
	}
	currentStage, minimumStage := stageIndex(current.Stage), stageIndex(minimum.Stage)
	if currentStage < 0 || minimumStage < 0 {
		return false
	}
	return currentStage >= minimumStage
}

// ValidateTechniqueDefinition refuses a method that exceeds the supported rules,
// before it can be registered.
//
// The design rejects registering first and clamping afterwards: a definition
// that cannot be honoured as written must not exist at all, because the player
// would otherwise be shown a method whose promised numbers are silently cut.
func ValidateTechniqueDefinition(technique TechniqueDef) error {
	if err := technique.DefinitionDetails.Validate(); err != nil {
		return err
	}
	if technique.QualityGrade != "" && !technique.QualityGrade.Valid() {
		return ErrTechniqueUnknownQuality
	}
	if technique.ID == "" {
		return ErrTechniqueMissingID
	}
	if technique.Version < 1 {
		return ErrTechniqueVersionOutOfRange
	}
	if qualityRank(technique.Quality) < 0 {
		return ErrTechniqueUnknownQuality
	}
	if !IsCirculationModel(technique.Circulation) {
		return ErrTechniqueUnknownCirculation
	}
	if _, ok := ParseStageKey(technique.MinimumStage); !ok {
		return ErrTechniqueUnknownStage
	}
	if technique.SupportsThrough != "" {
		if _, ok := ParseStageKey(technique.SupportsThrough); !ok {
			return ErrTechniqueUnknownStage
		}
	}
	if technique.GrowthCoefficient <= 0 || math.IsNaN(technique.GrowthCoefficient) || math.IsInf(technique.GrowthCoefficient, 0) {
		return ErrTechniqueGrowthOutOfRange
	}
	if technique.StandardLearningMinutes <= 0 {
		return ErrTechniqueLearningTimeOutOfRange
	}
	if technique.QualityAttack < 0 || technique.QualityAttack > techniqueGradeAttackBonus[QualityAdvanced] {
		return ErrTechniqueQualityAttackOutOfRange
	}
	if technique.MPCostReduction < 0 || technique.MPCostReduction > 0.20 {
		return ErrTechniqueCostReductionOverCap
	}
	if technique.StrainReduction < 0 || technique.StrainReduction > 0.20 {
		return ErrTechniqueCostReductionOverCap
	}
	for attribute, value := range technique.Specialty {
		ceiling, ok := techniqueSourceCap[attribute]
		if !ok {
			return ErrTechniqueUnknownAttribute
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return ErrTechniqueNonFiniteValue
		}
		if value > ceiling || value < -ceiling {
			return ErrTechniqueSpecialtyOverCap
		}
	}
	return nil
}

// builtinTechniques is the published 凡人 set. Numbers are the author contract's
// method entries; the quality correction is kept separate from the specialty
// profile because §4.3 budgets them separately.
var builtinTechniques = append([]TechniqueDef{
	{
		ID: "method.basic_breathing", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("white")}, Version: 1, Name: "基础吐纳法",
		Quality: QualityOrdinary, Rarity: RarityCommon,
		Circulation:       CirculationNeutral,
		MinimumStage:      "qi_1",
		SupportsThrough:   "foundation_late",
		GrowthCoefficient: 1.00, StandardLearningMinutes: 4320,
		Specialty: map[AttributeName]float64{},
	},
	{
		ID: "method.spirit_beast_breathing", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("white")}, Version: 1, Name: "本能吐纳",
		Quality: QualityOrdinary, Rarity: RarityCommon,
		Circulation:  CirculationNeutral,
		MinimumStage: "qi_1", SupportsThrough: "foundation_late",
		GrowthCoefficient: 0.60, StandardLearningMinutes: 4320,
		Specialty: map[AttributeName]float64{},
	},
	{
		ID: "method.warm_origin", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("blue")}, Version: 1, Name: "温元诀",
		Quality: QualityGood, Rarity: RarityUncommon,
		Circulation:       CirculationWarm,
		MinimumStage:      "qi_3",
		SupportsThrough:   "foundation_late",
		GrowthCoefficient: 1.15, BreakthroughBonus: 0.05,
		StandardLearningMinutes: 14400,
		Specialty: map[AttributeName]float64{
			AttrAttack:    0.10,
			AttrHealthMax: -0.05,
			AttrDefense:   -0.20,
		},
		QualityAttack:      0.05,
		LearnableSkillRefs: []string{"skill.fire_bolt", "skill.fire_wave"},
	},
	{
		ID: "method.cold_spring", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("blue")}, Version: 1, Name: "寒泉行气诀",
		Quality: QualityGood, Rarity: RarityRare,
		Circulation:       CirculationCold,
		MinimumStage:      "qi_3",
		SupportsThrough:   "foundation_late",
		GrowthCoefficient: 1.15, BreakthroughBonus: 0.05,
		StandardLearningMinutes: 14400,
		Specialty:               map[AttributeName]float64{},
		QualityAttack:           0.05,
	},
	{
		ID: "method.origin_guard", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("blue")}, Version: 1, Name: "凝元护体法",
		Quality: QualityGood, Rarity: RarityUncommon,
		Circulation:       CirculationNeutral,
		MinimumStage:      "qi_3",
		SupportsThrough:   "foundation_late",
		GrowthCoefficient: 1.15, BreakthroughBonus: 0.05,
		StandardLearningMinutes: 14400,
		Specialty: map[AttributeName]float64{
			AttrHealthMax: 0.10,
			AttrDefense:   0.20,
			AttrAttack:    -0.10,
			AttrAgility:   -0.05,
		},
		QualityAttack:      0.05,
		LearnableSkillRefs: []string{"skill.body_shield"},
	},
	{
		ID: "method.flowing_wind", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("blue")}, Version: 1, Name: "流风行气法",
		Quality: QualityGood, Rarity: RarityUncommon,
		Circulation:       CirculationNeutral,
		MinimumStage:      "qi_3",
		SupportsThrough:   "foundation_late",
		GrowthCoefficient: 1.15, BreakthroughBonus: 0.05,
		StandardLearningMinutes: 14400,
		Specialty: map[AttributeName]float64{
			AttrAgility: 0.25,
			AttrDefense: -0.10,
			AttrAttack:  -0.05,
		},
		QualityAttack: 0.05,
	},
	{
		ID: "method.clear_mind", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("purple")}, Version: 1, Name: "澄心观照诀",
		Quality: QualityAdvanced, Rarity: RarityRare,
		Circulation:       CirculationMindFocus,
		MinimumStage:      "qi_6",
		SupportsThrough:   "foundation_late",
		GrowthCoefficient: 1.30, BreakthroughBonus: 0.10,
		StandardLearningMinutes: 43200,
		Specialty: map[AttributeName]float64{
			AttrPerception: 0.30,
			AttrManaMax:    0.10,
			AttrAttack:     -0.05,
			AttrDefense:    -0.05,
		},
		QualityAttack: 0.10,
	},
	{
		ID: "method.body_cycle", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("green")}, Version: 1, Name: "锻体小周天",
		Quality: QualityOrdinary, Rarity: RarityUncommon,
		Circulation:       CirculationBodyTraining,
		MinimumStage:      "qi_1",
		SupportsThrough:   "foundation_late",
		GrowthCoefficient: 1.00, StandardLearningMinutes: 4320,
		Specialty: map[AttributeName]float64{
			AttrHealthMax: 0.10,
			AttrDefense:   0.05,
			AttrAgility:   -0.05,
		},
	},
	{
		ID: "method.economical_breath", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("green")}, Version: 1, Name: "节息诀",
		Quality: QualityOrdinary, Rarity: RarityRare,
		Circulation:       CirculationNeutral,
		MinimumStage:      "qi_1",
		SupportsThrough:   "foundation_late",
		GrowthCoefficient: 1.00, StandardLearningMinutes: 4320,
		Specialty:       map[AttributeName]float64{},
		MPCostReduction: 0.05,
	},
}, spiritRealmTechniques...)

// TechniqueTable returns the published methods in registration order.
func TechniqueTable(rules ...*Rules) []TechniqueDef {
	if len(rules) > 0 && rules[0] != nil {
		return append([]TechniqueDef(nil), rules[0].Techniques...)
	}
	out := make([]TechniqueDef, len(builtinTechniques))
	copy(out, builtinTechniques)
	return out
}

// TechniqueByID returns one published method.
func TechniqueByID(id string, rules ...*Rules) (TechniqueDef, bool) {
	for _, technique := range TechniqueTable(rules...) {
		if technique.ID == id {
			return technique, true
		}
	}
	return TechniqueDef{}, false
}
