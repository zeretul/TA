package character

import "testing"

// TestAffinityBandsFollowTheDesignCutPoints pins the seven ranges the design
// names. They are the whole reason a number is shown as a word, so an off-by-one
// at a boundary would quietly relabel how a character behaves.
func TestAffinityBandsFollowTheDesignCutPoints(t *testing.T) {
	cases := []struct {
		value int
		band  AffinityBand
	}{
		{-100, AffinityHostile}, {-60, AffinityHostile},
		{-59, AffinityWary}, {-20, AffinityWary},
		{-19, AffinityNeutral}, {0, AffinityNeutral}, {19, AffinityNeutral},
		{20, AffinityFamiliar}, {39, AffinityFamiliar},
		{40, AffinityFriendly}, {59, AffinityFriendly},
		{60, AffinityClose}, {79, AffinityClose},
		{80, AffinityDeep}, {100, AffinityDeep},
	}
	for _, testCase := range cases {
		if got := BandFor(testCase.value); got != testCase.band {
			t.Fatalf("BandFor(%d) = %q, want %q", testCase.value, got, testCase.band)
		}
	}
}

// TestAffinityClampsAtBothEnds is the rule that a value can never leave the
// legal range, however large the proposed change was.
func TestAffinityClampsAtBothEnds(t *testing.T) {
	if got := ClampAffinity(140); got != AffinityMax {
		t.Fatalf("ClampAffinity(140) = %d, want %d", got, AffinityMax)
	}
	if got := ClampAffinity(-400); got != AffinityMin {
		t.Fatalf("ClampAffinity(-400) = %d, want %d", got, AffinityMin)
	}
	if got := ClampAffinity(37); got != 37 {
		t.Fatalf("ClampAffinity(37) = %d, want 37", got)
	}
}

// TestAffinityDeltaRangesMatchTheTable checks every scale's own bounds. A size
// outside its scale is what the projection refuses, so these numbers are the
// contract the model is corrected against.
func TestAffinityDeltaRangesMatchTheTable(t *testing.T) {
	cases := []struct {
		kind     AffinityKind
		min, max int
	}{
		{AffinityNone, 0, 0},
		{AffinityPleasant, 1, 3},
		{AffinityMeaningful, 4, 8},
		{AffinityMajor, 9, 15},
		{AffinityOffense, -8, -2},
		{AffinityBetrayal, -30, -10},
	}
	for _, testCase := range cases {
		min, max, ok := testCase.kind.DeltaRange()
		if !ok || min != testCase.min || max != testCase.max {
			t.Fatalf("%q range = (%d, %d, %v), want (%d, %d)", testCase.kind, min, max, ok, testCase.min, testCase.max)
		}
	}
	if _, _, ok := AffinityKind("friendly").DeltaRange(); ok {
		t.Fatalf("an unknown scale reported a range")
	}
}

func affinityChange(id, characterID string, delta int, basis string) AffinityChange {
	before := 0
	return AffinityChange{
		ID: id, StoryID: "st", BranchID: "main", CharacterID: characterID,
		Kind: AffinityPleasant, Before: before, After: before + delta, Delta: delta,
		Band: BandFor(before + delta), Basis: basis, WorldTime: "第1日",
	}
}

// TestApplyAffinitiesSettlesAFactOnce is the anti-farming rule at the record
// layer: a repeated gift or a second thank-you must add nothing, however many
// times it arrives.
func TestApplyAffinitiesSettlesAFactOnce(t *testing.T) {
	first := affinityChange("aff_1", "npc", 3, "主角替她挑回了丢失的药材")
	settled := ApplyAffinities(nil, []AffinityChange{first})
	if len(settled) != 1 {
		t.Fatalf("first settlement = %d records, want 1", len(settled))
	}
	// The same fact, restated with different spacing and capitalization, is the
	// same settlement key.
	repeat := affinityChange("aff_2", "npc", 3, "  主角替她挑回了丢失的药材 ")
	after := ApplyAffinities(settled, []AffinityChange{repeat})
	if len(after) != 1 || after[0].ID != "aff_1" {
		t.Fatalf("repeated fact = %+v, want the original record only", after)
	}
	// A different fact for the same character does count.
	other := affinityChange("aff_3", "npc", 2, "主角记得她怕冷")
	if got := ApplyAffinities(settled, []AffinityChange{other}); len(got) != 2 {
		t.Fatalf("a new fact = %d records, want 2", len(got))
	}
	// The same fact for a different character is that character's own settlement.
	theirs := affinityChange("aff_4", "other", 3, "主角替她挑回了丢失的药材")
	if got := ApplyAffinities(settled, []AffinityChange{theirs}); len(got) != 2 {
		t.Fatalf("the same fact for another character = %d records, want 2", len(got))
	}
}

// TestApplyAffinitiesDropsAChangeThatSaysNothing keeps a 0 out of the record.
// If it were stored, its settlement key would stop the same fact from ever
// being judged later when there is something to judge.
func TestApplyAffinitiesDropsAChangeThatSaysNothing(t *testing.T) {
	zero := affinityChange("aff_0", "npc", 0, "只是打了个招呼")
	if got := ApplyAffinities(nil, []AffinityChange{zero}); len(got) != 0 {
		t.Fatalf("a zero change was recorded: %+v", got)
	}
}

// TestCurrentAffinitySumsEverySettledChange is how the value is read: the
// branch's own history, nothing else. Each record carries the running value, so
// the last one is what stands.
func TestCurrentAffinitySumsEverySettledChange(t *testing.T) {
	changes := []AffinityChange{
		affinityChange("a", "npc", 5, "一件"),
		func() AffinityChange {
			next := affinityChange("b", "npc", -2, "另一件")
			next.Before = 5
			next.After = 3
			next.Band = BandFor(3)
			return next
		}(),
		affinityChange("c", "other", 9, "别人的事"),
	}
	if got := CurrentAffinity("npc", changes); got != 3 {
		t.Fatalf("CurrentAffinity = %d, want 3", got)
	}
	if got := CurrentAffinity("stranger", changes); got != 0 {
		t.Fatalf("a stranger's affinity = %d, want 0", got)
	}
}

// TestRelationshipMinimumIsAFloorNotATrigger records the design's split between
// a number and a bond: the number can block a declaration, never make one.
func TestRelationshipMinimumIsAFloorNotATrigger(t *testing.T) {
	if minimum, ok := RelationshipMinimum(RelationshipRomantic); !ok || minimum != 60 {
		t.Fatalf("romantic minimum = (%d, %v), want 60", minimum, ok)
	}
	if _, ok := RelationshipMinimum(RelationshipHostile); ok {
		t.Fatalf("a hostile relationship reported an affinity floor")
	}
	if !RelationshipRomantic.Valid() || !RelationshipPartner.Romantic() {
		t.Fatalf("relationship kinds are misclassified")
	}
	if RelationshipFriend.Romantic() {
		t.Fatalf("friendship was classified as romance")
	}
}

// TestRelationshipNeedsBothHalves enforces that a bond is established by what
// was actually said. An expression alone is a proposal, not a relationship.
func TestRelationshipNeedsBothHalves(t *testing.T) {
	base := Relationship{
		ID: "rel_1", StoryID: "st", BranchID: "main", CharacterID: "npc",
		Kind: RelationshipFriend, Statement: "两人正式结为朋友",
		ExpressedBy: "ex_1", AnsweredBy: "ex_2", WorldTime: "第1日",
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("a complete friendship was rejected: %v", err)
	}
	unanswered := base
	unanswered.AnsweredBy = ""
	if err := unanswered.Validate(); err == nil {
		t.Fatalf("a friendship with no answer was accepted")
	}
	// Breaking a bond needs only the words that broke it.
	broken := base
	broken.Kind = RelationshipEstranged
	broken.AnsweredBy = ""
	if err := broken.Validate(); err != nil {
		t.Fatalf("a parting with only its cause was rejected: %v", err)
	}
	// Romance is never implied by the record type: it is its own kind and needs
	// the same two halves.
	romance := base
	romance.Kind = RelationshipRomantic
	romance.AnsweredBy = ""
	if err := romance.Validate(); err == nil {
		t.Fatalf("a romance with no answer was accepted")
	}
}

// TestCurrentRelationshipReturnsTheLatestSettledBond means the window's title
// follows the most recent fact rather than the first one ever recorded.
func TestCurrentRelationshipReturnsTheLatestSettledBond(t *testing.T) {
	relationships := []Relationship{
		{ID: "rel_1", CharacterID: "npc", Kind: RelationshipFriend, Statement: "成为朋友", ExpressedBy: "a", AnsweredBy: "b", StoryID: "st", BranchID: "main", WorldTime: "第1日"},
		{ID: "rel_2", CharacterID: "npc", Kind: RelationshipEstranged, Statement: "分道扬镳", ExpressedBy: "c", StoryID: "st", BranchID: "main", WorldTime: "第5日"},
	}
	current, ok := CurrentRelationship("npc", relationships)
	if !ok || current.ID != "rel_2" {
		t.Fatalf("current relationship = %+v (%v), want rel_2", current, ok)
	}
	if _, ok := CurrentRelationship("stranger", relationships); ok {
		t.Fatalf("a stranger reported a relationship")
	}
}
