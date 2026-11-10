package interactive

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// AdventureGeneratedDesign is a proposal, not an unrestricted state delta.
// The frozen catalog supplies effect implementations, professions, resource
// values and machinery. The same derivation runs at registration and replay.
type AdventureGeneratedDesign struct {
	Power             *int              `json:"power,omitempty" jsonschema_description:"拟定实际威力，1至100；只改已有实际消费者的主属性，不能凭描述添加能力。"`
	Cost              *int              `json:"cost,omitempty" jsonschema_description:"技能每次消耗。不得低于规则按效果总量算出的下限；装备和配方不填。"`
	Effects           []AdventureEffect `json:"effects,omitempty" jsonschema_description:"技能可组合已学基础中的 attack、guard、heal、control、poison、repair，每类最多一次、最多三类。只填真正需要的效果；复杂条件技能不在本次自由组合范围内。"`
	Foundations       []string          `json:"foundations,omitempty" jsonschema_description:"组合技能所依赖的额外已学冻结技能ID，最多3个，同一尺度且保留其真实门槛。"`
	MethodID          string            `json:"method_id,omitempty" jsonschema_description:"装备改制所用的冻结制作方法ID，产物须与改制品同种类和尺度；craft类已由template_id指定。"`
	Materials         map[string]int    `json:"materials,omitempty" jsonschema_description:"制作或改制每批实际消耗的材料ID与数量。省略按基础方法增加；自定材料须覆盖基础核心材料及更强效果的成本，不可降低。"`
	ProductTemplateID string            `json:"product_template_id,omitempty" jsonschema_description:"craft类的产物基础物品ID；省略取原方法产物，不得改变种类、尺度或复制独特物品。"`
	ProductName       string            `json:"product_name,omitempty" jsonschema_description:"craft类实际新产物名称，最多40字；方法登记不直接获得产物，仍须执行craft。"`
}

type adventureGenerationWork struct {
	hours     int
	skillID   string
	skillRank int
	materials map[string]int
	assetID   string
	fuel      int
}

func adventureDesignFactor(power, base int) int { return max(1, (power+max(1, base)-1)/max(1, base)) }

func adventurePlainSkillEffects(s AdventureSkill) ([]AdventureEffect, bool) {
	if s.SpaceCombat != nil || s.Mobility != 0 || s.OncePerTarget || s.Backlash != nil || len(s.ConditionalEffects) != 0 || len(s.EnemyEffects) != 0 || s.RangeMeters != 0 || s.Rush || s.WeaponStyle || len(s.DrainCounterSkills) != 0 {
		return nil, false
	}
	effects := s.Effects
	if len(effects) == 0 {
		effects = []AdventureEffect{{Kind: s.Kind, Power: s.Power}}
	}
	if _, err := adventureDesignEffectBudget(effects, s.Scale); err != nil {
		return nil, false
	}
	return effects, true
}

func adventureDesignEffectBudget(effects []AdventureEffect, scale string) (int, error) {
	if len(effects) < 1 || len(effects) > 3 {
		return 0, fmt.Errorf("generated skill requires one to three supported effects")
	}
	seen := map[string]bool{}
	budget := 0
	for _, e := range effects {
		if seen[e.Kind] || e.Power < 1 || e.Power > 100 {
			return 0, fmt.Errorf("generated effect must be distinct and have power in 1..100")
		}
		seen[e.Kind] = true
		switch e.Kind {
		case "attack", "guard":
		case "heal", "control", "poison":
			if scale != "person" {
				return 0, fmt.Errorf("biological generated effect requires person scale")
			}
		case "repair":
			if scale == "person" {
				return 0, fmt.Errorf("generated repair requires an asset scale")
			}
		default:
			return 0, fmt.Errorf("generated effect has no supported composition consumer: %s", e.Kind)
		}
		budget += e.Power
		if e.Kind == "control" {
			budget += 20
		}
	}
	return budget + 10*(len(effects)-1), nil
}

func deriveAdventureGeneratedSkill(r AdventureRules, base AdventureSkill, d AdventureGeneratedDefinition) (AdventureSkill, adventureGenerationWork, error) {
	v := base
	design := d.Design
	if design == nil {
		return v, adventureGenerationWork{}, nil
	}
	if design.MethodID != "" || len(design.Materials) > 0 || design.ProductName != "" || design.ProductTemplateID != "" || len(design.Foundations) > 3 {
		return v, adventureGenerationWork{}, fmt.Errorf("skill design contains unrelated crafting fields")
	}
	baseEffects, ok := adventurePlainSkillEffects(base)
	if !ok {
		return v, adventureGenerationWork{}, fmt.Errorf("this conditional skill cannot be freely recomposed")
	}
	available := map[string]bool{}
	minimumCost := 0
	for _, e := range baseEffects {
		available[e.Kind] = true
	}
	seen := map[string]bool{base.ID: true}
	for _, id := range design.Foundations {
		if seen[id] || strings.HasPrefix(id, "gen.") {
			return v, adventureGenerationWork{}, fmt.Errorf("generated foundations must be distinct frozen skills")
		}
		seen[id] = true
		found := false
		for _, s := range r.Skills {
			if s.ID == id {
				effects, plain := adventurePlainSkillEffects(s)
				if !plain || s.Scale != base.Scale || len(s.Materials) > 0 || s.RequiredTool != "" || s.RequiredWeapon != "" || s.RequiredWeaponClass != "" {
					return v, adventureGenerationWork{}, fmt.Errorf("foundation has incompatible scale or additional operating conditions")
				}
				for _, e := range effects {
					available[e.Kind] = true
				}
				minimumCost = max(minimumCost, s.Cost)
				found = true
				break
			}
		}
		if !found {
			return v, adventureGenerationWork{}, fmt.Errorf("unknown generated skill foundation")
		}
	}
	effects := slices.Clone(baseEffects)
	if len(design.Effects) > 0 {
		effects = slices.Clone(design.Effects)
	}
	if design.Power != nil {
		if len(effects) != 1 || len(design.Effects) > 0 {
			return v, adventureGenerationWork{}, fmt.Errorf("set effects rather than an unused power for a composite skill")
		}
		effects[0].Power = *design.Power
	}
	budget, err := adventureDesignEffectBudget(effects, base.Scale)
	if err != nil {
		return v, adventureGenerationWork{}, err
	}
	for _, e := range effects {
		if !available[e.Kind] {
			return v, adventureGenerationWork{}, fmt.Errorf("effect %s requires a learned supporting foundation", e.Kind)
		}
	}
	baseBudget, _ := adventureDesignEffectBudget(baseEffects, base.Scale)
	factor := adventureDesignFactor(budget, baseBudget)
	if factor > 4 {
		return v, adventureGenerationWork{}, fmt.Errorf("generated skill exceeds fourfold foundation budget")
	}
	minimumCost = max(minimumCost, (base.Cost*budget+baseBudget-1)/baseBudget+5*max(0, len(effects)-len(baseEffects)))
	v.Cost = minimumCost
	if design.Cost != nil {
		v.Cost = *design.Cost
	}
	if v.Cost < minimumCost || v.Cost > 100 {
		return v, adventureGenerationWork{}, fmt.Errorf("generated skill cost must be between %d and 100", minimumCost)
	}
	v.Effects = effects
	v.Power = effects[0].Power
	v.RequiredRank = base.RequiredRank + factor - 1
	v.TrainingHours = base.TrainingHours * factor
	v.Prerequisite = base.ID
	if v.RequiredRank > AdventureMaxRank(r) || v.TrainingHours > 100000 {
		return v, adventureGenerationWork{}, fmt.Errorf("generated skill exceeds progression limits")
	}
	return v, adventureGenerationWork{hours: v.TrainingHours, skillID: base.ID, skillRank: factor}, nil
}

func deriveAdventureGeneratedItem(r AdventureRules, base AdventureItem, method AdventureRecipe, d AdventureGeneratedDefinition) (AdventureItem, adventureGenerationWork, error) {
	v := base
	design := d.Design
	if design == nil {
		return v, adventureGenerationWork{}, nil
	}
	if design.Cost != nil || len(design.Effects) > 0 || len(design.Foundations) > 0 || design.Power == nil {
		return v, adventureGenerationWork{}, fmt.Errorf("item design requires an actual power and no skill-only fields")
	}
	if base.Unique || base.MaxCharges > 0 || base.CooldownSeconds > 0 || base.Power < 1 || method.OutputCount != 1 {
		return v, adventureGenerationWork{}, fmt.Errorf("item requires an ordinary single-output foundation with a consumed power stat")
	}
	switch base.Kind {
	case "weapon", "armor", "machine", "ship":
	case "consumable":
		if base.Effect != "heal" && base.Effect != "energy" && base.Effect != "poison" && base.Effect != "attack" {
			return v, adventureGenerationWork{}, fmt.Errorf("consumable power is not consumed by this effect")
		}
	default:
		return v, adventureGenerationWork{}, fmt.Errorf("item has no supported adjustable power consumer")
	}
	v.Power = *design.Power
	if v.Power < 1 || v.Power > 100 {
		return v, adventureGenerationWork{}, fmt.Errorf("generated item power must be in 1..100")
	}
	var original AdventureItem
	for _, i := range r.Items {
		if i.ID == method.OutputID {
			original = i
			break
		}
	}
	if original.ID == "" || original.Kind != base.Kind || original.Scale != base.Scale || method.ID == "" {
		return v, adventureGenerationWork{}, fmt.Errorf("crafting foundation must produce the same item kind and scale")
	}
	factor := max(adventureDesignFactor(v.Power, base.Power), adventureDesignFactor(v.Power, original.Power))
	if factor > 4 {
		return v, adventureGenerationWork{}, fmt.Errorf("generated item exceeds fourfold crafting foundation power")
	}
	costFactor := max(factor*factor, adventureDesignFactor(base.Price, original.Price))
	if costFactor > 1000 {
		return v, adventureGenerationWork{}, fmt.Errorf("generated material value exceeds recipe capacity")
	}
	materials := maps.Clone(method.Materials)
	for id, n := range materials {
		materials[id] = n * costFactor
	}
	if len(design.Materials) > 0 {
		materials = maps.Clone(design.Materials)
	}
	if len(materials) == 0 || len(materials) > 16 {
		return v, adventureGenerationWork{}, fmt.Errorf("generated item requires bounded real materials")
	}
	// Core ingredients cannot disappear by replacing expensive parts with an
	// unrelated cheap stack. Extra material choices remain narrative proposals.
	for id, n := range method.Materials {
		if materials[id] < n {
			return v, adventureGenerationWork{}, fmt.Errorf("generated craft omits a core material %s", id)
		}
	}
	value, baseline := 0, 0
	for id, n := range materials {
		found := false
		for _, i := range r.Items {
			if i.ID == id && i.Kind == "material" && !i.Unique && i.Price > 0 && !strings.HasPrefix(id, "gen.") {
				value += i.Price * n
				found = true
				break
			}
		}
		if !found || n < 1 || n > 10000 {
			return v, adventureGenerationWork{}, fmt.Errorf("invalid priced crafting material %s", id)
		}
	}
	for id, n := range method.Materials {
		for _, i := range r.Items {
			if i.ID == id {
				baseline += i.Price * n
				break
			}
		}
	}
	if baseline < 1 || value < baseline*costFactor {
		return v, adventureGenerationWork{}, fmt.Errorf("material value is below the generated effect budget")
	}
	rank := method.SkillRank + factor - 1
	cap := 10
	if r.Family == "wuxia" {
		cap = 4
	}
	hours := method.Hours * factor
	if rank > cap || hours > 100000 {
		return v, adventureGenerationWork{}, fmt.Errorf("generated craft exceeds profession or work limits")
	}
	return v, adventureGenerationWork{hours: hours, skillID: method.SkillID, skillRank: rank, materials: materials, assetID: method.RequiredAsset, fuel: method.FuelCost * factor}, nil
}

func adventureGenerationMethod(r AdventureRules, id string) (AdventureRecipe, error) {
	if strings.HasPrefix(id, "gen.") {
		return AdventureRecipe{}, fmt.Errorf("generation requires a frozen method")
	}
	for _, m := range r.Recipes {
		if m.ID == id {
			return m, nil
		}
	}
	return AdventureRecipe{}, fmt.Errorf("a registered crafting foundation is required")
}

func adventureGeneratedProductID(d AdventureGeneratedDefinition) string {
	id, _ := d.id()
	return id + ".product"
}

func settleAdventureGenerationWork(r AdventureRules, a *AdventureActor, d AdventureGeneratedDefinition, hours int, now int64) (int64, error) {
	if d.Design == nil {
		return 0, nil
	}
	var work adventureGenerationWork
	var err error
	switch d.Category {
	case "skill":
		for _, s := range r.Skills {
			if s.ID == d.TemplateID {
				_, work, err = deriveAdventureGeneratedSkill(r, s, d)
				break
			}
		}
		for _, id := range d.Design.Foundations {
			if a.Skills[id] < 1 {
				return 0, fmt.Errorf("all composing foundations must actually be learned")
			}
		}
	case "equipment":
		m, e := adventureGenerationMethod(r, d.Design.MethodID)
		if e != nil {
			return 0, e
		}
		if m.RequiredTool != "" && AdventureOwnedItemCount(*a, m.RequiredTool) < 1 {
			return 0, fmt.Errorf("generated work requires its owned reusable tool")
		}
		for _, i := range r.Items {
			if i.ID == d.TemplateID {
				_, work, err = deriveAdventureGeneratedItem(r, i, m, d)
				break
			}
		}
	case "craft":
		return 0, nil // Registering a method never manufactures its output.
	}
	if err != nil {
		return 0, err
	}
	if work.hours < 1 || hours < work.hours || a.Skills[work.skillID] < work.skillRank || a.Injury >= 50 {
		return 0, fmt.Errorf("generation requires %d authorized hours and foundation mastery %d without severe injury", work.hours, work.skillRank)
	}
	spent, e := adventureItemCosts(*a, work.materials)
	if e != nil {
		return 0, e
	}
	if work.assetID != "" {
		id := adventureDeployedAssetID(*a, work.assetID)
		asset, ok := a.Assets[id]
		if !ok || asset.Health < 1 || asset.DisabledUntil > now || asset.Fuel < work.fuel {
			return 0, fmt.Errorf("generated work requires its powered working asset")
		}
		asset.Fuel -= work.fuel
		a.Assets[id] = asset
	}
	for id, delta := range spent {
		a.Inventory[id] += delta
	}
	return int64(work.hours) * 3600, nil
}

func cloneAdventureGeneratedDefinition(d AdventureGeneratedDefinition) AdventureGeneratedDefinition {
	if d.Design == nil {
		return d
	}
	design := *d.Design
	design.Effects = slices.Clone(design.Effects)
	design.Foundations = slices.Clone(design.Foundations)
	design.Materials = maps.Clone(design.Materials)
	if design.Power != nil {
		value := *design.Power
		design.Power = &value
	}
	if design.Cost != nil {
		value := *design.Cost
		design.Cost = &value
	}
	d.Design = &design
	return d
}
