// Command economy-balance reports calculations from the independent economy
// package. It never writes the candidate source or performs a game transaction.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ta/internal/economy"
)

const (
	defaultInput  = "设计稿/资源清单/凡人人界_经济尺度候选_20261015.json"
	defaultOutput = "work/luna6-economy"
	reportKind    = "independent_economy_candidate_analysis_not_runtime_validation"
)

var supportingSources = []string{
	"设计稿/自制-灵石经济与简单交易.md",
	"设计稿/自制-修仙数值规则.md",
	"设计稿/自制-技能与施展.md",
	"设计稿/自制-装备与物品品级.md",
	"设计稿/自制-功法与技能内容.md",
	"设计稿/自制-事件与任务.md",
	"设计稿/自制-剧本包.md",
	"设计稿/世界包与玩法模块_方案总览.md",
	"设计稿/凡人人界_修仙事件素材与探索战斗任务.md",
}

type sourceHash struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type candidateSummary struct {
	ID               string `json:"id,omitempty"`
	DesignVersion    string `json:"design_version,omitempty"`
	Status           string `json:"status,omitempty"`
	CurrencyID       string `json:"currency_id,omitempty"`
	AmountLimit      int64  `json:"amount_limit,omitempty"`
	QuantityLimit    int64  `json:"quantity_limit,omitempty"`
	WealthBenchmark  int64  `json:"wealth_benchmark,omitempty"`
	ValidPriceBands  int    `json:"valid_price_bands"`
	ValidWealthBands int    `json:"valid_wealth_reference_bands"`
	ValidRewardBands int    `json:"valid_reward_reference_bands"`
}

type unitBandSummary struct {
	ID         string `json:"id"`
	CurrencyID string `json:"currency_id"`
	UnitID     string `json:"unit_id"`
	Min        int64  `json:"min"`
	Max        int64  `json:"max"`
}

type quoteExample struct {
	Name            string `json:"name"`
	BandID          string `json:"band_id"`
	BaseUnitPrice   int64  `json:"base_unit_price"`
	Quantity        int64  `json:"quantity"`
	UnitID          string `json:"unit_id"`
	MarketPermille  int64  `json:"market_factor_permille"`
	BuybackPermille int64  `json:"buyback_factor_permille"`
	RetailUnit      int64  `json:"retail_unit_price"`
	RetailTotal     int64  `json:"retail_total"`
	BuybackUnit     int64  `json:"buyback_unit_price"`
	BuybackTotal    int64  `json:"buyback_total"`
}

type purchasingPower struct {
	CurrencyID                     string `json:"currency_id"`
	BenchmarkAmount                int64  `json:"benchmark_amount"`
	LowRecoveryDosesAtMinimumPrice *int64 `json:"low_recovery_doses_at_minimum_price"`
	LowRecoveryDosesAtMaximumPrice *int64 `json:"low_recovery_doses_at_maximum_price"`
	MidHighRecoveryDosesAtMinPrice *int64 `json:"mid_high_recovery_doses_at_minimum_price"`
	MidHighRecoveryDosesAtMaxPrice *int64 `json:"mid_high_recovery_doses_at_maximum_price"`
	ShortTaskNetDosesLowCase       *int64 `json:"short_task_net_doses_low_case"`
	ShortTaskNetDosesHighCase      *int64 `json:"short_task_net_doses_high_case"`
	ArtifactQuantityAtMinPrice     *int64 `json:"common_low_artifact_quantity_at_min_price"`
	ArtifactQuantityAtMaxPrice     *int64 `json:"common_low_artifact_quantity_at_max_price"`
	ArtifactCostShareAtMinPermille *int64 `json:"artifact_cost_share_at_min_price_permille"`
	ArtifactCostShareAtMaxPermille *int64 `json:"artifact_cost_share_at_max_price_permille"`
}

type actionCost struct {
	DoseCount            int64  `json:"dose_count"`
	MinimumSpend         int64  `json:"minimum_spend"`
	MaximumSpend         int64  `json:"maximum_spend"`
	MajorActionsConsumed int64  `json:"major_actions_consumed"`
	ExistingRuleNote     string `json:"existing_rule_note"`
	SimulationStatus     string `json:"simulation_status"`
}

type budgetExample struct {
	CurrencyID              string `json:"currency_id"`
	Balance                 int64  `json:"balance"`
	ProtectedReserve        int64  `json:"protected_reserve"`
	Committed               int64  `json:"committed"`
	Available               int64  `json:"available"`
	AuthorizationMode       string `json:"authorization_mode"`
	AuthorizationLimit      int64  `json:"authorization_limit"`
	AuthorizationSpent      int64  `json:"authorization_spent"`
	AllowedForThisOperation int64  `json:"allowed_for_this_operation"`
}

type valueExample struct {
	BandID            string `json:"band_id"`
	MaturityPermille  int64  `json:"maturity_factor_permille"`
	ConditionPermille int64  `json:"condition_factor_permille"`
	Low               int64  `json:"low"`
	High              int64  `json:"high"`
	Interpretation    string `json:"interpretation"`
}

type report struct {
	Kind               string               `json:"kind"`
	Candidate          candidateSummary     `json:"candidate"`
	SourceHashes       []sourceHash         `json:"source_hashes"`
	PriceBands         []unitBandSummary    `json:"price_bands"`
	WealthBands        []unitBandSummary    `json:"wealth_reference_bands"`
	RewardBands        []unitBandSummary    `json:"reward_reference_bands"`
	PurchasingPower    purchasingPower      `json:"purchasing_power"`
	BattleActionCost   actionCost           `json:"battle_action_cost"`
	QuoteExamples      []quoteExample       `json:"quote_examples"`
	BudgetExample      budgetExample        `json:"budget_example"`
	ValueEstimates     []valueExample       `json:"value_estimates"`
	Diagnostics        []economy.Diagnostic `json:"diagnostics"`
	ValidationBoundary []string             `json:"validation_boundary"`
}

func main() {
	os.Exit(execute(os.Args[1:], os.Stderr))
}

func execute(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("economy-balance", flag.ContinueOnError)
	flags.SetOutput(stderr)
	input := flags.String("input", defaultInput, "candidate parameter JSON (read-only)")
	output := flags.String("output", defaultOutput, "directory for deterministic report files")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := run(*input, *output); err != nil {
		fmt.Fprintln(stderr, "economy-balance:", err)
		return 2
	}
	return 0
}

func run(inputPath, outputDir string) error {
	result := report{Kind: reportKind, Diagnostics: []economy.Diagnostic{}}
	result.ValidationBoundary = []string{
		"Candidate ranges and arithmetic only; no item registry, inventory, wallet, or transaction state is read or written.",
		"Quote calculations are not player consent, stock availability, identification, buyer commitment, or a completed trade.",
		"Budget checks do not grant authority, store balances, reserve funds, or deduct payment.",
		"Reward idempotency, atomic settlement, Story JSONL, branch recovery, and live game behavior are not tested here.",
		"Combat action and recovery semantics are cited from existing design rules; combat is not simulated.",
	}

	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	inputBytes, inputErr := os.ReadFile(inputPath)
	if inputErr != nil {
		result.Diagnostics = append(result.Diagnostics, economy.Diagnostic{Code: "candidate_read_error", Path: displayPath(root, inputPath), Message: inputErr.Error()})
	} else {
		result.SourceHashes = append(result.SourceHashes, sourceHash{Path: displayPath(root, inputPath), SHA256: digest(inputBytes)})
		candidate, diagnostics, loadErr := economy.LoadCandidate(bytes.NewReader(inputBytes))
		result.Diagnostics = append(result.Diagnostics, diagnostics...)
		if loadErr != nil {
			result.Diagnostics = append(result.Diagnostics, economy.Diagnostic{Code: "candidate_metadata_error", Path: displayPath(root, inputPath), Message: loadErr.Error()})
		} else {
			result.Candidate = summarizeCandidate(candidate)
			result.PriceBands = summarizePriceBands(candidate.PriceBands)
			result.WealthBands = summarizeAmountBands(candidate.WealthReferenceBands)
			result.RewardBands = summarizeAmountBands(candidate.RewardReferenceBands)
			if source, hashErr := sourceHashForReference(root, candidate.Source); hashErr != nil {
				result.Diagnostics = append(result.Diagnostics, economy.Diagnostic{Code: "source_hash_error", Path: candidate.Source, Message: hashErr.Error()})
			} else {
				result.SourceHashes = append(result.SourceHashes, source)
			}
			benchmarks, benchmarkDiagnostics := analyze(candidate)
			result.PurchasingPower = benchmarks.power
			result.BattleActionCost = benchmarks.battle
			result.QuoteExamples = benchmarks.quotes
			result.BudgetExample = benchmarks.budget
			result.ValueEstimates = benchmarks.estimates
			result.Diagnostics = append(result.Diagnostics, benchmarkDiagnostics...)
		}
	}
	for _, reference := range supportingSources {
		if source, hashErr := sourceHashForReference(root, reference); hashErr != nil {
			result.Diagnostics = append(result.Diagnostics, economy.Diagnostic{Code: "supporting_source_hash_error", Path: reference, Message: hashErr.Error()})
		} else {
			result.SourceHashes = append(result.SourceHashes, source)
		}
	}
	result.SourceHashes = uniqueSortedHashes(result.SourceHashes)
	sort.SliceStable(result.Diagnostics, func(i, j int) bool {
		if result.Diagnostics[i].Path != result.Diagnostics[j].Path {
			return result.Diagnostics[i].Path < result.Diagnostics[j].Path
		}
		if result.Diagnostics[i].Code != result.Diagnostics[j].Code {
			return result.Diagnostics[i].Code < result.Diagnostics[j].Code
		}
		return result.Diagnostics[i].Message < result.Diagnostics[j].Message
	})

	if err := writeOutputs(outputDir, result); err != nil {
		return err
	}
	if len(result.Diagnostics) != 0 {
		return fmt.Errorf("report generated with %d diagnostic(s); see %s", len(result.Diagnostics), filepath.Join(outputDir, "economy-balance-issues.json"))
	}
	fmt.Printf("Candidate %s (%s): report generated; %d source hashes, no diagnostics.\n", result.Candidate.ID, result.Candidate.DesignVersion, len(result.SourceHashes))
	return nil
}

type analysis struct {
	power     purchasingPower
	battle    actionCost
	quotes    []quoteExample
	budget    budgetExample
	estimates []valueExample
}

func analyze(candidate economy.Candidate) (analysis, []economy.Diagnostic) {
	result := analysis{power: purchasingPower{CurrencyID: candidate.CurrencyID, BenchmarkAmount: candidate.WealthBenchmark}}
	var diagnostics []economy.Diagnostic
	addError := func(code, path string, err error) {
		if err != nil {
			diagnostics = append(diagnostics, economy.Diagnostic{Code: code, Path: path, Message: err.Error()})
		}
	}
	findPriceBand := func(id string) (economy.PriceBand, bool) {
		band, err := candidate.PriceBand(id)
		if err != nil {
			addError("missing_reference_band", "price_bands."+id, err)
			return economy.PriceBand{}, false
		}
		return band, true
	}
	findAmountBand := func(bands []economy.AmountBand, id, field string) (economy.AmountBand, bool) {
		for _, band := range bands {
			if band.ID == id {
				return band, true
			}
		}
		addError("missing_reference_band", field+"."+id, errors.New("required analysis band is absent"))
		return economy.AmountBand{}, false
	}
	benchmark := economy.Money{Amount: candidate.WealthBenchmark, CurrencyID: candidate.CurrencyID}

	if band, ok := findPriceBand("ordinary_qi_recovery_low"); ok {
		result.power.LowRecoveryDosesAtMinimumPrice = affordableQuantityMetric(candidate, benchmark, band.Min, band.CurrencyID, band.UnitID, "purchasing_power.low_recovery_doses_at_minimum_price", &diagnostics)
		result.power.LowRecoveryDosesAtMaximumPrice = affordableQuantityMetric(candidate, benchmark, band.Max, band.CurrencyID, band.UnitID, "purchasing_power.low_recovery_doses_at_maximum_price", &diagnostics)
		lowQuote, lowErr := candidate.RetailQuote(band.ID, economy.Money{Amount: band.Min, CurrencyID: band.CurrencyID}, economy.Quantity{Count: 3, UnitID: band.UnitID}, 1000)
		addError("recovery_quote_error", "price_bands."+band.ID, lowErr)
		highQuote, highErr := candidate.RetailQuote(band.ID, economy.Money{Amount: band.Max, CurrencyID: band.CurrencyID}, economy.Quantity{Count: 3, UnitID: band.UnitID}, 1000)
		addError("recovery_quote_error", "price_bands."+band.ID, highErr)
		if lowErr == nil && highErr == nil {
			result.battle = actionCost{
				DoseCount:            3,
				MinimumSpend:         lowQuote.Total.Amount,
				MaximumSpend:         highQuote.Total.Amount,
				MajorActionsConsumed: 3,
				ExistingRuleNote:     "One registered low-grade dose restores its fixed 10% registered-stage baseline and consumes one major action; see the existing item and cultivation rules.",
				SimulationStatus:     "price_and_action_cost_only_combat_not_simulated",
			}
		}
	}
	if band, ok := findPriceBand("ordinary_qi_recovery_mid_high"); ok {
		result.power.MidHighRecoveryDosesAtMinPrice = affordableQuantityMetric(candidate, benchmark, band.Min, band.CurrencyID, band.UnitID, "purchasing_power.mid_high_recovery_doses_at_minimum_price", &diagnostics)
		result.power.MidHighRecoveryDosesAtMaxPrice = affordableQuantityMetric(candidate, benchmark, band.Max, band.CurrencyID, band.UnitID, "purchasing_power.mid_high_recovery_doses_at_maximum_price", &diagnostics)
	}
	if reward, ok := findAmountBand(candidate.RewardReferenceBands, "short_low_risk_net_reward", "reward_reference_bands"); ok {
		if recovery, recoveryOK := findPriceBand("ordinary_qi_recovery_low"); recoveryOK {
			result.power.ShortTaskNetDosesLowCase = affordableQuantityMetric(candidate, economy.Money{Amount: reward.Min, CurrencyID: reward.CurrencyID}, recovery.Max, recovery.CurrencyID, recovery.UnitID, "purchasing_power.short_task_net_doses_low_case", &diagnostics)
			result.power.ShortTaskNetDosesHighCase = affordableQuantityMetric(candidate, economy.Money{Amount: reward.Max, CurrencyID: reward.CurrencyID}, recovery.Min, recovery.CurrencyID, recovery.UnitID, "purchasing_power.short_task_net_doses_high_case", &diagnostics)
		}
	}
	if band, ok := findPriceBand("ordinary_qi_implement_low"); ok {
		result.power.ArtifactQuantityAtMinPrice = affordableQuantityMetric(candidate, benchmark, band.Min, band.CurrencyID, band.UnitID, "purchasing_power.common_low_artifact_quantity_at_minimum_price", &diagnostics)
		result.power.ArtifactQuantityAtMaxPrice = affordableQuantityMetric(candidate, benchmark, band.Max, band.CurrencyID, band.UnitID, "purchasing_power.common_low_artifact_quantity_at_maximum_price", &diagnostics)
		result.power.ArtifactCostShareAtMinPermille = sharePermilleMetric(candidate, economy.Money{Amount: band.Min, CurrencyID: band.CurrencyID}, benchmark, "purchasing_power.common_low_artifact_cost_share_at_minimum_price_permille", &diagnostics)
		result.power.ArtifactCostShareAtMaxPermille = sharePermilleMetric(candidate, economy.Money{Amount: band.Max, CurrencyID: band.CurrencyID}, benchmark, "purchasing_power.common_low_artifact_cost_share_at_maximum_price_permille", &diagnostics)
	}
	result.quotes = append(result.quotes, makeQuoteExample(candidate, "fixed_case_low_recovery", "ordinary_qi_recovery_low", 3, 4, "dose", 1000, 600, &diagnostics))
	result.quotes = append(result.quotes, makeQuoteExample(candidate, "fixed_case_low_implement", "ordinary_qi_implement_low", 15, 1, "piece", 800, 700, &diagnostics))
	result.budget = makeBudgetExample(candidate, &diagnostics)
	result.estimates = append(result.estimates,
		makeValueExample(candidate, "foundation_key_material", 1000, 800, &diagnostics),
		makeValueExample(candidate, "foundation_key_material", 2000, 1000, &diagnostics),
	)
	return result, diagnostics
}

func affordableQuantityMetric(candidate economy.Candidate, balance economy.Money, unitPrice int64, currencyID, unitID, path string, diagnostics *[]economy.Diagnostic) *int64 {
	quantity, err := candidate.AffordableQuantity(balance, economy.Money{Amount: unitPrice, CurrencyID: currencyID}, unitID)
	if err != nil {
		*diagnostics = append(*diagnostics, economy.Diagnostic{Code: "purchasing_power_calculation_error", Path: path, Message: err.Error()})
		return nil
	}
	return &quantity
}

func sharePermilleMetric(candidate economy.Candidate, part, whole economy.Money, path string, diagnostics *[]economy.Diagnostic) *int64 {
	share, err := candidate.SharePermille(part, whole)
	if err != nil {
		*diagnostics = append(*diagnostics, economy.Diagnostic{Code: "purchasing_power_calculation_error", Path: path, Message: err.Error()})
		return nil
	}
	return &share
}

func makeQuoteExample(candidate economy.Candidate, name, bandID string, baseAmount, quantity int64, unitID string, marketPermille, buybackPermille int64, diagnostics *[]economy.Diagnostic) quoteExample {
	result := quoteExample{Name: name, BandID: bandID, BaseUnitPrice: baseAmount, Quantity: quantity, UnitID: unitID, MarketPermille: marketPermille, BuybackPermille: buybackPermille}
	base := economy.Money{Amount: baseAmount, CurrencyID: candidate.CurrencyID}
	units := economy.Quantity{Count: quantity, UnitID: unitID}
	retail, err := candidate.RetailQuote(bandID, base, units, marketPermille)
	if err != nil {
		*diagnostics = append(*diagnostics, economy.Diagnostic{Code: "fixed_quote_error", Path: "price_bands." + bandID, Message: err.Error()})
	} else {
		result.RetailUnit = retail.QuotedUnitPrice.Amount
		result.RetailTotal = retail.Total.Amount
	}
	buyback, err := candidate.BuybackQuote(bandID, base, units, marketPermille, buybackPermille)
	if err != nil {
		*diagnostics = append(*diagnostics, economy.Diagnostic{Code: "fixed_quote_error", Path: "price_bands." + bandID, Message: err.Error()})
	} else {
		result.BuybackUnit = buyback.QuotedUnitPrice.Amount
		result.BuybackTotal = buyback.Total.Amount
	}
	return result
}

func makeBudgetExample(candidate economy.Candidate, diagnostics *[]economy.Diagnostic) budgetExample {
	result := budgetExample{
		CurrencyID: candidate.CurrencyID, Balance: 200_000, ProtectedReserve: 120_000, Committed: 30_000,
		AuthorizationMode: "limited", AuthorizationLimit: 200, AuthorizationSpent: 80,
	}
	budget, err := candidate.AvailableBudget(economy.BudgetSnapshot{
		Balance:          economy.Money{Amount: result.Balance, CurrencyID: candidate.CurrencyID},
		ProtectedReserve: economy.Money{Amount: result.ProtectedReserve, CurrencyID: candidate.CurrencyID},
		Committed:        economy.Money{Amount: result.Committed, CurrencyID: candidate.CurrencyID},
	}, economy.SpendingAuthorization{Mode: economy.AuthorizationLimited, CurrencyID: candidate.CurrencyID, Limit: result.AuthorizationLimit, Spent: result.AuthorizationSpent})
	if err != nil {
		*diagnostics = append(*diagnostics, economy.Diagnostic{Code: "fixed_budget_example_error", Path: "budget_example", Message: err.Error()})
		return result
	}
	result.Available = budget.Available
	result.AllowedForThisOperation = budget.Allowed
	return result
}

func makeValueExample(candidate economy.Candidate, bandID string, maturityPermille, conditionPermille int64, diagnostics *[]economy.Diagnostic) valueExample {
	result := valueExample{BandID: bandID, MaturityPermille: maturityPermille, ConditionPermille: conditionPermille}
	estimate, err := candidate.EstimateValue(bandID, maturityPermille, conditionPermille)
	if err != nil {
		*diagnostics = append(*diagnostics, economy.Diagnostic{Code: "fixed_value_example_error", Path: "price_bands." + bandID, Message: err.Error()})
		return result
	}
	result.Low = estimate.Low
	result.High = estimate.High
	result.Interpretation = estimate.Interpretation
	return result
}

func summarizeCandidate(candidate economy.Candidate) candidateSummary {
	return candidateSummary{
		ID: candidate.ID, DesignVersion: candidate.DesignVersion, Status: candidate.Status,
		CurrencyID: candidate.CurrencyID, AmountLimit: candidate.AmountLimit, QuantityLimit: candidate.QuantityLimit,
		WealthBenchmark: candidate.WealthBenchmark, ValidPriceBands: len(candidate.PriceBands),
		ValidWealthBands: len(candidate.WealthReferenceBands), ValidRewardBands: len(candidate.RewardReferenceBands),
	}
}

func summarizePriceBands(bands []economy.PriceBand) []unitBandSummary {
	result := make([]unitBandSummary, 0, len(bands))
	for _, band := range bands {
		result = append(result, unitBandSummary{ID: band.ID, CurrencyID: band.CurrencyID, UnitID: band.UnitID, Min: band.Min, Max: band.Max})
	}
	return result
}

func summarizeAmountBands(bands []economy.AmountBand) []unitBandSummary {
	result := make([]unitBandSummary, 0, len(bands))
	for _, band := range bands {
		result = append(result, unitBandSummary{ID: band.ID, CurrencyID: band.CurrencyID, UnitID: band.UnitID, Min: band.Min, Max: band.Max})
	}
	return result
}

func sourceHashForReference(root, reference string) (sourceHash, error) {
	if strings.TrimSpace(reference) == "" || filepath.IsAbs(reference) {
		return sourceHash{}, errors.New("source reference must be a non-empty relative path")
	}
	clean := filepath.Clean(filepath.FromSlash(reference))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return sourceHash{}, errors.New("source reference escapes the repository root")
	}
	absolute := filepath.Join(root, clean)
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return sourceHash{}, fmt.Errorf("resolve source reference: %w", err)
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return sourceHash{}, errors.New("resolved source reference escapes the repository root")
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return sourceHash{}, fmt.Errorf("read source reference: %w", err)
	}
	return sourceHash{Path: filepath.ToSlash(relative), SHA256: digest(data)}, nil
}

func uniqueSortedHashes(hashes []sourceHash) []sourceHash {
	seen := make(map[string]struct{}, len(hashes))
	result := make([]sourceHash, 0, len(hashes))
	for _, item := range hashes {
		if item.Path == "" {
			continue
		}
		if _, exists := seen[item.Path]; exists {
			continue
		}
		seen[item.Path] = struct{}{}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func displayPath(root, path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Base(path)
	}
	relative, err := filepath.Rel(root, abs)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.Base(path)
	}
	return filepath.ToSlash(relative)
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeOutputs(directory string, result report) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	jsonBytes = append(jsonBytes, '\n')
	issuesBytes, err := json.MarshalIndent(result.Diagnostics, "", "  ")
	if err != nil {
		return fmt.Errorf("encode diagnostics: %w", err)
	}
	issuesBytes = append(issuesBytes, '\n')
	markdown := renderMarkdown(result)
	outputs := []struct {
		name string
		data []byte
	}{
		{"economy-balance-issues.json", issuesBytes},
		{"economy-balance-report.json", jsonBytes},
		{"economy-balance-report.md", []byte(markdown)},
	}
	for _, output := range outputs {
		if err := os.WriteFile(filepath.Join(directory, output.name), output.data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", output.name, err)
		}
	}
	return nil
}

func renderMarkdown(result report) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Independent economy candidate report\n\n")
	fmt.Fprintf(&builder, "Status: `%s`  \n", result.Kind)
	if result.Candidate.ID != "" {
		fmt.Fprintf(&builder, "Candidate: `%s` · design `%s` · `%s`  \n", result.Candidate.ID, result.Candidate.DesignVersion, result.Candidate.Status)
		fmt.Fprintf(&builder, "Currency: `%s` · purchasing-power benchmark: %d base units  \n", result.Candidate.CurrencyID, result.Candidate.WealthBenchmark)
		fmt.Fprintf(&builder, "Loaded ranges: %d price, %d wealth, %d reward\n\n", result.Candidate.ValidPriceBands, result.Candidate.ValidWealthBands, result.Candidate.ValidRewardBands)
	}
	if len(result.SourceHashes) > 0 {
		builder.WriteString("## Source hashes\n\n")
		for _, item := range result.SourceHashes {
			fmt.Fprintf(&builder, "- `%s`: `%s`\n", item.Path, item.SHA256)
		}
		builder.WriteString("\n")
	}
	fmt.Fprintf(&builder, "## Purchasing power\n\n")
	fmt.Fprintf(&builder, "- %d-base-unit reference buys %s low recovery doses across the candidate price range.\n", result.PurchasingPower.BenchmarkAmount, metricRange(result.PurchasingPower.LowRecoveryDosesAtMaximumPrice, result.PurchasingPower.LowRecoveryDosesAtMinimumPrice))
	fmt.Fprintf(&builder, "- Mid/high ordinary recovery: %s doses.\n", metricRange(result.PurchasingPower.MidHighRecoveryDosesAtMaxPrice, result.PurchasingPower.MidHighRecoveryDosesAtMinPrice))
	fmt.Fprintf(&builder, "- Short low-risk net reward reference covers %s low recovery doses; this is not a universal task cap.\n", metricRange(result.PurchasingPower.ShortTaskNetDosesLowCase, result.PurchasingPower.ShortTaskNetDosesHighCase))
	fmt.Fprintf(&builder, "- Common low implement reference: %s units, costing %s–%s of the benchmark.\n\n", metricRange(result.PurchasingPower.ArtifactQuantityAtMaxPrice, result.PurchasingPower.ArtifactQuantityAtMinPrice), optionalPercent(result.PurchasingPower.ArtifactCostShareAtMinPermille), optionalPercent(result.PurchasingPower.ArtifactCostShareAtMaxPermille))
	fmt.Fprintf(&builder, "## Medicine cost in combat context\n\n")
	fmt.Fprintf(&builder, "%d doses cost %d–%d base units and consume %d major actions under the cited existing rule. %s This is a cost illustration, not combat simulation.\n\n", result.BattleActionCost.DoseCount, result.BattleActionCost.MinimumSpend, result.BattleActionCost.MaximumSpend, result.BattleActionCost.MajorActionsConsumed, result.BattleActionCost.ExistingRuleNote)
	fmt.Fprintf(&builder, "## Fixed quote examples (independent of purchasing-power benchmark)\n\n| Case | Base | Qty | Market | Retail/unit total | Buyback/unit total |\n|---|---:|---:|---:|---:|---:|\n")
	for _, item := range result.QuoteExamples {
		fmt.Fprintf(&builder, "| %s | %d | %d | %d‰ | %d / %d | %d / %d |\n", item.Name, item.BaseUnitPrice, item.Quantity, item.MarketPermille, item.RetailUnit, item.RetailTotal, item.BuybackUnit, item.BuybackTotal)
	}
	builder.WriteString("\n## Faction spendability example\n\n")
	fmt.Fprintf(&builder, "Balance %d − protected %d − committed %d = available %d; this operation's limited authority has %d remaining, so its allowed spend is %d. This calculation grants no authority and changes no balance.\n\n", result.BudgetExample.Balance, result.BudgetExample.ProtectedReserve, result.BudgetExample.Committed, result.BudgetExample.Available, result.BudgetExample.AuthorizationLimit-result.BudgetExample.AuthorizationSpent, result.BudgetExample.AllowedForThisOperation)
	builder.WriteString("## Registered-use value examples\n\n")
	for _, item := range result.ValueEstimates {
		fmt.Fprintf(&builder, "- `%s`, maturity %d‰, condition %d‰: %d–%d. Interpretation: `%s`.\n", item.BandID, item.MaturityPermille, item.ConditionPermille, item.Low, item.High, item.Interpretation)
	}
	builder.WriteString("\n## Validation boundary\n\n")
	for _, item := range result.ValidationBoundary {
		fmt.Fprintf(&builder, "- %s\n", item)
	}
	if len(result.Diagnostics) != 0 {
		fmt.Fprintf(&builder, "\n## Diagnostics\n\n%d input or example issue(s); see `economy-balance-issues.json`.\n", len(result.Diagnostics))
	} else {
		builder.WriteString("\n## Diagnostics\n\nNo candidate or example diagnostics.\n")
	}
	return builder.String()
}

func percent(value int64) string {
	return fmt.Sprintf("%d.%d%%", value/10, value%10)
}

func metricRange(minimum, maximum *int64) string {
	if minimum == nil || maximum == nil {
		return "unavailable"
	}
	return fmt.Sprintf("%d–%d", *minimum, *maximum)
}

func optionalPercent(value *int64) string {
	if value == nil {
		return "unavailable"
	}
	return percent(*value)
}
