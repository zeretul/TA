package interactive

import (
	"fmt"
	"strings"

	"ta/internal/character"
)

func bindFactionConsent(plan *factionPlannedAction, record *CharacterRecordEvent) error {
	if plan.PendingConsent == nil {
		return nil
	}
	if record == nil {
		return fmt.Errorf("共同事务缺少已正式记录的本人回应")
	}
	expected := plan.PendingConsent
	var matched string
	for _, e := range record.Exchanges {
		d := e.ParticipationDecision
		if e.CharacterID == plan.Matter.CharacterID && e.Speaker == character.SpeakerCharacter && e.Channel == character.ChannelScene && e.AddressedTo == DefaultActorID && e.SourceEventID == record.ID && e.Text == strings.TrimSpace(expected.Text) && d != nil && d.Kind == character.ParticipationFaction && d.TargetID == plan.Matter.Faction.ProjectID && d.Decision == character.ParticipationAccept {
			// The planner requires the final scoped response. Projection retains
			// exchange order, so repeated identical words bind the last real reply.
			matched = e.ID
		}
	}
	if matched == "" {
		return fmt.Errorf("共同事务同意未绑定真实回应")
	}
	plan.Matter.ExchangeID = matched
	plan.Matter.Faction.ConsentExchangeID = matched
	return nil
}

func factionSettlementEvents(p *storyBranchProjection, system StoryDirectorActorStateSystem, branchID, parentID, now, turnID string, plan factionPlannedAction) ([]any, string, error) {
	id := newID("chr")
	m := plan.Matter.Normalize()
	m.SourceEventID = id
	m.TurnID = turnID
	m.BranchID = branchID
	if m.Faction.ConsentExchangeID == "pending" {
		return nil, "", fmt.Errorf("共同事务尚无正式本人同意")
	}
	if m.Faction.MaterialsSourceID == "pending" {
		m.Faction.MaterialsSourceID = id
	}
	if m.Faction.OutputSourceID == "pending" {
		m.Faction.OutputSourceID = id
	}
	event := CharacterRecordEvent{V: schemaVersion, Type: StoryEventTypeCharacterRecord, ID: id, ParentID: parentID, BranchID: branchID, Ts: now, TurnID: turnID, Cursor: id, FactionMatter: &m}
	if err := validateCharacterRecordEvent(event); err != nil {
		return nil, "", err
	}
	events := []any{event}
	head := id
	if plan.Clock != nil {
		interval, next := storyIntervalEvents(intervalProjectionAfterOps(p, plan.Ops), system, branchID, head, now, turnID, m.CharacterID, plan.Clock.Before, plan.Clock.After)
		events = append(events, interval...)
		head = next
	}
	return events, head, nil
}

// Replay verifies the source already on this branch, not merely a nonempty ID.
func validateFactionRecordSource(state character.State, event CharacterRecordEvent) error {
	m := event.FactionMatter
	if m == nil || m.Faction == nil {
		return fmt.Errorf("faction record has no terms")
	}
	f := m.Faction
	found := false
	for _, e := range state.Exchanges {
		d := e.ParticipationDecision
		if e.ID == f.ConsentExchangeID && e.CharacterID == m.CharacterID && e.StoryID == m.StoryID && e.Speaker == character.SpeakerCharacter && e.Channel == character.ChannelScene && e.AddressedTo == DefaultActorID && e.SourceEventID != "" && (f.Phase == "cancelled" || e.TurnID == event.TurnID) && d != nil && d.Kind == character.ParticipationFaction && d.TargetID == f.ProjectID && d.Decision == character.ParticipationAccept {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("faction record lacks its actual consenting speaker")
	}
	fundedBefore := false
	for _, prior := range state.Matters {
		if prior.ID == m.ID && prior.Faction != nil {
			fundedBefore = prior.Faction.MaterialsSourceID != ""
		}
	}
	if !fundedBefore && f.WorkedHours > 0 && f.MaterialsSourceID != event.ID {
		return fmt.Errorf("first faction work must name its own material event")
	}
	if f.Phase == "completed" && f.OutputSourceID != event.ID {
		return fmt.Errorf("faction output must name its own completion event")
	}
	return nil
}

func factionModuleConflict(result *TurnResult) bool {
	if result == nil || result.World == nil || strings.TrimSpace(result.World.Action) != TurnWorldActionFaction {
		return false
	}
	return result.Items != nil && !result.Items.Empty() || result.Trade != nil && !result.Trade.Empty() || result.Skills != nil && !result.Skills.Empty() || result.Techniques != nil && !result.Techniques.Empty()
}
