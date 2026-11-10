package interactive_test

import (
	"reflect"
	"testing"

	"ta/internal/exploration"
	"ta/internal/interactive"
)

// This uses the unchanged authored worker and existing finite opportunities.
// It is a deterministic, no-labor alternative, not a model-play transcript.
func TestAdventureFiniteSalvageRescueMachineIter4(t *testing.T) {
	c := newNormalResourceChainIter2(t)
	c.journey("xinghai-l02")
	c.learn("s03", 4)
	c.journey("xinghai-l05")
	beforeSalvage := c.read()
	checkpoint, _, err := c.store.AppendTurnWithState(c.storyID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "检查废井外侧的检修舱", Narrative: "你在入口观察电缆与隔离标识。", TurnResult: &interactive.TurnResult{Choices: []string{"回收", "查看", "离开"}}})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := c.store.CreateBranch(c.storyID, interactive.CreateBranchRequest{ParentEventID: checkpoint.ID, Title: "Before finite salvage"})
	if err != nil {
		t.Fatal(err)
	}
	const realm = exploration.RealmShaftServiceBay
	instance := ""
	apply := func(action, method, destination string) interactive.RealmResult {
		t.Helper()
		command := c.command()
		req := interactive.RealmRequest{BranchID: "main", RealmID: realm, InstanceID: instance, Action: action, CommandID: command, CommandHash: interactive.ContentHashOf([]string{action, method, destination})}
		if method != "" {
			req.ApproachID = realm + "." + method
		}
		if destination != "" {
			req.DestinationAreaID = realm + "." + destination
		}
		first, err := c.store.ApplyRealm(c.storyID, req)
		if err != nil {
			t.Fatalf("%s %s: %v", action, method, err)
		}
		committed := c.read()
		replayed, err := c.store.ApplyRealm(c.storyID, req)
		if err != nil || !replayed.Replayed || replayed.EventID != first.EventID || !reflect.DeepEqual(committed, c.read()) {
			t.Fatalf("realm retry changed resources/time: %v", err)
		}
		return first
	}
	entered := apply(interactive.RealmActionEnter, "", "")
	instance = entered.InstanceID
	apply(interactive.RealmActionResolve, "outer", "")
	if c.read().Actor.Inventory["xinghai-i34"] != 1 {
		t.Fatal("outer salvage was not carried")
	}
	apply(interactive.RealmActionApproach, "repair", "")
	if c.read().Actor.Inventory["xinghai-i34"] != 0 || beforeSalvage.Actor.Energy-c.read().Actor.Energy != 6 {
		t.Fatal("repair did not consume the recovered wire and learned skill energy")
	}
	apply(interactive.RealmActionMove, "", "core")
	apply(interactive.RealmActionResolve, "strip", "")
	if c.read().Actor.Inventory["xinghai-i34"] != 2 {
		t.Fatal("finite core salvage did not enter real inventory")
	}
	apply(interactive.RealmActionMove, "", "entry")
	apply(interactive.RealmActionExit, "", "")
	c.refuse(interactive.AdventureAction{Action: "sell", MarketID: "xinghai-market-l02", ItemID: "xinghai-i34", Quantity: 2})
	c.journey("xinghai-l02")
	c.action(interactive.AdventureAction{Action: "sell", MarketID: "xinghai-market-l02", ItemID: "xinghai-i34", Quantity: 2})
	if c.read().Actor.Currency != 50 || c.read().Actor.Inventory["xinghai-i34"] != 0 {
		t.Fatal("salvage sale did not pay exactly half its actual catalog value")
	}
	c.action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l02", ItemID: "xinghai-i19"})
	if c.read().Actor.Currency != 20 || c.read().Actor.Inventory["xinghai-i19"] != 1 {
		t.Fatal("rescue tool was not paid from finite salvage")
	}
	c.action(interactive.AdventureAction{Action: "equip", ItemID: "xinghai-i19"})
	c.refuse(interactive.AdventureAction{Action: "sell", MarketID: "xinghai-market-l02", ItemID: "xinghai-i19"})
	c.journey("xinghai-l01", "xinghai-l04")
	c.learn("s26", 4)
	c.journey("xinghai-l06")
	beforeRescue := c.read()
	c.choose("xinghai-a03", "xinghai-a03.c2")
	afterRescue := c.read()
	if afterRescue.Actor.Inventory["xinghai-i09"] != 1 || afterRescue.ClockSeconds-beforeRescue.ClockSeconds != 450 || afterRescue.Actor.Currency != 20 || afterRescue.Actor.Inventory["xinghai-i19"] != 1 {
		t.Fatal("earned rescue reward/time or held-tool ownership was not preserved")
	}
	c.action(interactive.AdventureAction{Action: "deploy", ItemID: "xinghai-i09"})
	if c.read().Actor.Equipment["machine:asset"] != "xinghai-i09" || c.read().Actor.Assets["xinghai-i09"].Fuel != 100 {
		t.Fatal("earned machine could not be deployed with the actually learned operator skill")
	}
	beforeRepeat := c.read()
	_, err = c.store.ApplyQuest(c.storyID, interactive.QuestRequest{BranchID: "main", TaskID: "xinghai-a03", Action: "choose", ChoiceID: "xinghai-a03.c2", CommandID: c.command(), CommandHash: "repeat-rescue"})
	if err == nil || !reflect.DeepEqual(beforeRepeat, c.read()) {
		t.Fatal("completed rescue was farmable or refusal partially mutated state")
	}
	c.coldCheck()
	old, err := c.store.AdventureWindow(c.storyID, fork.ID)
	if err != nil || !reflect.DeepEqual(old.Actor, beforeSalvage.Actor) || old.ClockSeconds != beforeSalvage.ClockSeconds {
		t.Fatal("sale, rescue reward or time leaked into the earlier branch")
	}
	if c.read().ClockSeconds > 36*3600 {
		t.Fatal("existing finite alternative unexpectedly exceeds thirty-six story hours")
	}
	t.Logf("no-labor finite chain: salvage=2 wire sold=50 cutter=30 balance=20 rescue_machine=1 formal_seconds=%d", c.read().ClockSeconds)
}
