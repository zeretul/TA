package interactive

import "sort"

// FactionAffairView is a read-only next-turn summary. Looking at it never
// spends time, reveals an unencountered person or claims a reward.
type FactionAffairView struct {
	ProjectID     string `json:"project_id"`
	GroupName     string `json:"group_name"`
	PartnerID     string `json:"partner_id"`
	PartnerName   string `json:"partner_name"`
	RecipeID      string `json:"recipe_id"`
	RecipeName    string `json:"recipe_name"`
	WorkedHours   int    `json:"worked_hours"`
	RequiredHours int    `json:"required_hours"`
	Phase         string `json:"phase"`
	CanWork       bool   `json:"can_work"`
}
type FactionRecipeView struct {
	PartnerID            string         `json:"partner_id"`
	PartnerName          string         `json:"partner_name"`
	RecipeID             string         `json:"recipe_id"`
	RecipeName           string         `json:"recipe_name"`
	RequiredHours        int            `json:"required_hours"`
	PartnerEnergyPerHour int            `json:"partner_energy_per_hour"`
	Materials            map[string]int `json:"materials"`
	OutputID             string         `json:"output_id"`
	OutputCount          int            `json:"output_count"`
}

type FactionAffairsView struct {
	Projects []FactionAffairView `json:"projects,omitempty"`
	Recipes  []FactionRecipeView `json:"recipes,omitempty"`
}

func factionAffairsView(p *storyBranchProjection, meta StoryMeta) FactionAffairsView {
	view := FactionAffairsView{}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, StoryDirectorActorStateSystem{})
	if p == nil || p.Characters == nil || system.AdventureRules == nil {
		return view
	}
	rules := *system.AdventureRules
	inside := false
	for _, instance := range p.Realms {
		inside = inside || instance.Snapshot.Presence == "inside"
	}
	for _, m := range p.Characters.Matters {
		if m.Faction == nil {
			continue
		}
		f := m.Faction
		partner, actor, err := factionPartner(p, p.State, rules, m.CharacterID)
		canWork := !inside && err == nil && f.Phase == "active" && realmActionBlocked(p, p.State, meta) == nil
		recipe, _, recipeErr := factionRecipe(rules, actor, f.RecipeID)
		view.Projects = append(view.Projects, FactionAffairView{ProjectID: f.ProjectID, GroupName: f.GroupName, PartnerID: m.CharacterID, PartnerName: partner.Name, RecipeID: f.RecipeID, RecipeName: recipe.Name, WorkedHours: f.WorkedHours, RequiredHours: f.RequiredHours, Phase: f.Phase, CanWork: canWork && recipeErr == nil})
	}
	sort.SliceStable(view.Projects, func(i, j int) bool {
		a, b := view.Projects[i], view.Projects[j]
		if (a.Phase == "active") != (b.Phase == "active") {
			return a.Phase == "active"
		}
		return a.ProjectID < b.ProjectID
	})
	// Closed affairs remain in character history; the next-turn page needs only
	// bounded recent summaries, while every active affair must remain visible.
	if len(view.Projects) > 48 {
		view.Projects = view.Projects[:48]
	}
	if realmActionBlocked(p, p.State, meta) != nil {
		return view
	}
	for _, instance := range p.Realms {
		if instance.Snapshot.Presence == "inside" {
			return view
		}
	}
	profiles := p.Characters.Profiles
	for _, profile := range profiles {
		partner, actor, err := factionPartner(p, p.State, rules, profile.ID)
		if err != nil {
			continue
		}
		for _, candidate := range rules.Recipes {
			recipe, skill, err := factionRecipe(rules, actor, candidate.ID)
			if err != nil {
				continue
			}
			materials := map[string]int{}
			for id, n := range recipe.Materials {
				materials[id] = n
			}
			view.Recipes = append(view.Recipes, FactionRecipeView{PartnerID: partner.ID, PartnerName: partner.Name, RecipeID: recipe.ID, RecipeName: recipe.Name, RequiredHours: recipe.Hours, PartnerEnergyPerHour: max(1, skill.Cost), Materials: materials, OutputID: recipe.OutputID, OutputCount: recipe.OutputCount})
		}
	}
	sort.Slice(view.Recipes, func(i, j int) bool {
		a, b := view.Recipes[i], view.Recipes[j]
		if a.PartnerID != b.PartnerID {
			return a.PartnerID < b.PartnerID
		}
		return a.RecipeID < b.RecipeID
	})
	if len(view.Recipes) > 96 {
		view.Recipes = view.Recipes[:96]
	}
	return view
}
