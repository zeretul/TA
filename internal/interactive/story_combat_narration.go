package interactive

import (
	"fmt"
	"strings"
	"time"
)

// This file is where an exchange's settled facts become prose.
//
// §6.6 of the battle design puts the AI after the rules: the round is settled
// first, then the prose is written from what the settlement produced, and both
// are saved together as the official state. §8 requires the prose to be kept
// when it exists and a technical failure to be reported as a failure rather
// than re-fought, so three things are deliberate here:
//
//   - The narration is its own append-only record joined to the round it
//     describes. A record that replaced the round's own record would lose the
//     settled numbers if the model failed, and §8 is explicit that a committed
//     result is not rolled back because its expression failed.
//
//   - It is keyed by the round's event ID, not by round number. Two branches can
//     both be on round 3 of different fights, and a narration that addressed
//     "round 3" would attach to whichever battle was read last.
//
//   - It changes no model context of its own. The settled facts already reached
//     the next turn through the combat record; the prose restates them, and a
//     second context change for the same exchange would make the next turn
//     believe two things happened.

// CombatNarrationEvent is the prose rendering of one settled exchange.
type CombatNarrationEvent struct {
	V        int    `json:"v"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	BranchID string `json:"branch_id"`
	Ts       string `json:"ts"`
	// SessionID names the battle this round belongs to.
	SessionID string `json:"session_id"`
	// RoundEventID is the committed combat record this prose describes. It is the
	// identity that joins prose to settlement, so a retry of the same round finds
	// the prose already written instead of asking for a second rendering.
	RoundEventID string `json:"round_event_id"`
	// Round is carried for display only. It is never the join key.
	Round int `json:"round"`
	// Narrative is the prose itself.
	Narrative string `json:"narrative"`
	// CommandID and CommandHash are the idempotency identity the prose was
	// written under. They are the same command as the round's, so a retry of a
	// round whose prose was already written is answered from what is stored.
	CommandID   string `json:"command_id,omitempty"`
	CommandHash string `json:"command_hash,omitempty"`
	// StyleID names the combat style the prose was written in. §8 requires a hot
	// style switch to affect the next generation and to leave existing prose
	// alone, so the style that produced a passage travels with it.
	StyleID string `json:"style_id,omitempty"`
}

// validateCombatNarrationEvent rejects a narration that could not be joined to
// the round it claims to describe. An unattached narration is prose about
// nothing, and the window would have no round to show it against.
func validateCombatNarrationEvent(event CombatNarrationEvent) error {
	if strings.TrimSpace(event.BranchID) == "" {
		return fmt.Errorf("战斗演绎缺少 branch_id / a combat narration requires a branch")
	}
	if strings.TrimSpace(event.SessionID) == "" {
		return fmt.Errorf("战斗演绎缺少 session_id / a combat narration requires a session")
	}
	if strings.TrimSpace(event.RoundEventID) == "" {
		return fmt.Errorf("战斗演绎缺少所属回合记录 / a combat narration requires its round record")
	}
	if event.Round < 1 {
		return fmt.Errorf("战斗演绎回合数无效: %d / a combat narration needs a positive round", event.Round)
	}
	return nil
}

// CombatNarration is one round's prose as a reader receives it.
type CombatNarration struct {
	RoundEventID string `json:"round_event_id"`
	Round        int    `json:"round"`
	Narrative    string `json:"narrative"`
	StyleID      string `json:"style_id,omitempty"`
	EventID      string `json:"event_id,omitempty"`
}

// AppendCombatNarrationRequest is one passage to commit.
type AppendCombatNarrationRequest struct {
	BranchID string
	// SessionID is the battle the round belongs to.
	SessionID string
	// RoundEventID is the committed combat record this prose describes. The
	// append is refused when it names no round on this branch, because prose
	// attached to a round that does not exist would be shown against nothing.
	RoundEventID string
	Narrative    string
	StyleID      string
	CommandID    string
	CommandHash  string
}

// AppendCombatNarration commits one round's prose as an append-only side record.
//
// It deliberately does not advance the branch head. §8 requires existing prose
// and settled numbers to survive a style switch, and the head is the position
// the next turn is parented at: moving it for a passage of description would
// make the story's own commit position depend on whether a model call succeeded.
// The record is joined to its round by RoundEventID instead, which is stable
// whatever the head is doing.
func (s *Store) AppendCombatNarration(storyID string, req AppendCombatNarrationRequest) (CombatNarration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return CombatNarration{}, err
	}
	defer releaseStory()

	meta, records, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return CombatNarration{}, err
	}
	branchID := strings.TrimSpace(req.BranchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	if _, ok := meta.Branches[branchID]; !ok {
		return CombatNarration{}, fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return CombatNarration{}, fmt.Errorf("战斗演绎缺少 session_id / a combat narration requires a session")
	}
	roundEventID := strings.TrimSpace(req.RoundEventID)
	round, found := combatRoundForNarration(records, branchID, sessionID, roundEventID)
	if !found {
		return CombatNarration{}, fmt.Errorf(
			"%w: session=%s round=%s / the round this narration describes is not committed on this branch",
			ErrCombatNarrationUnattached, sessionID, roundEventID,
		)
	}
	commandID := strings.TrimSpace(req.CommandID)
	commandHash := strings.TrimSpace(req.CommandHash)
	// A retry must not buy a second rendering. The committed passage is the
	// durable answer, so it is consulted before anything is written: a lost
	// response or a repeated click returns the prose the player already has
	// rather than a differently-worded second version of the same exchange.
	if commandID != "" {
		existing, found, err := combatNarrationForCommand(records, branchID, commandID, commandHash)
		if err != nil {
			return CombatNarration{}, err
		}
		if found {
			return existing, nil
		}
	}
	narrative := strings.TrimSpace(req.Narrative)
	if narrative == "" {
		// An empty passage is not committed. §8 requires a failure to be reported
		// rather than stored, and a stored empty passage would read as a round
		// that had no description when it in fact had none written.
		return CombatNarration{}, fmt.Errorf("%w: session=%s round=%s", ErrCombatNarrationEmpty, sessionID, roundEventID)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	eventID := newID("cbn")
	event := CombatNarrationEvent{
		V: schemaVersion, Type: StoryEventTypeCombatNarration, ID: eventID,
		ParentID: roundEventID, BranchID: branchID, Ts: now,
		SessionID: sessionID, RoundEventID: roundEventID, Round: round,
		Narrative: trimBytes(narrative, maxInteractiveTextBytes),
		CommandID: commandID, CommandHash: commandHash,
		StyleID: strings.TrimSpace(req.StyleID),
	}
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return CombatNarration{}, err
	}
	return CombatNarration{
		RoundEventID: roundEventID, Round: round,
		Narrative: event.Narrative, StyleID: event.StyleID, EventID: eventID,
	}, nil
}

// CombatNarrations returns one battle's committed prose, keyed by the round
// record each passage describes.
//
// It reads the branch's own records rather than a process-local cache, so the
// prose a player sees after a restart is the prose that was committed.
func (s *Store) CombatNarrations(storyID, branchID, sessionID string) (map[string]CombatNarration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return nil, err
	}
	defer release()
	meta, records, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(branchID) == "" {
		branchID = meta.CurrentBranch
	}
	return combatNarrationsFor(records, strings.TrimSpace(branchID), strings.TrimSpace(sessionID)), nil
}

// combatNarrationsFor collects one battle's prose.
//
// A passage whose round record is not on this branch is skipped: a narration a
// regenerated turn left behind describes a fight the branch no longer contains,
// and showing it would put prose about a replaced exchange in front of the
// player.
func combatNarrationsFor(records []StoryEventRecord, branchID, sessionID string) map[string]CombatNarration {
	if sessionID == "" {
		return nil
	}
	roundIDs := make(map[string]bool)
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeCombatRecord || record.Envelope.BranchID != branchID {
			continue
		}
		var event CombatRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			continue
		}
		if event.Session.ID != sessionID {
			continue
		}
		// The opening record is the encounter, not an exchange, so it is not a
		// round a passage can be attached to.
		if !combatRecordSettlesExchange(event.Outcome) {
			continue
		}
		roundIDs[event.ID] = true
	}
	narrations := make(map[string]CombatNarration)
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeCombatNarration || record.Envelope.BranchID != branchID {
			continue
		}
		var event CombatNarrationEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			continue
		}
		if event.SessionID != sessionID || !roundIDs[event.RoundEventID] {
			continue
		}
		// A later passage for the same round replaces an earlier one, so a
		// deliberately re-rendered round shows the version the player asked for
		// last rather than the first attempt.
		narrations[event.RoundEventID] = CombatNarration{
			RoundEventID: event.RoundEventID, Round: event.Round,
			Narrative: event.Narrative, StyleID: event.StyleID, EventID: event.ID,
		}
	}
	return narrations
}

// combatRoundForNarration finds the committed round a passage describes and
// returns its round number.
//
// An opening record answers false: it names the encounter, and prose written
// against it would describe an exchange that was never played.
func combatRoundForNarration(records []StoryEventRecord, branchID, sessionID, roundEventID string) (int, bool) {
	if roundEventID == "" {
		return 0, false
	}
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeCombatRecord || record.Envelope.ID != roundEventID {
			continue
		}
		var event CombatRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return 0, false
		}
		if event.BranchID != branchID || event.Session.ID != sessionID {
			return 0, false
		}
		if !combatRecordSettlesExchange(event.Outcome) {
			return 0, false
		}
		return event.Outcome.Round, true
	}
	return 0, false
}

// combatNarrationForCommand looks for the passage a previous attempt committed
// under this command identity, so a retry is a read rather than a second
// rendering. A found command ID with a different hash is a conflict: the caller
// reused one identity for two different passages, and either answer would put
// prose against the wrong round.
func combatNarrationForCommand(records []StoryEventRecord, branchID, commandID, commandHash string) (CombatNarration, bool, error) {
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeCombatNarration {
			continue
		}
		var event CombatNarrationEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return CombatNarration{}, false, fmt.Errorf("decode committed combat narration: %w", err)
		}
		if event.BranchID != branchID || strings.TrimSpace(event.CommandID) != commandID {
			continue
		}
		if strings.TrimSpace(event.CommandHash) != commandHash {
			return CombatNarration{}, false, fmt.Errorf(
				"%w: command_id=%q existing_hash=%q requested_hash=%q",
				ErrCombatCommandConflict, commandID, event.CommandHash, commandHash,
			)
		}
		return CombatNarration{
			RoundEventID: event.RoundEventID, Round: event.Round,
			Narrative: event.Narrative, StyleID: event.StyleID, EventID: event.ID,
		}, true, nil
	}
	return CombatNarration{}, false, nil
}

// CombatNarrationForCommand returns what a previous attempt committed under this
// command identity, so a caller can answer a retry without generating.
func (s *Store) CombatNarrationForCommand(storyID, branchID, commandID, commandHash string) (CombatNarration, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return CombatNarration{}, false, err
	}
	defer release()
	meta, records, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return CombatNarration{}, false, err
	}
	if strings.TrimSpace(branchID) == "" {
		branchID = meta.CurrentBranch
	}
	return combatNarrationForCommand(records, strings.TrimSpace(branchID), strings.TrimSpace(commandID), strings.TrimSpace(commandHash))
}
