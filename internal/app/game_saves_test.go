package app

import (
	"context"
	"ta/config"
	"ta/internal/interactive"
	"testing"
)

func TestGameSavesListsIndependentWorldsWithoutSwitching(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	a, err := New(ctx, &config.Config{DenovaDir: root, OpenAIModel: "test-model", ResumeLastWorkspace: false})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	wanted := map[string]string{}
	for _, name := range []string{"First world", "Second world"} {
		created, err := a.CreateBook(ctx, root, name, "", "")
		if err != nil {
			t.Fatal(err)
		}
		story, err := a.interactive.CreateStory(interactive.CreateStoryRequest{Title: name + " save"})
		if err != nil {
			t.Fatal(err)
		}
		wanted[story.ID] = created.ProjectID
	}
	workspace := a.workspace
	current, err := a.interactive.Index()
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		catalog, err := a.GameSaves(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(catalog.Unavailable) != 0 {
			t.Fatalf("unavailable = %v", catalog.Unavailable)
		}
		found := map[string]string{}
		for _, save := range catalog.Saves {
			if _, ok := wanted[save.Story.ID]; ok {
				found[save.Story.ID] = save.ProjectID
			}
			if save.Workspace == "" || save.WorldName == "" {
				t.Fatalf("missing save metadata: %+v", save)
			}
		}
		if len(found) != len(wanted) {
			t.Fatalf("catalog missing saves: got %v want %v", found, wanted)
		}
		for id, pid := range wanted {
			if found[id] != pid {
				t.Fatalf("save project mismatch for %s", id)
			}
		}
		if a.workspace != workspace {
			t.Fatal("catalog switched foreground workspace")
		}
		after, err := a.interactive.Index()
		if err != nil || after.CurrentStoryID != current.CurrentStoryID {
			t.Fatalf("selection changed: %+v %v", after, err)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := a.GameSaves(cancelled); err == nil {
		t.Fatal("cancelled catalog request succeeded")
	}
}
