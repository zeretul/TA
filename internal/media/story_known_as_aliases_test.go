package media

import "testing"

func TestStoryRuleReviewedKnownAsAliases(t *testing.T) {
	for _, tc := range []struct {
		name    string
		aliases []string
		known   string
		want    bool
	}{
		{"legacy", nil, "米勒娃·麦格", true}, {"legacy rejects title", nil, "麦格教授", false},
		{"reviewed", []string{"麦格教授"}, "麦格教授", true}, {"retains canonical", []string{"麦格教授"}, "米勒娃·麦格", true},
		{"unknown", []string{"麦格教授"}, "", false}, {"generic", []string{"麦格教授"}, "教授", false},
		{"prefix", []string{"麦格教授"}, "麦格教授的助手", false}, {"unapproved", []string{"麦格教授"}, "Minerva", false},
		{"empty entry", []string{"麦格教授", ""}, "麦格教授", false}, {"leading space", []string{" 麦格教授"}, "麦格教授", false},
		{"trailing space", []string{"麦格教授 "}, "麦格教授 ", false}, {"duplicate", []string{"麦格教授", "麦格教授"}, "麦格教授", false},
		{"canonical duplicate", []string{"米勒娃·麦格"}, "米勒娃·麦格", false},
		{"control", []string{"麦格\n教授"}, "麦格\n教授", false}, {"invisible format", []string{"麦格\u200b教授"}, "麦格\u200b教授", false},
		{"invalid utf8", []string{"\xff"}, "\xff", false}, {"invalid list rejects canonical", []string{""}, "米勒娃·麦格", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := StoryRule{KnownAs: "米勒娃·麦格", KnownAsAliases: tc.aliases}
			if got := r.AllowsKnownAs(tc.known); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	if (StoryRule{KnownAsAliases: []string{"麦格教授"}}).AllowsKnownAs("麦格教授") {
		t.Fatal("required known_as bypassed")
	}
}
