package interactiveapp

import (
	"encoding/json"
	"fmt"
	"strings"

	"ta/internal/interactive"
)

func writeWorldFactionAffairs(sb *strings.Builder, view interactive.FactionAffairsView, maxBytes int) {
	if len(view.Projects) == 0 && len(view.Recipes) == 0 {
		return
	}
	intro := "### 可选组织共同事务（action=faction）\n来源：本分支人物记录与冻结制作规则。至多显示48条事务摘要（未完最多32条）和96个人物配方组合；超额只省略本页展示，完整历史保留在人物记录中。\n玩家可独行，也可把与熟识者的制作事务作为经营的一小部分；主故事自然商议，不派每日任务、不定时收菜，不授予组织身份、公库权限或NPC控制权。下面是已在场本人真实本领可做的配方，不代表本人已同意；不得借目录泄露其未说出的经历。\nstart填写新project_id、group_name、partner_id、recipe_id，只建立本件约定，不花时间或发产物。work只填写project_id、玩家本轮明确允许的hours及source_exchange_index；start与work均须source_exchange_index引用本轮本人明确接受；每次工作须本人当场向主角明确接受，在character_facts.exchanges对应回应填写participation_decision={kind:faction,target_id:project_id,decision:accept}。引用对此事最后一条本人回应；拒绝或不在场就保留故事而不用work，不可替本人答应。source_text逐字引用玩家本轮授权。\n主角提供自己的全部材料，首次真实工作时一次扣除；主角每小时耗1精力，成员按下列每小时消耗，正式世界钟同步推进。可以分段、休养后再议，等待/离开/读页面不自动生产。最后工时完成时原配方产物自动进入主角实际库存，不用另领，不传授技能或刷好感。cancel只写project_id停止，已经消耗的材料和时间不退，未开工可无损退出。组织资产仍须另有来源，不能把别人的私物当公库。\n"
	if sb.Len()+len(intro) > maxBytes {
		return
	}
	sb.WriteString(intro)
	for _, p := range view.Projects {
		entry := fmt.Sprintf("- 既有 project_id `%s`：%s，与%s(%s)，配方 `%s`，%d/%d小时，状态=%s，在场可复议=%t。\n", p.ProjectID, p.GroupName, p.PartnerName, p.PartnerID, p.RecipeID, p.WorkedHours, p.RequiredHours, p.Phase, p.CanWork)
		if sb.Len()+len(entry) > maxBytes {
			break
		}
		sb.WriteString(entry)
	}
	for _, r := range view.Recipes {
		materials, _ := json.Marshal(r.Materials)
		entry := fmt.Sprintf("- 本人在场 partner_id `%s`（%s），recipe_id `%s`（%s），共%d小时，本人每小时耗%d精力，玩家材料=%s，完成交付 `%s` ×%d。\n", r.PartnerID, r.PartnerName, r.RecipeID, r.RecipeName, r.RequiredHours, r.PartnerEnergyPerHour, string(materials), r.OutputID, r.OutputCount)
		if sb.Len()+len(entry) > maxBytes {
			break
		}
		sb.WriteString(entry)
	}
	sb.WriteString("\n")
}
