package interactive

import (
	"fmt"
	"strings"
	"ta/internal/exploration"
)

// New optional paths, not edits to the author's existing roads. Durations are
// conservative authored gameplay budgets for avoiding one local detour, not
// claims of measured geography or a change to the protagonist's travel speed.
type realmShortcut struct {
	ID, RealmID, From, To, Name, NameKey string
	Minutes                              int64
}

var realmShortcuts = []realmShortcut{
	{"realm-shortcut-herbcave", exploration.RealmMountainHerbCave, "yue.market", "yue.market.outskirts", "药窟背路", "game.realm.shortcuts.herbCave", 15},
	{"realm-shortcut-tideboat", exploration.RealmStrandedTideBoat, "lanchuan-l02", "lanchuan-l03", "高岸纤道", "game.realm.shortcuts.tideBoat", 660},
	{"realm-shortcut-servicebay", exploration.RealmShaftServiceBay, "xinghai-l01", "xinghai-l05", "维护通道", "game.realm.shortcuts.serviceBay", 5},
}

func init() {
	for _, shortcut := range realmShortcuts {
		for _, direction := range []string{"forward", "reverse"} {
			from, to := shortcut.From, shortcut.To
			if direction == "reverse" {
				from, to = to, from
			}
			id := shortcut.ID + ":" + direction
			route := exploration.RouteDefinition{ID: id, Version: "1.0", OriginLocationID: from, DestinationLocationID: to, Segments: []exploration.RouteSegment{{ID: id + ":segment", FromLocationID: from, ToLocationID: to, DurationMinutes: shortcut.Minutes, MovementMethodID: journeyMovementMethod}}}
			if _, _, diagnostics := exploration.ValidateRoute(route, journeySupportedMethods()); len(diagnostics) > 0 {
				panic(fmt.Sprintf("invalid exploration shortcut %s: %+v", id, diagnostics))
			}
			journeyRouteDefinitions = append(journeyRouteDefinitions, route)
		}
	}
}

func realmShortcutFor(routeID string) (realmShortcut, bool) {
	for _, s := range realmShortcuts {
		if routeID == s.ID+":forward" || routeID == s.ID+":reverse" {
			return s, true
		}
	}
	return realmShortcut{}, false
}

// Knowing a method is not knowing the route. A real return action and walking
// into the side area are both necessary. This branch-local geographic discovery
// persists; taking a finite cache later never replenishes it or grants materials.
func realmShortcutAvailable(p *storyBranchProjection, routeID string) bool {
	shortcut, ok := realmShortcutFor(routeID)
	if !ok {
		return !strings.HasPrefix(routeID, "realm-shortcut-")
	}
	if p == nil {
		return false
	}
	for _, instance := range p.Realms {
		s := instance.Snapshot
		if s.Binding.DefinitionID != shortcut.RealmID || s.ExploreState == nil {
			continue
		}
		returned, walked := false, false
		for _, id := range s.ExploreState.UsedApproachIDs {
			if id == shortcut.RealmID+".return" {
				returned = true
			}
		}
		for _, d := range s.Discoveries {
			if d.Kind == exploration.DiscoveryArea && d.ID == shortcut.RealmID+".side" {
				walked = true
			}
		}
		if returned && walked {
			return true
		}
	}
	return false
}
