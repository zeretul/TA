package app

import (
	"testing"

	"ta/internal/character"
)

// characterWindowChannelState is one character who has spoken twice in the main
// story and twice in the window. The scene lines reach the character through the
// audience rule, so ExchangesFor returns all four; the window must show two.
func characterWindowChannelState() character.State {
	return character.State{
		Version: character.StateVersion,
		Profiles: []character.Profile{
			{ID: "a-gui", StoryID: "st", Name: "阿贵", FirstSeenWorldTime: "第1日"},
		},
		Exchanges: []character.Exchange{
			{
				ID: "xch_scene_1", StoryID: "st", BranchID: "main", CharacterID: "a-gui",
				Speaker: character.SpeakerCharacter, Channel: character.ChannelScene,
				Text: "陆先生。", WorldTime: "第1日", TurnID: "turn-1",
				Audience: []string{"a-gui", "lu"},
			},
			{
				ID: "xch_scene_2", StoryID: "st", BranchID: "main", CharacterID: "a-gui",
				Speaker: character.SpeakerCharacter, Channel: character.ChannelScene,
				Text: "你上回说的话，还作数吧。", WorldTime: "第1日", TurnID: "turn-1",
				Audience: []string{"a-gui", "lu"},
			},
			{
				ID: "xch_private_1", StoryID: "st", BranchID: "main", CharacterID: "a-gui",
				Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate,
				Text: "作数。", WorldTime: "第2日",
			},
			{
				ID: "xch_private_2", StoryID: "st", BranchID: "main", CharacterID: "a-gui",
				Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate,
				Text: "那我等你回话。", WorldTime: "第2日",
			},
		},
		Revision: 1,
	}
}

// TestCharacterWindowExchangesDropSceneDialogue is the history-projection half of
// U4. The main story's dialogue is already the prose the player is reading; the
// character window is the separate conversation with that person. ExchangesFor
// merges the two on purpose, because a scene line is memory the narrator and the
// recall pass need, so the window has to project the channel itself rather than
// inherit the unfiltered read.
func TestCharacterWindowExchangesDropSceneDialogue(t *testing.T) {
	state := characterWindowChannelState()

	// The unfiltered read still carries everything: the memory path is unchanged.
	if reached := state.ExchangesFor("a-gui"); len(reached) != 4 {
		t.Fatalf("ExchangesFor = %d exchanges, want the 4 the journal holds", len(reached))
	}

	projected := characterWindowExchanges(state, "a-gui")
	if len(projected) != 2 {
		t.Fatalf("window transcript = %d exchanges, want only the 2 private ones: %v", len(projected), projected)
	}
	for _, exchange := range projected {
		if exchange.Channel != character.ChannelPrivate {
			t.Fatalf("window transcript still carries a %q exchange: %+v", exchange.Channel, exchange)
		}
	}
	if projected[0].Text != "作数。" || projected[1].Text != "那我等你回话。" {
		t.Fatalf("window transcript order = %q then %q, want commit order",
			projected[0].Text, projected[1].Text)
	}
}

// TestCharacterWindowExchangePageCutsTheProjectedTranscript checks that the
// filter runs before paging. Filtering afterwards would let a page be spent on
// scene lines the window never draws, so a page of two could arrive empty while
// has_more still said the conversation continued.
func TestCharacterWindowExchangePageCutsTheProjectedTranscript(t *testing.T) {
	state := characterWindowChannelState()
	projected := characterWindowExchanges(state, "a-gui")

	page := characterWindowExchangePage(projected, "", 1)
	if len(page.exchanges) != 1 || page.exchanges[0].ID != "xch_private_2" {
		t.Fatalf("tail page = %v, want the newest private exchange", page.exchanges)
	}
	if !page.hasMore || page.cursor != "xch_private_2" {
		t.Fatalf("tail page cursor = %q (hasMore %v), want xch_private_2", page.cursor, page.hasMore)
	}
	earlier := characterWindowExchangePage(projected, page.cursor, 1)
	if len(earlier.exchanges) != 1 || earlier.exchanges[0].ID != "xch_private_1" {
		t.Fatalf("earlier page = %v, want the remaining private exchange", earlier.exchanges)
	}
	if earlier.hasMore {
		t.Fatalf("earlier page still reports more after the whole private transcript was paged")
	}
}

// TestCharacterWindowRosterPreviewsThePrivateConversation is the chat list's
// half of the unified shell. The list shows a one-line preview and a story time
// per person, and both have to come from the transcript the window itself
// draws. Reading them off ExchangesFor instead would preview the story's prose,
// so a person whose only line was spoken in a scene would show a preview for a
// conversation that has not happened.
func TestCharacterWindowRosterPreviewsThePrivateConversation(t *testing.T) {
	state := characterWindowChannelState()
	state.Profiles = append(state.Profiles, character.Profile{
		ID: "silent", StoryID: "st", Name: "没说过话的人",
	})

	byID := map[string]CharacterWindowCharacter{}
	for _, row := range characterWindowCast(state, character.Scene{}, pictureSources{}) {
		byID[row.ID] = row
	}

	// The newest private line is the character's, not the scene line that came
	// after it in nothing but the fixture's own ordering.
	row := byID["a-gui"]
	if row.LastExchange != "那我等你回话。" {
		t.Fatalf("last_exchange = %q, want the newest private line", row.LastExchange)
	}
	if row.LastWorldTime != "第2日" {
		t.Fatalf("last_world_time = %q, want the private line's time", row.LastWorldTime)
	}

	// Someone the player has never spoken with has no preview rather than an
	// empty bubble, so the list can tell an untouched contact from an empty one.
	silent := byID["silent"]
	if silent.LastExchange != "" || silent.LastWorldTime != "" {
		t.Fatalf("silent character previewed %q at %q", silent.LastExchange, silent.LastWorldTime)
	}
}
