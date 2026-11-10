package app

import (
	"ta/internal/character"
	"testing"
)

func singleSourceState(s character.State) character.State {
	s.Exchanges = s.Exchanges[1:]
	s.Knowledge[0].Visual.ContactExchangeID = s.Exchanges[0].ID
	s.Knowledge[0].SourceExchangeID = s.Exchanges[0].ID
	return s
}

func TestStoryMediaSingleSourceIntroduction(t *testing.T) {
	a, c, s := storyMediaGateFixture(t)
	s = singleSourceState(s)
	if !storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("single witnessed self introduction denied")
	}
}

func TestStoryMediaSingleSourceRetainsGuards(t *testing.T) {
	tests := map[string]func(*character.State){
		"third party":    func(s *character.State) { s.Exchanges[0].CharacterID = "impostor" },
		"wrong observer": func(s *character.State) { s.Exchanges[0].AddressedTo = "other" },
		"wrong holder":   func(s *character.State) { s.Knowledge[0].CharacterID = "other" },
		"wrong subject":  func(s *character.State) { s.Knowledge[0].Visual.SubjectID = "other" },
		"alias prefix": func(s *character.State) {
			s.Exchanges[0].Text = "我是奥罗拉·辛尼斯塔的助手，天文学教授。"
		},
		"denial": func(s *character.State) {
			s.Exchanges[0].Text = "我不是奥罗拉·辛尼斯塔，天文学教授。"
		},
		"guess": func(s *character.State) {
			s.Exchanges[0].Text = "我是奥罗拉·辛尼斯塔，可能是天文学教授。"
		},
		"role mismatch":                func(s *character.State) { s.Exchanges[0].Text = "我是奥罗拉·辛尼斯塔，魔药学教授。" },
		"name only cannot unlock role": func(s *character.State) { s.Exchanges[0].Text = "我是奥罗拉·辛尼斯塔。" },
		"private":                      func(s *character.State) { s.Exchanges[0].Channel = character.ChannelPrivate },
		"letter":                       func(s *character.State) { s.Exchanges[0].Channel = character.Channel("letter") },
		"observer audience":            func(s *character.State) { s.Exchanges[0].Audience = []string{"sinistra"} },
		"subject audience":             func(s *character.State) { s.Exchanges[0].Audience = []string{"protagonist"} },
		"wrong source":                 func(s *character.State) { s.Exchanges[0].SourceEventID = "other" },
		"wrong turn":                   func(s *character.State) { s.Exchanges[0].TurnID = "other" },
		"wrong time":                   func(s *character.State) { s.Exchanges[0].WorldTime = "1992年9月1日" },
		"wrong branch":                 func(s *character.State) { s.Exchanges[0].BranchID = "other" },
		"wrong story":                  func(s *character.State) { s.Exchanges[0].StoryID = "other" },
		"wrong snapshot source":        func(s *character.State) { s.Knowledge[0].Visual.SourceKey = "other" },
		"wrong appearance":             func(s *character.State) { s.Knowledge[0].Visual.Appearance = "人类女性，穿红衣" },
		"wrong form":                   func(s *character.State) { s.Knowledge[0].Visual.Form = "猫" },
		"retracted":                    func(s *character.State) { s.Knowledge[0].RetractedBy = "later" },
		"superseded":                   func(s *character.State) { s.Knowledge[0].SupersededBy = "later" },
		"missing formal commit":        func(s *character.State) { s.Knowledge[0].FormalCommitID = "" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			a, c, s := storyMediaGateFixture(t)
			s = singleSourceState(s)
			mutate(&s)
			if storyAssetAllowed(a, c, s, "sinistra") {
				t.Fatal("unsafe single source accepted")
			}
		})
	}
}
