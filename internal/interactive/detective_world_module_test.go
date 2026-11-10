package interactive

import "testing"

func TestDetectiveWorldModulesProvideInvestigationStateAndBindings(t *testing.T) {
	for _, module := range builtinRuleSystemModules() {
		if module.ID == RuleSystemDetectiveStatsID {
			if module.ActorStateID == "" {
				t.Fatal("detective rule module lost its actor state ID")
			}
			if err := validateRuleSystemModule(module); err != nil {
				t.Fatalf("detective rule module failed validation: %v", err)
			}
		}
	}
	root := t.TempDir()
	state, err := NewActorStateLibrary(root).Get(ActorStateDetectiveID)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateActorStateSystem(state.ActorState); err != nil {
		t.Fatalf("detective state module is invalid: %v", err)
	}
	protagonist := actorStateTemplateByID(state.ActorState, DefaultActorID)
	if _, ok := actorStateFieldByID(protagonist, "等级"); ok {
		t.Fatal("a no-growth investigation world inherited the generic default level")
	}
	if health, ok := actorStateFieldByID(protagonist, "生命"); !ok || health.Type != "string" {
		t.Fatalf("the investigation world lost its useful health status: %#v", health)
	}
	for _, ability := range []string{"观察", "逻辑", "交涉", "体能"} {
		field, ok := actorStateFieldByID(protagonist, ability)
		if !ok || field.Type != "number" || field.Min == nil || *field.Min != 1 || field.Max == nil || *field.Max != 10 || field.Default != float64(5) {
			t.Fatalf("detective ability %q is not a bounded, initialized numeric field: %#v", ability, field)
		}
	}
	for _, field := range actorStateTemplateByID(state.ActorState, ActorStateStoryContextTemplateID).Fields {
		if field.Name == "本次交战" {
			t.Fatal("a world without independent combat exposes the combat engagement field")
		}
	}

	rules, err := NewRuleSystemLibrary(root).Get(RuleSystemDetectiveStatsID)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRuleSystemModule(rules); err != nil {
		t.Fatalf("mystery rule module is invalid: %v", err)
	}
	if len(rules.TRPGSystem.RuleTemplates) != 1 {
		t.Fatalf("want one investigation check template, got %d", len(rules.TRPGSystem.RuleTemplates))
	}
	bindings := rules.TRPGSystem.RuleTemplates[0].StateBindings
	if len(bindings) != 4 {
		t.Fatalf("want four action-specific state bindings, got %#v", bindings)
	}
	for _, binding := range bindings {
		if len(binding.Modifiers) != 1 || binding.Modifiers[0].Source != "actor" || binding.Modifiers[0].Effect != "advantage" || binding.Modifiers[0].Scale != 1 || binding.Modifiers[0].Offset != -5 {
			t.Fatalf("binding %q does not convert the actual score into a bounded check modifier: %#v", binding.ID, binding)
		}
	}
}

func TestDetectiveSearchBindingChangesTheSameFixedRoll(t *testing.T) {
	root := t.TempDir()
	stateModule, err := NewActorStateLibrary(root).Get(ActorStateDetectiveID)
	if err != nil {
		t.Fatal(err)
	}
	ruleModule, err := NewRuleSystemLibrary(root).Get(RuleSystemDetectiveStatsID)
	if err != nil {
		t.Fatal(err)
	}
	director := normalizeStoryDirector(StoryDirector{
		ActorState: stateModule.ActorState,
		TRPGSystem: ruleModule.TRPGSystem,
	})
	req := sampleTurnCheckRequest()
	req.Rule.TemplateID = RuleSystemMysteryClueForwardID
	req.Rule.BindingID = "search"
	req.Rule.ActorID = DefaultActorID
	resolve := func(score float64) RuleResolution {
		state := initialStoryState()
		state[actorStateRoot] = map[string]any{DefaultActorID: map[string]any{
			"template_id": DefaultActorID,
			"state":       map[string]any{"观察": score},
		}}
		resolution, err := resolveTurnRulesWithSeedAndDirector("same-story", "main", state, director, req, 713)
		if err != nil {
			t.Fatal(err)
		}
		if resolution.StateBinding == nil || len(resolution.StateBinding.StateInputs) != 1 {
			t.Fatalf("the check did not read one frozen observation value: %#v", resolution.StateBinding)
		}
		return resolution
	}
	low := resolve(2)
	high := resolve(8)
	if low.StateBinding.StateInputs[0].RawValue != 2 || high.StateBinding.StateInputs[0].RawValue != 8 {
		t.Fatalf("the check did not read the changed scores: low=%#v high=%#v", low.StateBinding, high.StateBinding)
	}
	if low.StateBinding.BindingBonusTotal != -3 || high.StateBinding.BindingBonusTotal != 3 ||
		low.Result.KeptRoll != high.Result.KeptRoll || low.Result.BonusTotal == high.Result.BonusTotal {
		t.Fatalf("the same die roll did not produce different rule totals: low=%#v high=%#v", low.Result, high.Result)
	}
}
