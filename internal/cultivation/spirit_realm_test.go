package cultivation

import (
	"math"
	"testing"
)

func TestSpiritRealmProgressionKeepsBudgetsAndExplicitBreakthroughs(t *testing.T) {
	m := TrainingModifiers{Talent: TalentNormal, Quality: QualityOrdinary, Environment: EnvironmentOrdinary, Resource: SupportNone, HealthFraction: 1}
	for _, tc := range []struct {
		before, after Realm
		target        BreakthroughTarget
		health        int
		days          float64
	}{
		{RealmSpirit, RealmVoid, BreakthroughVoid, 625000, 360},
		{RealmVoid, RealmIntegration, BreakthroughIntegration, 3125000, 720},
		{RealmIntegration, RealmMahayana, BreakthroughMahayana, 15625000, 1080},
	} {
		t.Run(string(tc.after), func(t *testing.T) {
			prior := Cultivation{Realm: tc.before, Stage: StageLate}
			need, _, ok := ProgressRequired(prior)
			if !ok {
				t.Fatal("missing prior progress")
			}
			attrs := BaseAttributes(prior)
			before := AdvanceCultivationFrom(prior, need, 100, m)
			if before.Cultivation != prior || before.ConsumedDays != 0 {
				t.Fatalf("ordinary training crossed major realm: %+v", before)
			}
			req := CanAttemptBreakthrough(prior, need, attrs, tc.target, false)
			if !req.Eligible || req.Days != tc.days {
				t.Fatalf("unreachable breakthrough: %+v", req)
			}
			win, ok := ResolveBreakthrough(prior, need, attrs, tc.target, BreakthroughModifiers{Talent: TalentNormal, Quality: QualityOrdinary}, 0)
			if !ok || !win.Succeeded || win.Cultivation.Realm != tc.after || win.Health != attrs.Health {
				t.Fatalf("breakthrough/pools: %+v", win)
			}
			if BaseAttributes(win.Cultivation).HealthMax != tc.health {
				t.Fatal("wrong extended base")
			}
			required, monthly, _ := ProgressRequired(win.Cultivation)
			days := required/monthly*DaysPerRuleMonth + 30
			whole := AdvanceCultivationFrom(win.Cultivation, 0, days, m)
			half := AdvanceCultivationFrom(win.Cultivation, 0, days/2, m)
			rest := AdvanceCultivationFrom(half.Cultivation, half.Accumulated, days/2, m)
			if whole.Cultivation != rest.Cultivation || math.Abs(whole.Accumulated-rest.Accumulated) > .00001 {
				t.Fatalf("partition changed high growth: %+v %+v", whole, rest)
			}
			fail, ok := ResolveBreakthrough(prior, need, attrs, tc.target, BreakthroughModifiers{}, 1)
			if !ok || fail.Succeeded || fail.Cultivation != prior || fail.RecoveryDays != tc.days || fail.Health >= attrs.Health {
				t.Fatalf("failure lost real penalties: %+v", fail)
			}
		})
	}
	if AtTerminalStage(Cultivation{Realm: RealmSpirit, Stage: StageLate}) || !AtTerminalStage(Cultivation{Realm: RealmMahayana, Stage: StageLate}) {
		t.Fatal("wrong expansion boundary")
	}
}

func TestSpiritRealmLifespanIsKnownWithoutInventingAgeDeath(t *testing.T) {
	for _, realm := range []Realm{RealmVoid, RealmIntegration, RealmMahayana} {
		rule, known := LifespanRuleFor(realm)
		if !known || !rule.NoFixedLimit || rule.Years != 0 {
			t.Fatalf("%s: %+v %v", realm, rule, known)
		}
		if years, finite := LifespanYears(realm); finite || years != 0 {
			t.Fatal("invented finite age death")
		}
	}
	if _, known := LifespanRuleFor("immortal"); known {
		t.Fatal("unimplemented immortal became supported")
	}
}

func TestSpiritRealmContinuationMethodsBridgeRealPrerequisites(t *testing.T) {
	for _, method := range spiritRealmTechniques {
		if err := ValidateTechniqueDefinition(method); err != nil {
			t.Fatal(err)
		}
		from, _ := ParseStageKey(method.MinimumStage)
		to, _ := ParseStageKey(method.SupportsThrough)
		if RealmGap(to.Realm, from.Realm) != 1 || from.Stage != StageLate || to.Stage != StageLate {
			t.Fatalf("unreachable method: %+v", method)
		}
		if method.StandardLearningMinutes < 43200 {
			t.Fatal("advanced method granted free learning")
		}
	}
	stage := Cultivation{Realm: RealmVoid, Stage: StageEarly}
	offensive, _ := TechniqueByID("method.void_unification")
	defensive, _ := TechniqueByID("method.spirit_continuation")
	// At equal realms, method identity changes the tradeoff within the existing
	// shared budget. It is never a new final-damage multiplier.
	a := AggregateAttributes(BaseAttributes(stage), TechniqueModifiers([]TechniqueDef{offensive}))
	b := AggregateAttributes(BaseAttributes(stage), TechniqueModifiers([]TechniqueDef{defensive}))
	if a.Attack <= b.Attack || a.Defense >= b.Defense {
		t.Fatalf("same-realm builds have no tradeoff: %+v %+v", a, b)
	}
}
