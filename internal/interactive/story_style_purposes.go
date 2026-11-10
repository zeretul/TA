package interactive

import "strings"

// StoryStylePurposes is a story's expression configuration: which style governs
// the main narrative, which governs dialogue, and which governs combat
// description. Each purpose is chosen independently, and the latter two may
// follow the main narrative instead of naming their own style.
//
// The split exists so a player can run a serious main narrative with comic
// dialogue and hot-blooded exchanges. It deliberately is not three world
// execution chains: the three purposes only decide how the same already-settled
// events are told, and the fragments are assembled lean rather than by stacking
// three whole system prompts into one request.
//
// An empty Dialogue or Combat means "follow the main narrative". That is stored
// as empty rather than as a copy of the main id, so changing the main style
// later carries the inheriting purposes with it — which is what "follow" has to
// mean to be useful. An empty Main falls back to the story's own teller id.
type StoryStylePurposes struct {
	Main     string `json:"main,omitempty"`
	Dialogue string `json:"dialogue,omitempty"`
	Combat   string `json:"combat,omitempty"`
}

// ResolvedStyleID returns the style id one purpose actually uses, following the
// main narrative where the purpose inherits. storyTellerID is the story's
// own style selection, which is what a purpose falls back to when the purpose
// config names nothing.
//
// The result is never empty: a story always has a main style, so a purpose can
// always resolve to one.
func (p StoryStylePurposes) ResolvedStyleID(purpose, storyTellerID string) string {
	main := firstNonEmptyStyleID(p.Main, storyTellerID)
	switch purpose {
	case PurposeDialogue:
		return firstNonEmptyStyleID(p.Dialogue, main)
	case PurposeCombat:
		return firstNonEmptyStyleID(p.Combat, main)
	default:
		return main
	}
}

// Inherits reports whether a purpose follows the main narrative rather than
// naming its own style. The UI shows the inherited style rather than a blank
// selection, and this is what tells it to.
func (p StoryStylePurposes) Inherits(purpose string) bool {
	switch purpose {
	case PurposeDialogue:
		return strings.TrimSpace(p.Dialogue) == ""
	case PurposeCombat:
		return strings.TrimSpace(p.Combat) == ""
	default:
		return strings.TrimSpace(p.Main) == ""
	}
}

func firstNonEmptyStyleID(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// Normalize trims the stored ids. It does not validate that a style exists:
// resolution and fallback happen against the live library at generation time,
// so a story whose chosen style was deleted still opens and reports the
// difference instead of failing to load.
func (p StoryStylePurposes) Normalize() StoryStylePurposes {
	return StoryStylePurposes{
		Main:     strings.TrimSpace(p.Main),
		Dialogue: strings.TrimSpace(p.Dialogue),
		Combat:   strings.TrimSpace(p.Combat),
	}
}

// PurposeMain, PurposeDialogue and PurposeCombat name the three generation
// purposes. They mirror the teller package's constants; they are redeclared
// here so this package does not depend on the preset library for a vocabulary
// it stores in its own journal.
const (
	PurposeMain     = "main"
	PurposeDialogue = "dialogue"
	PurposeCombat   = "combat"
)
