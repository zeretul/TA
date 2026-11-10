package interactive

import (
	"strings"

	"ta/internal/cultivation"
)

// This file derives what the main story has not yet been told about a battle.
//
// A battle is fought in its own window and committed as a side record on the
// canonical journal. That record advances the branch head, so the journal knows
// the fight happened — but nothing renders it into the model's prompt, and the
// next main-story turn would otherwise narrate as though no fight had taken
// place. §2 of the handoff design names the failure exactly: the main story must
// not put a fled enemy back on the road at full health.
//
// The derivation is a property of the journal rather than a remembered flag. A
// battle is pending exactly when its records sit on the branch's active ancestry
// *after* the last Turn. Once a Turn absorbs it, that Turn becomes the latest
// Turn on the path and the battle stops being pending — so "settle exactly once"
// is structural, in the same way pendingPlayerInputsForBranch makes a player
// input consumed exactly once. Nothing has to be written down, and a restart
// re-derives the same answer from the same records.

// PendingCombatHandoff is one battle the main story has not yet absorbed.
type PendingCombatHandoff struct {
	// SessionID is the battle's identity in the journal.
	SessionID string
	// EventID is the committed record this was read from, so a caller can name
	// the exact position in the journal the handoff was prepared at.
	EventID string
	// WorldTime is the in-fiction time of the last committed exchange.
	WorldTime string
	// Session is the battle as of that record.
	Session cultivation.CombatSession
	// Outcome is the exchange that record settled.
	Outcome cultivation.ExchangeOutcome
	// Settled reports whether the battle has reached a result. An unsettled
	// battle is still live: it is handed over so the main story does not restage
	// it, not because the player has finished with it.
	Settled bool
}

// PendingCombatHandoffs returns the branch's battles the main story has not yet
// absorbed, oldest first.
//
// It reads the branch's own records rather than a process-local index, so the
// answer after a restart is the answer the journal supports.
func (s *Store) PendingCombatHandoffs(storyID, branchID string) ([]PendingCombatHandoff, error) {
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
	branch, ok := meta.Branches[branchID]
	if !ok {
		return nil, nil
	}
	return pendingCombatHandoffsFor(branch.Head, records), nil
}

// pendingCombatHandoffsFor is the walk itself, over one branch's records.
//
// The last Turn on the active path is the boundary. A battle recorded after it
// has happened since the story last moved, so the story has not seen it; a
// battle recorded before it was already in front of the narrator when that Turn
// was written, and repeating it would restate what the story already absorbed.
func pendingCombatHandoffsFor(head string, records []StoryEventRecord) []PendingCombatHandoff {
	path, _, err := walkEventPath(head, eventsByID(records))
	if err != nil {
		// A damaged parent chain has no reliable boundary to derive from. The
		// journal layer reports the damage through its own reads; handing back
		// nothing here is the honest answer rather than guessing a boundary from
		// a path that cannot be read.
		return nil
	}
	lastTurn := -1
	for index, record := range path {
		if record.Envelope.Type == StoryEventTypeTurn {
			lastTurn = index
		}
	}
	// A battle may commit many records — one per exchange — so the last record
	// per session is the battle's current state. Earlier records of the same
	// fight are its own history, not separate battles.
	latest := make(map[string]int)
	order := make([]string, 0)
	for index := lastTurn + 1; index < len(path); index++ {
		record := path[index]
		if record.Envelope.Type != StoryEventTypeCombatRecord {
			continue
		}
		var event CombatRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			continue
		}
		sessionID := strings.TrimSpace(event.Session.ID)
		if sessionID == "" {
			continue
		}
		if _, seen := latest[sessionID]; !seen {
			order = append(order, sessionID)
		}
		latest[sessionID] = index
	}
	handoffs := make([]PendingCombatHandoff, 0, len(order))
	for _, sessionID := range order {
		record := path[latest[sessionID]]
		var event CombatRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			continue
		}
		handoffs = append(handoffs, PendingCombatHandoff{
			SessionID: sessionID,
			EventID:   record.Envelope.ID,
			WorldTime: strings.TrimSpace(event.WorldTime),
			Session:   event.Session,
			Outcome:   event.Outcome,
			Settled:   event.Session.Ended,
		})
	}
	return handoffs
}
