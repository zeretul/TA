package interactive

import (
	"reflect"
	"strings"

	"ta/internal/cultivation"
)

// A generated cultivation method keeps its foundation's study, circulation
// and growth rules. Optional specialty proposals use the same native budgets
// and are revalidated from the Story record on every projection.
func generatedTechniqueFromRecord(cultivationRules *cultivation.Rules, record map[string]any) (cultivation.TechniqueDef, bool) {
	if xiuxianEntryText(record, "类别") != "technique" || validateGeneratedContent(record) != nil {
		return cultivation.TechniqueDef{}, false
	}
	base, ok := cultivation.TechniqueByID(xiuxianEntryText(record, "模板"), cultivationRules)
	if !ok || base.ID == "method.spirit_beast_breathing" {
		return cultivation.TechniqueDef{}, false
	}
	name := xiuxianEntryText(record, "名称")
	id, err := generatedContentID("technique", base.ID, name)
	if err != nil || id != xiuxianEntryText(record, "定义") {
		return cultivation.TechniqueDef{}, false
	}
	if record["功法设计"] != nil {
		var design GeneratedTechniqueDesign
		if decodeGeneratedDesign(record, "功法设计", &design) != nil {
			return cultivation.TechniqueDef{}, false
		}
		var err error
		base, err = deriveGeneratedTechnique(base, design)
		if err != nil {
			return cultivation.TechniqueDef{}, false
		}
	}
	base.ID, base.Name = id, name
	base.Version = 1
	base.DefinitionDetails.Description = xiuxianEntryText(record, "描述")
	base.DefinitionDetails.Background = xiuxianEntryText(record, "来源")
	base.DefinitionDetails.Localizations, base.DefinitionDetails.Art = nil, nil
	return base, true
}

func techniqueLearningDefinition(cultivationRules *cultivation.Rules, state map[string]any, records map[string]any, request TurnTechniqueLearning) (cultivation.TechniqueDef, map[string]any, bool) {
	if strings.TrimSpace(request.TemplateID) == "" {
		if request.Design != nil {
			return cultivation.TechniqueDef{}, nil, false
		}
		definition, ok := resolveTechnique(cultivationRules,
			strings.TrimSpace(request.DefID), request.Version, state)
		return definition, nil, ok
	}
	if strings.TrimSpace(request.DefID) != "" {
		return cultivation.TechniqueDef{}, nil, false
	}
	id, err := generatedContentID("technique", strings.TrimSpace(request.TemplateID), request.Name)
	if err != nil {
		return cultivation.TechniqueDef{}, nil, false
	}
	record := generatedContentRecord("technique", id, strings.TrimSpace(request.TemplateID), request.Name, request.Source, request.Description)
	if request.Design != nil {
		record["功法设计"] = generatedDesignRecord(request.Design)
		if record["功法设计"] == nil {
			return cultivation.TechniqueDef{}, nil, false
		}
	}
	if existing, ok := records[id].(map[string]any); ok {
		if request.Design != nil && !reflect.DeepEqual(existing["功法设计"], record["功法设计"]) {
			return cultivation.TechniqueDef{}, nil, false
		}
		record = existing
	} else if !generatedRegistryCanAdd(records, record) {
		return cultivation.TechniqueDef{}, nil, false
	}
	definition, ok := generatedTechniqueFromRecord(cultivationRules,
		record)
	if request.Version > 0 && request.Version != definition.Version {
		ok = false
	}
	return definition, record, ok
}
