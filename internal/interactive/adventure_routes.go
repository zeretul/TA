package interactive

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"ta/internal/exploration"
	"ta/internal/publiquevent"
	"ta/internal/quest"
)

const (
	tideGaugeShortRoute = "lanchuan-route-tide-gauge-short"
	tideGaugeOuterRoute = "lanchuan-route-tide-gauge-outer"
)

// These routes are authored adaptation data: local hub connections and a sparse
// regional spine, with both directions. Durations are paid by the existing
// journey reducer and frozen by its definition hash, never inferred from prose.
//
//go:embed assets/adventure-routes.json
var adventureRouteData []byte

func init() {
	var catalog struct {
		Routes  []exploration.RouteDefinition `json:"routes"`
		Labels  map[string]string             `json:"labels"`
		Aliases map[string][]string           `json:"aliases"`
	}
	if err := json.Unmarshal(adventureRouteData, &catalog); err != nil {
		panic(err)
	}
	seen := map[string]bool{}
	for _, r := range journeyRouteDefinitions {
		seen[r.ID] = true
	}
	for _, r := range catalog.Routes {
		if _, _, d := exploration.ValidateRoute(r, journeySupportedMethods()); len(d) > 0 {
			panic(fmt.Sprintf("invalid adventure route %s: %+v", r.ID, d))
		}
		if seen[r.ID] {
			panic("duplicate adventure route " + r.ID)
		}
		seen[r.ID] = true
		journeyRouteDefinitions = append(journeyRouteDefinitions, r)
	}
	for id, label := range catalog.Labels {
		journeyPlaceLabels[id] = label
	}
	for id, aliases := range catalog.Aliases {
		journeyPlaceAliases[id] = aliases
	}
}

func adventureRouteAvailable(p *storyBranchProjection, routeID string) bool {
	if _, exists := worldJourneyRoute(p, routeID); !exists {
		return false
	}
	if p != nil && p.worldContent() != nil {
		return ownedRouteAvailable(p, routeID)
	}
	if !spiritWorldRouteAvailable(p, routeID) {
		return false
	}
	if !membershipRouteAvailable(p, routeID) {
		return false
	}
	if !realmShortcutAvailable(p, routeID) {
		return false
	}
	switch routeID {
	case tideGaugeShortRoute, tideGaugeOuterRoute:
		if p == nil {
			return false
		}
		participation := p.PublicEvents[publiquevent.PublicEventIDTideGauge]
		if routeID == tideGaugeShortRoute {
			return participation.Action == publiquevent.ActionHelp && participation.JoinedStage == publiquevent.TideGaugeBeforeRise
		}
		return participation.Action == publiquevent.ActionInvestigate && participation.JoinedStage == publiquevent.TideGaugeAfterRise
	}
	if !strings.HasSuffix(routeID, "-water") {
		return true
	}
	// A06's compromise or escort plan opens small-boat cooperation only after
	// the boatmen have had time to implement it. Supporting the old monopoly
	// keeps the ordinary land route, so both choices remain playable.
	for _, choiceID := range []string{"lanchuan-a06.c1", "lanchuan-a06.c3"} {
		if questEchoGateOpen(p, &quest.EchoGate{TaskID: "lanchuan-a06", ChoiceID: choiceID, Outcome: "success"}) {
			return true
		}
	}
	return false
}
