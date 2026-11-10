package app

import (
	"encoding/json"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/media"
	"testing"
)

// JSON injection makes this regression compile against the pre-feature baseline.
func publicTitleFixture(t *testing.T) (media.StoryAsset, interactive.StoryContext, character.State) {
	a, c, s := storyMediaGateFixture(t)
	a.Rule.KnownAs = "米勒娃·麦格"
	a.Rule.CanonicalName = "米勒娃·麦格"
	s.Profiles[0].Name = "米勒娃·麦格"
	c.Meta.PackCatalog.Characters[0].Name = "米勒娃·麦格"
	raw, err := json.Marshal(a.Rule)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err = json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	obj["known_as_aliases"] = []string{"麦格教授"}
	raw, err = json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, a.Rule); err != nil {
		t.Fatal(err)
	}
	s.Profiles[0].KnownAs = "麦格教授"
	s.Knowledge[0].Visual.KnownAs = "麦格教授"
	s.Exchanges[1].Text = "我是麦格教授，天文学教授。"
	return a, c, s
}
func TestReviewedPublicTitleAccepted(t *testing.T) {
	a, c, s := publicTitleFixture(t)
	if !storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("reviewed public title with affirmative committed self-identification denied")
	}
}
func TestReviewedPublicTitleDoesNotWeakenEvidence(t *testing.T) {
	cases := map[string]func(*media.StoryAsset, *interactive.StoryContext, *character.State){
		"prefix self identification": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Exchanges[1].Text = "我是麦格教授的助手，天文学教授。"
		},
		"generic self identification": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Exchanges[1].Text = "我是教授，天文学教授。"
		},
		"another hidden persona": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Exchanges[1].Text = "我是神秘访客，天文学教授。"
		},

		"no whitelist": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			raw, _ := json.Marshal(a.Rule)
			var obj map[string]any
			json.Unmarshal(raw, &obj)
			delete(obj, "known_as_aliases")
			raw, _ = json.Marshal(obj)
			a.Rule = &media.StoryRule{}
			json.Unmarshal(raw, a.Rule)
		},
		"unapproved": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Profiles[0].KnownAs = "隐藏身份"
		},
		"generic": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Profiles[0].KnownAs = "教授"
		},
		"prefix": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Profiles[0].KnownAs = "麦格教授的助手"
		},
		"third party": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Exchanges[1].CharacterID = "other"
		},
		"denial": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Exchanges[1].Text = "我不是麦格教授，天文学教授。"
		},
		"wrong role": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Profiles[0].Role = "访客"
		},
		"wrong form": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].Visual.Form = "猫"
		},
		"wrong year": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].Visual.Year = 1992
		},
		"wrong source": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Profiles[0].SourceKey = "other"
		},
		"wrong canonical": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Profiles[0].Name = "another"
		},
		"old full name recognition": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Exchanges[1].Text = "我是米勒娃·麦格，天文学教授。"
		},
		"old full name visual": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].Visual.KnownAs = a.Rule.KnownAs
		},
		"retracted": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].RetractedBy = "later"
		},
		"unknown": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) { s.Profiles[0].KnownAs = "" },
		"holder": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].CharacterID = "other"
		},
		"commit": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].FormalCommitID = ""
		},
		"branch": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Exchanges[1].BranchID = "other"
		},
		"unreviewed": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) { a.Reviewed = false },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			a, c, s := publicTitleFixture(t)
			change(&a, &c, &s)
			if storyAssetAllowed(a, c, s, "sinistra") {
				t.Fatal("unsafe alias portrait authorized")
			}
		})
	}
}
