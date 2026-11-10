package interactive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func fantasyOrigin(t *testing.T, id string) (AdventureRules, AdventureActor) {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join("..", "storypack", "assets", "sui-guan-realms.json"))
	if e != nil {
		t.Fatal(e)
	}
	var p struct {
		Story struct {
			Gameplay struct {
				Rules AdventureRules `json:"adventure_rules"`
			} `json:"gameplay"`
		} `json:"story"`
		Origins []struct {
			ID    string                     `json:"id"`
			State map[string]json.RawMessage `json:"state"`
		} `json:"protagonist_templates"`
	}
	if e = json.Unmarshal(raw, &p); e != nil {
		t.Fatal(e)
	}
	if e = p.Story.Gameplay.Rules.Validate(); e != nil {
		t.Fatal(e)
	}
	p.Story.Gameplay.Rules.world = authoredWorldContent(t, "sui-guan-realms")
	for _, o := range p.Origins {
		if o.ID == id {
			var a AdventureActor
			if e = json.Unmarshal(o.State[AdventureStateField], &a); e != nil {
				t.Fatal(e)
			}
			return p.Story.Gameplay.Rules, a
		}
	}
	// Released origins remain useful recovery and rule fixtures after the new-game menu is simplified.
	legacy, err := os.ReadFile(filepath.Join("testdata", "fantasy-released-origins.json"))
	if err != nil {
		t.Fatal(err)
	}
	var released map[string]AdventureActor
	if err := json.Unmarshal(legacy, &released); err != nil {
		t.Fatal(err)
	}
	if actor, ok := released[id]; ok {
		return p.Story.Gameplay.Rules, actor
	}
	t.Fatal("missing authored origin")
	return AdventureRules{}, AdventureActor{}
}

func TestFantasyEarnBuyLearnCraftAndBattle(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-road")
	initial := a
	apply := func(in AdventureAction) {
		t.Helper()
		next, _, e := ResolveAdventureAction(r, a, in, "fantasy-campaign")
		if e != nil {
			t.Fatalf("%+v: %v", in, e)
		}
		a = next
	}
	apply(AdventureAction{Action: "work", MarketID: "sg-market-01", Hours: 8})
	for _, purchase := range []struct {
		id string
		n  int
	}{{"sg-manual-forge", 1}, {"sg-forge-tools", 1}, {"sg-iron", 2}, {"sg-wood", 1}} {
		apply(AdventureAction{Action: "buy", MarketID: "sg-market-01", ItemID: purchase.id, Quantity: purchase.n})
	}
	apply(AdventureAction{Action: "learn", SkillID: "sg-forge", Hours: 3})
	if a.Skills["sg-forge"] != 1 || initial.Skills["sg-forge"] != 0 {
		t.Fatal("learning source did not grant actual learned method")
	}
	var withoutTool AdventureActor
	raw, _ := json.Marshal(a)
	_ = json.Unmarshal(raw, &withoutTool)
	delete(withoutTool.Inventory, "sg-forge-tools")
	refused, _, err := ResolveAdventureAction(r, withoutTool, AdventureAction{Action: "craft", RecipeID: "sg-recipe-blade", Hours: 3}, "no-tool")
	if err == nil || !reflect.DeepEqual(withoutTool, refused) {
		t.Fatal("craft without reusable tool was not atomically refused")
	}
	before := a.Inventory["sg-travel-blade"]
	apply(AdventureAction{Action: "craft", RecipeID: "sg-recipe-blade", Hours: 3})
	if a.Inventory["sg-iron"] != 0 || a.Inventory["sg-travel-blade"] != before+1 || a.Inventory["sg-forge-tools"] != 1 {
		t.Fatal("craft did not consume materials and preserve reusable tool")
	}
	apply(AdventureAction{Action: "battle_start", OpponentID: "sg-practice"})
	for n := 0; n < 20 && a.Battle.Status == "active"; n++ {
		apply(AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "sg-blade"})
	}
	if a.Battle.Status != "victory" {
		t.Fatalf("lawful starter did not finish practice: %s", a.Battle.Status)
	}
}

func TestFantasySpellNeedsEquippedFocusAndConsumesReagent(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-learner")
	// Isolated combat fixture checks rules; this is not earned travel to Ravenbridge.
	next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "sg-practice"}, "spell-start")
	if err != nil {
		t.Fatal(err)
	}
	a = next
	before := a.Inventory["sg-charcoal"]
	energy := a.Energy
	next, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "sg-spark"}, "spell")
	if err != nil {
		t.Fatal(err)
	}
	if next.Inventory["sg-charcoal"] != before-1 || next.Energy != energy-7 {
		t.Fatal("spell did not consume reagent and effort")
	}
	delete(a.Equipment, "person:weapon")
	refused, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "sg-spark"}, "unarmed")
	if err == nil || !reflect.DeepEqual(a, refused) {
		t.Fatal("spell accepted without equipped staff")
	}
}

func TestFantasyStoryCommitColdReplayBranchAndTravel(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-road")
	s, id := fantasyBoundStory(t, r, a, "渡鸦桥镇")
	defer s.Close()
	before, e := s.Snapshot(id, "main")
	if e != nil {
		t.Fatal(e)
	}
	fork, e := s.CreateBranch(id, CreateBranchRequest{ParentEventID: before.CurrentTurn.ID, Title: "Before work"})
	if e != nil {
		t.Fatal(e)
	}
	in := DomainCommitIdentity{CommandID: "fantasy-work", OperationID: "work", Cycle: 1}
	player, e := NewPlayerInputIntent(in, "main", "在桥头做两小时装卸")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CommitPlayerInput(id, player); e != nil {
		t.Fatal(e)
	}
	req := AppendTurnWithStateRequest{BranchID: "main", User: player.Text, Narrative: "完成这一段装卸，领取约定报酬。", AgentCommandID: in.CommandID, AgentOperationID: in.OperationID, AgentCycle: in.Cycle, TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: player.Text, Adventure: &AdventureAction{Action: "work", MarketID: "sg-market-01", Hours: 2}}}}
	first, _, e := s.AppendTurnWithState(id, req)
	if e != nil {
		t.Fatal(e)
	}
	retry, _, e := s.AppendTurnWithState(id, req)
	if e != nil || first.ID != retry.ID {
		t.Fatal("non-idempotent work", e)
	}
	w, e := s.AdventureWindow(id, "main")
	if e != nil || w.Actor.Currency != 46 || w.ClockSeconds != 7200 {
		t.Fatalf("work state %+v %v", w, e)
	}
	old, e := s.AdventureWindow(id, fork.ID)
	if e != nil || old.Actor.Currency != 24 || old.ClockSeconds != 0 {
		t.Fatal("branch inherited future work", e)
	}
	trip, e := s.ApplyJourney(id, JourneyRequest{BranchID: "main", RouteID: "sg-route-l01-l02", Arrive: true, CommandID: "fantasy-travel", CommandHash: "fantasy-travel"})
	if e != nil {
		t.Fatal(e)
	}
	_ = trip
	root := s.Root()
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s = NewStore(root)
	defer s.Close()
	cold, e := s.AdventureWindow(id, "main")
	if e != nil || cold.ClockSeconds != 9000 || cold.Actor.Currency != 46 {
		t.Fatalf("cold travel lost state %+v %v", cold, e)
	}
	for _, field := range []string{"内力", "气力", "灵力"} {
		snap, _ := s.Snapshot(id, "main")
		if actorStateFieldValue(snap.State, DefaultActorID, field) != nil {
			t.Fatalf("foreign resource %s", field)
		}
	}
}

func TestFantasyAffairEchoRequiresTimeEncounterAndSurvivesBranch(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-road")
	s, id := fantasyBoundStory(t, r, a, "渡鸦桥镇")
	dir := s.Root()
	snap, err := s.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	opening := snap.CurrentTurn.ID
	fork, e := s.CreateBranch(id, CreateBranchRequest{ParentEventID: opening, Title: "No promise"})
	if e != nil {
		t.Fatal(e)
	}
	questApplyOn(t, s, id, "main", "sg-q01", QuestActionAccept, "", "accept")
	fantasyMeet(t, s, id, "main", "sg-p01", "mare")
	first := questApplyOn(t, s, id, "main", "sg-q01", QuestActionChoose, "sg-q01.c2", "resolve")
	state, e := s.QuestStateFor(id, "main")
	if e != nil {
		t.Fatal(e)
	}
	if questAvailable(state, "sg-q26") {
		t.Fatal("echo unlocked immediately")
	}
	questWait(t, s, id, "main", 21600)
	state, e = s.QuestStateFor(id, "main")
	if e != nil {
		t.Fatal(e)
	}
	if questAvailable(state, "sg-q26") {
		t.Fatal("remote echo knowledge leaked")
	}
	fantasyBridgeFollowUp(t, s, id)
	state, e = s.QuestStateFor(id, "main")
	if e != nil || !questAvailable(state, "sg-q26") {
		t.Fatalf("acknowledged echo did not open followup %v", e)
	}
	replay := questApplyOn(t, s, id, "main", "sg-q01", QuestActionChoose, "sg-q01.c2", "resolve")
	if !replay.Replayed || replay.EventID != first.EventID {
		t.Fatal("repeated affair paid again")
	}
	other, e := s.QuestStateFor(id, fork.ID)
	if e != nil || questAvailable(other, "sg-q26") {
		t.Fatal("future leaked to branch")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	cold := NewStore(dir)
	defer cold.Close()
	state, e = cold.QuestStateFor(id, "main")
	if e != nil || !questAvailable(state, "sg-q26") {
		t.Fatal("cold replay lost echo", e)
	}
	t.Log(fmt.Sprintf("one promise, two branches, delayed encounter, cold replay: %s", id))
}

func fantasyBridgeFollowUp(t *testing.T, s *Store, id string) {
	t.Helper()
	_, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "到镇长面前问上回修桥的后续", Narrative: "玛蕾在桥头检查修缮安排，你来到她面前。", ActorOps: []ActorStateOp{{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{"mare"}}}, TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{CharacterID: "mare", Name: "玛蕾·灰榆", SourceKey: "sg-p01"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "这段修缮后来如何？", Narrative: "玛蕾向你说明后续的安排。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: TurnWorldActionQuest, TaskID: "sg-q01", QuestAction: QuestActionFollowUp, QuestChoiceID: "sg-q01.c2"}}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFantasyCraftGenerationCannotBypassTools(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-smith")
	// Explicit supplies isolate the shared crafting gate, not an earned campaign.
	a.Inventory["sg-iron"] = 2
	a.Inventory["sg-wood"] = 1
	delete(a.Equipment, "person:weapon")
	power := 8
	d := AdventureGeneratedDefinition{Category: "equipment", TemplateID: "sg-travel-blade", Name: "平衡修整短剑", Source: "按既有锻法整理随身剑", Design: &AdventureGeneratedDesign{MethodID: "sg-recipe-blade", Power: &power}}
	request := AdventureAction{Action: "generate", Generate: &d, Hours: 3}
	delete(a.Inventory, "sg-forge-tools")
	refused, _, e := ResolveAdventureAction(r, a, request, "missing-tool")
	if e == nil || !reflect.DeepEqual(refused, a) {
		t.Fatal("generated equipment bypassed real tool requirement")
	}
	a.Inventory["sg-forge-tools"] = 1
	next, out, e := ResolveAdventureAction(r, a, request, "real-tool")
	if e != nil {
		t.Fatal(e)
	}
	id, _ := d.id()
	if next.Inventory[id] != 1 || next.Inventory["sg-travel-blade"] != 0 || next.Inventory["sg-forge-tools"] != 1 || next.Inventory["sg-iron"] != 0 || out.DurationSeconds != 10800 {
		t.Fatal("generated item did not consume lawful source, materials and work time")
	}
}

func TestFantasyArrowsAndDangerousAnimalRetreat(t *testing.T) {
	for _, skill := range []string{"sg-bow", "sg-bindshot", "sg-pathkeeper"} {
		t.Run(skill, func(t *testing.T) {
			r, a := fantasyOrigin(t, "sg-start-scout")
			a.Rank = 18
			a.Skills[skill] = 1
			a.Inventory["sg-arrow"] = 4
			a, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "sg-practice"}, "arrow-start")
			if e != nil {
				t.Fatal(e)
			}
			next, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: skill}, "arrow")
			if e != nil {
				t.Fatal(e)
			}
			spent := 1
			if skill == "sg-pathkeeper" {
				spent = 3
			}
			if next.Inventory["sg-arrow"] != 4-spent {
				t.Fatalf("unexpected arrow charge: %d", next.Inventory["sg-arrow"])
			}
		})
	}
	r, a := fantasyOrigin(t, "sg-start-road")
	a, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "sg-op01"}, "wild-start")
	if e != nil {
		t.Fatal(e)
	}
	refused, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "surrender"}, "wild-surrender")
	if e == nil || !reflect.DeepEqual(refused, a) {
		t.Fatal("dangerous wolf allowed safe surrender")
	}
}
