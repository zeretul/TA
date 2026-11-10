package interactiveapp

import (
	"strings"
	"ta/internal/exploration"
	"ta/internal/interactive"
	"testing"
)

func TestWorldRealmExploreShowsOnlyCurrentMethodsAndConsequences(t *testing.T) {
	state := renderTestWorld("越国坊市外围散修人家")
	state.Realms.Available = nil
	state.Realms.Instances = []interactive.RealmStateView{{InstanceID: "instance.known", RealmID: exploration.RealmMountainHerbCave, Presence: "inside", Name: "山溪旧药窟", Explore: &exploration.ExploreView{Pressure: 3, PressureLimit: 4, PressureLabel: "渗水余地", ReturnDueSeconds: 7200, Approaches: []exploration.ExploreApproachView{{ID: "known.outer", Kind: exploration.ExploreResolve, Name: "带走外缘材料", DurationSeconds: 300, PressureDelta: 1, Available: true, CausesSetback: true}, {ID: "known.ward", Kind: exploration.ExploreApproachRoute, Name: "护持通过", DurationSeconds: 600, Available: false, BlockedReason: "capability", Costs: []exploration.ExploreResourceCost{{Kind: exploration.ExploreConsumeItem, RefID: "talisman.ward.lower", NameKey: "item.ward", Quantity: 1}}}}}}}
	page := renderWorldActions(state, interactive.StoryContextMaxBytes)
	for _, want := range []string{"approach_id `known.outer`", "realm_action=resolve", "耗时 300 秒", "警告：这次会触及界线", "approach_id `known.ward`", "暂不可用原因=capability", "consume_item item.ward ×1", "当前 3600 秒", "实际已持有物品/已学技能", "所得自动结算"} {
		if !strings.Contains(page, want) {
			t.Fatalf("missing %q: %s", want, page)
		}
	}
	for _, hidden := range []string{"cache_reward", "realm.mountain_herb_cave.side", "Use realm_action=move with connection_id"} {
		if strings.Contains(page, hidden) {
			t.Fatalf("unexpected hidden or outdated guidance %q", hidden)
		}
	}
	if strings.Index(page, "approach_id `known.outer`") > strings.Index(page, "### Tasks") {
		t.Fatal("active exploration must precede unrelated world lists")
	}
}
