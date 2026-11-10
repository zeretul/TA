package exploration

import (
	"reflect"
	"testing"
)

func createExploreTestInstance(t *testing.T, id string) (DungeonDefinition, DungeonScope, DungeonInstanceSnapshot) {
	t.Helper()
	def, ok := RealmDefinition(id)
	if !ok {
		t.Fatalf("missing realm %s", id)
	}
	scope := instanceScope()
	permit := testCreateInstancePermit(t, def, scope, "instance.explore", "scene.enter")
	permit.TargetID = def.EntryAreaIDs[0]
	result, err := CreateDungeonInstance(CreateDungeonInstanceInput{Definition: def, Scope: scope, InstanceID: "instance.explore", EntryAreaID: def.EntryAreaIDs[0], Permit: permit, Battle: BattleNone, Blocking: BlockingNone})
	if err != nil {
		t.Fatal(err)
	}
	return def, scope, result.Snapshot
}
func exploreTestApply(t *testing.T, def DungeonDefinition, scope DungeonScope, s DungeonInstanceSnapshot, id string, now int64) ExploreTransition {
	t.Helper()
	ctx := testInstanceContext(s, scope, testInstancePermit(ActionExplore, id, "", "", "scene.action"))
	result, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: id, StoryTimeSeconds: now, Capabilities: []string{id}})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func exploreTestMove(t *testing.T, def DungeonDefinition, scope DungeonScope, s DungeonInstanceSnapshot, to string) DungeonInstanceSnapshot {
	t.Helper()
	for _, c := range def.Connections {
		if c.FromAreaID == s.CurrentAreaID && c.ToAreaID == to || c.Bidirectional && c.ToAreaID == s.CurrentAreaID && c.FromAreaID == to {
			ctx := testInstanceContext(s, scope, testInstancePermit(ActionMove, c.ID, s.CurrentAreaID, to, "scene.move"))
			r, err := MoveDungeonInstance(def, scope, s, ctx, MoveDungeonInstanceInput{ConnectionID: c.ID, DestinationAreaID: to})
			if err != nil {
				t.Fatal(err)
			}
			return r.Snapshot
		}
	}
	t.Fatal("missing connection")
	return s
}
func TestExploreSurveyChoiceSetbackAndExit(t *testing.T) {
	def, scope, s := createExploreTestInstance(t, RealmMountainHerbCave)
	prefix := def.ID
	before := cloneInstanceSnapshot(s)
	view, ds := ProjectExploreView(def, scope, s, ExploreViewContext{})
	if len(ds) > 0 {
		t.Fatal(ds)
	}
	for _, a := range view.Approaches {
		if a.ID == prefix+".slow" || a.ID == prefix+".preserve" {
			t.Fatalf("secret method leaked: %+v", a)
		}
	}
	if !reflect.DeepEqual(before, s) {
		t.Fatal("projection mutated snapshot")
	}
	s = exploreTestApply(t, def, scope, s, prefix+".outer", 0).Snapshot
	if len(s.GrantedRewards) != 1 {
		t.Fatal("partial reward not granted")
	}
	s = exploreTestApply(t, def, scope, s, prefix+".survey", 300).Snapshot
	s = exploreTestApply(t, def, scope, s, prefix+".slow", 900).Snapshot
	s = exploreTestMove(t, def, scope, s, prefix+".core")
	result := exploreTestApply(t, def, scope, s, prefix+".preserve", 2100)
	s = result.Snapshot
	if !s.ExploreState.Setback || s.ExploreState.Pressure != 4 || s.ExploreState.ReturnDueSeconds != 0 || len(result.Rewards) != 0 || result.DurationSeconds != 1200 {
		t.Fatalf("wrong setback: %+v", result)
	}
	s = exploreTestMove(t, def, scope, s, prefix+".entry")
	s = exploreTestApply(t, def, scope, s, prefix+".residue", 3300).Snapshot
	ctx := testInstanceContext(s, scope, testInstancePermit(ActionExit, s.CurrentAreaID, "", "", "scene.exit"))
	out, err := ExitDungeonInstance(def, scope, s, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Snapshot.GrantedRewards) != 2 || !out.Snapshot.ExploreState.Setback {
		t.Fatal("setback erased partial results")
	}
}
func TestExploreReturnIsDerivedAndDoesNotFarm(t *testing.T) {
	def, scope, s := createExploreTestInstance(t, RealmMountainHerbCave)
	p := def.ID
	s = exploreTestApply(t, def, scope, s, p+".survey", 0).Snapshot
	s = exploreTestApply(t, def, scope, s, p+".slow", 600).Snapshot
	s = exploreTestMove(t, def, scope, s, p+".core")
	s = exploreTestApply(t, def, scope, s, p+".preserve", 1800).Snapshot
	if s.ExploreState.ReturnDueSeconds != 6600 {
		t.Fatalf("wrong due time %d", s.ExploreState.ReturnDueSeconds)
	}
	before := cloneInstanceSnapshot(s)
	for _, now := range []int64{6599, 6600, 9999} {
		v, ds := ProjectExploreView(def, scope, s, ExploreViewContext{StoryTimeSeconds: now, Capabilities: []string{p + ".return"}})
		if len(ds) > 0 {
			t.Fatal(ds)
		}
		if v.ReturnAvailable != (now >= 6600) {
			t.Fatal("wrong return availability")
		}
	}
	if !reflect.DeepEqual(before, s) {
		t.Fatal("clock read mutated state")
	}
	ctx := testInstanceContext(s, scope, testInstancePermit(ActionExplore, p+".strip", "", "", "scene.strip"))
	if _, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: p + ".strip", StoryTimeSeconds: 6600, Capabilities: []string{p + ".strip"}}); err == nil {
		t.Fatal("mutually exclusive disposal accepted")
	}
	s = exploreTestApply(t, def, scope, s, p+".return", 6600).Snapshot
	s = exploreTestMove(t, def, scope, s, p+".side")
	s = exploreTestApply(t, def, scope, s, p+".cache", 7200).Snapshot
	ctx = testInstanceContext(s, scope, testInstancePermit(ActionExplore, p+".cache", "", "", "scene.repeat"))
	if _, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: p + ".cache", StoryTimeSeconds: 7500, Capabilities: []string{p + ".cache"}}); err == nil {
		t.Fatal("repeat reward accepted")
	}
}
