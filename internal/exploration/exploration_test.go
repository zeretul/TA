package exploration

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func testRoute() RouteDefinition {
	return RouteDefinition{ID: "road", Version: "1", OriginLocationID: "start", DestinationLocationID: "end", Segments: []RouteSegment{
		{ID: "a", FromLocationID: "start", ToLocationID: "mid", DurationMinutes: 10, MovementMethodID: "walk"},
		{ID: "b", FromLocationID: "mid", ToLocationID: "end", DurationMinutes: 20, MovementMethodID: "walk"},
	}}
}

func TestAdvanceToUsesCumulativeTimeAndSameTargetAddsZero(t *testing.T) {
	route := testRoute()
	initial, err := PrepareJourney(route, "journey", []string{"walk"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := AdvanceTo(route, initial, 15, []string{"walk"})
	if err != nil {
		t.Fatal(err)
	}
	if first.AddedMinutes != 15 || first.Snapshot.ElapsedMinutes != 15 || first.Snapshot.CurrentSegmentID != "b" || first.Snapshot.SegmentElapsedMinutes != 5 || first.Position.SegmentElapsedMinutes != 5 {
		t.Fatalf("unexpected first progress: %+v", first)
	}
	if !validID(first.Position.ReferenceID) {
		t.Fatalf("mid-route position is not a reusable stable origin: %q", first.Position.ReferenceID)
	}
	redirect := RouteDefinition{ID: "detour", Version: "1", OriginLocationID: first.Position.ReferenceID, DestinationLocationID: "camp", Segments: []RouteSegment{{ID: "leave-road", FromLocationID: first.Position.ReferenceID, ToLocationID: "camp", DurationMinutes: 5, MovementMethodID: "walk"}}}
	if _, err := PrepareJourney(redirect, "detour-journey", []string{"walk"}); err != nil {
		t.Fatalf("route from exact current position rejected: %v", err)
	}
	second, err := AdvanceTo(route, first.Snapshot, 22, []string{"walk"})
	if err != nil {
		t.Fatal(err)
	}
	if second.AddedMinutes != 7 || second.Snapshot.SegmentElapsedMinutes != 12 {
		t.Fatalf("unexpected continuation: %+v", second)
	}
	retry, err := AdvanceTo(route, second.Snapshot, 22, []string{"walk"})
	if err != nil {
		t.Fatal(err)
	}
	if retry.AddedMinutes != 0 || !reflect.DeepEqual(retry.Snapshot, second.Snapshot) {
		t.Fatalf("same cumulative target changed snapshot: %+v", retry)
	}
	finished, err := AdvanceTo(route, second.Snapshot, 30, []string{"walk"})
	if err != nil || !finished.Done || finished.RemainingMinutes != 0 {
		t.Fatalf("route completion was not explicit: result=%+v err=%v", finished, err)
	}
	if _, err := AdvanceTo(route, second.Snapshot, 21, []string{"walk"}); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("backward target error=%v", err)
	}
	if _, err := AdvanceTo(route, second.Snapshot, 31, []string{"walk"}); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("overrun error=%v", err)
	}
}

func TestZeroTargetSnapshotCanRetryRoundTripAndContinue(t *testing.T) {
	route := RouteDefinition{ID: "zero-start", Version: "1", OriginLocationID: "here", DestinationLocationID: "there", Segments: []RouteSegment{{ID: "walk", FromLocationID: "here", ToLocationID: "there", DurationMinutes: 10, MovementMethodID: "foot"}}}
	initial, err := PrepareJourney(route, "zero-journey", []string{"foot"})
	if err != nil {
		t.Fatal(err)
	}
	zero, err := AdvanceTo(route, initial, 0, []string{"foot"})
	if err != nil {
		t.Fatal(err)
	}
	if zero.Snapshot.CompletedSegmentIDs == nil {
		t.Fatal("zero target returned a nil completed-segment prefix")
	}
	retry, err := AdvanceTo(route, zero.Snapshot, 0, []string{"foot"})
	if err != nil {
		t.Fatalf("same zero target retry failed: %v", err)
	}
	if retry.AddedMinutes != 0 {
		t.Fatalf("zero retry added %d minutes", retry.AddedMinutes)
	}
	encoded, err := json.Marshal(zero.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var restored JourneySnapshot
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	continued, err := AdvanceTo(route, restored, 1, []string{"foot"})
	if err != nil {
		t.Fatalf("continuation after zero target/JSON round-trip failed: %v", err)
	}
	if continued.AddedMinutes != 1 {
		t.Fatalf("continuation added %d minutes", continued.AddedMinutes)
	}
	withZeroEdge := RouteDefinition{ID: "zero-edge", Version: "1", OriginLocationID: "a", DestinationLocationID: "d", Segments: []RouteSegment{
		{ID: "first", FromLocationID: "a", ToLocationID: "b", DurationMinutes: 10, MovementMethodID: "foot"},
		{ID: "connection", FromLocationID: "b", ToLocationID: "c", MovementMethodID: "foot", ZeroTimeConnection: true},
		{ID: "last", FromLocationID: "c", ToLocationID: "d", DurationMinutes: 10, MovementMethodID: "foot"},
	}}
	zeroInitial, err := PrepareJourney(withZeroEdge, "zero-edge-journey", []string{"foot"})
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := AdvanceTo(withZeroEdge, zeroInitial, 10, []string{"foot"})
	if err != nil {
		t.Fatal(err)
	}
	if len(boundary.Snapshot.CompletedSegmentIDs) != 2 || boundary.Snapshot.CurrentSegmentID != "last" || boundary.Snapshot.CurrentLocationID != "c" {
		t.Fatalf("zero-time boundary progress is wrong: %+v", boundary.Snapshot)
	}
	boundaryRetry, err := AdvanceTo(withZeroEdge, boundary.Snapshot, 10, []string{"foot"})
	if err != nil || boundaryRetry.AddedMinutes != 0 {
		t.Fatalf("zero-time boundary retry failed: result=%+v err=%v", boundaryRetry, err)
	}
	if _, err := AdvanceTo(withZeroEdge, boundaryRetry.Snapshot, 11, []string{"foot"}); err != nil {
		t.Fatalf("could not continue past zero-time boundary: %v", err)
	}
}

func TestRouteValidatesSupportedMethodsConnectionsAndOverflow(t *testing.T) {
	route := testRoute()
	bad := route
	bad.Segments = append([]RouteSegment(nil), route.Segments...)
	bad.Segments[1].FromLocationID = "elsewhere"
	if _, _, ds := ValidateRoute(bad, []string{"walk"}); len(ds) == 0 {
		t.Fatal("disconnected route passed")
	}
	bad = route
	bad.Segments = append([]RouteSegment(nil), route.Segments...)
	bad.Segments[0].MovementMethodID = "ride"
	if _, _, ds := ValidateRoute(bad, []string{"walk"}); len(ds) == 0 {
		t.Fatal("unsupported movement method passed")
	}
	bad = route
	bad.Segments = append([]RouteSegment(nil), route.Segments...)
	bad.Segments[0].DurationMinutes = math.MaxInt64
	bad.Segments[1].DurationMinutes = 1
	_, _, ds := ValidateRoute(bad, []string{"walk"})
	if len(ds) == 0 {
		t.Fatal("duration overflow passed")
	}
	zero := RouteDefinition{ID: "z", Version: "1", OriginLocationID: "a", DestinationLocationID: "b", Segments: []RouteSegment{{ID: "edge", FromLocationID: "a", ToLocationID: "b", MovementMethodID: "walk", ZeroTimeConnection: true}}}
	s, err := PrepareJourney(zero, "j", []string{"walk"})
	if err != nil {
		t.Fatal(err)
	}
	advanced, err := AdvanceTo(zero, s, 0, []string{"walk"})
	if err != nil {
		t.Fatal(err)
	}
	if advanced.Snapshot.CurrentLocationID != "b" || advanced.RemainingMinutes != 0 {
		t.Fatalf("zero-time edge not consumed: %+v", advanced)
	}
}

func testDefinition() DungeonDefinition {
	return DungeonDefinition{SchemaVersion: 1, ID: "test", Version: "1", PackageID: "pkg", PackageVersion: "1", RulesetID: "rules", RulesetVersion: "1", Theme: "A test place.",
		Objectives: []Objective{{ID: "goal", Description: "Reach the exit."}}, Areas: []Area{{ID: "entry", Name: "Entry", Description: "The entrance."}, {ID: "middle", Name: "Middle", Description: "The room."}, {ID: "exit", Name: "Exit", Description: "The exit."}},
		Connections: []DungeonConnection{{ID: "one", FromAreaID: "entry", ToAreaID: "middle", Bidirectional: true, PlayerVisible: true}, {ID: "secret", FromAreaID: "middle", ToAreaID: "exit", Bidirectional: false, PlayerVisible: false}}, EntryAreaIDs: []string{"entry"}, EntryLocationIDs: []string{"test.outside"}, ExitAreaIDs: []string{"exit"},
		Challenges: []Challenge{{ID: "inspect", AreaID: "middle", ReferenceID: "challenge.inspect"}}, ContentRefs: []ContentRef{{ID: "resource", AreaID: "middle", Kind: "resource", ReferenceID: "content.herb", Quantity: 2}},
		CompletionPaths: []CompletionPath{{ID: "investigate", ObjectiveIDs: []string{"goal"}, ChallengeIDs: []string{"inspect"}, RequiredAbilityIDs: []string{"ability.inspect"}}, {ID: "fight", ObjectiveIDs: []string{"goal"}, ChallengeIDs: []string{}, RequiredAbilityIDs: []string{"ability.combat"}}}}
}

func TestDungeonDiagnosticsKeepSupportedPathWhenOptionalAbilityUnavailable(t *testing.T) {
	def := testDefinition()
	catalog := Catalog{PackageVersions: map[string]string{"pkg": "1"}, RulesetVersions: map[string]string{"rules": "1"}, AbilityIDs: []string{"ability.inspect", "ability.combat"}, ChallengeIDs: []string{"challenge.inspect"}, ContentIDs: []string{"content.herb"}}
	result := ValidateDungeon(def, catalog, []string{"ability.inspect"})
	if len(result.Diagnostics) != 0 || !result.Playable || !result.PathDiagnostics[0].Available || result.PathDiagnostics[1].Available || !reflect.DeepEqual(result.PathDiagnostics[1].MissingAbilityIDs, []string{"ability.combat"}) {
		t.Fatalf("unexpected ability diagnostics: %+v", result)
	}
	result = ValidateDungeon(def, catalog, []string{})
	if result.Playable {
		t.Fatal("dungeon reported playable without any available completion path")
	}
}

func TestDungeonGraphAllowsLoopsAndRejectsUnreachableExitAndBindingMismatch(t *testing.T) {
	def := testDefinition()
	def.Connections = append(def.Connections, DungeonConnection{ID: "loop", FromAreaID: "middle", ToAreaID: "entry", Bidirectional: false, PlayerVisible: true})
	if result := ValidateDungeon(def, Catalog{}, nil); len(result.ReachableExitAreaIDs) != 1 {
		t.Fatalf("loop prevented reachability: %+v", result)
	}
	def.ExitAreaIDs = []string{"entry"}
	def.EntryAreaIDs = []string{"middle"}
	def.Connections = []DungeonConnection{{ID: "one-way", FromAreaID: "entry", ToAreaID: "middle", PlayerVisible: true}}
	result := ValidateDungeon(def, Catalog{}, nil)
	if len(result.ReachableExitAreaIDs) != 0 {
		t.Fatalf("directed unreachable exit reported reachable: %+v", result)
	}
	binding := DungeonInstanceBinding{InstanceID: "instance", WorldID: "world", BranchID: "branch", DefinitionID: "test", DefinitionVersion: "1", PackageID: "pkg", PackageVersion: "1", RulesetID: "rules", RulesetVersion: "1"}
	scope := DungeonScope{WorldID: "world", BranchID: "branch"}
	if ds := ValidateInstanceBinding(binding, scope, def); len(ds) != 0 {
		t.Fatalf("valid exact binding rejected: %+v", ds)
	}
	binding.DefinitionVersion = "2"
	if ds := ValidateInstanceBinding(binding, scope, def); len(ds) == 0 {
		t.Fatal("definition version mismatch passed")
	}
}

func TestInstanceBindingRejectsForeignWorldAndBranch(t *testing.T) {
	def := testDefinition()
	scope := DungeonScope{WorldID: "trusted-world", BranchID: "trusted-branch"}
	binding := DungeonInstanceBinding{InstanceID: "instance", WorldID: scope.WorldID, BranchID: scope.BranchID, DefinitionID: def.ID, DefinitionVersion: def.Version, PackageID: def.PackageID, PackageVersion: def.PackageVersion, RulesetID: def.RulesetID, RulesetVersion: def.RulesetVersion}
	if ds := ValidateInstanceBinding(binding, scope, def); len(ds) != 0 {
		t.Fatalf("correct scope rejected: %+v", ds)
	}
	for _, tc := range []struct {
		name   string
		change func(*DungeonInstanceBinding)
		code   string
	}{
		{"foreign world", func(b *DungeonInstanceBinding) { b.WorldID = "wrong-world" }, "world_scope_mismatch"},
		{"foreign branch", func(b *DungeonInstanceBinding) { b.BranchID = "wrong-branch" }, "branch_scope_mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := binding
			tc.change(&candidate)
			ds := ValidateInstanceBinding(candidate, scope, def)
			if !hasDiagnostic(ds, tc.code) {
				t.Fatalf("missing %s: %+v", tc.code, ds)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		change func(*DungeonInstanceBinding)
	}{
		{"definition id", func(b *DungeonInstanceBinding) { b.DefinitionID = "other-definition" }},
		{"definition", func(b *DungeonInstanceBinding) { b.DefinitionVersion = "2" }},
		{"package id", func(b *DungeonInstanceBinding) { b.PackageID = "other-package" }},
		{"package", func(b *DungeonInstanceBinding) { b.PackageVersion = "2" }},
		{"ruleset id", func(b *DungeonInstanceBinding) { b.RulesetID = "other-ruleset" }},
		{"ruleset", func(b *DungeonInstanceBinding) { b.RulesetVersion = "2" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := binding
			tc.change(&candidate)
			if ds := ValidateInstanceBinding(candidate, scope, def); len(ds) == 0 {
				t.Fatal("mismatched content version passed")
			}
		})
	}
	invalidScope := DungeonScope{WorldID: "", BranchID: "trusted-branch"}
	if ds := ValidateInstanceBinding(binding, invalidScope, def); !hasDiagnostic(ds, "invalid_id") {
		t.Fatalf("empty expected scope not rejected: %+v", ds)
	}
}

func TestBoundKnownProjectionRejectsForeignScope(t *testing.T) {
	def := testDefinition()
	snapshot := KnowledgeSnapshot{CurrentAreaID: "entry", DiscoveredAreaIDs: []string{}, DiscoveredConnectionIDs: []string{}, KnownObjectiveIDs: []string{}, ConfirmedFacts: []KnownFact{}, Rumors: []KnownFact{}}
	scope := DungeonScope{WorldID: "trusted-world", BranchID: "trusted-branch"}
	binding := DungeonInstanceBinding{InstanceID: "instance", WorldID: "foreign-world", BranchID: scope.BranchID, DefinitionID: def.ID, DefinitionVersion: def.Version, PackageID: def.PackageID, PackageVersion: def.PackageVersion, RulesetID: def.RulesetID, RulesetVersion: def.RulesetVersion}
	view, ds := ProjectBoundKnownView(def, binding, scope, snapshot)
	if !hasDiagnostic(ds, "world_scope_mismatch") {
		t.Fatalf("foreign projection had no binding diagnostic: %+v", ds)
	}
	if view.CurrentArea.ID != "" || len(view.Areas) != 0 {
		t.Fatalf("foreign instance knowledge was projected: %+v", view)
	}
	binding.WorldID = scope.WorldID
	view, ds = ProjectBoundKnownView(def, binding, scope, snapshot)
	if len(ds) != 0 || view.CurrentArea.ID != "entry" {
		t.Fatalf("correctly bound projection failed: view=%+v diagnostics=%+v", view, ds)
	}
}

func TestDefinitionRequiresGoalsAndEveryCompletionPathMustReferenceOne(t *testing.T) {
	def := testDefinition()
	def.Objectives = []Objective{}
	for i := range def.CompletionPaths {
		def.CompletionPaths[i].ObjectiveIDs = []string{}
	}
	encoded, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	_, diagnostics, parseErr := ParseDungeonDefinition(encoded)
	if parseErr != nil {
		t.Fatalf("well-formed empty-goal definition should be semantic diagnostics: %v", parseErr)
	}
	if len(diagnostics) == 0 {
		t.Fatal("definition with no goals and empty completion paths had no semantic diagnostics")
	}
	def = testDefinition()
	def.CompletionPaths[0].ObjectiveIDs = []string{}
	encoded, err = json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	_, diagnostics, parseErr = ParseDungeonDefinition(encoded)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if len(diagnostics) == 0 {
		t.Fatal("completion path with no objective reference passed")
	}
	def = testDefinition()
	def.CompletionPaths[0].ObjectiveIDs = []string{"goal", "goal"}
	encoded, err = json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	_, diagnostics, parseErr = ParseDungeonDefinition(encoded)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if !hasDiagnostic(diagnostics, "duplicate_objective_reference") {
		t.Fatalf("duplicate objective reference was not diagnosed: %+v", diagnostics)
	}
	def.CompletionPaths = []CompletionPath{}
	encoded, err = json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	_, diagnostics, parseErr = ParseDungeonDefinition(encoded)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if !hasDiagnostic(diagnostics, "missing_completion_paths") {
		t.Fatalf("missing completion paths were not diagnosed: %+v", diagnostics)
	}
}

func TestSameMalformedDefinitionHasStableDiagnostic(t *testing.T) {
	data, err := json.Marshal(testDefinition())
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	delete(raw["areas"].([]any)[0].(map[string]any), "name")
	delete(raw["objectives"].([]any)[0].(map[string]any), "description")
	badBytes, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	bad := string(badBytes)
	first := ""
	for i := 0; i < 100; i++ {
		_, _, parseErr := ParseDungeonDefinition([]byte(bad))
		if parseErr == nil {
			t.Fatal("malformed definition unexpectedly parsed")
		}
		if i == 0 {
			first = parseErr.Error()
		} else if parseErr.Error() != first {
			t.Fatalf("diagnostic order changed for identical bytes: first=%q now=%q", first, parseErr.Error())
		}
	}
	if !strings.Contains(first, "objectives[0].description") {
		t.Fatalf("ordered diagnostic does not follow the stable objectives-first schema order: %q", first)
	}
}

func TestParserRejectsUnknownMissingAndNullFields(t *testing.T) {
	data, _ := json.Marshal(testDefinition())
	if _, _, err := ParseDungeonDefinition(data); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseDungeonDefinition([]byte(strings.Replace(string(data), `"id":"test",`, `"id":"test","unexpected":true,`, 1))); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("unknown field error=%v", err)
	}
	if _, _, err := ParseDungeonDefinition([]byte(strings.Replace(string(data), `"theme":"A test place.",`, "", 1))); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("missing field error=%v", err)
	}
	if _, _, err := ParseDungeonDefinition([]byte(strings.Replace(string(data), `"theme":"A test place.",`, `"theme":null,`, 1))); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("null field error=%v", err)
	}
}

func TestKnownProjectionDoesNotInferLinksAndDoesNotMutateSnapshot(t *testing.T) {
	def := testDefinition()
	snapshot := KnowledgeSnapshot{CurrentAreaID: "entry", KnownObjectiveIDs: []string{"goal"}, DiscoveredAreaIDs: []string{"middle", "exit"}, DiscoveredConnectionIDs: []string{"one", "secret"},
		ConfirmedFacts: []KnownFact{{ID: "fact", Text: "Observed", PubliclyDisplayable: true}, {ID: "private", Text: "Hidden", PubliclyDisplayable: false}}, Rumors: []KnownFact{{ID: "rumor", Text: "Unconfirmed", PubliclyDisplayable: true}}}
	before := snapshot
	view, ds := ProjectKnownView(def, snapshot)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	if len(view.Connections) != 1 || view.Connections[0].ID != "one" {
		t.Fatalf("hidden or undiscovered connection leaked: %+v", view.Connections)
	}
	if len(view.ConfirmedFacts) != 1 || len(view.Rumors) != 1 {
		t.Fatalf("fact/rumor projection wrong: %+v", view)
	}
	if !reflect.DeepEqual(snapshot, before) {
		t.Fatal("projection mutated caller snapshot")
	}
}

func TestKnownAreaDoesNotImplyDiscoveredConnectionAndRoundTripProjection(t *testing.T) {
	def := testDefinition()
	s := KnowledgeSnapshot{CurrentAreaID: "entry", DiscoveredAreaIDs: []string{"middle"}, DiscoveredConnectionIDs: []string{}, ConfirmedFacts: []KnownFact{}, Rumors: []KnownFact{}}
	view, ds := ProjectKnownView(def, s)
	if len(ds) != 0 || len(view.Connections) != 0 {
		t.Fatalf("area knowledge implied a link: %+v %+v", view, ds)
	}
	raw, _ := json.Marshal(s)
	var restored KnowledgeSnapshot
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	again, _ := ProjectKnownView(def, restored)
	if !reflect.DeepEqual(view, again) {
		t.Fatal("JSON round-trip changed projection")
	}
}

func hasDiagnostic(items []Diagnostic, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}
