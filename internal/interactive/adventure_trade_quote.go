package interactive

import (
	"fmt"
	"strings"
)

// AdventureTradeQuote is a read-only preview against the same frozen catalog,
// location and balance as the adventure settlement path.
type AdventureTradeQuote struct {
	MarketID  string `json:"market_id"`
	ItemID    string `json:"item_id"`
	Name      string `json:"name"`
	Direction string `json:"direction"`
	Quantity  int    `json:"quantity"`
	UnitPrice int    `json:"unit_price"`
	Total     int    `json:"total"`
	Balance   int    `json:"balance"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

func adventureTradeUnitPrice(item AdventureItem, direction string) int {
	if direction == "sell" {
		return item.Price / 2
	}
	return item.Price
}

// QuoteTrade previews a local market transaction without writing the story.
// An omitted market ID is unambiguous only if exactly one local market offers
// the item. The settlement still independently checks the current state.
func (window AdventureWindowState) QuoteTrade(marketID, itemID, direction string, quantity int) (AdventureTradeQuote, error) {
	direction = strings.TrimSpace(direction)
	if direction != "buy" && direction != "sell" {
		return AdventureTradeQuote{}, fmt.Errorf("trade direction must be buy or sell")
	}
	if quantity == 0 {
		quantity = 1
	}
	if quantity < 1 || quantity > 100 {
		return AdventureTradeQuote{}, fmt.Errorf("trade quantity is outside the supported range")
	}
	rules, err := adventureRulesForActor(window.Rules, window.Actor, itemID)
	if err != nil {
		return AdventureTradeQuote{}, err
	}
	var item AdventureItem
	for _, candidate := range rules.Items {
		if candidate.ID == itemID {
			item = candidate
			break
		}
	}
	if item.ID == "" || item.Price < 1 || item.Unique || direction == "buy" && item.Acquisition != "market" {
		return AdventureTradeQuote{}, fmt.Errorf("item is not tradable")
	}
	offerID := item.ID
	if direction == "sell" {
		offerID = AdventureItemTemplateID(window.Actor, item.ID)
	}
	var match string
	for _, market := range rules.Markets {
		if marketID != "" && market.ID != marketID {
			continue
		}
		if !matchesAdventureLocation(window.Location, market.Location, market.LocationAliases, rules.world) ||
			!adventureMarketOffers(rules, market.ID, offerID) {
			continue
		}
		if match != "" {
			return AdventureTradeQuote{}, fmt.Errorf("several local markets offer this item; specify market_id")
		}
		match = market.ID
	}
	if match == "" {
		return AdventureTradeQuote{}, fmt.Errorf("no matching market offers this item at the current location")
	}
	price := adventureTradeUnitPrice(item, direction)
	quote := AdventureTradeQuote{
		MarketID: match, ItemID: item.ID, Name: item.Name, Direction: direction,
		Quantity: quantity, UnitPrice: price, Total: price * quantity, Balance: window.Actor.Currency,
	}
	_, _, err = ResolveAdventureActionAt(rules, window.Actor, AdventureAction{
		Action: direction, MarketID: match, ItemID: item.ID, Quantity: quantity,
	}, "trade-quote", window.ClockSeconds)
	quote.Available = err == nil
	if err != nil {
		quote.Reason = err.Error()
	}
	return quote, nil
}
