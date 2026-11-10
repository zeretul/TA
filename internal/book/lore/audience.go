package lore

import "strings"

// NormalizeAudience preserves legacy public entries and fails closed for unknown
// non-empty values. Conditional author material remains GM-only until specific
// facts are committed to a character's knowledge; reading lore never grants it.
func NormalizeAudience(value string) string {
	switch strings.TrimSpace(value) {
	case "":
		return ""
	case "public":
		return "public"
	default:
		return "gm"
	}
}

// IsPublic is shared by character context and player-facing lore pictures.
// Author editors and the narrative referee deliberately retain GM access.
func IsPublic(item Item) bool { return NormalizeAudience(item.Audience) != "gm" }
