package cultivation

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestWorldRulesPreserveReferenceCalculations(t *testing.T) {
	rules := ReferenceRules()
	if err := rules.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, key := range StageKeys() {
		t.Run(key, func(t *testing.T) {
			stage, _ := ParseStageKey(key)
			bound := stage.WithRules(rules)
			if got, want := BaseAttributes(bound), BaseAttributes(stage); got != want {
				t.Fatalf("attributes=%+v, want %+v", got, want)
			}
			for _, talent := range []Talent{"", TalentPoor, TalentExceptional} {
				m := TrainingModifiers{Talent: talent, HealthFraction: 1}
				got := AdvanceCultivationFrom(bound, 0, 360, m)
				want := AdvanceCultivationFrom(stage, 0, 360, m)
				got.Cultivation = got.Cultivation.WithRules(nil)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("training differs for %s: %+v / %+v", talent, got, want)
				}
			}
			for target := range rules.Breakthrough.Targets {
				required, _, _ := ProgressRequired(stage)
				attrs := BaseAttributes(stage)
				if got, want := CanAttemptBreakthrough(bound, required, attrs, target, false), CanAttemptBreakthrough(stage, required, attrs, target, false); !reflect.DeepEqual(got, want) {
					t.Fatalf("requirement differs: %+v / %+v", got, want)
				}
			}
		})
	}
}

func TestTwoWorldRuleSnapshotsRemainIndependent(t *testing.T) {
	first := ReferenceRules()
	second := CloneRules(first)
	row := second.Stages["qi_1"]
	row.Base.HealthMax *= 2
	row.Base.Attack *= 3
	row.MonthlyProgress *= 2
	second.Stages["qi_1"] = row
	stage, _ := ParseStageKey("qi_1")
	a, b := stage.WithRules(first), stage.WithRules(second)
	if BaseAttributes(b).HealthMax != 2*BaseAttributes(a).HealthMax {
		t.Fatal("world base tables shared")
	}
	m := TrainingModifiers{HealthFraction: 1}
	if AdvanceCultivation(b, 1, m).Gained != 2*AdvanceCultivation(a, 1, m).Gained {
		t.Fatal("world progress tables shared")
	}
	data, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	var restored Rules
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if err = restored.Validate(); err != nil {
		t.Fatal(err)
	}
	second.Stages["qi_1"] = StageRule{}
	if BaseAttributes(stage.WithRules(&restored)).HealthMax != 2*BaseAttributes(a).HealthMax {
		t.Fatal("frozen rule data changed with its author")
	}
}
