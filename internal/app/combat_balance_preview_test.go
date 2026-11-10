package app

import (
	"ta/internal/cultivation"
	"testing"
)

func TestCombatWindowPreviewUsesActualCirculatingManaAndStrain(t *testing.T) {
	stage := cultivation.Cultivation{Realm: cultivation.RealmCore, Stage: cultivation.StageEarly}
	skill, _ := cultivation.SkillByID("core_pierce")
	actor := cultivation.Combatant{Cultivation: stage, Attributes: cultivation.BaseAttributes(stage), Burden: cultivation.BurdenState{Current: 62}, Skills: []cultivation.SkillDef{skill}}
	ordinary := combatWindowSkills(actor)
	actor.ManaReduction = .15
	actor.StrainReduction = .10
	reduced := combatWindowSkills(actor)
	if len(ordinary) != 1 || len(reduced) != 1 {
		t.Fatal("missing skill preview")
	}
	if ordinary[0].Castable || !reduced[0].Castable || reduced[0].BurdenCost != 38 || reduced[0].ManaCost >= ordinary[0].ManaCost {
		t.Fatalf("preview did not match actual paid cast: ordinary=%+v reduced=%+v", ordinary, reduced)
	}
}

func TestCombatWindowPreviewIncludesOnlyAffordablePoweredStabilization(t *testing.T) {
	stage := cultivation.Cultivation{Realm: cultivation.RealmCore, Stage: cultivation.StageEarly}
	skill, _ := cultivation.SkillByID("core_pierce")
	actor := cultivation.Combatant{Cultivation: stage, Attributes: cultivation.BaseAttributes(stage), Burden: cultivation.BurdenState{Current: 64}, Skills: []cultivation.SkillDef{skill}, HasImplement: true, Equipment: []cultivation.EquipmentCopy{{Slot: cultivation.EquipmentSlotWeapon, OperatingManaCost: 12}, {Slot: cultivation.EquipmentSlotGarment, OperatingManaCost: 20, StrainReduction: .15}}}
	preview := combatWindowSkills(actor)
	plan := cultivation.CastPlanFor(actor, skill, 0)
	if len(preview) != 1 || !preview[0].Castable || preview[0].ManaCost != skill.ManaCost(stage)+32 || preview[0].ManaCost != plan.Check.ManaCost || preview[0].BurdenCost != 36 {
		t.Fatalf("powered preview does not show actual total cast: %+v", preview)
	}
	actor.Attributes.Mana = skill.ManaCost(stage) + 12
	actor.Equipment[1].Driven = true // Last round payment is not prepayment for the next action.
	unpaid := combatWindowSkills(actor)
	if unpaid[0].Castable || unpaid[0].BurdenCost != 42 {
		t.Fatal("unaffordable armor offered a free cast in the preview")
	}
}
