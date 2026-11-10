package app

import (
	"encoding/json"
	"reflect"
	"testing"

	"ta/internal/interactive"
	"ta/internal/storypack"
)

func TestLearnedRecipeDetailsNameOnlyItsReferencesWithoutGrantingInventory(t *testing.T) {
	pack, err := storypack.NewLibrary(t.TempDir()).Get("xinghai-689")
	if err != nil {
		t.Fatal(err)
	}
	rules := *pack.Story.Gameplay.AdventureRules
	actor := interactive.NewAdventureActor(rules)
	actor.Rank = 4
	actor.Skills["xinghai-s29"] = 1
	definition := interactive.AdventureGeneratedDefinition{Category: "craft", TemplateID: "xinghai-decode-hopping", Name: "听潮解码法", Source: "已学信号分析的改良笔记"}
	actor, _, err = interactive.ResolveAdventureAction(rules, actor, interactive.AdventureAction{Action: "generate", Generate: &definition}, "learned-recipe-fixture")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(actor)
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	system := interactive.StoryDirectorActorStateSystem{AdventureRules: &rules,
		Templates:     []interactive.ActorStateTemplate{{ID: interactive.DefaultActorID, Name: "Player", Fields: interactive.AdventureStateFields(rules)}},
		InitialActors: []interactive.ActorStateInitialActor{{ID: interactive.DefaultActorID, Name: "Player", TemplateID: interactive.DefaultActorID, State: map[string]any{interactive.AdventureStateField: value}}}}
	store := interactive.NewStore(t.TempDir())
	defer store.Close()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Read-only recipe fixture", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	application := &App{interactive: store}
	before, err := store.AdventureWindow(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	window, err := application.AdventureWindow(story.ID, "main", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	var learned *AdventureWindowCraftMethod
	for i := range window.CraftMethods {
		if window.CraftMethods[i].Name == "听潮解码法" {
			learned = &window.CraftMethods[i]
		}
	}
	if learned == nil {
		t.Fatalf("learned method missing: %+v", window.CraftMethods)
	}
	raw, _ = json.Marshal(learned)
	var recipe struct {
		ItemNames map[string]string `json:"item_names"`
		SkillName string            `json:"skill_name"`
	}
	if err := json.Unmarshal(raw, &recipe); err != nil {
		t.Fatal(err)
	}
	wantNames := map[string]string{}
	for _, item := range rules.Items {
		if item.ID == "xinghai-hopping-recording" || item.ID == "xinghai-signal-fragments" || item.ID == "xinghai-signal-processor" {
			wantNames[item.ID] = item.Name
		}
	}
	if !reflect.DeepEqual(recipe.ItemNames, wantNames) || recipe.SkillName == "" {
		t.Fatalf("reference names missing or overbroad: %+v, want %+v", recipe, wantNames)
	}
	for _, item := range window.Items {
		if _, referenced := wantNames[item.ID]; referenced {
			t.Fatalf("recipe reference became an owned item: %s", item.ID)
		}
	}
	after, err := store.AdventureWindow(story.ID, "main")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("read-only names changed canonical state", err)
	}
}
