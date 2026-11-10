package interactive

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"ta/internal/cultivation"
)

// AdventureGeneratedDefinition is a Story-local adaptation of a frozen rule
// template. Optional design proposals are checked by the existing rule budgets.
// Registration grants no stock. Equipment adapts one real uncharged held copy,
// skills require an already learned foundation, and recipes retain all costs.
type AdventureGeneratedDefinition struct {
	QualityGrade cultivation.QualityGrade  `json:"quality_grade,omitempty" jsonschema_description:"六色展示品级 white、green、blue、purple、orange、red；不替代效果预算、材料和资格。"`
	Design       *AdventureGeneratedDesign `json:"design,omitempty" jsonschema_description:"可选规则内机制提案。省略保留模板机制；填写则真正校验效果、资格、材料与工作时间。"`
	Category     string                    `json:"category" jsonschema:"enum=skill,enum=equipment,enum=craft" jsonschema_description:"新技能、装备或制作方法。"`
	TemplateID   string                    `json:"template_id" jsonschema_description:"本局冻结目录中已有的同类效果模板ID；不能复制另一个生成定义。"`
	Name         string                    `json:"name" jsonschema_description:"故事中的新名称，可自由创作，最多40字。"`
	Source       string                    `json:"source" jsonschema_description:"本次实际创作或改制的来历。技能须有已学基础；装备须实际持有可改制的模板物品。"`
	Description  string                    `json:"description,omitempty" jsonschema_description:"新内容的简介与表现，最多8KiB。实际效果与门槛由模板及通过校验的design决定。"`
}

func (d AdventureGeneratedDefinition) id() (string, error) {
	return generatedContentID(d.Category, d.TemplateID, d.Name)
}

// adventureRulesForActor returns a fresh catalog without editing world defaults.
// Archive identities and capacity are checked before resolving the working set.
// Requested mechanics are revalidated on lookup, never silently substituted.
func adventureRulesForActor(r AdventureRules, a AdventureActor, requested ...string) (AdventureRules, error) {
	if len(a.GeneratedDefinitions) == 0 {
		if r.generatedCatalog {
			return adventureFrozenRules(r), nil
		}
		return r, nil
	}
	if err := validateAdventureGeneratedArchive(a.GeneratedDefinitions); err != nil {
		return r, err
	}
	wanted := adventureGeneratedWorkingSet(a, requested)
	r.Skills = slices.DeleteFunc(slices.Clone(r.Skills), func(v AdventureSkill) bool { return strings.HasPrefix(v.ID, "gen.") })
	r.Items = slices.DeleteFunc(slices.Clone(r.Items), func(v AdventureItem) bool { return strings.HasPrefix(v.ID, "gen.") })
	r.Recipes = slices.DeleteFunc(slices.Clone(r.Recipes), func(v AdventureRecipe) bool { return strings.HasPrefix(v.ID, "gen.") })
	r.generatedCatalog = false
	base := r
	r.generatedCatalog = true
	seen := map[string]bool{}
	templates := map[string]map[string]bool{"skill": {}, "equipment": {}, "craft": {}}
	for _, v := range base.Skills {
		templates["skill"][v.ID] = !v.OncePerTarget
	}
	for _, v := range base.Items {
		templates["equipment"][v.ID] = v.Kind != "material" && !v.Unique && v.MaxCharges == 0 && v.CooldownSeconds == 0
	}
	for _, v := range base.Recipes {
		templates["craft"][v.ID] = true
	}
	for _, d := range a.GeneratedDefinitions {
		id, err := d.id()
		if err != nil || strings.HasPrefix(d.TemplateID, "gen.") || strings.TrimSpace(d.Source) == "" || len(d.Source) > 8192 || len(d.Description) > 8192 || seen[id] || !templates[d.Category][d.TemplateID] {
			return r, fmt.Errorf("invalid generated adventure definition")
		}
		seen[id] = true
		if d.QualityGrade != "" && !d.QualityGrade.Valid() {
			return r, fmt.Errorf("invalid generated quality grade")
		}
		if !wanted[id] && !wanted[id+".product"] {
			continue
		}
		metadata := func(original cultivation.DefinitionDetails) cultivation.DefinitionDetails {
			grade := original.QualityGrade
			if d.QualityGrade != "" {
				grade = d.QualityGrade
			}
			return cultivation.DefinitionDetails{QualityGrade: grade, Description: d.Description, Background: d.Source}
		}
		found := false
		switch d.Category {
		case "skill":
			for _, v := range base.Skills {
				if v.ID == d.TemplateID && !v.OncePerTarget {
					var err error
					v, _, err = deriveAdventureGeneratedSkill(base, v, d)
					if err != nil {
						return r, err
					}
					v.ID, v.Name = id, d.Name
					v.GeneratedFrom = d.TemplateID
					v.DefinitionDetails = metadata(v.DefinitionDetails)
					r.Skills = append(r.Skills, v)
					found = true
					break
				}
			}
		case "equipment":
			for _, v := range base.Items {
				if v.ID == d.TemplateID && v.Kind != "material" && !v.Unique && v.MaxCharges == 0 && v.CooldownSeconds == 0 {
					if d.Design != nil {
						m, err := adventureGenerationMethod(base, d.Design.MethodID)
						if err != nil {
							return r, err
						}
						v, _, err = deriveAdventureGeneratedItem(base, v, m, d)
						if err != nil {
							return r, err
						}
					}
					v.ID, v.Name = id, d.Name
					v.GeneratedFrom = d.TemplateID
					v.DefinitionDetails = metadata(v.DefinitionDetails)
					r.Items = append(r.Items, v)
					found = true
					break
				}
			}
		case "craft":
			for _, v := range base.Recipes {
				if v.ID == d.TemplateID {
					if d.Design != nil {
						if d.Design.MethodID != "" || strings.TrimSpace(d.Design.ProductName) == "" {
							return r, fmt.Errorf("generated recipe requires a product name and no second method")
						}
						var product AdventureItem
						for _, item := range base.Items {
							if item.ID == d.Design.ProductTemplateID {
								product = item
								break
							}
						}
						if _, err := generatedContentID("item", product.ID, d.Design.ProductName); err != nil || product.ID == "" {
							return r, fmt.Errorf("invalid generated product identity")
						}
						var work adventureGenerationWork
						var err error
						product, work, err = deriveAdventureGeneratedItem(base, product, v, d)
						if err != nil {
							return r, err
						}
						product.GeneratedFrom = product.ID
						product.ID = adventureGeneratedProductID(d)
						product.Name = d.Design.ProductName
						product.DefinitionDetails = metadata(product.DefinitionDetails)
						product.Acquisition = "craft"
						r.Items = append(r.Items, product)
						v.OutputID = product.ID
						v.Materials = work.materials
						v.Hours = work.hours
						v.SkillRank = work.skillRank
						v.FuelCost = work.fuel
					}
					v.ID, v.Name = id, d.Name
					v.DefinitionDetails = metadata(v.DefinitionDetails)
					r.Recipes = append(r.Recipes, v)
					found = true
					break
				}
			}
		default:
			return r, fmt.Errorf("unknown generated adventure category")
		}
		if !found {
			return r, fmt.Errorf("unsupported generated adventure template %s", d.TemplateID)
		}
	}
	// The frozen base already passed full rule validation. Do not call Validate
	// here: it validates starting actors, whose reader resolves this catalog.
	return r, nil
}

func generateAdventureContent(r AdventureRules, a *AdventureActor, d AdventureGeneratedDefinition) error {
	id, err := d.id()
	if err != nil {
		return err
	}
	for _, existing := range a.GeneratedDefinitions {
		previous, _ := existing.id()
		if previous == id {
			if !reflect.DeepEqual(existing, d) {
				return fmt.Errorf("an existing generated identity cannot change its definition")
			}
			return nil
		}
	}
	if len(a.GeneratedDefinitions) >= generatedDefinitionLimit {
		return fmt.Errorf("generated adventure catalog is full")
	}
	next := *a
	next.GeneratedDefinitions = append(slices.Clone(a.GeneratedDefinitions), d)
	catalog, err := adventureRulesForActor(r, next, id)
	if err != nil {
		return err
	}
	if err = catalog.Validate(); err != nil {
		return err
	}
	switch d.Category {
	case "skill":
		if a.Skills[d.TemplateID] < 1 {
			return fmt.Errorf("a learned foundation is required to develop this technique")
		}
		for _, skill := range catalog.Skills {
			if skill.ID == id && a.Rank < skill.RequiredRank {
				return fmt.Errorf("generated skill rank requirement is not met")
			}
		}

		a.Skills[id] = 1
	case "equipment":
		if a.Inventory[d.TemplateID] < 1 {
			return fmt.Errorf("a real held copy is required for adaptation")
		}
		for _, equipped := range a.Equipment {
			if equipped == d.TemplateID {
				return fmt.Errorf("unequip the actual item before adaptation")
			}
		}
		a.Inventory[d.TemplateID]--
		a.Inventory[id]++
		a.Acquired[id] = true
		// A withdrawn asset remains the same physical object. Transfer all wear,
		// fuel and power-lock facts instead of creating a fresh deployment.
		if asset, exists := a.Assets[d.TemplateID]; exists {
			for _, item := range catalog.Items {
				if item.ID == id {
					asset.Shield = min(asset.Shield, item.Power)
					break
				}
			}
			a.Assets[id] = asset
			delete(a.Assets, d.TemplateID)
		}
	case "craft":
		for _, recipe := range catalog.Recipes {
			if recipe.ID == id && a.Skills[recipe.SkillID] < recipe.SkillRank {
				return fmt.Errorf("crafting skill requirement is not met")
			}
		}
	}
	a.GeneratedDefinitions = next.GeneratedDefinitions
	return nil
}

// AdventureItemTemplateID resolves only a committed Story-local adaptation.
// It does not infer identity from a name, grant ownership or edit the catalog.
func AdventureItemTemplateID(a AdventureActor, itemID string) string {
	for _, d := range a.GeneratedDefinitions {
		if d.Category == "craft" && d.Design != nil && itemID == adventureGeneratedProductID(d) {
			return d.Design.ProductTemplateID
		}
		if d.Category == "equipment" {
			if id, err := d.id(); err == nil && id == itemID {
				return d.TemplateID
			}
		}
	}
	return itemID
}

// AdventureOwnedItemCount counts real held copies satisfying a frozen template
// requirement. Requiring a specific generated identity still matches only it.
func AdventureOwnedItemCount(a AdventureActor, requiredID string) int {
	count := max(0, a.Inventory[requiredID])
	seen := map[string]bool{requiredID: true}
	for _, d := range a.GeneratedDefinitions {
		candidate := ""
		if d.Category == "craft" && d.Design != nil && d.Design.ProductTemplateID == requiredID {
			candidate = adventureGeneratedProductID(d)
		}
		if d.Category == "equipment" && d.TemplateID == requiredID {
			candidate, _ = d.id()
		}
		if candidate != "" {
			if id := candidate; !seen[id] {
				count += max(0, a.Inventory[id])
				seen[id] = true
			}
		}
	}
	return count
}

// Only the actual deployed copy may supply a template's machine capability.
func adventureDeployedAssetID(a AdventureActor, requiredID string) string {
	for _, scale := range []string{"machine", "ship"} {
		id := a.Equipment[scale+":asset"]
		if id != "" && a.Inventory[id] > 0 && (id == requiredID || AdventureItemTemplateID(a, id) == requiredID) {
			return id
		}
	}
	return ""
}

// Authored costs consume real copies, not a second balance under a template ID.
// Prefer the unmodified object, then stable IDs; never consume equipped or
// tracked assets. The returned deductions are applied by the existing journal
// transaction only after every requirement and outcome has been validated.
func adventureItemCosts(a AdventureActor, costs map[string]int) (map[string]int, error) {
	blocked := map[string]bool{}
	for _, id := range a.Equipment {
		blocked[id] = true
	}
	for id := range a.Assets {
		blocked[id] = true
	}
	ids, requirements := make([]string, 0, len(a.Inventory)), make([]string, 0, len(costs))
	for id := range a.Inventory {
		ids = append(ids, id)
	}
	for id := range costs {
		requirements = append(requirements, id)
	}
	slices.Sort(ids)
	slices.Sort(requirements)
	spent := map[string]int{}
	for _, required := range requirements {
		remaining := costs[required]
		if remaining < 1 || remaining > 1000000 {
			return nil, fmt.Errorf("invalid item cost %s", required)
		}
		for _, id := range append([]string{required}, ids...) {
			if blocked[id] || (id != required && AdventureItemTemplateID(a, id) != required) {
				continue
			}
			quantity := min(remaining, max(0, a.Inventory[id]+spent[id]))
			if quantity == 0 {
				continue
			}
			spent[id] -= quantity
			remaining -= quantity
			if remaining == 0 {
				break
			}
		}
		if remaining > 0 {
			return nil, fmt.Errorf("required item cost is not owned or available: %s", required)
		}
	}
	return spent, nil
}

// The archive may outlive thousands of sold items or unused recipes. Settlement
// needs the actor's actual references plus this action's exact targets, not every
// historical recipe. A small recent tail keeps new methods discoverable.
func adventureGeneratedWorkingSet(a AdventureActor, requested []string) map[string]bool {
	wanted := map[string]bool{}
	for _, id := range requested {
		wanted[id] = true
	}
	for id := range a.Skills {
		wanted[id] = true
	}
	for id := range a.Training {
		wanted[id] = true
	}
	for id := range a.Inventory {
		wanted[id] = true
	}
	for _, id := range a.Equipment {
		wanted[id] = true
	}
	for id := range a.Assets {
		wanted[id] = true
	}
	for id := range a.ItemCharges {
		wanted[id] = true
	}
	for id := range a.ItemReadyAt {
		wanted[id] = true
	}
	start := max(0, len(a.GeneratedDefinitions)-generatedContextDefinitionLimit)
	for _, d := range a.GeneratedDefinitions[start:] {
		id, _ := d.id()
		wanted[id] = true
	}
	return wanted
}

// Frozen validation and prompt budgets never grow with the Story archive.
func adventureFrozenRules(r AdventureRules) AdventureRules {
	r.Skills = slices.DeleteFunc(slices.Clone(r.Skills), func(v AdventureSkill) bool { return strings.HasPrefix(v.ID, "gen.") })
	r.Items = slices.DeleteFunc(slices.Clone(r.Items), func(v AdventureItem) bool { return strings.HasPrefix(v.ID, "gen.") })
	r.Recipes = slices.DeleteFunc(slices.Clone(r.Recipes), func(v AdventureRecipe) bool { return strings.HasPrefix(v.ID, "gen.") })
	r.generatedCatalog = false
	return r
}
