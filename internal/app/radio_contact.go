package app

import (
	"errors"
	"strings"
	"ta/internal/character"
	"ta/internal/interactive"
)

var ErrRadioContactUnavailable = errors.New("radio contact requires an owned device and an established reachable channel")

// Reachability combines committed contact facts with current ownership. Losing
// the named device revokes access immediately; a world capability alone grants
// neither a contact address nor a connection.
func radioContactAvailable(ctx interactive.StoryContext, state character.State, characterID string) bool {
	if storyCommunicationMode(ctx.Meta.PackBinding) != "radio" {
		return false
	}
	profile, ok := state.Profile(characterID)
	if !ok || profile.RadioContact == nil {
		return false
	}
	contact := profile.RadioContact
	if contact.Status != "connected" || strings.TrimSpace(contact.Channel) == "" || strings.TrimSpace(contact.DeviceItemID) == "" {
		return false
	}
	// In adventure worlds an owned item is not automatically a transceiver.
	// Adventure ownership comes only from the canonical actor. Free-form item
	// descriptions cannot restore sold/lost hardware or create a second balance.
	var schema = ctx.Snapshot.ActorStateSchema
	if schema == nil {
		schema = ctx.Meta.ActorStateSchema
	}
	if schema != nil && schema.System.AdventureRules != nil {
		actor, err := interactive.AdventureActorFromState(ctx.Snapshot.State, interactive.DefaultActorID)
		if err != nil || interactive.ValidateAdventureActor(*schema.System.AdventureRules, actor) != nil {
			return false
		}
		templateID := interactive.AdventureItemTemplateID(actor, contact.DeviceItemID)
		capable := false
		for _, item := range schema.System.AdventureRules.Items {
			if item.ID != templateID {
				continue
			}
			capable = item.RadioDevice
			// The preceding unreleased catalog already authored this exact radio
			// before capability metadata existed. Keep that frozen device usable,
			// without updating its catalog or accepting arbitrary item names.
			if ctx.Meta.PackBinding != nil && ctx.Meta.PackBinding.PackID == "xinghai-689" && item.ID == "xinghai-radio" && item.Name == "全频段便携无线电" && item.Kind == "tool" && item.Scale == "person" {
				capable = true
			}
			break
		}
		// An adaptation keeps the physical device's frozen capability and a
		// previously established channel, but creates neither one from prose.
		return capable && interactive.AdventureOwnedItemCount(actor, contact.DeviceItemID) > 0
	}
	if actor, err := interactive.AdventureActorFromState(ctx.Snapshot.State, interactive.DefaultActorID); err == nil && actor.Inventory[contact.DeviceItemID] > 0 {
		return true
	}
	actors, _ := ctx.Snapshot.State["actors"].(map[string]any)
	actor, _ := actors[interactive.DefaultActorID].(map[string]any)
	values, _ := actor["state"].(map[string]any)
	items, _ := values["重要物品"].(map[string]any)
	item := items[contact.DeviceItemID]
	if entry, ok := item.(map[string]any); ok {
		item = entry["数量"]
		if item == nil {
			item = entry["quantity"]
		}
	}
	switch quantity := item.(type) {
	case float64:
		return quantity > 0
	case int:
		return quantity > 0
	case int64:
		return quantity > 0
	default:
		return false
	}
}
