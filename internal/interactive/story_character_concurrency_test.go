package interactive

import (
	"sync"
	"testing"

	"ta/internal/character"
)

// A repeated click and a retried request can arrive at the same moment, and the
// window's own UI does not serialize them: the send button is disabled while a
// request is in flight, but a second client, a double-submitted form or a
// recovery re-read can still overlap. Two attempts under one command ID must
// settle the exchange once, not twice.
//
// Two Store instances are used on purpose. One Store's mutex would serialize the
// calls and prove only that the mutex works; what is actually at risk is the
// read/CAS/write section, which is guarded across instances by the story
// mutation lease.
func TestConcurrentCharacterAppendUnderOneCommandIDSettlesOnce(t *testing.T) {
	store, storyID := characterTestStory(t)
	second := NewStore(store.root)

	request := AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 辰时",
		Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
		Profiles: characterTestProfiles(),
		Facts: character.TurnCharacterFacts{
			Exchanges: []character.ExchangeDraft{
				{CharacterID: "guardsman", Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate, Text: "开个价吧"},
				{CharacterID: "guardsman", Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate, Text: "二百四十文。"},
			},
		},
		CommandID: "cmd-race", CommandHash: "sha256:race",
	}

	const attempts = 4
	stores := []*Store{store, second}
	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	errs := make([]error, attempts)
	ids := make([][]string, attempts)
	for i := 0; i < attempts; i += 1 {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			start.Wait()
			projected, err := stores[i%len(stores)].AppendCharacterFacts(storyID, request)
			errs[i] = err
			for _, exchange := range projected.Exchanges {
				ids[i] = append(ids[i], exchange.ID)
			}
		}(i)
	}
	start.Done()
	done.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("attempt %d failed: %v", i, err)
		}
	}

	state, err := NewStore(store.root).CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Exchanges) != 2 {
		t.Fatalf("the journal holds %d exchanges after %d concurrent attempts, want 2", len(state.Exchanges), attempts)
	}
	// Every attempt must have observed the same committed records. An attempt
	// that minted its own IDs would have written a second exchange set, and one
	// that returned an empty projection would have the caller commit nothing.
	for i, got := range ids {
		if len(got) != len(ids[0]) {
			t.Fatalf("attempt %d returned %d exchanges, attempt 0 returned %d", i, len(got), len(ids[0]))
		}
		for j := range got {
			if got[j] != ids[0][j] {
				t.Fatalf("attempt %d minted exchange %q, attempt 0 saw %q", i, got[j], ids[0][j])
			}
		}
	}
}

// Two different commands are two different player actions, so they must both
// land. A lease that dropped one of them would be a lost conversation rather
// than a deduplicated one, and the two failure modes are easy to confuse when
// only the "no duplicate" direction is asserted.
func TestConcurrentCharacterAppendUnderDistinctCommandsBothLand(t *testing.T) {
	store, storyID := characterTestStory(t)
	second := NewStore(store.root)
	stores := []*Store{store, second}

	makeRequest := func(commandID, text string) AppendCharacterFactsRequest {
		return AppendCharacterFactsRequest{
			BranchID: "main", WorldTime: "第3日 辰时",
			Scene:    character.Scene{NodeID: "north-gate", Present: []string{"guardsman"}},
			Profiles: characterTestProfiles(),
			Facts: character.TurnCharacterFacts{
				Exchanges: []character.ExchangeDraft{
					{CharacterID: "guardsman", Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate, Text: text},
				},
			},
			CommandID: commandID, CommandHash: "sha256:" + commandID,
		}
	}

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	errs := make([]error, 2)
	requests := []AppendCharacterFactsRequest{makeRequest("cmd-a", "先问价"), makeRequest("cmd-b", "再问期限")}
	for i := 0; i < 2; i += 1 {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			start.Wait()
			_, errs[i] = stores[i].AppendCharacterFacts(storyID, requests[i])
		}(i)
	}
	start.Done()
	done.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("command %d failed: %v", i, err)
		}
	}
	state, err := NewStore(store.root).CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, exchange := range state.Exchanges {
		seen[exchange.Text] = true
	}
	if !seen["先问价"] || !seen["再问期限"] {
		t.Fatalf("a distinct command was dropped: %+v", state.Exchanges)
	}
}
