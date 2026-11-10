package interactive

import (
	"fmt"
	"testing"
)

func TestAdventureBattleClockSurvivesLaterForkAndRegeneration(t *testing.T) {
	s, id := adventureTestStory(t)
	defer s.Close()
	_, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "迎战", Narrative: "对手上前。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "迎战", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "rival"}}}})
	if err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 10; round++ {
		w, e := s.AdventureWindow(id, "main")
		if e != nil {
			t.Fatal(e)
		}
		if w.Actor.Battle.Status != "active" {
			break
		}
		if _, e = s.CommitAdventureAction(id, "main", fmt.Sprintf("battle-clock-%d", round), w.Head, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "strike"}); e != nil {
			t.Fatal(e)
		}
	}
	battle, e := s.Snapshot(id, "main")
	if e != nil {
		t.Fatal(e)
	}
	if battle.Clock == nil || battle.Clock.Seconds != 60 {
		t.Fatalf("expected three actual battle rounds: %+v", battle.Clock)
	}
	req := AppendTurnWithStateRequest{BranchID: "main", User: "休养一小时", Narrative: "你安顿下来休息。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "休养一小时", Adventure: &AdventureAction{Action: "rest", Hours: 1}}}}
	rest, _, e := s.AppendTurnWithState(id, req)
	if e != nil {
		t.Fatal(e)
	}
	fork, e := s.CreateBranch(id, CreateBranchRequest{ParentEventID: rest.ID, Title: "After battle and rest"})
	if e != nil {
		t.Fatal(e)
	}
	restored, e := s.Snapshot(id, fork.ID)
	if e != nil || restored.Clock == nil || restored.Clock.Seconds != 3660 {
		t.Fatalf("fork lost battle time: %+v %v", restored.Clock, e)
	}
	w, e := s.AdventureWindow(id, "main")
	if e != nil {
		t.Fatal(e)
	}
	req.ExpectedParentID = &w.Head
	req.ReplaceTurnID = rest.ID
	req.User = "休养两小时"
	req.TurnResult.World.SourceText = req.User
	req.TurnResult.World.Adventure.Hours = 2
	if _, _, e = s.AppendTurnWithState(id, req); e != nil {
		t.Fatal(e)
	}
	changed, e := s.Snapshot(id, "main")
	if e != nil || changed.Clock == nil || changed.Clock.Seconds != 7260 {
		t.Fatalf("regeneration lost or doubled battle time: %+v %v", changed.Clock, e)
	}
}
