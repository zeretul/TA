package storypack

import (
	"errors"
	"fmt"
	"os"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// materializeGameplayModules builds the two ordinary director modules from the
// confirmed, typed mechanics stored in the pack. Their IDs include pack identity
// and mechanics content, making these files rebuildable artifacts rather than
// the only copy of a world's rules.
func (l *Library) materializeGameplayModules(pack *Pack) (createdGameplayModules, error) {
	if pack != nil && pack.Story.Gameplay.StatusTemplate == WorldStatusCultivation && pack.Story.Gameplay.CultivationRules == nil {
		return createdGameplayModules{}, &WorldGameplayValidationError{Key: "api.storyPack.unsupportedGameplay"}
	}
	if pack == nil || !pack.Story.Gameplay.Confirmed {
		return createdGameplayModules{}, nil
	}
	stateID, ruleID := gameplayModuleIDs(pack.ID, pack.Story.Gameplay)
	stateLibrary := interactive.NewActorStateLibrary(l.dataRoot)
	ruleLibrary := interactive.NewRuleSystemLibrary(l.dataRoot)
	stateBaseID, ruleBaseID := interactive.ActorStateDetectiveID, interactive.RuleSystemDetectiveStatsID
	if pack.Story.Gameplay.StatusTemplate == WorldStatusCultivation {
		stateBaseID = interactive.ActorStateXiuxianID
		ruleBaseID = DefaultRuleSystemID
	}
	stateBase, err := stateLibrary.Get(stateBaseID)
	if err != nil {
		return createdGameplayModules{}, fmt.Errorf("load supported investigation state: %w", err)
	}
	ruleBase, err := ruleLibrary.Get(ruleBaseID)
	if err != nil {
		return createdGameplayModules{}, fmt.Errorf("load supported investigation checks: %w", err)
	}

	stateModule := buildWorldActorStateModule(stateBase, stateID, pack.Story.Gameplay)
	stateModule.ActorState = interactive.NormalizeActorStateSystem(stateModule.ActorState)
	ruleModule := buildWorldRuleSystemModule(ruleBase, ruleID, stateID, pack.Story.Gameplay)
	created := createdGameplayModules{}
	if existing, getErr := stateLibrary.Get(stateID); getErr == nil {
		if gameplayModuleContentHash(existing.ActorState) != gameplayModuleContentHash(stateModule.ActorState) {
			return created, fmt.Errorf("world state module %s differs from its pack definition", stateID)
		}
	} else {
		if !errors.Is(getErr, os.ErrNotExist) {
			return created, fmt.Errorf("read world state module %s: %w", stateID, getErr)
		}
		if _, err := stateLibrary.Create(stateModule); err != nil {
			return created, fmt.Errorf("create world state module: %w", err)
		}
		created.actorStateID = stateID
	}
	if existing, getErr := ruleLibrary.Get(ruleID); getErr == nil {
		if gameplayModuleContentHash(existing.TRPGSystem) != gameplayModuleContentHash(ruleModule.TRPGSystem) || existing.ActorStateID != ruleModule.ActorStateID {
			created.rollback(stateLibrary, ruleLibrary)
			return createdGameplayModules{}, fmt.Errorf("world rule module %s differs from its pack definition", ruleID)
		}
	} else {
		if !errors.Is(getErr, os.ErrNotExist) {
			created.rollback(stateLibrary, ruleLibrary)
			return createdGameplayModules{}, fmt.Errorf("read world rule module %s: %w", ruleID, getErr)
		}
		if _, err := ruleLibrary.Create(ruleModule); err != nil {
			created.rollback(stateLibrary, ruleLibrary)
			return createdGameplayModules{}, fmt.Errorf("create world rule module: %w", err)
		}
		created.ruleSystemID = ruleID
	}

	pack.Story.ModuleRefs.ActorStateID = stateID
	pack.Story.ModuleRefs.RuleSystemID = ruleID
	return created, nil
}

type createdGameplayModules struct {
	actorStateID string
	ruleSystemID string
}

func (created createdGameplayModules) rollback(states *interactive.ActorStateLibrary, rules *interactive.RuleSystemLibrary) {
	if created.ruleSystemID != "" {
		_ = rules.Delete(created.ruleSystemID)
	}
	if created.actorStateID != "" {
		_ = states.Delete(created.actorStateID)
	}
}

func gameplayModuleIDs(packID string, gameplay WorldGameplay) (string, string) {
	authoredActorSchemaVersion := 0
	if len(gameplay.AuthoredStateFields) > 0 {
		authoredActorSchemaVersion = 1
	}
	key := interactive.ContentHashOf(struct {
		CultivationRules           *cultivation.Rules                       `json:"cultivation_rules,omitempty"`
		WorldContent               *interactive.WorldContent                `json:"world_content,omitempty"`
		DefinitionDetails          map[string]cultivation.DefinitionDetails `json:"definition_details,omitempty"`
		CraftMethods               []cultivation.CraftMethodDef             `json:"craft_methods,omitempty"`
		Rules                      *interactive.AdventureRules              `json:"adventure_rules,omitempty"`
		PackID                     string                                   `json:"pack_id"`
		StatusTemplate             string                                   `json:"status_template"`
		AuthoredActorSchemaVersion int                                      `json:"authored_actor_schema_version,omitempty"`
		AuthoredStateFields        []interactive.ActorStateField            `json:"authored_state_fields,omitempty"`
		StateFields                []WorldStateField                        `json:"state_fields"`
		CheckBindings              []WorldCheckBinding                      `json:"check_bindings"`
	}{CultivationRules: gameplay.CultivationRules, WorldContent: gameplay.WorldContent, DefinitionDetails: gameplay.DefinitionDetails, CraftMethods: gameplay.CraftMethods, Rules: gameplay.AdventureRules, PackID: packID, StatusTemplate: gameplay.StatusTemplate, AuthoredActorSchemaVersion: authoredActorSchemaVersion, AuthoredStateFields: gameplay.AuthoredStateFields, StateFields: gameplay.StateFields, CheckBindings: gameplay.CheckBindings})
	if len(key) > 20 {
		key = key[:20]
	}
	return "world-actor-state-" + key, "world-rule-system-" + key
}

func gameplayModuleContentHash(value any) string {
	return interactive.ContentHashOf(value)
}

func buildWorldActorStateModule(base interactive.ActorStateModule, id string, gameplay WorldGameplay) interactive.ActorStateModule {
	base.ID = id
	base.ActorState.WorldContent = interactive.CloneWorldContent(gameplay.WorldContent)
	base.ActorState.CultivationRules = cultivation.CloneRules(gameplay.CultivationRules)
	base.ActorState.DefinitionDetails = gameplay.DefinitionDetails
	base.ActorState.AdventureRules = gameplay.AdventureRules
	if gameplay.AdventureRules != nil {
		base.ActorState.Templates = append(base.ActorState.Templates, interactive.ActorStateTemplate{ID: interactive.AdventureNPCTemplateID, Name: "人物历练", Fields: interactive.AdventureNPCStateFields(*gameplay.AdventureRules)})
	}
	if gameplay.StatusTemplate == WorldStatusCultivation {
		base.ActorState.CraftMethods = append(cultivation.NewCraftRegistryWithBuiltins(gameplay.CultivationRules).Methods(), gameplay.CraftMethods...)
		return base
	}
	base.Name = "世界专属行动状态"
	base.Description = "由世界包的有界状态字段定义构建；故事创建后按状态快照冻结。"
	for templateIndex := range base.ActorState.Templates {
		template := &base.ActorState.Templates[templateIndex]
		if template.ID == interactive.ActorStateImportantCharacterTemplateID || template.ID == interactive.ActorStateOpponentTemplateID {
			// The declaration is shared, never the protagonist's values. NPC facts
			// stay in the same journal but are withheld by the existing player projection.
			names := map[string]bool{}
			for _, field := range gameplay.AuthoredStateFields {
				names[field.Name] = true
			}
			fields := make([]interactive.ActorStateField, 0, len(template.Fields)+len(names))
			for _, field := range template.Fields {
				if !names[field.Name] {
					fields = append(fields, field)
				}
			}
			for _, field := range gameplay.AuthoredStateFields {
				field.Visibility = interactive.ActorStateVisibilityPrivate
				field.Default = nil
				fields = append(fields, field)
			}
			template.Fields = fields
			if len(gameplay.AuthoredStateFields) > 0 {
				template.Description += "\n人物正式状态是裁判事实，不等于本人或主角知情。仅当本人亲历或有意识行动确实获知钱物、技能、成长等变化时，在同一回合通过character_facts.knowledge为该稳定人物ID提交具体自知事实与来源；变化后的旧知识按supersedes/retracts更新。暗伤、未识别物品真相和后台状态不得自动写成自知。玩家获知另按实际交流记录，不直接公开私有状态。"
			}
			continue
		}
		if template.ID != interactive.DefaultActorID {
			continue
		}
		fields := make([]interactive.ActorStateField, 0, len(template.Fields)+len(gameplay.StateFields))
		configuredNames := map[string]bool{"观察": true, "逻辑": true, "交涉": true, "体能": true, "判断": true, "交际": true, "身手": true}
		for _, field := range gameplay.StateFields {
			configuredNames[field.Name] = true
		}
		for _, field := range gameplay.AuthoredStateFields {
			configuredNames[field.Name] = true
		}
		for _, field := range template.Fields {
			if !configuredNames[field.Name] {
				fields = append(fields, field)
			}
		}
		for _, field := range gameplay.StateFields {
			minimum, maximum := float64(field.Minimum), float64(field.Maximum)
			fields = append(fields, interactive.ActorStateField{
				Name: field.Name, Type: "number", Default: field.Initial,
				Min: &minimum, Max: &maximum, Group: "行动能力", Display: "stat",
				Description:       field.Description,
				UpdateInstruction: "这是世界定义的行动能力，保持在声明范围内，仅由已提交的正式规则改变。",
			})
		}
		if gameplay.AdventureRules != nil {
			owned := interactive.AdventureStateFields(*gameplay.AdventureRules)
			filtered := fields[:0]
			for _, field := range fields {
				if !interactive.AdventureOwnsField(field.Name) && field.Name != "技能与能力" && field.Name != "重要物品" {
					filtered = append(filtered, field)
				}
			}
			fields = append(filtered, owned...)
		}
		fields = append(fields, gameplay.AuthoredStateFields...)
		template.Fields = fields
	}
	return base
}

func buildWorldRuleSystemModule(base interactive.RuleSystemModule, id, stateID string, gameplay WorldGameplay) interactive.RuleSystemModule {
	base.ID = id
	base.Name = "世界专属行动检定"
	base.Description = "使用世界包冻结的检定字段绑定与受限参数。"
	base.ActorStateID = stateID
	if gameplay.StatusTemplate == WorldStatusCultivation {
		return base
	}
	for templateIndex := range base.TRPGSystem.RuleTemplates {
		template := &base.TRPGSystem.RuleTemplates[templateIndex]
		bindings := make([]interactive.RuleStateBinding, 0, len(template.StateBindings)+len(gameplay.CheckBindings))
		for _, binding := range template.StateBindings {
			if !supportedInvestigationAction(binding.ID) {
				bindings = append(bindings, binding)
			}
		}
		for _, configured := range gameplay.CheckBindings {
			minimum, maximum := configured.Minimum, configured.Maximum
			bindings = append(bindings, interactive.RuleStateBinding{
				ID: configured.Action, Label: configured.Field + "检定", Trigger: configured.Action,
				ActorTemplateID: interactive.DefaultActorID,
				Modifiers: []interactive.RuleStateBindingModifier{{
					Source: "actor", FieldID: configured.Field, Effect: "advantage",
					Scale: configured.Scale, Offset: configured.Offset,
					Min: &minimum, Max: &maximum, Rounding: "floor",
				}},
			})
		}
		template.StateBindings = bindings
	}
	return base
}

func supportedInvestigationAction(action string) bool {
	switch action {
	case "search", "evidence_review", "persuasion", "chase_or_escape":
		return true
	default:
		return false
	}
}
