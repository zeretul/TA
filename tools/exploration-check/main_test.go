package main

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestBundledSyntheticDefinitionsAndProjection(t *testing.T) {
	for _, name := range []string{"investigation-cave.json", "resource-site.json", "multi-party-ruin.json"} {
		t.Run(name, func(t *testing.T) {
			_, sourceFile, _, _ := runtime.Caller(0)
			report, err := analyze(filepath.Join(filepath.Dir(sourceFile), "samples", name))
			if err != nil {
				t.Fatal(err)
			}
			if !report.Valid || !report.Playable {
				t.Fatalf("sample failed validation: %+v", report.Diagnostics)
			}
			if !report.SecretConnectionAbsent {
				t.Fatal("secret route appeared in player view")
			}
			if !report.KnowledgeRoundTripStable || report.RoundTripIsGameRecovery {
				t.Fatalf("knowledge round-trip report inaccurate: %+v", report)
			}
			if len(report.KnownView.Rumors) != 1 || len(report.KnownView.ConfirmedFacts) != 1 {
				t.Fatal("confirmed facts and rumors were not kept separate")
			}
			if name == "investigation-cave.json" && (len(report.PathDiagnostics) != 2 || !report.PathDiagnostics[0].Available || report.PathDiagnostics[1].Available) {
				t.Fatalf("optional combat path availability was not reported independently: %+v", report.PathDiagnostics)
			}
		})
	}
}

func TestRouteDemoShowsCumulativeTimeAndNoDoubleCharge(t *testing.T) {
	report, err := routeDemo()
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalMinutes != 420 || report.FirstAddedMinutes != 135 || report.SecondAddedMinutes != 45 || report.RetryAddedMinutes != 0 || report.RemainingMinutes != 240 {
		t.Fatalf("unexpected route demonstration: %+v", report)
	}
}
