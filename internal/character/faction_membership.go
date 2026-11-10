package character

import (
	"fmt"
	"strings"
)

// FactionMembership is the protagonist's known standing in one organization.
// It is a journal fact, not a role or permission directory. Formal membership
// alone grants no command over other people or their property.
type FactionMembership struct {
	ID               string `json:"id"`
	FactionID        string `json:"faction_id"`
	FactionName      string `json:"faction_name"`
	Status           string `json:"status"`
	Role             string `json:"role,omitempty"`
	Statement        string `json:"statement"`
	AuthorityID      string `json:"authority_id,omitempty"`
	SourceExchangeID string `json:"source_exchange_id,omitempty"`
	SourceKind       string `json:"source_kind,omitempty"`
	WorldTime        string `json:"world_time"`
	ClockSeconds     int64  `json:"clock_seconds"`
	TurnID           string `json:"turn_id,omitempty"`
	SourceEventID    string `json:"source_event_id"`
}

func (m FactionMembership) Normalize() FactionMembership {
	m.ID, m.FactionID, m.FactionName = strings.TrimSpace(m.ID), strings.TrimSpace(m.FactionID), strings.TrimSpace(m.FactionName)
	m.Status, m.Role, m.Statement = strings.TrimSpace(m.Status), strings.TrimSpace(m.Role), strings.TrimSpace(m.Statement)
	m.AuthorityID, m.SourceExchangeID = strings.TrimSpace(m.AuthorityID), strings.TrimSpace(m.SourceExchangeID)
	m.SourceKind, m.WorldTime = strings.TrimSpace(m.SourceKind), strings.TrimSpace(m.WorldTime)
	m.TurnID, m.SourceEventID = strings.TrimSpace(m.TurnID), strings.TrimSpace(m.SourceEventID)
	return m
}

func (m FactionMembership) Validate() error {
	m = m.Normalize()
	if m.ID == "" || m.FactionID == "" || m.FactionName == "" || m.Statement == "" || m.WorldTime == "" || m.SourceEventID == "" || m.ClockSeconds < 0 {
		return fmt.Errorf("faction membership requires identity, known source and formal time")
	}
	switch m.Status {
	case "pending", "formal", "temporary", "left":
	default:
		return fmt.Errorf("invalid faction membership status %q", m.Status)
	}
	if m.SourceKind == "identity" {
		if m.Status != "formal" || m.TurnID != "" || m.SourceExchangeID != "" {
			return fmt.Errorf("initial membership must be the confirmed formal identity")
		}
	} else if m.SourceKind == "player" {
		if m.Status != "left" || m.TurnID == "" || m.SourceExchangeID != "" {
			return fmt.Errorf("player membership source only supports actual withdrawal")
		}
	} else if m.SourceKind != "scene" || m.SourceExchangeID == "" || m.TurnID == "" {
		return fmt.Errorf("faction membership requires its actual scene exchange")
	}
	return nil
}

// CurrentFactionMemberships retains each stable organization's latest standing,
// including left and pending, so a projection cannot revive an earlier title.
func CurrentFactionMemberships(history []FactionMembership) []FactionMembership {
	out := make([]FactionMembership, 0)
	positions := map[string]int{}
	for _, raw := range history {
		m := raw.Normalize()
		if m.FactionID == "" {
			continue
		}
		if position, ok := positions[m.FactionID]; ok {
			out[position] = m
		} else {
			positions[m.FactionID] = len(out)
			out = append(out, m)
		}
	}
	return out
}

func (s *State) ApplyFactionMemberships(records []FactionMembership, cursor string) error {
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return err
		}
	}
	for _, raw := range records {
		m := raw.Normalize()
		if err := m.Validate(); err != nil {
			return err
		}
		exists := false
		for _, previous := range s.FactionMemberships {
			if previous.ID == m.ID {
				exists = true
				break
			}
		}
		if !exists {
			s.FactionMemberships = append(s.FactionMemberships, m)
			s.Revision++
		}
	}
	if len(records) > 0 {
		s.Cursor = cursor
	}
	return nil
}
