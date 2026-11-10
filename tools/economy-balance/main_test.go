package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type benchmarkReport struct {
	Candidate struct {
		WealthBenchmark int64 `json:"wealth_benchmark"`
	} `json:"candidate"`
	PurchasingPower struct {
		BenchmarkAmount                int64  `json:"benchmark_amount"`
		LowRecoveryDosesAtMinimumPrice *int64 `json:"low_recovery_doses_at_minimum_price"`
		LowRecoveryDosesAtMaximumPrice *int64 `json:"low_recovery_doses_at_maximum_price"`
		MidHighRecoveryDosesAtMinPrice *int64 `json:"mid_high_recovery_doses_at_minimum_price"`
		MidHighRecoveryDosesAtMaxPrice *int64 `json:"mid_high_recovery_doses_at_maximum_price"`
		ArtifactQuantityAtMinPrice     *int64 `json:"common_low_artifact_quantity_at_min_price"`
		ArtifactQuantityAtMaxPrice     *int64 `json:"common_low_artifact_quantity_at_max_price"`
	} `json:"purchasing_power"`
	Diagnostics []struct {
		Code string `json:"code"`
		Path string `json:"path"`
	} `json:"diagnostics"`
}

func TestCLIReportsConfiguredBenchmarkAndRejectsUnavailableQuantityMetrics(t *testing.T) {
	candidateBytes, err := os.ReadFile(filepath.Join("testdata", "candidate.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Chdir(root)
	// Source hashes are part of the CLI contract. All cited inputs are explicit
	// synthetic fixtures, never the user's historical author documents.
	for _, reference := range append([]string{"synthetic-design.md"}, supportingSources...) {
		file := filepath.Join(root, filepath.FromSlash(reference))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("Synthetic CLI test source; not installed game content.\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	revisionDirectory := t.TempDir()

	for _, testCase := range []struct {
		name            string
		benchmark       string
		wantExitCode    int
		wantLowPrice    int64
		wantHighPrice   int64
		wantUnavailable bool
	}{
		{name: "benchmark-100", benchmark: "100", wantExitCode: 0, wantLowPrice: 50, wantHighPrice: 33},
		{name: "benchmark-200", benchmark: "200", wantExitCode: 0, wantLowPrice: 100, wantHighPrice: 66},
		{name: "quantity-over-limit", benchmark: "1000000000000", wantExitCode: 2, wantUnavailable: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			candidatePath := filepath.Join(revisionDirectory, testCase.name+".json")
			input := string(candidateBytes)
			if testCase.benchmark != "100" {
				input = strings.Replace(input, `"wealth_benchmark": 100`, `"wealth_benchmark": `+testCase.benchmark, 1)
			}
			if !strings.Contains(input, `"wealth_benchmark": `+testCase.benchmark+`,`) {
				t.Fatal("failed to prepare benchmark candidate copy")
			}
			if err := os.WriteFile(candidatePath, []byte(input), 0o644); err != nil {
				t.Fatal(err)
			}
			outputDirectory := filepath.Join(revisionDirectory, testCase.name)
			stderr := &bytes.Buffer{}
			args := []string{"--input", candidatePath, "--output", outputDirectory}
			exitCode := execute(args, stderr)
			if exitCode != testCase.wantExitCode {
				t.Fatalf("CLI exit code = %d, want %d; stderr: %s", exitCode, testCase.wantExitCode, stderr.String())
			}

			reportBytes, err := os.ReadFile(filepath.Join(outputDirectory, "economy-balance-report.json"))
			if err != nil {
				t.Fatalf("read generated report: %v", err)
			}
			var report benchmarkReport
			if err := json.Unmarshal(reportBytes, &report); err != nil {
				t.Fatalf("decode generated report: %v", err)
			}
			benchmark, err := parseBenchmark(testCase.benchmark)
			if err != nil {
				t.Fatal(err)
			}
			if report.Candidate.WealthBenchmark != benchmark || report.PurchasingPower.BenchmarkAmount != benchmark {
				t.Fatalf("report benchmark candidate=%d purchasing_power=%d, want %d", report.Candidate.WealthBenchmark, report.PurchasingPower.BenchmarkAmount, benchmark)
			}
			markdownBytes, err := os.ReadFile(filepath.Join(outputDirectory, "economy-balance-report.md"))
			if err != nil {
				t.Fatal(err)
			}
			markdown := string(markdownBytes)
			if testCase.wantUnavailable {
				if report.PurchasingPower.LowRecoveryDosesAtMinimumPrice != nil || report.PurchasingPower.LowRecoveryDosesAtMaximumPrice != nil || report.PurchasingPower.MidHighRecoveryDosesAtMinPrice != nil || report.PurchasingPower.MidHighRecoveryDosesAtMaxPrice != nil || report.PurchasingPower.ArtifactQuantityAtMinPrice != nil || report.PurchasingPower.ArtifactQuantityAtMaxPrice != nil {
					t.Fatalf("failed quantity metrics must be null, report=%+v", report.PurchasingPower)
				}
				wantDiagnosticPaths := map[string]bool{
					"purchasing_power.low_recovery_doses_at_minimum_price":           false,
					"purchasing_power.low_recovery_doses_at_maximum_price":           false,
					"purchasing_power.mid_high_recovery_doses_at_minimum_price":      false,
					"purchasing_power.mid_high_recovery_doses_at_maximum_price":      false,
					"purchasing_power.common_low_artifact_quantity_at_minimum_price": false,
					"purchasing_power.common_low_artifact_quantity_at_maximum_price": false,
				}
				for _, diagnostic := range report.Diagnostics {
					if diagnostic.Code == "purchasing_power_calculation_error" {
						if _, exists := wantDiagnosticPaths[diagnostic.Path]; exists {
							wantDiagnosticPaths[diagnostic.Path] = true
						}
					}
				}
				for path, found := range wantDiagnosticPaths {
					if !found {
						t.Errorf("missing calculation diagnostic for %s", path)
					}
				}
				if len(report.Diagnostics) != len(wantDiagnosticPaths) || !strings.Contains(markdown, "1000000000000-base-unit reference buys unavailable") || strings.Contains(markdown, "reference buys 0–0") {
					t.Fatalf("unavailable benchmark result was not reported clearly: diagnostics=%+v markdown excerpt=%q", report.Diagnostics, markdown)
				}
			} else {
				if len(report.Diagnostics) != 0 {
					t.Fatalf("normal benchmark produced diagnostics: %+v", report.Diagnostics)
				}
				if report.PurchasingPower.LowRecoveryDosesAtMinimumPrice == nil || *report.PurchasingPower.LowRecoveryDosesAtMinimumPrice != testCase.wantLowPrice || report.PurchasingPower.LowRecoveryDosesAtMaximumPrice == nil || *report.PurchasingPower.LowRecoveryDosesAtMaximumPrice != testCase.wantHighPrice {
					t.Fatalf("low recovery purchase range at benchmark %d = %+v / %+v", benchmark, report.PurchasingPower.LowRecoveryDosesAtMaximumPrice, report.PurchasingPower.LowRecoveryDosesAtMinimumPrice)
				}
				wantRange := fmt.Sprintf("%d-base-unit reference buys %d–%d low recovery doses", benchmark, testCase.wantHighPrice, testCase.wantLowPrice)
				if !strings.Contains(markdown, wantRange) {
					t.Fatalf("markdown does not reflect input benchmark; want %q in %q", wantRange, markdown)
				}
				if !strings.Contains(markdown, "## Fixed quote examples (independent of purchasing-power benchmark)") {
					t.Fatal("fixed examples are not visibly separated from the input benchmark")
				}
			}
		})
	}
}

func TestCLIReportsMissingInputWithoutClaimingValidation(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	output := filepath.Join(root, "report")
	stderr := &bytes.Buffer{}
	if code := execute([]string{"--input", "missing.json", "--output", output}, stderr); code != 2 {
		t.Fatalf("missing input exit = %d, want 2", code)
	}
	data, err := os.ReadFile(filepath.Join(output, "economy-balance-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report benchmarkReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Code == "candidate_read_error" {
			return
		}
	}
	t.Fatalf("missing candidate diagnostic was lost: %+v", report.Diagnostics)
}

func parseBenchmark(value string) (int64, error) {
	var result int64
	_, err := fmt.Sscan(value, &result)
	return result, err
}
