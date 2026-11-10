package storypack

import (
	"strings"
	"testing"
)

// fanrenPack loads the shipped 凡人人界 pack for the catalog tests. It is the
// original catalog fixture; newer worlds have their own authored coverage tests.
func fanrenPack(t *testing.T) Pack {
	t.Helper()
	for _, pack := range builtinPacks() {
		if pack.ID == "fanren-jie" {
			return pack
		}
	}
	t.Fatal("the fanren-jie builtin pack is missing")
	return Pack{}
}

// TestShippedCatalogCoversEveryVerifiedProfile is the coverage half of the
// batch's acceptance: "目录应能检索全部已校验正式资料". Every profile body the
// pack ships must be reachable from a catalog row, and every row must point at a
// body that exists. A row without a body would offer a person the story cannot
// read; a body without a row would make a verified person unreachable no matter
// what the player asks for.
func TestShippedCatalogCoversEveryVerifiedProfile(t *testing.T) {
	pack := fanrenPack(t)
	if len(pack.Characters) != 107 {
		t.Fatalf("the catalog holds %d people, want the 103 original plus 4 verified Spirit World profiles", len(pack.Characters))
	}
	for _, id := range []string{"spirit_qiao_yan", "spirit_ruan_qingluo", "spirit_lu_shiheng", "spirit_ning_shutong"} {
		found := false
		for _, person := range pack.Characters {
			if person.SourceKey == id {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing new verified spirit profile %s", id)
		}
	}
	worldIDs := map[string]bool{}
	for _, entry := range pack.World {
		worldIDs[strings.TrimSpace(entry.ID)] = true
	}
	for _, character := range pack.Characters {
		character = character.Normalize()
		if err := character.Validate(); err != nil {
			t.Fatalf("catalog row is not usable: %v", err)
		}
		if character.LoreID == "" {
			t.Fatalf("%s (%s) has no profile entry, so its body can never be read",
				character.Name, character.SourceKey)
		}
		if !worldIDs[character.LoreID] {
			t.Fatalf("%s (%s) points at missing world entry %q",
				character.Name, character.SourceKey, character.LoreID)
		}
		if len(character.RegionTokens) == 0 {
			t.Fatalf("%s (%s) carries no region tokens, so it can never be placed in a scene",
				character.Name, character.SourceKey)
		}
	}
}

// TestCatalogBodiesCarryNoAuthoringPipeline is the §6.2 boundary: the world book
// is played, not read. A profile body that names the author's source key or the
// actor id hands the model a fact about the program instead of about the person,
// and the model then writes it into prose.
func TestCatalogBodiesCarryNoAuthoringPipeline(t *testing.T) {
	pack := fanrenPack(t)
	for _, entry := range pack.World {
		for _, term := range []string{"source_key", "作者来源键", "ActorID", "运行导入", "实现资料"} {
			if strings.Contains(entry.Content, term) {
				t.Fatalf("world entry %s leaks the authoring term %q", entry.ID, term)
			}
		}
	}
}

// TestShippedBodiesKeepTheAuthoredVoiceSections is the regression guard for the
// extraction that builds these bodies. The profile layout puts sections 6-8
// (表达与关系反应, 与谁有往来, 情境表达样例) in the character material and
// only the later asset/implementation section outside it, but the extractor used
// to stop at the first section numbered 6 or higher. That dropped all 156
// sections — every authored line about how a person speaks and what they do and
// do not know — while looking like a successful build.
//
// The counts are pinned rather than derived from the sources, because the whole
// failure mode is a regenerated pack that quietly contains less. A person whose
// author source really has no such sections (the canon and leader profiles end
// at 5, the boss has his own six-section layout) simply does not carry them, so
// this asserts the 52 that do rather than demanding them of everyone.
func TestShippedBodiesKeepTheAuthoredVoiceSections(t *testing.T) {
	pack := fanrenPack(t)
	const voiceSections = 52
	var withSix, withSeven, withEight int
	for _, entry := range pack.World {
		if strings.TrimSpace(entry.Type) != "character" {
			continue
		}
		body := entry.Content
		hasSix := strings.Contains(body, "## 6. 表达与关系反应")
		hasSeven := strings.Contains(body, "## 7. 与谁有往来")
		hasEight := strings.Contains(body, "## 8. 情境表达样例")
		if hasSix {
			withSix++
			if !hasSeven || !hasEight {
				t.Fatalf("%s keeps its voice section but lost the relation or sample section (7=%v 8=%v)",
					entry.ID, hasSeven, hasEight)
			}
		}
		if hasSeven {
			withSeven++
		}
		if hasEight {
			withEight++
		}
		for _, term := range []string{"能力、资产与改编约束", "使用与改编约束", "接入边界"} {
			if strings.Contains(body, term) {
				t.Fatalf("world entry %s ships the implementation section %q", entry.ID, term)
			}
		}
	}
	if withSix != voiceSections || withSeven != voiceSections || withEight != voiceSections {
		t.Fatalf("voice sections were dropped: 6=%d 7=%d 8=%d, want %d each",
			withSix, withSeven, withEight, voiceSections)
	}
}

// TestCandidatesLadderPutsNamedFirstThenLocalThenReachable is the §4.2 selection
// rule. Selection is by fit, never by position: the design's WD-04 defect was a
// catalog truncated by ID, which makes the same few people the only ones any
// story can ever find.
func TestCandidatesLadderPutsNamedFirstThenLocalThenReachable(t *testing.T) {
	pack := fanrenPack(t)
	catalog := pack.CatalogBySourceKey()
	local := catalog["npc_song_guanlan"]
	if local.SourceKey == "" {
		t.Fatal("the shipped catalog lost 宋观澜, the local array-study contact")
	}
	remote := catalog["canon_xin_ruyin"]
	if remote.SourceKey == "" {
		t.Fatal("the shipped catalog lost 辛如音")
	}

	// A request made at 黄枫谷 for someone who understands arrays.
	candidates := pack.CharacterCandidates(CandidateRequest{
		Location: "越国黄枫谷外的坊市",
		Action:   "我想找一位懂阵法的人请教残阵",
	})
	if len(candidates) == 0 {
		t.Fatal("a request that names a real field and a real place produced no candidates")
	}
	if candidates[0].Character.SourceKey != local.SourceKey {
		t.Fatalf("the first candidate is %s (%s), want the local array teacher %s",
			candidates[0].Character.Name, candidates[0].Character.SourceKey, local.SourceKey)
	}
	if candidates[0].Reach != ReachLocal {
		t.Fatalf("the local array teacher was offered as %q, want local", candidates[0].Reach)
	}
	// 辛如音 is in 元武国. She may be offered as a lead, and must never be offered
	// as being at this place.
	for _, candidate := range candidates {
		if candidate.Character.SourceKey == remote.SourceKey && candidate.Reach == ReachLocal {
			t.Fatalf("%s is in %s but was offered as local",
				remote.Name, remote.Country)
		}
	}
	// A local candidate with no topic match is still local: the scene's own
	// people are what a player can actually walk up to.
	for _, candidate := range candidates {
		if candidate.Reach == ReachLocal && len(candidate.Reasons) == 0 {
			t.Fatalf("%s was offered with no stated reason", candidate.Character.Name)
		}
	}
}

// TestPlaceClassTokenDoesNotPlaceForeignersLocally is the 瞬移 guard, and it was
// a real defect: 坊市, 山门, 居处, 静修处 and 外居 are tokens for people in 越国,
// 元武国 and 紫金国 alike, because every country has a market and a gate. Matching
// the token alone therefore placed a 元武国 person at the pack's own opening scene
// — 越国公共坊市的外围 — as someone who "may be met, introduced, or appear here".
//
// The scene's country and the person's country both have to agree before a token
// counts as evidence of being here.
func TestPlaceClassTokenDoesNotPlaceForeignersLocally(t *testing.T) {
	pack := fanrenPack(t)
	// The pack's own opening, and a market in each of the other two countries.
	scenes := []struct {
		location string
		country  string
	}{
		{"越国公共坊市的外围", "越国"},
		{"越国黄枫谷外的坊市", "越国"},
		{"元武国天星宗坊市", "元武国"},
	}
	for _, scene := range scenes {
		candidates := pack.CharacterCandidates(CandidateRequest{
			Location: scene.location,
			Action:   "我先在坊市外围打听消息",
		})
		if len(candidates) == 0 {
			t.Fatalf("no candidates at %s", scene.location)
		}
		for _, candidate := range candidates {
			if candidate.Reach != ReachLocal {
				continue
			}
			if candidate.Character.Country != scene.country {
				t.Fatalf("at %s (a %s scene) the %s person %s was offered as local — "+
					"a place-class token placed them here",
					scene.location, scene.country, candidate.Character.Country,
					candidate.Character.Name)
			}
		}
	}
	// The two 元武国 people who were actually misplaced, named explicitly so this
	// test fails for the right reason if the catalog is re-authored.
	for _, sourceKey := range []string{"npc_fan_jingmei", "npc_yin_nishang"} {
		character := pack.CatalogBySourceKey()[sourceKey]
		if character.SourceKey == "" {
			t.Fatalf("the shipped catalog lost %s", sourceKey)
		}
		for _, candidate := range pack.CharacterCandidates(CandidateRequest{
			Location: "越国公共坊市的外围",
			Action:   "我先打听消息",
		}) {
			if candidate.Character.SourceKey == sourceKey && candidate.Reach == ReachLocal {
				t.Fatalf("%s is in %s and was offered as local at a 越国 market",
					character.Name, character.Country)
			}
		}
	}
}

// TestSceneWithNoCountryPlacesNobody is the safe degradation: a location that
// names no country and no single-place token is not evidence of being anywhere, so
// nobody is offered as present. Guessing a country here is how the previous defect
// would come back.
func TestSceneWithNoCountryPlacesNobody(t *testing.T) {
	pack := fanrenPack(t)
	for _, candidate := range pack.CharacterCandidates(CandidateRequest{
		Location: "一处不知名的坊市外围",
		Action:   "我先打听消息",
	}) {
		if candidate.Reach == ReachLocal {
			t.Fatalf("%s was offered as local at a location that names no place",
				candidate.Character.Name)
		}
	}
}

// TestLocalPersonStaysLocalInTheirOwnCountry is the other half: the country gate
// must not make the pack's own opening people unreachable. A 越国 shopkeeper is
// local at a 越国 market even though 坊市 is not by itself a unique place.
func TestLocalPersonStaysLocalInTheirOwnCountry(t *testing.T) {
	pack := fanrenPack(t)
	candidates := pack.CharacterCandidates(CandidateRequest{
		Location: "越国公共坊市的外围",
		Action:   "我想买些丹药",
	})
	want := map[string]bool{
		"npc_b_fs_hf_zhanggui": false, // 钱守信, 丹药铺掌柜
		"npc_b_fs_tn_tanzhu":   false, // 王婆子, 符箓摊主
	}
	for _, candidate := range candidates {
		if _, ok := want[candidate.Character.SourceKey]; !ok {
			continue
		}
		if candidate.Reach != ReachLocal {
			t.Fatalf("%s is at this 越国 market but was offered as %q",
				candidate.Character.Name, candidate.Reach)
		}
		want[candidate.Character.SourceKey] = true
	}
	for sourceKey, found := range want {
		if !found {
			t.Fatalf("%s was not offered at all at their own market", sourceKey)
		}
	}
}

// rule: the ladder ranks by fit, but an explicit name is not a ranking, it is an
// instruction. A player who writes 辛如音 has asked for 辛如音, wherever she is.
func TestNamedRequestReachesAPersonInAnotherCountry(t *testing.T) {
	pack := fanrenPack(t)
	mentioned := pack.MentionedNames("我想去元武国找辛如音")
	if len(mentioned) != 1 || mentioned[0] != "辛如音" {
		t.Fatalf("a verbatim name in the player's text resolved to %v, want 辛如音", mentioned)
	}
	// A request for a field, with nobody named, must not resolve to a person.
	if got := pack.MentionedNames("我想找一位懂阵法的同道"); len(got) != 0 {
		t.Fatalf("a field request was read as naming %v", got)
	}

	candidates := pack.CharacterCandidates(CandidateRequest{
		Location: "越国黄枫谷外的坊市",
		Action:   "我想去元武国找辛如音",
		Named:    mentioned,
	})
	if len(candidates) == 0 {
		t.Fatal("a named request produced no candidates")
	}
	if candidates[0].Character.Name != "辛如音" || !candidates[0].Named {
		t.Fatalf("the first candidate is %s (named=%v), want the named 辛如音",
			candidates[0].Character.Name, candidates[0].Named)
	}
	if candidates[0].Reach == ReachLocal {
		t.Fatal("辛如音 is in another country and must not be offered as being here")
	}
}

// TestCandidatePageIsBoundedAndDropsWholeCards checks the capacity rule: the
// catalog holds 102 people and a turn is offered a page of them, and a card is
// never half-written.
func TestCandidatePageIsBoundedAndDropsWholeCards(t *testing.T) {
	pack := fanrenPack(t)
	candidates := pack.CharacterCandidates(CandidateRequest{
		Location: "越国黄枫谷外的坊市",
		Action:   "我想找一位懂阵法的人",
	})
	if len(candidates) > DefaultCandidateLimit {
		t.Fatalf("candidate page holds %d, over the limit of %d",
			len(candidates), DefaultCandidateLimit)
	}
	// Every card is whole: the block ends on a card boundary, never mid-card. It
	// is a prefix of the card sequence, so a card is either entirely present or
	// entirely absent.
	markdown := CandidateCardsMarkdown(candidates, DefaultCandidateMaxBytes)
	if len([]byte(markdown)) > DefaultCandidateMaxBytes {
		t.Fatalf("the rendered page is %d bytes, over its budget", len([]byte(markdown)))
	}
	var whole strings.Builder
	whole.WriteString(discoverablePeopleHeader)
	rendered := 0
	for _, candidate := range candidates {
		card := candidateCard(candidate)
		if whole.Len()+len([]byte(card)) > DefaultCandidateMaxBytes {
			break
		}
		whole.WriteString(card)
		rendered++
	}
	if markdown != strings.TrimSpace(whole.String()) {
		t.Fatalf("the page is not a whole-card prefix: %d of %d cards rendered",
			rendered, len(candidates))
	}
	if rendered == 0 {
		t.Fatal("the default budget rendered no cards at all")
	}
	// A tight budget must lose whole cards, not truncate one. A budget that fits
	// the header and exactly one card renders that card in full and nothing of
	// the next.
	oneCard := len(discoverablePeopleHeader) + len([]byte(candidateCard(candidates[0])))
	tight := CandidateCardsMarkdown(candidates, oneCard)
	if tight == "" {
		t.Fatal("a budget that fits one card rendered none")
	}
	if !strings.HasSuffix(tight, strings.TrimSpace(candidateCard(candidates[0]))) {
		t.Fatal("a one-card budget truncated the first card")
	}
	if strings.Contains(tight, candidateCard(candidates[1])) {
		t.Fatal("a one-card budget rendered a second card")
	}
	// A budget too small for any card yields no block at all, rather than a
	// header announcing a roster the model is not being shown.
	if got := CandidateCardsMarkdown(candidates, len(discoverablePeopleHeader)); got != "" {
		t.Fatalf("an over-tight budget produced %q, want no block", got)
	}
}

// TestMatchedLeadsSurviveALargeLocalPage is the capacity half of the ladder, and
// it was a real defect. 越国黄枫谷外的坊市 places forty people at the scene. A
// request about arrays matched one of them; the two people in the same country
// who actually know arrays ranked fortieth and forty-first, so the page of twelve
// offered one relevant person and eleven who had nothing to say about arrays.
// 查阵法 is answered by 当地同道或异地引荐, so a page that can never reach the
// second half has not answered the request.
func TestMatchedLeadsSurviveALargeLocalPage(t *testing.T) {
	pack := fanrenPack(t)
	candidates := pack.CharacterCandidates(CandidateRequest{
		Location: "越国黄枫谷外的坊市",
		Action:   "我想找一位懂阵法的人",
	})
	if len(candidates) > DefaultCandidateLimit {
		t.Fatalf("candidate page holds %d, over the limit of %d",
			len(candidates), DefaultCandidateLimit)
	}
	// The people who match the request and are not here must be on the page.
	wantLeads := map[string]bool{"canon_qi_yunxiao": false, "canon_xin_ruyin": false}
	leads := 0
	for _, candidate := range candidates {
		if candidate.Reach == ReachLocal {
			continue
		}
		leads++
		if _, ok := wantLeads[candidate.Character.SourceKey]; ok {
			wantLeads[candidate.Character.SourceKey] = true
		}
	}
	for sourceKey, found := range wantLeads {
		if !found {
			t.Fatalf("the array request never reached %s: a large local page "+
				"crowded out the lead that answers it", sourceKey)
		}
	}
	if leads > DefaultCandidateLeadLimit {
		t.Fatalf("leads took %d seats of %d, over the reserved share of %d",
			leads, DefaultCandidateLimit, DefaultCandidateLeadLimit)
	}
	// The reservation is a share, not a takeover: the people who can actually be
	// walked up to still hold most of the page.
	if leads > DefaultCandidateLimit/2 {
		t.Fatalf("%d of %d seats went to people who are not here", leads, DefaultCandidateLimit)
	}
}

// TestLocalPageIsNotShortenedWithoutLeads is the other half of the reservation:
// a scene whose request has no reachable lead elsewhere must not lose local seats
// to a quota nobody can fill.
func TestLocalPageIsNotShortenedWithoutLeads(t *testing.T) {
	pack := fanrenPack(t)
	candidates := pack.CharacterCandidates(CandidateRequest{
		Location: "越国黄枫谷外的坊市",
		Action:   "我想同门请教功法",
	})
	if len(candidates) != DefaultCandidateLimit {
		t.Fatalf("a scene with a full local roster offered %d candidates, want the full page of %d",
			len(candidates), DefaultCandidateLimit)
	}
	locals := 0
	for _, candidate := range candidates {
		if candidate.Reach == ReachLocal {
			locals++
		}
	}
	if locals < DefaultCandidateLimit-DefaultCandidateLeadLimit {
		t.Fatalf("only %d of %d seats went to people at this place, so the page was "+
			"shortened for leads that do not exist", locals, DefaultCandidateLimit)
	}
}

// TestEstablishedPeopleAreNotReOffered is what stops a story from introducing
// someone it has already met: once a source key is bound to an actor on this
// branch, the catalog stops offering them as a discovery.
func TestEstablishedPeopleAreNotReOffered(t *testing.T) {
	pack := fanrenPack(t)
	req := CandidateRequest{
		Location: "越国黄枫谷外的坊市",
		Action:   "我想找一位懂阵法的人",
	}
	before := pack.CharacterCandidates(req)
	if len(before) == 0 {
		t.Fatal("no candidates to filter")
	}
	first := before[0].Character.SourceKey

	req.Established = map[string]bool{first: true}
	for _, candidate := range pack.CharacterCandidates(req) {
		if candidate.Character.SourceKey == first {
			t.Fatalf("%s was re-offered after being established", candidate.Character.Name)
		}
	}
}

// TestSameNameInTwoPacksStaysTwoPeople is the cross-pack identity rule. A name is
// not an identity: two packs may each ship a 陈巧倩, and a story in one must
// never resolve the other's actor.
func TestSameNameInTwoPacksStaysTwoPeople(t *testing.T) {
	first := Pack{
		Version: 1, ID: "pack-a", Name: "甲",
		Story: PackStory{Title: "甲", Origin: "甲"},
		Characters: []CatalogCharacter{{
			SourceKey: "a_chen", Name: "陈巧倩", Country: "越国",
			RegionTokens: []string{"黄枫谷"}, LoreID: "a-chen",
		}},
		World: []WorldEntry{{ID: "a-chen", Name: "陈巧倩", Content: "甲包的陈巧倩。"}},
	}
	second := Pack{
		Version: 1, ID: "pack-b", Name: "乙",
		Story: PackStory{Title: "乙", Origin: "乙"},
		Characters: []CatalogCharacter{{
			SourceKey: "b_chen", Name: "陈巧倩", Country: "越国",
			RegionTokens: []string{"黄枫谷"}, LoreID: "b-chen",
		}},
		World: []WorldEntry{{ID: "b-chen", Name: "陈巧倩", Content: "乙包的陈巧倩。"}},
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("first pack is invalid: %v", err)
	}
	if err := second.Validate(); err != nil {
		t.Fatalf("second pack is invalid: %v", err)
	}
	// The hash is what a story binds to, so two packs sharing a name but not a
	// person must not share a revision either.
	if first.ContentHash() == second.ContentHash() {
		t.Fatal("two packs with different people hashed the same")
	}
	// And each pack resolves the name to its own person only.
	if got := first.FindCatalogByName("陈巧倩"); len(got) != 1 || got[0].SourceKey != "a_chen" {
		t.Fatalf("first pack resolved 陈巧倩 to %+v", got)
	}
	if got := second.FindCatalogByName("陈巧倩"); len(got) != 1 || got[0].SourceKey != "b_chen" {
		t.Fatalf("second pack resolved 陈巧倩 to %+v", got)
	}
}

// TestDuplicateSourceKeysAreRejected keeps the author key unique inside one pack:
// two rows sharing it would make the same person two actors.
func TestDuplicateSourceKeysAreRejected(t *testing.T) {
	pack := Pack{
		Version: 1, ID: "dup", Name: "重复",
		Story: PackStory{Title: "t", Origin: "o"},
		World: []WorldEntry{{ID: "w", Name: "w", Content: "c"}},
		Characters: []CatalogCharacter{
			{SourceKey: "same", Name: "甲", LoreID: "w"},
			{SourceKey: "same", Name: "乙", LoreID: "w"},
		},
	}
	if err := pack.Validate(); err == nil {
		t.Fatal("two catalog rows sharing a source key were accepted")
	}
}

// TestASceneNamedByThePlaceARouteCommitsStillFindsItsPeople is the discovery
// page's first real premise: the protagonist walks to the market, the journey
// commits 公共坊市 as 当前详细地点, and every later turn has to read that string as
// somewhere in 越国.
//
// 公共坊市 is the creation page's name for yue.market, not a token any catalog row
// carries, so before the place registry fed the index the answer was "no place
// here" and the page was withheld for the rest of the story. The market's own
// people were never offered, and nothing in the block said why.
func TestASceneNamedByThePlaceARouteCommitsStillFindsItsPeople(t *testing.T) {
	pack := fanrenPack(t)
	for _, scene := range []string{"公共坊市", "越国公共坊市", "公共坊市西门"} {
		if !pack.LocationNamesAPlace(scene) {
			t.Fatalf("%q is where a journey can put the character, and names no place", scene)
		}
	}
	candidates := pack.CharacterCandidates(CandidateRequest{
		Location: "公共坊市",
		Action:   "我想买点疗伤药",
	})
	if len(candidates) == 0 {
		t.Fatal("standing in the market offered nobody")
	}
	var local int
	for _, candidate := range candidates {
		if candidate.Reach == ReachLocal {
			local++
			if candidate.Character.Country != "越国" {
				t.Fatalf("%s is local to the market but in %s", candidate.Character.SourceKey, candidate.Character.Country)
			}
		}
	}
	if local == 0 {
		t.Fatal("nobody in the market reads as being in the market")
	}
}

// TestOneCountrysPlaceStillPlacesTheSceneEvenWhenItHasManyHalls is the other half
// of the same judgement.
//
// 黄枫谷 is twenty-four halls in 越国 and nowhere else, so a scene that says
// 黄枫谷 is in 越国 whichever hall it means. Requiring the token to name exactly
// one place threw that away and withheld the page at the sect's own gate. What
// must still be refused is a word two countries use for a kind of place: 坊市 and
// 山门 name places in all three, and on their own they say only what kind of
// place this is.
func TestOneCountrysPlaceStillPlacesTheSceneEvenWhenItHasManyHalls(t *testing.T) {
	pack := fanrenPack(t)
	for _, scene := range []string{"黄枫谷", "越国黄枫谷", "黄枫谷外门修行区", "医馆", "越国医馆"} {
		if !pack.LocationNamesAPlace(scene) {
			t.Fatalf("%q names one country's places and was refused", scene)
		}
	}
	for _, scene := range []string{"坊市", "山门", "静修处", "居处", "外居"} {
		if pack.LocationNamesAPlace(scene) {
			t.Fatalf("%q is a kind of place several countries have, and placed the scene in one", scene)
		}
	}
}

// TestTheAuthoredBossIsOfferedWhereHeActuallyWaits pins the defect that made a
// regional boss a background legend instead of an encounter.
//
// The boss used to exist only as a world entry typed "setting". A setting is
// normalized to "other", and the candidate page is built from the character
// catalog alone, so he was never offered to anyone: the model read his whole
// entry, understood him as lore, and invented a generic highway robber for the
// scene. An opponent nobody can be handed is not an authored boss. What this
// asks is the question the turn actually asks — standing on his road, looking
// for whoever waits there — and the answer has to include him, at his own stage,
// reachable as someone physically present.
func TestTheAuthoredBossIsOfferedWhereHeActuallyWaits(t *testing.T) {
	pack := fanrenPack(t)
	const sourceKey = "npc_g11_duandao_ke"
	person, ok := pack.CatalogBySourceKey()[sourceKey]
	if !ok {
		t.Fatalf("the authored boss is missing from the catalog: %s", sourceKey)
	}
	if person.InitialStage != "炼气5层" {
		t.Fatalf("the boss is catalogued at %q, the content card says 炼气5层", person.InitialStage)
	}
	found := false
	for _, candidate := range pack.CharacterCandidates(CandidateRequest{
		Location: "越国坊市外围散修人家",
		Action:   "我在坊市外围的土路上打听，最近这一带夜里有没有人拦路。",
	}) {
		if candidate.Character.SourceKey != sourceKey {
			continue
		}
		found = true
		if candidate.Reach != ReachLocal {
			t.Fatalf("the boss is offered as %q at the road he waits on, want local", candidate.Reach)
		}
	}
	if !found {
		t.Fatal("the boss is not on the candidate page at his own road, so no turn can stage him")
	}
}

// TestTheBossAndHisEntryAreTheSamePerson keeps the two records from drifting:
// the catalog row and the world entry describe one person, and a candidate card
// that says one thing while the entry says another is how a boss stops matching
// his own introduction.
func TestTheBossAndHisEntryAreTheSamePerson(t *testing.T) {
	pack := fanrenPack(t)
	const sourceKey = "npc_g11_duandao_ke"
	person, ok := pack.CatalogBySourceKey()[sourceKey]
	if !ok {
		t.Fatalf("the authored boss is missing from the catalog: %s", sourceKey)
	}
	var entry WorldEntry
	for _, candidate := range pack.World {
		if candidate.ID == person.LoreID {
			entry = candidate
			break
		}
	}
	if entry.ID == "" {
		t.Fatalf("the catalog row points at %q, which is not in the world book", person.LoreID)
	}
	if entry.Type != "character" {
		t.Fatalf("the boss's entry is typed %q; anything else is read as lore rather than a person", entry.Type)
	}
	if entry.Name != person.Name {
		t.Fatalf("catalog says %q, the entry says %q", person.Name, entry.Name)
	}
	if strings.TrimSpace(entry.BriefDescription) != strings.TrimSpace(person.Brief) {
		t.Fatal("the candidate card and the lore entry would introduce him differently")
	}
}
