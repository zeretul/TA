package character

import "strings"

// The gender vocabulary a profile stores. It is closed and short on purpose:
// 男, 男性 and 男角色 must not become three values the window cannot compare,
// and an empty value is a real state — nobody settled it — rather than a gap.
const (
	GenderMale   = "male"
	GenderFemale = "female"
)

// NormalizeGender maps whatever a caller wrote onto the vocabulary. An
// unrecognised value becomes empty rather than being stored as free text: a
// profile field that holds 男性 sometimes and male other times cannot be
// displayed consistently, and guessing at an unknown word is how a person ends
// up with a gender nobody decided.
func NormalizeGender(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "male", "男", "男性":
		return GenderMale
	case "female", "女", "女性":
		return GenderFemale
	default:
		return ""
	}
}
