package interactiveapp

import (
	"strings"
	"testing"

	"ta/internal/interactive"
)

func TestFactionAffairMainStoryContextPreservesOptionalityAndCosts(t *testing.T) {
	state := interactive.WorldState{FactionAffairs: interactive.FactionAffairsView{Projects: []interactive.FactionAffairView{{ProjectID: "aid", GroupName: "工坊互助", PartnerID: "worker", PartnerName: "药师", RecipeID: "salve", WorkedHours: 1, RequiredHours: 4, Phase: "active"}}, Recipes: []interactive.FactionRecipeView{{PartnerID: "worker", PartnerName: "药师", RecipeID: "salve", RecipeName: "金疮药", RequiredHours: 4, PartnerEnergyPerHour: 1, Materials: map[string]int{"herb": 3}, OutputID: "medicine", OutputCount: 2}}}}
	text := renderWorldActions(state, 20000)
	for _, want := range []string{"action=faction", "project_id `aid`", "partner_id `worker`", "recipe_id `salve`", "1/4小时", "每次工作须本人", "不派每日任务", "等待/离开/读页面不自动生产", "材料=", "`medicine` ×2", "source_text", "cancel", "不传授技能", "公库权限"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	var empty strings.Builder
	writeWorldFactionAffairs(&empty, interactive.FactionAffairsView{}, 20000)
	if empty.Len() != 0 {
		t.Fatal("empty knowledge invented an affair")
	}
	var tiny strings.Builder
	writeWorldFactionAffairs(&tiny, state.FactionAffairs, 30)
	if tiny.Len() != 0 {
		t.Fatal("context budget split an affair header")
	}
}
