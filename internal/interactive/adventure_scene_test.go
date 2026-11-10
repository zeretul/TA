package interactive

import (
	"reflect"
	"testing"
)

func TestAdventureSceneResolvesTheCommittedRegisteredPlace(t *testing.T) {
	rules := adventureTestRules()
	rules.world = authoredWorldContent(t, "lanchuan-qunxia")
	rules.Markets[0].Location = "潮信渡"
	rules.Opponents[0].Location = "潮信渡"
	for _, tc := range []struct {
		name, location string
		allowed        bool
	}{
		{"authored opening", "澜口·潮信渡（渡公宋老橹的破木茅屋）", true},
		{"local scene detail", "潮信渡 · 石碑前泥滩", true},
		{"registered identifier", "lanchuan-l02", true},
		{"different registered place", "白鹭渔村 · 打听潮信渡消息", false},
		{"unregistered remote place", "外太空", false},
		{"ambiguous registered places", "潮信渡与青盐滩之间", false},
		{"ambiguous decorated scene", "潮信渡 · 青盐滩", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := map[string]any{"actors": map[string]any{DefaultStoryContextActorID: map[string]any{"state": map[string]any{storySceneLocationField: tc.location}}}}
			for _, action := range []AdventureAction{{Action: "battle_start", OpponentID: "rival"}, {Action: "buy", MarketID: "dock", ItemID: "blade"}, {Action: "repair", MarketID: "dock"}} {
				if err := validateAdventureScene(rules, state, action); (err == nil) != tc.allowed {
					t.Fatalf("%s at %q allowed=%v: %v", action.Action, tc.location, tc.allowed, err)
				}
			}
		})
	}
}

func TestAdventureOpeningSceneCommitsBattleWithoutRelocation(t *testing.T) {
	rules := adventureTestRules()
	rules.world = authoredWorldContent(t, "lanchuan-qunxia")
	rules.Opponents[0].Location = "潮信渡"
	actor := NewAdventureActor(rules)
	store, id := adventureFixtureStoryWithActor(t, rules, actor, "澜口·潮信渡（渡公宋老橹的破木茅屋）")
	defer store.Close()
	before, err := store.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "开始切磋", Narrative: "对手同意切磋，你们摆好架势。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "开始切磋", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "rival"}}}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := AdventureActorFromState(after.State, DefaultActorID)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Battle == nil || committed.Battle.Status != "active" || committed.Battle.OpponentID != "rival" {
		t.Fatalf("battle not established: %+v", committed.Battle)
	}
	if StorySceneFromState(before.State, "").Location != StorySceneFromState(after.State, "").Location {
		t.Fatal("battle setup rewrote the committed location")
	}
	if committed.Currency != actor.Currency || !reflect.DeepEqual(committed.Inventory, actor.Inventory) {
		t.Fatal("battle setup injected resources")
	}
}
