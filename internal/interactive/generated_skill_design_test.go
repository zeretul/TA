package interactive

import (
	"math"
	"reflect"
	"testing"

	"ta/internal/cultivation"
)

func TestGeneratedCultivationSkillDesignSpendsStudyAndCastingCosts(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, "qi_7", nil)
	base, _ := cultivation.SkillByID("fire_bolt")
	request := TurnSkillLearning{TemplateID: "fire_bolt", Name: "凝火一线", Source: "完整火术心得", Description: "火光凝成细线。", SkillDesign: &GeneratedSkillDesign{Power: generatedBonus(base.Power * 1.4)}}
	ops, result, _ := xiuxianSkillLearningOps(state, system, DefaultActorID, TurnSkillLearningInput{Skills: []TurnSkillLearning{request}}, "register")
	if len(result.Refused) > 0 || len(result.Learned) > 0 {
		t.Fatalf("registration must not grant mastery: %+v", result)
	}
	registered := appliedXiuxianState(t, state, ops)
	id := cultivation.GeneratedSkillID("fire_bolt", request.Name)
	def, ok := storySkillRegistryFromState(cultivation.ReferenceRules(),
		registered).Current(id)
	if !ok || def.Power <= base.Power || def.ManaFraction <= base.ManaFraction || def.Burden < base.Burden {
		t.Fatalf("strong skill did not price actual mechanics: %+v", def)
	}
	required := skillStudyMinutes(def, registered, DefaultActorID)
	baseTime := skillStudyMinutes(base, state, DefaultActorID)
	if required != baseTime*2 {
		t.Fatalf("strong skill learning did not increase: %v %v", required, baseTime)
	}
	studyOps, studied, _ := xiuxianSkillLearningOps(registered, system, DefaultActorID, TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: id, Minutes: required}}}, "study")
	if len(studied.Learned) != 1 || len(studied.Refused) > 0 {
		t.Fatalf("study cannot finish: %+v", studied)
	}
	learned := appliedXiuxianState(t, registered, studyOps)
	registry := storySkillRegistryFromState(cultivation.ReferenceRules(),
		learned)
	if !learnedSkillsFromState(learned, DefaultActorID, registry).Knows(id) {
		t.Fatal("learned definition is absent from cast set")
	}
	current, _ := registry.Current(id)
	stage := cultivation.Qi(7)
	attrs := cultivation.BaseAttributes(stage)
	attrs.Mana = attrs.ManaMax
	cast := cultivation.CanPerform(current, stage, attrs, cultivation.BurdenState{}, 0, true)
	if !cast.Allowed || cast.ManaCost <= base.ManaCost(stage) {
		t.Fatalf("native cast ignores design: %+v", cast)
	}
	target := attrs
	target.Defense = 10
	strongHit := cultivation.ResolveAttack(attrs, target, stage.Realm, stage.Realm, current, cultivation.NeutralDamageContext(), cultivation.Shield{}, cultivation.CombatRolls{Hit: 0, Damage: .5})
	baseHit := cultivation.ResolveAttack(attrs, target, stage.Realm, stage.Realm, base, cultivation.NeutralDamageContext(), cultivation.Shield{}, cultivation.CombatRolls{Hit: 0, Damage: .5})
	if strongHit.Damage <= baseHit.Damage {
		t.Fatalf("native attack ignored generated power: %+v versus %+v", strongHit, baseHit)
	}
	paid, burden := cultivation.ApplyCast(cast, attrs, cultivation.BurdenState{})
	if paid.Mana != attrs.Mana-cast.ManaCost || burden.Current != current.Burden {
		t.Fatal("actual skill cost did not settle")
	}
	altered := request
	altered.SkillDesign = &GeneratedSkillDesign{Power: generatedBonus(base.Power * 1.2)}
	if ops, result, _ := xiuxianSkillLearningOps(learned, system, DefaultActorID, TurnSkillLearningInput{Skills: []TurnSkillLearning{altered}}, "rewrite"); len(ops) != 0 || len(result.Refused) != 1 {
		t.Fatal("same skill identity rewrote settled mechanics")
	}
}

func TestGeneratedCultivationSkillRejectsFreeAndNonfiniteDesign(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, "qi_7", nil)
	for _, design := range []GeneratedSkillDesign{{Power: generatedBonus(1000)}, {Power: generatedBonus(math.NaN())}, {ManaFraction: generatedBonus(0)}, {ShieldFraction: generatedBonus(.2)}, {IgnoreDefense: generatedBonus(.21)}} {
		request := TurnSkillLearning{TemplateID: "fire_bolt", Name: "非法新术", Source: "测试完整资料", SkillDesign: &design}
		if ops, result, _ := xiuxianSkillLearningOps(state, system, DefaultActorID, TurnSkillLearningInput{Skills: []TurnSkillLearning{request}}, "reject"); len(ops) != 0 || len(result.Refused) != 1 {
			t.Fatalf("invalid design accepted: %+v %+v", design, result)
		}
	}
}

func TestGeneratedTechniqueDesignRedistributesRealSpecialty(t *testing.T) {
	base, _ := cultivation.TechniqueByID("method.body_cycle")
	design := GeneratedTechniqueDesign{Specialty: map[cultivation.AttributeName]float64{cultivation.AttrHealthMax: .05, cultivation.AttrDefense: .15, cultivation.AttrAgility: -.05}}
	derived, err := deriveGeneratedTechnique(base, design)
	if err != nil {
		t.Fatal(err)
	}
	if derived.Specialty[cultivation.AttrDefense] != .15 || reflect.DeepEqual(base.Specialty, derived.Specialty) {
		t.Fatal("allocation did not change native mechanics")
	}
	request := TurnTechniqueLearning{TemplateID: base.ID, Name: "护骨周天", Source: "研习完整锻体法后调整行功", Description: "气息贴骨游走。", Design: &design}
	definition, record, ok := techniqueLearningDefinition(cultivation.ReferenceRules(),
		nil, map[string]any{}, request)
	if !ok || record == nil || definition.Specialty[cultivation.AttrDefense] != .15 {
		t.Fatal("design was not registered")
	}
	replay, ok := generatedTechniqueFromRecord(cultivation.ReferenceRules(),
		record)
	if !ok || !reflect.DeepEqual(definition, replay) {
		t.Fatal("specialty changed during journal projection")
	}
	// Native modifier aggregation sees the new defense rather than a display-only description.
	oldModifiers := cultivation.TechniqueModifiers([]cultivation.TechniqueDef{base})
	newModifiers := cultivation.TechniqueModifiers([]cultivation.TechniqueDef{derived})
	if reflect.DeepEqual(oldModifiers, newModifiers) {
		t.Fatal("native modifier consumer ignored specialty")
	}
	for _, invalid := range []GeneratedTechniqueDesign{
		{Specialty: map[cultivation.AttributeName]float64{cultivation.AttrDefense: .2}},
		{Specialty: map[cultivation.AttributeName]float64{cultivation.AttrDefense: math.NaN()}},
		{Specialty: map[cultivation.AttributeName]float64{"luck": .1}},
	} {
		request.Design = &invalid
		if _, _, ok := techniqueLearningDefinition(cultivation.ReferenceRules(),
			nil, map[string]any{}, request); ok {
			t.Fatal("unbudgeted specialty accepted")
		}
	}
}

func TestGeneratedProposalDraftCloneIsIndependent(t *testing.T) {
	in := &TurnSkillLearningInput{Skills: []TurnSkillLearning{{SkillDesign: &GeneratedSkillDesign{Power: generatedBonus(1.3)}, CraftDesign: &GeneratedCraftDesign{Item: GeneratedItemDesign{MainBonus: generatedBonus(.1)}, Inputs: []cultivation.CraftInput{{DefID: "ore", Quantity: 3}}}}}}
	copy := cloneTurnSkillLearningInput(in)
	*in.Skills[0].SkillDesign.Power = 9
	*in.Skills[0].CraftDesign.Item.MainBonus = .9
	in.Skills[0].CraftDesign.Inputs[0].Quantity = 999
	if *copy.Skills[0].SkillDesign.Power != 1.3 || *copy.Skills[0].CraftDesign.Item.MainBonus != .1 || copy.Skills[0].CraftDesign.Inputs[0].Quantity != 3 {
		t.Fatal("accepted proposal aliased mutable tool input")
	}
}
