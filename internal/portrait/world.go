package portrait

import "ta/internal/catalogart"

// WorldURL keeps cross-world identities isolated while retaining the historical
// Fanren defaults for unbound legacy stories. It never enumerates a catalog.
func (l *Library) WorldURL(world, sourceKey string, display bool) string {
	role := "avatar"
	if display {
		role = "display"
	}
	if url := catalogart.Default().URL(world, "npc", sourceKey, 1, role); url != "" {
		return url
	}
	if world != "" && world != "fanren-jie" {
		return ""
	}
	if display {
		return l.DisplayURL(sourceKey)
	}
	return l.URL(sourceKey)
}
