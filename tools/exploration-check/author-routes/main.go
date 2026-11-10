package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	catalogFlag := flag.String("catalog", "work/luna6-world-content/static-world-catalog.json", "read-only static catalog path; test overrides are limited to the Q1 output directory")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(2)
	}
	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not determine repository root")
		os.Exit(2)
	}
	catalogPath := filepath.Clean(filepath.FromSlash(*catalogFlag))
	result := conversion{Report: Report{SchemaVersion: 1, Contract: "denova author-route candidate v1 (not installed game data)", NotInstallable: true, Counts: map[string]int{}, Demonstrations: []RouteDemonstration{}, Diagnostics: []Diagnostic{}}, Candidates: []CandidateRoute{}}
	if !catalogOverrideAllowed(catalogPath, repoRoot) {
		result.Report.Diagnostics = append(result.Report.Diagnostics, Diagnostic{"catalog_path_not_allowed", "catalog", "catalog input must be the repository static catalog or a temporary Q1-local test input"})
	} else {
		result = loadConversion(repoRoot, filepath.ToSlash(catalogPath))
	}
	if len(result.Candidates) == 0 && len(result.Report.Diagnostics) == 0 {
		result.Report.Diagnostics = append(result.Report.Diagnostics, Diagnostic{"no_candidate_routes", "routes", "conversion produced no reviewable route candidates"})
	}
	result.Report.Valid = len(result.Report.Diagnostics) == 0 && len(result.Candidates) > 0
	if result.Report.Diagnostics == nil {
		result.Report.Diagnostics = []Diagnostic{}
	}
	if result.Report.Demonstrations == nil {
		result.Report.Demonstrations = []RouteDemonstration{}
	}
	outputDir := filepath.Join(repoRoot, "work", "luna6-exploration", "q1")
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "could not create Q1 output directory:", err)
		os.Exit(2)
	}
	candidate := CandidateFile{SchemaVersion: 1, Contract: result.Report.Contract, Valid: result.Report.Valid, NotInstallable: true, Hashes: result.Report.Hashes, Routes: result.Candidates, Diagnostics: result.Report.Diagnostics}
	if candidate.Routes == nil {
		candidate.Routes = []CandidateRoute{}
	}
	for _, output := range []struct {
		name  string
		value any
	}{{"candidate-routes.json", candidate}, {"report.json", result.Report}, {"diagnostics.json", result.Report.Diagnostics}} {
		if err := writeJSON(filepath.Join(outputDir, output.name), output.value); err != nil {
			fmt.Fprintln(os.Stderr, "could not write Q1 output:", err)
			os.Exit(2)
		}
	}
	if !result.Report.Valid {
		fmt.Fprintf(os.Stderr, "author route conversion rejected input with %d diagnostic(s); report: work/luna6-exploration/q1/report.json\n", len(result.Report.Diagnostics))
		os.Exit(2)
	}
	fmt.Println("author route candidate validation completed; not installable")
}
