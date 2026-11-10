package storypack

import (
	"regexp"
	"strings"
)

// The gender vocabulary a catalog row stores. It is closed and short on
// purpose: a person written as 男, 男性 and 男角色 must not become three values
// the window cannot compare, and an empty value is a real state — the authored
// body stated none — rather than a gap to be filled by a guess.
const (
	GenderMale   = "male"
	GenderFemale = "female"
)

// genderPatterns read a gender the author actually wrote down. As with age, the
// pack has no structured gender field in most authored bodies, so the sentence
// is the data.
//
// Only an explicit statement counts. A pronoun is not a declaration — a body
// may say 她 while describing someone else in the same paragraph — and a role
// or a name says nothing about a person's gender that the program may assume.
// That is why nothing here infers from 少女, 掌柜 or the shape of a name.
//
// 男性 and 女性 are the written forms, so the second pattern accepts any
// following character except the two that make it a different word: 性别 is the
// label and 性格 is a temperament, and neither states anyone's gender.
var genderPatterns = []*regexp.Regexp{
	regexp.MustCompile(`性别[：:]\s*([男女])`),
	regexp.MustCompile(`([男女])性[^别格]`),
}

// ParseGender returns the gender an authored body states, or "" when it states
// none.
//
// A body that states both — a list of a group's members, say — is ambiguous
// rather than male-then-female, so conflicting statements resolve to unset. A
// wrong value here would be pinned into the catalog and read by every later
// turn, which is worse than the person simply having no stated gender.
func ParseGender(text string) string {
	stated := ""
	for _, pattern := range genderPatterns {
		for _, match := range pattern.FindAllStringSubmatch(text, -1) {
			if len(match) < 2 {
				continue
			}
			value := GenderMale
			if match[1] == "女" {
				value = GenderFemale
			}
			if stated != "" && stated != value {
				return ""
			}
			stated = value
		}
	}
	return stated
}

// normalizeGender maps a value an author wrote in a catalog row onto the
// vocabulary, so a row that says 男 and one that says male mean the same thing.
// An unrecognised value is unset rather than stored as free text.
func normalizeGender(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "male", "男":
		return GenderMale
	case "female", "女":
		return GenderFemale
	default:
		return ""
	}
}
