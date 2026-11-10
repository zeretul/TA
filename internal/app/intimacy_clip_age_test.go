package app

import (
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
)

func TestIntimacyMissingFrozenAgeReadsOnlySameAuthoredRevision(t *testing.T) {
	root := t.TempDir()
	pack, err := storypack.NewLibrary(root).Get("fanren-jie")
	if err != nil {
		t.Fatal(err)
	}
	context := interactive.StoryContext{Meta: interactive.StoryMeta{
		PackBinding: pack.Binding(),
		PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{
			{SourceKey: "npc_xie_qingdai"}, {SourceKey: "npc_yinxi"}, {SourceKey: "npc_song_yu", Age: 16}, {SourceKey: "not-authored"},
		}},
	}}
	ages := intimacyAdultAges(context, root)
	if ages["npc_xie_qingdai"] != 25 || ages["npc_yinxi"] != 58 || ages["npc_song_yu"] != 16 || ages["not-authored"] != 0 || len(ages) != 4 {
		t.Fatalf("unexpected same-revision age projection: %v", ages)
	}
	context.Meta.PackBinding.ContentHash = "different-revision"
	ages = intimacyAdultAges(context, root)
	if ages["npc_xie_qingdai"] != 0 || ages["npc_yinxi"] != 0 || ages["npc_song_yu"] != 16 {
		t.Fatalf("changed world revision altered frozen age facts: %v", ages)
	}
}
