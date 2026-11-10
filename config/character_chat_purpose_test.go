package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// §T24 puts a private conversation's model and thinking level in the system
// settings' model-purpose area. The purpose has no Agent behind it, so the two
// things that could silently break are where it is written and where it is
// read back from.
func TestCharacterChatPurposeIsItsOwnModelSetting(t *testing.T) {
	if _, ok := LookupAgentKind(AgentKindCharacterChat); ok {
		t.Fatal("character_chat must not be registered as a runnable Agent kind: it has no session, tools or prompt contract")
	}

	path, err := AgentProfilePath(AgentKindCharacterChat)
	if err != nil {
		t.Fatalf("resolve character_chat profile path: %v", err)
	}
	if path != "main/game.toml" {
		t.Fatalf("character_chat writes %q, want main/game.toml", path)
	}

	settings := AgentModelSettings{
		InteractiveStory: AgentModelOverride{ThinkingLevel: "low"},
		CharacterChat:    AgentModelOverride{ProfileID: "gemini-3-8", ThinkingLevel: "medium"},
	}
	if got := agentModelOverrideFor(settings, AgentKindCharacterChat); got != settings.CharacterChat {
		t.Fatalf("character_chat resolves to %+v, want its own override", got)
	}
	if got := agentModelOverrideFor(settings, AgentKindInteractiveStory); got != settings.InteractiveStory {
		t.Fatalf("interactive_story resolves to %+v, want its own override", got)
	}
}

// An unset level must send "off" rather than the provider default. The provider
// default omits the control, which would let a gateway decide how much a
// character reasons before answering — and this purpose has an opinion.
func TestCharacterChatThinkingLevelDefaultsToOff(t *testing.T) {
	if got := CharacterChatThinkingLevel(AgentModelSettings{}); got != CharacterChatThinkingDefault {
		t.Fatalf("unset level = %q, want %q", got, CharacterChatThinkingDefault)
	}
	explicit := AgentModelSettings{CharacterChat: AgentModelOverride{ThinkingLevel: "medium"}}
	if got := CharacterChatThinkingLevel(explicit); got != "medium" {
		t.Fatalf("explicit level = %q, want medium", got)
	}
	// A level chosen for the main story must not leak into this purpose.
	story := AgentModelSettings{InteractiveStory: AgentModelOverride{ThinkingLevel: "high"}}
	if got := CharacterChatThinkingLevel(story); got != CharacterChatThinkingDefault {
		t.Fatalf("main story's level leaked into character chat: %q", got)
	}
}

func TestCharacterChatPurposeRoundTripsThroughGameProfile(t *testing.T) {
	dataDir := t.TempDir()
	if err := EnsureAgentProfiles(dataDir); err != nil {
		t.Fatalf("seed Agent Profiles: %v", err)
	}
	settings := Settings{
		AgentModels: AgentModelSettings{
			InteractiveStory: AgentModelOverride{ThinkingLevel: "low"},
			CharacterChat:    AgentModelOverride{ProfileID: "gemini-3-8", ThinkingLevel: "medium"},
		},
	}
	if err := writeAgentProfileSettingsLocked(dataDir, settings); err != nil {
		t.Fatalf("write profiles: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, "agents", "main", "game.toml"))
	if err != nil {
		t.Fatalf("read game.toml: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "[character_chat]") {
		t.Fatalf("game.toml does not carry a character_chat block:\n%s", body)
	}
	if !strings.Contains(body, "gemini-3-8") {
		t.Fatalf("game.toml lost the character_chat profile:\n%s", body)
	}

	loaded, ready, err := LoadAgentProfileSettings(dataDir)
	if err != nil {
		t.Fatalf("read profiles back: %v", err)
	}
	if !ready {
		t.Fatal("Agent Profile marker missing after write")
	}
	if loaded.AgentModels.CharacterChat.ProfileID != "gemini-3-8" {
		t.Fatalf("character_chat profile = %q, want gemini-3-8", loaded.AgentModels.CharacterChat.ProfileID)
	}
	if loaded.AgentModels.CharacterChat.ThinkingLevel != "medium" {
		t.Fatalf("character_chat level = %q, want medium", loaded.AgentModels.CharacterChat.ThinkingLevel)
	}
	if loaded.AgentModels.InteractiveStory.ThinkingLevel != "low" {
		t.Fatalf("interactive_story level = %q, want low", loaded.AgentModels.InteractiveStory.ThinkingLevel)
	}
}
