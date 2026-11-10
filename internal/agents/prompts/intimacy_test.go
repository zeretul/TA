package prompts

import (
	"strings"
	"testing"
)

// TestIntimacyInvitationDoesNotClaimConsent is §3.1's "不能强制同意" at the prompt
// level. The invitation is the question; a prompt that opens by telling the
// person everyone has already agreed has answered it for them.
func TestIntimacyInvitationDoesNotClaimConsent(t *testing.T) {
	instruction := BuildIntimacyInvitationInstruction(IntimacyInvitationInput{})

	if strings.Contains(instruction, "everyone in it has agreed") {
		t.Fatalf("the invitation tells the invited person the answer is already given:\n%s", instruction)
	}
	if !strings.Contains(instruction, "whether you agree is yours to answer") {
		t.Fatalf("the invitation does not leave the answer with the person:\n%s", instruction)
	}
	// The answer is still the output contract, not something the prompt assumes.
	if !strings.Contains(instruction, `"status":"accepted|declined|conditional"`) {
		t.Fatalf("the invitation lost its answer contract:\n%s", instruction)
	}
}

// TestIntimacyRoundStatesTheRosterConsented is the other half: a round only
// exists after a confirmed roster, so there the agreement is a fact and must be
// stated as one.
func TestIntimacyRoundStatesTheRosterConsented(t *testing.T) {
	instruction := BuildIntimacyRoundInstruction(IntimacyRoundInput{
		Members: []GroupChatMember{{ID: "lin", Name: "林晚"}},
	})

	if !strings.Contains(instruction, "everyone in it has agreed to be here") {
		t.Fatalf("the round does not state that the roster agreed:\n%s", instruction)
	}
}

// TestIntimacyPromptsKeepTheClosedGainRule is §5: no cultivation, recovery,
// spirit or breakthrough gain exists in this window. Both prompts must say so,
// because a model that is never told will invent one.
func TestIntimacyPromptsKeepTheClosedGainRule(t *testing.T) {
	for name, instruction := range map[string]string{
		"invitation": BuildIntimacyInvitationInstruction(IntimacyInvitationInput{}),
		"round":      BuildIntimacyRoundInstruction(IntimacyRoundInput{}),
	} {
		if !strings.Contains(instruction, "Nothing here grants cultivation progress") {
			t.Fatalf("%s prompt lost the no-gain rule:\n%s", name, instruction)
		}
		if !strings.Contains(instruction, "Nobody outside this room knows it happened") {
			t.Fatalf("%s prompt lost the privacy rule:\n%s", name, instruction)
		}
	}
}
