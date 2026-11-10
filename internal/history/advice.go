package history

import (
	"strings"
	"unicode/utf8"
)

// SavedAdvice is a player-owned bookmark of a canonical reply, never an order.
// The journal owns its lifecycle; checkpoints restore the saved collection.
type SavedAdvice struct {
	ID         string `json:"id"`
	ChatID     string `json:"chat_id"`
	MinisterID string `json:"minister_id"`
	Text       string `json:"text"`
	Title      string `json:"title"`
	Notes      string `json:"notes"`
	Turn       int    `json:"turn"`
}
type AdviceChange struct {
	Action     string `json:"action"`
	ID         string `json:"id,omitempty"`
	ChatID     string `json:"chat_id,omitempty"`
	MinisterID string `json:"minister_id,omitempty"`
	Title      string `json:"title,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

func applyAdvice(s *State, commandID string, change *AdviceChange) error {
	if change == nil {
		return invalid("advice change missing")
	}
	if utf8.RuneCountInString(change.Title) > 160 || utf8.RuneCountInString(change.Notes) > 2000 {
		return invalid("advice annotation too long")
	}
	switch change.Action {
	case "save":
		for _, advice := range s.Advice {
			if advice.ChatID == change.ChatID && advice.MinisterID == change.MinisterID {
				return nil
			}
		}
		if len(s.Advice) >= 256 {
			return invalid("advice collection capacity exceeded")
		}
		for _, chat := range s.Chats {
			if chat.ID != change.ChatID || discarded(*s, chat.Revision) {
				continue
			}
			for _, reply := range chat.Replies {
				if reply.MinisterID == change.MinisterID {
					s.Advice = append(s.Advice, SavedAdvice{ID: shortID("advice-", commandID), ChatID: chat.ID, MinisterID: reply.MinisterID, Text: reply.Text, Title: strings.TrimSpace(change.Title), Notes: strings.TrimSpace(change.Notes), Turn: chat.Turn})
					return nil
				}
			}
		}
		return invalid("advice source reply unavailable")
	case "update", "remove":
		for i := range s.Advice {
			if s.Advice[i].ID == change.ID {
				if change.Action == "remove" {
					s.Advice = append(s.Advice[:i], s.Advice[i+1:]...)
				} else {
					s.Advice[i].Title = strings.TrimSpace(change.Title)
					s.Advice[i].Notes = strings.TrimSpace(change.Notes)
				}
				return nil
			}
		}
		return invalid("saved advice not found")
	default:
		return invalid("unknown advice operation")
	}
}
