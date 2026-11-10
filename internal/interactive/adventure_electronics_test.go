package interactive

import (
	"encoding/json"
	"reflect"
	"testing"
)

func spaceElectronicsFixture(t *testing.T) (AdventureRules, AdventureActor) {
	t.Helper()
	r, a := spaceMentalFixture(t)
	a.Skills["xinghai-s23"] = 1
	a.Inventory["xinghai-i09"] = 1
	for _, v := range r.Items {
		if v.ID == "xinghai-i09" {
			a.Assets[v.ID] = AdventureAsset{Health: v.Durability, MaxHealth: v.Durability, Shield: v.Power, Fuel: 100}
		}
	}
	a.Equipment["machine:asset"] = "xinghai-i09"
	n, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "xinghai-e05"}, "electronic-source")
	if e != nil {
		t.Fatal(e)
	}
	return r, n
}

func TestSpacePlasmaDisruptsActualNearbyElectronicsNotHumanHealth(t *testing.T) {
	r, a := spaceElectronicsFixture(t)
	n, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s23"}, "plasma")
	if e != nil {
		t.Fatal("nearby machine electronics were rejected as wrong scale", e)
	}
	if n.Battle.Health != a.Battle.Health || n.Assets["xinghai-i09"].Health != a.Assets["xinghai-i09"].Health || n.Assets["xinghai-i09"].Shield != a.Assets["xinghai-i09"].Shield || n.Energy >= a.Energy || n.Injury <= a.Injury {
		t.Fatal("plasma did not trade real energy/heat for one non-damaging electronic interruption")
	}
	next, _, err := ResolveAdventureAction(r, n, AdventureAction{Action: "battle_round", Choice: "guard"}, "node-reboots")
	if err != nil {
		t.Fatal(err)
	}
	if next.Assets["xinghai-i09"].Health == n.Assets["xinghai-i09"].Health && next.Assets["xinghai-i09"].Shield == n.Assets["xinghai-i09"].Shield {
		t.Fatal("brief electronic interruption became a permanent machine shutdown")
	}
	a.Battle = nil
	person, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "xinghai-e02"}, "human")
	if e != nil {
		t.Fatal(e)
	}
	if got, _, err := ResolveAdventureAction(r, person, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s23"}, "not-person"); err == nil || !reflect.DeepEqual(got, person) {
		t.Fatal("person was treated as arbitrary electronics")
	}
}

func TestSpacePlasmaRequiresRegisteredElectronicsWithinTenMeters(t *testing.T) {
	r, a := spaceElectronicsFixture(t)
	b, _ := json.Marshal(a)
	for _, distance := range []int{0, 11} {
		var candidate AdventureActor
		_ = json.Unmarshal(b, &candidate)
		// Exercise the same serialized private encounter fact used by actual source.
		raw, _ := json.Marshal(candidate.Battle)
		var battle map[string]any
		_ = json.Unmarshal(raw, &battle)
		battle["electronic_distance_meters"] = distance
		raw, _ = json.Marshal(battle)
		_ = json.Unmarshal(raw, candidate.Battle)
		got, _, err := ResolveAdventureAction(r, candidate, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s23"}, "outside-arc")
		if err == nil || !reflect.DeepEqual(got, candidate) {
			t.Fatal("unregistered or out-of-range machinery was disrupted or charged")
		}
	}
}

func TestSpacePlasmaStoreCostsCASAndColdReplay(t *testing.T) {
	r, a := spaceElectronicsFixture(t)
	a.Battle = nil
	location := ""
	for _, op := range r.Opponents {
		if op.ID == "xinghai-e05" {
			location = op.Location
		}
	}
	s, id := adventureFixtureStoryWithActor(t, r, a, location)
	defer func() { _ = s.Close() }()
	_, _, e := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "接近铁墓近防节点", Narrative: "瞄准仪表的电光映在舱壁上。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "接近铁墓近防节点", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "xinghai-e05"}}}})
	if e != nil {
		t.Fatal(e)
	}
	w, e := s.AdventureWindow(id, "main")
	if e != nil {
		t.Fatal(e)
	}
	in := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s23"}
	first, e := s.CommitAdventureAction(id, "main", "actual-plasma", w.Head, in)
	if e != nil {
		t.Fatal(e)
	}
	retry, e := s.CommitAdventureAction(id, "main", "actual-plasma", w.Head, in)
	if e != nil || !reflect.DeepEqual(first, retry) {
		t.Fatal("plasma retry spent energy/heat twice", e)
	}
	if _, e = s.CommitAdventureAction(id, "main", "stale-plasma", w.Head, in); e == nil {
		t.Fatal("stale plasma was accepted")
	}
	hot, e := s.AdventureWindow(id, "main")
	if e != nil || hot.ClockSeconds != 20 {
		t.Fatal("plasma did not settle formal duration", e)
	}
	fork, e := s.CreateBranch(id, CreateBranchRequest{ParentEventID: w.Head, Title: "Before the arc"})
	if e != nil {
		t.Fatal(e)
	}
	old, e := s.AdventureWindow(id, fork.ID)
	if e != nil || old.Actor.Energy != a.Energy || old.Actor.Injury != a.Injury {
		t.Fatal("heat or energy cost leaked into earlier branch", e)
	}
	root := s.Root()
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s = NewStore(root)
	cold, e := s.AdventureWindow(id, "main")
	if e != nil || !reflect.DeepEqual(hot, cold) {
		t.Fatal("cold replay lost electronic interruption costs", e)
	}
}

func TestSpacePersonalPlasmaNodeNeedsNoOwnedMachineAndCannotControlShip(t *testing.T) {
	r, a := spaceMentalFixture(t)
	a.Skills["xinghai-s23"] = 1
	node, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "xinghai-electronic-node"}, "node-on-foot")
	if e != nil {
		t.Fatal("registered local node required a machine", e)
	}
	n, _, e := ResolveAdventureAction(r, node, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s23"}, "personal-arc")
	if e != nil || n.Battle.Health != node.Battle.Health || len(n.Assets) != 0 {
		t.Fatal("personal power became a mounted gun or damaged mainframe", e)
	}
	r, ship := spaceOverclockFixture(t)
	ship.Skills["xinghai-s23"] = 1
	if got, _, e := ResolveAdventureAction(r, ship, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s23"}, "not-a-ship-emp"); e == nil || !reflect.DeepEqual(got, ship) {
		t.Fatal("personal plasma became whole-ship control")
	}
}

func TestSpacePersonalPlasmaExceptionRejectsMixedPhysicalPayload(t *testing.T) {
	r, a := spaceElectronicsFixture(t)
	for i := range r.Skills {
		if r.Skills[i].ID == "xinghai-s23" {
			r.Skills[i].Effects = append(r.Skills[i].Effects, AdventureEffect{Kind: "attack", Power: 20})
		}
	}
	got, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s23"}, "mixed-scale-payload")
	if e == nil || !reflect.DeepEqual(got, a) {
		t.Fatal("adding electronics to a physical attack bypassed the person/machine scale gate or charged costs")
	}
}
