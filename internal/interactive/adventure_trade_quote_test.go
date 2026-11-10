package interactive

import "testing"

func TestAdventureTradeQuoteUsesLocalMarketAndDoesNotSettle(t *testing.T) {
	rules := adventureTestRules()
	actor := NewAdventureActor(rules)
	window := AdventureWindowState{Location: "船坞", Rules: rules, Actor: actor}
	quote, err := window.QuoteTrade("", "scrap", "buy", 2)
	if err != nil || !quote.Available || quote.MarketID != "dock" || quote.Total != 4 || quote.Balance != 100 {
		t.Fatalf("local quote: %+v %v", quote, err)
	}
	next, _, err := ResolveAdventureAction(rules, actor, AdventureAction{Action: "buy", MarketID: quote.MarketID, ItemID: quote.ItemID, Quantity: quote.Quantity}, "buy")
	if err != nil || actor.Currency-next.Currency != quote.Total || actor.Currency != 100 || actor.Inventory["scrap"] != 10 {
		t.Fatalf("quote wrote state or differed from settlement: %+v %v", next, err)
	}
	sale, err := window.QuoteTrade("dock", "scrap", "sell", 2)
	if err != nil || !sale.Available || sale.Total != 2 {
		t.Fatalf("sale quote: %+v %v", sale, err)
	}
	window.Actor.Currency = 1
	poor, err := window.QuoteTrade("dock", "scrap", "buy", 1)
	if err != nil || poor.Available || poor.Total != 2 || poor.Reason == "" {
		t.Fatalf("unaffordable quote: %+v %v", poor, err)
	}
	window.Location = "异地"
	if _, err := window.QuoteTrade("dock", "scrap", "buy", 1); err == nil {
		t.Fatal("remote market quoted")
	}
}
