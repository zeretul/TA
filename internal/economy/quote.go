package economy

import "fmt"

// RetailQuote calculates a quote from a registered band and fixed base unit
// price. The market factor already includes all justified local adjustments;
// callers must not stack separate discounts on top of this calculation.
func (candidate Candidate) RetailQuote(bandID string, base Money, quantity Quantity, marketPermille int64) (Quote, error) {
	return candidate.quote(TradeDirectionRetail, bandID, base, quantity, marketPermille, 0)
}

// BuybackQuote calculates an estimate of what a merchant could pay. A rounded
// zero result means there is no whole-coin buyback quote, not a zero-cost trade.
func (candidate Candidate) BuybackQuote(bandID string, base Money, quantity Quantity, marketPermille, buybackPermille int64) (Quote, error) {
	return candidate.quote(TradeDirectionBuyback, bandID, base, quantity, marketPermille, buybackPermille)
}

func (candidate Candidate) quote(direction TradeDirection, bandID string, base Money, quantity Quantity, marketPermille, buybackPermille int64) (Quote, error) {
	band, err := candidate.PriceBand(bandID)
	if err != nil {
		return Quote{}, err
	}
	if !validIdentifier(band.ID) || !validIdentifier(band.UnitID) || band.CurrencyID != candidate.CurrencyID || band.Min <= 0 || band.Max < band.Min || band.Max > candidate.amountLimit() {
		return Quote{}, fmt.Errorf("%w: invalid registered price band %q", ErrOutOfRange, bandID)
	}
	if err := validateMoney(base, candidate.CurrencyID, candidate.amountLimit()); err != nil {
		return Quote{}, fmt.Errorf("base price: %w", err)
	}
	if base.Amount < band.Min || base.Amount > band.Max {
		return Quote{}, fmt.Errorf("%w: base price %d outside band [%d,%d]", ErrOutOfRange, base.Amount, band.Min, band.Max)
	}
	if err := validateQuantity(quantity, band.UnitID, candidate.quantityLimit()); err != nil {
		return Quote{}, err
	}
	if marketPermille < candidate.MarketFactorPermille.Min || marketPermille > candidate.MarketFactorPermille.Max {
		return Quote{}, fmt.Errorf("%w: market factor %d", ErrOutOfRange, marketPermille)
	}
	if direction != TradeDirectionRetail && direction != TradeDirectionBuyback {
		return Quote{}, ErrInvalidDirection
	}

	var unitPrice int64
	switch direction {
	case TradeDirectionRetail:
		numerator, multiplyErr := multiplyChecked(base.Amount, marketPermille)
		if multiplyErr != nil {
			return Quote{}, multiplyErr
		}
		unitPrice, err = divideCeiling(numerator, permille)
		if err != nil {
			return Quote{}, err
		}
	case TradeDirectionBuyback:
		if buybackPermille < candidate.BuybackFactorPermille.Min || buybackPermille > candidate.BuybackFactorPermille.Max {
			return Quote{}, fmt.Errorf("%w: buyback factor %d", ErrOutOfRange, buybackPermille)
		}
		numerator, multiplyErr := multiplyChecked(base.Amount, marketPermille)
		if multiplyErr != nil {
			return Quote{}, multiplyErr
		}
		numerator, multiplyErr = multiplyChecked(numerator, buybackPermille)
		if multiplyErr != nil {
			return Quote{}, multiplyErr
		}
		unitPrice = numerator / (permille * permille)
		if unitPrice == 0 {
			return Quote{}, ErrNoBuybackQuote
		}
		retailNumerator, multiplyErr := multiplyChecked(base.Amount, marketPermille)
		if multiplyErr != nil {
			return Quote{}, multiplyErr
		}
		retail, divideErr := divideCeiling(retailNumerator, permille)
		if divideErr != nil {
			return Quote{}, divideErr
		}
		if unitPrice >= retail {
			return Quote{}, fmt.Errorf("%w: buyback price is not below retail", ErrOutOfRange)
		}
	}
	if unitPrice <= 0 || unitPrice > candidate.amountLimit() {
		return Quote{}, fmt.Errorf("%w: quoted unit price %d", ErrOutOfRange, unitPrice)
	}
	total, err := multiplyChecked(unitPrice, quantity.Count)
	if err != nil {
		return Quote{}, err
	}
	if total > candidate.amountLimit() {
		return Quote{}, fmt.Errorf("%w: quote total %d", ErrOutOfRange, total)
	}
	return Quote{
		Direction:       direction,
		BandID:          band.ID,
		BaseUnitPrice:   base,
		QuotedUnitPrice: Money{Amount: unitPrice, CurrencyID: candidate.CurrencyID},
		Quantity:        quantity,
		Total:           Money{Amount: total, CurrencyID: candidate.CurrencyID},
		MarketPermille:  marketPermille,
		BuybackPermille: buybackPermille,
	}, nil
}

// EstimateValue applies only the registered use band and two caller-supplied,
// permitted permille factors. The result is a valuation range, not an appraisal,
// buyer commitment, or authority to sell.
func (candidate Candidate) EstimateValue(bandID string, maturityPermille, conditionPermille int64) (ValueEstimate, error) {
	band, err := candidate.PriceBand(bandID)
	if err != nil {
		return ValueEstimate{}, err
	}
	if band.CurrencyID != candidate.CurrencyID || !validIdentifier(band.UnitID) || band.Min <= 0 || band.Max < band.Min || band.Max > candidate.amountLimit() {
		return ValueEstimate{}, fmt.Errorf("%w: invalid registered value band %q", ErrOutOfRange, bandID)
	}
	if maturityPermille < candidate.MaturityFactorPermille.Min || maturityPermille > candidate.MaturityFactorPermille.Max {
		return ValueEstimate{}, fmt.Errorf("%w: maturity factor %d", ErrOutOfRange, maturityPermille)
	}
	if conditionPermille < candidate.ConditionFactorPermille.Min || conditionPermille > candidate.ConditionFactorPermille.Max {
		return ValueEstimate{}, fmt.Errorf("%w: condition factor %d", ErrOutOfRange, conditionPermille)
	}
	low, err := estimateEndpoint(band.Min, maturityPermille, conditionPermille, false)
	if err != nil {
		return ValueEstimate{}, err
	}
	high, err := estimateEndpoint(band.Max, maturityPermille, conditionPermille, true)
	if err != nil {
		return ValueEstimate{}, err
	}
	if low > candidate.amountLimit() || high > candidate.amountLimit() {
		return ValueEstimate{}, fmt.Errorf("%w: estimated value exceeds amount limit", ErrOutOfRange)
	}
	return ValueEstimate{
		BandID:            band.ID,
		CurrencyID:        candidate.CurrencyID,
		ReferenceUnitID:   band.UnitID,
		Low:               low,
		High:              high,
		MaturityPermille:  maturityPermille,
		ConditionPermille: conditionPermille,
		Interpretation:    "estimate_only_not_identification_or_buyer_commitment",
	}, nil
}

func estimateEndpoint(value, maturityPermille, conditionPermille int64, roundUp bool) (int64, error) {
	numerator, err := multiplyChecked(value, maturityPermille)
	if err != nil {
		return 0, err
	}
	numerator, err = multiplyChecked(numerator, conditionPermille)
	if err != nil {
		return 0, err
	}
	if roundUp {
		return divideCeiling(numerator, permille*permille)
	}
	return numerator / (permille * permille), nil
}
