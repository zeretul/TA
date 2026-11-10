package app

import (
	"testing"

	"ta/config"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// TestAPackSuppliesItsDefaultVoiceAsADefaultNotALock is the acceptance check for
// the rule that a world now carries its own default voice.
//
// The old rule — a pack could never contribute a style — is superseded: a world
// declares how its new games are told, the start copies that into the story, and
// the player's own explicit choice still wins. This drives the real path — each
// shipped pack through the product's own creation defaults into the story store
// — so the page and the started story cannot disagree about the voice.
func TestAPackSuppliesItsDefaultVoiceAsADefaultNotALock(t *testing.T) {
	dataRoot := t.TempDir()
	service := &InteractiveAppService{app: &App{
		workspace: dataRoot,
		cfg:       &config.Config{DenovaDir: dataRoot},
	}}

	packs, err := storypack.NewLibrary(dataRoot).List()
	if err != nil {
		t.Fatalf("list packs: %v", err)
	}
	if len(packs) == 0 {
		t.Fatal("no packs are available to check")
	}

	for _, summary := range packs {
		pack, err := storypack.NewLibrary(dataRoot).Get(summary.ID)
		if err != nil {
			t.Fatalf("read pack %s: %v", summary.ID, err)
		}
		effective := pack.EffectiveSettings()
		req := pack.StoryRequest()
		if req.StoryTellerID == "" {
			t.Fatalf("pack %s supplied no default voice", summary.ID)
		}
		if req.StoryTellerID != effective.StylePurposes.Main {
			t.Fatalf("pack %s request style %q disagrees with its effective settings %q",
				summary.ID, req.StoryTellerID, effective.StylePurposes.Main)
		}

		req, err = service.withStoryCreationDefaults(req)
		if err != nil {
			t.Fatalf("apply creation defaults for pack %s: %v", summary.ID, err)
		}
		store := interactive.NewStore(t.TempDir())
		t.Cleanup(func() { _ = store.Close() })
		story, err := store.CreateStory(req)
		if err != nil {
			t.Fatalf("create story from pack %s: %v", summary.ID, err)
		}
		if story.StoryTellerID != effective.StylePurposes.Main {
			t.Fatalf("a story created from pack %s runs on style %q, want the pack default %q",
				summary.ID, story.StoryTellerID, effective.StylePurposes.Main)
		}
		// The frozen module graph carries a style of its own as a creation
		// default. It must agree with the story's actual style: if a pack left a
		// different id there, the tuning panel and the generation would disagree
		// about which voice the game is using.
		if refs := story.ModuleRefs; refs != nil && refs.NarrativeStyleID != "" && refs.NarrativeStyleID != story.StoryTellerID {
			t.Fatalf("pack %s left style %q in the frozen module graph while the story runs on %q",
				summary.ID, refs.NarrativeStyleID, story.StoryTellerID)
		}

		// The default is not a lock: an explicit choice for this game replaces it
		// and is what the story keeps.
		chosen := "classic"
		if effective.StylePurposes.Main == chosen {
			chosen = "screenwriter"
		}
		explicit := pack.StoryRequest()
		explicit.StoryTellerID = chosen
		explicit.StylePurposes = interactive.StoryStylePurposes{Main: chosen}
		explicit, err = service.withStoryCreationDefaults(explicit)
		if err != nil {
			t.Fatalf("apply creation defaults for the explicit choice of pack %s: %v", summary.ID, err)
		}
		explicitStory, err := store.CreateStory(explicit)
		if err != nil {
			t.Fatalf("create story with an explicit choice for pack %s: %v", summary.ID, err)
		}
		if explicitStory.StoryTellerID != chosen {
			t.Fatalf("pack %s overrode the player's explicit style %q with %q",
				summary.ID, chosen, explicitStory.StoryTellerID)
		}
	}
}
