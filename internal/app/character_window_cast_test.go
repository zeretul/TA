package app

import (
	"fmt"
	"strings"
	"testing"

	"ta/internal/character"
)

// TestCharacterWindowListsEveryActorPastTheListCap is the NAR-09 rule at the
// window. The story-side Actor projection is capped at 24 entries and keeps the
// foundational Actors ahead of unrelated ones; the window is a different read
// with a different job, and it must not inherit that cap. A story with more than
// two dozen people in it still lists all of them, because the player's roster is
// the one place they can see who they have met.
//
// The failure this guards against is specific: capping the roster would drop
// exactly the characters whose IDs sort last, which is an arbitrary set that has
// nothing to do with who matters. The cast is ordered present-first for that
// reason, and the assertion below checks a character who is present *and* sorts
// last, so neither an ID cap nor a "first N" cap can pass it.
func TestCharacterWindowListsEveryActorPastTheListCap(t *testing.T) {
	// One more than the story-side list cap, so a cap of that size would bite.
	const castSize = 24 + 3

	state := character.State{Version: character.StateVersion}
	ids := make([]string, 0, castSize)
	for i := 0; i < castSize; i++ {
		// IDs are zero-padded so the lexicographic order matches the numeric one,
		// which makes "sorts last" unambiguous.
		id := fmt.Sprintf("actor-%03d", i)
		ids = append(ids, id)
		if err := state.UpsertProfile(character.Profile{
			ID: id, StoryID: "st", Name: fmt.Sprintf("角色%d", i), Revision: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// The protagonist is the first actor and a key role is last; the scene places
	// the last one on stage. If a cap kept "the first N by ID" the protagonist
	// would survive and the key role would not, which is why both are asserted.
	protagonist := ids[0]
	onStage := ids[len(ids)-1]

	rows := characterWindowCast(state, character.Scene{Present: []string{onStage}}, pictureSources{})
	if len(rows) != castSize {
		t.Fatalf("the window listed %d characters of %d; a story-side list cap leaked into the roster", len(rows), castSize)
	}

	byID := map[string]CharacterWindowCharacter{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	for _, id := range []string{protagonist, onStage} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("the window dropped %q", id)
		}
	}
	// The present character is first and is the only one on the scene channel,
	// so the window still says who the player is standing in front of.
	if rows[0].ID != onStage || !rows[0].Present || rows[0].Channel != string(character.ChannelScene) {
		t.Fatalf("the present character is not the first row: %+v", rows[0])
	}
	// Everyone else is reachable by message, not by speaking into the scene.
	for _, row := range rows[1:] {
		if row.Present || row.Channel != string(character.ChannelMessage) {
			t.Fatalf("a character who is not in the scene is listed as present: %+v", row)
		}
	}
}

// TestCharacterWindowCastIsStableAcrossReads pins the ordering rule. The roster
// is rebuilt on every read, so an unstable order would make the list jump under
// the player between refreshes. Sorting present-first and then by ID is what
// makes two reads of one state identical.
func TestCharacterWindowCastIsStableAcrossReads(t *testing.T) {
	state := character.State{Version: character.StateVersion}
	for _, id := range []string{"zhu", "an", "mu"} {
		if err := state.UpsertProfile(character.Profile{ID: id, StoryID: "st", Name: id, Revision: 1}); err != nil {
			t.Fatal(err)
		}
	}
	scene := character.Scene{Present: []string{"mu"}}

	first := characterWindowCast(state, scene, pictureSources{})
	second := characterWindowCast(state, scene, pictureSources{})
	if len(first) != len(second) {
		t.Fatalf("two reads returned %d and %d rows", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("row %d differs between reads: %q then %q", i, first[i].ID, second[i].ID)
		}
	}
	// The order is present-first, then by ID: mu (present), then an, zhu.
	got := make([]string, 0, len(first))
	for _, row := range first {
		got = append(got, row.ID)
	}
	if want := "mu,an,zhu"; strings.Join(got, ",") != want {
		t.Fatalf("cast order = %v, want %s", got, want)
	}
}

func TestCharacterWindowProjectsStoryAndCatalogIdentitySources(t *testing.T) {
	state := character.State{Version: character.StateVersion}
	if err := state.UpsertProfile(character.Profile{
		ID: "gray", StoryID: "st", Name: "灰团", Revision: 1,
		Lifeform: character.LifeformSpiritBeast, CultivationMethodID: "method.spirit_beast_breathing",
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.UpsertProfile(character.Profile{
		ID: "catalog-person", StoryID: "st", Name: "苏玉兰", SourceKey: "npc.su-yulan", Revision: 1,
		Lifeform: character.LifeformHuman,
	}); err != nil {
		t.Fatal(err)
	}
	rows := characterWindowCast(state, character.Scene{}, pictureSources{})
	byID := make(map[string]CharacterWindowCharacter, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	gray := byID["gray"]
	if gray.IdentitySource != "story" || gray.SourceKey != "" || gray.Lifeform != character.LifeformSpiritBeast || gray.CultivationMethodID != "method.spirit_beast_breathing" {
		t.Fatalf("new story identity was not projected as the same registered character: %+v", gray)
	}
	if got := byID["catalog-person"]; got.IdentitySource != "author_catalog" || got.SourceKey != "npc.su-yulan" {
		t.Fatalf("author-catalog identity source was not projected: %+v", got)
	}
}
