package interactive

import (
	"fmt"
	"sort"
	"strings"

	"ta/internal/character"
	"ta/internal/quest"
)

// QuestFollowUp is an available encounter, not a prediction of its answer.
// Only the narrator receives these IDs; the player's panel gets committed facts.
type QuestFollowUp struct {
	ChoiceID    string
	CharacterID string
}

type questEchoAcknowledgement struct {
	Partner     character.Profile
	Statement   string
	StatementEN string
	Clock       StoryClock
}

// A source must be the same established person and genuinely able to take part
// here. An author source key alone cannot summon, revive or speak for an NPC.
func questEchoPartner(p *storyBranchProjection, sourceKey string) (character.Profile, bool) {
	if p == nil || p.Characters == nil || sourceKey == "" {
		return character.Profile{}, false
	}
	var found character.Profile
	for _, profile := range p.Characters.Profiles {
		if profile.SourceKey != sourceKey {
			continue
		}
		if found.ID != "" {
			return character.Profile{}, false
		}
		found = profile
	}
	if found.ID == "" || found.ID == DefaultActorID || found.StoryID == "" || !npcGrowthSharesScene(p.State, found.ID) || actorIsArchived(p.State, found.ID) {
		return character.Profile{}, false
	}
	native, nativeObject := getPath(p.State, actorStateFieldPath(found.ID, AdventureStateField)).(map[string]any)
	// Native templates contain zero placeholders until the first paid interval
	// seeds their authored state. They are not a committed death or injury.
	uninitialized := nativeObject && len(native) == 0 &&
		getPath(p.State, actorStateRoot+"."+found.ID+".template_id") == AdventureNPCTemplateID &&
		xiuxianStoredInt(p.State, found.ID, xiuxianFieldHealthMax) == 0 &&
		xiuxianStoredInt(p.State, found.ID, "历练等级") == 0
	if !uninitialized {
		if actorStateFieldValue(p.State, found.ID, xiuxianFieldHealth) != nil && xiuxianStoredInt(p.State, found.ID, xiuxianFieldHealth) <= 0 {
			return character.Profile{}, false
		}
		if getPath(p.State, actorStateFieldPath(found.ID, AdventureStateField)) != nil {
			a, err := AdventureActorFromState(p.State, found.ID)
			if err != nil || a.Health <= 0 || a.Battle != nil && a.Battle.Status == "active" {
				return character.Profile{}, false
			}
		}
	}
	arrangement, _ := actorStateFieldValue(p.State, found.ID, xiuxianFieldArrangement).(string)
	if arrangement == xiuxianArrangementOccupied || arrangement == xiuxianArrangementTravel || arrangement == xiuxianArrangementRecovering {
		return character.Profile{}, false
	}
	for _, combat := range p.Combats {
		if combat.Ended {
			continue
		}
		for _, actor := range combat.Combatants {
			if normalizeStatePanelActorID(actor.ID) == found.ID {
				return character.Profile{}, false
			}
		}
	}
	return found, true
}

func planQuestFollowUp(p *storyBranchProjection, d quest.Definition, prior quest.Instance, exists bool, req QuestRequest) (questPlannedAction, error) {
	fail := func(reason string) (questPlannedAction, error) {
		return questPlannedAction{}, fmt.Errorf("%w: %s", quest.ErrInvalidState, reason)
	}
	if p == nil || !exists {
		return fail("no committed source for this follow-up")
	}
	next := cloneQuestInstance(prior)
	var reaction *quest.EchoReaction
	var english string
	localized := quest.LocalizedDefinition(d, "en-US")
	choiceID := strings.TrimSpace(req.ChoiceID)
	if choiceID == "" && d.CompletionEcho != nil {
		e := next.CompletionEcho
		if e == nil || !prior.SettledRewards || !e.Resolved || e.DueSeconds > p.clockSeconds() || e.AcknowledgedBy != "" {
			return fail("the completed handover is not ready for a new follow-up")
		}
		reaction = &d.CompletionEcho.Reaction
		if localized.CompletionEcho != nil {
			english = localized.CompletionEcho.Reaction.Statement
		}
		// These are committed objective facts, not the player's free-form claim.
		for _, objective := range d.Objectives {
			if prior.SourceRefs[objective.ID] == "" {
				return fail("the completed handover lacks source evidence")
			}
		}
	} else {
		c, ok := d.Choice(choiceID)
		if !ok {
			return fail("follow-up requires the original authored choice")
		}
		r, ok := next.Choices[c.ObjectiveID]
		if !ok || r.ChoiceID != c.ID || !r.EchoResolved || r.EchoDueSeconds > p.clockSeconds() || r.EchoAcknowledgedBy != "" {
			return fail("the chosen outcome is not ready for a new follow-up")
		}
		reaction = c.Reaction(r.Outcome)
		if reaction == nil {
			return fail("this outcome has no authored personal follow-up")
		}
		if translated, ok := localized.Choice(c.ID); ok && translated.Reaction(r.Outcome) != nil {
			english = translated.Reaction(r.Outcome).Statement
		}
	}
	partner, ok := questEchoPartner(p, reaction.CharacterSourceKey)
	if !ok {
		return fail("the original person is not available in the current scene")
	}
	if choiceID == "" {
		next.CompletionEcho.AcknowledgedBy, next.CompletionEcho.AcknowledgedAtSeconds = partner.ID, p.clockSeconds()
	} else {
		c, _ := d.Choice(choiceID)
		r := next.Choices[c.ObjectiveID]
		r.EchoAcknowledgedBy, r.EchoAcknowledgedAtSeconds = partner.ID, p.clockSeconds()
		next.Choices[c.ObjectiveID] = r
	}
	clock := NewStoryClock(storySceneText(p.State, storySceneTimeField))
	if p.Clock != nil {
		clock = *p.Clock
	}
	if diagnostics := quest.ValidateInstance(d, next); len(diagnostics) != 0 {
		return fail(diagnostics[0].Message)
	}
	return questPlannedAction{Definition: d, Action: QuestActionFollowUp, Instance: next, Echo: &questEchoAcknowledgement{Partner: partner, Statement: reaction.Statement, StatementEN: english, Clock: clock}}, nil
}

// The acknowledgement and its directed character facts are in one transaction.
// No affinity score, resource or organization-wide permission is granted here.
// Knowing one's own attested response is not learning the player's secret
// method. The receipt supplies provenance, not an invented confession.
func questEchoCharacterEvent(branchID, parentID, now, turnID string, plan questPlannedAction) *CharacterRecordEvent {
	if plan.Echo == nil {
		return nil
	}
	e := plan.Echo
	id := newID("chr")
	worldTime := storyClockText(e.Clock)
	if worldTime == "" {
		worldTime = "当前"
	}
	translations := map[string]string(nil)
	if e.StatementEN != "" && e.StatementEN != e.Statement {
		translations = map[string]string{"en-US": e.StatementEN}
	}
	return &CharacterRecordEvent{V: schemaVersion, Type: StoryEventTypeCharacterRecord, ID: id, ParentID: parentID, BranchID: branchID, Ts: now, TurnID: turnID, Cursor: id,
		Knowledge: []character.KnowledgeRecord{{ID: id + ":source", CharacterID: e.Partner.ID, Statement: e.Statement, Nature: character.NatureExperienced, Audience: character.AudienceSingle, SourceEventID: parentID, SourceTurnID: turnID, FormalCommitID: id, WorldTime: worldTime}},
		Matters:   []character.Matter{{ID: id + ":attitude", StoryID: e.Partner.StoryID, BranchID: branchID, CharacterID: e.Partner.ID, Kind: character.MatterAttitude, Status: character.MatterOpen, Statement: e.Statement, StatementTranslations: translations, Counterparty: DefaultActorID, SourceEventID: parentID, TurnID: turnID, WorldTime: worldTime}},
	}
}

func questFollowUps(p *storyBranchProjection, d quest.Definition, i quest.Instance) []QuestFollowUp {
	var result []QuestFollowUp
	for _, c := range d.Choices {
		r, ok := i.Choices[c.ObjectiveID]
		if !ok || r.ChoiceID != c.ID || !r.EchoResolved || r.EchoDueSeconds > p.clockSeconds() || r.EchoAcknowledgedBy != "" || c.Reaction(r.Outcome) == nil {
			continue
		}
		if partner, ok := questEchoPartner(p, c.Reaction(r.Outcome).CharacterSourceKey); ok {
			result = append(result, QuestFollowUp{ChoiceID: c.ID, CharacterID: partner.ID})
		}
	}
	if e := i.CompletionEcho; d.CompletionEcho != nil && e != nil && e.Resolved && e.DueSeconds <= p.clockSeconds() && e.AcknowledgedBy == "" {
		if partner, ok := questEchoPartner(p, d.CompletionEcho.Reaction.CharacterSourceKey); ok {
			result = append(result, QuestFollowUp{CharacterID: partner.ID})
		}
	}
	return result
}

// Every due node is emitted at its own formal instant in stable order. This is
// not an independent timer: the owning time transaction appends all these rows.
func questEchoEvents(p *storyBranchProjection, branchID, turnID, parentID, now string, toSeconds int64) ([]any, string) {
	if p == nil {
		return nil, parentID
	}
	type dueEcho struct {
		taskID, objectiveID string
		at                  int64
	}
	var due []dueEcho
	for id, instance := range p.Quests {
		for objective, r := range instance.Choices {
			if !r.EchoResolved && r.EchoDueSeconds <= toSeconds {
				due = append(due, dueEcho{id, objective, r.EchoDueSeconds})
			}
		}
		if e := instance.CompletionEcho; e != nil && !e.Resolved && e.DueSeconds <= toSeconds {
			due = append(due, dueEcho{id, "", e.DueSeconds})
		}
	}
	sort.Slice(due, func(i, j int) bool {
		if due[i].at != due[j].at {
			return due[i].at < due[j].at
		}
		if due[i].taskID != due[j].taskID {
			return due[i].taskID < due[j].taskID
		}
		return due[i].objectiveID < due[j].objectiveID
	})
	instances := cloneQuestInstances(p.Quests)
	var events []any
	head := parentID
	for _, item := range due {
		next := cloneQuestInstance(instances[item.taskID])
		if item.objectiveID == "" {
			next.CompletionEcho.Resolved = true
		} else {
			r := next.Choices[item.objectiveID]
			r.EchoResolved = true
			next.Choices[item.objectiveID] = r
		}
		// The deadline pass runs first. Preserve its final status when emitting
		// the same instance, rather than reviving an expired unfinished task.
		next, _ = next.Expire(toSeconds)
		id := newID("qstrec")
		clock := StoryClock{Version: StoryClockVersion, Seconds: item.at}
		if p.Clock != nil {
			clock.Anchor = p.Clock.Anchor
		}
		events = append(events, QuestRecordEvent{V: schemaVersion, Type: StoryEventTypeQuestRecord, ID: id, ParentID: head, BranchID: branchID, Ts: now, TurnID: turnID, Cursor: id, WorldTime: storyClockText(clock), TaskID: item.taskID, TaskVersion: next.TaskVersion, Action: QuestActionEcho, Instance: next})
		instances[item.taskID] = next
		head = id
	}
	return events, head
}
