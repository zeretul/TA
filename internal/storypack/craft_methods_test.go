package storypack

import (
	"context"
	"reflect"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"testing"
)

func TestCollectCraftMethodVersionsWorldAndFreezesNewGames(t *testing.T) {
	root := t.TempDir()
	library := NewLibrary(root)
	draft := draftTestWorld()
	draft.Gameplay.StatusTemplate = WorldStatusCultivation
	draft.Gameplay.NumericScale = WorldScaleCultivation
	draft.Gameplay.Growth = WorldGrowthCultivation
	draft.Gameplay.Combat = WorldCombatCultivation
	saved, err := library.SaveWorld(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	method, _ := cultivation.NewCraftRegistryWithBuiltins().Method("craft.talisman.ward")
	method.ID = "world.teacher.ward"
	method.Source = "teacher manuscript"
	method.FailureCost = "consume_inputs"
	method.LossChance = 0
	entry := WorldEntryDraft{ID: "teacher-ward", Name: "师传护符法", Type: "technique", Content: "符师传授的护身符法。", SourceRef: "story/teacher-method", CraftMethod: &method}
	receipt, err := library.CollectWorldEntry(context.Background(), saved.PackID, entry)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.CraftMethod == nil || receipt.CraftMethod.Version != 1 {
		t.Fatal("missing persisted method receipt")
	}
	pack, err := library.Get(saved.PackID)
	if err != nil {
		t.Fatal(err)
	}
	module, err := interactive.NewActorStateLibrary(root).Get(pack.Story.ModuleRefs.ActorStateID)
	if err != nil {
		t.Fatal(err)
	}
	frozen := interactive.FreezeActorStateSchema(module.ActorState, false)
	retry, err := library.CollectWorldEntry(context.Background(), saved.PackID, entry)
	if err != nil || retry.ContentVersion != receipt.ContentVersion {
		t.Fatalf("retry changed world: %+v %v", retry, err)
	}
	method.MinutesPerBatch = 240
	if _, err := library.CollectWorldEntry(context.Background(), saved.PackID, entry); err == nil {
		t.Fatal("same version accepted changed method")
	}
	method.Version = 2
	updated, err := library.CollectWorldEntry(context.Background(), saved.PackID, entry)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContentVersion != receipt.ContentVersion+1 {
		t.Fatal("method-only edit failed to move content revision")
	}
	next, err := library.Get(saved.PackID)
	if err != nil {
		t.Fatal(err)
	}
	if next.Story.ModuleRefs.ActorStateID == pack.Story.ModuleRefs.ActorStateID {
		t.Fatal("new method reused old frozen module")
	}
	nextModule, err := interactive.NewActorStateLibrary(root).Get(next.Story.ModuleRefs.ActorStateID)
	if err != nil {
		t.Fatal(err)
	}
	oldDef, newDef := frozen.System.CraftMethods[len(frozen.System.CraftMethods)-1], nextModule.ActorState.CraftMethods[len(nextModule.ActorState.CraftMethods)-1]
	if oldDef.Version != 1 || oldDef.MinutesPerBatch != 120 || newDef.Version != 2 || newDef.MinutesPerBatch != 240 {
		t.Fatalf("version freeze: old %+v new %+v", oldDef, newDef)
	}
	book, err := NewLibrary(root).WorldBook(saved.PackID)
	if err != nil || !reflect.DeepEqual(book.CraftMethods, []cultivation.CraftMethodDef{method}) {
		t.Fatalf("readback differs: %+v %v", book.CraftMethods, err)
	}
	other, err := library.SaveWorld(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	otherBook, err := library.WorldBook(other.PackID)
	if err != nil || len(otherBook.CraftMethods) != 0 {
		t.Fatal("method leaked across worlds")
	}
	bad := method
	bad.Outputs = []cultivation.CraftOutput{{DefID: "fake.effect", Quantity: 1, Chance: 1}}
	bad.Version++
	entry.CraftMethod = &bad
	if _, err := library.CollectWorldEntry(context.Background(), saved.PackID, entry); err == nil {
		t.Fatal("invalid effect persisted")
	}
	after, err := library.WorldBook(saved.PackID)
	if err != nil || !reflect.DeepEqual(after.CraftMethods, book.CraftMethods) {
		t.Fatal("failed collection changed stored methods")
	}
}
