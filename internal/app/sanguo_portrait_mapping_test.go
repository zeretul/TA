package app

import (
	"fmt"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/media"
	"testing"
)

func sanguoPortraitFixture0(t *testing.T) (media.StoryAsset, interactive.StoryContext, character.State) {
	t.Helper()
	p := character.Profile{ID: "sinistra", StoryID: "st", Name: "曹操", KnownAs: "曹操", SourceKey: "sanguo-person-caocao", Role: "汉末人物", Lifeform: "human", Gender: "male", Revision: 1}
	appearance := "人类男性，穿普通衣甲"
	scene := character.Scene{NodeID: "turn", WorldTime: "190年9月1日", Location: "已提交的合法会面地点", Present: []string{"protagonist", "sinistra"}}
	n := 0
	facts := character.TurnCharacterFacts{Exchanges: []character.ExchangeDraft{{CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "晚上好。"}, {CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "我叫曹操。"}}, Knowledge: []character.KnowledgeDraft{{CharacterID: "protagonist", Statement: "亲眼看见并认识她。", Nature: character.NatureExperienced, Channel: character.ChannelScene, Visual: &character.VisualObservationDraft{SubjectID: p.ID, Form: "人类男性", ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}}}
	projected, err := character.Project(character.ProjectionContext{StoryID: "st", BranchID: "main", TurnID: "turn", CommitID: "commit", WorldTime: scene.WorldTime, Scene: scene, KnownCharacters: []string{p.ID}, VisualContext: &character.VisualObservationContext{ObserverID: "protagonist", Subjects: map[string]character.VisualSubject{p.ID: {Profile: p, Appearance: appearance}}}, NextID: func(prefix string) string { n++; return fmt.Sprintf("%s%d", prefix, n) }}, facts)
	if err != nil {
		t.Fatal(err)
	}
	state := character.State{Profiles: []character.Profile{p}, Exchanges: projected.Exchanges, Knowledge: projected.Knowledge}
	ctx := interactive.StoryContext{Meta: interactive.StoryMeta{PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{SourceKey: p.SourceKey, Name: p.Name}}}}, Snapshot: interactive.Snapshot{StoryID: "st", BranchID: "main", CurrentTurn: &interactive.TurnEvent{ID: "turn"}, State: map[string]any{"actors": map[string]any{"story": map[string]any{"state": map[string]any{"当前时间": scene.WorldTime, "当前详细地点": scene.Location, "在场角色": []any{"protagonist", p.ID}}}, p.ID: map[string]any{"state": map[string]any{"外貌与装扮": appearance}}}}}}
	asset := media.StoryAsset{ID: "p11o-art-hp-person-sinistra-v1", SourceKey: p.SourceKey, Reviewed: true, Rule: &media.StoryRule{Kind: "observed_named_portrait", RoleSemantics: "background_classification", Year: 190, Form: "人类男性", Lifeform: "human", Gender: "male", KnownAs: p.KnownAs, CanonicalName: p.Name, Role: p.Role, AdultIdentity: true}}
	return asset, ctx, state
}

func TestSanguoPortraitCandidate0(t *testing.T) {
	a, c, s := sanguoPortraitFixture0(t)
	if !storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("formal synthetic 190 source denied")
	}
	s.Knowledge[0].RetractedBy = "retracted"
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("retracted knowledge allowed")
	}
	a, c, s = sanguoPortraitFixture0(t)
	a.Rule.Year = 191
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("other year allowed")
	}
	a, c, s = sanguoPortraitFixture0(t)
	s.Profiles[0].Role = "未来王号"
	// A committed profile change advances its revision; the prior observation
	// must not authorize the character's later identity.
	s.Profiles[0].Revision++
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("future role allowed")
	}
}
func sanguoPortraitFixture1(t *testing.T) (media.StoryAsset, interactive.StoryContext, character.State) {
	t.Helper()
	p := character.Profile{ID: "sinistra", StoryID: "st", Name: "刘备", KnownAs: "刘备", SourceKey: "sanguo-person-liubei", Role: "汉末人物", Lifeform: "human", Gender: "male", Revision: 1}
	appearance := "人类男性，穿普通衣甲"
	scene := character.Scene{NodeID: "turn", WorldTime: "190年9月1日", Location: "已提交的合法会面地点", Present: []string{"protagonist", "sinistra"}}
	n := 0
	facts := character.TurnCharacterFacts{Exchanges: []character.ExchangeDraft{{CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "晚上好。"}, {CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "我叫刘备。"}}, Knowledge: []character.KnowledgeDraft{{CharacterID: "protagonist", Statement: "亲眼看见并认识她。", Nature: character.NatureExperienced, Channel: character.ChannelScene, Visual: &character.VisualObservationDraft{SubjectID: p.ID, Form: "人类男性", ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}}}
	projected, err := character.Project(character.ProjectionContext{StoryID: "st", BranchID: "main", TurnID: "turn", CommitID: "commit", WorldTime: scene.WorldTime, Scene: scene, KnownCharacters: []string{p.ID}, VisualContext: &character.VisualObservationContext{ObserverID: "protagonist", Subjects: map[string]character.VisualSubject{p.ID: {Profile: p, Appearance: appearance}}}, NextID: func(prefix string) string { n++; return fmt.Sprintf("%s%d", prefix, n) }}, facts)
	if err != nil {
		t.Fatal(err)
	}
	state := character.State{Profiles: []character.Profile{p}, Exchanges: projected.Exchanges, Knowledge: projected.Knowledge}
	ctx := interactive.StoryContext{Meta: interactive.StoryMeta{PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{SourceKey: p.SourceKey, Name: p.Name}}}}, Snapshot: interactive.Snapshot{StoryID: "st", BranchID: "main", CurrentTurn: &interactive.TurnEvent{ID: "turn"}, State: map[string]any{"actors": map[string]any{"story": map[string]any{"state": map[string]any{"当前时间": scene.WorldTime, "当前详细地点": scene.Location, "在场角色": []any{"protagonist", p.ID}}}, p.ID: map[string]any{"state": map[string]any{"外貌与装扮": appearance}}}}}}
	asset := media.StoryAsset{ID: "p11o-art-hp-person-sinistra-v1", SourceKey: p.SourceKey, Reviewed: true, Rule: &media.StoryRule{Kind: "observed_named_portrait", RoleSemantics: "background_classification", Year: 190, Form: "人类男性", Lifeform: "human", Gender: "male", KnownAs: p.KnownAs, CanonicalName: p.Name, Role: p.Role, AdultIdentity: true}}
	return asset, ctx, state
}

func TestSanguoPortraitCandidate1(t *testing.T) {
	a, c, s := sanguoPortraitFixture1(t)
	if !storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("formal synthetic 190 source denied")
	}
	s.Knowledge[0].RetractedBy = "retracted"
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("retracted knowledge allowed")
	}
	a, c, s = sanguoPortraitFixture1(t)
	a.Rule.Year = 191
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("other year allowed")
	}
	a, c, s = sanguoPortraitFixture1(t)
	s.Profiles[0].Role = "未来王号"
	s.Profiles[0].Revision++
	if storyAssetAllowed(a, c, s, "sinistra") {
		t.Fatal("future role allowed")
	}
}
