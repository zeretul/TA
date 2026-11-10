package app

import (
	"context"
	"ta/internal/agents/configresource"
	"ta/internal/cultivation"
	"ta/internal/storypack"
	"testing"
)

func TestScopedAssistantCollectsAndReadsRealCraftMethods(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	library := storypack.NewLibrary(application.cfg.DataDir())
	saved, err := library.SaveWorld(context.Background(), storypack.WorldDraft{Name: "制符山居", Title: "山居", Origin: "你在山居学习技艺。", World: []storypack.DraftWorldEntry{{Name: "山居", Type: "place", Content: "符师隐居的山间小屋。"}}, Gameplay: storypack.WorldGameplay{StatusTemplate: storypack.WorldStatusCultivation, NumericScale: storypack.WorldScaleCultivation, Growth: storypack.WorldGrowthCultivation, Combat: storypack.WorldCombatCultivation, Communication: storypack.WorldCommunicationLocal, Opening: storypack.WorldOpeningDaily, Confirmed: true}})
	if err != nil {
		t.Fatal(err)
	}
	writer := agentScopedSettings{app: application}
	method, _ := cultivation.NewCraftRegistryWithBuiltins().Method("craft.talisman.ward")
	method.ID = "world.ward.assistant"
	method.Source = "author confirmed teacher method"
	method.FailureCost = "consume_inputs"
	receipt, err := writer.CollectWorldEntry(context.Background(), saved.PackID, configresource.WorldEntryValue{Name: "山居护符法", Type: "technique", Content: "山居符师的护符制作方法。", SourceRef: "author/ward", CraftMethod: &method})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Entry.CraftMethod == nil || receipt.Entry.CraftMethod.ID != method.ID {
		t.Fatal("assistant did not receive saved method")
	}
	book, err := writer.ReadWorldBook(context.Background(), saved.PackID)
	if err != nil || len(book.CraftMethods) != 1 || book.CraftMethods[0].ID != method.ID {
		t.Fatalf("assistant cannot read back formal methods: %+v %v", book, err)
	}
	persisted, err := library.Get(saved.PackID)
	if err != nil || len(persisted.Story.Gameplay.CraftMethods) != 1 {
		t.Fatalf("method was not stored in actual world: %v", err)
	}
}
