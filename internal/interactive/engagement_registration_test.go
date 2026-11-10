package interactive

import (
	"testing"

	interactivestate "ta/internal/interactive/state"
)

func TestEngagementRequiresItsOwnRulesActorAndAcceptsSameTurnRegistration(t *testing.T) {
	system := combatTestSystem()
	for i := range system.Templates {
		if system.Templates[i].ID == ActorStateStoryContextTemplateID {
			system.Templates[i].Fields = append(system.Templates[i].Fields,
				objectStateFieldWithInstruction("scene.engagement", storySceneEngagementField, engagementFieldDescription, "当前场景", engagementFieldInstruction),
				listStateField("scene.present_actors", storyScenePresentField, "Present people", "当前场景"))
		}
	}
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "山路交战", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	state := combatTestState(t, store, story.ID)
	updates := []interactivestate.Update{
		{Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{"protagonist", "杜横"}},
		{Op: interactivestate.Replace, Path: "/story/本次交战", Value: map[string]any{"杜横": "正在挥刀攻击主角"}},
	}
	choices := testTurnChoices()
	ctx := TurnSubmissionContext{ActorState: system, CurrentState: state, ChoiceCount: len(choices)}
	prepared, receipt := PrepareTurnSubmission(ctx, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &choices})
	if receipt.Ready || receipt.ModuleStatus.StateChanges != TurnSubmissionModuleRejected {
		t.Fatalf("a prose-only opponent cannot open a rules battle: %#v", receipt)
	}
	updates = append(updates, interactivestate.Update{Op: interactivestate.Create, Path: "/杜横", Value: map[string]any{
		"template_id": "rival", "name": "杜横", "state": map[string]any{xiuxianFieldRealm: "qi_12"},
	}})
	_, receipt = PrepareTurnSubmission(ctx, prepared, TurnSubmissionInput{StateUpdates: &updates})
	if !receipt.Ready {
		t.Fatalf("same-turn registration must repair the rejected module: %#v", receipt)
	}
}
