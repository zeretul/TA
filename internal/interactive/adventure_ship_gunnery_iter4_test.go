package interactive_test

import (
	"reflect"
	"ta/internal/interactive"
	"testing"
)

func TestAdventureShipGunneryNeedsRealOperatingSourceIter4(t *testing.T) {
	c := newNormalResourceChainIter2(t)
	c.journey("xinghai-l02", "xinghai-l01", "xinghai-l03")
	c.learn("s04", 4)
	c.journey("xinghai-l04", "xinghai-l21")
	_, err := c.store.ApplyQuest(c.storyID, interactive.QuestRequest{BranchID: "main", TaskID: "xinghai-a18", Action: "accept", CommandID: c.command(), CommandHash: "gunnery-accept"})
	if err != nil {
		t.Fatal(err)
	}
	before := c.read()
	if len(before.Actor.Assets) != 0 || before.Actor.Inventory["xinghai-frigate"] != 0 {
		t.Fatal("fixture unexpectedly owns a ship")
	}
	_, err = c.store.ApplyQuest(c.storyID, interactive.QuestRequest{BranchID: "main", TaskID: "xinghai-a18", Action: "choose", ChoiceID: "xinghai-a18.c2", CommandID: c.command(), CommandHash: "gunnery-unowned"})
	after := c.read()
	if err == nil {
		t.Fatalf("ship gunnery succeeded without any operating asset/source: currency %+d, seconds %+d, assets=%+v", after.Actor.Currency-before.Actor.Currency, after.ClockSeconds-before.ClockSeconds, after.Actor.Assets)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("refused unqualified gunnery changed formal state")
	}
}
