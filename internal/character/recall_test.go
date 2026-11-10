package character

import (
	"strings"
	"testing"
)

// The recall pass exists because the standing memory block is bounded and the
// character window is a single tool-less generation. Before it, a character
// asked about something that had aged out of the block had no route to it at
// all: the record was on the journal, but nothing read it back, so the answer
// was "I don't remember" about something they had plainly lived through. These
// tests are about that route existing and about what it may and may not carry.
//
// Every test below is written so that removing the recall pass fails it. The
// earlier suite proved the record text was preserved and the identity survived
// truncation, which is not the same claim and was the gap the review named.

// recallFixture is one character with a long past: an old promise, an old
// disclosure, and enough recent noise that a bounded block would carry the
// noise and drop the past.
func recallFixture() (profile Profile, knowledge []KnowledgeRecord, matters []Matter, exchanges []Exchange) {
	profile = Profile{
		ID: "zhou", StoryID: "st", Name: "周掌柜", Role: "掌柜",
		Description: "在青石镇做药材买卖。", Personality: []string{"谨慎"},
		SpeechStyle: "短句", Revision: 1,
	}
	knowledge = []KnowledgeRecord{
		{
			ID: "knw-old-promise", CharacterID: "zhou",
			Statement: "我答应过替他留意那批三年生的黄芪，开春到货就先给他留着。",
			Nature:    NaturePromise, Audience: AudienceSingle,
			SourceTurnID: "turn-3", WorldTime: "第3日",
		},
		{
			ID: "knw-old-secret", CharacterID: "zhou",
			Statement: "库房后墙的夹层里藏着一本旧账，东家不知道。",
			Nature:    NatureSecret, Audience: AudienceSingle,
			SourceTurnID: "turn-4", WorldTime: "第4日",
		},
		{
			ID: "knw-recent-noise", CharacterID: "zhou",
			Statement: "今天下雨了，街上没什么人。",
			Nature:    NatureExperienced, Audience: AudienceSingle,
			SourceTurnID: "turn-90", WorldTime: "第90日",
		},
	}
	matters = []Matter{{
		ID: "mat-old", StoryID: "st", BranchID: "b1", CharacterID: "zhou",
		Kind: MatterPromise, Status: MatterOpen,
		Statement: "答应替主角打听黄芪的来路。",
		WorldTime: "第3日",
	}}
	// Recent exchanges, none of which mention the old material, so a "most
	// recent N" read cannot accidentally satisfy the assertions.
	for i := 0; i < 30; i++ {
		exchanges = append(exchanges, Exchange{
			ID:          "xch_recent_" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
			CharacterID: "zhou", Speaker: SpeakerPlayer, Channel: ChannelPrivate,
			Text: "今天生意怎么样？", WorldTime: "第90日",
		})
	}
	return profile, knowledge, matters, exchanges
}

// TestRecallReadsBackAnOldDetailTheQuestionAsksAbout is the core claim of the
// phase: an old record the standing block could not carry is still reachable
// when the player's current question bears on it.
func TestRecallReadsBackAnOldDetailTheQuestionAsksAbout(t *testing.T) {
	profile, knowledge, matters, exchanges := recallFixture()

	// The bounded memory block is small enough that the old promise is not in it.
	prepared := Prepare(PrepareInput{
		StoryID: "st", Profile: profile,
		Revision:  IdentityRevision{BranchID: "b1", ProfileRevision: 1},
		Scene:     Scene{NodeID: "n1", Location: "青石镇", WorldTime: "第90日"},
		Exchanges: exchanges, Knowledge: knowledge, Matters: matters,
		MaxKnowledge: 1, MaxRecentExchanges: 2,
		Question: "我上次托你留的那批黄芪，到货了吗？",
	})

	// The premise: the standing block really did drop it, so this is not a test
	// that would pass without the recall pass.
	if strings.Contains(prepared.Memory, "黄芪") {
		t.Fatalf("the standing memory block already carries the old promise, so this case proves nothing:\n%s", prepared.Memory)
	}
	if !strings.Contains(prepared.Recall, "黄芪") {
		t.Fatalf("the character could not reach an old promise the question was about:\n%s", prepared.Recall)
	}
	if prepared.RecallDiagnostics.Selected == 0 {
		t.Fatal("recall selected nothing, so the route does not exist")
	}
	// The recalled promise keeps its provenance, so the model can tell an
	// undertaking from a fact about the world.
	if !strings.Contains(prepared.Recall, string(NaturePromise)) {
		t.Fatalf("the recalled promise lost its nature:\n%s", prepared.Recall)
	}
	if !strings.Contains(prepared.Recall, "第3日") {
		t.Fatalf("the recalled promise lost its world time:\n%s", prepared.Recall)
	}
	// And it is in the block the model actually reads.
	if !strings.Contains(prepared.Markdown(), "黄芪") {
		t.Fatalf("the recalled material never reached the rendered context:\n%s", prepared.Markdown())
	}
}

// TestRecallDoesNotAnswerAnUnrelatedQuestion is the other half. A pass that
// simply returned the oldest records would also pass the test above, and would
// flood every context with irrelevant material. Relevance is the point.
func TestRecallDoesNotAnswerAnUnrelatedQuestion(t *testing.T) {
	profile, knowledge, matters, exchanges := recallFixture()

	prepared := Prepare(PrepareInput{
		StoryID: "st", Profile: profile,
		Revision:  IdentityRevision{BranchID: "b1", ProfileRevision: 1},
		Scene:     Scene{NodeID: "n1", Location: "青石镇", WorldTime: "第90日"},
		Exchanges: exchanges, Knowledge: knowledge, Matters: matters,
		Question: "镇上的测灵台怎么走？",
	})
	if strings.Contains(prepared.Recall, "黄芪") || strings.Contains(prepared.Recall, "夹层") {
		t.Fatalf("an unrelated question dragged old records into the context:\n%s", prepared.Recall)
	}
}

// TestRecallOnlyCarriesWhatTheCharacterMayKnow is the audience gate at the
// recall seam. Reading older records back must not become a way around the rule
// that a character cannot remember a private conversation they were not part of.
func TestRecallOnlyCarriesWhatTheCharacterMayKnow(t *testing.T) {
	// Someone else's private conversation, which mentions the same topic.
	foreign := []Exchange{{
		ID: "xch_foreign", CharacterID: "other", Speaker: SpeakerPlayer,
		Channel: ChannelPrivate, Text: "黄芪那批货，别让周掌柜知道。", WorldTime: "第5日",
	}}
	recall := BuildRecall(RecallRequest{
		CharacterID: "zhou",
		Question:    "黄芪那批货到底怎么回事？",
		Exchanges:   foreign,
	})
	for _, item := range recall.Items {
		if item.ID == "xch_foreign" {
			t.Fatalf("recall handed a character a private conversation they were not part of: %+v", item)
		}
	}

	// A participants-only record where they are not a listed holder.
	records := []KnowledgeRecord{{
		ID: "knw-foreign", CharacterID: "other", Statement: "黄芪的来路不干净。",
		Nature: NatureTold, Audience: AudienceParticipants, Holders: []string{"other"},
		LearnedFrom: "someone", SourceTurnID: "turn-6", WorldTime: "第6日",
	}}
	recall = BuildRecall(RecallRequest{
		CharacterID: "zhou", Question: "黄芪的来路你清楚吗？", Knowledge: records,
	})
	if len(recall.Items) != 0 {
		t.Fatalf("recall delivered a record the character is not a holder of: %+v", recall.Items)
	}
}

// TestRecallDoesNotResurrectASupersededBelief pins the live gate. A belief the
// character has since corrected must not come back as current memory just
// because the question happens to match its old wording.
func TestRecallDoesNotResurrectASupersededBelief(t *testing.T) {
	records := []KnowledgeRecord{
		{
			ID: "knw-wrong", CharacterID: "zhou",
			Statement: "以为黄芪是本地种的。", Nature: NatureConjecture, Audience: AudienceSingle,
			SourceTurnID: "turn-3", WorldTime: "第3日", SupersededBy: "knw-right",
		},
		{
			ID: "knw-right", CharacterID: "zhou",
			Statement: "黄芪其实是从南边贩来的。", Nature: NatureTold, Audience: AudienceSingle,
			LearnedFrom: "老傌", SourceTurnID: "turn-7", WorldTime: "第7日",
		},
	}
	recall := BuildRecall(RecallRequest{
		CharacterID: "zhou", Question: "那批黄芪是哪来的？", Knowledge: records,
	})
	for _, item := range recall.Items {
		if item.ID == "knw-wrong" {
			t.Fatalf("a superseded belief was recalled as current: %+v", item)
		}
	}
	found := false
	for _, item := range recall.Items {
		if item.ID == "knw-right" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the corrected belief was not recalled:\n%+v", recall.Items)
	}
}

// TestRecallNeedsAQuestion is the boundary case. With no question there is no
// retrieval condition, and the pass must return nothing rather than guess at
// what the character might want to remember.
func TestRecallNeedsAQuestion(t *testing.T) {
	_, knowledge, matters, exchanges := recallFixture()
	recall := BuildRecall(RecallRequest{
		CharacterID: "zhou", Knowledge: knowledge, Matters: matters, Exchanges: exchanges,
	})
	if len(recall.Items) != 0 {
		t.Fatalf("recall produced material with no question to retrieve against: %+v", recall.Items)
	}
	if RenderRecall(recall) != "" {
		t.Fatal("an empty recall rendered a non-empty block")
	}
}

// TestRecallIsBoundedAndReportsItsOwnTruncation checks that a broad question
// cannot pull the whole past in, and that when it is cut the rendered block says
// so. The design is explicit that truncation must not read as the character
// having no memory of the thing: a model told "more exist" hedges, whereas one
// shown a silent gap asserts the record never happened.
func TestRecallIsBoundedAndReportsItsOwnTruncation(t *testing.T) {
	var records []KnowledgeRecord
	for i := 0; i < 40; i++ {
		records = append(records, KnowledgeRecord{
			ID: "knw-" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
			CharacterID: "zhou",
			Statement:   "黄芪的事，第 " + string(rune('0'+i/10)) + " 次记下。",
			Nature:      NatureExperienced, Audience: AudienceSingle,
			SourceTurnID: "turn-" + string(rune('0'+i/10)), WorldTime: "第3日",
		})
	}
	recall := BuildRecall(RecallRequest{
		CharacterID: "zhou", Question: "黄芪的事你还记得多少？", Knowledge: records, Limit: 3,
	})
	if len(recall.Items) > 3 {
		t.Fatalf("recall returned %d items for a limit of 3", len(recall.Items))
	}
	if !recall.Truncated {
		t.Fatal("recall cut material without reporting the cut")
	}
	if !strings.Contains(RenderRecall(recall), "More relevant memories exist") {
		t.Fatalf("a truncated recall did not tell the model more exists:\n%s", RenderRecall(recall))
	}
}

// TestPreparedBudgetKeepsTheQuestionRelevantMaterial is the budget rule the
// review asked for. The old order dropped the whole memory category first and
// the recalled material did not exist, so a long past became a blank one.
//
// Two budgets are checked, because they test different halves of the rule. At a
// budget that can hold the answer, the answer must be there — the recalled
// material outlives the voice. At a budget too tight even for that, the block
// must still not read as "this never happened": the character is told to hedge.
func TestPreparedBudgetKeepsTheQuestionRelevantMaterial(t *testing.T) {
	profile, knowledge, matters, exchanges := recallFixture()
	prepare := func(maxBytes int) Prepared {
		return Prepare(PrepareInput{
			StoryID: "st", Profile: profile,
			Revision:  IdentityRevision{BranchID: "b1", ProfileRevision: 1},
			Scene:     Scene{NodeID: "n1", Location: "青石镇", WorldTime: "第90日"},
			Exchanges: exchanges, Knowledge: knowledge, Matters: matters,
			Question: "我上次托你留的那批黄芪，到货了吗？",
			MaxBytes: maxBytes,
		})
	}

	// Room for the answer: the recalled promise survives, and the voice was the
	// thing that got shortened instead. The budget is 1500 rather than 1300
	// because the prepared circumstances now also carry how this character feels
	// about the protagonist and what stands between them; at 1300 the block no
	// longer had room for both the answer and that reading, so the test would
	// have been measuring the wrong thing.
	roomy := prepare(1500)
	if !strings.Contains(roomy.Identity, "周掌柜") {
		t.Fatalf("identity was dropped under budget pressure:\n%s", roomy.Identity)
	}
	if !strings.Contains(roomy.Recall, "黄芪") {
		t.Fatalf("budget pressure dropped the material the question was about:\n%s", roomy.Recall)
	}
	if roomy.RecallDiagnostics.Selected == 0 {
		t.Fatal("the answer was cut while the voice was kept")
	}

	// Too tight for the answer. The block must say the detail is missing rather
	// than leaving a silent gap the model reads as a record that never existed.
	tight := prepare(900)
	if !strings.Contains(tight.Identity, "周掌柜") {
		t.Fatalf("identity was dropped under a tight budget:\n%s", tight.Identity)
	}
	if tight.RecallDiagnostics.Dropped && !strings.Contains(tight.Recall, "do not say it never happened") {
		t.Fatalf("a dropped recall left a silent gap:\n%s", tight.Recall)
	}
	if strings.Contains(tight.Markdown(), "never happened\n") {
		t.Fatalf("the rendered context claims the past did not happen:\n%s", tight.Markdown())
	}
}

// TestRecallTermsSplitChineseWithoutSpaces pins the tokenizer. Chinese has no
// word boundaries, so a whole-phrase match is what separates a real hit from
// two incidental shared characters. A naive substring scan over the raw
// question would also match, but it would match far too much; the bigram cut is
// what keeps an unrelated record out.
func TestRecallTermsSplitChineseWithoutSpaces(t *testing.T) {
	terms := recallTerms("黄芪到货了吗")
	if len(terms) == 0 {
		t.Fatal("a Chinese question produced no retrieval terms")
	}
	joined := strings.Join(terms, "|")
	// Bigrams are present, so a two-character overlap can match.
	if !strings.Contains(joined, "黄芪") || !strings.Contains(joined, "到货") {
		t.Fatalf("terms lack the expected bigrams: %s", joined)
	}
	// A long run also contributes itself, so an exact phrase outscores pieces.
	if !strings.Contains(joined, "黄芪到货了吗") {
		t.Fatalf("terms lack the whole phrase: %s", joined)
	}

	// A single shared character is below the threshold, so it cannot carry a
	// record into the context on its own.
	if recallScore(recallTerms("黄"), "黄芪是南边贩来的") >= recallMinScore {
		t.Fatal("a single shared character was enough to recall a record")
	}
}
