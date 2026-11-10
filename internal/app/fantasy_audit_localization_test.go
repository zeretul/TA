package app

import (
	"strings"
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
	"unicode"
)

// Optional regression; NOT RUN. Frozen definitions already contain translated
// names, so old-save progression need not depend on the current installed pack.
func TestFantasyAuditFrozenEnglishFoundationNames(t *testing.T) {
	p, err := storypack.NewLibrary(t.TempDir()).Get("sui-guan-realms")
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := p.FrozenGameplay()
	if err != nil {
		t.Fatal(err)
	}
	a := interactive.NewAdventureActor(*frozen.AdventureRules)
	view := adventureProgressionView(*frozen.AdventureRules, a, "en-US")
	if view == nil || strings.IndexFunc(view.FoundationName, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
		t.Fatalf("frozen English progression still contains canonical Chinese names: %+v", view)
	}
}
