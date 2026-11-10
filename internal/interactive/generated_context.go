package interactive

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
)

const generatedContextDefinitionLimit = 32

// Bound the model projection before serializing it. The complete archive stays
// in the branch, and an exact ID request still resolves an omitted definition.
func adventureGeneratedContextRules(r AdventureRules) (AdventureRules, string) {
	if !r.generatedCatalog {
		return r, ""
	}
	out := adventureFrozenRules(r)
	included, total := 0, 0
	fits := func(candidate AdventureRules) bool {
		data, err := json.Marshal(adventureMechanicalRules(candidate))
		return err == nil && len(data) <= 96*1024
	}
	for _, item := range r.Items {
		if !strings.HasPrefix(item.ID, "gen.") {
			continue
		}
		total++
		if included >= generatedContextDefinitionLimit {
			continue
		}
		candidate := out
		candidate.Items = append(append([]AdventureItem(nil), out.Items...), item)
		if included < generatedContextDefinitionLimit && fits(candidate) {
			out = candidate
			included++
		}
	}
	for _, skill := range r.Skills {
		if !strings.HasPrefix(skill.ID, "gen.") {
			continue
		}
		total++
		if included >= generatedContextDefinitionLimit {
			continue
		}
		candidate := out
		candidate.Skills = append(append([]AdventureSkill(nil), out.Skills...), skill)
		if included < generatedContextDefinitionLimit && fits(candidate) {
			out = candidate
			included++
		}
	}
	for _, recipe := range r.Recipes {
		if !strings.HasPrefix(recipe.ID, "gen.") {
			continue
		}
		total++
		if included >= generatedContextDefinitionLimit {
			continue
		}
		candidate := out
		candidate.Recipes = append(append([]AdventureRecipe(nil), out.Recipes...), recipe)
		if included < generatedContextDefinitionLimit && fits(candidate) {
			out = candidate
			included++
		}
	}
	note := ""
	if total > included {
		note = fmt.Sprintf("\n本轮仅展示%d项生成规则，另%d项已取用规则未展开；完整历史定义保留在本局档案，可用search_generated_definitions按ID、名称或分页读取。", included, total-included)
	}
	return out, note
}

// The handbook describes the archive, not a second unbounded copy of it.
// This projection never changes authoritative state or learned/owned facts.
func generatedRuntimeFieldValue(fieldID string, value any) any {
	if fieldID == normalizeActorStateFieldName(xiuxianFieldSkillRegistry) {
		records, ok := value.(map[string]any)
		if !ok {
			return value
		}
		view := map[string]any{}
		keys := sortedKeys(records)
		for _, id := range keys[:min(len(keys), generatedContextDefinitionLimit)] {
			record, _ := records[id].(map[string]any)
			view[id] = map[string]any{"类别": record["类别"], "名称": record["名称"], "模板": record["模板"]}
		}
		return map[string]any{"档案总数": len(records), "定义索引样本": view, "说明": "仅展示部分稳定ID；可用search_generated_definitions按ID、名称或分页读取真实效果，未展示不代表未登记。"}
	}
	if fieldID == normalizeActorStateFieldName(xiuxianFieldLearnedSkills) || fieldID == normalizeActorStateFieldName(xiuxianFieldTechniques) || fieldID == normalizeActorStateFieldName(xiuxianFieldImportantItems) {
		records, ok := value.(map[string]any)
		if ok && len(records) > generatedContextDefinitionLimit {
			projected := map[string]any{}
			keys := sortedKeys(records)
			// Current circulation must remain visible even in a large library.
			for _, key := range keys {
				record, _ := records[key].(map[string]any)
				if xiuxianEntryText(record, "运转") == "运转" && len(projected) < generatedContextDefinitionLimit {
					projected[key] = generatedContextEntry(record)
				}
			}
			for _, key := range keys {
				if len(projected) >= generatedContextDefinitionLimit {
					break
				}
				value := records[key]
				if record, ok := value.(map[string]any); ok {
					value = generatedContextEntry(record)
				} else if text, ok := value.(string); ok {
					value = trimBytes(text, 512)
				}
				projected[key] = value
			}
			projected["_档案说明"] = fmt.Sprintf("记录总数%d；只展开部分条目，完整已学与持有事实仍保存，未展示不代表没有。", len(records))
			return projected
		}
	}
	if fieldID == normalizeActorStateFieldName(AdventureStateField) {
		record, ok := value.(map[string]any)
		if !ok {
			return value
		}
		projected := maps.Clone(record)
		if defs, ok := record["generated_definitions"].([]any); ok {
			delete(projected, "generated_definitions")
			projected["generated_definition_count"] = len(defs)
		}
		// Large learned/inventory maps are facts, not a request to serialize the
		// entire lifetime history into every narration. Exact consumers use state.
		for _, key := range []string{"skills", "training", "inventory", "acquired"} {
			if values, ok := record[key].(map[string]any); ok && len(values) > generatedContextDefinitionLimit {
				subset := map[string]any{}
				ids := sortedKeys(values)
				for _, id := range ids[:generatedContextDefinitionLimit] {
					subset[id] = values[id]
				}
				projected[key] = subset
				projected[key+"_archive_count"] = len(values)
			}
		}
		return projected
	}
	return value
}

func generatedContextEntry(record map[string]any) map[string]any {
	out := maps.Clone(record)
	for key, value := range out {
		if text, ok := value.(string); ok && len(text) > 512 {
			out[key] = trimBytes(text, 512)
		}
	}
	return out
}
