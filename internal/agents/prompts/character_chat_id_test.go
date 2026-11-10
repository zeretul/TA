package prompts

import (
	"strings"
	"testing"
)

// TestCharacterChatNamesTheCharactersExactID guards the one field a window reply
// cannot work without.
//
// The prepared context calls this person by their display name only. When the
// model is not told the id it writes for itself, it invents one — a real answer
// once came back naming itself `chen-popo` for a character whose id was
// `陈婆婆` — and every entry it produced was then discarded as belonging to
// somebody else: no feeling was recorded and no thread was opened.
func TestCharacterChatNamesTheCharactersExactID(t *testing.T) {
	instruction := BuildCharacterChatSystemInstruction(CharacterChatSystemInstructionInput{
		OutputLanguage:   "zh-CN",
		ReplyTargetChars: 200,
		CharacterID:      "陈婆婆",
	})
	if !strings.Contains(instruction, "`陈婆婆`") {
		t.Fatalf("the prompt does not name the character's exact id:\n%s", instruction)
	}
	for _, field := range []string{"affinities", "relationships", "matters"} {
		if !strings.Contains(instruction, field) {
			t.Fatalf("the id instruction does not cover `%s`:\n%s", field, instruction)
		}
	}
}

// TestCharacterChatOmitsTheIDWhenItIsUnknown keeps a caller that has no id from
// emitting an empty backtick pair the model would read as an answer.
func TestCharacterChatOmitsTheIDWhenItIsUnknown(t *testing.T) {
	instruction := BuildCharacterChatSystemInstruction(CharacterChatSystemInstructionInput{
		OutputLanguage:   "zh-CN",
		ReplyTargetChars: 200,
	})
	if strings.Contains(instruction, "你的 `character_id` 是") {
		t.Fatalf("the prompt names an id it was never given:\n%s", instruction)
	}
}
