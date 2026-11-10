package interactive

import "ta/internal/cultivation"

// Display metadata is not model rule context. Copy only the slices we change;
// settlement fields remain byte-for-byte identical and the frozen source stays whole.
func adventureMechanicalRules(r AdventureRules) AdventureRules {
	r.Skills = append([]AdventureSkill(nil), r.Skills...)
	r.Items = append([]AdventureItem(nil), r.Items...)
	r.Recipes = append([]AdventureRecipe(nil), r.Recipes...)
	for i := range r.Skills {
		r.Skills[i].DefinitionDetails = cultivation.DefinitionDetails{}
	}
	for i := range r.Items {
		r.Items[i].DefinitionDetails = cultivation.DefinitionDetails{}
	}
	for i := range r.Recipes {
		r.Recipes[i].DefinitionDetails = cultivation.DefinitionDetails{}
	}
	return r
}
