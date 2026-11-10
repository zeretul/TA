package interactive

import (
	"fmt"
	"log/slog"
)

// AdventureOpeningHazard is a frozen, origin-authored appointment with the
// world, not an accepted quest. DueSeconds is on the branch clock from creation.
// Leaving the named area resolves exposure; ignoring it pays the declared harm
// once and moves the survivor to the declared safe place.
type AdventureOpeningHazard struct {
	ID                string   `json:"id"`
	DueSeconds        int64    `json:"due_seconds"`
	LocationIDs       []string `json:"location_ids"`
	EscapeLocationID  string   `json:"escape_location_id"`
	Damage            int      `json:"damage"`
	Injury            int      `json:"injury"`
	Description       string   `json:"description"`
	Consequence       string   `json:"consequence"`
	Resolved          bool     `json:"resolved,omitempty"`
	Outcome           string   `json:"outcome,omitempty"`
	ResolvedAtSeconds int64    `json:"resolved_at_seconds,omitempty"`
}

func validateAdventureOpeningHazards(hazards []AdventureOpeningHazard) error {
	if len(hazards) > 8 {
		return fmt.Errorf("too many opening hazards")
	}
	seen := map[string]bool{}
	for _, h := range hazards {
		if h.ID == "" || seen[h.ID] || h.DueSeconds <= 0 || h.DueSeconds > 31536000 || len(h.LocationIDs) == 0 || h.EscapeLocationID == "" || h.Damage < 0 || h.Damage > 100 || h.Injury < 0 || h.Injury > 100 || h.Description == "" || h.Consequence == "" || len(h.Description) > 2048 || len(h.Consequence) > 2048 {
			return fmt.Errorf("invalid opening hazard %s", h.ID)
		}
		if h.Resolved && (h.Outcome != "escaped" && h.Outcome != "exposed") {
			return fmt.Errorf("invalid opening hazard outcome")
		}
		if !h.Resolved && (h.Outcome != "" || h.ResolvedAtSeconds != 0) {
			return fmt.Errorf("unresolved opening hazard carries an outcome")
		}
		seen[h.ID] = true
	}
	return nil
}

// adventureOpeningBoundary cuts explicit waiting at the next pending hazard.
// Productive work is refused if it would run across this boundary: its full
// reward must not be paid for a training/crafting interval that was interrupted.
func adventureOpeningBoundary(state map[string]any, from, to int64, worlds ...*WorldContent) int64 {
	a, err := AdventureActorFromState(state, DefaultActorID)
	if err != nil {
		return to
	}
	limit := to
	for _, h := range a.OpeningHazards {
		if !h.Resolved && storyLocationIsAny(state, h.LocationIDs, worlds...) && h.DueSeconds > from && h.DueSeconds < limit {
			limit = h.DueSeconds
		}
	}
	return limit
}

func adventureOpeningEvents(p *storyBranchProjection, system StoryDirectorActorStateSystem, branchID, parentID, now, turnID string, clockAfter StoryClock) ([]any, string) {
	if p == nil || system.AdventureRules == nil {
		return nil, parentID
	}
	a, err := AdventureActorFromState(p.State, DefaultActorID)
	if err != nil {
		return nil, parentID
	}
	changed := refreshAdventureFogExposure(*system.AdventureRules, &a, p.State)
	if refreshAdventureShields(*system.AdventureRules, &a, clockAfter.Seconds) {
		changed = true
	}
	escape := ""
	for i, h := range a.OpeningHazards {
		if h.Resolved {
			continue
		}
		present := storyLocationIsAny(p.State, h.LocationIDs, p.worldContent())
		if present && clockAfter.Seconds < h.DueSeconds {
			continue
		}
		h.Resolved = true
		h.ResolvedAtSeconds = clockAfter.Seconds
		h.Outcome = "escaped"
		if present {
			h.Outcome = "exposed"
			h.ResolvedAtSeconds = h.DueSeconds
			a.Health = max(1, a.Health-h.Damage)
			a.Injury = min(100, a.Injury+h.Injury)
			escape = h.EscapeLocationID
		}
		a.OpeningHazards[i] = h
		changed = true
		slog.Info("[interactive-story] opening hazard resolved", "hazard", h.ID, "branch", branchID, "outcome", h.Outcome, "at_seconds", h.ResolvedAtSeconds)
	}
	if !changed {
		return nil, parentID
	}
	ops := adventureActorOps(a, turnID)
	if escape != "" {
		ops = append(ops, ActorStateOp{ActorID: DefaultStoryContextActorID, FieldID: storySceneLocationField, Op: "set", Value: worldPlaceLabel(p.worldContent(), escape), Reason: "opening hazard forced evacuation", SourceTurnID: turnID})
	}
	id := newID("sd")
	return []any{newStateDeltaEventWithActorOps(id, parentID, branchID, now, nil, ops)}, id
}

// Interval consumers see the just-committed costs and rewards, so an injury
// record cannot overwrite the inventory or progress from the same action.
func intervalProjectionAfterOps(p *storyBranchProjection, ops []ActorStateOp) *storyBranchProjection {
	if p == nil {
		return nil
	}
	view := *p
	view.State = cloneStoryState(p.State)
	applyStateDeltaToProjection(view.State, newStateDeltaWithActorOps(nil, ops))
	return &view
}
