package app

import (
	"encoding/json"
	"fmt"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/media"
	"testing"
)

// These are synthetic gate fixtures, not approved image mappings or saved game
// facts. The three real manifest assets remain unreviewed and have no rule.
func storyAuthoredLocationFixture(t *testing.T, sourceKey, name, role, location string) (media.StoryAsset, interactive.StoryContext, character.State) {
	t.Helper()
	p := character.Profile{ID: "subject", StoryID: "st", Name: name, KnownAs: name, SourceKey: sourceKey, Role: role, Lifeform: "human", Gender: "female", Revision: 1}
	appearance := "人类女性，穿着普通长袍"
	scene := character.Scene{NodeID: "turn", WorldTime: "1991年9月1日", Location: location, Present: []string{"protagonist", p.ID}}
	facts := character.TurnCharacterFacts{Exchanges: []character.ExchangeDraft{
		{CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "你好。"},
		{CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "我是" + name + "，" + role + "。"},
	}, Knowledge: []character.KnowledgeDraft{{CharacterID: "protagonist", Statement: "亲眼看见并认识她。", Nature: character.NatureExperienced, Channel: character.ChannelScene, Visual: &character.VisualObservationDraft{SubjectID: p.ID, Form: "人类女性", ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}}}
	n := 0
	projected, err := character.Project(character.ProjectionContext{StoryID: "st", BranchID: "main", TurnID: "turn", CommitID: "commit", WorldTime: scene.WorldTime, Scene: scene, KnownCharacters: []string{p.ID}, VisualContext: &character.VisualObservationContext{ObserverID: "protagonist", Subjects: map[string]character.VisualSubject{p.ID: {Profile: p, Appearance: appearance}}}, NextID: func(prefix string) string { n++; return fmt.Sprintf("%s%d", prefix, n) }}, facts)
	if err != nil {
		t.Fatal(err)
	}
	state := character.State{Profiles: []character.Profile{p}, Exchanges: projected.Exchanges, Knowledge: projected.Knowledge}
	ctx := interactive.StoryContext{Meta: interactive.StoryMeta{PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{SourceKey: p.SourceKey, Name: p.Name}}}}, Snapshot: interactive.Snapshot{StoryID: "st", BranchID: "main", CurrentTurn: &interactive.TurnEvent{ID: "turn"}, State: map[string]any{"actors": map[string]any{"story": map[string]any{"state": map[string]any{"当前时间": scene.WorldTime, "当前详细地点": scene.Location, "在场角色": []any{"protagonist", p.ID}}}, p.ID: map[string]any{"state": map[string]any{"外貌与装扮": appearance}}}}}}
	asset := media.StoryAsset{ID: "test-only-location", SourceKey: p.SourceKey, Reviewed: true, Rule: &media.StoryRule{Kind: "observed_named_portrait_at_location", Year: 1991, Form: "人类女性", Lifeform: "human", Gender: "female", KnownAs: p.KnownAs, CanonicalName: p.Name, Role: p.Role, AdultIdentity: true, Location: location}}
	return asset, ctx, state
}

func TestStoryMediaAuthoredLocationGate(t *testing.T) {
	for _, person := range []struct{ source, name, role, place string }{
		{"hp-npc-pince", "伊尔玛·平斯", "图书管理员", "霍格沃茨图书馆"},
		{"hp-npc-pomfrey", "波皮·庞弗雷", "校医", "霍格沃茨医务室"},
		{"hp-npc-malkin", "摩金夫人", "长袍商店经营者", "摩金夫人长袍专卖店"},
	} {
		t.Run(person.source, func(t *testing.T) {
			for _, scene := range []string{person.place, "霍格沃茨礼堂", person.place + "门外", "尚未到达" + person.place, person.place + "内的桌旁"} {
				t.Run(scene, func(t *testing.T) {
					a, c, s := storyAuthoredLocationFixture(t, person.source, person.name, person.role, person.place)
					// Reproduce the old blind spot: current scene and committed visual
					// agree with each other, but not with the image's authored place.
					c.Snapshot.State["actors"].(map[string]any)["story"].(map[string]any)["state"].(map[string]any)["当前详细地点"] = scene
					s.Knowledge[0].Visual.Location = scene
					if got, want := storyAssetAllowed(a, c, s, "subject"), scene == person.place; got != want {
						t.Fatalf("gate = %v, want %v for scene %q", got, want, scene)
					}
					// Explicitly removing the test-only location restriction recovers
					// baseline behavior and proves this fixture isolates the new gate.
					a.Rule.Kind, a.Rule.Location = "observed_named_portrait", ""
					if !storyAssetAllowed(a, c, s, "subject") {
						t.Fatal("legacy control unexpectedly denied")
					}
				})
			}
		})
	}
}

func TestStoryMediaAuthoredLocationPreservesOtherGates(t *testing.T) {
	changes := map[string]func(*media.StoryAsset, *interactive.StoryContext, *character.State){
		"unreviewed":     func(a *media.StoryAsset, _ *interactive.StoryContext, _ *character.State) { a.Reviewed = false },
		"no observation": func(_ *media.StoryAsset, _ *interactive.StoryContext, s *character.State) { s.Knowledge = nil },
		"retracted": func(_ *media.StoryAsset, _ *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].RetractedBy = "later"
		},
		"superseded": func(_ *media.StoryAsset, _ *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].SupersededBy = "later"
		},
		"unknown identity": func(_ *media.StoryAsset, _ *interactive.StoryContext, s *character.State) { s.Profiles[0].KnownAs = "" },
		"wrong source key": func(a *media.StoryAsset, _ *interactive.StoryContext, _ *character.State) {
			a.SourceKey = "other-person"
		},
		"wrong canonical name": func(a *media.StoryAsset, _ *interactive.StoryContext, _ *character.State) {
			a.Rule.CanonicalName = "other-person"
		},
		"wrong world story": func(_ *media.StoryAsset, c *interactive.StoryContext, _ *character.State) {
			c.Snapshot.StoryID = "other"
		},
		"changed role snapshot": func(_ *media.StoryAsset, _ *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].Visual.Role = "已离职"
		},
		"different observed place": func(_ *media.StoryAsset, _ *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].Visual.Location = "霍格沃茨礼堂"
		},
		"different year": func(a *media.StoryAsset, _ *interactive.StoryContext, _ *character.State) { a.Rule.Year = 1992 },
		"child": func(_ *media.StoryAsset, _ *interactive.StoryContext, s *character.State) {
			s.Profiles[0].Age = 12
			s.Knowledge[0].Visual.Age = 12
		},
		"different form": func(a *media.StoryAsset, _ *interactive.StoryContext, _ *character.State) { a.Rule.Form = "猫" },
		"different branch source": func(_ *media.StoryAsset, _ *interactive.StoryContext, s *character.State) {
			s.Exchanges[0].BranchID = "other"
		},
		"private source": func(_ *media.StoryAsset, _ *interactive.StoryContext, s *character.State) {
			s.Exchanges[0].Channel = character.ChannelPrivate
		},
		"wrong audience": func(_ *media.StoryAsset, _ *interactive.StoryContext, s *character.State) {
			s.Exchanges[0].Audience = nil
		},
		"missing formal commit": func(_ *media.StoryAsset, _ *interactive.StoryContext, s *character.State) {
			s.Knowledge[0].FormalCommitID = ""
		},
		"unknown kind": func(a *media.StoryAsset, _ *interactive.StoryContext, _ *character.State) {
			a.Rule.Kind = "unknown_kind"
		},
		"empty exact location": func(a *media.StoryAsset, _ *interactive.StoryContext, _ *character.State) { a.Rule.Location = "" },
		"not present": func(_ *media.StoryAsset, c *interactive.StoryContext, _ *character.State) {
			c.Snapshot.State["actors"].(map[string]any)["story"].(map[string]any)["state"].(map[string]any)["在场角色"] = []any{"protagonist"}
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			a, c, s := storyAuthoredLocationFixture(t, "hp-npc-pince", "伊尔玛·平斯", "图书管理员", "霍格沃茨图书馆")
			if !storyAssetAllowed(a, c, s, "subject") {
				t.Fatal("positive fixture denied")
			}
			change(&a, &c, &s)
			before, err := json.Marshal([]any{a, c, s})
			if err != nil {
				t.Fatal(err)
			}
			if storyAssetAllowed(a, c, s, "subject") {
				t.Fatal("authored location bypassed another gate")
			}
			after, err := json.Marshal([]any{a, c, s})
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("media authorization changed game facts")
			}
		})
	}
}
