package economy

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const fixtureMetadata = `{
  "schema_version":1,"id":"test-candidate","design_version":"0.2","status":"candidate_not_installed",
  "source":"test-design.md","currency_id":"low_grade_spirit_stone","amount_limit":1000000000000,"quantity_limit":1000000000,"wealth_benchmark":100,
  "market_factor_permille":{"min":800,"max":1200},"buyback_factor_permille":{"min":500,"max":700},
  "maturity_factor_permille":{"min":500,"max":2000},"condition_factor_permille":{"min":0,"max":1000},
  "wealth_reference_bands":[{"id":"test_cash","currency_id":"low_grade_spirit_stone","unit_id":"cash_balance","min":300,"max":2000}],
  "reward_reference_bands":[{"id":"test_reward","currency_id":"low_grade_spirit_stone","unit_id":"completed_net","min":3,"max":8}]
}`

func loadTestCandidate(t *testing.T, priceBands string) (Candidate, []Diagnostic) {
	t.Helper()
	input := strings.TrimSuffix(fixtureMetadata, "}") + `,"price_bands":[` + priceBands + "]}"
	candidate, diagnostics, err := LoadCandidate(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadCandidate returned error: %v", err)
	}
	return candidate, diagnostics
}

func standardBands() string {
	return `{"id":"ordinary_recovery","currency_id":"low_grade_spirit_stone","unit_id":"dose","min":2,"max":3},` +
		`{"id":"ordinary_artifact","currency_id":"low_grade_spirit_stone","unit_id":"piece","min":15,"max":30},` +
		`{"id":"value_sample","currency_id":"low_grade_spirit_stone","unit_id":"usable_batch","min":100,"max":500},` +
		`{"id":"large_value","currency_id":"low_grade_spirit_stone","unit_id":"usable_batch","min":1000000000000,"max":1000000000000}`
}

func TestFixedQuoteExamples(t *testing.T) {
	candidate, diagnostics := loadTestCandidate(t, standardBands())
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}

	retail, err := candidate.RetailQuote("ordinary_recovery", Money{Amount: 3, CurrencyID: candidate.CurrencyID}, Quantity{Count: 4, UnitID: "dose"}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if retail.QuotedUnitPrice.Amount != 3 || retail.Total.Amount != 12 {
		t.Fatalf("fixed retail example = unit %d total %d, want 3 and 12", retail.QuotedUnitPrice.Amount, retail.Total.Amount)
	}
	buyback, err := candidate.BuybackQuote("ordinary_recovery", Money{Amount: 3, CurrencyID: candidate.CurrencyID}, Quantity{Count: 4, UnitID: "dose"}, 1000, 600)
	if err != nil {
		t.Fatal(err)
	}
	if buyback.QuotedUnitPrice.Amount != 1 || buyback.Total.Amount != 4 {
		t.Fatalf("fixed buyback example = unit %d total %d, want 1 and 4", buyback.QuotedUnitPrice.Amount, buyback.Total.Amount)
	}

	retail, err = candidate.RetailQuote("ordinary_artifact", Money{Amount: 15, CurrencyID: candidate.CurrencyID}, Quantity{Count: 1, UnitID: "piece"}, 800)
	if err != nil || retail.QuotedUnitPrice.Amount != 12 {
		t.Fatalf("second retail example = %+v, %v; want unit 12", retail, err)
	}
	buyback, err = candidate.BuybackQuote("ordinary_artifact", Money{Amount: 15, CurrencyID: candidate.CurrencyID}, Quantity{Count: 1, UnitID: "piece"}, 800, 700)
	if err != nil || buyback.QuotedUnitPrice.Amount != 8 {
		t.Fatalf("second buyback example = %+v, %v; want unit 8", buyback, err)
	}
}

func TestAllMarketAndBuybackFactorValuesHaveNoSameMarketArbitrage(t *testing.T) {
	candidate, _ := loadTestCandidate(t, standardBands())
	for market := int64(800); market <= 1200; market++ {
		for buybackFactor := int64(500); buybackFactor <= 700; buybackFactor++ {
			retail, retailErr := candidate.RetailQuote("ordinary_artifact", Money{Amount: 15, CurrencyID: candidate.CurrencyID}, Quantity{Count: 1, UnitID: "piece"}, market)
			buyback, buybackErr := candidate.BuybackQuote("ordinary_artifact", Money{Amount: 15, CurrencyID: candidate.CurrencyID}, Quantity{Count: 1, UnitID: "piece"}, market, buybackFactor)
			if retailErr != nil {
				t.Fatalf("market %d retail: %v", market, retailErr)
			}
			if errors.Is(buybackErr, ErrNoBuybackQuote) {
				continue
			}
			if buybackErr != nil {
				t.Fatalf("market %d buyback %d: %v", market, buybackFactor, buybackErr)
			}
			if buyback.QuotedUnitPrice.Amount >= retail.QuotedUnitPrice.Amount {
				t.Fatalf("arbitrage at market=%d buyback=%d: buyback=%d retail=%d", market, buybackFactor, buyback.QuotedUnitPrice.Amount, retail.QuotedUnitPrice.Amount)
			}
		}
	}
}

func TestLowPriceRoundingAndSplitMergeEquality(t *testing.T) {
	candidate, _ := loadTestCandidate(t, standardBands())
	one, err := candidate.RetailQuote("ordinary_recovery", Money{Amount: 2, CurrencyID: candidate.CurrencyID}, Quantity{Count: 1, UnitID: "dose"}, 800)
	if err != nil || one.QuotedUnitPrice.Amount != 2 {
		t.Fatalf("low-price retail rounding = %+v, %v; want ceil(1.6)=2", one, err)
	}
	combined, err := candidate.RetailQuote("ordinary_recovery", Money{Amount: 3, CurrencyID: candidate.CurrencyID}, Quantity{Count: 4, UnitID: "dose"}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	var splitTotal int64
	for range 4 {
		separate, quoteErr := candidate.RetailQuote("ordinary_recovery", Money{Amount: 3, CurrencyID: candidate.CurrencyID}, Quantity{Count: 1, UnitID: "dose"}, 1000)
		if quoteErr != nil {
			t.Fatal(quoteErr)
		}
		splitTotal += separate.Total.Amount
	}
	if splitTotal != combined.Total.Amount || combined.Total.Amount != 12 {
		t.Fatalf("split total %d != combined total %d", splitTotal, combined.Total.Amount)
	}
}

func TestInvalidQuoteInputsAndOverflow(t *testing.T) {
	candidate, _ := loadTestCandidate(t, standardBands()+`,{"id":"large","currency_id":"low_grade_spirit_stone","unit_id":"unit","min":2,"max":1000000000000}`)
	base := Money{Amount: 3, CurrencyID: candidate.CurrencyID}
	if _, err := candidate.RetailQuote("ordinary_recovery", base, Quantity{Count: 0, UnitID: "dose"}, 1000); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("zero quantity error = %v", err)
	}
	if _, err := candidate.RetailQuote("ordinary_recovery", Money{Amount: 3, CurrencyID: "other_coin"}, Quantity{Count: 1, UnitID: "dose"}, 1000); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("currency mismatch error = %v", err)
	}
	if _, err := candidate.RetailQuote("ordinary_recovery", base, Quantity{Count: 1, UnitID: "piece"}, 1000); !errors.Is(err, ErrUnitMismatch) {
		t.Fatalf("unit mismatch error = %v", err)
	}
	if _, err := candidate.RetailQuote("ordinary_recovery", base, Quantity{Count: 1, UnitID: "dose"}, 799); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("out-of-range market error = %v", err)
	}
	if _, err := candidate.RetailQuote("large", Money{Amount: 10_000_000_000, CurrencyID: candidate.CurrencyID}, Quantity{Count: 1_000_000_000, UnitID: "unit"}, 1000); !errors.Is(err, ErrOverflow) {
		t.Fatalf("overflow error = %v", err)
	}
	if _, err := candidate.BuybackQuote("ordinary_recovery", Money{Amount: 2, CurrencyID: candidate.CurrencyID}, Quantity{Count: 1, UnitID: "dose"}, 800, 500); !errors.Is(err, ErrNoBuybackQuote) {
		t.Fatalf("rounded-zero buyback error = %v", err)
	}
}

func TestCalculationAndCandidateLimits(t *testing.T) {
	candidate, _ := loadTestCandidate(t, standardBands())
	if err := candidate.ValidateMoney(Money{Amount: MaxAmount, CurrencyID: candidate.CurrencyID}); err != nil {
		t.Fatalf("maximum amount rejected: %v", err)
	}
	if err := candidate.ValidateMoney(Money{Amount: MaxAmount + 1, CurrencyID: candidate.CurrencyID}); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("amount above generic bound error = %v", err)
	}
	if err := candidate.ValidateQuantity(Quantity{Count: MaxQuantity, UnitID: "dose"}, "dose"); err != nil {
		t.Fatalf("maximum quantity rejected: %v", err)
	}
	if err := candidate.ValidateQuantity(Quantity{Count: MaxQuantity + 1, UnitID: "dose"}, "dose"); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("quantity above generic bound error = %v", err)
	}
}

func TestValueEstimateBoundariesAndMeaning(t *testing.T) {
	candidate, _ := loadTestCandidate(t, standardBands())
	estimate, err := candidate.EstimateValue("value_sample", 1000, 800)
	if err != nil || estimate.Low != 80 || estimate.High != 400 {
		t.Fatalf("value example = %+v, %v; want [80,400]", estimate, err)
	}
	estimate, err = candidate.EstimateValue("value_sample", 2000, 1000)
	if err != nil || estimate.Low != 200 || estimate.High != 1000 {
		t.Fatalf("upper boundary example = %+v, %v; want [200,1000]", estimate, err)
	}
	estimate, err = candidate.EstimateValue("value_sample", 500, 0)
	if err != nil || estimate.Low != 0 || estimate.High != 0 {
		t.Fatalf("zero-condition estimate = %+v, %v; want [0,0]", estimate, err)
	}
	if estimate.Interpretation != "estimate_only_not_identification_or_buyer_commitment" {
		t.Fatalf("valuation interpretation = %q", estimate.Interpretation)
	}
	minimumMaturity, err := candidate.EstimateValue("value_sample", 500, 1000)
	if err != nil || minimumMaturity.Low != 50 || minimumMaturity.High != 250 {
		t.Fatalf("minimum maturity estimate = %+v, %v; want [50,250]", minimumMaturity, err)
	}
	if _, err := candidate.EstimateValue("value_sample", 2001, 1000); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("out-of-range maturity error = %v", err)
	}
	if _, err := candidate.EstimateValue("value_sample", 1000, 1001); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("out-of-range condition error = %v", err)
	}
	if _, err := candidate.EstimateValue("value_sample", 2000, 1000); err != nil {
		t.Fatalf("boundary estimate: %v", err)
	}
	if _, err := candidate.EstimateValue("large_value", 2000, 1000); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("estimate above calculation limit error = %v", err)
	}
}

func TestSpendableBudgetExampleZeroAndAuthorizationModes(t *testing.T) {
	candidate, _ := loadTestCandidate(t, standardBands())
	snapshot := BudgetSnapshot{
		Balance:          Money{Amount: 200_000, CurrencyID: candidate.CurrencyID},
		ProtectedReserve: Money{Amount: 120_000, CurrencyID: candidate.CurrencyID},
		Committed:        Money{Amount: 30_000, CurrencyID: candidate.CurrencyID},
	}
	authority := SpendingAuthorization{Mode: AuthorizationLimited, CurrencyID: candidate.CurrencyID, Limit: 200, Spent: 80}
	budget, err := candidate.AvailableBudget(snapshot, authority)
	if err != nil || budget.Available != 50_000 || budget.AuthorizedLeft != 120 || budget.Allowed != 120 {
		t.Fatalf("budget example = %+v, %v; want 50000/120/120", budget, err)
	}
	if err := candidate.CheckSpend(budget, Money{Amount: 121, CurrencyID: candidate.CurrencyID}); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("over-authorization spend error = %v", err)
	}

	zeroSnapshot := BudgetSnapshot{
		Balance:          Money{CurrencyID: candidate.CurrencyID},
		ProtectedReserve: Money{CurrencyID: candidate.CurrencyID},
		Committed:        Money{CurrencyID: candidate.CurrencyID},
	}
	zeroAuthority := SpendingAuthorization{Mode: AuthorizationLimited, CurrencyID: candidate.CurrencyID}
	zeroBudget, err := candidate.AvailableBudget(zeroSnapshot, zeroAuthority)
	if err != nil || zeroBudget.Allowed != 0 {
		t.Fatalf("zero balance/authority = %+v, %v", zeroBudget, err)
	}
	if err := candidate.CheckSpend(zeroBudget, Money{CurrencyID: candidate.CurrencyID}); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("zero spend amount error = %v", err)
	}
	unrestrictedZero, err := candidate.AvailableBudget(zeroSnapshot, SpendingAuthorization{Mode: AuthorizationUnrestricted, CurrencyID: candidate.CurrencyID})
	if err != nil || unrestrictedZero.Available != 0 || unrestrictedZero.Allowed != 0 {
		t.Fatalf("zero balance unrestricted budget = %+v, %v", unrestrictedZero, err)
	}
	if _, err := candidate.AvailableBudget(zeroSnapshot, SpendingAuthorization{CurrencyID: candidate.CurrencyID}); !errors.Is(err, ErrInvalidAuthority) {
		t.Fatalf("unspecified authority mode error = %v", err)
	}
	if _, err := candidate.AvailableBudget(zeroSnapshot, SpendingAuthorization{Mode: AuthorizationMode(99), CurrencyID: candidate.CurrencyID}); !errors.Is(err, ErrInvalidAuthority) {
		t.Fatalf("unknown authority mode error = %v", err)
	}
	if _, err := candidate.AvailableBudget(snapshot, SpendingAuthorization{Mode: AuthorizationUnrestricted, CurrencyID: "other_coin"}); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("authorization currency error = %v", err)
	}
	if _, err := candidate.AvailableBudget(snapshot, SpendingAuthorization{Mode: AuthorizationLimited, CurrencyID: candidate.CurrencyID, Limit: 5, Spent: 6}); !errors.Is(err, ErrInvalidAuthority) {
		t.Fatalf("overspent authority error = %v", err)
	}
}

func TestBudgetRejectsOvercommittedSnapshotWithoutMutation(t *testing.T) {
	candidate, _ := loadTestCandidate(t, standardBands())
	snapshot := BudgetSnapshot{
		Balance:          Money{Amount: 10, CurrencyID: candidate.CurrencyID},
		ProtectedReserve: Money{Amount: 6, CurrencyID: candidate.CurrencyID},
		Committed:        Money{Amount: 5, CurrencyID: candidate.CurrencyID},
	}
	original := snapshot
	_, err := candidate.AvailableBudget(snapshot, SpendingAuthorization{Mode: AuthorizationUnrestricted, CurrencyID: candidate.CurrencyID})
	if !errors.Is(err, ErrInvalidBudget) {
		t.Fatalf("overcommitted snapshot error = %v", err)
	}
	if snapshot != original {
		t.Fatal("budget validation mutated its input")
	}
}

func TestCandidateParserKeepsIndependentValidBandsAndRejectsEveryDuplicate(t *testing.T) {
	rows := `{"id":"duplicate","currency_id":"low_grade_spirit_stone","unit_id":"dose","min":2,"max":3},` +
		`{"id":"duplicate","currency_id":"low_grade_spirit_stone","unit_id":"dose","min":4,"max":5},` +
		`{"id":"valid","currency_id":"low_grade_spirit_stone","unit_id":"piece","min":15,"max":30}`
	candidate, diagnostics := loadTestCandidate(t, rows)
	if len(candidate.PriceBands) != 1 || candidate.PriceBands[0].ID != "valid" {
		t.Fatalf("valid bands = %+v; duplicates must all be omitted", candidate.PriceBands)
	}
	duplicates := 0
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "duplicate_id" {
			duplicates++
		}
	}
	if duplicates != 2 {
		t.Fatalf("duplicate diagnostics = %d, want one for each duplicate: %+v", duplicates, diagnostics)
	}
}

func TestCandidateParserReportsBadItemWithoutDroppingValidItem(t *testing.T) {
	rows := `{"id":"decimal","currency_id":"low_grade_spirit_stone","unit_id":"dose","min":2.5,"max":3},` +
		`{"id":"valid","currency_id":"low_grade_spirit_stone","unit_id":"piece","min":15,"max":30},` +
		`{"id":"bad_range","currency_id":"low_grade_spirit_stone","unit_id":"piece","min":20,"max":10},` +
		`{"id":"bad_currency","currency_id":"other_coin","unit_id":"dose","min":2,"max":3},` +
		`{"id":"too_large","currency_id":"low_grade_spirit_stone","unit_id":"unit","min":2,"max":1000000000001}`
	candidate, diagnostics := loadTestCandidate(t, rows)
	if len(candidate.PriceBands) != 1 || candidate.PriceBands[0].ID != "valid" {
		t.Fatalf("independent valid band was lost: %+v", candidate.PriceBands)
	}
	if len(diagnostics) != 4 {
		t.Fatalf("got %d diagnostics, want four invalid records: %+v", len(diagnostics), diagnostics)
	}
}

func TestCandidateParserRejectsMissingMalformedAndUnknownMetadata(t *testing.T) {
	for name, input := range map[string]string{
		"missing field":  strings.Replace(fixtureMetadata, `"quantity_limit":1000000000,`, "", 1),
		"malformed JSON": `{"schema_version":`,
		"NaN":            strings.Replace(fixtureMetadata, `"wealth_benchmark":100`, `"wealth_benchmark":NaN`, 1),
		"unknown status": strings.Replace(fixtureMetadata, `"candidate_not_installed"`, `"enabled"`, 1),
		"unknown field":  strings.TrimSuffix(fixtureMetadata, "}") + `,"unexpected":1}`,
		"zero limit":     strings.Replace(fixtureMetadata, `"amount_limit":1000000000000`, `"amount_limit":0`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			input = strings.TrimSuffix(input, "}") + `,"price_bands":[],"wealth_reference_bands":[],"reward_reference_bands":[]}`
			_, _, err := LoadCandidate(strings.NewReader(input))
			if err == nil {
				t.Fatal("LoadCandidate unexpectedly accepted invalid metadata")
			}
		})
	}
}

func TestCandidateParserRequiresBothRangeBoundsAndPreservesExplicitZero(t *testing.T) {
	validRanges := map[string]string{
		"market_factor_permille":    `"market_factor_permille":{"min":800,"max":1200}`,
		"buyback_factor_permille":   `"buyback_factor_permille":{"min":500,"max":700}`,
		"maturity_factor_permille":  `"maturity_factor_permille":{"min":500,"max":2000}`,
		"condition_factor_permille": `"condition_factor_permille":{"min":0,"max":1000}`,
	}
	malformed := []string{
		`{}`,
		`{"min":0}`,
		`{"max":1000}`,
		`{"min":null,"max":1000}`,
	}
	for field, valid := range validRanges {
		for index, badRange := range malformed {
			t.Run(fmt.Sprintf("%s/bad_%d", field, index), func(t *testing.T) {
				input := strings.Replace(fixtureMetadata, valid, `"`+field+`":`+badRange, 1)
				input = strings.TrimSuffix(input, "}") + `,"price_bands":[],"wealth_reference_bands":[],"reward_reference_bands":[]}`
				if _, _, err := LoadCandidate(strings.NewReader(input)); err == nil {
					t.Fatalf("LoadCandidate accepted %s range %s", field, badRange)
				}
			})
		}
	}

	candidate, diagnostics, err := LoadCandidate(strings.NewReader(strings.TrimSuffix(fixtureMetadata, "}") + `,"price_bands":[],"wealth_reference_bands":[],"reward_reference_bands":[]}`))
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("candidate with explicit legal zero boundary rejected: diagnostics=%+v error=%v", diagnostics, err)
	}
	if candidate.ConditionFactorPermille.Min != 0 || candidate.ConditionFactorPermille.Max != 1000 {
		t.Fatalf("explicit condition zero was not preserved: %+v", candidate.ConditionFactorPermille)
	}
}

func TestAffordableQuantityAndShareUseIntegerCurrencyChecks(t *testing.T) {
	candidate, _ := loadTestCandidate(t, standardBands())
	balance := Money{Amount: 100, CurrencyID: candidate.CurrencyID}
	price := Money{Amount: 3, CurrencyID: candidate.CurrencyID}
	count, err := candidate.AffordableQuantity(balance, price, "dose")
	if err != nil || count != 33 {
		t.Fatalf("affordable count = %d, %v; want 33", count, err)
	}
	share, err := candidate.SharePermille(Money{Amount: 15, CurrencyID: candidate.CurrencyID}, balance)
	if err != nil || share != 150 {
		t.Fatalf("share = %d, %v; want 150 permille", share, err)
	}
	if _, err := candidate.AffordableQuantity(balance, Money{Amount: 3, CurrencyID: "other_coin"}, "dose"); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("cross-currency affordable count error = %v", err)
	}
	if _, err := candidate.AffordableQuantity(Money{Amount: MaxAmount, CurrencyID: candidate.CurrencyID}, Money{Amount: 1, CurrencyID: candidate.CurrencyID}, "dose"); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("quantity above candidate limit error = %v", err)
	}
	if _, err := candidate.SharePermille(balance, Money{CurrencyID: candidate.CurrencyID}); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("zero denominator share error = %v", err)
	}
}

func TestCheckSpendRejectsForgedInconsistentBudget(t *testing.T) {
	candidate, _ := loadTestCandidate(t, standardBands())
	amount := Money{Amount: 1, CurrencyID: candidate.CurrencyID}
	for _, budget := range []SpendableBudget{
		{CurrencyID: candidate.CurrencyID, Available: 0, AuthorizedLeft: 1, Allowed: 1},
		{CurrencyID: candidate.CurrencyID, Available: 1, AuthorizedLeft: 0, Allowed: 1},
		{CurrencyID: candidate.CurrencyID, Available: -1, AuthorizedLeft: 1, Allowed: 0},
	} {
		if err := candidate.CheckSpend(budget, amount); !errors.Is(err, ErrInvalidBudget) {
			t.Fatalf("inconsistent budget %+v error = %v", budget, err)
		}
	}
}

func TestCalculationsDoNotMutateCandidateOrInputSnapshots(t *testing.T) {
	candidate, _ := loadTestCandidate(t, standardBands())
	before := append([]PriceBand(nil), candidate.PriceBands...)
	quote, err := candidate.RetailQuote("ordinary_recovery", Money{Amount: 3, CurrencyID: candidate.CurrencyID}, Quantity{Count: 4, UnitID: "dose"}, 1000)
	if err != nil || quote.Total.Amount != 12 {
		t.Fatalf("quote = %+v, %v", quote, err)
	}
	if fmt.Sprint(candidate.PriceBands) != fmt.Sprint(before) {
		t.Fatal("quote mutated candidate bands")
	}
}
