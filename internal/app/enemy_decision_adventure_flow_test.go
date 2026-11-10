package app

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"ta/internal/interactive"
)

func enemyDecisionAdventureTestApp(t *testing.T, family string, idle ...bool) (*App, string) {
	t.Helper()
	rules := interactive.AdventureRules{
		Family: family, Currency: "coins",
		Skills: []interactive.AdventureSkill{
			{ID: "enemy-feint", Name: "试探", Kind: "attack", Scale: "person", Power: 1, Cost: 1, RequiredRank: 1, TrainingHours: 1, RequiredItem: "enemy-manual"},
			{ID: "enemy-heavy", Name: "强攻", Kind: "attack", Scale: "person", Power: 40, Cost: 1, RequiredRank: 1, TrainingHours: 1, RequiredItem: "enemy-manual"},
		},
		Items:     []interactive.AdventureItem{{ID: "enemy-manual", Name: "学习来源", Kind: "material", Scale: "person", Acquisition: "reward"}},
		Opponents: []interactive.AdventureOpponent{{ID: "rival", Name: "对手", Location: "山道", Scale: "person", Health: 100, Attack: 5, Defense: 2, SkillIDs: []string{"enemy-feint", "enemy-heavy"}}},
	}
	actor := interactive.NewAdventureActor(rules)
	actor.Battle = &interactive.AdventureBattle{ID: "fight", OpponentID: "rival", OpponentName: "对手", Scale: "person", Health: 100, MaxHealth: 100, Attack: 5, Defense: 2, EnemyRank: 1, EnemyEnergy: 100, EnemyMaxEnergy: 100, Status: "active"}
	if len(idle) > 0 && idle[0] {
		actor.Battle = nil
	}
	raw, err := json.Marshal(actor)
	if err != nil {
		t.Fatal(err)
	}
	var actorValue map[string]any
	if err := json.Unmarshal(raw, &actorValue); err != nil {
		t.Fatal(err)
	}
	module, err := interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.DefaultActorStateModuleID)
	if err != nil {
		t.Fatal(err)
	}
	system := module.ActorState
	system.AdventureRules = &rules
	for i := range system.Templates {
		if system.Templates[i].ID == interactive.DefaultActorID {
			system.Templates[i].Fields = interactive.AdventureStateFields(rules)
		}
	}
	for i := range system.InitialActors {
		if system.InitialActors[i].ID == interactive.DefaultActorID {
			system.InitialActors[i].State = map[string]any{interactive.AdventureStateField: actorValue}
		}
	}
	if len(idle) > 0 && idle[0] {
		system.Templates = append(system.Templates, interactive.ActorStateTemplate{ID: "ai-test-scene", Name: "场景", Fields: []interactive.ActorStateField{{Name: "当前详细地点", Type: "string", Default: "山道"}}})
		found := false
		for i := range system.InitialActors {
			if system.InitialActors[i].ID == interactive.DefaultStoryContextActorID {
				system.InitialActors[i].TemplateID = "ai-test-scene"
				system.InitialActors[i].State = map[string]any{"当前详细地点": "山道"}
				found = true
			}
		}
		if !found {
			system.InitialActors = append(system.InitialActors, interactive.ActorStateInitialActor{ID: interactive.DefaultStoryContextActorID, Name: "场景", TemplateID: "ai-test-scene", State: map[string]any{"当前详细地点": "山道"}})
		}
	}
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Enemy decision", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	return &App{workspace: root, interactive: store}, story.ID
}

func TestAdventureRoundUsesModelInsteadOfHighestDamageAndReplays(t *testing.T) {
	for _, family := range []string{"wuxia", "space"} {
		t.Run(family, func(t *testing.T) {
			application, storyID := enemyDecisionAdventureTestApp(t, family)
			var calls atomic.Int32
			application.cfg = enemyDecisionTestConfig(t, application.workspace, func(request enemyDecisionTestRequest) string {
				calls.Add(1)
				return `{"candidate_id":"skill:enemy-feint"}`
			})
			before, err := application.AdventureWindow(storyID, "main", "zh-CN")
			if err != nil {
				t.Fatal(err)
			}
			input := interactive.AdventureAction{Action: "battle_round", Choice: "attack"}
			first, err := application.PlayAdventureRoundContext(context.Background(), storyID, "main", "model-round", before.Head, input)
			if err != nil {
				t.Fatal(err)
			}
			after, err := application.interactive.AdventureWindow(storyID, "main")
			if err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 || after.Actor.Battle.EnemyLastSkillID != "enemy-feint" || after.Actor.Battle.EnemyEnergy != 99 {
				t.Fatalf("model choice did not govern actual skill and cost: calls = %d, battle = %+v", calls.Load(), after.Actor.Battle)
			}
			replayed, err := application.PlayAdventureRound(storyID, "main", "model-round", before.Head, input)
			if err != nil || replayed != first || calls.Load() != 1 {
				t.Fatalf("retry regenerated enemy intent: calls = %d, first = %s, replay = %s, err = %v", calls.Load(), first, replayed, err)
			}
			if _, err := application.PlayAdventureRound(storyID, "main", "stale-round", before.Head, input); err == nil || calls.Load() != 1 {
				t.Fatalf("stale input reached model or was committed: calls = %d, err = %v", calls.Load(), err)
			}
		})
	}
}

func TestEnemyDecisionPinsAdventureBranchWhileModelCreatesFork(t *testing.T) {
	application, storyID := enemyDecisionAdventureTestApp(t, "wuxia", true)
	// Start the real encounter in a turn checkpoint before attempting to fork it.
	if _, _, err := application.interactive.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "迎战", Narrative: "双方对峙，尚未交换下一招。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}, World: &interactive.TurnWorldActionInput{Action: "adventure", SourceText: "迎战", Adventure: &interactive.AdventureAction{Action: "battle_start", OpponentID: "rival"}}},
	}); err != nil {
		t.Fatal(err)
	}
	before, err := application.interactive.AdventureWindow(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	created := make(chan string, 1)
	application.cfg = enemyDecisionTestConfig(t, application.workspace, func(request enemyDecisionTestRequest) string {
		fork, err := application.interactive.CreateBranch(storyID, interactive.CreateBranchRequest{ParentEventID: before.Head, Title: "Fork while enemy thinks"})
		if err != nil {
			t.Errorf("create competing branch: %v", err)
		}
		created <- fork.ID
		return `{"candidate_id":"skill:enemy-feint"}`
	})
	// The incoming empty alias meant main when preparation started. The fork
	// becomes current inside Generate while retaining the exact same head.
	eventID, err := application.PlayAdventureRoundContext(context.Background(), storyID, "", "branch-pinned-round", before.Head, interactive.AdventureAction{Action: "battle_round", Choice: "attack"})
	if err != nil {
		t.Fatal(err)
	}
	var forkID string
	select {
	case forkID = <-created:
	default:
		t.Fatal("the configured model was not called")
	}
	if forkID == "" {
		t.Fatal("model callback did not create the branch")
	}
	main, err := application.interactive.AdventureWindow(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	fork, err := application.interactive.AdventureWindow(storyID, forkID)
	if err != nil {
		t.Fatal(err)
	}
	if eventID == "" || main.Actor.Battle.Round != before.Actor.Battle.Round+1 || main.Actor.Battle.EnemyLastSkillID != "enemy-feint" {
		t.Fatalf("original branch did not receive its own command: before=%+v after=%+v", before.Actor.Battle, main.Actor.Battle)
	}
	if fork.Head != before.Head || fork.Actor.Battle.Round != before.Actor.Battle.Round || fork.Actor.Battle.EnemyEnergy != before.Actor.Battle.EnemyEnergy || fork.Actor.Health != before.Actor.Health {
		t.Fatalf("current-branch switch redirected a pending decision into the fork: before=%+v fork=%+v", before, fork)
	}
}
