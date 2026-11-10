package interactiveapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ta/internal/interactive"
	"ta/internal/presetlayout"
	"ta/internal/storypack"
)

// offeredPage is the block the discovery renderer produces for a set of
// candidates, which is what the binding is allowed to read.
func offeredPage(candidates []storypack.Candidate) string {
	return storypack.CandidateCardsMarkdown(candidates, storypack.DefaultCandidateMaxBytes)
}

func localCandidate(pack storypack.Pack, sourceKey string) storypack.Candidate {
	return storypack.Candidate{
		Character: pack.CatalogBySourceKey()[sourceKey],
		Reach:     storypack.ReachLocal,
		Reasons:   []string{"matched this place"},
	}
}

// TestStagedPersonIsBoundToTheAuthorKeyWithoutTheModelSupplyingOne is the C4
// identity requirement. §7.1 says the source-key-to-actor mapping must not depend
// on the model minting an id, and a real run showed the model reading the page,
// using the catalog's own name, and submitting no key at all — so the preset
// person was registered as a brand-new actor and would be cast twice.
func TestStagedPersonIsBoundToTheAuthorKeyWithoutTheModelSupplyingOne(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	shopkeeper := pack.CatalogBySourceKey()["npc_b_fs_hf_zhanggui"]
	if shopkeeper.SourceKey == "" {
		t.Fatal("the shipped catalog lost the shopkeeper this test needs")
	}

	conversation.recordOfferedPeople(
		[]storypack.Candidate{localCandidate(pack, "npc_b_fs_hf_zhanggui")},
		offeredPage([]storypack.Candidate{localCandidate(pack, "npc_b_fs_hf_zhanggui")}),
	)

	facts := &interactive.TurnCharacterFactsInput{
		Profiles: []interactive.TurnCharacterProfileInput{{
			CharacterID: shopkeeper.Name,
			Name:        shopkeeper.Name,
			KnownAs:     "钱掌柜",
			Role:        "越国坊市丹铺掌柜",
		}},
	}
	conversation.bindStagedPeople(facts)

	if got := facts.Profiles[0].SourceKey; got != shopkeeper.SourceKey {
		t.Fatalf("a person staged from the page carries source_key %q, want %q", got, shopkeeper.SourceKey)
	}
	// The protagonist's own name for them is not a rename, and the binding must
	// not have touched it.
	if facts.Profiles[0].KnownAs != "钱掌柜" {
		t.Fatalf("the binding rewrote known_as to %q", facts.Profiles[0].KnownAs)
	}
}

// TestInventedPersonIsNotGivenAPresetIdentity is the other half: a person the
// story made up must not borrow an author key, or every invented shopkeeper would
// become the preset one.
func TestInventedPersonIsNotGivenAPresetIdentity(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")

	conversation.recordOfferedPeople(
		[]storypack.Candidate{localCandidate(pack, "npc_b_fs_hf_zhanggui")},
		offeredPage([]storypack.Candidate{localCandidate(pack, "npc_b_fs_hf_zhanggui")}),
	)

	facts := &interactive.TurnCharacterFactsInput{
		Profiles: []interactive.TurnCharacterProfileInput{
			{CharacterID: "陈三", Name: "陈三", Role: "卖炊饼的"},
			{CharacterID: "老陈", Name: "老陈", Role: "青禾村的采药人"},
		},
	}
	conversation.bindStagedPeople(facts)

	for _, profile := range facts.Profiles {
		if profile.SourceKey != "" {
			t.Fatalf("invented person %s was given the preset key %q", profile.Name, profile.SourceKey)
		}
	}
}

// TestAPersonInAnotherCountryIsNeverBoundAsBeingHere guards the boundary the
// batch calls 瞬移: a person the page offered as a distant lead must not be bound,
// because binding them would present their arrival as a correct identity.
func TestAPersonInAnotherCountryIsNeverBoundAsBeingHere(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	remote := pack.CatalogBySourceKey()["canon_xin_ruyin"]
	if remote.SourceKey == "" {
		t.Fatal("the shipped catalog lost the remote person this test needs")
	}

	candidates := []storypack.Candidate{{
		Character: remote,
		Reach:     storypack.ReachRemote,
		Reasons:   []string{"named by the player"},
	}}
	conversation.recordOfferedPeople(candidates, offeredPage(candidates))

	facts := &interactive.TurnCharacterFactsInput{
		Profiles: []interactive.TurnCharacterProfileInput{{
			CharacterID: remote.Name, Name: remote.Name,
		}},
	}
	conversation.bindStagedPeople(facts)

	if facts.Profiles[0].SourceKey != "" {
		t.Fatalf("%s is in %s but was bound as being at this scene", remote.Name, remote.Country)
	}
}

// TestAnAmbiguousNameIsRefusedRatherThanGuessed locks the design's rule that two
// people are never merged for sharing a name. When one printed name answers to
// two different author keys, neither may be assumed.
func TestAnAmbiguousNameIsRefusedRatherThanGuessed(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	shared := storypack.CatalogCharacter{SourceKey: "npc_a", Name: "陈巧倩", Country: "越国"}
	other := storypack.CatalogCharacter{SourceKey: "npc_b", Name: "陈巧倩", Country: "越国"}
	candidates := []storypack.Candidate{
		{Character: shared, Reach: storypack.ReachLocal, Reasons: []string{"matched this place"}},
		{Character: other, Reach: storypack.ReachLocal, Reasons: []string{"matched this place"}},
	}
	conversation.recordOfferedPeople(candidates, offeredPage(candidates))

	facts := &interactive.TurnCharacterFactsInput{
		Profiles: []interactive.TurnCharacterProfileInput{{
			CharacterID: "陈巧倩", Name: "陈巧倩",
		}},
	}
	conversation.bindStagedPeople(facts)

	if facts.Profiles[0].SourceKey != "" {
		t.Fatalf("an ambiguous name was resolved to %q instead of being refused", facts.Profiles[0].SourceKey)
	}
}

// TestAKeyTheModelSuppliedIsNeverOverridden keeps the binding subordinate to the
// model's own evidence: a submitted key is authoritative and is left alone.
func TestAKeyTheModelSuppliedIsNeverOverridden(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")

	conversation.recordOfferedPeople(
		[]storypack.Candidate{localCandidate(pack, "npc_b_fs_hf_zhanggui")},
		offeredPage([]storypack.Candidate{localCandidate(pack, "npc_b_fs_hf_zhanggui")}),
	)

	facts := &interactive.TurnCharacterFactsInput{
		Profiles: []interactive.TurnCharacterProfileInput{{
			CharacterID: "钱守信", Name: "钱守信", SourceKey: "npc_some_other_person",
		}},
	}
	conversation.bindStagedPeople(facts)

	if facts.Profiles[0].SourceKey != "npc_some_other_person" {
		t.Fatalf("the model's own key was overwritten with %q", facts.Profiles[0].SourceKey)
	}
}

// TestAPageThatWasWithheldBindsNobody covers the silent-failure path: when the
// page cannot be built the narrator is shown nothing, so nothing may be bound
// against it. A stale record from an earlier cycle must not survive.
func TestAPageThatWasWithheldBindsNobody(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	shopkeeper := pack.CatalogBySourceKey()["npc_b_fs_hf_zhanggui"]

	// A first cycle that did offer the shopkeeper.
	conversation.recordOfferedPeople(
		[]storypack.Candidate{localCandidate(pack, "npc_b_fs_hf_zhanggui")},
		offeredPage([]storypack.Candidate{localCandidate(pack, "npc_b_fs_hf_zhanggui")}),
	)
	// A later pass whose page was withheld. The recording clears.
	conversation.storyDiscoveryContext(
		interactive.StoryContext{
			Meta: interactive.StoryMeta{PackBinding: pack.Binding()},
			Snapshot: interactive.Snapshot{State: map[string]any{"actors": map[string]any{
				interactive.DefaultStoryContextActorID: map[string]any{
					"state": map[string]any{"当前详细地点": "一处不知名的落脚处。"},
				},
			}}},
		}, "我在坊市里想找个人问问丹药的行情")

	facts := &interactive.TurnCharacterFactsInput{
		Profiles: []interactive.TurnCharacterProfileInput{{
			CharacterID: shopkeeper.Name, Name: shopkeeper.Name,
		}},
	}
	conversation.bindStagedPeople(facts)

	if facts.Profiles[0].SourceKey != "" {
		t.Fatalf("a withheld page still bound %q", facts.Profiles[0].SourceKey)
	}
}

// TestThePageRecordsOnlyWhatTheNarratorWasShown ties the binding to the rendered
// block: the renderer drops whole cards that do not fit, so a person whose card
// never reached the narrator must not be bindable either.
func TestThePageRecordsOnlyWhatTheNarratorWasShown(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	shopkeeper := pack.CatalogBySourceKey()["npc_b_fs_hf_zhanggui"]

	candidates := []storypack.Candidate{localCandidate(pack, "npc_b_fs_hf_zhanggui")}
	// A block that is not the rendered page — the narrator was shown no card.
	conversation.recordOfferedPeople(candidates, "## Discoverable People\n\n")

	facts := &interactive.TurnCharacterFactsInput{
		Profiles: []interactive.TurnCharacterProfileInput{{
			CharacterID: shopkeeper.Name, Name: shopkeeper.Name,
		}},
	}
	conversation.bindStagedPeople(facts)

	if facts.Profiles[0].SourceKey != "" {
		t.Fatalf("a person absent from the shown page was bound to %q", facts.Profiles[0].SourceKey)
	}
}

// TestTheBindingSurvivesTheRealPageShape runs the whole path through the real
// discovery context, so the regexp and the renderer cannot drift apart: the page
// the narrator actually receives is the page the binding reads.
func TestTheBindingSurvivesTheRealPageShape(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	shopkeeper := pack.CatalogBySourceKey()["npc_b_fs_hf_zhanggui"]

	block := conversation.storyDiscoveryContext(
		discoveryStoryContext(pack, "main", "越国西境坊市，主街斜对面巷口茶棚"),
		"我在坊市里想找个人问问丹药的行情").Block
	if block == "" {
		t.Fatal("no discovery block was built for the shopkeeper's own market")
	}
	if !strings.Contains(block, shopkeeper.Name) {
		t.Fatalf("the shopkeeper %s is missing from the page built at his own market", shopkeeper.Name)
	}

	facts := &interactive.TurnCharacterFactsInput{
		Profiles: []interactive.TurnCharacterProfileInput{{
			CharacterID: shopkeeper.Name, Name: shopkeeper.Name, KnownAs: "钱掌柜",
		}},
	}
	conversation.bindStagedPeople(facts)

	if got := facts.Profiles[0].SourceKey; got != shopkeeper.SourceKey {
		t.Fatalf("the real page did not bind %s: got %q, want %q", shopkeeper.Name, got, shopkeeper.SourceKey)
	}
}

// TestAForgedSourceKeyIsReplacedByThePacksOwnKey is §U3's "来源key必须由当前绑定
// 世界包核对，模型不能凭任意key换人": a key the model wrote from memory must be
// checked against the pack the story is bound to, not trusted.
func TestAForgedSourceKeyIsReplacedByThePacksOwnKey(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	shopkeeper := pack.CatalogBySourceKey()["npc_b_fs_hf_zhanggui"]
	other := pack.CatalogBySourceKey()["canon_xin_ruyin"]
	if shopkeeper.SourceKey == "" || other.SourceKey == "" {
		t.Fatal("the shipped catalog lost the people this test needs")
	}

	facts := &interactive.TurnCharacterFactsInput{
		Profiles: []interactive.TurnCharacterProfileInput{{
			CharacterID: shopkeeper.Name, Name: shopkeeper.Name,
			// A real key, but not this person's: the model remembered the pack has
			// such a person and attached the wrong one.
			SourceKey: other.SourceKey,
		}},
	}
	conversation.sourceKeyAliases(facts)

	if got := facts.Profiles[0].SourceKey; got != shopkeeper.SourceKey {
		t.Fatalf("the wrong key %q survived; got %q, want the pack's own key %q", other.SourceKey, got, shopkeeper.SourceKey)
	}
}

// TestAnInventedSourceKeyIsDroppedRatherThanBound is the other half: a key the
// bound pack does not ship at all cannot name anyone here, and inventing a
// binding is worse than having none.
func TestAnInventedSourceKeyIsDroppedRatherThanBound(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)

	facts := &interactive.TurnCharacterFactsInput{
		Profiles: []interactive.TurnCharacterProfileInput{{
			CharacterID: "陈三", Name: "陈三",
			SourceKey: "npc_that_no_pack_ships",
		}},
	}
	conversation.sourceKeyAliases(facts)

	if got := facts.Profiles[0].SourceKey; got != "" {
		t.Fatalf("a key no pack ships survived as %q", got)
	}
}

// TestAKeyFromAnotherPackCannotBind is the cross-pack case: a story in the mortal
// world must not accept an author key that only another pack ships. The second
// pack is written into this test's own data root, because the builtin detective
// pack ships no character catalog to borrow a key from.
func TestAKeyFromAnotherPackCannotBind(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	fanren := fanrenFormalPack(t, "")
	foreignKey := "detective_landlord_zhou"
	writeForeignPack(t, conversation.novaDir, foreignKey)

	facts := &interactive.TurnCharacterFactsInput{
		Profiles: []interactive.TurnCharacterProfileInput{{
			CharacterID: "陈三", Name: "陈三", SourceKey: foreignKey,
		}},
	}
	conversation.sourceKeyAliases(facts)

	if got := facts.Profiles[0].SourceKey; got != "" {
		t.Fatalf("a key from another pack survived in a story bound to %s: %q", fanren.ID, got)
	}
}

// writeForeignPack writes a second, unrelated pack into the story's own data root
// so a cross-pack key is a key the library really can resolve — just not for this
// story.
func writeForeignPack(t *testing.T, dataRoot, sourceKey string) {
	t.Helper()
	dir := presetlayout.StoryPacks(dataRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pack := storypack.Pack{
		Version: 1, ID: "wumian-zhentan", Name: "雾月侦探", ContentVersion: 1,
		Characters: []storypack.CatalogCharacter{{
			SourceKey: sourceKey, Name: "周房东", Role: "雾月街的房东",
		}},
	}
	raw, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wumian-zhentan.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestAVerifiedAliasKeyIsAcceptedAsWritten is the ordinary case: a key the bound
// pack really ships for this person is left exactly as the model submitted it,
// including when the profile uses one of the author's verified aliases.
func TestAVerifiedAliasKeyIsAcceptedAsWritten(t *testing.T) {
	workspace := t.TempDir()
	novaDir := t.TempDir()
	dir := presetlayout.StoryPacks(novaDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pack := storypack.Pack{
		Version: 1, ID: "aliased-world", Name: "别名世界", ContentVersion: 1,
		Characters: []storypack.CatalogCharacter{{
			SourceKey: "npc_aliased_innkeeper", Name: "柳掌柜",
			Aliases: []string{"柳老板", "柳三娘"},
		}},
	}
	raw, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "aliased-world.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "别名世界", Origin: "测试原点", ReplyTargetChars: 800,
		PackBinding: pack.Binding(),
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, novaDir, workspace, story.ID, "", "我先四处看看", story.ReplyTargetChars, nil)

	facts := &interactive.TurnCharacterFactsInput{
		Profiles: []interactive.TurnCharacterProfileInput{{
			CharacterID: "柳老板", Name: "柳老板", SourceKey: "npc_aliased_innkeeper",
		}},
	}
	conversation.sourceKeyAliases(facts)

	if got := facts.Profiles[0].SourceKey; got != "npc_aliased_innkeeper" {
		t.Fatalf("a verified alias key was rewritten to %q", got)
	}
}
