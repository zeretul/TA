package exploration

import (
	"fmt"
	"math"
	"sort"
)

// FindExploreApproach returns a detached copy for trusted cost evaluation. The
// definition, including hidden outcomes, must not be returned to public callers.
func FindExploreApproach(def DungeonDefinition, id string) (ExploreApproach, bool) {
	if def.ExploreRules != nil {
		for _, a := range def.ExploreRules.Approaches {
			if a.ID == id {
				return cloneExploreApproach(a), true
			}
		}
	}
	return ExploreApproach{}, false
}

// ApplyExploreApproach proposes one atomic action. Story must commit its actor
// costs, rewards and clock together with this detached instance transition.
func ApplyExploreApproach(def DungeonDefinition, scope DungeonScope, snapshot DungeonInstanceSnapshot, ctx InstanceActionContext, input ExploreApproachInput) (ExploreTransition, error) {
	if err := validateActionContext(def, scope, snapshot, ctx, ActionExplore, input.ApproachID, "", "", true); err != nil {
		return ExploreTransition{}, err
	}
	if def.ExploreRules == nil || snapshot.ExploreState == nil || snapshot.Presence != InstanceInside {
		return ExploreTransition{}, fmt.Errorf("%w: exploration requires an active authored place", ErrInvalidInstanceAction)
	}
	a, ok := FindExploreApproach(def, input.ApproachID)
	if !ok {
		return ExploreTransition{}, fmt.Errorf("%w: approach is not declared", ErrInvalidInstanceAction)
	}
	if input.StoryTimeSeconds < 0 || input.StoryTimeSeconds > math.MaxInt64-a.DurationSeconds {
		return ExploreTransition{}, ErrTimeOverflow
	}
	if !exploreApproachVisible(snapshot, a) {
		return ExploreTransition{}, fmt.Errorf("%w: approach has not been discovered here", ErrActionNotAllowed)
	}
	if reason := exploreBlockedReason(snapshot, a, input.StoryTimeSeconds, input.Capabilities); reason != "" {
		return ExploreTransition{}, fmt.Errorf("%w: %s", ErrActionNotAllowed, reason)
	}
	after := cloneInstanceSnapshot(snapshot)
	state := after.ExploreState
	state.UsedApproachIDs = append(state.UsedApproachIDs, a.ID)
	if a.ExclusiveGroup != "" {
		state.Decisions = append(state.Decisions, ExploreDecision{GroupID: a.ExclusiveGroup, ApproachID: a.ID})
	}
	state.Pressure += a.PressureDelta
	if state.Pressure < 0 {
		state.Pressure = 0
	}
	setbackNow := !state.Setback && state.Pressure >= def.ExploreRules.PressureLimit
	if setbackNow {
		state.Setback = true
	}
	rewards := []InstanceReward{}
	if !setbackNow {
		for _, d := range a.Discoveries {
			d.SourceRef = ctx.Permit.SourceRef
			if err := applyDiscovery(&after, def, d); err != nil {
				return ExploreTransition{}, err
			}
		}
		for _, id := range a.ResolveChallengeIDs {
			var err error
			after.ResolvedChallenges, err = appendProgress(after.ResolvedChallenges, id, ctx.Permit.SourceRef)
			if err != nil {
				return ExploreTransition{}, err
			}
		}
		for _, id := range a.RewardRefIDs {
			ref, _ := findContentRef(def, id)
			if containsRewardID(after.GrantedRewards, id) {
				return ExploreTransition{}, fmt.Errorf("%w: reward already granted", ErrInstanceConflict)
			}
			reward := InstanceReward{ID: id, RefID: ref.ReferenceID, Quantity: ref.Quantity, SourceRef: ctx.Permit.SourceRef}
			after.GrantedRewards = append(after.GrantedRewards, reward)
			rewards = append(rewards, reward)
			if ref.ObjectiveID != "" {
				if err := applyDiscovery(&after, def, InstanceDiscovery{Kind: DiscoveryObjective, ID: ref.ObjectiveID, SourceRef: ctx.Permit.SourceRef}); err != nil {
					return ExploreTransition{}, err
				}
				var err error
				after.CompletedObjectives, err = appendProgress(after.CompletedObjectives, ref.ObjectiveID, ctx.Permit.SourceRef)
				if err != nil {
					return ExploreTransition{}, err
				}
			}
		}
		if a.StartsReturn {
			end := input.StoryTimeSeconds + a.DurationSeconds
			if end > math.MaxInt64-def.ExploreRules.ReturnAfterSeconds {
				return ExploreTransition{}, ErrTimeOverflow
			}
			state.ReturnDueSeconds = end + def.ExploreRules.ReturnAfterSeconds
		}
	}
	sort.Slice(after.GrantedRewards, func(i, j int) bool { return after.GrantedRewards[i].ID < after.GrantedRewards[j].ID })
	transition, err := finishInstanceTransition(snapshot, after, ctx.Permit.SourceRef)
	if err != nil {
		return ExploreTransition{}, err
	}
	if ds := ValidateDungeonInstance(def, scope, transition.Snapshot); len(ds) > 0 {
		return ExploreTransition{}, instanceDiagnosticError(ds)
	}
	return ExploreTransition{InstanceTransition: transition, DurationSeconds: a.DurationSeconds, Costs: append([]ExploreResourceCost{}, a.Costs...), Rewards: rewards}, nil
}

// ProjectExploreView only exposes methods at the current known place. Clock
// comparisons derive availability without changing the stored return moment.
func ProjectExploreView(def DungeonDefinition, scope DungeonScope, snapshot DungeonInstanceSnapshot, ctx ExploreViewContext) (ExploreView, []Diagnostic) {
	view := ExploreView{Approaches: []ExploreApproachView{}}
	if ds := ValidateDungeonInstance(def, scope, snapshot); len(ds) > 0 {
		return view, ds
	}
	if ctx.StoryTimeSeconds < 0 {
		return view, []Diagnostic{{"invalid_story_time", "story_time_seconds", "story time must be non-negative"}}
	}
	if def.ExploreRules == nil {
		return view, nil
	}
	rules, state := def.ExploreRules, snapshot.ExploreState
	view.Pressure = state.Pressure
	view.PressureLimit = rules.PressureLimit
	view.PressureLabel = rules.PressureLabel
	view.PressureLabelKey = rules.PressureLabelKey
	view.Setback = state.Setback
	view.ReturnDueSeconds = state.ReturnDueSeconds
	for _, a := range rules.Approaches {
		if a.RequiresReturn && containsString(state.UsedApproachIDs, a.ID) {
			view.ReturnOpened = true
		}
	}
	view.ReturnAvailable = state.ReturnDueSeconds > 0 && ctx.StoryTimeSeconds >= state.ReturnDueSeconds && !state.Setback && !view.ReturnOpened
	if snapshot.Presence != InstanceInside {
		return view, nil
	}
	for _, a := range rules.Approaches {
		if !exploreApproachVisible(snapshot, a) {
			continue
		}
		reason := exploreBlockedReason(snapshot, a, ctx.StoryTimeSeconds, ctx.Capabilities)
		view.Approaches = append(view.Approaches, ExploreApproachView{ID: a.ID, Kind: a.Kind, Name: a.Name, NameKey: a.NameKey, Description: a.Description, DescriptionKey: a.DescriptionKey, DurationSeconds: a.DurationSeconds, PressureDelta: a.PressureDelta, Costs: append([]ExploreResourceCost{}, a.Costs...), Available: reason == "", BlockedReason: reason, CausesSetback: !state.Setback && state.Pressure+a.PressureDelta >= rules.PressureLimit})
	}
	return view, nil
}

func exploreApproachVisible(s DungeonInstanceSnapshot, a ExploreApproach) bool {
	if s.CurrentAreaID != a.AreaID {
		return false
	}
	for _, id := range a.RequiredFactIDs {
		if !hasExploreFact(s, id) {
			return false
		}
	}
	if a.RequiresSetback && !s.ExploreState.Setback {
		return false
	}
	if a.RequiresReturn && s.ExploreState.ReturnDueSeconds == 0 {
		return false
	}
	return true
}
func exploreBlockedReason(s DungeonInstanceSnapshot, a ExploreApproach, now int64, capabilities []string) string {
	state := s.ExploreState
	if containsString(state.UsedApproachIDs, a.ID) {
		return "used"
	}
	if state.Setback && !a.AllowedAfterSetback {
		return "setback"
	}
	for _, d := range state.Decisions {
		if a.ExclusiveGroup != "" && d.GroupID == a.ExclusiveGroup {
			return "choice_made"
		}
	}
	for _, id := range a.ExcludedFactIDs {
		if hasExploreFact(s, id) {
			if a.RequiresSetback {
				return "depleted"
			}
			return "damaged"
		}
	}
	if a.RequiresReturn && now < state.ReturnDueSeconds {
		return "return_pending"
	}
	if !containsString(capabilities, a.ID) {
		if a.CooperationProjectID != "" {
			return "cooperation_required"
		}
		return "capability"
	}
	return ""
}
func hasExploreFact(s DungeonInstanceSnapshot, id string) bool {
	for _, d := range s.Discoveries {
		if d.Kind == DiscoveryFact && d.ID == id && d.PubliclyDisplayable {
			return true
		}
	}
	return false
}

// ExploreConnectionAvailable prevents guessed IDs from bypassing an approach.
// A setback closes forward exploration, but never removes a known retreat edge.
func ExploreConnectionAvailable(def DungeonDefinition, s DungeonInstanceSnapshot, connectionID, destinationID string) bool {
	if def.ExploreRules == nil {
		return true
	}
	if s.ExploreState == nil || s.Presence != InstanceInside {
		return false
	}
	known := false
	for _, d := range s.Discoveries {
		if d.Kind == DiscoveryConnection && d.ID == connectionID {
			known = true
			break
		}
	}
	if !known {
		return false
	}
	c, ok := findConnection(def, connectionID)
	if !ok {
		return false
	}
	if c.FromAreaID == s.CurrentAreaID && c.ToAreaID == destinationID {
		return !s.ExploreState.Setback
	}
	return c.Bidirectional && c.ToAreaID == s.CurrentAreaID && c.FromAreaID == destinationID
}

func cloneExploreApproach(a ExploreApproach) ExploreApproach {
	a.RequiredFactIDs = append([]string(nil), a.RequiredFactIDs...)
	a.ExcludedFactIDs = append([]string(nil), a.ExcludedFactIDs...)
	a.Costs = append([]ExploreResourceCost(nil), a.Costs...)
	a.Discoveries = append([]InstanceDiscovery(nil), a.Discoveries...)
	a.ResolveChallengeIDs = append([]string(nil), a.ResolveChallengeIDs...)
	a.RewardRefIDs = append([]string(nil), a.RewardRefIDs...)
	return a
}
func cloneExploreRules(r *ExploreRules) *ExploreRules {
	if r == nil {
		return nil
	}
	out := *r
	out.Approaches = make([]ExploreApproach, len(r.Approaches))
	for i, a := range r.Approaches {
		out.Approaches[i] = cloneExploreApproach(a)
	}
	return &out
}
func cloneExploreState(s *ExploreState) *ExploreState {
	if s == nil {
		return nil
	}
	out := *s
	out.UsedApproachIDs = append([]string{}, s.UsedApproachIDs...)
	out.Decisions = append([]ExploreDecision{}, s.Decisions...)
	return &out
}
