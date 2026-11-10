package app

import (
	"fmt"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/media"
	"testing"
)

func doupoCurrentObservationFixture0(t *testing.T) (media.StoryAsset, interactive.StoryContext, character.State) {
	t.Helper()
	p := character.Profile{ID: "sinistra", StoryID: "st", Name: "法犸", KnownAs: "法犸", SourceKey: "dp-fa-ma", Role: "加玛帝国炼药师公会会长，关心本国炼药人才和公会事务。", Lifeform: "human", Gender: "male", Revision: 1}
	appearance := "老年人类男性，正常衣着"
	scene := character.Scene{NodeID: "turn", WorldTime: "本局第1日清晨", Location: "已提交的真实会面地点", Present: []string{"protagonist", "sinistra"}}
	n := 0
	facts := character.TurnCharacterFacts{Exchanges: []character.ExchangeDraft{{CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "晚上好。"}, {CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "我是法犸，加玛帝国炼药师公会会长。"}}, Knowledge: []character.KnowledgeDraft{{CharacterID: "protagonist", Statement: "亲眼看见并认识他。", Nature: character.NatureExperienced, Channel: character.ChannelScene, Visual: &character.VisualObservationDraft{SubjectID: p.ID, Form: "老年人类男性", ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}}}
	projected, err := character.Project(character.ProjectionContext{StoryID: "st", BranchID: "main", TurnID: "turn", CommitID: "commit", WorldTime: scene.WorldTime, Scene: scene, KnownCharacters: []string{p.ID}, VisualContext: &character.VisualObservationContext{ObserverID: "protagonist", Subjects: map[string]character.VisualSubject{p.ID: {Profile: p, Appearance: appearance}}}, NextID: func(prefix string) string { n++; return fmt.Sprintf("%s%d", prefix, n) }}, facts)
	if err != nil {
		t.Fatal(err)
	}
	state := character.State{Profiles: []character.Profile{p}, Exchanges: projected.Exchanges, Knowledge: projected.Knowledge}
	ctx := interactive.StoryContext{Meta: interactive.StoryMeta{PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{SourceKey: p.SourceKey, Name: p.Name}}}}, Snapshot: interactive.Snapshot{StoryID: "st", BranchID: "main", CurrentTurn: &interactive.TurnEvent{ID: "turn"}, State: map[string]any{"actors": map[string]any{"story": map[string]any{"state": map[string]any{"当前时间": scene.WorldTime, "当前详细地点": scene.Location, "在场角色": []any{"protagonist", p.ID}}}, p.ID: map[string]any{"state": map[string]any{"外貌与装扮": appearance}}}}}}
	asset := media.StoryAsset{ID: "p11o-art-hp-person-sinistra-v1", SourceKey: p.SourceKey, Reviewed: true, Rule: &media.StoryRule{Kind: "observed_named_portrait", Year: 0, TimeSemantics: "current_observation", Form: "老年人类男性", Lifeform: "human", Gender: "male", KnownAs: p.KnownAs, CanonicalName: p.Name, Role: p.Role, RoleRecognition: "加玛帝国炼药师公会会长", AdultIdentity: false}}
	return asset, ctx, state
}

func TestDoupoCurrentObservationMapped0(t *testing.T) {
	a, c, s := doupoCurrentObservationFixture0(t)
	if !storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("current source denied")
	}
	a.Rule.Form = "年轻人类男性"
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("young form accepted for old portrait")
	}
	a, c, s = doupoCurrentObservationFixture0(t)
	s.Knowledge[0].Visual.Form = "未知形态"
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("unknown form accepted")
	}
	a, c, s = doupoCurrentObservationFixture0(t)
	a.Rule.TimeSemantics = ""
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("zero year enabled without explicit time semantics")
	}
}
func doupoCurrentObservationFixture1(t *testing.T) (media.StoryAsset, interactive.StoryContext, character.State) {
	t.Helper()
	p := character.Profile{ID: "sinistra", StoryID: "st", Name: "苏千", KnownAs: "苏千", SourceKey: "dp-su-qian", Role: "迦南学院内院大长老，负责学院安全及内院事务。", Lifeform: "human", Gender: "male", Revision: 1}
	appearance := "老年人类男性，正常衣着"
	scene := character.Scene{NodeID: "turn", WorldTime: "本局第1日清晨", Location: "已提交的真实会面地点", Present: []string{"protagonist", "sinistra"}}
	n := 0
	facts := character.TurnCharacterFacts{Exchanges: []character.ExchangeDraft{{CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "晚上好。"}, {CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "我是苏千，迦南学院内院大长老。"}}, Knowledge: []character.KnowledgeDraft{{CharacterID: "protagonist", Statement: "亲眼看见并认识他。", Nature: character.NatureExperienced, Channel: character.ChannelScene, Visual: &character.VisualObservationDraft{SubjectID: p.ID, Form: "老年人类男性", ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}}}
	projected, err := character.Project(character.ProjectionContext{StoryID: "st", BranchID: "main", TurnID: "turn", CommitID: "commit", WorldTime: scene.WorldTime, Scene: scene, KnownCharacters: []string{p.ID}, VisualContext: &character.VisualObservationContext{ObserverID: "protagonist", Subjects: map[string]character.VisualSubject{p.ID: {Profile: p, Appearance: appearance}}}, NextID: func(prefix string) string { n++; return fmt.Sprintf("%s%d", prefix, n) }}, facts)
	if err != nil {
		t.Fatal(err)
	}
	state := character.State{Profiles: []character.Profile{p}, Exchanges: projected.Exchanges, Knowledge: projected.Knowledge}
	ctx := interactive.StoryContext{Meta: interactive.StoryMeta{PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{SourceKey: p.SourceKey, Name: p.Name}}}}, Snapshot: interactive.Snapshot{StoryID: "st", BranchID: "main", CurrentTurn: &interactive.TurnEvent{ID: "turn"}, State: map[string]any{"actors": map[string]any{"story": map[string]any{"state": map[string]any{"当前时间": scene.WorldTime, "当前详细地点": scene.Location, "在场角色": []any{"protagonist", p.ID}}}, p.ID: map[string]any{"state": map[string]any{"外貌与装扮": appearance}}}}}}
	asset := media.StoryAsset{ID: "p11o-art-hp-person-sinistra-v1", SourceKey: p.SourceKey, Reviewed: true, Rule: &media.StoryRule{Kind: "observed_named_portrait", Year: 0, TimeSemantics: "current_observation", Form: "老年人类男性", Lifeform: "human", Gender: "male", KnownAs: p.KnownAs, CanonicalName: p.Name, Role: p.Role, RoleRecognition: "迦南学院内院大长老", AdultIdentity: false}}
	return asset, ctx, state
}

func TestDoupoCurrentObservationMapped1(t *testing.T) {
	a, c, s := doupoCurrentObservationFixture1(t)
	if !storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("current source denied")
	}
	a.Rule.Form = "年轻人类男性"
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("young form accepted for old portrait")
	}
	a, c, s = doupoCurrentObservationFixture1(t)
	s.Knowledge[0].Visual.Form = "未知形态"
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("unknown form accepted")
	}
	a, c, s = doupoCurrentObservationFixture1(t)
	a.Rule.TimeSemantics = ""
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("zero year enabled without explicit time semantics")
	}
}
