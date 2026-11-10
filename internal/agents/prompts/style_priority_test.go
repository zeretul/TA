package prompts

import (
	"strings"
	"testing"
)

// A style governs how something is told, never what is true. The dialogue and
// combat purposes are separate lean fragments, so the guard that keeps them
// subordinate to character identity and settled results has to be assembled with
// them — otherwise a comic register could make a grave character joke, and a
// heroic one could rewrite an exchange the rules had already decided.
func TestStyleFragmentsCarryThePriorityGuard(t *testing.T) {
	composition, err := ComposeInteractiveStoryInstruction(nil, nil, InteractiveStorySystemInstructionInput{
		StoryTellerID:           "source-drama",
		StoryTellerName:         "Source Drama",
		StoryTellerSystemPrompt: "Write tightly.",
		DialogueStylePrompt:     "DIALOGUE MARKER",
		CombatStylePrompt:       "COMBAT MARKER",
	})
	if err != nil {
		t.Fatalf("compose interactive story instruction: %v", err)
	}

	for _, fragment := range composition.Fragments() {
		if fragment.ID != "interactive_dialogue_style" && fragment.ID != "interactive_combat_style" {
			continue
		}
		if !strings.Contains(fragment.Content, interactiveStylePriorityInstruction) {
			t.Fatalf("fragment %s lost the priority guard:\n%s", fragment.ID, fragment.Content)
		}
	}

	text := composition.Instruction()
	for _, marker := range []string{"DIALOGUE MARKER", "COMBAT MARKER"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("the assembled instruction dropped %q", marker)
		}
	}
	// The guard must appear with each fragment, not once for the pair, so a
	// reader of either section sees the limit before the style text.
	if count := strings.Count(text, interactiveStylePriorityInstruction); count != 2 {
		t.Fatalf("priority guard appears %d times, want one per style fragment", count)
	}
}

// A story with no purpose overrides must not gain the guard: an empty fragment
// is the correct outcome, and adding a guard with nothing to guard would be a
// stray instruction about a style the story is not using.
func TestNoStyleFragmentsMeansNoPriorityGuard(t *testing.T) {
	composition, err := ComposeInteractiveStoryInstruction(nil, nil, InteractiveStorySystemInstructionInput{
		StoryTellerID:           "classic",
		StoryTellerSystemPrompt: "Write plainly.",
	})
	if err != nil {
		t.Fatalf("compose interactive story instruction: %v", err)
	}
	if strings.Contains(composition.Instruction(), interactiveStylePriorityInstruction) {
		t.Fatalf("a story with no style override still carried the priority guard:\n%s", composition.Instruction())
	}
}
