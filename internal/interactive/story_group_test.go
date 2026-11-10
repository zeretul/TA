package interactive

import (
	"strings"
	"testing"

	"ta/internal/character"
)

func groupTestGroup() character.Group {
	return character.Group{
		ID: "grp-1", Type: character.GroupSelf, Name: "北门夜话",
		Channel: character.ChannelScene, Version: 1,
		Members: []character.GroupMember{
			{CharacterID: "guardsman", JoinedVersion: 1, JoinedWorldTime: "第3日 辰时"},
			{CharacterID: "steward", JoinedVersion: 1, JoinedWorldTime: "第3日 辰时"},
		},
	}
}

// TestGroupSurvivesAJournalRebuild is the single-save guarantee for groups: the
// membership and the conversation are both on the story journal, so reopening
// the store reproduces the group rather than losing it. A separate group
// database would be a second source of truth and could disagree with the story.
func TestGroupSurvivesAJournalRebuild(t *testing.T) {
	store, storyID := characterTestStory(t)
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman", "steward"}},
		Profiles: characterTestProfiles(),
		Groups:   []character.Group{groupTestGroup()},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman", "steward"}},
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{
				{Speaker: character.SpeakerPlayer, Channel: character.ChannelGroup, GroupID: "grp-1", Text: "今晚都到北门来。"},
				{CharacterID: "guardsman", Speaker: character.SpeakerCharacter, Channel: character.ChannelGroup, GroupID: "grp-1", Text: "我今夜值守。"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	reopened := NewStore(store.root)
	state, err := reopened.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	group, ok := state.Group("grp-1")
	if !ok {
		t.Fatal("the group did not survive the rebuild")
	}
	if group.Name != "北门夜话" {
		t.Fatalf("group name after reopen = %q", group.Name)
	}
	if len(group.ActiveMembers()) != 2 {
		t.Fatalf("members after reopen = %v", group.ActiveMembers())
	}
	lines := state.GroupExchanges("grp-1")
	if len(lines) != 2 {
		t.Fatalf("group transcript after reopen = %d lines, want 2", len(lines))
	}
	if lines[0].Text != "今晚都到北门来。" || lines[1].Text != "我今夜值守。" {
		t.Fatalf("group transcript order after reopen = %q then %q", lines[0].Text, lines[1].Text)
	}
	if lines[1].AuthorID != "guardsman" {
		t.Fatalf("author after reopen = %q", lines[1].AuthorID)
	}
}

// TestGroupStaysOnItsOwnBranch keeps a group from being shared across branches.
// A group made on a fork is that branch's history; the main branch must not see
// it, and its conversation must not leak either.
func TestGroupStaysOnItsOwnBranch(t *testing.T) {
	store, storyID := characterTestStory(t)
	turn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "进城", Narrative: "主角来到北门。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: turn.ID, Title: "支线"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: branch.ID, WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman", "steward"}},
		Profiles: characterTestProfiles(),
		Groups:   []character.Group{groupTestGroup()},
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{
				{Speaker: character.SpeakerPlayer, Channel: character.ChannelGroup, GroupID: "grp-1", Text: "支线专属的群聊"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	mainState, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := mainState.Group("grp-1"); ok {
		t.Fatal("a group created on a fork appeared on main")
	}
	if len(mainState.GroupExchanges("grp-1")) != 0 {
		t.Fatal("a fork's group conversation leaked into main")
	}

	forkState, err := store.CharacterState(storyID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := forkState.Group("grp-1"); !ok {
		t.Fatal("the fork did not record its own group")
	}
	if len(forkState.GroupExchanges("grp-1")) != 1 {
		t.Fatal("the fork did not record its own group conversation")
	}
}

// TestGroupRoundIsIdempotentOnCommandID is the retry rule: a repeated click or a
// lost response must return what was committed rather than producing a second
// round.
func TestGroupRoundIsIdempotentOnCommandID(t *testing.T) {
	store, storyID := characterTestStory(t)
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman", "steward"}},
		Profiles: characterTestProfiles(),
		Groups:   []character.Group{groupTestGroup()},
	}); err != nil {
		t.Fatal(err)
	}
	request := AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:     character.Scene{NodeID: "north-gate", Present: []string{"guardsman", "steward"}},
		CommandID: "cmd-group-1", CommandHash: "sha256:abc",
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{
				{Speaker: character.SpeakerPlayer, Channel: character.ChannelGroup, GroupID: "grp-1", Text: "今晚都到北门来。"},
				{CharacterID: "guardsman", Speaker: character.SpeakerCharacter, Channel: character.ChannelGroup, GroupID: "grp-1", Text: "我今夜值守。"},
			},
		},
	}
	first, err := store.AppendCharacterFacts(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Exchanges) != 2 {
		t.Fatalf("first commit produced %d exchanges", len(first.Exchanges))
	}
	second, err := store.AppendCharacterFacts(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Exchanges) != len(first.Exchanges) {
		t.Fatalf("a retry returned %d exchanges, want the %d it committed", len(second.Exchanges), len(first.Exchanges))
	}
	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if lines := state.GroupExchanges("grp-1"); len(lines) != 2 {
		t.Fatalf("the journal holds %d group lines after a retry, want 2", len(lines))
	}
}

// TestGroupRoundRejectsAConflictingCommandID keeps one command id from being
// reused for different content. Silently accepting it would make the retry
// return a round that does not match what the player asked.
func TestGroupRoundRejectsAConflictingCommandID(t *testing.T) {
	store, storyID := characterTestStory(t)
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman", "steward"}},
		Profiles: characterTestProfiles(),
		Groups:   []character.Group{groupTestGroup()},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:     character.Scene{NodeID: "north-gate", Present: []string{"guardsman", "steward"}},
		CommandID: "cmd-group-1", CommandHash: "sha256:abc",
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{
				{Speaker: character.SpeakerPlayer, Channel: character.ChannelGroup, GroupID: "grp-1", Text: "今晚都到北门来。"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:     character.Scene{NodeID: "north-gate", Present: []string{"guardsman", "steward"}},
		CommandID: "cmd-group-1", CommandHash: "sha256:def",
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{
				{Speaker: character.SpeakerPlayer, Channel: character.ChannelGroup, GroupID: "grp-1", Text: "改主意了。"},
			},
		},
	})
	if err == nil {
		t.Fatal("a reused command id with different content was accepted")
	}
}

// TestGroupMembershipChangeIsCommittedWithItsLines checks that a membership
// change and the lines that follow it land in one transaction, so a line cannot
// be committed against a roster that was never saved.
func TestGroupMembershipChangeIsCommittedWithItsLines(t *testing.T) {
	store, storyID := characterTestStory(t)
	group := groupTestGroup()
	group.Members = group.Members[:1]
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman", "steward"}},
		Profiles: characterTestProfiles(),
		Groups:   []character.Group{group},
	}); err != nil {
		t.Fatal(err)
	}
	steward := character.Group{ID: "grp-1", Type: character.GroupSelf, Name: "北门夜话",
		Channel: character.ChannelScene, Version: 2,
		Members: []character.GroupMember{
			{CharacterID: "guardsman", JoinedVersion: 1},
			{CharacterID: "steward", JoinedVersion: 2},
		}}
	projected, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 午时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman", "steward"}},
		Groups:   []character.Group{steward},
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{
				{Speaker: character.SpeakerPlayer, Channel: character.ChannelGroup, GroupID: "grp-1", Text: "管事也来了。"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Groups) != 1 {
		t.Fatalf("projection reported %d groups", len(projected.Groups))
	}
	// The line committed in the same transaction resolved its audience against
	// the new roster, so the person who just joined is in it.
	line := projected.Exchanges[0]
	if !character.ContainsID(line.AudienceMembers(), "steward") {
		t.Fatalf("a line committed with a join did not reach the joiner: %v", line.Audience)
	}
}

// TestGroupMembershipCannotRollBack checks the store-level guard: an older
// member version is refused rather than applied, so a stale or retried write
// cannot quietly remove someone who has since joined.
func TestGroupMembershipCannotRollBack(t *testing.T) {
	store, storyID := characterTestStory(t)
	newer := groupTestGroup()
	newer.Version = 3
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Profiles: characterTestProfiles(),
		Groups:   []character.Group{newer},
	}); err != nil {
		t.Fatal(err)
	}
	older := groupTestGroup()
	older.Version = 2
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 午时",
		Groups:   []character.Group{older},
	}); err == nil {
		t.Fatal("an older group version was applied over a newer one")
	}
	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	group, ok := state.Group("grp-1")
	if !ok || group.Version != 3 {
		t.Fatalf("stored group version = %d, want 3", group.Version)
	}
}

// TestGroupLineWithoutAGroupIsRejected keeps a group line from being filed where
// no membership exists to resolve its audience.
func TestGroupLineWithoutAGroupIsRejected(t *testing.T) {
	store, storyID := characterTestStory(t)
	if _, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Profiles: characterTestProfiles(),
	}); err != nil {
		t.Fatal(err)
	}
	_, err := store.AppendCharacterFacts(storyID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{
				{Speaker: character.SpeakerPlayer, Channel: character.ChannelGroup, GroupID: "grp-missing", Text: "有人吗？"},
			},
		},
	})
	if err == nil {
		t.Fatal("a line for an unknown group was committed")
	}
	if !strings.Contains(err.Error(), "grp-missing") {
		t.Fatalf("error does not name the missing group: %v", err)
	}
}
