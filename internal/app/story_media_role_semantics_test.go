package app

import (
	"fmt"
	"ta/internal/character"
	"testing"
)

// Rebuild through the formal projection, not by inventing a visual assertion.
func nameOnlyStoryMediaState(t *testing.T, role string) character.State {
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
			{CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "我是" + p.KnownAs + "。"},
		},
		Knowledge: []character.KnowledgeDraft{{CharacterID: "protagonist", Statement: "我亲眼见到她并听到她介绍姓名。", Nature: character.NatureExperienced, Channel: character.ChannelScene, Visual: &character.VisualObservationDraft{SubjectID: p.ID, Form: "人类女性", ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return character.State{Profiles: []character.Profile{p}, Exchanges: out.Exchanges, Knowledge: out.Knowledge}
}

func TestStoryMediaNameOnlyRequiresExplicitBackgroundClassification(t *testing.T) {
	for _, semantics := range []string{"", "current_role", "background_classification", "Background_Classification", " background_classification", "background_classification ", "unknown"} {
		t.Run(semantics, func(t *testing.T) {
			a, c, _ := storyMediaGateFixture(t)
			s := nameOnlyStoryMediaState(t, "背景人物")
			a.Rule.Role = "背景人物"
			a.Rule.RoleSemantics = semantics
			got := storyAssetAllowed(a, c, s, "sinistra")
			if got != (semantics == "background_classification") {
				t.Fatalf("semantics %q: allowed=%v", semantics, got)
			}
		})
	}
	// Existing HP mapping cannot infer the professor role from the GM profile.
	a, c, _ := storyMediaGateFixture(t)
	s := nameOnlyStoryMediaState(t, a.Rule.Role)
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("name-only observation unlocked old HP occupational gate")
	}
}

func TestStoryMediaOldRoleSourceAndDefaultRemainCompatible(t *testing.T) {
	for _, semantics := range []string{"", "current_role"} {
		a, c, s := storyMediaGateFixture(t)
		a.Rule.RoleSemantics = semantics
		if s.Knowledge[0].Visual.ProfileRevision != 0 {
			t.Fatal("fixture no longer models old observation")
		}
		if !storyAssetAllowed(a, c, s, "sinistra") {
			t.Fatalf("old source rejected for %q", semantics)
		}
	}
}

func TestStoryMediaBackgroundClassificationRetainsGuards(t *testing.T) {
	for _, mutate := range []string{"profile role", "same-role profile update", "snapshot revision", "denial", "representative", "private", "source", "retracted", "year", "canonical identity"} {
		t.Run(mutate, func(t *testing.T) {
			a, c, _ := storyMediaGateFixture(t)
			s := nameOnlyStoryMediaState(t, "背景人物")
			a.Rule.Role, a.Rule.RoleSemantics = "背景人物", "background_classification"
			switch mutate {
			case "profile role":
				s.Profiles[0].Role = "新分类"
				s.Profiles[0].Revision++
				a.Rule.Role = "新分类"
			case "same-role profile update":
				s.Profiles[0].Revision++
			case "snapshot revision":
				s.Knowledge[0].Visual.ProfileRevision++
			case "denial":
				s.Exchanges[1].Text = "我不是奥罗拉·辛尼斯塔。"
			case "representative":
				s.Exchanges[1].Text = "我是奥罗拉·辛尼斯塔的助手。"
			case "private":
				s.Exchanges[1].Channel = character.ChannelPrivate
			case "source":
				s.Exchanges[1].SourceEventID = "elsewhere"
			case "retracted":
				s.Knowledge[0].RetractedBy = "later"
			case "year":
				a.Rule.Year = 1992
			case "canonical identity":
				a.Rule.CanonicalName = "冒名者"
			}
			if storyAssetAllowed(a, c, s, "sinistra") {
				t.Fatal("classification bypassed an existing guard")
			}
		})
	}
}
