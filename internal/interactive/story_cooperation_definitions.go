package interactive

import "ta/internal/exploration"

// CooperationProjectDefinition describes one bounded authored collaboration.
// It never accepts client-provided prices, rewards, participants or abilities.
// The participant's real frozen skill supplies its own energy cost at work time.
type CooperationProjectDefinition struct {
	Name, Description, PlayerRole, PartnerRole             string
	ID, Version, RealmID, PartnerSourceKey, LocationID     string
	NameKey, DescriptionKey, PlayerRoleKey, PartnerRoleKey string
	PartnerSkillID, PartnerEnergyLabelKey                  string
	PlayerItemID                                           string
	PlayerItemQuantity                                     int
	DurationSeconds                                        int64
}

var cooperationProjects = []CooperationProjectDefinition{
	{
		Name: "水迹复核", Description: "与熟识的同伴核对亲查水痕，准备一次分段沿槽换位的走法。",
		PlayerRole: "说明自己实际观察的水痕顺序，校对这一条公开记录，之后自己走现场。", PartnerRole: "把两段观察对应、查漏核记，不替主角闯阵，不使用或赠送术法。",
		ID: "coop.herbcave.notes", Version: "1.0", RealmID: exploration.RealmMountainHerbCave,
		PartnerSourceKey: "npc_zhao_ling", LocationID: "yue.market.outskirts",
		NameKey: "game.cooperation.projects.herbcave.name", DescriptionKey: "game.cooperation.projects.herbcave.description",
		PlayerRoleKey: "game.cooperation.projects.herbcave.playerRole", PartnerRoleKey: "game.cooperation.projects.herbcave.partnerRole",
		DurationSeconds: 600,
	},
	{
		Name: "辨清借力点", Description: "请在场且愿意的船工协助研判，准备一次沿舷慢行的次序。",
		PlayerRole: "提供真实吃水与受力观察，之后自己沿舷行动，不借走对方私物。", PartnerRole: "运用已有舟楫水战知识指出本次借力和退身次序，自付技能内力，不代替主角涉险。",
		ID: "coop.tideboat.current", Version: "1.0", RealmID: exploration.RealmStrandedTideBoat,
		PartnerSourceKey: "lanchuan-p01", LocationID: "lanchuan-l02",
		NameKey: "game.cooperation.projects.tideboat.name", DescriptionKey: "game.cooperation.projects.tideboat.description",
		PlayerRoleKey: "game.cooperation.projects.tideboat.playerRole", PartnerRoleKey: "game.cooperation.projects.tideboat.partnerRole",
		PartnerSkillID: "lanchuan-s34", PartnerEnergyLabelKey: "game.cooperation.energy.inner",
		DurationSeconds: 600,
	},
	{
		Name: "离线标记整备", Description: "在47号长廊共同整理一次离线标记，之后由主角独自回检修舱逐段断电。",
		PlayerRole: "说明亲查隔离标识并自付一份合金丝，之后自己逐段断电、绝缘穿行。", PartnerRole: "按基础机械组装能力整理普通离线标记，自付技能气力；不远程抢修、扫描或授予权限。",
		ID: "coop.servicebay.markers", Version: "1.0", RealmID: exploration.RealmShaftServiceBay,
		PartnerSourceKey: "xinghai-p25", LocationID: "xinghai-l01",
		NameKey: "game.cooperation.projects.servicebay.name", DescriptionKey: "game.cooperation.projects.servicebay.description",
		PlayerRoleKey: "game.cooperation.projects.servicebay.playerRole", PartnerRoleKey: "game.cooperation.projects.servicebay.partnerRole",
		PartnerSkillID: "xinghai-s01", PartnerEnergyLabelKey: "game.cooperation.energy.stamina",
		PlayerItemID: "xinghai-i34", PlayerItemQuantity: 1, DurationSeconds: 600,
	},
}

func cooperationProject(id string, worlds ...*WorldContent) (CooperationProjectDefinition, bool) {
	for _, definition := range worldCooperationProjects(worlds...) {
		if definition.ID == id {
			return definition, true
		}
	}
	return CooperationProjectDefinition{}, false
}

func (d CooperationProjectDefinition) assistedApproachID() string {
	return d.RealmID + ".assisted_slow"
}

func worldCooperationProjects(worlds ...*WorldContent) []CooperationProjectDefinition {
	if len(worlds) > 0 && worlds[0] != nil {
		return worlds[0].CooperationProjects
	}
	return cooperationProjects
}
