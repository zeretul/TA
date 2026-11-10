package interactive

import (
	"crypto/sha256"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"ta/internal/cultivation"
)

// Generated content reuses the rules-owned Story definition journal. Records
// contain template identities, prose and optional checked design proposals.
// Native domain budgets remain authoritative when the journal is replayed.
// Separate categories prevent an item or recipe from becoming a combat skill.
// Archive guard, independent of prompt and active-catalog sizes. The byte guard
// additionally leaves headroom under the 16 MiB Story journal line limit.
const generatedDefinitionLimit = 16384
const generatedArchiveMaxBytes = 4 << 20

func generatedContentID(category, templateID, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 40 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", fmt.Errorf("generated content requires a name of 1-40 characters without control characters")
	}
	return fmt.Sprintf("gen.%s.%s.%x", category, templateID, sha256.Sum256([]byte(name))), nil
}

func generatedContentRecord(category, id, templateID, name, source, description string) map[string]any {
	return map[string]any{"类别": category, "定义": id, "版本": float64(1), "模板": templateID, "名称": strings.TrimSpace(name), "来源": strings.TrimSpace(source), "描述": strings.TrimSpace(description)}
}

func validateGeneratedContent(record map[string]any) error {
	if strings.TrimSpace(xiuxianEntryText(record, "来源")) == "" {
		return fmt.Errorf("generated content requires its actual story source")
	}
	for _, key := range []string{"来源", "描述"} {
		if len(xiuxianEntryText(record, key)) > 8192 {
			return fmt.Errorf("generated content prose exceeds 8 KiB per field")
		}
	}
	return nil
}

func generatedItemFromRecord(cultivationRules *cultivation.Rules, record map[string]any) (cultivation.ItemDef, bool) {
	if xiuxianEntryText(record, "类别") != "equipment" || validateGeneratedContent(record) != nil {
		return cultivation.ItemDef{}, false
	}
	base, ok := cultivation.NewItemCatalogWithFirstVersion(cultivationRules).Definition(xiuxianEntryText(record, "模板"))
	if !ok || base.Kind == cultivation.KindMaterial || base.Rarity == cultivation.RarityUnique {
		return cultivation.ItemDef{}, false
	}
	id, err := generatedContentID("item", base.ID, xiuxianEntryText(record, "名称"))
	if err != nil || id != xiuxianEntryText(record, "定义") {
		return cultivation.ItemDef{}, false
	}
	var design GeneratedItemDesign
	if err := decodeGeneratedDesign(record, "设计", &design); err != nil {
		return cultivation.ItemDef{}, false
	}
	var designErr error
	base, designErr = applyGeneratedItemDesign(base, design)
	if designErr != nil {
		return cultivation.ItemDef{}, false
	}
	base.ID = id
	base.NameKey = xiuxianEntryText(record, "名称")
	base.DefinitionDetails.Art = nil
	base.DefinitionDetails.Description = xiuxianEntryText(record, "描述")
	base.DefinitionDetails.Background = xiuxianEntryText(record, "来源")
	base.DefinitionDetails.Localizations = nil
	return base, true
}

func storyItemCatalogFromState(cultivationRules *cultivation.Rules, state map[string]any) *cultivation.ItemCatalog {
	records, _ := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	return cultivation.NewItemCatalogWithFirstVersion(cultivationRules).WithLookup(func(id string) (cultivation.ItemDef, bool) {
		record, _ := records[id].(map[string]any)
		def, ok := generatedItemFromRecord(cultivationRules,
			record)
		return def, ok && def.ID == id
	}, func() []string { return generatedRecordIDs(records, "equipment") })
}

func storyCraftRegistryFromState(system StoryDirectorActorStateSystem, state map[string]any) *cultivation.CraftRegistry {
	records, _ := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	return storyCraftRegistry(system).WithLookup(func(id string) (cultivation.CraftMethodDef, bool) {
		record, _ := records[id].(map[string]any)
		method, ok := generatedCraftFromRecord(system, record)
		return method, ok && method.ID == id
	}, func() []string { return generatedRecordIDs(records, "craft") })
}

func generatedRecordIDs(records map[string]any, category string) []string {
	ids := []string{}
	for id, value := range records {
		record, ok := value.(map[string]any)
		if ok && xiuxianEntryText(record, "类别") == category {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

func generatedCraftFromRecord(system StoryDirectorActorStateSystem, record map[string]any) (cultivation.CraftMethodDef, bool) {
	if xiuxianEntryText(record, "类别") != "craft" || validateGeneratedContent(record) != nil {
		return cultivation.CraftMethodDef{}, false
	}
	method, ok := storyCraftRegistry(system).Method(xiuxianEntryText(record, "模板"))
	if !ok {
		return cultivation.CraftMethodDef{}, false
	}
	id, err := generatedContentID("craft", method.ID, xiuxianEntryText(record, "名称"))
	if err != nil || id != xiuxianEntryText(record, "定义") {
		return cultivation.CraftMethodDef{}, false
	}
	method.ID, method.Version = id, 1
	method.NameKey = xiuxianEntryText(record, "名称")
	method.Source = xiuxianEntryText(record, "来源")
	method.RequiresLearning = true
	if record["制作设计"] != nil {
		var design GeneratedCraftDesign
		if decodeGeneratedDesign(record, "制作设计", &design) != nil {
			return cultivation.CraftMethodDef{}, false
		}
		var err error
		method, _, err = deriveGeneratedCraftDesign(system, method, record, design)
		if err != nil {
			return cultivation.CraftMethodDef{}, false
		}
	}
	return method, method.Validate() == nil
}

func generatedDefinitionOp(records map[string]any, turnID, sourceKind string) ActorStateOp {
	return ActorStateOp{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: normalizeActorStateFieldName(xiuxianFieldSkillRegistry), Value: maps.Clone(records), Reason: "由本局定义登记结算写入。", SourceTurnID: turnID, SourceKind: sourceKind, SourceID: "generated_definitions"}
}
