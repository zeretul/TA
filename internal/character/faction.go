package character

import (
	"fmt"
	"reflect"
	"strings"
)

// FactionTerms records one voluntary, bounded production affair. It is not a
// membership, treasury, standing work order or permission to command its owner.
// Costs and outputs are copied from the story's frozen recipe by the rules.
type FactionTerms struct {
	ProjectID         string `json:"project_id"`
	GroupName         string `json:"group_name"`
	RecipeID          string `json:"recipe_id"`
	RequiredHours     int    `json:"required_hours"`
	WorkedHours       int    `json:"worked_hours"`
	Phase             string `json:"phase"`
	ConsentExchangeID string `json:"consent_exchange_id"`
	MaterialsSourceID string `json:"materials_source_id,omitempty"`
	OutputSourceID    string `json:"output_source_id,omitempty"`
}

func (f FactionTerms) Validate() error {
	if strings.TrimSpace(f.ProjectID) == "" || strings.TrimSpace(f.GroupName) == "" || strings.TrimSpace(f.RecipeID) == "" || f.RequiredHours < 1 || f.WorkedHours < 0 || f.WorkedHours > f.RequiredHours || f.ConsentExchangeID == "" {
		return fmt.Errorf("invalid faction affair terms")
	}
	if (f.WorkedHours > 0) != (f.MaterialsSourceID != "") {
		return fmt.Errorf("faction work requires exactly one funded material source")
	}
	switch f.Phase {
	case "active":
		if f.WorkedHours >= f.RequiredHours || f.OutputSourceID != "" {
			return fmt.Errorf("active faction affair cannot contain finished output")
		}
	case "completed":
		if f.WorkedHours != f.RequiredHours || f.OutputSourceID == "" {
			return fmt.Errorf("completed faction affair requires all work and its output source")
		}
	case "cancelled":
		if f.WorkedHours >= f.RequiredHours || f.OutputSourceID != "" {
			return fmt.Errorf("cancelled faction affair cannot contain output")
		}
	default:
		return fmt.Errorf("invalid faction affair phase")
	}
	return nil
}

// ApplyFactionMatter is the only reducer permitted to advance these terms.
// A fork may rebind the inherited matter to itself; its caller owns ancestry.
func ApplyFactionMatter(existing []Matter, next Matter) ([]Matter, error) {
	next = next.Normalize()
	if next.Faction == nil || next.Cooperation != nil || next.Kind != MatterAction {
		return nil, fmt.Errorf("faction update requires its own action matter")
	}
	if err := next.Validate(); err != nil {
		return nil, err
	}
	if next.Status == MatterOpen && next.Faction.Phase != "active" || next.Status != MatterOpen && next.Faction.Phase == "active" {
		return nil, fmt.Errorf("faction matter status disagrees with work phase")
	}
	out := make([]Matter, len(existing))
	index := -1
	for i, m := range existing {
		out[i] = m.Normalize()
		if m.ID == next.ID {
			if index != -1 {
				return nil, fmt.Errorf("duplicate faction matter")
			}
			index = i
		} else if m.Faction != nil && m.Faction.ProjectID == next.Faction.ProjectID {
			return nil, fmt.Errorf("faction project identity already exists")
		}
	}
	if index < 0 {
		if next.Faction.Phase != "active" || next.Faction.WorkedHours != 0 {
			return nil, fmt.Errorf("new faction affair must begin without work")
		}
		return append(out, next), nil
	}
	before := out[index]
	if reflect.DeepEqual(before, next) {
		return out, nil
	}
	if before.Faction == nil || before.StoryID != next.StoryID || before.CharacterID != next.CharacterID || before.Counterparty != next.Counterparty {
		return nil, fmt.Errorf("faction identity cannot change")
	}
	a, b := before.Faction, next.Faction
	if a.ProjectID != b.ProjectID || a.GroupName != b.GroupName || a.RecipeID != b.RecipeID || a.RequiredHours != b.RequiredHours || a.Phase != "active" || b.WorkedHours < a.WorkedHours || a.MaterialsSourceID != "" && a.MaterialsSourceID != b.MaterialsSourceID {
		return nil, fmt.Errorf("invalid faction work transition")
	}
	if b.Phase == "cancelled" {
		if b.WorkedHours != a.WorkedHours || b.ConsentExchangeID != a.ConsentExchangeID {
			return nil, fmt.Errorf("cancellation cannot perform work")
		}
	} else if b.WorkedHours <= a.WorkedHours || b.ConsentExchangeID == a.ConsentExchangeID {
		return nil, fmt.Errorf("faction work requires new work and fresh personal consent")
	}
	out[index] = next
	return out, nil
}
