package interactiveapp

import (
	"strings"
	"testing"

	"ta/internal/character"
)

// 自制-并行事项与到期事件 §五 reaches the player through two contexts: the main
// story's scene block and the advisor's standing context. Both used to carry only
// what the scene itself held, so a letter that arrived while the protagonist was
// away was invisible to the narrator — and the return to the main story was
// written as though nothing had happened, or as though the offer were being made
// now.
//
// These assert the rendering, not the rule: the rule lives in
// internal/character and is proven there. What is checked here is that the words
// reach the model, keep their own hour, and say plainly that a passed hour is not
// an offer.

func sceneMessageState() character.State {
	return character.State{
		Version: character.StateVersion,
		Profiles: []character.Profile{
			{ID: "guardsman", Name: "陈九", Role: "北门守卒", Revision: 1},
		},
		Messages: []character.Message{
			{
				ID: "msg-live", StoryID: "story-1", BranchID: "main", CharacterID: "guardsman",
				Text: "三日后北门换防，我在门内等你。", Basis: "主角此前与他约定在北门再会",
				WorldTime: "第3日 酉时", Deadline: "三日内", DeadlineSeconds: 3 * 24 * 60 * 60,
				Status: character.MessageUnread,
			},
		},
	}.Normalize()
}

func TestSceneBlockStatesALettersOwnHourRatherThanTheScenes(t *testing.T) {
	state := sceneMessageState()
	block := storySceneMessagesBlock(state, "第4日 辰时", 24*60*60, StoryRuntimeContextMaxBytes)
	if block == "" {
		t.Fatal("a pending letter was left out of the scene block")
	}
	// The sender's name, the hour it was written and its own words.
	if !strings.Contains(block, "陈九") {
		t.Fatalf("the block does not name the sender:\n%s", block)
	}
	if !strings.Contains(block, "第3日 酉时") {
		t.Fatalf("the block does not carry the hour it was written:\n%s", block)
	}
	if !strings.Contains(block, "三日后北门换防") {
		t.Fatalf("the block does not quote the letter:\n%s", block)
	}
	// The narrator must not write it as speech in this scene.
	if !strings.Contains(block, "not happening in this scene") {
		t.Fatalf("the block does not say the letter is not spoken here:\n%s", block)
	}
	// A letter still inside its hour is stated as running, not as passed.
	if !strings.Contains(block, "its hour runs to") {
		t.Fatalf("a live deadline was not stated:\n%s", block)
	}
	if strings.Contains(block, "its hour passed") {
		t.Fatalf("a letter inside its hour was reported as passed:\n%s", block)
	}
}

func TestSceneBlockMarksAPassedHourAsNoLongerOffered(t *testing.T) {
	state := sceneMessageState()
	// Five days on, against a three-day invitation.
	block := storySceneMessagesBlock(state, "第8日 辰时", 5*24*60*60, StoryRuntimeContextMaxBytes)
	if !strings.Contains(block, "its hour passed at 三日内") {
		t.Fatalf("a passed hour was not marked:\n%s", block)
	}
	if !strings.Contains(block, "no longer stands") {
		t.Fatalf("the block does not say the offer stopped:\n%s", block)
	}
	// The words are still quoted: the story says someone wrote, not that the
	// letter never existed.
	if !strings.Contains(block, "三日后北门换防") {
		t.Fatalf("a lapsed letter lost its words:\n%s", block)
	}
}

func TestSceneBlockGivesAGreetingNoCountdown(t *testing.T) {
	state := character.State{
		Version: character.StateVersion,
		Profiles: []character.Profile{
			{ID: "guardsman", Name: "陈九", Revision: 1},
		},
		Messages: []character.Message{{
			ID: "msg-hello", StoryID: "story-1", BranchID: "main", CharacterID: "guardsman",
			Text: "夜里添件衣裳。", Basis: "主角上次离城时说过要回来",
			WorldTime: "第3日 酉时", Status: character.MessageUnread,
		}},
	}.Normalize()
	block := storySceneMessagesBlock(state, "第40日 辰时", 39*24*60*60, StoryRuntimeContextMaxBytes)
	if block == "" {
		t.Fatal("a plain greeting was dropped")
	}
	if strings.Contains(block, "its hour") {
		t.Fatalf("a greeting with no deadline was given a countdown:\n%s", block)
	}
	if !strings.Contains(block, "第3日 酉时") {
		t.Fatalf("the greeting lost its send time:\n%s", block)
	}
}

func TestSceneBlockDropsLettersThePlayerAlreadySettled(t *testing.T) {
	state := sceneMessageState()
	state.Messages[0].Status = character.MessageHandled
	if block := storySceneMessagesBlock(state, "第4日 辰时", 24*60*60, StoryRuntimeContextMaxBytes); block != "" {
		t.Fatalf("a handled letter was reopened in the scene block:\n%s", block)
	}
}

func TestSceneBlockStaysInsideItsBudget(t *testing.T) {
	state := sceneMessageState()
	// A budget too small for even the header yields nothing rather than a
	// half-written section the model would read as a truncated fact.
	if block := storySceneMessagesBlock(state, "第4日 辰时", 24*60*60, 16); block != "" {
		t.Fatalf("an undersized budget produced a section:\n%s", block)
	}
	block := storySceneMessagesBlock(state, "第4日 辰时", 24*60*60, 400)
	if len([]byte(block)) > 400 {
		t.Fatalf("the section is %d bytes against a 400-byte budget", len([]byte(block)))
	}
}

func TestOpenLetterContextDistinguishesTransitFromArrivalThreshold(t *testing.T) {
	letter := character.Matter{
		ID: "mat_letter_1", CharacterID: "su-yulan", Kind: character.MatterAction,
		Status: character.MatterOpen, Statement: "林澄询问旧邮戳的登记去向。",
		Letter: &character.LetterDelivery{Body: "请告知旧邮戳的去向。", SentAtSeconds: 0, EarliestDeliverySeconds: 86_400},
	}
	member := character.CastMember{Profile: character.Profile{ID: "su-yulan", Name: "苏玉兰"}, OpenMatters: []character.Matter{letter}}
	var before strings.Builder
	writeSceneOpenMatters(&before, []character.CastMember{member}, 3_600, StoryRuntimeContextMaxBytes)
	if !strings.Contains(before.String(), "mat_letter_1") || !strings.Contains(before.String(), "in transit") {
		t.Fatalf("letter below the threshold lacks its real id or transit state:\n%s", before.String())
	}
	var after strings.Builder
	writeSceneOpenMatters(&after, []character.CastMember{member}, 90_000, StoryRuntimeContextMaxBytes)
	if !strings.Contains(after.String(), "arrival threshold reached") || !strings.Contains(after.String(), "receipt and response remain unrecorded") {
		t.Fatalf("reached threshold was not distinguished from receipt/reply:\n%s", after.String())
	}
	if strings.Contains(after.String(), "in transit") || strings.Contains(after.String(), "delivered") {
		t.Fatalf("elapsed delivery time was mislabeled as transit or proof of delivery:\n%s", after.String())
	}
}
