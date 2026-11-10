package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"ta/internal/character"
	"testing"
)

// Rebuild through the formal projection, not by inventing a visual assertion.
func rolePhraseStoryMediaState(t *testing.T, role, utterance string) character.State {
	t.Helper()
	_, _, s := storyMediaGateFixture(t)
	p := s.Profiles[0]
	p.Role = role
	n := 0
	out, err := character.Project(character.ProjectionContext{
		StoryID: "st", BranchID: "main", TurnID: "turn", CommitID: "commit", WorldTime: "1991年9月1日",
		Scene:           character.Scene{NodeID: "turn", WorldTime: "1991年9月1日", Location: "天文塔", Present: []string{"protagonist", p.ID}},
		KnownCharacters: []string{p.ID},
		VisualContext:   &character.VisualObservationContext{ObserverID: "protagonist", Subjects: map[string]character.VisualSubject{p.ID: {Profile: p, Appearance: "人类女性，穿深色教师长袍"}}},
		NextID:          func(prefix string) string { n++; return fmt.Sprintf("%s%d", prefix, n) },
	}, character.TurnCharacterFacts{
		Exchanges: []character.ExchangeDraft{
			{CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "晚上好。"},
			{CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: utterance},
		},
		Knowledge: []character.KnowledgeDraft{{CharacterID: "protagonist", Statement: "我亲眼见到她并听到她介绍姓名。", Nature: character.NatureExperienced, Channel: character.ChannelScene, Visual: &character.VisualObservationDraft{SubjectID: p.ID, Form: "人类女性", ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return character.State{Profiles: []character.Profile{p}, Exchanges: out.Exchanges, Knowledge: out.Knowledge}
}

func TestStoryMediaReviewedRolePhraseDoesNotRequireGMBackground(t *testing.T) {
	const role = "天文学教授，关心学生的课程与私人生活。"
	const phrase = "天文学教授"
	const line = "我是奥罗拉·辛尼斯塔，天文学教授。"
	for _, semantics := range []string{"", "current_role"} {
		a, c, _ := storyMediaGateFixture(t)
		a.Rule.Role, a.Rule.RoleSemantics, a.Rule.RoleRecognition = role, semantics, phrase
		s := rolePhraseStoryMediaState(t, role, line)
		v := s.Knowledge[0].Visual
		if v.Role != "" || v.ProfileRevision < 1 {
			t.Fatal("partial role source copied GM background")
		}
		raw, err := json.Marshal(s.Knowledge)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), role) {
			t.Fatal("unspoken GM role leaked into knowledge")
		}
		if !storyAssetAllowed(a, c, s, "sinistra") {
			t.Fatalf("reviewed phrase rejected for %q", semantics)
		}
		a.Rule.RoleRecognition = ""
		if storyAssetAllowed(a, c, s, "sinistra") {
			t.Fatal("missing override weakened the full-role default")
		}
	}
}

func TestStoryMediaRolePhraseRejectsUnsupportedMappingsAndSources(t *testing.T) {
	const role = "天文学教授，关心学生的课程与私人生活。"
	cases := []string{"unrelated phrase", "whitespace phrase", "leading space", "trailing space", "classification conflict", "unknown role semantics", "name only", "different role", "changed profile", "changed full role", "role denial", "source mismatch"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			a, c, _ := storyMediaGateFixture(t)
			a.Rule.Role, a.Rule.RoleSemantics, a.Rule.RoleRecognition = role, "current_role", "天文学教授"
			line := "我是奥罗拉·辛尼斯塔，天文学教授。"
			if name == "name only" {
				line = "我是奥罗拉·辛尼斯塔。"
			}
			if name == "different role" {
				line = "我是奥罗拉·辛尼斯塔，图书管理员。"
			}
			s := rolePhraseStoryMediaState(t, role, line)
			switch name {
			case "unrelated phrase":
				a.Rule.RoleRecognition = "图书管理员"
			case "whitespace phrase":
				a.Rule.RoleRecognition = " "
			case "leading space":
				a.Rule.RoleRecognition = " 天文学教授"
			case "trailing space":
				a.Rule.RoleRecognition = "天文学教授 "
			case "classification conflict":
				a.Rule.RoleSemantics = "background_classification"
			case "unknown role semantics":
				a.Rule.RoleSemantics = "Current_Role"
			case "changed profile":
				s.Profiles[0].Revision++
			case "changed full role":
				s.Profiles[0].Role = "天文学教授，新的背景。"
			case "role denial":
				s.Exchanges[1].Text = "我是奥罗拉·辛尼斯塔，但不是天文学教授。"
			case "source mismatch":
				s.Exchanges[1].SourceEventID = "other"
			}
			if storyAssetAllowed(a, c, s, "sinistra") {
				t.Fatal("unsupported role phrase authorized image")
			}
		})
	}
}

func TestStoryMediaRolePhrasePreservesLegacySource(t *testing.T) {
	a, c, s := storyMediaGateFixture(t)
	a.Rule.RoleRecognition = a.Rule.Role
	if !storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("explicit full-role phrase rejected old visual")
	}
	a.Rule.RoleRecognition = ""
	if !storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("default changed old visual contract")
	}
}

func TestStoryMediaRolePhraseRetainsCurrentObservationWithCalendarTime(t *testing.T) {
	a, c, _ := storyMediaGateFixture(t)
	a.Rule.Role = "天文学教授，关心学生的课程与私人生活。"
	a.Rule.RoleRecognition = "天文学教授"
	a.Rule.TimeSemantics, a.Rule.Year = "current_observation", 0
	s := rolePhraseStoryMediaState(t, a.Rule.Role, "我是奥罗拉·辛尼斯塔，天文学教授。")
	if !storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("yearless authored rule rejected actual calendar observation")
	}
	c.Snapshot.State["actors"].(map[string]any)["story"].(map[string]any)["state"].(map[string]any)["当前时间"] = "1991年9月2日"
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("role phrase bypassed exact current-observation time")
	}
}
