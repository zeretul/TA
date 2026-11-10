package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"ta/internal/interactive"
)

func TestGeneratedDefinitionReadToolIsRegisteredBoundedAndReadOnly(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	defer store.Close()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Archive read"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AppendTurn(story.ID, interactive.AppendTurnRequest{BranchID: "main", Narrative: "Empty archive."})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	defs, err := newInteractiveHistoryTools(InteractiveContext{Store: store, StoryID: story.ID, BranchID: "main", MaxResultBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	tool := findTool(t, defs, "search_generated_definitions")
	info, err := tool.Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	schema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := schema.Properties.Get("max_bytes"); ok {
		t.Fatal("internal result budget exposed as model control")
	}
	result, err := tool.Tool.Run(context.Background(), `{"query":"lost sword","limit":4}`)
	if err != nil {
		t.Fatal(err)
	}
	var page interactive.GeneratedDefinitionSearchResult
	if err = json.Unmarshal([]byte(result.ModelContent), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 0 || page.Notice == "" || len(result.ModelContent) > 8192 {
		t.Fatalf("invalid read result: %s", result.ModelContent)
	}
	after, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.State, after.State) || before.TurnCount != after.TurnCount {
		t.Fatal("archive read changed story")
	}
}
