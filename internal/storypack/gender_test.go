package storypack

import (
	"strings"
	"testing"
)

// TestParseGenderReadsOnlyAStatement is the design's 不根据名字、头像或职业反推 in
// its narrowest form: a body that never states a gender must not be given one.
func TestParseGenderReadsOnlyAStatement(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"stated on its own line", "身份：家族出身弟子；性别：女；境界：炼气12层。", GenderFemale},
		{"stated after a colon", "性别: 男", GenderMale},
		{"stated as a word", "一名男性修士", GenderMale},
		{"stated as a word with punctuation", "她是女性，早年离开家族。", GenderFemale},
		{"a pronoun is not a statement", "她走进院子，没有看任何人。", ""},
		{"a role is not a statement", "身份：家族出身弟子；境界：炼气12层。", ""},
		{"a word about a group is not a statement", "不会为了一个来客命所有女修出场。", ""},
		{"性别 is the label, not the word 男性", "性别：女。", GenderFemale},
		{"性格 is a temperament", "性格：沉稳", ""},
		{"nothing at all", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseGender(tc.body); got != tc.want {
				t.Fatalf("ParseGender(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

// TestParseGenderRefusesAConflictingBody covers the case a body states both —
// a passage describing a group rather than a person. Returning either one would
// pin a coin flip into the catalog and hand it to every later turn, so the
// answer is unset.
func TestParseGenderRefusesAConflictingBody(t *testing.T) {
	body := "领头的是一名男性修士，身后跟着两名女性弟子。"
	if got := ParseGender(body); got != "" {
		t.Fatalf("a body that states both genders resolved to %q, want unset", got)
	}
	// The same statement twice is not a conflict.
	if got := ParseGender("性别：男。此人是一名男性修士。"); got != GenderMale {
		t.Fatalf("a body repeating one gender resolved to %q, want %q", got, GenderMale)
	}
}

// TestNormalizeGenderFoldsTheAuthoredSpellings keeps a row that says 男 and one
// that says male comparable, and keeps an unrecognised word out of the field
// rather than storing it as free text.
func TestNormalizeGenderFoldsTheAuthoredSpellings(t *testing.T) {
	cases := map[string]string{
		"male": GenderMale, "Male": GenderMale, " 男 ": GenderMale,
		"female": GenderFemale, "女": GenderFemale,
		"": "", "unspecified": "", "不详": "",
	}
	for input, want := range cases {
		if got := normalizeGender(input); got != want {
			t.Fatalf("normalizeGender(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestCatalogRowGenderWinsOverTheBody states the precedence: a row's own field
// was written about this person specifically, while a body's sentence is read
// out of prose. Only when the row is silent does the body answer.
func TestCatalogRowGenderWinsOverTheBody(t *testing.T) {
	pack := Pack{
		World: []WorldEntry{{ID: "body-a", Content: "性别：女"}},
		Characters: []CatalogCharacter{
			{SourceKey: "row-wins", Name: "甲", LoreID: "body-a", Gender: GenderMale, RegionTokens: []string{"甲地"}},
			{SourceKey: "body-answers", Name: "乙", LoreID: "body-a", RegionTokens: []string{"甲地"}},
		},
	}
	rows := pack.normalizedCatalog()
	if len(rows) != 2 {
		t.Fatalf("catalog normalized to %d rows, want 2", len(rows))
	}
	byKey := map[string]CatalogCharacter{}
	for _, row := range rows {
		byKey[row.SourceKey] = row
	}
	if got := byKey["row-wins"].Gender; got != GenderMale {
		t.Fatalf("a row that states its own gender came out %q, want %q", got, GenderMale)
	}
	if got := byKey["body-answers"].Gender; got != GenderFemale {
		t.Fatalf("a silent row did not take the body's stated gender: got %q, want %q", got, GenderFemale)
	}
}

// gendersTheAuthorNeverSettled lists the shipped people whose bodies and
// profiles state no gender at all. It is the other half of the guard below: a
// row that states a gender must have been decided from the author's own words,
// and these are the ones no such words exist for, so they must stay unset.
// Adding a name here is a claim that the author never said; removing one is a
// claim that they did, and the body must then be able to answer.
//
// A portrait never settles this on its own. The pack's gender column is read as
// a statement about the author, and catalog.go says it is never inferred from a
// picture; the profiles say the same in their first section. Several of the
// people who used to sit in this list had only a portrait behind their value,
// and some of those portraits are shared job templates rather than a picture of
// that person at all. The twelve originals the author settled explicitly now
// carry the setting in their profile text. 辛如音 left the list once her scenes
// were checked: chapter 277 has her as 蓝衣女子 and saying 小女子辛如音, which is
// the novel stating it rather than a guess. See
// .ta-dev/copy-rework/gender-evidence.json for the per-person finding.
var gendersTheAuthorNeverSettled = map[string]bool{
	"species_lingshoushan_qingfeng_que": true, // 青风雀（灵兽，性别不适用）
}

// TestShippedCatalogLeavesUnknownGendersUnset is the guard against a later
// batch deciding to fill the column by guessing. Every row whose person the
// author never wrote a gender for must come out unset — a name, a role and a
// body that only uses pronouns are not evidence.
//
// The complement matters as much: every other row must state one. A row that
// is silently blank where the author did decide leaves the model to guess on
// its own, which is the same failure from the other side.
func TestShippedCatalogLeavesUnknownGendersUnset(t *testing.T) {
	pack := fanrenPack(t)
	rows := pack.normalizedCatalog()
	// A key that no longer names a row would make the list a silent no-op, so
	// every entry has to still be there.
	present := make(map[string]bool, len(rows))
	for _, row := range rows {
		present[row.SourceKey] = true
	}
	for key := range gendersTheAuthorNeverSettled {
		if !present[key] {
			t.Fatalf("the unsettled list names %s, which is not in the shipped catalog", key)
		}
	}
	for _, row := range rows {
		if gendersTheAuthorNeverSettled[row.SourceKey] {
			if row.Gender != "" {
				t.Fatalf("%s (%s) came out %q, but the pack never states a gender for them",
					row.Name, row.SourceKey, row.Gender)
			}
			continue
		}
		if row.Gender != GenderMale && row.Gender != GenderFemale {
			t.Fatalf("%s (%s) came out %q, but the author did settle it and the row must say so",
				row.Name, row.SourceKey, row.Gender)
		}
	}
}

// TestCandidateCardCarriesTheAuthoredGender is the hand-off from the pack to the
// turn: a person the author already decided about reaches the model with that
// answer, so the profile the turn submits does not re-decide it.
func TestCandidateCardCarriesTheAuthoredGender(t *testing.T) {
	card := candidateCard(Candidate{
		Character: CatalogCharacter{SourceKey: "npc.a", Name: "甲", Gender: GenderFemale},
		Reach:     ReachLocal,
	})
	if !strings.Contains(card, "- Gender: female") {
		t.Fatalf("the card does not state the authored gender:\n%s", card)
	}
	// An unsettled person gets no line at all. A blank "Gender:" would read as
	// something the model is meant to fill in, which is exactly the guessing the
	// design forbids.
	silent := candidateCard(Candidate{Character: CatalogCharacter{SourceKey: "npc.b", Name: "乙"}, Reach: ReachLocal})
	if strings.Contains(silent, "Gender") {
		t.Fatalf("a person with no settled gender still got a gender line:\n%s", silent)
	}
}
