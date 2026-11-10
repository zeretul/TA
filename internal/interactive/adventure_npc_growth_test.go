package interactive

import (
	"reflect"
	"ta/internal/character"
	"testing"
)

func TestAdventureNPCGrowthUsesSourcesClockBranchesAndColdReplay(t *testing.T) {
	for _, family := range []string{"space", "wuxia"} {
		t.Run(family, func(t *testing.T) {
			r := adventureTestRules()
			if family == "wuxia" {
				r.Family = "wuxia"
				r.Skills = r.Skills[:2]
				r.StartingSkills = []string{"strike"}
				r.Items = []AdventureItem{r.Items[0], r.Items[1], r.Items[2]}
				r.StartingItems = map[string]int{"scrap": 10}
				r.Recipes = nil
				r.Opponents = r.Opponents[:1]
				r.Markets[0].Items = []string{"scrap", "blade"}
			}
			npc := NewAdventureActor(r)
			npc.Inventory = map[string]int{}
			npc.Skills = map[string]int{"strike": 1}
			r.NPCs = []AdventureNPC{{CharacterSourceKey: "person.student", Actor: npc, Plan: AdventureNPCPlan{Action: "train", SkillID: "strike", TrainingPercent: 50, RankLimit: 2}}, {CharacterSourceKey: "person.blocked", Actor: npc, Plan: AdventureNPCPlan{Action: "learn", SkillID: "advanced", TrainingPercent: 100, RankLimit: 2}}, {CharacterSourceKey: "person.unseen", Actor: npc, Plan: AdventureNPCPlan{Action: "train", SkillID: "strike", TrainingPercent: 100, RankLimit: 2}}}
			store, id := adventureTestStoryWithRules(t, r)
			_, err := store.AppendCharacterFacts(id, AppendCharacterFactsRequest{BranchID: "main", WorldTime: "第1日", Scene: character.Scene{Location: "码头", Present: []string{"student", "blocked"}}, Profiles: []character.Profile{{ID: "student", Name: "Student", SourceKey: "person.student", Revision: 1}, {ID: "blocked", Name: "Blocked", SourceKey: "person.blocked", Revision: 1}}})
			if err != nil {
				t.Fatal(err)
			}
			turn, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "observe", Narrative: "They arrange their study.", TurnResult: &TurnResult{Choices: testTurnChoices()}})
			if err != nil {
				t.Fatal(err)
			}
			fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: turn.ID, Title: "before training"})
			if err != nil {
				t.Fatal(err)
			}
			train := func(hours int) {
				t.Helper()
				_, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "rest", Narrative: "Time passes.", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "rest", Adventure: &AdventureAction{Action: "rest", Hours: hours}}}})
				if err != nil {
					t.Fatal(err)
				}
			}
			train(8)
			ctx, err := store.StoryContext(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			grown, err := AdventureActorFromState(ctx.Snapshot.State, "student")
			if err != nil {
				t.Fatal(err)
			}
			blocked, err := AdventureActorFromState(ctx.Snapshot.State, "blocked")
			if err != nil {
				t.Fatal(err)
			}
			if grown.Skills["strike"] != 2 || grown.Growth == nil || grown.Growth.Status != "settled" || blocked.Skills["advanced"] != 0 || blocked.Growth.Status != "blocked" {
				t.Fatalf("growth bypassed plan/source: %+v / %+v", grown, blocked)
			}
			if _, err = AdventureActorFromState(ctx.Snapshot.State, "unseen"); err == nil {
				t.Fatal("simulated an unestablished character")
			}
			branchCtx, err := store.StoryContext(id, fork.ID)
			if err != nil {
				t.Fatal(err)
			}
			if branchActor, err := AdventureActorFromState(branchCtx.Snapshot.State, "student"); err == nil && branchActor.Growth != nil {
				t.Fatal("growth leaked into earlier branch")
			}
			store.mu.Lock()
			projection, err := store.storyBranchProjectionLocked(id, "main")
			system := npcGrowthTestSystemUnlocked(store, id)
			store.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			events, _ := adventureNPCGrowthEvents(projection, system, "main", projection.Head, "now", "", "", StoryClock{Version: StoryClockVersion, Seconds: 0}, *projection.Clock)
			if len(events) != 0 {
				t.Fatal("same interval settled twice")
			}
			// Store reconstruction is tested with a fresh object over the same data root.
			reopened := NewStore(store.Root())
			defer reopened.Close()
			restoredCtx, err := reopened.StoryContext(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			restored, err := AdventureActorFromState(restoredCtx.Snapshot.State, "student")
			if err != nil || !reflect.DeepEqual(grown, restored) {
				t.Fatalf("NPC cold replay changed state: %v", err)
			}
		})
	}
}
func npcGrowthTestSystemUnlocked(store *Store, id string) StoryDirectorActorStateSystem {
	meta, _, _ := store.readStoryRecentLocked(id, "main")
	return actorStateSystemFromSnapshot(meta.ActorStateSchema, store.storyDirectorForMeta(meta).ActorState)
}

func TestAdventureEncounterUsesSameNPCAndPersistsInjury(t *testing.T) {
	r := adventureTestRules()
	r.Opponents[0].CharacterSourceKey = "person.rival"
	npc := NewAdventureActor(r)
	npc.Health = 40
	npc.MaxHealth = 100
	npc.Energy = 30
	npc.SourceKey = "person.rival"
	npc.Inventory = map[string]int{} // The rival never owned the player's unique starter relics.
	npc.Skills = map[string]int{"strike": 1}
	r.NPCs = []AdventureNPC{{CharacterSourceKey: "person.rival", Actor: npc, Plan: AdventureNPCPlan{Action: "none", RankLimit: 1}}}
	store, id := adventureTestStoryWithRules(t, r)
	_, err := store.AppendCharacterFacts(id, AppendCharacterFactsRequest{BranchID: "main", WorldTime: "第1日", Scene: character.Scene{Location: "码头", Present: []string{"rival"}}, Profiles: []character.Profile{{ID: "rival", Name: "Rival", SourceKey: "person.rival", Revision: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "迎战", Narrative: "双方摆开架势。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "迎战", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "rival"}}}})
	if err != nil {
		t.Fatal(err)
	}
	window, err := store.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if window.Actor.Battle.EnemyActorID != "rival" || window.Actor.Battle.Health != 40 || window.Actor.Battle.EnemyEnergy != 30 {
		t.Fatalf("encounter ignored actual NPC: %+v", window.Actor.Battle)
	}
	result, err := store.CommitAdventureAction(id, "main", "round", window.Head, AdventureAction{Action: "battle_round", Choice: "attack"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := store.StoryContext(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := AdventureActorFromState(ctx.Snapshot.State, "rival")
	if err != nil {
		t.Fatal(err)
	}
	if actual.Health != result.Actor.Battle.Health || actual.SourceKey != "person.rival" {
		t.Fatalf("injury not attached to same NPC: %+v", actual)
	}
	if template, ok := actorTemplateIDFromStateOrSystem(ctx.Snapshot.State, StoryDirectorActorStateSystem{}, "rival"); !ok || template != AdventureNPCTemplateID {
		t.Fatalf("NPC did not retain private template: %s", template)
	}
}
