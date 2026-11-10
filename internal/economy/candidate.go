package economy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const maxCandidateBytes int64 = 4 << 20

const ErrInvalidCandidate ErrorCode = "invalid economy candidate metadata"

type rawCandidate struct {
	SchemaVersion           int             `json:"schema_version"`
	ID                      string          `json:"id"`
	DesignVersion           string          `json:"design_version"`
	Status                  string          `json:"status"`
	Source                  string          `json:"source"`
	CurrencyID              string          `json:"currency_id"`
	AmountLimit             int64           `json:"amount_limit"`
	QuantityLimit           int64           `json:"quantity_limit"`
	WealthBenchmark         int64           `json:"wealth_benchmark"`
	MarketFactorPermille    json.RawMessage `json:"market_factor_permille"`
	BuybackFactorPermille   json.RawMessage `json:"buyback_factor_permille"`
	MaturityFactorPermille  json.RawMessage `json:"maturity_factor_permille"`
	ConditionFactorPermille json.RawMessage `json:"condition_factor_permille"`
	PriceBands              json.RawMessage `json:"price_bands"`
	WealthReferenceBands    json.RawMessage `json:"wealth_reference_bands"`
	RewardReferenceBands    json.RawMessage `json:"reward_reference_bands"`
}

type parsedRecord[T any] struct {
	value T
	id    string
	err   error
}

// LoadCandidate decodes one strict JSON document. Invalid top-level metadata
// rejects the candidate; invalid individual bands produce diagnostics while
// independent valid bands remain available. Every duplicate-ID record is
// omitted, so file order can never choose a winner.
func LoadCandidate(reader io.Reader) (Candidate, []Diagnostic, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxCandidateBytes+1))
	if err != nil {
		return Candidate{}, nil, fmt.Errorf("read candidate: %w", err)
	}
	if int64(len(data)) > maxCandidateBytes {
		return Candidate{}, nil, fmt.Errorf("%w: candidate exceeds %d bytes", ErrInvalidCandidate, maxCandidateBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var raw rawCandidate
	if err := decoder.Decode(&raw); err != nil {
		return Candidate{}, nil, fmt.Errorf("%w: decode metadata: %v", ErrInvalidCandidate, err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return Candidate{}, nil, fmt.Errorf("%w: %v", ErrInvalidCandidate, err)
	}
	if err := validateRawMetadata(raw); err != nil {
		return Candidate{}, nil, err
	}

	candidate := Candidate{
		SchemaVersion:           raw.SchemaVersion,
		ID:                      raw.ID,
		DesignVersion:           raw.DesignVersion,
		Status:                  raw.Status,
		Source:                  raw.Source,
		CurrencyID:              raw.CurrencyID,
		AmountLimit:             raw.AmountLimit,
		QuantityLimit:           raw.QuantityLimit,
		WealthBenchmark:         raw.WealthBenchmark,
		MarketFactorPermille:    decodeRange(raw.MarketFactorPermille),
		BuybackFactorPermille:   decodeRange(raw.BuybackFactorPermille),
		MaturityFactorPermille:  decodeRange(raw.MaturityFactorPermille),
		ConditionFactorPermille: decodeRange(raw.ConditionFactorPermille),
	}

	var diagnostics []Diagnostic
	candidate.PriceBands, diagnostics = loadRecords(raw.PriceBands, "price_bands", decodePriceBand, func(value PriceBand) error {
		if value.CurrencyID != raw.CurrencyID {
			return ErrCurrencyMismatch
		}
		if value.Max > raw.AmountLimit {
			return ErrOutOfRange
		}
		return validatePriceBand(value)
	})
	var recordsDiagnostics []Diagnostic
	candidate.WealthReferenceBands, recordsDiagnostics = loadRecords(raw.WealthReferenceBands, "wealth_reference_bands", decodeAmountBand, func(value AmountBand) error {
		if value.CurrencyID != raw.CurrencyID {
			return ErrCurrencyMismatch
		}
		if value.Max > raw.AmountLimit {
			return ErrOutOfRange
		}
		return validateAmountBand(value)
	})
	diagnostics = append(diagnostics, recordsDiagnostics...)
	candidate.RewardReferenceBands, recordsDiagnostics = loadRecords(raw.RewardReferenceBands, "reward_reference_bands", decodeAmountBand, func(value AmountBand) error {
		if value.CurrencyID != raw.CurrencyID {
			return ErrCurrencyMismatch
		}
		if value.Max > raw.AmountLimit {
			return ErrOutOfRange
		}
		return validateAmountBand(value)
	})
	diagnostics = append(diagnostics, recordsDiagnostics...)
	return candidate, diagnostics, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return fmt.Errorf("trailing data: %w", err)
	}
	return nil
}

func validateRawMetadata(raw rawCandidate) error {
	var problems []string
	if raw.SchemaVersion != 1 {
		problems = append(problems, "schema_version must be 1")
	}
	if !validIdentifier(raw.ID) {
		problems = append(problems, "id is missing or malformed")
	}
	if strings.TrimSpace(raw.DesignVersion) == "" {
		problems = append(problems, "design_version is required")
	}
	if raw.Status != "candidate_not_installed" {
		problems = append(problems, "status must be candidate_not_installed")
	}
	if strings.TrimSpace(raw.Source) == "" {
		problems = append(problems, "source is required")
	}
	if !validIdentifier(raw.CurrencyID) {
		problems = append(problems, "currency_id is missing or malformed")
	}
	if err := validateCandidateAmountLimit(raw.AmountLimit); err != nil {
		problems = append(problems, err.Error())
	}
	if err := validateCandidateQuantityLimit(raw.QuantityLimit); err != nil {
		problems = append(problems, err.Error())
	}
	if raw.WealthBenchmark <= 0 || raw.WealthBenchmark > raw.AmountLimit {
		problems = append(problems, "wealth_benchmark must be positive and within amount_limit")
	}
	for _, value := range []struct {
		name string
		raw  json.RawMessage
		min  int64
		max  int64
	}{
		{"market_factor_permille", raw.MarketFactorPermille, 800, 1200},
		{"buyback_factor_permille", raw.BuybackFactorPermille, 500, 700},
		{"maturity_factor_permille", raw.MaturityFactorPermille, 500, 2000},
		{"condition_factor_permille", raw.ConditionFactorPermille, 0, 1000},
	} {
		rangeValue, err := decodeRequiredRange(value.raw)
		if err != nil {
			problems = append(problems, value.name+": "+err.Error())
			continue
		}
		if err := validateIntegerRange(rangeValue, value.min, value.max); err != nil {
			problems = append(problems, value.name+": "+err.Error())
		}
	}
	for _, value := range []struct {
		name string
		raw  json.RawMessage
	}{
		{"price_bands", raw.PriceBands},
		{"wealth_reference_bands", raw.WealthReferenceBands},
		{"reward_reference_bands", raw.RewardReferenceBands},
	} {
		if err := validateRequiredArray(value.raw); err != nil {
			problems = append(problems, value.name+": "+err.Error())
		}
	}
	if len(problems) != 0 {
		return fmt.Errorf("%w: %s", ErrInvalidCandidate, strings.Join(problems, "; "))
	}
	return nil
}

func validateRequiredArray(raw json.RawMessage) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("required array is missing")
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil || values == nil {
		return errors.New("must be an array")
	}
	return nil
}

func decodeRange(raw json.RawMessage) IntegerRange {
	rangeValue, _ := decodeRequiredRange(raw)
	return rangeValue
}

func decodeRequiredRange(raw json.RawMessage) (IntegerRange, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return IntegerRange{}, errors.New("required range is missing")
	}
	var value struct {
		Min *int64 `json:"min"`
		Max *int64 `json:"max"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return IntegerRange{}, err
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return IntegerRange{}, err
	}
	if value.Min == nil || value.Max == nil {
		return IntegerRange{}, errors.New("min and max are required integer fields")
	}
	return IntegerRange{Min: *value.Min, Max: *value.Max}, nil
}

func loadRecords[T any](raw json.RawMessage, path string, decode func(json.RawMessage) (T, string, error), validate func(T) error) ([]T, []Diagnostic) {
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, []Diagnostic{{Code: "invalid_collection", Path: path, Message: "must be an array"}}
	}
	parsed := make([]parsedRecord[T], len(rows))
	counts := make(map[string]int, len(rows))
	for index, row := range rows {
		value, id, err := decode(row)
		if err == nil {
			err = validate(value)
		}
		parsed[index] = parsedRecord[T]{value: value, id: id, err: err}
		if id != "" {
			counts[id]++
		}
	}

	valid := make([]T, 0, len(parsed))
	var diagnostics []Diagnostic
	for index, record := range parsed {
		rowPath := fmt.Sprintf("%s[%d]", path, index)
		invalid := false
		if record.err != nil {
			diagnostics = append(diagnostics, Diagnostic{Code: "invalid_record", Path: rowPath, Message: record.err.Error()})
			invalid = true
		}
		if record.id != "" && counts[record.id] > 1 {
			diagnostics = append(diagnostics, Diagnostic{Code: "duplicate_id", Path: rowPath, Message: "all records with this ID are omitted"})
			invalid = true
		}
		if !invalid {
			valid = append(valid, record.value)
		}
	}
	return valid, diagnostics
}

func decodePriceBand(raw json.RawMessage) (PriceBand, string, error) {
	var value PriceBand
	if err := decodeStrict(raw, &value); err != nil {
		return value, value.ID, err
	}
	return value, value.ID, nil
}

func decodeAmountBand(raw json.RawMessage) (AmountBand, string, error) {
	var value AmountBand
	if err := decodeStrict(raw, &value); err != nil {
		return value, value.ID, err
	}
	return value, value.ID, nil
}

func decodeStrict(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return ensureJSONEnd(decoder)
}

func validatePriceBand(value PriceBand) error {
	if !validIdentifier(value.ID) || !validIdentifier(value.CurrencyID) || !validIdentifier(value.UnitID) {
		return errors.New("id, currency_id, and unit_id must be stable identifiers")
	}
	if value.Min <= 0 || value.Max < value.Min || value.Max > MaxAmount {
		return fmt.Errorf("min/max must satisfy 0 < min <= max <= %d", MaxAmount)
	}
	return nil
}

func validateAmountBand(value AmountBand) error {
	if !validIdentifier(value.ID) || !validIdentifier(value.CurrencyID) || !validIdentifier(value.UnitID) {
		return errors.New("id, currency_id, and unit_id must be stable identifiers")
	}
	if value.Min <= 0 || value.Max < value.Min || value.Max > MaxAmount {
		return fmt.Errorf("min/max must satisfy 0 < min <= max <= %d", MaxAmount)
	}
	return nil
}
