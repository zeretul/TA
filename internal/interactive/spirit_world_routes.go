package interactive

import (
	"fmt"
	"strings"
	"ta/internal/cultivation"
	"ta/internal/exploration"
)

const spiritAscentRoute = "spirit-world.ascent"
const spiritCrossingMethod = "prepared_spatial_node_crossing"
const spiritWorldCatalogMarker = "spirit_qiao_yan"

// Only the revised frozen world has this adaptation. Installing new code cannot
// open a new continent in an older Story with the same numeric content version.
func spiritWorldEnabled(catalog *PackCatalogSnapshot) bool {
	if catalog == nil {
		return false
	}
	for _, p := range catalog.Characters {
		if p.SourceKey == spiritWorldCatalogMarker && p.PlaceCandidate == "spirit.border.camp" {
			return true
		}
	}
	return false
}

var spiritPlaceEnglishLabels = map[string]string{
	"yue.boundary.node": "Mountain boundary in Yue", "spirit.border.camp": "Tianyuan boundary camp", "spirit.sunset.market": "Sunset City workshops", "spirit.tianyuan.market": "Deep Heaven City southern market", "spirit.quiet.valley": "Qixia Quiet Valley",
}

func spiritPlaceNameKey(id string) string {
	if _, ok := spiritPlaceEnglishLabels[id]; ok {
		return "game.spirit.place." + id
	}
	return ""
}
func init() {
	for id, label := range map[string]string{
		"yue.boundary.node": "越国远山界隙", "spirit.border.camp": "灵界天元境界隙营地", "spirit.sunset.market": "灵界落日城百工坊", "spirit.tianyuan.market": "灵界天渊城南坊", "spirit.quiet.valley": "灵界天元境栖霞静谷",
	} {
		journeyPlaceLabels[id] = label
		journeyPlaceAliases[id] = []string{label, spiritPlaceEnglishLabels[id]}
	}
	for _, r := range []struct {
		id, from, to string
		minutes      int64
	}{
		{"spirit-world.human-road", "yue.market", "yue.boundary.node", 21600},
		{"spirit-world.sunset-road", "spirit.border.camp", "spirit.sunset.market", 14400},
		{"spirit-world.tianyuan-road", "spirit.sunset.market", "spirit.tianyuan.market", 43200},
		{"spirit-world.quiet-road", "spirit.tianyuan.market", "spirit.quiet.valley", 64800},
	} {
		for _, reverse := range []bool{false, true} {
			from, to, id := r.from, r.to, r.id+":out"
			if reverse {
				from, to, id = r.to, r.from, r.id+":back"
			}
			registerSpiritRoute(id, from, to, r.minutes, journeyMovementMethod)
		}
	}
	registerSpiritRoute(spiritAscentRoute, "yue.boundary.node", "spirit.border.camp", 43200, spiritCrossingMethod)
}
func registerSpiritRoute(id, from, to string, minutes int64, method string) {
	r := exploration.RouteDefinition{ID: id, Version: "1.0", OriginLocationID: from, DestinationLocationID: to, Segments: []exploration.RouteSegment{{ID: id + ":segment", FromLocationID: from, ToLocationID: to, DurationMinutes: minutes, MovementMethodID: method}}}
	if _, _, d := exploration.ValidateRoute(r, journeySupportedMethods()); len(d) > 0 {
		panic(fmt.Sprintf("invalid spirit route %s: %+v", id, d))
	}
	journeyRouteDefinitions = append(journeyRouteDefinitions, r)
}
func spiritWorldRouteAvailable(p *storyBranchProjection, id string) bool {
	if !strings.HasPrefix(id, "spirit-world.") {
		return true
	}
	if p == nil || !spiritWorldEnabled(p.PackCatalog) {
		return false
	}
	if id != spiritAscentRoute {
		return true
	}
	for _, j := range p.Journeys {
		if j.RouteID == id && !journeyFinished(j) {
			return true
		}
	}
	_, err := spiritCrossingOps(p.State, []string{DefaultActorID})
	return err == nil
}

// Crossing is a declared one-way expedition, never automatic at breakthrough.
// Its prepared anchor and bounded bodily cost settle ONCE at departure. Normal
// continuing journeys retain the paid state, including cold replay and forks.
func spiritCrossingOps(state map[string]any, travelers []string) ([]ActorStateOp, error) {
	var ops []ActorStateOp
	for _, actor := range travelers {
		stage, ok := cultivation.ParseStageKey(xiuxianStoredStageKey(state, actor))
		if !ok || !stageReaches(stage, "spirit_late") {
			return nil, fmt.Errorf("跨界需要每位同行者达到化神后期并各自备妥护界锚")
		}
		base := cultivation.BaseAttributes(stage)
		hp, mp := xiuxianStoredInt(state, actor, xiuxianFieldHealth), xiuxianStoredInt(state, actor, xiuxianFieldMana)
		healthCost, manaCost := base.HealthMax/4, base.ManaMax/2
		if hp <= healthCost || mp < manaCost {
			return nil, fmt.Errorf("跨界前需保留超过四分之一基础生命和至少一半基础灵力；先调息，不会带伤强行出发")
		}
		pack, _ := xiuxianEquipmentEntry(state, actor, xiuxianFieldImportantItems)
		next := make(map[string]any, len(pack))
		consumed := false
		for _, key := range sortedKeys(pack) {
			raw := pack[key]
			entry, _ := raw.(map[string]any)
			if !consumed && xiuxianEntryText(entry, "定义") == "material.spirit.boundary_anchor" && xiuxianCarriedQuantity(entry) > 0 {
				copy := map[string]any{}
				for k, v := range entry {
					copy[k] = v
				}
				copy["数量"] = float64(xiuxianCarriedQuantity(entry) - 1)
				next[key] = copy
				consumed = true
			} else {
				next[key] = raw
			}
		}
		if !consumed {
			return nil, fmt.Errorf("尚未完成界隙勘验与护界锚准备，不能把远行描述当成已能跨界")
		}
		for _, f := range []struct {
			name  string
			value any
		}{{xiuxianFieldImportantItems, next}, {xiuxianFieldHealth, float64(hp - healthCost)}, {xiuxianFieldMana, float64(mp - manaCost)}} {
			ops = append(ops, ActorStateOp{Op: "set", ActorID: actor, FieldID: normalizeActorStateFieldName(f.name), Value: f.value, Reason: "单向跨界消耗护界锚及抵御界面压力的生命、灵力，修为与已学能力不被清除", SourceKind: StateOpSourceJourney, SourceID: spiritAscentRoute})
		}
	}
	return ops, nil
}

// These four expeditions and their high-tier sources do not appear in a Story
// frozen before the region existed. Retreat is never gated after entry.
func spiritRealmEligible(meta StoryMeta, def exploration.DungeonDefinition, state map[string]any) bool {
	if content := worldContentForMeta(meta); content != nil {
		minimum := content.RealmMinimumStages[def.ID]
		if minimum == "" {
			return true
		}
		stage, ok := cultivation.ParseStageKey(xiuxianStoredStageKey(state, DefaultActorID), cultivationRulesForMeta(meta))
		return ok && stageReaches(stage, minimum)
	}
	minimum := ""
	switch def.ID {
	case exploration.RealmSpiritBoundary:
		minimum = "spirit_late"
	case exploration.RealmSpiritRift:
		minimum = "spirit_late"
	case exploration.RealmSpiritArray:
		minimum = "void_late"
	case exploration.RealmSpiritArchive:
		minimum = "integration_late"
	default:
		return true
	}
	if !spiritWorldEnabled(meta.PackCatalog) {
		return false
	}
	stage, ok := cultivation.ParseStageKey(xiuxianStoredStageKey(state, DefaultActorID), cultivationRulesForMeta(meta))
	return ok && stageReaches(stage, minimum)
}
