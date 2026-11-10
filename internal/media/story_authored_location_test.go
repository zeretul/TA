package media

import (
	"encoding/json"
	"testing"
)

func TestStoryRuleAuthoredLocationExact(t *testing.T) {
	// Values are copied from the authored place records, not inferred from pixels.
	for _, place := range []string{"霍格沃茨图书馆", "霍格沃茨医务室", "摩金夫人长袍专卖店"} {
		t.Run(place, func(t *testing.T) {
			rule := StoryRule{Kind: "observed_named_portrait_at_location", Location: place}
			for _, tc := range []struct {
				name  string
				scene string
				want  bool
			}{
				{"exact", place, true},
				{"unknown", "", false},
				{"elsewhere", "霍格沃茨礼堂", false},
				{"nearby", place + "附近", false},
				{"doorway", place + "门外", false},
				{"sublocation", place + "内的桌旁", false},
				{"not arrived", "尚未到达" + place, false},
				{"left", "已离开" + place, false},
				{"intention", "前往" + place + "途中", false},
				{"multiple places", place + "或霍格沃茨礼堂", false},
				{"quoted mention", "听说过“" + place + "”", false},
				{"leading space", " " + place, false},
				{"trailing space", place + " ", false},
				{"format character", place + "\u200b", false},
				{"line break", place + "\n", false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if got := rule.AllowsLocation(tc.scene); got != tc.want {
						t.Fatalf("AllowsLocation(%q) = %v, want %v", tc.scene, got, tc.want)
					}
				})
			}
		})
	}
	for _, alias := range []string{"hp-location-library", "图书馆", "Hogwarts Library", "霍格沃兹图书馆"} {
		if (StoryRule{Kind: "observed_named_portrait_at_location", Location: "霍格沃茨图书馆"}).AllowsLocation(alias) {
			t.Fatalf("unapproved location spelling/ID accepted: %q", alias)
		}
	}
}

func TestStoryRuleAuthoredLocationMalformed(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule StoryRule
	}{
		{"legacy kind with location", StoryRule{Kind: "observed_named_portrait", Location: "霍格沃茨图书馆"}},
		{"unknown kind", StoryRule{Kind: "unknown_kind", Location: "霍格沃茨图书馆"}},
		{"unknown empty kind", StoryRule{Kind: "unknown"}},
		{"padded kind", StoryRule{Kind: "observed_named_portrait_at_location ", Location: "霍格沃茨图书馆"}},
		{"missing value", StoryRule{Kind: "observed_named_portrait_at_location"}},
		{"leading space", StoryRule{Kind: "observed_named_portrait_at_location", Location: " 霍格沃茨图书馆"}},
		{"trailing space", StoryRule{Kind: "observed_named_portrait_at_location", Location: "霍格沃茨图书馆 "}},
		{"control", StoryRule{Kind: "observed_named_portrait_at_location", Location: "霍格沃茨\n图书馆"}},
		{"invisible format", StoryRule{Kind: "observed_named_portrait_at_location", Location: "霍格沃茨\u200b图书馆"}},
		{"invalid utf8", StoryRule{Kind: "observed_named_portrait_at_location", Location: "\xff"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Invalid restrictions fail even when both bad strings are identical.
			for _, scene := range []string{tc.rule.Location, "", "霍格沃茨图书馆"} {
				if tc.rule.AllowsLocation(scene) {
					t.Fatalf("malformed location rule accepted scene %q", scene)
				}
			}
		})
	}
}

func TestStoryRuleAuthoredLocationJSONAndLegacy(t *testing.T) {
	for _, scene := range []string{"", "霍格沃茨图书馆", "霍格沃茨礼堂"} {
		if !(StoryRule{Kind: "observed_named_portrait"}).AllowsLocation(scene) {
			t.Fatalf("legacy location behavior changed: %q", scene)
		}
	}
	var rule StoryRule
	if err := json.Unmarshal([]byte(`{"kind":"observed_named_portrait_at_location","location":"霍格沃茨图书馆"}`), &rule); err != nil {
		t.Fatal(err)
	}
	if !rule.AllowsLocation("霍格沃茨图书馆") || rule.AllowsLocation("霍格沃茨图书馆门外") {
		t.Fatal("JSON restriction was not enforced")
	}
	raw, err := json.Marshal(rule)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip StoryRule
	if err := json.Unmarshal(raw, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if roundtrip.Location != rule.Location || roundtrip.Kind != rule.Kind {
		t.Fatal("JSON round trip lost authored location restriction")
	}
	for _, raw := range []string{
		`{"kind":"observed_named_portrait_at_location"}`,
		`{"kind":"observed_named_portrait_at_location","location":null}`,
		`{"location":"霍格沃茨图书馆"}`,
		`{"kind":"future_location_kind","location":"hp-location-library"}`,
	} {
		var invalid StoryRule
		if err := json.Unmarshal([]byte(raw), &invalid); err != nil {
			t.Fatal(err)
		}
		if invalid.AllowsLocation("霍格沃茨图书馆") {
			t.Fatalf("incomplete or unknown JSON contract accepted: %s", raw)
		}
	}
}
