package interactive

import (
	"ta/internal/exploration"
)

// The travel method every authored route in this build was written for. A route
// names the method it is walkable by, and the rules refuse one whose method the
// caller does not support, so the set is declared here rather than assumed.
const journeyMovementMethod = "ordinary_low_tier_travel_with_normal_rests"

// journeyRouteDefinitions is the authored route set this build can actually
// travel.
//
// §4.2 of 自制-探索与行路 requires the elapsed time to come from a concrete
// connection rather than one uniform number per country pair, so these are the
// author's own records from 凡人人界_首版衔接清单_20261015.json, copied
// verbatim: their ids, versions, segment ids, durations and movement method are
// the source data's, and the hash AdvanceTo compares against is computed from
// exactly these bytes. Nothing here is a number this file invented.
//
// The first batch deliberately registers one road. §10 asks for a route that
// can really be walked end to end before the map grows, and a wrong duration on
// a hundred routes is harder to notice than a wrong duration on one.
//
// Both directions are registered because the author record is bidirectional:
// a character who walks to the market and then finds no road home has not
// travelled a road, they have fallen off the map. The two directions are the
// author's own two records, not one record mirrored by this file.
var journeyRouteDefinitions = []exploration.RouteDefinition{
	{
		ID:                    "route_036:forward",
		Version:               "1.0",
		OriginLocationID:      "yue.market",
		DestinationLocationID: "yue.market.outskirts",
		Segments: []exploration.RouteSegment{{
			ID:                 "route_036:forward:segment",
			FromLocationID:     "yue.market",
			ToLocationID:       "yue.market.outskirts",
			DurationMinutes:    30,
			MovementMethodID:   journeyMovementMethod,
			ZeroTimeConnection: false,
		}},
	},
	{
		ID:                    "route_036:reverse",
		Version:               "1.0",
		OriginLocationID:      "yue.market.outskirts",
		DestinationLocationID: "yue.market",
		Segments: []exploration.RouteSegment{{
			ID:                 "route_036:reverse:segment",
			FromLocationID:     "yue.market.outskirts",
			ToLocationID:       "yue.market",
			DurationMinutes:    30,
			MovementMethodID:   journeyMovementMethod,
			ZeroTimeConnection: false,
		}},
	},
}

// journeyPlaceLabels is the human wording a stop is reported under.
//
// A location id is a routing fact, not prose: §3.1 lets the player speak about
// "坊市" without knowing a route id, so the record and the scene write carry the
// place's own name while the route keeps the stable identity. The labels come
// from the same author records the route does — the place's own label where it
// has one, and the character it is authored around where it does not.
var journeyPlaceLabels = map[string]string{
	"yue.market":           "公共坊市",
	"yue.market.outskirts": "越国坊市外围散修人家",
}

// journeyPlaceAliases are the other wordings prose really uses for these
// places.
//
// §5.2 lets the scene's single location field carry the place together with its
// necessary parent scope, so a real scene says "越国公共坊市外围，行旅落脚处最里头
// 一间小屋" where the registry's label is "越国坊市外围散修人家". Matching only the
// label would read that scene as the market and withhold the road that starts
// outside it — which is what happened to the first story played on this build.
//
// These are author wordings, not a parser: each one is how a place is named in
// the story's own prose, and the longest match decides. A bare "坊市" is
// deliberately absent, because it would resolve a scene in another country's
// market to this one. So is a bare "坊市外围": the world book places 赵灵 at
// 越国坊市外围散修家庭 and nowhere else, but 坊市 is a kind of place all three
// countries have, so the bare wording names no single country's outskirts and
// guessing 越国 would put a scene in 元武国 outside a market it is not outside.
//
// 越国坊市 is how the pack's own rows name the market — 越国坊市正街, 越国坊市医馆,
// 越国坊市丹铺, 越国坊市符摊 — while the registry's label says 公共坊市.
// Recognising only the label left every scene inside the market unresolved, so
// the entry offered no road at all from the first turn: the narrator then read
// "no road starts here" and correctly settled nothing, and the character could
// never leave. The aliases are the pack's own place names rather than the bare
// prefix 越国坊市, because that prefix also appears in a scene that merely
// refers to the market from elsewhere ("越国坊市北面半里的山中"), and matching it
// would report the character as standing inside the market while they are in a
// cave half a li north of it.
var journeyPlaceAliases = map[string][]string{
	"yue.market": {
		"公共坊市",
		"越国坊市正街",
		"越国坊市医馆",
		"越国坊市丹铺",
		"越国坊市符摊",
	},
	"yue.market.outskirts": {
		"越国坊市外围散修人家",
		"越国坊市外围散修家庭",
		"越国坊市外围",
		"公共坊市外围",
	},
}

func journeySupportedMethods() []string {
	return []string{journeyMovementMethod, spiritCrossingMethod}
}

// journeyRoute returns one registered route by id.
func journeyRoute(routeID string) (exploration.RouteDefinition, bool) {
	for _, route := range journeyRouteDefinitions {
		if route.ID == routeID {
			return route, true
		}
	}
	return exploration.RouteDefinition{}, false
}

// journeyPlaceLabel reports a place's display name, falling back to its id so a
// registered route without a label still reports something a reader can use
// rather than an empty string.
func journeyPlaceLabel(locationID string) string {
	if label := journeyPlaceLabels[locationID]; label != "" {
		return label
	}
	return locationID
}

// JourneyPlaceLabel is the display name for a place on a registered route.
//
// The application layer reads it so a window can name the endpoints without
// keeping a second copy of the wording that the record writes.
func JourneyPlaceLabel(locationID string) string {
	return journeyPlaceLabel(locationID)
}
