// Package economy contains deterministic, storage-free checks and calculations
// for versioned economy parameters. It does not own items, balances, authority,
// transactions, or a journal; callers pass snapshots and submit any real change
// through the application's existing domain boundary.
package economy

import (
	"fmt"
	"math"
)

const (
	// MaxAmount and MaxQuantity are generic calculation safety bounds. A world
	// candidate may choose lower limits, but cannot raise these bounds.
	MaxAmount   int64 = 1_000_000_000_000
	MaxQuantity int64 = 1_000_000_000
	permille          = 1_000
)

// ErrorCode values are immutable sentinels usable with errors.Is.
type ErrorCode string

func (code ErrorCode) Error() string { return string(code) }

const (
	ErrInvalidAmount     ErrorCode = "invalid amount"
	ErrInvalidQuantity   ErrorCode = "invalid quantity"
	ErrCurrencyMismatch  ErrorCode = "currency mismatch"
	ErrUnitMismatch      ErrorCode = "unit mismatch"
	ErrOutOfRange        ErrorCode = "value outside permitted range"
	ErrOverflow          ErrorCode = "integer overflow"
	ErrNoBuybackQuote    ErrorCode = "no positive buyback quote"
	ErrInsufficientFunds ErrorCode = "insufficient spendable funds"
	ErrInvalidBudget     ErrorCode = "invalid budget snapshot"
	ErrInvalidAuthority  ErrorCode = "invalid spending authority"
	ErrUnknownBand       ErrorCode = "unknown price band"
	ErrInvalidDirection  ErrorCode = "unknown trade direction"
)

// Money is an integer count of the currency's indivisible base unit. The
// package never converts currencies or introduces fractional coin values.
type Money struct {
	Amount     int64  `json:"amount"`
	CurrencyID string `json:"currency_id"`
}

// Quantity is an integer count of one stable, caller-defined measure.
type Quantity struct {
	Count  int64  `json:"count"`
	UnitID string `json:"unit_id"`
}

// PriceBand is a candidate reference range for one currency and measure.
type PriceBand struct {
	ID         string `json:"id"`
	CurrencyID string `json:"currency_id"`
	UnitID     string `json:"unit_id"`
	Min        int64  `json:"min"`
	Max        int64  `json:"max"`
}

// AmountBand represents wealth or reward guidance, not an item sale price.
type AmountBand struct {
	ID         string `json:"id"`
	CurrencyID string `json:"currency_id"`
	UnitID     string `json:"unit_id"`
	Min        int64  `json:"min"`
	Max        int64  `json:"max"`
}

// IntegerRange is an inclusive candidate range for integer permille factors.
type IntegerRange struct {
	Min int64 `json:"min"`
	Max int64 `json:"max"`
}

// Candidate is a read-only content snapshot after per-record diagnostics.
// Invalid individual records are omitted; duplicate IDs omit every duplicate.
type Candidate struct {
	SchemaVersion           int          `json:"schema_version"`
	ID                      string       `json:"id"`
	DesignVersion           string       `json:"design_version"`
	Status                  string       `json:"status"`
	Source                  string       `json:"source"`
	CurrencyID              string       `json:"currency_id"`
	AmountLimit             int64        `json:"amount_limit"`
	QuantityLimit           int64        `json:"quantity_limit"`
	WealthBenchmark         int64        `json:"wealth_benchmark"`
	MarketFactorPermille    IntegerRange `json:"market_factor_permille"`
	BuybackFactorPermille   IntegerRange `json:"buyback_factor_permille"`
	MaturityFactorPermille  IntegerRange `json:"maturity_factor_permille"`
	ConditionFactorPermille IntegerRange `json:"condition_factor_permille"`
	PriceBands              []PriceBand  `json:"price_bands"`
	WealthReferenceBands    []AmountBand `json:"wealth_reference_bands"`
	RewardReferenceBands    []AmountBand `json:"reward_reference_bands"`
}

// Diagnostic identifies one invalid candidate field or record without
// suppressing independent valid records.
type Diagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// TradeDirection is explicit so an omitted or unknown operation cannot silently
// become a retail purchase or a buyback.
type TradeDirection uint8

const (
	TradeDirectionUnspecified TradeDirection = iota
	TradeDirectionRetail
	TradeDirectionBuyback
)

// Quote is a calculation result only. It is not buyer consent, stock, payment,
// item transfer, or permission to complete a transaction.
type Quote struct {
	Direction       TradeDirection `json:"direction"`
	BandID          string         `json:"band_id"`
	BaseUnitPrice   Money          `json:"base_unit_price"`
	QuotedUnitPrice Money          `json:"quoted_unit_price"`
	Quantity        Quantity       `json:"quantity"`
	Total           Money          `json:"total"`
	MarketPermille  int64          `json:"market_factor_permille"`
	BuybackPermille int64          `json:"buyback_factor_permille,omitempty"`
}

// ValueEstimate describes a range derived from an already registered use band.
// It never claims the item is identified, for sale, or wanted by a buyer.
type ValueEstimate struct {
	BandID            string `json:"band_id"`
	CurrencyID        string `json:"currency_id"`
	ReferenceUnitID   string `json:"reference_unit_id"`
	Low               int64  `json:"low"`
	High              int64  `json:"high"`
	MaturityPermille  int64  `json:"maturity_factor_permille"`
	ConditionPermille int64  `json:"condition_factor_permille"`
	Interpretation    string `json:"interpretation"`
}

// AuthorizationMode must be selected explicitly for each budget check.
type AuthorizationMode uint8

const (
	AuthorizationUnspecified AuthorizationMode = iota
	AuthorizationUnrestricted
	AuthorizationLimited
)

// SpendingAuthorization is a snapshot of one handler's authority for this
// operation. It neither grants authority nor records future spending.
type SpendingAuthorization struct {
	Mode       AuthorizationMode
	CurrencyID string
	Limit      int64
	Spent      int64
}

// BudgetSnapshot separates cash, protected funds, and outstanding commitments.
type BudgetSnapshot struct {
	Balance          Money
	ProtectedReserve Money
	Committed        Money
}

// SpendableBudget is derived from a snapshot and authority; it is not stored.
type SpendableBudget struct {
	CurrencyID     string `json:"currency_id"`
	Available      int64  `json:"available"`
	AuthorizedLeft int64  `json:"authorized_left"`
	Allowed        int64  `json:"allowed"`
}

func validIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func validateMoney(value Money, currencyID string, amountLimit int64) error {
	if value.CurrencyID == "" || value.CurrencyID != currencyID {
		return fmt.Errorf("%w: got %q, want %q", ErrCurrencyMismatch, value.CurrencyID, currencyID)
	}
	if value.Amount < 0 || value.Amount > amountLimit {
		return fmt.Errorf("%w: %d", ErrInvalidAmount, value.Amount)
	}
	return nil
}

func validateQuantity(value Quantity, unitID string, quantityLimit int64) error {
	if value.UnitID == "" || value.UnitID != unitID {
		return fmt.Errorf("%w: got %q, want %q", ErrUnitMismatch, value.UnitID, unitID)
	}
	if value.Count <= 0 || value.Count > quantityLimit {
		return fmt.Errorf("%w: %d", ErrInvalidQuantity, value.Count)
	}
	return nil
}

func validateCandidateAmountLimit(value int64) error {
	if value <= 0 || value > MaxAmount {
		return fmt.Errorf("%w: amount limit %d", ErrOutOfRange, value)
	}
	return nil
}

func validateCandidateQuantityLimit(value int64) error {
	if value <= 0 || value > MaxQuantity {
		return fmt.Errorf("%w: quantity limit %d", ErrOutOfRange, value)
	}
	return nil
}

func validateIntegerRange(value IntegerRange, minimum, maximum int64) error {
	if value.Min < minimum || value.Max > maximum || value.Min > value.Max {
		return fmt.Errorf("%w: [%d,%d] outside [%d,%d]", ErrOutOfRange, value.Min, value.Max, minimum, maximum)
	}
	return nil
}

func multiplyChecked(left, right int64) (int64, error) {
	if left < 0 || right < 0 {
		return 0, ErrInvalidAmount
	}
	if left != 0 && right > math.MaxInt64/left {
		return 0, ErrOverflow
	}
	return left * right, nil
}

func divideCeiling(numerator, denominator int64) (int64, error) {
	if denominator <= 0 || numerator < 0 {
		return 0, ErrInvalidAmount
	}
	quotient := numerator / denominator
	if numerator%denominator != 0 {
		if quotient == math.MaxInt64 {
			return 0, ErrOverflow
		}
		quotient++
	}
	return quotient, nil
}

func (candidate Candidate) amountLimit() int64 {
	if candidate.AmountLimit > 0 && candidate.AmountLimit <= MaxAmount {
		return candidate.AmountLimit
	}
	return MaxAmount
}

func (candidate Candidate) quantityLimit() int64 {
	if candidate.QuantityLimit > 0 && candidate.QuantityLimit <= MaxQuantity {
		return candidate.QuantityLimit
	}
	return MaxQuantity
}

// ValidateMoney and ValidateQuantity are reusable input checks for callers.
// Zero money is valid; zero quantity is not a trade.
func (candidate Candidate) ValidateMoney(value Money) error {
	return validateMoney(value, candidate.CurrencyID, candidate.amountLimit())
}

func (candidate Candidate) ValidateQuantity(value Quantity, unitID string) error {
	if !validIdentifier(unitID) {
		return fmt.Errorf("%w: invalid required unit id", ErrUnitMismatch)
	}
	return validateQuantity(value, unitID, candidate.quantityLimit())
}

// PriceBand returns a copy of a registered band by stable ID.
func (candidate Candidate) PriceBand(id string) (PriceBand, error) {
	for _, band := range candidate.PriceBands {
		if band.ID == id {
			return band, nil
		}
	}
	return PriceBand{}, fmt.Errorf("%w: %s", ErrUnknownBand, id)
}

// AvailableBudget validates a read-only budget snapshot and one explicit
// authority mode. It never changes the snapshot or tracks spending.
func (candidate Candidate) AvailableBudget(snapshot BudgetSnapshot, authority SpendingAuthorization) (SpendableBudget, error) {
	if err := validateMoney(snapshot.Balance, candidate.CurrencyID, candidate.amountLimit()); err != nil {
		return SpendableBudget{}, fmt.Errorf("balance: %w", err)
	}
	if err := validateMoney(snapshot.ProtectedReserve, candidate.CurrencyID, candidate.amountLimit()); err != nil {
		return SpendableBudget{}, fmt.Errorf("protected reserve: %w", err)
	}
	if err := validateMoney(snapshot.Committed, candidate.CurrencyID, candidate.amountLimit()); err != nil {
		return SpendableBudget{}, fmt.Errorf("committed funds: %w", err)
	}
	if snapshot.ProtectedReserve.Amount > snapshot.Balance.Amount || snapshot.Committed.Amount > snapshot.Balance.Amount-snapshot.ProtectedReserve.Amount {
		return SpendableBudget{}, ErrInvalidBudget
	}
	available := snapshot.Balance.Amount - snapshot.ProtectedReserve.Amount - snapshot.Committed.Amount
	if authority.CurrencyID != candidate.CurrencyID {
		return SpendableBudget{}, fmt.Errorf("authorization: %w", ErrCurrencyMismatch)
	}

	var authorizedLeft int64
	switch authority.Mode {
	case AuthorizationUnrestricted:
		if authority.Limit != 0 || authority.Spent != 0 {
			return SpendableBudget{}, ErrInvalidAuthority
		}
		authorizedLeft = available
	case AuthorizationLimited:
		if authority.Limit < 0 || authority.Limit > candidate.amountLimit() || authority.Spent < 0 || authority.Spent > authority.Limit {
			return SpendableBudget{}, ErrInvalidAuthority
		}
		authorizedLeft = authority.Limit - authority.Spent
	case AuthorizationUnspecified:
		return SpendableBudget{}, ErrInvalidAuthority
	default:
		return SpendableBudget{}, ErrInvalidAuthority
	}

	allowed := available
	if authorizedLeft < allowed {
		allowed = authorizedLeft
	}
	return SpendableBudget{
		CurrencyID:     candidate.CurrencyID,
		Available:      available,
		AuthorizedLeft: authorizedLeft,
		Allowed:        allowed,
	}, nil
}

// CheckSpend checks a positive amount against a previously calculated budget.
// The caller must still re-read current state and submit atomically.
func (candidate Candidate) CheckSpend(budget SpendableBudget, amount Money) error {
	if budget.CurrencyID != candidate.CurrencyID || amount.CurrencyID != candidate.CurrencyID {
		return ErrCurrencyMismatch
	}
	if amount.Amount <= 0 {
		return ErrInvalidAmount
	}
	if amount.Amount > candidate.amountLimit() {
		return ErrOutOfRange
	}
	if budget.Available < 0 || budget.Available > candidate.amountLimit() || budget.AuthorizedLeft < 0 || budget.AuthorizedLeft > candidate.amountLimit() || budget.Allowed < 0 || budget.Allowed > candidate.amountLimit() || budget.Allowed > budget.Available || budget.Allowed > budget.AuthorizedLeft {
		return ErrInvalidBudget
	}
	if amount.Amount > budget.Allowed {
		return ErrInsufficientFunds
	}
	return nil
}

// AffordableQuantity returns how many whole units fit a valid balance. It does
// not inspect inventory or assert that a seller can fulfill the quantity.
func (candidate Candidate) AffordableQuantity(balance, unitPrice Money, unitID string) (int64, error) {
	if err := validateMoney(balance, candidate.CurrencyID, candidate.amountLimit()); err != nil {
		return 0, err
	}
	if err := validateMoney(unitPrice, candidate.CurrencyID, candidate.amountLimit()); err != nil {
		return 0, err
	}
	if unitPrice.Amount <= 0 {
		return 0, ErrInvalidAmount
	}
	if !validIdentifier(unitID) {
		return 0, ErrUnitMismatch
	}
	count := balance.Amount / unitPrice.Amount
	if count > candidate.quantityLimit() {
		return 0, fmt.Errorf("%w: affordable quantity %d", ErrOutOfRange, count)
	}
	return count, nil
}

// SharePermille computes the floored share of a positive total using integers.
func (candidate Candidate) SharePermille(part, whole Money) (int64, error) {
	if err := validateMoney(part, candidate.CurrencyID, candidate.amountLimit()); err != nil {
		return 0, err
	}
	if err := validateMoney(whole, candidate.CurrencyID, candidate.amountLimit()); err != nil {
		return 0, err
	}
	if whole.Amount == 0 {
		return 0, ErrInvalidAmount
	}
	numerator, err := multiplyChecked(part.Amount, permille)
	if err != nil {
		return 0, err
	}
	return numerator / whole.Amount, nil
}
