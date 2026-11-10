package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"ta/internal/exploration"
)

type SourceReport struct {
	Path                     string                       `json:"path"`
	SHA256                   string                       `json:"sha256"`
	Schema                   string                       `json:"schema"`
	Valid                    bool                         `json:"valid"`
	Playable                 bool                         `json:"playable"`
	Diagnostics              []exploration.Diagnostic     `json:"diagnostics"`
	PathDiagnostics          []exploration.PathDiagnostic `json:"path_diagnostics,omitempty"`
	KnownView                *exploration.KnownView       `json:"known_view,omitempty"`
	SecretConnectionAbsent   bool                         `json:"secret_connection_absent"`
	KnowledgeRoundTripStable bool                         `json:"knowledge_round_trip_stable"`
	RoundTripIsGameRecovery  bool                         `json:"round_trip_is_game_recovery"`
}
type RouteReport struct {
	TotalMinutes        int64                       `json:"total_minutes"`
	FirstTargetElapsed  int64                       `json:"first_target_elapsed"`
	FirstPosition       exploration.JourneyPosition `json:"first_position"`
	FirstAddedMinutes   int64                       `json:"first_added_minutes"`
	SecondTargetElapsed int64                       `json:"second_target_elapsed"`
	SecondAddedMinutes  int64                       `json:"second_added_minutes"`
	RetryAddedMinutes   int64                       `json:"retry_added_minutes"`
	RemainingMinutes    int64                       `json:"remaining_minutes"`
	Done                bool                        `json:"done"`
}
type Report struct {
	Contract string         `json:"contract"`
	Sources  []SourceReport `json:"sources"`
	Route    RouteReport    `json:"route"`
}

func main() {
	input := flag.String("input", "", "validate one definition JSON file instead of the three bundled synthetic samples")
	flag.Parse()
	paths := []string{}
	if *input != "" {
		paths = append(paths, *input)
	} else {
		for _, name := range []string{"investigation-cave.json", "resource-site.json", "multi-party-ruin.json"} {
			paths = append(paths, filepath.Join("tools", "exploration-check", "samples", name))
		}
	}
	report := Report{Contract: "denova exploration local candidate schema v1 (not an installed Story Pack schema)", Sources: []SourceReport{}}
	failed := false
	for _, path := range paths {
		entry, err := analyze(path)
		if err != nil {
			report.Sources = append(report.Sources, SourceReport{Path: path, Schema: report.Contract, Valid: false, Diagnostics: []exploration.Diagnostic{{Code: "input_error", Path: path, Message: err.Error()}}})
			failed = true
			continue
		}
		report.Sources = append(report.Sources, entry)
		if !entry.Valid {
			failed = true
		}
	}
	route, err := routeDemo()
	if err != nil {
		fmt.Fprintln(os.Stderr, "route demo failed:", err)
		os.Exit(2)
	}
	report.Route = route
	sort.Slice(report.Sources, func(i, j int) bool { return report.Sources[i].Path < report.Sources[j].Path })
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if _, err := os.Stdout.Write(out.Bytes()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if failed {
		os.Exit(2)
	}
}

func analyze(path string) (SourceReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SourceReport{}, err
	}
	sum := sha256.Sum256(data)
	def, _, err := exploration.ParseDungeonDefinition(data)
	if err != nil {
		return SourceReport{Path: path, SHA256: hex.EncodeToString(sum[:]), Schema: "denova exploration local candidate schema v1", Valid: false, Playable: false,
			Diagnostics: []exploration.Diagnostic{{Code: "invalid_definition", Path: path, Message: err.Error()}}, PathDiagnostics: []exploration.PathDiagnostic{}}, nil
	}
	cat := exploration.Catalog{PackageVersions: map[string]string{"sample-world": "1"}, RulesetVersions: map[string]string{"sample-rules": "1"},
		AbilityIDs:   []string{"ability.hearing", "ability.swordplay", "ability.fieldcraft", "ability.naturalist", "ability.coordination", "ability.lever-work", "ability.climbing", "ability.teamwork"},
		ChallengeIDs: []string{"challenge.echo-test", "challenge.counterweights", "challenge.beam-crossing"}, ContentIDs: []string{"resource.saltgrass", "content.seal-cache"}}
	available := []string{"ability.hearing", "ability.fieldcraft", "ability.naturalist", "ability.coordination", "ability.lever-work", "ability.climbing", "ability.teamwork"}
	validation := exploration.ValidateDungeon(def, cat, available)
	diagnostics := append([]exploration.Diagnostic(nil), validation.Diagnostics...)
	if len(def.Areas) == 0 {
		return SourceReport{Path: path, SHA256: hex.EncodeToString(sum[:]), Schema: "denova exploration local candidate schema v1", Valid: false, Playable: false, Diagnostics: diagnostics, PathDiagnostics: validation.PathDiagnostics}, nil
	}
	first := def.Areas[0]
	knowledge := exploration.KnowledgeSnapshot{CurrentAreaID: first.ID, KnownObjectiveIDs: []string{}, DiscoveredAreaIDs: []string{}, DiscoveredConnectionIDs: []string{}, ConfirmedFacts: []exploration.KnownFact{}, Rumors: []exploration.KnownFact{}}
	if len(def.Areas) > 1 {
		knowledge.DiscoveredAreaIDs = append(knowledge.DiscoveredAreaIDs, def.Areas[1].ID)
	}
	for _, c := range def.Connections {
		if c.PlayerVisible && c.FromAreaID == first.ID && len(knowledge.DiscoveredAreaIDs) > 0 && c.ToAreaID == knowledge.DiscoveredAreaIDs[0] {
			knowledge.DiscoveredConnectionIDs = append(knowledge.DiscoveredConnectionIDs, c.ID)
		}
	}
	knowledge.ConfirmedFacts = []exploration.KnownFact{{ID: "fact-entry", Text: "The entry has been reached.", PubliclyDisplayable: true}}
	knowledge.Rumors = []exploration.KnownFact{{ID: "rumor-signal", Text: "Someone heard a signal beyond the known path.", PubliclyDisplayable: true}}
	scope := exploration.DungeonScope{WorldID: "sample-world", BranchID: "sample-branch"}
	binding := exploration.DungeonInstanceBinding{InstanceID: "instance:" + def.ID, WorldID: scope.WorldID, BranchID: scope.BranchID, DefinitionID: def.ID, DefinitionVersion: def.Version, PackageID: def.PackageID, PackageVersion: def.PackageVersion, RulesetID: def.RulesetID, RulesetVersion: def.RulesetVersion}
	view, viewDiagnostics := exploration.ProjectBoundKnownView(def, binding, scope, knowledge)
	diagnostics = append(diagnostics, viewDiagnostics...)
	rawKnowledge, err := json.Marshal(knowledge)
	if err != nil {
		return SourceReport{}, err
	}
	var restored exploration.KnowledgeSnapshot
	if err := json.Unmarshal(rawKnowledge, &restored); err != nil {
		return SourceReport{}, err
	}
	viewAgain, roundTripDiagnostics := exploration.ProjectBoundKnownView(def, binding, scope, restored)
	diagnostics = append(diagnostics, roundTripDiagnostics...)
	secretAbsent := true
	knownConnections := map[string]bool{}
	for _, c := range view.Connections {
		knownConnections[c.ID] = true
	}
	for _, c := range def.Connections {
		if !c.PlayerVisible && !knownConnections[c.ID] {
			continue
		}
		if !c.PlayerVisible && knownConnections[c.ID] {
			secretAbsent = false
		}
	}
	return SourceReport{Path: path, SHA256: hex.EncodeToString(sum[:]), Schema: "denova exploration local candidate schema v1", Valid: len(diagnostics) == 0 && validation.Playable, Playable: validation.Playable,
		Diagnostics: diagnostics, PathDiagnostics: validation.PathDiagnostics, KnownView: &view, SecretConnectionAbsent: secretAbsent, KnowledgeRoundTripStable: len(roundTripDiagnostics) == 0 && reflect.DeepEqual(view, viewAgain), RoundTripIsGameRecovery: false}, nil
}

func routeDemo() (RouteReport, error) {
	route := exploration.RouteDefinition{ID: "demo-route", Version: "1", OriginLocationID: "town", DestinationLocationID: "shrine", Segments: []exploration.RouteSegment{
		{ID: "town-crossing", FromLocationID: "town", ToLocationID: "crossroads", DurationMinutes: 120, MovementMethodID: "walk"},
		{ID: "crossing-pass", FromLocationID: "crossroads", ToLocationID: "pass", DurationMinutes: 180, MovementMethodID: "walk"},
		{ID: "pass-shrine", FromLocationID: "pass", ToLocationID: "shrine", DurationMinutes: 120, MovementMethodID: "walk"},
	}}
	snapshot, err := exploration.PrepareJourney(route, "demo-journey", []string{"walk"})
	if err != nil {
		return RouteReport{}, err
	}
	first, err := exploration.AdvanceTo(route, snapshot, 135, []string{"walk"})
	if err != nil {
		return RouteReport{}, err
	}
	second, err := exploration.AdvanceTo(route, first.Snapshot, 180, []string{"walk"})
	if err != nil {
		return RouteReport{}, err
	}
	retry, err := exploration.AdvanceTo(route, second.Snapshot, 180, []string{"walk"})
	if err != nil {
		return RouteReport{}, err
	}
	return RouteReport{TotalMinutes: snapshot.TotalMinutes, FirstTargetElapsed: 135, FirstPosition: first.Position, FirstAddedMinutes: first.AddedMinutes, SecondTargetElapsed: 180, SecondAddedMinutes: second.AddedMinutes, RetryAddedMinutes: retry.AddedMinutes, RemainingMinutes: second.RemainingMinutes, Done: second.Done}, nil
}
