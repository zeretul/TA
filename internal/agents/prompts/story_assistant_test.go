package prompts

import (
	"strings"
	"testing"
)

// 自制-剧情助手与选项分析 asks the two actions to have 不同人设 while sharing one
// boundary. These tests hold both halves of that: the voices differ, and every
// rule that decides what the answer may contain is identical between them.

// assistantSharedBoundaryRules are the sentences that must appear in both
// prompts. They are the boundary rather than the tone: a warmer persona must not
// be a laxer one, so a change that dropped one of these from either action would
// be a real regression even though the prose still reads well.
var assistantSharedBoundaryRules = []string{
	"Everything you say is advice.",
	"Never decide for the player",
	"Never invent a probability",
	"Use only what the protagonist knows.",
	"Never mention that you were given author-side notes",
	"- What is still standing — promises, permissions, duties and taken-on work",
	"- A thread marked as having come due no longer binds.",
	`"suggestions"`,
}

func TestTheTwoAssistantActionsHaveDifferentPersonas(t *testing.T) {
	analyze := BuildStoryAssistantSystemInstruction(StoryAssistantSystemInstructionInput{Action: StoryAssistantActionAnalyze})
	inspire := BuildStoryAssistantSystemInstruction(StoryAssistantSystemInstructionInput{Action: StoryAssistantActionInspire})

	analyzePersona := storyAssistantPersona(StoryAssistantActionAnalyze)
	inspirePersona := storyAssistantPersona(StoryAssistantActionInspire)
	if analyzePersona == inspirePersona {
		t.Fatal("the two actions must not share one voice")
	}
	// The language rule is stated above the persona on purpose: the persona opens
	// the voice, and the language governs every field the voice writes in. The
	// persona is therefore the first thing after that block, not the first thing
	// in the string.
	if !strings.Contains(analyze, analyzePersona) {
		t.Fatalf("the analyse prompt does not carry its own persona:\n%s", analyze)
	}
	if !strings.Contains(inspire, inspirePersona) {
		t.Fatalf("the inspire prompt does not carry its own persona:\n%s", inspire)
	}
	if at := strings.Index(analyze, analyzePersona); at < 0 || strings.Index(analyze, "# 语言") > at {
		t.Fatalf("the analyse prompt states its persona before its language rule:\n%s", analyze)
	}
	// Each persona stays in its own prompt: a player asking for ideas must not be
	// answered by the reader who takes things apart.
	if strings.Contains(analyze, inspirePersona) || strings.Contains(inspire, analyzePersona) {
		t.Fatal("one action's persona leaked into the other's prompt")
	}
}

func TestTheTwoAssistantActionsShareOneBoundary(t *testing.T) {
	for _, action := range []string{StoryAssistantActionAnalyze, StoryAssistantActionInspire} {
		instruction := BuildStoryAssistantSystemInstruction(StoryAssistantSystemInstructionInput{
			Action:       action,
			PlayerInput:  "我想找条稳妥的路出城",
			KnownContext: "### Where the protagonist is\n\n- Place: 城门",
			BranchPlan:   "第三章会有一场雨夜的追捕。",
		})
		for _, rule := range assistantSharedBoundaryRules {
			if !strings.Contains(instruction, rule) {
				t.Fatalf("%s prompt lost the shared rule %q:\n%s", action, rule, instruction)
			}
		}
		// Both still hand the model the same author-side warning, which is the one
		// thing it may read and must never reveal.
		if !strings.Contains(instruction, "author-side; never reveal") {
			t.Fatalf("%s prompt dropped the author-side marking:\n%s", action, instruction)
		}
	}
}

// TestAnUnknownActionFallsBackToTheCarefulReader keeps a bad action from being
// answered in a voice nobody chose. The service refuses unknown actions before
// this point, so the fallback is a second line of defence rather than the rule.
func TestAnUnknownActionFallsBackToTheCarefulReader(t *testing.T) {
	if got := storyAssistantPersona("predict"); got != storyAssistantAnalyzePersona {
		t.Fatalf("an unknown action must fall back to the careful reader, got %q", got)
	}
}

// The choices are in the context the model already receives, so "offer something
// different" was read as "list the choices again". Measured against the same
// scene, the inspire answer reproduced all three options on the page, one of
// them verbatim. The baseline has to be named for the two buttons to be two
// services rather than one with different adjectives.
func TestInspirationTreatsTheListedChoicesAsTheBaseline(t *testing.T) {
	inspire := BuildStoryAssistantSystemInstruction(StoryAssistantSystemInstructionInput{
		Action:       StoryAssistantActionInspire,
		KnownContext: "### The choices the story is offering\n\n1. 买符\n2. 留宿\n3. 赶路",
	})
	for _, want := range []string{
		"Treat the choices listed above as what the player has already been offered",
		"restates one of them is not an idea",
		"change the aim, the method, who it is done through, when it is done",
	} {
		if !strings.Contains(inspire, want) {
			t.Fatalf("the inspire prompt lost its de-duplication baseline %q:\n%s", want, inspire)
		}
	}
	// The analysis button must not start inventing options instead: that is the
	// other button's job, and the two used to converge from both sides.
	analyze := BuildStoryAssistantSystemInstruction(StoryAssistantSystemInstructionInput{
		Action:       StoryAssistantActionAnalyze,
		KnownContext: "### The choices the story is offering\n\n1. 买符\n2. 留宿\n3. 赶路",
	})
	if !strings.Contains(analyze, "must belong to one of the choices already listed") {
		t.Fatalf("the analyse prompt no longer keeps its phrasings inside the listed choices:\n%s", analyze)
	}
	if strings.Contains(analyze, "Treat the choices listed above as what the player has already been offered") {
		t.Fatalf("the de-duplication baseline leaked into the analyse prompt:\n%s", analyze)
	}
}

// 自制-剧情助手与选项分析 asks each direction to carry 新在哪里、可以先做什么、还不确定什么.
// Those three have to land in the existing label/detail/action fields; a prompt
// that asks for them somewhere the panel does not render would be invisible.
func TestInspirationAsksWhatIsNewAndWhatIsFirst(t *testing.T) {
	inspire := BuildStoryAssistantSystemInstruction(StoryAssistantSystemInstructionInput{Action: StoryAssistantActionInspire})
	if !strings.Contains(inspire, "what is new about it and what the player could do first") {
		t.Fatalf("the inspire prompt does not ask for the new part and the first step:\n%s", inspire)
	}
	if !strings.Contains(inspire, "`detail` 用一句话说清它新在哪里、玩家第一步会做什么") {
		t.Fatalf("the inspire output description does not put them in `detail`:\n%s", inspire)
	}
	// An idea may be a question rather than a claim. The old wording ("do not
	// propose something that depends on a fact they have not learned") read as a
	// ban on finding anything out, which is how every answer fell back to the
	// options already on the page.
	if !strings.Contains(inspire, "proposing to find out whether something is true does not claim it is true") {
		t.Fatalf("the inspire prompt does not allow proposing to find something out:\n%s", inspire)
	}
	// The read is a way in, not a second analysis.
	if !strings.Contains(inspire, "Keep `read` to one short paragraph") {
		t.Fatalf("the inspire prompt still asks for a full analysis in `read`:\n%s", inspire)
	}
	if strings.Contains(inspire, "`read` 就是分析本身") {
		t.Fatalf("the inspire prompt kept the analyse description of `read`:\n%s", inspire)
	}
}

// Asking again is the documented way to get another angle. If a second press
// only reshuffles the first answer, the button cannot deliver on that.
func TestInspirationTakesAnotherAngleWhenAskedAgain(t *testing.T) {
	inspire := BuildStoryAssistantSystemInstruction(StoryAssistantSystemInstructionInput{Action: StoryAssistantActionInspire})
	if !strings.Contains(inspire, "take a different angle rather than rewording the previous answer") {
		t.Fatalf("the inspire prompt does not ask for a different angle on a repeat:\n%s", inspire)
	}
}

// The per-story creative-direction layer was cancelled. The assistant must not
// be able to revive it, so the input carries no such field and the prompt has
// no such section even when a story still stores text from an older workspace.
func TestTheAssistantPromptHasNoRetiredDirectionSection(t *testing.T) {
	for _, action := range []string{StoryAssistantActionAnalyze, StoryAssistantActionInspire} {
		instruction := BuildStoryAssistantSystemInstruction(StoryAssistantSystemInstructionInput{
			Action:       action,
			PlayerInput:  "我该往哪走",
			KnownContext: "### Where the protagonist is\n\n- Place: 城门",
		})
		if strings.Contains(instruction, "standing direction") {
			t.Fatalf("%s prompt reintroduced the retired direction layer:\n%s", action, instruction)
		}
	}
}
