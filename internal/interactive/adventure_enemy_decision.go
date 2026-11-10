package interactive

import (
	"fmt"
	"sort"
	"strings"
)

func adventureDecisionWeapon(r AdventureRules, b *AdventureBattle, items map[string]AdventureItem) AdventureItem {
	if b.EnemyActorID != "" || b.EnemyEquipment[b.Scale+":weapon"] != "" {
		return adventureEnemyEquippedItem(b, items, "weapon")
	}
	for _, op := range r.Opponents {
		if op.ID == b.OpponentID {
			return items[op.WeaponID]
		}
	}
	return AdventureItem{}
}

// Legality is separate from tactical utility: a model may prefer a lawful ward
// or recovery over the heuristic's maximum-damage response, but cannot invent
// an unlearned move, equipment ownership, fuel, ammunition or free casting.
func adventureLegalEnemyTechniques(r AdventureRules, a *AdventureActor, weapon AdventureItem) []adventureEnemyTechnique {
	b := a.Battle
	if b == nil {
		return nil
	}
	authored := map[string]bool{}
	for _, op := range r.Opponents {
		if op.ID == b.OpponentID {
			for _, id := range op.SkillIDs {
				authored[id] = true
			}
		}
	}
	var options []adventureEnemyTechnique
	items := map[string]AdventureItem{}
	for _, item := range r.Items {
		items[item.ID] = item
	}
	for _, skill := range r.Skills {
		learned := authored[skill.ID]
		if b.EnemyActorID != "" {
			learned = b.EnemySkillRanks[skill.ID] > 0
		}
		if !learned || skill.Scale != b.Scale || skill.Cost > b.EnemyEnergy || skill.RequiredRank > max(1, b.EnemyRank) || skill.Rush && b.EnemyRushBlocked > 0 || skill.OncePerTarget {
			continue
		}
		effects := append([]AdventureEffect(nil), skill.Effects...)
		if len(effects) == 0 {
			effects = []AdventureEffect{{Kind: skill.Kind, Power: skill.Power}}
		}
		for _, conditional := range skill.ConditionalEffects {
			if b.EnemyInventory[conditional.RequiredItem] > 0 {
				effects = append(effects, conditional.Effects...)
			}
		}
		if skill.RangeMeters > 0 && (b.Scale != "machine" || b.ElectronicDistanceMeters < 1 || b.ElectronicDistanceMeters > skill.RangeMeters) || adventureEffectsSeal(effects) && b.EnemyLastSeal {
			continue
		}
		selfHarm, _ := adventureBacklashCost(AdventureActor{MaxHealth: b.MaxHealth, Skills: b.EnemySkillRanks}, skill.Backlash)
		if !adventureSkillWeaponAvailable(r, skill, b.EnemyInventory, b.EnemyEquipment) {
			continue
		}
		if selfHarm >= b.Health || skill.RequiredTool != "" && b.EnemyInventory[skill.RequiredTool] < 1 {
			continue
		}
		materials := map[string]int{}
		for id, n := range skill.Materials {
			materials[id] += n
		}
		if adventureEnemyUsesWeapon(effects) {
			for id, n := range weapon.Materials {
				materials[id] += n
			}
		}
		available := true
		for id, n := range materials {
			if b.EnemyInventory[id] < n {
				available = false
			}
			for _, equipped := range b.EnemyEquipment {
				if equipped == id {
					available = false
				}
			}
		}
		if !available {
			continue
		}
		if _, supported := adventureEnemyTechniqueScore(b, effects, adventureEnemySkillMultiplier(b, skill.ID, items), 0, 0, skill.Cost, a.ControlRecovery == 0); !supported {
			continue
		}
		options = append(options, adventureEnemyTechnique{Skill: skill, Effects: effects})
	}
	sort.Slice(options, func(i, j int) bool { return options[i].Skill.ID < options[j].Skill.ID })
	return options
}

func adventureEnemyDecisionCandidates(r AdventureRules, a *AdventureActor) []EnemyDecisionCandidate {
	b := a.Battle
	items := map[string]AdventureItem{}
	for _, item := range r.Items {
		items[item.ID] = item
	}
	weapon := adventureDecisionWeapon(r, b, items)
	result := []EnemyDecisionCandidate{{ID: "ordinary", Description: "普通攻击，使用本人实际装备；弹药不足时只能徒手，不凭空补充资源。"}}
	result = append(result, adventureDisengagementCandidates(r, a)...)
	names := map[string]string{"attack": "进攻", "guard": "防护", "heal": "治疗", "energy": "回气", "control": "控制", "poison": "毒", "repair": "修复", "pilot": "操纵防护", "reflect": "借力反击", "drain": "吸取内力", "seal": "封穴", "antidote": "解毒", "mental_guard": "精神防护", "mental_attack": "精神冲击（不叠武器、不附控制或侵蚀）", "recoil": "自身反噬", "electronic_disrupt": "电子干扰"}
	for _, option := range adventureLegalEnemyTechniques(r, a, weapon) {
		skill := option.Skill
		parts := []string{}
		for _, effect := range option.Effects {
			parts = append(parts, fmt.Sprintf("%s%d", names[effect.Kind], effect.Power*adventureEnemySkillMultiplier(b, skill.ID, items)))
		}
		description := fmt.Sprintf("施展本人已学%s，耗气%d，当前火候%d；%s。", skill.Name, skill.Cost, max(1, b.EnemySkillRanks[skill.ID]), strings.Join(parts, "、"))
		if harm, _ := adventureBacklashCost(AdventureActor{MaxHealth: b.MaxHealth, Skills: b.EnemySkillRanks}, skill.Backlash); harm > 0 {
			description += fmt.Sprintf("另付本人生命%d及真实反噬伤势。", harm)
		}
		if len(skill.Materials) > 0 || adventureEnemyUsesWeapon(option.Effects) && len(weapon.Materials) > 0 {
			description += "按本人实际携带的耗材与武器弹药扣除，不能凭空补足。"
		}
		result = append(result, EnemyDecisionCandidate{ID: "skill:" + skill.ID, Description: description})
	}
	return result
}

// The transient choice is a server-side intent, never a persisted future plan.
// Invalid/stale choices simply leave the established safe policy in charge.
func adventureEnemyTechniqueForChoice(r AdventureRules, a *AdventureActor, weapon AdventureItem, choice string) (adventureEnemyTechnique, bool) {
	if choice == "ordinary" {
		return adventureEnemyTechnique{}, true
	}
	for _, option := range adventureLegalEnemyTechniques(r, a, weapon) {
		if choice == "skill:"+option.Skill.ID {
			return option, true
		}
	}
	return adventureEnemyTechnique{}, false
}
