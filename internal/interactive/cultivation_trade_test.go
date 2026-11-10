package interactive

import (
	"fmt"
	"strings"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/economy"
)

// These tests cover 《自制-灵石经济与简单交易》 as it happens during play: a
// merchant quotes a price, the character accepts it, and the purse, the pack and
// the merchant's own cash all move once, together.
//
// The design's separation is what is asserted here. A price is calculated from a
// registered definition and cannot follow the wallet (§4.3); asking costs
// nothing (§9); a whole order is checked and written in one transaction (§10);
// and a shop's cash is a real limit rather than a willingness to buy (§12.4).

// tradeTestState builds a character with a declared purse and pack plus the
// story-context actor a merchant record lives on.
func tradeTestState(t *testing.T, purse float64, pack map[string]any, merchants map[string]any) map[string]any {
	t.Helper()
	if pack == nil {
		pack = map[string]any{}
	}
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldCurrency:       purse,
		xiuxianFieldImportantItems: pack,
	})
	storyContext := map[string]any{}
	if merchants != nil {
		storyContext[xiuxianFieldMerchants] = merchants
	}
	state[actorStateRoot].(map[string]any)[DefaultStoryContextActorID] = map[string]any{
		"id": DefaultStoryContextActorID, "template_id": ActorStateStoryContextTemplateID,
		"state": storyContext,
	}
	return state
}

// TestTheDesignsFixedQuoteFixtures is §15's worked example run through the
// story's own quote path: B=3, M=1000, K=600, Q=4 gives a retail unit price of 3
// and a total of 12, and a buyback unit price of 1 and a total of 4.
func TestTheDesignsFixedQuoteFixtures(t *testing.T) {
	retail, err := xiuxianQuotePurchase(cultivation.ReferenceRules(),
		"pill.restore.health.lower", "回春丹", 4, xiuxianTradeMarketPermille)
	if err != nil {
		t.Fatal(err)
	}
	if retail.UnitPrice != 3 || retail.Total != 12 {
		t.Errorf("retail = unit %d total %d, want 3 and 12", retail.UnitPrice, retail.Total)
	}

	buyback, err := xiuxianQuoteSale(cultivation.ReferenceRules(),
		"pill.restore.health.lower", "回春丹", 4, xiuxianTradeMarketPermille, xiuxianTradeBuybackPermille)
	if err != nil {
		t.Fatal(err)
	}
	if buyback.UnitPrice != 1 || buyback.Total != 4 {
		t.Errorf("buyback = unit %d total %d, want 1 and 4", buyback.UnitPrice, buyback.Total)
	}
}

// TestAQuoteDoesNotMoveWithTheWallet is §4.3: the same goods in the same market
// cost the same whether the character is rich or poor. Only affordability moves.
func TestAQuoteDoesNotMoveWithTheWallet(t *testing.T) {
	poor := tradeTestState(t, 2, nil, nil)
	rich := tradeTestState(t, 500, nil, nil)

	poorQuote, err := QuoteTrade(cultivation.ReferenceRules(),
		poor, TradeQuoteRequest{Merchant: "坊市药铺", DefID: "implement.middle.rare", Direction: "buy", Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	richQuote, err := QuoteTrade(cultivation.ReferenceRules(),
		rich, TradeQuoteRequest{Merchant: "坊市药铺", DefID: "implement.middle.rare", Direction: "buy", Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if poorQuote.UnitPrice != richQuote.UnitPrice || poorQuote.Total != richQuote.Total {
		t.Errorf("price followed the wallet: poor %d/%d, rich %d/%d",
			poorQuote.UnitPrice, poorQuote.Total, richQuote.UnitPrice, richQuote.Total)
	}
	if poorQuote.Payable {
		t.Error("a 2-stone purse must not be reported as able to pay 60")
	}
	if !richQuote.Payable {
		t.Error("a 500-stone purse must be reported as able to pay 60")
	}
	if poorQuote.Affordable != 0 || richQuote.Affordable != 8 {
		t.Errorf("affordable = %d and %d, want 0 and 8", poorQuote.Affordable, richQuote.Affordable)
	}
}

// TestAskingAPriceWritesNothing is §9: quoting, haggling and walking away are
// free, so the tool that answers the question must not settle anything.
func TestAskingAPriceWritesNothing(t *testing.T) {
	state := tradeTestState(t, 20, nil, nil)
	before := cloneActorStateRoot(state)

	if _, err := QuoteTrade(cultivation.ReferenceRules(),
		state, TradeQuoteRequest{Merchant: "坊市药铺", DefID: "pill.restore.health.lower", Direction: "buy", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := QuoteTrade(cultivation.ReferenceRules(),
		state, TradeQuoteRequest{Merchant: "坊市药铺", DefID: "pill.restore.health.lower", Direction: "sell", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if got, want := stateHashForTest(before), stateHashForTest(state); got != want {
		t.Errorf("quoting changed state: %s -> %s", got, want)
	}
}

// TestAPurchaseSettlesTheWholeAmountOnce is §10: the price, the goods and the
// merchant's record are written in one pass, and the balance pays exactly the
// quoted total.
func TestAPurchaseSettlesTheWholeAmountOnce(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradeTestState(t, 20, nil, nil)

	ops, result, ok := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Merchant: "坊市药铺",
		Buys:     []TurnTradeBuy{{DefID: "implement.lower.common", Name: "青锋剑"}},
	}, "ev1")
	if !ok {
		t.Fatal("a tracked character with the declared fields must accept the module")
	}
	if len(result.Purchased) != 1 || len(result.Refused) != 0 {
		t.Fatalf("purchased = %+v refused = %+v", result.Purchased, result.Refused)
	}
	if result.Balance != 0 {
		t.Errorf("balance = %d, want 0 after paying 20 of 20", result.Balance)
	}

	applied := appliedXiuxianState(t, state, ops)
	if got := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency); got != 0 {
		t.Errorf("committed purse = %d, want 0", got)
	}
	pack, held := xiuxianEquipmentEntry(applied, DefaultActorID, xiuxianFieldImportantItems)
	if !held {
		t.Fatal("the pack was not written")
	}
	entry, ok := pack["青锋剑"].(map[string]any)
	if !ok {
		t.Fatalf("pack = %+v, want a 青锋剑 entry", pack)
	}
	if got := xiuxianEntryText(entry, "定义"); got != "implement.lower.common" {
		t.Errorf("recorded definition = %q", got)
	}
	if got := xiuxianCarriedQuantity(entry); got != 1 {
		t.Errorf("carried = %d, want 1", got)
	}
	if cash := merchantCashForTest(applied, "坊市药铺"); cash != xiuxianMerchantCash+20 {
		t.Errorf("merchant cash = %d, want the prepared %d plus the 20 it took in", cash, xiuxianMerchantCash)
	}
}

// TestAnUnaffordablePurchaseWritesNothing is §10 and §11 together: the whole
// total is checked before anything is written, so a refusal leaves the purse and
// the pack exactly as they were — no partial deduction, no goods on credit.
func TestAnUnaffordablePurchaseWritesNothing(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradeTestState(t, 5, nil, nil)
	before := cloneActorStateRoot(state)

	ops, result, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Merchant: "坊市药铺",
		Buys:     []TurnTradeBuy{{DefID: "implement.lower.common", Name: "青锋剑"}},
	}, "ev1")
	if len(ops) != 0 {
		t.Fatalf("an unaffordable purchase wrote state: %+v", ops)
	}
	if len(result.Refused) != 1 {
		t.Fatalf("refused = %+v, want one refusal", result.Refused)
	}
	if !strings.Contains(result.Refused[0].Reason, "灵石不足") {
		t.Errorf("refusal reason = %q, want it to name the missing funds", result.Refused[0].Reason)
	}
	if got, want := stateHashForTest(before), stateHashForTest(state); got != want {
		t.Error("a refused purchase mutated the input state")
	}
}

// TestSellingRequiresARealCopy is the pack half of the same rule: selling never
// creates stock, so a copy the character does not hold cannot be parted with.
func TestSellingRequiresARealCopy(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradeTestState(t, 20, nil, nil)

	ops, result, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Merchant: "坊市药铺",
		Sells:    []TurnTradeSell{{InstanceID: "回春丹"}},
	}, "ev1")
	if len(ops) != 0 {
		t.Fatalf("selling an unheld copy wrote state: %+v", ops)
	}
	if len(result.Refused) != 1 || !strings.Contains(result.Refused[0].Reason, "行囊中没有这件物品") {
		t.Fatalf("refused = %+v, want the missing-copy reason", result.Refused)
	}
}

// TestSellingIsLimitedByTheMerchantsRealCash is §12.4 and §10: a merchant is not
// an unlimited buyer. The copy is worth 300 and the shop has 10, so the deal is
// refused rather than paid from nowhere.
func TestSellingIsLimitedByTheMerchantsRealCash(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradeTestState(t, 20, map[string]any{
		"斩仙剑": map[string]any{"定义": "implement.top.unique", "数量": float64(1)},
	}, map[string]any{
		"坊市药铺": map[string]any{"现银": float64(10), "回购率": float64(600)},
	})
	before := cloneActorStateRoot(state)

	ops, result, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Merchant: "坊市药铺",
		Sells:    []TurnTradeSell{{InstanceID: "斩仙剑"}},
	}, "ev1")
	if len(ops) != 0 {
		t.Fatalf("a sale the shop cannot pay for wrote state: %+v", ops)
	}
	if len(result.Refused) != 1 || !strings.Contains(result.Refused[0].Reason, "现银") {
		t.Fatalf("refused = %+v, want the cash reason", result.Refused)
	}
	if got, want := stateHashForTest(before), stateHashForTest(state); got != want {
		t.Error("a refused sale mutated the input state")
	}
}

// TestSellingTransfersTheCopyAndPays is the positive half: one held dose leaves
// the pack, the purse gains the quoted total, and the shop's cash falls by the
// same amount. §12.1's separation is what makes this a transfer rather than a
// credit.
func TestSellingTransfersTheCopyAndPays(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradeTestState(t, 20, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(2)},
	}, map[string]any{
		"坊市药铺": map[string]any{"现银": float64(100), "回购率": float64(600)},
	})

	ops, result, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Merchant: "坊市药铺",
		Sells:    []TurnTradeSell{{InstanceID: "回春丹", Quantity: 1}},
	}, "ev1")
	if len(result.Sold) != 1 || len(result.Refused) != 0 {
		t.Fatalf("sold = %+v refused = %+v", result.Sold, result.Refused)
	}
	if result.Sold[0].UnitPrice != 1 || result.Sold[0].Total != 1 {
		t.Errorf("sale = unit %d total %d, want 1 and 1", result.Sold[0].UnitPrice, result.Sold[0].Total)
	}

	applied := appliedXiuxianState(t, state, ops)
	if got := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency); got != 21 {
		t.Errorf("purse = %d, want 21", got)
	}
	pack, _ := xiuxianEquipmentEntry(applied, DefaultActorID, xiuxianFieldImportantItems)
	entry, held := pack["回春丹"].(map[string]any)
	if !held {
		t.Fatal("the remaining copy was lost")
	}
	if got := xiuxianCarriedQuantity(entry); got != 1 {
		t.Errorf("remaining = %d, want 1", got)
	}
	if cash := merchantCashForTest(applied, "坊市药铺"); cash != 99 {
		t.Errorf("merchant cash = %d, want 99", cash)
	}
}

// TestSellingTheWholeStackRemovesTheEntry keeps the pack from carrying a
// zero-quantity ghost the interface would still list.
func TestSellingTheWholeStackRemovesTheEntry(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradeTestState(t, 20, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(1)},
	}, map[string]any{
		"坊市药铺": map[string]any{"现银": float64(100), "回购率": float64(600)},
	})

	ops, _, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Merchant: "坊市药铺",
		Sells:    []TurnTradeSell{{InstanceID: "回春丹"}},
	}, "ev1")
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, DefaultActorID, xiuxianFieldImportantItems)
	if _, held := pack["回春丹"]; held {
		t.Fatalf("pack still holds an emptied entry: %+v", pack)
	}
}

// TestAPreparedMerchantIsNotRaisedOnASecondVisit is §12.4's "prepare once". The
// record written by the first deal is what the second deal reads; a later turn
// cannot quietly choose a richer shop because the player has something to sell.
func TestAPreparedMerchantIsNotRaisedOnASecondVisit(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradeTestState(t, 20, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(1)},
	}, nil)

	ops, _, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Merchant: "坊市药铺",
		Sells:    []TurnTradeSell{{InstanceID: "回春丹"}},
	}, "ev1")
	afterFirst := appliedXiuxianState(t, state, ops)
	if cash := merchantCashForTest(afterFirst, "坊市药铺"); cash != xiuxianMerchantCash-1 {
		t.Fatalf("first deal left cash %d, want %d", cash, xiuxianMerchantCash-1)
	}

	second, prepared := xiuxianPreparedMerchant(cultivation.ReferenceRules(),
		afterFirst, "坊市药铺")
	if prepared {
		t.Error("an existing merchant record was prepared a second time")
	}
	if second.Cash != xiuxianMerchantCash-1 {
		t.Errorf("second visit read cash %d, want the committed %d", second.Cash, xiuxianMerchantCash-1)
	}
}

// TestARefusedDealDoesNotOpenTheShopsBooks keeps §12.4's one preparation for a
// moment that genuinely matters: a refused deal is not that moment, so the shop
// record stays absent and the next attempt prepares it exactly once.
func TestARefusedDealDoesNotOpenTheShopsBooks(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradeTestState(t, 0, nil, nil)

	ops, _, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Merchant: "坊市药铺",
		Buys:     []TurnTradeBuy{{DefID: "pill.restore.health.lower", Name: "回春丹"}},
	}, "ev1")
	if len(ops) != 0 {
		t.Fatalf("a refused deal wrote state: %+v", ops)
	}
	if _, exists := xiuxianMerchantRecord(state, "坊市药铺"); exists {
		t.Error("a refused deal committed a merchant record")
	}
}

// TestAnUnregisteredDefinitionIsRefusedRatherThanPriced is §7's separation of a
// definition from a copy: the rules cannot quote what the catalogue does not
// publish.
func TestAnUnregisteredDefinitionIsRefusedRatherThanPriced(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradeTestState(t, 1000, nil, nil)

	ops, result, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Merchant: "坊市药铺",
		Buys:     []TurnTradeBuy{{DefID: "implement.invented.treasure", Name: "自造神器"}},
	}, "ev1")
	if len(ops) != 0 {
		t.Fatalf("an unregistered definition wrote state: %+v", ops)
	}
	if len(result.Refused) != 1 || !strings.Contains(result.Refused[0].Reason, "没有这个已登记物品定义") {
		t.Fatalf("refused = %+v, want the missing-definition reason", result.Refused)
	}
}

// TestAnUnpricedDefinitionHasNoBand is the price-band half: a definition the
// catalogue holds but does not price must not fall back to a guessed number.
func TestAnUnpricedDefinitionHasNoBand(t *testing.T) {
	_, err := xiuxianPriceBandFor(cultivation.ItemDef{ID: "item.unpriced"})
	if err == nil {
		t.Fatal("an unpriced definition produced a band")
	}
	if !strings.Contains(xiuxianTradeRefusalWords(err), "没有公布价格") {
		t.Errorf("refusal words = %q", xiuxianTradeRefusalWords(err))
	}
}

// TestTheCandidateRegistersOneBandPerPricedDefinition keeps the candidate
// derived from the catalogue: a definition that is priced is tradeable, and one
// that is not simply has no band.
func TestTheCandidateRegistersOneBandPerPricedDefinition(t *testing.T) {
	candidate := xiuxianEconomyCandidate(cultivation.ReferenceRules())
	priced := 0
	for _, definition := range storyItemCatalog(cultivation.ReferenceRules()).Definitions() {
		if definition.BasePrice > 0 {
			priced++
		}
	}
	if len(candidate.PriceBands) != priced {
		t.Fatalf("candidate has %d bands, want %d priced definitions", len(candidate.PriceBands), priced)
	}
	for _, band := range candidate.PriceBands {
		if band.Min != band.Max || band.Min <= 0 {
			t.Errorf("band %s = [%d,%d], want one fixed positive price", band.ID, band.Min, band.Max)
		}
		if band.CurrencyID != candidate.CurrencyID || band.UnitID != xiuxianTradeUnitID {
			t.Errorf("band %s uses %s/%s", band.ID, band.CurrencyID, band.UnitID)
		}
	}
	if candidate.Status != "candidate_not_installed" {
		t.Errorf("status = %q, want candidate_not_installed", candidate.Status)
	}
}

// TestTheTradeModuleRejectsASubmissionTheRulesCannotActOn covers the shape
// checks: they are about what the model failed to say, not about the outcome.
func TestTheTradeModuleRejectsASubmissionTheRulesCannotActOn(t *testing.T) {
	cases := []struct {
		name  string
		input TurnTradeInput
		path  string
	}{
		{"no merchant", TurnTradeInput{Buys: []TurnTradeBuy{{DefID: "pill.restore.health.lower", Name: "回春丹"}}}, "/trade/merchant"},
		{"no definition", TurnTradeInput{Merchant: "坊市药铺", Buys: []TurnTradeBuy{{Name: "回春丹"}}}, "/trade/buys/0/def_id"},
		{"no name", TurnTradeInput{Merchant: "坊市药铺", Buys: []TurnTradeBuy{{DefID: "pill.restore.health.lower"}}}, "/trade/buys/0/name"},
		{"negative buy", TurnTradeInput{Merchant: "坊市药铺", Buys: []TurnTradeBuy{{DefID: "pill.restore.health.lower", Name: "回春丹", Quantity: -1}}}, "/trade/buys/0/quantity"},
		{"no instance", TurnTradeInput{Merchant: "坊市药铺", Sells: []TurnTradeSell{{}}}, "/trade/sells/0/instance_id"},
		{"negative sell", TurnTradeInput{Merchant: "坊市药铺", Sells: []TurnTradeSell{{InstanceID: "回春丹", Quantity: -1}}}, "/trade/sells/0/quantity"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			diagnostic := validateTurnTradeModule(&testCase.input)
			if diagnostic == nil {
				t.Fatal("the submission was accepted")
			}
			if diagnostic.Path != testCase.path {
				t.Errorf("path = %q, want %q", diagnostic.Path, testCase.path)
			}
		})
	}
	if diagnostic := validateTurnTradeModule(&TurnTradeInput{}); diagnostic != nil {
		t.Errorf("an empty module was rejected: %+v", diagnostic)
	}
}

// TestTheTradeModuleRefusesToRunWithoutItsDeclaredFields keeps the module honest
// about where it may write: a system with no purse, no pack or no merchant
// record has no place for the numbers.
func TestTheTradeModuleRefusesToRunWithoutItsDeclaredFields(t *testing.T) {
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)
	input := TurnTradeInput{
		Merchant: "坊市药铺",
		Buys:     []TurnTradeBuy{{DefID: "pill.restore.health.lower", Name: "回春丹"}},
	}

	// The default preset has neither 灵石 nor 商贾, so the xiuxian module must
	// decline rather than write a value no field can read.
	if _, _, ok := xiuxianTradeOps(state, defaultActorStateSystem(), DefaultActorID, input, "ev1"); ok {
		t.Error("the module ran against a non-cultivation system")
	}

	// The cultivation preset declares both, so the same input is accepted. This
	// half keeps the guard from passing merely because everything is refused.
	accepted := tradeTestState(t, 20, nil, nil)
	if _, _, ok := xiuxianTradeOps(accepted, xiuxianTestSystem(), DefaultActorID, input, "ev1"); !ok {
		t.Error("the module declined a character whose schema declares its fields")
	}
}

// TestBuyingIntoAnExistingStackAddsToIt keeps the pack one entry per item rather
// than one entry per purchase.
func TestBuyingIntoAnExistingStackAddsToIt(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradeTestState(t, 20, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(1)},
	}, nil)

	ops, result, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Merchant: "坊市药铺",
		Buys:     []TurnTradeBuy{{DefID: "pill.restore.health.lower", Name: "回春丹", Quantity: 2}},
	}, "ev1")
	if len(result.Refused) != 0 {
		t.Fatalf("refused = %+v", result.Refused)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, DefaultActorID, xiuxianFieldImportantItems)
	entry, _ := pack["回春丹"].(map[string]any)
	if got := xiuxianCarriedQuantity(entry); got != 3 {
		t.Errorf("stack = %d, want 3", got)
	}
	if got := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency); got != 14 {
		t.Errorf("purse = %d, want 14", got)
	}
}

// TestANameAlreadyBoundToAnotherDefinitionIsRefused keeps one pack key from
// describing two different things, which would make a later sale ambiguous.
func TestANameAlreadyBoundToAnotherDefinitionIsRefused(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradeTestState(t, 200, map[string]any{
		"青锋剑": map[string]any{"定义": "implement.lower.common", "数量": float64(1)},
	}, nil)

	ops, result, _ := xiuxianTradeOps(state, system, DefaultActorID, TurnTradeInput{
		Merchant: "坊市药铺",
		Buys:     []TurnTradeBuy{{DefID: "implement.middle.rare", Name: "青锋剑"}},
	}, "ev1")
	if len(ops) != 0 {
		t.Fatalf("a name collision wrote state: %+v", ops)
	}
	if len(result.Refused) != 1 || !strings.Contains(result.Refused[0].Reason, "同名物品") {
		t.Fatalf("refused = %+v, want the name-collision reason", result.Refused)
	}
}

// TestTheNarratorCannotMintCurrency is the field's own gate: 灵石 is settled by
// the trade rules, so a narrative state change must not write it.
func TestTheNarratorCannotMintCurrency(t *testing.T) {
	if !IsXiuxianSettledField(xiuxianTestSystem(), xiuxianFieldCurrency) {
		t.Fatalf("%s is not in the settled set", xiuxianFieldCurrency)
	}
}

// TestTheBuybackFactorStaysBelowRetailForEveryPricedDefinition is §4.2's
// invariant checked across the whole catalogue rather than one sample: no item
// may be bought and sold back at a profit in the same market.
func TestTheBuybackFactorStaysBelowRetailForEveryPricedDefinition(t *testing.T) {
	for _, definition := range storyItemCatalog(cultivation.ReferenceRules()).Definitions() {
		if definition.BasePrice <= 0 {
			continue
		}
		retail, err := xiuxianQuotePurchase(cultivation.ReferenceRules(),
			definition.ID, "", 1, xiuxianTradeMarketPermille)
		if err != nil {
			t.Fatalf("%s retail: %v", definition.ID, err)
		}
		buyback, err := xiuxianQuoteSale(cultivation.ReferenceRules(),
			definition.ID, "", 1, xiuxianTradeMarketPermille, xiuxianTradeBuybackPermille)
		if err != nil {
			// A base price too low to round to a whole coin has no buyback
			// quote at all, which is the design's stated outcome rather than a
			// failure.
			if err == economy.ErrNoBuybackQuote {
				continue
			}
			t.Fatalf("%s buyback: %v", definition.ID, err)
		}
		if buyback.UnitPrice >= retail.UnitPrice {
			t.Errorf("%s: buyback %d is not below retail %d", definition.ID, buyback.UnitPrice, retail.UnitPrice)
		}
	}
}

func merchantCashForTest(state map[string]any, name string) int64 {
	record, _ := xiuxianMerchantRecord(state, name)
	return record.Cash
}

// stateHashForTest is only an identity check: two snapshots of the same
// untouched state must compare equal.
func stateHashForTest(state map[string]any) string {
	return fmt.Sprintf("%#v", state)
}

// TestAPaidCopyIsNotAcquiredTwice is the live acceptance run's bug in one test.
//
// The model submitted the purchase in both modules: items.acquire for the two
// doses it had just paid for, and trade.buys for the same two. Items run first
// and trade runs after them, so the pack went 2 -> 4 -> 6 while the purse paid
// for only two. §10 says the trade module settles the whole amount once, so the
// paid copies must not be acquired a second time by the item module.
func TestAPaidCopyIsNotAcquiredTwice(t *testing.T) {
	trade := &TurnTradeInput{
		Merchant: "周氏药铺",
		Buys:     []TurnTradeBuy{{DefID: "pill.restore.health.lower", Name: "回春丹", Quantity: 2}},
	}
	trimmed := withoutPaidAcquisitions(TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "pill.restore.health.lower", Name: "回春丹", Quantity: 2}},
	}, trade)
	if !trimmed.Empty() {
		t.Fatalf("the paid acquisition survived: %+v", trimmed.Acquire)
	}

	// A free copy of the same name is a real second fact, so only the paid
	// amount is removed from it.
	partial := withoutPaidAcquisitions(TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "pill.restore.health.lower", Name: "回春丹", Quantity: 3}},
	}, trade)
	if len(partial.Acquire) != 1 || partial.Acquire[0].Quantity != 1 {
		t.Fatalf("free copy = %+v, want one dose", partial.Acquire)
	}

	// An unrelated acquisition is untouched.
	other := withoutPaidAcquisitions(TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "implement.lower.common", Name: "铁剑", Quantity: 1}},
	}, trade)
	if len(other.Acquire) != 1 || other.Acquire[0].Name != "铁剑" {
		t.Fatalf("unrelated acquisition = %+v", other.Acquire)
	}
}

// TestThePackReflectsOnePurchaseRatherThanTwo walks the two modules in the
// order the commit applies them, which is what the unit test above cannot see.
func TestThePackReflectsOnePurchaseRatherThanTwo(t *testing.T) {
	system := xiuxianTestSystem()
	state := tradeTestState(t, 20, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(2)},
	}, nil)
	trade := &TurnTradeInput{
		Merchant: "周氏药铺",
		Buys:     []TurnTradeBuy{{DefID: "pill.restore.health.lower", Name: "回春丹", Quantity: 2}},
	}
	items := withoutPaidAcquisitions(TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "pill.restore.health.lower", Name: "回春丹", Quantity: 2}},
	}, trade)

	afterItems := state
	if !items.Empty() {
		ops, _, ok := xiuxianItemOps(state, system, DefaultActorID, items, "ev1")
		if !ok {
			t.Fatal("the item module did not run")
		}
		afterItems = appliedXiuxianState(t, state, ops)
	}
	tradeOps, result, _ := xiuxianTradeOps(afterItems, system, DefaultActorID, *trade, "ev1")
	if len(result.Purchased) != 1 || len(result.Refused) != 0 {
		t.Fatalf("purchased = %+v refused = %+v", result.Purchased, result.Refused)
	}
	applied := appliedXiuxianState(t, afterItems, tradeOps)

	if got := xiuxianStoredInt(applied, DefaultActorID, xiuxianFieldCurrency); got != 14 {
		t.Errorf("purse = %d, want 14", got)
	}
	pack, _ := xiuxianEquipmentEntry(applied, DefaultActorID, xiuxianFieldImportantItems)
	entry, _ := pack["回春丹"].(map[string]any)
	if got := xiuxianCarriedQuantity(entry); got != 4 {
		t.Errorf("held = %d, want 4 (two seeded plus two bought)", got)
	}
}

// TestTheProseCannotRewriteARegisteredCopy is the sell turn's bug in one test.
//
// The prose wrote 回春丹 = 3 into 重要物品 after the character had four, and the
// trade module then deducted one more, so the character ended the turn with two
// after selling one of four. A copy with a registered definition is priced and
// counted by the rules; only entries the catalogue does not know belong to the
// prose.
func TestTheProseCannotRewriteARegisteredCopy(t *testing.T) {
	current := map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(4)},
		"旧信":  map[string]any{"作用": "信物"},
	}
	proposed := map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(3)},
		"旧信":  map[string]any{"作用": "随身带着的信物"},
		"药锄":  map[string]any{"作用": "采药"},
	}
	merged, _ := xiuxianProseCarriedItems(cultivation.ReferenceRules(),
		current, proposed).(map[string]any)
	dose, _ := merged["回春丹"].(map[string]any)
	if got := xiuxianCarriedQuantity(dose); got != 4 {
		t.Errorf("registered copy = %d, want the committed 4", got)
	}
	letter, _ := merged["旧信"].(map[string]any)
	if letter["作用"] != "随身带着的信物" {
		t.Errorf("unregistered entry was not applied: %+v", letter)
	}
	if _, found := merged["药锄"]; !found {
		t.Error("a new unregistered entry was dropped")
	}

	// A story may still remove its own entry entirely.
	removed, _ := xiuxianProseCarriedItems(cultivation.ReferenceRules(),
		current, map[string]any{
			"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(4)},
		}).(map[string]any)
	if _, found := removed["旧信"]; found {
		t.Error("the prose's own entry survived its removal")
	}
}
