package storypack

import "testing"

// The authored bodies state an age in either script. Both must read, because a
// body that states an age and parses as zero silently loses the person's aging
// and every adult-only surface that depends on it.
func TestParseAgeReadsBothNumeralScripts(t *testing.T) {
	cases := []struct {
		name string
		text string
		want int
	}{
		{"arabic in the labelled form", "实际年龄：46岁，外貌二十四五岁。", 46},
		{"chinese in the labelled form", "实际年龄四十六岁，外貌看起来二十四五岁。", 46},
		{"chinese over a hundred", "实际年龄一百五十六岁，外貌看起来四十出头。", 156},
		{"chinese with a zero digit", "实际年龄一百零三岁。", 103},
		{"chinese teen", "实际年龄十六岁。", 16},
		{"chinese tens", "实际年龄七十岁。", 70},
		{"arabic in the bare form", "实际148岁，外貌五十余岁。", 148},
		{"chinese in the bare form", "实际一百四十八岁，外貌五十余岁。", 148},
		{"protagonist profile", "性别：女，年龄：24 岁。", 24},
		{"no age at all", "他是越国边境上的一个外务修士。", 0},
		{"empty", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseAge(tc.text); got != tc.want {
				t.Fatalf("ParseAge(%q) = %d, want %d", tc.text, got, tc.want)
			}
		})
	}
}

func TestAuthoredAgeSentenceKeepsActualAndAppearanceSeparate(t *testing.T) {
	for _, tc := range []struct {
		body string
		age  int
	}{
		{"二十五岁，外貌二十余岁。", 25},
		{"是一位琴修。今年五十八岁，外貌看起来二十七岁。", 58},
		{"外貌二十五岁，生辰未记。", 0},
		{"二十五岁时离家修行。", 0},
		{"外貌二十岁。实际年龄十六岁。", 16},
	} {
		if got := parseAuthoredAge(tc.body); got != tc.age {
			t.Errorf("parseAuthoredAge(%q) = %d, want %d", tc.body, got, tc.age)
		}
	}
}
