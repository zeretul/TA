package interactive

import (
	"reflect"
	"testing"

	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

func TestKnownSocialFactsCommitReopenAndFork(t *testing.T) {
	store := NewStore(t.TempDir())
	system := actorStateSystemForPreset(cultivation.ReferenceRules(),
		defaultActorStatePresetSpec())
	story, err := store.CreateStory(CreateStoryRequest{Title: "Social knowledge", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "询问门派与同伴", Narrative: "两人分别介绍了自己的归属与相识经过。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{
			{Op: interactivestate.Replace, Path: "/protagonist/所属势力", Value: []any{"青山书院"}},
			createActorUpdate("陈砚", ActorStateImportantCharacterTemplateID, map[string]any{
				"基本身份": "掌柜", "所属势力": []any{map[string]any{"faction_id": "academy", "name": "青山书院", "status": "formal"}}, "出场与位置": "亲眼见于码头",
				"已知位置":   map[string]any{"location": "码头", "learned_at": "第一日午时", "source": "observation", "nature": "experienced", "confidence": "confirmed"},
				"已知人物关系": map[string]any{"苏浅": map[string]any{"kind": "同门", "source": "conversation", "nature": "told", "confidence": "uncertain", "learned_at": "第一日午时"}},
			}),
			createActorUpdate("苏浅", ActorStateImportantCharacterTemplateID, map[string]any{"基本身份": "同行者"}),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: first.ID, Title: "Before departure"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "听他说明辞别", Narrative: "陈砚当面说明已经离开书院，此后去向未定。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{
			{Op: interactivestate.Replace, Path: "/陈砚/所属势力", Value: []any{map[string]any{"faction_id": "academy", "name": "青山书院", "status": "left"}}},
			{Op: interactivestate.Replace, Path: "/陈砚/出场与位置", Value: "辞别后去向未明"},
			{Op: interactivestate.Replace, Path: "/陈砚/已知位置", Value: map[string]any{"location": "据说往山口去", "learned_at": "第二日辰时", "source": "conversation", "nature": "hearsay", "confidence": "uncertain"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(store.root)
	read := func(branchID string) map[string]any {
		t.Helper()
		snapshot, err := reopened.Snapshot(story.ID, branchID)
		if err != nil {
			t.Fatal(err)
		}
		actor := snapshot.State["actors"].(map[string]any)["陈砚"].(map[string]any)
		return actor["state"].(map[string]any)
	}
	main, fork := read("main"), read(branch.ID)
	if !reflect.DeepEqual(main["所属势力"], []any{map[string]any{"faction_id": "academy", "name": "青山书院", "status": "left"}}) || main["出场与位置"] != "辞别后去向未明" {
		t.Fatalf("latest social facts did not replay: %#v", main)
	}
	if !reflect.DeepEqual(fork["所属势力"], []any{map[string]any{"faction_id": "academy", "name": "青山书院", "status": "formal"}}) || fork["出场与位置"] != "亲眼见于码头" {
		t.Fatalf("later knowledge leaked into fork: %#v", fork)
	}
	if !reflect.DeepEqual(main["已知人物关系"], fork["已知人物关系"]) {
		t.Fatal("unrelated update rewrote learned relationships")
	}
	mainLocation := main["已知位置"].(map[string]any)
	forkLocation := fork["已知位置"].(map[string]any)
	if mainLocation["learned_at"] != "第二日辰时" || mainLocation["nature"] != "hearsay" || mainLocation["confidence"] != "uncertain" || forkLocation["learned_at"] != "第一日午时" || forkLocation["source"] != "observation" {
		t.Fatalf("knowledge provenance did not stay branch-scoped: main=%#v fork=%#v", mainLocation, forkLocation)
	}

	cast, err := reopened.CharacterState(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(cast.CharacterIDs(), "陈砚") || !containsString(cast.CharacterIDs(), "苏浅") {
		t.Fatalf("committed people missing from normal character window: %v", cast.CharacterIDs())
	}
}
