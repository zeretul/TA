package interactive

import (
	"bytes"
	"os"
	"reflect"
	"testing"

	"ta/internal/character"
)

func TestFactionMembershipVersionSelectionUsesSelectedDecision(t *testing.T) {
	store, id := membershipStoreFixture(t)
	ctx, facts := membershipRuleFixture()
	undecided, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: ctx.PlayerInput, Narrative: "掌门尚未答应。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	expected := undecided.ID
	accepted, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", ExpectedParentID: &expected, ReplaceTurnID: undecided.ID, User: ctx.PlayerInput, Narrative: "掌门明确接纳。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SwitchTurnVersion(id, SwitchTurnVersionRequest{BranchID: "main", TurnID: accepted.ID, VersionTurnID: undecided.ID}); err != nil {
		t.Fatalf("cannot select undecided version: %v", err)
	}
	state, err := store.CharacterState(id, "main")
	if err != nil || len(state.FactionMemberships) != 0 {
		t.Fatalf("discarded version kept identity: %+v %v", state.FactionMemberships, err)
	}
	if err = store.SwitchTurnVersion(id, SwitchTurnVersionRequest{BranchID: "main", TurnID: undecided.ID, VersionTurnID: accepted.ID}); err != nil {
		t.Fatalf("cannot reselect accepted version: %v", err)
	}
	state, err = store.CharacterState(id, "main")
	if err != nil || len(state.FactionMemberships) != 1 || state.FactionMemberships[0].TurnID != accepted.ID {
		t.Fatalf("selected identity not restored: %+v %v", state.FactionMemberships, err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(store.root)
	t.Cleanup(func() { _ = reopened.Close() })
	state, err = reopened.CharacterState(id, "main")
	if err != nil || len(state.FactionMemberships) != 1 || state.FactionMemberships[0].TurnID != accepted.ID {
		t.Fatalf("cold version identity mismatch: %+v %v", state.FactionMemberships, err)
	}
}

func TestFactionMembershipVersionSelectionPreservesPrivateWindowFacts(t *testing.T) {
	store, id := membershipStoreFixture(t)
	ctx, facts := membershipRuleFixture()
	undecided, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: ctx.PlayerInput, Narrative: "掌门尚未答应。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	expected := undecided.ID
	accepted, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", ExpectedParentID: &expected, ReplaceTurnID: undecided.ID, User: ctx.PlayerInput, Narrative: "掌门明确接纳。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	acceptedState, err := store.CharacterState(id, "main")
	if err != nil || len(acceptedState.FactionMemberships) != 1 {
		t.Fatalf("accepted version has no membership: %+v %v", acceptedState.FactionMemberships, err)
	}

	// A private window is independent of the main turn's admission decision.
	// Its logical records and audience must survive either selected version.
	window, err := store.AppendCharacterFacts(id, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第1日 午后", Scene: ctx.Scene,
		CommandID: "membership-version-private", CommandHash: "private-facts-v1",
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{
				{CharacterID: "掌门", Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate, Text: "请只在私下记住，我在寻找一本旧游记。"},
				{CharacterID: "掌门", Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate, Text: "我记住了，也答应替你留意。"},
			},
			Knowledge: []character.KnowledgeDraft{
				{CharacterID: "掌门", Statement: "主角在寻找一本旧游记。", Nature: character.NatureTold, Audience: character.AudienceSingle, Channel: character.ChannelPrivate, LearnedFrom: DefaultActorID},
			},
			Matters: []character.MatterDraft{
				{CharacterID: "掌门", Kind: character.MatterPromise, Status: character.MatterOpen, Statement: "答应替主角留意旧游记。", Counterparty: DefaultActorID},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(window.Exchanges) != 2 || len(window.Knowledge) != 1 || len(window.Matters) != 1 {
		t.Fatalf("incomplete private window fixture: %+v", window)
	}
	journal, err := os.ReadFile(store.storyPath(id))
	if err != nil {
		t.Fatal(err)
	}

	assertSelected := func(s *Store, selected string) character.State {
		t.Helper()
		state, err := s.CharacterState(id, "main")
		if err != nil {
			t.Fatal(err)
		}
		wantExchanges := len(window.Exchanges)
		if selected == accepted.ID {
			wantExchanges += len(acceptedState.Exchanges)
			if !reflect.DeepEqual(state.FactionMemberships, acceptedState.FactionMemberships) {
				t.Fatalf("selected acceptance changed its membership or source: %+v", state.FactionMemberships)
			}
		} else if len(state.FactionMemberships) != 0 {
			t.Fatalf("discarded acceptance leaked into undecided version: %+v", state.FactionMemberships)
		}
		if len(state.Exchanges) != wantExchanges {
			t.Fatalf("version %s lost or duplicated exchanges: got %d, want %d", selected, len(state.Exchanges), wantExchanges)
		}
		for _, original := range window.Exchanges {
			matches := 0
			for _, exchange := range state.Exchanges {
				if exchange.ID == original.ID {
					matches++
					if exchange.TurnID != "" || !reflect.DeepEqual(exchange, original) {
						t.Fatalf("private exchange identity, content or audience changed: %+v", exchange)
					}
				}
			}
			if matches != 1 {
				t.Fatalf("private exchange %s appears %d times", original.ID, matches)
			}
		}
		if !reflect.DeepEqual(state.Knowledge, window.Knowledge) || !reflect.DeepEqual(state.Matters, window.Matters) {
			t.Fatalf("private memory was lost, duplicated or changed: knowledge=%+v matters=%+v", state.Knowledge, state.Matters)
		}
		snapshot, err := s.Snapshot(id, "main")
		if err != nil || snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != selected {
			t.Fatalf("wrong active version after selection: %+v %v", snapshot.CurrentTurn, err)
		}
		return state
	}

	current := accepted.ID
	for _, selected := range []string{undecided.ID, accepted.ID, undecided.ID, accepted.ID} {
		if err := store.SwitchTurnVersion(id, SwitchTurnVersionRequest{BranchID: "main", TurnID: current, VersionTurnID: selected}); err != nil {
			t.Fatalf("cannot switch from %s to %s with private suffix: %v", current, selected, err)
		}
		live := assertSelected(store, selected)
		after, err := os.ReadFile(store.storyPath(id))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(after, journal) {
			t.Fatal("version selection rewrote an existing journal event")
		}
		journal = after

		root := store.root
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		store = NewStore(root)
		reopened := store
		t.Cleanup(func() { _ = reopened.Close() })
		cold := assertSelected(store, selected)
		if !reflect.DeepEqual(cold, live) {
			t.Fatalf("cold selection changed character facts: live=%+v cold=%+v", live, cold)
		}
		after, err = os.ReadFile(store.storyPath(id))
		if err != nil || !bytes.Equal(after, journal) {
			t.Fatalf("cold projection changed the canonical journal: %v", err)
		}
		current = selected
	}
}
