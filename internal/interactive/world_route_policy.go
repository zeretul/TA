package interactive

import (
	"fmt"
	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/exploration"
	"ta/internal/quest"
)

// WorldRoutePolicy contains authored requirements, never client-supplied costs.
type WorldRoutePolicy struct {
	Name               string              `json:"name,omitempty"`
	NameKey            string              `json:"name_key,omitempty"`
	OriginNameKey      string              `json:"origin_name_key,omitempty"`
	DestinationNameKey string              `json:"destination_name_key,omitempty"`
	Event              *WorldEventGate     `json:"event,omitempty"`
	AnyEcho            []quest.EchoGate    `json:"any_echo,omitempty"`
	Discovery          *WorldDiscoveryGate `json:"discovery,omitempty"`
	FactionID          string              `json:"faction_id,omitempty"`
	Crossing           *WorldCrossingCost  `json:"crossing,omitempty"`
}
type WorldEventGate struct {
	EventID string `json:"event_id"`
	Action  string `json:"action"`
	StageID string `json:"stage_id"`
}
type WorldDiscoveryGate struct {
	RealmID    string `json:"realm_id"`
	ApproachID string `json:"approach_id"`
	AreaID     string `json:"area_id"`
}
type WorldCrossingCost struct {
	MinimumStage   string  `json:"minimum_stage"`
	ItemID         string  `json:"item_id"`
	Quantity       int     `json:"quantity"`
	HealthFraction float64 `json:"health_fraction"`
	ManaFraction   float64 `json:"mana_fraction"`
}

func ownedRouteAvailable(p *storyBranchProjection, id string) bool {
	policy := p.worldContent().RoutePolicies[id]
	for _, j := range p.Journeys {
		if j.RouteID == id && !journeyFinished(j) {
			return true
		}
	}
	if g := policy.Event; g != nil {
		v := p.PublicEvents[g.EventID]
		if string(v.Action) != g.Action || v.JoinedStage != g.StageID {
			return false
		}
	}
	if len(policy.AnyEcho) > 0 {
		open := false
		for i := range policy.AnyEcho {
			if questEchoGateOpen(p, &policy.AnyEcho[i]) {
				open = true
			}
		}
		if !open {
			return false
		}
	}
	if g := policy.Discovery; g != nil {
		open := false
		for _, i := range p.Realms {
			s := i.Snapshot
			if s.Binding.DefinitionID != g.RealmID || s.ExploreState == nil {
				continue
			}
			returned, walked := false, false
			for _, a := range s.ExploreState.UsedApproachIDs {
				if a == g.ApproachID {
					returned = true
				}
			}
			for _, d := range s.Discoveries {
				if d.Kind == exploration.DiscoveryArea && d.ID == g.AreaID {
					walked = true
				}
			}
			if returned && walked {
				open = true
			}
		}
		if !open {
			return false
		}
	}
	if policy.FactionID != "" {
		open := false
		if p.Characters != nil {
			for _, m := range character.CurrentFactionMemberships(p.Characters.FactionMemberships) {
				if m.FactionID == policy.FactionID && m.Status == "formal" {
					open = true
				}
			}
		}
		if !open {
			return false
		}
	}
	if policy.Crossing != nil {
		_, err := worldCrossingOps(p.State, []string{DefaultActorID}, p.cultivationRules(), id, *policy.Crossing)
		return err == nil
	}
	return true
}

func worldCrossingOps(state map[string]any, travelers []string, rules *cultivation.Rules, routeID string, cost WorldCrossingCost) ([]ActorStateOp, error) {
	var ops []ActorStateOp
	for _, actor := range travelers {
		stage, ok := cultivation.ParseStageKey(xiuxianStoredStageKey(state, actor), rules)
		if !ok || !stageReaches(stage, cost.MinimumStage) {
			return nil, fmt.Errorf("%w: traveler below crossing requirement", ErrJourneyUnknownRoute)
		}
		base := cultivation.BaseAttributes(stage)
		hp, mp := xiuxianStoredInt(state, actor, xiuxianFieldHealth), xiuxianStoredInt(state, actor, xiuxianFieldMana)
		healthCost, manaCost := int(float64(base.HealthMax)*cost.HealthFraction), int(float64(base.ManaMax)*cost.ManaFraction)
		if hp <= healthCost || mp < manaCost {
			return nil, fmt.Errorf("%w: insufficient crossing reserves", ErrJourneyUnknownRoute)
		}
		pack, _ := xiuxianEquipmentEntry(state, actor, xiuxianFieldImportantItems)
		next := make(map[string]any, len(pack))
		remaining := cost.Quantity
		for _, key := range sortedKeys(pack) {
			raw := pack[key]
			entry, _ := raw.(map[string]any)
			next[key] = raw
			if remaining > 0 && xiuxianEntryText(entry, "定义") == cost.ItemID && xiuxianCarriedQuantity(entry) > 0 {
				copy := map[string]any{}
				for k, v := range entry {
					copy[k] = v
				}
				quantity := xiuxianCarriedQuantity(entry)
				used := min(quantity, remaining)
				copy["数量"] = float64(quantity - used)
				next[key] = copy
				remaining -= used
			}
		}
		if remaining > 0 {
			return nil, fmt.Errorf("%w: missing crossing supplies", ErrJourneyUnknownRoute)
		}
		for _, f := range []struct {
			name  string
			value any
		}{{xiuxianFieldImportantItems, next}, {xiuxianFieldHealth, float64(hp - healthCost)}, {xiuxianFieldMana, float64(mp - manaCost)}} {
			ops = append(ops, ActorStateOp{Op: "set", ActorID: actor, FieldID: normalizeActorStateFieldName(f.name), Value: f.value, Reason: "按本世界路线消耗已准备的物品和生命、灵力，保留修为与已学能力", SourceKind: StateOpSourceJourney, SourceID: routeID})
		}
	}
	return ops, nil
}

func journeyDepartureCosts(p *storyBranchProjection, id string, travelers []string) ([]ActorStateOp, error) {
	if c := p.worldContent(); c != nil {
		cost := c.RoutePolicies[id].Crossing
		if cost == nil {
			return nil, nil
		}
		return worldCrossingOps(p.State, travelers, p.cultivationRules(), id, *cost)
	}
	if id == spiritAscentRoute {
		return spiritCrossingOps(p.State, travelers)
	}
	return nil, nil
}
