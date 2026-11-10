package exploration

import (
	"fmt"
	"sort"
)

// RegisterInstanceDiscovery adds one already-confirmed discovery or rumor.
// Repeating the same identity is a no-op; contradictory fact content is rejected.
func RegisterInstanceDiscovery(def DungeonDefinition, scope DungeonScope, snapshot DungeonInstanceSnapshot, ctx InstanceActionContext, item InstanceDiscovery) (InstanceTransition, error) {
	if def.ExploreRules != nil {
		return InstanceTransition{}, fmt.Errorf("%w: authored exploration discoveries require an approach", ErrActionNotAllowed)
	}
	if err := validateActionContext(def, scope, snapshot, ctx, ActionDiscover, item.ID, "", "", false); err != nil {
		return InstanceTransition{}, err
	}
	if !validID(item.SourceRef) || item.SourceRef != ctx.Permit.SourceRef {
		return InstanceTransition{}, fmt.Errorf("%w: discovery source must match the trusted action permit", ErrInvalidInstanceAction)
	}
	after := cloneInstanceSnapshot(snapshot)
	if err := applyDiscovery(&after, def, item); err != nil {
		return InstanceTransition{}, err
	}
	return finishInstanceTransition(snapshot, after, ctx.Permit.SourceRef)
}

// RecordInstanceProgress records a sourced confirmed objective or challenge
// result; it does not create tasks, resolve conditions, or grant rewards.
func RecordInstanceProgress(def DungeonDefinition, scope DungeonScope, snapshot DungeonInstanceSnapshot, ctx InstanceActionContext, progress InstanceProgress) (InstanceTransition, error) {
	if def.ExploreRules != nil {
		return InstanceTransition{}, fmt.Errorf("%w: authored exploration progress requires an approach", ErrActionNotAllowed)
	}
	if progress.Outcome != OutcomeConfirmed {
		return InstanceTransition{}, fmt.Errorf("%w: progress requires an explicit confirmed result", ErrInvalidInstanceAction)
	}
	if err := validateActionContext(def, scope, snapshot, ctx, ActionProgress, progress.ID, "", "", false); err != nil {
		return InstanceTransition{}, err
	}
	if !validID(progress.SourceRef) || progress.SourceRef != ctx.Permit.SourceRef {
		return InstanceTransition{}, fmt.Errorf("%w: result source must match the trusted action permit", ErrInvalidInstanceAction)
	}
	after := cloneInstanceSnapshot(snapshot)
	var err error
	switch progress.Kind {
	case ProgressObjective:
		if !hasObjective(def, progress.ID) {
			return InstanceTransition{}, fmt.Errorf("%w: objective does not exist", ErrInvalidInstanceAction)
		}
		if err := applyDiscovery(&after, def, InstanceDiscovery{Kind: DiscoveryObjective, ID: progress.ID, SourceRef: progress.SourceRef}); err != nil {
			return InstanceTransition{}, err
		}
		after.CompletedObjectives, err = appendProgress(after.CompletedObjectives, progress.ID, progress.SourceRef)
	case ProgressChallenge:
		if !hasChallenge(def, progress.ID) {
			return InstanceTransition{}, fmt.Errorf("%w: challenge does not exist", ErrInvalidInstanceAction)
		}
		challenge := findChallenge(def, progress.ID)
		if snapshot.Presence != InstanceInside || challenge.AreaID != snapshot.CurrentAreaID {
			return InstanceTransition{}, fmt.Errorf("%w: challenge result is not at the current instance area", ErrInvalidInstanceAction)
		}
		if err := applyDiscovery(&after, def, InstanceDiscovery{Kind: DiscoveryArea, ID: challenge.AreaID, SourceRef: progress.SourceRef}); err != nil {
			return InstanceTransition{}, err
		}
		after.ResolvedChallenges, err = appendProgress(after.ResolvedChallenges, progress.ID, progress.SourceRef)
	default:
		return InstanceTransition{}, fmt.Errorf("%w: progress kind is not supported", ErrInvalidInstanceAction)
	}
	if err != nil {
		return InstanceTransition{}, err
	}
	return finishInstanceTransition(snapshot, after, ctx.Permit.SourceRef)
}

// MoveDungeonInstance proposes a directed adjacent move only with a fresh,
// exact permit and explicitly clear battle/block states. The traversed edge
// and destination become sourced discoveries in the proposed snapshot.
func MoveDungeonInstance(def DungeonDefinition, scope DungeonScope, snapshot DungeonInstanceSnapshot, ctx InstanceActionContext, input MoveDungeonInstanceInput) (InstanceTransition, error) {
	if snapshot.Presence != InstanceInside {
		return InstanceTransition{}, fmt.Errorf("%w: movement requires inside presence", ErrInvalidInstanceAction)
	}
	if err := validateActionContext(def, scope, snapshot, ctx, ActionMove, input.ConnectionID, snapshot.CurrentAreaID, input.DestinationAreaID, true); err != nil {
		return InstanceTransition{}, err
	}
	connection, ok := findConnection(def, input.ConnectionID)
	if !ok || !validID(input.DestinationAreaID) {
		return InstanceTransition{}, fmt.Errorf("%w: connection or destination does not exist", ErrInvalidInstanceAction)
	}
	if !ExploreConnectionAvailable(def, snapshot, input.ConnectionID, input.DestinationAreaID) {
		return InstanceTransition{}, fmt.Errorf("%w: exploration connection is unavailable", ErrActionNotAllowed)
	}
	from, to := snapshot.CurrentAreaID, input.DestinationAreaID
	if !((connection.FromAreaID == from && connection.ToAreaID == to) || (connection.Bidirectional && connection.ToAreaID == from && connection.FromAreaID == to)) {
		return InstanceTransition{}, fmt.Errorf("%w: destination is not reachable through the selected directed connection", ErrInvalidInstanceAction)
	}
	after := cloneInstanceSnapshot(snapshot)
	if err := applyDiscovery(&after, def, InstanceDiscovery{Kind: DiscoveryConnection, ID: connection.ID, SourceRef: ctx.Permit.SourceRef}); err != nil {
		return InstanceTransition{}, err
	}
	if err := applyDiscovery(&after, def, InstanceDiscovery{Kind: DiscoveryArea, ID: to, SourceRef: ctx.Permit.SourceRef}); err != nil {
		return InstanceTransition{}, err
	}
	after.CurrentAreaID = to
	return finishInstanceTransition(snapshot, after, ctx.Permit.SourceRef)
}

// ExitDungeonInstance transitions outside only from the actual current legal
// exit, retaining the last exit as history and clearing current area.
func ExitDungeonInstance(def DungeonDefinition, scope DungeonScope, snapshot DungeonInstanceSnapshot, ctx InstanceActionContext) (InstanceTransition, error) {
	if snapshot.Presence != InstanceInside || !containsString(def.ExitAreaIDs, snapshot.CurrentAreaID) {
		return InstanceTransition{}, fmt.Errorf("%w: exit is only legal from the current defined exit area", ErrInvalidInstanceAction)
	}
	if err := validateActionContext(def, scope, snapshot, ctx, ActionExit, snapshot.CurrentAreaID, "", "", true); err != nil {
		return InstanceTransition{}, err
	}
	after := cloneInstanceSnapshot(snapshot)
	after.Presence = InstanceOutside
	after.LastExitAreaID = snapshot.CurrentAreaID
	after.CurrentAreaID = ""
	return finishInstanceTransition(snapshot, after, ctx.Permit.SourceRef)
}

// ReenterDungeonInstance starts at the supplied legal arrival entry and keeps
// the same instance history; it never restores the previous deep position.
func ReenterDungeonInstance(def DungeonDefinition, scope DungeonScope, snapshot DungeonInstanceSnapshot, ctx InstanceActionContext, entryAreaID string) (InstanceTransition, error) {
	if snapshot.Presence != InstanceOutside {
		return InstanceTransition{}, fmt.Errorf("%w: re-entry requires outside presence", ErrInvalidInstanceAction)
	}
	if !containsString(def.EntryAreaIDs, entryAreaID) {
		return InstanceTransition{}, fmt.Errorf("%w: arrival area is not a defined entry", ErrInvalidInstanceAction)
	}
	if err := validateActionContext(def, scope, snapshot, ctx, ActionReenter, entryAreaID, "", "", true); err != nil {
		return InstanceTransition{}, err
	}
	after := cloneInstanceSnapshot(snapshot)
	if err := applyDiscovery(&after, def, InstanceDiscovery{Kind: DiscoveryArea, ID: entryAreaID, SourceRef: ctx.Permit.SourceRef}); err != nil {
		return InstanceTransition{}, err
	}
	after.Presence = InstanceInside
	after.CurrentAreaID = entryAreaID
	return finishInstanceTransition(snapshot, after, ctx.Permit.SourceRef)
}

// ProjectDungeonInstanceView verifies the binding before using the existing
// known-view projector. Outside views expose history with no current area.
func ProjectDungeonInstanceView(def DungeonDefinition, scope DungeonScope, snapshot DungeonInstanceSnapshot) (DungeonInstanceView, []Diagnostic) {
	empty := emptyInstanceView(snapshot.Binding.InstanceID, snapshot.Presence)
	if diagnostics := ValidateDungeonInstance(def, scope, snapshot); len(diagnostics) > 0 {
		return empty, diagnostics
	}
	projectionArea := snapshot.CurrentAreaID
	if snapshot.Presence == InstanceOutside {
		projectionArea = snapshot.LastExitAreaID
	}
	known := KnowledgeSnapshot{CurrentAreaID: projectionArea, KnownObjectiveIDs: []string{}, DiscoveredAreaIDs: []string{}, DiscoveredConnectionIDs: []string{}, ConfirmedFacts: []KnownFact{}, Rumors: []KnownFact{}}
	for _, item := range snapshot.Discoveries {
		switch item.Kind {
		case DiscoveryArea:
			known.DiscoveredAreaIDs = append(known.DiscoveredAreaIDs, item.ID)
		case DiscoveryConnection:
			known.DiscoveredConnectionIDs = append(known.DiscoveredConnectionIDs, item.ID)
		case DiscoveryObjective:
			known.KnownObjectiveIDs = append(known.KnownObjectiveIDs, item.ID)
		case DiscoveryFact:
			known.ConfirmedFacts = append(known.ConfirmedFacts, KnownFact{ID: item.ID, Text: item.Text, TextKey: item.TextKey, PubliclyDisplayable: item.PubliclyDisplayable})
		case DiscoveryRumor:
			known.Rumors = append(known.Rumors, KnownFact{ID: item.ID, Text: item.Text, TextKey: item.TextKey, PubliclyDisplayable: item.PubliclyDisplayable})
		}
	}
	projected, diagnostics := ProjectBoundKnownView(def, snapshot.Binding, scope, known)
	if len(diagnostics) > 0 {
		return empty, diagnostics
	}
	view := emptyInstanceView(snapshot.Binding.InstanceID, snapshot.Presence)
	view.LastExitAreaID = snapshot.LastExitAreaID
	if snapshot.Presence == InstanceInside {
		current := projected.CurrentArea
		view.CurrentArea = &current
	}
	view.Objectives = projected.Objectives
	view.Areas = projected.Areas
	view.Connections = projected.Connections
	view.ConfirmedFacts = projected.ConfirmedFacts
	view.Rumors = projected.Rumors
	knownObjectives := map[string]bool{}
	knownAreas := map[string]bool{}
	for _, objective := range projected.Objectives {
		knownObjectives[objective.ID] = true
	}
	for _, area := range projected.Areas {
		knownAreas[area.ID] = true
	}
	for _, progress := range snapshot.CompletedObjectives {
		if knownObjectives[progress.ID] {
			view.CompletedObjectives = append(view.CompletedObjectives, progress.ID)
		}
	}
	for _, progress := range snapshot.ResolvedChallenges {
		challenge := findChallenge(def, progress.ID)
		if knownAreas[challenge.AreaID] {
			view.ResolvedChallenges = append(view.ResolvedChallenges, progress.ID)
		}
	}
	view.GrantedRewards = append(view.GrantedRewards, snapshot.GrantedRewards...)
	sort.Strings(view.CompletedObjectives)
	sort.Strings(view.ResolvedChallenges)
	return view, nil
}

func validateActionContext(def DungeonDefinition, scope DungeonScope, snapshot DungeonInstanceSnapshot, ctx InstanceActionContext, action InstanceActionKind, targetID, fromAreaID, toAreaID string, requireClearScene bool) error {
	if diagnostics := ValidateDungeonInstance(def, scope, snapshot); len(diagnostics) > 0 {
		return instanceDiagnosticError(diagnostics)
	}
	if ctx.ExpectedRevision != snapshot.Revision {
		return fmt.Errorf("%w: expected %d, current %d", ErrStaleInstanceRevision, ctx.ExpectedRevision, snapshot.Revision)
	}
	if ctx.Scope != scope {
		return fmt.Errorf("%w: action scope does not match the trusted current scope", ErrInvalidInstanceAction)
	}
	if err := validatePermit(ctx.Permit, action, targetID, fromAreaID, toAreaID); err != nil {
		return err
	}
	expectedPermitBinding := InstancePermitBinding{InstanceID: snapshot.Binding.InstanceID, WorldID: scope.WorldID, BranchID: scope.BranchID, DefinitionHash: snapshot.DefinitionHash, Revision: snapshot.Revision}
	if err := validatePermitBinding(ctx.Permit, expectedPermitBinding); err != nil {
		return err
	}
	if requireClearScene {
		return validateClearScene(ctx.Battle, ctx.Blocking)
	}
	return nil
}

func validatePermitBinding(permit InstanceActionPermit, expected InstancePermitBinding) error {
	actual := permit.Binding
	if actual.InstanceID != expected.InstanceID || actual.WorldID != expected.WorldID || actual.BranchID != expected.BranchID || actual.DefinitionHash != expected.DefinitionHash {
		return fmt.Errorf("%w: permit instance, world, branch, or definition hash does not match", ErrInvalidBinding)
	}
	if actual.Revision != expected.Revision {
		return fmt.Errorf("%w: permit was issued for revision %d, current revision is %d", ErrStaleInstanceRevision, actual.Revision, expected.Revision)
	}
	return nil
}

func validatePermit(permit InstanceActionPermit, action InstanceActionKind, targetID, fromAreaID, toAreaID string) error {
	if permit.Decision == DecisionBlocked {
		return ErrActionNotAllowed
	}
	if permit.Decision != DecisionAllowed || permit.Action != action || permit.TargetID != targetID || permit.FromAreaID != fromAreaID || permit.ToAreaID != toAreaID || !validID(permit.SourceRef) {
		return fmt.Errorf("%w: permission, source reference, action, or exact target is missing or mismatched", ErrInvalidInstanceAction)
	}
	if !validID(permit.TargetID) {
		return fmt.Errorf("%w: target ID must be valid", ErrInvalidInstanceAction)
	}
	if fromAreaID != "" && !validID(permit.FromAreaID) || toAreaID != "" && !validID(permit.ToAreaID) {
		return fmt.Errorf("%w: directed target area is invalid", ErrInvalidInstanceAction)
	}
	return nil
}

func validateClearScene(battle BattleState, blocking BlockingState) error {
	if battle == BattleActive || blocking == BlockingPending {
		return ErrActionBlocked
	}
	if battle != BattleNone || blocking != BlockingNone {
		return fmt.Errorf("%w: battle and blocking state must be explicitly clear", ErrInvalidInstanceAction)
	}
	return nil
}

func applyInitialDiscoveries(snapshot *DungeonInstanceSnapshot, def DungeonDefinition, items []InstanceDiscovery) error {
	for _, item := range items {
		if !validID(item.SourceRef) {
			return fmt.Errorf("%w: initial discovery source reference is required", ErrInvalidInstanceAction)
		}
		if err := applyDiscovery(snapshot, def, item); err != nil {
			return err
		}
	}
	sortDiscoveries(snapshot.Discoveries)
	return nil
}

func applyDiscovery(snapshot *DungeonInstanceSnapshot, def DungeonDefinition, item InstanceDiscovery) error {
	if !validID(item.ID) || !validID(item.SourceRef) {
		return fmt.Errorf("%w: discovery requires stable ID and source reference", ErrInvalidInstanceAction)
	}
	if err := validateDiscoveryReference(def, item); err != nil {
		return err
	}
	for i, prior := range snapshot.Discoveries {
		if prior.ID != item.ID || (prior.Kind != item.Kind && !(isFactKind(prior.Kind) && isFactKind(item.Kind))) {
			continue
		}
		if isFactKind(prior.Kind) && isFactKind(item.Kind) {
			if prior.Text != item.Text || prior.TextKey != item.TextKey || prior.PubliclyDisplayable != item.PubliclyDisplayable {
				return fmt.Errorf("%w: fact %s has conflicting content", ErrInstanceConflict, item.ID)
			}
			if prior.Kind == DiscoveryRumor && item.Kind == DiscoveryFact {
				snapshot.Discoveries[i] = item
				sortDiscoveries(snapshot.Discoveries)
			}
			return nil
		}
		return nil
	}
	if len(snapshot.Discoveries) >= MaxInstanceDiscoveries {
		return fmt.Errorf("%w: discovery capacity %d reached", ErrInstanceCapacity, MaxInstanceDiscoveries)
	}
	snapshot.Discoveries = append(snapshot.Discoveries, item)
	sortDiscoveries(snapshot.Discoveries)
	return nil
}

func validateDiscoveryReference(def DungeonDefinition, item InstanceDiscovery) error {
	switch item.Kind {
	case DiscoveryArea:
		if !hasArea(def, item.ID) || item.Text != "" || item.TextKey != "" || item.PubliclyDisplayable {
			return fmt.Errorf("%w: invalid area discovery", ErrInvalidInstanceAction)
		}
	case DiscoveryConnection:
		if !hasConnection(def, item.ID) || item.Text != "" || item.TextKey != "" || item.PubliclyDisplayable {
			return fmt.Errorf("%w: invalid connection discovery", ErrInvalidInstanceAction)
		}
	case DiscoveryObjective:
		if !hasObjective(def, item.ID) || item.Text != "" || item.TextKey != "" || item.PubliclyDisplayable {
			return fmt.Errorf("%w: invalid objective discovery", ErrInvalidInstanceAction)
		}
	case DiscoveryFact, DiscoveryRumor:
		if !validText(item.Text, MaxDescriptionBytes) {
			return fmt.Errorf("%w: fact text must be non-empty and bounded", ErrInvalidInstanceAction)
		}
	default:
		return fmt.Errorf("%w: discovery kind is unknown", ErrInvalidInstanceAction)
	}
	return nil
}

func appendProgress(items []SourcedProgress, id, sourceRef string) ([]SourcedProgress, error) {
	for _, prior := range items {
		if prior.ID != id {
			continue
		}
		if prior.SourceRef != sourceRef {
			return items, fmt.Errorf("%w: progress %s has a conflicting source", ErrInstanceConflict, id)
		}
		return items, nil
	}
	if len(items) >= MaxInstanceProgressRecords {
		return items, fmt.Errorf("%w: progress capacity %d reached", ErrInstanceCapacity, MaxInstanceProgressRecords)
	}
	items = append(items, SourcedProgress{ID: id, SourceRef: sourceRef})
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}
