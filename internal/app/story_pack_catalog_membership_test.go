package app

import (
	"reflect"
	"testing"

	"ta/internal/interactive"
	"ta/internal/storypack"
)

func TestMembershipCatalogCreationSnapshotSurvivesJournal(t *testing.T) {
	frozen := storypack.NewFrozen(7, []storypack.FrozenPerson{{SourceKey: "elder", Name: "Elder", FactionID: "sect", FactionName: "Sect", MembershipAdmission: []string{"formal", "temporary"}}}, nil, nil)
	snapshot := catalogSnapshot(frozen)
	frozen.People[0].MembershipAdmission[0] = "changed"
	store := interactive.NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Membership freeze", PackCatalog: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	context, err := store.StoryContext(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if context.Meta.PackCatalog == nil || len(context.Meta.PackCatalog.Characters) != 1 {
		t.Fatal("journal lost frozen catalog")
	}
	person := context.Meta.PackCatalog.Characters[0]
	if person.FactionID != "sect" || person.FactionName != "Sect" || !reflect.DeepEqual(person.MembershipAdmission, []string{"formal", "temporary"}) {
		t.Fatalf("creation or journal lost membership authority: %#v", person)
	}
}
