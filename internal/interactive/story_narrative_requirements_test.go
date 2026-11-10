package interactive

import (
	"strings"
	"testing"
)

func TestCreateStoryStoresItsOwnNarrativeRequirements(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:                 "Own direction",
		NarrativeRequirements: "  慢节奏，多写环境与沉默。\r\n  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if story.NarrativeRequirements != "慢节奏，多写环境与沉默。" {
		t.Fatalf("summary requirements = %q", story.NarrativeRequirements)
	}
	ctx, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	meta := ctx.Meta
	if meta.NarrativeRequirements != "慢节奏，多写环境与沉默。" {
		t.Fatalf("stored requirements = %q", meta.NarrativeRequirements)
	}
}

func TestUpdateStoryNarrativeRequirementsPointerSemantics(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "Pointer", NarrativeRequirements: "第一版"})
	if err != nil {
		t.Fatal(err)
	}

	// Omitting the field leaves the stored value alone.
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{Title: "Pointer renamed"}); err != nil {
		t.Fatal(err)
	}
	ctx, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	meta := ctx.Meta
	if meta.NarrativeRequirements != "第一版" {
		t.Fatalf("omitted field changed the value: %q", meta.NarrativeRequirements)
	}

	// A value replaces it.
	next := "第二版"
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{NarrativeRequirements: &next}); err != nil {
		t.Fatal(err)
	}
	ctx, err = store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	meta = ctx.Meta
	if meta.NarrativeRequirements != "第二版" {
		t.Fatalf("value did not replace: %q", meta.NarrativeRequirements)
	}

	// An explicit empty string clears it, falling back to the workspace default.
	empty := ""
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{NarrativeRequirements: &empty}); err != nil {
		t.Fatal(err)
	}
	ctx, err = store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	meta = ctx.Meta
	if meta.NarrativeRequirements != "" {
		t.Fatalf("explicit empty did not clear: %q", meta.NarrativeRequirements)
	}
}

func TestStoryNarrativeRequirementsScope(t *testing.T) {
	own, scope := StoryNarrativeRequirements(StoryMeta{NarrativeRequirements: "本局"}, "工作区")
	if own != "本局" || scope != StoryNarrativeRequirementsScopeStory {
		t.Fatalf("own = %q scope = %q", own, scope)
	}
	// A story with its own direction reads only that value; merging it with the
	// workspace default is exactly the leak that puts one story's rules into
	// another.
	if strings.Contains(own, "工作区") {
		t.Fatalf("own direction merged with the workspace default: %q", own)
	}
	fallback, scope := StoryNarrativeRequirements(StoryMeta{}, "工作区")
	if fallback != "工作区" || scope != StoryNarrativeRequirementsScopeWorkspace {
		t.Fatalf("fallback = %q scope = %q", fallback, scope)
	}
	none, scope := StoryNarrativeRequirements(StoryMeta{}, "   ")
	if none != "" || scope != StoryNarrativeRequirementsScopeNone {
		t.Fatalf("none = %q scope = %q", none, scope)
	}
}

func TestCreateStoryRejectsOversizeNarrativeRequirements(t *testing.T) {
	store := NewStore(t.TempDir())
	_, err := store.CreateStory(CreateStoryRequest{
		Title:                 "Too long",
		NarrativeRequirements: strings.Repeat("x", MaxStoryNarrativeRequirementsBytes+1),
	})
	if err == nil {
		t.Fatal("expected an oversize requirement to be rejected at creation")
	}
}
