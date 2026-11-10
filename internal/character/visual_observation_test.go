package character

import (
	"fmt"
	"testing"
)

func visualFixture() (ProjectionContext, TurnCharacterFacts) {
	p := Profile{ID: "sinistra", StoryID: "story", Name: "奥罗拉·辛尼斯塔", KnownAs: "奥罗拉·辛尼斯塔", SourceKey: "hp-npc-sinistra", Role: "天文学教授", Lifeform: "human", Gender: "female", Revision: 1}
	n := 0
	ctx := ProjectionContext{StoryID: "story", BranchID: "main", TurnID: "turn", WorldTime: "1991年9月1日", CommitID: "commit", Scene: Scene{NodeID: "turn", WorldTime: "1991年9月1日", Location: "天文塔", Present: []string{"protagonist", "sinistra"}}, KnownCharacters: []string{"sinistra"}, VisualContext: &VisualObservationContext{ObserverID: "protagonist", Subjects: map[string]VisualSubject{"sinistra": {Profile: p, Appearance: "人类女性，穿深色教师长袍"}}}, NextID: func(prefix string) string { n++; return fmt.Sprintf("%s-%d", prefix, n) }}
	facts := TurnCharacterFacts{Exchanges: []ExchangeDraft{{CharacterID: "sinistra", Speaker: SpeakerCharacter, Channel: ChannelScene, AddressedTo: "protagonist", Text: "晚上好。"}, {CharacterID: "sinistra", Speaker: SpeakerCharacter, Channel: ChannelScene, AddressedTo: "protagonist", Text: "我是奥罗拉·辛尼斯塔，天文学教授。"}}, Knowledge: []KnowledgeDraft{{CharacterID: "protagonist", Statement: "我当面见到并听她介绍自己，实际看清她的人类女性形态。", Nature: NatureExperienced, Channel: ChannelScene, Visual: &VisualObservationDraft{SubjectID: "sinistra", Form: "人类女性", ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}}}
	return ctx, facts
}
func TestVisualObservationUsesCommittedSourceRelations(t *testing.T) {
	ctx, facts := visualFixture()
	out, err := Project(ctx, facts)
	if err != nil {
		t.Fatal(err)
	}
	v := out.Knowledge[0].Visual
	if v == nil || v.Year != 1991 || v.SourceKey != "hp-npc-sinistra" || v.ContactExchangeID != out.Exchanges[0].ID || v.RecognitionExchangeID != out.Exchanges[1].ID || out.Knowledge[0].SourceExchangeID != out.Exchanges[0].ID {
		t.Fatalf("source lost: %+v", out)
	}
}
func TestVisualObservationRefusesUnsupportedClaims(t *testing.T) {
	tests := map[string]func(*ProjectionContext, *TurnCharacterFacts){
		"identity denial": func(c *ProjectionContext, f *TurnCharacterFacts) {
			f.Exchanges[1].Text = "我不是奥罗拉·辛尼斯塔，天文学教授。"
		},
		"role denial": func(c *ProjectionContext, f *TurnCharacterFacts) {
			f.Exchanges[1].Text = "我是奥罗拉·辛尼斯塔，但不是天文学教授。"
		},
		"hearsay identity": func(c *ProjectionContext, f *TurnCharacterFacts) {
			f.Exchanges[1].Text = "听说我是奥罗拉·辛尼斯塔，天文学教授。"
		},
		"unknown role": func(c *ProjectionContext, f *TurnCharacterFacts) {
			v := c.VisualContext.Subjects["sinistra"]
			v.Profile.Role = ""
			c.VisualContext.Subjects["sinistra"] = v
		},
		"side write":  func(c *ProjectionContext, f *TurnCharacterFacts) { c.VisualContext = nil },
		"hearsay":     func(c *ProjectionContext, f *TurnCharacterFacts) { f.Knowledge[0].Nature = NatureHearsay },
		"private":     func(c *ProjectionContext, f *TurnCharacterFacts) { f.Exchanges[0].Channel = ChannelPrivate },
		"same source": func(c *ProjectionContext, f *TurnCharacterFacts) { f.Knowledge[0].Visual.RecognitionExchangeIndex = 1 },
		"unknown name": func(c *ProjectionContext, f *TurnCharacterFacts) {
			s := c.VisualContext.Subjects["sinistra"]
			s.Profile.KnownAs = ""
			c.VisualContext.Subjects["sinistra"] = s
		},
		"no naming utterance": func(c *ProjectionContext, f *TurnCharacterFacts) { f.Exchanges[1].Text = "天气不错。" },
		"not present":         func(c *ProjectionContext, f *TurnCharacterFacts) { c.Scene.Present = []string{"protagonist"} },
		"relative clock":      func(c *ProjectionContext, f *TurnCharacterFacts) { c.Scene.WorldTime = "第1991日" },
		"form invented":       func(c *ProjectionContext, f *TurnCharacterFacts) { f.Knowledge[0].Visual.Form = "幼年形态" },
		"contact to another":  func(c *ProjectionContext, f *TurnCharacterFacts) { f.Exchanges[0].AddressedTo = "someone-else" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			c, f := visualFixture()
			mutate(&c, &f)
			if _, err := Project(c, f); err == nil {
				t.Fatal("unsupported observation accepted")
			}
		})
	}
}
func TestVisualYearDoesNotGuessFromNarrative(t *testing.T) {
	for _, s := range []string{"第1991日", "廷根早期", "1991", "1991年前后", "chapter 1991"} {
		if ExplicitVisualYear(s) != 0 {
			t.Fatalf("guessed year from %q", s)
		}
	}
}
