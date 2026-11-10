package prompts

import (
	"strings"
	"testing"
)

// TestAttentionRuleStatesAllFourLevels is the design's §4 rule arriving at the
// narrator. 升温／维持／降温／再升温 is a reading of what is already committed, so
// the turn has to be told what raises attention, what holds it, what lowers it,
// and what brings it back — and equally what none of that is allowed to change.
// Without the last part the rule reads as permission to write someone out or to
// re-roll a relationship.
func TestAttentionRuleStatesAllFourLevels(t *testing.T) {
	instruction := BuildInteractiveStoryFlowInstruction(InteractiveStorySystemInstructionInput{ReplyTargetChars: 800, ChoiceCount: 5})
	if !strings.Contains(instruction, interactiveCharacterAttentionInstruction) {
		t.Fatal("the narrator flow instruction carries no attention rule")
	}
	for _, want := range []struct{ needle, why string }{
		{"Raise attention", "what raises attention"},
		{"Keep attention while", "what holds a scene open"},
		{"Lower it", "what lowers attention"},
		{"Raise it again", "what brings someone back"},
		{"not an exit", "lowering attention is not a departure"},
		{"Do not re-introduce them as a stranger", "a return is not a first meeting"},
		{"never a number you track", "no heat field is introduced"},
	} {
		if !strings.Contains(instruction, want.needle) {
			t.Fatalf("the attention rule never says %s (%q)", want.why, want.needle)
		}
	}
}
