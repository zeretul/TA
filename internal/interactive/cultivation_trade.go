package interactive

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"ta/internal/cultivation"
	"ta/internal/economy"
)

// This file binds the deterministic economy core to the story's own state.
//
// 《自制-灵石经济与简单交易》 separates four facts the design refuses to merge:
// a definition exists, a price band is registered, a quote is calculated, and a
// transaction really happened. The economy package already owns the middle two
// and nothing else. This file is the only place that turns a calculated quote
// into committed state, and it does so exactly once per trade:
//
//   - §12.1 keeps personal cash, shop funds and faction treasuries apart, so the
//     buyer's balance and the merchant's balance are separate fields.
//   - §10 requires the whole amount to be checked and both sides written in one
//     transaction. A purchase the character cannot pay for writes nothing at
//     all, rather than deducting part of it.
//   - §4.3 says prices do not move with the player's wallet, so the market
//     factor comes from the merchant's prepared record, never from the balance.
//   - §14 says the quoted unit price is rounded once and then multiplied by the
//     quantity, so splitting one order into two does not change the total.

// xiuxianTradeCurrencyID is the economy candidate's stable currency identifier.
//
// It is an ASCII identifier because economy.Candidate validates identifiers as
// [a-z0-9_.-]; the player-facing wording is 下品灵石 and lives on the field.
const xiuxianTradeCurrencyID = "low_grade_spirit_stone"

// xiuxianTradeUnitID is the measure one item definition is counted in.
//
// §13.1 prices a standard usable batch of a material, one dose of a pill and one
// piece of gear, and the first-version catalogue has no stackable material yet,
// so every registered entry is priced per piece.
const xiuxianTradeUnitID = "piece"

// xiuxianTradeMarketPermille is the first-version local market factor.
//
// §14 permits 800–1200‰ and requires the factor to be fixed per market rather
// than re-rolled. The public market of the first world pack is an ordinary
// bazaar, which the design places at the neutral point.
const xiuxianTradeMarketPermille int64 = 1000

// xiuxianTradeBuybackPermille is the merchant's first-version buyback factor.
//
// §4.2 permits 500–700‰ and requires the buyback price to stay below retail.
// 600‰ is the design's own worked example.
const xiuxianTradeBuybackPermille int64 = 600

// xiuxianTradeWealthBenchmark is the reference purchasing power §4.2 publishes
// for an ordinary 炼气 cultivator: about 100 whole 下品灵石. It is a sample of
// what such a person can spend, not a default purse and not an income.
const xiuxianTradeWealthBenchmark int64 = 100

// xiuxianTradeAmountLimit bounds one quote and one balance.
//
// §14 sets the integer ceiling at 10^12. The story's own wallet is far below it;
// the limit exists so an overflow is a refusal rather than a wrapped number.
const xiuxianTradeAmountLimit int64 = economy.MaxAmount

// xiuxianTradeQuantityLimit bounds one order.
const xiuxianTradeQuantityLimit int64 = economy.MaxQuantity

// xiuxianTradePriceBandPrefix makes each definition's band name stable and
// unique, so a band can never be confused with another definition's.
const xiuxianTradePriceBandPrefix = "item."

// xiuxianEconomyCandidate builds the first-version candidate in Go.
//
// There is no candidate JSON file: §5 requires the world pack to carry static
// rules and candidate entries, but the first version's item catalogue is code
// (internal/cultivation), so its prices are code too. Deriving the bands from
// the catalogue is what keeps the two from drifting — a definition added without
// a price simply has no band, and a quote for it is refused rather than guessed.
func xiuxianEconomyCandidate(cultivationRules *cultivation.Rules, states ...map[string]any) economy.Candidate {
	parameters := xiuxianTradeParameters(cultivationRules)
	candidate := parameters.Candidate
	candidate.PriceBands = nil
	for _, definition := range storyItemCatalog(cultivationRules, states...).Definitions() {
		if definition.BasePrice <= 0 {
			continue
		}
		candidate.PriceBands = append(candidate.PriceBands, economy.PriceBand{ID: xiuxianTradePriceBandPrefix + definition.ID, CurrencyID: candidate.CurrencyID, UnitID: parameters.UnitID, Min: definition.BasePrice, Max: definition.BasePrice})
	}
	return candidate
}
func xiuxianTradeParameters(rules *cultivation.Rules) cultivation.EconomyRules {
	if rules != nil {
		return rules.Economy
	}
	return cultivation.ReferenceEconomyRules()
}

// xiuxianPriceBandFor returns the registered band for one definition.
func xiuxianPriceBandFor(definition cultivation.ItemDef) (string, error) {
	if definition.BasePrice <= 0 {
		return "", cultivation.ErrItemUnpriced
	}
	return xiuxianTradePriceBandPrefix + definition.ID, nil
}

// xiuxianTradeRefusalWords turns an economy refusal into the sentence a player
// reads.
//
// §11 requires a failure to name its real cause and to leave no partial
// deduction, so "you cannot afford it" and "the merchant will not pay for it"
// are distinct sentences rather than one generic error.
func xiuxianTradeRefusalWords(err error) string {
	switch {
	case errors.Is(err, economy.ErrInsufficientFunds):
		return "灵石不足，这笔交易不成立；余额与货物都不会有任何变化。"
	case errors.Is(err, economy.ErrNoBuybackQuote):
		return "当前条件下没有整枚灵石的回购报价。"
	case errors.Is(err, economy.ErrUnknownBand):
		return "这件物品没有登记过的价格区间，规则不能给它报价。"
	case errors.Is(err, cultivation.ErrItemUnpriced):
		return "这件物品没有公布价格，不能按市价买卖。"
	case errors.Is(err, cultivation.ErrItemDefinitionNotFound):
		return "没有这个已登记物品定义。"
	case errors.Is(err, economy.ErrOutOfRange):
		return "数量或金额超出规则允许的范围。"
	case errors.Is(err, economy.ErrCurrencyMismatch), errors.Is(err, economy.ErrUnitMismatch):
		return "报价的币种或计量单位不符，不能混算。"
	case errors.Is(err, economy.ErrOverflow):
		return "金额计算溢出，规则拒绝这笔交易。"
	}
	return err.Error()
}

// xiuxianMerchantBalance is one merchant's prepared cash position.
//
// §12.4 allows the application to prepare a faction's cash position once, the
// first time it genuinely matters, and forbids agreeing later to a number the
// player asks for. The record is therefore committed state: the first trade with
// a merchant writes it, and every later trade reads it back rather than choosing
// a new one.
type xiuxianMerchantBalance struct {
	Name    string
	Cash    int64
	Buyback int64
}

// xiuxianMerchantRecord reads one merchant's committed cash position.
//
// A merchant the story has never dealt with has no record yet, which is the
// state §12.4 describes as "not yet prepared". It is reported as absent rather
// than defaulted to zero: zero would mean a merchant who cannot pay, and the
// rules would then refuse every sale to a shop that has simply never been
// visited.
func xiuxianMerchantRecord(state map[string]any, name string) (xiuxianMerchantBalance, bool) {
	entries, ok := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldMerchants)
	if !ok {
		return xiuxianMerchantBalance{}, false
	}
	entry, ok := entries[name].(map[string]any)
	if !ok {
		return xiuxianMerchantBalance{}, false
	}
	cash, cashOK := actorStateNumber(entry["现银"])
	buyback, buybackOK := actorStateNumber(entry["回购率"])
	if !cashOK || !buybackOK {
		return xiuxianMerchantBalance{}, false
	}
	return xiuxianMerchantBalance{
		Name:    name,
		Cash:    int64(math.Round(cash)),
		Buyback: int64(math.Round(buyback)),
	}, true
}

// xiuxianMerchantCash is the prepared opening cash for a merchant.
//
// §12.4's 普通小商铺 band is 300–2,000. The design says to prepare a reasonable
// initial position once and never back-fill it to suit a player, so the value is
// fixed rather than rolled: a small bazaar shop opens with 600 whole 下品灵石,
// enough for ordinary gear and consumables and not enough for a top-grade
// treasure. That is a real limit the player can meet, which is the point of the
// rule.
const xiuxianMerchantCash int64 = 600

// xiuxianMerchantBuybackPermille is the buyback factor a prepared shop opens at.
//
// §4.2's permitted range is 500–700‰. 600‰ is the design's own worked example
// and the same factor the quote uses, so a merchant's prepared record and the
// calculation agree instead of disagreeing by omission.
const xiuxianMerchantBuybackPermille int64 = 600

// xiuxianPreparedMerchant returns the merchant's committed record, preparing it
// once when it is genuinely absent.
//
// prepared reports whether this call created the record, so the caller knows to
// write it back. §12.4 permits exactly one preparation; a record that already
// exists is returned unchanged, which is what stops a later turn from quietly
// raising a shop's cash because the player wants to sell something expensive.
func xiuxianPreparedMerchant(cultivationRules *cultivation.Rules, state map[string]any, name string) (xiuxianMerchantBalance, bool) {
	if record, ok := xiuxianMerchantRecord(state, name); ok {
		return record, false
	}
	return xiuxianMerchantBalance{
		Name:    name,
		Cash:    xiuxianTradeParameters(cultivationRules).MerchantCash,
		Buyback: xiuxianTradeParameters(cultivationRules).MerchantBuybackPermille,
	}, true
}

// xiuxianMerchantOps writes the story's merchant record.
//
// The record lives on the story-context actor because a shop is part of the
// story's world rather than of any one character, and because §12.1 keeps the
// shop's funds separate from the character's own purse.
func xiuxianMerchantOps(state map[string]any, record xiuxianMerchantBalance, turnID string) []ActorStateOp {
	entries, _ := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldMerchants)
	next := make(map[string]any, len(entries)+1)
	for key, value := range entries {
		next[key] = value
	}
	entry := map[string]any{
		"现银":  float64(record.Cash),
		"回购率": float64(record.Buyback),
	}
	next[record.Name] = entry
	return []ActorStateOp{{
		Op:           "set",
		ActorID:      DefaultStoryContextActorID,
		FieldID:      normalizeActorStateFieldName(xiuxianFieldMerchants),
		Value:        next,
		Reason:       "由交易结算写入商贾现银与回购率，不由叙述改写。",
		SourceTurnID: turnID,
		SourceKind:   StateOpSourceTrade,
		SourceID:     "trade",
	}}
}

// xiuxianTradeQuote is one calculated quote, in the story's own terms.
//
// It is deliberately a calculation result: §9 says seeing a price, asking it and
// negotiating it all cost nothing, and only an accepted quote becomes a
// transaction.
type xiuxianTradeQuote struct {
	CurrencyLabel string
	Direction     economy.TradeDirection
	Definition    cultivation.ItemDef
	Name          string
	Quantity      int64
	UnitPrice     int64
	Total         int64
	BandID        string
}

// xiuxianQuotePurchase calculates a retail quote for one definition.
func xiuxianQuotePurchase(cultivationRules *cultivation.Rules, defID, name string, quantity int64, marketPermille int64, states ...map[string]any) (xiuxianTradeQuote, error) {
	catalog := storyItemCatalog(cultivationRules,
		states...)
	definition, ok := catalog.Definition(strings.TrimSpace(defID))
	if !ok {
		return xiuxianTradeQuote{}, cultivation.ErrItemDefinitionNotFound
	}
	bandID, err := xiuxianPriceBandFor(definition)
	if err != nil {
		return xiuxianTradeQuote{}, err
	}
	candidate := xiuxianEconomyCandidate(cultivationRules,
		states...)
	quote, err := candidate.RetailQuote(
		bandID,
		economy.Money{Amount: definition.BasePrice, CurrencyID: candidate.CurrencyID},
		economy.Quantity{Count: quantity, UnitID: xiuxianTradeParameters(cultivationRules).UnitID},
		marketPermille,
	)
	if err != nil {
		return xiuxianTradeQuote{}, err
	}
	return xiuxianTradeQuote{
		Direction:     economy.TradeDirectionRetail,
		CurrencyLabel: xiuxianTradeParameters(cultivationRules).CurrencyLabel,
		Definition:    definition,
		Name:          name,
		Quantity:      quantity,
		UnitPrice:     quote.QuotedUnitPrice.Amount,
		Total:         quote.Total.Amount,
		BandID:        bandID,
	}, nil
}

// xiuxianQuoteSale calculates a buyback quote for one definition.
func xiuxianQuoteSale(cultivationRules *cultivation.Rules, defID, name string, quantity int64, marketPermille, buybackPermille int64, states ...map[string]any) (xiuxianTradeQuote, error) {
	catalog := storyItemCatalog(cultivationRules,
		states...)
	definition, ok := catalog.Definition(strings.TrimSpace(defID))
	if !ok {
		return xiuxianTradeQuote{}, cultivation.ErrItemDefinitionNotFound
	}
	bandID, err := xiuxianPriceBandFor(definition)
	if err != nil {
		return xiuxianTradeQuote{}, err
	}
	candidate := xiuxianEconomyCandidate(cultivationRules,
		states...)
	quote, err := candidate.BuybackQuote(
		bandID,
		economy.Money{Amount: definition.BasePrice, CurrencyID: candidate.CurrencyID},
		economy.Quantity{Count: quantity, UnitID: xiuxianTradeParameters(cultivationRules).UnitID},
		marketPermille,
		buybackPermille,
	)
	if err != nil {
		return xiuxianTradeQuote{}, err
	}
	return xiuxianTradeQuote{
		Direction:     economy.TradeDirectionBuyback,
		CurrencyLabel: xiuxianTradeParameters(cultivationRules).CurrencyLabel,
		Definition:    definition,
		Name:          name,
		Quantity:      quantity,
		UnitPrice:     quote.QuotedUnitPrice.Amount,
		Total:         quote.Total.Amount,
		BandID:        bandID,
	}, nil
}

// TradeQuoteRecord is the model-facing form of a quote.
//
// §7 permits the application to give the model the current quote and the
// available authority, and forbids exposing a faction's whole treasury. This
// record carries exactly what the model needs to narrate the counter: what was
// priced, at what unit price, for how much, and whether the character can pay.
type TradeQuoteRecord struct {
	Direction    string `json:"direction"`
	DefID        string `json:"def_id"`
	Name         string `json:"name"`
	Quantity     int64  `json:"quantity"`
	UnitPrice    int64  `json:"unit_price"`
	Total        int64  `json:"total"`
	Currency     string `json:"currency"`
	Balance      int64  `json:"balance"`
	Affordable   int64  `json:"affordable_quantity"`
	Payable      bool   `json:"payable"`
	Merchant     string `json:"merchant"`
	MerchantCash int64  `json:"merchant_cash"`
	Note         string `json:"note"`
}

// tradeQuoteRecordFor builds the record for one calculated quote.
func tradeQuoteRecordFor(quote xiuxianTradeQuote, balance, merchantCash int64) TradeQuoteRecord {
	record := TradeQuoteRecord{
		DefID:        quote.Definition.ID,
		Name:         quote.Name,
		Quantity:     quote.Quantity,
		UnitPrice:    quote.UnitPrice,
		Total:        quote.Total,
		Currency:     quote.CurrencyLabel,
		Balance:      balance,
		MerchantCash: merchantCash,
	}
	switch quote.Direction {
	case economy.TradeDirectionRetail:
		record.Direction = "buy"
		record.Payable = quote.Total <= balance
		if unit := quote.UnitPrice; unit > 0 {
			record.Affordable = balance / unit
		}
	case economy.TradeDirectionBuyback:
		record.Direction = "sell"
		record.Payable = quote.Total <= merchantCash
	default:
		record.Direction = "unknown"
	}
	return record
}

// xiuxianTradeInputFingerprint is the identity of one trade request.
//
// §4.3 requires the same goods in the same market under the same conditions to
// reuse the promised quote rather than re-roll it on a retry. The fingerprint is
// what a transaction can be checked against: a request whose fingerprint does not
// match the one the quote was calculated for is a different trade and must be
// re-quoted rather than silently accepted at the old price.
func xiuxianTradeInputFingerprint(merchant, defID string, direction economy.TradeDirection, quantity int64) string {
	return fmt.Sprintf("%s|%s|%d|%d", strings.TrimSpace(merchant), strings.TrimSpace(defID), direction, quantity)
}
