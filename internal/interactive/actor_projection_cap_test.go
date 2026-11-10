package interactive

import (
	"fmt"
	"strings"
	"testing"
)

// TestActorProjectionKeepsFoundationalActorsPastTheCap is the NAR-09 guarantee
// at the point the model reads state. The projection is bounded, and the bound
// used to be applied to an ID-sorted list: with more than 24 Actors whose IDs
// sort before "protagonist", the protagonist, story and world Actors were the
// ones that fell off, so the turn lost the very state every submission targets.
func TestActorProjectionKeepsFoundationalActorsPastTheCap(t *testing.T) {
	system := defaultActorStateSystem()
	state := map[string]any{}
	actors := map[string]any{}
	for _, actor := range defaultActorStateInitialActors() {
		actors[actor.ID] = map[string]any{"name": actor.Name, "template_id": actor.TemplateID}
	}
	// Dynamic Actors that all sort before the foundational ones.
	for i := 0; i < maxInteractiveListItems+8; i++ {
		id := fmt.Sprintf("AAA%02d", i)
		actors[id] = map[string]any{"name": id, "template_id": ActorStateImportantCharacterTemplateID}
	}
	state[actorStateRoot] = actors

	projected := ActorStateRuntimeProjection(system, state)
	kept, _ := projected[actorStateRoot].(map[string]any)
	for _, id := range []string{DefaultActorID, DefaultStoryContextActorID, DefaultWorldEntitiesActorID} {
		if _, ok := kept[id]; !ok {
			t.Fatalf("the projection dropped the foundational Actor %q: kept %d of %d", id, len(kept), len(actors))
		}
	}
	if len(kept) != maxInteractiveListItems {
		t.Fatalf("the projection kept %d Actors, want the cap %d", len(kept), maxInteractiveListItems)
	}
	// The truncation is still reported, so an omitted Actor is never mistaken
	// for a nonexistent one.
	projection, _ := projected["projection"].(map[string]any)
	if projection["active_truncated"] != true || projection["active_total"] != len(actors) {
		t.Fatalf("projection summary = %+v", projection)
	}
}

// TestActorRuntimeContextKeepsTheSceneOnStage is the same rule applied to the
// Markdown handbook: the characters the scene places on stage are the ones this
// turn's action is about, so they survive the cap ahead of unrelated Actors.
func TestActorRuntimeContextKeepsTheSceneOnStage(t *testing.T) {
	system := defaultActorStateSystem()
	state := map[string]any{}
	actors := map[string]any{}
	for _, actor := range defaultActorStateInitialActors() {
		actors[actor.ID] = map[string]any{"name": actor.Name, "template_id": actor.TemplateID}
	}
	for i := 0; i < maxInteractiveListItems+8; i++ {
		id := fmt.Sprintf("AAA%02d", i)
		actors[id] = map[string]any{"name": id, "template_id": ActorStateImportantCharacterTemplateID}
	}
	// A character who sorts last but is standing in the scene right now.
	onStage := "周掌柜"
	actors[onStage] = map[string]any{"name": onStage, "template_id": ActorStateImportantCharacterTemplateID}
	// The scene's own Actor carries the present list, nested under `state` the
	// way committed Actor state is.
	actors[DefaultStoryContextActorID] = map[string]any{
		"name": "故事状态", "template_id": ActorStateStoryContextTemplateID,
		"state": map[string]any{storyScenePresentField: []any{onStage}},
	}
	state[actorStateRoot] = actors

	context := ActorStateRuntimeContext(system, state, StoryContextMaxBytes, 3)
	if !strings.Contains(context, "`"+onStage+"`") {
		t.Fatalf("the handbook dropped a character the scene places on stage:\n%s", context)
	}
	for _, id := range []string{DefaultActorID, DefaultStoryContextActorID} {
		if !strings.Contains(context, "`"+id+"`") {
			t.Fatalf("the handbook dropped the foundational Actor %q", id)
		}
	}
}
