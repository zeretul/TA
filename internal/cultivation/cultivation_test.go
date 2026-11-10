package cultivation

import (
	"math"
	"testing"
)

// TestStageTableMatchesThePublishedValues checks the rows the design states
// directly, so the table cannot drift from the document it came from.
func TestStageTableMatchesThePublishedValues(t *testing.T) {
	cases := []struct {
		name    string
		c       Cultivation
		health  int
		mana    int
		attack  int
		defense int
		agility int
		percept int
	}{
		{"mortal", Mortal(), 80, 0, 10, 5, 8, 0},
		{"qi 1", Qi(1), 100, 80, 20, 10, 10, 10},
		{"qi 6", Qi(6), 200, 160, 40, 20, 20, 20},
		{"qi 13", Qi(13), 340, 272, 68, 34, 34, 34},
		{"foundation early", Cultivation{Realm: RealmFoundation, Stage: StageEarly}, 1000, 800, 200, 100, 100, 100},
		{"foundation middle", Cultivation{Realm: RealmFoundation, Stage: StageMiddle}, 1400, 1120, 280, 140, 140, 140},
		{"foundation late", Cultivation{Realm: RealmFoundation, Stage: StageLate}, 1900, 1520, 380, 190, 190, 190},
		{"core early", Cultivation{Realm: RealmCore, Stage: StageEarly}, 5000, 4000, 1000, 500, 500, 500},
		{"core late", Cultivation{Realm: RealmCore, Stage: StageLate}, 9500, 7600, 1900, 950, 950, 950},
		{"nascent late", Cultivation{Realm: RealmNascent, Stage: StageLate}, 47500, 38000, 9500, 4750, 4750, 4750},
		{"spirit late", Cultivation{Realm: RealmSpirit, Stage: StageLate}, 237500, 190000, 47500, 23750, 23750, 23750},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BaseAttributes(tc.c)
			if got.HealthMax != tc.health || got.ManaMax != tc.mana {
				t.Errorf("health/mana = %d/%d, want %d/%d", got.HealthMax, got.ManaMax, tc.health, tc.mana)
			}
			if got.Attack != tc.attack || got.Defense != tc.defense {
				t.Errorf("attack/defense = %d/%d, want %d/%d", got.Attack, got.Defense, tc.attack, tc.defense)
			}
			if got.Agility != tc.agility || got.Perception != tc.percept {
				t.Errorf("agility/perception = %d/%d, want %d/%d", got.Agility, got.Perception, tc.agility, tc.percept)
			}
			// A freshly derived character is at full pools; the rules only ever
			// lower the current value from a real cause.
			if got.Health != got.HealthMax || got.Mana != got.ManaMax {
				t.Errorf("current pools %d/%d are not full against maxima %d/%d", got.Health, got.Mana, got.HealthMax, got.ManaMax)
			}
		})
	}
}

// TestMortalCannotUseTechniques guards the rule that zero mana must not become
// free casting. Eligibility is a stage property, checked before cost maths.
func TestMortalCannotUseTechniques(t *testing.T) {
	if CanUseTechniques(Mortal()) {
		t.Error("a mortal was allowed to perform techniques")
	}
	if !CanUseTechniques(Qi(1)) {
		t.Error("a first-layer cultivator was refused techniques")
	}
}

// TestRealmGapOrdersTheRealms checks the ordering that decides the
// cross-realm damage penalty.
func TestRealmGapOrdersTheRealms(t *testing.T) {
	cases := []struct {
		attacker, target Realm
		want             int
	}{
		{RealmQi, RealmQi, 0},
		{RealmFoundation, RealmQi, 1},
		{RealmQi, RealmFoundation, -1},
		{RealmSpirit, RealmMortal, 5},
		{Realm("unknown"), RealmQi, 0},
	}
	for _, tc := range cases {
		if got := RealmGap(tc.attacker, tc.target); got != tc.want {
			t.Errorf("RealmGap(%s, %s) = %d, want %d", tc.attacker, tc.target, got, tc.want)
		}
	}
}

// TestProgressThresholdsMatchTheTable checks 100×L² and 20×L per month.
func TestProgressThresholdsMatchTheTable(t *testing.T) {
	cases := []struct {
		layer    int
		required float64
		monthly  float64
	}{
		{1, 100, 20},
		{6, 3600, 120},
		{13, 16900, 260},
	}
	for _, tc := range cases {
		required, monthly, ok := ProgressRequired(Qi(tc.layer))
		if !ok {
			t.Fatalf("layer %d reported no progress requirement", tc.layer)
		}
		if required != tc.required || monthly != tc.monthly {
			t.Errorf("layer %d = %v/%v, want %v/%v", tc.layer, required, monthly, tc.required, tc.monthly)
		}
	}
}

// TestOneLongAdvanceEqualsTheSameTimeSplit is the design's stated consistency
// requirement: a single six-month advance and six one-month advances must reach
// the same place. It is the check that catches rounding the accumulator.
func TestOneLongAdvanceEqualsTheSameTimeSplit(t *testing.T) {
	mods := TrainingModifiers{Talent: TalentNormal, Quality: QualityOrdinary, Environment: EnvironmentOrdinary, Resource: SupportNone, HealthFraction: 1.0}

	long := AdvanceCultivation(Qi(1), 6*DaysPerRuleMonth, mods)

	split := Qi(1)
	accumulated := 0.0
	for i := 0; i < 6; i++ {
		// Feed the running total back in so the split advance carries its own
		// remainder, exactly as a sequence of real turns would.
		step := AdvanceCultivationFrom(split, accumulated, DaysPerRuleMonth, mods)
		split, accumulated = step.Cultivation, step.Accumulated
	}

	if long.Cultivation != split {
		t.Errorf("long advance reached %+v, split advance reached %+v", long.Cultivation, split)
	}
	if math.Abs(long.Accumulated-accumulated) > 1e-9 {
		t.Errorf("long advance accumulated %v, split accumulated %v", long.Accumulated, accumulated)
	}
}

// TestTheDesignWorkedExample reproduces the example in §6.1: from layer 1 with
// zero progress, six ordinary months reach layer 2 with 40/400 banked.
func TestTheDesignWorkedExample(t *testing.T) {
	mods := TrainingModifiers{Talent: TalentNormal, Quality: QualityOrdinary, Environment: EnvironmentOrdinary, Resource: SupportNone, HealthFraction: 1.0}
	got := AdvanceCultivation(Qi(1), 6*DaysPerRuleMonth, mods)
	if got.Cultivation != Qi(2) {
		t.Fatalf("reached %+v, want the second refining layer", got.Cultivation)
	}
	if math.Abs(got.Accumulated-40) > 1e-9 {
		t.Errorf("accumulated %v, want 40", got.Accumulated)
	}
}

// TestAdvanceStopsAtTheTopRefiningLayer guards the boundary before a major
// breakthrough: filling layer 13 must not roll into Foundation on its own.
func TestAdvanceStopsAtTheTopRefiningLayer(t *testing.T) {
	mods := TrainingModifiers{Talent: TalentExceptional, Quality: QualityAdvanced, Environment: EnvironmentStable, Resource: SupportStrong, HealthFraction: 1.0}
	got := AdvanceCultivation(Qi(13), 100*365*DaysPerRuleMonth, mods)
	if got.Cultivation.Realm != RealmQi {
		t.Fatalf("advancing past the top refining layer entered %s without a breakthrough", got.Cultivation.Realm)
	}
	if got.Cultivation.Layer != QiLayers {
		t.Errorf("ended on layer %d, want %d", got.Cultivation.Layer, QiLayers)
	}
}

// TestInjuredBodyBlocksCultivation checks that the body-condition scale of zero
// is a prohibition, not a slow rate that the global minimum would lift.
func TestInjuredBodyBlocksCultivation(t *testing.T) {
	mods := TrainingModifiers{Talent: TalentNormal, Quality: QualityOrdinary, Environment: EnvironmentOrdinary, Resource: SupportNone, HealthFraction: 0.10}
	got := AdvanceCultivation(Qi(1), 10*DaysPerRuleMonth, mods)
	if got.Gained != 0 {
		t.Errorf("a badly injured character gained %v progress", got.Gained)
	}
}

// TestCombinedScaleIsClamped checks the 0.20–4.00 bound on the product.
func TestCombinedScaleIsClamped(t *testing.T) {
	// Every factor at its maximum: 2.0 × 1.30 × 1.25 × 1.40 × 1.00 = 4.55.
	high := TrainingModifiers{Talent: TalentExceptional, Quality: QualityAdvanced, Environment: EnvironmentStable, Resource: SupportStrong, HealthFraction: 1.0}
	if got := combinedScale(high); math.Abs(got-4.00) > 1e-9 {
		t.Errorf("maximum scale = %v, want the 4.00 cap", got)
	}
	// Every factor at its minimum, above zero: 0.50 × 1.00 × 0.70 × 1.00 × 0.40 = 0.14.
	low := TrainingModifiers{Talent: TalentPoor, Quality: QualityOrdinary, Environment: EnvironmentPoor, Resource: SupportNone, HealthFraction: 0.50}
	if got := combinedScale(low); math.Abs(got-0.20) > 1e-9 {
		t.Errorf("minimum scale = %v, want the 0.20 floor", got)
	}
}

// TestBreakthroughChancesMatchTheTable checks the base rates and the worked
// example: ordinary talent and technique with no help is 30%; an auxiliary
// pill takes it to 55%; adding a stable site and a qualified guardian, 65%.
func TestBreakthroughChancesMatchTheTable(t *testing.T) {
	plain := BreakthroughModifiers{Talent: TalentNormal, Quality: QualityOrdinary}
	chance, ok := BreakthroughChance(BreakthroughFoundation, plain)
	if !ok || math.Abs(chance-0.30) > 1e-9 {
		t.Errorf("plain foundation chance = %v, want 0.30", chance)
	}
	withPill := BreakthroughModifiers{Talent: TalentNormal, Quality: QualityOrdinary, AuxiliaryMedicine: true}
	if chance, _ := BreakthroughChance(BreakthroughFoundation, withPill); math.Abs(chance-0.55) > 1e-9 {
		t.Errorf("with pill = %v, want 0.55", chance)
	}
	full := BreakthroughModifiers{Talent: TalentNormal, Quality: QualityOrdinary, AuxiliaryMedicine: true, StableSite: true, QualifiedGuardian: true}
	if chance, _ := BreakthroughChance(BreakthroughFoundation, full); math.Abs(chance-0.65) > 1e-9 {
		t.Errorf("with all help = %v, want 0.65", chance)
	}
}

// TestBreakthroughChanceIsClamped checks the 5%–90% bound in both directions.
func TestBreakthroughChanceIsClamped(t *testing.T) {
	worst := BreakthroughModifiers{Talent: TalentPoor, Quality: QualityOrdinary}
	if chance, _ := BreakthroughChance(BreakthroughSpirit, worst); chance < 0.05-1e-9 {
		t.Errorf("worst case = %v, want at least 0.05", chance)
	}
	best := BreakthroughModifiers{Talent: TalentExceptional, Quality: QualityAdvanced, AuxiliaryMedicine: true, StableSite: true, QualifiedGuardian: true}
	if chance, _ := BreakthroughChance(BreakthroughFoundation, best); chance > 0.90+1e-9 {
		t.Errorf("best case = %v, want at most 0.90", chance)
	}
}

// TestBreakthroughRequiresThePreconditions checks that a missing precondition
// means no attempt at all, rather than an attempt that fails and costs.
func TestBreakthroughRequiresThePreconditions(t *testing.T) {
	full := BaseAttributes(Qi(13))
	// Health below 90% of maximum.
	hurt := full
	hurt.Health = int(0.80 * float64(full.HealthMax))
	if req := CanAttemptBreakthrough(Qi(13), 16900, hurt, BreakthroughFoundation, false); req.Eligible {
		t.Error("a wounded character was allowed to attempt breakthrough")
	}
	// Progress short of the threshold.
	if req := CanAttemptBreakthrough(Qi(13), 100, full, BreakthroughFoundation, false); req.Eligible {
		t.Error("a character short of the threshold was allowed to attempt breakthrough")
	}
	// The correct position and pools.
	req := CanAttemptBreakthrough(Qi(13), 16900, full, BreakthroughFoundation, false)
	if !req.Eligible {
		t.Fatalf("a fully prepared character was refused: %v", req.Reasons)
	}
	// 炼气13层 mana maximum is 272, so the cost is ceil(0.8 × 272) = 218.
	if req.ManaCost != 218 {
		t.Errorf("mana cost = %d, want 218", req.ManaCost)
	}
	if req.Days != 7 {
		t.Errorf("attempt takes %v days, want 7", req.Days)
	}
}

// TestFailedBreakthroughAppliesTheDesignPenalties reproduces the §7.2 example:
// failing from a full layer-13 position costs 10% of the threshold, 10% of the
// health maximum, and imposes seven days of recovery.
func TestFailedBreakthroughAppliesTheDesignPenalties(t *testing.T) {
	attrs := BaseAttributes(Qi(13))
	mods := BreakthroughModifiers{Talent: TalentNormal, Quality: QualityOrdinary}
	got, ok := ResolveBreakthrough(Qi(13), 16900, attrs, BreakthroughFoundation, mods, 0.99)
	if !ok {
		t.Fatal("resolution reported the target as unsupported")
	}
	if got.Succeeded {
		t.Fatal("a roll of 0.99 against a 0.30 chance succeeded")
	}
	if math.Abs(got.Accumulated-15210) > 1e-9 {
		t.Errorf("accumulated %v, want 15210", got.Accumulated)
	}
	if got.Health != 306 {
		t.Errorf("health %d, want 306", got.Health)
	}
	if got.RecoveryDays != 7 {
		t.Errorf("recovery %v days, want 7", got.RecoveryDays)
	}
}

// TestSuccessfulBreakthroughDoesNotRefillPools checks the rule that a
// breakthrough does not restore health or mana for free.
func TestSuccessfulBreakthroughDoesNotRefillPools(t *testing.T) {
	attrs := BaseAttributes(Qi(13))
	attrs.Health = 310
	attrs.Mana = 250
	mods := BreakthroughModifiers{Talent: TalentNormal, Quality: QualityOrdinary}
	got, ok := ResolveBreakthrough(Qi(13), 16900, attrs, BreakthroughFoundation, mods, 0.0)
	if !ok || !got.Succeeded {
		t.Fatal("a roll of 0.0 against a 0.30 chance did not succeed")
	}
	if got.Cultivation.Realm != RealmFoundation || got.Cultivation.Stage != StageEarly {
		t.Errorf("entered %+v, want foundation early", got.Cultivation)
	}
	if got.Health != 310 || got.Mana != 250 {
		t.Errorf("pools became %d/%d, want the real remainders 310/250", got.Health, got.Mana)
	}
	if got.Accumulated != 0 {
		t.Errorf("accumulated %v, want a reset to 0", got.Accumulated)
	}
}

// TestSameRollGivesTheSameResult is the property a retried command depends on:
// resolution is a pure function of its inputs, so re-sending a request cannot
// re-roll it.
func TestSameRollGivesTheSameResult(t *testing.T) {
	attrs := BaseAttributes(Qi(13))
	mods := BreakthroughModifiers{Talent: TalentGood, Quality: QualityGood, AuxiliaryMedicine: true}
	first, _ := ResolveBreakthrough(Qi(13), 16900, attrs, BreakthroughFoundation, mods, 0.42)
	second, _ := ResolveBreakthrough(Qi(13), 16900, attrs, BreakthroughFoundation, mods, 0.42)
	if first != second {
		t.Errorf("the same draw produced %+v then %+v", first, second)
	}
}

// TestSplitAdvanceCarriesBankedProgress checks the case a real caller is in:
// starting part-way toward the threshold. The banked amount must be counted,
// not discarded because the call began mid-stage.
func TestSplitAdvanceCarriesBankedProgress(t *testing.T) {
	mods := TrainingModifiers{Talent: TalentNormal, Quality: QualityOrdinary, Environment: EnvironmentOrdinary, Resource: SupportNone, HealthFraction: 1.0}
	// 40 progress into layer 2 (threshold 400), then one more ordinary month at
	// 40 per month, reaches 80 rather than restarting at 40.
	got := AdvanceCultivationFrom(Qi(2), 40, DaysPerRuleMonth, mods)
	if got.Cultivation != Qi(2) {
		t.Fatalf("ended at %+v, want to stay on the second layer", got.Cultivation)
	}
	if math.Abs(got.Accumulated-80) > 1e-9 {
		t.Errorf("accumulated %v, want 80", got.Accumulated)
	}
}
