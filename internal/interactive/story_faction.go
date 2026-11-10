package interactive

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"ta/internal/character"
)

const TurnWorldActionFaction = "faction"

// FactionActionInput schedules one shared production affair in the main story.
// It grants no authority over a faction, and never accepts invented costs,
// skills, rewards or NPC decisions from the player's proposed action.
type FactionActionInput struct {
	Action              string `json:"action" jsonschema:"enum=start,enum=work,enum=cancel" jsonschema_description:"start只约定一件制作事务；work按玩家授权工时共同做工，首次做工扣全套材料，全部完成才交付；cancel停止，已用材料与时间不返还。"`
	ProjectID           string `json:"project_id" jsonschema_description:"start使用本局尚未使用的简短稳定ID，之后沿用；不能复用已完成或取消的ID。"`
	GroupName           string `json:"group_name,omitempty" jsonschema_description:"仅start：玩家自愿参与的互助小组或实际组织名称。只是本件事务的称呼，不授予成员、领袖或公库权限。"`
	PartnerID           string `json:"partner_id,omitempty" jsonschema_description:"仅start：已认识、真实在场且已有规则状态的本人Actor ID。"`
	RecipeID            string `json:"recipe_id,omitempty" jsonschema_description:"仅start：当前列出的该成员已掌握的冻结配方。产物交主角，不代替传授。"`
	Hours               int    `json:"hours,omitempty" jsonschema_description:"仅work：玩家本轮明确同意的正整数工时，不超过剩余工时。时间只走正式世界钟，不是现实等待。"`
	SourceExchangeIndex int    `json:"source_exchange_index,omitempty" jsonschema_description:"start/work：本轮本人向主角实际scene回应的1起索引；participation_decision.kind=faction,target_id=project_id,decision=accept。须是对此事务最后回应，每次工作重新尊重本人意愿。"`
}

type factionPlannedAction struct {
	Matter         character.Matter
	PendingConsent *TurnCharacterExchangeInput
	Ops            []ActorStateOp
	Clock          *storyClockAdvance
}

func validateFactionInput(world *TurnWorldActionInput) string {
	in := world.Faction
	if in == nil || strings.TrimSpace(world.SourceText) == "" {
		return "组织事务需要具体行动及玩家本轮原话"
	}
	if strings.TrimSpace(in.ProjectID) == "" || len(in.ProjectID) > 120 || strings.ContainsAny(in.ProjectID, "/\\\x00\n\r") {
		return "组织事务需要简短稳定标识"
	}
	// No unrelated world fields may hide an extra journey, wait or task.
	clean := *world
	clean.Action = ""
	clean.SourceText = ""
	clean.Faction = nil
	if !clean.Empty() {
		return "一回合只能处理一种世界行动"
	}
	switch in.Action {
	case "start":
		if strings.TrimSpace(in.GroupName) == "" || utf8.RuneCountInString(in.GroupName) > 80 || in.PartnerID == "" || in.RecipeID == "" || in.Hours != 0 || in.SourceExchangeIndex < 1 {
			return "开始事务需要小组称呼、本人、配方及本轮同意，不提前计工时"
		}
	case "work":
		if in.Hours < 1 || in.SourceExchangeIndex < 1 || in.GroupName != "" || in.PartnerID != "" || in.RecipeID != "" {
			return "继续事务只填写已登记标识、授权工时及本人本轮同意"
		}
	case "cancel":
		if in.Hours != 0 || in.SourceExchangeIndex != 0 || in.GroupName != "" || in.PartnerID != "" || in.RecipeID != "" {
			return "停止事务只填写已登记标识，不执行或退回既有投入"
		}
	default:
		return "组织事务行动应为start、work或cancel"
	}
	return ""
}

func factionMatter(p *storyBranchProjection, id string) (*character.Matter, error) {
	var found *character.Matter
	if p != nil && p.Characters != nil {
		for _, m := range p.Characters.Matters {
			if m.Faction != nil && m.Faction.ProjectID == id {
				if found != nil {
					return nil, fmt.Errorf("faction affair identity is ambiguous")
				}
				copy := m.Normalize()
				found = &copy
			}
		}
	}
	return found, nil
}

func factionPartner(p *storyBranchProjection, scene map[string]any, rules AdventureRules, partnerID string) (character.Profile, AdventureActor, error) {
	if p == nil || p.Characters == nil || partnerID == DefaultActorID {
		return character.Profile{}, AdventureActor{}, fmt.Errorf("组织事务缺少已认识的本人")
	}
	partner, known := p.Characters.Profile(partnerID)
	if !known || !npcGrowthSharesScene(scene, partnerID) || actorIsArchived(scene, partnerID) {
		return partner, AdventureActor{}, fmt.Errorf("本人不在场，不能远程命令或凭关系调来")
	}
	arrangement, _ := actorStateFieldValue(scene, partnerID, xiuxianFieldArrangement).(string)
	if arrangement == xiuxianArrangementOccupied || arrangement == xiuxianArrangementTravel || arrangement == xiuxianArrangementRecovering {
		return partner, AdventureActor{}, fmt.Errorf("本人已有事务、在途中或正休养")
	}
	actor, err := AdventureActorFromState(scene, partnerID)
	if err != nil || ValidateAdventureActor(rules, actor) != nil || actor.Health <= 0 || actor.Battle != nil && actor.Battle.Status == "active" || partner.SourceKey != "" && actor.SourceKey != partner.SourceKey {
		return partner, actor, fmt.Errorf("本人当前能力、健康或来源不足以工作")
	}
	for _, b := range p.Combats {
		if !b.Ended {
			for _, a := range b.Combatants {
				if normalizeStatePanelActorID(a.ID) == partnerID {
					return partner, actor, fmt.Errorf("本人正在交战")
				}
			}
		}
	}
	return partner, actor, nil
}

// Only ordinary portable outputs are supported in this first slice. Powered
// assets and unique goods retain their existing owner-specific crafting path.
func factionRecipe(rules AdventureRules, actor AdventureActor, id string) (AdventureRecipe, AdventureSkill, error) {
	for _, recipe := range rules.Recipes {
		if recipe.ID != id {
			continue
		}
		if recipe.Hours < 1 || recipe.RequiredAsset != "" || recipe.FuelCost != 0 || recipe.OutputCount < 1 || len(recipe.Materials) == 0 {
			break
		}
		supported := false
		for _, item := range rules.Items {
			if item.ID == recipe.OutputID {
				supported = !item.Unique && item.Kind != "machine" && item.Kind != "ship" && item.MaxCharges == 0 && item.CooldownSeconds == 0
			}
		}
		if !supported {
			break
		}
		for _, skill := range rules.Skills {
			if skill.ID == recipe.SkillID && actor.Skills[skill.ID] >= max(1, recipe.SkillRank) && actor.Rank >= skill.RequiredRank && (skill.RequiredTool == "" || AdventureOwnedItemCount(actor, skill.RequiredTool) > 0) {
				return recipe, skill, nil
			}
		}
		break
	}
	return AdventureRecipe{}, AdventureSkill{}, fmt.Errorf("本人未掌握这份可共同制作的配方或缺少自己的工具")
}

func factionConsent(facts *TurnCharacterFactsInput, in FactionActionInput, partner string) (*TurnCharacterExchangeInput, error) {
	if facts == nil || in.SourceExchangeIndex < 1 || in.SourceExchangeIndex > len(facts.Exchanges) {
		return nil, fmt.Errorf("缺少本人对这件事务本轮的明确答复")
	}
	e := facts.Exchanges[in.SourceExchangeIndex-1]
	d := e.ParticipationDecision
	if e.CharacterID != partner || e.Speaker != string(character.SpeakerCharacter) || e.Channel != string(character.ChannelScene) || e.AddressedTo != DefaultActorID || strings.TrimSpace(e.Text) == "" || d == nil || d.Kind != character.ParticipationFaction || d.TargetID != in.ProjectID || d.Decision != character.ParticipationAccept {
		return nil, fmt.Errorf("本人未明确同意；拒绝、主角命令或普通承诺不算同意")
	}
	for _, later := range facts.Exchanges[in.SourceExchangeIndex:] {
		if later.CharacterID == partner && later.ParticipationDecision != nil && later.ParticipationDecision.Kind == character.ParticipationFaction && later.ParticipationDecision.TargetID == in.ProjectID {
			return nil, fmt.Errorf("应尊重本人对这件事务最后作出的回应")
		}
	}
	decision := d.Normalize()
	e.ParticipationDecision = &decision
	return &e, nil
}

func planFactionAction(p *storyBranchProjection, scene map[string]any, meta StoryMeta, system StoryDirectorActorStateSystem, branchID, turnID string, in FactionActionInput, facts *TurnCharacterFactsInput) (factionPlannedAction, error) {
	plan := factionPlannedAction{}
	if p == nil || system.AdventureRules == nil {
		return plan, fmt.Errorf("本题材尚无可用的组织共同制作规则")
	}
	prior, err := factionMatter(p, in.ProjectID)
	if err != nil {
		return plan, err
	}
	if in.Action == "start" {
		if prior != nil {
			return plan, fmt.Errorf("这件事务已登记，不能重开或重复领产物")
		}
		count := 0
		if p.Characters != nil {
			for _, m := range p.Characters.Matters {
				if m.Faction != nil && m.Faction.Phase == "active" {
					count++
				}
			}
		}
		if count >= 32 {
			return plan, fmt.Errorf("未完成共同事务已达32件，请先完成或停止旧事务")
		}
	} else {
		if prior == nil || prior.Faction.Phase != "active" {
			return plan, fmt.Errorf("没有可继续的这件共同事务")
		}
		plan.Matter = prior.Normalize()
		plan.Matter.BranchID = branchID
		if in.Action == "cancel" {
			plan.Matter.Faction.Phase = "cancelled"
			plan.Matter.Status = character.MatterAbandoned
			setFactionStatement(&plan.Matter)
			return plan, nil
		}
		in.PartnerID = prior.CharacterID
		in.RecipeID = prior.Faction.RecipeID
	}
	if err := realmActionBlocked(p, scene, meta); err != nil {
		return plan, err
	}
	for _, instance := range p.Realms {
		if instance.Snapshot.Presence == "inside" {
			return plan, fmt.Errorf("请先离开探索现场再安排共同事务")
		}
	}
	rules := *system.AdventureRules
	partner, npc, err := factionPartner(p, scene, rules, in.PartnerID)
	if err != nil {
		return plan, err
	}
	recipe, skill, err := factionRecipe(rules, npc, in.RecipeID)
	if err != nil {
		return plan, err
	}
	consent, err := factionConsent(facts, in, partner.ID)
	if err != nil {
		return plan, err
	}
	plan.PendingConsent = consent
	if in.Action == "start" {
		plan.Matter = character.Matter{ID: "faction:" + in.ProjectID, StoryID: meta.StoryID, BranchID: branchID, CharacterID: partner.ID, Kind: character.MatterAction, Status: character.MatterOpen, Counterparty: DefaultActorID, SourceEventID: "pending", ExchangeID: "pending", WorldTime: firstNonEmptyString(storySceneText(scene, storySceneTimeField), "当前"), Faction: &character.FactionTerms{ProjectID: in.ProjectID, GroupName: strings.TrimSpace(in.GroupName), RecipeID: recipe.ID, RequiredHours: recipe.Hours, Phase: "active", ConsentExchangeID: "pending"}}
		setFactionStatement(&plan.Matter)
		return plan, nil
	}
	terms := plan.Matter.Faction
	if recipe.Hours != terms.RequiredHours || in.Hours > terms.RequiredHours-terms.WorkedHours {
		return plan, fmt.Errorf("工时须在原配方剩余工时内，不能凭叙述缩短制作")
	}
	player, err := AdventureActorFromState(scene, DefaultActorID)
	if err != nil || ValidateAdventureActor(rules, player) != nil || player.Health <= 0 || player.Battle != nil && player.Battle.Status == "active" {
		return plan, fmt.Errorf("主角当前不能共同做工")
	}
	// Both people participate for the whole paid interval. The helper neither
	// lends the skill nor silently funds the protagonist's private production.
	npcCost := max(1, skill.Cost) * in.Hours
	if in.Hours > player.Energy || npcCost > npc.Energy {
		return plan, fmt.Errorf("双方现有精力不足，不能替本人透支或凭空恢复")
	}
	if terms.WorkedHours == 0 {
		for id, n := range recipe.Materials {
			if n < 1 || player.Inventory[id] < n {
				return plan, fmt.Errorf("玩家真实材料不足：%s", id)
			}
			for _, equipped := range player.Equipment {
				if equipped == id {
					return plan, fmt.Errorf("不能把已装备物品直接投入共同制作")
				}
			}
			if _, ok := player.Assets[id]; ok {
				return plan, fmt.Errorf("不能拆用已部署资产")
			}
			if _, ok := player.ItemCharges[id]; ok {
				return plan, fmt.Errorf("不能抹去物品现有充能记录")
			}
			if _, ok := player.ItemReadyAt[id]; ok {
				return plan, fmt.Errorf("不能抹去物品现有冷却记录")
			}
		}
		for id, n := range recipe.Materials {
			player.Inventory[id] -= n
		}
		terms.MaterialsSourceID = "pending"
	}
	player.Energy -= in.Hours
	npc.Energy -= npcCost
	terms.WorkedHours += in.Hours
	terms.ConsentExchangeID = "pending"
	if terms.WorkedHours == terms.RequiredHours {
		player.Inventory[recipe.OutputID] += recipe.OutputCount
		if player.Acquired == nil {
			player.Acquired = map[string]bool{}
		}
		player.Acquired[recipe.OutputID] = true
		terms.Phase = "completed"
		terms.OutputSourceID = "pending"
		plan.Matter.Status = character.MatterSettled
	}
	if err := ValidateAdventureActor(rules, player); err != nil {
		return plan, err
	}
	if err := ValidateAdventureActor(rules, npc); err != nil {
		return plan, err
	}
	plan.Ops = adventureActorOps(player, turnID)
	for _, op := range adventureActorOps(npc, turnID) {
		op.ActorID = partner.ID
		plan.Ops = append(plan.Ops, op)
	}
	for i := range plan.Ops {
		plan.Ops[i].SourceKind = TurnWorldActionFaction
	}
	clock := NewStoryClock(storySceneText(scene, storySceneTimeField))
	if p.Clock != nil {
		clock = *p.Clock
	}
	after, err := AdvanceStoryClock(clock, int64(in.Hours)*3600)
	if err != nil {
		return plan, err
	}
	if adventureOpeningBoundary(scene, clock.Seconds, after.Seconds, p.worldContent()) < after.Seconds {
		return plan, fmt.Errorf("眼前危险会打断这段工作，请先离开危险或缩短工时")
	}
	plan.Clock = &storyClockAdvance{Before: clock, After: after}
	setFactionStatement(&plan.Matter)
	return plan, nil
}

func setFactionStatement(m *character.Matter) {
	f := m.Faction
	phases := map[string]string{"active": "进行中", "completed": "已完成并交付原配方产物", "cancelled": "已停止，已投入的材料和时间不返还"}
	m.Statement = fmt.Sprintf("%s：与主角按约定配方共同制作，已投入 %d/%d 小时，%s。", f.GroupName, f.WorkedHours, f.RequiredHours, phases[f.Phase])
	m.StatementTranslations = map[string]string{"en-US": fmt.Sprintf("%s: shared production using the agreed recipe, %d/%d hours worked; %s. Materials already used are not refunded.", f.GroupName, f.WorkedHours, f.RequiredHours, f.Phase)}
}
