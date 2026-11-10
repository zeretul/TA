package interactive

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func spaceShieldFixture(t *testing.T) (AdventureRules, AdventureActor) {
	t.Helper()
	r, a := spaceMentalFixture(t)
	// These tests exercise real impacts, not the ordinary rank-nine defense
	// completely stopping the author's eight-power introductory training beam.
	for i := range r.Opponents {
		if r.Opponents[i].ID == "xinghai-sparring" {
			r.Opponents[i].Attack = 18
		}
	}
	a.Inventory["xinghai-i11"] = 1
	a.Equipment["person:armor"] = "xinghai-i11"
	a.Skills["xinghai-s06"] = 1
	a.Inventory["xinghai-i38"] = 2
	a.Inventory["xinghai-weapon-cell"] = 6
	n, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "xinghai-sparring"}, "shield-start")
	if e != nil {
		t.Fatal(e)
	}
	return r, n
}

// Shield tests explicitly select a continued ordinary beam, rather than the
// fallback's lawful ceasefire after three fully blocked, no-progress exchanges.
func TestSpaceBraceletHasThreeActualLaserBlocksNotPermanentArmor(t *testing.T) {
	r, a := spaceShieldFixture(t)
	for round := 0; round < 3; round++ {
		n, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "ordinary"}, "shield-hit")
		if e != nil {
			t.Fatal(e)
		}
		if n.Health != a.Health {
			t.Fatal("a registered laser hit passed through a charged bracelet")
		}
		a = n
	}
	hot, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "ordinary"}, "hot-bracelet")
	if e != nil {
		t.Fatal(e)
	}
	if hot.Health >= a.Health || hot.Inventory["xinghai-i11"] != 1 {
		t.Fatal("third hit did not overload the same owned bracelet")
	}
}

func TestSpaceShieldBoostPaysMaterialsAndRaisesSingleHitPeakByHalf(t *testing.T) {
	r, a := spaceShieldFixture(t)
	a.Battle.Attack = 60
	base, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "base-peak")
	if e != nil {
		t.Fatal(e)
	}
	boosted, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s06"}, "boost-peak")
	if e != nil {
		t.Fatal(e)
	}
	if base.Health >= a.Health || boosted.Health != a.Health || boosted.Inventory["xinghai-i38"] != a.Inventory["xinghai-i38"]-1 || boosted.Inventory["xinghai-weapon-cell"] != a.Inventory["xinghai-weapon-cell"]-3 {
		t.Fatal("boost did not increase actual deflection peak by50% with capacitor and battery consumption")
	}
}

func TestSpaceShieldCooldownsUseFormalImpactTimeAndDoNotResetOnReequip(t *testing.T) {
	r, a := spaceShieldFixture(t)
	now := int64(0)
	for round := 0; round < 3; round++ {
		n, out, e := ResolveAdventureActionAt(r, a, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "ordinary"}, "laser", now)
		if e != nil {
			t.Fatal(e)
		}
		a = n
		now += out.DurationSeconds
	}
	if now != 60 || a.ItemReadyAt["xinghai-i11"] != 660 || a.ItemCharges["xinghai-i11"] != 0 {
		t.Fatal("overload did not preserve the exact ten-minute formal deadline")
	}
	hot, _, e := ResolveAdventureActionAt(r, a, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "ordinary"}, "too-early", 600)
	if e != nil {
		t.Fatal(e)
	}
	if hot.Health >= a.Health || hot.ItemCharges["xinghai-i11"] != 0 {
		t.Fatal("the depleted shield restarted before cooldown")
	}
	ready, _, e := ResolveAdventureActionAt(r, a, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "ordinary"}, "boundary-impact", 640)
	if e != nil {
		t.Fatal(e)
	}
	if ready.Health != a.Health || ready.ItemCharges["xinghai-i11"] != 2 {
		t.Fatal("shield was not ready at the actual660-second impact boundary")
	}
	a.Battle.Status = "escaped"
	n, _, e := ResolveAdventureActionAt(r, a, AdventureAction{Action: "unequip", ItemID: "xinghai-i11"}, "unequip", 60)
	if e != nil {
		t.Fatal(e)
	}
	n, _, e = ResolveAdventureActionAt(r, n, AdventureAction{Action: "equip", ItemID: "xinghai-i11"}, "reequip", 60)
	if e != nil {
		t.Fatal(e)
	}
	if n.ItemCharges["xinghai-i11"] != 0 || n.ItemReadyAt["xinghai-i11"] != 660 {
		t.Fatal("equipment cycling refreshed the same overloaded object")
	}
}

func TestSpaceShieldBoostHasTwoMinuteZeroShieldAndAtomicPrerequisites(t *testing.T) {
	r, a := spaceShieldFixture(t)
	in := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s06"}
	boosted, _, e := ResolveAdventureActionAt(r, a, in, "boost", 0)
	if e != nil {
		t.Fatal(e)
	}
	if boosted.ItemReadyAt["xinghai-i11"] != 140 || boosted.ItemCharges["xinghai-i11"] != 2 {
		t.Fatal("boost did not force two minutes of zero shield after impact")
	}
	hot, _, e := ResolveAdventureActionAt(r, boosted, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "ordinary"}, "zero-shield", 100)
	if e != nil {
		t.Fatal(e)
	}
	if hot.Health >= boosted.Health || hot.ItemCharges["xinghai-i11"] != 2 {
		t.Fatal("boost cooldown retained passive protection")
	}
	ready, _, e := ResolveAdventureActionAt(r, boosted, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "ordinary"}, "boost-boundary", 120)
	if e != nil {
		t.Fatal(e)
	}
	if ready.Health != boosted.Health || ready.ItemCharges["xinghai-i11"] != 1 {
		t.Fatal("boost cooling erased remaining charges or did not restore protection at the exact boundary")
	}
	for _, test := range []struct {
		name string
		edit func(*AdventureActor)
	}{
		{"missing capacitor", func(a *AdventureActor) { a.Inventory["xinghai-i38"] = 0 }},
		{"short battery pack", func(a *AdventureActor) { a.Inventory["xinghai-weapon-cell"] = 2 }},
		{"not equipped", func(a *AdventureActor) { delete(a.Equipment, "person:armor") }},
		{"still cooling", func(a *AdventureActor) { a.ItemReadyAt = map[string]int64{"xinghai-i11": 600} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, _ := json.Marshal(a)
			var candidate AdventureActor
			_ = json.Unmarshal(b, &candidate)
			test.edit(&candidate)
			before, _ := json.Marshal(candidate)
			got, _, e := ResolveAdventureActionAt(r, candidate, in, "refused", 0)
			after, _ := json.Marshal(got)
			if e == nil || string(before) != string(after) {
				t.Fatal("refusal consumed energy/materials/charges or time")
			}
		})
	}
}

func TestSpaceShieldBlocksOnlyPoweredLaserAndProtectsActualNPCEquipment(t *testing.T) {
	r, a := spaceShieldFixture(t)
	// The actual registered pulse sidearm is a beam weapon, with real ammo cost.
	a.Inventory["xinghai-i06"] = 1
	a.Equipment["person:weapon"] = "xinghai-i06"
	for _, item := range r.Items {
		if item.ID == "xinghai-i06" {
			for id, n := range item.Materials {
				a.Inventory[id] = n
			}
		}
	}
	bare, _, e := ResolveAdventureActionAt(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "bare-target", 0)
	if e != nil {
		t.Fatal(e)
	}
	a.Battle.EnemyInventory["xinghai-i11"] = 1
	a.Battle.EnemyEquipment = map[string]string{"person:armor": "xinghai-i11"}
	protected, _, e := ResolveAdventureActionAt(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "equipped-target", 0)
	if e != nil {
		t.Fatal(e)
	}
	if protected.Battle.Health <= bare.Battle.Health || protected.Battle.EnemyItemCharges["xinghai-i11"] != 2 {
		t.Fatal("NPC's actual equipped bracelet did not intercept the player's powered beam")
	}
	// An empty gun cannot keep labeling its fallback punch as a laser.
	r, a = spaceShieldFixture(t)
	// This isolated rig has enough unarmed force to hurt the qualified
	// rank-nine player; otherwise the zero-damage rule hides the comparison.
	a.Battle.EnemyRank = a.Rank
	for i := range r.Opponents {
		if r.Opponents[i].ID == "xinghai-sparring" {
			r.Opponents[i].Rank = a.Rank
		}
	}
	a.Battle.EnemyEquipment = map[string]string{"person:weapon": "xinghai-i06"}
	// The slot alone is not ownership. The test concerns an actually owned
	// empty gun, not the training target's separate built-in laser attack.
	a.Battle.EnemyInventory["xinghai-i06"] = 1
	delete(a.Battle.EnemyInventory, "xinghai-pistol-ammo")
	unpowered, _, e := ResolveAdventureActionAt(r, a, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "ordinary"}, "empty-gun", 0)
	if e != nil {
		t.Fatal(e)
	}
	if unpowered.Health >= a.Health || len(unpowered.ItemCharges) != 0 {
		t.Fatal("empty enemy gun generated a phantom laser or consumed a shield charge")
	}
}

func TestSpaceShieldStoreCooldownCASBranchAndColdReplay(t *testing.T) {
	r, a := spaceShieldFixture(t)
	a.Battle = nil
	s, id := adventureFixtureStoryWithActor(t, r, a, "枢纽三号真空改装船坞")
	defer func() { _ = s.Close() }()
	start, _, e := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "开启激光校验", Narrative: "校验光束亮起。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "开启激光校验", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "xinghai-sparring"}}}})
	if e != nil {
		t.Fatal(e)
	}
	for round := 0; round < 3; round++ {
		w, e := s.AdventureWindow(id, "main")
		if e != nil {
			t.Fatal(e)
		}
		command := fmt.Sprintf("shield-round-%d", round)
		in := AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "ordinary"}
		first, e := s.CommitAdventureAction(id, "main", command, w.Head, in)
		if e != nil {
			t.Fatal(e)
		}
		if round == 2 {
			retry, e := s.CommitAdventureAction(id, "main", command, w.Head, in)
			if e != nil || !reflect.DeepEqual(first, retry) {
				t.Fatal("retry counted the laser twice", e)
			}
		}
	}
	hot, e := s.AdventureWindow(id, "main")
	if e != nil || hot.ClockSeconds != 60 || hot.Actor.ItemReadyAt["xinghai-i11"] != 660 {
		t.Fatal("Store did not bind shield cooldown to formal battle time", e)
	}
	fork, e := s.CreateBranch(id, CreateBranchRequest{ParentEventID: start.ID, Title: "Before laser interception"})
	if e != nil {
		t.Fatal(e)
	}
	earlier, e := s.AdventureWindow(id, fork.ID)
	if e != nil || len(earlier.Actor.ItemReadyAt) != 0 {
		t.Fatal("overload leaked into earlier branch", e)
	}
	root := s.Root()
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s = NewStore(root)
	restored, e := s.AdventureWindow(id, "main")
	if e != nil || !reflect.DeepEqual(restored, hot) {
		t.Fatal("cold replay reset the same bracelet", e)
	}
	for round := 0; round < 5; round++ {
		w, e := s.AdventureWindow(id, "main")
		if e != nil {
			t.Fatal(e)
		}
		if w.Actor.Battle.Status != "active" {
			break
		}
		if _, e = s.CommitAdventureAction(id, "main", fmt.Sprintf("stop-rig-%d", round), w.Head, AdventureAction{Action: "battle_round", Choice: "attack"}); e != nil {
			t.Fatal(e)
		}
	}
	_, _, e = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "休养一小时", Narrative: "手环终于冷却下来。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "休养一小时", Adventure: &AdventureAction{Action: "rest", Hours: 1}}}})
	if e != nil {
		t.Fatal(e)
	}
	cooled, e := s.AdventureWindow(id, "main")
	if e != nil || cooled.ClockSeconds < 3660 || cooled.Actor.ItemCharges["xinghai-i11"] != 3 || len(cooled.Actor.ItemReadyAt) != 0 || cooled.Actor.Inventory["xinghai-i11"] != 1 {
		t.Fatal("real elapsed rest did not rearm the same owned bracelet", e)
	}
}

func TestSpaceShieldDoesNotSpendChargeOnFullyStoppedBeam(t *testing.T) {
	r, a := spaceShieldFixture(t)
	a.Battle.Attack = 8
	next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "ordinary"}, "fully-stopped-beam")
	if err != nil {
		t.Fatal(err)
	}
	if next.Health != a.Health || len(next.ItemCharges) != 0 || len(next.ItemReadyAt) != 0 || next.Inventory["xinghai-i11"] != a.Inventory["xinghai-i11"] {
		t.Fatal("a zero-damage beam consumed a charge or fabricated an impact")
	}
}
