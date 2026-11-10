package storypack

import (
	"encoding/json"

	"ta/internal/interactive"
)

// identityInitialState projects the same canonical template value that story
// creation seeds. Native adventure state must not fall through to cultivation
// projections or authored preview prose, neither of which owns its numbers.
func (p Pack) identityInitialState(template ProtagonistTemplate) (*CreationInitialState, error) {
	rules := p.Story.Gameplay.AdventureRules
	if rules == nil {
		if p.Story.Gameplay.StatusTemplate == WorldStatusGeneralActions && len(p.Story.Gameplay.AuthoredStateFields) > 0 {
			initial := &CreationInitialState{}
			for _, field := range p.Story.Gameplay.AuthoredStateFields {
				value, exists := template.State[field.Name]
				if !exists {
					continue
				}
				// Detach the proposal from the pack's authoritative initial state.
				raw, err := json.Marshal(value)
				if err != nil {
					return nil, err
				}
				var copied any
				if err := json.Unmarshal(raw, &copied); err != nil {
					return nil, err
				}
				initial.AuthoredFields = append(initial.AuthoredFields, CreationAuthoredField{Name: field.Name, Type: field.Type, Value: copied})
			}
			return initial, nil
		}
		return initialStateOf(template), nil
	}
	raw, err := json.Marshal(template.State[interactive.AdventureStateField])
	if err != nil {
		return nil, err
	}
	var actor interactive.AdventureActor
	if err := json.Unmarshal(raw, &actor); err != nil {
		return nil, err
	}
	if err := interactive.ValidateAdventureActor(*rules, actor); err != nil {
		return nil, err
	}
	initial := &CreationInitialState{
		Family:        rules.Family,
		Rank:          actor.Rank,
		CurrencyLabel: rules.Currency,
		Currency:      actor.Currency,
	}
	// Catalog order is stable across previews; map iteration is not. Equipped
	// items already occur in canonical inventory and must not be listed twice.
	for _, item := range rules.Items {
		if actor.Inventory[item.ID] > 0 {
			initial.Items = append(initial.Items, item.Name)
		}
		if actor.Equipment[item.Scale+":"+item.Kind] == item.ID {
			initial.Equipped = append(initial.Equipped, item.Name)
		}
	}
	for _, skill := range rules.Skills {
		if actor.Skills[skill.ID] > 0 {
			initial.Techniques = append(initial.Techniques, skill.Name)
		}
	}
	return initial, nil
}
