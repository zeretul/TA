package cultivation

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestQualityMetadataDoesNotChangeSettlementOrTemplateGrade(t *testing.T) {
	original, _ := SkillByID("fire_bolt")
	changed := original
	changed.DefinitionDetails = DefinitionDetails{QualityGrade: QualityRed, Description: "Display only", Background: "Display only"}
	c := Qi(3)
	attrs := BaseAttributes(c)
	before := CanPerform(original, c, attrs, BurdenState{}, 0, false)
	after := CanPerform(changed, c, attrs, BurdenState{}, 0, false)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("color changed cast: %+v %+v", before, after)
	}
	template, _ := SkillTemplateByID("fire_bolt")
	generated, err := template.Instantiate("独创名字")
	if err != nil {
		t.Fatal(err)
	}
	if generated.QualityGrade != original.QualityGrade || generated.Power != original.Power || generated.ManaFraction != original.ManaFraction {
		t.Fatalf("generated definition escaped approved template: %+v", generated)
	}
	var d DefinitionDetails
	if json.Unmarshal([]byte(`{"quality_grade":"gold"}`), &d) == nil {
		t.Fatal("unsupported color accepted")
	}
	if (DefinitionDetails{QualityGrade: QualityGrade("gold")}).Validate() == nil {
		t.Fatal("direct invalid color accepted")
	}
	if (DefinitionDetails{QualityGrade: QualityRed, Description: strings.Repeat("a", 8193)}).Validate() == nil {
		t.Fatal("unbounded display prose accepted")
	}
	for id, d := range BuiltinDefinitionDetails() {
		if !d.QualityGrade.Valid() {
			t.Errorf("missing explicit authored grade %s", id)
		}
	}
}
