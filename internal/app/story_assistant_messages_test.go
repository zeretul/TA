package app

import (
	"strings"
	"testing"

	"ta/internal/character"
)

// The advisor's standing context reads 历史承诺与期限. A letter is where an offer
// with an hour often arrives, and the promise behind it was never spoken in a
// scene the advisor saw — so before this the advisor planned around an
// invitation it had no way to know about, and could not tell a lapsed one from
// a live one.
//
// The rule (what lapses, and when) lives in internal/character and is proven
// there. These check the rendering the advisor actually reads.

func assistantMessageState() character.State {
	return character.State{
		Version: character.StateVersion,
		Profiles: []character.Profile{
			{ID: "mu_peiling", Name: "慕沛灵", Role: "药园管事", Revision: 1},
		},
		Messages: []character.Message{{
			ID: "msg-herbs", StoryID: "story-1", BranchID: "main", CharacterID: "mu_peiling",
			Text:  "东畦的药苗我留了一份，三日内来取，过了就给别人了。",
			Basis: "主角替她查过东畦", WorldTime: "第3日 酉时",
			Deadline: "三日内", DeadlineSeconds: 3 * 24 * 60 * 60,
			Status: character.MessageUnread,
		}},
	}.Normalize()
}

func TestTheAssistantReadsALetterAndItsHour(t *testing.T) {
	var sb strings.Builder
	writeStoryAssistantMessages(&sb, assistantMessageState(), "第4日 辰时", 24*60*60)
	rendered := sb.String()

	if !strings.Contains(rendered, "### Word that has reached the protagonist") {
		t.Fatalf("the assistant is not told a letter arrived:\n%s", rendered)
	}
	for _, want := range []string{
		"慕沛灵",
		"第3日 酉时",
		"东畦的药苗我留了一份",
		// Receiving a letter is not accepting what it offers, which is the one
		// thing an advisor must not get wrong when it suggests a next step.
		"receiving one accepts nothing",
		"it names 三日内",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("the assistant's context is missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "its hour passed") {
		t.Fatalf("a letter inside its hour is reported as passed:\n%s", rendered)
	}
}

func TestTheAssistantReadsALapsedLetterAsNoLongerOffered(t *testing.T) {
	var sb strings.Builder
	writeStoryAssistantMessages(&sb, assistantMessageState(), "第8日 辰时", 5*24*60*60)
	rendered := sb.String()

	if !strings.Contains(rendered, "its hour passed at 三日内") {
		t.Fatalf("the passed hour is not stated:\n%s", rendered)
	}
	if !strings.Contains(rendered, "no longer stands") {
		t.Fatalf("the advisor is not told the offer stopped:\n%s", rendered)
	}
	// The words survive: the story still has to be able to say she wrote.
	if !strings.Contains(rendered, "东畦的药苗我留了一份") {
		t.Fatalf("a lapsed letter lost its words:\n%s", rendered)
	}
}

func TestTheAssistantGivesAGreetingNoCountdown(t *testing.T) {
	state := character.State{
		Version: character.StateVersion,
		Profiles: []character.Profile{
			{ID: "mu_peiling", Name: "慕沛灵", Revision: 1},
		},
		Messages: []character.Message{{
			ID: "msg-hello", StoryID: "story-1", BranchID: "main", CharacterID: "mu_peiling",
			Text: "山里夜凉，自己添件衣裳。", Basis: "主角上回替她查过东畦",
			WorldTime: "第3日 酉时", Status: character.MessageUnread,
		}},
	}.Normalize()
	var sb strings.Builder
	writeStoryAssistantMessages(&sb, state, "第40日 辰时", 39*24*60*60)
	rendered := sb.String()

	if !strings.Contains(rendered, "山里夜凉") {
		t.Fatalf("a plain greeting did not reach the advisor:\n%s", rendered)
	}
	if strings.Contains(rendered, "its hour") || strings.Contains(rendered, "it names") {
		t.Fatalf("a greeting with no deadline was given a countdown:\n%s", rendered)
	}
}

func TestTheAssistantDropsLettersThePlayerAlreadySettled(t *testing.T) {
	state := assistantMessageState()
	state.Messages[0].Status = character.MessageHandled
	var sb strings.Builder
	writeStoryAssistantMessages(&sb, state, "第4日 辰时", 24*60*60)
	if rendered := sb.String(); rendered != "" {
		t.Fatalf("a handled letter was reopened for the advisor:\n%s", rendered)
	}
}

// TestTheAssistantDoesNotRepeatALetterAcrossTurns keeps the block from becoming
// a second inbox: a letter already listed once is not listed twice for the same
// content, and the section carries no engineering vocabulary.
func TestTheAssistantMessageBlockUsesNoEngineeringWords(t *testing.T) {
	var sb strings.Builder
	writeStoryAssistantMessages(&sb, assistantMessageState(), "第4日 辰时", 24*60*60)
	rendered := strings.ToLower(sb.String())
	for _, word := range []string{"permission", "transaction", "authority", "deadline_seconds"} {
		if strings.Contains(rendered, word) {
			t.Fatalf("the advisor's context carries the engineering word %q:\n%s", word, rendered)
		}
	}
}
