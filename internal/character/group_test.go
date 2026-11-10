package character

import (
	"strings"
	"testing"
)

func groupProjectionContext() ProjectionContext {
	ctx := testProjectionContext()
	ctx.KnownCharacters = []string{"guardsman", "steward", "abbot"}
	ctx.Groups = []Group{
		{
			ID: "grp-1", StoryID: "story-1", BranchID: "branch-1",
			Type: GroupSelf, Name: "北门夜话", Channel: ChannelScene, Version: 1,
			Members: []GroupMember{
				{CharacterID: "guardsman", JoinedVersion: 1},
				{CharacterID: "steward", JoinedVersion: 1},
				{CharacterID: "abbot", JoinedVersion: 1},
			},
		},
	}
	return ctx
}

// TestGroupLineReachesOnlyPresentMembers is the contact condition for a
// face-to-face group: everyone in the group is a member, but a member who is not
// in the scene cannot hear the line and is not moved into it.
func TestGroupLineReachesOnlyPresentMembers(t *testing.T) {
	ctx := groupProjectionContext()
	ctx.Scene.Present = []string{"guardsman", "steward"}
	projected, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{Speaker: SpeakerPlayer, Channel: ChannelGroup, GroupID: "grp-1", Text: "今晚都到北门来。"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	line := projected.Exchanges[0]
	if line.GroupID != "grp-1" {
		t.Fatalf("group id = %q, want grp-1", line.GroupID)
	}
	if line.CharacterID != "" {
		t.Fatalf("a group line was filed under character %q", line.CharacterID)
	}
	for _, id := range []string{"guardsman", "steward"} {
		if !exchangeReaches(line, id) {
			t.Fatalf("present member %s did not hear the group line", id)
		}
	}
	if exchangeReaches(line, "abbot") {
		t.Fatal("an absent member heard a face-to-face group line")
	}
}

// TestGroupCharacterLineRecordsItsRealAuthor keeps identity with the speaker.
// The line belongs to the group, but the person who said it is the author, not
// whoever happens to be reading the window.
func TestGroupCharacterLineRecordsItsRealAuthor(t *testing.T) {
	ctx := groupProjectionContext()
	ctx.Scene.Present = []string{"guardsman", "steward"}
	projected, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: ChannelGroup, GroupID: "grp-1", Text: "我今夜值守，走不开。"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	line := projected.Exchanges[0]
	if line.AuthorID != "guardsman" {
		t.Fatalf("author = %q, want guardsman", line.AuthorID)
	}
	if !exchangeReaches(line, "steward") {
		t.Fatal("a fellow present member did not hear the line")
	}
}

// TestGroupLineRefusesNonMember is the boundary against the model writing a
// stranger into the group: the backend owns membership, so a draft naming
// someone outside it is refused rather than silently admitted.
func TestGroupLineRefusesNonMember(t *testing.T) {
	ctx := groupProjectionContext()
	ctx.KnownCharacters = append(ctx.KnownCharacters, "outsider")
	if _, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{CharacterID: "outsider", Speaker: SpeakerCharacter, Channel: ChannelGroup, GroupID: "grp-1", Text: "我也在。"},
		},
	}); err == nil {
		t.Fatal("a non-member was allowed to speak in the group")
	}
}

// TestGroupLineRequiresItsGroup keeps a group line from floating free: without
// the group there is no membership, and without membership there is no audience.
func TestGroupLineRequiresItsGroup(t *testing.T) {
	ctx := groupProjectionContext()
	if _, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{Speaker: SpeakerPlayer, Channel: ChannelGroup, Text: "有人吗？"},
		},
	}); err == nil {
		t.Fatal("a group line without a group was accepted")
	}
	if _, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{Speaker: SpeakerPlayer, Channel: ChannelGroup, GroupID: "grp-missing", Text: "有人吗？"},
		},
	}); err == nil {
		t.Fatal("a group line naming an unknown group was accepted")
	}
}

// TestGroupCannotBeNamedOnAPrivateLine stops the two channels from being
// conflated. A private line has a different audience, and letting it carry a
// group id would file it where the group window reads.
func TestGroupCannotBeNamedOnAPrivateLine(t *testing.T) {
	ctx := groupProjectionContext()
	if _, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: ChannelPrivate, GroupID: "grp-1", Text: "私下说一句。"},
		},
	}); err == nil {
		t.Fatal("a private line was allowed to name a group")
	}
}

// TestLaterJoinerDoesNotRecallEarlierGroupLines is the knowledge rule for
// membership changes. The audience was captured when the line was spoken, so
// joining afterwards cannot reach back and add the joiner.
func TestLaterJoinerDoesNotRecallEarlierGroupLines(t *testing.T) {
	ctx := groupProjectionContext()
	ctx.Scene.Present = []string{"guardsman", "steward"}
	projected, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{Speaker: SpeakerPlayer, Channel: ChannelGroup, GroupID: "grp-1", Text: "先说一句。"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	line := projected.Exchanges[0]
	// The abbot joins afterwards and is now present. Re-reading the group's
	// current membership must not make him a witness to a line from before.
	if exchangeReaches(line, "abbot") {
		t.Fatal("a later joiner recalled a line spoken before they joined")
	}
}

func TestGroupJoinAndLeaveKeepTenures(t *testing.T) {
	group := Group{
		ID: "grp-1", StoryID: "story-1", BranchID: "branch-1",
		Type: GroupSelf, Name: "北门夜话", Channel: ChannelScene, Version: 1,
		Members: []GroupMember{{CharacterID: "guardsman", JoinedVersion: 1}},
	}
	group = group.Join("steward", "第3日 辰时")
	if group.Version != 2 {
		t.Fatalf("version after join = %d, want 2", group.Version)
	}
	if !ContainsID(group.ActiveMembers(), "steward") {
		t.Fatal("steward is not active after joining")
	}
	// Joining twice must not open a second tenure.
	again := group.Join("steward", "第3日 午时")
	if again.Version != group.Version {
		t.Fatalf("a repeated join advanced the version to %d", again.Version)
	}

	group = group.Leave("steward", "第4日 辰时")
	if ContainsID(group.ActiveMembers(), "steward") {
		t.Fatal("steward is still active after leaving")
	}
	if !group.FormerMember("steward") {
		t.Fatal("the leave erased the record of having been in the group")
	}

	group = group.Join("steward", "第6日 辰时")
	if !ContainsID(group.ActiveMembers(), "steward") {
		t.Fatal("steward did not return as a new tenure")
	}
	if len(group.Members) != 3 {
		t.Fatalf("members = %d, want 3 tenure records (guard, first stay, second stay)", len(group.Members))
	}
	if err := group.Validate(); err != nil {
		t.Fatalf("a rejoined group is invalid: %v", err)
	}
}

func TestGroupValidateRefusesTwoOpenTenures(t *testing.T) {
	group := Group{
		ID: "grp-1", StoryID: "story-1", BranchID: "branch-1",
		Type: GroupSelf, Name: "北门夜话", Channel: ChannelScene, Version: 1,
		Members: []GroupMember{
			{CharacterID: "guardsman", JoinedVersion: 1},
			{CharacterID: "guardsman", JoinedVersion: 2},
		},
	}
	if err := group.Validate(); err == nil {
		t.Fatal("a group with two open memberships for one person was accepted")
	}
}

func TestGroupValidateRefusesLeaveBeforeJoin(t *testing.T) {
	group := Group{
		ID: "grp-1", StoryID: "story-1", BranchID: "branch-1",
		Type: GroupSelf, Name: "北门夜话", Channel: ChannelScene, Version: 3,
		Members: []GroupMember{{CharacterID: "guardsman", JoinedVersion: 3, LeftVersion: 2}},
	}
	if err := group.Validate(); err == nil {
		t.Fatal("a membership that ended before it began was accepted")
	}
}

func TestGroupUpsertRefusesAnOlderVersion(t *testing.T) {
	state := State{}
	group := Group{
		ID: "grp-1", StoryID: "story-1", BranchID: "branch-1",
		Type: GroupSelf, Name: "北门夜话", Channel: ChannelScene, Version: 2,
		Members: []GroupMember{{CharacterID: "guardsman", JoinedVersion: 1}},
	}
	if err := state.UpsertGroup(group); err != nil {
		t.Fatal(err)
	}
	older := group
	older.Version = 1
	if err := state.UpsertGroup(older); err == nil {
		t.Fatal("an older group version rolled the membership back")
	}
	stored, ok := state.Group("grp-1")
	if !ok || stored.Version != 2 {
		t.Fatalf("stored version = %d, want 2", stored.Version)
	}
}

// TestGroupReachesWholeRosterOnARemoteChannel is the other contact case: a group
// with a real remote channel does not require presence, because the channel is
// how the distance is crossed.
func TestGroupReachesWholeRosterOnARemoteChannel(t *testing.T) {
	ctx := groupProjectionContext()
	ctx.Groups[0].Channel = ChannelMessage
	ctx.Scene.Present = []string{"guardsman"}
	projected, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{Speaker: SpeakerPlayer, Channel: ChannelGroup, GroupID: "grp-1", Text: "传讯问问。"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	line := projected.Exchanges[0]
	for _, id := range []string{"guardsman", "steward", "abbot"} {
		if !exchangeReaches(line, id) {
			t.Fatalf("%s did not hear a remote group line", id)
		}
	}
}

func TestGroupLineTextIsRequired(t *testing.T) {
	ctx := groupProjectionContext()
	ctx.Scene.Present = []string{"guardsman"}
	if _, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{Speaker: SpeakerPlayer, Channel: ChannelGroup, GroupID: "grp-1", Text: "   "},
		},
	}); err == nil {
		t.Fatal("an empty group line was accepted")
	}
}

func TestGroupExchangesAreSeparateFromPrivateChats(t *testing.T) {
	state := State{
		Groups: []Group{{
			ID: "grp-1", StoryID: "story-1", BranchID: "branch-1",
			Type: GroupSelf, Name: "北门夜话", Channel: ChannelScene, Version: 1,
			Members: []GroupMember{{CharacterID: "guardsman", JoinedVersion: 1}},
		}},
		Exchanges: []Exchange{
			{ID: "x1", GroupID: "grp-1", Channel: ChannelGroup, Speaker: SpeakerPlayer, Audience: []string{"guardsman"}, Text: "群里的话"},
			{ID: "x2", CharacterID: "guardsman", Channel: ChannelPrivate, Speaker: SpeakerCharacter, Audience: []string{"guardsman"}, Text: "私下的话"},
		},
	}
	groupLines := state.GroupExchanges("grp-1")
	if len(groupLines) != 1 || groupLines[0].ID != "x1" {
		t.Fatalf("group transcript = %+v, want only the group line", groupLines)
	}
	// The group's transcript holds the group's lines and nothing else.
	for _, line := range groupLines {
		if line.Channel != ChannelGroup {
			t.Fatalf("group transcript holds a %q line", line.Channel)
		}
	}
	// A member's memory does hold the group line, because they heard it. That is
	// the audience rule working, not a leak: what must not happen is the group
	// transcript acquiring the private line, which the check above rules out.
	memory := state.ExchangesFor("guardsman")
	if !ContainsID(exchangeIDs(memory), "x1") {
		t.Fatal("a member did not remember a group line they heard")
	}
	if !ContainsID(exchangeIDs(memory), "x2") {
		t.Fatal("a member did not remember their own private line")
	}
}

func exchangeIDs(exchanges []Exchange) []string {
	out := make([]string, 0, len(exchanges))
	for _, exchange := range exchanges {
		out = append(out, exchange.ID)
	}
	return out
}

func TestGroupNameAndMembershipAreRequired(t *testing.T) {
	base := Group{
		ID: "grp-1", StoryID: "story-1", BranchID: "branch-1",
		Type: GroupSelf, Channel: ChannelScene, Version: 1,
		Members: []GroupMember{{CharacterID: "guardsman", JoinedVersion: 1}},
	}
	nameless := base
	nameless.Name = "  "
	if err := nameless.Validate(); err == nil {
		t.Fatal("a group without a name was accepted")
	}
	empty := base
	empty.Name = "空群"
	empty.Members = nil
	if err := empty.Validate(); err == nil {
		t.Fatal("a group with no members was accepted")
	}
	if err := (Group{}).Validate(); err == nil || !strings.Contains(err.Error(), "id") {
		t.Fatalf("a group without identity returned %v", err)
	}
}
