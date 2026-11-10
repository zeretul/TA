package interactive

import (
	"strings"
	"ta/internal/cultivation"
)

// Craft methods share the protected learned-record journal with techniques.
// They are resolved by the craft registry and never added to combat skills.
func craftMethodKnown(state map[string]any, actorID string, method cultivation.CraftMethodDef) bool {
	record, _ := xiuxianEquipmentEntry(state, actorID, xiuxianFieldLearnedSkills)
	entry, _ := record[method.ID].(map[string]any)
	version, _ := actorStateNumber(entry["版本"])
	return xiuxianEntryText(entry, "定义") == method.ID && version == float64(method.Version) && xiuxianEntryText(entry, "掌握程度") == "已学会"
}

func craftLearningRefusal(state map[string]any, actorID string, method cultivation.CraftMethodDef, request TurnSkillLearning) string {
	if request.Version != 0 && request.Version != method.Version {
		return "这个制作方法没有所指定的版本。"
	}
	if request.TemplateID != "" || request.Name != "" || request.Quality != "" || request.Rarity != "" {
		return "制作方法使用已登记的定义，不能通过模板、改名或品级改变配方。"
	}
	if strings.TrimSpace(request.Source) == "" {
		return "学习制作方法需要本次实际传授或研习的来历。"
	}
	stage, ok := cultivation.ParseStageKey(xiuxianStoredStageKey(state, actorID))
	if !ok || !stageReaches(stage, method.MinimumStage) {
		return "当前境界还达不到这个制作方法的学习要求。"
	}
	return ""
}
