package interactive

import (
	"errors"
	"testing"
)

func TestAdventureWindowAndCommitRequireMatchingFrozenFamily(t *testing.T) {
	rules := AdventureRules{Family: "wuxia", Currency: "铜钱"}
	system := StoryDirectorActorStateSystem{
		WorldContent: rules.world, AdventureRules: &rules,
		Templates:     []ActorStateTemplate{{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(rules)}},
		InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID}},
	}
	for _, declared := range []string{"none", "cultivation", "space", "fantasy", "unknown", "wuxia", "legacy"} {
		t.Run(declared, func(t *testing.T) {
			store := NewStore(t.TempDir())
			t.Cleanup(func() { _ = store.Close() })
			binding := &StoryPackBinding{PackID: "test", Scope: "pack:test"}
			if declared != "legacy" {
				binding.Gameplay = &StoryGameplayProfile{StatusTemplate: "wuxia-state-v1", Growth: "wuxia", Combat: declared, Opening: "scene"}
			}
			story, err := store.CreateStory(CreateStoryRequest{Title: "Family boundary", ActorState: &system, PackBinding: binding})
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.AdventureWindow(story.ID, "")
			if declared == "wuxia" || declared == "legacy" {
				if err != nil {
					t.Fatalf("valid native world became unavailable: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrAdventureUnavailableForWorld) {
				t.Fatalf("adventure read did not reject the incompatible declaration: %v", err)
			}
			// Direct callers must be refused by the store too, not only by the UI.
			ctx, err := store.StoryContext(story.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			before := ctx.Snapshot.ContextRevision
			_, err = store.CommitAdventureAction(story.ID, "main", "wrong-family", ctx.Meta.Branches["main"].Head, AdventureAction{Action: "battle_round"})
			if !errors.Is(err, ErrAdventureUnavailableForWorld) {
				t.Fatalf("adventure commit did not reject the incompatible declaration: %v", err)
			}
			after, err := store.StoryContext(story.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			if after.Snapshot.ContextRevision != before {
				t.Fatal("refused action changed the story")
			}
		})
	}
}
