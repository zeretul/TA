package interactive

import (
	"reflect"
	"testing"
)

func TestJourneyCannotReplaceBattleEscapeIter4(t *testing.T) {
	for _, adventure := range []bool{false, true} {
		name := "cultivation"
		if adventure {
			name = "adventure"
		}
		t.Run(name, func(t *testing.T) {
			s, id := realmExploreSafetyStory(t, adventure)
			if adventure {
				worldFlowTurn(t, s, id, "迎战", "对手上前。", TurnWorldActionInput{Action: "adventure", SourceText: "迎战", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "rival"}})
			} else {
				if _, err := s.StartBattle(id, BattleStartRequest{BranchID: "main", Combatants: combatTestSpecs(), CommandID: "battle", CommandHash: "battle"}); err != nil {
					t.Fatal(err)
				}
			}
			before, err := s.Snapshot(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.ApplyJourney(id, JourneyRequest{BranchID: "main", RouteID: "route_036:reverse", Arrive: true, CommandID: "walk-out-of-battle", CommandHash: "walk-out-of-battle"})
			if err == nil {
				t.Fatal("ordinary road travel moved an active combatant without the real escape action")
			}
			_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "走到坊市", Narrative: "走到坊市。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: TurnWorldActionJourney, RouteID: "route_036:reverse", Arrive: true}}})
			if err == nil {
				t.Fatal("main-turn travel replaced the actual battle escape")
			}
			after, err := s.Snapshot(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(before.Clock, after.Clock) {
				t.Fatal("refused battle travel changed state/time")
			}
		})
	}
}
