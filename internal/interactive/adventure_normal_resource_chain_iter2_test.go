package interactive_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"ta/internal/interactive"
	"ta/internal/storypack"
)

// This campaign executes deterministic player actions, not model output. It
// starts with the unchanged authored worker and uses no post-creation state
// edits, injected money, materials, skills, assets or qualifications.
type normalResourceChainIter2 struct {
	t        *testing.T
	store    *interactive.Store
	storyID  string
	root     string
	sequence int
}

func newNormalResourceChainIter2(t *testing.T) *normalResourceChainIter2 {
	t.Helper()
	root := t.TempDir()
	pack, err := storypack.NewLibrary(root).Get("xinghai-689")
	if err != nil {
		t.Fatal(err)
	}
	req, authored, err := pack.StoryRequestForTemplate("xinghai-c01")
	if err != nil {
		t.Fatal(err)
	}
	module, err := interactive.NewActorStateLibrary(root).Get(pack.Story.ModuleRefs.ActorStateID)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := interactive.NewRuleSystemLibrary(root).Get(pack.Story.ModuleRefs.RuleSystemID)
	if err != nil {
		t.Fatal(err)
	}
	foundActor, foundScene := false, false
	for i := range module.ActorState.InitialActors {
		a := &module.ActorState.InitialActors[i]
		switch a.ID {
		case interactive.DefaultActorID:
			foundActor = true
			if a.State == nil {
				a.State = map[string]any{}
			}
			for k, v := range authored {
				a.State[k] = v
			}
		case interactive.DefaultStoryContextActorID:
			foundScene = true
			if a.State == nil {
				a.State = map[string]any{}
			}
			a.State["当前详细地点"] = req.Origin
		}
	}
	if !foundActor || !foundScene {
		t.Fatal("authored module is missing protagonist or scene")
	}
	req.ActorState = &module.ActorState
	req.TRPGSystem = &rules.TRPGSystem
	req.PackTemplate = &interactive.StoryProtagonistTemplateRef{PackID: pack.ID, TemplateID: "xinghai-c01"}
	req.Title = "Deterministic authored-resource chain; not natural AI"
	store := interactive.NewStore(root)
	c := &normalResourceChainIter2{t: t, store: store, root: root}
	t.Cleanup(func() { _ = c.store.Close() })
	created, err := store.CreateStory(req)
	if err != nil {
		t.Fatal(err)
	}
	c.storyID = created.ID
	// Compare the entire canonical actor to the real source before any action.
	encoded, err := json.Marshal(authored[interactive.AdventureStateField])
	if err != nil {
		t.Fatal(err)
	}
	var expected interactive.AdventureActor
	if err = json.Unmarshal(encoded, &expected); err != nil {
		t.Fatal(err)
	}
	initial := c.read()
	if !reflect.DeepEqual(initial.Actor, expected) {
		t.Fatalf("creation changed authored canonical state: got=%+v source=%+v", initial.Actor, expected)
	}
	if initial.Actor.Currency != 0 || initial.Actor.Experience != 0 || len(initial.Actor.Assets) != 0 || initial.Actor.Skills["xinghai-s03"] != 0 {
		t.Fatal("worker was prequalified or funded")
	}
	_, _, err = store.AppendTurnWithState(c.storyID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "检查自己的工具和所在地点", Narrative: "你仍在原来的长廊，准备从正常路径寻找工作。", TurnResult: &interactive.TurnResult{Choices: []string{"离开长廊", "了解工作", "暂缓"}}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func (c *normalResourceChainIter2) command() string {
	c.sequence++
	return fmt.Sprintf("normal-chain-iter2-%03d", c.sequence)
}
func (c *normalResourceChainIter2) read() interactive.AdventureWindowState {
	c.t.Helper()
	w, e := c.store.AdventureWindow(c.storyID, "main")
	if e != nil {
		c.t.Fatal(e)
	}
	return w
}
func (c *normalResourceChainIter2) action(in interactive.AdventureAction) interactive.AdventureWindowState {
	c.t.Helper()
	command := c.command()
	if in.Action == "battle_round" {
		before := c.read()
		_, e := c.store.CommitAdventureAction(c.storyID, "main", command, before.Head, in)
		if e != nil {
			c.t.Fatalf("%+v: %v", in, e)
		}
		return c.read()
	}
	text := fmt.Sprintf("执行%s，指定%s%s%s，持续%d小时", in.Action, in.SkillID, in.ItemID, in.RecipeID, in.Hours)
	identity := interactive.DomainCommitIdentity{CommandID: command, OperationID: command + "-turn", Cycle: 1}
	intent, err := interactive.NewPlayerInputIntent(identity, "main", text)
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err = c.store.CommitPlayerInput(c.storyID, intent); err != nil {
		c.t.Fatal(err)
	}
	req := interactive.AppendTurnWithStateRequest{BranchID: "main", User: text, Narrative: "你按自己的选择完成这一段行动。", AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: 1, TurnResult: &interactive.TurnResult{Choices: []string{"继续", "查看结果", "离开"}, World: &interactive.TurnWorldActionInput{Action: "adventure", SourceText: text, Adventure: &in}}}
	first, _, err := c.store.AppendTurnWithState(c.storyID, req)
	if err != nil {
		c.t.Fatalf("%+v: %v", in, err)
	}
	result := c.read()
	// Resource-bearing main-turn retries must neither duplicate payment nor time.
	if in.Action == "work" || in.Action == "craft" || in.Action == "repair" {
		retry, _, e := c.store.AppendTurnWithState(c.storyID, req)
		if e != nil || retry.ID != first.ID || !reflect.DeepEqual(result, c.read()) {
			c.t.Fatalf("%s retry changed committed resources: %v", in.Action, e)
		}
	}
	return result
}

// Refused commands are checked before any narrative or resource outcome commits.
func (c *normalResourceChainIter2) refuse(in interactive.AdventureAction) {
	c.t.Helper()
	before := c.read()
	text := fmt.Sprintf("尝试%s %s%s", in.Action, in.SkillID, in.ItemID)
	_, _, err := c.store.AppendTurnWithState(c.storyID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: text, Narrative: "此结果不应提交。", TurnResult: &interactive.TurnResult{Choices: []string{"准备", "离开", "暂缓"}, World: &interactive.TurnWorldActionInput{Action: "adventure", SourceText: text, Adventure: &in}}})
	if err == nil || !reflect.DeepEqual(before, c.read()) {
		c.t.Fatalf("unqualified action committed or partially changed state: %+v %v", in, err)
	}
}

func (c *normalResourceChainIter2) journey(destinations ...string) {
	c.t.Helper()
	for _, destination := range destinations {
		options, e := c.store.JourneyOptionsFor(c.storyID, "main")
		if e != nil {
			c.t.Fatal(e)
		}
		found := false
		for _, r := range options.Routes {
			if r.DestinationID == destination {
				found = true
				out, e := c.store.ApplyJourney(c.storyID, interactive.JourneyRequest{BranchID: "main", RouteID: r.RouteID, Arrive: true, CommandID: c.command(), CommandHash: interactive.ContentHashOf(r.RouteID)})
				if e != nil || !out.Arrived {
					c.t.Fatalf("journey to %s: %v %+v", destination, e, out)
				}
				break
			}
		}
		if !found {
			c.t.Fatalf("no authored route from %s to %s", options.CurrentLocation, destination)
		}
	}
}
func (c *normalResourceChainIter2) choose(task, choice string) {
	c.t.Helper()
	for _, action := range []string{"accept", "choose"} {
		out, e := c.store.ApplyQuest(c.storyID, interactive.QuestRequest{BranchID: "main", TaskID: task, Action: action, ChoiceID: choice, CommandID: c.command(), CommandHash: interactive.ContentHashOf([]string{task, action, choice})})
		if e != nil {
			c.t.Fatalf("%s %s: %v", task, action, e)
		}
		if action == "choose" {
			found := false
			for _, resolution := range out.Instance.Choices {
				if resolution.ChoiceID == choice {
					found = true
					if resolution.Outcome != "success" {
						c.t.Fatalf("qualified authored choice did not succeed: %+v", resolution)
					}
				}
			}
			if !found {
				c.t.Fatal("choice produced no formal resolution")
			}
		}
	}
}
func (c *normalResourceChainIter2) learn(skill string, hours int) {
	c.t.Helper()
	c.choose("xinghai-training-"+skill, "xinghai-training-"+skill+".qualify")
	before := c.read()
	id := "xinghai-" + skill
	if before.Actor.Skills[id] != 0 || before.Actor.Inventory["xinghai-training-"+skill] != 1 {
		c.t.Fatal("source qualification pre-granted the learned skill")
	}
	after := c.action(interactive.AdventureAction{Action: "learn", SkillID: id, Hours: hours})
	if after.Actor.Skills[id] < 1 || after.ClockSeconds-before.ClockSeconds != int64(hours)*3600 {
		c.t.Fatal("lesson did not consume actual training time")
	}
}
func (c *normalResourceChainIter2) fight(opponent, scale string) interactive.AdventureWindowState {
	c.t.Helper()
	c.action(interactive.AdventureAction{Action: "battle_start", OpponentID: opponent, Scale: scale})
	rounds := 0
	for c.read().Actor.Battle.Status == "active" && rounds < 100 {
		c.action(interactive.AdventureAction{Action: "battle_round", Choice: "attack"})
		rounds++
	}
	out := c.read()
	if rounds == 0 || out.Actor.Battle.Status == "active" {
		c.t.Fatalf("representative fight did not reach an honest terminal outcome: %+v", out.Actor.Battle)
	}
	c.t.Logf("%s battle: outcome=%s rounds=%d body_health=%d", scale, out.Actor.Battle.Status, rounds, out.Actor.Health)
	return out
}
func (c *normalResourceChainIter2) coldCheck() {
	c.t.Helper()
	before := c.read()
	if e := c.store.Close(); e != nil {
		c.t.Fatal(e)
	}
	c.store = interactive.NewStore(c.root)
	if after := c.read(); !reflect.DeepEqual(before.Actor, after.Actor) || before.ClockSeconds != after.ClockSeconds || !reflect.DeepEqual(before.Rules, after.Rules) {
		c.t.Fatal("cold replay changed the earned actor, clock or frozen rules")
	}
}

func TestAdventureNormalResourceChainIter2(t *testing.T) {
	c := newNormalResourceChainIter2(t)
	c.refuse(interactive.AdventureAction{Action: "learn", SkillID: "xinghai-s03", Hours: 4})
	c.refuse(interactive.AdventureAction{Action: "craft", RecipeID: "xinghai-assemble-exoskeleton", Hours: 12})
	c.refuse(interactive.AdventureAction{Action: "deploy", ItemID: "xinghai-frigate"})
	// Leave the actual opening hazard by a registered journey, not a state edit.
	c.journey("xinghai-l02")
	if h := c.read().Actor.OpeningHazards; len(h) != 1 || !h[0].Resolved || h[0].Outcome != "escaped" {
		t.Fatal("normal departure did not resolve the opening hazard")
	}
	c.learn("s03", 4)
	c.journey("xinghai-l01", "xinghai-l03")
	beforeWork := c.read()
	// 54 hours of authored dock work yield 810, with actual paid effort and rest.
	for _, hours := range []int{12, 12, 12, 12, 6} {
		if c.read().Actor.Energy < hours*15/2 {
			c.action(interactive.AdventureAction{Action: "rest", Hours: 5})
		}
		c.action(interactive.AdventureAction{Action: "work", MarketID: "xinghai-dock-l03", Hours: hours})
	}
	earned := c.read()
	if earned.Actor.Currency-beforeWork.Actor.Currency != 810 || earned.Actor.Experience != beforeWork.Actor.Experience || !reflect.DeepEqual(earned.Actor.Skills, beforeWork.Actor.Skills) {
		t.Fatal("dock wages changed qualifications or failed to pay authored rates")
	}
	c.journey("xinghai-l01", "xinghai-l02")
	// Eight extra plates are bought now for the later repair; nothing is granted.
	for _, purchase := range []struct {
		id string
		n  int
	}{{"xinghai-i33", 12}, {"xinghai-i31", 2}, {"xinghai-i37", 2}} {
		c.action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l02", ItemID: purchase.id, Quantity: purchase.n})
	}
	if c.read().Actor.Currency != 10 {
		t.Fatal("materials were not bought at the 800-Iina catalog price")
	}
	c.journey("xinghai-l01", "xinghai-l03")
	// Branches bind a committed main turn. A zero-cost observation makes the
	// latest side-record journeys part of that checkpoint without editing state.
	checkpoint, _, err := c.store.AppendTurnWithState(c.storyID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "确认装配前的物资", Narrative: "你核对已经买下的材料，准备开始装配。", TurnResult: &interactive.TurnResult{Choices: []string{"装配", "休息", "离开"}}})
	if err != nil {
		t.Fatal(err)
	}
	beforeCraft := c.read()
	priorBranch, err := c.store.CreateBranch(c.storyID, interactive.CreateBranchRequest{ParentEventID: checkpoint.ID, Title: "Before earned assembly"})
	if err != nil {
		t.Fatal(err)
	}
	built := c.action(interactive.AdventureAction{Action: "craft", RecipeID: "xinghai-assemble-exoskeleton", Hours: 12})
	if built.Actor.Inventory["xinghai-i09"] != 1 || built.Actor.Inventory["xinghai-i33"] != 8 || built.Actor.Inventory["xinghai-i31"] != 0 || built.Actor.Inventory["xinghai-i37"] != 0 || built.ClockSeconds-beforeCraft.ClockSeconds != 12*3600 {
		t.Fatal("assembly did not consume earned materials and twelve hours")
	}
	c.action(interactive.AdventureAction{Action: "deploy", ItemID: "xinghai-i09"})
	c.journey("xinghai-l04", "xinghai-l21", "xinghai-l22", "xinghai-l23", "xinghai-l38")
	bodyBefore := c.read().Actor.Health
	fought := c.fight("xinghai-e05", "machine")
	asset := fought.Actor.Assets["xinghai-i09"]
	if asset.Health >= asset.MaxHealth || asset.Fuel >= 100 || fought.Actor.Health != bodyBefore {
		t.Fatal("machine use did not preserve scale and record real wear")
	}
	c.journey("xinghai-l23", "xinghai-l22")
	beforeRepair := c.read()
	repaired := c.action(interactive.AdventureAction{Action: "repair", TargetID: "xinghai-i09", SkillID: "xinghai-s03", Hours: 8})
	restored := repaired.Actor.Assets["xinghai-i09"]
	if restored.Health != restored.MaxHealth || restored.Fuel != 100 || repaired.Actor.Inventory["xinghai-i33"] != 0 || repaired.ClockSeconds-beforeRepair.ClockSeconds != 8*3600 {
		t.Fatal("repair did not consume the eight normally bought plates and eight hours")
	}
	c.action(interactive.AdventureAction{Action: "withdraw", ItemID: "xinghai-i09"})
	c.action(interactive.AdventureAction{Action: "deploy", ItemID: "xinghai-i09"})
	if c.read().Actor.Assets["xinghai-i09"] != restored {
		t.Fatal("redeployment changed repaired wear")
	}
	c.coldCheck()
	t.Logf("machine chain: earned=810 spent=800 repair_materials=8 clock=%d inventory_machine=%d", c.read().ClockSeconds, c.read().Actor.Inventory["xinghai-i09"])

	// A separate later goal in the same earned history: actual shipbuilding and
	// piloting qualifications, not a support craft renamed into a combat ship.
	for _, skill := range []string{"s01", "s02", "s03"} {
		c.action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-" + skill, Hours: 216})
	}
	c.journey("xinghai-l21", "xinghai-l04", "xinghai-l03")
	for _, skill := range []string{"s04", "s05"} {
		c.learn(skill, 4)
		c.action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-" + skill, Hours: 216})
	}
	// Two more actual work hours fund the breaching tool, not a phantom gunship.
	c.action(interactive.AdventureAction{Action: "work", MarketID: "xinghai-dock-l03", Hours: 2})
	c.journey("xinghai-l04", "xinghai-l21", "xinghai-l22", "xinghai-l23")
	c.choose("xinghai-a06", "xinghai-a06.c2")
	c.journey("xinghai-l22")
	c.learn("s28", 4)
	c.action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l22", ItemID: "xinghai-i19"})
	c.journey("xinghai-l21")
	beforePay := c.read()
	c.choose("xinghai-a18", "xinghai-a18.c1")
	if c.read().Actor.Currency-beforePay.Actor.Currency != 50000 {
		t.Fatal("source-authored breaching reward did not fund the later ship")
	}
	c.journey("xinghai-l04", "xinghai-l03", "xinghai-l01", "xinghai-l02")
	c.learn("s06", 96)
	c.action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-s06", Hours: 192})
	c.journey("xinghai-l01", "xinghai-l03")
	c.learn("s07", 96)
	c.action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-s07", Hours: 192})
	c.journey("xinghai-l04", "xinghai-l16", "xinghai-l17")
	c.learn("s08", 96)
	c.action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-s08", Hours: 192})
	c.journey("xinghai-l16", "xinghai-l04", "xinghai-l21", "xinghai-l25")
	c.learn("s09", 96)
	c.action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-s09", Hours: 192})
	c.learn("s32", 4)
	if c.read().Actor.Rank < 7 {
		t.Fatal("normal finite study has not met shipbuilding rank")
	}
	c.journey("xinghai-l21", "xinghai-l22")
	c.action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l22", ItemID: "xinghai-i12"})
	c.journey("xinghai-l23", "xinghai-l38")
	c.choose("xinghai-a11", "xinghai-a11.c3")
	c.learn("s10", 168)
	c.journey("xinghai-l23", "xinghai-l22")
	beforeShipStock := c.read()
	for _, purchase := range []struct {
		id string
		n  int
	}{{"xinghai-i33", 30}, {"xinghai-i34", 20}, {"xinghai-i31", 10}, {"xinghai-i39", 10}} {
		c.action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l22", ItemID: purchase.id, Quantity: purchase.n})
	}
	if beforeShipStock.Actor.Currency-c.read().Actor.Currency != 3500 {
		t.Fatal("ship stock was not purchased")
	}
	beforeShip := c.read()
	shipBuilt := c.action(interactive.AdventureAction{Action: "craft", RecipeID: "xinghai-assemble-frigate", Hours: 120})
	if shipBuilt.Actor.Inventory["xinghai-frigate"] != 1 || shipBuilt.ClockSeconds-beforeShip.ClockSeconds != 120*3600 || shipBuilt.Actor.Inventory["xinghai-i34"] != beforeShip.Actor.Inventory["xinghai-i34"]-20 {
		t.Fatal("frigate assembly did not consume real materials and time")
	}
	c.action(interactive.AdventureAction{Action: "deploy", ItemID: "xinghai-frigate"})
	bodyBefore = c.read().Actor.Health
	shipFought := c.fight("xinghai-raider-frigate", "ship")
	ship := shipFought.Actor.Assets["xinghai-frigate"]
	if ship.Fuel >= 100 || ship.Health >= ship.MaxHealth || shipFought.Actor.Health != bodyBefore {
		t.Fatal("ship operation did not record ship-scale costs")
	}
	c.action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l22", ItemID: "xinghai-i33", Quantity: 8})
	c.journey("xinghai-l21", "xinghai-l04", "xinghai-l03")
	beforeDock := c.read()
	afterDock := c.action(interactive.AdventureAction{Action: "repair", MarketID: "xinghai-dock-l03", TargetID: "xinghai-frigate", Hours: 8})
	ship = afterDock.Actor.Assets["xinghai-frigate"]
	if ship.Health != ship.MaxHealth || ship.Fuel != 100 || beforeDock.Actor.Currency-afterDock.Actor.Currency != 400 || beforeDock.Actor.Inventory["xinghai-i33"]-afterDock.Actor.Inventory["xinghai-i33"] != 8 || afterDock.ClockSeconds-beforeDock.ClockSeconds != 8*3600 {
		t.Fatal("ship dock service did not pay and repair")
	}
	c.journey("xinghai-l01")
	beforeRegistration := c.read()
	c.choose("xinghai-q03", "xinghai-q03.c1")
	afterRegistration := c.read()
	if beforeRegistration.Actor.Currency != afterRegistration.Actor.Currency || !reflect.DeepEqual(beforeRegistration.Actor.Inventory, afterRegistration.Actor.Inventory) {
		t.Fatal("ship verification minted a vessel or paid an unrelated debt")
	}
	c.coldCheck()
	prior, err := c.store.AdventureWindow(c.storyID, priorBranch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prior.Actor, beforeCraft.Actor) || prior.ClockSeconds != beforeCraft.ClockSeconds {
		t.Fatal("later machinery, ship or recovery leaked into the pre-assembly branch")
	}
	t.Logf("ship chain: rank=%d craft_hours=120 skill_s10=%d pilot_s32=%d ending_currency=%d clock=%d", c.read().Actor.Rank, c.read().Actor.Skills["xinghai-s10"], c.read().Actor.Skills["xinghai-s32"], c.read().Actor.Currency, c.read().ClockSeconds)
}
