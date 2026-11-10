package interactive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// These fixtures keep the real package revision and cast. Arrival is explicit,
// but no unbound-story exemption can hide a missing content registration.
func fantasyBoundStory(t *testing.T, r AdventureRules, a AdventureActor, location string) (*Store, string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "storypack", "assets", "sui-guan-realms.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog PackCatalogSnapshot
	if err = json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, person := range catalog.Characters {
		catalog.Places = append(catalog.Places, PackCatalogPlace{Token: person.PlaceLabel, Country: person.Country, Place: person.Country + "\x00" + person.PlaceCandidate + "\x00" + person.PlaceLabel})
	}
	values := map[string]any{}
	for _, op := range adventureActorOps(a, "fixture") {
		values[op.FieldID] = op.Value
	}
	system := StoryDirectorActorStateSystem{WorldContent: r.world, AdventureRules: &r, Templates: []ActorStateTemplate{
		{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(r)},
		{ID: DefaultStoryContextActorID, Name: "场景", Fields: []ActorStateField{{Name: storySceneLocationField, Type: "string", Default: location}}},
	}, InitialActors: []ActorStateInitialActor{
		{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: values},
		{ID: DefaultStoryContextActorID, Name: "场景", TemplateID: DefaultStoryContextActorID, State: map[string]any{storySceneLocationField: location}},
	}}
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "Shattered Crown authored fixture", ActorState: &system, PackBinding: &StoryPackBinding{PackID: "sui-guan-realms", ContentVersion: 1, Scope: "sui-guan-realms", Gameplay: &StoryGameplayProfile{StatusTemplate: "fantasy-state-v1", Growth: "fantasy", Combat: "fantasy", Communication: "letter", LetterDeliverySeconds: 172800, Opening: "scene"}}, PackCatalog: &catalog})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "观察当前位置", Narrative: "你确认眼前的地点和随身物品。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// Establish a real authored identity in the committed scene, rather than
// assuming the world book or a location silently made that person present.
func fantasyMeet(t *testing.T, s *Store, id, branch, source, actorID string) {
	t.Helper()
	name := ""
	for _, person := range fantasyCatalog(t).Characters {
		if person.SourceKey == source {
			name = person.Name
		}
	}
	if name == "" {
		t.Fatal("unknown fixture source", source)
	}
	_, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: branch, User: "当面拜访" + name, Narrative: name + "与你当面见面。", ActorOps: []ActorStateOp{{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{actorID}}}, TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{CharacterID: actorID, Name: name, SourceKey: source}}}}})
	if err != nil {
		t.Fatal(err)
	}
}
