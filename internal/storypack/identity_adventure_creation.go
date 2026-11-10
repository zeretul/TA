package storypack

import (
	"encoding/json"
	"fmt"
	"strings"
	"ta/internal/interactive"
)

func (p Pack) resolveAdventureCreation(choice IdentityChoice, identity *Identity) error {
	c := p.Creation
	if c == nil {
		return nil
	}
	type selection struct {
		group, id string
		options   []CreationOption
		set       func(CreationOption)
	}
	groups := []selection{
		{"class", choice.AdventureClassID, c.AdventureClasses, func(o CreationOption) { identity.AdventureClassID = o.ID; identity.AdventureClassName = o.Name }},
		{"weapon", choice.AdventureWeaponID, c.AdventureWeapons, func(o CreationOption) { identity.AdventureWeaponID = o.ID; identity.AdventureWeaponName = o.Name }},
		{"method", choice.AdventureMethodID, c.AdventureMethods, func(o CreationOption) { identity.AdventureMethodID = o.ID; identity.AdventureMethodName = o.Name }},
	}
	var ids []string
	for _, group := range groups {
		id := strings.TrimSpace(group.id)
		if len(group.options) == 0 {
			if id != "" {
				return identityError("api.storyPack.identity.adventureOptionUnknown", "id", id)
			}
			continue
		}
		var option CreationOption
		var ok bool
		if id == "" {
			option, ok = DefaultOption(group.options)
		} else {
			option, ok = optionByID(group.options, id)
		}
		if !ok {
			return identityError("api.storyPack.identity.adventureOptionUnknown", "id", id)
		}
		group.set(option)
		ids = append(ids, option.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	if p.Story.Gameplay.AdventureRules == nil {
		return fmt.Errorf("adventure creation options need native rules")
	}
	template, ok := p.ProtagonistTemplate(identity.TemplateID)
	if !ok {
		return fmt.Errorf("adventure state basis is missing")
	}
	raw, err := json.Marshal(template.State[interactive.AdventureStateField])
	if err != nil {
		return err
	}
	var actor interactive.AdventureActor
	if err = json.Unmarshal(raw, &actor); err != nil {
		return err
	}
	actor, err = interactive.ApplyAdventureStartingLoadouts(*p.Story.Gameplay.AdventureRules, actor, ids...)
	if err != nil {
		return err
	}
	identity.AdventureState = &actor
	// The preview and the committed overlay are projections of this exact value.
	updated := template
	updated.State = map[string]any{}
	for key, value := range template.State {
		updated.State[key] = value
	}
	updated.State[interactive.AdventureStateField] = actor
	identity.InitialState, err = p.identityInitialState(updated)
	return err
}

func (p Pack) validateAdventureCreation() error {
	c := p.Creation
	if c == nil {
		return nil
	}
	groups := map[string][]CreationOption{"class": c.AdventureClasses, "weapon": c.AdventureWeapons, "method": c.AdventureMethods}
	for group, options := range groups {
		seen := map[string]bool{}
		for _, option := range options {
			if strings.TrimSpace(option.ID) == "" || strings.TrimSpace(option.Name) == "" || seen[option.ID] {
				return fmt.Errorf("invalid adventure creation option: %s", option.ID)
			}
			seen[option.ID] = true
			r := p.Story.Gameplay.AdventureRules
			if r == nil {
				return fmt.Errorf("adventure creation options need native rules")
			}
			found := false
			for _, kit := range r.StartingLoadouts {
				if kit.ID == option.ID && kit.Group == group {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("creation option has no matching frozen loadout: %s", option.ID)
			}
		}
	}
	return nil
}

func (identity Identity) AdventureTrainingNames() string {
	names := []string{}
	for _, name := range []string{identity.AdventureClassName, identity.AdventureWeaponName, identity.AdventureMethodName} {
		if name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, " · ")
}
