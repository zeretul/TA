package quest

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode"
)

func TestAdventureLocalizationCoversDisplayWithoutChangingRules(t *testing.T) {
	for _, d := range Definitions() {
		if d.PackageID != "lanchuan-qunxia" && d.PackageID != "xinghai-689" {
			continue
		}
		t.Run(d.ID, func(t *testing.T) {
			before, _ := json.Marshal(d)
			got := LocalizedDefinition(d, "en-US")
			text, ok := englishText[d.ID]
			if !ok || text.Title == "" || text.Brief == "" || len(text.Objectives) != len(d.Objectives) || len(text.Choices) != len(d.Choices) {
				t.Fatal("incomplete display translation")
			}
			raw, _ := json.Marshal(text)
			if strings.IndexFunc(string(raw), func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
				t.Fatal("Chinese leaked into English display")
			}
			for i, c := range got.Choices {
				if c.Title == "" || c.Description == "" || c.Success == "" || c.Failure == "" || c.SuccessEcho == "" || c.FailureEcho == "" {
					t.Fatal("missing outcome")
				}
				c.Title, c.Description, c.Success, c.Failure, c.SuccessEcho, c.FailureEcho = d.Choices[i].Title, d.Choices[i].Description, d.Choices[i].Success, d.Choices[i].Failure, d.Choices[i].SuccessEcho, d.Choices[i].FailureEcho
				for _, reaction := range []*EchoReaction{c.SuccessReaction, c.FailureReaction} {
					if reaction != nil && strings.IndexFunc(reaction.Statement, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
						t.Fatal("reaction translation missing")
					}
				}
				c.SuccessReaction, c.FailureReaction = d.Choices[i].SuccessReaction, d.Choices[i].FailureReaction
				if !reflect.DeepEqual(c, d.Choices[i]) {
					t.Fatal("translation changed choice rules")
				}
			}
			got.Title = "mutated"
			got.Objectives[0].Description = "mutated"
			if len(got.Choices) > 0 {
				got.Choices[0].Success = "mutated"
			}
			after, _ := json.Marshal(d)
			if string(after) != string(before) {
				t.Fatal("translated view aliases the registered definition")
			}
			if !reflect.DeepEqual(LocalizedDefinition(d, "zh-CN"), d) {
				t.Fatal("Chinese view changed")
			}
		})
	}
	custom := Definition{ID: "custom", Title: "作者原文"}
	if !reflect.DeepEqual(LocalizedDefinition(custom, "en-US"), custom) {
		t.Fatal("custom text fallback lost")
	}
}
