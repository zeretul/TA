package interactive

import (
	"fmt"
	"sort"

	"ta/internal/quest"
)

// The frozen initial owner remains a provenance reservation even after losing
// the item. A generic reward must not remint a lost or already transferred relic.
// Live ownership is also checked so a later established NPC cannot be bypassed.
func adventureUniqueItemOwner(r AdventureRules, state map[string]any, itemID string) (string, error) {
	owner := ""
	claim := func(source string) error {
		if source == "" || owner != "" && owner != source {
			return fmt.Errorf("unique item ownership is ambiguous: %s", itemID)
		}
		owner = source
		return nil
	}
	for _, npc := range r.NPCs {
		if npc.Actor.Inventory[itemID] > 0 {
			if err := claim(npc.CharacterSourceKey); err != nil {
				return "", err
			}
		}
	}
	actors, _ := getPath(state, actorStateRoot).(map[string]any)
	for id := range actors {
		if id == DefaultActorID {
			continue
		}
		actor, err := AdventureActorFromState(state, id)
		if err == nil && actor.Inventory[itemID] > 0 {
			if err := claim(actor.SourceKey); err != nil {
				return "", err
			}
		}
	}
	return owner, nil
}

// Called only by the formal quest planners after acceptance and the authored
// success/completion checks. Giver identifies the person making that promise;
// victory or scene presence alone does not authorize confiscating their gear.
func adventureQuestOwnedRewardOps(p *storyBranchProjection, system StoryDirectorActorStateSystem, d quest.Definition, currency int, items map[string]int, experience int, sourceID string) ([]ActorStateOp, error) {
	if p == nil || system.AdventureRules == nil {
		return nil, fmt.Errorf("item handover requires a frozen branch")
	}
	rules := *system.AdventureRules
	unique := map[string]bool{}
	for _, item := range rules.Items {
		unique[item.ID] = item.Unique
	}
	ids := make([]string, 0, len(items))
	for id, n := range items {
		if n > 0 && unique[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	transferred := map[string]string{}
	owners := map[string]AdventureActor{}
	for _, id := range ids {
		source, err := adventureUniqueItemOwner(rules, p.State, id)
		if err != nil {
			return nil, err
		}
		if source == "" {
			continue
		}
		player, playerErr := AdventureActorFromState(p.State, DefaultActorID)
		if playerErr != nil || player.Health <= 0 || player.Battle != nil && player.Battle.Status == "active" {
			return nil, fmt.Errorf("unique item handover requires an active player outside battle")
		}
		if source != d.Giver || items[id] != 1 {
			return nil, fmt.Errorf("unique reward requires its actual owner's agreed handover: %s", id)
		}
		profile, present := questEchoPartner(p, source)
		if !present {
			return nil, fmt.Errorf("unique item owner is not available for the agreed handover: %s", id)
		}
		actor, loaded := owners[profile.ID]
		if !loaded {
			actor, err = AdventureActorFromState(p.State, profile.ID)
			if err != nil {
				// The same frozen record used by encounters supplies an established
				// person's first inventory. A malformed existing record is not replaced.
				if getPath(p.State, actorStateFieldPath(profile.ID, AdventureStateField)) != nil {
					return nil, err
				}
				for _, npc := range rules.NPCs {
					if npc.CharacterSourceKey == source {
						actor = npc.Actor
						actor.SourceKey = npc.CharacterSourceKey
						break
					}
				}
			}
			if actor.SourceKey != source || actor.Rank < 1 {
				return nil, fmt.Errorf("unique item owner has no confirmed inventory: %s", id)
			}
			// Actor maps must be detached even for a lazily materialized frozen NPC.
			detached := intervalProjectionAfterOps(p, actorOpsForAdventureOwner(actor, profile.ID, sourceID))
			actor, err = AdventureActorFromState(detached.State, profile.ID)
			if err != nil {
				return nil, err
			}
		}
		if actor.Inventory[id] != 1 {
			return nil, fmt.Errorf("unique item owner no longer holds the promised item: %s", id)
		}
		if _, deployed := actor.Assets[id]; deployed {
			return nil, fmt.Errorf("deployed unique assets cannot be handed over: %s", id)
		}
		// Stateful charged/cooldown relics need an explicit state-transfer contract.
		// Refuse rather than reset their remaining uses while transferring ownership.
		for _, def := range rules.Items {
			if def.ID == id && (def.MaxCharges > 0 || def.CooldownSeconds > 0) {
				return nil, fmt.Errorf("stateful unique item handover is not supported: %s", id)
			}
		}
		actor.Inventory[id] = 0
		if actor.Acquired == nil {
			actor.Acquired = map[string]bool{}
		}
		actor.Acquired[id] = true
		for slot, equipped := range actor.Equipment {
			if equipped == id {
				delete(actor.Equipment, slot)
			}
		}
		if err := ValidateAdventureActor(rules, actor); err != nil {
			return nil, err
		}
		owners[profile.ID] = actor
		transferred[id] = source
	}
	ops, err := adventureRewardOps(p.State, system, currency, items, experience, sourceID, transferred)
	if err != nil {
		return nil, err
	}
	ownerIDs := make([]string, 0, len(owners))
	for id := range owners {
		ownerIDs = append(ownerIDs, id)
	}
	sort.Strings(ownerIDs)
	for _, id := range ownerIDs {
		ops = append(ops, actorOpsForAdventureOwner(owners[id], id, sourceID)...)
	}
	return ops, nil
}

func actorOpsForAdventureOwner(a AdventureActor, actorID, sourceID string) []ActorStateOp {
	ops := adventureActorOps(a, sourceID)
	for i := range ops {
		ops[i].ActorID = actorID
	}
	return ops
}

// Successful choice handovers preserve ownership in the same transaction as
// player costs. The input view already includes rewards, so a recipient who also
// gave a reward cannot have that outgoing inventory overwritten by this update.
// Stateful assets and generated substitutes remain outside this bounded path.
func adventureQuestItemHandoverOps(p *storyBranchProjection, system StoryDirectorActorStateSystem, c quest.Choice, sourceID string) ([]ActorStateOp, error) {
	if p == nil || system.AdventureRules == nil {
		return nil, fmt.Errorf("item handover requires a frozen branch")
	}
	rules := *system.AdventureRules
	items := map[string]AdventureItem{}
	for _, item := range rules.Items {
		items[item.ID] = item
	}
	owners := map[string]AdventureActor{}
	for _, handover := range c.ItemHandovers {
		profile, present := questEchoPartner(p, handover.CharacterSourceKey)
		if !present || profile.ID == DefaultActorID {
			return nil, fmt.Errorf("item recipient is not available for the agreed handover: %s", handover.CharacterSourceKey)
		}
		actor, err := AdventureActorFromState(p.State, profile.ID)
		if err != nil {
			// Missing initial state may use its frozen author record. Malformed
			// committed state must never be replaced by a fresh inventory.
			if getPath(p.State, actorStateFieldPath(profile.ID, AdventureStateField)) != nil {
				return nil, err
			}
			for _, npc := range rules.NPCs {
				if npc.CharacterSourceKey == handover.CharacterSourceKey {
					actor = npc.Actor
					actor.SourceKey = npc.CharacterSourceKey
					break
				}
			}
		}
		if actor.SourceKey != handover.CharacterSourceKey || actor.Rank < 1 || actor.Health <= 0 || actor.Battle != nil && actor.Battle.Status == "active" {
			return nil, fmt.Errorf("item recipient has no confirmed inventory: %s", handover.CharacterSourceKey)
		}
		if err := ValidateAdventureActor(rules, actor); err != nil {
			return nil, err
		}
		detached := intervalProjectionAfterOps(p, actorOpsForAdventureOwner(actor, profile.ID, sourceID))
		actor, err = AdventureActorFromState(detached.State, profile.ID)
		if err != nil {
			return nil, err
		}
		if actor.Inventory == nil {
			actor.Inventory = map[string]int{}
		}
		if actor.Acquired == nil {
			actor.Acquired = map[string]bool{}
		}
		for id, quantity := range handover.Items {
			item, known := items[id]
			if !known || quantity < 1 || item.Kind == "machine" || item.Kind == "ship" || item.MaxCharges > 0 || item.CooldownSeconds > 0 {
				return nil, fmt.Errorf("handover requires a registered stateless physical item: %s", id)
			}
			if quantity > 1000000-actor.Inventory[id] || item.Unique && (quantity != 1 || actor.Inventory[id] != 0) {
				return nil, fmt.Errorf("item recipient cannot hold this quantity: %s", id)
			}
			actor.Inventory[id] += quantity
			actor.Acquired[id] = true
		}
		if err := ValidateAdventureActor(rules, actor); err != nil {
			return nil, err
		}
		owners[profile.ID] = actor
	}
	ids := make([]string, 0, len(owners))
	for id := range owners {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var ops []ActorStateOp
	for _, id := range ids {
		ops = append(ops, actorOpsForAdventureOwner(owners[id], id, sourceID)...)
	}
	return ops, nil
}
