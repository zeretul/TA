package interactive

import (
	"fmt"
	"strings"

	"ta/internal/cultivation"
	"ta/internal/exploration"
)

func realmExploreAction(action string) bool {
	switch strings.TrimSpace(action) {
	case RealmActionSurvey, RealmActionPrepare, RealmActionApproach, RealmActionResolve:
		return true
	}
	return false
}

func realmPlanDuration(plan realmPlannedAction) int64 {
	if plan.ClockBefore == nil || plan.ClockAfter == nil {
		return 0
	}
	return plan.ClockAfter.Seconds - plan.ClockBefore.Seconds
}

// Eligibility is always read from the committed actor. Proposed narration cannot
// mint the tool or learn the skill that authorizes its own successful approach.
func planRealmExplore(p *storyBranchProjection, scene map[string]any, meta StoryMeta, scope exploration.DungeonScope, prior RealmInstanceState, def exploration.DungeonDefinition, req RealmRequest) (realmPlannedAction, error) {
	if p == nil || def.ExploreRules == nil {
		return realmPlannedAction{}, fmt.Errorf("此处没有已登记的探索方法")
	}
	method, ok := exploration.FindExploreApproach(def, strings.TrimSpace(req.ApproachID))
	if !ok || string(method.Kind) != strings.TrimSpace(req.Action) {
		return realmPlannedAction{}, fmt.Errorf("探索方法与行动种类不符")
	}
	if method.CooperationProjectID != "" && !cooperationCapability(p, method.CooperationProjectID, prior.Snapshot) {
		return realmPlannedAction{}, fmt.Errorf("尚没有这次共同工作所得的可用方案")
	}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, StoryDirectorActorStateSystem{})
	if _, err := realmExploreActorOps(scene, system, def, method.Costs, nil, method.ID); err != nil {
		return realmPlannedAction{}, err
	}
	clock := NewStoryClock(storySceneText(scene, storySceneTimeField))
	if p.Clock != nil {
		clock = *p.Clock
	}
	after, err := AdvanceStoryClock(clock, method.DurationSeconds)
	if err != nil {
		return realmPlannedAction{}, err
	}
	if boundary := lifespanBoundary(meta, scene, DefaultActorID); boundary != nil && after.Seconds > boundary.At {
		return realmPlannedAction{}, fmt.Errorf("剩余寿元不足以完成这段探索，可先退出")
	}
	if adventureOpeningBoundary(scene, clock.Seconds, after.Seconds, p.worldContent()) < after.Seconds {
		return realmPlannedAction{}, fmt.Errorf("正在逼近的危险会打断这段探索；先脱离危险")
	}
	ctx, err := realmActionContext(prior.Snapshot, scope, exploration.ActionExplore, method.ID, "", "", "realm."+method.ID)
	if err != nil {
		return realmPlannedAction{}, err
	}
	transition, err := exploration.ApplyExploreApproach(def, scope, prior.Snapshot, ctx, exploration.ExploreApproachInput{ApproachID: method.ID, StoryTimeSeconds: clock.Seconds, Capabilities: []string{method.ID}})
	if err != nil {
		return realmPlannedAction{}, err
	}
	// Costs and all rewards share one planned actor/map. Never add rewards from
	// the old inventory after spending a cost: that would silently refund it.
	ops, err := realmExploreActorOps(p.State, system, def, transition.Costs, transition.Rewards, method.ID)
	if err != nil {
		return realmPlannedAction{}, err
	}
	return realmPlannedAction{Definition: def, Action: req.Action, Instance: transition.Snapshot, OriginLocation: prior.OriginLocation, Ops: ops, Rewards: transition.Rewards, ClockBefore: &clock, ClockAfter: &after}, nil
}

func realmExploreActorOps(state map[string]any, system StoryDirectorActorStateSystem, def exploration.DungeonDefinition, costs []exploration.ExploreResourceCost, rewards []exploration.InstanceReward, source string) ([]ActorStateOp, error) {
	if system.AdventureRules != nil {
		return realmAdventureExploreOps(state, system, costs, rewards, source)
	}
	catalog := storyItemCatalog(system.CultivationRules,
		state)
	pack, _ := xiuxianEquipmentEntry(state, DefaultActorID, xiuxianFieldImportantItems)
	next := make(map[string]any, len(pack)+len(rewards))
	for key, v := range pack {
		next[key] = v
	}
	changed := false
	for _, cost := range costs {
		if cost.Quantity < 1 || cost.Quantity > 1000000 || cost.Kind == exploration.ExploreUseSkill {
			return nil, fmt.Errorf("探索成本未登记")
		}
		if _, ok := catalog.Definition(cost.RefID); !ok {
			return nil, fmt.Errorf("探索所需物品未登记：%s", cost.RefID)
		}
		remaining := int(cost.Quantity)
		keys := sortedKeys(next)
		for _, key := range keys {
			entry, _ := next[key].(map[string]any)
			id := xiuxianEntryText(entry, "定义")
			item, ok := catalog.Definition(id)
			if !ok || !realmCultivationItemMatches(system.CultivationRules,
				state, id, cost.RefID) {
				continue
			}
			available := max(0, xiuxianCarriedQuantity(entry))
			if available == 0 {
				continue
			}
			if item.Talisman != nil {
				stage, known := cultivation.ParseStageKey(xiuxianStoredStageKey(state, DefaultActorID), system.CultivationRules)
				if !known || cultivation.TalismanUseReason(cultivation.Combatant{Cultivation: stage}, *item.Talisman, id) != "" {
					continue
				}
			}
			take := min(remaining, available)
			if cost.Kind == exploration.ExploreConsumeItem {
				copy := make(map[string]any, len(entry))
				for k, v := range entry {
					copy[k] = v
				}
				copy["数量"] = float64(available - take)
				next[key] = copy
				changed = true
			} else if cost.Kind != exploration.ExploreRequireItem {
				return nil, fmt.Errorf("探索成本种类未登记")
			}
			remaining -= take
			if remaining == 0 {
				break
			}
		}
		if remaining > 0 {
			return nil, fmt.Errorf("缺少实际可用物品或尚未达到使用境界：%s × %d", cost.RefID, cost.Quantity)
		}
	}
	for _, reward := range rewards {
		item, ok := catalog.Definition(reward.RefID)
		if !ok || reward.Quantity < 1 || reward.Quantity > 1000000 {
			return nil, fmt.Errorf("探索所得未登记或数量超过上限")
		}
		if item.Kind == cultivation.KindImplement || item.Kind == cultivation.KindArmor {
			// Equipment is a physical copy, not a material stack. Keep an older
			// copy (including its wear/source) intact instead of replacing it.
			if reward.Quantity != 1 {
				return nil, fmt.Errorf("探索装备所得须逐件登记")
			}
			name := item.ID
			entry := xiuxianItemPackEntryWithFlavor(item, name, 1, "", realmRewardProvenance(def, reward))
			reserved := make(map[string]any, len(next)+len(xiuxianSlotOrder))
			for k, v := range next {
				reserved[k] = v
			}
			for _, slot := range xiuxianSlotOrder {
				if worn, ok := xiuxianEquipmentEntry(state, DefaultActorID, slot); ok {
					if n := xiuxianItemName(worn); n != "" {
						reserved[n] = worn
					}
				}
			}
			if _, held := reserved[name]; held {
				name = xiuxianDistinctPackKey(reserved, name, entry)
				entry = xiuxianItemPackEntryWithFlavor(item, name, 1, "", realmRewardProvenance(def, reward))
			}
			next[name] = entry
			changed = true
			continue
		}
		if item.Kind != cultivation.KindMaterial {
			return nil, fmt.Errorf("探索所得不是已支持的材料或装备")
		}
		prior, _ := next[item.ID].(map[string]any)
		n := int(reward.Quantity)
		if prior != nil {
			n += max(0, xiuxianCarriedQuantity(prior))
		}
		if n > 1000000 {
			return nil, fmt.Errorf("探索材料数量超过上限")
		}
		next[item.ID] = xiuxianItemPackEntryWithFlavor(item, item.ID, n, xiuxianEntryText(prior, "描述"), realmRewardProvenance(def, reward))
		changed = true
	}
	if !changed {
		return nil, nil
	}
	return []ActorStateOp{{Op: "set", ActorID: DefaultActorID, FieldID: normalizeActorStateFieldName(xiuxianFieldImportantItems), Value: next, Reason: "探索方法实际消耗与所得一并结算", SourceKind: StateOpSourceRealm, SourceID: source}}, nil
}

func realmCultivationItemMatches(cultivationRules *cultivation.Rules, state map[string]any, id, required string) bool {
	if id == required {
		return true
	}
	records, _ := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	record, _ := records[id].(map[string]any)
	item, ok := generatedItemFromRecord(cultivationRules,
		record)
	return ok && item.ID == id && xiuxianEntryText(record, "模板") == required
}

func realmAdventureExploreOps(state map[string]any, system StoryDirectorActorStateSystem, costs []exploration.ExploreResourceCost, rewards []exploration.InstanceReward, source string) ([]ActorStateOp, error) {
	actor, err := AdventureActorFromState(state, DefaultActorID)
	if err != nil {
		return nil, err
	}
	rules, err := adventureRulesForActor(*system.AdventureRules, actor)
	if err != nil {
		return nil, err
	}
	if err := ValidateAdventureActor(rules, actor); err != nil {
		return nil, err
	}
	if actor.Health <= 0 || actor.Battle != nil && actor.Battle.Status == "active" {
		return nil, fmt.Errorf("先结束交手并恢复行动能力")
	}
	deductions := map[string]int{}
	energy := 0
	for _, cost := range costs {
		if cost.Quantity < 1 || cost.Quantity > 1000000 {
			return nil, fmt.Errorf("探索成本数量无效")
		}
		switch cost.Kind {
		case exploration.ExploreRequireItem:
			known := false
			for _, item := range rules.Items {
				if item.ID == cost.RefID {
					known = true
					break
				}
			}
			if !known || AdventureOwnedItemCount(actor, cost.RefID) < int(cost.Quantity) {
				return nil, fmt.Errorf("缺少实际持有工具：%s", cost.RefID)
			}
		case exploration.ExploreConsumeItem:
			deductions[cost.RefID] += int(cost.Quantity)
		case exploration.ExploreUseSkill:
			found := false
			for _, skill := range rules.Skills {
				if (skill.ID == cost.RefID || skill.GeneratedFrom == cost.RefID) && actor.Skills[skill.ID] > 0 && actor.Rank >= skill.RequiredRank {
					if cost.Quantity != 1 || skill.Cost < 0 {
						return nil, fmt.Errorf("探索技能成本无效")
					}
					if skill.RequiredTool != "" && AdventureOwnedItemCount(actor, skill.RequiredTool) < 1 {
						continue
					}
					energy += skill.Cost
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("尚未学会或未满足技能条件：%s", cost.RefID)
			}
		default:
			return nil, fmt.Errorf("探索成本种类无效")
		}
	}
	if actor.Energy < energy {
		return nil, fmt.Errorf("精力不足：需要 %d，现有 %d", energy, actor.Energy)
	}
	items, err := adventureItemCosts(actor, deductions)
	if err != nil {
		return nil, err
	}
	// Validate costs before netting them with rewards. A same-action grant cannot
	// finance the method that is supposed to earn it.
	for _, reward := range rewards {
		if reward.Quantity < 1 || reward.Quantity > 1000000 {
			return nil, fmt.Errorf("探索所得数量无效")
		}
		items[reward.RefID] += int(reward.Quantity)
	}
	ops, err := AdventureRewardOps(state, system, 0, items, 0, source)
	if err != nil {
		return nil, err
	}
	after := intervalProjectionAfterOps(&storyBranchProjection{State: state}, ops)
	actor, err = AdventureActorFromState(after.State, DefaultActorID)
	if err != nil {
		return nil, err
	}
	actor.Energy -= energy
	if err := ValidateAdventureActor(rules, actor); err != nil {
		return nil, err
	}
	if len(costs) == 0 && len(rewards) == 0 {
		return nil, nil
	}
	ops = adventureActorOps(actor, source)
	for i := range ops {
		ops[i].SourceKind = StateOpSourceRealm
		ops[i].Reason = "探索方法实际消耗与所得一并结算"
	}
	return ops, nil
}

func realmExploreView(meta StoryMeta, p *storyBranchProjection, def exploration.DungeonDefinition, scope exploration.DungeonScope, snapshot exploration.DungeonInstanceSnapshot) *exploration.ExploreView {
	if def.ExploreRules == nil || p == nil {
		return nil
	}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, StoryDirectorActorStateSystem{})
	caps := []string{}
	for _, a := range def.ExploreRules.Approaches {
		if a.CooperationProjectID != "" && !cooperationCapability(p, a.CooperationProjectID, snapshot) {
			continue
		}
		if _, err := realmExploreActorOps(p.State, system, def, a.Costs, nil, a.ID); err == nil {
			caps = append(caps, a.ID)
		}
	}
	view, diagnostics := exploration.ProjectExploreView(def, scope, snapshot, exploration.ExploreViewContext{StoryTimeSeconds: p.clockSeconds(), Capabilities: caps})
	if len(diagnostics) > 0 {
		return nil
	}
	blocked := realmActionBlocked(p, p.State, meta) != nil
	for i := range view.Approaches {
		if blocked && view.Approaches[i].Available {
			view.Approaches[i].Available = false
			view.Approaches[i].BlockedReason = "cannot_act"
		}
		for j := range view.Approaches[i].Costs {
			realmExploreCostLabel(&view.Approaches[i].Costs[j], system, p.State)
		}
	}
	return &view
}

func realmExploreCostLabel(cost *exploration.ExploreResourceCost, system StoryDirectorActorStateSystem, state map[string]any) {
	if system.AdventureRules != nil {
		for _, item := range system.AdventureRules.Items {
			if item.ID == cost.RefID {
				cost.Name = item.Name
				return
			}
		}
		for _, skill := range system.AdventureRules.Skills {
			if skill.ID == cost.RefID {
				cost.Name = skill.Name
				cost.EnergyCost = skill.Cost
				return
			}
		}
	} else if item, ok := storyItemCatalog(system.CultivationRules,
		state).Definition(cost.RefID); ok {
		cost.NameKey = item.NameKey
	}
}

func realmConnectionKnown(def exploration.DungeonDefinition, snapshot exploration.DungeonInstanceSnapshot, id string) bool {
	if def.ExploreRules != nil {
		for _, c := range def.Connections {
			if c.ID == id {
				to := c.ToAreaID
				if c.ToAreaID == snapshot.CurrentAreaID {
					to = c.FromAreaID
				}
				return exploration.ExploreConnectionAvailable(def, snapshot, id, to)
			}
		}
		return false
	}
	for _, c := range def.Connections {
		if c.ID == id {
			return c.PlayerVisible
		}
	}
	return false
}

func applyRealmClock(branch *storyBranchProjection, event RealmRecordEvent) error {
	if event.ClockBefore == nil && event.ClockAfter == nil {
		if realmExploreAction(event.Action) {
			return fmt.Errorf("探索方法记录缺少正式耗时")
		}
		return nil
	}
	if !realmExploreAction(event.Action) || event.ClockBefore == nil || event.ClockAfter == nil || event.ClockBefore.Validate() != nil || event.ClockAfter.Validate() != nil {
		return fmt.Errorf("探索时钟记录无效")
	}
	def, ok := worldRealmDefinition(branch.worldContent(), event.RealmID)
	if !ok {
		return ErrRealmUnknownRealm
	}
	method, ok := exploration.FindExploreApproach(def, event.ApproachID)
	if !ok || string(method.Kind) != event.Action || event.ClockAfter.Seconds-event.ClockBefore.Seconds != method.DurationSeconds || event.ClockBefore.Seconds != branch.clockSeconds() {
		return fmt.Errorf("探索时钟与实际方法不一致")
	}
	branch.Clock = cloneStoryClock(event.ClockAfter)
	return nil
}

// Realm has no independent combat or teleport permission. An unfinished fight
// blocks every entrance, movement, reward and retreat through this module.
func realmActionBlocked(p *storyBranchProjection, scene map[string]any, meta StoryMeta) error {
	if p == nil {
		return fmt.Errorf("探索缺少当前分支")
	}
	for _, combat := range p.Combats {
		if !combat.Ended {
			for _, actor := range combat.Combatants {
				if normalizeStatePanelActorID(actor.ID) == DefaultActorID {
					return ErrStoryTurnDuringBattle
				}
			}
		}
	}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, StoryDirectorActorStateSystem{})
	if system.AdventureRules != nil {
		actor, err := AdventureActorFromState(scene, DefaultActorID)
		if err != nil {
			return err
		}
		if actor.Battle != nil && actor.Battle.Status == "active" {
			return ErrStoryTurnDuringBattle
		}
		if actor.Health <= 0 {
			return fmt.Errorf("当前无法行动")
		}
	}
	if system.AdventureRules == nil && actorStateFieldValue(scene, DefaultActorID, xiuxianFieldHealth) != nil && xiuxianStoredInt(scene, DefaultActorID, xiuxianFieldHealth) <= 0 {
		return fmt.Errorf("当前生命已耗尽，不能进行探索行动")
	}
	return StoryTurnBlockedByBattle(scene, system, p.Combats)
}

func realmNameKey(def exploration.DungeonDefinition) string {
	if def.ExploreRules != nil {
		return def.ExploreRules.NameKey
	}
	return ""
}
func realmDescriptionKey(def exploration.DungeonDefinition) string {
	if def.ExploreRules != nil {
		return def.ExploreRules.DescriptionKey
	}
	return ""
}
