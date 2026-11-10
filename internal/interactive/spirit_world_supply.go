package interactive

import (
	"fmt"
	"strings"
)

type WorldSupplyRule struct {
	LocationID string   `json:"location_id"`
	Merchants  []string `json:"merchants"`
}

func spiritSupplyAvailable(state map[string]any, defID, merchant string, worlds ...*WorldContent) error {
	baseID := defID
	records, _ := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	if entry, ok := records[defID].(map[string]any); ok {
		if template := xiuxianEntryText(entry, "模板"); template != "" {
			baseID = template
		}
	}
	if len(worlds) > 0 && worlds[0] != nil {
		c := worlds[0]
		g, ok := c.SupplyRules[baseID]
		if !ok {
			return nil
		}
		if !storyLocationIsAny(state, []string{g.LocationID}, c) {
			return fmt.Errorf("这份物品需在%s询问货源", worldPlaceLabel(c, g.LocationID))
		}
		for _, name := range g.Merchants {
			if strings.TrimSpace(merchant) == name {
				return nil
			}
		}
		return fmt.Errorf("这份物品需要向当地已知货源询价")
	}
	realm := ""
	for _, r := range []string{"void", "integration", "mahayana"} {
		if baseID == "manual.spirit."+r || strings.HasPrefix(baseID, "equipment."+r+"_") {
			realm = r
			break
		}
	}
	if realm == "" {
		return nil
	}
	locations := map[string]string{"void": "spirit.sunset.market", "integration": "spirit.tianyuan.market", "mahayana": "spirit.quiet.valley"}
	names := map[string][]string{"void": {"百工坊书器铺", "Sunset Book and Gear"}, "integration": {"南坊修器铺", "Southern Array Workshop"}, "mahayana": {"栖霞旧卷斋", "Qixia Manuscripts"}}
	place, ok := storyLocationID(state)
	if !ok || place != locations[realm] {
		return fmt.Errorf("这份高阶传承或器物的货源在%s，当前不在该处", journeyPlaceLabel(locations[realm]))
	}
	for _, name := range names[realm] {
		if strings.TrimSpace(merchant) == name {
			return nil
		}
	}
	return fmt.Errorf("这份传承或器物须向本地已知货源%s询价，不因目录存在就让任意商人备货", names[realm][0])
}

// The original recovery remains historical evidence, but an in-person review
// still needs the physical sample now. This does not consume the player's copy.
func spiritQuestReviewReady(state map[string]any, taskID string, worlds ...*WorldContent) error {
	id := map[string]string{"quest.spirit_rift_review": "material.spirit.rift_sample", "quest.spirit_array_review": "material.spirit.formation_record", "quest.spirit_archive_review": "material.spirit.ancient_rubbing"}[taskID]
	if len(worlds) > 0 && worlds[0] != nil {
		id = worlds[0].QuestReviewItems[taskID]
	}
	if id != "" && !cultivationManualHeld(state, DefaultActorID, id) {
		return fmt.Errorf("复验需带来当初所得实物；已有勘取记录不能代替如今仍持有样品")
	}
	return nil
}
