package interactive

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"ta/internal/character"
	"ta/internal/exploration"
)

const TurnWorldActionCooperation = "cooperation"
const StateOpSourceCooperation = "cooperation"

// CooperationActionInput proposes only a bounded action. Prices, participants,
// abilities and plan results always come from committed facts and frozen rules.
type CooperationActionInput struct {
	ProjectID        string   `json:"project_id" jsonschema_description:"当前探索页列出的协作项目，不得自造。"`
	InstanceID       string   `json:"instance_id" jsonschema_description:"玩家亲自调查过的本分支实例。"`
	Action           string   `json:"action" jsonschema:"enum=propose,enum=work_if_agreed,enum=work,enum=report" jsonschema_description:"propose询问，可不分享；work_if_agreed明确分享并请求若对方同意就共同工作；work执行已有正式同意；report仅汇报玩家选择的真实结果。费用不足或本人拒绝不能写成工作成功，普通Promise不是同意。"`
	ShareDiscovery   bool     `json:"share_discovery,omitempty" jsonschema_description:"仅玩家本轮明确愿意分享调查线索时为true。询问不等于分享。"`
	ReportSourceRefs []string `json:"report_source_refs,omitempty" jsonschema_description:"report专用：仅复制玩家明确选择分享的结果来源，最多四类。未选择的事实不能告诉对方。"`
}

type cooperationPlannedAction struct {
	Definition              CooperationProjectDefinition
	PartnerID               string
	Matter                  *character.Matter
	ShareSource             string
	ShareStatement          string
	Reports                 []CooperationReportFactView
	PendingConsent          *TurnCharacterExchangeInput
	ClockBefore, ClockAfter *StoryClock
	Ops                     []ActorStateOp
	Outcome                 string
}

func cooperationPartner(p *storyBranchProjection, d CooperationProjectDefinition) (character.Profile, bool) {
	if p == nil || p.Characters == nil {
		return character.Profile{}, false
	}
	var found character.Profile
	for _, profile := range p.Characters.Profiles {
		if profile.SourceKey == d.PartnerSourceKey {
			if found.ID != "" {
				return character.Profile{}, false
			}
			found = profile
		}
	}
	return found, found.ID != "" && found.ID != DefaultActorID
}

func cooperationMatter(p *storyBranchProjection, projectID, instanceID string) (*character.Matter, bool) {
	if p == nil || p.Characters == nil {
		return nil, false
	}
	inst, ok := p.Realms[instanceID]
	if !ok {
		return nil, false
	}
	d, ok := cooperationProject(projectID, p.worldContent())
	if !ok {
		return nil, false
	}
	partner, ok := cooperationPartner(p, d)
	if !ok {
		return nil, false
	}
	for _, m := range p.Characters.Matters {
		c := m.Cooperation
		if c != nil && m.CharacterID == partner.ID && m.BranchID == inst.Snapshot.Binding.BranchID && m.StoryID == inst.Snapshot.Binding.WorldID && c.ProjectID == projectID && c.ProjectVersion == d.Version && c.RealmInstanceID == instanceID {
			copy := m.Normalize()
			return &copy, true
		}
	}
	return nil, false
}

func cooperationCapability(p *storyBranchProjection, projectID string, snapshot exploration.DungeonInstanceSnapshot) bool {
	m, ok := cooperationMatter(p, projectID, snapshot.Binding.InstanceID)
	return ok && m.Cooperation.Phase == character.CooperationWorked && m.Cooperation.UsedBy == "" && m.Cooperation.WorkSourceRef != "" && m.BranchID == snapshot.Binding.BranchID
}

func cooperationParticipants(p *storyBranchProjection, scene map[string]any, meta StoryMeta, d CooperationProjectDefinition) (character.Profile, string) {
	partner, ok := cooperationPartner(p, d)
	if !ok {
		return partner, "not_present"
	}
	if location, ok := storyLocationID(scene, p.worldContent()); !ok || location != d.LocationID {
		return partner, "not_here"
	}
	for _, r := range p.Realms {
		if r.Snapshot.Presence == exploration.InstanceInside {
			return partner, "inside_realm"
		}
	}
	if !npcGrowthSharesScene(scene, partner.ID) || actorIsArchived(scene, partner.ID) {
		return partner, "not_present"
	}
	if realmActionBlocked(p, scene, meta) != nil {
		return partner, "cannot_act"
	}
	arrangement, _ := actorStateFieldValue(scene, partner.ID, xiuxianFieldArrangement).(string)
	if arrangement == xiuxianArrangementOccupied || arrangement == xiuxianArrangementTravel || arrangement == xiuxianArrangementRecovering {
		return partner, "partner_unavailable"
	}
	for _, combat := range p.Combats {
		if !combat.Ended {
			for _, a := range combat.Combatants {
				if normalizeStatePanelActorID(a.ID) == partner.ID {
					return partner, "partner_unavailable"
				}
			}
		}
	}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, StoryDirectorActorStateSystem{})
	if system.AdventureRules != nil && getPath(scene, actorStateFieldPath(partner.ID, AdventureStateField)) != nil {
		actual, err := AdventureActorFromState(scene, partner.ID)
		if err != nil || actual.SourceKey != partner.SourceKey || actual.Health <= 0 || actual.Battle != nil && actual.Battle.Status == "active" {
			return partner, "partner_unavailable"
		}
	}
	if actorStateFieldValue(scene, partner.ID, xiuxianFieldHealth) != nil && xiuxianStoredInt(scene, partner.ID, xiuxianFieldHealth) <= 0 {
		return partner, "partner_unavailable"
	}
	return partner, ""
}

// This check is shared by read views and formal work. It reads the committed
// actor only; same-turn narration cannot supply tools, skills, health or presence.
func cooperationWorkEligibility(p *storyBranchProjection, scene map[string]any, meta StoryMeta, d CooperationProjectDefinition, instance RealmInstanceState) ([]ActorStateOp, int, string) {
	partner, known := cooperationPartner(p, d)
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, StoryDirectorActorStateSystem{})
	cost := 0
	var npc AdventureActor
	var npcOps []ActorStateOp
	if d.PartnerSkillID != "" && known && system.AdventureRules != nil {
		var err error
		npc, _, err = cooperationNPCActor(scene, *system.AdventureRules, partner)
		// Only a genuinely uninitialized established actor may use its frozen
		// initial definition. Existing damaged or corrupt state is never reset.
		if err != nil || npc.SourceKey != partner.SourceKey {
			return nil, 0, "partner_unavailable"
		}
		rules, err := adventureRulesForActor(*system.AdventureRules, npc)
		if err != nil || ValidateAdventureActor(rules, npc) != nil {
			return nil, 0, "partner_unavailable"
		}
		found := false
		for _, skill := range rules.Skills {
			if (skill.ID == d.PartnerSkillID || skill.GeneratedFrom == d.PartnerSkillID) && npc.Skills[skill.ID] > 0 && npc.Rank >= skill.RequiredRank && (skill.RequiredTool == "" || AdventureOwnedItemCount(npc, skill.RequiredTool) > 0) {
				cost = skill.Cost
				found = true
				break
			}
		}
		if !found || cost < 0 || npc.Health <= 0 || npc.Battle != nil && npc.Battle.Status == "active" {
			return nil, cost, "partner_unavailable"
		}
	}
	if _, reason := cooperationParticipants(p, scene, meta, d); reason != "" {
		return nil, cost, reason
	}
	s := instance.Snapshot
	if s.Binding.DefinitionID != d.RealmID || s.ExploreState == nil || instance.ExploreSources["clue"] == "" || !stringSliceContains(s.ExploreState.UsedApproachIDs, d.RealmID+".survey") {
		return nil, cost, "missing_discovery"
	}
	if s.ExploreState.Setback {
		return nil, cost, "setback"
	}
	for _, decision := range s.ExploreState.Decisions {
		if decision.GroupID == "ingress" {
			return nil, cost, "ingress_taken"
		}
	}
	if d.PartnerSkillID != "" {
		if system.AdventureRules == nil || npc.Energy < cost {
			return nil, cost, "partner_energy"
		}
		npc.Energy -= cost
		npcOps = adventureActorOps(npc, d.ID)
		for i := range npcOps {
			npcOps[i].ActorID = partner.ID
			npcOps[i].SourceKind = StateOpSourceCooperation
		}
	}
	var costs []exploration.ExploreResourceCost
	if d.PlayerItemID != "" {
		costs = []exploration.ExploreResourceCost{{Kind: exploration.ExploreConsumeItem, RefID: d.PlayerItemID, Quantity: int64(d.PlayerItemQuantity)}}
	}
	def, ok := worldRealmDefinition(p.worldContent(), d.RealmID)
	if !ok {
		return nil, cost, "missing_discovery"
	}
	ops, err := realmExploreActorOps(scene, system, def, costs, nil, d.ID)
	if err != nil {
		return nil, cost, "missing_material"
	}
	for i := range ops {
		ops[i].SourceKind = StateOpSourceCooperation
	}
	return append(ops, npcOps...), cost, ""
}

func planCooperationAction(p *storyBranchProjection, scene map[string]any, meta StoryMeta, branchID, turnID string, input *CooperationActionInput, facts *TurnCharacterFactsInput) (cooperationPlannedAction, error) {
	if p == nil || input == nil {
		return cooperationPlannedAction{}, fmt.Errorf("协作缺少已提交的现场")
	}
	d, ok := cooperationProject(strings.TrimSpace(input.ProjectID), p.worldContent())
	if !ok {
		return cooperationPlannedAction{}, fmt.Errorf("未登记的协作项目")
	}
	inst, ok := p.Realms[input.InstanceID]
	if !ok || inst.Snapshot.Binding.BranchID != branchID || inst.Snapshot.Binding.WorldID != meta.StoryID || inst.Snapshot.Binding.DefinitionID != d.RealmID || inst.ExploreSources["clue"] == "" {
		return cooperationPlannedAction{}, fmt.Errorf("尚未亲自调查本分支的这一处场地")
	}
	definition, valid := cooperationViewDefinition(inst, p.worldContent())
	if !valid || !storyPackAllows(meta, definition.PackageID, definition.PackageVersion) || !cooperationViewHasClue(inst, definition) {
		return cooperationPlannedAction{}, fmt.Errorf("协作场地版本或真实调查来源不符")
	}
	partner, reason := cooperationParticipants(p, scene, meta, d)
	if reason != "" {
		return cooperationPlannedAction{}, fmt.Errorf("协作尚不能进行：%s", reason)
	}
	plan := cooperationPlannedAction{Definition: d, PartnerID: partner.ID, Outcome: "asked"}
	prior, hasMatter := cooperationMatter(p, d.ID, input.InstanceID)
	if facts != nil {
		for _, k := range facts.Knowledge {
			if k.CharacterID == partner.ID {
				return plan, fmt.Errorf("此次协作/汇报只由服务写入明确共享的知识")
			}
		}
	}
	if input.Action == "report" {
		if !hasMatter || prior.Cooperation.Phase == character.CooperationAgreed {
			return plan, fmt.Errorf("没有这次共同工作的真实记录")
		}
		available := cooperationReportFacts(p, input.InstanceID)
		if len(input.ReportSourceRefs) == 0 || len(input.ReportSourceRefs) > 4 {
			return plan, fmt.Errorf("只汇报玩家选择的真实新结果")
		}
		seen := map[string]bool{}
		for _, ref := range input.ReportSourceRefs {
			if seen[ref] {
				return plan, fmt.Errorf("重复的汇报结果")
			}
			seen[ref] = true
			if stringSliceContains(prior.Cooperation.ReportSourceRefs, ref) {
				continue
			}
			found := false
			for _, f := range available {
				if f.SourceRef == ref {
					plan.Reports = append(plan.Reports, f)
					found = true
					break
				}
			}
			if !found {
				return plan, fmt.Errorf("该结果不是本实例已经发生的事实")
			}
		}
		if len(plan.Reports) == 0 {
			plan.Outcome = "already_reported"
			return plan, nil
		}
		prior.Cooperation.Phase = character.CooperationReported
		for _, f := range plan.Reports {
			prior.Cooperation.ReportSourceRefs = append(prior.Cooperation.ReportSourceRefs, f.SourceRef)
		}
		prior.Status = character.MatterSettled
		plan.Matter = prior
		plan.Outcome = "reported"
		return plan, nil
	}
	if hasMatter && prior.Cooperation.Phase != character.CooperationAgreed {
		plan.Outcome = "already_worked"
		return plan, nil
	}
	// Sharing is explicit and survives refusal, without implying agreement.
	if input.ShareDiscovery {
		plan.ShareSource = inst.ExploreSources["clue"]
		for _, discovery := range inst.Snapshot.Discoveries {
			if discovery.Kind == exploration.DiscoveryFact && discovery.ID == d.RealmID+".clue" {
				plan.ShareStatement = discovery.Text
				break
			}
		}
	}
	ops, _, blocked := cooperationWorkEligibility(p, scene, meta, d, inst)
	if blocked == "ingress_taken" || blocked == "setback" || blocked == "missing_discovery" {
		return plan, fmt.Errorf("这次调查已不能再准备新方案：%s", blocked)
	}
	var consent *character.Exchange
	if facts != nil {
		for _, e := range facts.Exchanges {
			c := e.CooperationDecision
			if c == nil || c.ProjectID != d.ID || c.RealmInstanceID != input.InstanceID {
				continue
			}
			if e.CharacterID != partner.ID || e.Speaker != string(character.SpeakerCharacter) || (e.Channel != string(character.ChannelScene) && e.Channel != string(character.ChannelPrivate)) || e.Text == "" || c.Validate() != nil || e.AddressedTo != "" && e.AddressedTo != DefaultActorID {
				return plan, fmt.Errorf("协作回应必须是现场本人向主角作出的具体回应")
			}
			if plan.PendingConsent != nil {
				return plan, fmt.Errorf("本轮有相互冲突的协作回应")
			}
			copy := e
			decision := c.Normalize()
			copy.CooperationDecision = &decision
			plan.PendingConsent = &copy
		}
	}
	if plan.PendingConsent != nil {
		if plan.PendingConsent.CooperationDecision.Decision == character.CooperationRefuse {
			plan.Outcome = "refused"
			return plan, nil
		}
		if input.ShareDiscovery || hasMatter {
			consent = &character.Exchange{ID: "pending:" + turnID, CharacterID: partner.ID, Text: plan.PendingConsent.Text}
		}
	} else if input.ShareDiscovery || hasMatter {
		consent = cooperationLatestConsent(p, d, input.InstanceID, partner.ID)
	}

	if consent == nil {
		if input.Action == "work" || input.Action == "work_if_agreed" {
			return plan, fmt.Errorf("缺少本人对这件事的正式同意；不能用普通承诺代替")
		}
		return plan, nil
	}
	if !hasMatter {
		prior = &character.Matter{ID: "coop:" + branchID + ":" + d.ID + ":" + input.InstanceID + ":" + partner.ID, StoryID: meta.StoryID, BranchID: branchID, CharacterID: partner.ID, Kind: character.MatterAction, Status: character.MatterOpen, Statement: firstNonEmptyString(consent.Text, "共同核对亲自调查所得，为这一处场地准备一次行进方案"), Counterparty: DefaultActorID, WorldTime: firstNonEmptyString(storySceneText(scene, storySceneTimeField), "当前"), TurnID: turnID, ExchangeID: consent.ID, SourceEventID: "pending", Cooperation: &character.CooperationTerms{ProjectID: d.ID, ProjectVersion: d.Version, RealmInstanceID: input.InstanceID, ConsentExchangeID: consent.ID, SharedDiscoverySourceRef: inst.ExploreSources["clue"], Phase: character.CooperationAgreed}}
	}
	if !hasMatter || prior.Cooperation.ConsentExchangeID != consent.ID {
		prior.Cooperation.ConsentExchangeID = consent.ID
		prior.ExchangeID = consent.ID
		plan.Matter = prior
	}
	plan.Outcome = "agreed"
	if input.Action == "propose" {
		return plan, nil
	}
	if blocked != "" {
		plan.Outcome = blocked
		return plan, nil
	}
	// Cost-owning fields cannot also be rewritten by narration in this turn.
	for _, op := range ops {
		if !reflect.DeepEqual(actorStateFieldValue(scene, op.ActorID, op.FieldID), actorStateFieldValue(p.State, op.ActorID, op.FieldID)) {
			return plan, fmt.Errorf("协作费用字段不能同时由正文改写")
		}
	}
	clock := NewStoryClock(storySceneText(scene, storySceneTimeField))
	if p.Clock != nil {
		clock = *p.Clock
	}
	after, err := AdvanceStoryClock(clock, d.DurationSeconds)
	if err != nil {
		return plan, err
	}
	if boundary := lifespanBoundary(meta, scene, DefaultActorID); boundary != nil && after.Seconds > boundary.At {
		return plan, fmt.Errorf("剩余寿元不足以完成共同工作")
	}
	if adventureOpeningBoundary(scene, clock.Seconds, after.Seconds, p.worldContent()) < after.Seconds {
		return plan, fmt.Errorf("眼前危险会打断共同工作")
	}
	plan.Matter = prior
	prior.Cooperation.Phase = character.CooperationWorked
	prior.Cooperation.WorkSourceRef = "pending"
	plan.ClockBefore = &clock
	plan.ClockAfter = &after
	plan.Ops = ops
	plan.Outcome = "worked"
	return plan, nil
}

func validateCooperationInput(world *TurnWorldActionInput) string {
	c := world.Cooperation
	if c == nil || strings.TrimSpace(c.ProjectID) == "" || strings.TrimSpace(c.InstanceID) == "" || strings.TrimSpace(world.SourceText) == "" {
		return "协作须填写已知项目、实例与玩家本轮原话"
	}
	other := *world
	other.Cooperation = nil
	other.Action = ""
	other.SourceText = ""
	if !other.Empty() {
		return "每回合只结算一项协作，不能夹带其他世界行动"
	}
	switch c.Action {
	case "propose", "work_if_agreed", "work":
		if len(c.ReportSourceRefs) != 0 {
			return "工作请求不能夹带结果汇报"
		}
		if c.Action == "work_if_agreed" && !c.ShareDiscovery {
			return "条件共同工作须由玩家明确分享调查线索"
		}
	case "report":
		if c.ShareDiscovery || len(c.ReportSourceRefs) == 0 || len(c.ReportSourceRefs) > 4 {
			return "仅报告玩家选中的一至四类真实结果"
		}
	default:
		return "未知协作行动"
	}
	return ""
}

// A profile may be formally established before an actor has ever needed a
// mechanical state. Materialize that frozen initial state only once, at actual
// work, rather than requiring an artificial wait to trigger NPC growth first.
// Existing empty/corrupt canonical data or any mechanical field is not absence.
func cooperationNPCActor(scene map[string]any, rules AdventureRules, profile character.Profile) (AdventureActor, bool, error) {
	if getPath(scene, actorStateFieldPath(profile.ID, AdventureStateField)) != nil {
		actor, err := AdventureActorFromState(scene, profile.ID)
		return actor, false, err
	}
	if actors, ok := scene[actorStateRoot].(map[string]any); ok {
		if entry, ok := actors[profile.ID].(map[string]any); ok {
			if fields, ok := entry["state"].(map[string]any); ok && len(fields) > 0 {
				return AdventureActor{}, false, fmt.Errorf("partial NPC state cannot be reinitialized")
			}
		}
	}
	for _, npc := range rules.NPCs {
		if npc.CharacterSourceKey != profile.SourceKey {
			continue
		}
		var actor AdventureActor
		data, err := json.Marshal(npc.Actor)
		if err != nil {
			return actor, false, err
		}
		if err = json.Unmarshal(data, &actor); err != nil {
			return actor, false, err
		}
		actor.SourceKey = profile.SourceKey
		return actor, true, nil
	}
	return AdventureActor{}, false, fmt.Errorf("NPC has no frozen initial state")
}

// The branch's canonical character projection already filters provenance to
// its ancestry. A fork may therefore keep a genuine ancestor's answer without
// pretending that answer was spoken again on the new branch. Newer refusals
// revoke older acceptance for this exact instance, not every future project.
func cooperationLatestConsent(p *storyBranchProjection, d CooperationProjectDefinition, instanceID, partnerID string) *character.Exchange {
	if p == nil || p.Characters == nil {
		return nil
	}
	instance, ok := p.Realms[instanceID]
	if !ok {
		return nil
	}
	for i := len(p.Characters.Exchanges) - 1; i >= 0; i-- {
		e := p.Characters.Exchanges[i]
		c := e.CooperationDecision
		if e.CharacterID != partnerID || c == nil || c.ProjectID != d.ID || c.RealmInstanceID != instanceID {
			continue
		}
		if e.StoryID != instance.Snapshot.Binding.WorldID || e.SourceEventID == "" || e.Speaker != character.SpeakerCharacter || (e.Channel != character.ChannelScene && e.Channel != character.ChannelPrivate) || e.AddressedTo != "" && e.AddressedTo != DefaultActorID || c.Validate() != nil {
			return nil
		}
		if c.Decision == character.CooperationRefuse {
			return nil
		}
		return &e
	}
	return nil
}
