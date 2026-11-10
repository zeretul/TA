package interactive

import (
	"reflect"
	"testing"
)

func TestFantasyCryptInvestigationKeepsDistinctKeyOutcomes(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-smith")
	a.Inventory["sg-iron"] = 2
	a.Inventory["sg-crystal"] = 1
	s, id := fantasyBoundStory(t, r, a, "倒旗墓道")
	defer s.Close()
	fantasyMeet(t, s, id, "main", "sg-p12", "joven")
	task := "sg-crypt-inquiry"
	questApplyOn(t, s, id, "main", task, QuestActionAccept, "", "accept-crypt")
	before, e := s.AdventureWindow(id, "main")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: task, Action: QuestActionChoose, ChoiceID: task + ".stage2.c1", CommandID: "skip", CommandHash: "skip"}); e == nil {
		t.Fatal("skipped investigation")
	}
	after, e := s.AdventureWindow(id, "main")
	if e != nil || !reflect.DeepEqual(before.Actor, after.Actor) || before.ClockSeconds != after.ClockSeconds {
		t.Fatal("refused skip spent resources", e)
	}
	questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".stage1.c2", "ask-keeper")
	if _, e = s.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: task, Action: QuestActionChoose, ChoiceID: task + ".stage2.c2", CommandID: "fake-victory", CommandHash: "fake-victory"}); e == nil {
		t.Fatal("prose substituted for actual combat victory")
	}
	questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".stage2.c1", "repair-muster")
	owned, e := s.AdventureWindow(id, "main")
	if e != nil || owned.Actor.Inventory["sg-oath-key"] != 1 || owned.Actor.Inventory["sg-iron"] != 0 || owned.Actor.Inventory["sg-crystal"] != 0 || owned.Actor.Inventory["sg-forge-tools"] != 1 {
		t.Fatal("repair failed to settle real materials and source key", e)
	}
	anchor, _, e := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "想一想钥匙的去处", Narrative: "旧闸钥仍在你手里。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if e != nil {
		t.Fatal(e)
	}
	fork, e := s.CreateBranch(id, CreateBranchRequest{ParentEventID: anchor.ID, Title: "Keep the key"})
	if e != nil {
		t.Fatal(e)
	}
	questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".stage3.c1", "return-key")
	questApplyOn(t, s, id, fork.ID, task, QuestActionChoose, task+".stage3.c2", "keep-key")
	main, e := s.AdventureWindow(id, "main")
	if e != nil || main.Actor.Inventory["sg-oath-key"] != 0 {
		t.Fatal("returned key remained owned", e)
	}
	kept, e := s.AdventureWindow(id, fork.ID)
	if e != nil || kept.Actor.Inventory["sg-oath-key"] != 1 {
		t.Fatal("alternate key outcome leaked", e)
	}
	root := s.Root()
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	cold := NewStore(root)
	defer cold.Close()
	kept, e = cold.AdventureWindow(id, fork.ID)
	if e != nil || kept.Actor.Inventory["sg-oath-key"] != 1 {
		t.Fatal("cold restore lost branch item", e)
	}
}

func TestFantasyWatergateRequiresActualTravelBeforeSellingLedger(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-smith")
	a.Inventory["sg-rope"] = 1
	s, id := fantasyBoundStory(t, r, a, "沉王水宫")
	defer s.Close()
	fantasyMeet(t, s, id, "main", "sg-p35", "asha")
	task := "sg-watergate-inquiry"
	questApplyOn(t, s, id, "main", task, QuestActionAccept, "", "accept-watergate")
	questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".stage1.c2", "route-with-diver")
	questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".stage2.c1", "retrieve-box")
	before, e := s.AdventureWindow(id, "main")
	if e != nil || before.Actor.Inventory["sg-drowned-ledger"] != 1 || before.Actor.Inventory["sg-rope"] != 0 {
		t.Fatal("salvage source or rope not settled", e)
	}
	if _, e = s.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: task, Action: QuestActionChoose, ChoiceID: task + ".stage3.c2", CommandID: "remote-sale", CommandHash: "remote-sale"}); e == nil {
		t.Fatal("sold ledger to absent recipient in another location")
	}
	for _, route := range []string{"sg-route-l35-l31", "sg-route-l31-l34"} {
		if _, e = s.ApplyJourney(id, JourneyRequest{BranchID: "main", RouteID: route, Arrive: true, CommandID: route, CommandHash: route}); e != nil {
			t.Fatal(e)
		}
	}
	fantasyMeet(t, s, id, "main", "sg-p34", "mute")
	paid := questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".stage3.c2", "sell-original")
	after, e := s.AdventureWindow(id, "main")
	if e != nil || after.Actor.Currency != 49 || after.Actor.Inventory["sg-drowned-ledger"] != 0 || after.ClockSeconds != before.ClockSeconds+18000+3600 {
		t.Fatalf("local sale failed: %+v %v", after, e)
	}
	retry := questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".stage3.c2", "sell-original")
	if !retry.Replayed || retry.EventID != paid.EventID {
		t.Fatal("sale paid twice")
	}
}
