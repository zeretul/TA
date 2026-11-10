package prompts

import (
	"strings"
	"testing"
)

func TestInteractiveStoryRuntimeContextHidesStoryCheckSettings(t *testing.T) {
	enabled := InteractiveStoryRuntimeContext(InteractiveStoryPromptInput{
		ChoiceCount: 5, RuleChecksEnabled: true, StoryRuleCatalog: "rule catalog",
	})
	for _, want := range []string{
		"## Fixed-rule Checks",
		"Status: enabled",
		"Story Rule Catalog",
	} {
		if !strings.Contains(enabled, want) {
			t.Fatalf("enabled runtime context missing %q:\n%s", want, enabled)
		}
	}
	for _, hidden := range []string{"difficulty shift", "roll modifier"} {
		if strings.Contains(strings.ToLower(enabled), hidden) {
			t.Fatalf("enabled runtime context exposed %q:\n%s", hidden, enabled)
		}
	}

	disabled := InteractiveStoryRuntimeContext(InteractiveStoryPromptInput{
		ChoiceCount: 5, RuleChecksEnabled: false, StoryRuleCatalog: "must stay hidden",
	})
	if !strings.Contains(disabled, "Status: disabled. Do not call prepare_interactive_turn") {
		t.Fatalf("disabled runtime context lacks the tool boundary:\n%s", disabled)
	}
	if strings.Contains(disabled, "must stay hidden") {
		t.Fatalf("disabled runtime context exposed a rule catalog:\n%s", disabled)
	}
}

func TestInteractiveStoryPromptUsesLetterWorldSettlementContract(t *testing.T) {
	instruction := InteractiveStoryRuntimeContext(InteractiveStoryPromptInput{
		ChoiceCount: 5, CommunicationMode: "letter",
	})
	for _, required := range []string{
		"Status: letter-only",
		"Do not submit character_facts.messages or remote character_facts.exchanges",
		"settle that original letter in this same turn",
		"resolves=<the exact original letter matter id>",
		"This linked matter is the sole structured record of the incoming reply",
		"A wait never forces a character to reply",
	} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("interactive story instruction missing %q:\n%s", required, instruction)
		}
	}
	if strings.Contains(instruction, "submit its incoming character_facts.messages record") {
		t.Fatal("letter-only runtime context contradicted its supported Matter settlement channel")
	}
}

func TestInteractiveStoryPromptKeepsMessageRuleOutsideLetterWorld(t *testing.T) {
	instruction := BuildInteractiveStoryFlowInstruction(InteractiveStorySystemInstructionInput{})
	if !strings.Contains(instruction, "When this world's communication rules support incoming messages") {
		t.Fatalf("message rule is no longer scoped to supported communication modes:\n%s", instruction)
	}
}
