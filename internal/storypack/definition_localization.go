package storypack

import (
	"encoding/json"
	"fmt"

	"ta/internal/cultivation"
)

// FrozenGameplay captures sidecar definition prose only at story creation.
// Pack author documents and reusable mechanics modules remain canonical; the
// returned independent copy may be persisted in a Story, never written to Pack.
func (p Pack) FrozenGameplay() (WorldGameplay, error) {
	data, err := json.Marshal(p.Story.Gameplay)
	if err != nil {
		return WorldGameplay{}, fmt.Errorf("clone pack gameplay for story: %w", err)
	}
	var gameplay WorldGameplay
	if err := json.Unmarshal(data, &gameplay); err != nil {
		return WorldGameplay{}, fmt.Errorf("clone pack gameplay for story: %w", err)
	}
	english := p.Localizations["en-US"].Definitions
	attach := func(id string, details *cultivation.DefinitionDetails) {
		if prose, ok := english[id]; ok {
			details.Localizations = &cultivation.DefinitionLocalizations{English: prose}
		}
	}
	for id, details := range gameplay.DefinitionDetails {
		attach(id, &details)
		gameplay.DefinitionDetails[id] = details
	}
	if rules := gameplay.AdventureRules; rules != nil {
		for i := range rules.Skills {
			attach(rules.Skills[i].ID, &rules.Skills[i].DefinitionDetails)
		}
		for i := range rules.Recipes {
			attach(rules.Recipes[i].ID, &rules.Recipes[i].DefinitionDetails)
		}
		for i := range rules.Items {
			attach(rules.Items[i].ID, &rules.Items[i].DefinitionDetails)
		}
	}
	return gameplay, nil
}

// Pack authoring and Story persistence deliberately have different boundaries:
// the shared DefinitionDetails type may carry frozen translations in a Story,
// but accepting them in a Pack would recreate a second author-language source.
func validateDefinitionLocaleSeparation(gameplay WorldGameplay) error {
	check := func(id string, details cultivation.DefinitionDetails) error {
		if details.Localizations != nil {
			return fmt.Errorf("canonical definition %s must keep translations in a localization sidecar", id)
		}
		return nil
	}
	for id, details := range gameplay.DefinitionDetails {
		if err := check(id, details); err != nil {
			return err
		}
	}
	if rules := gameplay.AdventureRules; rules != nil {
		for _, skill := range rules.Skills {
			if err := check(skill.ID, skill.DefinitionDetails); err != nil {
				return err
			}
		}
		for _, recipe := range rules.Recipes {
			if err := check(recipe.ID, recipe.DefinitionDetails); err != nil {
				return err
			}
		}
		for _, item := range rules.Items {
			if err := check(item.ID, item.DefinitionDetails); err != nil {
				return err
			}
		}
	}
	return nil
}
