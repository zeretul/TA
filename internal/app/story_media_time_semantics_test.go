package app

import (
	"fmt"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/media"
	"testing"
)

// Synthetic facts only: no official story or asset is altered to create an unlock.
func currentObservationFixture(t *testing.T) (media.StoryAsset, interactive.StoryContext, character.State) {
	t.Helper()
	return currentObservationFixtureAt(t, "第3日午后")
}

func currentObservationFixtureAt(t *testing.T, worldTime string) (media.StoryAsset, interactive.StoryContext, character.State) {
	t.Helper()
	p := character.Profile{ID: "elder", StoryID: "st", Name: "测试长老", KnownAs: "测试长老", SourceKey: "test-elder", Role: "山门长老", Lifeform: "human", Gender: "male", Revision: 1}
	appearance := "老年人类男性，身着灰袍"
	scene := character.Scene{NodeID: "turn", WorldTime: worldTime, Location: "山门", Present: []string{"protagonist", p.ID}}
	n := 0
	projected, err := character.Project(character.ProjectionContext{StoryID: "st", BranchID: "main", TurnID: "turn", CommitID: "commit", WorldTime: scene.WorldTime, Scene: scene, KnownCharacters: []string{p.ID}, VisualContext: &character.VisualObservationContext{ObserverID: "protagonist", Subjects: map[string]character.VisualSubject{p.ID: {Profile: p, Appearance: appearance}}}, NextID: func(prefix string) string { n++; return fmt.Sprintf("%s%d", prefix, n) }}, character.TurnCharacterFacts{Exchanges: []character.ExchangeDraft{{CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "请进。"}, {CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "我是测试长老，山门长老。"}}, Knowledge: []character.KnowledgeDraft{{CharacterID: "protagonist", Statement: "当面见到并听他介绍姓名与职务。", Nature: character.NatureExperienced, Channel: character.ChannelScene, Visual: &character.VisualObservationDraft{SubjectID: p.ID, Form: "老年人类男性", ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	s := character.State{Profiles: []character.Profile{p}, Exchanges: projected.Exchanges, Knowledge: projected.Knowledge}
	c := interactive.StoryContext{Meta: interactive.StoryMeta{PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{SourceKey: p.SourceKey, Name: p.Name}}}}, Snapshot: interactive.Snapshot{StoryID: "st", BranchID: "main", CurrentTurn: &interactive.TurnEvent{ID: "turn"}, State: map[string]any{"actors": map[string]any{"story": map[string]any{"state": map[string]any{"当前时间": scene.WorldTime, "当前详细地点": scene.Location, "在场角色": []any{"protagonist", p.ID}}}, p.ID: map[string]any{"state": map[string]any{"外貌与装扮": appearance}}}}}}
	a := media.StoryAsset{ID: "test-elder-image", SourceKey: p.SourceKey, Reviewed: true, Rule: &media.StoryRule{Kind: "observed_named_portrait", TimeSemantics: "current_observation", Year: 0, Form: "老年人类男性", Lifeform: p.Lifeform, Gender: p.Gender, KnownAs: p.KnownAs, CanonicalName: p.Name, Role: p.Role, AdultIdentity: true}}
	return a, c, s
}

func TestStoryMediaCurrentObservationAndCalendarDefaults(t *testing.T) {
	a, c, s := currentObservationFixture(t)
	if !storyAssetAllowed(a, c, s, "elder") {
		t.Fatal("bound yearless observation denied")
	}
	for _, semantics := range []string{"", "explicit_year", "unknown", "Current_Observation", " current_observation", "current_observation "} {
		a.Rule.TimeSemantics = semantics
		if storyAssetAllowed(a, c, s, "elder") {
			t.Fatalf("yearless observation accepted with %q", semantics)
		}
	}
	for _, semantics := range []string{"", "explicit_year"} {
		a, c, s := storyMediaGateFixture(t)
		a.Rule.TimeSemantics = semantics
		if !storyAssetAllowed(a, c, s, "sinistra") {
			t.Fatalf("legacy calendar observation rejected: %q", semantics)
		}
	}
	a, c, s = storyMediaGateFixture(t)
	a.Rule.TimeSemantics = "current_observation"
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("calendar rule bypassed via current_observation")
	}
}

func TestStoryMediaCurrentObservationRetainsGuards(t *testing.T) {
	for _, change := range []string{"positive year", "negative year", "empty time", "blank time", "time advances", "explicit scene year", "knowledge time", "contact time", "recognition time", "observation year", "missing role source", "changed role", "changed appearance", "changed location", "absent subject", "absent observer", "retracted", "superseded", "branch source", "commit source", "missing contact", "unreviewed", "form mismatch"} {
		t.Run(change, func(t *testing.T) {
			a, c, s := currentObservationFixture(t)
			story := c.Snapshot.State["actors"].(map[string]any)["story"].(map[string]any)["state"].(map[string]any)
			switch change {
			case "positive year":
				a.Rule.Year = 1991
			case "negative year":
				a.Rule.Year = -1
			case "empty time", "blank time":
				label := ""
				if change == "blank time" {
					label = "   "
				}
				story["当前时间"] = label
				s.Knowledge[0].WorldTime = label
				for i := range s.Exchanges {
					s.Exchanges[i].WorldTime = label
				}
			case "time advances":
				story["当前时间"] = "第4日午后"
			case "explicit scene year":
				story["当前时间"] = "1991年9月1日"
			case "knowledge time":
				s.Knowledge[0].WorldTime = "第4日午后"
			case "contact time":
				s.Exchanges[0].WorldTime = "第4日午后"
			case "recognition time":
				s.Exchanges[1].WorldTime = "第4日午后"
			case "observation year":
				s.Knowledge[0].Visual.Year = 1991
			case "missing role source":
				s.Exchanges[1].Text = "我是测试长老。"
			case "changed role":
				s.Profiles[0].Role = "已离任"
			case "changed appearance":
				c.Snapshot.State["actors"].(map[string]any)["elder"].(map[string]any)["state"].(map[string]any)["外貌与装扮"] = "青年人类男性"
			case "changed location":
				story["当前详细地点"] = "城门"
			case "absent subject":
				story["在场角色"] = []any{"protagonist"}
			case "absent observer":
				story["在场角色"] = []any{"elder"}
			case "retracted":
				s.Knowledge[0].RetractedBy = "later"
			case "superseded":
				s.Knowledge[0].SupersededBy = "later"
			case "branch source":
				s.Exchanges[0].BranchID = "unrelated"
			case "commit source":
				s.Exchanges[0].SourceEventID = "unrelated"
			case "missing contact":
				s.Exchanges = s.Exchanges[1:]
			case "unreviewed":
				a.Reviewed = false
			case "form mismatch":
				a.Rule.Form = "青年人类男性"
			}
			if storyAssetAllowed(a, c, s, "elder") {
				t.Fatal("yearless rule bypassed guard")
			}
		})
	}
}

func TestStoryMediaCurrentObservationPreservesActualCivilYear(t *testing.T) {
	for _, change := range []string{"none", "fabricated zero", "fabricated positive", "mixed rule year", "same year later time"} {
		t.Run(change, func(t *testing.T) {
			a, c, s := currentObservationFixtureAt(t, "1349年6月28日午后")
			if s.Knowledge[0].Visual.Year != 1349 {
				t.Fatal("projection lost real civil year")
			}
			switch change {
			case "fabricated zero":
				s.Knowledge[0].Visual.Year = 0
			case "fabricated positive":
				s.Knowledge[0].Visual.Year = 1350
			case "mixed rule year":
				a.Rule.Year = 1349
			case "same year later time":
				c.Snapshot.State["actors"].(map[string]any)["story"].(map[string]any)["state"].(map[string]any)["当前时间"] = "1349年6月29日午后"
			}
			if storyAssetAllowed(a, c, s, "elder") != (change == "none") {
				t.Fatal("current-observation civil-year contract violated")
			}
		})
	}
}
