package storypack

import (
	"strings"
	"unicode/utf8"
)

// compoundSurnames are the two-character surnames a player's name may start
// with. Without the list 欧阳明 would derive 阳氏, and a household named after
// the wrong half of its own surname reads as somebody else's house.
var compoundSurnames = []string{
	"欧阳", "太史", "端木", "上官", "司马", "东方", "独孤", "南宫", "万俟",
	"闻人", "夏侯", "诸葛", "尉迟", "公羊", "赫连", "澹台", "皇甫", "宗政",
	"濮阳", "公冶", "太叔", "申屠", "公孙", "慕容", "仲孙", "钟离", "长孙",
	"宇文", "司徒", "鲜于", "司空", "闾丘", "子车", "亓官", "司寇", "巫马",
	"公西", "颛孙", "壤驷", "公良", "漆雕", "乐正", "宰父", "谷梁", "拓跋",
	"夹谷", "轩辕", "令狐", "段干", "百里", "呼延", "东郭", "南门", "羊舌",
	"微生", "梁丘", "左丘", "东门", "西门", "第五",
}

// DeriveSurname returns the surname a player's name starts with.
func DeriveSurname(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	for _, surname := range compoundSurnames {
		if strings.HasPrefix(name, surname) {
			return surname
		}
	}
	r, _ := utf8.DecodeRuneInString(name)
	if r == utf8.RuneError {
		return ""
	}
	return string(r)
}

// DeriveFamilyName is the household a new character's surname belongs to, in the
// wording this world uses for families (叶流云 → 叶氏, 欧阳明 → 欧阳氏).
func DeriveFamilyName(name string) string {
	surname := DeriveSurname(name)
	if surname == "" {
		return ""
	}
	return surname + "氏"
}

// ownFamilyAffiliationID is the affiliation that means "the household my own
// surname belongs to". It is one option rather than a free-text field: the name
// is derived from the character's name, so it cannot be filled in with a family
// this world already has.
const ownFamilyAffiliationID = "own"

// ownFamilyName is what a new household is called for a character with this
// name. An empty name leaves the option unnamed, which the page reports rather
// than inventing a family the player never chose.
func ownFamilyName(playerName string) string {
	return DeriveFamilyName(playerName)
}

// applyOriginPlaceholders fills the names an origin's own wording may refer to.
// A sect origin says 宗门 until the player picks one; substituting the chosen
// sect here keeps the authored copy reading as a sentence about this character
// rather than a generic one, and keeps it from naming a sect they did not join.
func applyOriginPlaceholders(text, affiliationName string) string {
	return strings.ReplaceAll(text, affiliationPlaceholder, affiliationName)
}

const affiliationPlaceholder = "{affiliation}"
