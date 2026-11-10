package interactive

import "testing"

// A deliberately qualified isolated solver fixture, not earned/model gameplay.
// Normal acquisition campaigns live in internal/app and keep authored origins.
func adventureFixtureStoryWithActor(t *testing.T, r AdventureRules, a AdventureActor, location string) (*Store, string) {
	t.Helper()
	values := map[string]any{}
	for _, op := range adventureActorOps(a, "fixture") {
		values[op.FieldID] = op.Value
	}
	system := StoryDirectorActorStateSystem{WorldContent: r.world, AdventureRules: &r, Templates: []ActorStateTemplate{{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(r)}, {ID: DefaultStoryContextActorID, Name: "场景", Fields: []ActorStateField{{Name: storySceneLocationField, Type: "string", Default: location}}}}, InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: values}, {ID: DefaultStoryContextActorID, Name: "场景", TemplateID: DefaultStoryContextActorID, State: map[string]any{storySceneLocationField: location}}}}
	s := NewStore(t.TempDir())
	story, e := s.CreateStory(CreateStoryRequest{Title: "Qualified source solver fixture", ActorState: &system})
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "观察当前情形", Narrative: "你确认了自己的物品与所在位置。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); e != nil {
		t.Fatal(e)
	}
	return s, story.ID
}
