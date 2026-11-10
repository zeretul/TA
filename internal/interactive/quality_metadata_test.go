package interactive

import (
	"encoding/json"
	"reflect"
	"strings"
	"ta/internal/cultivation"
	"testing"
)

func TestQualityAdventureProseCannotEvictMechanicalContext(t *testing.T) {
	rules := adventureTestRules()
	original := AdventureRulesContext(rules)
	details := cultivation.DefinitionDetails{QualityGrade: cultivation.QualityRed, Description: strings.Repeat("文", 1000), Background: strings.Repeat("史", 1000), Localizations: &cultivation.DefinitionLocalizations{English: cultivation.DefinitionProse{Description: strings.Repeat("d", 3000), Background: strings.Repeat("b", 3000)}}}
	for i := range rules.Skills {
		rules.Skills[i].DefinitionDetails = details
	}
	for i := range rules.Items {
		rules.Items[i].DefinitionDetails = details
	}
	if err := rules.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := AdventureRulesContext(rules); got != original {
		t.Fatal("display metadata changed or displaced mechanical context")
	}
	if rules.Skills[0].Description == "" {
		t.Fatal("projection mutated frozen source")
	}
	actor := NewAdventureActor(rules)
	before := actor
	for _, in := range []AdventureAction{{Action: "learn", SkillID: rules.Skills[0].ID, Hours: 1}, {Action: "equip", ItemID: "missing"}, {Action: "battle_round", Choice: "skill", SkillID: rules.Skills[0].ID}} {
		_, _, _ = ResolveAdventureAction(rules, actor, in, "preview")
		if !reflect.DeepEqual(actor, before) {
			t.Fatal("read-only preview mutated state")
		}
	}
}
func TestQualityOrdinaryAttackCannotBeLearned(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, "qi_3", nil)
	ops, result, _ := xiuxianSkillLearningOps(state, system, DefaultActorID, TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: cultivation.OrdinaryAttackSkillID}}}, "ordinary")
	if len(ops) != 0 || len(result.Refused) != 1 {
		t.Fatalf("ordinary attack became learned art: %+v %+v", ops, result)
	}
}

func TestQualityAdventureRealGatesRefuseAtomically(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*AdventureRules, *AdventureActor)
		in     AdventureAction
	}{
		{"rank", func(r *AdventureRules, a *AdventureActor) { r.Skills[1].RequiredRank = 2 }, AdventureAction{Action: "learn", SkillID: "advanced", Hours: 2}},
		{"prerequisite", func(r *AdventureRules, a *AdventureActor) { delete(a.Skills, "strike") }, AdventureAction{Action: "learn", SkillID: "advanced", Hours: 2}},
		{"source", func(r *AdventureRules, a *AdventureActor) { delete(a.Inventory, "book") }, AdventureAction{Action: "learn", SkillID: "advanced", Hours: 2}},
		{"inventory", func(r *AdventureRules, a *AdventureActor) {}, AdventureAction{Action: "equip", ItemID: "blade"}},
		{"wrong-scale", func(r *AdventureRules, a *AdventureActor) {
			a.Battle = &AdventureBattle{ID: "battle", Status: "active", Scale: "person", OpponentID: "rival", Health: 60, MaxHealth: 60, Attack: 10, Defense: 2}
		}, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "ship-gun"}},
		{"resources", func(r *AdventureRules, a *AdventureActor) {
			a.Energy = 0
			a.Battle = &AdventureBattle{ID: "battle", Status: "active", Scale: "person", OpponentID: "rival", Health: 60, MaxHealth: 60, Attack: 10, Defense: 2}
		}, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "strike"}},
		{"support-only", func(r *AdventureRules, a *AdventureActor) {
			r.Items[3].SupportOnly = true
			a.Assets["ship"] = AdventureAsset{Health: 100, MaxHealth: 100, Fuel: 100}
			a.Equipment["ship:asset"] = "ship"
		}, AdventureAction{Action: "battle_start", OpponentID: "frigate", Scale: "ship"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := adventureTestRules()
			a := NewAdventureActor(r)
			test.change(&r, &a)
			for i := range r.Skills {
				r.Skills[i].DefinitionDetails = cultivation.DefinitionDetails{QualityGrade: cultivation.QualityRed}
			}
			for i := range r.Items {
				r.Items[i].DefinitionDetails = cultivation.DefinitionDetails{QualityGrade: cultivation.QualityRed}
			}
			before, _ := json.Marshal(a)
			next, out, err := ResolveAdventureAction(r, a, test.in, "quality-refusal")
			if err == nil {
				t.Fatal("red metadata bypassed authoritative gate")
			}
			after, _ := json.Marshal(next)
			unchanged, _ := json.Marshal(a)
			if string(before) != string(after) || string(before) != string(unchanged) || out.DurationSeconds != 0 {
				t.Fatalf("refusal spent state/time: %s => %s, %+v", before, after, out)
			}
		})
	}
	// Color does not introduce a universal high-rank gate either.
	r := adventureTestRules()
	r.Skills[1].QualityGrade = cultivation.QualityRed
	a := NewAdventureActor(r)
	next, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "learn", SkillID: "advanced", Hours: 2}, "red-legal")
	if err != nil || next.Skills["advanced"] != 1 || out.DurationSeconds != 7200 {
		t.Fatalf("legal source+study chain blocked by color: %+v %+v %v", next, out, err)
	}
}

func TestQualityGeneratedLearningInheritsApprovedTemplate(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, "qi_3", nil)
	var request TurnSkillLearningInput
	if err := json.Unmarshal([]byte(`{"skills":[{"template_id":"fire_bolt","name":"赤帝神炎","quality_grade":"red","description":"a personal impression","source":"a real lesson","minutes":1440}]}`), &request); err != nil {
		t.Fatal(err)
	}
	ops, result, handled := xiuxianSkillLearningOps(state, system, DefaultActorID, request, "template-grade")
	if !handled || len(result.Learned) != 1 || len(ops) == 0 {
		t.Fatalf("valid template learning failed: %+v", result)
	}
	approved, _ := cultivation.SkillByID("fire_bolt")
	learned := result.Learned[0]
	if learned.QualityGrade != approved.QualityGrade || learned.Power != approved.Power || learned.ManaFraction != approved.ManaFraction {
		t.Fatalf("model color or name upgraded approved template: %+v", learned)
	}
}
