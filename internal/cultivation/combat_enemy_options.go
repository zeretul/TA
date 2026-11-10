package cultivation

import (
	"fmt"
	"sort"
	"strings"
)

// EnemyActionOption binds an opaque model choice to a canonical, already
// affordable intent. The model never supplies a SkillDef, cost or item effect.
// Candidates are recomputed against the committed state before settlement.
type EnemyActionOption struct {
	ID          string
	Description string
	Action      CombatAction
}

func EnemyActionOptions(session CombatSession, actorID string) []EnemyActionOption {
	actor := session.CombatantByID(actorID)
	if actor == nil || !actor.Active() {
		return nil
	}
	start := actor.atExchangeStart()
	result := []EnemyActionOption{{ID: "rest", Description: "调息一轮；若本轮被击伤则无法恢复灵力。", Action: CombatAction{Kind: ActionRest}}}
	if start.RouteExists {
		result = append(result, EnemyActionOption{ID: "escape", Description: "沿已存在的退路尝试脱离，是否成功仍由双方真实速度判定。", Action: CombatAction{Kind: ActionEscape}})
	}
	if start.OfferText == "" {
		result = append(result, EnemyActionOption{ID: "surrender", Description: "提出求饶；对方未接受前不自动停战。", Action: CombatAction{Kind: ActionSurrender, OfferText: "求饶"}})
	}
	if _, ok := session.Preparations[actorID]; ok {
		result = append(result, EnemyActionOption{ID: "continue", Description: "继续已支付的蓄势；仍可能被打断。", Action: CombatAction{Kind: ActionContinuePreparation}}, EnemyActionOption{ID: "cancel", Description: "放弃当前蓄势，不退还已支付消耗。", Action: CombatAction{Kind: ActionCancelPreparation}})
	}
	skills := append([]SkillDef(nil), start.Skills...)
	if strike, ok := NewSkillRegistryWithBuiltins(actor.Cultivation.Rules()).Current("strike"); ok {
		skills = append(skills, strike)
	}
	seen := map[string]bool{}
	for _, skill := range skills {
		if skill.ID == "" || seen[skill.ID] {
			continue
		}
		seen[skill.ID] = true
		plan := CastPlanFor(start, skill, 0)
		if !plan.Check.Allowed {
			continue
		}
		variants := append([]CombatVariant{VariantNone}, VariantsForDefinition(skill)...)
		for _, target := range session.Combatants {
			if skillShields(skill) {
				if target.ID != actorID {
					continue
				}
			} else if !legalTechniqueTarget(&start, &target) {
				continue
			}
			for _, variant := range variants {
				action := CombatAction{Kind: ActionTechnique, Skill: skill, Variant: variant, TargetID: target.ID}
				name := skill.Name
				if name == "" {
					name = skill.ID
				}
				power := skill.Power
				if variant != VariantNone {
					if variantPower, ok := VariantPowerFor(skill, variant); ok {
						power = variantPower
					}
				}
				description := fmt.Sprintf("施展本人已学%s，目标%s；威力系数%.3g，耗灵%d，经脉负担%d，占用%d次行动。", name, target.ID, power, plan.Check.ManaCost, plan.Check.BurdenCost, maxInt(1, skill.ExecutionActions))
				if variant == VariantBurn {
					description += "命中造成实际伤口后附加两轮灼烧。"
				}
				if variant == VariantSlow {
					description += "命中造成实际伤口后附加迟滞。"
				}
				if skill.IgnoreDefense > 0 {
					description += fmt.Sprintf("具有%.0f%%忽防，与装备共用规则上限。", clampFloat(skill.IgnoreDefense, 0, .20)*100)
				}
				if skill.LeavesResidual {
					description += "施展后留下必须安全调养的经脉残伤。"
				}
				if skill.HitOffset != 0 {
					description += fmt.Sprintf("招式自身命中修正%+.0f%%，不保证命中。", skill.HitOffset*100)
				}
				if skillShields(skill) {
					description += fmt.Sprintf("护盾以本人基础生命的%.0f%%计算。", skill.ShieldBaseHealthFraction*100)
				}
				result = append(result, EnemyActionOption{ID: fmt.Sprintf("technique:%s:%s:%s", skill.ID, target.ID, variant), Description: description, Action: action})
				if skill.Targets > 1 && !skillShields(skill) {
					var extra []string
					for _, candidate := range session.Combatants {
						if candidate.ID != target.ID && legalTechniqueTarget(&start, &candidate) {
							extra = append(extra, candidate.ID)
						}
					}
					sort.Strings(extra)
					if len(extra) > skill.Targets-1 {
						extra = extra[:skill.Targets-1]
					}
					if len(extra) > 0 {
						multi := action
						multi.ExtraTargetIDs = extra
						result = append(result, EnemyActionOption{ID: fmt.Sprintf("technique:%s:%s:%s:spread:%s", skill.ID, target.ID, variant, strings.Join(extra, ",")), Description: description + "同时覆盖已列明的其他目标：" + strings.Join(extra, "、") + "。", Action: multi})
					}
				}
			}
		}
	}
	for _, dose := range start.Carried {
		action := CombatAction{Kind: ActionUseItem, ItemInstanceID: dose.InstanceID}
		probe := session.clone()
		copy := probe.CombatantByID(actorID)
		*copy = copy.atExchangeStart()
		if step := resolveUseItem(copy, action); step.ItemApplied {
			result = append(result, EnemyActionOption{ID: "item:" + dose.InstanceID, Description: fmt.Sprintf("使用真实持有的%s，余量%d，用途%s；实际消耗一份。", dose.DefID, dose.Quantity, dose.Pill.Purpose), Action: action})
		}
	}
	for _, spare := range start.Spares {
		action := CombatAction{Kind: ActionSwapEquipment, SwapInstanceID: spare.InstanceID}
		probe := session.clone()
		copy := probe.CombatantByID(actorID)
		*copy = copy.atExchangeStart()
		if step := resolveSwapEquipment(copy, action); step.EquipmentSwapped {
			result = append(result, EnemyActionOption{ID: "swap:" + spare.InstanceID, Description: fmt.Sprintf("花费整轮换入随身备品%s；本轮不能同时驱动。", spare.DefID), Action: action})
		}
	}
	for _, target := range session.Combatants {
		if target.Side != actor.Side && target.Active() && target.OfferText != "" {
			result = append(result, EnemyActionOption{ID: "accept:" + target.ID, Description: "接受对方已提出的求饶：" + target.ID, Action: CombatAction{Kind: ActionAcceptSurrender, TargetID: target.ID}})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// EnemyActionForChoice does not parse model text as a declaration. Only an
// exact match in the newly computed legal list can become a real action.
func EnemyActionForChoice(session CombatSession, actorID, choice string) (CombatAction, bool) {
	for _, option := range EnemyActionOptions(session, actorID) {
		if option.ID == choice {
			return option.Action, true
		}
	}
	return CombatAction{}, false
}

// CombatDeclarationReason performs only the engine's declaration-time checks.
// It does not draw initiative, simulate an opponent or spend any real pool.
// Later damage can still invalidate a valid cast and must remain a spent turn.
func CombatDeclarationReason(session CombatSession, actorID string, action CombatAction) string {
	probe := session.clone()
	for i := range probe.Combatants {
		probe.Combatants[i] = probe.Combatants[i].atExchangeStart()
	}
	actions := map[string]CombatAction{actorID: action}
	refused := illegalTechniqueDeclarations(&probe, actions)
	affordableTechniqueDeclarations(probe, actions, refused)
	freezeOtherInputRefusals(probe, actions, refused)
	return refused[actorID]
}
