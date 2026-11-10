package interactive

import (
	"errors"
	"strings"
)

var (
	ErrPlayerLetterUnavailable          = errors.New("this story does not support letters")
	ErrPlayerLetterRecipientUnavailable = errors.New("letter recipient is not registered in this story")
	ErrInvalidPlayerLetter              = errors.New("letter text or recipient is invalid")
)

// PlayerLetterIntent is the explicit recipient and exact body entered in the
// character window. The app binds it to one main-story turn, and the store
// records it only when that turn commits.
type PlayerLetterIntent struct {
	CharacterID string `json:"character_id"`
	Body        string `json:"body"`
}

func (intent PlayerLetterIntent) Normalize() PlayerLetterIntent {
	intent.CharacterID = strings.TrimSpace(intent.CharacterID)
	intent.Body = strings.TrimSpace(intent.Body)
	return intent
}
