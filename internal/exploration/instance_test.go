package exploration

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
)

func testInstanceDefinition() DungeonDefinition {
	return DungeonDefinition{
		SchemaVersion: SchemaVersion, ID: "synthetic.cave", Version: "1", PackageID: "synthetic.package", PackageVersion: "1", RulesetID: "synthetic.rules", RulesetVersion: "1", Theme: "A synthetic archive cave.",
		Objectives:   []Objective{{ID: "recover.record", Description: "Recover the archive record."}},
		Areas:        []Area{{ID: "entrance", Name: "Entrance", Description: "The known entrance."}, {ID: "archive", Name: "Archive", Description: "A chamber beyond the entrance."}, {ID: "exit", Name: "Exit", Description: "A separate legal exit."}},
		Connections:  []DungeonConnection{{ID: "passage", FromAreaID: "entrance", ToAreaID: "archive", Bidirectional: true, PlayerVisible: true}, {ID: "one_way_exit", FromAreaID: "archive", ToAreaID: "exit", Bidirectional: false, PlayerVisible: true}},
		EntryAreaIDs: []string{"entrance"}, ExitAreaIDs: []string{"exit"},
		Challenges:      []Challenge{{ID: "seal", AreaID: "archive", ReferenceID: "challenge.synthetic-seal"}},
		ContentRefs:     []ContentRef{},
		CompletionPaths: []CompletionPath{{ID: "investigate", ObjectiveIDs: []string{"recover.record"}, ChallengeIDs: []string{"seal"}, RequiredAbilityIDs: []string{}}},
	}
}

func instanceScope() DungeonScope {
	return DungeonScope{WorldID: "world.alpha", BranchID: "branch.main"}
}

func testInstancePermit(action InstanceActionKind, targetID, from, to, source string) InstanceActionPermit {
	return InstanceActionPermit{Decision: DecisionAllowed, Action: action, TargetID: targetID, FromAreaID: from, ToAreaID: to, SourceRef: source}
}

func testInstanceContext(snapshot DungeonInstanceSnapshot, scope DungeonScope, permit InstanceActionPermit) InstanceActionContext {
	if permit.Binding == (InstancePermitBinding{}) {
		permit.Binding = InstancePermitBinding{InstanceID: snapshot.Binding.InstanceID, WorldID: snapshot.Binding.WorldID, BranchID: snapshot.Binding.BranchID, DefinitionHash: snapshot.DefinitionHash, Revision: snapshot.Revision}
	}
	return InstanceActionContext{ExpectedRevision: snapshot.Revision, Scope: scope, Permit: permit, Battle: BattleNone, Blocking: BlockingNone}
}

func testCreateInstancePermit(t *testing.T, def DungeonDefinition, scope DungeonScope, instanceID, sourceRef string) InstanceActionPermit {
	t.Helper()
	hash, err := DungeonDefinitionContentHash(def)
	if err != nil {
		t.Fatal(err)
	}
	permit := testInstancePermit(ActionEnter, "entrance", "", "", sourceRef)
	permit.Binding = InstancePermitBinding{InstanceID: instanceID, WorldID: scope.WorldID, BranchID: scope.BranchID, DefinitionHash: hash, Revision: 0}
	return permit
}

func createTestInstance(t *testing.T) (DungeonDefinition, DungeonScope, DungeonInstanceSnapshot) {
	t.Helper()
	def := testInstanceDefinition()
	scope := instanceScope()
	created, err := CreateDungeonInstance(CreateDungeonInstanceInput{Definition: def, Scope: scope, InstanceID: "instance.alpha", EntryAreaID: "entrance",
		Permit: testCreateInstancePermit(t, def, scope, "instance.alpha", "scene.entry.1"), Battle: BattleNone, Blocking: BlockingNone,
		InitialDiscoveries: []InstanceDiscovery{{Kind: DiscoveryObjective, ID: "recover.record", SourceRef: "briefing.objective"}}})
	if err != nil {
		t.Fatal(err)
	}
	return def, scope, created.Snapshot
}

func TestDungeonInstanceLifecycleProjectsOnlyKnownAndPreservesReentry(t *testing.T) {
	def, scope, snapshot := createTestInstance(t)
	if snapshot.Presence != InstanceInside || snapshot.CurrentAreaID != "entrance" || snapshot.Revision != 1 {
		t.Fatalf("unexpected initial snapshot: %+v", snapshot)
	}
	view, diagnostics := ProjectDungeonInstanceView(def, scope, snapshot)
	if len(diagnostics) != 0 || view.CurrentArea == nil || view.CurrentArea.ID != "entrance" || len(view.Areas) != 1 || len(view.Connections) != 0 {
		t.Fatalf("initial projection exposed undiscovered information: view=%+v diagnostics=%+v", view, diagnostics)
	}

	transition := registerTestDiscovery(t, def, scope, snapshot, InstanceDiscovery{Kind: DiscoveryConnection, ID: "passage", SourceRef: "inspection.passage"})
	snapshot = transition.Snapshot
	view, diagnostics = ProjectDungeonInstanceView(def, scope, snapshot)
	if len(diagnostics) != 0 || len(view.Connections) != 0 {
		t.Fatalf("known edge leaked before its destination area was known: %+v %+v", view, diagnostics)
	}
	transition = registerTestDiscovery(t, def, scope, snapshot, InstanceDiscovery{Kind: DiscoveryArea, ID: "archive", SourceRef: "inspection.archive"})
	snapshot = transition.Snapshot
	view, diagnostics = ProjectDungeonInstanceView(def, scope, snapshot)
	if len(diagnostics) != 0 || len(view.Areas) != 2 || len(view.Connections) != 1 || view.Connections[0].ID != "passage" {
		t.Fatalf("confirmed area and edge were not projected: %+v %+v", view, diagnostics)
	}

	moveCtx := testInstanceContext(snapshot, scope, testInstancePermit(ActionMove, "passage", "entrance", "archive", "movement.entry-archive"))
	moved, err := MoveDungeonInstance(def, scope, snapshot, moveCtx, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"})
	if err != nil || moved.Snapshot.CurrentAreaID != "archive" || moved.Snapshot.Revision != snapshot.Revision+1 {
		t.Fatalf("authorized movement failed: transition=%+v err=%v", moved, err)
	}
	snapshot = moved.Snapshot
	progressCtx := testInstanceContext(snapshot, scope, testInstancePermit(ActionProgress, "recover.record", "", "", "result.record-verified"))
	progress, err := RecordInstanceProgress(def, scope, snapshot, progressCtx, InstanceProgress{Kind: ProgressObjective, ID: "recover.record", SourceRef: "result.record-verified", Outcome: OutcomeConfirmed})
	if err != nil || len(progress.Snapshot.CompletedObjectives) != 1 {
		t.Fatalf("confirmed objective progress failed: transition=%+v err=%v", progress, err)
	}
	snapshot = progress.Snapshot
	challengeCtx := testInstanceContext(snapshot, scope, testInstancePermit(ActionProgress, "seal", "", "", "result.seal-opened"))
	challenge, err := RecordInstanceProgress(def, scope, snapshot, challengeCtx, InstanceProgress{Kind: ProgressChallenge, ID: "seal", SourceRef: "result.seal-opened", Outcome: OutcomeConfirmed})
	if err != nil || len(challenge.Snapshot.ResolvedChallenges) != 1 {
		t.Fatalf("confirmed challenge result failed: transition=%+v err=%v", challenge, err)
	}
	snapshot = challenge.Snapshot
	moveCtx = testInstanceContext(snapshot, scope, testInstancePermit(ActionMove, "one_way_exit", "archive", "exit", "movement.archive-exit"))
	moved, err = MoveDungeonInstance(def, scope, snapshot, moveCtx, MoveDungeonInstanceInput{ConnectionID: "one_way_exit", DestinationAreaID: "exit"})
	if err != nil {
		t.Fatalf("legal exit path movement failed: %v", err)
	}
	snapshot = moved.Snapshot
	exitCtx := testInstanceContext(snapshot, scope, testInstancePermit(ActionExit, "exit", "", "", "scene.exit"))
	exited, err := ExitDungeonInstance(def, scope, snapshot, exitCtx)
	if err != nil || exited.Snapshot.Presence != InstanceOutside || exited.Snapshot.CurrentAreaID != "" || exited.Snapshot.LastExitAreaID != "exit" {
		t.Fatalf("legal exit did not produce explicit outside state: transition=%+v err=%v", exited, err)
	}
	snapshot = exited.Snapshot
	view, diagnostics = ProjectDungeonInstanceView(def, scope, snapshot)
	if len(diagnostics) != 0 || view.Presence != InstanceOutside || view.CurrentArea != nil || view.LastExitAreaID != "exit" || len(view.Areas) != 3 || len(view.CompletedObjectives) != 1 || len(view.ResolvedChallenges) != 1 {
		t.Fatalf("outside projection lost history or claimed an inside position: view=%+v diagnostics=%+v", view, diagnostics)
	}
	if snapshot.Discoveries == nil || len(snapshot.Discoveries) < 5 {
		t.Fatalf("outside snapshot lost discovery history: %+v", snapshot.Discoveries)
	}

	reenterCtx := testInstanceContext(snapshot, scope, testInstancePermit(ActionReenter, "entrance", "", "", "scene.reentry"))
	reentered, err := ReenterDungeonInstance(def, scope, snapshot, reenterCtx, "entrance")
	if err != nil || reentered.Snapshot.Presence != InstanceInside || reentered.Snapshot.CurrentAreaID != "entrance" || reentered.Snapshot.LastExitAreaID != "exit" {
		t.Fatalf("re-entry did not use the actual arrival entrance: transition=%+v err=%v", reentered, err)
	}
	if !reflect.DeepEqual(snapshot.Discoveries, reentered.Snapshot.Discoveries) || !reflect.DeepEqual(snapshot.CompletedObjectives, reentered.Snapshot.CompletedObjectives) || !reflect.DeepEqual(snapshot.ResolvedChallenges, reentered.Snapshot.ResolvedChallenges) {
		t.Fatal("re-entry reset prior discoveries or progress")
	}
}

func TestPermitCannotBeReusedAcrossInstancesOrRewrappedAtNewRevision(t *testing.T) {
	def, scope, instanceA := createTestInstance(t)
	createdB, err := CreateDungeonInstance(CreateDungeonInstanceInput{Definition: def, Scope: scope, InstanceID: "instance.beta", EntryAreaID: "entrance",
		Permit: testCreateInstancePermit(t, def, scope, "instance.beta", "scene.entry.beta"), Battle: BattleNone, Blocking: BlockingNone,
		InitialDiscoveries: []InstanceDiscovery{{Kind: DiscoveryObjective, ID: "recover.record", SourceRef: "briefing.objective"}}})
	if err != nil {
		t.Fatal(err)
	}
	permit := testInstancePermit(ActionMove, "passage", "entrance", "archive", "movement.signed-for-a")
	permitCtx := testInstanceContext(instanceA, scope, permit)
	if _, err := MoveDungeonInstance(def, scope, instanceA, permitCtx, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"}); err != nil {
		t.Fatalf("permit bound to A's own revision should work: %v", err)
	}
	otherBefore := cloneInstanceSnapshot(createdB.Snapshot)
	if _, err := MoveDungeonInstance(def, scope, createdB.Snapshot, permitCtx, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"}); err == nil {
		t.Fatal("permit signed for instance A was accepted by instance B")
	}
	if !reflect.DeepEqual(otherBefore, createdB.Snapshot) {
		t.Fatal("cross-instance permit changed instance B")
	}
	createTarget := func(targetDef DungeonDefinition, targetScope DungeonScope) DungeonInstanceSnapshot {
		t.Helper()
		created, createErr := CreateDungeonInstance(CreateDungeonInstanceInput{Definition: targetDef, Scope: targetScope, InstanceID: "instance.alpha", EntryAreaID: "entrance",
			Permit: testCreateInstancePermit(t, targetDef, targetScope, "instance.alpha", "scene.foreign-scope"), Battle: BattleNone, Blocking: BlockingNone,
			InitialDiscoveries: []InstanceDiscovery{{Kind: DiscoveryObjective, ID: "recover.record", SourceRef: "briefing.objective"}}})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return created.Snapshot
	}
	foreignWorld := scope
	foreignWorld.WorldID = "world.beta"
	foreignBranch := scope
	foreignBranch.BranchID = "branch.beta"
	changedDefinition := def
	changedDefinition.Areas = append([]Area(nil), def.Areas...)
	changedDefinition.Areas[0].Description = "Same definition ID and version, different content."
	for _, tc := range []struct {
		name       string
		definition DungeonDefinition
		scope      DungeonScope
	}{
		{name: "world", definition: def, scope: foreignWorld},
		{name: "branch", definition: def, scope: foreignBranch},
		{name: "definition hash", definition: changedDefinition, scope: scope},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := createTarget(tc.definition, tc.scope)
			foreignCtx := permitCtx
			foreignCtx.Scope = tc.scope
			foreignCtx.ExpectedRevision = target.Revision
			if _, moveErr := MoveDungeonInstance(tc.definition, tc.scope, target, foreignCtx, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"}); !errors.Is(moveErr, ErrInvalidBinding) {
				t.Fatalf("permit crossed %s binding: %v", tc.name, moveErr)
			}
		})
	}

	advanced, err := registerTestDiscoveryTransition(t, def, scope, instanceA, InstanceDiscovery{Kind: DiscoveryArea, ID: "archive", SourceRef: "inspection.archive"})
	if err != nil {
		t.Fatal(err)
	}
	oldPermitContext := permitCtx
	oldPermitContext.ExpectedRevision = advanced.Snapshot.Revision
	if _, err := MoveDungeonInstance(def, scope, advanced.Snapshot, oldPermitContext, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"}); !errors.Is(err, ErrStaleInstanceRevision) {
		t.Fatalf("updating only the outer expected revision reused an old permit: %v", err)
	}
}

func TestCreateRequiresPermitBoundToNewInstanceAndRevisionZero(t *testing.T) {
	def := testInstanceDefinition()
	scope := instanceScope()
	input := CreateDungeonInstanceInput{Definition: def, Scope: scope, InstanceID: "instance.new", EntryAreaID: "entrance", Battle: BattleNone, Blocking: BlockingNone}
	input.Permit = testCreateInstancePermit(t, def, scope, input.InstanceID, "scene.entry")
	input.Permit.Binding.Revision = 1
	if _, err := CreateDungeonInstance(input); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("creation accepted a permit not issued at pre-create revision zero: %v", err)
	}
	input.Permit = testCreateInstancePermit(t, def, scope, input.InstanceID, "scene.entry")
	input.Permit.Binding.InstanceID = "instance.other"
	if _, err := CreateDungeonInstance(input); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("creation accepted a permit for another instance: %v", err)
	}
}

func TestMovementExitAndReentryDiffsRetainCurrentActionSourceWithoutReplacingHistory(t *testing.T) {
	def, scope, snapshot := createTestInstance(t)
	passage := InstanceDiscovery{Kind: DiscoveryConnection, ID: "passage", SourceRef: "inspection.passage"}
	snapshot = registerTestDiscovery(t, def, scope, snapshot, passage).Snapshot
	snapshot = registerTestDiscovery(t, def, scope, snapshot, InstanceDiscovery{Kind: DiscoveryArea, ID: "archive", SourceRef: "inspection.archive"}).Snapshot
	outbound, err := moveFromCurrent(t, def, scope, snapshot, "passage", "archive", "movement.outbound")
	if err != nil {
		t.Fatal(err)
	}
	returned, err := moveFromCurrent(t, def, scope, outbound.Snapshot, "passage", "entrance", "movement.return")
	if err != nil {
		t.Fatal(err)
	}
	if !hasChangeSource(returned.Diff.Changes, "position_changed", "movement.return") {
		t.Fatalf("return movement diff lost its action source: %+v", returned.Diff)
	}
	if got := discoverySource(returned.Snapshot, DiscoveryArea, "entrance"); got != "scene.entry.1" {
		t.Fatalf("return movement overwrote the original entrance discovery source: %q", got)
	}
	if got := discoverySource(returned.Snapshot, DiscoveryConnection, "passage"); got != "inspection.passage" {
		t.Fatalf("traversing a known passage overwrote its first discovery source: %q", got)
	}

	archiveAgain, err := moveFromCurrent(t, def, scope, returned.Snapshot, "passage", "archive", "movement.archive-again")
	if err != nil {
		t.Fatal(err)
	}
	exitPath, err := moveFromCurrent(t, def, scope, archiveAgain.Snapshot, "one_way_exit", "exit", "movement.exit-path")
	if err != nil {
		t.Fatal(err)
	}
	exitCtx := testInstanceContext(exitPath.Snapshot, scope, testInstancePermit(ActionExit, "exit", "", "", "scene.exit-source"))
	exited, err := ExitDungeonInstance(def, scope, exitPath.Snapshot, exitCtx)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"presence_changed", "position_changed", "last_exit_changed"} {
		if !hasChangeSource(exited.Diff.Changes, kind, "scene.exit-source") {
			t.Fatalf("exit proposal diff %q lost the action source: %+v", kind, exited.Diff)
		}
	}
	reenterCtx := testInstanceContext(exited.Snapshot, scope, testInstancePermit(ActionReenter, "entrance", "", "", "scene.reentry-source"))
	reentered, err := ReenterDungeonInstance(def, scope, exited.Snapshot, reenterCtx, "entrance")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"presence_changed", "position_changed"} {
		if !hasChangeSource(reentered.Diff.Changes, kind, "scene.reentry-source") {
			t.Fatalf("re-entry proposal diff %q lost the action source: %+v", kind, reentered.Diff)
		}
	}
	if got := discoverySource(reentered.Snapshot, DiscoveryArea, "entrance"); got != "scene.entry.1" {
		t.Fatalf("re-entry rewrote historical discovery source: %q", got)
	}
}

func registerTestDiscovery(t *testing.T, def DungeonDefinition, scope DungeonScope, snapshot DungeonInstanceSnapshot, item InstanceDiscovery) InstanceTransition {
	t.Helper()
	ctx := testInstanceContext(snapshot, scope, testInstancePermit(ActionDiscover, item.ID, "", "", item.SourceRef))
	transition, err := RegisterInstanceDiscovery(def, scope, snapshot, ctx, item)
	if err != nil {
		t.Fatal(err)
	}
	return transition
}

func registerTestDiscoveryTransition(t *testing.T, def DungeonDefinition, scope DungeonScope, snapshot DungeonInstanceSnapshot, item InstanceDiscovery) (InstanceTransition, error) {
	t.Helper()
	ctx := testInstanceContext(snapshot, scope, testInstancePermit(ActionDiscover, item.ID, "", "", item.SourceRef))
	return RegisterInstanceDiscovery(def, scope, snapshot, ctx, item)
}

func moveFromCurrent(t *testing.T, def DungeonDefinition, scope DungeonScope, snapshot DungeonInstanceSnapshot, connectionID, destination, sourceRef string) (InstanceTransition, error) {
	t.Helper()
	ctx := testInstanceContext(snapshot, scope, testInstancePermit(ActionMove, connectionID, snapshot.CurrentAreaID, destination, sourceRef))
	return MoveDungeonInstance(def, scope, snapshot, ctx, MoveDungeonInstanceInput{ConnectionID: connectionID, DestinationAreaID: destination})
}

func hasChangeSource(items []InstanceChange, kind, sourceRef string) bool {
	for _, item := range items {
		if item.Kind == kind && item.SourceRef == sourceRef {
			return true
		}
	}
	return false
}

func discoverySource(snapshot DungeonInstanceSnapshot, kind InstanceDiscoveryKind, id string) string {
	for _, item := range snapshot.Discoveries {
		if item.Kind == kind && item.ID == id {
			return item.SourceRef
		}
	}
	return ""
}

func TestInstanceRepeatDiscoveryNoOpRumorUpgradeAndFactConflict(t *testing.T) {
	def, scope, snapshot := createTestInstance(t)
	first := registerTestDiscovery(t, def, scope, snapshot, InstanceDiscovery{Kind: DiscoveryRumor, ID: "rumor.archive", SourceRef: "heard.traveler", Text: "A record may remain.", PubliclyDisplayable: true})
	repeated := registerTestDiscovery(t, def, scope, first.Snapshot, InstanceDiscovery{Kind: DiscoveryRumor, ID: "rumor.archive", SourceRef: "heard.traveler", Text: "A record may remain.", PubliclyDisplayable: true})
	if repeated.Snapshot.Revision != first.Snapshot.Revision || len(repeated.Diff.Changes) != 0 || !reflect.DeepEqual(first.Snapshot, repeated.Snapshot) {
		t.Fatalf("repeated knowledge created a new revision: first=%+v repeated=%+v", first, repeated)
	}
	confirmed := registerTestDiscovery(t, def, scope, first.Snapshot, InstanceDiscovery{Kind: DiscoveryFact, ID: "rumor.archive", SourceRef: "inspection.archive-record", Text: "A record may remain.", PubliclyDisplayable: true})
	if containsDiscovery(confirmed.Snapshot.Discoveries, DiscoveryRumor, "rumor.archive") || !containsDiscovery(confirmed.Snapshot.Discoveries, DiscoveryFact, "rumor.archive") {
		t.Fatalf("rumor upgrade retained contradictory current knowledge: %+v", confirmed.Snapshot.Discoveries)
	}
	if !containsInstanceChange(confirmed.Diff.Changes, "rumor_confirmed", "rumor.archive") {
		t.Fatalf("rumor upgrade had no explicit diff: %+v", confirmed.Diff)
	}
	before := cloneInstanceSnapshot(confirmed.Snapshot)
	conflict := InstanceDiscovery{Kind: DiscoveryFact, ID: "rumor.archive", SourceRef: "inspection.conflict", Text: "A different incompatible claim.", PubliclyDisplayable: true}
	ctx := testInstanceContext(before, scope, testInstancePermit(ActionDiscover, conflict.ID, "", "", conflict.SourceRef))
	if _, err := RegisterInstanceDiscovery(def, scope, before, ctx, conflict); !errors.Is(err, ErrInstanceConflict) {
		t.Fatalf("conflicting fact was not rejected: %v", err)
	}
	if !reflect.DeepEqual(before, confirmed.Snapshot) {
		t.Fatal("rejected conflicting fact mutated the old snapshot")
	}
}

func TestInstanceKnownViewFiltersPrivateFactsAndNeverMutatesSnapshot(t *testing.T) {
	def, scope, snapshot := createTestInstance(t)
	private := registerTestDiscovery(t, def, scope, snapshot, InstanceDiscovery{Kind: DiscoveryFact, ID: "fact.private", SourceRef: "source.private", Text: "Internal note.", PubliclyDisplayable: false})
	publicRumor := registerTestDiscovery(t, def, scope, private.Snapshot, InstanceDiscovery{Kind: DiscoveryRumor, ID: "rumor.public", SourceRef: "source.rumor", Text: "A corridor might exist.", PubliclyDisplayable: true})
	before := cloneInstanceSnapshot(publicRumor.Snapshot)
	view, diagnostics := ProjectDungeonInstanceView(def, scope, publicRumor.Snapshot)
	if len(diagnostics) != 0 || len(view.ConfirmedFacts) != 0 || len(view.Rumors) != 1 || view.Rumors[0].ID != "rumor.public" {
		t.Fatalf("known view leaked private facts or lost a public rumor: view=%+v diagnostics=%+v", view, diagnostics)
	}
	if !reflect.DeepEqual(before, publicRumor.Snapshot) {
		t.Fatal("read-only projection mutated the snapshot")
	}
	contradictory := cloneInstanceSnapshot(publicRumor.Snapshot)
	contradictory.Discoveries = append(contradictory.Discoveries, InstanceDiscovery{Kind: DiscoveryFact, ID: "rumor.public", SourceRef: "source.confirmed", Text: "A corridor might exist.", PubliclyDisplayable: true})
	if !containsDiagnostic(ValidateDungeonInstance(def, scope, contradictory), "conflicting_knowledge_kind") {
		t.Fatal("snapshot with simultaneous rumor and fact passed validation")
	}
}

func TestInstanceJSONRoundTripAndDerivedSnapshotsDoNotShareMutableSlices(t *testing.T) {
	def, scope, snapshot := createTestInstance(t)
	before := cloneInstanceSnapshot(snapshot)
	left := registerTestDiscovery(t, def, scope, snapshot, InstanceDiscovery{Kind: DiscoveryFact, ID: "fact.left", SourceRef: "source.left", Text: "The left mark is fresh.", PubliclyDisplayable: true})
	right := registerTestDiscovery(t, def, scope, snapshot, InstanceDiscovery{Kind: DiscoveryFact, ID: "fact.right", SourceRef: "source.right", Text: "The right mark is old.", PubliclyDisplayable: true})
	if reflect.DeepEqual(left.Snapshot.Discoveries, right.Snapshot.Discoveries) {
		t.Fatal("independent proposals unexpectedly contain identical knowledge")
	}
	left.Snapshot.Discoveries[0].Text = "mutated proposal"
	if !reflect.DeepEqual(before, snapshot) || !containsDiscoveryText(right.Snapshot.Discoveries, "fact.right", "The right mark is old.") {
		t.Fatal("derived proposals share or mutated the source snapshot")
	}

	raw, err := json.Marshal(right.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var restored DungeonInstanceSnapshot
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	first, diagnostics := ProjectDungeonInstanceView(def, scope, right.Snapshot)
	second, restoredDiagnostics := ProjectDungeonInstanceView(def, scope, restored)
	if len(diagnostics) != 0 || len(restoredDiagnostics) != 0 || !reflect.DeepEqual(first, second) {
		t.Fatalf("snapshot JSON representation changed the view: before=%+v after=%+v diagnostics=%+v %+v", first, second, diagnostics, restoredDiagnostics)
	}
}

func TestInstanceBindingHashRevisionAndAuthorizationRejectWithoutMutation(t *testing.T) {
	def, scope, snapshot := createTestInstance(t)
	otherWorld := scope
	otherWorld.WorldID = "world.foreign"
	if view, diagnostics := ProjectDungeonInstanceView(def, otherWorld, snapshot); len(diagnostics) == 0 || len(view.Areas) != 0 {
		t.Fatalf("foreign world received an instance projection: view=%+v diagnostics=%+v", view, diagnostics)
	}
	otherBranch := scope
	otherBranch.BranchID = "branch.foreign"
	if _, diagnostics := ProjectDungeonInstanceView(def, otherBranch, snapshot); len(diagnostics) == 0 {
		t.Fatal("foreign branch passed instance projection")
	}
	changedDefinition := def
	changedDefinition.Areas = append([]Area(nil), def.Areas...)
	changedDefinition.Areas[1].Description = "Content swapped without a version change."
	if _, diagnostics := ProjectDungeonInstanceView(changedDefinition, scope, snapshot); !hasDiagnostic(diagnostics, "definition_content_mismatch") {
		t.Fatalf("same-ID/version content swap passed: %+v", diagnostics)
	}

	before := cloneInstanceSnapshot(snapshot)
	assertRejectedUnchanged := func(name string, action func() error, want error) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			if err := action(); want != nil && !errors.Is(err, want) {
				t.Fatalf("error=%v, want errors.Is(_, %v)", err, want)
			} else if err == nil {
				t.Fatal("invalid action unexpectedly succeeded")
			}
			if !reflect.DeepEqual(before, snapshot) {
				t.Fatal("rejected action mutated the old snapshot")
			}
		})
	}
	assertRejectedUnchanged("nonadjacent destination", func() error {
		ctx := testInstanceContext(snapshot, scope, testInstancePermit(ActionMove, "passage", "entrance", "exit", "move.bad"))
		_, err := MoveDungeonInstance(def, scope, snapshot, ctx, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "exit"})
		return err
	}, ErrInvalidInstanceAction)
	assertRejectedUnchanged("missing permit", func() error {
		ctx := InstanceActionContext{ExpectedRevision: snapshot.Revision, Scope: scope}
		_, err := MoveDungeonInstance(def, scope, snapshot, ctx, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"})
		return err
	}, ErrInvalidInstanceAction)
	assertRejectedUnchanged("active battle", func() error {
		ctx := testInstanceContext(snapshot, scope, testInstancePermit(ActionMove, "passage", "entrance", "archive", "move.battle"))
		ctx.Battle = BattleActive
		_, err := MoveDungeonInstance(def, scope, snapshot, ctx, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"})
		return err
	}, ErrActionBlocked)
	assertRejectedUnchanged("pending block", func() error {
		ctx := testInstanceContext(snapshot, scope, testInstancePermit(ActionMove, "passage", "entrance", "archive", "move.blocked"))
		ctx.Blocking = BlockingPending
		_, err := MoveDungeonInstance(def, scope, snapshot, ctx, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"})
		return err
	}, ErrActionBlocked)
	assertRejectedUnchanged("stale revision", func() error {
		ctx := testInstanceContext(snapshot, scope, testInstancePermit(ActionMove, "passage", "entrance", "archive", "move.stale"))
		ctx.ExpectedRevision--
		_, err := MoveDungeonInstance(def, scope, snapshot, ctx, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"})
		return err
	}, ErrStaleInstanceRevision)
	assertRejectedUnchanged("decision blocked", func() error {
		permit := testInstancePermit(ActionMove, "passage", "entrance", "archive", "move.denied")
		permit.Decision = DecisionBlocked
		ctx := testInstanceContext(snapshot, scope, permit)
		_, err := MoveDungeonInstance(def, scope, snapshot, ctx, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"})
		return err
	}, ErrActionNotAllowed)
	assertRejectedUnchanged("scene state unspecified", func() error {
		ctx := testInstanceContext(snapshot, scope, testInstancePermit(ActionMove, "passage", "entrance", "archive", "move.unknown"))
		ctx.Battle = BattleUnknown
		_, err := MoveDungeonInstance(def, scope, snapshot, ctx, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"})
		return err
	}, ErrInvalidInstanceAction)
	assertRejectedUnchanged("one-way reverse", func() error {
		forwardCtx := testInstanceContext(snapshot, scope, testInstancePermit(ActionMove, "passage", "entrance", "archive", "move.archive"))
		forward, err := MoveDungeonInstance(def, scope, snapshot, forwardCtx, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"})
		if err != nil {
			return err
		}
		archiveSnapshot := forward.Snapshot
		ctx := testInstanceContext(archiveSnapshot, scope, testInstancePermit(ActionMove, "one_way_exit", "archive", "archive", "move.reverse"))
		_, err = MoveDungeonInstance(def, scope, archiveSnapshot, ctx, MoveDungeonInstanceInput{ConnectionID: "one_way_exit", DestinationAreaID: "archive"})
		return err
	}, ErrInvalidInstanceAction)
	assertRejectedUnchanged("wrong exit", func() error {
		ctx := testInstanceContext(snapshot, scope, testInstancePermit(ActionExit, "entrance", "", "", "exit.wrong"))
		_, err := ExitDungeonInstance(def, scope, snapshot, ctx)
		return err
	}, ErrInvalidInstanceAction)

	progress := InstanceProgress{Kind: ProgressObjective, ID: "recover.record", SourceRef: "result.unconfirmed", Outcome: OutcomeUnconfirmed}
	ctx := testInstanceContext(snapshot, scope, testInstancePermit(ActionProgress, progress.ID, "", "", progress.SourceRef))
	if _, err := RecordInstanceProgress(def, scope, snapshot, ctx, progress); !errors.Is(err, ErrInvalidInstanceAction) {
		t.Fatalf("unconfirmed progress passed: %v", err)
	}
}

func TestActiveBattleAndPendingBlockAlsoStopExitAndReentry(t *testing.T) {
	def, scope, snapshot := createTestInstance(t)
	archive, err := MoveDungeonInstance(def, scope, snapshot, testInstanceContext(snapshot, scope, testInstancePermit(ActionMove, "passage", "entrance", "archive", "move.archive")), MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	exit, err := MoveDungeonInstance(def, scope, archive.Snapshot, testInstanceContext(archive.Snapshot, scope, testInstancePermit(ActionMove, "one_way_exit", "archive", "exit", "move.exit")), MoveDungeonInstanceInput{ConnectionID: "one_way_exit", DestinationAreaID: "exit"})
	if err != nil {
		t.Fatal(err)
	}
	activeExit := testInstanceContext(exit.Snapshot, scope, testInstancePermit(ActionExit, "exit", "", "", "exit.blocked"))
	activeExit.Battle = BattleActive
	if _, err := ExitDungeonInstance(def, scope, exit.Snapshot, activeExit); !errors.Is(err, ErrActionBlocked) {
		t.Fatalf("active battle did not block exit: %v", err)
	}
	exited, err := ExitDungeonInstance(def, scope, exit.Snapshot, testInstanceContext(exit.Snapshot, scope, testInstancePermit(ActionExit, "exit", "", "", "exit.ok")))
	if err != nil {
		t.Fatal(err)
	}
	pendingReentry := testInstanceContext(exited.Snapshot, scope, testInstancePermit(ActionReenter, "entrance", "", "", "reentry.blocked"))
	pendingReentry.Blocking = BlockingPending
	if _, err := ReenterDungeonInstance(def, scope, exited.Snapshot, pendingReentry, "entrance"); !errors.Is(err, ErrActionBlocked) {
		t.Fatalf("pending block did not prevent re-entry: %v", err)
	}
	unknownReentry := testInstanceContext(exited.Snapshot, scope, testInstancePermit(ActionReenter, "entrance", "", "", "reentry.unknown"))
	unknownReentry.Battle = BattleUnknown
	if _, err := ReenterDungeonInstance(def, scope, exited.Snapshot, unknownReentry, "entrance"); !errors.Is(err, ErrInvalidInstanceAction) {
		t.Fatalf("unspecified battle state allowed re-entry: %v", err)
	}
}

func TestOneWayDirectedMovementAndDefinitionHashChangeAreStrict(t *testing.T) {
	def, scope, snapshot := createTestInstance(t)
	forwardCtx := testInstanceContext(snapshot, scope, testInstancePermit(ActionMove, "passage", "entrance", "archive", "move.archive"))
	forward, err := MoveDungeonInstance(def, scope, snapshot, forwardCtx, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	archive := forward.Snapshot
	ctx := testInstanceContext(archive, scope, testInstancePermit(ActionMove, "one_way_exit", "archive", "exit", "move.exit"))
	transition, err := MoveDungeonInstance(def, scope, archive, ctx, MoveDungeonInstanceInput{ConnectionID: "one_way_exit", DestinationAreaID: "exit"})
	if err != nil || transition.Snapshot.CurrentAreaID != "exit" {
		t.Fatalf("forward one-way movement rejected: transition=%+v err=%v", transition, err)
	}
	reverse := cloneInstanceSnapshot(transition.Snapshot)
	ctx = testInstanceContext(reverse, scope, testInstancePermit(ActionMove, "one_way_exit", "exit", "archive", "move.reverse"))
	if _, err := MoveDungeonInstance(def, scope, reverse, ctx, MoveDungeonInstanceInput{ConnectionID: "one_way_exit", DestinationAreaID: "archive"}); !errors.Is(err, ErrInvalidInstanceAction) {
		t.Fatalf("reverse one-way movement passed: %v", err)
	}
	if !reflect.DeepEqual(reverse, transition.Snapshot) {
		t.Fatal("rejected reverse movement mutated snapshot")
	}

	changed := def
	changed.Areas = append([]Area(nil), def.Areas...)
	changed.Areas[0].Name = "Changed entrance under same version"
	ctx = testInstanceContext(snapshot, scope, testInstancePermit(ActionMove, "passage", "entrance", "archive", "move.changed-definition"))
	if _, err := MoveDungeonInstance(changed, scope, snapshot, ctx, MoveDungeonInstanceInput{ConnectionID: "passage", DestinationAreaID: "archive"}); !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("operation accepted changed definition content: %v", err)
	}
}

func TestInstanceRevisionOverflowAndMalformedSnapshotAreRejected(t *testing.T) {
	def, scope, snapshot := createTestInstance(t)
	snapshot.Revision = math.MaxUint64
	item := InstanceDiscovery{Kind: DiscoveryFact, ID: "overflow.fact", SourceRef: "source.overflow", Text: "A bounded fact.", PubliclyDisplayable: true}
	ctx := testInstanceContext(snapshot, scope, testInstancePermit(ActionDiscover, item.ID, "", "", item.SourceRef))
	if _, err := RegisterInstanceDiscovery(def, scope, snapshot, ctx, item); !errors.Is(err, ErrRevisionOverflow) {
		t.Fatalf("revision overflow was not rejected: %v", err)
	}
	if !containsDiagnostic(ValidateDungeonInstance(def, scope, DungeonInstanceSnapshot{SchemaVersion: InstanceSnapshotSchemaVersion, Binding: snapshot.Binding, DefinitionHash: snapshot.DefinitionHash, Revision: 1, Presence: InstanceOutside, CurrentAreaID: "entrance", Discoveries: snapshot.Discoveries, CompletedObjectives: []SourcedProgress{}, ResolvedChallenges: []SourcedProgress{}}), "outside_has_current_area") {
		t.Fatal("outside snapshot retained a current dungeon position")
	}
	if !containsDiagnostic(ValidateDungeonInstance(def, scope, DungeonInstanceSnapshot{SchemaVersion: InstanceSnapshotSchemaVersion, Binding: snapshot.Binding, DefinitionHash: snapshot.DefinitionHash, Revision: 0, Presence: InstanceInside, CurrentAreaID: "entrance", Discoveries: snapshot.Discoveries, CompletedObjectives: []SourcedProgress{}, ResolvedChallenges: []SourcedProgress{}}), "invalid_revision") {
		t.Fatal("zero revision passed snapshot validation")
	}
	oversized := cloneInstanceSnapshot(snapshot)
	oversized.Discoveries = make([]InstanceDiscovery, MaxInstanceDiscoveries+1)
	for index := range oversized.Discoveries {
		oversized.Discoveries[index] = InstanceDiscovery{Kind: DiscoveryArea, ID: "entrance", SourceRef: "source.bound"}
	}
	if !containsDiagnostic(ValidateDungeonInstance(def, scope, oversized), "too_many_discoveries") {
		t.Fatal("discovery capacity overflow had no explicit diagnostic")
	}
	emptyDefinition := DungeonDefinition{SchemaVersion: SchemaVersion, ID: "empty", Version: "1", PackageID: "pkg", PackageVersion: "1", RulesetID: "rules", RulesetVersion: "1", Theme: "Empty", Areas: []Area{{ID: "entry", Name: "Entry", Description: "Entry"}}, EntryAreaIDs: []string{"entry"}, ExitAreaIDs: []string{"entry"}, Objectives: []Objective{}, Connections: []DungeonConnection{}, Challenges: []Challenge{}, ContentRefs: []ContentRef{}, CompletionPaths: []CompletionPath{}}
	emptyPermit := testCreateInstancePermit(t, emptyDefinition, scope, "empty.instance", "entry.source")
	if _, err := CreateDungeonInstance(CreateDungeonInstanceInput{Definition: emptyDefinition, Scope: scope, InstanceID: "empty.instance", EntryAreaID: "entry", Permit: emptyPermit, Battle: BattleNone, Blocking: BlockingNone}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("definition without a goal or completion path created an instance: %v", err)
	}
}

func containsDiscovery(items []InstanceDiscovery, kind InstanceDiscoveryKind, id string) bool {
	for _, item := range items {
		if item.Kind == kind && item.ID == id {
			return true
		}
	}
	return false
}

func containsDiscoveryText(items []InstanceDiscovery, id, text string) bool {
	for _, item := range items {
		if item.ID == id && item.Text == text {
			return true
		}
	}
	return false
}

func containsInstanceChange(items []InstanceChange, kind, id string) bool {
	for _, item := range items {
		if item.Kind == kind && item.ID == id {
			return true
		}
	}
	return false
}

func containsDiagnostic(items []Diagnostic, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}
