package interactiveapp

import (
	"strings"
	"testing"

	"ta/internal/interactive"
	"ta/internal/storypack"
)

// This file is the cross-region half of 自制-NPC出场与升降温 §6: 与人物A完成一件事→
// 离开做B的事. The discovery page is what decides whether a person in another
// country can be walked up to, and the exclusion list is what stops someone the
// story has already met from being offered again as a stranger.
//
// The catalog's own placement rules are covered in internal/storypack. What is
// checked here is the story-side chain: a branch that has bound a 越国 person
// asks for an expert while standing in 元武国, and the page it gets must place
// everyone honestly — the people of 元武国 as reachable, the 越国 person the
// story already knows as not being offered at all.

// TestAnEstablishedPersonIsNotOfferedAgainFromAnotherCountry is the 离开做B的事
// step seen from the discovery page. 慕沛灵 is already an actor on this branch
// with her own id and her own settled thread, so a page built in 元武国 must not
// re-offer her: re-offering is how a story introduces someone it has met as if
// they were new, and it is also how a second actor with the same face gets
// minted for a person standing in a different country.
func TestAnEstablishedPersonIsNotOfferedAgainFromAnotherCountry(t *testing.T) {
	conversation, store, story := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	met := pack.CatalogBySourceKey()["npc_mu_peiling"]
	if met.SourceKey == "" {
		t.Fatal("the shipped catalog lost the 药园管事")
	}

	// The branch meets her in 越国 and binds the author's key to a local actor.
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "我到黄枫谷药园找慕管事。", Narrative: "慕沛灵把册子合上。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"问今日的差事", "看东畦", "问她药园的规矩", "记下她的名字", "告辞离开"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Profiles: []interactive.TurnCharacterProfileInput{{
					CharacterID: "mu_peiling", Name: met.Name, SourceKey: met.SourceKey,
					Role: "药园管事", Description: "黄枫谷药园的管事，说话短，账目记得清。",
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the turn that meets her: %v", err)
	}

	// The protagonist travels to 元武国 and asks about arrays there. The scene's
	// location is the one the story recorded, so this is the page the turn gets.
	ctx := discoveryStoryContext(pack, "main", "元武国天星宗对外入口")
	outcome := conversation.storyDiscoveryContext(ctx, "我想找一位懂阵法的人请教残阵")
	if outcome.Failure != nil {
		t.Fatalf("the page could not be built: %v", outcome.Failure)
	}
	block := outcome.Block
	if block == "" {
		t.Fatal("no page was built for a 元武国 scene, so the narrator has nobody to reach for")
	}
	if strings.Contains(block, "source_key: "+met.SourceKey) {
		t.Fatalf("%s was offered again from another country after being met:\n%s", met.Name, block)
	}

	// Everyone the page places in this scene is really in 元武国, and everyone it
	// does not place here is told so in words the narrator must act on.
	catalog := pack.CatalogBySourceKey()
	placed, leads := 0, 0
	for _, card := range strings.Split(block, "\n### ") {
		key := sourceKeyOfCard(card)
		if key == "" {
			continue
		}
		character, ok := catalog[key]
		if !ok {
			t.Fatalf("the page offered %q, which is not in this world's catalog:\n%s", key, card)
		}
		if strings.Contains(card, "at this place") {
			placed++
			if character.Country != "元武国" {
				t.Fatalf("%s is from %s but was offered as standing in a 元武国 scene:\n%s",
					character.Name, character.Country, card)
			}
			continue
		}
		leads++
		if !strings.Contains(card, "must not appear") {
			t.Fatalf("%s is offered without saying they cannot be staged here:\n%s", character.Name, card)
		}
	}
	if placed == 0 {
		t.Fatalf("a 元武国 scene was offered nobody who is actually there:\n%s", block)
	}
	if leads == 0 {
		t.Fatalf("the array request found no lead at all, which the fixture does not expect:\n%s", block)
	}
}

// sourceKeyOfCard reads the author key a rendered card was printed with. It is
// the same string the model is told to copy into the profile it submits, so a
// test that reads it is reading the binding the game actually relies on.
func sourceKeyOfCard(card string) string {
	const marker = "(source_key: "
	start := strings.Index(card, marker)
	if start < 0 {
		return ""
	}
	rest := card[start+len(marker):]
	end := strings.Index(rest, ")")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

// TestThePageNeverPlacesTheKnownPersonInTheNewCountry is the other side of the
// same rule, stated as the failure it prevents: a person the story knows must
// not be quietly re-placed by a later scene. The 越国 药园管事 carries tokens
// (药园, 黄枫谷) that a 元武国 scene does not name, so the catalog itself keeps
// her out — and this asserts it through the story path rather than the catalog.
func TestThePageNeverPlacesTheKnownPersonInTheNewCountry(t *testing.T) {
	pack := fanrenFormalPack(t, "")
	met := pack.CatalogBySourceKey()["npc_mu_peiling"]
	if met.SourceKey == "" {
		t.Fatal("the shipped catalog lost the 药园管事")
	}

	// The catalog answers the placement question for a scene that names 元武国.
	candidates := pack.CharacterCandidates(storypack.CandidateRequest{
		Location: "元武国天星宗对外入口",
		Action:   "我想找一位懂阵法的人请教残阵",
	})
	for _, candidate := range candidates {
		if candidate.Character.SourceKey != met.SourceKey {
			continue
		}
		if candidate.Reach == storypack.ReachLocal {
			t.Fatalf("%s is in %s but was placed at a 元武国 scene",
				met.Name, candidate.Character.Country)
		}
		if candidate.Reach == storypack.ReachRegion {
			t.Fatalf("%s is in %s and cannot be in the same country as a 元武国 scene",
				met.Name, candidate.Character.Country)
		}
	}

	// And once she is an actor on the branch, she is not on the page at all.
	candidates = pack.CharacterCandidates(storypack.CandidateRequest{
		Location:    "元武国天星宗对外入口",
		Action:      "我想找一位懂阵法的人请教残阵",
		Established: map[string]bool{met.SourceKey: true},
	})
	for _, candidate := range candidates {
		if candidate.Character.SourceKey == met.SourceKey {
			t.Fatal("an established person was offered again after the story moved country")
		}
	}
}
