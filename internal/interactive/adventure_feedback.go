package interactive

import "strings"

// Domain errors remain diagnostic English; model-facing repair guidance is
// Chinese and never asks the model to invent missing resources or permissions.
func adventureRefusalForModel(err error) string {
	message := strings.TrimPrefix(err.Error(), "adventure action refused: ")
	translations := map[string]string{
		"named opponent is not present in the committed scene":                    "该人物不在当前已确认场景中，不能凭目录召来交战。",
		"named opponent has not been established on this branch":                  "该人物尚未在本分支实际建立，须先通过故事相遇，不能把作者资料当作到场事实。",
		"foundation progression is unavailable or complete":                       "本局未开放深修，或已达成长上限；仍可继续当前故事。",
		"foundation refinement requires a positive interval and stable condition": "深修须有明确授权时长，并先处理重伤、中毒或失常状态。",
		"the current stage requires its own foundation":                           "本阶段须使用成长表指定的基础，不能用旧阶段技能替代。",
		"foundation skill mastery is insufficient":                                "须先取得并研习当前阶段基础，达到要求火候。",
		"foundation refinement supplies are unaffordable":                         "钱财不足以支付一小时深修物资；本次没有推进时间。",
		"this market has no registered work":                                      "这里没有登记的日常劳作来源，可继续询问或前往有招工的市集；本次没有结算报酬。",
		"work requires an authorized positive interval":                           "请按玩家明确授权的劳作时长提交hours。",
		"work skill qualification is missing":                                     "尚未掌握这项工作需要的实际技能，可先学习或选择不要求技艺的短工。",
		"severe injury prevents work":                                             "伤势严重，须先休养，不会预支未完成的工资。",
		"insufficient energy for the work interval":                               "当前精力不足以完成这段工作，可缩短时长或先休养；本次没有扣费或领薪。",
		"work payment exceeds the currency limit":                                 "本次报酬超出正式钱袋容量，未推进工时或改变资源。",
		"mental recovery requires a registered actual corruption source":          "尚无登记遭遇造成的实际精神侵蚀，祈愿石不是普通补血药，本次未耗次数。",
		"the item has no remaining charges":                                       "这件物品的可用次数已耗尽，不能通过重开窗口恢复。",
		"full-force self-injury exceeds surviving health":                         "当前生命不足以承担全力招式的自伤代价，请选择其他行动或先恢复。",
		"recipe requires its owned deployed powered working asset":                "缺少实际持有、已经部署且耐久与燃料充足的指定作业设备。",

		"skill prerequisites or training interval not met":          "技能等级、前置能力或研习时长不满足。",
		"a committed learning source is required":                   "尚未持有该能力需要的秘籍或传授资格凭证，请先按正式任务取得。",
		"the skill has not been learned":                            "尚未学会该能力，不能直接练习提高。",
		"severe injury prevents training":                           "伤势严重，须先休养再训练。",
		"skill has reached its mastery ceiling":                     "该能力已达到本题材的火候上限。",
		"item unavailable or insufficient currency":                 "商品不存在、不可交易或钱财不足。",
		"this market does not sell the item":                        "该市场不出售这种物品，独有奖励必须按其正式来源取得。",
		"this market does not buy the item":                         "该市场不收购这种物品。",
		"adventure market is not at the committed scene location":   "当前正式位置不在所选市场，不能在此买卖。",
		"adventure opponent is not at the committed scene location": "该对手不在当前正式位置，不能隔空开战。",
		"sale requires owned tradable stock":                        "没有足量可出售的实物。",
		"unequip or withdraw the item before selling":               "先卸下装备或撤回机械资产，再出售。",
		"equipment or qualification missing":                        "没有持有该装备，或尚未取得使用资格。",
		"consumable is not owned":                                   "没有持有该消耗品。",
		"recipe, skill qualification or interval missing":           "制作方法、技艺资格或投入时间不满足。",
		"unfinished craft quantity differs":                         "同一制作尚未完成，请沿用原批次的数量继续。",
		"unique recipe output already in progress":                  "该独特物品已有未完成制作，请先继续原制作。",
		"craft materials are insufficient":                          "制作材料不足；本次没有扣料或产出。",
		"equipped items cannot be crafting materials":               "正在装备的物品不能作为制作材料消耗。",
		"operational assets cannot be crafting materials":           "已登记的机械资产不能作为散料消耗。",
		"repair materials are missing":                              "维修耗材不足，不能用叙述假定已经买到。",
		"asset or operator qualification missing":                   "没有持有该机械资产，或缺少操控资格。",
		"a functioning deployed asset is required":                  "该规模的交战需要已部署、可用且有燃料的机械或舰船。",
		"this opponent has already been defeated on this branch":    "本分支已击败该对手，不能重复开战领取战利品。",
		"finish the active battle in its window first":              "交战尚未结束，请先在战斗窗口行动。",
		"the actor is incapacitated":                                "角色已失去战力，普通败北应先休养。",
		"the lethal outcome is terminal":                            "本分支已发生正式致命结果，不能继续行动；可回到之前的分支。",
	}
	if translated, ok := translations[message]; ok {
		return translated
	}
	return "该行动未通过本局冻结规则的资格、资源或输入校验，没有扣除资源。请核对当前正式状态、行动类型和目录ID，不要用正文宣称成功。"
}
