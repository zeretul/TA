package interactive

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func spaceOverclockFixture(t *testing.T) (AdventureRules, AdventureActor) {
	t.Helper()
	r, a := spaceMentalFixture(t)
	a.Skills["xinghai-s09"] = 1
	a.Skills["xinghai-s32"] = 1
	a.Inventory["xinghai-frigate"] = 1
	var ship AdventureItem
	for _, v := range r.Items {
		if v.ID == "xinghai-frigate" {
			ship = v
		}
	}
	a.Assets[ship.ID] = AdventureAsset{Health: ship.Durability, MaxHealth: ship.Durability, Shield: ship.Power, Fuel: 100}
	a.Equipment["ship:asset"] = ship.ID
	n, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "xinghai-raider-frigate", Scale: "ship"}, "ship-start")
	if e != nil {
		t.Fatal(e)
	}
	return r, n
}

func TestSpaceOverclockEscapesWithRealCostsAndDeterministicRisk(t *testing.T) {
	r, a := spaceOverclockFixture(t)
	seenEscape, seenAccident := false, false
	for i := 0; i < 200; i++ {
		n, out, e := ResolveAdventureActionAt(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s09"}, fmt.Sprint("overclock-", i), 100)
		if e != nil {
			t.Fatal(e)
		}
		ship := n.Assets["xinghai-frigate"]
		if n.Energy != 0 || ship.Fuel != 0 || ship.MaxHealth >= a.Assets["xinghai-frigate"].MaxHealth || out.DurationSeconds != 20 {
			t.Fatal("overclock did not pay full energy, fuel and permanent sleeve damage")
		}
		if n.Battle.Status == "escaped" {
			seenEscape = true
		}
		if ship.Health == 0 && n.Battle.Status == "defeat" {
			seenAccident = true
		}
		repeat, _, e := ResolveAdventureActionAt(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s09"}, fmt.Sprint("overclock-", i), 100)
		if e != nil || !reflect.DeepEqual(n, repeat) {
			t.Fatal("same formal attempt rerolled")
		}
	}
	if !seenEscape || !seenAccident {
		t.Fatal("no bounded escape and real engine-melt outcomes")
	}
}

func TestSpaceOverclockCannotRepairOrCrossScale(t *testing.T) {
	r, a := spaceOverclockFixture(t)
	a.Battle.Status = "escaped"
	a.Inventory[r.RepairMaterialID] = 10
	before := a
	if _, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "repair", SkillID: "xinghai-s09", TargetID: "xinghai-frigate", Hours: 1}, "not-repair"); e == nil {
		t.Fatal("engine overclock still repairs and refuels the ship")
	}
	if !reflect.DeepEqual(a, before) {
		t.Fatal("rejected repair mutated source")
	}
	a.Battle = nil
	person, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "xinghai-sparring"}, "person")
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = ResolveAdventureAction(r, person, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s09"}, "wrong-scale"); e == nil {
		t.Fatal("ship overclock acted on a person")
	}
}

func TestSpaceOverclockDockServicePreservesDamageAndFormalLock(t *testing.T) {
	r, a := spaceOverclockFixture(t)
	var seed string
	for i := 0; i < 100; i++ {
		s := fmt.Sprint("successful-", i)
		if adventurePercentile(s, "engine-melt") >= 10 && adventurePercentile(s, "overclock-escape") < 95 {
			seed = s
			break
		}
	}
	n, _, e := ResolveAdventureActionAt(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s09"}, seed, 100)
	if e != nil || n.Battle.Status != "escaped" {
		t.Fatal("qualified escape fixture", e)
	}
	ship := n.Assets["xinghai-frigate"]
	if ship.DisabledUntil != 10920 || ship.PermanentDamage < 1 {
		t.Fatal("no formal three-hour post-impact lock")
	}
	n.Currency = 1000
	n.Inventory[r.RepairMaterialID] = 10
	before := n
	for _, edit := range []func(*AdventureActor){func(x *AdventureActor) { x.Currency = 0 }, func(x *AdventureActor) { x.Inventory[r.RepairMaterialID] = 0 }, func(x *AdventureActor) { delete(x.Skills, "xinghai-s09") }} {
		b, _ := json.Marshal(before)
		var candidate AdventureActor
		_ = json.Unmarshal(b, &candidate)
		edit(&candidate)
		got, _, err := ResolveAdventureActionAt(r, candidate, AdventureAction{Action: "repair", MarketID: "xinghai-dock-l03", TargetID: "xinghai-frigate", Hours: 1}, "dock-refusal", 120)
		if err == nil || !reflect.DeepEqual(got, candidate) {
			t.Fatal("dock refusal was not atomic")
		}
	}
	fixed, out, e := ResolveAdventureActionAt(r, n, AdventureAction{Action: "repair", MarketID: "xinghai-dock-l03", TargetID: "xinghai-frigate", Hours: 1}, "dock-hour", 120)
	if e != nil {
		t.Fatal(e)
	}
	repaired := fixed.Assets["xinghai-frigate"]
	if fixed.Currency != 950 || fixed.Inventory[r.RepairMaterialID] != 9 || out.DurationSeconds != 3600 || repaired.MaxHealth != ship.MaxHealth || repaired.PermanentDamage != ship.PermanentDamage || repaired.DisabledUntil != ship.DisabledUntil {
		t.Fatal("paid service erased permanent wear/lock or invented funds/time")
	}
	if _, _, e = ResolveAdventureActionAt(r, fixed, AdventureAction{Action: "deploy", ItemID: "xinghai-frigate"}, "still-disabled", 3720); e == nil {
		t.Fatal("repair or redeploy bypassed power lock")
	}
	if _, _, e = ResolveAdventureActionAt(r, fixed, AdventureAction{Action: "battle_start", OpponentID: "xinghai-raider-frigate"}, "still-disabled-battle", 3720); e == nil {
		t.Fatal("disabled ship started another encounter")
	}
	if _, _, e = ResolveAdventureActionAt(r, fixed, AdventureAction{Action: "deploy", ItemID: "xinghai-frigate"}, "ready-boundary", 10920); e != nil {
		t.Fatal("power did not recover at formal boundary", e)
	}
	state := map[string]any{}
	if validateAdventureScene(r, state, AdventureAction{Action: "repair", MarketID: "xinghai-dock-l03"}) == nil {
		t.Fatal("remote dock service bypassed committed location")
	}
}

func TestSpaceOverclockStoreCASBranchAndColdReplay(t *testing.T) {
	r, a := spaceOverclockFixture(t)
	a.Battle = nil
	location := ""
	for _, op := range r.Opponents {
		if op.ID == "xinghai-raider-frigate" {
			location = op.Location
		}
	}
	s, id := adventureFixtureStoryWithActor(t, r, a, location)
	_, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "迎战拦截护卫舰", Narrative: "追兵切入航路。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "迎战拦截护卫舰", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "xinghai-raider-frigate"}}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	w, e := s.AdventureWindow(id, "main")
	if e != nil {
		t.Fatal(e)
	}
	var command string
	for i := 0; i < 100; i++ {
		c := fmt.Sprint("store-overclock-", i)
		if adventurePercentile(c, "engine-melt") >= 10 && adventurePercentile(c, "overclock-escape") < 95 {
			command = c
			break
		}
	}
	in := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s09"}
	first, e := s.CommitAdventureAction(id, "main", command, w.Head, in)
	if e != nil {
		t.Fatal(e)
	}
	retry, e := s.CommitAdventureAction(id, "main", command, w.Head, in)
	if e != nil || !reflect.DeepEqual(first, retry) {
		t.Fatal("same overclock spent resources or rerolled twice", e)
	}
	if _, e = s.CommitAdventureAction(id, "main", "stale-overclock", w.Head, in); e == nil {
		t.Fatal("stale head accepted")
	}
	hot, e := s.AdventureWindow(id, "main")
	if e != nil {
		t.Fatal(e)
	}
	if hot.ClockSeconds != 20 || hot.Actor.Assets["xinghai-frigate"].DisabledUntil != 10820 {
		t.Fatal("formal battle clock did not own the three-hour lock")
	}
	fork, e := s.CreateBranch(id, CreateBranchRequest{ParentEventID: w.Head, Title: "Before engine overdrive"})
	if e != nil {
		t.Fatal(e)
	}
	old, e := s.AdventureWindow(id, fork.ID)
	if e != nil || old.Actor.Assets["xinghai-frigate"].PermanentDamage != 0 {
		t.Fatal("engine wear leaked into earlier branch", e)
	}
	root := s.Root()
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s = NewStore(root)
	restored, e := s.AdventureWindow(id, "main")
	if e != nil || !reflect.DeepEqual(hot, restored) {
		t.Fatal("cold replay reset sleeve wear or power lock", e)
	}
	_, _, e = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "休养三小时", Narrative: "你在受损的动力炉旁等了三小时。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "休养三小时", Adventure: &AdventureAction{Action: "rest", Hours: 3}}}})
	if e != nil {
		t.Fatal(e)
	}
	ready, e := s.AdventureWindow(id, "main")
	if e != nil || ready.ClockSeconds != 10820 || ready.Actor.Assets["xinghai-frigate"].Fuel != 0 || ready.Actor.Assets["xinghai-frigate"].PermanentDamage == 0 {
		t.Fatal("formal waiting repaired or refuelled ship instead of just passing lock time", e)
	}
}
