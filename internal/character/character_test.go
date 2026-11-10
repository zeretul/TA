package character

import (
	"strings"
	"testing"
)

func testProjectionContext() ProjectionContext {
	counter := 0
	return ProjectionContext{
		StoryID:         "story-1",
		BranchID:        "branch-1",
		TurnID:          "turn-7",
		WorldTime:       "第3日 辰时",
		CommitID:        "commit-7",
		KnownCharacters: []string{"guardsman", "steward"},
		Scene: Scene{
			NodeID: "north-gate", Title: "北门盘查", Location: "北门",
			WorldTime: "第3日 辰时", TurnID: "turn-7",
			Present: []string{"guardsman", "steward"},
		},
		NextID: func(prefix string) string {
			counter++
			return prefix + "-" + string(rune('a'+counter-1))
		},
	}
}

// TestPrivateExchangeIsInvisibleToBystanders is the audience rule at its most
// basic. A conversation held in the window is private because of where it
// happened; a character standing in the same city must not be able to recall
// it. Without this, the window becomes a second, quieter way to make private
// speech public.
func TestPrivateExchangeIsInvisibleToBystanders(t *testing.T) {
	ctx := testProjectionContext()
	projected, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: ChannelPrivate, Text: "别声张，北门今夜不查。"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	private := projected.Exchanges[0]
	if len(private.Audience) != 1 || private.Audience[0] != "guardsman" {
		t.Fatalf("private exchange audience = %v, want only the speaker", private.Audience)
	}
	if exchangeReaches(private, "steward") {
		t.Fatal("a private exchange reached a bystander")
	}
	if !exchangeReaches(private, "guardsman") {
		t.Fatal("the speaker cannot recall their own words")
	}
}

// TestSceneExchangeReachesEveryonePresent is the other half: speech in the main
// story is witnessed by the scene, and the audience is resolved by the backend
// from the scene rather than chosen by the model.
func TestSceneExchangeReachesEveryonePresent(t *testing.T) {
	ctx := testProjectionContext()
	projected, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: ChannelScene, Text: "通关文牒。"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	scene := projected.Exchanges[0]
	if !ContainsID(scene.AudienceMembers(), "steward") {
		t.Fatalf("scene exchange audience = %v, want the whole scene", scene.Audience)
	}
	if !exchangeReaches(scene, "steward") {
		t.Fatal("a character present in the scene cannot recall what was said")
	}
}

// TestUnknownCharacterIsRejected keeps identity honest: a model cannot conjure
// a character into existence by speaking as them, because a character nobody
// has met cannot have a profile, a voice, or a memory.
func TestUnknownCharacterIsRejected(t *testing.T) {
	ctx := testProjectionContext()
	if _, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{CharacterID: "stranger", Speaker: SpeakerCharacter, Channel: ChannelScene, Text: "…"},
		},
	}); err == nil {
		t.Fatal("an unregistered character was allowed to speak")
	}
	if _, err := Project(ctx, TurnCharacterFacts{
		Knowledge: []KnowledgeDraft{{CharacterID: "stranger", Statement: "x", Nature: NatureTold, Audience: AudienceSingle, LearnedFrom: "guardsman"}},
	}); err == nil {
		t.Fatal("knowledge was recorded for an unregistered character")
	}
}

// TestSecretIsNotPublicKnowledge covers the rule that a character may hold a
// secret without it becoming shared. Being written down is not the same as
// being known, and an independent window does not make anything confidential.
func TestSecretIsNotPublicKnowledge(t *testing.T) {
	ctx := testProjectionContext()
	projected, err := Project(ctx, TurnCharacterFacts{
		Knowledge: []KnowledgeDraft{{
			CharacterID: "guardsman", Statement: "北门的换防暗号是长庚。",
			Nature: NatureSecret, Audience: AudienceWorld,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	record := projected.Knowledge[0]
	if record.Audience != AudienceParticipants {
		t.Fatalf("a secret was recorded with audience %q", record.Audience)
	}
	if HolderCanKnow(record, "steward") {
		t.Fatal("a bystander could retrieve another character's secret")
	}
	if !HolderCanKnow(record, "guardsman") {
		t.Fatal("the holder cannot retrieve their own secret")
	}
}

// TestHearsayCannotBeAssertedAsExperience covers the distinction the design
// requires between what a character did, was told, heard, guessed, promised and
// keeps secret. Collapsing them lets a rumour harden into fact.
func TestHearsayCannotBeAssertedAsExperience(t *testing.T) {
	ctx := testProjectionContext()
	projected, err := Project(ctx, TurnCharacterFacts{
		Knowledge: []KnowledgeDraft{
			{CharacterID: "steward", Statement: "听说北门昨夜有火光。", Nature: NatureHearsay, Audience: AudienceSingle},
			{CharacterID: "steward", Statement: "我亲眼看见守夜人换了岗。", Nature: NatureExperienced, Audience: AudienceSingle},
			{CharacterID: "steward", Statement: "我猜他们在等什么大人物。", Nature: NatureConjecture, Audience: AudienceSingle},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := projected.Knowledge[0].Nature.Certainty(); got != "uncertain" {
		t.Fatalf("hearsay certainty = %q, want uncertain", got)
	}
	if got := projected.Knowledge[1].Nature.Certainty(); got != "certain" {
		t.Fatalf("experienced certainty = %q, want certain", got)
	}
	if got := projected.Knowledge[2].Nature.Certainty(); got != "uncertain" {
		t.Fatalf("conjecture certainty = %q, want uncertain", got)
	}
	view := BuildKnowledgeView("steward", projected.Knowledge, 0)
	if len(view.Records) != 3 {
		t.Fatalf("view holds %d records, want 3", len(view.Records))
	}
	// The rendered material must mark the guess as a guess, so the model cannot
	// present it as established.
	markdown := Prepare(PrepareInput{
		StoryID: "story-1", Profile: Profile{ID: "steward", StoryID: "story-1", Name: "管事", Revision: 1},
		Knowledge: projected.Knowledge,
	}).Memory
	if !strings.Contains(markdown, "conjecture") {
		t.Fatalf("prepared memory does not mark conjecture:\n%s", markdown)
	}
}

// TestCounterEvidenceRetiresTheOldBelief is the cognitive-evolution case: a
// later record that supersedes or retracts an earlier one must remove the old
// belief from what the character is told, without deleting the history.
func TestCounterEvidenceRetiresTheOldBelief(t *testing.T) {
	ctx := testProjectionContext()
	first, err := Project(ctx, TurnCharacterFacts{
		Knowledge: []KnowledgeDraft{{CharacterID: "steward", Statement: "库房丢了三十石米。", Nature: NatureTold, Audience: AudienceSingle, LearnedFrom: "guardsman"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := State{Version: StateVersion, Knowledge: first.Knowledge}

	ctx.CommitID = "commit-9"
	ctx.WorldTime = "第5日 未时"
	second, err := Project(ctx, TurnCharacterFacts{
		Knowledge: []KnowledgeDraft{{
			CharacterID: "steward", Statement: "库房其实一粒未少，是账目抄错了。",
			Nature: NatureExperienced, Audience: AudienceSingle, Retracts: "knw-a",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	state.Knowledge = ApplyKnowledge(state.Knowledge, second.Knowledge)

	if len(state.Knowledge) != 2 {
		t.Fatalf("history was lost: %d records", len(state.Knowledge))
	}
	if state.Knowledge[0].RetractedBy != "knw-b" {
		t.Fatalf("the retracted record was not linked: %+v", state.Knowledge[0])
	}
	view := BuildKnowledgeView("steward", state.Knowledge, 0)
	if len(view.Records) != 1 || view.Records[0].ID != "knw-b" {
		t.Fatalf("the character still holds a retracted belief: %+v", view.Records)
	}
}

// TestPermissionIsNotTheActItPermits is the distinction the appended task calls
// out by name: 通行许可 and 实际入城 are separate facts, and a chat must not
// announce an undetermined result as success.
func TestPermissionIsNotTheActItPermits(t *testing.T) {
	ctx := testProjectionContext()
	projected, err := Project(ctx, TurnCharacterFacts{
		Matters: []MatterDraft{{
			CharacterID: "guardsman", Kind: MatterPermission, Status: MatterOpen,
			Statement: "守夜人允许主角今夜从北门入城。", Counterparty: "player",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	matter := projected.Matters[0]
	if matter.Kind != MatterPermission {
		t.Fatalf("kind = %q", matter.Kind)
	}
	guidance := matterGuidance(matter)
	if !strings.Contains(guidance, "does not mean the permitted act has happened") {
		t.Fatalf("permission guidance does not separate the right from the act:\n%s", guidance)
	}

	// Entering the city is a separate fact, and it settles the permission
	// rather than replacing it: 通行许可 and 实际入城 are recorded apart.
	ctx.CommitID = "commit-8"
	ctx.WorldTime = "第3日 亥时"
	entry, err := Project(ctx, TurnCharacterFacts{
		Matters: []MatterDraft{{
			CharacterID: "guardsman", Kind: MatterAction, Status: MatterSettled,
			Statement: "主角已于亥时自北门入城。", Counterparty: "player", Resolves: "mat-a",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	merged := ApplyMatters(projected.Matters, entry.Matters)
	if len(merged) != 2 {
		t.Fatalf("permission and act were collapsed: %+v", merged)
	}
	if merged[0].Status != MatterSettled || merged[0].SettledBy != "mat-b" {
		t.Fatalf("the permission was not settled by the act: %+v", merged[0])
	}
	if merged[1].Kind != MatterAction || merged[1].Status != MatterSettled {
		t.Fatalf("the act was not recorded as its own settled fact: %+v", merged[1])
	}
	if open := OpenMatters("guardsman", merged); len(open) != 0 {
		t.Fatalf("open matters = %+v, want none once the act happened", open)
	}
}

// TestRepeatedRequestDoesNotSettleTwice is the retry and recovery guarantee: a
// repeated request, a replayed turn and a recovery must not produce a second
// obligation or a second settlement.
func TestRepeatedRequestDoesNotSettleTwice(t *testing.T) {
	ctx := testProjectionContext()
	first, err := Project(ctx, TurnCharacterFacts{
		Matters: []MatterDraft{{CharacterID: "steward", Kind: MatterPromise, Status: MatterOpen, Statement: "管事答应替主角打听城门值守。"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// The identical promise stated again in a later turn.
	ctx.CommitID = "commit-8"
	repeat, err := Project(ctx, TurnCharacterFacts{
		Matters: []MatterDraft{{CharacterID: "steward", Kind: MatterPromise, Status: MatterOpen, Statement: "管事答应替主角打听城门值守。"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	merged := ApplyMatters(first.Matters, repeat.Matters)
	if len(merged) != 1 {
		t.Fatalf("a repeated request created %d matters, want 1", len(merged))
	}

	// Now settle it, then replay the settlement. The discharge is its own
	// recorded fact; the promise it discharges is settled in place exactly once.
	ctx.CommitID = "commit-9"
	settle, err := Project(ctx, TurnCharacterFacts{
		Matters: []MatterDraft{{
			CharacterID: "steward", Kind: MatterAction, Status: MatterSettled,
			Statement: "管事回报了城门值守的换班时辰。", Resolves: "mat-a",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	merged = ApplyMatters(merged, settle.Matters)
	promise, ok := MatterByID(merged, "mat-a")
	if !ok {
		t.Fatal("the original promise disappeared")
	}
	if promise.Status != MatterSettled || promise.SettledBy != settle.Matters[0].ID {
		t.Fatalf("the promise was not settled by the discharge: %+v", promise)
	}
	if promise.Statement != "管事答应替主角打听城门值守。" {
		t.Fatalf("settling rewrote the original statement: %q", promise.Statement)
	}
	// Replaying the same settlement must be a no-op, including its link.
	replay := settle.Matters[0]
	replay.ID = "mat-d"
	mergedAgain := ApplyMatters(merged, []Matter{replay})
	if len(mergedAgain) != len(merged) {
		t.Fatalf("a replayed settlement changed the record: %d -> %d", len(merged), len(mergedAgain))
	}
	replayedPromise, _ := MatterByID(mergedAgain, "mat-a")
	if replayedPromise.SettledBy != settle.Matters[0].ID {
		t.Fatalf("a replay re-settled an already settled matter: %+v", replayedPromise)
	}
	if len(OpenMatters("steward", mergedAgain)) != 0 {
		t.Fatalf("a settled matter is still open: %+v", OpenMatters("steward", mergedAgain))
	}
}

// TestProjectionRequiresProvenance stops a record from being created out of
// nothing. A fact with no source cannot be re-checked, retracted, or told apart
// from invention.
func TestProjectionRequiresProvenance(t *testing.T) {
	record := KnowledgeRecord{
		ID: "knw-x", CharacterID: "steward", Statement: "某事", Nature: NatureTold,
		Audience: AudienceSingle, WorldTime: "第1日", LearnedFrom: "guardsman",
	}
	if err := record.Validate(); err == nil {
		t.Fatal("a record with no source event, exchange or turn was accepted")
	}
	record.SourceTurnID = "turn-1"
	if err := record.Validate(); err != nil {
		t.Fatalf("a sourced record was rejected: %v", err)
	}
	record.LearnedFrom = ""
	if err := record.Validate(); err == nil {
		t.Fatal("a told record with no teller was accepted")
	}
}

// TestProjectionRequiresWorldTime keeps the record honest about when it was
// learned, which is what lets an old message still report its original send
// time after the player has been away.
func TestProjectionRequiresWorldTime(t *testing.T) {
	ctx := testProjectionContext()
	ctx.WorldTime = ""
	if _, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{{CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: ChannelScene, Text: "…"}},
	}); err == nil {
		t.Fatal("an exchange with no world time was accepted")
	}
}

// TestMessageRequiresCausalBasis is the anti-schedule rule: messages are caused
// by events, appointments and circumstances, not emitted every round for every
// character.
func TestMessageRequiresCausalBasis(t *testing.T) {
	message := Message{
		ID: "msg-1", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman",
		Text: "今夜北门有变。", WorldTime: "第3日 酉时",
	}
	if err := message.Validate(); err == nil {
		t.Fatal("a message with no stated basis was accepted")
	}
	message.Basis = "北门守夜人换防，主角与他有未了的约定"
	if err := message.Validate(); err != nil {
		t.Fatalf("a caused message was rejected: %v", err)
	}
}

// TestMessageTimeUsesWorldTimeNotWallClock proves the player's real waiting and
// program restarts do not age anything: expiry is decided by world time alone.
func TestMessageTimeUsesWorldTimeNotWallClock(t *testing.T) {
	message := Message{
		ID: "msg-1", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman",
		Text: "三日内给我答复。", Basis: "临别约定", WorldTime: "第3日 酉时", Deadline: "第6日",
	}
	if err := message.Validate(); err != nil {
		t.Fatal(err)
	}
	if message.Lapsed("第3日 酉时") {
		t.Fatal("a message lapsed at its own send time")
	}
	if message.Lapsed("第5日") {
		t.Fatal("a message lapsed before its deadline")
	}
	if !message.Lapsed("第7日") {
		t.Fatal("a message did not lapse after its deadline")
	}
	if message.Lapsed("") {
		t.Fatal("an unknown current world time expired a message")
	}
}

func TestMessagesAreDeduplicatedAndKeepOriginalTime(t *testing.T) {
	state := State{Version: StateVersion}
	message := Message{
		ID: "msg-1", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman",
		Text: "北门见。", Basis: "约定", WorldTime: "第3日 酉时",
	}
	if err := state.AddMessages([]Message{message}); err != nil {
		t.Fatal(err)
	}
	if err := state.AddMessages([]Message{message}); err != nil {
		t.Fatal(err)
	}
	if len(state.Messages) != 1 {
		t.Fatalf("a retried delivery duplicated the message: %d", len(state.Messages))
	}
	if err := state.MarkMessage("msg-1", MessageRead, "第4日"); err != nil {
		t.Fatal(err)
	}
	if state.Messages[0].WorldTime != "第3日 酉时" {
		t.Fatalf("reading rewrote the original send time: %q", state.Messages[0].WorldTime)
	}
	if state.Messages[0].ReadWorldTime != "第4日" {
		t.Fatalf("read world time = %q", state.Messages[0].ReadWorldTime)
	}
	pending := PendingMessages(state.Messages, "第4日")
	if len(pending) != 1 {
		t.Fatalf("a read message dropped out of the pending list: %d", len(pending))
	}
	if err := state.MarkMessage("msg-1", MessageIgnored, "第4日"); err != nil {
		t.Fatal(err)
	}
	if pending := PendingMessages(state.Messages, "第4日"); len(pending) != 0 {
		t.Fatalf("an ignored message still shows as pending: %+v", pending)
	}
}

// TestASameDayDeadlineExpiresAtALaterShichen is the fixed Windows probe's
// finding as a test. The probe ran a deadline of 第3日 酉时 against a current
// time of 第3日 戌时 and Message.Lapsed still answered false, so the arrangement
// never came due: 酉 and 戌 are the same day, and the comparison stopped at the
// day number.
//
// 时辰 are an exact ordinal — 子丑寅卯辰巳午未申酉戌亥 — so within one day the
// label states the order without a calendar. That is the one finer key this
// function can honestly use, and without it every same-day deadline is
// permanent.
func TestASameDayDeadlineExpiresAtALaterShichen(t *testing.T) {
	message := Message{
		ID: "msg-1", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman",
		Text: "酉时之前给我回话。", Basis: "临别约定", WorldTime: "第3日 辰时", Deadline: "第3日 酉时",
	}
	if err := message.Validate(); err != nil {
		t.Fatal(err)
	}
	if message.Lapsed("第3日 辰时") {
		t.Fatal("a message lapsed before its own deadline, in an earlier 时辰 of the same day")
	}
	if message.Lapsed("第3日 酉时") {
		t.Fatal("a message lapsed exactly at its deadline; the deadline itself is still in force")
	}
	if !message.Lapsed("第3日 戌时") {
		t.Fatal("the probe's own case: 戌时 is after 酉时 on the same day and the deadline has passed")
	}
	if !message.Lapsed("第4日 子时") {
		t.Fatal("a later day must still expire it")
	}
}

// TestADayWithNoShichenStaysUnordered is the conservative half of the same fix.
//
// "第3日" as a deadline most likely means the day as a whole. Reading it as
// midnight would expire a thread at the first hour of the day it was due on,
// which is earlier than the prose promised. A bare day states no hour, so the
// pair is unordered and the deadline keeps running until a later day settles it.
func TestADayWithNoShichenStaysUnordered(t *testing.T) {
	if got := compareWorldTime("第3日 戌时", "第3日"); got != 0 {
		t.Fatalf("a bare day compared as %d against an hour of the same day, want 0 (unordered)", got)
	}
	if got := compareWorldTime("第3日", "第3日 戌时"); got != 0 {
		t.Fatalf("the reversed pair compared as %d, want 0", got)
	}
	// A later day still orders it, because the day number settles that much.
	if got := compareWorldTime("第4日 子时", "第3日"); got != 1 {
		t.Fatalf("a later day compared as %d against an earlier bare day, want 1", got)
	}
}

// TestAShichenCharacterInANameIsNotATime keeps the ordinal from being read out
// of vocabulary. Only "X时" is a time; a lone 酉 or 戌 inside a place or a
// person's name is not, and treating it as one would order two labels by an
// accident of what they happen to be about.
func TestAShichenCharacterInANameIsNotATime(t *testing.T) {
	if _, ok := shichenOrdinal("第3日 酉时"); !ok {
		t.Fatal("酉时 was not read as a 时辰")
	}
	if _, ok := shichenOrdinal("第3日 酉"); ok {
		t.Fatal("a bare 酉 was read as a 时辰")
	}
	if _, ok := shichenOrdinal("第3日 酉家巷"); ok {
		t.Fatal("酉 in a place name was read as a 时辰")
	}
}

// TestPrepareCarriesAllSixCategories is the design's preparation contract. All
// six categories are separate, so the ledger can record what was actually
// supplied rather than one opaque blob.
func TestPrepareCarriesAllSixCategories(t *testing.T) {
	profile := Profile{
		ID: "guardsman", StoryID: "story-1", Name: "陈九", KnownAs: "北门守卒",
		Role: "北门守卒", Description: "守北门十年。",
		Personality: []string{"谨慎", "口硬心软"}, SpeechStyle: "短句，官腔",
		Motives: []string{"保住差事"}, Attitude: "戒备但讲理",
		Bounds: []string{"不当值时不谈换防"}, Revision: 3,
		FirstSeenWorldTime: "第1日",
	}
	scene := Scene{NodeID: "north-gate", Location: "北门", WorldTime: "第3日 辰时", Present: []string{"guardsman", "steward"}}
	prepared := Prepare(PrepareInput{
		StoryID: "story-1",
		Revision: IdentityRevision{
			PackID: "pack-xianxia", WorldID: "world-1", BranchID: "branch-1",
			ProfileRevision: 3, FormalCommitID: "commit-7", LoreRevision: "lore-9",
		},
		Profile: profile,
		Scene:   scene,
		Exchanges: []Exchange{
			{ID: "xch-1", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman", Speaker: SpeakerPlayer, Channel: ChannelPrivate, Text: "今夜能过吗？", WorldTime: "第3日 辰时"},
			{ID: "xch-2", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: ChannelPrivate, Text: "看你带没带文牒。", WorldTime: "第3日 辰时"},
		},
		Knowledge: []KnowledgeRecord{{
			ID: "knw-1", CharacterID: "guardsman", Statement: "主角自称要进城寻人。",
			Nature: NatureTold, Audience: AudienceSingle, LearnedFrom: "protagonist",
			WorldTime: "第3日 辰时", SourceTurnID: "turn-7", FormalCommitID: "commit-7",
		}},
		Matters: []Matter{{
			ID: "mat-1", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman",
			Kind: MatterPromise, Status: MatterOpen, Statement: "答应替主角看一眼北门的名册。",
			WorldTime: "第3日 辰时", ExchangeID: "xch-2",
		}},
		Lore: []string{"北门入夜落锁，落锁后只认文书。"},
	})
	for name, body := range map[string]string{
		"identity": prepared.Identity, "voice": prepared.Voice, "circumstances": prepared.Circumstances,
		"interactions": prepared.Interactions, "recent_dialogue": prepared.RecentDialogue, "memory": prepared.Memory,
	} {
		if strings.TrimSpace(body) == "" {
			t.Fatalf("prepared category %s is empty", name)
		}
	}
	if !strings.Contains(prepared.Identity, "北门守卒") {
		t.Fatalf("identity does not use the name the player knows:\n%s", prepared.Identity)
	}
	if !strings.Contains(prepared.RecentDialogue, "今夜能过吗") {
		t.Fatalf("recent dialogue is missing the conversation:\n%s", prepared.RecentDialogue)
	}
	if !strings.Contains(prepared.Interactions, "答应替主角看一眼北门的名册") {
		t.Fatalf("unfinished business is missing:\n%s", prepared.Interactions)
	}
	if !strings.Contains(prepared.Memory, "北门入夜落锁") {
		t.Fatalf("common knowledge is missing:\n%s", prepared.Memory)
	}
	markdown := prepared.Markdown()
	if !strings.Contains(markdown, "### Identity and self-knowledge") {
		t.Fatalf("prepared markdown has no identity section:\n%s", markdown)
	}
}

// TestPrepareStatesGenderWithoutTurningItIntoATemperament covers the design's
// 不以性别套固定性格. The gender reaches the voice block because a person the
// player has met must answer the same way every conversation, but the line is
// followed by the warning that stops the model from reading it as a personality.
func TestPrepareStatesGenderWithoutTurningItIntoATemperament(t *testing.T) {
	profile := Profile{
		ID: "shopkeeper", StoryID: "story-1", Name: "周氏", Role: "药铺掌柜",
		Gender: GenderFemale, SpeechStyle: "说话慢，先问用途再报价",
		Personality: []string{"仔细"}, Revision: 1,
	}
	prepared := Prepare(PrepareInput{
		StoryID: "story-1",
		Revision: IdentityRevision{
			PackID: "p", WorldID: "w", BranchID: "b", ProfileRevision: 1,
			FormalCommitID: "c", LoreRevision: "l",
		},
		Profile: profile,
	})
	if !strings.Contains(prepared.Voice, "female") {
		t.Fatalf("the voice block does not state the settled gender:\n%s", prepared.Voice)
	}
	if !strings.Contains(prepared.Voice, "not a personality") {
		t.Fatalf("the voice block states a gender with no warning against stereotyping:\n%s", prepared.Voice)
	}
	// The voice itself is still the profile's own words, not a derived one.
	if !strings.Contains(prepared.Voice, "先问用途再报价") {
		t.Fatalf("the profile's own speech style is missing:\n%s", prepared.Voice)
	}

	// An unsettled gender says nothing at all rather than a blank line.
	silent := Prepare(PrepareInput{
		StoryID: "story-1",
		Revision: IdentityRevision{
			PackID: "p", WorldID: "w", BranchID: "b", ProfileRevision: 1,
			FormalCommitID: "c", LoreRevision: "l",
		},
		Profile: Profile{ID: "x", StoryID: "story-1", Name: "某人", Revision: 1},
	})
	if strings.Contains(silent.Voice, "Gender") {
		t.Fatalf("an unsettled gender still produced a gender line:\n%s", silent.Voice)
	}
}

// TestCacheKeySeparatesEveryScopingInput is what stops a prepared context from
// one world, branch, profile revision or commit position being served for
// another. A cache key that ignored any of these would leak a same-named
// character from a different world into this story.
func TestCacheKeySeparatesEveryScopingInput(t *testing.T) {
	base := IdentityRevision{PackID: "pack-a", WorldID: "world-1", BranchID: "branch-1", ProfileRevision: 1, FormalCommitID: "c1", LoreRevision: "l1"}
	baseKey := base.CacheKey("guardsman", "story-1")
	different := []IdentityRevision{
		{PackID: "pack-b", WorldID: "world-1", BranchID: "branch-1", ProfileRevision: 1, FormalCommitID: "c1", LoreRevision: "l1"},
		{PackID: "pack-a", WorldID: "world-2", BranchID: "branch-1", ProfileRevision: 1, FormalCommitID: "c1", LoreRevision: "l1"},
		{PackID: "pack-a", WorldID: "world-1", BranchID: "branch-2", ProfileRevision: 1, FormalCommitID: "c1", LoreRevision: "l1"},
		{PackID: "pack-a", WorldID: "world-1", BranchID: "branch-1", ProfileRevision: 2, FormalCommitID: "c1", LoreRevision: "l1"},
		{PackID: "pack-a", WorldID: "world-1", BranchID: "branch-1", ProfileRevision: 1, FormalCommitID: "c2", LoreRevision: "l1"},
		{PackID: "pack-a", WorldID: "world-1", BranchID: "branch-1", ProfileRevision: 1, FormalCommitID: "c1", LoreRevision: "l2"},
	}
	for index, revision := range different {
		if key := revision.CacheKey("guardsman", "story-1"); key == baseKey {
			t.Fatalf("cache key %d collided with the base key: %s", index, key)
		}
	}
	if other := base.CacheKey("steward", "story-1"); other == baseKey {
		t.Fatal("two characters share a cache key")
	}
	if other := base.CacheKey("guardsman", "story-2"); other == baseKey {
		t.Fatal("two stories share a cache key")
	}
}

// TestPrepareKeepsIdentityWhenTheBudgetBites proves the degradation order. A
// character with no memory is still themselves; a character with no identity is
// nobody, so identity is never the category that gets cut.
func TestPrepareKeepsIdentityWhenTheBudgetBites(t *testing.T) {
	profile := Profile{
		ID: "guardsman", StoryID: "story-1", Name: "陈九", Role: "北门守卒",
		Personality: []string{"谨慎"}, SpeechStyle: "短句", Revision: 1,
	}
	prepared := Prepare(PrepareInput{
		StoryID: "story-1", Profile: profile,
		Scene: Scene{NodeID: "gate", WorldTime: "第3日"},
		Lore:  []string{strings.Repeat("很长的世界常识。", 400)},
		Exchanges: []Exchange{
			{ID: "x", StoryID: "story-1", BranchID: "b", CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: ChannelPrivate, Text: strings.Repeat("话", 200), WorldTime: "第3日"},
		},
		MaxBytes: 1200,
	})
	if strings.TrimSpace(prepared.Identity) == "" {
		t.Fatal("identity was dropped to fit the budget")
	}
	if len(prepared.Truncated) == 0 {
		t.Fatal("a truncated preparation did not report what it cut")
	}
	if len([]byte(prepared.Markdown())) > 1200+512 {
		t.Fatalf("prepared context exceeded its budget: %d bytes", len([]byte(prepared.Markdown())))
	}
}

// TestStateListsEveryKnownCharacter is the "not two hardcoded characters" rule:
// the window lists from the story's own registry.
func TestStateListsEveryKnownCharacter(t *testing.T) {
	state := State{Version: StateVersion}
	for _, profile := range []Profile{
		{ID: "guardsman", StoryID: "s", Name: "陈九", Revision: 1},
		{ID: "steward", StoryID: "s", Name: "管事", Revision: 1},
		{ID: "herbalist", StoryID: "s", Name: "采药人", Revision: 1},
	} {
		if err := state.UpsertProfile(profile); err != nil {
			t.Fatal(err)
		}
	}
	ids := state.CharacterIDs()
	if len(ids) != 3 {
		t.Fatalf("character list = %v, want three characters", ids)
	}
	// Editing a profile moves its revision so a prepared context cannot be
	// reused for the new voice.
	updated := Profile{ID: "guardsman", StoryID: "s", Name: "陈九", SpeechStyle: "改了口音", Revision: 1}
	if err := state.UpsertProfile(updated); err != nil {
		t.Fatal(err)
	}
	stored, _ := state.Profile("guardsman")
	if stored.Revision != 2 {
		t.Fatalf("profile revision = %d, want 2 after an edit", stored.Revision)
	}
	if stored.SpeechStyle != "改了口音" {
		t.Fatalf("profile edit was lost: %+v", stored)
	}
}

// TestARevisitDoesNotEraseTheAuthorKey locks the identity of a person who comes
// back. A returning character is normally described without a source key, because
// the discovery page that supplied it no longer offers someone the story has
// already met. If that silence cleared the key, the next turn would find no actor
// for the preset person and register a second one — two actors sharing a name and
// neither sharing the other's memory.
func TestARevisitDoesNotEraseTheAuthorKey(t *testing.T) {
	state := State{Version: StateVersion}
	first := Profile{
		ID: "钱守信", StoryID: "s", Name: "钱守信", KnownAs: "钱掌柜",
		SourceKey: "npc_b_fs_hf_zhanggui", Revision: 1,
	}
	if err := state.UpsertProfile(first); err != nil {
		t.Fatal(err)
	}
	// The return visit: same actor, described again, saying nothing about who
	// they are to the author.
	if err := state.UpsertProfile(Profile{
		ID: "钱守信", StoryID: "s", Name: "钱守信", Attitude: "熟客了", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	stored, _ := state.Profile("钱守信")
	if stored.SourceKey != "npc_b_fs_hf_zhanggui" {
		t.Fatalf("the return visit erased the author key: got %q", stored.SourceKey)
	}
	// The turn's own words still land; only the silence was filled in.
	if stored.Attitude != "熟客了" {
		t.Fatalf("the return visit's update was lost: %+v", stored)
	}
	// And the actor is still findable by that key, which is the whole point.
	found, ok := state.ProfileBySourceKey("npc_b_fs_hf_zhanggui")
	if !ok || found.ID != "钱守信" {
		t.Fatalf("the preset person is no longer reachable by their author key: %+v ok=%v", found, ok)
	}
}

// TestAStatedGenderSurvivesEveryLaterTurn covers the design's 生成、正式保存、主故事／
// 聊天／群聊与重开保持一致. A person is introduced once with a gender the story
// settled, then described again several times without mentioning it. Every one
// of those turns must leave the answer alone: re-deciding it per turn would give
// the same person a different gender each time they speak, and clearing it would
// make the profile forget something the story already knows.
func TestAStatedGenderSurvivesEveryLaterTurn(t *testing.T) {
	state := State{Version: StateVersion}
	if err := state.UpsertProfile(Profile{
		ID: "阿秀", StoryID: "s", Name: "阿秀", Gender: GenderFemale, SpeechStyle: "话少，先听后答", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := state.UpsertProfile(Profile{
			ID: "阿秀", StoryID: "s", Name: "阿秀", Attitude: "渐渐熟络", Revision: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	stored, _ := state.Profile("阿秀")
	if stored.Gender != GenderFemale {
		t.Fatalf("a later turn that said nothing about gender erased it: got %q", stored.Gender)
	}
	if stored.SpeechStyle != "话少，先听后答" {
		t.Fatalf("the voice did not survive the later turns: %+v", stored)
	}
	// Re-opening the story reads the same record, so the window shows the same
	// answer. Normalize is what a load runs, and it must not drop the value.
	reloaded := stored.Normalize()
	if reloaded.Gender != GenderFemale {
		t.Fatalf("normalizing a stored profile lost its gender: got %q", reloaded.Gender)
	}
}

// TestAStatedGenderStillReplacesAnEarlierOne is the other half: preserving the
// value on silence must not make it immutable. A real correction in the story
// still lands, because only an empty draft is treated as silence.
func TestAStatedGenderStillReplacesAnEarlierOne(t *testing.T) {
	state := State{Version: StateVersion}
	if err := state.UpsertProfile(Profile{
		ID: "影子", StoryID: "s", Name: "影子", Gender: GenderMale, Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.UpsertProfile(Profile{
		ID: "影子", StoryID: "s", Name: "影子", Gender: GenderFemale, Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	stored, _ := state.Profile("影子")
	if stored.Gender != GenderFemale {
		t.Fatalf("a stated correction did not replace the earlier value: got %q", stored.Gender)
	}
}

// TestNormalizeGenderRefusesAnUnknownWord keeps free text out of the field. A
// value the program cannot display is not stored as-is: a profile that holds
// 男性 sometimes and male other times cannot be read consistently.
func TestNormalizeGenderRefusesAnUnknownWord(t *testing.T) {
	cases := map[string]string{
		"male": GenderMale, "男": GenderMale, "男性": GenderMale,
		"female": GenderFemale, "女": GenderFemale, "女性": GenderFemale,
		"": "", "不详": "", "unspecified": "",
	}
	for input, want := range cases {
		if got := NormalizeGender(input); got != want {
			t.Fatalf("NormalizeGender(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestAStatedKeyStillReplacesAnEarlierOne is the other half of the rule above:
// preserving a key on silence must not make a key that *was* stated immutable.
func TestAStatedKeyStillReplacesAnEarlierOne(t *testing.T) {
	state := State{Version: StateVersion}
	if err := state.UpsertProfile(Profile{
		ID: "陈三", StoryID: "s", Name: "陈三", SourceKey: "npc_wrong_guess", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.UpsertProfile(Profile{
		ID: "陈三", StoryID: "s", Name: "陈三", SourceKey: "npc_real_identity", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	stored, _ := state.Profile("陈三")
	if stored.SourceKey != "npc_real_identity" {
		t.Fatalf("a stated key did not replace the earlier one: got %q", stored.SourceKey)
	}
}

// TestStateApplyIsIdempotent is the retry guarantee at the aggregate level: the
// same projected turn applied twice must not double the record.
func TestStateApplyIsIdempotent(t *testing.T) {
	ctx := testProjectionContext()
	projected, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{{CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: ChannelPrivate, Text: "记住我的话。"}},
		Knowledge: []KnowledgeDraft{{CharacterID: "guardsman", Statement: "主角今夜要进城。", Nature: NatureTold, Audience: AudienceSingle, LearnedFrom: "protagonist"}},
		Matters:   []MatterDraft{{CharacterID: "guardsman", Kind: MatterAction, Status: MatterOpen, Statement: "主角承诺今夜入城。"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := State{Version: StateVersion}
	if err := state.Apply(projected, "commit-7"); err != nil {
		t.Fatal(err)
	}
	first := state.Normalize()
	if err := state.Apply(projected, "commit-7"); err != nil {
		t.Fatal(err)
	}
	second := state.Normalize()
	if len(second.Exchanges) != len(first.Exchanges) {
		t.Fatalf("replayed turn duplicated exchanges: %d -> %d", len(first.Exchanges), len(second.Exchanges))
	}
	if len(second.Knowledge) != len(first.Knowledge) {
		t.Fatalf("replayed turn duplicated knowledge: %d -> %d", len(first.Knowledge), len(second.Knowledge))
	}
	if len(second.Matters) != len(first.Matters) {
		t.Fatalf("replayed turn duplicated matters: %d -> %d", len(first.Matters), len(second.Matters))
	}
}

// TestMessageNeedsACauseAndKeepsItsOwnTime covers the proactive-message rules
// that make an incoming message honest: it must say why it was sent, and its
// send time is the turn's own world time rather than anything the model chose.
func TestMessageNeedsACauseAndKeepsItsOwnTime(t *testing.T) {
	ctx := testProjectionContext()
	// A message with no stated reason is refused rather than delivered, because a
	// character who writes for no reason is the fixed-schedule failure mode.
	if _, err := Project(ctx, TurnCharacterFacts{
		Messages: []MessageDraft{{CharacterID: "guardsman", Text: "在吗"}},
	}); err == nil {
		t.Fatal("a message with no stated basis was projected")
	}
	// A message from someone the story has never met is refused too.
	if _, err := Project(ctx, TurnCharacterFacts{
		Messages: []MessageDraft{{CharacterID: "stranger", Text: "北门见。", Basis: "路人搭话"}},
	}); err == nil {
		t.Fatal("an unregistered sender was allowed to write")
	}

	projected, err := Project(ctx, TurnCharacterFacts{
		Messages: []MessageDraft{{
			CharacterID: "guardsman", Text: "今夜北门有变，你若要走，趁换班。",
			Basis: "主角与他有未了的约定", Deadline: "第6日",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	message := projected.Messages[0]
	if message.WorldTime != ctx.WorldTime {
		t.Fatalf("message world time = %q, want the turn's own %q", message.WorldTime, ctx.WorldTime)
	}
	if message.BasisTurnID != ctx.TurnID || message.BasisEventID != ctx.CommitID {
		t.Fatalf("message is not pinned to the turn that caused it: %+v", message)
	}
	if message.Status != MessageUnread {
		t.Fatalf("a delivered message starts as %q, want unread", message.Status)
	}
}

// TestMessageIdentityIsStableAcrossReplay is the no-duplicate guarantee for
// proactive messages: the same character sending the same words for the same
// reason on the same branch is one message, so a replayed commit cannot deliver
// the same invitation twice.
func TestMessageIdentityIsStableAcrossReplay(t *testing.T) {
	ctx := testProjectionContext()
	facts := TurnCharacterFacts{
		Messages: []MessageDraft{{
			CharacterID: "guardsman", Text: "换班时来北门。", Basis: "白天的约定",
		}},
	}
	first, err := Project(ctx, facts)
	if err != nil {
		t.Fatal(err)
	}
	// A different commit position, the same content: still the same message.
	replayed := ctx
	replayed.CommitID = "commit-8"
	second, err := Project(replayed, facts)
	if err != nil {
		t.Fatal(err)
	}
	if first.Messages[0].ID != second.Messages[0].ID {
		t.Fatalf("a replayed message got a new identity: %q vs %q", first.Messages[0].ID, second.Messages[0].ID)
	}
	// A different reason is a different message: the character wrote twice.
	different := facts
	different.Messages = []MessageDraft{{
		CharacterID: "guardsman", Text: "换班时来北门。", Basis: "听闻城门要封",
	}}
	third, err := Project(ctx, different)
	if err != nil {
		t.Fatal(err)
	}
	if third.Messages[0].ID == first.Messages[0].ID {
		t.Fatal("two differently-caused messages collapsed into one")
	}

	state := State{Version: StateVersion}
	if err := state.Apply(first, "commit-7"); err != nil {
		t.Fatal(err)
	}
	if err := state.Apply(second, "commit-8"); err != nil {
		t.Fatal(err)
	}
	if len(state.Normalize().Messages) != 1 {
		t.Fatalf("replay delivered the message twice: %d", len(state.Normalize().Messages))
	}
}

// TestPendingMessagesExcludeDealtWithOnes is what keeps the stage's hint quiet.
// A message the player has handled or dismissed must not keep asking for
// attention, and an old message must still report its original send time.
func TestPendingMessagesExcludeDealtWithOnes(t *testing.T) {
	messages := []Message{
		{ID: "a", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman", Text: "一", Basis: "约", WorldTime: "第3日"},
		{ID: "b", StoryID: "story-1", BranchID: "branch-1", CharacterID: "steward", Text: "二", Basis: "约", WorldTime: "第5日"},
		{ID: "c", StoryID: "story-1", BranchID: "branch-1", CharacterID: "steward", Text: "三", Basis: "约", WorldTime: "第4日", Status: MessageHandled},
	}
	state := State{Version: StateVersion}
	if err := state.AddMessages(messages); err != nil {
		t.Fatal(err)
	}
	pending := PendingMessages(state.Messages, "第6日")
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2 (the handled one is done)", len(pending))
	}
	if pending[0].ID != "b" || pending[1].ID != "a" {
		t.Fatalf("pending is not newest first: %v", []string{pending[0].ID, pending[1].ID})
	}
	if pending[0].WorldTime != "第5日" {
		t.Fatalf("an unread message lost its send time: %q", pending[0].WorldTime)
	}
}

// TestARelativeDeadlineBindsToTheBranchClock is the time brief's own case.
//
// 三日内 is relative: it counts from the moment the arrangement was made, which
// is exactly what the branch clock records. Before this, the label was compared
// as text and "第3日 酉时" against "第3日 戌时" came out unordered, so the thread
// never came due. The brief is explicit that the fix is to give a relative
// deadline a structural instant rather than to keep reading a number out of its
// prose.
func TestARelativeDeadlineBindsToTheBranchClock(t *testing.T) {
	const now = int64(3 * 24 * 60 * 60)
	bound, ok := BindDeadlineSeconds("三日内", now)
	if !ok {
		t.Fatal("三日内 was not read as an interval")
	}
	if want := now + 3*secondsPerDay; bound != want {
		t.Fatalf("三日内 bound to %d, want %d", bound, want)
	}
	matter := Matter{
		ID: "mat-1", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman",
		Kind: MatterPromise, Status: MatterOpen, Statement: "把药送到北门。",
		Deadline: "三日内", DeadlineSeconds: bound,
	}
	if matter.LapsedAt("", now) {
		t.Fatal("a thread lapsed before its own interval had run")
	}
	if !matter.LapsedAt("", bound+1) {
		t.Fatal("a thread bound to the clock did not lapse once the clock passed its instant")
	}
}

// TestAnAbsoluteCalendarDeadlineStaysUnbound is the conservative half.
//
// 第3日 酉时 is a position in a calendar, not an interval. Turning it into
// seconds needs a registered calendar adapter, and the world has none — so the
// honest answer is to leave it unbound rather than invent an origin. An unbound
// record keeps the text comparison it always had, and that comparison now
// orders 时辰 within a day, so the probe's own case still closes.
func TestAnAbsoluteCalendarDeadlineStaysUnbound(t *testing.T) {
	if _, ok := BindDeadlineSeconds("第3日 酉时", 3*24*60*60); ok {
		t.Fatal("an absolute calendar position was bound to the clock, which would be a guess")
	}
	matter := Matter{
		ID: "mat-1", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman",
		Kind: MatterPromise, Status: MatterOpen, Statement: "酉时之前给我回话。",
		Deadline: "第3日 酉时",
	}
	if !matter.LapsedAt("第3日 戌时", 5*24*60*60) {
		t.Fatal("an unbound absolute deadline lost its text comparison")
	}
}

// TestAnAbsoluteDeadlineBindsOnceTheStoryFrozeACalendar is the other half of the
// test above: the label is unbound because nothing could place it, and once the
// story supplies a year length it can be placed exactly.
//
// 第3日 酉时 means the third day of the story, at the tenth two-hour slot. The
// origin day is 第1日, so the position is two whole days plus nine 时辰 — the
// story's own axis, not a calendar epoch.
func TestAnAbsoluteDeadlineBindsOnceTheStoryFrozeACalendar(t *testing.T) {
	units := CalendarUnits{SecondsPerDay: secondsPerDay, DaysPerMonth: 30, MonthsPerYear: 12}
	bound, ok := BindDeadlineSecondsOnCalendar("第3日 酉时", 0, units)
	if !ok {
		t.Fatal("an absolute deadline stayed unbound with a calendar in hand")
	}
	if want := 2*secondsPerDay + 9*secondsPerShichen; bound != want {
		t.Fatalf("第3日 酉时 bound to %d, want %d", bound, want)
	}
	// The first day at 子时 is the origin itself.
	if got, ok := BindDeadlineSecondsOnCalendar("第1日 子时", 0, units); !ok || got != 0 {
		t.Fatalf("第1日 子时 bound to %d (ok=%v), want 0", got, ok)
	}
	// A label that states a day with no 时辰 states no instant within it, so it
	// keeps the unbound behaviour rather than becoming midnight.
	if _, ok := BindDeadlineSecondsOnCalendar("第3日", 0, units); ok {
		t.Fatal("a day with no 时辰 was placed at an instant the label never stated")
	}
	// Without a calendar there is nothing to divide the axis by, and the label
	// stays exactly as unbound as it was before this existed.
	if _, ok := BindDeadlineSecondsOnCalendar("第3日 酉时", 0, CalendarUnits{}); ok {
		t.Fatal("an absolute deadline was placed with no calendar")
	}
	// Relative labels are untouched by the calendar path.
	now := int64(5 * 24 * 60 * 60)
	if got, ok := BindDeadlineSecondsOnCalendar("三日内", now, units); !ok || got != now+3*secondsPerDay {
		t.Fatalf("三日内 bound to %d (ok=%v), want %d", got, ok, now+3*secondsPerDay)
	}
}

// TestProjectStampsAnAbsoluteDeadlineWhenTheStoryFrozeACalendar is the wiring
// half: the projection context has to carry both the clock and the calendar, or
// an absolute label reaches the record unbound.
func TestProjectStampsAnAbsoluteDeadlineWhenTheStoryFrozeACalendar(t *testing.T) {
	ctx := testProjectionContext()
	ctx.Calendar = CalendarUnits{SecondsPerDay: secondsPerDay, DaysPerMonth: 30, MonthsPerYear: 12}
	facts := TurnCharacterFacts{Matters: []MatterDraft{{
		CharacterID: "guardsman", Kind: MatterPromise, Status: MatterOpen,
		Statement: "酉时之前给我回话。", Deadline: "第3日 酉时",
	}}}
	projected, err := Project(ctx, facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Matters) != 1 {
		t.Fatalf("projected %d matters, want 1", len(projected.Matters))
	}
	if want := 2*secondsPerDay + 9*secondsPerShichen; projected.Matters[0].DeadlineSeconds != want {
		t.Fatalf("DeadlineSeconds = %d, want %d", projected.Matters[0].DeadlineSeconds, want)
	}
}

// TestAnUnreadableDeadlineNeverExpires is what keeps a guess out of the clock.
//
// A label with no interval in it — a place name, a person's name, a sentence
// fragment — binds to nothing, and LapsedAt with no reading must not decide it.
// Reading a numeral out of 一并 or 半路 would expire a thread on the strength of
// a character that happened to be a digit.
func TestAnUnreadableDeadlineNeverExpires(t *testing.T) {
	for _, label := range []string{"", "一并办了", "半路碰头", "改日再说", "今日日落前"} {
		if _, ok := BindDeadlineSeconds(label, 3*24*60*60); ok {
			t.Fatalf("%q was read as an interval", label)
		}
	}
	// A bound record with no clock reading stays live rather than being guessed.
	matter := Matter{
		ID: "mat-1", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman",
		Kind: MatterPromise, Status: MatterOpen, Statement: "把药送到北门。",
		Deadline: "三日内", DeadlineSeconds: 4 * 24 * 60 * 60,
	}
	if matter.LapsedAt("", 0) {
		t.Fatal("a bound thread expired with no clock reading to decide it")
	}
}

// TestARelativeDayWordBindsToTheNextDay covers the other shape the brief names:
// 明日 counts whole days from when it was written, not from a calendar date.
func TestARelativeDayWordBindsToTheNextDay(t *testing.T) {
	const now = int64(10 * 24 * 60 * 60)
	bound, ok := BindDeadlineSeconds("明日午时前", now)
	if !ok {
		t.Fatal("明日 was not read as a relative day")
	}
	if want := now + secondsPerDay; bound != want {
		t.Fatalf("明日 bound to %d, want %d", bound, want)
	}
}

// TestAShichenIntervalBindsToItsHours keeps the smaller units working, including
// the 个 counter between the numeral and 时辰.
func TestAShichenIntervalBindsToItsHours(t *testing.T) {
	const now = int64(60 * 60)
	for _, label := range []string{"三个时辰", "三个时辰内", "半个时辰"} {
		if _, ok := BindDeadlineSeconds(label, now); !ok {
			t.Fatalf("%q was not read as an interval", label)
		}
	}
	if bound, _ := BindDeadlineSeconds("三个时辰", now); bound != now+3*secondsPerShichen {
		t.Fatalf("三个时辰 bound to %d, want %d", bound, now+3*secondsPerShichen)
	}
	if bound, _ := BindDeadlineSeconds("半个时辰", now); bound != now+secondsPerShichen/2 {
		t.Fatalf("半个时辰 bound to %d, want %d", bound, now+secondsPerShichen/2)
	}
}

// TestProjectStampsARelativeDeadlineFromTheCommitClock is the wiring half: a
// matter projected with a clock reading carries the absolute instant, and one
// projected without a reading carries none.
func TestProjectStampsARelativeDeadlineFromTheCommitClock(t *testing.T) {
	ctx := testProjectionContext()
	ctx.ClockSeconds = 3 * 24 * 60 * 60
	facts := TurnCharacterFacts{Matters: []MatterDraft{{
		CharacterID: "guardsman", Kind: MatterPromise, Status: MatterOpen,
		Statement: "把药送到北门。", Deadline: "三日内",
	}}}
	projected, err := Project(ctx, facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Matters) != 1 {
		t.Fatalf("matters = %d, want 1", len(projected.Matters))
	}
	if want := ctx.ClockSeconds + 3*secondsPerDay; projected.Matters[0].DeadlineSeconds != want {
		t.Fatalf("deadline_seconds = %d, want %d", projected.Matters[0].DeadlineSeconds, want)
	}

	// A zero reading is the branch's own origin, which is a real moment: a story
	// that has not yet spent formal time still writes letters, and 三日内 written
	// there runs from the origin. Only a negative reading — a caller with no clock
	// at all — leaves the label unbound.
	ctx.ClockSeconds = 0
	fromOrigin, err := Project(ctx, facts)
	if err != nil {
		t.Fatal(err)
	}
	if want := 3 * secondsPerDay; fromOrigin.Matters[0].DeadlineSeconds != want {
		t.Fatalf("a deadline written at the origin bound to %d, want %d", fromOrigin.Matters[0].DeadlineSeconds, want)
	}
	ctx.ClockSeconds = -1
	unbound, err := Project(ctx, facts)
	if err != nil {
		t.Fatal(err)
	}
	if unbound.Matters[0].DeadlineSeconds != 0 {
		t.Fatalf("a projection with no clock stamped %d", unbound.Matters[0].DeadlineSeconds)
	}
}

// AudienceParticipants is the design's answer for an event several people were
// in: 最小实现只记录本次交流的合理听见者 (§13.3). The record is written once,
// under the character whose point of view it states, and every listed holder may
// read it. Without that join the audience value is decoration: a second witness
// remembers nothing, and the narrator gives them a scene they were standing in
// and cannot describe.
func TestParticipantsAudienceReachesEveryHolder(t *testing.T) {
	ctx := testProjectionContext()
	projected, err := Project(ctx, TurnCharacterFacts{
		Knowledge: []KnowledgeDraft{{
			CharacterID: "guardsman", Statement: "今夜的口令是长庚。",
			Nature: NatureExperienced, Audience: AudienceParticipants,
			Channel: ChannelScene,
			Holders: []string{"guardsman", "steward"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := State{Version: StateVersion, Knowledge: projected.Knowledge}
	if got := state.KnowledgeFor("steward"); len(got) != 1 {
		t.Fatalf("a witness to the same moment holds %d records, want the one they were there for", len(got))
	}
	if got := state.KnowledgeFor("guardsman"); len(got) != 1 {
		t.Fatalf("the subject holds %d records, want their own", len(got))
	}
}

// The holder list is a witness list, so it is bounded by who was actually
// there. A model that names a confidant in another city would otherwise hand
// them a private moment they never saw, which is the leak §5.2 forbids: 本人未
// 听见或未被告知的他人私聊 is not a permitted source. Being listed is not being
// present, and the scene is what decides.
func TestParticipantsAudienceCannotNameSomeoneWhoWasNotThere(t *testing.T) {
	ctx := testProjectionContext()
	ctx.KnownCharacters = append(ctx.KnownCharacters, "outsider")
	projected, err := Project(ctx, TurnCharacterFacts{
		Knowledge: []KnowledgeDraft{{
			CharacterID: "guardsman", Statement: "暗格在第三块砖后。",
			Nature: NatureSecret, Audience: AudienceParticipants,
			Channel: ChannelPrivate,
			Holders: []string{"guardsman", "outsider", "stranger"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	record := projected.Knowledge[0]
	for _, holder := range record.Holders {
		if holder == "outsider" || holder == "stranger" {
			t.Fatalf("a private moment lists %q, who was not in it: %v", holder, record.Holders)
		}
	}
	if !HolderCanKnow(record, "guardsman") {
		t.Fatal("the subject lost their own secret")
	}
	state := State{Version: StateVersion, Knowledge: projected.Knowledge}
	if got := state.KnowledgeFor("outsider"); len(got) != 0 {
		t.Fatalf("someone outside the conversation can read it: %+v", got)
	}
}
