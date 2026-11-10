package tools

import (
	"context"
	"encoding/json"
	"strings"

	agent "ta/agent"

	"ta/internal/interactive"
)

// This file gives the model a way to learn a real price before it narrates one.
//
// 《自制-灵石经济与简单交易》 §4.3 fixes prices against the world content version
// and forbids them moving with the player's wallet, and §9 says asking a price
// costs nothing and is not a transaction. Both requirements point at the same
// shape: the price has to be calculated by the rules and handed to the model,
// rather than invented by the prose and then settled at a different number. The
// tool is read-only for exactly that reason — it answers a question and writes
// nothing, so a merchant can quote a price the player walks away from.

type quoteTradeInput struct {
	MarketID  string `json:"market_id,omitempty" jsonschema_description:"武侠、星海、西幻登记市场ID。同地点有多个可选市场时填写。"`
	ActorID   string `json:"actor_id,omitempty" jsonschema:"description=The character the price is for. Omit for the protagonist."`
	Merchant  string `json:"merchant,omitempty" jsonschema:"description=The shop or trader quoting, in the story's own words. Reuse the same name for every deal with that merchant: it is the key their prepared cash position is recorded under."`
	DefID     string `json:"def_id" jsonschema:"description=The registered item definition being priced, such as pill.restore.health.lower or implement.middle.rare."`
	Direction string `json:"direction" jsonschema:"enum=buy,enum=sell,description=buy for what the character would pay the merchant; sell for what the merchant would pay the character."`
	Quantity  int    `json:"quantity,omitempty" jsonschema:"minimum=1,description=How many copies are being priced. Omit for one."`
}

func newInteractiveTradeTools(ctx InteractiveContext) ([]agent.ToolDefinition, error) {
	if ctx.Store == nil || strings.TrimSpace(ctx.StoryID) == "" {
		return nil, nil
	}
	if ctx.MaxResultBytes <= 0 {
		ctx.MaxResultBytes = defaultToolResultMaxBytes
	}
	quoteTool, err := agent.InferTool("quote_trade",
		"查询当前正式规则的一件物品的价格、余额及交易可行性。报价只读。武侠、星海、西幻填写登记 def_id，可指定 market_id；玩家确认购买后用 world.action=adventure 和 adventure.action=buy/sell 提交返回的 market_id、item_id、quantity，并用 source_text 引用本轮授权。凡人填写 merchant，确认后用 trade 模块结算。仅问价时不写成交。",
		func(callCtx context.Context, input quoteTradeInput) (string, error) {
			_ = callCtx
			snapshot, err := ctx.Store.Snapshot(ctx.StoryID, ctx.BranchID)
			if err != nil {
				return "", err
			}
			if _, err := interactive.AdventureActorFromState(snapshot.State, interactive.DefaultActorID); err == nil {
				window, err := ctx.Store.AdventureWindow(ctx.StoryID, ctx.BranchID)
				if err != nil {
					return "", err
				}
				quote, err := window.QuoteTrade(input.MarketID, input.DefID, input.Direction, input.Quantity)
				if err != nil {
					return "", err
				}
				data, err := json.MarshalIndent(quote, "", "  ")
				return string(data), err
			}
			record, err := interactive.QuoteTrade(interactive.StoryCultivationRules(interactive.StoryMeta{ActorStateSchema: snapshot.ActorStateSchema}), snapshot.State, interactive.TradeQuoteRequest{
				ActorID:   input.ActorID,
				Merchant:  input.Merchant,
				DefID:     input.DefID,
				Direction: input.Direction,
				Quantity:  input.Quantity,
			}, interactive.StoryWorldContent(interactive.StoryMeta{ActorStateSchema: snapshot.ActorStateSchema}))
			if err != nil {
				return "", err
			}
			data, err := json.MarshalIndent(record, "", "  ")
			return string(data), err
		})
	if err != nil {
		return nil, err
	}
	descriptor := boundedReadDescriptor(ToolSourceLore, "", agent.ToolResultRecoveryRerun)
	if ctx.MaxResultBytes > 0 {
		descriptor.MaxResultBytes = ctx.MaxResultBytes
	}
	definedQuoteTool, err := defineTool(quoteTool, descriptor)
	if err != nil {
		return nil, err
	}
	return []agent.ToolDefinition{definedQuoteTool}, nil
}
