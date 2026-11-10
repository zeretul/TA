package interactive

import (
	"reflect"
	"testing"
)

func TestAdventureWithdrawCannotRemoveAnotherDeployedAsset(t *testing.T) {
	r := adventureTestRules()
	r.Items = append(r.Items, AdventureItem{ID: "other-ship", Name: "Other ship", Kind: "ship", Scale: "ship", Durability: 100, Acquisition: "craft"})
	r.StartingItems["other-ship"] = 1
	a := NewAdventureActor(r)
	for _, id := range []string{"ship", "other-ship"} {
		var err error
		a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "deploy", ItemID: id}, "deploy-"+id)
		if err != nil {
			t.Fatal(err)
		}
	}
	next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "withdraw", ItemID: "ship"}, "withdraw-inactive")
	if err == nil || !reflect.DeepEqual(next, a) {
		t.Fatalf("withdrawing an inactive asset changed the deployed ship: %v %+v", err, next.Equipment)
	}
}

func TestAdventureInoperableActiveAssetCanOnlySettleWithdrawalAsLoss(t *testing.T) {
	for _, condition := range []string{"empty", "destroyed", "disabled"} {
		t.Run(condition, func(t *testing.T) {
			r := adventureTestRules()
			a := NewAdventureActor(r)
			a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "deploy", ItemID: "ship"}, "deploy")
			if err != nil {
				t.Fatal(err)
			}
			a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "frigate"}, "start")
			if err != nil {
				t.Fatal(err)
			}
			asset := a.Assets["ship"]
			switch condition {
			case "empty":
				asset.Fuel = 0
			case "destroyed":
				asset.Health = 0
			case "disabled":
				asset.DisabledUntil = 100
			}
			a.Assets["ship"] = asset
			if _, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "forged-attack"); err == nil {
				t.Fatal("inoperable asset attacked")
			}
			next, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "escape"}, "withdraw")
			if err != nil || next.Battle.Status != "defeat" || next.Assets["ship"] != asset || next.Currency != a.Currency || next.Experience != a.Experience || out.DurationSeconds != adventureRoundSeconds {
				t.Fatalf("withdrawal granted success or free recovery: %v %+v", err, next)
			}
		})
	}
}

// An isolated low-fuel rule fixture, not normal acquisition or model gameplay.
func TestAdventureFuelExhaustionSettlesDefeatAndAllowsPaidRecovery(t *testing.T) {
	r := adventureTestRules()
	a := NewAdventureActor(r)
	a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "deploy", ItemID: "ship"}, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	asset := a.Assets["ship"]
	asset.Fuel = 1
	a.Assets["ship"] = asset
	s, id := adventureFixtureStoryWithActor(t, r, a, "船坞")
	defer s.Close()
	_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "迎战敌舰", Narrative: "敌舰进入视线。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "迎战敌舰", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "frigate"}}}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	in := AdventureAction{Action: "battle_round", Choice: "guard"}
	result, err := s.CommitAdventureAction(id, "main", "last-fuel", before.Head, in)
	if err != nil {
		t.Fatal(err)
	}
	if result.Actor.Battle.Status != "defeat" || result.Actor.Assets["ship"].Fuel != 0 || result.Actor.Assets["ship"].Health <= 0 || result.TerminalOutcome != nil || result.Actor.Defeated["frigate"] {
		t.Fatalf("fuel loss did not settle a recoverable, unrewarded defeat: %+v", result)
	}
	retry, err := s.CommitAdventureAction(id, "main", "last-fuel", before.Head, in)
	if err != nil || !reflect.DeepEqual(result, retry) {
		t.Fatalf("retry changed fuel result: %v", err)
	}
	root := s.Root()
	_ = s.Close()
	s = NewStore(root)
	defer s.Close()
	cold, err := s.AdventureWindow(id, "main")
	if err != nil || !reflect.DeepEqual(result.Actor, cold.Actor) {
		t.Fatalf("cold fuel state changed: %v", err)
	}
	if _, err = s.CommitAdventureAction(id, "main", "free-attack", cold.Head, AdventureAction{Action: "battle_round", Choice: "attack"}); err == nil {
		t.Fatal("continued attacking without fuel")
	}
	_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "维修补给一小时", Narrative: "你用一份零件检修舰船。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "维修补给一小时", Adventure: &AdventureAction{Action: "repair", TargetID: "ship", SkillID: "repair", Hours: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	repaired, err := s.AdventureWindow(id, "main")
	if err != nil || repaired.Actor.Inventory["scrap"] != a.Inventory["scrap"]-1 || repaired.Actor.Assets["ship"].Fuel != 10 || repaired.ClockSeconds != cold.ClockSeconds+3600 {
		t.Fatalf("recovery skipped material/time costs: %v %+v", err, repaired)
	}
	branch, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: before.Head, Title: "Before fuel loss"})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := s.AdventureWindow(id, branch.ID)
	if err != nil || fork.Actor.Assets["ship"].Fuel != 1 || fork.Actor.Battle.Status != "active" {
		t.Fatalf("later repair leaked to branch: %v %+v", err, fork.Actor)
	}
}
