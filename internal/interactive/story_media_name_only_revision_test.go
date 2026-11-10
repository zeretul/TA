package interactive

import (
	"encoding/json"
	"ta/internal/character"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/media"
	"testing"
)

func TestVisualNameOnlyRevisionSurvivesFormalCommitAndColdRead(t *testing.T) {
	root := t.TempDir()
	packID, packHash := "test-world", "test-native-hash"
	asset := media.StoryAsset{SourceKey: "hp-npc-sinistra", Rule: &media.StoryRule{KnownAs: "奥罗拉·辛尼斯塔", CanonicalName: "奥罗拉·辛尼斯塔", Role: "背景人物"}}
	store := NewStore(root)
	story, err := store.CreateStory(CreateStoryRequest{Title: "媒体门禁确定性验收", PackBinding: &StoryPackBinding{PackID: packID, ContentHash: packHash, Scope: "media-test"}, PackCatalog: &PackCatalogSnapshot{Characters: []PackCatalogPerson{{SourceKey: asset.SourceKey, Name: asset.Rule.CanonicalName, Role: asset.Rule.Role, Gender: "female", Lifeform: "human"}}}, InitialStateOps: []interactivestate.Op{
		{Op: "set", Path: "actors.story.id", Value: "story"}, {Op: "set", Path: "actors.story.state.当前时间", Value: "1991年9月1日"}, {Op: "set", Path: "actors.story.state.当前详细地点", Value: "天文塔"}, {Op: "set", Path: "actors.story.state.在场角色", Value: []any{"protagonist", "sinistra"}},
		{Op: "set", Path: "actors.sinistra.id", Value: "sinistra"}, {Op: "set", Path: "actors.sinistra.name", Value: asset.Rule.KnownAs}, {Op: "set", Path: "actors.sinistra.state.外貌与装扮", Value: "人类女性，穿深色教师长袍"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "向眼前教授问好并询问称呼", Narrative: "你当面看清她的外貌，听见她向你介绍自己。", TurnResult: &TurnResult{Choices: []string{"继续交谈", "观察星空", "询问课程", "安静等待", "告别离开"}, CharacterFacts: &TurnCharacterFactsInput{
		Profiles:  []TurnCharacterProfileInput{{CharacterID: "sinistra", Name: asset.Rule.CanonicalName, KnownAs: asset.Rule.KnownAs, SourceKey: asset.SourceKey, Role: asset.Rule.Role, Lifeform: "human", Gender: "female", Description: "眼前穿深色教师长袍的人类女性教授。"}},
		Exchanges: []TurnCharacterExchangeInput{{CharacterID: "sinistra", Speaker: "character", Channel: "scene", AddressedTo: "protagonist", Text: "晚上好。"}, {CharacterID: "sinistra", Speaker: "character", Channel: "scene", AddressedTo: "protagonist", Text: "我是" + asset.Rule.KnownAs + "。"}},
		Knowledge: []TurnCharacterKnowledgeInput{{CharacterID: "protagonist", Statement: "亲眼看清她的人类女性形态，听她介绍姓名。", Nature: "experienced", Channel: "scene", Visual: &character.VisualObservationDraft{SubjectID: "sinistra", Form: "人类女性", ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}},
	}}})
	if err != nil {
		t.Fatal(err)
	}

	// Reconstruct from the durable store, not the preflight projection.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cold := NewStore(root)
	defer cold.Close()
	_, state, err := cold.StoryMediaSnapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := state.Profile("sinistra")
	if !ok {
		t.Fatal("committed profile missing")
	}
	var observation *character.VisualObservation
	for _, k := range state.Knowledge {
		if k.Visual != nil {
			observation = k.Visual
			break
		}
	}
	if observation == nil || observation.Role != "" || observation.ProfileRevision != p.Revision || !character.VisualRoleSnapshotMatches(*observation, p) {
		t.Fatalf("committed revision mismatch: observation=%+v profile=%+v", observation, p)
	}
	raw, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	var persisted character.VisualObservation
	if err := json.Unmarshal(raw, &persisted); err != nil || !character.VisualRoleSnapshotMatches(persisted, p) {
		t.Fatal("revision lost through persisted JSON")
	}
	// A non-role profile edit also increments revision and invalidates the old observation.
	p.Description = "新的已提交人物描述"
	if err := state.UpsertProfile(p); err != nil {
		t.Fatal(err)
	}
	updated, _ := state.Profile(p.ID)
	if updated.Role != p.Role || updated.Revision <= p.Revision || character.VisualRoleSnapshotMatches(*observation, updated) {
		t.Fatal("same-role profile update failed to invalidate observation")
	}
}
