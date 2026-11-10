package interactiveapp

import (
	"strings"
	"ta/internal/interactive"
	"testing"
	"unicode/utf8"
)

func TestLocalAffairWorldPageUsesOnlyKnownFactsAndWholeEntries(t *testing.T) {
	state := renderTestWorld("Public market")
	state.LocalAffairs = []interactive.LocalAffairView{{ID: "observed-local-case", Title: "The damp basket", LocationID: "yue.market", Hook: "Two baskets have different leaf colors.", Status: "noticed", SourceEventID: "affair-known", Methods: []interactive.LocalAffairMethodView{{ID: "basket", Label: "Inspect the basket", LocationID: "yue.market"}, {ID: "shed", Label: "Compare the shed", LocationID: "yue.market.outskirts"}}}}
	for _, budget := range []int{1, 80, 700, 1500, 4096, interactive.StoryContextMaxBytes} {
		page := renderWorldActions(state, budget)
		if len(page) > budget || !utf8.ValidString(page) {
			t.Fatal("local affair page exceeded budget or split text", budget, len(page))
		}
		if strings.Contains(page, "observed-local-case") && !strings.Contains(page, "affair-known") {
			t.Fatal("local affair entry was cut through its identity/source")
		}
	}
	page := renderWorldActions(state, interactive.StoryContextMaxBytes)
	if !strings.Contains(page, "observed-local-case") || !strings.Contains(page, "The damp basket") || strings.Contains(page, "cause") {
		t.Fatal("unresolved public projection lost its hook or fabricated an answer", page)
	}
	state.LocalAffairs[0].Status = "resolved"
	state.LocalAffairs[0].Cause = "Only one basket stood against the damp wall."
	resolved := renderWorldActions(state, interactive.StoryContextMaxBytes)
	if !strings.Contains(resolved, state.LocalAffairs[0].Cause) {
		t.Fatal("committed payoff not available to next-turn context")
	}
}
