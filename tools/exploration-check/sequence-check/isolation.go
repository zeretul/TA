package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"ta/internal/exploration"
)

type isolationScenarioReport struct {
	InstanceID             string                   `json:"shared_instance_id"`
	ScopeA                 exploration.DungeonScope `json:"scope_a"`
	ScopeB                 exploration.DungeonScope `json:"scope_b"`
	RevisionA              uint64                   `json:"revision_a"`
	RevisionB              uint64                   `json:"revision_b"`
	AStableWhileBChanged   bool                     `json:"a_unchanged_while_b_changed"`
	BStableWhileAChanged   bool                     `json:"b_unchanged_while_a_changed"`
	CrossPermitRejected    bool                     `json:"cross_scope_permit_rejected"`
	CrossScopeRejected     bool                     `json:"wrong_scope_projection_rejected"`
	CrossSnapshotRejected  bool                     `json:"wrong_snapshot_projection_rejected"`
	CrossPermitError       string                   `json:"cross_permit_error"`
	ScopeDiagnosticCode    string                   `json:"scope_diagnostic_code"`
	SnapshotDiagnosticCode string                   `json:"snapshot_diagnostic_code"`
}

func runIsolationScenario(def exploration.DungeonDefinition, report *suiteReport) isolationScenarioReport {
	instanceID := "archive.shared"
	scopeA := exploration.DungeonScope{WorldID: "world.north", BranchID: "branch.alpha"}
	scopeB := exploration.DungeonScope{WorldID: "world.south", BranchID: "branch.beta"}
	aCreated, errA := createInstance(def, scopeA, instanceID, "entrance", "source.entry.shared")
	bCreated, errB := createInstance(def, scopeB, instanceID, "entrance", "source.entry.shared")
	if errA != nil || errB != nil {
		recordCheck(report, "scope_instances_created", false, "two valid snapshots with the same instance ID and distinct bindings", fmt.Sprintf("a=%v b=%v", errA, errB))
		return isolationScenarioReport{InstanceID: instanceID, ScopeA: scopeA, ScopeB: scopeB}
	}
	a, b := aCreated.Snapshot, bCreated.Snapshot
	recordCheck(report, "scope_instances_created", a.Binding.InstanceID == b.Binding.InstanceID && a.Binding.WorldID != b.Binding.WorldID && a.Binding.BranchID != b.Binding.BranchID, "same instance ID, distinct valid world and branch bindings", fmt.Sprintf("a=%s/%s b=%s/%s", a.Binding.WorldID, a.Binding.BranchID, b.Binding.WorldID, b.Binding.BranchID))

	bBeforeA, _ := json.Marshal(b)
	bViewBeforeA, bViewErr := projectJSON(def, scopeB, b)
	if bViewErr != nil {
		recordCheck(report, "scope_b_projection", false, "B projects under B scope", bViewErr.Error())
	}
	var aErr error
	a, aErr = evolveInstance(def, scopeA, a, "branch.alpha")
	if aErr != nil {
		recordCheck(report, "scope_a_evolution", false, "A discovery, progress, and movement succeed", aErr.Error())
		return isolationScenarioReport{InstanceID: instanceID, ScopeA: scopeA, ScopeB: scopeB}
	}
	bAfterA, _ := json.Marshal(b)
	bViewAfterA, bViewErr := projectJSON(def, scopeB, b)
	bStable := string(bBeforeA) == string(bAfterA) && bViewErr == nil && string(bViewBeforeA) == string(bViewAfterA)
	recordCheck(report, "scope_b_immutable_during_a", bStable, "B snapshot and projected view remain byte-identical while A evolves", fmt.Sprintf("snapshot_before=%s snapshot_after=%s view_equal=%t", digest(bBeforeA), digest(bAfterA), string(bViewBeforeA) == string(bViewAfterA)))

	aBeforeB, _ := json.Marshal(a)
	aViewBeforeB, aViewErr := projectJSON(def, scopeA, a)
	if aViewErr != nil {
		recordCheck(report, "scope_a_projection", false, "A projects under A scope", aViewErr.Error())
	}
	b, errB = evolveInstance(def, scopeB, b, "branch.beta")
	if errB != nil {
		recordCheck(report, "scope_b_evolution", false, "B discovery, progress, and movement succeed", errB.Error())
		return isolationScenarioReport{InstanceID: instanceID, ScopeA: scopeA, ScopeB: scopeB, BStableWhileAChanged: bStable}
	}
	aAfterB, _ := json.Marshal(a)
	aViewAfterB, aViewErr := projectJSON(def, scopeA, a)
	aStable := string(aBeforeB) == string(aAfterB) && aViewErr == nil && string(aViewBeforeB) == string(aViewAfterB)
	recordCheck(report, "scope_a_immutable_during_b", aStable, "A snapshot and projected view remain byte-identical while B evolves", fmt.Sprintf("snapshot_before=%s snapshot_after=%s view_equal=%t", digest(aBeforeB), digest(aAfterB), string(aViewBeforeB) == string(aViewAfterB)))

	crossPermit := actionPermit(def, scopeA, a, exploration.ActionMove, "outbound_ramp", "archive", "exit", "source.alpha.cross-attempt")
	bBeforeCross, _ := json.Marshal(b)
	_, crossErr := exploration.MoveDungeonInstance(def, scopeB, b, actionContext(scopeB, b, crossPermit), exploration.MoveDungeonInstanceInput{ConnectionID: "outbound_ramp", DestinationAreaID: "exit"})
	bAfterCross, _ := json.Marshal(b)
	crossPermitRejected := errors.Is(crossErr, exploration.ErrInvalidBinding) && string(bBeforeCross) == string(bAfterCross)
	recordCheck(report, "cross_scope_permit_rejected", crossPermitRejected, "A-bound permit is rejected against B and B remains unchanged", fmt.Sprintf("error=%v unchanged=%t", crossErr, string(bBeforeCross) == string(bAfterCross)))

	_, scopeDiagnostics := exploration.ProjectDungeonInstanceView(def, scopeA, b)
	_, snapshotDiagnostics := exploration.ProjectDungeonInstanceView(def, scopeB, a)
	scopeRejected := len(scopeDiagnostics) > 0 && len(snapshotDiagnostics) > 0
	recordCheck(report, "cross_scope_snapshot_rejected", scopeRejected, "B snapshot under A scope and A snapshot under B scope are rejected", fmt.Sprintf("a_scope_diagnostics=%v b_scope_diagnostics=%v", scopeDiagnostics, snapshotDiagnostics))
	return isolationScenarioReport{InstanceID: instanceID, ScopeA: scopeA, ScopeB: scopeB, RevisionA: a.Revision, RevisionB: b.Revision,
		AStableWhileBChanged: aStable, BStableWhileAChanged: bStable, CrossPermitRejected: crossPermitRejected, CrossScopeRejected: len(scopeDiagnostics) > 0,
		CrossSnapshotRejected: len(snapshotDiagnostics) > 0, CrossPermitError: errorText(crossErr), ScopeDiagnosticCode: firstDiagnostic(scopeDiagnostics), SnapshotDiagnosticCode: firstDiagnostic(snapshotDiagnostics)}
}

func evolveInstance(def exploration.DungeonDefinition, scope exploration.DungeonScope, snapshot exploration.DungeonInstanceSnapshot, suffix string) (exploration.DungeonInstanceSnapshot, error) {
	fact := exploration.InstanceDiscovery{Kind: exploration.DiscoveryFact, ID: "shared.marker", SourceRef: "source." + suffix + ".marker", Text: "A carved marker is visible.", PubliclyDisplayable: true}
	transition, err := exploration.RegisterInstanceDiscovery(def, scope, snapshot, actionContext(scope, snapshot, actionPermit(def, scope, snapshot, exploration.ActionDiscover, fact.ID, "", "", fact.SourceRef)), fact)
	if err != nil {
		return snapshot, err
	}
	snapshot = transition.Snapshot
	progress := exploration.InstanceProgress{Kind: exploration.ProgressObjective, ID: "recover.inscription", SourceRef: "source." + suffix + ".objective", Outcome: exploration.OutcomeConfirmed}
	transition, err = exploration.RecordInstanceProgress(def, scope, snapshot, actionContext(scope, snapshot, actionPermit(def, scope, snapshot, exploration.ActionProgress, progress.ID, "", "", progress.SourceRef)), progress)
	if err != nil {
		return snapshot, err
	}
	snapshot = transition.Snapshot
	transition, err = exploration.MoveDungeonInstance(def, scope, snapshot, actionContext(scope, snapshot, actionPermit(def, scope, snapshot, exploration.ActionMove, "entry_passage", "entrance", "archive", "source."+suffix+".move")), exploration.MoveDungeonInstanceInput{ConnectionID: "entry_passage", DestinationAreaID: "archive"})
	if err != nil {
		return snapshot, err
	}
	return transition.Snapshot, nil
}

func projectJSON(def exploration.DungeonDefinition, scope exploration.DungeonScope, snapshot exploration.DungeonInstanceSnapshot) ([]byte, error) {
	view, diagnostics := exploration.ProjectDungeonInstanceView(def, scope, snapshot)
	if len(diagnostics) != 0 {
		return nil, fmt.Errorf("projection diagnostics: %v", diagnostics)
	}
	return json.Marshal(view)
}

func firstDiagnostic(items []exploration.Diagnostic) string {
	if len(items) == 0 {
		return ""
	}
	return items[0].Code
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
