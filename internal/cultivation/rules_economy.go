package cultivation

import "ta/internal/economy"

// EconomyRules describes authored market defaults; balances remain story facts.
type EconomyRules struct {
	Candidate               economy.Candidate `json:"candidate"`
	CurrencyLabel           string            `json:"currency_label"`
	UnitID                  string            `json:"unit_id"`
	MarketPermille          int64             `json:"market_permille"`
	MerchantCash            int64             `json:"merchant_cash"`
	MerchantBuybackPermille int64             `json:"merchant_buyback_permille"`
}

// ReferenceEconomyRules is an explicit starter template, not a live-world fallback.
func ReferenceEconomyRules() EconomyRules {
	currency, unit := "low_grade_spirit_stone", "piece"
	return EconomyRules{CurrencyLabel: "下品灵石", UnitID: unit, MarketPermille: 1000, MerchantCash: 600, MerchantBuybackPermille: 600, Candidate: economy.Candidate{
		SchemaVersion: 1, ID: "denova.first_version.xiuxian_market", DesignVersion: "自制-灵石经济与简单交易 V0.1", Status: "candidate_not_installed", Source: "internal/cultivation first-version item catalogue", CurrencyID: currency,
		AmountLimit: economy.MaxAmount, QuantityLimit: economy.MaxQuantity, WealthBenchmark: 100,
		MarketFactorPermille: economy.IntegerRange{Min: 800, Max: 1200}, BuybackFactorPermille: economy.IntegerRange{Min: 500, Max: 700}, MaturityFactorPermille: economy.IntegerRange{Min: 500, Max: 2000}, ConditionFactorPermille: economy.IntegerRange{Min: 0, Max: 1000},
		WealthReferenceBands: []economy.AmountBand{{ID: "shop.small", CurrencyID: currency, UnitID: unit, Min: 300, Max: 2000}, {ID: "family.minor", CurrencyID: currency, UnitID: unit, Min: 2000, Max: 10000}, {ID: "clan.local", CurrencyID: currency, UnitID: unit, Min: 20000, Max: 100000}, {ID: "sect.seven", CurrencyID: currency, UnitID: unit, Min: 200000, Max: 800000}},
		RewardReferenceBands: []economy.AmountBand{{ID: "service.ordinary", CurrencyID: currency, UnitID: unit, Min: 1, Max: 5}, {ID: "material.ordinary", CurrencyID: currency, UnitID: unit, Min: 3, Max: 15}},
	}}
}

// Rules returns the immutable operation context supplied by the story owner.
func (c Cultivation) Rules() *Rules { return c.rules }
