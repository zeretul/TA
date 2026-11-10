package app

import (
	"errors"

	"ta/internal/interactive"
)

var ErrCharacterChatUnavailable = errors.New("this world's contact only supports committed letters and face-to-face scenes")

func storyCommunicationMode(binding *interactive.StoryPackBinding) string {
	if binding == nil || binding.Gameplay == nil {
		return ""
	}
	return binding.Gameplay.Communication
}

// ValidateInteractivePlayerLetter checks the frozen story capability and its
// registered recipient before a model cycle starts. The story-store commit
// repeats these checks at the point it writes the letter matter.
func (a *App) ValidateInteractivePlayerLetter(storyID, branchID string, raw *interactive.PlayerLetterIntent) error {
	if a == nil || raw == nil {
		return nil
	}
	a.mu.RLock()
	store := a.interactive
	a.mu.RUnlock()
	if store == nil {
		return ErrNoWorkspace
	}
	return validateInteractivePlayerLetter(store, storyID, branchID, raw)
}
