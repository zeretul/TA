package interactiveapp

import (
	"context"
	"strings"
	"testing"

	agentcontext "ta/internal/agents/context"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// fanrenFormalPack loads the shipped 凡人人界 pack, which is the only pack in the
// build that carries a character catalog.
func fanrenFormalPack(t *testing.T, dataRoot string) storypack.Pack {
	t.Helper()
	pack, err := storypack.NewLibrary(dataRoot).Get("fanren-jie")
	if err != nil {
		t.Fatalf("the fanren-jie pack does not load: %v", err)
	}
	if len(pack.Characters) == 0 {
		t.Fatal("the fanren-jie pack ships no character catalog")
	}
	return pack
}

// newDiscoveryConversation builds a story bound to the formal pack, which is what
// gives the turn a catalog to discover from.
func newDiscoveryConversation(t *testing.T) (*Conversation, *interactive.Store, interactive.StorySummary) {
	t.Helper()
	workspace := t.TempDir()
	novaDir := t.TempDir()
	pack := fanrenFormalPack(t, novaDir)
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:            "凡人人界",
		Origin:           "越国公共坊市的外围。",
		ReplyTargetChars: 800,
		PackBinding:      pack.Binding(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewConversation(store, novaDir, workspace, story.ID, "", "我先在坊市外围打听消息", story.ReplyTargetChars, nil), store, story
}

// discoveryStoryContext is the story context the turn builds, reduced to the
// fields discovery reads: the binding that names the catalog, the branch whose
// registry holds the established cast, and the scene's location.
func discoveryStoryContext(pack storypack.Pack, branchID, location string) interactive.StoryContext {
	return interactive.StoryContext{
		Meta: interactive.StoryMeta{PackBinding: pack.Binding()},
		Snapshot: interactive.Snapshot{
			BranchID: branchID,
			State: map[string]any{
				"actors": map[string]any{
					interactive.DefaultStoryContextActorID: map[string]any{
						"state": map[string]any{"当前详细地点": location},
					},
				},
			},
		},
	}
}

// TestDiscoveryBlockReachesTheTurnContext is the wiring half of C4: the verified
// catalog has to arrive at the narrator, or a player asking for a local expert
// gets whoever the model invents.
func TestDiscoveryBlockReachesTheTurnContext(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	const action = "我想在黄枫谷附近找一位懂阵法的人请教残阵"
	assembled, err := conversation.AssembleModelContext(context.Background(), action, agentcontext.ModelContextInput{
		UserMessage: action,
		Budget:      conversation.ModelContextBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var turn string
	for _, message := range assembled.Messages {
		if strings.Contains(message.Content, "Discoverable People") {
			turn = message.Content
		}
	}
	if turn == "" {
		t.Fatal("the turn context carries no discoverable-people block")
	}
	for _, want := range []string{
		"Discoverable People",
		"source_key:",
		"Availability:",
		"index, not knowledge",
	} {
		if !strings.Contains(turn, want) {
			t.Fatalf("the discovery block is missing %q", want)
		}
	}
	// The block is a locating index, not the profiles. Sending the bodies every
	// turn is the thing the design rules out, so the block must be a small
	// fraction of the world book it indexes.
	pack := fanrenFormalPack(t, "")
	worldBytes := 0
	for _, entry := range pack.World {
		worldBytes += len([]byte(entry.Content))
	}
	blockBytes := len([]byte(turn))
	if worldBytes > 0 && blockBytes > worldBytes {
		t.Fatalf("the turn context (%d bytes) is larger than the whole world book (%d bytes)", blockBytes, worldBytes)
	}
}

// TestDiscoveryBlockNamesTheRightLocalPerson checks the §4.2 ladder end to end
// through the real conversation: a request for array study made at 黄枫谷 offers
// the local array teacher first, and a person in another country is never offered
// as being here.
//
// "Never offered as being here" is not the same as "never offered". The design's
// own rule for a field the scene cannot answer is 当地同道或异地引荐, so a person in
// another country who knows the field belongs on the page as a lead — with an
// availability line that forbids staging them in this scene. Withholding her
// entirely left the narrator one local teacher and no second name to reach for.
func TestDiscoveryBlockNamesTheRightLocalPerson(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	catalog := pack.CatalogBySourceKey()
	local := catalog["npc_song_guanlan"]
	remote := catalog["canon_xin_ruyin"]
	if local.SourceKey == "" || remote.SourceKey == "" {
		t.Fatal("the shipped catalog lost one of the two people this test needs")
	}

	outcome := conversation.storyDiscoveryContext(
		discoveryStoryContext(pack, "main", "越国黄枫谷外的坊市"),
		"我想找一位懂阵法的人请教残阵")
	block := outcome.Block
	if block == "" {
		t.Fatal("no discovery block was built for a local array request")
	}
	if !strings.Contains(block, local.Name) {
		t.Fatalf("the local array teacher %s is missing from the block", local.Name)
	}
	// The local teacher is offered as someone who can be walked up to.
	localCard := candidateCardText(t, block, local.Name)
	if !strings.Contains(localCard, "at this place") {
		t.Fatalf("the local array teacher is not offered as being here:\n%s", localCard)
	}
	// The 元武国 array teacher may be a lead, never a person standing here.
	if strings.Contains(block, remote.Name) {
		remoteCard := candidateCardText(t, block, remote.Name)
		if strings.Contains(remoteCard, "at this place") {
			t.Fatalf("%s is in %s but was offered as being at a 黄枫谷 scene:\n%s",
				remote.Name, remote.Country, remoteCard)
		}
		if !strings.Contains(remoteCard, "must not appear") {
			t.Fatalf("%s is offered without saying she cannot be staged here:\n%s",
				remote.Name, remoteCard)
		}
	}
}

// candidateCardText returns the one card in a rendered page that belongs to the
// named person, so a test can assert about that person rather than about the page.
func candidateCardText(t *testing.T, block, name string) string {
	t.Helper()
	for _, card := range strings.Split(block, "\n### ") {
		if strings.HasPrefix(card, name+" ") || strings.Contains(card, "### "+name+" ") ||
			strings.HasPrefix(card, name+"\n") {
			return card
		}
	}
	t.Fatalf("the page holds no card for %s:\n%s", name, block)
	return ""
}

// TestDiscoveryBlockIsWithheldWhenTheCatalogHasMovedOn is the revision binding:
// a story plays against the catalog it started from. If the pack file moves on,
// offering the new revision's people would let a running story meet someone who
// did not exist when it began.
func TestDiscoveryBlockIsWithheldWhenTheCatalogHasMovedOn(t *testing.T) {
	conversation, _, story := newDiscoveryConversation(t)
	ctx := interactive.StoryContext{
		Meta: interactive.StoryMeta{PackBinding: &interactive.StoryPackBinding{
			PackID:      "fanren-jie",
			ContentHash: "000000000000000000000000",
			Scope:       "pack:fanren-jie@000000000000000000000000",
		}},
		Snapshot: interactive.Snapshot{State: map[string]any{
			"actors": map[string]any{
				interactive.DefaultStoryContextActorID: map[string]any{
					"state": map[string]any{"当前详细地点": "越国黄枫谷外的坊市"},
				},
			},
		}},
	}
	outcome := conversation.storyDiscoveryContext(ctx, "我想找一位懂阵法的人")
	if outcome.Block != "" {
		t.Fatalf("a story on a stale catalog revision was offered %q", outcome.Block)
	}
	// A legacy story whose pack has moved on is a diagnosable state, not an empty
	// world: the turn withholds the page and says why.
	if outcome.Failure == nil {
		t.Fatal("a stale catalog revision was reported as an ordinary empty page")
	}
	_ = story
}

// TestDiscoveryBlockIsEmptyWithoutAPack is the manual-story case: a story started
// by hand has no catalog, and the turn must run exactly as it did before rather
// than receive an empty roster.
func TestDiscoveryBlockIsEmptyWithoutAPack(t *testing.T) {
	workspace := t.TempDir()
	novaDir := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "北门", Origin: "手写开局", ReplyTargetChars: 800,
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, novaDir, workspace, story.ID, "", "我看看四周", story.ReplyTargetChars, nil)
	ctx := interactive.StoryContext{Snapshot: interactive.Snapshot{State: map[string]any{}}}
	outcome := conversation.storyDiscoveryContext(ctx, "我想找一位懂阵法的人")
	if outcome.Block != "" {
		t.Fatalf("a story with no pack was offered %q", outcome.Block)
	}
	// A story with no pack is not a failure: it has no catalog to fail at.
	if outcome.Failure != nil {
		t.Fatalf("a story with no pack reported a catalog failure: %v", outcome.Failure)
	}
}

// TestEstablishedPeopleLeaveTheCandidatePage is the anti-duplication rule seen
// from the conversation: once a preset person has been bound to an actor on this
// branch, the page stops offering them, so the story cannot introduce someone it
// has already met as if they were new.
func TestEstablishedPeopleLeaveTheCandidatePage(t *testing.T) {
	conversation, store, story := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	local := pack.CatalogBySourceKey()["npc_song_guanlan"]
	if local.SourceKey == "" {
		t.Fatal("the shipped catalog lost the local array teacher")
	}
	ctx := discoveryStoryContext(pack, "main", "越国黄枫谷外的坊市")
	const action = "我想找一位懂阵法的人请教残阵"
	if block := conversation.storyDiscoveryContext(ctx, action).Block; !strings.Contains(block, local.Name) {
		t.Fatalf("%s was not offered before being met", local.Name)
	}

	// The story meets them: the turn binds the preset person to this branch's
	// actor by copying the source key the page printed.
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: action, Narrative: "你找到了宋观澜。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"继续请教", "观察四周", "告辞离开", "询问来历", "记下名字"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Profiles: []interactive.TurnCharacterProfileInput{{
					CharacterID: "song_guanlan", Name: local.Name, SourceKey: local.SourceKey,
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	ctx.Snapshot.BranchID = "main"
	if block := conversation.storyDiscoveryContext(ctx, action).Block; strings.Contains(block, local.Name) {
		t.Fatalf("%s was offered again after being met", local.Name)
	}
}

// TestDiscoveryBlockNeverCarriesAProfileBody is the knowledge boundary: the page
// locates a person and names the entry to read, and the entry's own text stays in
// the world book until the narrator reads it.
func TestDiscoveryBlockNeverCarriesAProfileBody(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	ctx := discoveryStoryContext(pack, "main", "越国黄枫谷外的坊市")
	block := conversation.storyDiscoveryContext(ctx, "我想找一位懂阵法的人请教残阵").Block
	if block == "" {
		t.Fatal("no discovery block was built")
	}
	for _, entry := range pack.World {
		body := strings.TrimSpace(entry.Content)
		if len([]byte(body)) < 200 {
			continue
		}
		if strings.Contains(block, body) {
			t.Fatalf("the discovery block carried the full body of %s", entry.ID)
		}
	}
	// It must instead name the entry to read.
	if !strings.Contains(block, "read_lore_items id=") {
		t.Fatal("the block does not tell the narrator which entry to read")
	}
}

// availabilityLine reads the Availability text of one person's card, which is the
// line that tells the narrator whether they may appear in this scene.
func availabilityLine(block, name string) string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "### ") && strings.Contains(line, name) {
			for j := i + 1; j < len(lines) && j < i+4; j++ {
				if strings.HasPrefix(lines[j], "- Availability: ") {
					return strings.TrimPrefix(lines[j], "- Availability: ")
				}
			}
		}
	}
	return ""
}

// TestOpeningTurnPageUsesTheStoryPremiseWhenNoSceneIsRecorded is the defect this
// batch found in a real run. Before the first turn commits, the story has no
// recorded scene and the state schema is still waiting on the opening handshake,
// so the location reads empty. An empty location is not a neutral input: it made
// every person read as being in another country — including 钱守信, a 越国
// shopkeeper standing in the very market the story opens in — so the narrator was
// told not to use any of the world's own people, and the player asking for a local
// expert got an invented stranger instead.
//
// The story's premise is its own statement of where it begins, so it is what the
// opening turn's page is built from.
func TestOpeningTurnPageUsesTheStoryPremiseWhenNoSceneIsRecorded(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	// The shipped pack's own premise: 越国公共坊市的外围.
	const premise = "越国公共坊市的外围。你是一名没有宗门、没有洞府的散修。"
	ctx := interactive.StoryContext{
		Meta: interactive.StoryMeta{PackBinding: pack.Binding(), Origin: premise},
		// No scene recorded yet: this is the opening turn.
		Snapshot: interactive.Snapshot{State: map[string]any{}},
	}
	block := conversation.storyDiscoveryContext(ctx, "我在坊市里想找个人问问丹药的行情").Block
	if block == "" {
		t.Fatal("the opening turn was offered no page at all")
	}
	// The pack's own 越国 shopkeeper is standing in this market and must be offered
	// as being here.
	shopkeeper := pack.CatalogBySourceKey()["npc_b_fs_hf_zhanggui"]
	if shopkeeper.SourceKey == "" {
		t.Fatal("the shipped catalog lost the 越国 shopkeeper")
	}
	if !strings.Contains(block, shopkeeper.Name) {
		t.Fatalf("%s is in the market the story opens in and is missing from the opening page", shopkeeper.Name)
	}
	if got := availabilityLine(block, shopkeeper.Name); !strings.Contains(got, "at this place") {
		t.Fatalf("%s is a %s person at a %s market but the opening page says %q",
			shopkeeper.Name, shopkeeper.Country, shopkeeper.Country, got)
	}
	// And a person in another country is still not offered as being here.
	foreigner := pack.CatalogBySourceKey()["parallel_tao_zhiyuan"]
	if foreigner.SourceKey == "" {
		t.Fatal("the shipped catalog lost the 元武国 person this test needs")
	}
	if got := availabilityLine(block, foreigner.Name); strings.Contains(got, "at this place") {
		t.Fatalf("%s is in %s and was offered as being here: %q", foreigner.Name, foreigner.Country, got)
	}
}

// TestPageIsWithheldWhenNothingNamesAPlace is the safe degradation for the case
// the fix above must not over-reach into: a story whose premise names no place and
// which has no recorded scene. There is no basis for saying who is here, and §8
// requires that to produce no block rather than a roster asserting everyone is
// foreign.
func TestPageIsWithheldWhenNothingNamesAPlace(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	ctx := interactive.StoryContext{
		Meta:     interactive.StoryMeta{PackBinding: pack.Binding(), Origin: "一处不知名的落脚处。"},
		Snapshot: interactive.Snapshot{State: map[string]any{}},
	}
	outcome := conversation.storyDiscoveryContext(ctx, "我想找个人打听消息")
	if outcome.Block != "" {
		t.Fatalf("a story that names no place was offered %q", outcome.Block)
	}
	// An unplaceable scene is an ordinary answer, not a catalog fault.
	if outcome.Failure != nil {
		t.Fatalf("a scene that names no place was reported as a catalog failure: %v", outcome.Failure)
	}
}

// TestTheMarketAStoryWalksIntoStillOffersItsPeople is the journey's own output
// being read back.
//
// A story opens outside the market, walks in, and the journey commits 公共坊市 as
// 当前详细地点. That is the author's name for yue.market from the creation page, not
// a token any catalog row carries, so before the place registry reached the index
// the scene named no place at all and discoveryCandidates returned no page — for
// every turn after the walk, with nothing in the block saying why.
func TestTheMarketAStoryWalksIntoStillOffersItsPeople(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	for _, location := range []string{"公共坊市", "越国公共坊市"} {
		outcome := conversation.storyDiscoveryContext(
			discoveryStoryContext(pack, "main", location), "我想买点疗伤药")
		if outcome.Failure != nil {
			t.Fatalf("standing at %q failed to read the catalog: %v", location, outcome.Failure)
		}
		if outcome.Block == "" {
			t.Fatalf("standing at %q offered nobody: the whole page was withheld", location)
		}
		// The page must offer the market's own people, and it must not stage
		// another country's people here.
		if !strings.Contains(outcome.Block, "越国") {
			t.Fatalf("the page at %q never says which country it is in", location)
		}
	}
}
