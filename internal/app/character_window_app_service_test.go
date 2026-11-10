package app

import (
	"testing"

	"ta/internal/character"
)

// characterWindowState is a cast with one character who has written to the
// player and one who has not, plus a mix of messages the player has and has not
// dealt with. It is built by hand rather than through a turn so the assertions
// below describe the window's own contract instead of restating the write path.
func characterWindowState() character.State {
	return character.State{
		Version: character.StateVersion,
		Profiles: []character.Profile{
			{ID: "guardsman", StoryID: "st", Name: "陈九", Role: "北门守卒", FirstSeenWorldTime: "第1日"},
			{ID: "merchant", StoryID: "st", Name: "周管事", Role: "坊市管事", FirstSeenWorldTime: "第2日"},
		},
		Messages: []character.Message{
			{ID: "msg_a", StoryID: "st", BranchID: "main", CharacterID: "guardsman", Text: "今夜北门有变。", Basis: "主角与他约定今夜再会", WorldTime: "第3日 酉时", Status: character.MessageUnread},
			{ID: "msg_b", StoryID: "st", BranchID: "main", CharacterID: "guardsman", Text: "上一封已经看过了。", Basis: "此前的约定", WorldTime: "第2日 辰时", Status: character.MessageRead},
			{ID: "msg_c", StoryID: "st", BranchID: "main", CharacterID: "merchant", Text: "木牌的事有回话。", Basis: "白天的交涉", WorldTime: "第3日 未时", Status: character.MessageHandled},
			{ID: "msg_d", StoryID: "st", BranchID: "main", CharacterID: "merchant", Text: "这一封我忽略掉了。", Basis: "无关紧要", WorldTime: "第3日 申时", Status: character.MessageIgnored},
		},
		Matters: []character.Matter{
			{ID: "mt_a", StoryID: "st", BranchID: "main", CharacterID: "guardsman", Kind: character.MatterPermission, Status: character.MatterOpen, Statement: "允许主角今夜走北门", WorldTime: "第3日"},
			{ID: "mt_b", StoryID: "st", BranchID: "main", CharacterID: "merchant", Kind: character.MatterPromise, Status: character.MatterOpen, Statement: "三日内补齐差价", WorldTime: "第3日"},
			{ID: "mt_c", StoryID: "st", BranchID: "main", CharacterID: "guardsman", Kind: character.MatterAction, Status: character.MatterSettled, Statement: "已经办完的事", WorldTime: "第2日"},
		},
		Revision: 1,
	}
}

// TestCharacterWindowPendingKeepsOnlyUndeltWithMessages is the delivered-message
// half of proactive messaging at the read side. A message the player has already
// handled or ignored must not come back as pending, and a message they have only
// read stays pending: reading is not dealing with it, which is what keeps a
// quiet hint honest about what is still outstanding.
func TestCharacterWindowPendingKeepsOnlyUndeltWithMessages(t *testing.T) {
	state := characterWindowState()
	pending := characterWindowPending(state, "第3日 戌时", 0)

	ids := make([]string, 0, len(pending))
	for _, message := range pending {
		ids = append(ids, message.ID)
	}
	// Newest first: msg_a (第3日 酉时) then msg_b (第2日 辰时). The handled and
	// ignored messages are excluded.
	if len(ids) != 2 || ids[0] != "msg_a" || ids[1] != "msg_b" {
		t.Fatalf("pending = %v, want the unread and read messages newest first", ids)
	}
	// Every pending message names a character the cast actually lists, so the
	// stage's hint can always be resolved to a person.
	cast := map[string]bool{}
	for _, row := range characterWindowCast(state, character.Scene{}, pictureSources{}) {
		cast[row.ID] = true
	}
	for _, message := range pending {
		if !cast[message.CharacterID] {
			t.Fatalf("pending message %q names %q, who is not in the cast", message.ID, message.CharacterID)
		}
		if message.Status != string(character.MessageUnread) && message.Status != string(character.MessageRead) {
			t.Fatalf("pending message %q carries status %q", message.ID, message.Status)
		}
	}
	// The send time travels with the message rather than being replaced by the
	// time the player happens to read it.
	if pending[0].WorldTime != "第3日 酉时" {
		t.Fatalf("pending message world time = %q, want the original 第3日 酉时", pending[0].WorldTime)
	}
	// The roster's unread count and the pending list are two views of one state,
	// so unread can never exceed pending.
	unread := 0
	for _, row := range characterWindowCast(state, character.Scene{}, pictureSources{}) {
		unread += row.UnreadMessages
	}
	if unread > len(pending) {
		t.Fatalf("roster reports %d unread but pending lists %d", unread, len(pending))
	}
}

// TestCharacterWindowMattersAgreeWithTheRoster is the agreement half of the
// window read. The per-character matter list and the roster's open_matters count
// are computed from one projection, so they must agree; a disagreement would
// mean the window shows a thread that the roster denies, or the reverse.
func TestCharacterWindowMattersAgreeWithTheRoster(t *testing.T) {
	state := characterWindowState()
	rows := characterWindowCast(state, character.Scene{}, pictureSources{})

	byID := map[string]CharacterWindowCharacter{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	for _, id := range []string{"guardsman", "merchant"} {
		open := character.OpenMatters(id, state.Matters)
		row, ok := byID[id]
		if !ok {
			t.Fatalf("the cast is missing %q", id)
		}
		if row.OpenMatters != len(open) {
			t.Fatalf("%s: roster open_matters = %d, per-character list = %d", id, row.OpenMatters, len(open))
		}
	}
	// A settled matter is not open, so the character with only a settled thread
	// reports none. This is the "already resolved is not settled twice" rule as
	// the window presents it.
	if byID["guardsman"].OpenMatters != 1 {
		t.Fatalf("guardsman open matters = %d, want 1 (the permission, not the settled action)", byID["guardsman"].OpenMatters)
	}
	if byID["merchant"].OpenMatters != 1 {
		t.Fatalf("merchant open matters = %d, want 1", byID["merchant"].OpenMatters)
	}
}

func TestCharacterWindowRetainsDeliveredLetterAndCommittedReply(t *testing.T) {
	state := characterWindowState()
	state.Matters = append(state.Matters,
		character.Matter{
			ID: "mat-letter", StoryID: "st", BranchID: "main", CharacterID: "guardsman",
			Kind: character.MatterAction, Status: character.MatterSettled,
			Statement: "一封寄给陈九的信：请替我留一间房。", WorldTime: "第1日",
			SettledBy: "mat-reply",
			Letter:    &character.LetterDelivery{Body: "请替我留一间房。", SentAtSeconds: 0, EarliestDeliverySeconds: 86400},
		},
		character.Matter{
			ID: "mat-reply", StoryID: "st", BranchID: "main", CharacterID: "guardsman",
			Kind: character.MatterAction, Status: character.MatterSettled,
			Statement: "陈九收到来信后回信：房间已替你留好。", WorldTime: "第2日",
			SettledBy:   "mat-letter",
			LetterReply: &character.LetterReply{Body: "房间已替你留好。"},
		},
		character.Matter{
			ID: "mat-old", StoryID: "st", BranchID: "main", CharacterID: "guardsman",
			Kind: character.MatterAction, Status: character.MatterSettled,
			Statement: "早已结束的其他事情。", WorldTime: "第1日",
		},
	)

	visible := characterWindowMatters("guardsman", state.Matters)
	if len(visible) != 3 {
		t.Fatalf("visible character-window matters = %+v, want open items plus sent and answered letter", visible)
	}
	if visible[1].ID != "mat-letter" || visible[2].ID != "mat-reply" {
		t.Fatalf("letter history order = %+v", visible)
	}
	if visible[1].Letter == nil || visible[1].Letter.Body != "请替我留一间房。" || visible[2].LetterReply == nil || visible[2].LetterReply.Body != "房间已替你留好。" {
		t.Fatalf("the window lost the sent letter or actual reply: %+v", visible[1:])
	}
}

func TestCharacterWindowDistinguishesReceiptFromWrittenReply(t *testing.T) {
	matters := []character.Matter{
		{ID: "mat-letter", CharacterID: "guardsman", Status: character.MatterSettled, SettledBy: "mat-receipt", Letter: &character.LetterDelivery{Body: "留房", SentAtSeconds: 0, EarliestDeliverySeconds: 86400}},
		{ID: "mat-receipt", CharacterID: "guardsman", Status: character.MatterSettled, SettledBy: "mat-letter", Statement: "陈九当面说收到了信。"},
	}
	visible := characterWindowMatters("guardsman", matters)
	if len(visible) != 2 || visible[1].LetterReply != nil || visible[1].Statement != "陈九当面说收到了信。" {
		t.Fatalf("an oral receipt was presented as a written reply: %+v", visible)
	}
}

// TestCharacterWindowCarriesTheSettledGender is the last link in the chain the
// design names: 玩家展示. A gender the story settled on the profile must reach the
// window row, and a person nobody settled must come out empty rather than with
// a placeholder — an unsettled gender is a fact about the record, not a gap in
// what the player has found out.
func TestCharacterWindowCarriesTheSettledGender(t *testing.T) {
	state := characterWindowState()
	state.Profiles[0].Gender = character.GenderMale
	rows := characterWindowCast(state, character.Scene{}, pictureSources{})
	byID := map[string]CharacterWindowCharacter{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	if got := byID["guardsman"].Gender; got != character.GenderMale {
		t.Fatalf("the window row for a settled person reads %q, want %q", got, character.GenderMale)
	}
	if got := byID["merchant"].Gender; got != "" {
		t.Fatalf("a person nobody settled came out %q, want empty", got)
	}
}
