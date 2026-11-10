package interactive

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"ta/internal/character"
	interactivestate "ta/internal/interactive/state"
	"testing"
)

func TestPlayerAttentionJournalRetryColdReopenAndBranch(t *testing.T) {
	s, id := characterTestStory(t)
	defer s.Close()
	_, err := s.AppendCharacterFacts(id, AppendCharacterFactsRequest{BranchID: "main", WorldTime: "Day 1", Profiles: characterTestProfiles()})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "Look", Narrative: "A quiet day.", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.PlayerAttention(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	knownBefore, _ := s.CharacterState(id, "main")
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: turn.ID, Title: "before attention"})
	if err != nil {
		t.Fatal(err)
	}
	in := AttentionUpdate{BranchID: "main", ExpectedHead: before.Head, Kind: "character", ID: "guardsman", Following: true}
	after, err := s.UpdatePlayerAttention(id, in)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.UpdatePlayerAttention(id, in)
	if err != nil || !reflect.DeepEqual(after, retry) {
		t.Fatalf("retry: %+v %v", retry, err)
	}
	if after.Head == before.Head || !reflect.DeepEqual(after.Characters, []string{"guardsman"}) {
		t.Fatalf("not committed: %+v", after)
	}
	knownAfter, _ := s.CharacterState(id, "main")
	if !reflect.DeepEqual(knownBefore, knownAfter) {
		t.Fatal("attention changed character facts")
	}
	other, err := s.PlayerAttention(id, fork.ID)
	if err != nil || len(other.Characters) != 0 {
		t.Fatalf("future attention leaked: %+v %v", other, err)
	}
	root := s.root
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	defer reopened.Close()
	recovered, err := reopened.PlayerAttention(id, "main")
	if err != nil || !reflect.DeepEqual(after, recovered) {
		t.Fatalf("cold reopen: %+v %v", recovered, err)
	}
	_, err = reopened.UpdatePlayerAttention(id, AttentionUpdate{BranchID: "main", ExpectedHead: before.Head, Kind: "character", ID: "steward", Following: true})
	if !errors.Is(err, ErrAttentionConflict) {
		t.Fatalf("stale write accepted: %v", err)
	}
	nextTurn, _, err := reopened.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "Wait", Narrative: "Still quiet.", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	inherited, err := reopened.CreateBranch(id, CreateBranchRequest{ParentEventID: nextTurn.ID, Title: "after attention"})
	if err != nil {
		t.Fatal(err)
	}
	forkAttention, err := reopened.PlayerAttention(id, inherited.ID)
	if err != nil || !reflect.DeepEqual(forkAttention.Characters, after.Characters) {
		t.Fatalf("fork did not inherit prior attention: %+v %v", forkAttention, err)
	}
	current, _ := reopened.PlayerAttention(id, "main")
	unfollow, err := reopened.UpdatePlayerAttention(id, AttentionUpdate{BranchID: "main", ExpectedHead: current.Head, Kind: "character", ID: "guardsman", Following: false})
	if err != nil || len(unfollow.Characters) != 0 {
		t.Fatalf("unfollow: %+v %v", unfollow, err)
	}
	forkAfter, _ := reopened.PlayerAttention(id, inherited.ID)
	if !reflect.DeepEqual(forkAfter.Characters, after.Characters) {
		t.Fatal("unfollowing main changed the fork")
	}
}

func TestPlayerAttentionKnownTargetsLimitAndModelBoundary(t *testing.T) {
	s, id := characterTestStory(t)
	defer s.Close()
	profiles := []character.Profile{}
	for i := 0; i < 33; i++ {
		profiles = append(profiles, character.Profile{ID: fmt.Sprintf("person-%02d", i), Name: fmt.Sprintf("Person %d", i), Revision: 1})
	}
	if _, err := s.AppendCharacterFacts(id, AppendCharacterFactsRequest{BranchID: "main", WorldTime: "Day 1", Profiles: profiles}); err != nil {
		t.Fatal(err)
	}
	view, _ := s.PlayerAttention(id, "main")
	for _, kind := range []string{"character", "quest"} {
		_, err := s.UpdatePlayerAttention(id, AttentionUpdate{BranchID: "main", ExpectedHead: view.Head, Kind: kind, ID: "unknown", Following: true})
		if !errors.Is(err, ErrAttentionUnknown) {
			t.Fatalf("unknown %s: %v", kind, err)
		}
	}
	for i, p := range profiles {
		next, err := s.UpdatePlayerAttention(id, AttentionUpdate{BranchID: "main", ExpectedHead: view.Head, Kind: "character", ID: p.ID, Following: true})
		if i == 32 {
			if !errors.Is(err, ErrAttentionLimit) {
				t.Fatalf("limit: %v", err)
			}
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		view = next
	}
	_, err := CompileTurnStateUpdates(StoryDirectorActorStateSystem{}, nil, []interactivestate.Update{{Op: "create", Path: "/player_attention", Value: map[string]any{"v": 1}}}, TurnStateUpdateCompileOptions{})
	var validation *StateUpdateValidationError
	if !errors.As(err, &validation) || validation.Code != "player_owned_state" {
		t.Fatalf("missing player-owned boundary: %v", err)
	}
	if context := PlayerAttentionContext(map[string]any{playerAttentionRoot: view.PlayerAttention}); !strings.Contains(context, "person-00") || !strings.Contains(context, "不揭露秘密") || len(context) > 12*1024 {
		t.Fatalf("invalid bounded preference context: %q", context)
	}
}

func TestPlayerAttentionQuestDoesNotAcceptOrAdvanceClock(t *testing.T) {
	s, id, _, _ := authoredQuestStory(t, "xinghai-689", "朱伯利外环47号减压长廊", 1000, nil)
	before, err := s.QuestStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Available) == 0 {
		t.Fatal("fixture has no public known task")
	}
	stateBefore, err := s.StoryContext(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	attention, _ := s.PlayerAttention(id, "main")
	_, err = s.UpdatePlayerAttention(id, AttentionUpdate{BranchID: "main", ExpectedHead: attention.Head, Kind: "quest", ID: before.Available[0].TaskID, Following: true})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := s.QuestStateFor(id, "main")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("following accepted or changed a task")
	}
	stateAfter, _ := s.StoryContext(id, "main")
	if SnapshotClockSeconds(stateBefore.Snapshot) != SnapshotClockSeconds(stateAfter.Snapshot) {
		t.Fatal("attention spent world time")
	}
}
