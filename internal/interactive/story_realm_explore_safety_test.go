package interactive

import (
	"errors"
	"testing"

	"ta/internal/exploration"
)

// These are isolated Store regression fixtures, not natural-AI play evidence.
func realmExploreSafetyStory(t *testing.T, adventure bool) (*Store, string) {
	t.Helper()
	if adventure {
		r := adventureTestRules()
		r.Opponents[0].Location = realmTestOrigin
		s, id := adventureFixtureStoryWithActor(t, r, NewAdventureActor(r), realmTestOrigin)
		t.Cleanup(func() { _ = s.Close() })
		return s, id
	}
	system := combatTestSystem()
	for i := range system.Templates {
		if system.Templates[i].ID == ActorStateStoryContextTemplateID {
			system.Templates[i].Fields = append(system.Templates[i].Fields, ActorStateField{Name: storySceneLocationField, Type: "string", Default: realmTestOrigin})
		}
	}
	for i := range system.InitialActors {
		if system.InitialActors[i].ID == DefaultStoryContextActorID {
			system.InitialActors[i].State = map[string]any{storySceneLocationField: realmTestOrigin}
		}
	}
	s := NewStore(t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	story, err := s.CreateStory(CreateStoryRequest{Title: "Realm safety fixture", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "观察", Narrative: "站在洞口。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
		t.Fatal(err)
	}
	return s, story.ID
}

func realmExploreReachLegacyExit(t *testing.T, s *Store, id, instance string) {
	t.Helper()
	for _, area := range []string{"realm.formation_room", realmTestAlchemy, realmTestPassage} {
		realmTestApply(t, s, id, RealmRequest{InstanceID: instance, Action: RealmActionMove, DestinationAreaID: area, CommandID: "walk-" + area, CommandHash: "walk-" + area})
	}
}

func TestRealmExploreReenterRequiresActualEntrance(t *testing.T) {
	for _, action := range []string{RealmActionReenter, RealmActionEnter} {
		t.Run(action, func(t *testing.T) {
			s, id := realmTestStory(t)
			entered := realmTestEnter(t, s, id)
			realmExploreReachLegacyExit(t, s, id, entered.InstanceID)
			realmTestApply(t, s, id, RealmRequest{InstanceID: entered.InstanceID, Action: RealmActionExit, CommandID: "leave", CommandHash: "leave"})
			journeyTestApply(t, s, id, JourneyRequest{RouteID: "route_036:reverse", Arrive: true, CommandID: "away", CommandHash: "away"})
			before := realmTestRecordCount(t, s, id, "main")
			_, err := s.ApplyRealm(id, RealmRequest{BranchID: "main", RealmID: exploration.RealmGuXiuDongFu, InstanceID: entered.InstanceID, Action: action, CommandID: "far-reentry", CommandHash: "far-reentry"})
			if !errors.Is(err, ErrRealmNotAtEntrance) {
				t.Fatalf("outside re-entry from the distant market must be rejected, got %v", err)
			}
			if got := realmTestRecordCount(t, s, id, "main"); got != before {
				t.Fatalf("rejected re-entry committed records: %d -> %d", before, got)
			}
			if got := journeyTestLocation(t, s, id, "main"); got != "公共坊市" {
				t.Fatalf("rejected re-entry moved the actor: %q", got)
			}
		})
	}
}

func TestRealmExploreActiveBattleBlocksMovement(t *testing.T) {
	for _, adventure := range []bool{false, true} {
		name := "Combat"
		if adventure {
			name = "AdventureBattle"
		}
		for _, action := range []string{RealmActionEnter, RealmActionMove, RealmActionExit} {
			t.Run(name+"/"+action, func(t *testing.T) {
				s, id := realmExploreSafetyStory(t, adventure)
				instance := ""
				if action != RealmActionEnter {
					instance = realmTestEnter(t, s, id).InstanceID
					if action == RealmActionExit {
						realmExploreReachLegacyExit(t, s, id, instance)
					}
				}
				if adventure {
					worldFlowTurn(t, s, id, "迎战", "对手上前。", TurnWorldActionInput{Action: "adventure", SourceText: "迎战", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "rival"}})
					w, err := s.AdventureWindow(id, "main")
					if err != nil {
						t.Fatal(err)
					}
					if w.Actor.Battle == nil || w.Actor.Battle.Status != "active" {
						t.Fatal("fixture battle must be active")
					}
				} else {
					result, err := s.StartBattle(id, BattleStartRequest{BranchID: "main", Combatants: combatTestSpecs(), CommandID: "battle", CommandHash: "battle"})
					if err != nil {
						t.Fatal(err)
					}
					if result.Session.Ended {
						t.Fatal("fixture battle must be active")
					}
				}
				before := realmTestRecordCount(t, s, id, "main")
				location := journeyTestLocation(t, s, id, "main")
				_, err := s.ApplyRealm(id, RealmRequest{BranchID: "main", RealmID: exploration.RealmGuXiuDongFu, InstanceID: instance, Action: action, DestinationAreaID: realmTestPlot, CommandID: "during-battle", CommandHash: "during-battle"})
				if err == nil {
					t.Fatal("active battle must block realm movement; action was accepted")
				}
				if got := realmTestRecordCount(t, s, id, "main"); got != before {
					t.Fatalf("battle-blocked action committed records: %d -> %d", before, got)
				}
				if got := journeyTestLocation(t, s, id, "main"); got != location {
					t.Fatalf("battle-blocked action moved actor: %q -> %q", location, got)
				}
			})
		}
	}
}
