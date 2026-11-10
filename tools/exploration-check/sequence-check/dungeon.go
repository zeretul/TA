package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"ta/internal/exploration"
)

type transitionRecord struct {
	Action string                   `json:"action"`
	Diff   exploration.InstanceDiff `json:"diff"`
}

type dungeonRun struct {
	InsideCheckpoint      exploration.DungeonInstanceSnapshot `json:"inside_checkpoint"`
	OutsideCheckpoint     exploration.DungeonInstanceSnapshot `json:"outside_checkpoint"`
	FinalSnapshot         exploration.DungeonInstanceSnapshot `json:"final_snapshot"`
	FinalView             exploration.DungeonInstanceView     `json:"final_view"`
	Actions               []transitionRecord                  `json:"actions"`
	InsideRoundTripEqual  bool                                `json:"inside_round_trip_equal"`
	OutsideRoundTripEqual bool                                `json:"outside_round_trip_equal"`
	DuplicateNoOp         bool                                `json:"duplicate_fact_no_op"`
	HistoricalSource      string                              `json:"historical_fact_source"`
	ProjectionStable      bool                                `json:"projection_stable"`
}

type dungeonScenarioReport struct {
	Interrupted               dungeonRun        `json:"interrupted"`
	Uninterrupted             dungeonRun        `json:"uninterrupted"`
	FinalStatesEqual          bool              `json:"final_states_equal"`
	FinalViewsEqual           bool              `json:"final_views_equal"`
	StalePermitRejected       bool              `json:"stale_permit_rejected_without_mutation"`
	BattleRejected            bool              `json:"active_battle_rejected_without_mutation"`
	ChangedDefinitionRejected bool              `json:"changed_definition_rejected_without_mutation"`
	NegativeErrors            map[string]string `json:"negative_errors"`
}

func runDungeonScenario(def exploration.DungeonDefinition, report *suiteReport) dungeonScenarioReport {
	scope := exploration.DungeonScope{WorldID: "world.dungeon-sequence", BranchID: "branch.main"}
	interrupted, interruptedErr := runDungeonPath(def, scope, true)
	uninterrupted, uninterruptedErr := runDungeonPath(def, scope, false)
	if interruptedErr != nil || uninterruptedErr != nil {
		recordCheck(report, "dungeon_sequence_setup", false, "both core API action sequences complete", fmt.Sprintf("interrupted=%v uninterrupted=%v", interruptedErr, uninterruptedErr))
		return dungeonScenarioReport{}
	}
	recordCheck(report, "dungeon_sequence_setup", true, "both core API action sequences complete", fmt.Sprintf("final_revision=%d", interrupted.FinalSnapshot.Revision))
	recordCheck(report, "dungeon_inside_checkpoint_round_trip", interrupted.InsideRoundTripEqual, "inside snapshot survives JSON representation round trip", fmt.Sprintf("equal=%t", interrupted.InsideRoundTripEqual))
	recordCheck(report, "dungeon_outside_checkpoint_round_trip", interrupted.OutsideRoundTripEqual, "outside snapshot survives JSON representation round trip", fmt.Sprintf("equal=%t", interrupted.OutsideRoundTripEqual))
	finalEqual := reflect.DeepEqual(interrupted.FinalSnapshot, uninterrupted.FinalSnapshot)
	viewEqual := reflect.DeepEqual(interrupted.FinalView, uninterrupted.FinalView)
	recordCheck(report, "dungeon_round_trip_equivalence", finalEqual && viewEqual, "JSON checkpoint sequence and uninterrupted sequence have equal final snapshot and projected view", fmt.Sprintf("snapshot_equal=%t view_equal=%t revision=%d", finalEqual, viewEqual, interrupted.FinalSnapshot.Revision))

	noOp := interrupted.DuplicateNoOp && uninterrupted.DuplicateNoOp
	recordCheck(report, "dungeon_duplicate_fact_no_op", noOp, "duplicate fact preserves first source and creates no revision or diff", fmt.Sprintf("interrupted=%t source=%s", interrupted.DuplicateNoOp, interrupted.HistoricalSource))
	insideSources := hasTransitionSource(interrupted.Actions, "position_changed", "source.inside.to-exit") &&
		hasTransitionSource(interrupted.Actions, "presence_changed", "source.exit") &&
		hasTransitionSource(interrupted.Actions, "last_exit_changed", "source.exit") &&
		hasTransitionSource(interrupted.Actions, "position_changed", "source.reenter") &&
		hasTransitionSource(interrupted.Actions, "presence_changed", "source.reenter")
	recordCheck(report, "dungeon_transition_sources", insideSources, "move, exit, and re-entry diffs retain their current action source", fmt.Sprintf("sources_present=%t", insideSources))
	projectionStable := interrupted.ProjectionStable && uninterrupted.ProjectionStable
	recordCheck(report, "dungeon_projection_pure", projectionStable, "repeated view projection leaves snapshot bytes and revision unchanged", fmt.Sprintf("interrupted=%t uninterrupted=%t", interrupted.ProjectionStable, uninterrupted.ProjectionStable))

	staleRejected, staleError := checkStalePermit(def, scope, uninterrupted.FinalSnapshot)
	battleRejected, battleError := checkBattleBlock(def, scope, uninterrupted.FinalSnapshot)
	changedRejected, changedError := checkChangedDefinition(def, scope, uninterrupted.FinalSnapshot)
	recordCheck(report, "dungeon_stale_permit_immutable", staleRejected, "stale permit is rejected without mutating advanced snapshot", staleError)
	recordCheck(report, "dungeon_battle_block_immutable", battleRejected, "active battle blocks move without mutating snapshot", battleError)
	recordCheck(report, "dungeon_changed_definition_immutable", changedRejected, "same ID/version changed definition is rejected without mutating snapshot", changedError)
	return dungeonScenarioReport{Interrupted: interrupted, Uninterrupted: uninterrupted, FinalStatesEqual: finalEqual, FinalViewsEqual: viewEqual,
		StalePermitRejected: staleRejected, BattleRejected: battleRejected, ChangedDefinitionRejected: changedRejected,
		NegativeErrors: map[string]string{"stale_permit": staleError, "active_battle": battleError, "changed_definition": changedError}}
}

func runDungeonPath(def exploration.DungeonDefinition, scope exploration.DungeonScope, roundTrip bool) (dungeonRun, error) {
	created, err := createInstance(def, scope, "sequence.cave", "entrance", "source.entry")
	if err != nil {
		return dungeonRun{}, err
	}
	run := dungeonRun{Actions: []transitionRecord{{Action: "create", Diff: created.Diff}}}
	snapshot := created.Snapshot
	fact := exploration.InstanceDiscovery{Kind: exploration.DiscoveryFact, ID: "archive.marker", SourceRef: "source.marker.first", Text: "A carved marker is visible.", PubliclyDisplayable: true}
	transition, err := exploration.RegisterInstanceDiscovery(def, scope, snapshot, actionContext(scope, snapshot, actionPermit(def, scope, snapshot, exploration.ActionDiscover, fact.ID, "", "", fact.SourceRef)), fact)
	if err != nil {
		return run, err
	}
	run.Actions = append(run.Actions, transitionRecord{Action: "record_first_fact", Diff: transition.Diff})
	snapshot = transition.Snapshot
	firstRevision := snapshot.Revision
	repeat := fact
	repeat.SourceRef = "source.marker.repeat"
	transition, err = exploration.RegisterInstanceDiscovery(def, scope, snapshot, actionContext(scope, snapshot, actionPermit(def, scope, snapshot, exploration.ActionDiscover, repeat.ID, "", "", repeat.SourceRef)), repeat)
	if err != nil {
		return run, err
	}
	run.DuplicateNoOp = transition.Snapshot.Revision == firstRevision && transition.Diff.FromRevision == transition.Diff.ToRevision && len(transition.Diff.Changes) == 0
	snapshot = transition.Snapshot
	run.HistoricalSource = discoverySource(snapshot, exploration.DiscoveryFact, fact.ID)
	run.Actions = append(run.Actions, transitionRecord{Action: "repeat_fact_no_op", Diff: transition.Diff})

	progress := exploration.InstanceProgress{Kind: exploration.ProgressObjective, ID: "recover.inscription", SourceRef: "source.inscription.recovered", Outcome: exploration.OutcomeConfirmed}
	transition, err = exploration.RecordInstanceProgress(def, scope, snapshot, actionContext(scope, snapshot, actionPermit(def, scope, snapshot, exploration.ActionProgress, progress.ID, "", "", progress.SourceRef)), progress)
	if err != nil {
		return run, err
	}
	run.Actions = append(run.Actions, transitionRecord{Action: "confirm_objective", Diff: transition.Diff})
	snapshot = transition.Snapshot
	transition, err = exploration.MoveDungeonInstance(def, scope, snapshot, actionContext(scope, snapshot, actionPermit(def, scope, snapshot, exploration.ActionMove, "entry_passage", "entrance", "archive", "source.move.archive")), exploration.MoveDungeonInstanceInput{ConnectionID: "entry_passage", DestinationAreaID: "archive"})
	if err != nil {
		return run, err
	}
	run.Actions = append(run.Actions, transitionRecord{Action: "move_to_archive", Diff: transition.Diff})
	snapshot = transition.Snapshot
	run.InsideCheckpoint = snapshot
	if roundTrip {
		before := snapshot
		snapshot, err = jsonRoundTrip(snapshot)
		if err != nil {
			return run, err
		}
		run.InsideRoundTripEqual = sameDungeonCheckpoint(before, snapshot)
	} else {
		run.InsideRoundTripEqual = true
	}
	transition, err = exploration.MoveDungeonInstance(def, scope, snapshot, actionContext(scope, snapshot, actionPermit(def, scope, snapshot, exploration.ActionMove, "outbound_ramp", "archive", "exit", "source.inside.to-exit")), exploration.MoveDungeonInstanceInput{ConnectionID: "outbound_ramp", DestinationAreaID: "exit"})
	if err != nil {
		return run, err
	}
	run.Actions = append(run.Actions, transitionRecord{Action: "move_to_exit", Diff: transition.Diff})
	snapshot = transition.Snapshot
	transition, err = exploration.ExitDungeonInstance(def, scope, snapshot, actionContext(scope, snapshot, actionPermit(def, scope, snapshot, exploration.ActionExit, "exit", "", "", "source.exit")))
	if err != nil {
		return run, err
	}
	run.Actions = append(run.Actions, transitionRecord{Action: "exit_to_outside", Diff: transition.Diff})
	snapshot = transition.Snapshot
	run.OutsideCheckpoint = snapshot
	if roundTrip {
		before := snapshot
		snapshot, err = jsonRoundTrip(snapshot)
		if err != nil {
			return run, err
		}
		run.OutsideRoundTripEqual = sameDungeonCheckpoint(before, snapshot)
	} else {
		run.OutsideRoundTripEqual = true
	}
	transition, err = exploration.ReenterDungeonInstance(def, scope, snapshot, actionContext(scope, snapshot, actionPermit(def, scope, snapshot, exploration.ActionReenter, "entrance", "", "", "source.reenter")), "entrance")
	if err != nil {
		return run, err
	}
	run.Actions = append(run.Actions, transitionRecord{Action: "reenter_at_entry", Diff: transition.Diff})
	snapshot = transition.Snapshot
	if snapshot.Presence != exploration.InstanceInside || snapshot.CurrentAreaID != "entrance" || snapshot.LastExitAreaID != "exit" || !reflect.DeepEqual(snapshot.CompletedObjectives, run.InsideCheckpoint.CompletedObjectives) {
		return run, fmt.Errorf("re-entry did not preserve presence, actual arrival position, exit history, or objective progress")
	}
	transition, err = exploration.MoveDungeonInstance(def, scope, snapshot, actionContext(scope, snapshot, actionPermit(def, scope, snapshot, exploration.ActionMove, "entry_passage", "entrance", "archive", "source.resume.archive")), exploration.MoveDungeonInstanceInput{ConnectionID: "entry_passage", DestinationAreaID: "archive"})
	if err != nil {
		return run, err
	}
	run.Actions = append(run.Actions, transitionRecord{Action: "resume_inside", Diff: transition.Diff})
	snapshot = transition.Snapshot
	run.FinalSnapshot = snapshot
	view, projectionErr := projectView(def, scope, snapshot)
	if projectionErr != nil {
		return run, projectionErr
	}
	run.FinalView = view
	run.ProjectionStable = assertProjectionPure(def, scope, snapshot, view)
	return run, nil
}

func projectView(def exploration.DungeonDefinition, scope exploration.DungeonScope, snapshot exploration.DungeonInstanceSnapshot) (exploration.DungeonInstanceView, error) {
	view, diagnostics := exploration.ProjectDungeonInstanceView(def, scope, snapshot)
	if len(diagnostics) != 0 {
		return exploration.DungeonInstanceView{}, fmt.Errorf("projection diagnostics: %v", diagnostics)
	}
	return view, nil
}

func assertProjectionPure(def exploration.DungeonDefinition, scope exploration.DungeonScope, snapshot exploration.DungeonInstanceSnapshot, expected exploration.DungeonInstanceView) bool {
	before, _ := json.Marshal(snapshot)
	for range 3 {
		view, diagnostics := exploration.ProjectDungeonInstanceView(def, scope, snapshot)
		if len(diagnostics) != 0 || !reflect.DeepEqual(view, expected) {
			return false
		}
	}
	after, _ := json.Marshal(snapshot)
	return string(before) == string(after)
}

func checkStalePermit(def exploration.DungeonDefinition, scope exploration.DungeonScope, base exploration.DungeonInstanceSnapshot) (bool, string) {
	baseBefore, _ := json.Marshal(base)
	oldPermit := actionPermit(def, scope, base, exploration.ActionMove, "outbound_ramp", "archive", "exit", "source.stale.old")
	fact := exploration.InstanceDiscovery{Kind: exploration.DiscoveryFact, ID: "revision.marker", SourceRef: "source.revision.bump", Text: "A fresh revision marker.", PubliclyDisplayable: true}
	advanced, err := exploration.RegisterInstanceDiscovery(def, scope, base, actionContext(scope, base, actionPermit(def, scope, base, exploration.ActionDiscover, fact.ID, "", "", fact.SourceRef)), fact)
	if err != nil {
		return false, err.Error()
	}
	before, _ := json.Marshal(advanced.Snapshot)
	ctx := actionContext(scope, advanced.Snapshot, oldPermit)
	_, staleErr := exploration.MoveDungeonInstance(def, scope, advanced.Snapshot, ctx, exploration.MoveDungeonInstanceInput{ConnectionID: "outbound_ramp", DestinationAreaID: "exit"})
	after, _ := json.Marshal(advanced.Snapshot)
	baseAfter, _ := json.Marshal(base)
	passed := errors.Is(staleErr, exploration.ErrStaleInstanceRevision) && string(before) == string(after) && string(baseBefore) == string(baseAfter)
	return passed, errorText(staleErr)
}

func checkBattleBlock(def exploration.DungeonDefinition, scope exploration.DungeonScope, snapshot exploration.DungeonInstanceSnapshot) (bool, string) {
	before, _ := json.Marshal(snapshot)
	ctx := actionContext(scope, snapshot, actionPermit(def, scope, snapshot, exploration.ActionMove, "outbound_ramp", "archive", "exit", "source.battle.block"))
	ctx.Battle = exploration.BattleActive
	_, err := exploration.MoveDungeonInstance(def, scope, snapshot, ctx, exploration.MoveDungeonInstanceInput{ConnectionID: "outbound_ramp", DestinationAreaID: "exit"})
	after, _ := json.Marshal(snapshot)
	passed := errors.Is(err, exploration.ErrActionBlocked) && string(before) == string(after)
	return passed, errorText(err)
}

func checkChangedDefinition(def exploration.DungeonDefinition, scope exploration.DungeonScope, snapshot exploration.DungeonInstanceSnapshot) (bool, string) {
	changed := def
	changed.Areas = append([]exploration.Area(nil), def.Areas...)
	changed.Areas[0].Description += " Changed content with unchanged definition ID and version."
	before, _ := json.Marshal(snapshot)
	ctx := actionContext(scope, snapshot, actionPermit(def, scope, snapshot, exploration.ActionMove, "outbound_ramp", "archive", "exit", "source.definition.changed"))
	_, err := exploration.MoveDungeonInstance(changed, scope, snapshot, ctx, exploration.MoveDungeonInstanceInput{ConnectionID: "outbound_ramp", DestinationAreaID: "exit"})
	after, _ := json.Marshal(snapshot)
	passed := errors.Is(err, exploration.ErrInvalidInstance) && string(before) == string(after)
	return passed, errorText(err)
}

func hasTransitionSource(actions []transitionRecord, kind, source string) bool {
	for _, action := range actions {
		for _, change := range action.Diff.Changes {
			if change.Kind == kind && change.SourceRef == source {
				return true
			}
		}
	}
	return false
}

func discoverySource(snapshot exploration.DungeonInstanceSnapshot, kind exploration.InstanceDiscoveryKind, id string) string {
	for _, item := range snapshot.Discoveries {
		if item.Kind == kind && item.ID == id {
			return item.SourceRef
		}
	}
	return ""
}

// GrantedRewards is optional on the wire. An empty in-memory slice and its
// omitted JSON form represent the same facts; all nonempty rewards and every
// other checkpoint field must still match exactly.
func sameDungeonCheckpoint(a, b exploration.DungeonInstanceSnapshot) bool {
	if len(a.GrantedRewards) == 0 {
		a.GrantedRewards = nil
	}
	if len(b.GrantedRewards) == 0 {
		b.GrantedRewards = nil
	}
	return reflect.DeepEqual(a, b)
}
