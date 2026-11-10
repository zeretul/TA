package interactive

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// This is a deterministic rule fixture, not earned or model-driven gameplay.
func adventureWorkTestRules(t *testing.T) AdventureRules {
	t.Helper()
	r := adventureTestRules()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err = json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["markets"].([]any)[0].(map[string]any)["work_service"] = map[string]any{
		"name": "整理船材", "hours": 2, "pay": 15, "energy_cost": 10,
	}
	data, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAdventureWorkSettlesEarnedPayAndTime(t *testing.T) {
	r := adventureWorkTestRules(t)
	a := NewAdventureActor(r)
	next, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "work", MarketID: "dock", Hours: 2}, "work")
	if err != nil {
		t.Fatal(err)
	}
	if next.Currency != a.Currency+15 || next.Energy != a.Energy-10 || out.DurationSeconds != 7200 {
		t.Fatalf("work did not settle its authored wage, effort and time: actor=%+v outcome=%+v", next, out)
	}
	unchanged := NewAdventureActor(r)
	if !reflect.DeepEqual(a, unchanged) {
		t.Fatal("work mutated its original actor")
	}
}

func TestAdventureWorkRejectionsAreAtomic(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*AdventureRules, *AdventureActor, *AdventureAction)
	}{
		{"unknown market", func(_ *AdventureRules, _ *AdventureActor, in *AdventureAction) { in.MarketID = "elsewhere" }},
		{"no local work", func(r *AdventureRules, _ *AdventureActor, _ *AdventureAction) { r.Markets[0].WorkService = nil }},
		{"no authorized time", func(_ *AdventureRules, _ *AdventureActor, in *AdventureAction) { in.Hours = 0 }},
		{"too much time", func(_ *AdventureRules, _ *AdventureActor, in *AdventureAction) { in.Hours = 100001 }},
		{"exhausted", func(_ *AdventureRules, a *AdventureActor, _ *AdventureAction) { a.Energy = 9 }},
		{"injured", func(_ *AdventureRules, a *AdventureActor, _ *AdventureAction) { a.Injury = 50 }},
		{"unlearned skill", func(r *AdventureRules, _ *AdventureActor, _ *AdventureAction) {
			r.Markets[0].WorkService.RequiredSkill = "advanced"
			r.Markets[0].WorkService.SkillRank = 1
		}},
		{"insufficient mastery", func(r *AdventureRules, _ *AdventureActor, _ *AdventureAction) {
			r.Markets[0].WorkService.RequiredSkill = "repair"
			r.Markets[0].WorkService.SkillRank = 2
		}},
		{"wallet limit", func(_ *AdventureRules, a *AdventureActor, _ *AdventureAction) { a.Currency = 999999990 }},
		{"active battle", func(_ *AdventureRules, a *AdventureActor, _ *AdventureAction) {
			a.Battle = &AdventureBattle{Status: "active"}
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			r := adventureWorkTestRules(t)
			a := NewAdventureActor(r)
			in := AdventureAction{Action: "work", MarketID: "dock", Hours: 2}
			scenario.change(&r, &a, &in)
			before, _ := json.Marshal(a)
			after, _, err := ResolveAdventureAction(r, a, in, "refused-work")
			if err == nil {
				t.Fatal("invalid work was accepted")
			}
			got, _ := json.Marshal(after)
			unchanged, _ := json.Marshal(a)
			if string(before) != string(got) || string(before) != string(unchanged) {
				t.Fatal("refusal changed actor state")
			}
		})
	}
}

func TestAdventureWorkRateCannotBeMultipliedBySplitting(t *testing.T) {
	r := adventureWorkTestRules(t)
	r.Markets[0].WorkService.EnergyCost = 9
	a := NewAdventureActor(r)
	whole, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "work", MarketID: "dock", Hours: 2}, "whole")
	if err != nil {
		t.Fatal(err)
	}
	part, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "work", MarketID: "dock", Hours: 1}, "part-one")
	if err != nil {
		t.Fatal(err)
	}
	part, _, err = ResolveAdventureAction(r, part, AdventureAction{Action: "work", MarketID: "dock", Hours: 1}, "part-two")
	if err != nil {
		t.Fatal(err)
	}
	if part.Currency > whole.Currency || part.Energy > whole.Energy {
		t.Fatal("splitting minted wages or conserved required effort")
	}
	if whole.Currency != a.Currency+15 || part.Currency != a.Currency+14 || whole.Energy != a.Energy-9 || part.Energy != a.Energy-10 {
		t.Fatal("work rate rounding changed")
	}
}

func TestAdventureWorkCatalogRejectsInvalidTerms(t *testing.T) {
	for _, change := range []func(*AdventureWorkService){
		func(w *AdventureWorkService) { w.Name = "" },
		func(w *AdventureWorkService) { w.Hours = 0 },
		func(w *AdventureWorkService) { w.Pay = 0 },
		func(w *AdventureWorkService) { w.EnergyCost = 0 },
		func(w *AdventureWorkService) { w.RequiredSkill = "not-authored"; w.SkillRank = 1 },
		func(w *AdventureWorkService) { w.SkillRank = 1 },
	} {
		r := adventureWorkTestRules(t)
		change(r.Markets[0].WorkService)
		if r.Validate() == nil {
			t.Fatal("invalid work service was accepted")
		}
	}
}

func TestAdventureWorkStoryPayClockRetryColdReplayAndBranch(t *testing.T) {
	r := adventureWorkTestRules(t)
	r.StartingCurrency = 0
	s, id := adventureTestStoryWithRules(t, r)
	defer s.Close()
	before, err := s.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: before.CurrentTurn.ID, Title: "No work yet"})
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "ordinary-work", OperationID: "work-turn", Cycle: 1}
	input, err := NewPlayerInputIntent(identity, "main", "在船坞整理零件两小时")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CommitPlayerInput(id, input); err != nil {
		t.Fatal(err)
	}
	req := AppendTurnWithStateRequest{BranchID: "main", User: input.Text, Narrative: "你整理完这批零件，领到了这段工作的报酬。", AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle, TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: input.Text, Adventure: &AdventureAction{Action: "work", MarketID: "dock", Hours: 2}}}}
	first, _, err := s.AppendTurnWithState(id, req)
	if err != nil {
		t.Fatal(err)
	}
	retry, _, err := s.AppendTurnWithState(id, req)
	if err != nil || retry.ID != first.ID {
		t.Fatalf("work command was not idempotent: %v", err)
	}
	work, err := s.AdventureWindow(id, "main")
	if err != nil || work.Actor.Currency != 15 || work.Actor.Energy != 90 || work.ClockSeconds != 7200 {
		t.Fatalf("formal work state: %+v %v", work, err)
	}
	if work.Actor.Experience != 0 || work.Actor.Rank != 1 || !reflect.DeepEqual(work.Actor.Skills, NewAdventureActor(r).Skills) {
		t.Fatal("ordinary wages granted experience or qualifications")
	}
	old, err := s.AdventureWindow(id, fork.ID)
	if err != nil || old.Actor.Currency != 0 || old.Actor.Energy != 100 || old.ClockSeconds != 0 {
		t.Fatal("work leaked into the earlier branch")
	}
	root := s.Root()
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = NewStore(root)
	defer s.Close()
	retry, _, err = s.AppendTurnWithState(id, req)
	if err != nil || retry.ID != first.ID {
		t.Fatalf("cold retry paid twice: %v", err)
	}
	cold, err := s.AdventureWindow(id, "main")
	if err != nil || !reflect.DeepEqual(work.Actor, cold.Actor) || cold.ClockSeconds != work.ClockSeconds {
		t.Fatalf("cold replay changed wages or time: %v", err)
	}
}

func TestAdventureWorkRequiresTheCommittedSceneAndPlayerSource(t *testing.T) {
	for _, scenario := range []struct {
		name, location, source string
		hazard                 bool
	}{
		{"remote workplace", "别处", "做两小时短工", false},
		{"invented permission", "船坞", "模型自己决定工作两小时", false},
		{"missing player source", "船坞", "", false},
		{"interrupted by hazard", "船坞", "做两小时短工", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			r := adventureWorkTestRules(t)
			a := NewAdventureActor(r)
			if scenario.hazard {
				scenario.location = "悬崖岩壁黑市回廊"
				r.Markets[0].Location = scenario.location
				a.OpeningHazards = []AdventureOpeningHazard{{ID: "vent", DueSeconds: 1800, LocationIDs: []string{"xinghai-l17"}, EscapeLocationID: "xinghai-l02", Damage: 10, Injury: 5, Description: "Pressure vent", Consequence: "Evacuate"}}
			}
			s, id := adventureFixtureStoryWithActor(t, r, a, scenario.location)
			defer s.Close()
			before, _ := s.AdventureWindow(id, "main")
			_, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "做两小时短工", Narrative: "这是不得成立的报酬。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: scenario.source, Adventure: &AdventureAction{Action: "work", MarketID: "dock", Hours: 2}}}})
			if err == nil {
				t.Fatal("unauthorized, remote or interrupted work committed")
			}
			after, _ := s.AdventureWindow(id, "main")
			if !reflect.DeepEqual(before, after) {
				t.Fatal("rejected work changed head, clock or resources")
			}
		})
	}
}

func TestAdventureWorkAuthoredIncomeCanFundRadioWithoutGrantingIt(t *testing.T) {
	r := authoredQuestRules(t, "xinghai-689")
	r.StartingCurrency = 0
	a := NewAdventureActor(r)
	beforeInventory := map[string]int{}
	for id, n := range a.Inventory {
		beforeInventory[id] = n
	}
	for i := 0; i < 2; i++ {
		var err error
		a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "work", MarketID: "xinghai-market-l02", Hours: 8}, fmt.Sprintf("income-%d", i))
		if err != nil {
			t.Fatal(err)
		}
	}
	if a.Currency != 120 || a.Energy != 20 || !reflect.DeepEqual(a.Inventory, beforeInventory) {
		t.Fatal("work did not earn wages without granting equipment")
	}
	a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "buy", MarketID: "xinghai-market-l02", ItemID: "xinghai-radio"}, "buy-radio")
	if err != nil || a.Currency != 0 || a.Inventory["xinghai-radio"] != 1 {
		t.Fatalf("earned pay cannot buy the registered radio: %v", err)
	}
	if !strings.Contains(AdventureRulesContext(r), "work_service") {
		t.Fatal("local work is missing from the normal model rule context")
	}
}

func TestAdventureWorkDoesNotRetrofitFrozenStories(t *testing.T) {
	r := adventureWorkTestRules(t)
	service := r.Markets[0].WorkService
	r.Markets[0].WorkService = nil
	old, oldID := adventureTestStoryWithRules(t, r)
	defer old.Close()
	// Simulate the next author revision. The old Story already froze its own
	// catalog and must neither acquire a new wage source nor be rewritten.
	r.Markets[0].WorkService = service
	newer, newID := adventureTestStoryWithRules(t, r)
	defer newer.Close()
	oldWindow, err := old.AdventureWindow(oldID, "main")
	if err != nil {
		t.Fatal(err)
	}
	newWindow, err := newer.AdventureWindow(newID, "main")
	if err != nil {
		t.Fatal(err)
	}
	in := AdventureAction{Action: "work", MarketID: "dock", Hours: 2}
	if _, _, err = ResolveAdventureAction(oldWindow.Rules, oldWindow.Actor, in, "old"); err == nil {
		t.Fatal("a new service leaked into the old frozen Story")
	}
	if _, _, err = ResolveAdventureAction(newWindow.Rules, newWindow.Actor, in, "new"); err != nil {
		t.Fatal(err)
	}
	if oldWindow.Rules.Markets[0].WorkService != nil {
		t.Fatal("old catalog was rewritten")
	}
}

func TestAdventureWorkAuthoredRevisionRetainsExistingTasks(t *testing.T) {
	for _, packID := range []string{"lanchuan-qunxia", "xinghai-689"} {
		data, err := os.ReadFile("../storypack/assets/" + packID + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var pack struct {
			ContentVersion int `json:"content_version"`
		}
		if err = json.Unmarshal(data, &pack); err != nil {
			t.Fatal(err)
		}
		meta := StoryMeta{PackBinding: &StoryPackBinding{PackID: packID, ContentVersion: pack.ContentVersion}, ActorStateSchema: &ActorStateSchemaSnapshot{System: StoryDirectorActorStateSystem{WorldContent: authoredWorldContent(t, packID)}}}
		if !storyPackAllows(meta, packID, "1") {
			t.Fatalf("new work revision hid existing %s tasks", packID)
		}
		r := authoredQuestRules(t, packID)
		found := false
		for _, market := range r.Markets {
			if market.WorkService == nil {
				continue
			}
			found = true
			a := NewAdventureActor(r)
			if market.WorkService.RequiredSkill != "" && a.Skills[market.WorkService.RequiredSkill] < market.WorkService.SkillRank {
				continue
			}
			next, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "work", MarketID: market.ID, Hours: market.WorkService.Hours}, "authored-work")
			if err != nil || next.Currency <= a.Currency || out.DurationSeconds <= 0 {
				t.Fatalf("authored work source %s does not settle: %v", market.ID, err)
			}
		}
		if !found {
			t.Fatalf("no local livelihood in %s", packID)
		}
	}
}
