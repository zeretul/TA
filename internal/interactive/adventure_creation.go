package interactive

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AdventureStartingLoadout is an authored creation grant, frozen with the same
// rules as its skills and items. Player requests choose IDs, never its values.
// The grant adds a starting discipline; it is not a permanent learning lock.
type AdventureStartingLoadout struct {
	ID        string            `json:"id"`
	Group     string            `json:"group"`
	Skills    []string          `json:"skills"`
	Items     map[string]int    `json:"items"`
	Equipment map[string]string `json:"equipment,omitempty"`
}

func validateAdventureStartingLoadouts(r AdventureRules) error {
	if len(r.StartingLoadouts) > 24 {
		return fmt.Errorf("too many starting loadouts")
	}
	seen := map[string]bool{}
	for _, kit := range r.StartingLoadouts {
		if strings.TrimSpace(kit.ID) == "" || seen[kit.ID] {
			return fmt.Errorf("invalid or duplicate starting loadout %s", kit.ID)
		}
		seen[kit.ID] = true
		if r.Family == "space" && kit.Group != "class" || r.Family == "wuxia" && kit.Group != "weapon" && kit.Group != "method" {
			return fmt.Errorf("starting loadout has wrong family or group: %s", kit.ID)
		}
		if len(kit.Skills) < 1 || len(kit.Skills) > 4 || len(kit.Items) > 16 {
			return fmt.Errorf("invalid starting loadout size: %s", kit.ID)
		}
		// The plain starting actor checks each grant independently, so a loadout
		// cannot rely on a different optional discipline supplying its tools.
		if _, err := ApplyAdventureStartingLoadouts(r, NewAdventureActor(r), kit.ID); err != nil {
			return fmt.Errorf("starting loadout %s: %w", kit.ID, err)
		}
	}
	return nil
}

// ApplyAdventureStartingLoadouts is creation-only: callers supply an authored template, never a live save.
// The template may begin above rank one; selected disciplines do not change its rank or purse.
func ApplyAdventureStartingLoadouts(r AdventureRules, original AdventureActor, ids ...string) (AdventureActor, error) {
	raw, err := json.Marshal(original)
	if err != nil {
		return original, err
	}
	var actor AdventureActor
	if err = json.Unmarshal(raw, &actor); err != nil {
		return original, err
	}
	if actor.Rank < 1 || actor.Battle != nil {
		return original, fmt.Errorf("starting loadouts require a valid authored actor outside battle")
	}
	if actor.Skills == nil {
		actor.Skills = map[string]int{}
	}
	if actor.Inventory == nil {
		actor.Inventory = map[string]int{}
	}
	if actor.Equipment == nil {
		actor.Equipment = map[string]string{}
	}
	skills := map[string]AdventureSkill{}
	items := map[string]AdventureItem{}
	for _, s := range r.Skills {
		skills[s.ID] = s
	}
	for _, i := range r.Items {
		items[i.ID] = i
	}
	groups := map[string]bool{}
	for _, id := range ids {
		var kit *AdventureStartingLoadout
		for i := range r.StartingLoadouts {
			if r.StartingLoadouts[i].ID == id {
				kit = &r.StartingLoadouts[i]
				break
			}
		}
		if kit == nil || groups[kit.Group] {
			return original, fmt.Errorf("unknown or repeated starting loadout group: %s", id)
		}
		groups[kit.Group] = true
		for itemID, n := range kit.Items {
			item, ok := items[itemID]
			if !ok || n < 1 || n > 100 || item.Unique || item.Kind == "machine" || item.Kind == "ship" {
				return original, fmt.Errorf("invalid starting loadout item: %s", itemID)
			}
			actor.Inventory[itemID] += n
		}
		learned := map[string]bool{}
		for _, skillID := range kit.Skills {
			skill, ok := skills[skillID]
			if !ok || learned[skillID] || skill.RequiredRank > actor.Rank {
				return original, fmt.Errorf("invalid starting loadout skill: %s", skillID)
			}
			learned[skillID] = true
			if skill.Prerequisite != "" && actor.Skills[skill.Prerequisite] < 1 && !containsAdventureID(kit.Skills, skill.Prerequisite) {
				return original, fmt.Errorf("starting skill lacks prerequisite: %s", skillID)
			}
			if skill.RequiredItem != "" && actor.Inventory[skill.RequiredItem] < 1 {
				return original, fmt.Errorf("starting skill lacks a learning source: %s", skillID)
			}
			if skill.RequiredTool != "" && actor.Inventory[skill.RequiredTool] < 1 {
				return original, fmt.Errorf("starting skill lacks its tool: %s", skillID)
			}
			for material, n := range skill.Materials {
				if actor.Inventory[material] < 2*n {
					return original, fmt.Errorf("starting skill lacks two uses of materials: %s", skillID)
				}
			}
			actor.Skills[skillID] = max(1, actor.Skills[skillID])
		}
		for slot, itemID := range kit.Equipment {
			item, ok := items[itemID]
			if !ok || actor.Inventory[itemID] < 1 || item.Scale != "person" || (item.Kind != "weapon" && item.Kind != "armor" && item.Kind != "tool") || slot != item.Scale+":"+item.Kind {
				return original, fmt.Errorf("invalid starting equipment: %s", itemID)
			}
			if item.RequiredSkill != "" && actor.Skills[item.RequiredSkill] < 1 {
				return original, fmt.Errorf("starting equipment qualification missing: %s", itemID)
			}
			for material, n := range item.Materials {
				if actor.Inventory[material] < 2*n {
					return original, fmt.Errorf("starting equipment lacks two uses of ammunition: %s", itemID)
				}
			}
			actor.Equipment[slot] = itemID
		}
	}
	for _, id := range ids {
		for _, kit := range r.StartingLoadouts {
			if kit.ID != id {
				continue
			}
			for _, skillID := range kit.Skills {
				if !adventureSkillWeaponAvailable(r, skills[skillID], actor.Inventory, actor.Equipment) {
					return original, fmt.Errorf("starting skill lacks the equipped weapon it requires: %s", skillID)
				}
			}
		}
	}
	if err = ValidateAdventureActor(r, actor); err != nil {
		return original, err
	}
	return actor, nil
}

func containsAdventureID(ids []string, id string) bool {
	for _, value := range ids {
		if value == id {
			return true
		}
	}
	return false
}
