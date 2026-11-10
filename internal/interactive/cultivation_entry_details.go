package interactive

import (
	"fmt"
	"sort"
	"strings"
	"ta/internal/cultivation"
)

// CultivationEntryDetails is a read-only view of the protagonist's actual
// records. It never lists a global catalog, other actors, or learnable rewards.
type CultivationEntryDetails struct {
	CraftMethods []CultivationCraftDetail     `json:"craft_methods,omitempty"`
	Head         string                       `json:"head"`
	Skills       []CultivationSkillDetail     `json:"skills"`
	Techniques   []CultivationTechniqueDetail `json:"techniques"`
	Items        []CultivationItemDetail      `json:"items"`
}
type CultivationCraftDetail struct {
	cultivation.DefinitionDetails
	ID            string                     `json:"id"`
	Name          string                     `json:"name"`
	NameKey       string                     `json:"name_key,omitempty"`
	Source        string                     `json:"source,omitempty"`
	Mastered      bool                       `json:"mastered"`
	MaterialNames map[string]string          `json:"material_names"`
	Definition    cultivation.CraftMethodDef `json:"definition"`
}

type CultivationSkillDetail struct {
	RecordKey string `json:"record_key,omitempty"`
	NameKey   string `json:"name_key,omitempty"`
	cultivation.DefinitionDetails
	ID                      string               `json:"id"`
	Version                 int                  `json:"version"`
	Name                    string               `json:"name"`
	InstanceDescription     string               `json:"instance_description,omitempty"`
	Source                  string               `json:"source,omitempty"`
	Definition              cultivation.SkillDef `json:"definition"`
	Mastered                bool                 `json:"mastered"`
	LearningMinutes         float64              `json:"learning_minutes,omitempty"`
	RequiredLearningMinutes float64              `json:"required_learning_minutes,omitempty"`
	Castable                bool                 `json:"castable"`
	ReasonKey               string               `json:"reason_key,omitempty"`
	ManaCost                int                  `json:"mana_cost"`
	BurdenCost              int                  `json:"burden_cost"`
}
type CultivationTechniqueDetail struct {
	cultivation.DefinitionDetails
	ID                  string                   `json:"id"`
	Name                string                   `json:"name"`
	InstanceDescription string                   `json:"instance_description,omitempty"`
	Source              string                   `json:"source,omitempty"`
	Definition          cultivation.TechniqueDef `json:"definition"`
	Mastered            bool                     `json:"mastered"`
	Active              bool                     `json:"active"`
	LearningMinutes     float64                  `json:"learning_minutes"`
}
type CultivationItemDetail struct {
	RestoreAmount     *int     `json:"restore_amount,omitempty"`
	AttackBonus       *float64 `json:"attack_bonus,omitempty"`
	ShieldAmount      *int     `json:"shield_amount,omitempty"`
	DurationExchanges *int     `json:"duration_exchanges,omitempty"`
	cultivation.DefinitionDetails
	ID                       string              `json:"id"`
	DefID                    string              `json:"def_id"`
	Name                     string              `json:"name"`
	InstanceDescription      string              `json:"instance_description,omitempty"`
	Source                   string              `json:"source,omitempty"`
	Definition               cultivation.ItemDef `json:"definition"`
	Quantity                 int                 `json:"quantity"`
	Slot                     string              `json:"slot,omitempty"`
	StageEligible            bool                `json:"stage_eligible"`
	OperatingManaCost        int                 `json:"operating_mana_cost"`
	EffectiveStrainReduction float64             `json:"effective_strain_reduction,omitempty"`
	RecordedGrade            string              `json:"recorded_grade,omitempty"`
	RecordedRarity           string              `json:"recorded_rarity,omitempty"`
	RecordedEffect           string              `json:"recorded_effect,omitempty"`
	RecordedKind             string              `json:"recorded_kind,omitempty"`
	Uncatalogued             bool                `json:"uncatalogued,omitempty"`
}

func (s *Store) CultivationEntryDetails(storyID, branchID, locale string) (CultivationEntryDetails, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, _, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return CultivationEntryDetails{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	p, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return CultivationEntryDetails{}, err
	}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState)
	if !ActorStateSystemIsXiuxian(system) {
		return CultivationEntryDetails{}, fmt.Errorf("story has no cultivation rules")
	}
	out := CultivationEntryDetails{Head: p.Head, Skills: []CultivationSkillDetail{}, Techniques: []CultivationTechniqueDetail{}, Items: []CultivationItemDetail{}}
	state, actorID := p.State, DefaultActorID
	generatedRecords, _ := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	character, err := xiuxianCharacterFromState(p.cultivationRules(),
		state, actorID)
	if err != nil {
		return out, err
	}
	details := func(id string, fallback cultivation.DefinitionDetails) cultivation.DefinitionDetails {
		if frozen, ok := system.DefinitionDetails[id]; ok {
			return frozen.ForLocale(locale)
		}
		if record, ok := generatedRecords[id].(map[string]any); ok {
			if frozen, ok := system.DefinitionDetails[xiuxianEntryText(record, "模板")]; ok {
				frozen.QualityGrade = fallback.QualityGrade
				frozen.Description = xiuxianEntryText(record, "描述")
				frozen.Background = xiuxianEntryText(record, "来源")
				frozen.Localizations = nil
				frozen.Art = nil // A new definition never inherits a template illustration.
				return frozen.ForLocale(locale)
			}
		}
		return fallback.ForLocale(locale)
	}
	registry := storySkillRegistryFromState(p.cultivationRules(),
		state)
	records, _ := xiuxianEquipmentEntry(state, actorID, xiuxianFieldLearnedSkills)
	seenSkills := make(map[string]bool, len(records))
	craftRegistry := storyCraftRegistryFromState(system, state)
	for _, key := range sortedKeys(records) {
		record, ok := records[key].(map[string]any)
		if !ok {
			if text, ok := records[key].(string); ok && strings.TrimSpace(text) != "" {
				out.Skills = append(out.Skills, CultivationSkillDetail{RecordKey: key, Name: key, InstanceDescription: text})
				seenSkills[key] = true
			}
			continue
		}
		id := xiuxianEntryText(record, "定义")
		if method, ok := craftRegistry.Method(id); ok && craftMethodKnown(state, actorID, method) {
			nameKey := method.NameKey
			if strings.HasPrefix(id, "gen.craft.") {
				nameKey = ""
			}
			metadata := cultivation.DefinitionDetails{Description: xiuxianEntryText(record, "描述"), Background: cultivationEntrySource(record)}
			if len(method.Outputs) > 0 {
				if product, ok := storyItemCatalog(p.cultivationRules(),
					state).Definition(method.Outputs[0].DefID); ok {
					metadata.QualityGrade = details(product.ID, product.DefinitionDetails).QualityGrade
				}
			}
			materialNames := map[string]string{}
			for _, input := range method.Inputs {
				if item, ok := storyItemCatalog(p.cultivationRules(),
					state).Definition(input.DefID); ok {
					materialNames[item.ID] = item.NameKey
				}
			}
			for _, output := range method.Outputs {
				if item, ok := storyItemCatalog(p.cultivationRules(),
					state).Definition(output.DefID); ok {
					materialNames[item.ID] = item.NameKey
				}
			}
			for _, tool := range method.RequiredTools {
				if item, ok := storyItemCatalog(p.cultivationRules(),
					state).Definition(tool); ok {
					materialNames[item.ID] = item.NameKey
				}
			}
			out.CraftMethods = append(out.CraftMethods, CultivationCraftDetail{DefinitionDetails: metadata, ID: id, Name: method.NameKey, NameKey: nameKey, Source: cultivationEntrySource(record), Mastered: true, MaterialNames: materialNames, Definition: method})
			seenSkills[key] = true
			continue
		}
		version, _ := actorStateNumber(record["版本"])
		def, reason := registerAndValidateSkill(registry, id, int(version))
		if id == cultivation.OrdinaryAttackSkillID && key == id {
			continue
		}
		seenSkills[key] = true
		if reason != "" {
			out.Skills = append(out.Skills, CultivationSkillDetail{RecordKey: key, Name: key, InstanceDescription: xiuxianEntryText(record, "描述")})
			continue
		}
		def.DefinitionDetails = details(id, def.DefinitionDetails)
		mastered := skillRecordMastered(record)
		check := cultivation.CastCheck{ReasonKey: "entryDetails.learningIncomplete"}
		if mastered {
			check = cultivation.CastPlanFor(cultivation.Combatant{Cultivation: character.Cultivation, Attributes: recoveryCurrentAttributes(p, actorID), Burden: character.BurdenState(), ManaReduction: character.ManaReduction(), StrainReduction: character.StrainReduction(), HasImplement: xiuxianHasMainImplement(state, actorID), Equipment: xiuxianEquippedCopies(p.cultivationRules(),
				state, actorID)}, def, 0).Check
		}
		nameKey := ""
		name := key
		recordKey := key
		if key == id {
			recordKey = ""
			nameKey = def.NameKey
			if def.Name != "" {
				name = def.Name
			}
		}
		out.Skills = append(out.Skills, CultivationSkillDetail{DefinitionDetails: def.DefinitionDetails, ID: id, RecordKey: recordKey, Version: def.Version, Name: name, NameKey: nameKey, InstanceDescription: xiuxianEntryText(record, "描述"), Source: cultivationEntrySource(record), Definition: def, Mastered: mastered, LearningMinutes: skillRecordStudyMinutes(record), RequiredLearningMinutes: skillStudyMinutes(def, state, actorID, p.cultivationRules()), Castable: check.Allowed, ReasonKey: check.ReasonKey, ManaCost: check.ManaCost, BurdenCost: check.BurdenCost})
	}
	abilities, _ := xiuxianEquipmentEntry(state, actorID, "技能与能力")
	for _, name := range sortedKeys(abilities) {
		if seenSkills[name] {
			continue
		}
		description, _ := abilities[name].(string)
		if record, ok := abilities[name].(map[string]any); ok {
			description = xiuxianEntryText(record, "效果")
		}
		out.Skills = append(out.Skills, CultivationSkillDetail{RecordKey: name, Name: name, InstanceDescription: description})
	}
	methods, _ := readTechniqueRecords(state, actorID)
	methodKeys := make([]string, 0, len(methods))
	for key := range methods {
		methodKeys = append(methodKeys, key)
	}
	sort.Strings(methodKeys)
	for _, key := range methodKeys {
		record := methods[key]
		def, ok := resolveTechnique(p.cultivationRules(),
			record.DefID, record.Version, state)
		if !ok {
			continue
		}
		def.DefinitionDetails = details(def.ID, def.DefinitionDetails)
		name := key
		if strings.HasPrefix(def.ID, "gen.technique.") {
			name = def.Name
		} else if strings.HasPrefix(locale, "en") && (name == def.Name || name == def.ID) {
			// Translate only the builtin title. A player's own name remains theirs.
			if frozen, ok := system.DefinitionDetails[def.ID]; ok && frozen.Localizations != nil && strings.TrimSpace(frozen.Localizations.English.Name) != "" {
				name = frozen.Localizations.English.Name
			}
		}
		out.Techniques = append(out.Techniques, CultivationTechniqueDetail{DefinitionDetails: def.DefinitionDetails, ID: def.ID, Name: name, InstanceDescription: record.Description, Source: record.Source, Definition: def, Mastered: record.Mastered, Active: record.Active, LearningMinutes: record.Minutes})
	}
	legacyMethod, _ := actorStateFieldValue(state, actorID, xiuxianFieldMethod).(string)
	if len(methods) == 0 && (legacyMethod == "家族家传功法，强攻，良好。" || legacyMethod == "叠锋诀，强攻，良好。") {
		metadata := cultivation.DefinitionDetails{
			QualityGrade: cultivation.QualityBlue,
			Description:  "吐纳两转，气沿小臂聚到腕间。掌或剑递出时，后一道劲紧跟着前一道。",
			Background:   "家中剑坪有几根磨得发白的木桩。旧日教习在那里传下这路行气法，桩上头一道浅印，底下总还压着一道。",
		}
		name := "叠锋诀"
		if strings.HasPrefix(locale, "en") {
			name = "Layered Edge Method"
			metadata.Description = "Two turns of breath gather qi at the wrist. When the palm or sword moves, a second pulse follows close behind the first."
			metadata.Background = "The clan taught this method at a sword yard with posts worn pale. Below each shallow mark on the old wood lies a second."
		}
		out.Techniques = append(out.Techniques, CultivationTechniqueDetail{DefinitionDetails: metadata, ID: "legacy-layered-edge", Name: name, Definition: cultivation.TechniqueDef{Quality: cultivation.QualityGood}, Mastered: true, Active: true})
	}
	catalog := storyItemCatalog(p.cultivationRules(),
		state)
	appendItem := func(key string, record map[string]any, slot string) {
		quantity := xiuxianCarriedQuantity(record)
		name := xiuxianEntryText(record, "名称")
		if name == "" {
			name = strings.TrimSpace(key)
		}
		if quantity <= 0 || name == "" {
			return
		}
		item := CultivationItemDetail{ID: key, Name: name, InstanceDescription: xiuxianEntryText(record, "描述"), Source: cultivationEntrySource(record), Quantity: quantity, Slot: slot}
		def, ok := catalog.Definition(xiuxianEntryText(record, "定义"))
		if !ok {
			item.Uncatalogued = true
			item.RecordedGrade = xiuxianEntryText(record, "品级")
			item.RecordedRarity = xiuxianEntryText(record, "稀有度")
			item.RecordedEffect = xiuxianEntryText(record, "作用")
			item.RecordedKind = xiuxianEntryText(record, "类别")
			if item.RecordedKind == "" {
				item.RecordedKind = xiuxianEntryText(record, "类型")
			}
			out.Items = append(out.Items, item)
			return
		}
		def.DefinitionDetails = details(def.ID, def.DefinitionDetails)
		item.DefinitionDetails, item.DefID, item.Definition = def.DefinitionDetails, def.ID, def
		item.StageEligible = def.UsableBy(character.Cultivation) == nil
		item.OperatingManaCost = def.OperatingManaCost(character.Cultivation)
		item.EffectiveStrainReduction = def.EquipmentStrainReduction(character.Cultivation)
		if pill := def.Pill; pill != nil {
			switch pill.Purpose {
			case cultivation.PillRestoreHealth, cultivation.PillRestoreMana:
				amount := pill.RestoreAmount(recoveryCurrentAttributes(p, actorID))
				item.RestoreAmount = &amount
			case cultivation.PillBoostAttack:
				if bonus, ok := pill.AttackBonus(); ok {
					item.AttackBonus = &bonus
				}
				duration := pill.DurationExchanges
				item.DurationExchanges = &duration
			}
		}
		if talisman := def.Talisman; talisman != nil {
			shield := talisman.ShieldFor(character.Cultivation, "talisman:"+def.ID)
			amount := shield.Amount
			duration := shield.RemainingExchanges
			item.ShieldAmount = &amount
			item.DurationExchanges = &duration
		}
		out.Items = append(out.Items, item)
	}
	items, _ := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImportantItems)
	for _, key := range sortedKeys(items) {
		switch record := items[key].(type) {
		case map[string]any:
			appendItem(key, record, "")
		case string:
			appendItem(key, map[string]any{"描述": record}, "")
		default:
			if count, ok := actorStateNumber(record); ok {
				appendItem(key, map[string]any{"数量": count}, "")
			}
		}
	}
	for _, field := range xiuxianSlotOrder {
		slot, _ := xiuxianEngineEquipmentSlot(field)
		if record, ok := xiuxianEquipmentEntry(state, actorID, field); ok {
			appendItem(xiuxianItemName(record), record, string(slot))
		} else if name, ok := actorStateFieldValue(state, actorID, field).(string); ok && strings.TrimSpace(name) != "" {
			appendItem(field, map[string]any{"名称": name}, string(slot))
		}
	}
	return out, nil
}

// Current learning/acquisition records use 来历; generated registry facts use
// 来源. Both are actual instance provenance, never definition background.
func cultivationEntrySource(record map[string]any) string {
	if source := xiuxianEntryText(record, "来历"); source != "" {
		return source
	}
	return xiuxianEntryText(record, "来源")
}
