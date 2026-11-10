package storypack

import (
	"testing"

	"ta/internal/interactive"
)

// A pack's creative direction is a seed for the story it starts, not a live
// binding. It must arrive on the creation request so the story owns a copy, and
// editing the pack afterwards must leave that story's text alone.
func TestPackSeedsNarrativeRequirementsIntoTheStory(t *testing.T) {
	dataRoot := t.TempDir()
	writePackFile(t, dataRoot, "seeded.json", `{
  "version": 1,
  "id": "seeded",
  "name": "带默认要求的包",
  "story": {
    "title": "带默认要求的包",
    "origin": "外置包的开场。",
    "module_refs": {"event_package_ids": ["default"]},
    "narrative_requirements": "第三人称，克制，不写内心独白。"
  },
  "world": []
}`)

	pack, err := NewLibrary(dataRoot).Get("seeded")
	if err != nil {
		t.Fatalf("reading the pack: %v", err)
	}
	if got := pack.StoryRequest().NarrativeRequirements; got != "第三人称，克制，不写内心独白。" {
		t.Fatalf("pack request requirements = %q", got)
	}

	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(pack.StoryRequest())
	if err != nil {
		t.Fatal(err)
	}
	if story.NarrativeRequirements != "第三人称，克制，不写内心独白。" {
		t.Fatalf("story did not copy the pack default: %q", story.NarrativeRequirements)
	}

	// Editing the pack must not rewrite a story already under way: the story
	// holds its own copy, which is what keeps two worlds from sharing one text.
	writePackFile(t, dataRoot, "seeded.json", `{
  "version": 1,
  "id": "seeded",
  "name": "带默认要求的包",
  "story": {
    "title": "带默认要求的包",
    "origin": "外置包的开场。",
    "module_refs": {"event_package_ids": ["default"]},
    "narrative_requirements": "改写后的要求。"
  },
  "world": []
}`)
	edited, err := NewLibrary(dataRoot).Get("seeded")
	if err != nil {
		t.Fatalf("re-reading the pack: %v", err)
	}
	if edited.StoryRequest().NarrativeRequirements != "改写后的要求。" {
		t.Fatal("the edited pack did not produce the new value")
	}
	context, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if context.Meta.NarrativeRequirements != "第三人称，克制，不写内心独白。" {
		t.Fatalf("a pack edit rewrote a running story: %q", context.Meta.NarrativeRequirements)
	}
}
