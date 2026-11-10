package exploration

import (
	"encoding/json"
	"reflect"
	"testing"
)

// These fixtures exercise authored transitions, not a model or actor-resource
// simulation. Story integration is responsible for authenticating capabilities.
func TestShatteredBeaconPreservationReturnAndFiniteRecord(t *testing.T) {
	def, scope, s := createExploreTestInstance(t, RealmShatteredBeacon)
	p := def.ID
	if ds := ValidateDungeonDefinition(def); len(ds) != 0 {
		t.Fatal(ds)
	}
	encoded, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if _, ds, err := ParseDungeonDefinition(encoded); err != nil || len(ds) != 0 {
		t.Fatalf("definition round trip: %v %v", err, ds)
	}
	s = exploreTestApply(t, def, scope, s, p+".survey", 0).Snapshot
	s = exploreTestApply(t, def, scope, s, p+".slow", 600).Snapshot
	s = exploreTestMove(t, def, scope, s, p+".core")
	s = exploreTestApply(t, def, scope, s, p+".preserve", 2400).Snapshot
	if s.ExploreState.Pressure != 4 || s.ExploreState.ReturnDueSeconds != 17700 {
		t.Fatalf("wrong preservation: %+v", s.ExploreState)
	}
	beaconExpectBlocked(t, def, scope, s, "return", 17699)
	beaconExpectBlocked(t, def, scope, s, "strip", 17700)
	s = exploreTestMove(t, def, scope, s, p+".entry")
	ctx := testInstanceContext(s, scope, testInstancePermit(ActionExit, s.CurrentAreaID, "", "", "scene.exit"))
	out, err := ExitDungeonInstance(def, scope, s, ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Serialize the authoritative snapshot while outside, then restore and reenter.
	encoded, err = json.Marshal(out.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var restored DungeonInstanceSnapshot
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	roundtrip, err := json.Marshal(restored)
	if err != nil || string(encoded) != string(roundtrip) {
		t.Fatal("snapshot round trip changed progress")
	}
	ctx = testInstanceContext(restored, scope, testInstancePermit(ActionReenter, p+".entry", "", "", "scene.reenter"))
	reentered, err := ReenterDungeonInstance(def, scope, restored, ctx, p+".entry")
	if err != nil {
		t.Fatal(err)
	}
	s = exploreTestMove(t, def, scope, reentered.Snapshot, p+".core")
	s = exploreTestApply(t, def, scope, s, p+".return", 17700).Snapshot
	s = exploreTestMove(t, def, scope, s, p+".record")
	result := exploreTestApply(t, def, scope, s, p+".copy", 18300)
	s = result.Snapshot
	if len(result.Rewards) != 1 || result.Rewards[0].RefID != "sg-glass" || result.Rewards[0].Quantity != 1 || !hasExploreFact(s, p+".copied") {
		t.Fatal("transcription must preserve record and yield only the edge fragment")
	}
	beaconExpectBlocked(t, def, scope, s, "copy", 20100)
	beaconExpectBlocked(t, def, scope, s, "grind", 20100)
	s = exploreTestMove(t, def, scope, s, p+".core")
	s = exploreTestMove(t, def, scope, s, p+".entry")
	beaconExpectBlocked(t, def, scope, s, "wax", 20100)
	ctx = testInstanceContext(s, scope, testInstancePermit(ActionExit, s.CurrentAreaID, "", "", "scene.exit.again"))
	if _, err := ExitDungeonInstance(def, scope, s, ctx); err != nil {
		t.Fatal(err)
	}
}

func TestShatteredBeaconWaxTradesInformationForSafety(t *testing.T) {
	def, scope, s := createExploreTestInstance(t, RealmShatteredBeacon)
	p := def.ID
	s = exploreTestApply(t, def, scope, s, p+".survey", 0).Snapshot
	s = exploreTestApply(t, def, scope, s, p+".outer", 600).Snapshot
	result := exploreTestApply(t, def, scope, s, p+".wax", 900)
	s = result.Snapshot
	if s.ExploreState.Pressure != 0 || len(result.Costs) != 1 || result.Costs[0].RefID != "sg-wax" {
		t.Fatal("wax must cost a real item and reduce exposure")
	}
	beaconExpectBlocked(t, def, scope, s, "wax", 1500)
	s = exploreTestApply(t, def, scope, s, p+".slow", 1500).Snapshot
	s = exploreTestMove(t, def, scope, s, p+".core")
	s = exploreTestApply(t, def, scope, s, p+".preserve", 3300).Snapshot
	s = exploreTestApply(t, def, scope, s, p+".return", 18600).Snapshot
	s = exploreTestMove(t, def, scope, s, p+".record")
	beaconExpectBlocked(t, def, scope, s, "copy", 19200)
	result = exploreTestApply(t, def, scope, s, p+".grind", 19200)
	if len(result.Rewards) != 1 || result.Rewards[0].Quantity != 3 || !hasExploreFact(result.Snapshot, p+".erased") {
		t.Fatal("wax must retain salvage at the cost of the original record")
	}
	beaconExpectBlocked(t, def, scope, result.Snapshot, "grind", 20400)
}

func TestShatteredBeaconSetbackLeavesFiniteRetreat(t *testing.T) {
	def, scope, s := createExploreTestInstance(t, RealmShatteredBeacon)
	p := def.ID
	s = exploreTestApply(t, def, scope, s, p+".outer", 0).Snapshot
	s = exploreTestApply(t, def, scope, s, p+".survey", 300).Snapshot
	s = exploreTestApply(t, def, scope, s, p+".slow", 900).Snapshot
	s = exploreTestMove(t, def, scope, s, p+".core")
	result := exploreTestApply(t, def, scope, s, p+".preserve", 2700)
	s = result.Snapshot
	if !s.ExploreState.Setback || s.ExploreState.ReturnDueSeconds != 0 || len(result.Rewards) != 0 {
		t.Fatal("overexposure must not preserve records or grant hidden rewards")
	}
	s = exploreTestMove(t, def, scope, s, p+".entry")
	beaconExpectBlocked(t, def, scope, s, "wax", 3600)
	result = exploreTestApply(t, def, scope, s, p+".residue", 3600)
	s = result.Snapshot
	if len(s.GrantedRewards) != 2 || result.Rewards[0].Quantity != 1 {
		t.Fatal("setback must retain wire and allow only one fragment")
	}
	beaconExpectBlocked(t, def, scope, s, "residue", 4200)
	ctx := testInstanceContext(s, scope, testInstancePermit(ActionExit, s.CurrentAreaID, "", "", "scene.exit"))
	if _, err := ExitDungeonInstance(def, scope, s, ctx); err != nil {
		t.Fatal(err)
	}
}

func TestShatteredBeaconStrippingClosesPreservation(t *testing.T) {
	def, scope, s := createExploreTestInstance(t, RealmShatteredBeacon)
	p := def.ID
	s = exploreTestApply(t, def, scope, s, p+".tool", 0).Snapshot
	s = exploreTestMove(t, def, scope, s, p+".core")
	result := exploreTestApply(t, def, scope, s, p+".strip", 600)
	s = result.Snapshot
	if len(result.Rewards) != 1 || result.Rewards[0].RefID != "sg-crystal" || result.Rewards[0].Quantity != 2 {
		t.Fatal("stripping must grant two real crystal shards")
	}
	beaconExpectBlocked(t, def, scope, s, "preserve", 900)
	beaconExpectBlocked(t, def, scope, s, "return", 20000)
	beaconExpectBlocked(t, def, scope, s, "strip", 20000)
}

func beaconExpectBlocked(t *testing.T, def DungeonDefinition, scope DungeonScope, s DungeonInstanceSnapshot, method string, now int64) {
	t.Helper()
	id := def.ID + "." + method
	before := cloneInstanceSnapshot(s)
	ctx := testInstanceContext(s, scope, testInstancePermit(ActionExplore, id, "", "", "scene.blocked"))
	if _, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: id, StoryTimeSeconds: now, Capabilities: []string{id}}); err == nil {
		t.Fatalf("unexpectedly allowed %s", method)
	}
	if !reflect.DeepEqual(before, s) {
		t.Fatal("rejected action changed progress")
	}
}
