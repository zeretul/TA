package cultivation

import (
	"math"
	"testing"
)

func TestIter4MethodGrowthDoesNotDuplicateQuality(t *testing.T) {
	for _, tc := range []struct {
		name         string
		quality      TechniqueQuality
		growth, want float64
	}{
		{"legacy-grade", QualityGood, 0, 1.15}, {"good-method", QualityGood, 1.15, 1.15}, {"advanced-method", QualityAdvanced, 1.3, 1.3}, {"ordinary-selected", QualityAdvanced, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := combinedScale(TrainingModifiers{Talent: TalentNormal, Quality: tc.quality, GrowthCoefficient: tc.growth, Environment: EnvironmentOrdinary, Resource: SupportNone, HealthFraction: 1})
			if math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("one method coefficient applied twice: got=%v want=%v", got, tc.want)
			}
		})
	}
}
