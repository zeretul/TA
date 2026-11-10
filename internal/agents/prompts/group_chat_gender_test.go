package prompts

import (
	"strings"
	"testing"
)

// TestGroupRoundStatesGenderWithoutMakingItAVoice is the group half of 不以性别套
// 固定性格. The roster carries who each person is, so the round cannot quietly
// re-decide it — but a gender line the model reads as a manner of speech would
// make every man in the group sound alike, which is the failure the design
// names. The line therefore arrives with the voice, and with the warning.
func TestGroupRoundStatesGenderWithoutMakingItAVoice(t *testing.T) {
	instruction := BuildGroupChatSystemInstruction(GroupChatSystemInstructionInput{
		GroupName: "药铺后堂",
		Members: []GroupChatMember{{
			ID: "shopkeeper", Name: "周氏", Role: "药铺掌柜",
			Gender: GenderForTest, SpeechStyle: "说话慢，先问用途再报价", CanSpeak: true,
		}},
	})
	if !strings.Contains(instruction, "- Gender: female") {
		t.Fatalf("the roster does not state the settled gender:\n%s", instruction)
	}
	if !strings.Contains(instruction, "do not read this as a temperament") {
		t.Fatalf("the gender line arrives with no warning against stereotyping:\n%s", instruction)
	}
	if !strings.Contains(instruction, "先问用途再报价") {
		t.Fatalf("the person's own voice is missing from the same block:\n%s", instruction)
	}
}

// TestGroupRoundSaysNothingAboutAnUnsettledGender keeps the roster from printing
// a blank field the model would then fill in.
func TestGroupRoundSaysNothingAboutAnUnsettledGender(t *testing.T) {
	instruction := BuildGroupChatSystemInstruction(GroupChatSystemInstructionInput{
		GroupName: "药铺后堂",
		Members:   []GroupChatMember{{ID: "x", Name: "某人", CanSpeak: true}},
	})
	if strings.Contains(instruction, "Gender") {
		t.Fatalf("a member with no settled gender still got a gender line:\n%s", instruction)
	}
}

// GenderForTest is the value the character package's closed vocabulary stores.
// The prompts package does not import it, so the literal is repeated here rather
// than coupling two packages that otherwise do not know each other.
const GenderForTest = "female"
