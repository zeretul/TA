package app

import (
	"context"
	"errors"
	"testing"

	"ta/internal/character"
	"ta/internal/interactive"
)

// This file closes 第11项's remaining gap. The pieces already had tests — the
// invitation enters, a retry does not ask twice, the window hides a pending
// invite — but each of those tests stops at the write it was written for, and
// none of them follows one activity from the invitation through a spoken round,
// out of the intimacy view and back, and across a restart.
//
// What a player meets is one container: the person's chat, with 普通交谈 and
// 亲密交流 as two views of the same activity. So this is one chain through the
// production services, with a local model standing in for the person's voice:
// the window offers only people whose own authored age makes them adults; the
// invitation is answered and the activity opens; a line is said and answered;
// the same command said twice commits once; leaving the intimacy view ends
// nothing and the room stays readable; and a reopened store tells the same
// story. Nothing here grants cultivation: the position before and after the
// whole activity is the same number.

// intimacyTestPosition reads the protagonist's banked cultivation position, so
// the test can state that the activity moved nothing.
func intimacyTestPosition(t *testing.T, store *interactive.Store, storyID string) interactive.CultivationPosition {
	t.Helper()
	position, err := store.CultivationPosition(storyID, "main", interactive.DefaultActorID)
	if err != nil {
		t.Fatalf("read the protagonist's cultivation position: %v", err)
	}
	return position
}

// intimacyTestActivity returns the one activity the window lists for this story.
func intimacyTestActivity(t *testing.T, window IntimacyWindow, activityID string) IntimacyWindowActivity {
	t.Helper()
	for _, activity := range window.Activities {
		if activity.ID == activityID {
			return activity
		}
	}
	t.Fatalf("the window does not list activity %q: %+v", activityID, window.Activities)
	return IntimacyWindowActivity{}
}

// TestOneIntimacyActivityRunsFromInvitationToReopen is 第11项's chain.
func TestOneIntimacyActivityRunsFromInvitationToReopen(t *testing.T) {
	application, storyID := intimacyInviteTestApp(t)
	ctx := context.Background()
	before := intimacyTestPosition(t, application.interactive, storyID)

	// --- 合法成年：only the person whose own data says adult is offered ------
	window, err := application.IntimacyWindow(IntimacyWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatalf("read the intimacy window: %v", err)
	}
	if !character.ContainsID(window.Eligible, "chen_qiaoqian") {
		t.Fatalf("the adult in the room is not offered: %+v", window.Eligible)
	}
	// 林晚 is standing right there and the story knows her, so the only thing
	// keeping her out is her own authored age. That is the rule working, not a
	// person who happens to be missing.
	if character.ContainsID(window.Eligible, "lin_wan") {
		t.Fatalf("a person the story authored as a minor is offered: %+v", window.Eligible)
	}
	if !character.ContainsID(window.Scene.Present, "lin_wan") {
		t.Fatalf("the fixture does not actually have her in the scene: %+v", window.Scene.Present)
	}

	// The refusal is a stated condition, not a silent empty list: inviting her
	// directly is refused with the reason.
	if _, err := application.InviteIntimacy(ctx, IntimacyInviteRequest{
		StoryID: storyID, CharacterID: "lin_wan", OneOnOne: true, CommandID: "invite-minor",
	}); !errors.Is(err, character.ErrIntimacyNotAdult) {
		t.Fatalf("inviting a person the story authored as a minor returned %v, want ErrIntimacyNotAdult", err)
	}

	// --- 邀请：the person is really asked, and their own yes opens the room ---
	invited, err := application.InviteIntimacy(ctx, IntimacyInviteRequest{
		StoryID: storyID, CharacterID: "chen_qiaoqian", OneOnOne: true, CommandID: "invite-main",
	})
	if err != nil {
		t.Fatalf("invite the adult in the room: %v", err)
	}
	if invited.Activity.Phase != character.IntimacyActive {
		t.Fatalf("phase = %q after a one-to-one yes, want active", invited.Activity.Phase)
	}
	if invited.Activity.GroupID == "" {
		t.Fatal("the running activity names no room to hold its conversation")
	}
	// The answer is on file in her own words, so a yes is something the player can
	// read back rather than a status that silently changed. A one-to-one answer is
	// spoken privately: nobody else was asked and nobody else may hear it.
	activityID := invited.ActivityID
	registry, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	answered := false
	for _, exchange := range registry.ExchangesFor("chen_qiaoqian") {
		if exchange.Speaker == character.SpeakerCharacter && exchange.Channel == character.ChannelPrivate {
			answered = exchange.Text != ""
		}
	}
	if !answered {
		t.Fatalf("the person's own answer was never committed: %+v", registry.Exchanges)
	}

	// --- 正式活动：the window shows the room, its roster and the opening ------
	room, err := application.IntimacyWindow(IntimacyWindowRequest{StoryID: storyID, ActivityID: activityID})
	if err != nil {
		t.Fatalf("read the running activity: %v", err)
	}
	listed := intimacyTestActivity(t, room, activityID)
	if listed.Phase != character.IntimacyActive {
		t.Fatalf("the window lists the activity as %q, want active", listed.Phase)
	}
	if len(listed.Roster) != 1 || listed.Roster[0] != "chen_qiaoqian" {
		t.Fatalf("roster = %+v, want exactly the person who agreed", listed.Roster)
	}
	if listed.ParticipantNames["chen_qiaoqian"] != "陈巧倩" {
		t.Fatalf("the roster shows the id instead of her name: %+v", listed.ParticipantNames)
	}
	opening := room.Exchanges
	if len(opening) == 0 {
		t.Fatal("the activity opened with no committed line")
	}
	for _, exchange := range opening {
		if exchange.Channel != character.ChannelGroup || exchange.GroupID != listed.GroupID {
			t.Fatalf("an opening line is not in the activity's own room: %+v", exchange)
		}
	}
	openingCount := len(opening)

	// --- 交流：one line said, answered by the person in the room -------------
	round, err := application.SendIntimacyMessage(ctx, IntimacySendRequest{
		StoryID: storyID, ActivityID: activityID, Message: "灯再亮一点。", CommandID: "say-1",
	})
	if err != nil {
		t.Fatalf("say one line in the activity: %v", err)
	}
	if len(round.Lines) != 2 {
		t.Fatalf("the round committed %d lines, want the player's line and one answer: %+v", len(round.Lines), round.Lines)
	}
	if round.Lines[0].Speaker != character.SpeakerPlayer || round.Lines[0].Text != "灯再亮一点。" {
		t.Fatalf("the player's own line was not committed first: %+v", round.Lines[0])
	}
	if round.Lines[1].Speaker != character.SpeakerCharacter || round.Lines[1].AuthorID != "chen_qiaoqian" {
		t.Fatalf("the answer is not the person in the room: %+v", round.Lines[1])
	}
	for _, line := range round.Lines {
		if line.Channel != character.ChannelGroup || line.GroupID != listed.GroupID {
			t.Fatalf("a spoken line left the activity's room: %+v", line)
		}
	}

	// A retry of the same command is the same words, not a second exchange.
	retried, err := application.SendIntimacyMessage(ctx, IntimacySendRequest{
		StoryID: storyID, ActivityID: activityID, Message: "灯再亮一点。", CommandID: "say-1",
	})
	if err != nil {
		t.Fatalf("retry the same line: %v", err)
	}
	if len(retried.Lines) != len(round.Lines) || retried.Lines[0].ID != round.Lines[0].ID {
		t.Fatalf("the retry committed a second exchange: %+v vs %+v", retried.Lines, round.Lines)
	}
	afterRound, err := application.IntimacyWindow(IntimacyWindowRequest{StoryID: storyID, ActivityID: activityID})
	if err != nil {
		t.Fatalf("read the room after the round: %v", err)
	}
	if len(afterRound.Exchanges) != openingCount+2 {
		t.Fatalf("the room holds %d lines after one round, want %d", len(afterRound.Exchanges), openingCount+2)
	}

	// --- 切换：the ordinary view is a different view of the same activity ----
	//
	// Leaving 亲密交流 for 普通交谈 is the player's own view change. It must not
	// end anything, and the ordinary transcript must not silently absorb the
	// room's lines: the private conversation and the room are different
	// audiences, and mixing them would hand one to the other.
	ordinary, err := application.CharacterWindow(CharacterWindowRequest{
		StoryID: storyID, BranchID: "main", CharacterID: "chen_qiaoqian",
	})
	if err != nil {
		t.Fatalf("read the ordinary conversation: %v", err)
	}
	for _, exchange := range ordinary.Exchanges {
		if exchange.Channel == character.ChannelGroup {
			t.Fatalf("a room line appeared in the ordinary conversation: %+v", exchange)
		}
	}
	stillRunning, err := application.IntimacyWindow(IntimacyWindowRequest{StoryID: storyID, ActivityID: activityID})
	if err != nil {
		t.Fatalf("read the activity after switching views: %v", err)
	}
	if stillRunning.ActivityID != activityID || len(stillRunning.Exchanges) != openingCount+2 {
		t.Fatalf("switching views changed the activity: %+v", stillRunning.Activities)
	}
	if intimacyTestActivity(t, stillRunning, activityID).Phase != character.IntimacyActive {
		t.Fatal("switching views ended the activity")
	}

	// --- 无修炼收益：the whole activity moved no cultivation ------------------
	after := intimacyTestPosition(t, application.interactive, storyID)
	if after.Progress != before.Progress || after.Realm != before.Realm {
		t.Fatalf("the activity moved cultivation from %+v to %+v", before, after)
	}

	// --- 关闭重开：the journal, not the process, holds all of it -------------
	if err := application.interactive.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}
	reopenedStore := interactive.NewStore(application.workspace)
	t.Cleanup(func() { _ = reopenedStore.Close() })
	reopened := &App{cfg: application.cfg, workspace: application.workspace, interactive: reopenedStore}

	reopenedWindow, err := reopened.IntimacyWindow(IntimacyWindowRequest{StoryID: storyID, ActivityID: activityID})
	if err != nil {
		t.Fatalf("read the activity after reopening: %v", err)
	}
	reopenedActivity := intimacyTestActivity(t, reopenedWindow, activityID)
	if reopenedActivity.Phase != character.IntimacyActive {
		t.Fatalf("reopening changed the phase to %q", reopenedActivity.Phase)
	}
	if len(reopenedActivity.Roster) != 1 || reopenedActivity.Roster[0] != "chen_qiaoqian" {
		t.Fatalf("reopening changed the roster: %+v", reopenedActivity.Roster)
	}
	if len(reopenedWindow.Exchanges) != openingCount+2 {
		t.Fatalf("reopening kept %d lines, want %d", len(reopenedWindow.Exchanges), openingCount+2)
	}
	for index, exchange := range reopenedWindow.Exchanges {
		if exchange.ID != afterRound.Exchanges[index].ID || exchange.Text != afterRound.Exchanges[index].Text {
			t.Fatalf("line %d changed across the restart: %+v vs %+v", index, exchange, afterRound.Exchanges[index])
		}
	}
	if reopenedWindow.Participants["chen_qiaoqian"] != "陈巧倩" {
		t.Fatalf("reopening lost who spoke: %+v", reopenedWindow.Participants)
	}
	// The eligible list is computed from the frozen catalog, so it survives the
	// restart unchanged: the same adult is offered and the same minor is not.
	reopenedEligible, err := reopened.IntimacyWindow(IntimacyWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatalf("read the eligibility list after reopening: %v", err)
	}
	if !character.ContainsID(reopenedEligible.Eligible, "chen_qiaoqian") ||
		character.ContainsID(reopenedEligible.Eligible, "lin_wan") {
		t.Fatalf("reopening changed who may be invited: %+v", reopenedEligible.Eligible)
	}
	if position := intimacyTestPosition(t, reopenedStore, storyID); position.Progress != before.Progress {
		t.Fatalf("reopening changed cultivation progress from %v to %v", before.Progress, position.Progress)
	}
}
