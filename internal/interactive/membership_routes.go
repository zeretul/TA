package interactive

import (
	"fmt"
	"ta/internal/character"
	"ta/internal/exploration"
)

// These four connections are new adaptation content, not a change to route_036
// or a claim of a canonical novel distance. Ordinary travel includes normal
// rests: two days between the public entrance and market, thirty minutes from
// the gate to the internal meeting place. Every minute uses the journey ledger.
const (
	huangfengExitRoute         = "membership.huangfeng_gate:out"
	huangfengEntryRoute        = "membership.huangfeng_gate:in"
	huangfengMarketRoute       = "membership.huangfeng_market:out"
	huangfengMarketReturnRoute = "membership.huangfeng_market:in"
)

func init() {
	for _, item := range []struct {
		id, from, to string
		minutes      int64
	}{
		{huangfengExitRoute, "yue.huangfeng.sect", "yue.huangfeng.approach", 30},
		{huangfengEntryRoute, "yue.huangfeng.approach", "yue.huangfeng.sect", 30},
		{huangfengMarketRoute, "yue.huangfeng.approach", "yue.market", 2880},
		{huangfengMarketReturnRoute, "yue.market", "yue.huangfeng.approach", 2880},
	} {
		route := exploration.RouteDefinition{ID: item.id, Version: "1.0", OriginLocationID: item.from, DestinationLocationID: item.to, Segments: []exploration.RouteSegment{{ID: item.id + ":segment", FromLocationID: item.from, ToLocationID: item.to, DurationMinutes: item.minutes, MovementMethodID: journeyMovementMethod}}}
		if _, _, diagnostics := exploration.ValidateRoute(route, journeySupportedMethods()); len(diagnostics) > 0 {
			panic(fmt.Sprintf("invalid membership route %s: %+v", item.id, diagnostics))
		}
		journeyRouteDefinitions = append(journeyRouteDefinitions, route)
	}
	journeyPlaceLabels["yue.huangfeng.sect"] = "黄枫谷山门内"
	journeyPlaceLabels["yue.huangfeng.approach"] = "黄枫谷对外入口"
	journeyPlaceAliases["yue.huangfeng.sect"] = []string{"黄枫谷山门内"}
	journeyPlaceAliases["yue.huangfeng.approach"] = []string{"黄枫谷对外入口"}
}

// The outside road is public. Entering the inner gate requires an existing
// formal standing, not the model's claimed title or a pending request.
// Temporary records have no structured expiry yet, so they grant no passage.
// This grants no access to treasuries, command roles or anybody else's property.
func membershipRouteAvailable(p *storyBranchProjection, routeID string) bool {
	switch routeID {
	case huangfengExitRoute, huangfengEntryRoute, huangfengMarketRoute, huangfengMarketReturnRoute:
		// New adaptation routes do not rewrite a story frozen before revision 30.
		if p == nil || p.PackCatalog == nil || p.PackCatalog.ContentVersion < 30 {
			return false
		}
		authored := false
		for _, person := range p.PackCatalog.Characters {
			if person.SourceKey == "leader_zhong_lingdao" && person.FactionID == "yue.huangfeng" {
				authored = true
				break
			}
		}
		if !authored {
			return false
		}
	default:
		return true
	}
	if routeID != huangfengEntryRoute {
		return true
	}
	if p != nil {
		for _, journey := range p.Journeys {
			if journey.RouteID == routeID && !journeyFinished(journey) {
				return true
			}
		}
	}
	if p == nil || p.Characters == nil {
		return false
	}
	for _, m := range character.CurrentFactionMemberships(p.Characters.FactionMemberships) {
		if m.FactionID == "yue.huangfeng" {
			return m.Status == "formal"
		}
	}
	return false
}
