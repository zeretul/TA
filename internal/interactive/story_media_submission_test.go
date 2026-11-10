package interactive

import (
	"ta/internal/character"
	"testing"
)

func visualSubmissionFixture() (*TurnCharacterFactsInput, TurnSubmissionContext, *PreparedTurnSubmission) {
	name := "奥罗拉·辛尼斯塔"
	p := character.Profile{ID: "sinistra", StoryID: "story", Name: name, KnownAs: name, SourceKey: "hp-npc-sinistra", Role: "天文学教授", Lifeform: "human", Gender: "female", Revision: 1, Description: "当面相识的天文学教授。"}
	values := map[string]any{"actors": map[string]any{"story": map[string]any{"id": "story", "state": map[string]any{"当前时间": "1991年9月1日", "当前详细地点": "天文塔", "在场角色": []any{"protagonist", "sinistra"}}}, "sinistra": map[string]any{"id": "sinistra", "name": name, "state": map[string]any{"外貌与装扮": "人类女性，深色教师长袍"}}}}
	facts := &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{CharacterID: "sinistra", Speaker: "character", Channel: "scene", AddressedTo: "protagonist", Text: "晚上好。"}, {CharacterID: "sinistra", Speaker: "character", Channel: "scene", AddressedTo: "protagonist", Text: "我是奥罗拉·辛尼斯塔，天文学教授。"}}, Knowledge: []TurnCharacterKnowledgeInput{{CharacterID: "protagonist", Nature: "experienced", Channel: "scene", Statement: "已当面看见并听她介绍姓名。", Visual: &character.VisualObservationDraft{SubjectID: "sinistra", Form: "人类女性", ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}}}
	v := TurnSubmissionContext{CurrentState: values, KnownCharacters: []string{"sinistra"}, ExistingProfiles: []character.Profile{p}, Scene: StorySceneFromState(values, "prior"), Meta: StoryMeta{StoryID: "story", PackCatalog: &PackCatalogSnapshot{Characters: []PackCatalogPerson{{SourceKey: p.SourceKey, Name: name, Lifeform: "human", Gender: "female"}}}}, BranchID: "main"}
	return facts, v, &PreparedTurnSubmission{stateUpdatesAccepted: true}
}

func TestVisualKnowledgeUsesFormalSubmissionConsumer(t *testing.T) {
	f, v, p := visualSubmissionFixture()
	if d := validateTurnCharacterFactsWithVisuals(f, v, p, 0); d != nil {
		t.Fatalf("formal submission rejected: %+v", d)
	}
	if f.Knowledge[0].Visual == nil {
		t.Fatal("preflight mutated caller's visual fact")
	}
	// The full submit_turn module path must call this validator, not retain the
	// old sentinel-only validator which rejects every real observation.
	input := TurnSubmissionInput{CharacterFacts: f}
	prepared, receipt := PrepareTurnSubmission(v, p, input)
	if prepared.result.CharacterFacts == nil {
		t.Fatalf("submit_turn did not retain visual module: %+v", receipt)
	}
}
func TestVisualKnowledgeSubmissionFailsWithoutActualSources(t *testing.T) {
	cases := map[string]func(*TurnCharacterFactsInput, *TurnSubmissionContext, *PreparedTurnSubmission){
		"unaccepted state": func(f *TurnCharacterFactsInput, v *TurnSubmissionContext, p *PreparedTurnSubmission) {
			p.stateUpdatesAccepted = false
		},
		"missing naming": func(f *TurnCharacterFactsInput, v *TurnSubmissionContext, p *PreparedTurnSubmission) {
			f.Exchanges[1].Text = "你好。"
		},
		"remote contact": func(f *TurnCharacterFactsInput, v *TurnSubmissionContext, p *PreparedTurnSubmission) {
			f.Exchanges[0].Channel = "private"
		},
		"wrong observer": func(f *TurnCharacterFactsInput, v *TurnSubmissionContext, p *PreparedTurnSubmission) {
			f.Knowledge[0].CharacterID = "sinistra"
		},
		"missing appearance": func(f *TurnCharacterFactsInput, v *TurnSubmissionContext, p *PreparedTurnSubmission) {
			delete(v.CurrentState["actors"].(map[string]any)["sinistra"].(map[string]any)["state"].(map[string]any), "外貌与装扮")
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f, v, p := visualSubmissionFixture()
			change(f, &v, p)
			if d := validateTurnCharacterFactsWithVisuals(f, v, p, 0); d == nil {
				t.Fatal("unsupported fact accepted")
			}
		})
	}
}

func TestVisualKnowledgeRetractionUsesSameFormalTool(t *testing.T) {
	_, v, p := visualSubmissionFixture()
	retract := &TurnCharacterFactsInput{Knowledge: []TurnCharacterKnowledgeInput{{CharacterID: DefaultActorID, Statement: "先前观察判断有误，不再使用。", Nature: "experienced", Channel: "scene", Retracts: "previous-visual-record"}}}
	if d := validateTurnCharacterFactsWithVisuals(retract, v, p, 0); d != nil {
		t.Fatalf("real tool cannot retract its observation: %+v", d)
	}
}
