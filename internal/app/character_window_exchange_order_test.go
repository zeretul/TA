package app

import (
	"testing"

	"ta/internal/character"
)

// TestCharacterWindowExchangePageKeepsCommitOrder is the ordering half of the
// window's transcript. IDs are minted as prefix + timestamp + random entropy, so
// two exchanges committed in the same instant differ only in their random tail.
// Sorting by ID therefore does not reproduce commit order, and a question and the
// answer it prompted can come back reversed — which is what the window showed
// before this test existed. The page must preserve the order it was handed,
// because that order is the one the branch projection built by replay.
func TestCharacterWindowExchangePageKeepsCommitOrder(t *testing.T) {
	// Same timestamp prefix, entropy descending across the pair — exactly the
	// shape a real question and answer have (the question's ID sorts after the
	// answer's), and the shape an ID sort gets wrong.
	committed := []character.Exchange{
		{ID: "xch_dm4rgwd4epw8da0c1984", CharacterID: "merchant", Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate, Text: "你叫什么名字？"},
		{ID: "xch_dm4rgwd4epw8d596c5aa", CharacterID: "merchant", Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate, Text: "我姓周。"},
	}
	page := characterWindowExchangePage(committed, "", 50)
	if len(page.exchanges) != 2 {
		t.Fatalf("page holds %d exchanges, want 2", len(page.exchanges))
	}
	if page.exchanges[0].Speaker != character.SpeakerPlayer || page.exchanges[1].Speaker != character.SpeakerCharacter {
		t.Fatalf("page order = %q then %q, want the question before its answer",
			page.exchanges[0].Text, page.exchanges[1].Text)
	}
}

// TestCharacterWindowExchangePageCursorFollowsCommitOrder checks the cursor
// half of the same rule. Scrolling back must cut the transcript at the named
// exchange, so the page a client asks for next is the one immediately older than
// what it already has.
func TestCharacterWindowExchangePageCursorFollowsCommitOrder(t *testing.T) {
	committed := []character.Exchange{
		{ID: "xch_aaa1", CharacterID: "merchant", Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate, Text: "第一句"},
		{ID: "xch_zzz2", CharacterID: "merchant", Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate, Text: "第二句"},
		{ID: "xch_bbb3", CharacterID: "merchant", Speaker: character.SpeakerPlayer, Channel: character.ChannelPrivate, Text: "第三句"},
	}
	page := characterWindowExchangePage(committed, "xch_zzz2", 50)
	if len(page.exchanges) != 1 || page.exchanges[0].Text != "第一句" {
		t.Fatalf("page before xch_zzz2 = %v, want only the first exchange", page.exchanges)
	}
	// The most recent page keeps commit order too, and its cursor is the oldest
	// exchange it shows rather than the lexicographically smallest.
	tail := characterWindowExchangePage(committed, "", 2)
	if len(tail.exchanges) != 2 || !tail.hasMore || tail.cursor != "xch_zzz2" {
		t.Fatalf("tail page = %v (cursor %q, hasMore %v), want the last two from xch_zzz2",
			tail.exchanges, tail.cursor, tail.hasMore)
	}
	if tail.exchanges[0].Text != "第二句" || tail.exchanges[1].Text != "第三句" {
		t.Fatalf("tail page order = %q then %q, want commit order", tail.exchanges[0].Text, tail.exchanges[1].Text)
	}
}
