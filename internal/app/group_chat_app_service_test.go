package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ta/internal/character"
)

func groupRoundState() character.State {
	return character.State{
		Version: character.StateVersion,
		Profiles: []character.Profile{
			{ID: "a-gui", StoryID: "st", Name: "阿贵", Role: "门房"},
			{ID: "su-yue", StoryID: "st", Name: "苏月", Role: "药铺伙计"},
			{ID: "old-zhou", StoryID: "st", Name: "周老", Role: "账房"},
		},
		Groups: []character.Group{{
			ID: "grp-1", StoryID: "st", BranchID: "main",
			Type: character.GroupSelf, Name: "北门夜话",
			Channel: character.ChannelScene, Version: 1,
			Members: []character.GroupMember{
				{CharacterID: "a-gui", JoinedVersion: 1},
				{CharacterID: "su-yue", JoinedVersion: 1},
				{CharacterID: "old-zhou", JoinedVersion: 1},
			},
		}},
		Exchanges: []character.Exchange{
			{
				ID: "xch_1", StoryID: "st", BranchID: "main",
				Speaker: character.SpeakerPlayer, Channel: character.ChannelGroup,
				GroupID: "grp-1", Text: "今晚都到北门来。",
				Audience: []string{"a-gui", "su-yue", "old-zhou"},
			},
			{
				ID: "xch_2", StoryID: "st", BranchID: "main",
				CharacterID: "a-gui", AuthorID: "a-gui",
				Speaker: character.SpeakerCharacter, Channel: character.ChannelGroup,
				GroupID: "grp-1", Text: "我今夜值守，走不开。",
				Audience: []string{"a-gui", "su-yue", "old-zhou"},
			},
		},
		Revision: 1,
	}
}

func groupRoundScene() character.Scene {
	return character.Scene{
		NodeID: "north-gate", Title: "北门", Location: "北门",
		Present: []string{"a-gui", "su-yue"},
	}
}

// TestGroupRoundReachesOnlyPresentMembersForAFaceToFaceGroup is the contact rule
// at the application layer: the roster is three people, the scene holds two, and
// the third is reported as unreachable rather than silently dropped or made to
// answer from somewhere else.
func TestGroupRoundReachesOnlyPresentMembersForAFaceToFaceGroup(t *testing.T) {
	state := groupRoundState()
	group, _ := state.Group("grp-1")
	scene := groupRoundScene()

	if !groupMemberReachable(group, scene, "a-gui") {
		t.Fatal("a present member was reported unreachable")
	}
	if groupMemberReachable(group, scene, "old-zhou") {
		t.Fatal("an absent member was reported reachable for a face-to-face group")
	}
}

// TestGroupWindowReportsReachabilityPerMember is what the window draws: each
// member carries whether they can hear right now, so the player is told who is
// out of earshot instead of assuming the whole group heard.
func TestGroupWindowReportsReachabilityPerMember(t *testing.T) {
	state := groupRoundState()
	group, _ := state.Group("grp-1")
	row := groupWindowGroup(state, group, groupRoundScene(), pictureSources{})

	if row.ReachableMembers != 2 {
		t.Fatalf("reachable members = %d, want 2", row.ReachableMembers)
	}
	if len(row.Members) != 3 {
		t.Fatalf("roster = %d, want the whole active membership", len(row.Members))
	}
	byID := make(map[string]GroupWindowMember, len(row.Members))
	for _, member := range row.Members {
		byID[member.ID] = member
	}
	if !byID["a-gui"].Present || !byID["a-gui"].Reachable {
		t.Fatalf("a-gui row = %+v, want present and reachable", byID["a-gui"])
	}
	if byID["old-zhou"].Present || byID["old-zhou"].Reachable {
		t.Fatalf("old-zhou row = %+v, want absent and unreachable", byID["old-zhou"])
	}
	if row.LastExchange != "我今夜值守，走不开。" {
		t.Fatalf("last exchange = %q", row.LastExchange)
	}
}

// TestGroupJointHistoryIsTheIntersection is the knowledge boundary for a round.
// A line that only some of the listeners heard is not joint material: giving it
// to the round would tell the model something the others did not witness.
func TestGroupJointHistoryIsTheIntersection(t *testing.T) {
	state := groupRoundState()
	group, _ := state.Group("grp-1")
	state.Exchanges = append(state.Exchanges, character.Exchange{
		ID: "xch_3", StoryID: "st", BranchID: "main",
		CharacterID: "a-gui", AuthorID: "a-gui",
		Speaker: character.SpeakerCharacter, Channel: character.ChannelGroup,
		GroupID: "grp-1", Text: "这话只对苏月说。",
		Audience: []string{"a-gui", "su-yue"},
	})

	joint := groupJointHistory(state, group, []string{"a-gui", "su-yue"})
	if !strings.Contains(joint, "今晚都到北门来") {
		t.Fatalf("joint history dropped a line everyone heard:\n%s", joint)
	}
	if !strings.Contains(joint, "这话只对苏月说") {
		t.Fatalf("joint history dropped a line both listeners heard:\n%s", joint)
	}

	// Now the third member is reachable too. The line only two of them heard must
	// leave the joint block, because the new listener was not its audience.
	joint = groupJointHistory(state, group, []string{"a-gui", "su-yue", "old-zhou"})
	if strings.Contains(joint, "这话只对苏月说") {
		t.Fatalf("joint history handed one member's private remark to the whole group:\n%s", joint)
	}
}

// TestGroupRoundPayloadDropsUnreachableSpeakers is the boundary against the
// model widening the audience. A line naming someone unreachable is dropped, and
// a round with nothing left fails rather than inventing a reply.
func TestGroupRoundPayloadDropsUnreachableSpeakers(t *testing.T) {
	raw := `{"lines":[{"author_id":"old-zhou","text":"我听见了。"}],"matters":[]}`
	if _, err := parseGroupRoundPayload(raw, []string{"a-gui", "su-yue"}); err == nil {
		t.Fatal("a round spoken entirely by an unreachable member was accepted")
	}

	raw = `{"lines":[{"author_id":"a-gui","text":"我来。"},{"author_id":"old-zhou","text":"我也在。"}],"matters":[]}`
	payload, err := parseGroupRoundPayload(raw, []string{"a-gui", "su-yue"})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Lines) != 1 || payload.Lines[0].AuthorID != "a-gui" {
		t.Fatalf("lines = %+v, want only the reachable speaker", payload.Lines)
	}
}

// TestGroupRoundPayloadCapsSpeakers keeps a round from turning into a roll call.
// The design requires the relevant few, never the whole roster taking turns.
func TestGroupRoundPayloadCapsSpeakers(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"lines":[`)
	ids := []string{"m1", "m2", "m3", "m4", "m5", "m6"}
	for index, id := range ids {
		if index > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"author_id":"` + id + `","text":"说一句。"}`)
	}
	sb.WriteString(`],"matters":[]}`)
	payload, err := parseGroupRoundPayload(sb.String(), ids)
	if err != nil {
		t.Fatal(err)
	}
	speakers := make(map[string]bool)
	for _, line := range payload.Lines {
		speakers[line.AuthorID] = true
	}
	if len(speakers) > 4 {
		t.Fatalf("round had %d speakers, want at most 4", len(speakers))
	}
}

// TestGroupRoundPayloadRejectsDisclosureMatters keeps the private channel's
// business out of a group round. A disclosure is something told to one person;
// recording it here would file a private confidence as group knowledge.
func TestGroupRoundPayloadRejectsDisclosureMatters(t *testing.T) {
	raw := `{"lines":[{"author_id":"a-gui","text":"好。"}],"matters":[` +
		`{"character_id":"a-gui","kind":"disclosure","statement":"他其实是官府的人。"},` +
		`{"character_id":"a-gui","kind":"promise","statement":"今夜我去北门。"}]}`
	payload, err := parseGroupRoundPayload(raw, []string{"a-gui"})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Matters) != 1 {
		t.Fatalf("matters = %+v, want only the promise", payload.Matters)
	}
	if payload.Matters[0].Kind != "promise" {
		t.Fatalf("kept matter kind = %q, want promise", payload.Matters[0].Kind)
	}
}

// TestGroupRoundPayloadDropsMattersFromNonSpeakers keeps a matter attached to
// someone the round did not reach. A person who did not hear cannot have
// promised anything.
func TestGroupRoundPayloadDropsMattersFromNonSpeakers(t *testing.T) {
	raw := `{"lines":[{"author_id":"a-gui","text":"好。"}],"matters":[` +
		`{"character_id":"old-zhou","kind":"promise","statement":"我去。"}]}`
	payload, err := parseGroupRoundPayload(raw, []string{"a-gui"})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Matters) != 0 {
		t.Fatalf("matters = %+v, want none from an unreachable member", payload.Matters)
	}
}

// TestGroupRoundPayloadStripsAFence accepts a fenced object, because a model
// that ignores the "no fence" instruction still produced a usable answer.
func TestGroupRoundPayloadStripsAFence(t *testing.T) {
	raw := "```json\n{\"lines\":[{\"author_id\":\"a-gui\",\"text\":\"好。\"}],\"matters\":[]}\n```"
	payload, err := parseGroupRoundPayload(raw, []string{"a-gui"})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Lines) != 1 {
		t.Fatalf("lines = %+v", payload.Lines)
	}
}

// TestGroupWindowLeavesAnIntimacyRoomOutOfTheOrdinaryGroupList keeps an
// intimacy room out of the ordinary group list at the read side.
//
// An intimacy activity files its lines in a real group, which is what gives them
// a real audience. That group is not a conversation space the player made or
// joined, so listing it beside their own groups would offer the room's preview
// and membership to anyone browsing the group window.
func TestGroupWindowLeavesAnIntimacyRoomOutOfTheOrdinaryGroupList(t *testing.T) {
	state := intimacyWindowState()
	state.Groups = append(state.Groups, character.Group{
		ID: "grp-1", StoryID: "st", BranchID: "main",
		Type: character.GroupSelf, Name: "渡口三人",
		Channel: character.ChannelScene, Version: 1,
		Members: []character.GroupMember{{CharacterID: "lin", JoinedVersion: 1}},
	})

	groups := groupWindowGroups(state, character.Scene{Present: []string{"lin"}}, pictureSources{})
	ids := make(map[string]bool, len(groups))
	for _, group := range groups {
		ids[group.ID] = true
	}
	if !ids["grp-1"] {
		t.Fatal("the player's own group disappeared from the list")
	}
	for _, hidden := range []string{"int_1:space", "int_1:invite:bai", "int_0:space"} {
		if ids[hidden] {
			t.Fatalf("intimacy space %q was offered as an ordinary group", hidden)
		}
	}
}

func TestGroupCommandHashDistinguishesContent(t *testing.T) {
	first := groupCommandHash("send", "st", "main", "grp-1", "今晚来", nil, nil)
	second := groupCommandHash("send", "st", "main", "grp-1", "今晚别来", nil, nil)
	if first == second {
		t.Fatal("two different messages share a command hash")
	}
	repeat := groupCommandHash("send", "st", "main", "grp-1", "今晚来", nil, nil)
	if first != repeat {
		t.Fatal("the same message produced a different command hash")
	}
	// Membership order must not change the identity of the change.
	a := groupCommandHash("members", "st", "main", "grp-1", "", []string{"a-gui", "su-yue"}, nil)
	b := groupCommandHash("members", "st", "main", "grp-1", "", []string{"su-yue", "a-gui"}, nil)
	if a != b {
		t.Fatal("the same member set produced two command hashes")
	}
}

// An intimacy room is a real group with a real audience, but its membership is
// the roster the participants agreed to, and it changes only through their own
// answers. The group window already refuses to read it. Editing or speaking into
// it here would let the ordinary group surface rewrite who consented, and would
// address the room with the ordinary rules rather than the intimacy ones.
func TestGroupSurfaceRefusesAnIntimacyRoom(t *testing.T) {
	application, storyID := intimacyInviteTestApp(t)
	invited, err := application.InviteIntimacy(context.Background(), IntimacyInviteRequest{
		StoryID: storyID, CharacterID: "chen_qiaoqian", OneOnOne: true, CommandID: "invite-room",
	})
	if err != nil {
		t.Fatalf("a one-to-one invitation: %v", err)
	}
	roomID := invited.Activity.GroupID
	if roomID == "" {
		t.Fatal("the running activity has no room")
	}

	if _, err := application.ChangeGroupMembers(GroupMemberChangeRequest{
		StoryID: storyID, GroupID: roomID, Add: []string{"lin_wan"}, CommandID: "room-members",
	}); !errors.Is(err, ErrGroupUnknown) {
		t.Fatalf("adding someone to an intimacy room = %v, want it refused as an unknown group", err)
	}
	if _, err := application.SendGroupMessage(context.Background(), GroupSendRequest{
		StoryID: storyID, GroupID: roomID, Message: "我进来了。", CommandID: "room-send",
	}); !errors.Is(err, ErrGroupUnknown) {
		t.Fatalf("speaking in an intimacy room = %v, want it refused as an unknown group", err)
	}

	state, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	group, ok := state.Group(roomID)
	if !ok {
		t.Fatalf("the room %q disappeared", roomID)
	}
	for _, member := range group.ActiveMembers() {
		if member == "lin_wan" {
			t.Fatal("the refused change still added a member to the consented roster")
		}
	}
}
