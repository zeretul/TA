package interactive

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"ta/internal/cultivation"
	"ta/internal/economy"
)

// StateOpSourceTrade marks an Actor State operation the trade rules produced.
//
// It is its own source kind so the reroll path can drop and re-derive a settled
// trade without touching anything the narrative recorded. A trade is a rule
// outcome, not prose, and the two must stay separable in the journal.
const StateOpSourceTrade = "trade"

// TurnTradeInput is the model-facing trade module of one story turn.
//
// It carries no price. §4.3 fixes prices against the world content version and
// forbids them moving with the player's wallet, so there is nothing here for the
// prose to put a number into: the quote is calculated from the registered
// definition and the merchant's prepared record, and the module only names what
// changed hands.
type TurnTradeInput struct {
	// ActorID is whose purse and pack change. Omit for the protagonist.
	ActorID string `json:"actor_id,omitempty" jsonschema_description:"The character buying or selling. Omit for the protagonist."`
	// Merchant is the shop or trader the deal is with. Its name is the key its
	// prepared cash position is recorded under.
	Merchant string `json:"merchant,omitempty" jsonschema_description:"The shop or trader this deal is with, in the story's own words. Use the same name for every deal with that merchant: it is the key their prepared cash position is recorded under, and a second spelling would prepare a second shop."`
	// Buys are goods the character pays for this turn.
	Buys []TurnTradeBuy `json:"buys,omitempty" jsonschema_description:"Goods the character actually pays for this turn. A price merely heard, asked about or negotiated is not a purchase."`
	// Sells are held copies the character parts with for coin.
	Sells []TurnTradeSell `json:"sells,omitempty" jsonschema_description:"Copies the character actually hands over this turn, named by the key they are recorded under in 重要物品. The character must really hold them, and the merchant must really be able to pay."`
	// Payments are coin handed to someone this turn that is not a shop sale or
	// purchase: a toll, a bribe, a share, a debt paid, a price agreed with a
	// person standing on the road.
	//
	// It exists because 灵石 is rules-owned and the story cannot write it, so a
	// turn whose prose counted coin into another character's hand had no way to
	// record it — the balance stayed where it was and the prose contradicted the
	// panel. §5 still forbids a promise being recorded as paid: a payment is what
	// really changed hands this turn, not what was agreed to.
	Payments []TurnTradePayment `json:"payments,omitempty" jsonschema_description:"Coin that actually changes hands between two characters this turn outside a shop deal — a toll, a bribe, a share, a repayment, a price agreed on the road, or a promised reward finally paid. Set to to the recipient's Actor id exactly as the state handbook lists it, and amount to the whole number of 灵石 really handed over. Set from to the payer's Actor id when someone other than the acting character pays — an NPC paying the player, for instance; omit it when the acting character pays. Both sides must be Actors this story tracks and the payer's purse must really cover it. Do not list a promise, an agreed price not yet paid, or money that merely changed hands in conversation."`
}

// TurnTradePayment is coin handed to one person.
type TurnTradePayment struct {
	// From is the payer's Actor id when someone other than the turn's acting
	// character hands the coin over — an NPC repaying a debt, paying a reward, or
	// settling a price the player was promised. Omitting it means the acting
	// character pays.
	//
	// It exists because the payment used to have no payer at all: the purse read
	// was always actor_id's, so an NPC with six 灵石 could not pay the player four
	// of them — the module had no way to say who was paying. A live run reached
	// exactly that scene and the prose counted coin the rules never moved.
	From string `json:"from,omitempty" jsonschema_description:"The payer's Actor id exactly as the state handbook lists it, when a character other than the one in actor_id hands the coin over — an NPC repaying a debt, paying a promised reward, or settling a price. The rules deduct that character's own 灵石, so it must be an Actor this story tracks with a purse. Omit when the acting character pays."`
	// To is the recipient's Actor id. It is not a free-form name: the payment
	// writes the recipient's own purse, so it must be an Actor this story tracks.
	To string `json:"to" jsonschema_description:"The recipient's Actor id exactly as the state handbook lists it. The rules write that character's own 灵石, so it must be an Actor this story tracks; a shop is paid through merchant and buys instead."`
	// Amount is whole 下品灵石. §14 forbids fractional coin, so a part of one is
	// not a payment.
	Amount int `json:"amount" jsonschema_description:"How many whole 灵石 are handed over. Must be positive; fractional coin does not exist."`
	// Reason is what the payment was for, in the story's words. It is recorded
	// with the write so the journal says why the purse moved.
	Reason string `json:"reason,omitempty" jsonschema_description:"What the coin was for, in the story's own words — 过路钱, 谢礼, 还他的酒账. Omit when the prose already makes it plain."`
}

// TurnTradeBuy is one purchase.
type TurnTradeBuy struct {
	// DefID names a registered catalogue definition.
	DefID string `json:"def_id" jsonschema_description:"The stable identifier of a registered item definition, such as pill.restore.health.lower or implement.middle.rare. The price comes from the registered definition, never from this submission."`
	// Name is what the story calls the copy, and becomes its key in the pack.
	Name string `json:"name" jsonschema_description:"The item's name in the story's own words. It becomes the key the copy is recorded under in 重要物品, so use the same name for more of the same stack. A name is required: an unnamed purchase would record the copy under an English identifier the player cannot read."`
	// Quantity is how many copies are bought. An implement or a piece of armour
	// is one real object.
	Quantity int `json:"quantity,omitempty" jsonschema_description:"How many copies are bought. Omit for one. An implement or a piece of armour is always one real object."`
}

// TurnTradeSell is one sale.
type TurnTradeSell struct {
	// InstanceID is the copy's key under 重要物品.
	InstanceID string `json:"instance_id" jsonschema_description:"The key the copy is recorded under in 重要物品 — the item's own name. The character must really hold it; selling never creates stock."`
	// Quantity is how many copies are handed over.
	Quantity int `json:"quantity,omitempty" jsonschema_description:"How many copies are handed over. Omit for one, or for the whole stack when the merchant can pay for it."`
}

// Empty reports whether the module carries nothing to apply.
func (input TurnTradeInput) Empty() bool {
	return len(input.Buys) == 0 && len(input.Sells) == 0 && len(input.Payments) == 0
}

// cloneTurnTradeInput copies a trade module so an accepted one can be retained in
// a draft without the caller's slices being aliased.
func cloneTurnTradeInput(input *TurnTradeInput) *TurnTradeInput {
	if input == nil {
		return nil
	}
	cloned := *input
	cloned.Buys = append([]TurnTradeBuy(nil), input.Buys...)
	cloned.Sells = append([]TurnTradeSell(nil), input.Sells...)
	cloned.Payments = append([]TurnTradePayment(nil), input.Payments...)
	return &cloned
}

// validateTurnTradeModule rejects a module the rules could not act on.
//
// It checks shape, not outcome. Whether the definition exists, whether the copy
// is held, whether the character can pay and whether the merchant can are the
// rules' answers and are reported in the result, so the player reads a real
// reason rather than a submission error. What must not pass is a missing
// merchant on a shop deal, a missing definition, a missing instance key, a
// missing recipient, a non-positive payment or a negative quantity.
func validateTurnTradeModule(input *TurnTradeInput) *TurnSubmissionDiagnostic {
	if input == nil || input.Empty() {
		return nil
	}
	// A merchant is only required when a shop deal is being recorded. Coin handed
	// to a person on the road is not a deal with a shop, and demanding a merchant
	// for it is what left that payment unrecordable.
	if (len(input.Buys) > 0 || len(input.Sells) > 0) && strings.TrimSpace(input.Merchant) == "" {
		return newTurnSubmissionDiagnostic(
			TurnSubmissionModuleTrade, nil, TurnSubmissionDiagnosticInvalidModule,
			"/trade/merchant", "the shop or trader's name in the story's words",
			"", "A purchase or sale must name the merchant it is with.",
		)
	}
	for index, buy := range input.Buys {
		if strings.TrimSpace(buy.DefID) == "" {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleTrade, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/trade/buys/"+strconv.Itoa(index)+"/def_id", "a registered item definition identifier",
				"", "Each purchase must name a registered definition.",
			)
		}
		if strings.TrimSpace(buy.Name) == "" {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleTrade, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/trade/buys/"+strconv.Itoa(index)+"/name", "the item's name in the story's words",
				"", "Each purchase must name the copy, because the name becomes its key in 重要物品.",
			)
		}
		if buy.Quantity < 0 {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleTrade, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/trade/buys/"+strconv.Itoa(index)+"/quantity", "a positive count, or omitted for one",
				strconv.Itoa(buy.Quantity), "A purchased quantity must be positive, or omitted to buy one.",
			)
		}
	}
	for index, sell := range input.Sells {
		if strings.TrimSpace(sell.InstanceID) == "" {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleTrade, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/trade/sells/"+strconv.Itoa(index)+"/instance_id", "the key the copy is recorded under",
				"", "Each sale must name the copy the character holds.",
			)
		}
		if sell.Quantity < 0 {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleTrade, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/trade/sells/"+strconv.Itoa(index)+"/quantity", "a positive count, or omitted for one",
				strconv.Itoa(sell.Quantity), "A sold quantity must be positive, or omitted to sell one.",
			)
		}
	}
	for index, payment := range input.Payments {
		if strings.TrimSpace(payment.To) == "" {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleTrade, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/trade/payments/"+strconv.Itoa(index)+"/to", "the recipient's Actor id",
				"", "Each payment must name who the coin was handed to; the rules write that character's purse.",
			)
		}
		if payment.Amount <= 0 {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleTrade, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/trade/payments/"+strconv.Itoa(index)+"/amount", "a positive whole number of 灵石",
				strconv.Itoa(payment.Amount), "A payment must hand over at least one whole 灵石; fractional coin does not exist.",
			)
		}
	}
	return nil
}

// TradeResult reports what one turn's trade module did, in player-readable terms.
type TradeResult struct {
	// ActorID is the character whose purse and pack changed.
	ActorID string
	// Merchant is the shop the deals were with.
	Merchant string
	// Purchased lists the goods the character really paid for.
	Purchased []TradeLine
	// Sold lists the copies the character really handed over.
	Sold []TradeLine
	// Paid lists the coin really handed to a person outside a shop deal.
	Paid []TradePaymentLine
	// Balance is the purse after every settled line.
	Balance int64
	// Refused lists deals that were not applied, with the reason.
	Refused []TradeRefusal
}

// TradeLine is one settled deal.
type TradeLine struct {
	DefID     string
	Name      string
	Quantity  int64
	UnitPrice int64
	Total     int64
}

// TradePaymentLine is one settled payment between two people.
type TradePaymentLine struct {
	// From is the purse the coin left. It is not always ActorID: a payment may be
	// made by another tracked character, and the line has to say which one so the
	// journal and the refusal wording name the real payer.
	From   string
	To     string
	Amount int64
	Reason string
}

// TradeRefusal is one deal that was not applied, with the reason the player
// needs to read.
type TradeRefusal struct {
	Action string `json:"action"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// TradeQuoteRequest is one question about a price.
//
// Asking is free: §9 says seeing a price, asking it and negotiating it all cost
// nothing, and only an accepted quote becomes a transaction. Nothing here writes
// state, and the answer is what the model is allowed to narrate.
type TradeQuoteRequest struct {
	ActorID   string `json:"actor_id,omitempty" jsonschema_description:"The character the price is for. Omit for the protagonist."`
	Merchant  string `json:"merchant" jsonschema_description:"The shop or trader quoting, in the story's own words."`
	DefID     string `json:"def_id" jsonschema_description:"The registered definition being priced."`
	Direction string `json:"direction" jsonschema:"enum=buy,enum=sell" jsonschema_description:"buy for what the character would pay the merchant; sell for what the merchant would pay the character."`
	Quantity  int    `json:"quantity,omitempty" jsonschema_description:"How many copies are being priced. Omit for one."`
}

// QuoteTrade calculates a quote against committed story state without changing
// it.
//
// The same calculation runs inside the settlement, so a price the model quoted
// and a price the rules charge cannot disagree. A merchant the story has not
// dealt with yet is priced at the position it would be prepared with, because
// asking a shop for a price is not the same as opening its books: the record is
// only committed when a deal actually settles.
func QuoteTrade(cultivationRules *cultivation.Rules, state map[string]any, request TradeQuoteRequest, worlds ...*WorldContent) (TradeQuoteRecord, error) {
	actorID := normalizeStatePanelActorID(request.ActorID)
	if actorID == "" {
		actorID = DefaultActorID
	}
	merchant := strings.TrimSpace(request.Merchant)
	if merchant == "" {
		return TradeQuoteRecord{}, errors.New("trade: a quote needs a merchant")
	}
	quantity := int64(request.Quantity)
	if quantity <= 0 {
		quantity = 1
	}
	balance := int64(xiuxianStoredInt(state, actorID, xiuxianFieldCurrency))
	merchantRecord, _ := xiuxianPreparedMerchant(cultivationRules,
		state, merchant)

	var (
		quote xiuxianTradeQuote
		err   error
	)
	switch strings.ToLower(strings.TrimSpace(request.Direction)) {
	case "buy":
		if err := spiritSupplyAvailable(state, request.DefID, merchant, worlds...); err != nil {
			return TradeQuoteRecord{}, err
		}
		quote, err = xiuxianQuotePurchase(cultivationRules,
			request.DefID, "", quantity, xiuxianTradeParameters(cultivationRules).MarketPermille, state)
	case "sell":
		quote, err = xiuxianQuoteSale(cultivationRules,
			request.DefID, "", quantity, xiuxianTradeParameters(cultivationRules).MarketPermille, merchantRecord.Buyback, state)
	default:
		return TradeQuoteRecord{}, errors.New("trade: direction must be buy or sell")
	}
	if err != nil {
		return TradeQuoteRecord{}, err
	}
	record := tradeQuoteRecordFor(quote, balance, merchantRecord.Cash)
	record.Merchant = merchant
	record.Note = xiuxianTradeQuoteNote(quote.Direction)
	return record, nil
}

// xiuxianTradeQuoteNote states what a quote is and is not, in the model's own
// terms.
//
// §9 and §13.2 both insist that a price is not a promise to buy and that an
// estimate is not an appraisal. Saying so beside the number is what stops the
// prose from treating a quoted figure as a settled deal.
func xiuxianTradeQuoteNote(direction economy.TradeDirection) string {
	if direction == economy.TradeDirectionBuyback {
		return "这是商家在当前条件下愿意支付的整枚灵石单价，不是估价，也不是承诺无限量收购；实际成交还要核对商家的现银。"
	}
	return "这是当前条件下的零售单价，不会因角色身上灵石多少而改变；询价与议价本身不扣款，只有明确成交才结算。"
}

// xiuxianTradeOps applies one turn's trade module and returns the writes that
// record it.
//
// Each line is its own transaction: §10 requires the whole amount to be checked
// and both sides written at once, and forbids a partial deduction. A line the
// character cannot pay for, or a merchant cannot pay for, is refused and writes
// nothing — including not touching the lines that did settle, because those were
// separate deals. What never happens is a purchase that deducts part of its
// price, or a sale that takes the goods and pays nothing.
func xiuxianTradeOps(
	state map[string]any,
	system StoryDirectorActorStateSystem,
	actorID string,
	input TurnTradeInput,
	turnID string,
) ([]ActorStateOp, TradeResult, bool) {
	result := TradeResult{ActorID: normalizeStatePanelActorID(actorID), Merchant: strings.TrimSpace(input.Merchant)}
	if result.ActorID == "" || !ActorStateSystemIsXiuxian(system) {
		return nil, result, false
	}
	templateID, found := actorTemplateIDFromStateOrSystem(state, system, result.ActorID)
	if !found {
		return nil, result, false
	}
	template := actorStateTemplateByID(system, templateID)
	if !actorStateTemplateIsXiuxian(template, system.CultivationRules) {
		return nil, result, false
	}
	// A purse and a pack the schema never declared would be values the interface
	// cannot read, and a shop record nobody declared would be a number with no
	// place in the story.
	if !actorStateTemplateDeclaresField(template, xiuxianFieldCurrency) ||
		!actorStateTemplateDeclaresField(template, xiuxianFieldImportantItems) {
		return nil, result, false
	}
	if !actorStateTemplateDeclaresField(actorStateTemplateByID(system, ActorStateStoryContextTemplateID), xiuxianFieldMerchants) {
		return nil, result, false
	}
	shopDeal := len(input.Buys) > 0 || len(input.Sells) > 0
	if input.Empty() || (shopDeal && result.Merchant == "") {
		return nil, result, true
	}

	catalog := storyItemCatalog(system.CultivationRules,
		state)
	// Every purse this turn moves coin out of or into, as this module leaves it.
	//
	// It used to be a single `balance` for the acting character plus a map of
	// recipients, which made the payer a constant: an NPC holding six 灵石 could
	// not hand the player four, because there was nowhere to say who was paying
	// and the balance read was always actor_id's. A live run reached that scene
	// and the prose counted coin the rules never moved.
	//
	// The map is a projection, not a second ledger: each entry starts from the
	// committed purse the first time that character is touched and only moves
	// when a line is really accepted. Reading the opening purse per line would
	// make a second write overwrite the first and lose coin (payer 20, recipient
	// 7, pay 3 then 5 left the recipient at 12 rather than 15).
	purses := map[string]int64{}
	purseOpening := map[string]int64{}
	purseOrder := make([]string, 0, 4)
	purseReasons := map[string]string{}
	// The acting character's purse is always seeded: every buy and sell settles
	// against them, so the write has to know their opening balance even on a
	// turn that only hands coin to somebody else.
	purseOrder = append(purseOrder, result.ActorID)
	purseOpening[result.ActorID] = int64(xiuxianStoredInt(state, result.ActorID, xiuxianFieldCurrency))
	purses[result.ActorID] = purseOpening[result.ActorID]
	balanceOf := func(id string) int64 {
		if value, seen := purses[id]; seen {
			return value
		}
		value := int64(xiuxianStoredInt(state, id, xiuxianFieldCurrency))
		purses[id] = value
		purseOpening[id] = value
		purseOrder = append(purseOrder, id)
		return value
	}
	var merchantRecord xiuxianMerchantBalance
	merchantChanged := false
	if shopDeal {
		var prepared bool
		merchantRecord, prepared = xiuxianPreparedMerchant(system.CultivationRules,
			state, result.Merchant)
		merchantChanged = prepared
	}

	pack, _ := xiuxianEquipmentEntry(state, result.ActorID, xiuxianFieldImportantItems)
	nextPack := make(map[string]any, len(pack)+len(input.Buys))
	for key, value := range pack {
		nextPack[key] = value
	}
	packChanged := false
	// Buys and sells always settle against the acting character, so they run on a
	// local that is written back into purses before the payment loop reads it.
	balance := purseOpening[result.ActorID]

	// Purchases. The balance is checked against the whole total before anything
	// is written, which is §10's 一次性验证总额.
	for _, request := range input.Buys {
		defID := strings.TrimSpace(request.DefID)
		name := strings.TrimSpace(request.Name)
		quantity := int64(request.Quantity)
		if quantity <= 0 {
			quantity = 1
		}
		if err := spiritSupplyAvailable(state, defID, result.Merchant, system.WorldContent); err != nil {
			result.Refused = append(result.Refused, TradeRefusal{Action: "buy", Name: name, Reason: err.Error()})
			continue
		}
		definition, ok := catalog.Definition(defID)
		if !ok {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "buy", Name: name,
				Reason: xiuxianTradeRefusalWords(cultivation.ErrItemDefinitionNotFound),
			})
			continue
		}
		if definition.Rarity == cultivation.RarityUnique {
			// A unique definition is one object in the world. Buying more than one
			// copy in a single line, or buying a second one while already holding
			// it, would put two of the only object in the pack; the counter says
			// so before any coin moves.
			quantity = 1
			if heldName, held := xiuxianPackHeldDefinition(nextPack, definition.ID); held {
				result.Refused = append(result.Refused, TradeRefusal{
					Action: "buy", Name: name,
					Reason: "这件是世上仅此一件的独特之物，你名下已经有这件（记为「" + heldName + "」）；商铺不会再有第二件可卖。",
				})
				continue
			}
			if heldName, held := xiuxianHeldDefinition(state, result.ActorID, definition.ID); held {
				result.Refused = append(result.Refused, TradeRefusal{
					Action: "buy", Name: name,
					Reason: "这件是世上仅此一件的独特之物，你名下已经有这件（记为「" + heldName + "」）；商铺不会再有第二件可卖。",
				})
				continue
			}
		}
		quote, err := xiuxianQuotePurchase(system.CultivationRules,
			defID, name, quantity, xiuxianTradeParameters(system.CultivationRules).MarketPermille, state)
		if err != nil {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "buy", Name: name, Reason: xiuxianTradeRefusalWords(err),
			})
			continue
		}
		if quote.Total > balance {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "buy", Name: name,
				Reason: xiuxianTradeRefusalWords(economy.ErrInsufficientFunds),
			})
			continue
		}
		existing, held := nextPack[name].(map[string]any)
		stacked := int64(0)
		if held {
			if recorded := xiuxianEntryText(existing, "定义"); recorded != "" && recorded != definition.ID {
				result.Refused = append(result.Refused, TradeRefusal{
					Action: "buy", Name: name,
					Reason: "同名物品已按另一登记定义记入行囊；请换一个名称，以免两份不同物品共用一个键。",
				})
				continue
			}
			// Every kind stacks under its name, equipment included. The slot count
			// caps how many may be worn at once, not how many may be kept
			// (《自制-装备栏与法宝御使》§2), so a second sword the player paid for
			// stays in the pack. Counting it out of the stack while still
			// charging took 40 灵石 for one 20 灵石 sword.
			stacked = int64(xiuxianCarriedQuantity(existing))
		}
		// A worn copy is not in the pack, so the stack check above cannot see it.
		// Buying a second 青锋剑 of another grade while the first is on the
		// character passed the check, took the money, and then lost one of the two
		// when the slot was emptied into a name they both shared. The refusal
		// happens here, before any coin moves.
		if slot, clash := xiuxianWornDefinitionMismatch(state, result.ActorID, name, definition.ID); clash {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "buy", Name: name,
				Reason: "你身上正装备着一件同名但不同品级的物品（在「" + slot + "」），两件不能用同一个名称收进行囊；请先把身上那件收回或换一个名称。",
			})
			continue
		}
		balance -= quote.Total
		// The coin leaves one purse and enters the other. Selling already
		// debits the merchant; without this the same 20 灵石 would vanish
		// from the story on a purchase, and a shop could never earn back
		// what it paid for goods (design §12.1: closing cash = opening
		// cash + settled income - settled spend).
		merchantRecord.Cash += quote.Total
		merchantChanged = true
		packChanged = true
		// The quote and the line stay the quantity that was bought; the entry
		// records what the stack now holds. Adding the carried count to the
		// quantity before writing would charge for one amount and record another.
		// A stack already held keeps the wording it was first bought with, so
		// buying a second vial does not restyle the first.
		//
		// A copy with no recorded source is filled in from the merchant this deal
		// really settled against: §4.2 requires an item's actual source to be
		// confirmed when it is handed over, and an empty 来历 leaves the player
		// unable to tell a purchase from a find.
		source := xiuxianEntryText(existing, "来历")
		if source == "" {
			source = fmt.Sprintf("从%s购得。", result.Merchant)
		}
		nextPack[name] = xiuxianItemPackEntryWithFlavor(
			definition,
			name,
			int(quantity+stacked),
			xiuxianEntryText(existing, "描述"),
			source,
		)
		result.Purchased = append(result.Purchased, TradeLine{
			DefID: definition.ID, Name: name, Quantity: quantity,
			UnitPrice: quote.UnitPrice, Total: quote.Total,
		})
	}

	// Sales. The copy must really be held, and the merchant must really be able
	// to pay for it: §10 says a merchant is not an unlimited buyer by default.
	for _, request := range input.Sells {
		instanceID := strings.TrimSpace(request.InstanceID)
		entry, held := nextPack[instanceID].(map[string]any)
		if !held {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "sell", Name: instanceID,
				Reason: "行囊中没有这件物品；出售只能转移已持有的副本，不会凭空生成。",
			})
			continue
		}
		carried := int64(xiuxianCarriedQuantity(entry))
		if carried <= 0 {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "sell", Name: instanceID,
				Reason: xiuxianTradeRefusalWords(cultivation.ErrItemExhausted),
			})
			continue
		}
		defID := xiuxianEntryText(entry, "定义")
		definition, ok := catalog.Definition(defID)
		if !ok {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "sell", Name: instanceID,
				Reason: "这件物品没有登记定义，规则不能给它报价。",
			})
			continue
		}
		quantity := int64(request.Quantity)
		if quantity <= 0 || quantity > carried {
			quantity = carried
		}
		quote, err := xiuxianQuoteSale(system.CultivationRules,
			defID, instanceID, quantity, xiuxianTradeParameters(system.CultivationRules).MarketPermille, merchantRecord.Buyback, state)
		if err != nil {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "sell", Name: instanceID, Reason: xiuxianTradeRefusalWords(err),
			})
			continue
		}
		if quote.Total > merchantRecord.Cash {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "sell", Name: instanceID,
				Reason: "这家商铺当前的现银不足以支付这笔收购；商家能出多少钱是真实现金，不是一句愿意收就成立。",
			})
			continue
		}
		merchantRecord.Cash -= quote.Total
		merchantChanged = true
		balance += quote.Total
		packChanged = true
		if quantity >= carried {
			delete(nextPack, instanceID)
		} else {
			// Selling part of a stack keeps the wording: what is left is the same
			// copies the player was already reading about.
			nextPack[instanceID] = xiuxianItemPackEntryWithFlavor(
				definition,
				instanceID,
				int(carried-quantity),
				xiuxianEntryText(entry, "描述"),
				xiuxianEntryText(entry, "来历"),
			)
		}
		result.Sold = append(result.Sold, TradeLine{
			DefID: definition.ID, Name: instanceID, Quantity: quantity,
			UnitPrice: quote.UnitPrice, Total: quote.Total,
		})
	}

	// Payments. Coin handed to a person is not a shop deal: it moves the payer's
	// purse and the recipient's purse, and nothing else. Both sides are written
	// from the same read, so a payment cannot take the coin and leave the
	// recipient's balance where it was.
	//
	// The payer is per line, not a constant. `actor_id` still names whose pack
	// changes and who buys and sells, but a payment may be made by another tracked
	// character — an NPC repaying a debt or paying a promised reward — and the
	// purse that must cover it is that character's. A live run had exactly that
	// scene with no way to record it: the module could only read actor_id's purse,
	// so the prose counted four 灵石 out of an NPC's hand while the rules moved
	// nothing.
	//
	// Each purse is read from the committed state the first time this turn touches
	// it and then carried as a running total, because the writes are absolute set
	// operations applied in order: reading the opening purse per line would make a
	// second write overwrite the first and lose coin (payer 20, recipient 7, pay 3
	// then 5 left the recipient at 12 rather than 15).
	purses[result.ActorID] = balance
	for _, request := range input.Payments {
		to := normalizeStatePanelActorID(request.To)
		from := normalizeStatePanelActorID(request.From)
		if from == "" {
			from = result.ActorID
		}
		amount := int64(request.Amount)
		reason := strings.TrimSpace(request.Reason)
		if to == "" || to == from {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "pay", Name: request.To,
				Reason: "付款对象必须是本故事中另一个已登记的角色；不能付给自己，也不能付给一个不在 Actor 状态里的名字。",
			})
			continue
		}
		payerTemplateID, found := actorTemplateIDFromStateOrSystem(state, system, from)
		if !found {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "pay", Name: request.From,
				Reason: "付款方不是本分支里实际存在的角色，规则无处扣款；付款方必须是真的参与这次付款的人。",
			})
			continue
		}
		payerTemplate := actorStateTemplateByID(system, payerTemplateID)
		if !actorStateTemplateIsXiuxian(payerTemplate, system.CultivationRules) ||
			!actorStateTemplateDeclaresField(payerTemplate, xiuxianFieldCurrency) {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "pay", Name: request.From,
				Reason: "这个角色没有灵石这一项，规则不能从他名下扣款。",
			})
			continue
		}
		recipientTemplateID, found := actorTemplateIDFromStateOrSystem(state, system, to)
		if !found {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "pay", Name: request.To,
				Reason: "本故事没有这个 Actor，规则无处写入这笔钱；请先把对方建成 Actor，再记这笔付款。",
			})
			continue
		}
		recipientTemplate := actorStateTemplateByID(system, recipientTemplateID)
		if !actorStateTemplateIsXiuxian(recipientTemplate, system.CultivationRules) ||
			!actorStateTemplateDeclaresField(recipientTemplate, xiuxianFieldCurrency) {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "pay", Name: request.To,
				Reason: "这个角色没有灵石这一项，规则不能把钱记到他名下。",
			})
			continue
		}
		if amount <= 0 {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "pay", Name: request.To,
				Reason: "灵石只按整枚计，不足一枚不能算作付款。",
			})
			continue
		}
		payerBalance := balanceOf(from)
		if amount > payerBalance {
			result.Refused = append(result.Refused, TradeRefusal{
				Action: "pay", Name: from,
				Reason: xiuxianPaymentShortfallReason(from, payerBalance, amount),
			})
			continue
		}
		purses[from] = payerBalance - amount
		recipientBalance := balanceOf(to) + amount
		purses[to] = recipientBalance
		if _, recorded := purseReasons[from]; !recorded {
			purseReasons[from] = xiuxianPaymentReason(from, to, reason)
		}
		if _, recorded := purseReasons[to]; !recorded {
			purseReasons[to] = xiuxianPaymentReason(from, to, reason)
		}
		result.Paid = append(result.Paid, TradePaymentLine{From: from, To: to, Amount: amount, Reason: reason})
	}

	result.Balance = purses[result.ActorID]
	if len(result.Purchased) == 0 && len(result.Sold) == 0 && len(result.Paid) == 0 {
		// Nothing settled. A merchant record prepared for a deal that never
		// happened is not written either: §12.4 permits one preparation when the
		// merchant genuinely matters, and a refused deal is not that moment.
		return nil, result, true
	}

	ops := make([]ActorStateOp, 0, 3+len(purseOrder))
	// Every purse this turn moved gets exactly one absolute write, in the order
	// the purses were first touched. A payment that only moved coin between two
	// characters still writes both, because both really changed.
	for _, actorID := range purseOrder {
		next, seen := purses[actorID]
		if !seen || next == purseOpening[actorID] {
			continue
		}
		reason := purseReasons[actorID]
		if reason == "" {
			reason = "由交易结算写入余额，不由叙述改写。"
		}
		ops = append(ops, ActorStateOp{
			Op:           "set",
			ActorID:      actorID,
			FieldID:      normalizeActorStateFieldName(xiuxianFieldCurrency),
			Value:        float64(next),
			Reason:       reason,
			SourceTurnID: turnID,
			SourceKind:   StateOpSourceTrade,
			SourceID:     "trade",
		})
	}
	// The pack is written only when a buy or sell touched it. A payment that
	// hands over coin alone must not rewrite the pack, or a turn that only paid a
	// toll would claim the pack had changed.
	if packChanged {
		ops = append(ops, ActorStateOp{
			Op:           "set",
			ActorID:      result.ActorID,
			FieldID:      normalizeActorStateFieldName(xiuxianFieldImportantItems),
			Value:        nextPack,
			Reason:       "由买卖成交结算写入行囊，不由叙述改写。",
			SourceTurnID: turnID,
			SourceKind:   StateOpSourceTrade,
			SourceID:     "trade",
		})
	}
	if merchantChanged {
		ops = append(ops, xiuxianMerchantOps(state, merchantRecord, turnID)...)
	}
	return ops, result, true
}

// xiuxianPaymentReason is the journal wording for one hand-to-hand payment.
//
// The write carries both names, because either purse may be the one the player
// is looking at: an NPC paying the player is the payer's write on the payer's
// card and the recipient's write on the player's. The reason is the payer's own
// words when the prose supplied one.
func xiuxianPaymentReason(from, to, reason string) string {
	if reason == "" {
		return "由交易结算写入的付款（" + from + " 付给 " + to + "），不由叙述改写。"
	}
	return "由交易结算写入的付款（" + from + " 付给 " + to + "：" + reason + "），不由叙述改写。"
}

// xiuxianPaymentShortfallReason states a refused payment in terms of the purse
// that was actually short.
//
// The generic insufficient-funds sentence names the acting character, which is
// wrong when an NPC is the one paying: the model reads it, concludes the player
// could not afford it, and narrates the wrong refusal. A live run showed the
// cost — the story said 已付 while the rules had refused for the NPC's purse.
func xiuxianPaymentShortfallReason(payer string, balance, amount int64) string {
	return payer + " 的灵石不足：随身只有 " + strconv.FormatInt(balance, 10) +
		" 枚，这笔要付 " + strconv.FormatInt(amount, 10) +
		" 枚。付款没有发生，正文与事项都不能写成已付或已结清。"
}

// removeTradeActorOps drops a previous trade's writes.
//
// It mirrors the settlement's and the item module's removers so the reroll path
// can discard a settled trade along with the rule outcome it was derived from.
func removeTradeActorOps(ops []ActorStateOp) []ActorStateOp {
	if len(ops) == 0 {
		return nil
	}
	out := make([]ActorStateOp, 0, len(ops))
	for _, op := range ops {
		if op.SourceKind == StateOpSourceTrade {
			continue
		}
		out = append(out, op)
	}
	return normalizeActorStateOps(out)
}
