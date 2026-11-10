package app

import (
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/interactive"
)

// intimacyWindowState is one branch with a running activity, a finished one, and
// a person invited into the running one who has not answered yet.
//
// It is built by hand so the assertions below describe the window's read
// contract rather than restating the write path.
func intimacyWindowState() character.State {
	return character.State{
		Version: character.StateVersion,
		Profiles: []character.Profile{
			{ID: "lin", StoryID: "st", Name: "林晚", Role: "药师"},
			{ID: "shen", StoryID: "st", Name: "沈青", Role: "剑修"},
			{ID: "bai", StoryID: "st", Name: "白露", Role: "散修"},
		},
		Groups: []character.Group{{
			ID: "int_1:space", StoryID: "st", BranchID: "main",
			Type: character.GroupOther, Name: "双修", Channel: character.ChannelScene,
			Version: 3,
			Members: []character.GroupMember{
				{CharacterID: "lin", JoinedVersion: 1},
				{CharacterID: "shen", JoinedVersion: 2},
			},
		}, {
			ID: "int_1:invite:bai", StoryID: "st", BranchID: "main",
			Type: character.GroupOther, Name: "双修 · 邀请", Channel: character.ChannelScene,
			Version: 1,
			Members: []character.GroupMember{{CharacterID: "bai", JoinedVersion: 1}},
		}, {
			ID: "int_0:space", StoryID: "st", BranchID: "main",
			Type: character.GroupOther, Name: "双修", Channel: character.ChannelScene,
			Version: 2,
			Members: []character.GroupMember{{CharacterID: "lin", JoinedVersion: 1}},
		}},
		IntimacyActivities: []character.IntimacyActivity{
			{
				ID: "int_0", StoryID: "st", BranchID: "main", Phase: character.IntimacyEnded,
				GroupID: "int_0:space", Version: 3,
				Participants: []character.IntimacyParticipant{
					{CharacterID: "lin", Status: character.IntimacyAccepted, Adult: true},
				},
			},
			{
				ID: "int_1", StoryID: "st", BranchID: "main", Phase: character.IntimacyActive,
				GroupID: "int_1:space", Version: 4,
				Participants: []character.IntimacyParticipant{
					{CharacterID: "lin", Status: character.IntimacyAccepted, Adult: true},
					{CharacterID: "shen", Status: character.IntimacyAccepted, Adult: true},
					{CharacterID: "bai", Status: character.IntimacyPending, Adult: true},
				},
			},
		},
		Exchanges: []character.Exchange{
			{
				ID: "xch_room_1", StoryID: "st", BranchID: "main",
				CharacterID: "lin", AuthorID: "lin", Speaker: character.SpeakerCharacter,
				Channel: character.ChannelGroup, GroupID: "int_1:space", Text: "门关上了。",
				Audience: []string{"lin", "shen"},
			},
			{
				ID: "xch_room_2", StoryID: "st", BranchID: "main",
				CharacterID: "lin", Speaker: character.SpeakerPlayer,
				Channel: character.ChannelGroup, GroupID: "int_1:space", Text: "嗯。",
				Audience: []string{"lin", "shen"},
			},
			{
				ID: "xch_invite_1", StoryID: "st", BranchID: "main",
				CharacterID: "bai", AuthorID: "bai", Speaker: character.SpeakerCharacter,
				Channel: character.ChannelGroup, GroupID: "int_1:invite:bai",
				Text: "我只在门外说一句。", Audience: []string{"bai"},
			},
		},
		Revision: 1,
	}
}

// TestIntimacyInvitationPayloadRepairsAMissingComma is §5's separation of a
// technical fault from an answer. A model that emits a malformed object has not
// refused; the formatting is repaired and the person's stated answer is kept.
func TestIntimacyInvitationPayloadRepairsAMissingComma(t *testing.T) {
	payload, err := parseIntimacyInvitationPayload(`{"status":"accepted" "reply":"好。"}`)
	if err != nil {
		t.Fatalf("a missing comma became a technical failure: %v", err)
	}
	if payload.Status != string(character.IntimacyAccepted) {
		t.Fatalf("status = %q, want the person's own answer", payload.Status)
	}
	if payload.Reply != "好。" {
		t.Fatalf("reply = %q, want the words they said", payload.Reply)
	}
}

// TestIntimacyRoundPayloadRepairsAMissingComma is the same rule for a round.
func TestIntimacyRoundPayloadRepairsAMissingComma(t *testing.T) {
	payload, err := parseIntimacyRoundPayload(`{"lines":[{"author_id":"lin" "text":"门关上了。"}]}`, []string{"lin"})
	if err != nil {
		t.Fatalf("a missing comma became a technical failure: %v", err)
	}
	if len(payload.Lines) != 1 || payload.Lines[0].Text != "门关上了。" {
		t.Fatalf("lines = %+v, want the one line that was said", payload.Lines)
	}
}

// TestIntimacyInvitationGroupKeepsAnAllDeclinedInvitationWritable is §3.3's// "若所有人拒绝则不能开始" at the write side. The refusal has to be recordable:
// the invitation space it was spoken in must still be a valid group, or the
// whole transaction fails and a real answer becomes a technical error.
func TestIntimacyInvitationGroupKeepsAnAllDeclinedInvitationWritable(t *testing.T) {
	activity := character.IntimacyActivity{
		ID: "int_2", StoryID: "st", BranchID: "main", Phase: character.IntimacyInviting,
		Participants: []character.IntimacyParticipant{
			{CharacterID: "bai", Status: character.IntimacyDeclined, Adult: true},
		},
	}.Normalize()

	group := intimacyInvitationGroup(activity, character.State{}, "bai", "第一日")
	if err := group.Validate(); err != nil {
		t.Fatalf("the declined answer has no space to be recorded in: %v", err)
	}
	if !character.ContainsID(group.ActiveMembers(), "bai") {
		t.Fatalf("group members = %v, want the person whose answer this is", group.ActiveMembers())
	}
	if group.ID != activity.SpaceID() {
		t.Fatalf("group id = %q, want the invitation parley %q", group.ID, activity.SpaceID())
	}
}

// TestIntimacyInvitationGroupDropsADeclinedPersonFromLaterParley is the other
// half of the same rule: once their answer is in, a no ends their part, so a
// later person's answer is not spoken in front of them.
func TestIntimacyInvitationGroupDropsADeclinedPersonFromLaterParley(t *testing.T) {
	activity := character.IntimacyActivity{
		ID: "int_3", StoryID: "st", BranchID: "main", Phase: character.IntimacyInviting,
		Participants: []character.IntimacyParticipant{
			{CharacterID: "bai", Status: character.IntimacyDeclined, Adult: true},
			{CharacterID: "shen", Status: character.IntimacyAccepted, Adult: true},
		},
	}.Normalize()

	group := intimacyInvitationGroup(activity, character.State{}, "shen", "第一日")
	if err := group.Validate(); err != nil {
		t.Fatalf("the parley is not writable: %v", err)
	}
	members := group.ActiveMembers()
	if character.ContainsID(members, "bai") {
		t.Fatalf("group members = %v, want the declined person left out of later parley", members)
	}
	if !character.ContainsID(members, "shen") {
		t.Fatalf("group members = %v, want the person who accepted", members)
	}
}

// the read side. A person invited into a running activity has not consented, so
// the room's conversation must not be handed to them and their own words must
// not be shown as part of it.
func TestIntimacyWindowKeepsAPendingInviteOutOfTheRoom(t *testing.T) {
	state := intimacyWindowState()
	activity, ok := state.IntimacyActivityFor("int_1")
	if !ok {
		t.Fatal("the running activity is missing")
	}

	room := state.GroupExchanges(activity.SpaceID())
	if len(room) != 2 {
		t.Fatalf("room = %d lines, want the 2 spoken in it", len(room))
	}
	for _, exchange := range room {
		if exchange.GroupID == activity.InvitationSpaceID("bai") {
			t.Fatalf("the room read returned the invitation line: %+v", exchange)
		}
	}

	// The pending invitee's memory does not contain the room either: the
	// audience captured at commit is what decides this, not their being invited.
	for _, exchange := range state.ExchangesFor("bai") {
		if exchange.GroupID == activity.SpaceID() {
			t.Fatalf("a person who has not consented can read the room: %+v", exchange)
		}
	}
	// And the people in the room never received the invitation line.
	for _, exchange := range state.ExchangesFor("shen") {
		if exchange.GroupID == activity.InvitationSpaceID("bai") {
			t.Fatalf("the room received a pending invitee's private answer: %+v", exchange)
		}
	}
}

// TestIntimacyWindowRosterShowsOnlyRealParticipants is §3.3's "名单须显示实际
// 姓名". The roster is who is actually in the room, and the names are the ones
// the player knows them by.
func TestIntimacyWindowRosterShowsOnlyRealParticipants(t *testing.T) {
	state := intimacyWindowState()
	activity, _ := state.IntimacyActivityFor("int_1")

	group, ok := state.Group(activity.GroupID)
	if !ok {
		t.Fatal("the running activity's space is missing")
	}
	roster := group.ActiveMembers()
	if len(roster) != 2 || !character.ContainsID(roster, "lin") || !character.ContainsID(roster, "shen") {
		t.Fatalf("roster = %v, want exactly the two who agreed", roster)
	}
	if character.ContainsID(roster, "bai") {
		t.Fatalf("roster = %v, want the pending invitee left out", roster)
	}
	names := make(map[string]string, len(roster))
	for _, id := range roster {
		profile, ok := state.Profile(id)
		if !ok {
			t.Fatalf("%q is on the roster but has no profile", id)
		}
		names[id] = profile.DisplayName()
	}
	if names["lin"] != "林晚" || names["shen"] != "沈青" {
		t.Fatalf("roster names = %v, want the real names", names)
	}
}

// TestIntimacyWindowKeepsFinishedActivitiesReadable is the restore rule: a
// finished activity keeps its record and its lines, so re-opening the window
// reads history instead of replaying anything.
func TestIntimacyWindowKeepsFinishedActivitiesReadable(t *testing.T) {
	state := intimacyWindowState()
	finished, ok := state.IntimacyActivityFor("int_0")
	if !ok {
		t.Fatal("the finished activity is missing")
	}
	if finished.Phase != character.IntimacyEnded {
		t.Fatalf("phase = %q, want ended", finished.Phase)
	}
	if roster := finished.ConsentedRoster(); len(roster) != 1 {
		t.Fatalf("ConsentedRoster = %v, want the record of who took part", roster)
	}
	// The activity is no longer offered as something to resume.
	for _, open := range state.OpenIntimacyActivities() {
		if open.ID == finished.ID {
			t.Fatalf("a finished activity is still offered as open: %+v", open)
		}
	}
}

// TestIntimacyActivityIDDoesNotReuseAFinishedRecord is the second-invitation
// rule: once an activity has ended, asking the same person again opens a new
// activity. An id keyed only on the person names the finished record, and the
// new answer is then written onto it — the window keeps reporting "ended" and
// the invitation the player just made leaves no trace.
func TestIntimacyActivityIDDoesNotReuseAFinishedRecord(t *testing.T) {
	finished := character.IntimacyActivity{
		ID: "itm_finished", StoryID: "st", BranchID: "main", Phase: character.IntimacyEnded,
		Participants: []character.IntimacyParticipant{{CharacterID: "lin", Status: character.IntimacyDeclined}},
	}
	first := newIntimacyActivityID("st", "main", "lin", nil)
	second := newIntimacyActivityID("st", "main", "lin", []character.IntimacyActivity{finished})
	if first == second {
		t.Fatalf("a later invitation reused activity id %q", first)
	}
	// A retried request reads the same number, so it must still resolve to the
	// activity its first attempt would have opened.
	if again := newIntimacyActivityID("st", "main", "lin", []character.IntimacyActivity{finished}); again != second {
		t.Fatalf("a retry minted a different id: %q then %q", second, again)
	}
	if other := newIntimacyActivityID("st", "main", "shen", []character.IntimacyActivity{finished}); other == second {
		t.Fatalf("two different people were given the same activity id %q", other)
	}
}

// TestIntimacyAcceptanceOpensAndKeepsItsRecord walks the one path F12's loop
// needs end to end: an invitation is answered yes, the player confirms the
// roster they were shown, the room runs, and closing it keeps every line and
// every answer readable.
//
// A yes is not consent to start. It only makes the activity ready; the
// confirmation is what opens the room, and it opens with exactly the roster
// that agreed — someone who declined stays on the record as asked and does not
// enter.
func TestIntimacyAcceptanceOpensAndKeepsItsRecord(t *testing.T) {
	activity := character.IntimacyActivity{
		ID: "itm_1", StoryID: "st", BranchID: "main", Phase: character.IntimacySelecting,
		Participants: []character.IntimacyParticipant{
			{CharacterID: "lin", Status: character.IntimacyPending, Adult: true},
			{CharacterID: "shen", Status: character.IntimacyPending, Adult: true},
		},
	}.Normalize()

	answered := activity.
		WithResponse("lin", character.IntimacyAccepted, "", "春日，午后").
		WithResponse("shen", character.IntimacyDeclined, "", "春日，午后")

	if answered.Phase != character.IntimacyReady {
		t.Fatalf("phase after one yes = %q, want ready (the player's confirmation is still owed)", answered.Phase)
	}
	if roster := answered.ConsentedRoster(); len(roster) != 1 || roster[0] != "lin" {
		t.Fatalf("ConsentedRoster = %v, want exactly the one who agreed", roster)
	}

	opened, err := answered.Confirm("itm_1:space", "春日，傍晚", 0)
	if err != nil {
		t.Fatalf("confirming an accepted invitation: %v", err)
	}
	if opened.Phase != character.IntimacyActive {
		t.Fatalf("phase after confirm = %q, want active", opened.Phase)
	}
	if len(opened.Scene.Present) != 1 || opened.Scene.Present[0] != "lin" {
		t.Fatalf("room = %v, want only the person who agreed", opened.Scene.Present)
	}
	// The refusal is still on the record: the activity says who was asked.
	if participant, ok := opened.Participant("shen"); !ok || participant.Status != character.IntimacyDeclined {
		t.Fatalf("the declined invitee was dropped from the record: %+v", opened.Participants)
	}

	ended := opened.End("春日，入夜")
	if ended.Phase != character.IntimacyEnded {
		t.Fatalf("phase after end = %q, want ended", ended.Phase)
	}
	if roster := ended.ConsentedRoster(); len(roster) != 1 || roster[0] != "lin" {
		t.Fatalf("a finished activity lost who took part: %v", roster)
	}
	if ended.OpenedWorldTime != "春日，傍晚" || ended.EndedWorldTime != "春日，入夜" {
		t.Fatalf("the finished activity lost its times: opened=%q ended=%q", ended.OpenedWorldTime, ended.EndedWorldTime)
	}
}

// TestIntimacyConditionalIsNotConsent is the boundary the window must never
// cross: a conditional yes is a yes only once its condition is met, and nothing
// in this path settles conditions. A room must not open on one.
func TestIntimacyConditionalIsNotConsent(t *testing.T) {
	activity := character.IntimacyActivity{
		ID: "itm_2", StoryID: "st", BranchID: "main", Phase: character.IntimacySelecting,
		Participants: []character.IntimacyParticipant{
			{CharacterID: "lin", Status: character.IntimacyPending, Adult: true},
		},
	}.Normalize()

	answered := activity.WithResponse("lin", character.IntimacyConditional, "等她把那句话说清", "春日，午后")
	if answered.Phase != character.IntimacyEnded {
		t.Fatalf("phase after a conditional answer = %q, want ended: a condition is not a yes", answered.Phase)
	}
	if roster := answered.ConsentedRoster(); len(roster) != 0 {
		t.Fatalf("ConsentedRoster = %v, want nobody", roster)
	}
	if _, err := answered.Confirm("itm_2:space", "春日，傍晚", 0); err == nil {
		t.Fatal("a room opened on a conditional answer")
	}
}

// The invitation is spoken to one person, and what reaches them is the model's
// own prompt. A raw actor id there is a name the story never gave anyone: the
// person being asked would be told "lin" answered yes rather than that 林晚 did.
// 名单须显示实际姓名 (自制-双修窗口 §3.3).
func TestInvitationQuestionNamesPeopleTheWayTheStoryDoes(t *testing.T) {
	state := intimacyWindowState()
	activity, ok := state.IntimacyActivityFor("int_1")
	if !ok {
		t.Fatal("fixture lost its running activity")
	}
	question := intimacyInvitationQuestion(activity, state, "bai")
	for _, raw := range []string{"lin", "shen"} {
		if strings.Contains(question, raw) {
			t.Fatalf("the invitation names %q instead of the person: %s", raw, question)
		}
	}
	for _, name := range []string{"林晚", "沈青"} {
		if !strings.Contains(question, name) {
			t.Fatalf("the invitation does not name %q: %s", name, question)
		}
	}
}

// The roster note is read by the model as "how each of them answered", so it has
// to be an answer rather than the storage value. 界面逐人展示状态, and the design
// names those states in words. An English enum in the prompt is a protocol word,
// and the prompt's own built-in forbids carrying IDs, schemas or internal
// instructions into prose, so the note states them the way a person would hear
// them instead.
func TestActivityNoteStatesEachAnswerInWords(t *testing.T) {
	state := intimacyWindowState()
	activity, ok := state.IntimacyActivityFor("int_1")
	if !ok {
		t.Fatal("fixture lost its running activity")
	}
	note := intimacyActivityNote(activity, state, "lin")
	for _, raw := range []string{"accepted", "pending", "declined", "conditional"} {
		if strings.Contains(note, raw) {
			t.Fatalf("the roster note carries the storage value %q: %s", raw, note)
		}
	}
	if !strings.Contains(note, "沈青") || !strings.Contains(note, "has agreed") {
		t.Fatalf("the person who agreed is not stated as agreeing: %s", note)
	}
	// An unanswered person is not a refusal. 邀请中未回应者保持未回应，模型超时或
	// 连接失败保持技术错误；两者都不等于拒绝 (§3.3).
	if !strings.Contains(note, "白露") || !strings.Contains(note, "has not answered yet") {
		t.Fatalf("the person who has not answered is not stated as unanswered: %s", note)
	}
}

// The note is about who is in the invitation and how it stands, not a transcript
// of what was said to someone else. §13.1 keeps 一人的同意不代表其他人同意, and a
// condition is the asker's own business: 丙提出先结束眼前事务 is a thing said to
// the protagonist, not a fact handed to the next person being asked.
func TestActivityNoteDoesNotCarrySomeoneElsesCondition(t *testing.T) {
	state := intimacyWindowState()
	activity, ok := state.IntimacyActivityFor("int_1")
	if !ok {
		t.Fatal("fixture lost its running activity")
	}
	activity = activity.WithResponse("shen", character.IntimacyConditional, "先结束眼前事务", "")
	note := intimacyActivityNote(activity, state, "bai")
	if strings.Contains(note, "先结束眼前事务") || strings.Contains(note, "condition=") {
		t.Fatalf("the note hands over someone else's condition: %s", note)
	}
	// The answer itself still has to be visible, or a conditional yes would read
	// as an unqualified one: 条件未满足仍是待商议，不能按已同意处理 (§13.1).
	if !strings.Contains(note, "沈青") || !strings.Contains(note, "condition") {
		t.Fatalf("a conditional answer is not stated as conditional: %s", note)
	}
}

// The person being asked must be able to tell the question is theirs to answer.
// 邀请中未回应者保持未回应, and nobody has agreed on their behalf: telling them the
// room is settled would answer the question for them (§3.1: 不能强制同意).
func TestInvitationQuestionAsksWithoutAnsweringForThem(t *testing.T) {
	state := intimacyWindowState()
	activity, ok := state.IntimacyActivityFor("int_1")
	if !ok {
		t.Fatal("fixture lost its running activity")
	}
	question := strings.ToLower(intimacyInvitationQuestion(activity, state, "bai"))
	for _, leaked := range []string{"declined", "refused", "everyone has agreed", "all agreed", "already agreed"} {
		if strings.Contains(question, leaked) {
			t.Fatalf("the question answers for the person or claims agreement: %s", question)
		}
	}
	if !strings.Contains(question, "whether you agree") {
		t.Fatalf("the question does not actually ask them: %s", question)
	}
}

// The adult check follows the story's own clock, not the number the pack was
// written with. §2 of 自制-时间寿元与长时世界推进 says 年龄按实际游戏时间增加, and
// the cast block already ages everyone that way. Reading the frozen number alone
// left a person authored at 17 permanently barred from every window no matter how
// many years the story then spent, which contradicts the same clock the rest of
// the game reads.
func TestAnAdultIsJudgedByTheAgeTheStoryReached(t *testing.T) {
	storyContext := interactive.StoryContext{
		Meta: interactive.StoryMeta{PackCatalog: &interactive.PackCatalogSnapshot{
			Characters: []interactive.PackCatalogPerson{
				{SourceKey: "npc_young", Name: "少年", Age: 17},
				{SourceKey: "npc_grown", Name: "长者", Age: 30},
			},
		}},
	}
	perYear := int64(interactive.DaysPerRuleYear * interactive.StoryCalendarHoursPerDay * 60 * 60)

	if adult := intimacyProfileIsAdult(
		character.Profile{ID: "young", SourceKey: "npc_young"},
		intimacyAdultAges(storyContext, ""),
	); adult {
		t.Fatal("a person the story has not yet aged past the bar is already admitted")
	}

	storyContext.Snapshot.Clock = &interactive.StoryClock{Seconds: 2 * perYear}
	if adult := intimacyProfileIsAdult(
		character.Profile{ID: "young", SourceKey: "npc_young"},
		intimacyAdultAges(storyContext, ""),
	); !adult {
		t.Fatal("two years have passed in the story and the person is still counted as 17")
	}

	// The same reading must not age somebody past the bar on a partial year:
	// 显示整岁不丢失底层已过时长, so a remainder of months is kept on the clock and
	// never rounds an age up.
	storyContext.Snapshot.Clock = &interactive.StoryClock{Seconds: perYear/2 + 10*perYear}
	ages := intimacyAdultAges(storyContext, "")
	if ages["npc_young"] != 17+10 {
		t.Fatalf("age = %d after ten and a half years, want the whole years only", ages["npc_young"])
	}
	if ages["npc_grown"] != 30+10 {
		t.Fatalf("age = %d after ten and a half years, want the whole years only", ages["npc_grown"])
	}

	// A person whose body states no age keeps stating none: adding years to
	// "unknown" would invent a number rather than age a real one.
	storyContext.Meta.PackCatalog.Characters = append(
		storyContext.Meta.PackCatalog.Characters,
		interactive.PackCatalogPerson{SourceKey: "npc_unknown", Name: "无名"},
	)
	ages = intimacyAdultAges(storyContext, "")
	if ages["npc_unknown"] != 0 {
		t.Fatalf("an unstated age became %d", ages["npc_unknown"])
	}
	if intimacyProfileIsAdult(character.Profile{ID: "unknown", SourceKey: "npc_unknown"}, ages) {
		t.Fatal("a person with no authored age is admitted")
	}
}
