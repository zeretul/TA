package prompts

import (
	"strings"
	"testing"
)

func TestFantasyWorldCreationKeepsFrozenGenreAndOpenOrigins(t *testing.T) {
	for _, mode := range []string{WorldCreationModeGuided, WorldCreationModeAutomatic} {
		text := BuildWorldCreationSystemInstruction(WorldCreationSystemInstructionInput{Mode: mode, GameplayProfile: "fantasy-state-v1; communication=letter"})
		for _, expected := range []string{"原创西幻目录", "出身不锁职业", "starting positions that fit the player's chosen genre"} {
			if !strings.Contains(text, expected) {
				t.Fatalf("missing %q", expected)
			}
		}
		for _, forbidden := range []string{"This world runs on the cultivation state template", "basis` must be exactly one of"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("foreign world instruction %q", forbidden)
			}
		}
	}
}
