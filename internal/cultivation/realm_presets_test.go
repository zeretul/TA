package cultivation

import "testing"

func TestRealmPresetsGateGeneratedCastingAndEquipment(t *testing.T) {
	for _, skill := range highRealmSkills {
		t.Run(skill.ID, func(t *testing.T) {
			stage, ok := ParseStageKey(skill.ApplicableStage)
			if !ok {
				t.Fatal("missing minimum stage")
			}
			template, ok := SkillTemplateByID(skill.ID)
			if !ok {
				t.Fatal("preset is absent from generation")
			}
			generated, err := template.Instantiate("云纹新术")
			if err != nil {
				t.Fatal(err)
			}
			if generated.ApplicableStage != skill.ApplicableStage || generated.CanCast(Qi(13)) {
				t.Fatal("generation lost realm gate")
			}
			registry := NewSkillRegistryWithBuiltins()
			if err := registry.Register(generated); err != nil {
				t.Fatal(err)
			}
			learned := NewLearnedSkills()
			if learned.Knows(generated.ID) {
				t.Fatal("registration granted mastery")
			}
			if err := learned.Learn(registry, generated.ID, 1); err != nil {
				t.Fatal(err)
			}
			known, ok := learned.Known(registry, generated.ID)
			if !ok {
				t.Fatal("learned art cannot resolve")
			}
			attrs := BaseAttributes(stage)
			attrs.Health, attrs.Mana = attrs.HealthMax, attrs.ManaMax
			check := CanPerform(known, stage, attrs, BurdenState{}, 0, false)
			if !check.Allowed || check.ManaCost <= 0 || check.BurdenCost <= 0 {
				t.Fatalf("invalid cast %+v", check)
			}
			after, burden := ApplyCast(check, attrs, BurdenState{})
			if after.Mana >= attrs.Mana || burden.Current == 0 {
				t.Fatal("cast did not spend resources")
			}
		})
	}
	catalog := NewItemCatalogWithFirstVersion()
	for _, item := range highRealmItems {
		def, ok := catalog.Definition(item.ID)
		if !ok {
			t.Fatalf("item %s failed validation", item.ID)
		}
		stage, _ := ParseStageKey(def.ApplicableStage)
		if def.UsableBy(Qi(13)) == nil || def.UsableBy(stage) != nil || def.OperatingManaCost(stage) <= 0 {
			t.Fatalf("item %s bypassed stage or cost", item.ID)
		}
	}
	for _, method := range realmBuildTechniques {
		if err := ValidateTechniqueDefinition(method); err != nil {
			t.Fatalf("method %s: %v", method.ID, err)
		}
		lower, _ := ParseStageKey(method.MinimumStage)
		upper, _ := ParseStageKey(method.SupportsThrough)
		if stageAtLeast(lower, method.SupportsThrough) || !stageAtLeast(upper, method.MinimumStage) {
			t.Fatalf("invalid continuation %s", method.ID)
		}
	}
}
