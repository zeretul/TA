package storypack

import (
	"testing"

	booklore "ta/internal/book/lore"
)

// A world entry the reader files as 其他 is not on any of the knowledge page's
// shelves: it is not a person, a place, a faction, a rule or a thing. It lands
// beside the protagonist templates, so the world's own geography and rules show
// up under a heading that names neither.
//
// The shipped pack used to do exactly that: six entries typed "setting", a type
// the reader does not define. This asks the question the page asks — what does
// the reader make of every entry the pack hands it — rather than what the JSON
// says.
func TestEveryShippedWorldEntryHasARealShelf(t *testing.T) {
	pack := fanrenPack(t)
	inputs := pack.WorldInputs()
	if len(inputs) != len(pack.World) {
		t.Fatalf("the pack has %d world entries and handed over %d", len(pack.World), len(inputs))
	}
	for _, input := range inputs {
		got := booklore.NormalizeType(input.Type)
		if got == "other" {
			t.Errorf("world entry %s is typed %q and is filed as 其他", input.ID, input.Type)
		}
	}
}

// The reader has to be able to file the types older worlds still carry, because
// those worlds are on disk and being played. This pins the mapping itself.
func TestLegacyEntryTypesStillReachAShelf(t *testing.T) {
	for _, legacy := range []string{"setting", "history"} {
		if got := booklore.NormalizeType(legacy); got != "world" {
			t.Errorf("legacy type %q is filed as %q, want world", legacy, got)
		}
	}
}
