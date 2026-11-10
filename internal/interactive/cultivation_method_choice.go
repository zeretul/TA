package interactive

import (
	"fmt"
	"strings"
	"ta/internal/cultivation"
)

// This is a projection of existing mastery and circulation, not a second method
// store. Keeping the actual choice in the interval makes resume/replay stable.
func selectedCultivationMethod(cultivationRules *cultivation.Rules, state map[string]any, actorID, requested string) (*cultivation.TechniqueDef, string) {
	requested = strings.TrimSpace(requested)
	current, currentOK := cultivation.ParseStageKey(xiuxianStoredStageKey(state, actorID), cultivationRules)
	records, structured := readTechniqueRecords(state, actorID)
	if !structured || len(records) == 0 {
		if requested != "" {
			return nil, "所选功法尚未正式掌握并运转，不能据此修炼。"
		}
		if currentOK && cultivation.RealmGap(current.Realm, cultivation.RealmVoid) >= 0 {
			return nil, "灵界高阶修炼需要已掌握并运转的完整功法，不能只凭境界或功法名称取得修为。"
		}
		return nil, "" // Preserve the existing human-realm baseline without a save migration.
	}
	methods := circulatingTechniques(cultivationRules,
		records, state)
	var selected *cultivation.TechniqueDef
	for _, d := range methods {
		if requested != "" && d.ID != requested {
			continue
		}
		ceiling, ceilingOK := cultivation.ParseStageKey(d.SupportsThrough, cultivationRules)
		if !currentOK || !stageReaches(current, d.MinimumStage) || !ceilingOK || !stageReaches(ceiling, cultivation.StageKey(current)) {
			if requested != "" {
				return nil, fmt.Sprintf("%s 不支持当前修炼阶段，不能仅凭名称继续取得修为。", d.Name)
			}
			continue
		}
		// With equal growth choose the continuation that reaches farther; an
		// old active method must not hide an actually mastered higher one.
		better := selected == nil || d.GrowthCoefficient > selected.GrowthCoefficient
		if selected != nil && d.GrowthCoefficient == selected.GrowthCoefficient {
			better = d.SupportsThrough != selected.SupportsThrough && stageReaches(ceiling, selected.SupportsThrough) || d.SupportsThrough == selected.SupportsThrough && d.ID < selected.ID
		}
		if better {
			copy := d
			selected = &copy
		}
	}
	if selected == nil {
		return nil, "没有可用于本段修炼的已掌握运转功法；请先完成学习或安全调整运转。"
	}
	return selected, ""
}

func cultivationMethodSelectionDiagnostic(cultivationRules *cultivation.Rules, state map[string]any, input *TurnCultivationInput) *TurnSubmissionDiagnostic {
	if input == nil || input.Empty() {
		return nil
	}
	actorID := normalizeStatePanelActorID(input.ActorID)
	if actorID == "" {
		actorID = DefaultActorID
	}
	method, reason := selectedCultivationMethod(cultivationRules,
		state, actorID, input.MethodID)
	if reason == "" && method == nil && spiritRealmBreakthrough(cultivation.BreakthroughTarget(strings.TrimSpace(input.AttemptBreakthrough))) {
		reason = "突破灵界高阶境界须有已经掌握、运转且支持目标境界的完整功法。"
	}
	if reason == "" && method != nil && strings.TrimSpace(input.AttemptBreakthrough) != "" && !cultivationMethodSupportsTarget(*method, cultivation.BreakthroughTarget(strings.TrimSpace(input.AttemptBreakthrough))) {
		reason = "所选功法不支持目标境界，不能据此尝试突破。"
	}
	if reason == "" {
		return nil
	}
	return newTurnSubmissionDiagnostic(TurnSubmissionModuleCultivation, nil, TurnSubmissionDiagnosticInvalidModule, "/cultivation/method_id", "已掌握且适用于当前阶段的功法", input.MethodID, reason)
}

// The same frozen ceiling constrains preview and settlement. It grants no new
// realm content and does not infer a method for legacy unstructured records.
func cultivationMethodSupportsTarget(method cultivation.TechniqueDef, target cultivation.BreakthroughTarget) bool {
	ceiling, ok := cultivation.ParseStageKey(method.SupportsThrough)
	entered := npcGrowthEnteredRealm(target)
	return ok && entered.Realm != "" && stageReaches(ceiling, cultivation.StageKey(entered))
}

func spiritRealmBreakthrough(target cultivation.BreakthroughTarget) bool {
	switch target {
	case cultivation.BreakthroughVoid, cultivation.BreakthroughIntegration, cultivation.BreakthroughMahayana:
		return true
	default:
		return false
	}
}
