package main

import (
	"encoding/json"
	"testing"

	"ta/internal/exploration"
)

func TestSuiteScenariosUseCoreResultsAndAreDeterministic(t *testing.T) {
	first := runSuite()
	second := runSuite()
	if !first.Valid || len(first.Diagnostics) != 0 {
		t.Fatalf("sequence checks failed: diagnostics=%v checks=%+v", first.Diagnostics, first.Checks)
	}
	if len(first.Checks) < 12 {
		t.Fatalf("scenario coverage is incomplete: %d checks", len(first.Checks))
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatal("same synthetic inputs produced different reports")
	}
	if first.Journey.SnapshotsEqual != true || first.Journey.SameTargetAdded != 0 || first.Isolation.CrossPermitRejected != true || first.Dungeon.FinalStatesEqual != true {
		t.Fatalf("report omitted core scenario outcomes: journey=%+v isolation=%+v dungeon=%+v", first.Journey, first.Isolation, first.Dungeon)
	}
}

func TestScenarioComparisonDetectsActualSnapshotMismatch(t *testing.T) {
	want := []exploration.JourneySnapshot{{JourneyID: "j", ElapsedMinutes: 5}}
	got := []exploration.JourneySnapshot{{JourneyID: "j", ElapsedMinutes: 5}}
	if err := sameJourneySnapshots(want, got); err != nil {
		t.Fatalf("identical core snapshots were rejected: %v", err)
	}
	got[0].ElapsedMinutes++
	if err := sameJourneySnapshots(want, got); err == nil {
		t.Fatal("comparison detector accepted an actually changed journey snapshot")
	}
}

func TestCheckpointComparisonOnlyNormalizesOmittedEmptyRewards(t *testing.T) {
	before := exploration.DungeonInstanceSnapshot{Revision: 4, GrantedRewards: []exploration.InstanceReward{}}
	after, err := jsonRoundTrip(before)
	if err != nil {
		t.Fatal(err)
	}
	if !sameDungeonCheckpoint(before, after) {
		t.Fatal("empty omitted rewards changed checkpoint facts")
	}
	before.GrantedRewards = []exploration.InstanceReward{{ID: "claimed", RefID: "item", Quantity: 1, SourceRef: "confirmed"}}
	if sameDungeonCheckpoint(before, after) {
		t.Fatal("lost reward was accepted")
	}
	before.GrantedRewards = nil
	after.Revision++
	if sameDungeonCheckpoint(before, after) {
		t.Fatal("changed revision was accepted")
	}
}
