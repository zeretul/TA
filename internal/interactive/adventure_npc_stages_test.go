package interactive

import (
	"encoding/json"
	"fmt"
	"reflect"
	"ta/internal/character"
	"testing"
)

func TestAdventurePeriodNPCStagesAndOwnedLoadouts(t *testing.T) {
	for _, world := range []string{"xinghai-689", "lanchuan-qunxia"} {
		t.Run(world, func(t *testing.T) {
			r := authoredAdventureRules(t, world)
			if err := r.Validate(); err != nil {
				t.Fatal(err)
			}
			mechanical, _ := json.Marshal(adventureMechanicalRules(r))
			t.Logf("mechanical prefix bytes=%d", len(mechanical))
			if len(r.NPCs) != 60 {
				t.Fatal("native people were removed")
			}
			expected := map[string]int{"xinghai-p01": 23, "xinghai-p02": 44, "xinghai-p03": 45, "xinghai-p04": 50, "xinghai-p05": 49, "xinghai-p13": 20, "xinghai-p15": 6, "xinghai-p20": 2, "lanchuan-p14": 38, "lanchuan-p25": 37, "lanchuan-p33": 33, "lanchuan-p34": 34, "lanchuan-p40": 33, "lanchuan-p41": 17, "lanchuan-p54": 39, "lanchuan-p60": 38}
			for _, npc := range r.NPCs {
				a := npc.Actor
				if rank, ok := expected[npc.CharacterSourceKey]; ok && a.Rank != rank {
					t.Errorf("period mapping %s: %d != %d", npc.CharacterSourceKey, a.Rank, rank)
				}
				if r.Family == "space" && a.Rank >= 51 {
					t.Errorf("future godhood granted to %s", npc.CharacterSourceKey)
				}
				if npc.Plan.RankLimit != a.Rank {
					t.Errorf("unearned future growth for %s", npc.CharacterSourceKey)
				}
				if npc.CharacterSourceKey == "lanchuan-p41" && a.Skills["lanchuan-s16"] != 0 {
					t.Fatal("opening Linghu Chong inherited future Nine Swords")
				}
				for _, item := range r.Items {
					if a.Equipment["person:"+item.Kind] == item.ID && item.RequiredSkill != "" && a.Skills[item.RequiredSkill] == 0 {
						t.Errorf("%s lacks equipped qualification for %s", npc.CharacterSourceKey, item.ID)
					}
				}
				for _, skill := range r.Skills {
					if a.Skills[skill.ID] == 0 {
						continue
					}
					if skill.RequiredTool != "" && a.Inventory[skill.RequiredTool] == 0 {
						t.Errorf("%s cannot use known skill %s without tool", npc.CharacterSourceKey, skill.ID)
					}
					for material, n := range skill.Materials {
						if a.Inventory[material] < n {
							t.Errorf("%s lacks %s for %s", npc.CharacterSourceKey, material, skill.ID)
						}
					}
				}
				for _, op := range r.Opponents {
					if op.CharacterSourceKey == npc.CharacterSourceKey && (op.Rank != a.Rank || op.Health != a.MaxHealth || op.Energy != a.MaxEnergy || !op.RequiresPresence) {
						t.Errorf("stale named encounter %s", op.ID)
					}
				}
			}
		})
	}
}

func TestAdventureMappedNPCsUseTheirRealCombatSkillsAndEquipment(t *testing.T) {
	for _, world := range []string{"xinghai-689", "lanchuan-qunxia"} {
		r := authoredAdventureRules(t, world)
		for _, op := range r.Opponents {
			if !op.RequiresPresence {
				continue
			}
			t.Run(op.ID, func(t *testing.T) {
				var npc AdventureActor
				for _, n := range r.NPCs {
					if n.CharacterSourceKey == op.CharacterSourceKey {
						npc = n.Actor
					}
				}
				// A bounded solver fixture isolates the authored opponent, not an earned player build.
				a := NewAdventureActor(r)
				a.Rank = npc.Rank
				a.Health = 10000
				a.MaxHealth = 10000
				a.Energy = 1000
				a.MaxEnergy = 1000
				started, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: op.ID}, "mapped-npc")
				if err != nil {
					t.Fatal(err)
				}
				bindAdventureEncounter(started.Battle, "established-person", npc, r)
				if !reflect.DeepEqual(started.Battle.EnemySkillRanks, npc.Skills) || !reflect.DeepEqual(started.Battle.EnemyEquipment, npc.Equipment) {
					t.Fatal("binding lost actual learned or owned record")
				}
				next, _, err := ResolveAdventureAction(r, started, AdventureAction{Action: "battle_round", Choice: "attack"}, "mapped-round")
				if err != nil {
					t.Fatal(err)
				}
				b := next.Battle
				if b.EnemyLastSkillID == "" || npc.Skills[b.EnemyLastSkillID] == 0 {
					t.Fatalf("known combat skills not consumed: %+v", b)
				}
				t.Logf("rank=%d skill=%s mastery=%d player_damage=%d npc_damage=%d energy_spent=%d", npc.Rank, b.EnemyLastSkillID, npc.Skills[b.EnemyLastSkillID], a.Health-next.Health, npc.Health-b.Health, npc.Energy-b.EnemyEnergy)
				if op.CharacterSourceKey == "xinghai-p05" {
					if b.EnemyInventory["xinghai-weapon-cell"] != npc.Inventory["xinghai-weapon-cell"]-1 {
						t.Fatal("bound mechanical emitter did not spend owned ammunition")
					}
					a.Skills["xinghai-advanced-strike-6"] = 10
					forceful := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-advanced-strike-6"}
					loaded, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: op.ID}, "equipment-comparison")
					if e != nil {
						t.Fatal(e)
					}
					bindAdventureEncounter(loaded.Battle, "established-person", npc, r)
					equipped, _, e := ResolveAdventureAction(r, loaded, forceful, "equipment-round")
					if e != nil {
						t.Fatal(e)
					}
					raw, _ := json.Marshal(npc)
					var bare AdventureActor
					_ = json.Unmarshal(raw, &bare)
					bare.Equipment = map[string]string{}
					unarmed, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: op.ID}, "mapped-npc")
					if e != nil {
						t.Fatal(e)
					}
					bindAdventureEncounter(unarmed.Battle, "established-person", bare, r)
					naked, _, e := ResolveAdventureAction(r, unarmed, forceful, "equipment-round")
					if e != nil {
						t.Fatal(e)
					}
					if equipped.Health >= naked.Health || equipped.Battle.Health <= naked.Battle.Health {
						t.Fatal("real carried weapon/armor made no combat difference")
					}
				}
			})
		}
	}
}

func npcStageStory(t *testing.T, r AdventureRules, op AdventureOpponent, present bool) (*Store, string, string) {
	t.Helper()
	actorID := "period-person"
	system := StoryDirectorActorStateSystem{WorldContent: r.world, AdventureRules: &r, Templates: []ActorStateTemplate{{ID: DefaultActorID, Name: "Traveler", Fields: AdventureStateFields(r)}, {ID: AdventureNPCTemplateID, Name: "Person", Fields: AdventureNPCStateFields(r)}, {ID: DefaultStoryContextActorID, Name: "Scene", Fields: []ActorStateField{{Name: storySceneLocationField, Type: "string"}, {Name: storyScenePresentField, Type: "list"}}}}}
	a := NewAdventureActor(r)
	a.Rank = AdventureMaxRank(r)
	a.MaxHealth = 10000
	a.Health = 10000
	values := map[string]any{}
	for _, f := range AdventureStateFields(r) {
		values[f.Name] = f.Default
	}
	actorJSON, _ := json.Marshal(a)
	var actorValue map[string]any
	_ = json.Unmarshal(actorJSON, &actorValue)
	values[AdventureStateField] = actorValue
	participants := []any{}
	if present {
		participants = append(participants, actorID)
	}
	system.InitialActors = []ActorStateInitialActor{{ID: DefaultActorID, Name: "Traveler", TemplateID: DefaultActorID, State: values}, {ID: DefaultStoryContextActorID, Name: "Scene", TemplateID: DefaultStoryContextActorID, State: map[string]any{storySceneLocationField: op.Location, storyScenePresentField: participants}}}
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "Frozen period encounter", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "Arrive", Narrative: "The agreed location is reached.", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	return store, story.ID, actorID
}

func TestAdventureNamedMeetingRequiresEstablishedPresentPerson(t *testing.T) {
	r := authoredAdventureRules(t, "xinghai-689")
	var op AdventureOpponent
	for _, e := range r.Opponents {
		if e.CharacterSourceKey == "xinghai-p02" {
			op = e
		}
	}
	for _, present := range []bool{false, true} {
		t.Run(fmt.Sprint(present), func(t *testing.T) {
			meeting := op
			meeting.Location = "A meeting already established away from home"
			store, id, actorID := npcStageStory(t, r, meeting, present)
			start := func() error {
				_, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "Spar", Narrative: "The requested meeting begins.", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "Spar", Adventure: &AdventureAction{Action: "battle_start", OpponentID: op.ID}}}})
				return err
			}
			if err := start(); err == nil {
				t.Fatal("catalog entry summoned an unestablished person")
			}
			_, err := store.AppendCharacterFacts(id, AppendCharacterFactsRequest{BranchID: "main", WorldTime: "689", Scene: character.Scene{Location: op.Location}, Profiles: []character.Profile{{ID: actorID, Name: op.Name, SourceKey: op.CharacterSourceKey, Revision: 1}}})
			if err != nil {
				t.Fatal(err)
			}
			err = start()
			if present && err != nil {
				t.Fatal(err)
			}
			if !present && err == nil {
				t.Fatal("remote person was summoned into the battle")
			}
		})
	}
}

func TestAdventureNPCPeriodSnapshotSurvivesForkAndColdReopen(t *testing.T) {
	for _, world := range []string{"xinghai-689", "lanchuan-qunxia"} {
		t.Run(world, func(t *testing.T) {
			r := authoredAdventureRules(t, world)
			var op AdventureOpponent
			source := "xinghai-p01"
			if r.Family == "wuxia" {
				source = "lanchuan-p14"
			}
			for _, e := range r.Opponents {
				if e.CharacterSourceKey == source {
					op = e
				}
			}
			for _, old := range []bool{false, true} {
				raw, _ := json.Marshal(r)
				var frozen AdventureRules
				_ = json.Unmarshal(raw, &frozen)
				expectedRank := op.Rank
				if old { // Model an already-frozen pre-remap candidate, never upgrade it from the current source.
					expectedRank = 7
					for i := range frozen.NPCs {
						n := &frozen.NPCs[i]
						if n.CharacterSourceKey == source {
							n.Actor.Rank = 7
							n.Actor.Health = 250
							n.Actor.MaxHealth = 250
							n.Actor.Energy = 160
							n.Actor.MaxEnergy = 160
							n.Actor.Skills = map[string]int{frozen.StartingSkills[0]: 1}
							n.Actor.Inventory = map[string]int{}
							n.Actor.Equipment = map[string]string{}
							n.Plan = AdventureNPCPlan{Action: "none", RankLimit: 7}
						}
					}
					for i := range frozen.Opponents {
						if frozen.Opponents[i].ID == op.ID {
							frozen.Opponents[i].Rank = 7
							frozen.Opponents[i].Health = 250
							frozen.Opponents[i].Energy = 160
						}
					}
				}
				store, id, actorID := npcStageStory(t, frozen, op, true)
				_, err := store.AppendCharacterFacts(id, AppendCharacterFactsRequest{BranchID: "main", WorldTime: "opening", Scene: character.Scene{Location: op.Location}, Profiles: []character.Profile{{ID: actorID, Name: op.Name, SourceKey: source, Revision: 1}}})
				if err != nil {
					t.Fatal(err)
				}
				_, _, err = store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "Spar", Narrative: "A cautious exchange.", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "Spar", Adventure: &AdventureAction{Action: "battle_start", OpponentID: op.ID}}}})
				if err != nil {
					t.Fatal(err)
				}
				before, err := store.AdventureWindow(id, "main")
				if err != nil {
					t.Fatal(err)
				}
				if before.Actor.Battle.EnemyRank != expectedRank {
					t.Fatal("author update replaced frozen rank")
				}
				fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: before.Head, Title: "Before the exchange"})
				if err != nil {
					t.Fatal(err)
				}
				after, err := store.CommitAdventureAction(id, "main", "period-round", before.Head, AdventureAction{Action: "battle_round", Choice: "attack"})
				if err != nil {
					t.Fatal(err)
				}
				ctx, err := store.StoryContext(id, "main")
				if err != nil {
					t.Fatal(err)
				}
				npc, err := AdventureActorFromState(ctx.Snapshot.State, actorID)
				if err != nil || npc.Rank != expectedRank || npc.Health != after.Actor.Battle.Health {
					t.Fatalf("bound settlement diverged: %v %+v", err, npc)
				}
				reopened := NewStore(store.Root())
				restored, err := reopened.AdventureWindow(id, "main")
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(restored.Actor, after.Actor) {
					t.Fatal("cold reopen changed actor or battle")
				}
				prior, err := reopened.AdventureWindow(id, fork.ID)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(prior.Actor, before.Actor) {
					t.Fatal("battle result leaked into earlier branch")
				}
				reopened.Close()
			}
		})
	}
}
