package prompts

import (
	"strings"
	"testing"

	"ta/config"
)

func TestEnemyDecisionUsesStructuredProtocolWithoutReplacingStoryProse(t *testing.T) {
	decision, err := ComposeBuiltinSystemInstruction(&config.Config{}, config.AgentKindInteractiveStory, "enemy_decision", "", "enemy_decision", "Enemy decision", "Select a legal candidate", "DECISION RULES")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(decision.Instruction(), "Output only the JSON object") || strings.Contains(decision.Instruction(), "Output only the story prose") {
		t.Fatalf("decision protocol contradicts JSON selection: %s", decision.Instruction())
	}
	story, err := ComposeBuiltinSystemInstruction(&config.Config{}, config.AgentKindInteractiveStory, "story", "", "story", "Story", "Narrate", "STORY RULES")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(story.Instruction(), "Output only the story prose") {
		t.Fatal("adding enemy decisions replaced the normal story protocol")
	}
}
