package character

import (
	"strings"
	"testing"
)

// The affinity tests below are about the seam between what a model proposes and
// what the branch records. A number that a model could write directly would be a
// number a character could be talked into, so every assertion here is that the
// program, not the proposal, decides the value.

// TestProjectionComputesTheValueRatherThanBelievingTheModel is that rule. The
// model names a scale and a size inside it; the program computes the value from
// the branch's own history and stamps the record with where it came from.
func TestProjectionComputesTheValueRatherThanBelievingTheModel(t *testing.T) {
	ctx := testProjectionContext()
	ctx.ExistingAffinities = []AffinityChange{{
		ID: "aff-old", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman",
		Kind: AffinityMeaningful, Before: 0, After: 6, Delta: 6, Band: BandFor(6),
		Basis: "主角替他挡住了盘问", WorldTime: "第2日",
	}}
	projected, err := Project(ctx, TurnCharacterFacts{
		Affinities: []AffinityDraft{{
			CharacterID: "guardsman", Kind: AffinityPleasant, Delta: 2,
			Basis: "主角把热汤分给他",
		}},
	})
	if err != nil {
		t.Fatalf("a legal change was rejected: %v", err)
	}
	if len(projected.Affinities) != 1 {
		t.Fatalf("affinities = %d, want 1", len(projected.Affinities))
	}
	change := projected.Affinities[0]
	if change.Before != 6 || change.After != 8 || change.Delta != 2 {
		t.Fatalf("change = before %d after %d delta %d, want 6 -> 8 (+2)", change.Before, change.After, change.Delta)
	}
	if change.Band != AffinityNeutral {
		t.Fatalf("band = %q, want %q for 8", change.Band, AffinityNeutral)
	}
	if change.SourceEventID != "commit-7" || change.TurnID != "turn-7" || change.WorldTime != "第3日 辰时" {
		t.Fatalf("change lost its provenance: %+v", change)
	}
}

// TestProjectionRejectsASizeOutsideItsScale is the corrective diagnostic. A
// model that calls an ordinary courtesy "pleasant" and then asks for twelve
// points is not clamped into agreement: the turn fails with a message that
// states the allowed range.
func TestProjectionRejectsASizeOutsideItsScale(t *testing.T) {
	ctx := testProjectionContext()
	_, err := Project(ctx, TurnCharacterFacts{
		Affinities: []AffinityDraft{{
			CharacterID: "guardsman", Kind: AffinityPleasant, Delta: 12,
			Basis: "说了句客气话",
		}},
	})
	if err == nil {
		t.Fatal("a pleasant exchange worth 12 was accepted")
	}
	if !strings.Contains(err.Error(), "allows") {
		t.Fatalf("the diagnostic does not state the allowed range: %v", err)
	}
}

// TestProjectionSettlesTheSameFactOnlyOnce is the anti-farming rule on the write
// path. The same help proposed in a later turn, or twice in one turn, adds
// nothing at all.
func TestProjectionSettlesTheSameFactOnlyOnce(t *testing.T) {
	ctx := testProjectionContext()
	ctx.ExistingAffinities = []AffinityChange{{
		ID: "aff-old", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman",
		Kind: AffinityPleasant, Before: 0, After: 3, Delta: 3, Band: BandFor(3),
		Basis: "主角替他把药送了回来", WorldTime: "第2日",
	}}
	projected, err := Project(ctx, TurnCharacterFacts{
		Affinities: []AffinityDraft{{
			CharacterID: "guardsman", Kind: AffinityPleasant, Delta: 3,
			Basis: "主角替他把药送了回来",
		}},
	})
	if err != nil {
		t.Fatalf("a repeated fact errored instead of being ignored: %v", err)
	}
	if len(projected.Affinities) != 0 {
		t.Fatalf("a repeated fact was scored again: %+v", projected.Affinities)
	}

	// Twice in the same submission is one fact, not two.
	duplicated, err := Project(testProjectionContext(), TurnCharacterFacts{
		Affinities: []AffinityDraft{
			{CharacterID: "guardsman", Kind: AffinityPleasant, Delta: 2, Basis: "主角替他值了半个夜班"},
			{CharacterID: "guardsman", Kind: AffinityPleasant, Delta: 3, Basis: "主角替他值了半个夜班"},
		},
	})
	if err != nil {
		t.Fatalf("a duplicated fact errored: %v", err)
	}
	if len(duplicated.Affinities) != 1 {
		t.Fatalf("one fact was scored %d times", len(duplicated.Affinities))
	}
}

// TestProjectionNeverScoresAnUnperceivedFact is the knowledge boundary applied
// to feeling. A character who is not in the story cannot be moved by what
// happened, however generous the proposal.
func TestProjectionNeverScoresAnUnperceivedFact(t *testing.T) {
	ctx := testProjectionContext()
	_, err := Project(ctx, TurnCharacterFacts{
		Affinities: []AffinityDraft{{
			CharacterID: "someone-else", Kind: AffinityPleasant, Delta: 2,
			Basis: "主角在别处帮了他",
		}},
	})
	if err == nil {
		t.Fatal("a change for a character outside the roster was accepted")
	}
}

// TestProjectionClampsAtTheCapAndRecordsWhatActuallyHappened keeps the record
// self-consistent: at 100 the stored delta is what moved, not what was asked
// for, so Before and After still add up.
func TestProjectionClampsAtTheCapAndRecordsWhatActuallyHappened(t *testing.T) {
	ctx := testProjectionContext()
	ctx.ExistingAffinities = []AffinityChange{{
		ID: "aff-old", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman",
		Kind: AffinityMajor, Before: 92, After: 98, Delta: 6, Band: BandFor(98),
		Basis: "一起熬过了那场雪", WorldTime: "第2日",
	}}
	projected, err := Project(ctx, TurnCharacterFacts{
		Affinities: []AffinityDraft{{
			CharacterID: "guardsman", Kind: AffinityMajor, Delta: 12,
			Basis: "替他还了那笔旧债",
		}},
	})
	if err != nil {
		t.Fatalf("a change at the cap was rejected: %v", err)
	}
	if len(projected.Affinities) != 1 {
		t.Fatalf("affinities = %d, want 1", len(projected.Affinities))
	}
	change := projected.Affinities[0]
	if change.After != AffinityMax || change.Delta != 2 {
		t.Fatalf("change = after %d delta %d, want after %d delta 2", change.After, change.Delta, AffinityMax)
	}
	if change.Band != AffinityDeep {
		t.Fatalf("band = %q, want %q", change.Band, AffinityDeep)
	}
}

// TestProjectionNeverTurnsAffinityIntoRomance is the design's hardest line. A
// high number is not a bond: the words have to be answered, and the record has
// to cite both halves. What the program can enforce is that the answer exists
// and belongs to this character; whether the cited line is really an acceptance
// stays the model's judgement, which is why the record keeps the citation.
func TestProjectionNeverTurnsAffinityIntoRomance(t *testing.T) {
	ctx := testProjectionContextWithHighAffinity()
	_, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{CharacterID: "guardsman", Speaker: SpeakerPlayer, Channel: ChannelPrivate, Text: "我想与你结为道侣。"},
		},
		Relationships: []RelationshipDraft{{
			CharacterID: "guardsman", Kind: RelationshipPartner,
			Statement: "两人结为道侣", ExpressedByIndex: 1,
		}},
	})
	if err == nil {
		t.Fatal("a romance was accepted from an unanswered declaration")
	}
	if !strings.Contains(err.Error(), "answer") {
		t.Fatalf("the diagnostic does not say the answer is missing: %v", err)
	}

	// The same words with an actual acceptance become a real record that cites
	// both the line that offered it and the line that answered it.
	accepted, err := Project(testProjectionContextWithHighAffinity(), TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{CharacterID: "guardsman", Speaker: SpeakerPlayer, Channel: ChannelPrivate, Text: "我想与你结为道侣。"},
			{CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: ChannelPrivate, Text: "好。此生同行。"},
		},
		Relationships: []RelationshipDraft{{
			CharacterID: "guardsman", Kind: RelationshipPartner,
			Statement: "两人结为道侣", ExpressedByIndex: 1, AnsweredByIndex: 2,
		}},
	})
	if err != nil {
		t.Fatalf("an answered declaration was rejected: %v", err)
	}
	if len(accepted.Relationships) != 1 {
		t.Fatalf("relationships = %d, want 1", len(accepted.Relationships))
	}
	relationship := accepted.Relationships[0]
	if relationship.ExpressedBy == "" || relationship.AnsweredBy == "" {
		t.Fatalf("the relationship cites no exchange: %+v", relationship)
	}
	if relationship.ExpressedBy == relationship.AnsweredBy {
		t.Fatalf("the relationship cites one line as both offer and answer: %+v", relationship)
	}
	if relationship.Kind != RelationshipPartner {
		t.Fatalf("kind = %q, want %q", relationship.Kind, RelationshipPartner)
	}
}

// TestProjectionRefusesABondBelowItsFloor keeps a floor a floor. An oath of
// sworn siblinghood cannot be declared at a number that has not been earned,
// however sincerely it is worded.
func TestProjectionRefusesABondBelowItsFloor(t *testing.T) {
	ctx := testProjectionContext()
	_, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{CharacterID: "guardsman", Speaker: SpeakerPlayer, Channel: ChannelPrivate, Text: "我们结为生死之交吧。"},
			{CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: ChannelPrivate, Text: "好。"},
		},
		Relationships: []RelationshipDraft{{
			CharacterID: "guardsman", Kind: RelationshipSworn,
			Statement: "两人结为生死之交", ExpressedByIndex: 1, AnsweredByIndex: 2,
		}},
	})
	if err == nil {
		t.Fatal("a sworn bond was accepted from a stranger's affinity")
	}
}

// TestProjectionWillNotCiteSomeoneElsesLine stops a bond being pinned to words
// the other person never said. The citation is by position, so a model that
// miscounts must be corrected rather than allowed to attribute an acceptance.
func TestProjectionWillNotCiteSomeoneElsesLine(t *testing.T) {
	ctx := testProjectionContextWithHighAffinity()
	_, err := Project(ctx, TurnCharacterFacts{
		Exchanges: []ExchangeDraft{
			{CharacterID: "steward", Speaker: SpeakerCharacter, Channel: ChannelPrivate, Text: "北门的事我会替他遮着。"},
			{CharacterID: "guardsman", Speaker: SpeakerPlayer, Channel: ChannelPrivate, Text: "我想与你结为道侣。"},
		},
		Relationships: []RelationshipDraft{{
			CharacterID: "guardsman", Kind: RelationshipPartner,
			Statement: "两人结为道侣", ExpressedByIndex: 2, AnsweredByIndex: 1,
		}},
	})
	if err == nil {
		t.Fatal("a relationship cited another character's line as its answer")
	}
}

func testProjectionContextWithHighAffinity() ProjectionContext {
	ctx := testProjectionContext()
	ctx.ExistingAffinities = []AffinityChange{{
		ID: "aff-old", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman",
		// Above every positive floor, so each case below fails on what it is
		// actually about (the missing answer) rather than on the number.
		Kind: AffinityMajor, Before: 70, After: 85, Delta: 15, Band: BandFor(85),
		Basis: "生死与共", WorldTime: "第2日",
	}}
	return ctx
}

// TestProjectionDropsARestatedFactButKeepsADistinctOne is the rule the live run
// broke. The model scored the same concession twice, the second time with a
// longer basis that added a clause to the same sentence. Exact text did not
// match, so the branch recorded +2 twice for one fact.
//
// The other half matters just as much: a genuinely separate act must still
// score, so the check cannot be "anything that shares vocabulary is a repeat".
func TestProjectionDropsARestatedFactButKeepsADistinctOne(t *testing.T) {
	ctx := testProjectionContext()
	ctx.ExistingAffinities = []AffinityChange{{
		ID: "aff-old", StoryID: "story-1", BranchID: "branch-1", CharacterID: "guardsman",
		Kind: AffinityPleasant, Before: 0, After: 2, Delta: 2, Band: BandFor(2),
		Basis: "主角主动撤回索要夹层账的话，并答应只做接下的活、守着不多说。", WorldTime: "第2日",
	}}
	projected, err := Project(ctx, TurnCharacterFacts{
		Affinities: []AffinityDraft{{
			CharacterID: "guardsman", Kind: AffinityPleasant, Delta: 2,
			Basis: "主角主动撤回索要夹层账的话，并再次明确只做接下的活、不多说，守卫据此表示有限度的信任",
		}},
	})
	if err != nil {
		t.Fatalf("a restatement errored instead of being ignored: %v", err)
	}
	if len(projected.Affinities) != 0 {
		t.Fatalf("one fact was scored twice in different words: %+v", projected.Affinities)
	}

	// A different act by the same person still counts, even though it shares
	// the trade and the character.
	distinct, err := Project(ctx, TurnCharacterFacts{
		Affinities: []AffinityDraft{{
			CharacterID: "guardsman", Kind: AffinityMeaningful, Delta: 5,
			Basis: "主角把巡卫换班的消息带给了他，让他来得及把货挪走。",
		}},
	})
	if err != nil {
		t.Fatalf("a distinct act was rejected: %v", err)
	}
	if len(distinct.Affinities) != 1 {
		t.Fatalf("a distinct act did not settle: %+v", distinct.Affinities)
	}
}
