package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"ta/internal/exploration"
)

//go:embed samples/north-road.json samples/synthetic-archive-cave.json
var sampleFiles embed.FS

type checkResult struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	Expected string `json:"expected"`
	Observed string `json:"observed"`
}

type suiteReport struct {
	SchemaVersion  int                     `json:"schema_version"`
	Contract       string                  `json:"contract"`
	SyntheticOnly  bool                    `json:"synthetic_only"`
	NotInstallable bool                    `json:"not_installable"`
	Valid          bool                    `json:"valid"`
	InputSHA256    map[string]string       `json:"input_sha256"`
	DefinitionHash map[string]string       `json:"definition_hash"`
	Journey        journeyScenarioReport   `json:"journey"`
	Isolation      isolationScenarioReport `json:"scope_isolation"`
	Dungeon        dungeonScenarioReport   `json:"dungeon_round_trip"`
	Checks         []checkResult           `json:"checks"`
	Diagnostics    []string                `json:"diagnostics"`
}

func runSuite() suiteReport {
	report := suiteReport{
		SchemaVersion: 1, Contract: "synthetic exploration sequence checks v1", SyntheticOnly: true, NotInstallable: true,
		InputSHA256: map[string]string{}, DefinitionHash: map[string]string{}, Checks: []checkResult{}, Diagnostics: []string{},
	}
	roadBytes, err := sampleFiles.ReadFile("samples/north-road.json")
	if err != nil {
		return suiteError(report, "could not read synthetic route sample", err)
	}
	dungeonBytes, err := sampleFiles.ReadFile("samples/synthetic-archive-cave.json")
	if err != nil {
		return suiteError(report, "could not read synthetic dungeon sample", err)
	}
	report.InputSHA256["north_road"] = digest(roadBytes)
	report.InputSHA256["synthetic_archive_cave"] = digest(dungeonBytes)

	var route exploration.RouteDefinition
	if err := json.Unmarshal(roadBytes, &route); err != nil {
		return suiteError(report, "could not parse synthetic route sample", err)
	}
	total, routeHash, routeDiagnostics := exploration.ValidateRoute(route, []string{"walk", "pass-through"})
	report.DefinitionHash["north_road"] = routeHash
	recordCheck(&report, "route_definition_valid", len(routeDiagnostics) == 0, "core route validation has no diagnostics", fmt.Sprintf("total_minutes=%d diagnostics=%v", total, routeDiagnostics))
	if len(routeDiagnostics) != 0 {
		report.Diagnostics = append(report.Diagnostics, "synthetic route is invalid")
		return finishReport(report)
	}

	definition, diagnostics, err := exploration.ParseDungeonDefinition(dungeonBytes)
	if err != nil || len(diagnostics) != 0 {
		return suiteError(report, "could not parse synthetic dungeon sample", errors.Join(err, diagnosticError(diagnostics)))
	}
	dungeonHash, err := exploration.DungeonDefinitionContentHash(definition)
	if err != nil {
		return suiteError(report, "could not hash synthetic dungeon definition", err)
	}
	report.DefinitionHash["synthetic_archive_cave"] = dungeonHash
	recordCheck(&report, "dungeon_definition_valid", exploration.ValidateDungeon(definition, exploration.Catalog{}, nil).Playable, "synthetic dungeon is playable under core validation", "validated by exploration.ValidateDungeon")

	report.Journey = runJourneyScenario(route, total, &report)
	report.Isolation = runIsolationScenario(definition, &report)
	report.Dungeon = runDungeonScenario(definition, &report)
	return finishReport(report)
}

func finishReport(report suiteReport) suiteReport {
	report.Valid = len(report.Diagnostics) == 0
	for _, check := range report.Checks {
		report.Valid = report.Valid && check.Passed
	}
	return report
}

func suiteError(report suiteReport, message string, err error) suiteReport {
	if err != nil {
		report.Diagnostics = append(report.Diagnostics, message+": "+err.Error())
	} else {
		report.Diagnostics = append(report.Diagnostics, message)
	}
	return finishReport(report)
}

func recordCheck(report *suiteReport, name string, passed bool, expected, observed string) {
	report.Checks = append(report.Checks, checkResult{Name: name, Passed: passed, Expected: expected, Observed: observed})
	if !passed {
		report.Diagnostics = append(report.Diagnostics, name+" failed")
	}
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func diagnosticError(items []exploration.Diagnostic) error {
	if len(items) == 0 {
		return nil
	}
	return fmt.Errorf("definition diagnostics: %v", items)
}

func jsonRoundTrip[T any](value T) (T, error) {
	data, err := json.Marshal(value)
	if err != nil {
		var zero T
		return zero, err
	}
	var restored T
	if err := json.Unmarshal(data, &restored); err != nil {
		var zero T
		return zero, err
	}
	return restored, nil
}

func sameJourneySnapshots(want, got []exploration.JourneySnapshot) error {
	if !reflect.DeepEqual(want, got) {
		return fmt.Errorf("journey snapshots differ")
	}
	return nil
}

func sameJourneyStops(want, got []journeyStop) error {
	if !reflect.DeepEqual(want, got) {
		return fmt.Errorf("journey advances or snapshots differ")
	}
	return nil
}
