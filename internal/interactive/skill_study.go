package interactive

import (
	"math"
	"strings"

	"ta/internal/cultivation"
)

// Released initial actors already describe completed study or innate ability.
// New partial records are explicit and must never enter the casting set.
func skillRecordMastered(record map[string]any) bool {
	return len(record) > 0 && xiuxianEntryText(record, "定义") != "" && xiuxianEntryText(record, "掌握程度") != "修习中"
}

func skillStudyMinutes(def cultivation.SkillDef, state map[string]any, actorID string, profiles ...*cultivation.Rules) float64 {
	id := def.ID
	records, _ := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	if record, ok := records[id].(map[string]any); ok {
		id = xiuxianEntryText(record, "模板")
	}
	// Generated identities use the exact approved template prefix. The reader
	// still resolves the definition from the Story registry before calling here.
	if strings.HasPrefix(id, cultivation.GeneratedSkillPrefix) {
		for _, template := range cultivation.SkillTemplates(profiles...) {
			if strings.HasPrefix(id, cultivation.GeneratedSkillPrefix+template.Definition.ID+".") {
				id = template.Definition.ID
				break
			}
		}
	}
	template, ok := cultivation.SkillTemplateByID(id, profiles...)
	if !ok {
		return 0
	}
	days := 1
	switch template.Quality {
	case cultivation.QualityGood:
		days = 3
	case cultivation.QualityAdvanced:
		days = 7
	}
	factor := 1
	if baseBudget := generatedSkillBudget(template.Definition); baseBudget > 0 {
		factor = max(1, int(math.Ceil(generatedSkillBudget(def)/baseBudget-1e-9)))
	}
	return cultivation.LearningMinutesToComplete(xiuxianTalentFromState(state, actorID), days*24*60*factor, profiles...)
}

func skillLearningHasStudy(input *TurnSkillLearningInput) bool {
	if input != nil {
		for _, request := range input.Skills {
			if request.Minutes > 0 {
				return true
			}
		}
	}
	return false
}

func skillRecordStudyMinutes(record map[string]any) float64 {
	value, _ := actorStateNumber(record["已修习分钟"])
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func skillLearningRecord(records map[string]any, id string) (string, map[string]any) {
	for _, key := range sortedKeys(records) {
		if record, ok := records[key].(map[string]any); ok && xiuxianEntryText(record, "定义") == id {
			return key, record
		}
	}
	return id, nil
}
