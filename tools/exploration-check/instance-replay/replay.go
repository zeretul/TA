package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"ta/internal/exploration"
)

type replayStep struct {
	Action                string                   `json:"action"`
	Revision              uint64                   `json:"revision"`
	Presence              string                   `json:"presence"`
	CurrentAreaID         string                   `json:"current_area_id,omitempty"`
	LastExitAreaID        string                   `json:"last_exit_area_id,omitempty"`
	KnownAreaIDs          []string                 `json:"known_area_ids"`
	KnownConnectionIDs    []string                 `json:"known_connection_ids"`
	CompletedObjectiveIDs []string                 `json:"completed_objective_ids"`
	ResolvedChallengeIDs  []string                 `json:"resolved_challenge_ids"`
	Diff                  exploration.InstanceDiff `json:"diff"`
}

type expectedRejection struct {
	Case            string   `json:"case"`
	Rejected        bool     `json:"rejected"`
	InputUnchanged  bool     `json:"input_unchanged"`
	ExpectedError   string   `json:"expected_error,omitempty"`
	ObservedError   string   `json:"observed_error,omitempty"`
	DiagnosticCodes []string `json:"diagnostic_codes,omitempty"`
}

type replayReport struct {
	SchemaVersion      int                                 `json:"schema_version"`
	Contract           string                              `json:"contract"`
	SyntheticOnly      bool                                `json:"synthetic_only"`
	NotInstallable     bool                                `json:"not_installable"`
	Valid              bool                                `json:"valid"`
	InputPath          string                              `json:"input_path"`
	InputSHA256        string                              `json:"input_sha256"`
	DefinitionHash     string                              `json:"definition_hash,omitempty"`
	Playable           bool                                `json:"playable"`
	Steps              []replayStep                        `json:"steps"`
	ExpectedRejections []expectedRejection                 `json:"expected_rejections"`
	FinalSnapshot      exploration.DungeonInstanceSnapshot `json:"final_snapshot,omitempty"`
	Diagnostics        []exploration.Diagnostic            `json:"diagnostics"`
}

func newReplayReport(inputPath string, input []byte) replayReport {
	sum := sha256.Sum256(input)
	return replayReport{SchemaVersion: 1, Contract: "denova synthetic dungeon instance proposal v1", SyntheticOnly: true, NotInstallable: true, Valid: true,
		InputPath: inputPath, InputSHA256: hex.EncodeToString(sum[:]), Steps: []replayStep{}, ExpectedRejections: []expectedRejection{}, Diagnostics: []exploration.Diagnostic{}}
}

func executeReplay(def exploration.DungeonDefinition, inputPath string, input []byte) replayReport {
	report := newReplayReport(inputPath, input)
	validation := exploration.ValidateDungeon(def, exploration.Catalog{}, nil)
	report.Playable = validation.Playable
	if len(validation.Diagnostics) != 0 || !validation.Playable {
		report.Valid = false
		report.Diagnostics = append(report.Diagnostics, validation.Diagnostics...)
		if len(validation.Diagnostics) == 0 {
			report.Diagnostics = append(report.Diagnostics, exploration.Diagnostic{Code: "definition_unplayable", Path: "definition", Message: "synthetic definition has no structurally available completion path"})
		}
		return report
	}
	hash, err := exploration.DungeonDefinitionContentHash(def)
	if err != nil {
		return failReplay(report, "definition_hash_failed", "definition", err)
	}
	report.DefinitionHash = hash
	scope := exploration.DungeonScope{WorldID: "synthetic.world", BranchID: "synthetic.branch"}
	created, err := exploration.CreateDungeonInstance(exploration.CreateDungeonInstanceInput{Definition: def, Scope: scope, InstanceID: "synthetic.instance.01", EntryAreaID: "entrance",
		Permit: boundPermit(def, scope, "synthetic.instance.01", 0, permit(exploration.ActionEnter, "entrance", "", "", "result.entry-allowed")), Battle: exploration.BattleNone, Blocking: exploration.BlockingNone,
		InitialDiscoveries: []exploration.InstanceDiscovery{{Kind: exploration.DiscoveryObjective, ID: "recover.inscription", SourceRef: "result.briefing-objective"}}})
	if err != nil {
		return failReplay(report, "instance_create_failed", "create", err)
	}
	snapshot := created.Snapshot
	var transition exploration.InstanceTransition
	if err := appendReplayStep(&report, def, scope, "enter_at_legal_entry", snapshot, created.Diff); err != nil {
		return failReplay(report, "initial_projection_failed", "steps[0]", err)
	}
	if len(report.Steps[0].KnownAreaIDs) != 1 || report.Steps[0].KnownAreaIDs[0] != "entrance" || len(report.Steps[0].KnownConnectionIDs) != 0 {
		return failReplay(report, "initial_view_leaked", "steps[0]", errors.New("initial projection did not limit view to the entry"))
	}

	transition, err = discover(def, scope, snapshot, exploration.DiscoveryConnection, "entry_passage", "result.inspect-passage", "", false)
	if err != nil {
		return failReplay(report, "passage_discovery_failed", "discover.entry_passage", err)
	}
	snapshot = transition.Snapshot
	if err := appendReplayStep(&report, def, scope, "discover_one_connection_only", snapshot, transition.Diff); err != nil {
		return failReplay(report, "passage_projection_failed", "steps", err)
	}
	if len(report.Steps[len(report.Steps)-1].KnownConnectionIDs) != 0 {
		return failReplay(report, "connection_endpoints_leaked", "steps", errors.New("edge was shown before its destination was known"))
	}

	transition, err = discover(def, scope, snapshot, exploration.DiscoveryArea, "archive", "result.reach-archive-sightline", "", false)
	if err != nil {
		return failReplay(report, "area_discovery_failed", "discover.archive", err)
	}
	snapshot = transition.Snapshot
	if err := appendReplayStep(&report, def, scope, "discover_confirmed_area", snapshot, transition.Diff); err != nil {
		return failReplay(report, "area_projection_failed", "steps", err)
	}
	if contains(report.Steps[len(report.Steps)-1].KnownAreaIDs, "sealed_annex") || contains(report.Steps[len(report.Steps)-1].KnownConnectionIDs, "sealed_door") {
		return failReplay(report, "hidden_definition_leaked", "steps", errors.New("projection exposed an undiscovered hidden area or edge"))
	}

	transition, err = move(def, scope, snapshot, "entry_passage", "archive", "result.move-archive")
	if err != nil {
		return failReplay(report, "movement_failed", "move.entry_passage", err)
	}
	snapshot = transition.Snapshot
	if err := appendReplayStep(&report, def, scope, "move_over_confirmed_connection", snapshot, transition.Diff); err != nil {
		return failReplay(report, "movement_projection_failed", "steps", err)
	}
	transition, err = move(def, scope, snapshot, "entry_passage", "entrance", "result.return-entrance")
	if err != nil {
		return failReplay(report, "return_movement_failed", "move.return-entrance", err)
	}
	snapshot = transition.Snapshot
	if err := appendReplayStep(&report, def, scope, "return_to_entry_over_confirmed_connection", snapshot, transition.Diff); err != nil {
		return failReplay(report, "return_movement_projection_failed", "steps", err)
	}
	if !hasChangeSource(transition.Diff, "position_changed", "result.return-entrance") {
		return failReplay(report, "return_source_missing", "move.return-entrance", errors.New("return position change lost its action source"))
	}
	transition, err = move(def, scope, snapshot, "entry_passage", "archive", "result.resume-archive")
	if err != nil {
		return failReplay(report, "resume_movement_failed", "move.resume-archive", err)
	}
	snapshot = transition.Snapshot
	if err := appendReplayStep(&report, def, scope, "resume_to_archive", snapshot, transition.Diff); err != nil {
		return failReplay(report, "resume_movement_projection_failed", "steps", err)
	}

	transition, err = progress(def, scope, snapshot, exploration.InstanceProgress{Kind: exploration.ProgressObjective, ID: "recover.inscription", SourceRef: "result.inscription-recovered", Outcome: exploration.OutcomeConfirmed})
	if err != nil {
		return failReplay(report, "objective_progress_failed", "progress.recover.inscription", err)
	}
	snapshot = transition.Snapshot
	if err := appendReplayStep(&report, def, scope, "record_confirmed_objective", snapshot, transition.Diff); err != nil {
		return failReplay(report, "progress_projection_failed", "steps", err)
	}
	transition, err = progress(def, scope, snapshot, exploration.InstanceProgress{Kind: exploration.ProgressChallenge, ID: "archive.seal", SourceRef: "result.seal-resolved", Outcome: exploration.OutcomeConfirmed})
	if err != nil {
		return failReplay(report, "challenge_progress_failed", "progress.archive.seal", err)
	}
	snapshot = transition.Snapshot
	if err := appendReplayStep(&report, def, scope, "record_confirmed_challenge", snapshot, transition.Diff); err != nil {
		return failReplay(report, "challenge_projection_failed", "steps", err)
	}

	transition, err = move(def, scope, snapshot, "outbound_ramp", "exit", "result.move-exit")
	if err != nil {
		return failReplay(report, "exit_path_movement_failed", "move.outbound_ramp", err)
	}
	snapshot = transition.Snapshot
	if err := appendReplayStep(&report, def, scope, "follow_directed_path_to_exit", snapshot, transition.Diff); err != nil {
		return failReplay(report, "exit_path_projection_failed", "steps", err)
	}
	exitCtx := context(snapshot, scope, permit(exploration.ActionExit, "exit", "", "", "result.exit-allowed"))
	transition, err = exploration.ExitDungeonInstance(def, scope, snapshot, exitCtx)
	if err != nil {
		return failReplay(report, "exit_failed", "exit", err)
	}
	snapshot = transition.Snapshot
	if err := appendReplayStep(&report, def, scope, "leave_at_legal_exit", snapshot, transition.Diff); err != nil {
		return failReplay(report, "outside_projection_failed", "steps", err)
	}
	outsideView, ds := exploration.ProjectDungeonInstanceView(def, scope, snapshot)
	if len(ds) != 0 || outsideView.Presence != exploration.InstanceOutside || outsideView.CurrentArea != nil || outsideView.LastExitAreaID != "exit" || len(outsideView.Areas) != 3 {
		return failReplay(report, "outside_view_invalid", "outside_view", errors.New("outside view must preserve history without claiming an interior position"))
	}

	reenterCtx := context(snapshot, scope, permit(exploration.ActionReenter, "entrance", "", "", "result.reentry-allowed"))
	transition, err = exploration.ReenterDungeonInstance(def, scope, snapshot, reenterCtx, "entrance")
	if err != nil {
		return failReplay(report, "reentry_failed", "reenter", err)
	}
	if !reflect.DeepEqual(snapshot.Discoveries, transition.Snapshot.Discoveries) || !reflect.DeepEqual(snapshot.CompletedObjectives, transition.Snapshot.CompletedObjectives) || !reflect.DeepEqual(snapshot.ResolvedChallenges, transition.Snapshot.ResolvedChallenges) {
		return failReplay(report, "reentry_reset_history", "reenter", errors.New("re-entry changed prior discoveries or confirmed progress"))
	}
	snapshot = transition.Snapshot
	if err := appendReplayStep(&report, def, scope, "reenter_at_actual_arrival_entry", snapshot, transition.Diff); err != nil {
		return failReplay(report, "reentry_projection_failed", "steps", err)
	}
	if snapshot.CurrentAreaID != "entrance" || snapshot.LastExitAreaID != "exit" || contains(report.Steps[len(report.Steps)-1].KnownAreaIDs, "sealed_annex") {
		return failReplay(report, "reentry_position_invalid", "reenter", errors.New("re-entry did not use the actual arrival entry or leaked hidden state"))
	}
	report.FinalSnapshot = snapshot
	report.ExpectedRejections = exerciseRejections(def, scope, created.Snapshot, snapshot)
	for _, result := range report.ExpectedRejections {
		if !result.Rejected || !result.InputUnchanged {
			report.Valid = false
			report.Diagnostics = append(report.Diagnostics, exploration.Diagnostic{Code: "expected_rejection_failed", Path: "expected_rejections." + result.Case, Message: "invalid action was not safely rejected"})
		}
	}
	return report
}

func exerciseRejections(def exploration.DungeonDefinition, scope exploration.DungeonScope, initial, final exploration.DungeonInstanceSnapshot) []expectedRejection {
	results := []expectedRejection{}
	initialCopy := cloneSnapshotForCheck(initial)
	foreignWorld := scope
	foreignWorld.WorldID = "synthetic.foreign-world"
	view, diagnostics := exploration.ProjectDungeonInstanceView(def, foreignWorld, initial)
	results = append(results, rejection("foreign_world", len(diagnostics) > 0 && len(view.Areas) == 0, reflect.DeepEqual(initialCopy, initial), diagnosticCodes(diagnostics), "world_scope_mismatch", ""))
	foreignBranch := scope
	foreignBranch.BranchID = "synthetic.foreign-branch"
	view, diagnostics = exploration.ProjectDungeonInstanceView(def, foreignBranch, initial)
	results = append(results, rejection("foreign_branch", len(diagnostics) > 0 && len(view.Areas) == 0, reflect.DeepEqual(initialCopy, initial), diagnosticCodes(diagnostics), "branch_scope_mismatch", ""))

	changed := def
	changed.Areas = append([]exploration.Area(nil), def.Areas...)
	changed.Areas[0].Description += " changed"
	_, err := move(changed, scope, initial, "entry_passage", "archive", "result.changed-definition")
	results = append(results, rejection("definition_content_replaced_same_version", errors.Is(err, exploration.ErrInvalidInstance), reflect.DeepEqual(initialCopy, initial), []string{}, "invalid dungeon instance snapshot", errorText(err)))

	before := cloneSnapshotForCheck(initial)
	badCtx := context(initial, scope, permit(exploration.ActionMove, "entry_passage", "entrance", "exit", "result.nonadjacent"))
	_, err = exploration.MoveDungeonInstance(def, scope, initial, badCtx, exploration.MoveDungeonInstanceInput{ConnectionID: "entry_passage", DestinationAreaID: "exit"})
	results = append(results, rejection("nonadjacent_destination", errors.Is(err, exploration.ErrInvalidInstanceAction), reflect.DeepEqual(before, initial), []string{}, "invalid dungeon instance action context", errorText(err)))

	missingCtx := exploration.InstanceActionContext{ExpectedRevision: initial.Revision, Scope: scope, Battle: exploration.BattleNone, Blocking: exploration.BlockingNone}
	_, err = exploration.MoveDungeonInstance(def, scope, initial, missingCtx, exploration.MoveDungeonInstanceInput{ConnectionID: "entry_passage", DestinationAreaID: "archive"})
	results = append(results, rejection("missing_permission", errors.Is(err, exploration.ErrInvalidInstanceAction), reflect.DeepEqual(before, initial), []string{}, "invalid dungeon instance action context", errorText(err)))

	activeCtx := context(initial, scope, permit(exploration.ActionMove, "entry_passage", "entrance", "archive", "result.active-battle"))
	activeCtx.Battle = exploration.BattleActive
	_, err = exploration.MoveDungeonInstance(def, scope, initial, activeCtx, exploration.MoveDungeonInstanceInput{ConnectionID: "entry_passage", DestinationAreaID: "archive"})
	results = append(results, rejection("active_battle", errors.Is(err, exploration.ErrActionBlocked), reflect.DeepEqual(before, initial), []string{}, "dungeon instance action blocked", errorText(err)))

	blockedCtx := context(initial, scope, permit(exploration.ActionMove, "entry_passage", "entrance", "archive", "result.pending-block"))
	blockedCtx.Blocking = exploration.BlockingPending
	_, err = exploration.MoveDungeonInstance(def, scope, initial, blockedCtx, exploration.MoveDungeonInstanceInput{ConnectionID: "entry_passage", DestinationAreaID: "archive"})
	results = append(results, rejection("unresolved_block", errors.Is(err, exploration.ErrActionBlocked), reflect.DeepEqual(before, initial), []string{}, "dungeon instance action blocked", errorText(err)))

	staleCtx := context(initial, scope, permit(exploration.ActionMove, "entry_passage", "entrance", "archive", "result.stale"))
	staleCtx.ExpectedRevision--
	_, err = exploration.MoveDungeonInstance(def, scope, initial, staleCtx, exploration.MoveDungeonInstanceInput{ConnectionID: "entry_passage", DestinationAreaID: "archive"})
	results = append(results, rejection("stale_revision", errors.Is(err, exploration.ErrStaleInstanceRevision), reflect.DeepEqual(before, initial), []string{}, "stale dungeon instance revision", errorText(err)))

	foreignInstanceID := "synthetic.instance.foreign"
	createForeignPermit := boundPermit(def, scope, foreignInstanceID, 0, permit(exploration.ActionEnter, "entrance", "", "", "result.foreign-create"))
	foreignInstance, createErr := exploration.CreateDungeonInstance(exploration.CreateDungeonInstanceInput{Definition: def, Scope: scope, InstanceID: foreignInstanceID, EntryAreaID: "entrance", Permit: createForeignPermit, Battle: exploration.BattleNone, Blocking: exploration.BlockingNone})
	if createErr == nil {
		oldPermitCtx := context(initial, scope, permit(exploration.ActionMove, "entry_passage", "entrance", "archive", "result.instance-reuse"))
		_, err = exploration.MoveDungeonInstance(def, scope, foreignInstance.Snapshot, oldPermitCtx, exploration.MoveDungeonInstanceInput{ConnectionID: "entry_passage", DestinationAreaID: "archive"})
		results = append(results, rejection("permit_reused_across_instances", errors.Is(err, exploration.ErrInvalidBinding), reflect.DeepEqual(before, initial), []string{}, "invalid dungeon instance binding", errorText(err)))
	} else {
		results = append(results, rejection("permit_reused_across_instances", false, false, []string{}, "invalid dungeon instance binding", "setup failed: "+createErr.Error()))
	}

	oldPermitCtx := context(initial, scope, permit(exploration.ActionMove, "entry_passage", "entrance", "archive", "result.rewrapped-stale"))
	advanceCtx := context(initial, scope, permit(exploration.ActionDiscover, "rewrap.bump", "", "", "result.rewrap-bump"))
	advanced, advanceErr := exploration.RegisterInstanceDiscovery(def, scope, initial, advanceCtx, exploration.InstanceDiscovery{Kind: exploration.DiscoveryFact, ID: "rewrap.bump", SourceRef: "result.rewrap-bump", Text: "revision bump", PubliclyDisplayable: true})
	if advanceErr == nil {
		rewrapped := oldPermitCtx
		rewrapped.ExpectedRevision = advanced.Snapshot.Revision
		advancedBefore := cloneSnapshotForCheck(advanced.Snapshot)
		_, err = exploration.MoveDungeonInstance(def, scope, advanced.Snapshot, rewrapped, exploration.MoveDungeonInstanceInput{ConnectionID: "entry_passage", DestinationAreaID: "archive"})
		results = append(results, rejection("old_permit_rewrapped_at_new_revision", errors.Is(err, exploration.ErrStaleInstanceRevision), reflect.DeepEqual(advancedBefore, advanced.Snapshot), []string{}, "stale dungeon instance revision", errorText(err)))
	} else {
		results = append(results, rejection("old_permit_rewrapped_at_new_revision", false, false, []string{}, "stale dungeon instance revision", "setup failed: "+advanceErr.Error()))
	}

	wrongExitCtx := context(initial, scope, permit(exploration.ActionExit, "entrance", "", "", "result.wrong-exit"))
	_, err = exploration.ExitDungeonInstance(def, scope, initial, wrongExitCtx)
	results = append(results, rejection("wrong_exit", errors.Is(err, exploration.ErrInvalidInstanceAction), reflect.DeepEqual(before, initial), []string{}, "invalid dungeon instance action context", errorText(err)))

	archiveTransition, archiveErr := move(def, scope, initial, "entry_passage", "archive", "result.archive")
	if archiveErr == nil {
		exitTransition, exitErr := move(def, scope, archiveTransition.Snapshot, "outbound_ramp", "exit", "result.exit")
		if exitErr == nil {
			reverseCtx := context(exitTransition.Snapshot, scope, permit(exploration.ActionMove, "outbound_ramp", "exit", "archive", "result.reverse-one-way"))
			_, err = exploration.MoveDungeonInstance(def, scope, exitTransition.Snapshot, reverseCtx, exploration.MoveDungeonInstanceInput{ConnectionID: "outbound_ramp", DestinationAreaID: "archive"})
			exitBefore := cloneSnapshotForCheck(exitTransition.Snapshot)
			results = append(results, rejection("reverse_one_way_edge", errors.Is(err, exploration.ErrInvalidInstanceAction), reflect.DeepEqual(exitBefore, exitTransition.Snapshot), []string{}, "invalid dungeon instance action context", errorText(err)))
		} else {
			results = append(results, rejection("reverse_one_way_edge", false, false, []string{}, "invalid dungeon instance action context", "setup failed: "+exitErr.Error()))
		}
	} else {
		results = append(results, rejection("reverse_one_way_edge", false, false, []string{}, "invalid dungeon instance action context", "setup failed: "+archiveErr.Error()))
	}

	rumor := exploration.InstanceDiscovery{Kind: exploration.DiscoveryRumor, ID: "conflict.note", SourceRef: "result.rumor", Text: "A seal might be intact.", PubliclyDisplayable: true}
	rumorCtx := context(final, scope, permit(exploration.ActionDiscover, rumor.ID, "", "", rumor.SourceRef))
	rumorState, rumorErr := exploration.RegisterInstanceDiscovery(def, scope, final, rumorCtx, rumor)
	conflict := exploration.InstanceDiscovery{Kind: exploration.DiscoveryFact, ID: rumor.ID, SourceRef: "result.conflicting-fact", Text: "The seal is broken.", PubliclyDisplayable: true}
	if rumorErr == nil {
		conflictCtx := context(rumorState.Snapshot, scope, permit(exploration.ActionDiscover, conflict.ID, "", "", conflict.SourceRef))
		beforeConflict := cloneSnapshotForCheck(rumorState.Snapshot)
		_, err = exploration.RegisterInstanceDiscovery(def, scope, rumorState.Snapshot, conflictCtx, conflict)
		results = append(results, rejection("conflicting_fact", errors.Is(err, exploration.ErrInstanceConflict), reflect.DeepEqual(beforeConflict, rumorState.Snapshot), []string{}, "conflicting dungeon instance fact", errorText(err)))
	} else {
		results = append(results, rejection("conflicting_fact", false, false, []string{}, "conflicting dungeon instance fact", "setup failed: "+rumorErr.Error()))
	}
	return results
}

func rejection(name string, rejected, unchanged bool, codes []string, expected, observed string) expectedRejection {
	return expectedRejection{Case: name, Rejected: rejected, InputUnchanged: unchanged, ExpectedError: expected, ObservedError: observed, DiagnosticCodes: codes}
}

func appendReplayStep(report *replayReport, def exploration.DungeonDefinition, scope exploration.DungeonScope, action string, snapshot exploration.DungeonInstanceSnapshot, diff exploration.InstanceDiff) error {
	view, diagnostics := exploration.ProjectDungeonInstanceView(def, scope, snapshot)
	if len(diagnostics) > 0 {
		return fmt.Errorf("projection diagnostics: %v", diagnostics)
	}
	step := replayStep{Action: action, Revision: snapshot.Revision, Presence: string(snapshot.Presence), CurrentAreaID: snapshot.CurrentAreaID, LastExitAreaID: snapshot.LastExitAreaID,
		KnownAreaIDs: []string{}, KnownConnectionIDs: []string{}, CompletedObjectiveIDs: view.CompletedObjectives, ResolvedChallengeIDs: view.ResolvedChallenges, Diff: diff}
	for _, area := range view.Areas {
		step.KnownAreaIDs = append(step.KnownAreaIDs, area.ID)
	}
	for _, connection := range view.Connections {
		step.KnownConnectionIDs = append(step.KnownConnectionIDs, connection.ID)
	}
	report.Steps = append(report.Steps, step)
	return nil
}

func discover(def exploration.DungeonDefinition, scope exploration.DungeonScope, snapshot exploration.DungeonInstanceSnapshot, kind exploration.InstanceDiscoveryKind, id, sourceRef, text string, public bool) (exploration.InstanceTransition, error) {
	ctx := context(snapshot, scope, permit(exploration.ActionDiscover, id, "", "", sourceRef))
	return exploration.RegisterInstanceDiscovery(def, scope, snapshot, ctx, exploration.InstanceDiscovery{Kind: kind, ID: id, SourceRef: sourceRef, Text: text, PubliclyDisplayable: public})
}

func move(def exploration.DungeonDefinition, scope exploration.DungeonScope, snapshot exploration.DungeonInstanceSnapshot, connectionID, toAreaID, sourceRef string) (exploration.InstanceTransition, error) {
	permit := permit(exploration.ActionMove, connectionID, snapshot.CurrentAreaID, toAreaID, sourceRef)
	return exploration.MoveDungeonInstance(def, scope, snapshot, context(snapshot, scope, permit), exploration.MoveDungeonInstanceInput{ConnectionID: connectionID, DestinationAreaID: toAreaID})
}

func progress(def exploration.DungeonDefinition, scope exploration.DungeonScope, snapshot exploration.DungeonInstanceSnapshot, input exploration.InstanceProgress) (exploration.InstanceTransition, error) {
	ctx := context(snapshot, scope, permit(exploration.ActionProgress, input.ID, "", "", input.SourceRef))
	return exploration.RecordInstanceProgress(def, scope, snapshot, ctx, input)
}

func context(snapshot exploration.DungeonInstanceSnapshot, scope exploration.DungeonScope, p exploration.InstanceActionPermit) exploration.InstanceActionContext {
	if p.Binding == (exploration.InstancePermitBinding{}) {
		p.Binding = exploration.InstancePermitBinding{InstanceID: snapshot.Binding.InstanceID, WorldID: scope.WorldID, BranchID: scope.BranchID, DefinitionHash: snapshot.DefinitionHash, Revision: snapshot.Revision}
	}
	return exploration.InstanceActionContext{ExpectedRevision: snapshot.Revision, Scope: scope, Permit: p, Battle: exploration.BattleNone, Blocking: exploration.BlockingNone}
}

func boundPermit(def exploration.DungeonDefinition, scope exploration.DungeonScope, instanceID string, revision uint64, p exploration.InstanceActionPermit) exploration.InstanceActionPermit {
	hash, _ := exploration.DungeonDefinitionContentHash(def)
	p.Binding = exploration.InstancePermitBinding{InstanceID: instanceID, WorldID: scope.WorldID, BranchID: scope.BranchID, DefinitionHash: hash, Revision: revision}
	return p
}

func hasChangeSource(diff exploration.InstanceDiff, kind, source string) bool {
	for _, change := range diff.Changes {
		if change.Kind == kind && change.SourceRef == source {
			return true
		}
	}
	return false
}

func permit(action exploration.InstanceActionKind, target, from, to, source string) exploration.InstanceActionPermit {
	return exploration.InstanceActionPermit{Decision: exploration.DecisionAllowed, Action: action, TargetID: target, FromAreaID: from, ToAreaID: to, SourceRef: source}
}

func failReplay(report replayReport, code, path string, err error) replayReport {
	report.Valid = false
	report.Diagnostics = append(report.Diagnostics, exploration.Diagnostic{Code: code, Path: path, Message: stableError(err)})
	return report
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func stableError(err error) string {
	if err == nil {
		return "unknown failure"
	}
	return strings.TrimSpace(err.Error())
}

func diagnosticCodes(items []exploration.Diagnostic) []string {
	codes := []string{}
	for _, item := range items {
		codes = append(codes, item.Code)
	}
	return codes
}

func cloneSnapshotForCheck(snapshot exploration.DungeonInstanceSnapshot) exploration.DungeonInstanceSnapshot {
	copy := snapshot
	copy.Discoveries = append([]exploration.InstanceDiscovery{}, snapshot.Discoveries...)
	copy.CompletedObjectives = append([]exploration.SourcedProgress{}, snapshot.CompletedObjectives...)
	copy.ResolvedChallenges = append([]exploration.SourcedProgress{}, snapshot.ResolvedChallenges...)
	return copy
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
