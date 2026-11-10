package app

import (
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
)

func TestRecordOnlyCatalogCreationSnapshotSurvivesColdJournal(t *testing.T) {
	frozen := storypack.NewFrozen(6, []storypack.FrozenPerson{{SourceKey: "ancient", Name: "Ancient", RecordOnly: true}}, nil, nil)
	snapshot := catalogSnapshot(frozen)
	store := interactive.NewStore(t.TempDir())
	defer store.Close()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Historical availability freeze", PackCatalog: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	frozen.People[0].RecordOnly = false
	snapshot.Characters[0].RecordOnly = false
	cold := interactive.NewStore(store.Root())
	defer cold.Close()
	context, err := cold.StoryContext(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if context.Meta.PackCatalog == nil || !context.Meta.PackCatalog.Characters[0].RecordOnly {
		t.Fatal("creation or cold journal lost frozen historical boundary")
	}
}
