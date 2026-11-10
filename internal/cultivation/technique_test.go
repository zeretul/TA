package cultivation

import (
	"errors"
	"math"
	"testing"
)

// TestThePublishedWorkedExampleIsWhatThePlayerSees is §4.3's own acceptance
// example, and it is the anchor for the whole method system.
//
// 温元诀 (+10% attack specialty, −5% health, −20% defense) and 凝元护体法
// (+10% health, +20% defense, −10% attack, −5% agility) both circulating, over a
// base of 40 attack and 200 health, must give 42 attack and 210 health. The two
// quality corrections are the same grade and apply once, not twice; the negative
// entries do not cancel the positive ones of the same attribute, they subtract
// after it.
func TestThePublishedWorkedExampleIsWhatThePlayerSees(t *testing.T) {
	warm, ok := TechniqueByID("method.warm_origin")
	if !ok {
		t.Fatal("温元诀 is missing from the published table")
	}
	guard, ok := TechniqueByID("method.origin_guard")
	if !ok {
		t.Fatal("凝元护体法 is missing from the published table")
	}

	base := Attributes{HealthMax: 200, Health: 200, ManaMax: 100, Mana: 100, Attack: 40, Defense: 20, Agility: 10, Perception: 10}
	got := AggregateAttributes(base, TechniqueModifiers([]TechniqueDef{warm, guard}))

	if got.Attack != 42 {
		t.Fatalf("attack is %d, want 42", got.Attack)
	}
	if got.HealthMax != 210 {
		t.Fatalf("health max is %d, want 210", got.HealthMax)
	}
	if got.Defense != 20 {
		t.Fatalf("defense is %d, want 20 (net zero)", got.Defense)
	}
	// The design is explicit that a higher maximum does not heal: the current
	// value stays where it was.
	if got.Health != 200 {
		t.Fatalf("current health is %d, want 200 — a bonus must not heal", got.Health)
	}
}

// TestTheMethodCategoryHasItsOwnCeiling checks §4.3's method-category budget,
// which is applied before the global §3.4 caps.
//
// The quality correction shares the same 20% attack ceiling as the specialty
// bonus rather than sitting on top of it, so a set whose specialty is already at
// the ceiling keeps its specialty and the correction is the part that gives way.
func TestTheMethodCategoryHasItsOwnCeiling(t *testing.T) {
	atCeiling := TechniqueDef{
		ID: "test.ceiling", Quality: QualityGood, Circulation: CirculationNeutral,
		Specialty:     map[AttributeName]float64{AttrAttack: 0.20},
		QualityAttack: 0.05,
	}
	modifiers := TechniqueModifiers([]TechniqueDef{atCeiling})
	breakdown := AggregateAttribute(AttrAttack, 100, modifiers)
	if breakdown.Value != 120 {
		t.Fatalf("attack is %d, want 120: the method category is capped at 20 percent", breakdown.Value)
	}
	for _, id := range breakdown.Applied {
		if id == "technique_quality:test.ceiling" {
			t.Fatalf("the quality correction was applied on top of a full specialty budget: %v", breakdown.Applied)
		}
	}

	// With headroom the correction does apply, still inside the same ceiling.
	roomToSpare := atCeiling
	roomToSpare.ID = "test.headroom"
	roomToSpare.Specialty = map[AttributeName]float64{AttrAttack: 0.10}
	if got := AggregateAttribute(AttrAttack, 100, TechniqueModifiers([]TechniqueDef{roomToSpare})).Value; got != 115 {
		t.Fatalf("attack is %d, want 115", got)
	}
}

// TestOnlyTheBestSpecialtyEntryPerAttributeApplies states the aggregation rule
// for a set of three methods: the highest positive and the strongest negative
// per attribute, regardless of which method supplied them.
func TestOnlyTheBestSpecialtyEntryPerAttributeApplies(t *testing.T) {
	warm, _ := TechniqueByID("method.warm_origin")        // attack +10, defense −20
	guard, _ := TechniqueByID("method.origin_guard")      // defense +20, attack −10
	wind, _ := TechniqueByID("method.flowing_wind")       // agility +25, defense −10, attack −5
	modifiers := TechniqueModifiers([]TechniqueDef{warm, guard, wind})

	sums := map[AttributeName]float64{}
	for _, modifier := range modifiers {
		sums[modifier.Attribute] += modifier.Value
	}
	// Each attribute is the best positive specialty plus the strongest negative
	// one, with the quality correction counted once — never one entry per method.
	// Attack: 温元诀's +10% is the best positive, the 5% grade rides inside the
	// same 20% ceiling, and 凝元护体法's −10% is the strongest negative, so the
	// net is +5%. Agility: 流风行气法's +25% is the best positive and 凝元护体法's
	// −5% still counts, so the net is +20%.
	want := map[AttributeName]float64{AttrAttack: 0.05, AttrDefense: 0, AttrAgility: 0.20}
	for attribute, value := range want {
		if math.Abs(sums[attribute]-value) > 1e-9 {
			t.Fatalf("%s contribution is %v, want %v", attribute, sums[attribute], value)
		}
	}
}

// TestQualityAppliesOnceEvenAcrossManyMethods is the "highest once" rule. Three
// 良好 methods must not give three 5% corrections.
func TestQualityAppliesOnceEvenAcrossManyMethods(t *testing.T) {
	warm, _ := TechniqueByID("method.warm_origin")
	guard, _ := TechniqueByID("method.origin_guard")
	wind, _ := TechniqueByID("method.flowing_wind")

	quality := 0.0
	count := 0
	for _, modifier := range TechniqueModifiers([]TechniqueDef{warm, guard, wind}) {
		if modifier.Kind == SourceTechniqueQuality {
			quality += modifier.Value
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%d quality corrections applied, want exactly 1", count)
	}
	if quality != 0.05 {
		t.Fatalf("quality correction is %v, want 0.05", quality)
	}
}

// TestTheHigherGradeMethodSuppliesTheQualityCorrection checks that when grades
// differ the best one wins, not the first one listed.
func TestTheHigherGradeMethodSuppliesTheQualityCorrection(t *testing.T) {
	ordinary, _ := TechniqueByID("method.basic_breathing")
	advanced, _ := TechniqueByID("method.clear_mind")

	reversed := TechniqueModifiers([]TechniqueDef{advanced, ordinary})
	forward := TechniqueModifiers([]TechniqueDef{ordinary, advanced})
	if len(reversed) != len(forward) {
		t.Fatalf("aggregation depends on input order: %d vs %d entries", len(reversed), len(forward))
	}
	for index := range reversed {
		if reversed[index] != forward[index] {
			t.Fatalf("aggregation depends on input order at %d: %+v vs %+v", index, reversed[index], forward[index])
		}
	}
	for _, modifier := range forward {
		if modifier.Kind == SourceTechniqueQuality && modifier.Value != 0.10 {
			t.Fatalf("quality correction is %v, want the advanced 0.10", modifier.Value)
		}
	}
}

// TestWarmAndColdCannotRunTogether is the single conflicting pair.
func TestWarmAndColdCannotRunTogether(t *testing.T) {
	cases := []struct {
		name     string
		realm    Realm
		trait    bool
		conflict bool
	}{
		{"qi, no trait", RealmQi, false, true},
		{"qi, split channels", RealmQi, true, true},
		{"foundation, no trait", RealmFoundation, false, true},
		{"foundation, split channels", RealmFoundation, true, false},
		{"core, split channels", RealmCore, true, false},
	}
	for _, tc := range cases {
		err := CheckCirculationCompatibility(CirculationWarm, CirculationCold, tc.realm, tc.trait)
		if tc.conflict && !errors.Is(err, ErrTechniqueCirculationConflict) {
			t.Fatalf("%s: err = %v, want a conflict", tc.name, err)
		}
		if !tc.conflict && err != nil {
			t.Fatalf("%s: err = %v, want compatible", tc.name, err)
		}
	}
}

// TestEveryOtherPairIsCompatible walks the published matrix. Ownership is never
// a conflict; only simultaneous sustained circulation of warm and cold is.
func TestEveryOtherPairIsCompatible(t *testing.T) {
	for _, a := range CirculationModels {
		for _, b := range CirculationModels {
			err := CheckCirculationCompatibility(a, b, RealmQi, false)
			warmCold := (a == CirculationWarm && b == CirculationCold) || (a == CirculationCold && b == CirculationWarm)
			if warmCold && err == nil {
				t.Fatalf("%s + %s was allowed without the trait", a, b)
			}
			if !warmCold && err != nil {
				t.Fatalf("%s + %s was refused: %v", a, b, err)
			}
		}
	}
}

// TestLearningProgressFollowsThePublishedFormula checks aptitude scaling and
// the fact that a method is never learned faster than its standard time.
func TestLearningProgressFollowsThePublishedFormula(t *testing.T) {
	if got := LearningMinutesToComplete(TalentNormal, 4320); got != 4320 {
		t.Fatalf("normal aptitude needs %v minutes, want 4320", got)
	}
	if got := LearningMinutesToComplete(TalentExceptional, 4320); got != 2160 {
		t.Fatalf("exceptional aptitude needs %v minutes, want 2160", got)
	}
	if got := LearningMinutesToComplete(TalentPoor, 4320); got != 8640 {
		t.Fatalf("poor aptitude needs %v minutes, want 8640", got)
	}
	if got := LearningProgress(2160, TalentNormal, 4320); got != 0.5 {
		t.Fatalf("half the time gives %v, want 0.5", got)
	}
	// Six one-hour blocks and one six-hour block are the same elapsed time.
	segmented := 0.0
	for i := 0; i < 6; i++ {
		segmented += LearningProgress(60, TalentNormal, 4320) * 4320
	}
	whole := LearningProgress(360, TalentNormal, 4320) * 4320
	if segmented != whole {
		t.Fatalf("segmented learning is %v, whole is %v", segmented, whole)
	}
	if got := LearningProgress(999999, TalentNormal, 4320); got != 1 {
		t.Fatalf("over-learning gives %v, want 1", got)
	}
	if got := LearningProgress(100, TalentNormal, 0); got != 0 {
		t.Fatalf("a method with no standard time gave progress %v", got)
	}
}

// TestAReplacementMustBeAHigherGradeAndEligible is §4.6: a higher-grade method
// may take over, but only once mastered and only when the character qualifies.
func TestAReplacementMustBeAHigherGradeAndEligible(t *testing.T) {
	basic, _ := TechniqueByID("method.basic_breathing")
	warm, _ := TechniqueByID("method.warm_origin")
	advanced, _ := TechniqueByID("method.clear_mind")

	if err := TechniqueReplaces(basic, warm, Qi(3)); err != nil {
		t.Fatalf("温元诀 could not replace 基础吐纳法: %v", err)
	}
	if err := TechniqueReplaces(warm, basic, Qi(3)); !errors.Is(err, ErrTechniqueNotHigherGrade) {
		t.Fatalf("a lower grade replaced a higher one: %v", err)
	}
	if err := TechniqueReplaces(basic, warm, Qi(2)); !errors.Is(err, ErrTechniqueStageTooLow) {
		t.Fatalf("a method was adopted below its minimum stage: %v", err)
	}
	if err := TechniqueReplaces(basic, advanced, Qi(3)); !errors.Is(err, ErrTechniqueStageTooLow) {
		t.Fatalf("澄心观照诀 was adopted at 炼气三层: %v", err)
	}
}

// TestEveryPublishedMethodIsValid guards the table itself: the numbers were
// transcribed from the author contract, and a transcription slip should fail
// here rather than reach a player.
func TestEveryPublishedMethodIsValid(t *testing.T) {
	for _, technique := range TechniqueTable() {
		if err := ValidateTechniqueDefinition(technique); err != nil {
			t.Fatalf("%s is invalid: %v", technique.ID, err)
		}
		if _, ok := ParseStageKey(technique.MinimumStage); !ok {
			t.Fatalf("%s has an unreadable minimum stage %q", technique.ID, technique.MinimumStage)
		}
	}
}

// TestValidationRefusesAnOutOfBudgetDefinition is the "reject, do not clamp"
// rule: a definition that cannot be honoured as written must not register.
func TestValidationRefusesAnOutOfBudgetDefinition(t *testing.T) {
	valid := TechniqueDef{
		ID: "test.ok", Version: 1, Quality: QualityOrdinary, Circulation: CirculationNeutral,
		MinimumStage: "qi_1", GrowthCoefficient: 1, StandardLearningMinutes: 4320,
		Specialty: map[AttributeName]float64{AttrAttack: 0.20},
	}
	if err := ValidateTechniqueDefinition(valid); err != nil {
		t.Fatalf("the baseline definition is invalid: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*TechniqueDef)
		want   error
	}{
		{"no id", func(d *TechniqueDef) { d.ID = "" }, ErrTechniqueMissingID},
		{"unknown quality", func(d *TechniqueDef) { d.Quality = "legendary" }, ErrTechniqueUnknownQuality},
		{"unknown circulation", func(d *TechniqueDef) { d.Circulation = "spiral" }, ErrTechniqueUnknownCirculation},
		{"unknown stage", func(d *TechniqueDef) { d.MinimumStage = "qi_99" }, ErrTechniqueUnknownStage},
		{"no growth", func(d *TechniqueDef) { d.GrowthCoefficient = 0 }, ErrTechniqueGrowthOutOfRange},
		{"no learning time", func(d *TechniqueDef) { d.StandardLearningMinutes = 0 }, ErrTechniqueLearningTimeOutOfRange},
		{"attack over ceiling", func(d *TechniqueDef) { d.Specialty[AttrAttack] = 0.21 }, ErrTechniqueSpecialtyOverCap},
		{"unknown attribute", func(d *TechniqueDef) { d.Specialty["luck"] = 0.05 }, ErrTechniqueUnknownAttribute},
		{"quality over grade", func(d *TechniqueDef) { d.QualityAttack = 0.20 }, ErrTechniqueQualityAttackOutOfRange},
		{"cost reduction over cap", func(d *TechniqueDef) { d.MPCostReduction = 0.25 }, ErrTechniqueCostReductionOverCap},
	}
	for _, tc := range cases {
		def := valid
		def.Specialty = map[AttributeName]float64{AttrAttack: 0.20}
		tc.mutate(&def)
		if err := ValidateTechniqueDefinition(def); !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
}
