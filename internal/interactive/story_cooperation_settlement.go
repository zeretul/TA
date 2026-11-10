package interactive

import (
	"fmt"
	"strings"

	"ta/internal/character"
	"ta/internal/exploration"
)

// Bind only after the normal character projector has assigned the real exchange
// identity. A model-visible response candidate never becomes its own source.
func bindCooperationConsent(plan *cooperationPlannedAction, record *CharacterRecordEvent) error {
	if plan == nil || plan.Matter == nil || !strings.HasPrefix(plan.Matter.Cooperation.ConsentExchangeID, "pending:") {
		return nil
	}
	if plan.PendingConsent == nil || record == nil {
		return fmt.Errorf("协作同意未进入本轮真实人物回应")
	}
	expected := plan.PendingConsent
	matches := []character.Exchange{}
	for _, e := range record.Exchanges {
		c := e.CooperationDecision
		if e.CharacterID == plan.PartnerID && e.Speaker == character.SpeakerCharacter && string(e.Channel) == expected.Channel && e.Text == strings.TrimSpace(expected.Text) && e.SourceEventID == record.ID && c != nil && c.Decision == character.CooperationAccept && c.ProjectID == plan.Definition.ID && c.RealmInstanceID == plan.Matter.Cooperation.RealmInstanceID {
			matches = append(matches, e)
		}
	}
	if len(matches) != 1 {
		return fmt.Errorf("协作同意来源无法唯一核对")
	}
	plan.Matter.Cooperation.ConsentExchangeID = matches[0].ID
	plan.Matter.ExchangeID = matches[0].ID
	return nil
}

func cooperationSettlementEvents(p *storyBranchProjection, system StoryDirectorActorStateSystem, branchID, parentID, now, turnID string, plan cooperationPlannedAction) ([]any, string, error) {
	id := newID("cooprec")
	event := CooperationRecordEvent{V: schemaVersion, Type: StoryEventTypeCooperationRecord, ID: id, ParentID: parentID, BranchID: branchID, Ts: now, TurnID: turnID, Cursor: id, ClockBefore: cloneStoryClock(plan.ClockBefore), ClockAfter: cloneStoryClock(plan.ClockAfter)}
	if plan.Matter != nil {
		m := plan.Matter.Normalize()
		m.SourceEventID = id
		m.TurnID = turnID
		if m.Cooperation.WorkSourceRef == "pending" {
			m.Cooperation.WorkSourceRef = id
		}
		if strings.HasPrefix(m.Cooperation.ConsentExchangeID, "pending:") {
			return nil, "", fmt.Errorf("协作同意尚未绑定实际回应")
		}
		event.Matter = &m
	}
	addKnowledge := func(key, text string) {
		knowledgeID := "coop-knowledge:" + plan.PartnerID + ":" + key
		if p.Characters != nil {
			for _, k := range p.Characters.Knowledge {
				if k.ID == knowledgeID {
					return
				}
			}
		}
		event.Knowledge = append(event.Knowledge, character.KnowledgeRecord{ID: knowledgeID, CharacterID: plan.PartnerID, Statement: text, Nature: character.NatureTold, Audience: character.AudienceSingle, LearnedFrom: DefaultActorID, SourceEventID: id, SourceTurnID: turnID, FormalCommitID: id, WorldTime: firstNonEmptyString(storySceneText(p.State, storySceneTimeField), "当前")})
	}
	if plan.ShareSource != "" {
		addKnowledge("clue:"+plan.ShareSource, "主角向我分享了亲自调查所得："+plan.ShareStatement+"这是主角所告知，并非我亲历。")
	}
	for _, f := range plan.Reports {
		addKnowledge(f.SourceRef, cooperationReportStatement(plan.Definition, f, p.worldContent()))
	}
	if event.Matter == nil && len(event.Knowledge) == 0 {
		return nil, parentID, nil
	}
	if err := validateCooperationRecordEvent(event); err != nil {
		return nil, "", err
	}
	events := []any{event}
	head := id
	if plan.ClockAfter != nil {
		interval, next := storyIntervalEvents(intervalProjectionAfterOps(p, plan.Ops), system, branchID, head, now, turnID, plan.PartnerID, *plan.ClockBefore, *plan.ClockAfter)
		events = append(events, interval...)
		head = next
	}
	return events, head, nil
}

func cooperationReportStatement(d CooperationProjectDefinition, f CooperationReportFactView, content *WorldContent) string {
	label := ""
	if def, ok := worldRealmDefinition(content, d.RealmID); ok && def.ExploreRules != nil {
		for _, a := range def.ExploreRules.Approaches {
			if a.NameKey == f.ValueKey {
				label = a.Name
				break
			}
		}
	}
	switch f.Kind {
	case "ingress":
		return "主角告诉我，后来实际采用了“" + label + "”进入场地；我没有亲历。"
	case "disposition":
		return "主角告诉我，对核心发现选择了“" + label + "”；我没有亲历。"
	case "setback":
		return "主角告诉我，这一次探索曾经受挫；我没有亲历。"
	case "side":
		return "主角告诉我，后来确实走到了该处的侧区；我没有亲历。"
	}
	return ""
}

// Plan use is a consequence of the actual Realm record, not selecting a method
// in a preview. Both the standalone rule action and a formal turn call this.
func cooperationUseEvent(p *storyBranchProjection, branchID, parentID, now, turnID string, realm RealmRecordEvent) (*CooperationRecordEvent, error) {
	def, ok := worldRealmDefinition(p.worldContent(), realm.RealmID)
	if !ok {
		return nil, nil
	}
	method, ok := exploration.FindExploreApproach(def, realm.ApproachID)
	if !ok || method.CooperationProjectID == "" {
		return nil, nil
	}
	m, ok := cooperationMatter(p, method.CooperationProjectID, realm.Instance.Binding.InstanceID)
	if !ok || !cooperationCapability(p, method.CooperationProjectID, realm.Instance) {
		return nil, fmt.Errorf("本分支没有这份可用协作方案")
	}
	// An approach that hits the pressure boundary still spends the one plan: it
	// was attempted, not refunded by a later failure.
	id := newID("cooprec")
	m.Cooperation.Phase = character.CooperationUsed
	m.Cooperation.UsedBy = realm.ID
	m.SourceEventID = id
	m.TurnID = turnID
	m.Status = character.MatterSettled
	event := CooperationRecordEvent{V: schemaVersion, Type: StoryEventTypeCooperationRecord, ID: id, ParentID: parentID, BranchID: branchID, Ts: now, TurnID: turnID, Cursor: id, Matter: m}
	if err := validateCooperationRecordEvent(event); err != nil {
		return nil, err
	}
	return &event, nil
}
