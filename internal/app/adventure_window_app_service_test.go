package app

import (
	"encoding/json"
	"ta/internal/character"
	"ta/internal/interactive"
	"testing"
)

func TestAdventureWindowCommitsOneRoundAndKeepsEnemyInternalsPrivate(t *testing.T) {
	rules := interactive.AdventureRules{Family: "wuxia", Currency: "铜钱", Skills: []interactive.AdventureSkill{{ID: "enemy-technique", Name: "已施展的护体诀", Kind: "guard", Scale: "person", Power: 5, Cost: 5, RequiredRank: 1, TrainingHours: 1, RequiredItem: "enemy-manual"}}, Items: []interactive.AdventureItem{{ID: "enemy-manual", Name: "Secret manual", Kind: "material", Scale: "person", Acquisition: "reward"}}, Opponents: []interactive.AdventureOpponent{{ID: "rival", Name: "对手", Location: "山道", Scale: "person", Health: 100, Attack: 5, Defense: 2, SkillIDs: []string{"enemy-technique"}}}}
	actor := interactive.NewAdventureActor(rules)
	actor.Battle = &interactive.AdventureBattle{ID: "fight", OpponentID: "rival", OpponentName: "对手", Scale: "person", Health: 100, MaxHealth: 100, Attack: 5, Defense: 2, EnemyRank: 1, EnemyEnergy: 100, EnemyMaxEnergy: 100, Status: "active"}
	raw, _ := json.Marshal(actor)
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
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Adventure window", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	application, storyID := &App{workspace: root, interactive: store}, story.ID
	// This projection test needs an actually performed guard, not a particular
	// heuristic ranking. The independent model selects the legal guard explicitly.
	application.cfg = enemyDecisionTestConfig(t, root, func(request enemyDecisionTestRequest) string {
		return `{"candidate_id":"skill:enemy-technique"}`
	})
	before, err := application.AdventureWindow(storyID, "main", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if before.Actor.Battle == nil || before.EnemyCondition != "steady" || !before.Actor.Battle.CanSurrender {
		t.Fatalf("unsafe battle projection: %+v", before)
	}
	if before.Actor.Battle.LastEnemySkillName != "" {
		t.Fatal("an unperformed enemy skill was revealed")
	}
	viewJSON, _ := json.Marshal(before)
	var view map[string]any
	if err := json.Unmarshal(viewJSON, &view); err != nil {
		t.Fatal(err)
	}
	battleView := view["actor"].(map[string]any)["battle"].(map[string]any)
	for _, hidden := range []string{"health", "max_health", "attack", "defense", "enemy_inventory", "enemy_energy", "enemy_skill_ranks", "enemy_last_skill_id", "character_source_key", "lethal"} {
		if _, exists := battleView[hidden]; exists {
			t.Fatalf("private field %s leaked in battle view", hidden)
		}
	}
	// The opponent has actual incoming force to defend against.
	input := interactive.AdventureAction{Action: "battle_round", Choice: "attack"}
	event, err := application.PlayAdventureRound(storyID, "main", "round-1", before.Head, input)
	if err != nil {
		t.Fatal(err)
	}
	again, err := application.PlayAdventureRound(storyID, "main", "round-1", before.Head, input)
	if err != nil || event != again {
		t.Fatalf("retry changed result: %s %s %v", event, again, err)
	}
	after, err := application.AdventureWindow(storyID, "main", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if after.Actor.Battle.LastEnemySkillName != "已施展的护体诀" || len(after.Skills) != 0 {
		t.Fatalf("observed move projection leaked ownership or lost its name: %+v", after)
	}
	if after.Actor.Battle.Round != 1 {
		t.Fatalf("retry repeated round: %+v", after.Actor.Battle)
	}
	if _, err := application.PlayAdventureRound(storyID, "main", "stale", before.Head, input); err == nil {
		t.Fatal("stale head accepted")
	}
	if _, err := application.PlayAdventureRound(storyID, "main", "buy", after.Head, interactive.AdventureAction{Action: "buy"}); err == nil {
		t.Fatal("window bought an item")
	}
	if _, err := application.PlayAdventureRound(storyID, "main", "yield", after.Head, interactive.AdventureAction{Action: "battle_round", Choice: "surrender"}); err != nil {
		t.Fatal(err)
	}
	ended, err := application.AdventureWindow(storyID, "main", "en-US")
	if err != nil || ended.Actor.Battle.LastEnemySkillName != "" || ended.Actor.Battle.CanSurrender || ended.Actor.Battle.Status != "defeat" || ended.ClockSeconds != after.ClockSeconds+20 {
		t.Fatalf("surrender eligibility did not follow the committed round: %+v %v", ended.Actor.Battle, err)
	}
}

func TestRadioGroupReachesRemoteMembersButLocalGroupDoesNot(t *testing.T) {
	group := groupRoundState().Groups[0]
	scene := groupRoundScene()
	if groupMemberReachable(group, scene, "old-zhou") {
		t.Fatal("local group reached absent member")
	}
	group.Channel = character.ChannelMessage
	if groupMemberReachable(group, scene, "old-zhou") {
		t.Fatal("message channel alone granted reachability")
	}
	// Audiences remain membership-scoped in the existing projection.
	if got := group.AudienceFor("", scene.Present); len(got) != 3 {
		t.Fatalf("radio audience = %v", got)
	}
}

func TestRadioGroupCreationPersistsRemoteChannel(t *testing.T) {
	for _, mode := range []string{"local", "radio"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			store := interactive.NewStore(root)
			t.Cleanup(func() { _ = store.Close() })
			module, err := interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.DefaultActorStateModuleID)
			if err != nil {
				t.Fatal(err)
			}
			for i := range module.ActorState.InitialActors {
				if module.ActorState.InitialActors[i].ID == interactive.DefaultActorID {
					module.ActorState.InitialActors[i].State = map[string]any{"重要物品": map[string]any{"terminal": 1}}
				}
			}
			story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Communication", ActorState: &module.ActorState, PackBinding: &interactive.StoryPackBinding{PackID: "radio-test", Scope: "pack:radio-test", Gameplay: &interactive.StoryGameplayProfile{StatusTemplate: "space-state-v1", Growth: "space", Combat: "space", Communication: mode, Opening: "commission"}}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.AppendCharacterFacts(story.ID, interactive.AppendCharacterFactsRequest{BranchID: "main", WorldTime: "Day 1", Profiles: []character.Profile{{ID: "remote", Name: "Remote contact", StoryID: story.ID, RadioContact: &character.RadioContact{DeviceItemID: "terminal", Channel: "agreed-frequency", Status: "connected"}}}})
			if err != nil {
				t.Fatal(err)
			}
			application := &App{workspace: root, interactive: store}
			group, err := application.CreateGroup(GroupCreateRequest{StoryID: story.ID, BranchID: "main", Name: "Team", MemberIDs: []string{"remote"}, CommandID: "create-team"})
			if err != nil {
				t.Fatal(err)
			}
			want := character.ChannelScene
			if mode == "radio" {
				want = character.ChannelMessage
			}
			if group.Channel != want {
				t.Fatalf("channel = %s want %s", group.Channel, want)
			}
			state, err := store.CharacterState(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			saved, ok := state.Group(group.ID)
			if !ok || saved.Channel != want {
				t.Fatalf("saved channel = %+v", saved)
			}
		})
	}
}
