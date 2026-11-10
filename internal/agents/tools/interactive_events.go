package tools

import (
	"context"
	"fmt"
	"strings"

	agent "ta/agent"

	"ta/internal/interactive"
)

type readEventCardInput struct {
	CardID string `json:"card_id" jsonschema_description:"Event card ID exactly as shown in the event material catalog. When the same ID exists in more than one package, qualify it as package_id:card_id."`
}

type listEventCardsInput struct {
	Keywords   []string `json:"keywords,omitempty" jsonschema_description:"Optional search terms matched against card ID, name, category, intensity, tags, and directory summary. Each term matches independently."`
	Categories []string `json:"categories,omitempty" jsonschema_description:"Optional category filter, matched case-insensitively against the card's category."`
	Tags       []string `json:"tags,omitempty" jsonschema_description:"Optional tag filter. A card matches when it carries any listed tag."`
	Limit      int      `json:"limit,omitempty" jsonschema:"minimum=1,description=Maximum entries to return, default 20."`
}

// newInteractiveEventTools exposes the story's bound event material as a
// searchable directory plus a full-card read.
//
// The catalog is injected into the runtime context, but a large library cannot
// fit there whole. These tools are what keeps every card reachable at that
// point: without them the model could only act on cards that happened to be
// listed, which would make a story's usable material depend on listing order.
func newInteractiveEventTools(ctx InteractiveContext) ([]agent.ToolDefinition, error) {
	packages := ctx.EventPackages
	if len(packages) == 0 {
		return nil, nil
	}
	entries := interactive.EventCatalog(packages)

	readTool, err := agent.InferTool("read_event_card", "Read one event card's complete authored body, including its trigger context, applicability limits, progression, and consequences. The event material catalog lists cards by directory line only; read the card in full before using it. A card is reusable material, not an event that has already happened.", func(_ context.Context, input readEventCardInput) (string, error) {
		pkg, card, err := interactive.FindEventCard(packages, input.CardID)
		if err != nil {
			// FindEventCard reports exactly two outcomes, and neither is a technical
			// fault: the id is not in this story's packs, or it is ambiguous across
			// packs. §8.4 没有相关资料可以自然表达 and §13 NAR-13 分开正常未命中与真实
			// 系统故障 — surfacing either as a tool error would show the player a
			// system failure for a question that was simply answered "no".
			return fmt.Sprintf("No event card was read. This is a normal empty result, not a failure: %v. Continue from the cards already listed and from established fiction and state; do not invent card material.", err), nil
		}
		return interactive.FormatEventCard(pkg, card), nil
	})
	if err != nil {
		return nil, err
	}
	listTool, err := agent.InferTool("list_event_cards", "Search this story's enabled event card directory by keywords, categories, or tags. Returns directory entries only; read a matching card with read_event_card. Use this when the injected catalog does not list the material you need.", func(_ context.Context, input listEventCardsInput) (string, error) {
		matched := interactive.SearchEventCatalog(entries, input.Keywords, input.Categories, input.Tags)
		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		if len(matched) > limit {
			matched = matched[:limit]
		}
		if len(matched) == 0 {
			return "No event card matched. This is a normal empty result, not a failure; adjudicate the turn from established fiction and state without event material.", nil
		}
		lines := make([]string, 0, len(matched)+1)
		for _, entry := range matched {
			line := "- id=" + entry.CardID + " | " + firstNonEmpty(entry.TypeName, entry.CardID)
			if entry.Category != "" {
				line += " | category=" + entry.Category
			}
			if entry.PackageName != "" {
				line += " | package=" + entry.PackageName
			}
			if entry.Summary != "" {
				line += "\n  " + entry.Summary
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n"), nil
	})
	if err != nil {
		return nil, err
	}
	descriptor := boundedReadDescriptor(ToolSourceLore, "", agent.ToolResultRecoveryRerun)
	if ctx.MaxResultBytes > 0 {
		descriptor.MaxResultBytes = ctx.MaxResultBytes
	}
	definedRead, err := defineTool(readTool, descriptor)
	if err != nil {
		return nil, err
	}
	definedList, err := defineTool(listTool, descriptor)
	if err != nil {
		return nil, err
	}
	return []agent.ToolDefinition{definedList, definedRead}, nil
}
