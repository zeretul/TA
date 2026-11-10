package interactive

import (
	"fmt"
	"reflect"
	"strings"

	"ta/internal/character"
)

// CooperationRecordEvent publishes only verified cooperation and deliberately
// shared knowledge on the canonical story journal. Actor resource operations
// belong to the owning Turn.StateDelta and must never be applied here again.
type CooperationRecordEvent struct {
	V           int                         `json:"v"`
	Type        string                      `json:"type"`
	ID          string                      `json:"id"`
	ParentID    string                      `json:"parent_id"`
	BranchID    string                      `json:"branch_id"`
	Ts          string                      `json:"ts"`
	TurnID      string                      `json:"turn_id,omitempty"`
	Cursor      string                      `json:"cursor"`
	Matter      *character.Matter           `json:"matter,omitempty"`
	Knowledge   []character.KnowledgeRecord `json:"knowledge,omitempty"`
	ClockBefore *StoryClock                 `json:"clock_before,omitempty"`
	ClockAfter  *StoryClock                 `json:"clock_after,omitempty"`
}

func validateCooperationRecordEvent(event CooperationRecordEvent) error {
	if event.Type != StoryEventTypeCooperationRecord {
		return fmt.Errorf("invalid cooperation record type %q", event.Type)
	}
	if err := validateStoryEventEnvelope(StoryEventEnvelope{
		V: event.V, Type: event.Type, ID: event.ID, ParentID: event.ParentID, BranchID: event.BranchID, Ts: event.Ts,
	}); err != nil {
		return err
	}
	if strings.TrimSpace(event.Cursor) != strings.TrimSpace(event.ID) {
		return fmt.Errorf("cooperation cursor must name its canonical event")
	}
	if event.Matter == nil && len(event.Knowledge) == 0 {
		return fmt.Errorf("cooperation record requires a matter or shared knowledge")
	}
	worked := false
	if event.Matter != nil {
		matter := event.Matter.Normalize()
		if matter.Cooperation == nil {
			return fmt.Errorf("cooperation record cannot publish an ordinary matter")
		}
		if err := matter.Validate(); err != nil {
			return err
		}
		if matter.BranchID != strings.TrimSpace(event.BranchID) || matter.SourceEventID != strings.TrimSpace(event.ID) || matter.TurnID != strings.TrimSpace(event.TurnID) {
			return fmt.Errorf("cooperation matter does not match its canonical branch or source")
		}
		worked = matter.Cooperation.Phase == character.CooperationWorked
		if worked && matter.Cooperation.WorkSourceRef != strings.TrimSpace(event.ID) {
			return fmt.Errorf("cooperation work source must name its work event")
		}
	}
	seen := make(map[string]bool, len(event.Knowledge))
	for _, knowledge := range event.Knowledge {
		knowledge = knowledge.Normalize()
		if err := knowledge.Validate(); err != nil {
			return err
		}
		if seen[knowledge.ID] {
			return fmt.Errorf("cooperation record repeats knowledge %s", knowledge.ID)
		}
		seen[knowledge.ID] = true
		if knowledge.SourceEventID != strings.TrimSpace(event.ID) || knowledge.FormalCommitID != strings.TrimSpace(event.Cursor) || knowledge.SourceTurnID != strings.TrimSpace(event.TurnID) {
			return fmt.Errorf("cooperation knowledge does not match its canonical source")
		}
		if knowledge.Nature != character.NatureTold || knowledge.Audience != character.AudienceSingle {
			return fmt.Errorf("cooperation knowledge must be explicitly told to its single recipient")
		}
		if event.Matter != nil && knowledge.CharacterID != strings.TrimSpace(event.Matter.CharacterID) {
			return fmt.Errorf("cooperation knowledge names a different partner")
		}
	}
	if worked {
		if event.ClockBefore == nil || event.ClockAfter == nil {
			return fmt.Errorf("cooperation work requires its formal clock interval")
		}
		if err := event.ClockBefore.Validate(); err != nil {
			return err
		}
		if err := event.ClockAfter.Validate(); err != nil {
			return err
		}
		after, err := AdvanceStoryClock(*event.ClockBefore, 600)
		if err != nil || after != *event.ClockAfter {
			return fmt.Errorf("cooperation work must advance the same formal clock by 600 seconds")
		}
	} else if event.ClockBefore != nil || event.ClockAfter != nil {
		return fmt.Errorf("only completed cooperation work may advance its clock")
	}
	return nil
}

// applyCooperationRecordClock is separate because historical clock scans do not
// carry character state. They must not re-execute work or advance phases while
// reconstructing the exact same formal interval.
func applyCooperationRecordClock(branch *storyBranchProjection, event CooperationRecordEvent) error {
	if err := validateCooperationRecordEvent(event); err != nil {
		return err
	}
	if event.ClockAfter == nil {
		return nil
	}
	if branch.Clock == nil {
		if event.ClockBefore.Seconds != 0 {
			return fmt.Errorf("first cooperation work clock must start at zero seconds")
		}
	} else if *branch.Clock != *event.ClockBefore {
		// A Turn wait must not publish this same interval as well. Assigning a
		// final instant is not a license to accept an already-paid interval.
		return fmt.Errorf("cooperation clock before does not match the branch clock")
	}
	branch.Clock = cloneStoryClock(event.ClockAfter)
	return nil
}

func applyCooperationRecordCharacters(state *character.State, event CooperationRecordEvent, storyID string) (*character.State, error) {
	if err := validateCooperationRecordEvent(event); err != nil {
		return nil, err
	}
	if strings.TrimSpace(storyID) == "" {
		return nil, fmt.Errorf("cooperation replay requires the actual story identity")
	}
	if event.Matter != nil && strings.TrimSpace(event.Matter.StoryID) != strings.TrimSpace(storyID) {
		return nil, fmt.Errorf("cooperation matter belongs to a different story")
	}
	next := character.State{Version: character.StateVersion}.Normalize()
	if state != nil {
		next = state.Normalize()
	}
	if event.Matter != nil {
		matters, err := character.ApplyCooperationMatter(next.Matters, *event.Matter)
		if err != nil {
			return nil, err
		}
		next.Matters = matters
	}
	for _, knowledge := range event.Knowledge {
		knowledge = knowledge.Normalize()
		for _, existing := range next.Knowledge {
			if existing.ID == knowledge.ID && !reflect.DeepEqual(existing.Normalize(), knowledge) {
				return nil, fmt.Errorf("cooperation knowledge %s cannot change an existing record", knowledge.ID)
			}
		}
	}
	next.Knowledge = character.ApplyKnowledge(next.Knowledge, event.Knowledge)
	next.Cursor = event.Cursor
	if state == nil || !reflect.DeepEqual(state.Normalize(), next) {
		next.Revision++
	}
	return &next, nil
}

// applyCooperationRecord validates both halves before replacing either one, so
// a malformed phase or stale clock cannot partly publish knowledge or time.
func applyCooperationRecord(branch *storyBranchProjection, event CooperationRecordEvent, storyID string) error {
	characters, err := applyCooperationRecordCharacters(branch.Characters, event, storyID)
	if err != nil {
		return err
	}
	clockOnly := &storyBranchProjection{Clock: cloneStoryClock(branch.Clock)}
	if err := applyCooperationRecordClock(clockOnly, event); err != nil {
		return err
	}
	branch.Characters, branch.Clock = characters, clockOnly.Clock
	return nil
}
