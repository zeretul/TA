package interactive

import (
	"errors"
	"strings"

	"ta/agent/providers"
)

// This file holds the story's intimacy-purpose model binding.
//
// The intimacy window is not a second model system: it is one more purpose
// bound to an existing Endpoint/ModelProfile, exactly the way the character
// window binds its thinking level. What it must not be is a copy of the main
// story's route. A player who selects a different profile here expects that
// profile to serve these exchanges and the main story to keep its own, so the
// binding is stored per story and resolved on its own.
//
// The settings carry no credential. A ProfileID names a profile the existing
// local configuration already holds; keys stay in that configuration and are
// never copied into the story journal, a world pack, an export or a log.

// ErrIntimacyModelProfile means the bound profile does not exist. The window
// keeps its history and asks the player to choose again; §8 of the design
// forbids silently falling back to the main story's model.
var ErrIntimacyModelProfile = errors.New("invalid intimacy model profile")

// ErrIntimacyThinkingLevel means the saved thinking level is not one this build
// understands.
var ErrIntimacyThinkingLevel = errors.New("invalid intimacy thinking level")

// ErrIntimacyModelUnconfigured means no profile has been bound yet. It is
// separate from ErrIntimacyModelProfile because "you have not chosen one" and
// "the one you chose is gone" are different things to tell the player.
var ErrIntimacyModelUnconfigured = errors.New("intimacy model is not configured")

// IntimacyThinkingDefault is the default reasoning setting. §4.3 of the design
// keeps real thinking off unless a protocol is verified to support it, and an
// unset story is the unverified case.
const IntimacyThinkingDefault = providers.ThinkingLevelOff

// StoryIntimacySettings is the story's intimacy-purpose model binding.
type StoryIntimacySettings struct {
	// ProfileID names an existing model profile. It is deliberately not a copy
	// of that profile: editing the profile changes this purpose too, which is
	// what the existing settings UI already tells the player.
	ProfileID string `json:"profile_id,omitempty"`
	// ThinkingLevel is the saved choice. It applies to the next generation.
	ThinkingLevel string `json:"thinking_level,omitempty"`
}

// Configured reports whether a profile has been chosen. History stays readable
// when it has not; only generation is blocked.
func (settings StoryIntimacySettings) Configured() bool {
	return settings.ProfileID != ""
}

func normalizeStoryIntimacySettings(settings StoryIntimacySettings) StoryIntimacySettings {
	settings.ProfileID = strings.TrimSpace(settings.ProfileID)
	if settings.ThinkingLevel == "" {
		settings.ThinkingLevel = string(IntimacyThinkingDefault)
		return settings
	}
	settings.ThinkingLevel = string(providers.NormalizeThinkingLevel(settings.ThinkingLevel))
	return settings
}

// ResolvedThinkingLevel is the level to send. An empty or unrecognized value
// resolves to the default rather than being forwarded as-is.
func (settings StoryIntimacySettings) ResolvedThinkingLevel() providers.ThinkingLevel {
	level := providers.NormalizeThinkingLevel(settings.ThinkingLevel)
	if level == providers.ThinkingLevelDefault {
		return IntimacyThinkingDefault
	}
	return level
}

// validateStoryIntimacySettings checks the shape of the binding only. Whether
// the named profile exists is a question for the layer that holds the config;
// this package must not read credentials to answer it.
func validateStoryIntimacySettings(settings StoryIntimacySettings) error {
	if settings.ThinkingLevel == "" {
		return nil
	}
	if _, err := providers.ParseThinkingLevel(settings.ThinkingLevel); err != nil {
		return ErrIntimacyThinkingLevel
	}
	return nil
}
