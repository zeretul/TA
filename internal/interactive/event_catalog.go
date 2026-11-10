package interactive

import (
	"fmt"
	"strings"
)

// EventCatalogEntrySummaryChars bounds one catalog entry's summary. The catalog
// is a directory, so its job is to let the model recognise a relevant card and
// then read that card in full; a summary long enough to act on would invite
// using it as a substitute for the card's own trigger, limits and consequences.
const EventCatalogEntrySummaryChars = 240

// EventCatalogEntry is one discoverable event card. It carries identity and
// applicability only: the card's behaviour stays in the card body, which is
// read on demand through read_event_card.
type EventCatalogEntry struct {
	CardID      string
	PackageID   string
	PackageName string
	TypeName    string
	Category    string
	Intensity   string
	Tags        []string
	Summary     string
}

// EventCatalog lists every enabled card of every enabled package in authored
// order. Order follows the pack author's arrangement rather than an ID sort, so
// a large library does not systematically favour cards whose IDs sort first.
func EventCatalog(packages []EventPackage) []EventCatalogEntry {
	entries := make([]EventCatalogEntry, 0, len(packages))
	for _, pkg := range packages {
		if !pkg.Enabled {
			continue
		}
		for _, card := range pkg.Events {
			if !card.Enabled {
				continue
			}
			entries = append(entries, EventCatalogEntry{
				CardID:      strings.TrimSpace(card.ID),
				PackageID:   strings.TrimSpace(pkg.ID),
				PackageName: strings.TrimSpace(pkg.Name),
				TypeName:    strings.TrimSpace(card.TypeName),
				Category:    strings.TrimSpace(card.Category),
				Intensity:   strings.TrimSpace(card.Intensity),
				Tags:        append([]string(nil), card.Tags...),
				Summary:     truncateRunes(eventCardSummaryFromMarkdown(card.DescriptionMarkdown, card.TypeName), EventCatalogEntrySummaryChars),
			})
		}
	}
	return entries
}

// FindEventCard resolves one card by ID for a full read. A card ID is only
// unique within its package, so an ambiguous ID is reported instead of silently
// answering with whichever package happened to be listed first.
func FindEventCard(packages []EventPackage, cardID string) (EventPackage, EventCard, error) {
	cardID = strings.TrimSpace(cardID)
	if cardID == "" {
		return EventPackage{}, EventCard{}, fmt.Errorf("事件卡 ID 不能为空")
	}
	packageID := ""
	if index := strings.Index(cardID, ":"); index >= 0 {
		packageID = strings.TrimSpace(cardID[:index])
		cardID = strings.TrimSpace(cardID[index+1:])
	}
	var matches []struct {
		pkg  EventPackage
		card EventCard
	}
	for _, pkg := range packages {
		if !pkg.Enabled {
			continue
		}
		if packageID != "" && strings.TrimSpace(pkg.ID) != packageID {
			continue
		}
		for _, card := range pkg.Events {
			if !card.Enabled || strings.TrimSpace(card.ID) != cardID {
				continue
			}
			matches = append(matches, struct {
				pkg  EventPackage
				card EventCard
			}{pkg: pkg, card: card})
		}
	}
	switch len(matches) {
	case 0:
		return EventPackage{}, EventCard{}, fmt.Errorf("事件卡不存在: %s", cardID)
	case 1:
		return matches[0].pkg, matches[0].card, nil
	default:
		ids := make([]string, 0, len(matches))
		for _, match := range matches {
			ids = append(ids, strings.TrimSpace(match.pkg.ID)+":"+cardID)
		}
		return EventPackage{}, EventCard{}, fmt.Errorf("事件卡 ID 在多个事件包中重复，请使用限定 ID: %s", strings.Join(ids, ", "))
	}
}

// EventCatalogMarkdown renders the directory the model reads before deciding
// which card, if any, fits this turn.
//
// Unlike a concatenation of card bodies, this never carries a card's behaviour,
// so there is nothing here whose tail could be cut off. When the directory
// itself is larger than its budget, the omitted count is stated explicitly and
// the caller is told to search: silently dropping cards would make a story's
// available material depend on listing order, and pretending the directory was
// complete would let the model believe a card does not exist.
func EventCatalogMarkdown(packages []EventPackage, limitBytes int) string {
	entries := EventCatalog(packages)
	if len(entries) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "### Event material catalog\n\n%d enabled event cards are available to this story. Each entry is a directory line only: read a card in full with read_event_card before using it. A card is reusable material, not something that has already happened, and nothing here requires you to use a card this turn.\n\n", len(entries))
	header := sb.Len()
	listed := 0
	for _, entry := range entries {
		line := renderEventCatalogEntry(entry)
		if limitBytes > 0 && sb.Len()+len([]byte(line))+1 > limitBytes {
			break
		}
		sb.WriteString(line)
		sb.WriteString("\n")
		listed++
	}
	if listed < len(entries) {
		fmt.Fprintf(&sb, "\n%d further cards are not listed here because the catalog exceeded its %d-byte budget. Use list_event_cards with keywords, categories or tags to find them; the directory is searchable in full even when this summary is not.\n", len(entries)-listed, limitBytes)
	}
	if sb.Len() == header {
		// Not one entry fit. Returning the header alone would look like an empty
		// catalog, so say plainly that the budget, not the library, is the limit.
		fmt.Fprintf(&sb, "No card entry fits in the %d-byte catalog budget. Use list_event_cards to search the full catalog.\n", limitBytes)
	}
	return strings.TrimSpace(sb.String())
}

func renderEventCatalogEntry(entry EventCatalogEntry) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- id=%s | %s", entry.CardID, firstNonEmptyString(entry.TypeName, entry.CardID))
	if entry.Category != "" {
		fmt.Fprintf(&sb, " | category=%s", entry.Category)
	}
	if entry.Intensity != "" {
		fmt.Fprintf(&sb, " | intensity=%s", entry.Intensity)
	}
	if entry.PackageName != "" {
		fmt.Fprintf(&sb, " | package=%s", entry.PackageName)
	}
	if len(entry.Tags) > 0 {
		fmt.Fprintf(&sb, " | tags=%s", strings.Join(entry.Tags, ","))
	}
	if entry.Summary != "" {
		fmt.Fprintf(&sb, "\n  %s", entry.Summary)
	}
	return sb.String()
}

// FormatEventCard returns one card's complete authored body. The body is never
// trimmed here: the card's constraints live at its end, so a partial read would
// present a card whose limits the model never saw.
func FormatEventCard(pkg EventPackage, card EventCard) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Event card: %s\n\n", firstNonEmptyString(card.TypeName, card.ID))
	fmt.Fprintf(&sb, "- id: %s\n", card.ID)
	if pkg.ID != "" {
		fmt.Fprintf(&sb, "- package: %s\n", firstNonEmptyString(pkg.Name, pkg.ID))
	}
	if card.Category != "" {
		fmt.Fprintf(&sb, "- category: %s\n", card.Category)
	}
	if card.Intensity != "" {
		fmt.Fprintf(&sb, "- intensity: %s\n", card.Intensity)
	}
	if len(card.Tags) > 0 {
		fmt.Fprintf(&sb, "- tags: %s\n", strings.Join(card.Tags, ", "))
	}
	sb.WriteString("\n")
	sb.WriteString(strings.TrimSpace(card.DescriptionMarkdown))
	return strings.TrimSpace(sb.String())
}

// SearchEventCatalog filters the catalog by the same terms a model would use to
// find a card: keywords across identity and summary, plus category and tag
// constraints. Matching is case-insensitive and never consults card bodies, so
// search cannot become a second, lossier way to read a card.
func SearchEventCatalog(entries []EventCatalogEntry, keywords []string, categories []string, tags []string) []EventCatalogEntry {
	normalizedKeywords := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		if trimmed := strings.ToLower(strings.TrimSpace(keyword)); trimmed != "" {
			normalizedKeywords = append(normalizedKeywords, trimmed)
		}
	}
	categorySet := make(map[string]bool, len(categories))
	for _, category := range categories {
		if trimmed := strings.ToLower(strings.TrimSpace(category)); trimmed != "" {
			categorySet[trimmed] = true
		}
	}
	tagSet := make(map[string]bool, len(tags))
	for _, tag := range tags {
		if trimmed := strings.ToLower(strings.TrimSpace(tag)); trimmed != "" {
			tagSet[trimmed] = true
		}
	}
	result := make([]EventCatalogEntry, 0, len(entries))
	for _, entry := range entries {
		if len(categorySet) > 0 && !categorySet[strings.ToLower(entry.Category)] {
			continue
		}
		if len(tagSet) > 0 && !eventEntryHasAnyTag(entry, tagSet) {
			continue
		}
		if len(normalizedKeywords) > 0 && !eventEntryMatchesKeywords(entry, normalizedKeywords) {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func eventEntryHasAnyTag(entry EventCatalogEntry, wanted map[string]bool) bool {
	for _, tag := range entry.Tags {
		if wanted[strings.ToLower(strings.TrimSpace(tag))] {
			return true
		}
	}
	return false
}

func eventEntryMatchesKeywords(entry EventCatalogEntry, keywords []string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		entry.CardID, entry.TypeName, entry.Category, entry.Intensity, strings.Join(entry.Tags, " "), entry.Summary,
	}, " "))
	for _, keyword := range keywords {
		if strings.Contains(haystack, keyword) {
			return true
		}
	}
	return false
}
