package prompts

import (
	"strings"
	"testing"
	"unicode"
)

// The model-facing instructions in this package are written in Chinese, while
// the prose the model produces follows the page. These tests pin both halves:
// the instruction itself carries no English sentence, and the page's language
// still decides what the player reads and in which unit the length is counted.

// assertNoEnglishSentence fails on any clause of text that is written entirely in
// Latin words. The check runs per clause rather than per line because a Chinese
// instruction may legitimately name a field, an id or a protocol value in Latin
// in the middle of a Chinese sentence; what it may not do is contain a whole
// English sentence, and a line-level check would miss one glued onto a Chinese
// line.
func assertNoEnglishSentence(t *testing.T, name, text string) {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, clause := range splitClauses(trimmed) {
			// Code identifiers, ids, keys and protocol values are allowed to be
			// Latin; what must not happen is a sentence written in English.
			words := 0
			for _, field := range strings.FieldsFunc(clause, func(r rune) bool {
				return unicode.IsSpace(r) || strings.ContainsRune("`*_()[]{}:,.;\"'—-", r)
			}) {
				if hasHan(field) {
					continue
				}
				if len(field) > 2 && isLatinWord(field) {
					words++
				}
			}
			if words >= 4 && !hasHan(clause) {
				t.Fatalf("%s carries an English-only clause: %q", name, clause)
			}
		}
	}
}

// splitClauses cuts a line at the punctuation that ends a thought, in either
// script, so each clause can be judged on its own.
func splitClauses(line string) []string {
	return strings.FieldsFunc(line, func(r rune) bool {
		return strings.ContainsRune("。；！？，、.;!?,\t", r)
	})
}

func hasHan(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0
}

func isLatinWord(value string) bool {
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

func TestThePlayerFacingLanguageLineIsWrittenInChinese(t *testing.T) {
	line := playerFacingLanguageLine("zh-CN", "下方的故事材料")
	if !strings.Contains(line, "简体中文") {
		t.Fatalf("a Chinese page must name Chinese, got %q", line)
	}
	if !hasHan(line) {
		t.Fatalf("the instruction itself must be Chinese, got %q", line)
	}
	english := playerFacingLanguageLine("en-US", "下方的故事材料")
	if !strings.Contains(english, "英文") {
		t.Fatalf("an English page must still ask for English prose, got %q", english)
	}
	if !strings.Contains(english, "id、字段名和协议取值原样保留") {
		t.Fatalf("the instruction must still keep ids and protocol values untouched, got %q", english)
	}
}

func TestALocaleThatNamesNoLanguageFollowsTheMaterialInChinese(t *testing.T) {
	line := playerFacingLanguageLine("", "下方的故事材料")
	if !strings.Contains(line, "与下方的故事材料相同的语言") {
		t.Fatalf("an unnamed locale must follow the material, got %q", line)
	}
}

// A live English-page turn came back with English narration and every spoken
// line still in Chinese: the rule named prose, and quoted dialogue was read as
// material to reproduce rather than as prose to write. The line has to say that
// speech is prose too, and it has to keep the established-spelling clause from
// being read as permission to leave dialogue untranslated.
func TestTheLanguageLineCoversSpokenDialogue(t *testing.T) {
	for _, locale := range []string{"en-US", "zh-CN", ""} {
		line := playerFacingLanguageLine(locale, "下方的故事材料")
		if !strings.Contains(line, "对白") {
			t.Fatalf("locale %q: the language line does not name dialogue: %q", locale, line)
		}
	}
	english := playerFacingLanguageLine("en-US", "下方的故事材料")
	if !strings.Contains(english, "英文") {
		t.Fatalf("an English page must still ask for English prose, got %q", english)
	}
	// The clause that keeps names and terms in the story's spelling must not
	// cover speech, or it reintroduces the bug it is next to.
	if strings.Contains(english, "称号和术语沿用故事给出的写法，人物") {
		t.Fatalf("the established-spelling clause swallowed dialogue, got %q", english)
	}
}

func TestTheLanguageHeadingIsChinese(t *testing.T) {
	var sb strings.Builder
	writeLanguageFirst(&sb, "zh-CN", "下方的世界材料", "本轮散文与选项文字同样遵守。")
	built := sb.String()
	if !strings.HasPrefix(built, "# 语言\n\n") {
		t.Fatalf("the language block must open with a Chinese heading, got %q", built)
	}
	if strings.Contains(built, "# Language") {
		t.Fatal("the language block still carries the English heading")
	}
	assertNoEnglishSentence(t, "the language block", built)
}

func TestLengthTargetsCountInThePageLanguageUnit(t *testing.T) {
	if got := proseLengthTarget("zh-CN", 800); got != "800 个汉字" {
		t.Fatalf("a Chinese page counts in Han characters, got %q", got)
	}
	if got := proseLengthTarget("", 800); got != "800 个汉字" {
		t.Fatalf("the default page counts in Han characters, got %q", got)
	}
	english := proseLengthTarget("en-US", 800)
	if english != "约 520 个英文单词" {
		t.Fatalf("an English page counts in English words, got %q", english)
	}
	if strings.Contains(english, "汉字") {
		t.Fatalf("an English page must not be given a Han-character target, got %q", english)
	}
	if got := proseLengthTarget("en-US", 10); got != "约 20 个英文单词" {
		t.Fatalf("a very short target must keep the floor, got %q", got)
	}
	if got := proseLengthTarget("zh-CN", 0); got != "" {
		t.Fatalf("a zero target must render nothing, got %q", got)
	}
}

func TestTheInteractivePromptStatesItsLanguageOnce(t *testing.T) {
	english := BuildInteractiveStorySystemInstruction(InteractiveStorySystemInstructionInput{
		OutputLanguage:   "en-US",
		ReplyTargetChars: 800,
		ChoiceCount:      3,
	})
	if strings.Count(english, "所有面向玩家的散文都用") != 2 {
		t.Fatalf("the language rule belongs at the top and in the output protocol, no more:\n%s", english)
	}
	if !strings.Contains(english, "约 520 个英文单词") {
		t.Fatalf("an English page must be given an English word target:\n%s", english)
	}
	if strings.Contains(english, "个汉字") {
		t.Fatalf("an English page must not be given a Han-character target:\n%s", english)
	}

	chinese := BuildInteractiveStorySystemInstruction(InteractiveStorySystemInstructionInput{
		OutputLanguage:   "zh-CN",
		ReplyTargetChars: 800,
		ChoiceCount:      3,
	})
	if !strings.Contains(chinese, "所有面向玩家的散文都用简体中文书写") {
		t.Fatalf("a Chinese page must be told to write Chinese:\n%s", chinese)
	}
	if !strings.Contains(chinese, "800 个汉字") {
		t.Fatalf("a Chinese page must be given a Han-character target:\n%s", chinese)
	}
	if strings.Contains(chinese, "个英文单词") {
		t.Fatalf("a Chinese page must not be given an English word target:\n%s", chinese)
	}

	// The retired English wording of the two lines this batch rewrote must be
	// gone; the prompt's untouched English sections are out of scope here.
	for _, retired := range []string{
		"Write all player-facing prose in English",
		"Write all player-facing prose in the same language as the story material",
		"[Length Target] The target length of a normal interactive turn",
		"Stop the turn at a real conversation with someone present",
	} {
		if strings.Contains(chinese, retired) || strings.Contains(english, retired) {
			t.Fatalf("the prompt still carries the retired English wording %q", retired)
		}
	}
}

func TestTheChatPromptsStateTheirLanguageInChinese(t *testing.T) {
	chat := BuildCharacterChatSystemInstruction(CharacterChatSystemInstructionInput{
		OutputLanguage:   "en-US",
		ReplyTargetChars: 120,
	})
	if !strings.Contains(chat, "# 语言") || !strings.Contains(chat, "英文") {
		t.Fatalf("the character chat prompt does not state its language in Chinese:\n%s", chat)
	}
	group := BuildGroupChatSystemInstruction(GroupChatSystemInstructionInput{
		OutputLanguage:   "zh-CN",
		ReplyTargetChars: 120,
	})
	if !strings.Contains(group, "简体中文") {
		t.Fatalf("the group chat prompt does not name Chinese:\n%s", group)
	}
	combat := BuildCombatNarrationSystemInstruction(CombatNarrationSystemInstructionInput{
		OutputLanguage:       "zh-CN",
		NarrativeTargetChars: 450,
	})
	if !strings.Contains(combat, "450 个汉字") {
		t.Fatalf("the combat narration prompt does not count in Han characters:\n%s", combat)
	}
	assistant := BuildStoryAssistantSystemInstruction(StoryAssistantSystemInstructionInput{
		Action:         "analyze",
		OutputLanguage: "en-US",
	})
	if !strings.Contains(assistant, "# 语言") || !strings.Contains(assistant, "英文") {
		t.Fatalf("the assistant prompt does not state its language in Chinese:\n%s", assistant)
	}
}

// The batch that moved the language rule into Chinese also rewrote the opening
// line of every prompt it touched. Those lines are model-facing prose, so they
// must be Chinese too: the player's page language decides what the model writes,
// not what language the instruction itself is written in. The sections below
// each prompt are outside this batch and are checked on their own.
func TestTheTouchedPromptBodiesCarryNoEnglishSentence(t *testing.T) {
	cases := map[string]string{
		"character chat":         characterChatBuiltIn,
		"group chat":             groupChatBuiltIn,
		"intimacy":               intimacyBuiltIn,
		"story creation":         storyCreationBuiltIn,
		"world creation":         worldCreationBuiltIn,
		"story assistant reader": storyAssistantAnalyzePersona,
		"story assistant ideas":  storyAssistantInspirePersona,
	}
	for name, line := range cases {
		assertNoEnglishSentence(t, name, line)
	}
}

// The assistant's output description is the other half this batch rewrote: it is
// where the model learns what `read` and `detail` hold, so an English sentence
// there is an English instruction to a model whose page is Chinese.
func TestTheAssistantOutputDescriptionIsChinese(t *testing.T) {
	analyze := BuildStoryAssistantSystemInstruction(StoryAssistantSystemInstructionInput{
		Action:         StoryAssistantActionAnalyze,
		OutputLanguage: "zh-CN",
	})
	inspire := BuildStoryAssistantSystemInstruction(StoryAssistantSystemInstructionInput{
		Action:         StoryAssistantActionInspire,
		OutputLanguage: "zh-CN",
	})
	for name, text := range map[string]string{"analyze": analyze, "inspire": inspire} {
		// Every line that describes what the model must put in the three fields is
		// Chinese now. The surrounding protocol text ("Output only a JSON object…")
		// predates this batch and is out of scope here.
		for _, line := range strings.Split(text, "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "- `") && !strings.HasPrefix(trimmed, "- 不要添加字段") {
				continue
			}
			assertNoEnglishSentence(t, name+" output line", trimmed)
		}
	}
	// The two actions still say different things about `read`; the rewrite must
	// not have collapsed them into one description.
	if !strings.Contains(inspire, "`read` 就是上面说的那段简短开场") {
		t.Fatalf("the inspire prompt lost its own description of `read`:\n%s", inspire)
	}
	if !strings.Contains(analyze, "`read` 就是分析本身") {
		t.Fatalf("the analyse prompt lost its own description of `read`:\n%s", analyze)
	}
	for _, retired := range []string{
		"`read` is the short opening paragraph described above",
		"`read` is the analysis itself",
		"`suggestions` are the directions themselves",
		"`suggestions` are the concrete phrasings the player may drop into their input",
		"Return an empty `suggestions` array",
		"Do not add fields, do not wrap the object",
	} {
		if strings.Contains(analyze, retired) || strings.Contains(inspire, retired) {
			t.Fatalf("the assistant output description still carries the retired English wording %q", retired)
		}
	}
}
