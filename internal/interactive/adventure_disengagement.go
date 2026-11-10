package interactive

import (
	"fmt"
	"strings"
)

// Movement is a bounded encounter capability, not metres per second or a
// promise that a cultivation/progression label outruns every pursuer.
func adventureMovementBase(r AdventureRules, a *AdventureActor, enemy bool) int {
	b := a.Battle
	if b == nil {
		return 0
	}
	rank, injury, controlled, softened := a.Rank, a.Injury, a.Control, a.SoftLimbs
	scale, inventory, equipment := AdventurePlayerBattleScale(b), a.Inventory, a.Equipment
	if enemy {
		rank, injury, controlled, softened = b.EnemyRank, b.EnemyInjury, b.EnemyControl, b.EnemySoftLimbs
		scale, inventory, equipment = b.Scale, b.EnemyInventory, b.EnemyEquipment
	}
	if controlled > 0 || softened {
		return 0
	}
	power := 10 + rank/2
	if scale != "person" {
		power = 20
	}
	// Real equipped mobility is useful; a catalogue entry or an empty slot is not.
	for _, item := range r.Items {
		if item.Scale != scale || inventory[item.ID] < 1 {
			continue
		}
		if equipment[scale+":"+item.Kind] == item.ID || equipment[scale+":asset"] == item.ID {
			power += item.Mobility
		}
	}
	if enemy && b.Scale == "ship" && b.ShipCombat != nil {
		// Ship rank is not a personal tier. Its authored interception capability
		// replaces the unrelated anonymous-rank placeholder.
		power = max(power, b.ShipCombat.Interdiction)
	}
	if !enemy {
		if profile, ok := AdventurePersonalSpaceCombat(r, *a); ok {
			power = max(power, profile.Mobility)
		}
	}
	return max(0, min(100, power-injury/5))
}

// A movement technique is actively declared and paid. It grants only movement
// for this action, not its attack/ward payload on top of a free escape.
func adventureMovementPower(r AdventureRules, a *AdventureActor, enemy bool, skillID string, pay bool) (int, error) {
	base := adventureMovementBase(r, a, enemy)
	if skillID == "" {
		return base, nil
	}
	b := a.Battle
	if b == nil || base == 0 {
		return 0, fmt.Errorf("movement is prevented by the current encounter condition")
	}
	var skill AdventureSkill
	for _, candidate := range r.Skills {
		if candidate.ID == skillID {
			skill = candidate
			break
		}
	}
	rank, energy, health, maximum, skills := a.Rank, a.Energy, a.Health, a.MaxHealth, a.Skills
	scale, inventory, equipment := AdventurePlayerBattleScale(b), a.Inventory, a.Equipment
	blocked := adventureRushUnavailable(*a)
	if enemy {
		rank, energy, health, maximum, skills = b.EnemyRank, b.EnemyEnergy, b.Health, b.MaxHealth, b.EnemySkillRanks
		scale, inventory, equipment = b.Scale, b.EnemyInventory, b.EnemyEquipment
		blocked = b.EnemyRushBlocked > 0
	}
	mastery := skills[skill.ID]
	if enemy && b.EnemyActorID == "" && mastery == 0 {
		for _, opponent := range r.Opponents {
			if opponent.ID == b.OpponentID {
				for _, id := range opponent.SkillIDs {
					if id == skill.ID {
						mastery = 1
					}
				}
			}
		}
	}
	if skill.ID == "" || skill.Mobility <= 0 || mastery < 1 || rank < skill.RequiredRank || skill.Scale != scale || energy < skill.Cost || skill.OncePerTarget {
		return 0, fmt.Errorf("movement technique is unlearned, wrong scale, unqualified or unaffordable")
	}
	if skill.Rush && blocked {
		return 0, fmt.Errorf("rush movement is blocked by sealing or the current fog")
	}
	if !adventureSkillWeaponAvailable(r, skill, inventory, equipment) {
		return 0, fmt.Errorf("movement technique requires its actual equipped weapon")
	}
	if skill.RequiredTool != "" && inventory[skill.RequiredTool] < 1 {
		return 0, fmt.Errorf("skill tool is not owned")
	}
	for id, n := range skill.Materials {
		if inventory[id] < n {
			return 0, fmt.Errorf("skill materials are insufficient")
		}
		for _, equipped := range equipment {
			if equipped == id {
				return 0, fmt.Errorf("equipped item cannot be consumed as skill material")
			}
		}
	}
	existingInjury := a.Injury
	if enemy {
		existingInjury = b.EnemyInjury
	}
	harm, injury := adventureBacklashCost(AdventureActor{MaxHealth: maximum, Skills: skills}, skill.Backlash)
	if harm >= health {
		return 0, fmt.Errorf("full-force self-injury exceeds surviving health")
	}
	if pay {
		for id, n := range skill.Materials {
			inventory[id] -= n
		}
		if enemy {
			b.EnemyEnergy -= skill.Cost
			b.Health -= harm
			b.EnemyInjury = min(100, b.EnemyInjury+injury)
			b.EnemyLastSkillID = skill.ID
		} else {
			a.Energy -= skill.Cost
			a.Health -= harm
			a.Injury = min(100, a.Injury+injury)
		}
	}
	// Space traversal and a separately selected movement use are alternatives,
	// not two copies of the same speed bonus.
	return max(base, min(100, 10+rank/2+skill.Mobility*mastery-(existingInjury+injury)/5)), nil
}

func adventureRetreatProgress(previous, movement, interception int) int {
	if movement <= 0 {
		return 0
	}
	if interception <= 0 {
		return 100
	}
	// A faster dedicated pursuer can erase the escapee's preparation. Repeated
	// clicks by a slower actor cannot eventually draw a guaranteed free escape.
	return max(0, min(100, previous+(movement-interception)*3))
}

func adventureEnemyManeuverChoice(r AdventureRules, a *AdventureActor, choice string) (kind, skillID string, legal bool) {
	kind, skillID, _ = strings.Cut(choice, ":")
	switch kind {
	case "ceasefire":
		return kind, "", skillID == ""
	case "accept_ceasefire":
		return kind, "", skillID == "" && a.Battle.PlayerCeasefireOffer
	case "escape", "pursue":
		_, err := adventureMovementPower(r, a, true, skillID, false)
		return kind, skillID, err == nil && adventureMovementBase(r, a, true) > 0
	}
	return "", "", false
}

func adventureDisengagementCandidates(r AdventureRules, a *AdventureActor) []EnemyDecisionCandidate {
	candidates := []EnemyDecisionCandidate{{ID: "ceasefire", Description: "提出停战，花费本轮行动；对方未同意前仍可能攻击，不算任何一方获胜。"}}
	if a.Battle.PlayerCeasefireOffer {
		candidates = append(candidates, EnemyDecisionCandidate{ID: "accept_ceasefire", Description: "接受对方已经提出的停战请求；保留伤势和已付消耗，无战利奖励。"})
	}
	for _, kind := range []string{"escape", "pursue"} {
		label := "持续准备撤离，是否脱离取决于双方实际机动与拦截；本轮不攻击。"
		if kind == "pursue" {
			label = "主动截断退路，花费整轮追击，不附送普通攻击。"
		}
		if _, _, ok := adventureEnemyManeuverChoice(r, a, kind); ok {
			candidates = append(candidates, EnemyDecisionCandidate{ID: kind, Description: label})
		}
		for _, skill := range r.Skills {
			if skill.Mobility <= 0 {
				continue
			}
			key := kind + ":" + skill.ID
			if _, _, ok := adventureEnemyManeuverChoice(r, a, key); ok {
				candidates = append(candidates, EnemyDecisionCandidate{ID: key, Description: fmt.Sprintf("%s以本人已学%s行动，另付气力%d及真实材料，仅发挥本次机动，不附送招式攻击或护盾。", label, skill.Name, skill.Cost)})
			}
		}
	}
	return candidates
}

func adventureEnemyNonStrike(r AdventureRules, a *AdventureActor, choice string) (bool, int, string, error) {
	b := a.Battle
	if b.Status != "active" || b.EnemyControl > 0 {
		return false, 0, "", nil
	}
	if choice == "" && b.NoProgressRounds >= 3 {
		choice = "ceasefire"
		if b.PlayerCeasefireOffer {
			choice = "accept_ceasefire"
		}
	}
	kind, skill, ok := adventureEnemyManeuverChoice(r, a, choice)
	if !ok {
		return false, 0, "", nil
	}
	movement := 0
	switch kind {
	case "ceasefire":
		b.EnemyCeasefireOffer = true
	case "accept_ceasefire":
		b.EnemyCeasefireOffer = true
		b.Status = "standoff"
	case "escape", "pursue":
		var err error
		movement, err = adventureMovementPower(r, a, true, skill, true)
		if err != nil {
			return false, 0, "", err
		}
		b.EnemyCeasefireOffer = false
	}
	b.EnemyLastAction = kind
	return true, movement, kind, nil
}

func settleAdventureDisengagement(r AdventureRules, a *AdventureActor, playerAction, enemyAction string, playerMovement, enemyMovement int) {
	b := a.Battle
	if b.Status != "active" || a.Health <= 0 || b.Health <= 0 {
		return
	}
	if b.PlayerCeasefireOffer && b.EnemyCeasefireOffer {
		b.Status = "standoff"
		return
	}
	if playerAction == "escape" && enemyAction == "escape" {
		b.Status = "standoff"
		return
	}
	if playerAction == "escape" && a.Control == 0 {
		intercept := adventureMovementBase(r, a, true) / 2
		if enemyAction == "pursue" {
			intercept = min(100, enemyMovement+10)
		}
		if enemyAction == "ceasefire" || enemyAction == "controlled" || enemyAction == "softened" || b.EnemySoftLimbs || b.EnemyControl > 0 {
			intercept = 0
		}
		if b.Scale == "ship" && b.ShipCombat != nil && enemyAction != "ceasefire" && enemyAction != "controlled" {
			intercept = max(intercept, b.ShipCombat.Interdiction)
		}
		b.PlayerRetreatProgress = adventureRetreatProgress(b.PlayerRetreatProgress, playerMovement, intercept)
		if b.PlayerRetreatProgress >= 100 {
			b.Status = "escaped"
		}
	} else {
		b.PlayerRetreatProgress = 0
	}
	if enemyAction == "escape" && b.EnemyControl == 0 {
		intercept := adventureMovementBase(r, a, false) / 2
		if playerAction == "pursue" {
			intercept = min(100, playerMovement+10)
		}
		if playerAction == "ceasefire" || playerAction == "controlled" || a.SoftLimbs || a.Control > 0 {
			intercept = 0
		}
		b.EnemyRetreatProgress = adventureRetreatProgress(b.EnemyRetreatProgress, enemyMovement, intercept)
		if b.EnemyRetreatProgress >= 100 {
			b.Status = "enemy_escaped"
		}
	} else {
		b.EnemyRetreatProgress = 0
	}
}
