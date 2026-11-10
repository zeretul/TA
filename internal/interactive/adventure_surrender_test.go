package interactive

import (
	"reflect"
	"testing"
)

func TestWuxiaSurrenderCommitsLossAndContinuesLifeAcrossReplay(t *testing.T) {
	r := authoredAdventureRules(t, "lanchuan-qunxia")
	r.NPCs = nil
	r.Opponents = []AdventureOpponent{{ID: "sparring", Name: "切磋对手", Location: "船坞", Scale: "person", Health: 100, Attack: 10, RewardCurrency: 20, RewardExperience: 20}}
	a := NewAdventureActor(r)
	a.Health, a.Energy, a.Injury = 70, 40, 12
	s, id := adventureFixtureStoryWithActor(t, r, a, "船坞")
	defer s.Close()
	_, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "与对手切磋", Narrative: "两人站定。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "与对手切磋", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "sparring"}}}})
	if err != nil {
		t.Fatal(err)
	}
	window, _ := s.AdventureWindow(id, "main")
	if _, err = s.CommitAdventureAction(id, "main", "guard", window.Head, AdventureAction{Action: "battle_round", Choice: "guard"}); err != nil {
		t.Fatal(err)
	}
	before, _ := s.AdventureWindow(id, "main")
	in := AdventureAction{Action: "battle_round", Choice: "surrender"}
	result, err := s.CommitAdventureAction(id, "main", "concede", before.Head, in)
	if err != nil {
		t.Fatal(err)
	}
	if result.Actor.Battle.Status != "defeat" || result.Actor.Defeated["sparring"] || result.Actor.Health != before.Actor.Health || result.Actor.Injury != before.Actor.Injury || result.Actor.Energy != before.Actor.Energy || result.Actor.Currency != before.Actor.Currency || result.Actor.Experience != before.Actor.Experience || result.Outcome.DurationSeconds != adventureRoundSeconds || result.TerminalOutcome != nil {
		t.Fatalf("surrender changed costs, wounds or outcome: %+v", result)
	}
	retry, err := s.CommitAdventureAction(id, "main", "concede", before.Head, in)
	if err != nil || !reflect.DeepEqual(result, retry) {
		t.Fatalf("surrender retry changed settlement: %v", err)
	}
	root := s.Root()
	_ = s.Close()
	s = NewStore(root)
	defer s.Close()
	cold, err := s.AdventureWindow(id, "main")
	if err != nil || !reflect.DeepEqual(cold.Actor, result.Actor) || cold.ClockSeconds != before.ClockSeconds+adventureRoundSeconds {
		t.Fatalf("surrender replay changed state/time: %v", err)
	}
	_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "休养一小时再继续生活", Narrative: "你坐下调息。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "休养一小时再继续生活", Adventure: &AdventureAction{Action: "rest", Hours: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.AdventureWindow(id, "main")
	if err != nil || after.ClockSeconds != cold.ClockSeconds+3600 || after.Actor.Health != min(after.Actor.MaxHealth, cold.Actor.Health+10) || after.Actor.Injury != max(0, cold.Actor.Injury-2) {
		t.Fatalf("ordinary life did not continue after conceding: %v %+v", err, after)
	}
	branch, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: window.Head, Title: "Before the exchanges"})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := s.AdventureWindow(id, branch.ID)
	if err != nil || !reflect.DeepEqual(fork.Actor, window.Actor) || fork.ClockSeconds != window.ClockSeconds {
		t.Fatalf("surrender/rest leaked to earlier branch: %v", err)
	}
}

func TestWuxiaSurrenderCannotBypassLethalOrOtherGenreEncounters(t *testing.T) {
	wuxia, actor := wuxiaConditionFixture(t)
	actor.Control, actor.SoftLimbs = 1, true
	allowed, _, err := ResolveAdventureAction(wuxia, actor, AdventureAction{Action: "battle_round", Choice: "surrender"}, "concede-controlled")
	if err != nil || allowed.Battle.Status != "defeat" {
		t.Fatalf("nonviolent concession was turned into a forced combat action: %v", err)
	}
	actor.Battle.Lethal = true
	if _, _, err = ResolveAdventureAction(wuxia, actor, AdventureAction{Action: "battle_round", Choice: "surrender"}, "lethal-concession"); err == nil {
		t.Fatal("concession made a lethal encounter safe")
	}
	space := adventureTestRules()
	a := NewAdventureActor(space)
	a, _, err = ResolveAdventureAction(space, a, AdventureAction{Action: "battle_start", OpponentID: "rival"}, "space-start")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = ResolveAdventureAction(space, a, AdventureAction{Action: "battle_round", Choice: "surrender"}, "space-concession"); err == nil {
		t.Fatal("wuxia concession leaked to other genres")
	}
}
