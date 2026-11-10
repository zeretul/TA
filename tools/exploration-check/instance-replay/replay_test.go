package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"ta/internal/exploration"
)

func TestSyntheticReplayCoversLifecycleAndExpectedRejectionsDeterministically(t *testing.T) {
	_, sourceFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	input, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(defaultInput)))
	if err != nil {
		t.Fatal(err)
	}
	definition, diagnostics, err := exploration.ParseDungeonDefinition(input)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("synthetic input did not parse: diagnostics=%+v err=%v", diagnostics, err)
	}
	first := executeReplay(definition, defaultInput, input)
	second := executeReplay(definition, defaultInput, input)
	if !first.Valid || len(first.Diagnostics) != 0 || len(first.Steps) < 9 || len(first.ExpectedRejections) < 10 {
		t.Fatalf("replay did not complete its full contract: %+v", first)
	}
	if first.FinalSnapshot.Presence != exploration.InstanceInside {
		t.Fatalf("replay did not finish inside the actual re-entry area: %+v", first.FinalSnapshot)
	}
	for _, rejected := range first.ExpectedRejections {
		if !rejected.Rejected || !rejected.InputUnchanged || rejected.ObservedError == "" && len(rejected.DiagnosticCodes) == 0 {
			t.Fatalf("negative case was not actually rejected without mutation: %+v", rejected)
		}
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("same synthetic input produced different replay output")
	}
	if contains(first.Steps[0].KnownAreaIDs, "sealed_annex") || contains(first.Steps[0].KnownConnectionIDs, "sealed_door") {
		t.Fatalf("initial step leaked hidden definition: %+v", first.Steps[0])
	}
	for _, step := range first.Steps {
		if step.Presence == "outside" && step.CurrentAreaID != "" {
			t.Fatalf("outside replay step claims a current interior area: %+v", step)
		}
	}
}

func TestMalformedInputHasActualInvalidReportAndSafeInputPathGuard(t *testing.T) {
	bad := parseAndExecute([]byte(`{"schema_version":`), "work/luna6-exploration/q2/malformed.json")
	if bad.Valid || len(bad.Diagnostics) != 1 || bad.Diagnostics[0].Code != "definition_parse_failed" {
		t.Fatalf("malformed JSON was not clearly rejected: %+v", bad)
	}
	_, sourceFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	for _, invalid := range []string{"设计稿/自制-探索与行路.md", "work/luna6-exploration/q2/../q1/report.json"} {
		if _, _, err := resolveInput(repoRoot, invalid); err == nil {
			t.Fatalf("input path escaped the allowed sample/Q2 boundary: %q", invalid)
		}
	}
	if _, _, err := resolveInput(repoRoot, defaultInput); err != nil {
		t.Fatalf("default synthetic sample was rejected: %v", err)
	}
}
