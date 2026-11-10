package app

import (
	"encoding/json"
	"os"
	"ta/internal/character"
	"ta/internal/media"
	"testing"
)

func TestBackgroundClassificationActualRole(t *testing.T) {
	manifest := os.Getenv("TA_FINAL_PORTRAIT_MANIFEST")
	if manifest == "" {
		t.Skip("set TA_FINAL_PORTRAIT_MANIFEST to the reviewed runtime media manifest")
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Assets []media.StoryAsset `json:"assets"`
	}
	if err = json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, a := range m.Assets {
		if a.Rule == nil || a.Rule.RoleSemantics != "background_classification" {
			continue
		}
		count++
		t.Run(a.SourceKey, func(t *testing.T) {
			r := a.Rule
			for _, role := range []string{"军官", "本局已确认的地方官员"} {
				spec := approvedMappingCase{source: a.SourceKey, name: r.KnownAs, role: role, gender: r.Gender, form: r.Form, roleSemantics: r.RoleSemantics, recognition: "我叫" + r.KnownAs + "。", worldTime: "190年9月1日", year: 190, adult: true}
				_, c, s := approvedMappingFixture(t, spec)
				if !storyAssetAllowed(a, c, s, "sinistra") {
					t.Fatalf("reviewed classification denied exact committed actual role %q", role)
				}
			}
		})
	}
	// The final reviewed set contains the original 20 and 56 additional Sanguo classifications.
	if count != 76 {
		t.Fatalf("expected 76 reviewed background rules, got %d", count)
	}
}

func TestBackgroundClassificationActualRoleGuards(t *testing.T) {
	tests := map[string]func(*media.StoryAsset, *character.State){
		"empty role":              func(a *media.StoryAsset, s *character.State) { s.Profiles[0].Role = "" },
		"blank role":              func(a *media.StoryAsset, s *character.State) { s.Profiles[0].Role = "  " },
		"profile revision drift":  func(a *media.StoryAsset, s *character.State) { s.Profiles[0].Revision++ },
		"snapshot revision drift": func(a *media.StoryAsset, s *character.State) { s.Knowledge[0].Visual.ProfileRevision++ },
		"legacy role snapshot drift": func(a *media.StoryAsset, s *character.State) {
			s.Knowledge[0].Visual.ProfileRevision = 0
			s.Knowledge[0].Visual.Role = "旧职业"
		},
		"mixed role snapshot": func(a *media.StoryAsset, s *character.State) { s.Knowledge[0].Visual.Role = "军官" },
		"guess":               func(a *media.StoryAsset, s *character.State) { s.Exchanges[1].Text = "我可能是曹操。" },
		"third party":         func(a *media.StoryAsset, s *character.State) { s.Exchanges[1].CharacterID = "other" },
		"wrong source":        func(a *media.StoryAsset, s *character.State) { s.Exchanges[1].SourceEventID = "other" },
		"wrong name":          func(a *media.StoryAsset, s *character.State) { s.Profiles[0].Name = "other" },
		"wrong alias":         func(a *media.StoryAsset, s *character.State) { s.Profiles[0].KnownAs = "孟德" },
		"wrong source key":    func(a *media.StoryAsset, s *character.State) { s.Profiles[0].SourceKey = "other" },
		"wrong form":          func(a *media.StoryAsset, s *character.State) { s.Knowledge[0].Visual.Form = "猫" },
		"wrong year":          func(a *media.StoryAsset, s *character.State) { a.Rule.Year = 191 },
		"underage":            func(a *media.StoryAsset, s *character.State) { s.Profiles[0].Age = 12; s.Knowledge[0].Visual.Age = 12 },
		"missing commit":      func(a *media.StoryAsset, s *character.State) { s.Knowledge[0].FormalCommitID = "" },
		"wrong holder":        func(a *media.StoryAsset, s *character.State) { s.Knowledge[0].CharacterID = "other" },
		"wrong audience":      func(a *media.StoryAsset, s *character.State) { s.Exchanges[1].Audience = []string{"sinistra"} },
		"retracted":           func(a *media.StoryAsset, s *character.State) { s.Knowledge[0].RetractedBy = "later" },
		"superseded":          func(a *media.StoryAsset, s *character.State) { s.Knowledge[0].SupersededBy = "later" },
		"wrong branch":        func(a *media.StoryAsset, s *character.State) { s.Exchanges[1].BranchID = "other" },
		"wrong story":         func(a *media.StoryAsset, s *character.State) { s.Profiles[0].StoryID = "other" },
		"unreviewed":          func(a *media.StoryAsset, s *character.State) { a.Reviewed = false },
		"default still matches": func(a *media.StoryAsset, s *character.State) {
			a.Rule.RoleSemantics = ""
			s.Exchanges[1].Text = "我是曹操，军官。"
		},
		"current role still matches": func(a *media.StoryAsset, s *character.State) {
			a.Rule.RoleSemantics = "current_role"
			s.Exchanges[1].Text = "我是曹操，军官。"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			a, c, s := approvedMappingFixture(t, approvedMappingCase{source: "sanguo-person-caocao", name: "曹操", role: "军官", gender: "male", form: "人类男性", roleSemantics: "background_classification", recognition: "我叫曹操。", worldTime: "190年9月1日", year: 190, adult: true})
			a.Rule.Role = "汉末人物"
			mutate(&a, &s)
			if storyAssetAllowed(a, c, s, "sinistra") {
				t.Fatal("classification bypassed guard")
			}
		})
	}
}
