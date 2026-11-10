package app

import (
	"fmt"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/media"
	"testing"
)

func storyMediaGateFixture(t *testing.T) (media.StoryAsset, interactive.StoryContext, character.State) {
	t.Helper()
	p := character.Profile{ID: "sinistra", StoryID: "st", Name: "奥罗拉·辛尼斯塔", KnownAs: "奥罗拉·辛尼斯塔", SourceKey: "hp-npc-sinistra", Role: "天文学教授", Lifeform: "human", Gender: "female", Revision: 1}
	appearance := "人类女性，穿深色教师长袍"
	scene := character.Scene{NodeID: "turn", WorldTime: "1991年9月1日", Location: "天文塔", Present: []string{"protagonist", "sinistra"}}
	n := 0
	facts := character.TurnCharacterFacts{Exchanges: []character.ExchangeDraft{{CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "晚上好。"}, {CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "我是奥罗拉·辛尼斯塔，天文学教授。"}}, Knowledge: []character.KnowledgeDraft{{CharacterID: "protagonist", Statement: "亲眼看见并认识她。", Nature: character.NatureExperienced, Channel: character.ChannelScene, Visual: &character.VisualObservationDraft{SubjectID: p.ID, Form: "人类女性", ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}}}
	projected, err := character.Project(character.ProjectionContext{StoryID: "st", BranchID: "main", TurnID: "turn", CommitID: "commit", WorldTime: scene.WorldTime, Scene: scene, KnownCharacters: []string{p.ID}, VisualContext: &character.VisualObservationContext{ObserverID: "protagonist", Subjects: map[string]character.VisualSubject{p.ID: {Profile: p, Appearance: appearance}}}, NextID: func(prefix string) string { n++; return fmt.Sprintf("%s%d", prefix, n) }}, facts)
	if err != nil {
		t.Fatal(err)
	}
	state := character.State{Profiles: []character.Profile{p}, Exchanges: projected.Exchanges, Knowledge: projected.Knowledge}
	ctx := interactive.StoryContext{Meta: interactive.StoryMeta{PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{SourceKey: p.SourceKey, Name: p.Name}}}}, Snapshot: interactive.Snapshot{StoryID: "st", BranchID: "main", CurrentTurn: &interactive.TurnEvent{ID: "turn"}, State: map[string]any{"actors": map[string]any{"story": map[string]any{"state": map[string]any{"当前时间": scene.WorldTime, "当前详细地点": scene.Location, "在场角色": []any{"protagonist", p.ID}}}, p.ID: map[string]any{"state": map[string]any{"外貌与装扮": appearance}}}}}}
	asset := media.StoryAsset{ID: "p11o-art-hp-person-sinistra-v1", SourceKey: p.SourceKey, Reviewed: true, Rule: &media.StoryRule{Kind: "observed_named_portrait", Year: 1991, Form: "人类女性", Lifeform: "human", Gender: "female", KnownAs: p.KnownAs, CanonicalName: p.Name, Role: p.Role, AdultIdentity: true}}
	return asset, ctx, state
}
func TestStoryMediaGatePositiveAndProvenanceFailures(t *testing.T) {
	a, c, s := storyMediaGateFixture(t)
	if !storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("formal observed named portrait denied")
	}
	tests := map[string]func(*media.StoryAsset, *interactive.StoryContext, *character.State){
		"contradictory child age": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Profiles[0].Age = 12
			s.Knowledge[0].Visual.Age = 12
		},
		"changed role": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Profiles[0].Role = "已离职"
		},
		"unreviewed":     func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) { a.Reviewed = false },
		"no rule":        func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) { a.Rule = nil },
		"no observation": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) { s.Knowledge = nil },
		"retracted": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].RetractedBy = "later"
		},
		"superseded": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].SupersededBy = "later"
		},
		"wrong branch": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Exchanges[0].BranchID = "unrelated-source"
		},
		"wrong story": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			c.Snapshot.StoryID = "other"
		},
		"wrong holder": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].CharacterID = "another"
		},
		"missing exchange": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) { s.Exchanges = nil },
		"different exchange time": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Exchanges[0].WorldTime = "1992年1月1日"
		},
		"private source": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Exchanges[0].Channel = character.ChannelPrivate
		},
		"fabricated provenance": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].FormalCommitID = "invented"
		},
		"identity unknown": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) { s.Profiles[0].KnownAs = "" },
		"changed age":      func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) { s.Profiles[0].Age = 50 },
		"later year": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			c.Snapshot.State["actors"].(map[string]any)["story"].(map[string]any)["state"].(map[string]any)["当前时间"] = "1992年9月1日"
		},
		"changed form": func(a *media.StoryAsset, c *interactive.StoryContext, s *character.State) {
			c.Snapshot.State["actors"].(map[string]any)["sinistra"].(map[string]any)["state"].(map[string]any)["外貌与装扮"] = "变形后的猫"
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			a, c, s := storyMediaGateFixture(t)
			change(&a, &c, &s)
			if storyAssetAllowed(a, c, s, "sinistra") {
				t.Fatal("unsafe picture authorized")
			}
		})
	}
}
