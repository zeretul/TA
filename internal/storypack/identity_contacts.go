package storypack

import "strings"

// CreationContactCandidates lists the people from this world's author catalog
// whom a character starting with this affiliation, in this country, could
// plausibly already know.
//
// It exists so the draft's "plausible contacts" are drawn from the world rather
// than invented: the design forbids a creation draft from adding named people of
// consequence, and the catalog is the only list of named people the pack has
// verified. A candidate here is an offer, not a fact — the prompt says the draft
// may leave them out, and nothing binds them into the story until the player
// actually meets them in play.
//
// Only people reachable without a formal introduction are offered. A sect elder
// who requires an introduction or a major matter is not someone a new character
// knows, and offering them would invite the draft to invent the relationship the
// catalog explicitly withholds.
func (p Pack) CreationContactCandidates(affiliationID, country string, limit int) []CatalogCharacter {
	affiliationID = strings.TrimSpace(affiliationID)
	country = strings.TrimSpace(country)
	if country == "" {
		return nil
	}
	if limit <= 0 {
		limit = 6
	}
	out := make([]CatalogCharacter, 0, limit)
	for _, character := range p.normalizedCatalog() {
		if character.RecordOnly || character.Country != country {
			continue
		}
		if strings.TrimSpace(character.ContactGate) == "formal_introduction_or_major_matter" {
			continue
		}
		// A sect member knows people of their own sect; a loose cultivator knows
		// other unaffiliated people nearby. Crossing the line would let the draft
		// hand a stranger an inside connection.
		if affiliationID == "" || affiliationID == "none" {
			if character.FactionID != "" {
				continue
			}
		} else if character.FactionID != affiliationID {
			continue
		}
		out = append(out, character)
		if len(out) == limit {
			break
		}
	}
	return out
}

// ContactCandidateLine renders one candidate for the creation prompt. It is
// deliberately one line: the draft needs to know who exists and roughly what
// they do, not their profile body.
//
// The punctuation follows the locale. A line read on an English page is the
// prompt's own input as well as the list the page shows, so leaving 中文 full
// stops in it would put them in front of a player who never wrote them.
func ContactCandidateLine(character CatalogCharacter, locale string) string {
	open, close, colon, at := "（", "）", "：", "；常在"
	if isEnglishLocale(locale) {
		open, close, colon, at = " (", ")", ": ", "; usually at "
	}
	var sb strings.Builder
	sb.WriteString(character.Name)
	if role := strings.TrimSpace(character.Role); role != "" {
		sb.WriteString(open)
		sb.WriteString(role)
		sb.WriteString(close)
	}
	if brief := strings.TrimSpace(character.Brief); brief != "" && brief != strings.TrimSpace(character.Role) {
		sb.WriteString(colon)
		sb.WriteString(brief)
	}
	if place := strings.TrimSpace(character.PlaceLabel); place != "" {
		sb.WriteString(at)
		sb.WriteString(place)
	}
	return sb.String()
}
