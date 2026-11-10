package interactive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"ta/internal/cultivation"
)

// GeneratedItemDesign spends the existing cultivation grade budget. It exposes
// only properties with consumers, never free-form special-effect names.
type GeneratedItemDesign struct {
	MainBonus    *float64                 `json:"main_bonus,omitempty" jsonschema_description:"法器/护具/法宝/阵法实际主属性加成的小数提案，仍受所选品级与稀有度预算约束。"`
	Attribute    string                   `json:"attribute,omitempty" jsonschema_description:"阵法可选attack或defense，二选一；其他种类不填。"`
	ManaFraction *float64                 `json:"mana_fraction,omitempty" jsonschema_description:"法器/护具/法宝每次运转耗灵比例，只能达到模板或增强后所需的下限，不能免费驱动。"`
	PillPurpose  string                   `json:"pill_purpose,omitempty" jsonschema_description:"恢复类丹药可选restore_health或restore_mana，实际绝对恢复量仍按所选模板的品级与登记境界计算。"`
	QualityGrade cultivation.QualityGrade `json:"quality_grade,omitempty" jsonschema_description:"六色展示品级white/green/blue/purple/orange/red；不增加实际预算。"`
}

type GeneratedCraftDesign struct {
	OutputTemplateID string                   `json:"output_template_id" jsonschema_description:"同一制作类别的非独特产物模板ID；允许更高品级，但学习、制作和使用必须满足对应境界，材料和工时随产物价值增加。"`
	ProductName      string                   `json:"product_name" jsonschema_description:"实际新产物名称，最多40字。登记方法不直接创造物品。"`
	Item             GeneratedItemDesign      `json:"item,omitempty" jsonschema_description:"可选实际属性推导，仅支持现有已消费效果。"`
	Inputs           []cultivation.CraftInput `json:"inputs,omitempty" jsonschema_description:"可选每批材料提案；须保留基础核心材料，满足规则价值下限，不能消耗必需工具；省略自动按成本倍数增加。"`
}

func generatedDesignRecord(value any) map[string]any {
	data, _ := json.Marshal(value)
	var record map[string]any
	_ = json.Unmarshal(data, &record)
	return record
}

func decodeGeneratedDesign(record map[string]any, key string, destination any) error {
	raw, ok := record[key]
	if !ok {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func applyGeneratedItemDesign(base cultivation.ItemDef, design GeneratedItemDesign) (cultivation.ItemDef, error) {
	if design.QualityGrade != "" {
		if !design.QualityGrade.Valid() {
			return base, fmt.Errorf("invalid generated quality grade")
		}
		base.QualityGrade = design.QualityGrade
	}
	for _, number := range []*float64{design.MainBonus, design.ManaFraction} {
		if number != nil && (math.IsNaN(*number) || math.IsInf(*number, 0) || *number < 0 || *number > 1) {
			return base, fmt.Errorf("generated item fractions must be finite values in [0,1]")
		}
	}
	if design.PillPurpose != "" {
		if base.Pill == nil || (base.Pill.Purpose != cultivation.PillRestoreHealth && base.Pill.Purpose != cultivation.PillRestoreMana) || (design.PillPurpose != string(cultivation.PillRestoreHealth) && design.PillPurpose != string(cultivation.PillRestoreMana)) {
			return base, fmt.Errorf("only restoring pills can change restoration purpose")
		}
		pill := *base.Pill
		pill.Purpose = cultivation.PillPurpose(design.PillPurpose)
		base.Pill = &pill
	}
	if design.Attribute != "" && (base.Formation == nil || design.Attribute != "attack" && design.Attribute != "defense") {
		return base, fmt.Errorf("only formations may select an attack or defense attribute")
	}
	oldBonus, newBonus, oldMana := 0.0, 0.0, 0.0
	switch {
	case base.Implement != nil:
		p := *base.Implement
		oldBonus = p.AttackBonus
		oldMana = p.OperatingManaFraction
		if design.MainBonus != nil {
			p.AttackBonus = *design.MainBonus
		}
		newBonus = p.AttackBonus
		if design.ManaFraction != nil {
			p.OperatingManaFraction = *design.ManaFraction
		}
		base.Implement = &p
	case base.Armor != nil:
		p := *base.Armor
		oldBonus = p.DefenseBonus
		oldMana = p.OperatingManaFraction
		if design.MainBonus != nil {
			p.DefenseBonus = *design.MainBonus
		}
		newBonus = p.DefenseBonus
		if design.ManaFraction != nil {
			p.OperatingManaFraction = *design.ManaFraction
		}
		base.Armor = &p
	case base.Treasure != nil:
		p := *base.Treasure
		oldBonus = p.Bonus
		oldMana = p.OperatingManaFraction
		if design.MainBonus != nil {
			p.Bonus = *design.MainBonus
		}
		newBonus = p.Bonus
		if design.ManaFraction != nil {
			p.OperatingManaFraction = *design.ManaFraction
		}
		base.Treasure = &p
	case base.Formation != nil:
		if design.ManaFraction != nil {
			return base, fmt.Errorf("formation upkeep is fixed by its existing rules")
		}
		p := *base.Formation
		attribute := design.Attribute
		if attribute == "" {
			attribute = "attack"
			if p.DefenseBonus > 0 {
				attribute = "defense"
			}
		}
		bonus := p.AttackBonus + p.DefenseBonus
		if design.MainBonus != nil {
			bonus = *design.MainBonus
		}
		p.AttackBonus, p.DefenseBonus = 0, 0
		if attribute == "attack" {
			p.AttackBonus = bonus
		} else {
			p.DefenseBonus = bonus
		}
		base.Formation = &p
	default:
		if design.MainBonus != nil || design.ManaFraction != nil {
			return base, fmt.Errorf("this item has no adjustable main bonus or mana consumer")
		}
	}
	if oldMana > 0 || design.ManaFraction != nil {
		minimum := oldMana
		if oldBonus > 0 && newBonus > oldBonus {
			minimum *= newBonus / oldBonus
		}
		mana := oldMana
		if design.ManaFraction != nil {
			mana = *design.ManaFraction
		}
		if mana+1e-9 < minimum {
			return base, fmt.Errorf("generated item operating cost is below its effect budget")
		}
	}
	if err := base.Validate(); err != nil {
		return base, err
	}
	return base, nil
}

func deriveGeneratedCraftDesign(system StoryDirectorActorStateSystem, method cultivation.CraftMethodDef, record map[string]any, design GeneratedCraftDesign) (cultivation.CraftMethodDef, map[string]any, error) {
	if len(method.Outputs) != 1 || method.Outputs[0].Quantity != 1 {
		return method, nil, fmt.Errorf("generated craft needs a single-product foundation")
	}
	catalog := cultivation.NewItemCatalogWithFirstVersion(system.CultivationRules)
	original, ok := catalog.Definition(method.Outputs[0].DefID)
	if !ok {
		return method, nil, fmt.Errorf("craft foundation output missing")
	}
	target, ok := catalog.Definition(strings.TrimSpace(design.OutputTemplateID))
	if !ok || !generatedCraftArtSupports(method.Art, target.Kind) || target.Rarity == cultivation.RarityUnique || target.Kind == cultivation.KindMaterial || target.BasePrice < 1 || original.BasePrice < 1 {
		return method, nil, fmt.Errorf("generated craft output must be a priced compatible nonunique template")
	}
	if _, err := applyGeneratedItemDesign(target, design.Item); err != nil {
		return method, nil, err
	}
	id, err := generatedContentID("item", target.ID, design.ProductName)
	if err != nil {
		return method, nil, err
	}
	product := generatedContentRecord("equipment", id, target.ID, design.ProductName, xiuxianEntryText(record, "来源"), xiuxianEntryText(record, "描述"))
	product["设计"] = generatedDesignRecord(design.Item)
	if _, ok := generatedItemFromRecord(system.CultivationRules,
		product); !ok {
		return method, nil, fmt.Errorf("generated item exceeds its grade budget or supported effects")
	}
	factor := max(int64(1), (target.BasePrice+original.BasePrice-1)/original.BasePrice)
	if factor > 1000 || method.MinutesPerBatch > 525600/factor {
		return method, nil, fmt.Errorf("generated craft exceeds one-year batch limit")
	}
	baseline := int64(0)
	core := map[string]int{}
	for _, input := range method.Inputs {
		material, ok := catalog.Definition(input.DefID)
		if !ok || material.Kind != cultivation.KindMaterial || material.BasePrice < 1 {
			return method, nil, fmt.Errorf("craft foundation material is not priced")
		}
		baseline += material.BasePrice * int64(input.Quantity)
		core[input.DefID] = input.Quantity
	}
	inputs := slices.Clone(design.Inputs)
	if len(inputs) == 0 {
		for _, input := range method.Inputs {
			input.Quantity *= int(factor)
			inputs = append(inputs, input)
		}
	}
	if len(inputs) > 16 {
		return method, nil, fmt.Errorf("too many generated craft materials")
	}
	value := int64(0)
	seen := map[string]bool{}
	for _, input := range inputs {
		material, ok := catalog.Definition(input.DefID)
		if !ok || material.Kind != cultivation.KindMaterial || material.BasePrice < 1 || input.Quantity < 1 || input.Quantity > 1000000 || seen[input.DefID] || slices.Contains(method.RequiredTools, input.DefID) {
			return method, nil, fmt.Errorf("invalid generated craft material")
		}
		seen[input.DefID] = true
		value += material.BasePrice * int64(input.Quantity)
		if n := core[input.DefID]; n > 0 {
			if input.Quantity < n {
				return method, nil, fmt.Errorf("generated craft reduced a core material")
			}
			delete(core, input.DefID)
		}
	}
	if len(core) > 0 || value < baseline*factor {
		return method, nil, fmt.Errorf("generated craft material value is below its output budget")
	}
	method.Inputs = inputs
	method.MinutesPerBatch *= factor
	method.Outputs = []cultivation.CraftOutput{{DefID: id, Quantity: 1, Chance: 1}}
	if stage, ok := cultivation.ParseStageKey(method.MinimumStage, system.CultivationRules); ok && target.ApplicableStage != "" && !stageReaches(stage, target.ApplicableStage) {
		method.MinimumStage = target.ApplicableStage
	}
	return method, product, method.Validate()
}

func generatedCraftArtSupports(art cultivation.CraftArt, kind cultivation.ItemKind) bool {
	switch art {
	case cultivation.CraftArtSmithing:
		return kind == cultivation.KindImplement || kind == cultivation.KindArmor || kind == cultivation.KindAttackTreasure || kind == cultivation.KindDefenseTreasure || kind == cultivation.KindSupportTreasure
	case cultivation.CraftArtAlchemy:
		return kind == cultivation.KindPill
	case cultivation.CraftArtTalisman:
		return kind == cultivation.KindTalisman
	case cultivation.CraftArtFormation:
		return kind == cultivation.KindFormation
	}
	return false
}
