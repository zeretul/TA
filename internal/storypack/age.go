package storypack

import (
	"regexp"
	"strconv"
	"strings"
)

// This file reads a person's age out of the authored world-book body.
//
// Why it exists: the intimacy window may only admit adults, and §3.1 of that
// design is explicit that realm, appearance, a player's statement or a model's
// passing rewrite are not substitutes for the character's own data. The pack
// has no structured age field, but every catalog person's lore body states one
// in a fixed written form, so the authored sentence is the data.
//
// The parse happens once, when a story freezes its catalog, and the result is
// carried as a number. A row whose body states no age gets 0, which reads as
// "not established" and is never treated as an adult.

// agePatterns are the authored forms, most specific first. They are matched
// against the world entry named by the person's LoreID.
//
// The age is written either in Arabic digits (实际年龄：16岁) or in Chinese
// numerals (实际年龄十六岁). The authored prose mixes both, so a pattern that
// only accepted digits would silently read every Chinese-numeral body as
// "no age established", which drops the person out of aging and out of every
// adult-only surface.
var agePatterns = []*regexp.Regexp{
	// 实际年龄：16岁 / 实际年龄十六岁 / 实际年龄四十六岁
	regexp.MustCompile(`实际年龄[：:]?\s*([0-9]+|[零〇一二两三四五六七八九十百]+)岁`),
	// 实际148岁，外貌五十余岁 / 实际一百五十六岁
	regexp.MustCompile(`实际([0-9]+|[零〇一二两三四五六七八九十百]+)岁`),
	// The protagonist's own creation profile: 性别：女，年龄：24 岁。
	regexp.MustCompile(`年龄[：:]?\s*([0-9]+|[零〇一二两三四五六七八九十百]+)\s*岁`),
}

// Some authored person entries state the current age as its own sentence or
// opening clause. Requiring both boundaries excludes appearance and past ages.
var authoredAgeSentence = regexp.MustCompile(`(?:^|[。！？\n])\s*(?:今年)?([0-9]+|[零〇一二两三四五六七八九十百]+)\s*岁\s*(?:[，。；！？,;\n]|$)`)

func parseAuthoredAge(text string) int {
	if age := ParseAge(text); age > 0 {
		return age
	}
	if match := authoredAgeSentence.FindStringSubmatch(text); len(match) > 0 {
		return parseAgeNumber(match[1])
	}
	return 0
}

// ParseAge reads the age a piece of authored prose states. Zero means the prose
// states none.
func ParseAge(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	for _, pattern := range agePatterns {
		// Try each candidate span, longest first: the numeral class is greedy
		// over characters, not over meaning, so "四十六" can match as "四十"
		// and stop before the trailing 六. The longer span is the real number.
		matches := pattern.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if age := parseAgeNumber(match[1]); age > 0 {
				return age
			}
		}
	}
	return 0
}

// parseAgeNumber reads either an Arabic numeral or a Chinese numeral.
func parseAgeNumber(value string) int {
	if age, err := strconv.Atoi(value); err == nil {
		return age
	}
	return parseChineseAge(value)
}

func parseChineseAge(value string) int {
	digits := map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	units := map[rune]int{'十': 10, '百': 100}
	total := 0
	number := 0
	seen := false
	for _, ch := range value {
		if digit, ok := digits[ch]; ok {
			number = digit
			seen = true
			continue
		}
		unit, ok := units[ch]
		if !ok {
			return 0
		}
		seen = true
		if number == 0 {
			number = 1
		}
		total += number * unit
		number = 0
	}
	if !seen {
		return 0
	}
	return total + number
}

// adultAge is the threshold the intimacy window enforces. It is a constant
// rather than a pack setting: a world pack cannot lower the bar for who may be
// a participant.
const adultAge = 18

// AdultAge reports whether a parsed age is an established adult age. Zero is not
// adult: "the body did not say" must not be read as "old enough".
func AdultAge(age int) bool {
	return age >= adultAge
}
